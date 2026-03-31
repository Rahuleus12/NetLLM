/**
 * Netllm AI Provider - JavaScript SDK Client
 * ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
 *
 * A comprehensive JavaScript client library for the Netllm AI Provider API.
 * Supports synchronous and asynchronous inference, streaming (SSE),
 * model management, batch processing, and more.
 *
 * Works in both Node.js (>= 18) and modern browsers.
 *
 * Usage:
 *   import { NetllmClient } from './netllm-client.js';
 *
 *   const client = new NetllmClient({
 *     baseUrl: 'http://localhost:8080',
 *     apiKey: 'your-key',
 *   });
 *
 *   // Synchronous inference
 *   const response = await client.inference('model-id', {
 *     prompt: 'Hello, world!',
 *     maxTokens: 256,
 *   });
 *
 *   // Chat completion
 *   const chat = await client.chat('model-id', [
 *     { role: 'system', content: 'You are a helpful assistant.' },
 *     { role: 'user', content: 'What is AI?' },
 *   ]);
 *
 *   // Streaming
 *   for await (const chunk of client.inferenceStream('model-id', {
 *     prompt: 'Tell me a story',
 *   })) {
 *     process.stdout.write(chunk.delta);
 *   }
 *
 *   // Batch inference
 *   const batch = await client.batchInference('model-id', {
 *     prompts: ['Hi', 'Hello', 'Hey'],
 *   });
 *
 *   // Model management
 *   const models = await client.listModels();
 *   const model = await client.getModel('model-id');
 *   await client.registerModel({
 *     name: 'my-model',
 *     version: '1.0',
 *     format: 'gguf',
 *     sourceUrl: 'https://...',
 *   });
 */

// ---------------------------------------------------------------------------
// Version
// ---------------------------------------------------------------------------

export const VERSION = '1.0.0';

// ---------------------------------------------------------------------------
// Error Classes
// ---------------------------------------------------------------------------

/**
 * Base error for all Netllm SDK errors.
 */
export class NetllmError extends Error {
  constructor(message, { statusCode = null, errorType = null } = {}) {
    super(message);
    this.name = 'NetllmError';
    this.statusCode = statusCode;
    this.errorType = errorType;
  }
}

/** Raised on 401 responses. */
export class AuthenticationError extends NetllmError {
  constructor(message, opts = {}) {
    super(message, { statusCode: 401, ...opts });
    this.name = 'AuthenticationError';
  }
}

/** Raised on 403 responses. */
export class AuthorizationError extends NetllmError {
  constructor(message, opts = {}) {
    super(message, { statusCode: 403, ...opts });
    this.name = 'AuthorizationError';
  }
}

/** Raised on 404 responses. */
export class NotFoundError extends NetllmError {
  constructor(message, opts = {}) {
    super(message, { statusCode: 404, ...opts });
    this.name = 'NotFoundError';
  }
}

/** Raised on 429 responses. */
export class RateLimitError extends NetllmError {
  constructor(message, opts = {}) {
    super(message, { statusCode: 429, ...opts });
    this.name = 'RateLimitError';
  }
}

/** Raised on 400 responses. */
export class ValidationError extends NetllmError {
  constructor(message, opts = {}) {
    super(message, { statusCode: 400, ...opts });
    this.name = 'ValidationError';
  }
}

/** Raised when inference execution fails. */
export class InferenceError extends NetllmError {
  constructor(message, opts = {}) {
    super(message, opts);
    this.name = 'InferenceError';
  }
}

/** Raised on 5xx responses. */
export class ServerError extends NetllmError {
  constructor(message, opts = {}) {
    super(message, opts);
    this.name = 'ServerError';
  }
}

/** Raised when the connection to the server fails. */
export class ConnectionError extends NetllmError {
  constructor(message, opts = {}) {
    super(message, opts);
    this.name = 'ConnectionError';
  }
}

/** Raised when a request times out. */
export class TimeoutError extends NetllmError {
  constructor(message, opts = {}) {
    super(message, opts);
    this.name = 'TimeoutError';
  }
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function formatBytes(bytes) {
  if (bytes < 1024) return `${bytes} B`;
  const units = ['KB', 'MB', 'GB', 'TB'];
  let i = -1;
  let value = bytes;
  while (value >= 1024 && i < units.length - 1) {
    value /= 1024;
    i++;
  }
  return `${value.toFixed(1)} ${units[i]}`;
}

/**
 * Map an HTTP status code to the appropriate error class.
 */
function httpError(statusCode, message, errorType) {
  if (statusCode === 400) return new ValidationError(message, { statusCode, errorType });
  if (statusCode === 401) return new AuthenticationError(message, { statusCode, errorType });
  if (statusCode === 403) return new AuthorizationError(message, { statusCode, errorType });
  if (statusCode === 404) return new NotFoundError(message, { statusCode, errorType });
  if (statusCode === 429) return new RateLimitError(message, { statusCode, errorType });
  if (statusCode >= 500) return new ServerError(message, { statusCode, errorType });
  return new NetllmError(message, { statusCode, errorType });
}

/**
 * Parse an error response body (JSON or plain text).
 */
function parseErrorBody(body) {
  try {
    const parsed = JSON.parse(body);
    const errObj = parsed.error || parsed;
    return {
      message: typeof errObj === 'string' ? errObj : (errObj.message || body),
      errorType: typeof errObj === 'object' ? errObj.type : undefined,
    };
  } catch {
    return { message: body || 'Unknown error', errorType: undefined };
  }
}

/**
 * Build query string from an object, skipping null/undefined values.
 */
function buildQuery(params) {
  const parts = [];
  for (const [key, value] of Object.entries(params)) {
    if (value !== null && value !== undefined) {
      parts.push(`${encodeURIComponent(key)}=${encodeURIComponent(value)}`);
    }
  }
  return parts.length > 0 ? '?' + parts.join('&') : '';
}

// ---------------------------------------------------------------------------
// HTTP Transport
// ---------------------------------------------------------------------------

class HTTPTransport {
  constructor({
    baseUrl = 'http://localhost:8080',
    apiKey = null,
    jwtToken = null,
    timeout = 120000,
    userAgent = null,
    headers = {},
    fetchFn = null,
  }) {
    this.baseUrl = baseUrl.replace(/\/+$/, '');
    this.apiKey = apiKey;
    this.jwtToken = jwtToken;
    this.timeout = timeout;
    this.userAgent = userAgent || `netllm-js/${VERSION}`;
    this._extraHeaders = headers;

    // Allow injecting a custom fetch (useful for Node < 18 or testing)
    this._fetch = fetchFn || (typeof fetch !== 'undefined' ? fetch.bind(globalThis) : null);

    if (!this._fetch) {
      throw new NetllmError(
        'No fetch implementation found. Provide a fetchFn option or use Node.js >= 18.'
      );
    }
  }

  _buildHeaders(contentType = null) {
    const hdrs = {
      'User-Agent': this.userAgent,
      Accept: 'application/json',
      ...this._extraHeaders,
    };
    if (this.apiKey) hdrs['X-API-Key'] = this.apiKey;
    if (this.jwtToken) hdrs['Authorization'] = `Bearer ${this.jwtToken}`;
    if (contentType) hdrs['Content-Type'] = contentType;
    return hdrs;
  }

  async request(method, path, { body = null, params = null, timeout = null } = {}) {
    let url = `${this.baseUrl}${path}`;
    if (params) url += buildQuery(params);

    const headers = this._buildHeaders(body ? 'application/json' : null);
    const controller = new AbortController();
    const timeoutMs = timeout || this.timeout;
    const timer = setTimeout(() => controller.abort(), timeoutMs);

    try {
      const response = await this._fetch(url, {
        method,
        headers,
        body: body ? JSON.stringify(body) : null,
        signal: controller.signal,
      });

      clearTimeout(timer);
      const text = await response.text();

      if (!response.ok) {
        const { message, errorType } = parseErrorBody(text);
        throw httpError(response.status, message, errorType);
      }

      try {
        return JSON.parse(text);
      } catch {
        return { raw: text };
      }
    } catch (err) {
      clearTimeout(timer);
      if (err instanceof NetllmError) throw err;
      if (err.name === 'AbortError') {
        throw new TimeoutError(`Request to ${url} timed out after ${timeoutMs}ms`);
      }
      throw new ConnectionError(`Failed to connect to ${url}: ${err.message}`);
    }
  }

  /**
   * Perform a streaming SSE request and return an async generator of events.
   */
  async *requestSSE(method, path, { body = null, timeout = null } = {}) {
    const url = `${this.baseUrl}${path}`;
    const headers = this._buildHeaders(body ? 'application/json' : null);
    headers['Accept'] = 'text/event-stream';
    headers['Cache-Control'] = 'no-cache';

    const controller = new AbortController();
    const timeoutMs = timeout || this.timeout;
    const timer = setTimeout(() => controller.abort(), timeoutMs);

    let response;
    try {
      response = await this._fetch(url, {
        method,
        headers,
        body: body ? JSON.stringify(body) : null,
        signal: controller.signal,
      });
    } catch (err) {
      clearTimeout(timer);
      if (err.name === 'AbortError') {
        throw new TimeoutError(`Stream request to ${url} timed out after ${timeoutMs}ms`);
      }
      throw new ConnectionError(`Failed to connect to ${url}: ${err.message}`);
    }

    if (!response.ok) {
      clearTimeout(timer);
      const text = await response.text();
      const { message, errorType } = parseErrorBody(text);
      throw httpError(response.status, message, errorType);
    }

    const reader = response.body.getReader();
    const decoder = new TextDecoder();
    let buffer = '';
    let currentEvent = null;

    try {
      while (true) {
        const { done, value } = await reader.read();
        if (done) break;

        buffer += decoder.decode(value, { stream: true });

        while (buffer.includes('\n')) {
          const newlineIdx = buffer.indexOf('\n');
          const line = buffer.slice(0, newlineIdx).replace(/\r$/, '');
          buffer = buffer.slice(newlineIdx + 1);

          if (line.startsWith('event: ')) {
            currentEvent = line.slice(7).trim();
          } else if (line.startsWith('data: ')) {
            const dataStr = line.slice(6).trim();
            let eventData;
            try {
              eventData = JSON.parse(dataStr);
            } catch {
              eventData = { raw: dataStr };
            }

            if (currentEvent === 'error') {
              const msg = eventData?.error?.message || String(eventData);
              throw new InferenceError(msg);
            } else if (currentEvent === 'done') {
              return;
            } else if (['chunk', 'connected', 'usage'].includes(currentEvent)) {
              yield { event: currentEvent, data: eventData };
            } else {
              yield { event: currentEvent || 'message', data: eventData };
            }
            currentEvent = null;
          } else if (line === '') {
            currentEvent = null;
          }
        }
      }
    } finally {
      clearTimeout(timer);
      reader.releaseLock();
    }
  }
}

// ---------------------------------------------------------------------------
// Main Client
// ---------------------------------------------------------------------------

/**
 * Main client for the Netllm AI Provider API.
 *
 * Provides methods for inference, model management, system monitoring,
 * and batch processing. Works in both Node.js and modern browsers.
 *
 * @example
 * const client = new NetllmClient({
 *   baseUrl: 'http://localhost:8080',
 *   apiKey: 'sk-netllm-...',
 *   timeout: 120000,
 * });
 */
export class NetllmClient {
  /**
   * @param {Object} options
   * @param {string} [options.baseUrl='http://localhost:8080'] - API server base URL.
   * @param {string|null} [options.apiKey=null] - API key for authentication.
   * @param {string|null} [options.jwtToken=null] - JWT Bearer token for authentication.
   * @param {number} [options.timeout=120000] - Default request timeout in milliseconds.
   * @param {string|null} [options.userAgent=null] - Custom User-Agent header.
   * @param {Object} [options.headers={}] - Additional HTTP headers.
   * @param {Function|null} [options.fetchFn=null] - Custom fetch implementation.
   */
  constructor({
    baseUrl = 'http://localhost:8080',
    apiKey = null,
    jwtToken = null,
    timeout = 120000,
    userAgent = null,
    headers = {},
    fetchFn = null,
  } = {}) {
    this._transport = new HTTPTransport({
      baseUrl,
      apiKey,
      jwtToken,
      timeout,
      userAgent,
      headers,
      fetchFn,
    });
  }

  // -----------------------------------------------------------------------
  // Properties
  // -----------------------------------------------------------------------

  /** @returns {string} Base URL of the API server. */
  get baseUrl() { return this._transport.baseUrl; }

  /** @returns {string|null} Current API key. */
  get apiKey() { return this._transport.apiKey; }
  set apiKey(value) { this._transport.apiKey = value; }

  /** @returns {string|null} Current JWT token. */
  get jwtToken() { return this._transport.jwtToken; }
  set jwtToken(value) { this._transport.jwtToken = value; }

  // -----------------------------------------------------------------------
  // Health & System
  // -----------------------------------------------------------------------

  /**
   * Check the health of the API server.
   * @returns {Promise<Object>} Health response with status, uptime, and checks.
   */
  async health() {
    return this._transport.request('GET', '/health');
  }

  /**
   * Check if the API server is ready to accept traffic.
   * @returns {Promise<Object>} Readiness response with `ready` boolean.
   */
  async readiness() {
    return this._transport.request('GET', '/ready');
  }

  /**
   * Get version information from the API server.
   * @returns {Promise<Object>} Version info with version, buildTime, gitCommit.
   */
  async version() {
    return this._transport.request('GET', '/version');
  }

  /**
   * Lightweight liveness check.
   * @returns {Promise<Object>} Object with `message` ("pong") and `timestamp`.
   */
  async ping() {
    return this._transport.request('GET', '/ping');
  }

  /**
   * Get detailed system information (Go runtime, memory, build info).
   * @returns {Promise<Object>} Comprehensive system information.
   */
  async systemInfo() {
    return this._transport.request('GET', '/api/v1/system/info');
  }

  /**
   * Get a full diagnostics snapshot for troubleshooting.
   * @returns {Promise<Object>} Version, system, health, and config information.
   */
  async diagnostics() {
    return this._transport.request('GET', '/api/v1/system/diagnostics');
  }

  /**
   * Get system and runtime metrics.
   * @returns {Promise<Object>} Memory, runtime, and request metrics.
   */
  async systemMetrics() {
    return this._transport.request('GET', '/api/v1/system/metrics');
  }

  // -----------------------------------------------------------------------
  // Inference
  // -----------------------------------------------------------------------

  /**
   * Run synchronous inference on the specified model.
   *
   * Provide either `prompt` (string) or `messages` (chat format).
   *
   * @param {string} modelId - ID of the model to use.
   * @param {Object} [options={}] - Inference options.
   * @param {string} [options.prompt] - Text prompt for completion.
   * @param {Array<Object>} [options.messages] - Chat messages for conversation inference.
   * @param {number} [options.maxTokens=512] - Maximum tokens to generate.
   * @param {number} [options.temperature=0.7] - Sampling temperature (0.0 - 2.0).
   * @param {number} [options.topP=0.9] - Nucleus sampling threshold.
   * @param {number} [options.topK=40] - Top-K sampling parameter.
   * @param {string[]} [options.stop] - Stop sequences.
   * @param {number} [options.frequencyPenalty=0.0] - Frequency penalty.
   * @param {number} [options.presencePenalty=0.0] - Presence penalty.
   * @param {string} [options.user] - User identifier for tracking.
   * @param {Object} [options.metadata] - Additional metadata.
   * @param {number} [options.timeout] - Request-specific timeout in ms.
   * @returns {Promise<Object>} Inference response with content, token usage, etc.
   *
   * @example
   * const response = await client.inference('llama-3-8b', {
   *   prompt: 'Explain quantum computing in simple terms.',
   *   maxTokens: 256,
   *   temperature: 0.7,
   * });
   * console.log(response.content);
   */
  async inference(modelId, options = {}) {
    const body = this._buildInferenceBody(options);
    return this._transport.request('POST', `/api/v1/inference/${modelId}`, {
      body,
      timeout: options.timeout,
    });
  }

  /**
   * Run chat-style inference with a list of messages.
   *
   * Convenience method wrapping `inference()` with a messages-first API.
   *
   * @param {string} modelId - ID of the model to use.
   * @param {Array<Object>} messages - List of chat messages.
   * @param {Object} [options={}] - Additional inference options (maxTokens, temperature, etc.).
   * @returns {Promise<Object>} Inference response.
   *
   * @example
   * const response = await client.chat('llama-3-8b', [
   *   { role: 'system', content: 'You are a helpful assistant.' },
   *   { role: 'user', content: 'What is Python?' },
   * ], { maxTokens: 200 });
   */
  async chat(modelId, messages, options = {}) {
    return this.inference(modelId, { ...options, messages });
  }

  /**
   * Submit an asynchronous inference request.
   *
   * Returns immediately with a request ID. Use `getInferenceStatus()` to poll.
   *
   * @param {string} modelId - ID of the model.
   * @param {Object} [options={}] - Inference options (same as `inference()`).
   * @returns {Promise<Object>} Object with `id`, `model_id`, `status`, `created_at`.
   *
   * @example
   * const submission = await client.inferenceAsync('llama-3-8b', {
   *   prompt: 'Write a poem about code.',
   * });
   * // Poll for result
   * const status = await client.waitForInference(submission.id);
   */
  async inferenceAsync(modelId, options = {}) {
    const body = this._buildInferenceBody(options);
    return this._transport.request('POST', `/api/v1/inference/${modelId}/async`, { body });
  }

  /**
   * Get the status of an async inference request.
   *
   * @param {string} requestId - The ID returned by `inferenceAsync()`.
   * @returns {Promise<Object>} Request status with `status`, `error`, timestamps.
   */
  async getInferenceStatus(requestId) {
    return this._transport.request('GET', `/api/v1/inference/requests/${requestId}`);
  }

  /**
   * Cancel a pending or running inference request.
   *
   * @param {string} requestId - The request ID to cancel.
   * @returns {Promise<Object>} Object confirming cancellation with `id` and `status`.
   */
  async cancelInference(requestId) {
    return this._transport.request('DELETE', `/api/v1/inference/requests/${requestId}`);
  }

  /**
   * Run streaming inference using Server-Sent Events (SSE).
   *
   * Yields chunk objects as they arrive from the server.
   *
   * @param {string} modelId - ID of the model.
   * @param {Object} [options={}] - Inference options (same as `inference()`).
   * @yields {Object} Stream chunks with `delta`, `content`, `output_tokens`, etc.
   *
   * @example
   * for await (const chunk of client.inferenceStream('llama-3-8b', {
   *   prompt: 'Tell me a story about a robot.',
   * })) {
   *   process.stdout.write(chunk.delta || chunk.content || '');
   * }
   * console.log(); // trailing newline
   */
  async *inferenceStream(modelId, options = {}) {
    const body = this._buildInferenceBody(options);
    for await (const event of this._transport.requestSSE(
      'POST',
      `/api/v1/inference/${modelId}/stream`,
      { body, timeout: options.timeout }
    )) {
      if (event.event === 'chunk') {
        yield event.data;
      }
      // 'connected' and 'usage' events are silently consumed
    }
  }

  /**
   * Submit a batch of inference requests for parallel processing.
   *
   * Provide either `prompts` (array of strings) or `messageSets` (array of message arrays).
   *
   * @param {string} modelId - ID of the model.
   * @param {Object} options - Batch options.
   * @param {string[]} [options.prompts] - Array of text prompts.
   * @param {Array<Array<Object>>} [options.messageSets] - Array of message arrays.
   * @param {number} [options.maxTokens=512] - Max tokens per request.
   * @param {number} [options.temperature=0.7] - Sampling temperature.
   * @param {number} [options.topP=0.9] - Nucleus sampling.
   * @param {number} [options.topK=40] - Top-K sampling.
   * @param {string[]} [options.stop] - Stop sequences.
   * @param {number} [options.frequencyPenalty=0.0] - Frequency penalty.
   * @param {number} [options.presencePenalty=0.0] - Presence penalty.
   * @param {number} [options.priority=5] - Batch priority (1=low, 5=normal, 10=high, 15=critical).
   * @param {Object} [options.metadata] - Additional metadata.
   * @param {number} [options.timeout] - Request timeout in ms.
   * @returns {Promise<Object>} Batch response with results for each input.
   *
   * @example
   * const result = await client.batchInference('llama-3-8b', {
   *   prompts: ['What is AI?', 'What is ML?', 'What is DL?'],
   * });
   * for (const r of result.results) {
   *   if (r.success) console.log(r.response.content);
   * }
   */
  async batchInference(modelId, options = {}) {
    const {
      prompts,
      messageSets,
      priority = 5,
      metadata,
      timeout,
      ...inferenceOpts
    } = options;

    const requestsList = [];

    if (prompts && prompts.length > 0) {
      for (const p of prompts) {
        requestsList.push(this._buildInferenceBody({ ...inferenceOpts, prompt: p }));
      }
    } else if (messageSets && messageSets.length > 0) {
      for (const msgs of messageSets) {
        requestsList.push(this._buildInferenceBody({ ...inferenceOpts, messages: msgs }));
      }
    } else {
      throw new ValidationError('Either prompts or messageSets must be provided');
    }

    const body = {
      model: modelId,
      requests: requestsList,
      priority,
    };
    if (metadata) body.metadata = metadata;

    return this._transport.request('POST', '/api/v1/inference/batch', { body, timeout });
  }

  /**
   * Get inference executor statistics.
   *
   * @returns {Promise<Object>} Stats with throughput, latency, and queue metrics.
   */
  async getInferenceStats() {
    return this._transport.request('GET', '/api/v1/inference/stats');
  }

  /**
   * List all currently active inference requests.
   *
   * @returns {Promise<Array>} Array of request status objects.
   */
  async listActiveRequests() {
    const data = await this._transport.request('GET', '/api/v1/inference/requests');
    return data.data || [];
  }

  // -----------------------------------------------------------------------
  // Model Management
  // -----------------------------------------------------------------------

  /**
   * List registered models with filtering and pagination.
   *
   * @param {Object} [options={}] - Filter and pagination options.
   * @param {number} [options.page=1] - Page number (1-indexed).
   * @param {number} [options.perPage=20] - Items per page (max 100).
   * @param {string} [options.status] - Filter by model status.
   * @param {string} [options.format] - Filter by model format.
   * @param {string} [options.search] - Search term for name/description.
   * @param {string} [options.sortBy='created_at'] - Sort field.
   * @param {string} [options.sortOrder='DESC'] - 'ASC' or 'DESC'.
   * @returns {Promise<Object>} Paginated model list with `data`, `total`, `page`.
   */
  async listModels(options = {}) {
    const params = {
      page: options.page || 1,
      per_page: options.perPage || 20,
      sort_by: options.sortBy || 'created_at',
      sort_order: options.sortOrder || 'DESC',
      status: options.status || undefined,
      format: options.format || undefined,
      search: options.search || undefined,
    };
    return this._transport.request('GET', '/api/v1/models', { params });
  }

  /**
   * Get details of a specific model.
   *
   * @param {string} modelId - The model ID.
   * @returns {Promise<Object>} Model details.
   */
  async getModel(modelId) {
    return this._transport.request('GET', `/api/v1/models/${modelId}`);
  }

  /**
   * Register a new model in the system.
   *
   * @param {Object} options - Model registration options.
   * @param {string} options.name - Model name.
   * @param {string} options.version - Semantic version string.
   * @param {string} options.format - Model format ('gguf', 'onnx', 'pytorch', 'safetensors').
   * @param {string} [options.sourceUrl] - URL to download the model from.
   * @param {string} [options.description] - Human-readable description.
   * @param {string} [options.checksum] - Expected checksum for validation.
   * @param {Object} [options.metadata] - Additional metadata.
   * @returns {Promise<Object>} Newly registered model info.
   *
   * @example
   * const model = await client.registerModel({
   *   name: 'llama-3-8b-instruct',
   *   version: '1.0.0',
   *   format: 'gguf',
   *   sourceUrl: 'https://huggingface.co/.../model.gguf',
   *   description: 'Llama 3 8B Instruct',
   * });
   */
  async registerModel(options) {
    const body = {
      name: options.name,
      version: options.version,
      format: options.format,
    };
    if (options.sourceUrl) body.source_url = options.sourceUrl;
    if (options.description) body.description = options.description;
    if (options.checksum) body.checksum = options.checksum;
    if (options.metadata) body.metadata = options.metadata;

    return this._transport.request('POST', '/api/v1/models', { body });
  }

  /**
   * Update a model's configuration.
   *
   * @param {string} modelId - The model ID.
   * @param {Object} [options={}] - Fields to update.
   * @param {string} [options.description] - Updated description.
   * @param {Object} [options.metadata] - Updated metadata.
   * @param {Object} [options.config] - Updated runtime config.
   * @returns {Promise<Object>} Updated model info.
   */
  async updateModel(modelId, options = {}) {
    const body = {};
    if (options.description !== undefined) body.description = options.description;
    if (options.metadata !== undefined) body.metadata = options.metadata;
    if (options.config !== undefined) body.config = options.config;

    return this._transport.request('PUT', `/api/v1/models/${modelId}`, { body });
  }

  /**
   * Delete a model from the system.
   *
   * @param {string} modelId - The model ID.
   * @param {boolean} [force=false] - Force deletion even if model is active.
   */
  async deleteModel(modelId, force = false) {
    const params = force ? { force: 'true' } : null;
    await this._transport.request('DELETE', `/api/v1/models/${modelId}`, { params });
  }

  /**
   * Start downloading a model's files from its source URL.
   *
   * @param {string} modelId - The model ID.
   * @returns {Promise<Object>} Object confirming download started.
   */
  async downloadModel(modelId) {
    return this._transport.request('POST', `/api/v1/models/${modelId}/download`);
  }

  /**
   * Get the download progress for a model.
   *
   * @param {string} modelId - The model ID.
   * @returns {Promise<Object>} Progress details (percentage, bytes downloaded, etc.).
   */
  async getDownloadProgress(modelId) {
    return this._transport.request('GET', `/api/v1/models/${modelId}/download`);
  }

  /**
   * Cancel a model download in progress.
   *
   * @param {string} modelId - The model ID.
   * @returns {Promise<Object>} Object confirming cancellation.
   */
  async cancelDownload(modelId) {
    return this._transport.request('DELETE', `/api/v1/models/${modelId}/download`);
  }

  /**
   * Validate a model's integrity and configuration.
   *
   * @param {string} modelId - The model ID.
   * @returns {Promise<Object>} Validation result.
   */
  async validateModel(modelId) {
    return this._transport.request('POST', `/api/v1/models/${modelId}/validate`);
  }

  /**
   * Activate a model for inference.
   *
   * @param {string} modelId - The model ID.
   * @returns {Promise<Object>} Object confirming activation.
   */
  async activateModel(modelId) {
    return this._transport.request('POST', `/api/v1/models/${modelId}/activate`);
  }

  /**
   * Deactivate a model (stop serving inference).
   *
   * @param {string} modelId - The model ID.
   * @returns {Promise<Object>} Object confirming deactivation.
   */
  async deactivateModel(modelId) {
    return this._transport.request('POST', `/api/v1/models/${modelId}/deactivate`);
  }

  /**
   * Get a model's runtime configuration.
   *
   * @param {string} modelId - The model ID.
   * @returns {Promise<Object>} Model configuration.
   */
  async getModelConfig(modelId) {
    return this._transport.request('GET', `/api/v1/models/${modelId}/config`);
  }

  /**
   * Update a model's runtime configuration.
   *
   * @param {string} modelId - The model ID.
   * @param {Object} config - New configuration object.
   * @returns {Promise<Object>} Updated configuration.
   */
  async updateModelConfig(modelId, config) {
    return this._transport.request('PUT', `/api/v1/models/${modelId}/config`, { body: config });
  }

  /**
   * List all versions of a model.
   *
   * @param {string} modelId - The model ID.
   * @returns {Promise<Object>} Object with `data` array of versions.
   */
  async listModelVersions(modelId) {
    return this._transport.request('GET', `/api/v1/models/${modelId}/versions`);
  }

  /**
   * Get aggregate statistics for all models.
   *
   * @returns {Promise<Object>} Model statistics.
   */
  async getModelStats() {
    return this._transport.request('GET', '/api/v1/models/stats');
  }

  /**
   * Search models by name or description.
   *
   * @param {string} query - Search query string.
   * @returns {Promise<Array>} Array of matching model objects.
   */
  async searchModels(query) {
    const data = await this._transport.request('GET', '/api/v1/models/search', {
      params: { q: query },
    });
    return data.data || [];
  }

  // -----------------------------------------------------------------------
  // Convenience Methods
  // -----------------------------------------------------------------------

  /**
   * Poll an async inference request until completion.
   *
   * @param {string} requestId - The async request ID.
   * @param {Object} [options={}] - Polling options.
   * @param {number} [options.pollInterval=1000] - Milliseconds between polls.
   * @param {number} [options.maxWait=300000] - Maximum wait time in milliseconds.
   * @returns {Promise<Object>} Final request status.
   * @throws {TimeoutError} If the request doesn't complete within maxWait.
   */
  async waitForInference(requestId, { pollInterval = 1000, maxWait = 300000 } = {}) {
    const start = Date.now();
    const terminalStates = ['completed', 'complete', 'failed', 'cancelled', 'error'];

    while (true) {
      const status = await this.getInferenceStatus(requestId);
      if (terminalStates.includes(status.status || status.Status)) {
        return status;
      }
      const elapsed = Date.now() - start;
      if (elapsed >= maxWait) {
        throw new TimeoutError(
          `Inference request ${requestId} did not complete within ${maxWait}ms ` +
          `(current status: ${status.status || status.Status})`
        );
      }
      await new Promise((resolve) => setTimeout(resolve, pollInterval));
    }
  }

  /**
   * Simple text generation that returns just the generated string.
   *
   * Convenience wrapper around `inference()` that returns only the content.
   *
   * @param {string} modelId - Model ID.
   * @param {string} prompt - Text prompt.
   * @param {Object} [options={}] - Additional inference options.
   * @returns {Promise<string>} Generated text string.
   */
  async generate(modelId, prompt, options = {}) {
    const response = await this.inference(modelId, { ...options, prompt });
    return response.content || '';
  }

  // -----------------------------------------------------------------------
  // Internal Helpers
  // -----------------------------------------------------------------------

  /**
   * Build the request body for an inference request.
   * @private
   */
  _buildInferenceBody(options = {}) {
    const body = {
      max_tokens: options.maxTokens || 512,
      temperature: options.temperature ?? 0.7,
      top_p: options.topP ?? 0.9,
      top_k: options.topK ?? 40,
      frequency_penalty: options.frequencyPenalty ?? 0.0,
      presence_penalty: options.presencePenalty ?? 0.0,
    };

    if (options.prompt) body.prompt = options.prompt;
    if (options.messages) body.messages = options.messages;
    if (options.stop) body.stop = options.stop;
    if (options.user) body.user = options.user;
    if (options.metadata) body.metadata = options.metadata;

    return body;
  }
}

// ---------------------------------------------------------------------------
// Client Builder
// ---------------------------------------------------------------------------

/**
 * Builder for constructing a NetllmClient with optional parameters.
 *
 * @example
 * const client = new NetllmClientBuilder()
 *   .baseUrl('http://localhost:8080')
 *   .apiKey('sk-netllm-...')
 *   .timeout(120000)
 *   .header('X-Custom-Header', 'value')
 *   .build();
 */
export class NetllmClientBuilder {
  constructor() {
    this._baseUrl = 'http://localhost:8080';
    this._apiKey = null;
    this._jwtToken = null;
    this._timeout = 120000;
    this._userAgent = null;
    this._headers = {};
    this._fetchFn = null;
  }

  /** Set the base URL. */
  baseUrl(url) { this._baseUrl = url; return this; }

  /** Set the API key. */
  apiKey(key) { this._apiKey = key; return this; }

  /** Set the JWT Bearer token. */
  jwtToken(token) { this._jwtToken = token; return this; }

  /** Set the default request timeout in milliseconds. */
  timeout(ms) { this._timeout = ms; return this; }

  /** Set a custom User-Agent header. */
  userAgent(ua) { this._userAgent = ua; return this; }

  /** Add a custom header. */
  header(key, value) { this._headers[key] = value; return this; }

  /** Add multiple custom headers. */
  headers(hdrs) { Object.assign(this._headers, hdrs); return this; }

  /** Provide a custom fetch implementation. */
  fetchFn(fn) { this._fetchFn = fn; return this; }

  /** Build and return the configured NetllmClient. */
  build() {
    return new NetllmClient({
      baseUrl: this._baseUrl,
      apiKey: this._apiKey,
      jwtToken: this._jwtToken,
      timeout: this._timeout,
      userAgent: this._userAgent,
      headers: this._headers,
      fetchFn: this._fetchFn,
    });
  }
}

// ---------------------------------------------------------------------------
// Package-level convenience
// ---------------------------------------------------------------------------

/**
 * Create a new NetllmClient instance.
 *
 * @param {Object} [options={}] - Client options (same as NetllmClient constructor).
 * @returns {NetllmClient} Configured client instance.
 */
export function createClient(options = {}) {
  return new NetllmClient(options);
}

// Default export for CommonJS compatibility
export default NetllmClient;
