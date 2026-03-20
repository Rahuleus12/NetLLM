package inference

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

// InferenceExecutor manages inference request execution and coordination
type InferenceExecutor struct {
	loader    *ModelLoader
	scheduler *ResourceScheduler
	config    *ExecutorConfig

	// Request tracking
	activeRequests map[string]*InferenceRequest
	requestQueue   chan *InferenceRequest

	// Statistics
	stats *ExecutorStats

	// Control
	stopChan chan struct{}
	running  bool
	mu       sync.RWMutex
}

// ExecutorConfig represents configuration for the inference executor
type ExecutorConfig struct {
	MaxConcurrentRequests int           `json:"max_concurrent_requests"`
	DefaultTimeout        time.Duration `json:"default_timeout"`
	MaxQueueSize          int           `json:"max_queue_size"`
	EnableRequestLogging  bool          `json:"enable_request_logging"`
	RetryAttempts         int           `json:"retry_attempts"`
	RetryDelay            time.Duration `json:"retry_delay"`
	EnableMetrics         bool          `json:"enable_metrics"`
}

// ExecutorStats represents statistics for the executor
type ExecutorStats struct {
	TotalRequests      int64         `json:"total_requests"`
	ActiveRequests     int64         `json:"active_requests"`
	QueuedRequests     int64         `json:"queued_requests"`
	CompletedRequests  int64         `json:"completed_requests"`
	FailedRequests     int64         `json:"failed_requests"`
	CancelledRequests  int64         `json:"cancelled_requests"`
	AvgLatency         time.Duration `json:"avg_latency"`
	TotalLatency       time.Duration `json:"total_latency"`
	MaxLatency         time.Duration `json:"max_latency"`
	MinLatency         time.Duration `json:"min_latency"`
	TotalInputTokens   int64         `json:"total_input_tokens"`
	TotalOutputTokens  int64         `json:"total_output_tokens"`
	RequestsPerSecond  float64       `json:"requests_per_second"`
	LastRequestTime    time.Time     `json:"last_request_time"`
	StartTime          time.Time     `json:"start_time"`
	UpdatedAt          time.Time     `json:"updated_at"`
	mu                 sync.RWMutex
}

// NewInferenceExecutor creates a new inference executor
func NewInferenceExecutor(loader *ModelLoader, scheduler *ResourceScheduler, config *ExecutorConfig) *InferenceExecutor {
	if config == nil {
		config = &ExecutorConfig{
			MaxConcurrentRequests: 100,
			DefaultTimeout:        30 * time.Second,
			MaxQueueSize:          1000,
			EnableRequestLogging:  true,
			RetryAttempts:         3,
			RetryDelay:            100 * time.Millisecond,
			EnableMetrics:         true,
		}
	}

	executor := &InferenceExecutor{
		loader:         loader,
		scheduler:      scheduler,
		config:         config,
		activeRequests: make(map[string]*InferenceRequest),
		requestQueue:   make(chan *InferenceRequest, config.MaxQueueSize),
		stats: &ExecutorStats{
			StartTime: time.Now(),
			MinLatency: 24 * time.Hour, // Start with max value
		},
		stopChan: make(chan struct{}),
	}

	// Start worker routines
	executor.Start()

	log.Printf("Inference executor initialized: max_concurrent=%d, queue_size=%d",
		config.MaxConcurrentRequests, config.MaxQueueSize)

	return executor
}

// Start starts the executor worker routines
func (e *InferenceExecutor) Start() {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.running {
		return
	}

	e.running = true

	// Start request processor workers
	for i := 0; i < e.config.MaxConcurrentRequests; i++ {
		go e.requestWorker(i)
	}

	// Start statistics collector
	go e.statsCollector()

	log.Println("Inference executor started")
}

// Stop stops the executor
func (e *InferenceExecutor) Stop() {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.running {
		return
	}

	close(e.stopChan)
	e.running = false

	log.Println("Inference executor stopped")
}

// Execute executes a synchronous inference request
func (e *InferenceExecutor) Execute(ctx context.Context, req *InferenceRequest) (*InferenceResponse, error) {
	// Validate request
	if err := e.validateRequest(req); err != nil {
		return nil, err
	}

	// Set request defaults
	e.setRequestDefaults(req)

	// Generate request ID if not provided
	if req.ID == "" {
		req.ID = uuid.New().String()
	}

	// Set context if not provided
	if req.Context == nil {
		req.Context = ctx
	}

	// Create response channel
	responseChan := make(chan *executorResponse, 1)

	// Track request
	e.trackRequest(req)

	// Queue request
	select {
	case e.requestQueue <- req:
		// Request queued successfully
	default:
		e.untrackRequest(req.ID)
		return nil, NewError(ErrRequestQueueFull, "Request queue is full").
			WithRetryable(true)
	}

	// Wait for response or timeout
	select {
	case resp := <-responseChan:
		e.untrackRequest(req.ID)
		if resp.err != nil {
			e.updateStatsFailed()
			return nil, resp.err
		}
		e.updateStatsSuccess(resp.response)
		return resp.response, nil
	case <-ctx.Done():
		e.untrackRequest(req.ID)
		e.updateStatsCancelled()
		return nil, NewError(ErrInferenceCancelled, "Request cancelled by client").
			WithRequestID(req.ID)
	case <-time.After(req.Timeout):
		e.untrackRequest(req.ID)
		e.updateStatsFailed()
		return nil, ErrInferenceTimeoutError(req.ID, req.Timeout)
	}
}

// ExecuteAsync executes an asynchronous inference request
func (e *InferenceExecutor) ExecuteAsync(ctx context.Context, req *InferenceRequest) (string, error) {
	// Validate request
	if err := e.validateRequest(req); err != nil {
		return "", err
	}

	// Set request defaults
	e.setRequestDefaults(req)

	// Generate request ID if not provided
	if req.ID == "" {
		req.ID = uuid.New().String()
	}

	// Set context if not provided
	if req.Context == nil {
		req.Context = ctx
	}

	// Track request
	e.trackRequest(req)

	// Queue request
	select {
	case e.requestQueue <- req:
		return req.ID, nil
	default:
		e.untrackRequest(req.ID)
		return "", NewError(ErrRequestQueueFull, "Request queue is full").
			WithRetryable(true)
	}
}

// GetRequestStatus gets the status of an async request
func (e *InferenceExecutor) GetRequestStatus(requestID string) (*RequestStatus, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	req, exists := e.activeRequests[requestID]
	if !exists {
		return nil, NewError(ErrRequestInvalid, "Request not found").
			WithRequestID(requestID)
	}

	status := &RequestStatus{
		RequestID: requestID,
		ModelID:   req.ModelID,
		Status:    "processing",
		CreatedAt: req.CreatedAt,
	}

	return status, nil
}

// CancelRequest cancels an active request
func (e *InferenceExecutor) CancelRequest(requestID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	_, exists := e.activeRequests[requestID]
	if !exists {
		return NewError(ErrRequestInvalid, "Request not found").
			WithRequestID(requestID)
	}

	// Mark as cancelled (the request worker will check this)
	delete(e.activeRequests, requestID)
	atomic.AddInt64(&e.stats.CancelledRequests, 1)

	log.Printf("Request %s cancelled", requestID)

	return nil
}

// requestWorker processes requests from the queue
func (e *InferenceExecutor) requestWorker(workerID int) {
	log.Printf("Request worker %d started", workerID)

	for {
		select {
		case req := <-e.requestQueue:
			e.processRequest(req)
		case <-e.stopChan:
			log.Printf("Request worker %d stopped", workerID)
			return
		}
	}
}

// processRequest processes a single inference request
func (e *InferenceExecutor) processRequest(req *InferenceRequest) {
	startTime := time.Now()

	if e.config.EnableRequestLogging {
		log.Printf("Processing request %s for model %s", req.ID, req.ModelID)
	}

	// Get instances for the model
	instances := e.loader.ListInstances(&ListInstancesFilter{
		ModelID: req.ModelID,
	})

	if len(instances) == 0 {
		e.handleRequestError(req, ErrNoAvailableInstanceError(req.ModelID))
		return
	}

	// Select best instance using scheduler
	instance, err := e.scheduler.SelectInstance(req.ModelID, instances)
	if err != nil {
		e.handleRequestError(req, err)
		return
	}

	// Execute with retry logic
	var resp *InferenceResponse
	for attempt := 0; attempt <= e.config.RetryAttempts; attempt++ {
		if attempt > 0 {
			time.Sleep(e.config.RetryDelay)
			log.Printf("Retrying request %s (attempt %d/%d)", req.ID, attempt, e.config.RetryAttempts)
		}

		resp, err = e.executeOnInstance(req, instance)
		if err == nil {
			break
		}

		// Check if error is retryable
		if !IsRetryable(err) {
			break
		}

		// Select different instance for retry
		if attempt < e.config.RetryAttempts {
			instance, err = e.scheduler.SelectInstance(req.ModelID, instances)
			if err != nil {
				break
			}
		}
	}

	// Handle result
	if err != nil {
		e.handleRequestError(req, err)
		return
	}

	// Update latency
	resp.Latency = time.Since(startTime)

	// Complete request in scheduler
	e.scheduler.CompleteRequest(req, nil)

	if e.config.EnableRequestLogging {
		log.Printf("Request %s completed in %v", req.ID, resp.Latency)
	}
}

// executeOnInstance executes a request on a specific instance
func (e *InferenceExecutor) executeOnInstance(req *InferenceRequest, instance *ModelInstance) (*InferenceResponse, error) {
	// Check if instance is ready
	if !instance.IsReady() {
		return nil, ErrInstanceNotReadyError(instance.ID, instance.GetState())
	}

	// Queue request to instance
	if err := instance.QueueRequest(req); err != nil {
		return nil, err
	}

	// The instance will process the request and return response
	// For now, we'll execute synchronously
	ctx := req.Context
	if ctx == nil {
		ctx = context.Background()
	}

	// Set timeout
	ctx, cancel := context.WithTimeout(ctx, req.Timeout)
	defer cancel()

	// Execute based on mode
	switch req.Mode {
	case ModeSync:
		return instance.executeSync(ctx, req)
	case ModeStreaming:
		return nil, NewError(ErrNotImplemented, "Streaming mode not yet implemented")
	case ModeBatch:
		return instance.executeSync(ctx, req)
	default:
		return instance.executeSync(ctx, req)
	}
}

// handleRequestError handles a request error
func (e *InferenceExecutor) handleRequestError(req *InferenceRequest, err error) {
	e.scheduler.CompleteRequest(req, err)
	e.untrackRequest(req.ID)

	if e.config.EnableRequestLogging {
		log.Printf("Request %s failed: %v", req.ID, err)
	}
}

// validateRequest validates an inference request
func (e *InferenceExecutor) validateRequest(req *InferenceRequest) error {
	if req.ModelID == "" {
		return NewError(ErrRequestInvalid, "Model ID is required")
	}

	if req.Prompt == "" && len(req.Messages) == 0 {
		return NewError(ErrRequestInvalid, "Prompt or messages are required")
	}

	if req.MaxTokens < 0 {
		return NewError(ErrRequestInvalid, "Max tokens cannot be negative")
	}

	return nil
}

// setRequestDefaults sets default values for a request
func (e *InferenceExecutor) setRequestDefaults(req *InferenceRequest) {
	if req.Mode == "" {
		req.Mode = ModeSync
	}

	if req.Priority == 0 {
		req.Priority = PriorityNormal
	}

	if req.Timeout == 0 {
		req.Timeout = e.config.DefaultTimeout
	}

	if req.MaxTokens == 0 {
		req.MaxTokens = 512 // Default max tokens
	}

	if req.CreatedAt.IsZero() {
		req.CreatedAt = time.Now()
	}
}

// trackRequest tracks an active request
func (e *InferenceExecutor) trackRequest(req *InferenceRequest) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.activeRequests[req.ID] = req
	atomic.AddInt64(&e.stats.TotalRequests, 1)
	atomic.AddInt64(&e.stats.ActiveRequests, 1)
	e.stats.LastRequestTime = time.Now()
}

// untrackRequest removes a request from tracking
func (e *InferenceExecutor) untrackRequest(requestID string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	delete(e.activeRequests, requestID)
	atomic.AddInt64(&e.stats.ActiveRequests, -1)
}

// updateStatsSuccess updates statistics for successful request
func (e *InferenceExecutor) updateStatsSuccess(resp *InferenceResponse) {
	e.stats.mu.Lock()
	defer e.stats.mu.Unlock()

	atomic.AddInt64(&e.stats.CompletedRequests, 1)
	atomic.AddInt64(&e.stats.TotalInputTokens, int64(resp.InputTokens))
	atomic.AddInt64(&e.stats.TotalOutputTokens, int64(resp.OutputTokens))

	// Update latency stats
	e.stats.TotalLatency += resp.Latency
	if resp.Latency > e.stats.MaxLatency {
		e.stats.MaxLatency = resp.Latency
	}
	if resp.Latency < e.stats.MinLatency {
		e.stats.MinLatency = resp.Latency
	}

	// Calculate average latency
	completed := atomic.LoadInt64(&e.stats.CompletedRequests)
	if completed > 0 {
		e.stats.AvgLatency = time.Duration(int64(e.stats.TotalLatency) / completed)
	}

	e.stats.UpdatedAt = time.Now()
}

// updateStatsFailed updates statistics for failed request
func (e *InferenceExecutor) updateStatsFailed() {
	atomic.AddInt64(&e.stats.FailedRequests, 1)
	e.stats.UpdatedAt = time.Now()
}

// updateStatsCancelled updates statistics for cancelled request
func (e *InferenceExecutor) updateStatsCancelled() {
	atomic.AddInt64(&e.stats.CancelledRequests, 1)
	e.stats.UpdatedAt = time.Now()
}

// statsCollector periodically collects and updates statistics
func (e *InferenceExecutor) statsCollector() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			e.updateRequestsPerSecond()
		case <-e.stopChan:
			return
		}
	}
}

// updateRequestsPerSecond calculates requests per second
func (e *InferenceExecutor) updateRequestsPerSecond() {
	e.stats.mu.Lock()
	defer e.stats.mu.Unlock()

	elapsed := time.Since(e.stats.StartTime).Seconds()
	if elapsed > 0 {
		completed := atomic.LoadInt64(&e.stats.CompletedRequests)
		e.stats.RequestsPerSecond = float64(completed) / elapsed
	}
}

// GetStats returns current executor statistics
func (e *InferenceExecutor) GetStats() *ExecutorStats {
	e.stats.mu.RLock()
	defer e.stats.mu.RUnlock()

	// Create a copy without the mutex
	stats := &ExecutorStats{
		TotalRequests:     atomic.LoadInt64(&e.stats.TotalRequests),
		ActiveRequests:    atomic.LoadInt64(&e.stats.ActiveRequests),
		QueuedRequests:    int64(len(e.requestQueue)),
		CompletedRequests: atomic.LoadInt64(&e.stats.CompletedRequests),
		FailedRequests:    atomic.LoadInt64(&e.stats.FailedRequests),
		CancelledRequests: atomic.LoadInt64(&e.stats.CancelledRequests),
		AvgLatency:        e.stats.AvgLatency,
		TotalLatency:      e.stats.TotalLatency,
		MaxLatency:        e.stats.MaxLatency,
		MinLatency:        e.stats.MinLatency,
		TotalInputTokens:  atomic.LoadInt64(&e.stats.TotalInputTokens),
		TotalOutputTokens: atomic.LoadInt64(&e.stats.TotalOutputTokens),
		RequestsPerSecond: e.stats.RequestsPerSecond,
		LastRequestTime:   e.stats.LastRequestTime,
		StartTime:         e.stats.StartTime,
		UpdatedAt:         e.stats.UpdatedAt,
	}

	return stats
}

// GetActiveRequests returns a list of active requests
func (e *InferenceExecutor) GetActiveRequests() []*InferenceRequest {
	e.mu.RLock()
	defer e.mu.RUnlock()

	requests := make([]*InferenceRequest, 0, len(e.activeRequests))
	for _, req := range e.activeRequests {
		requests = append(requests, req)
	}
	return requests
}

// GetQueueSize returns the current queue size
func (e *InferenceExecutor) GetQueueSize() int {
	return len(e.requestQueue)
}

// GetQueueCapacity returns the queue capacity
func (e *InferenceExecutor) GetQueueCapacity() int {
	return cap(e.requestQueue)
}

// Shutdown gracefully shuts down the executor
func (e *InferenceExecutor) Shutdown(ctx context.Context) error {
	log.Println("Shutting down inference executor...")

	// Stop accepting new requests
	e.Stop()

	// Wait for active requests to complete
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	timeout := time.After(30 * time.Second)

	for {
		select {
		case <-ticker.C:
			active := atomic.LoadInt64(&e.stats.ActiveRequests)
			if active == 0 {
				log.Println("All requests completed")
				return nil
			}
			log.Printf("Waiting for %d active requests to complete...", active)
		case <-timeout:
			active := atomic.LoadInt64(&e.stats.ActiveRequests)
			return fmt.Errorf("shutdown timeout: %d requests still active", active)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// wrappedRequest wraps a request with a response channel
type wrappedRequest struct {
	request      *InferenceRequest
	responseChan chan *executorResponse
}

// executorResponse represents a response from the executor
type executorResponse struct {
	response *InferenceResponse
	err      error
}

// RequestStatus represents the status of an async request
type RequestStatus struct {
	RequestID string    `json:"request_id"`
	ModelID   string    `json:"model_id"`
	Status    string    `json:"status"`
	Error     string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ExecuteBatch executes a batch of inference requests
func (e *InferenceExecutor) ExecuteBatch(ctx context.Context, batchReq *BatchRequest) (*BatchResponse, error) {
	if len(batchReq.Requests) == 0 {
		return nil, NewError(ErrBatchInvalid, "Batch request must contain at least one request")
	}

	// Create batch response
	response := &BatchResponse{
		ID:        uuid.New().String(),
		BatchID:   batchReq.ID,
		ModelID:   batchReq.ModelID,
		Status:    BatchRunning,
		Results:   make([]BatchResult, len(batchReq.Requests)),
		Total:     len(batchReq.Requests),
		CreatedAt: time.Now(),
	}

	startTime := time.Now()

	// Process requests concurrently
	var wg sync.WaitGroup
	var mu sync.Mutex

	for i := range batchReq.Requests {
		wg.Add(1)

		go func(index int, request *InferenceRequest) {
			defer wg.Done()

			reqStartTime := time.Now()

			// Execute request
			resp, err := e.Execute(ctx, request)

			duration := time.Since(reqStartTime)

			// Lock to safely update results
			mu.Lock()
			defer mu.Unlock()

			result := BatchResult{
				Index:    index,
				RequestID: request.ID,
				Success:  err == nil,
				Duration: duration,
			}

			if err != nil {
				errorCode := GetErrorCode(err)
				result.Error = &BatchError{
					Code:    int(errorCode[0]), // Convert first character to int as simple error code
					Message: err.Error(),
					Type:    "execution_error",
				}
				response.Failed++
			} else {
				result.Response = resp
				response.Succeeded++
			}

			response.Results[index] = result
		}(i, &batchReq.Requests[i])
	}

	// Wait for all requests to complete
	wg.Wait()

	// Update batch status
	response.Duration = time.Since(startTime)
	completedAt := time.Now()
	response.CompletedAt = &completedAt

	if response.Failed == 0 {
		response.Status = BatchComplete
	} else if response.Succeeded == 0 {
		response.Status = BatchFailed
	} else {
		response.Status = BatchPartial
	}

	return response, nil
}
