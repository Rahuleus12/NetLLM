package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
)

// PluginAPI manages plugin API communication
type PluginAPI struct {
	manager    *Manager
	routes     map[string]*PluginRoute
	middleware []APIMiddleware
	mu         sync.RWMutex
}

// PluginRoute represents a plugin API route
type PluginRoute struct {
	PluginID string
	Path     string
	Method   string
	Handler  RouteHandler
}

// RouteHandler handles plugin API requests
type RouteHandler func(ctx context.Context, req *PluginAPIRequest) (*PluginAPIResponse, error)

// APIMiddleware represents middleware for plugin API
type APIMiddleware func(next RouteHandler) RouteHandler

// NewPluginAPI creates a new plugin API instance
func NewPluginAPI(manager *Manager) *PluginAPI {
	api := &PluginAPI{
		manager:    manager,
		routes:     make(map[string]*PluginRoute),
		middleware: make([]APIMiddleware, 0),
	}

	// Add default middleware
	api.Use(api.loggingMiddleware)
	api.Use(api.authMiddleware)
	api.Use(api.rateLimitMiddleware)

	return api
}

// Use adds middleware to the plugin API
func (a *PluginAPI) Use(middleware APIMiddleware) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.middleware = append(a.middleware, middleware)
}

// RegisterRoute registers a plugin API route
func (a *PluginAPI) RegisterRoute(pluginID, method, path string, handler RouteHandler) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	routeKey := fmt.Sprintf("%s:%s:%s", pluginID, method, path)

	if _, exists := a.routes[routeKey]; exists {
		return fmt.Errorf("route already registered: %s %s", method, path)
	}

	// Wrap handler with middleware
	wrappedHandler := handler
	for i := len(a.middleware) - 1; i >= 0; i-- {
		wrappedHandler = a.middleware[i](wrappedHandler)
	}

	a.routes[routeKey] = &PluginRoute{
		PluginID: pluginID,
		Path:     path,
		Method:   method,
		Handler:  wrappedHandler,
	}

	return nil
}

// UnregisterRoute unregisters a plugin API route
func (a *PluginAPI) UnregisterRoute(pluginID, method, path string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	routeKey := fmt.Sprintf("%s:%s:%s", pluginID, method, path)
	delete(a.routes, routeKey)
}

// UnregisterAllRoutes unregisters all routes for a plugin
func (a *PluginAPI) UnregisterAllRoutes(pluginID string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	for key, route := range a.routes {
		if route.PluginID == pluginID {
			delete(a.routes, key)
		}
	}
}

// HandleRequest handles an API request to a plugin
func (a *PluginAPI) HandleRequest(ctx context.Context, req *PluginAPIRequest) (*PluginAPIResponse, error) {
	// Validate request
	if err := a.validateRequest(req); err != nil {
		return &PluginAPIResponse{
			StatusCode: http.StatusBadRequest,
			Error:      err.Error(),
		}, nil
	}

	// Find route
	a.mu.RLock()
	routeKey := fmt.Sprintf("%s:%s:%s", req.PluginID, req.Method, req.Path)
	route, exists := a.routes[routeKey]
	a.mu.RUnlock()

	if !exists {
		return &PluginAPIResponse{
			StatusCode: http.StatusNotFound,
			Error:      fmt.Sprintf("route not found: %s %s", req.Method, req.Path),
		}, nil
	}

	// Check if plugin is running
	plugin, err := a.manager.Get(req.PluginID)
	if err != nil {
		return &PluginAPIResponse{
			StatusCode: http.StatusNotFound,
			Error:      fmt.Sprintf("plugin not found: %s", req.PluginID),
		}, nil
	}

	if plugin.Status != StatusRunning {
		return &PluginAPIResponse{
			StatusCode: http.StatusServiceUnavailable,
			Error:      fmt.Sprintf("plugin is not running: %s (status: %s)", req.PluginID, plugin.Status),
		}, nil
	}

	// Execute handler
	resp, err := route.Handler(ctx, req)
	if err != nil {
		return &PluginAPIResponse{
			StatusCode: http.StatusInternalServerError,
			Error:      err.Error(),
		}, nil
	}

	return resp, nil
}

// CallPlugin calls a plugin method directly
func (a *PluginAPI) CallPlugin(ctx context.Context, pluginID, method string, params map[string]interface{}) (map[string]interface{}, error) {
	// Get plugin instance
	a.manager.mu.RLock()
	instance, exists := a.manager.instances[pluginID]
	a.manager.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("plugin instance not found: %s", pluginID)
	}

	// Check if plugin supports direct calls
	if configurable, ok := instance.(interface {
		CallMethod(ctx context.Context, method string, params map[string]interface{}) (map[string]interface{}, error)
	}); ok {
		return configurable.CallMethod(ctx, method, params)
	}

	return nil, fmt.Errorf("plugin does not support direct method calls")
}

// GetRoutes returns all registered routes
func (a *PluginAPI) GetRoutes(pluginID string) []*PluginRoute {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var routes []*PluginRoute
	for _, route := range a.routes {
		if pluginID == "" || route.PluginID == pluginID {
			routes = append(routes, route)
		}
	}

	return routes
}

// validateRequest validates an API request
func (a *PluginAPI) validateRequest(req *PluginAPIRequest) error {
	if req.PluginID == "" {
		return fmt.Errorf("plugin ID is required")
	}

	if req.Method == "" {
		return fmt.Errorf("method is required")
	}

	if req.Path == "" {
		return fmt.Errorf("path is required")
	}

	// Validate HTTP method
	validMethods := map[string]bool{
		http.MethodGet:     true,
		http.MethodPost:    true,
		http.MethodPut:     true,
		http.MethodDelete:  true,
		http.MethodPatch:   true,
		http.MethodOptions: true,
		http.MethodHead:    true,
	}

	if !validMethods[req.Method] {
		return fmt.Errorf("invalid HTTP method: %s", req.Method)
	}

	return nil
}

// loggingMiddleware logs plugin API requests
func (a *PluginAPI) loggingMiddleware(next RouteHandler) RouteHandler {
	return func(ctx context.Context, req *PluginAPIRequest) (*PluginAPIResponse, error) {
		start := time.Now()

		resp, err := next(ctx, req)

		duration := time.Since(start)

		// Log request
		a.manager.log(req.PluginID, "INFO", fmt.Sprintf(
			"API Request: %s %s - Status: %d - Duration: %v",
			req.Method, req.Path, resp.StatusCode, duration,
		))

		return resp, err
	}
}

// authMiddleware handles authentication for plugin API
func (a *PluginAPI) authMiddleware(next RouteHandler) RouteHandler {
	return func(ctx context.Context, req *PluginAPIRequest) (*PluginAPIResponse, error) {
		// Check for API key or token in headers
		apiKey := req.Headers["X-API-Key"]
		authToken := req.Headers["Authorization"]

		// Validate authentication (simplified)
		// In production, implement proper authentication
		if apiKey == "" && authToken == "" {
			// Allow internal calls without auth
			if req.Headers["X-Internal-Call"] != "true" {
				return &PluginAPIResponse{
					StatusCode: http.StatusUnauthorized,
					Error:      "authentication required",
				}, nil
			}
		}

		return next(ctx, req)
	}
}

// rateLimitMiddleware implements rate limiting for plugin API
func (a *PluginAPI) rateLimitMiddleware(next RouteHandler) RouteHandler {
	// Simple in-memory rate limiter
	// In production, use Redis-based rate limiting
	requestCounts := make(map[string][]time.Time)
	var mu sync.Mutex

	return func(ctx context.Context, req *PluginAPIRequest) (*PluginAPIResponse, error) {
		// Skip rate limiting for internal calls
		if req.Headers["X-Internal-Call"] == "true" {
			return next(ctx, req)
		}

		key := fmt.Sprintf("%s:%s", req.PluginID, req.Path)

		mu.Lock()
		defer mu.Unlock()

		now := time.Now()
		windowStart := now.Add(-time.Minute)

		// Clean old requests
		var validRequests []time.Time
		for _, t := range requestCounts[key] {
			if t.After(windowStart) {
				validRequests = append(validRequests, t)
			}
		}
		requestCounts[key] = validRequests

		// Check rate limit (100 requests per minute)
		if len(validRequests) >= 100 {
			return &PluginAPIResponse{
				StatusCode: http.StatusTooManyRequests,
				Error:      "rate limit exceeded",
				Headers: map[string]string{
					"X-RateLimit-Limit":     "100",
					"X-RateLimit-Remaining": "0",
					"X-RateLimit-Reset":     fmt.Sprintf("%d", windowStart.Add(time.Minute).Unix()),
				},
			}, nil
		}

		// Record request
		requestCounts[key] = append(requestCounts[key], now)

		return next(ctx, req)
	}
}

// PluginAPIServer provides an HTTP server for plugin APIs
type PluginAPIServer struct {
	api      *PluginAPI
	server   *http.Server
	addr     string
}

// NewPluginAPIServer creates a new plugin API server
func NewPluginAPIServer(api *PluginAPI, addr string) *PluginAPIServer {
	return &PluginAPIServer{
		api:  api,
		addr: addr,
	}
}

// Start starts the plugin API server
func (s *PluginAPIServer) Start() error {
	mux := http.NewServeMux()

	// Handle all plugin API requests
	mux.HandleFunc("/api/v1/plugins/", s.handlePluginRequest)

	s.server = &http.Server{
		Addr:    s.addr,
		Handler: mux,
	}

	return s.server.ListenAndServe()
}

// Stop stops the plugin API server
func (s *PluginAPIServer) Stop(ctx context.Context) error {
	if s.server != nil {
		return s.server.Shutdown(ctx)
	}
	return nil
}

// handlePluginRequest handles HTTP requests to plugins
func (s *PluginAPIServer) handlePluginRequest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse request
	apiReq, err := s.parseHTTPRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Handle request
	resp, err := s.api.HandleRequest(ctx, apiReq)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Write response
	s.writeHTTPResponse(w, resp)
}

// parseHTTPRequest parses an HTTP request into PluginAPIRequest
func (s *PluginAPIServer) parseHTTPRequest(r *http.Request) (*PluginAPIRequest, error) {
	// Extract plugin ID and path from URL
	// Expected format: /api/v1/plugins/{plugin_id}/*
	path := r.URL.Path
	if len(path) < len("/api/v1/plugins/") {
		return nil, fmt.Errorf("invalid plugin API path")
	}

	pluginPath := path[len("/api/v1/plugins/"):]

	// Split plugin ID and remaining path
	var pluginID, apiPath string
	for i, c := range pluginPath {
		if c == '/' {
			pluginID = pluginPath[:i]
			apiPath = pluginPath[i:]
			break
		}
	}

	if pluginID == "" {
		pluginID = pluginPath
		apiPath = "/"
	}

	// Parse body
	var body json.RawMessage
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil && err.Error() != "EOF" {
			// Body is not JSON, try to read as raw bytes
			body = json.RawMessage("{}")
		}
	}

	// Extract headers
	headers := make(map[string]string)
	for k, v := range r.Header {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}

	// Extract query params
	params := make(map[string]interface{})
	for k, v := range r.URL.Query() {
		if len(v) > 0 {
			params[k] = v[0]
		}
	}

	return &PluginAPIRequest{
		PluginID: pluginID,
		Method:   r.Method,
		Path:     apiPath,
		Headers:  headers,
		Body:     body,
		Params:   params,
	}, nil
}

// writeHTTPResponse writes a PluginAPIResponse to HTTP response writer
func (s *PluginAPIServer) writeHTTPResponse(w http.ResponseWriter, resp *PluginAPIResponse) {
	// Set headers
	for k, v := range resp.Headers {
		w.Header().Set(k, v)
	}

	// Set content type
	w.Header().Set("Content-Type", "application/json")

	// Set status code
	w.WriteHeader(resp.StatusCode)

	// Write body
	if resp.Body != nil {
		w.Write(resp.Body)
	} else if resp.Data != nil {
		json.NewEncoder(w).Encode(resp.Data)
	} else if resp.Error != "" {
		json.NewEncoder(w).Encode(map[string]string{
			"error": resp.Error,
		})
	}
}

// APIClient provides a client for making plugin API requests
type APIClient struct {
	baseURL    string
	httpClient *http.Client
	headers    map[string]string
}

// NewAPIClient creates a new plugin API client
func NewAPIClient(baseURL string) *APIClient {
	return &APIClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		headers: make(map[string]string),
	}
}

// SetHeader sets a default header for all requests
func (c *APIClient) SetHeader(key, value string) {
	c.headers[key] = value
}

// Request makes an API request to a plugin
func (c *APIClient) Request(ctx context.Context, req *PluginAPIRequest) (*PluginAPIResponse, error) {
	// Create HTTP request
	url := fmt.Sprintf("%s/api/v1/plugins/%s%s", c.baseURL, req.PluginID, req.Path)

	var bodyReader interface{}
	if req.Body != nil {
		bodyReader = req.Body
	}

	httpReq, err := http.NewRequestWithContext(ctx, req.Method, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	for k, v := range c.headers {
		httpReq.Header.Set(k, v)
	}
	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}

	// Execute request
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Parse response
	apiResp := &PluginAPIResponse{
		StatusCode: resp.StatusCode,
		Headers:    make(map[string]string),
	}

	for k, v := range resp.Header {
		if len(v) > 0 {
			apiResp.Headers[k] = v[0]
		}
	}

	if err := json.NewDecoder(resp.Body).Decode(&apiResp.Body); err != nil {
		apiResp.Error = "Failed to parse response body"
	}

	return apiResp, nil
}

// Get makes a GET request to a plugin
func (c *APIClient) Get(ctx context.Context, pluginID, path string, params map[string]interface{}) (*PluginAPIResponse, error) {
	return c.Request(ctx, &PluginAPIRequest{
		PluginID: pluginID,
		Method:   http.MethodGet,
		Path:     path,
		Params:   params,
	})
}

// Post makes a POST request to a plugin
func (c *APIClient) Post(ctx context.Context, pluginID, path string, body interface{}) (*PluginAPIResponse, error) {
	bodyJSON, _ := json.Marshal(body)
	return c.Request(ctx, &PluginAPIRequest{
		PluginID: pluginID,
		Method:   http.MethodPost,
		Path:     path,
		Body:     bodyJSON,
	})
}

// Put makes a PUT request to a plugin
func (c *APIClient) Put(ctx context.Context, pluginID, path string, body interface{}) (*PluginAPIResponse, error) {
	bodyJSON, _ := json.Marshal(body)
	return c.Request(ctx, &PluginAPIRequest{
		PluginID: pluginID,
		Method:   http.MethodPut,
		Path:     path,
		Body:     bodyJSON,
	})
}

// Delete makes a DELETE request to a plugin
func (c *APIClient) Delete(ctx context.Context, pluginID, path string) (*PluginAPIResponse, error) {
	return c.Request(ctx, &PluginAPIRequest{
		PluginID: pluginID,
		Method:   http.MethodDelete,
		Path:     path,
	})
}

// PluginAPIStats tracks plugin API statistics
type PluginAPIStats struct {
	TotalRequests      int64
	SuccessfulRequests int64
	FailedRequests     int64
	AverageLatency     time.Duration
	RequestsByMethod   map[string]int64
	RequestsByPlugin   map[string]int64
	mu                 sync.RWMutex
}

// NewPluginAPIStats creates new API stats tracker
func NewPluginAPIStats() *PluginAPIStats {
	return &PluginAPIStats{
		RequestsByMethod: make(map[string]int64),
		RequestsByPlugin: make(map[string]int64),
	}
}

// RecordRequest records a plugin API request
func (s *PluginAPIStats) RecordRequest(pluginID, method string, duration time.Duration, success bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.TotalRequests++
	if success {
		s.SuccessfulRequests++
	} else {
		s.FailedRequests++
	}

	s.RequestsByMethod[method]++
	s.RequestsByPlugin[pluginID]++

	// Update average latency (simple moving average)
	if s.AverageLatency == 0 {
		s.AverageLatency = duration
	} else {
		s.AverageLatency = (s.AverageLatency + duration) / 2
	}
}

// GetStats returns current API statistics
func (s *PluginAPIStats) GetStats() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return map[string]interface{}{
		"total_requests":       s.TotalRequests,
		"successful_requests":  s.SuccessfulRequests,
		"failed_requests":      s.FailedRequests,
		"success_rate":         float64(s.SuccessfulRequests) / float64(s.TotalRequests),
		"average_latency_ms":   s.AverageLatency.Milliseconds(),
		"requests_by_method":   s.RequestsByMethod,
		"requests_by_plugin":   s.RequestsByPlugin,
	}
}

// GenerateRequestID generates a unique request ID
func GenerateRequestID() string {
	return uuid.New().String()
}
