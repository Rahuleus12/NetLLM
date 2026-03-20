package inference

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ai-provider/internal/models"
)

// ModelInstance represents a loaded model instance in memory
type ModelInstance struct {
	ID         string           `json:"id"`
	ModelID    string           `json:"model_id"`
	Model      *models.Model    `json:"model"`
	Config     *InstanceConfig  `json:"config"`
	State      InstanceState    `json:"state"`
	Allocation *ResourceAllocation `json:"allocation"`
	LoadedAt   time.Time        `json:"loaded_at"`
	LastUsed   time.Time        `json:"last_used"`

	runtime   ModelRuntime
	metrics   *PerformanceMetrics
	requests  chan *InferenceRequest
	stopChan  chan struct{}

	// Request processing state
	currentRequests int64
	totalRequests   int64
	failedRequests  int64

	mu sync.RWMutex
}

// GetStatus returns the current status of the model instance
func (i *ModelInstance) GetStatus() *InstanceStatus {
	i.mu.RLock()
	defer i.mu.RUnlock()

	uptime := time.Since(i.LoadedAt)



	// Get runtime-specific metrics if available
	if i.runtime != nil {
		runtimeInfo := i.runtime.GetInfo()
		_ = runtimeInfo // Could be used for additional info
	}

	// Calculate averages from metrics
	if i.metrics != nil {
		status.AvgLatency = i.metrics.AvgLatency
		status.CPUUsage = i.metrics.CPUUsage
		status.GPUUsage = i.metrics.GPUUsage
		status.GPUMemoryUsed = i.metrics.GPUMemoryUsed
		status.GPUMemoryTotal = i.metrics.GPUMemoryTotal
	}

	return status
}

// HealthCheck performs a health check on the instance
func (i *ModelInstance) HealthCheck() error {
	i.mu.RLock()
	defer i.mu.RUnlock()

	// Check if instance is in a valid state
	switch i.State {
	case StateLoading, StateUnloading, StateStopped, StateError:
		return nil // Skip health check for these states
	case StateActive, StateIdle, StateBusy:
		// Continue with health check
	default:
		return fmt.Errorf("instance in unknown state: %s", i.State)
	}

	// Perform runtime health check if available
	if i.runtime != nil {
		if err := i.runtime.HealthCheck(i); err != nil {
			return fmt.Errorf("runtime health check failed: %w", err)
		}
	}

	// Check if request queue is not backed up
	queueSize := len(i.requests)
	if queueSize > i.Config.MaxQueueSize/2 {
		return fmt.Errorf("request queue is backed up: %d/%d", queueSize, i.Config.MaxQueueSize)
	}

	return nil
}

// processRequests processes incoming inference requests
func (i *ModelInstance) processRequests() {
	log.Printf("Starting request processor for instance %s", i.ID)

	for {
		select {
		case req := <-i.requests:
			i.processRequest(req)
		case <-i.stopChan:
			log.Printf("Stopping request processor for instance %s", i.ID)
			return
		}
	}
}

// processRequest processes a single inference request
func (i *ModelInstance) processRequest(req *InferenceRequest) {
	// Update instance state
	i.mu.Lock()
	i.State = StateBusy
	i.LastUsed = time.Now()
	i.mu.Unlock()

	// Track request processing
	atomic.AddInt64(&i.currentRequests, 1)
	atomic.AddInt64(&i.totalRequests, 1)

	defer func() {
		atomic.AddInt64(&i.currentRequests, -1)

		// Update state back to active/idle
		i.mu.Lock()
		if i.State == StateBusy {
			if len(i.requests) > 0 {
				i.State = StateActive
			} else {
				i.State = StateIdle
			}
		}
		i.mu.Unlock()
	}()

	// Create response channel if not provided
	if req.Context == nil {
		req.Context = context.Background()
	}

	// Set up timeout
	ctx, cancel := context.WithTimeout(req.Context, req.Timeout)
	defer cancel()

	// Execute inference based on mode
	var resp *InferenceResponse
	var err error

	startTime := time.Now()

	switch req.Mode {
	case ModeSync:
		resp, err = i.executeSync(ctx, req)
	case ModeStreaming:
		err = i.executeStreaming(ctx, req)
	case ModeBatch:
		resp, err = i.executeSync(ctx, req) // Batch uses sync internally
	default:
		resp, err = i.executeSync(ctx, req) // Default to sync
	}

	latency := time.Since(startTime)

	// Update metrics
	i.updateMetrics(latency, err)

	// Handle error
	if err != nil {
		atomic.AddInt64(&i.failedRequests, 1)
		log.Printf("Request %s failed on instance %s: %v", req.ID, i.ID, err)

		// Could send error response here if needed
		return
	}

	// Log successful request
	log.Printf("Request %s completed on instance %s in %v", req.ID, i.ID, latency)
	_ = resp // Response would be sent back through callback or channel
}

// executeSync executes a synchronous inference request
func (i *ModelInstance) executeSync(ctx context.Context, req *InferenceRequest) (*InferenceResponse, error) {
	if i.runtime == nil {
		return nil, NewError(ErrRuntimeNotInitialized, "Runtime not initialized")
	}

	// Validate request
	if err := i.validateRequest(req); err != nil {
		return nil, err
	}

	// Execute inference
	resp, err := i.runtime.Execute(ctx, i, req)
	if err != nil {
		return nil, ErrInferenceFailedError(req.ID, err)
	}

	// Add instance metadata to response
	resp.InstanceID = i.ID
	resp.RequestID = req.ID
	resp.ModelID = i.ModelID

	return resp, nil
}

// executeStreaming executes a streaming inference request
func (i *ModelInstance) executeStreaming(ctx context.Context, req *InferenceRequest) error {
	if i.runtime == nil {
		return NewError(ErrRuntimeNotInitialized, "Runtime not initialized")
	}

	// Validate request
	if err := i.validateRequest(req); err != nil {
		return err
	}

	// Execute streaming inference
	streamChan, err := i.runtime.ExecuteStream(ctx, i, req)
	if err != nil {
		return ErrStreamingFailedError(req.ID, err)
	}

	// Process stream chunks (would be connected to WebSocket or similar)
	for chunk := range streamChan {
		if chunk.Error != nil {
			log.Printf("Streaming error for request %s: %v", req.ID, chunk.Error)
			return chunk.Error
		}
		// Stream chunk would be sent to client here
		_ = chunk
	}

	return nil
}

// validateRequest validates an inference request
func (i *ModelInstance) validateRequest(req *InferenceRequest) error {
	// Check prompt
	if req.Prompt == "" && len(req.Messages) == 0 {
		return NewError(ErrRequestInvalid, "Request must have either prompt or messages")
	}

	// Check token limits
	if req.MaxTokens > 0 && req.MaxTokens > i.Config.MaxTokens {
		return ErrTokenLimitExceededError(req.MaxTokens, i.Config.MaxTokens)
	}

	// Additional validation could be added here
	// e.g., context length, stop sequences, etc.

	return nil
}

// updateMetrics updates performance metrics after a request
func (i *ModelInstance) updateMetrics(latency time.Duration, err error) {
	i.mu.Lock()
	defer i.mu.Unlock()

	if i.metrics == nil {
		i.metrics = &PerformanceMetrics{
			InstanceID: i.ID,
			ModelID:    i.ModelID,
			MinLatency: latency,
			MaxLatency: latency,
			UpdatedAt:  time.Now(),
		}
	}

	// Update request counts
	i.metrics.RequestCount++
	if err == nil {
		i.metrics.SuccessCount++
	} else {
		i.metrics.ErrorCount++
	}

	// Update latency metrics
	if latency < i.metrics.MinLatency {
		i.metrics.MinLatency = latency
	}
	if latency > i.metrics.MaxLatency {
		i.metrics.MaxLatency = latency
	}

	// Calculate average latency (simple moving average)
	if i.metrics.RequestCount == 1 {
		i.metrics.AvgLatency = latency
	} else {
		total := i.metrics.AvgLatency * time.Duration(i.metrics.RequestCount-1)
		i.metrics.AvgLatency = (total + latency) / time.Duration(i.metrics.RequestCount)
	}

	// Update timestamp
	i.metrics.UpdatedAt = time.Now()
}

// waitForCompletion waits for all current requests to complete
func (i *ModelInstance) waitForCompletion() {
	// Wait for current requests to finish
	for atomic.LoadInt64(&i.currentRequests) > 0 {
		time.Sleep(100 * time.Millisecond)
	}
}

// QueueRequest queues an inference request for processing
func (i *ModelInstance) QueueRequest(req *InferenceRequest) error {
	i.mu.RLock()
	defer i.mu.RUnlock()

	// Check if instance is ready
	if i.State != StateActive && i.State != StateIdle && i.State != StateBusy {
		return ErrInstanceNotReadyError(i.ID, i.State)
	}

	// Check queue capacity
	if len(i.requests) >= i.Config.MaxQueueSize {
		return ErrRequestQueueFullError(i.ID, i.Config.MaxQueueSize)
	}

	// Set request defaults
	if req.Timeout == 0 {
		req.Timeout = i.Config.Timeout
	}
	if req.CreatedAt.IsZero() {
		req.CreatedAt = time.Now()
	}

	// Queue the request
	select {
	case i.requests <- req:
		return nil
	default:
		return ErrRequestQueueFullError(i.ID, i.Config.MaxQueueSize)
	}
}

// GetMetrics returns the current performance metrics
func (i *ModelInstance) GetMetrics() *PerformanceMetrics {
	i.mu.RLock()
	defer i.mu.RUnlock()

	// Return a copy to avoid race conditions
	metrics := *i.metrics
	metrics.Uptime = time.Since(i.LoadedAt)

	return &metrics
}

// SetState sets the instance state
func (i *ModelInstance) SetState(state InstanceState) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.State = state
}

// GetState returns the current instance state
func (i *ModelInstance) GetState() InstanceState {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.State
}

// IsReady returns true if the instance is ready to process requests
func (i *ModelInstance) IsReady() bool {
	i.mu.RLock()
	defer i.mu.RUnlock()

	switch i.State {
	case StateActive, StateIdle, StateBusy:
		return true
	default:
		return false
	}
}

// GetQueueSize returns the current queue size
func (i *ModelInstance) GetQueueSize() int {
	return len(i.requests)
}

// GetCapacity returns the remaining capacity of the request queue
func (i *ModelInstance) GetCapacity() int {
	return i.Config.MaxQueueSize - len(i.requests)
}

// UpdateLastUsed updates the last used timestamp
func (i *ModelInstance) UpdateLastUsed() {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.LastUsed = time.Now()
}

// IdleDuration returns how long the instance has been idle
func (i *ModelInstance) IdleDuration() time.Duration {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return time.Since(i.LastUsed)
}

// Uptime returns how long the instance has been running
func (i *ModelInstance) Uptime() time.Duration {
	return time.Since(i.LoadedAt)
}

// ErrStreamingFailedError creates a streaming failed error
func ErrStreamingFailedError(requestID string, cause error) *InferenceError {
	return WrapError(ErrStreamingFailed, "Streaming inference failed", cause).
		WithRequestID(requestID).
		WithRetryable(false)
}

// ErrTokenLimitExceededError creates a token limit exceeded error
func ErrTokenLimitExceededError(requested, max int) *InferenceError {
	return NewError(ErrTokenLimitExceeded, "Token limit exceeded").
		WithDetails(fmt.Sprintf("Requested %d tokens, maximum is %d", requested, max)).
		WithMetadata("requested_tokens", requested).
		WithMetadata("max_tokens", max)
}
