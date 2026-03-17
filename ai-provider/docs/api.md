# AI Provider API Documentation

## Overview

The AI Provider API is a RESTful web service that enables you to manage and run AI models in a containerized environment. The API provides endpoints for model management, inference, configuration, and monitoring.

- **Base URL**: `http://localhost:8080/api/v1`
- **Content-Type**: `application/json`
- **API Version**: v1

## Table of Contents

1. [Authentication](#authentication)
2. [Common Patterns](#common-patterns)
3. [Error Handling](#error-handling)
4. [Rate Limiting](#rate-limiting)
5. [Health & Status Endpoints](#health--status-endpoints)
6. [Model Management](#model-management)
7. [Inference](#inference)
8. [Configuration](#configuration)
9. [Monitoring](#monitoring)
10. [WebSocket Streaming](#websocket-streaming)
11. [Examples](#examples)

---

## Authentication

### API Key Authentication

Most API endpoints require authentication using an API key. Include your API key in the request header:

```http
X-API-Key: your-api-key-here
```

### JWT Authentication (Optional)

For enhanced security, JWT tokens can be used:

```http
Authorization: Bearer your-jwt-token-here
```

### Obtaining an API Key

API keys can be created through the CLI or directly in the database:

```bash
ai-provider auth create-key --name "my-app" --permissions "read,write"
```

---

## Common Patterns

### Pagination

List endpoints support pagination using query parameters:

```http
GET /api/v1/models?page=1&per_page=20&sort=created_at&order=desc
```

**Pagination Response Format:**
```json
{
  "data": [...],
  "pagination": {
    "page": 1,
    "per_page": 20,
    "total_pages": 5,
    "total_count": 98,
    "has_next": true,
    "has_prev": false
  }
}
```

### Filtering

Filter results using query parameters:

```http
GET /api/v1/models?status=active&format=gguf
```

### Timestamps

All timestamps are in ISO 8601 format (UTC):
```
2024-01-15T10:30:00Z
```

---

## Error Handling

### Error Response Format

All errors follow a consistent format:

```json
{
  "error": {
    "code": "MODEL_NOT_FOUND",
    "message": "Model with ID '123e4567-e89b-12d3-a456-426614174000' not found",
    "details": {
      "model_id": "123e4567-e89b-12d3-a456-426614174000"
    },
    "request_id": "req_abc123def456"
  }
}
```

### HTTP Status Codes

| Status Code | Description |
|------------|-------------|
| 200 | Success |
| 201 | Created |
| 204 | No Content (successful deletion) |
| 400 | Bad Request - Invalid parameters |
| 401 | Unauthorized - Invalid or missing API key |
| 403 | Forbidden - Insufficient permissions |
| 404 | Not Found - Resource doesn't exist |
| 409 | Conflict - Resource already exists |
| 422 | Unprocessable Entity - Validation error |
| 429 | Too Many Requests - Rate limit exceeded |
| 500 | Internal Server Error |
| 503 | Service Unavailable - System overloaded |

### Error Codes

| Code | Description |
|------|-------------|
| `INVALID_PARAMETERS` | Request parameters are invalid |
| `UNAUTHORIZED` | Authentication required |
| `FORBIDDEN` | Permission denied |
| `NOT_FOUND` | Resource not found |
| `ALREADY_EXISTS` | Resource already exists |
| `VALIDATION_ERROR` | Data validation failed |
| `RATE_LIMIT_EXCEEDED` | Too many requests |
| `MODEL_NOT_FOUND` | Specified model doesn't exist |
| `MODEL_LOAD_FAILED` | Failed to load model |
| `INFERENCE_ERROR` | Error during inference |
| `CONTAINER_ERROR` | Container runtime error |
| `RESOURCE_EXHAUSTED` | System resources insufficient |

---

## Rate Limiting

API requests are rate-limited to ensure fair usage:

- **Default**: 100 requests per minute
- **Authenticated**: 1000 requests per minute
- **Burst**: Up to 20 requests per second

Rate limit headers are included in responses:

```http
X-RateLimit-Limit: 100
X-RateLimit-Remaining: 95
X-RateLimit-Reset: 1642234560
```

---

## Health & Status Endpoints

### Health Check

Check if the service is running.

```http
GET /health
```

**Response:**
```json
{
  "status": "healthy",
  "timestamp": "2024-01-15T10:30:00Z",
  "version": "1.0.0"
}
```

### Readiness Check

Check if the service is ready to accept requests.

```http
GET /ready
```

**Response:**
```json
{
  "ready": true,
  "timestamp": "2024-01-15T10:30:00Z",
  "checks": {
    "database": "healthy",
    "cache": "healthy",
    "models": "healthy"
  }
}
```

### Version Information

Get version and build information.

```http
GET /version
```

**Response:**
```json
{
  "version": "1.0.0",
  "build_time": "2024-01-15T08:00:00Z",
  "git_commit": "abc123d"
}
```

---

## Model Management

### List Models

Retrieve a list of all registered models.

```http
GET /api/v1/models
```

**Query Parameters:**
- `page` (int): Page number (default: 1)
- `per_page` (int): Items per page (default: 20, max: 100)
- `status` (string): Filter by status (active, inactive, loading, error)
- `format` (string): Filter by format (gguf, onnx, pytorch)
- `search` (string): Search by name or description

**Response:**
```json
{
  "data": [
    {
      "id": "123e4567-e89b-12d3-a456-426614174000",
      "name": "llama-2-7b",
      "version": "1.0.0",
      "format": "gguf",
      "status": "active",
      "instances": 2,
      "context_length": 4096,
      "max_tokens": 2048,
      "created_at": "2024-01-15T10:30:00Z",
      "updated_at": "2024-01-15T10:30:00Z"
    }
  ],
  "pagination": {
    "page": 1,
    "per_page": 20,
    "total_pages": 1,
    "total_count": 1
  }
}
```

### Get Model Details

Retrieve detailed information about a specific model.

```http
GET /api/v1/models/{model_id}
```

**Path Parameters:**
- `model_id` (string): Unique model identifier

**Response:**
```json
{
  "id": "123e4567-e89b-12d3-a456-426614174000",
  "name": "llama-2-7b",
  "version": "1.0.0",
  "description": "Llama 2 7B parameter model",
  "format": "gguf",
  "source": "https://huggingface.co/TheBloke/Llama-2-7B-GGUF",
  "checksum": "sha256:abc123...",
  "status": "active",
  "instances": 2,
  "config": {
    "context_length": 4096,
    "temperature": 0.7,
    "max_tokens": 2048,
    "top_p": 0.9,
    "top_k": 40
  },
  "requirements": {
    "ram_min": 8192,
    "gpu_memory": 4096,
    "cpu_cores": 4
  },
  "metrics": {
    "total_requests": 15234,
    "avg_latency_ms": 245,
    "last_used": "2024-01-15T10:25:00Z"
  },
  "created_at": "2024-01-15T10:30:00Z",
  "updated_at": "2024-01-15T10:30:00Z"
}
```

### Register Model

Register a new model in the system.

```http
POST /api/v1/models
```

**Request Body:**
```json
{
  "name": "llama-2-7b",
  "version": "1.0.0",
  "description": "Llama 2 7B parameter model",
  "format": "gguf",
  "source": "https://huggingface.co/TheBloke/Llama-2-7B-GGUF/llama-2-7b.Q4_K_M.gguf",
  "checksum": "sha256:abc123...",
  "config": {
    "context_length": 4096,
    "temperature": 0.7,
    "max_tokens": 2048,
    "top_p": 0.9,
    "top_k": 40
  },
  "requirements": {
    "ram_min": 8192,
    "gpu_memory": 4096,
    "cpu_cores": 4
  },
  "auto_start": true,
  "instances": 1
}
```

**Response:** `201 Created`
```json
{
  "id": "123e4567-e89b-12d3-a456-426614174000",
  "name": "llama-2-7b",
  "version": "1.0.0",
  "status": "loading",
  "message": "Model registration initiated. Download in progress.",
  "created_at": "2024-01-15T10:30:00Z"
}
```

### Update Model Configuration

Update a model's configuration.

```http
PUT /api/v1/models/{model_id}
```

**Request Body:**
```json
{
  "config": {
    "temperature": 0.8,
    "max_tokens": 1024,
    "top_p": 0.95
  },
  "instances": 3
}
```

**Response:**
```json
{
  "id": "123e4567-e89b-12d3-a456-426614174000",
  "name": "llama-2-7b",
  "status": "active",
  "message": "Model configuration updated successfully",
  "updated_at": "2024-01-15T10:35:00Z"
}
```

### Delete Model

Remove a model from the system.

```http
DELETE /api/v1/models/{model_id}
```

**Query Parameters:**
- `force` (boolean): Force deletion even if instances are running (default: false)
- `keep_files` (boolean): Keep downloaded model files (default: false)

**Response:** `204 No Content`

### Start Model

Start model instances.

```http
POST /api/v1/models/{model_id}/start
```

**Request Body:**
```json
{
  "instances": 2,
  "gpu_devices": [0, 1],
  "resource_limits": {
    "cpu": 2,
    "memory": "4GB"
  }
}
```

**Response:**
```json
{
  "id": "123e4567-e89b-12d3-a456-426614174000",
  "status": "starting",
  "instances_requested": 2,
  "message": "Model instances starting"
}
```

### Stop Model

Stop model instances.

```http
POST /api/v1/models/{model_id}/stop
```

**Request Body:**
```json
{
  "force": false,
  "timeout": 30
}
```

**Response:**
```json
{
  "id": "123e4567-e89b-12d3-a456-426614174000",
  "status": "stopping",
  "message": "Model instances stopping gracefully"
}
```

---

## Inference

### Run Inference

Execute inference on a model.

```http
POST /api/v1/inference/{model_id}
```

**Path Parameters:**
- `model_id` (string): Model ID or name (e.g., "llama-2-7b" or "123e4567-e89b-12d3-a456-426614174000")

**Request Body:**
```json
{
  "prompt": "Explain quantum computing in simple terms.",
  "parameters": {
    "temperature": 0.7,
    "max_tokens": 500,
    "top_p": 0.9,
    "top_k": 40,
    "stop": ["\n", "Human:"],
    "frequency_penalty": 0.0,
    "presence_penalty": 0.0
  },
  "stream": false,
  "metadata": {
    "user_id": "user123",
    "session_id": "session456"
  }
}
```

**Response:**
```json
{
  "id": "inf_abc123def456",
  "model_id": "123e4567-e89b-12d3-a456-426614174000",
  "model_name": "llama-2-7b",
  "result": {
    "text": "Quantum computing is like having a super-powered calculator...",
    "tokens": 156,
    "finish_reason": "stop"
  },
  "usage": {
    "prompt_tokens": 12,
    "completion_tokens": 156,
    "total_tokens": 168
  },
  "performance": {
    "latency_ms": 1234,
    "tokens_per_second": 126.4
  },
  "created_at": "2024-01-15T10:40:00Z"
}
```

### Batch Inference

Run inference on multiple prompts in a single request.

```http
POST /api/v1/inference/batch
```

**Request Body:**
```json
{
  "model_id": "llama-2-7b",
  "requests": [
    {
      "prompt": "What is machine learning?",
      "parameters": {
        "max_tokens": 100,
        "temperature": 0.7
      }
    },
    {
      "prompt": "Explain neural networks.",
      "parameters": {
        "max_tokens": 100,
        "temperature": 0.7
      }
    }
  ],
  "parallel": true
}
```

**Response:**
```json
{
  "batch_id": "batch_xyz789",
  "model_id": "123e4567-e89b-12d3-a456-426614174000",
  "status": "completed",
  "results": [
    {
      "id": "inf_abc123",
      "prompt": "What is machine learning?",
      "result": {
        "text": "Machine learning is a subset of artificial intelligence...",
        "tokens": 95,
        "finish_reason": "stop"
      },
      "usage": {
        "prompt_tokens": 5,
        "completion_tokens": 95,
        "total_tokens": 100
      }
    },
    {
      "id": "inf_def456",
      "prompt": "Explain neural networks.",
      "result": {
        "text": "Neural networks are computing systems inspired by biological neural networks...",
        "tokens": 88,
        "finish_reason": "stop"
      },
      "usage": {
        "prompt_tokens": 4,
        "completion_tokens": 88,
        "total_tokens": 92
      }
    }
  ],
  "summary": {
    "total_requests": 2,
    "successful": 2,
    "failed": 0,
    "total_tokens": 192,
    "total_time_ms": 2345
  },
  "created_at": "2024-01-15T10:45:00Z"
}
```

### Chat Completion

Run chat-based inference with conversation history.

```http
POST /api/v1/inference/{model_id}/chat
```

**Request Body:**
```json
{
  "messages": [
    {
      "role": "system",
      "content": "You are a helpful AI assistant."
    },
    {
      "role": "user",
      "content": "What is the capital of France?"
    },
    {
      "role": "assistant",
      "content": "The capital of France is Paris."
    },
    {
      "role": "user",
      "content": "What is its population?"
    }
  ],
  "parameters": {
    "temperature": 0.7,
    "max_tokens": 200
  }
}
```

**Response:**
```json
{
  "id": "chat_abc123",
  "model_id": "123e4567-e89b-12d3-a456-426614174000",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "Paris has a population of approximately 2.1 million people in the city proper..."
      },
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 45,
    "completion_tokens": 78,
    "total_tokens": 123
  },
  "created_at": "2024-01-15T10:50:00Z"
}
```

---

## Configuration

### Get System Configuration

Retrieve current system configuration.

```http
GET /api/v1/config
```

**Response:**
```json
{
  "system": {
    "host": "0.0.0.0",
    "port": 8080,
    "workers": 4
  },
  "compute": {
    "gpu_enabled": true,
    "gpu_devices": [0, 1],
    "cpu_threads": 8,
    "memory_limit": "16GB"
  },
  "models": {
    "max_concurrent": 10,
    "auto_scale": true,
    "scale_threshold": 0.8,
    "idle_timeout": 300
  },
  "api": {
    "rate_limit": 100,
    "auth_enabled": true
  }
}
```

### Update Configuration

Update system configuration (requires admin privileges).

```http
PUT /api/v1/config
```

**Request Body:**
```json
{
  "models": {
    "max_concurrent": 15,
    "auto_scale": true,
    "scale_threshold": 0.75
  },
  "api": {
    "rate_limit": 150
  }
}
```

**Response:**
```json
{
  "status": "success",
  "message": "Configuration updated successfully",
  "restart_required": false,
  "updated_at": "2024-01-15T11:00:00Z"
}
```

### Get Model Configuration

Get configuration for a specific model.

```http
GET /api/v1/config/models/{model_id}
```

**Response:**
```json
{
  "model_id": "123e4567-e89b-12d3-a456-426614174000",
  "model_name": "llama-2-7b",
  "config": {
    "context_length": 4096,
    "temperature": 0.7,
    "max_tokens": 2048,
    "top_p": 0.9,
    "top_k": 40
  },
  "resource_limits": {
    "cpu": 2,
    "memory": "4GB",
    "gpu": 1
  }
}
```

---

## Monitoring

### Get Metrics

Retrieve Prometheus metrics.

```http
GET /metrics
```

**Response:** (Prometheus text format)
```
# HELP ai_provider_http_requests_total Total number of HTTP requests
# TYPE ai_provider_http_requests_total counter
ai_provider_http_requests_total{method="GET",path="/api/v1/models",status="200"} 1234

# HELP ai_provider_model_inference_duration_seconds Duration of model inference
# TYPE ai_provider_model_inference_duration_seconds histogram
ai_provider_model_inference_duration_seconds_bucket{model_id="123e4567",le="0.1"} 10
```

### Get System Stats

Get real-time system statistics.

```http
GET /api/v1/stats
```

**Response:**
```json
{
  "system": {
    "uptime_seconds": 86400,
    "goroutines": 45,
    "version": "1.0.0"
  },
  "resources": {
    "cpu_usage_percent": 45.2,
    "memory_used_mb": 4096,
    "memory_total_mb": 16384,
    "gpu": [
      {
        "id": 0,
        "name": "NVIDIA RTX 3090",
        "memory_used_mb": 8192,
        "memory_total_mb": 24576,
        "utilization_percent": 65.4,
        "temperature_celsius": 72
      }
    ]
  },
  "models": {
    "total": 5,
    "active": 3,
    "loading": 1,
    "inactive": 1
  },
  "requests": {
    "total": 15234,
    "successful": 14999,
    "failed": 235,
    "avg_latency_ms": 245
  },
  "containers": {
    "running": 6,
    "stopped": 2,
    "total_cpu_percent": 125.6,
    "total_memory_mb": 12288
  },
  "database": {
    "connections_active": 10,
    "connections_idle": 5,
    "queries_total": 45678
  },
  "cache": {
    "hits": 12345,
    "misses": 456,
    "hit_rate_percent": 96.4,
    "memory_used_mb": 512
  }
}
```

### Get Inference History

Get recent inference requests.

```http
GET /api/v1/inference/history
```

**Query Parameters:**
- `model_id` (string): Filter by model ID
- `status` (string): Filter by status (success, error)
- `start_date` (string): Start date (ISO 8601)
- `end_date` (string): End date (ISO 8601)
- `page` (int): Page number
- `per_page` (int): Items per page

**Response:**
```json
{
  "data": [
    {
      "id": "inf_abc123",
      "model_id": "123e4567-e89b-12d3-a456-426614174000",
      "model_name": "llama-2-7b",
      "status": "success",
      "latency_ms": 1234,
      "tokens_input": 12,
      "tokens_output": 156,
      "created_at": "2024-01-15T10:40:00Z"
    }
  ],
  "pagination": {
    "page": 1,
    "per_page": 20,
    "total_count": 15234
  }
}
```

---

## WebSocket Streaming

### Stream Inference

Connect to WebSocket for streaming inference responses.

**Endpoint:** `ws://localhost:8080/api/v1/inference/stream`

**Connection:**
```javascript
const ws = new WebSocket('ws://localhost:8080/api/v1/inference/stream');

ws.onopen = () => {
  ws.send(JSON.stringify({
    model_id: "llama-2-7b",
    prompt: "Write a story about a robot.",
    parameters: {
      max_tokens: 500,
      temperature: 0.8
    },
    stream: true
  }));
};

ws.onmessage = (event) => {
  const data = JSON.parse(event.data);
  console.log(data);
};
```

**Stream Message Format:**
```json
{
  "type": "token",
  "inference_id": "inf_abc123",
  "token": "Once",
  "token_id": 1234,
  "logprob": -0.234,
  "finished": false
}
```

**Stream Completion:**
```json
{
  "type": "done",
  "inference_id": "inf_abc123",
  "usage": {
    "prompt_tokens": 12,
    "completion_tokens": 156,
    "total_tokens": 168
  },
  "finish_reason": "stop"
}
```

**Error Message:**
```json
{
  "type": "error",
  "inference_id": "inf_abc123",
  "error": {
    "code": "INFERENCE_ERROR",
    "message": "Model encountered an error"
  }
}
```

---

## Examples

### cURL Examples

#### List Models
```bash
curl -X GET http://localhost:8080/api/v1/models \
  -H "X-API-Key: your-api-key" \
  -H "Content-Type: application/json"
```

#### Run Inference
```bash
curl -X POST http://localhost:8080/api/v1/inference/llama-2-7b \
  -H "X-API-Key: your-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "prompt": "What is artificial intelligence?",
    "parameters": {
      "max_tokens": 200,
      "temperature": 0.7
    }
  }'
```

#### Register Model
```bash
curl -X POST http://localhost:8080/api/v1/models \
  -H "X-API-Key: your-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "llama-2-7b",
    "version": "1.0.0",
    "format": "gguf",
    "source": "https://example.com/models/llama-2-7b.gguf",
    "config": {
      "context_length": 4096,
      "max_tokens": 2048
    }
  }'
```

### Python Example

```python
import requests
import json

class AIProviderClient:
    def __init__(self, base_url="http://localhost:8080", api_key=None):
        self.base_url = base_url
        self.api_key = api_key
        self.headers = {
            "Content-Type": "application/json"
        }
        if api_key:
            self.headers["X-API-Key"] = api_key
    
    def list_models(self, status=None):
        """List all models"""
        params = {}
        if status:
            params["status"] = status
        
        response = requests.get(
            f"{self.base_url}/api/v1/models",
            headers=self.headers,
            params=params
        )
        response.raise_for_status()
        return response.json()
    
    def inference(self, model_id, prompt, **kwargs):
        """Run inference on a model"""
        payload = {
            "prompt": prompt,
            "parameters": kwargs.get("parameters", {}),
            "stream": False
        }
        
        response = requests.post(
            f"{self.base_url}/api/v1/inference/{model_id}",
            headers=self.headers,
            json=payload
        )
        response.raise_for_status()
        return response.json()
    
    def chat(self, model_id, messages, **kwargs):
        """Run chat completion"""
        payload = {
            "messages": messages,
            "parameters": kwargs.get("parameters", {})
        }
        
        response = requests.post(
            f"{self.base_url}/api/v1/inference/{model_id}/chat",
            headers=self.headers,
            json=payload
        )
        response.raise_for_status()
        return response.json()

# Usage
client = AIProviderClient(api_key="your-api-key")

# List models
models = client.list_models(status="active")
print(f"Active models: {len(models['data'])}")

# Run inference
result = client.inference(
    model_id="llama-2-7b",
    prompt="What is machine learning?",
    parameters={"max_tokens": 100, "temperature": 0.7}
)
print(result["result"]["text"])

# Chat completion
messages = [
    {"role": "system", "content": "You are a helpful assistant."},
    {"role": "user", "content": "Hello!"}
]
response = client.chat("llama-2-7b", messages)
print(response["choices"][0]["message"]["content"])
```

### JavaScript/Node.js Example

```javascript
const axios = require('axios');

class AIProviderClient {
  constructor(baseUrl = 'http://localhost:8080', apiKey = null) {
    this.baseUrl = baseUrl;
    this.headers = {
      'Content-Type': 'application/json'
    };
    if (apiKey) {
      this.headers['X-API-Key'] = apiKey;
    }
  }

  async listModels(status = null) {
    const params = status ? { status } : {};
    const response = await axios.get(`${this.baseUrl}/api/v1/models`, {
      headers: this.headers,
      params
    });
    return response.data;
  }

  async inference(modelId, prompt, parameters = {}) {
    const response = await axios.post(
      `${this.baseUrl}/api/v1/inference/${modelId}`,
      {
        prompt,
        parameters,
        stream: false
      },
      { headers: this.headers }
    );
    return response.data;
  }

  async chat(modelId, messages, parameters = {}) {
    const response = await axios.post(
      `${this.baseUrl}/api/v1/inference/${modelId}/chat`,
      {
        messages,
        parameters
      },
      { headers: this.headers }
    );
    return response.data;
  }
}

// Usage
(async () => {
  const client = new AIProviderClient('http://localhost:8080', 'your-api-key');
  
  // List models
  const models = await client.listModels('active');
  console.log(`Active models: ${models.data.length}`);
  
  // Run inference
  const result = await client.inference(
    'llama-2-7b',
    'What is machine learning?',
    { max_tokens: 100, temperature: 0.7 }
  );
  console.log(result.result.text);
  
  // Chat completion
  const messages = [
    { role: 'system', content: 'You are a helpful assistant.' },
    { role: 'user', content: 'Hello!' }
  ];
  const response = await client.chat('llama-2-7b', messages);
  console.log(response.choices[0].message.content);
})();
```

---

## SDK & Client Libraries

Official client libraries are available for:

- **Python**: `pip install ai-provider-client`
- **JavaScript/Node.js**: `npm install ai-provider-client`
- **Go**: `go get github.com/ai-provider/client-go`
- **Java**: Maven dependency available

---

## Best Practices

### Performance

1. **Reuse Connections**: Keep HTTP connections alive for better performance
2. **Batch Requests**: Use batch inference for multiple prompts
3. **Streaming**: Use WebSocket streaming for long-form generation
4. **Caching**: Cache frequently used model configurations
5. **Load Balancing**: Distribute requests across multiple model instances

### Error Handling

1. **Retry Logic**: Implement exponential backoff for 5xx errors
2. **Timeout Handling**: Set appropriate timeouts for long-running requests
3. **Rate Limiting**: Respect rate limit headers and implement backoff
4. **Validation**: Validate inputs before sending to API

### Security

1. **API Keys**: Store API keys securely (environment variables, secret managers)
2. **HTTPS**: Always use HTTPS in production
3. **Input Sanitization**: Sanitize user inputs before sending to API
4. **Access Control**: Use appropriate permissions for API keys

---

## Support

- **Documentation**: https://docs.ai-provider.io
- **API Status**: https://status.ai-provider.io
- **GitHub Issues**: https://github.com/ai-provider/ai-provider/issues
- **Community Discord**: https://discord.gg/ai-provider
- **Email Support**: support@ai-provider.io

---

## Changelog

### v1.0.0 (2024-01-15)
- Initial API release
- Model management endpoints
- Inference endpoints
- Configuration management
- Monitoring and metrics
- WebSocket streaming support
- Batch inference support

---

**Last Updated**: 2024-01-15  
**API Version**: v1  
**Documentation Version**: 1.0.0