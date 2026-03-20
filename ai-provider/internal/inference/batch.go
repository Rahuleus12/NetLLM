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

// BatchProcessor handles batch inference requests
type BatchProcessor struct {
	executor   *InferenceExecutor
	scheduler  *ResourceScheduler

	// Batch tracking
	batches    map[string]*BatchRequest
	results    map[string]*BatchResponse

	// Configuration
	config     *BatchConfig

	// Statistics
	stats      *BatchStats

	mu         sync.RWMutex
	stopChan   chan struct{}
}

// BatchConfig represents configuration for batch processing
type BatchConfig struct {
	MaxBatchSize       int           `json:"max_batch_size"`
	MaxConcurrentBatches int         `json:"max_concurrent_batches"`
	BatchTimeout       time.Duration `json:"batch_timeout"`
	EnableDynamicBatching bool       `json:"enable_dynamic_batching"`
	DynamicBatchWindow  time.Duration `json:"dynamic_batch_window"`
	MaxWaitTime        time.Duration `json:"max_wait_time"`
	EnableOptimization bool          `json:"enable_optimization"`
}

// BatchStats represents statistics for batch processing
type BatchStats struct {
	TotalBatches       int64         `json:"total_batches"`
	CompletedBatches   int64         `json:"completed_batches"`
	FailedBatches      int64         `json:"failed_batches"`
	CancelledBatches   int64         `json:"cancelled_batches"`
	TotalRequests      int64         `json:"total_requests"`
	SuccessfulRequests int64         `json:"successful_requests"`
	FailedRequests     int64         `json:"failed_requests"`
	AvgBatchSize       float64       `json:"avg_batch_size"`
	AvgLatency         time.Duration `json:"avg_latency"`
	ActiveBatches      int           `json:"active_batches"`
	QueuedBatches      int           `json:"queued_batches"`
	UpdatedAt          time.Time     `json:"updated_at"`
}

// NewBatchProcessor creates a new batch processor instance
func NewBatchProcessor(executor *InferenceExecutor, scheduler *ResourceScheduler, config *BatchConfig) *BatchProcessor {
	if config == nil {
		config = &BatchConfig{
			MaxBatchSize:         100,
			MaxConcurrentBatches: 10,
			BatchTimeout:         30 * time.Minute,
			EnableDynamicBatching: true,
			DynamicBatchWindow:   100 * time.Millisecond,
			MaxWaitTime:          5 * time.Second,
			EnableOptimization:   true,
		}
	}

	bp := &BatchProcessor{
		executor:  executor,
		scheduler: scheduler,
		batches:   make(map[string]*BatchRequest),
		results:   make(map[string]*BatchResponse),
		config:    config,
		stats: &BatchStats{
			UpdatedAt: time.Now(),
		},
		stopChan: make(chan struct{}),
	}

	// Start batch processing routine
	go bp.processBatches()

	log.Printf("Batch processor initialized: max_batch_size=%d, max_concurrent=%d",
		config.MaxBatchSize, config.MaxConcurrentBatches)

	return bp
}

// CreateBatch creates a new batch request
func (bp *BatchProcessor) CreateBatch(ctx context.Context, req *BatchRequest) (*BatchResponse, error) {
	// Validate batch
	if err := bp.validateBatch(req); err != nil {
		return nil, err
	}

	bp.mu.Lock()
	defer bp.mu.Unlock()

	// Generate batch ID
	if req.ID == "" {
		req.ID = uuid.New().String()
	}

	// Initialize batch
	req.Status = BatchPending
	req.CreatedAt = time.Now()
	req.Progress = 0.0

	// Create response
	resp := &BatchResponse{
		BatchID:   req.ID,
		ModelID:   req.ModelID,
		Status:    BatchPending,
		Results:   make([]BatchResult, len(req.Requests)),
		Total:     len(req.Requests),
		Succeeded: 0,
		Failed:    0,
		CreatedAt: time.Now(),
	}

	// Store batch and response
	bp.batches[req.ID] = req
	bp.results[req.ID] = resp

	// Update stats
	atomic.AddInt64(&bp.stats.TotalBatches, 1)
	atomic.AddInt64(&bp.stats.TotalRequests, int64(len(req.Requests)))
	atomic.AddInt64(&bp.stats.QueuedBatches, 1)

	log.Printf("Created batch %s with %d requests for model %s",
		req.ID, len(req.Requests), req.ModelID)

	return resp, nil
}

// GetBatch retrieves a batch by ID
func (bp *BatchProcessor) GetBatch(batchID string) (*BatchRequest, error) {
	bp.mu.RLock()
	defer bp.mu.RUnlock()

	batch, exists := bp.batches[batchID]
	if !exists {
		return nil, ErrBatchNotFoundError(batchID)
	}

	return batch, nil
}

// GetBatchResponse retrieves the response for a batch
func (bp *BatchProcessor) GetBatchResponse(batchID string) (*BatchResponse, error) {
	bp.mu.RLock()
	defer bp.mu.RUnlock()

	resp, exists := bp.results[batchID]
	if !exists {
		return nil, ErrBatchNotFoundError(batchID)
	}

	return resp, nil
}

// CancelBatch cancels a batch request
func (bp *BatchProcessor) CancelBatch(batchID string) error {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	batch, exists := bp.batches[batchID]
	if !exists {
		return ErrBatchNotFoundError(batchID)
	}

	// Check if batch can be cancelled
	if batch.Status == BatchComplete || batch.Status == BatchCancelled {
		return NewError(ErrBatchInvalid, "Cannot cancel completed or already cancelled batch").
			WithMetadata("batch_id", batchID).
			WithMetadata("status", batch.Status)
	}

	// Update batch status
	batch.Status = BatchCancelled
	now := time.Now()
	batch.CompletedAt = &now

	// Update response
	if resp, exists := bp.results[batchID]; exists {
		resp.Status = BatchCancelled
		resp.CompletedAt = &now
	}

	// Update stats
	atomic.AddInt64(&bp.stats.CancelledBatches, 1)
	if batch.Status == BatchPending {
		atomic.AddInt64(&bp.stats.QueuedBatches, -1)
	}

	log.Printf("Cancelled batch %s", batchID)

	return nil
}

// GetBatchStatus retrieves the status of a batch
func (bp *BatchProcessor) GetBatchStatus(batchID string) (*BatchStatusInfo, error) {
	bp.mu.RLock()
	defer bp.mu.RUnlock()

	batch, exists := bp.batches[batchID]
	if !exists {
		return nil, ErrBatchNotFoundError(batchID)
	}

	resp := bp.results[batchID]

	statusInfo := &BatchStatusInfo{
		BatchID:    batch.ID,
		ModelID:    batch.ModelID,
		Status:     batch.Status,
		Progress:   batch.Progress,
		Total:      resp.Total,
		Succeeded:  resp.Succeeded,
		Failed:     resp.Failed,
		CreatedAt:  batch.CreatedAt,
		StartedAt:  batch.StartedAt,
		CompletedAt: batch.CompletedAt,
		Duration:   resp.Duration,
	}

	return statusInfo, nil
}

// ListBatches lists batches based on filter
func (bp *BatchProcessor) ListBatches(filter *BatchFilter) []*BatchRequest {
	bp.mu.RLock()
	defer bp.mu.RUnlock()

	var batches []*BatchRequest
	for _, batch := range bp.batches {
		// Apply filters
		if filter != nil {
			if filter.ModelID != "" && batch.ModelID != filter.ModelID {
				continue
			}
			if filter.Status != "" && batch.Status != filter.Status {
				continue
			}
			if !filter.StartTime.IsZero() && batch.CreatedAt.Before(filter.StartTime) {
				continue
			}
			if !filter.EndTime.IsZero() && batch.CreatedAt.After(filter.EndTime) {
				continue
			}
		}
		batches = append(batches, batch)
	}

	// Sort by creation time (newest first)
	// Could use sort.Slice here if needed

	return batches
}

// processBatches processes batches in the background
func (bp *BatchProcessor) processBatches() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			bp.processPendingBatches()
		case <-bp.stopChan:
			return
		}
	}
}

// processPendingBatches processes pending batches
func (bp *BatchProcessor) processPendingBatches() {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	// Count active batches
	activeCount := 0
	for _, batch := range bp.batches {
		if batch.Status == BatchRunning {
			activeCount++
		}
	}

	// Check if we can start more batches
	if activeCount >= bp.config.MaxConcurrentBatches {
		return
	}

	// Find pending batches
	var pendingBatches []*BatchRequest
	for _, batch := range bp.batches {
		if batch.Status == BatchPending {
			pendingBatches = append(pendingBatches, batch)
		}
	}

	// Sort by priority (higher priority first)
	// Could implement custom sorting here

	// Start processing batches
	for _, batch := range pendingBatches {
		if activeCount >= bp.config.MaxConcurrentBatches {
			break
		}

		// Start batch processing in background
		go bp.executeBatch(batch)
		activeCount++
	}
}

// executeBatch executes a batch request
func (bp *BatchProcessor) executeBatch(batch *BatchRequest) {
	bp.mu.Lock()
	batch.Status = BatchRunning
	now := time.Now()
	batch.StartedAt = &now
	bp.results[batch.ID].Status = BatchRunning
	atomic.AddInt64(&bp.stats.QueuedBatches, -1)
	bp.mu.Unlock()

	log.Printf("Starting batch %s with %d requests", batch.ID, len(batch.Requests))

	startTime := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), bp.config.BatchTimeout)
	defer cancel()

	// Execute requests in the batch
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, 10) // Limit concurrent requests within batch

	for i, req := range batch.Requests {
		wg.Add(1)
		go func(index int, request *InferenceRequest) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			// Execute request
			result := bp.executeBatchRequest(ctx, batch, index, request)

			// Update result
			bp.mu.Lock()
			resp := bp.results[batch.ID]
			resp.Results[index] = result

			if result.Success {
				resp.Succeeded++
				atomic.AddInt64(&bp.stats.SuccessfulRequests, 1)
			} else {
				resp.Failed++
				atomic.AddInt64(&bp.stats.FailedRequests, 1)
			}

			// Update progress
			completed := resp.Succeeded + resp.Failed
			batch.Progress = float64(completed) / float64(resp.Total) * 100
			bp.mu.Unlock()
		}(i, req)
	}

	// Wait for all requests to complete
	wg.Wait()

	// Finalize batch
	bp.finalizeBatch(batch, startTime)
}

// executeBatchRequest executes a single request within a batch
func (bp *BatchProcessor) executeBatchRequest(ctx context.Context, batch *BatchRequest, index int, req *InferenceRequest) BatchResult {
	result := BatchResult{
		Index:     index,
		RequestID: req.ID,
		Success:   false,
	}

	// Set request metadata
	if req.ID == "" {
		req.ID = uuid.New().String()
	}
	req.ModelID = batch.ModelID
	req.Mode = ModeBatch
	req.Context = ctx

	// Execute inference
	startTime := time.Now()
	resp, err := bp.executor.Execute(ctx, req)
	result.Duration = time.Since(startTime)

	if err != nil {
		result.Error = &BatchError{
			Code:    int(GetErrorCode(err)),
			Message: err.Error(),
			Type:    string(GetErrorCode(err)),
		}
		return result
	}

	result.Success = true
	result.Response = resp
	return result
}

// finalizeBatch finalizes a completed batch
func (bp *BatchProcessor) finalizeBatch(batch *BatchRequest, startTime time.Time) {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	now := time.Now()
	batch.CompletedAt = &now
	batch.Progress = 100.0

	resp := bp.results[batch.ID]
	resp.Duration = time.Since(startTime)
	resp.CompletedAt = &now

	// Determine final status
	if resp.Failed == 0 {
		batch.Status = BatchComplete
		resp.Status = BatchComplete
	} else if resp.Succeeded == 0 {
		batch.Status = BatchFailed
		resp.Status = BatchFailed
	} else {
		batch.Status = BatchPartial
		resp.Status = BatchPartial
	}

	// Update stats
	atomic.AddInt64(&bp.stats.CompletedBatches, 1)
	if batch.Status == BatchFailed {
		atomic.AddInt64(&bp.stats.FailedBatches, 1)
	}

	// Calculate average batch size
	totalBatches := atomic.LoadInt64(&bp.stats.TotalBatches)
	if totalBatches > 0 {
		totalRequests := atomic.LoadInt64(&bp.stats.TotalRequests)
		bp.stats.AvgBatchSize = float64(totalRequests) / float64(totalBatches)
	}

	bp.stats.UpdatedAt = time.Now()

	log.Printf("Batch %s completed: status=%s, succeeded=%d, failed=%d, duration=%v",
		batch.ID, batch.Status, resp.Succeeded, resp.Failed, resp.Duration)
}

// validateBatch validates a batch request
func (bp *BatchProcessor) validateBatch(batch *BatchRequest) error {
	// Check batch size
	if len(batch.Requests) == 0 {
		return NewError(ErrBatchInvalid, "Batch must contain at least one request")
	}

	if len(batch.Requests) > bp.config.MaxBatchSize {
		return ErrBatchSizeExceededError(len(batch.Requests), bp.config.MaxBatchSize)
	}

	// Validate each request
	for i, req := range batch.Requests {
		if req.Prompt == "" && len(req.Messages) == 0 {
			return NewError(ErrRequestInvalid, "Request must have either prompt or messages").
				WithMetadata("request_index", i)
		}
	}

	// Validate model ID
	if batch.ModelID == "" {
		return NewError(ErrRequestInvalid, "Model ID is required")
	}

	return nil
}

// GetStats returns batch processing statistics
func (bp *BatchProcessor) GetStats() *BatchStats {
	bp.mu.RLock()
	defer bp.mu.RUnlock()

	// Count active and queued batches
	var activeCount, queuedCount int
	for _, batch := range bp.batches {
		if batch.Status == BatchRunning {
			activeCount++
		} else if batch.Status == BatchPending {
			queuedCount++
		}
	}

	stats := *bp.stats
	stats.ActiveBatches = activeCount
	stats.QueuedBatches = queuedCount

	return &stats
}

// OptimizeBatch optimizes a batch request for better performance
func (bp *BatchProcessor) OptimizeBatch(batch *BatchRequest) (*BatchRequest, error) {
	if !bp.config.EnableOptimization {
		return batch, nil
	}

	// Create optimized batch
	optimized := &BatchRequest{
		ID:        batch.ID,
		ModelID:   batch.ModelID,
		Priority:  batch.Priority,
		Metadata:  batch.Metadata,
	}

	// Group requests by similarity (e.g., similar max_tokens, temperature)
	// This is a simplified optimization - real implementation would be more sophisticated
	requestGroups := make(map[string][]*InferenceRequest)

	for _, req := range batch.Requests {
		// Create a key based on request parameters
		key := fmt.Sprintf("%d_%f_%f", req.MaxTokens, req.Temperature, req.TopP)
		requestGroups[key] = append(requestGroups[key], req)
	}

	// Reorder requests for optimal processing
	for _, group := range requestGroups {
		optimized.Requests = append(optimized.Requests, group...)
	}

	return optimized, nil
}

// CleanupOldBatches removes old completed batches from memory
func (bp *BatchProcessor) CleanupOldBatches(maxAge time.Duration) error {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	var removed int

	for batchID, batch := range bp.batches {
		if batch.CompletedAt != nil && batch.CompletedAt.Before(cutoff) {
			delete(bp.batches, batchID)
			delete(bp.results, batchID)
			removed++
		}
	}

	if removed > 0 {
		log.Printf("Cleaned up %d old batches", removed)
	}

	return nil
}

// Shutdown shuts down the batch processor
func (bp *BatchProcessor) Shutdown() error {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	// Stop processing routine
	close(bp.stopChan)

	// Cancel all pending batches
	for _, batch := range bp.batches {
		if batch.Status == BatchPending || batch.Status == BatchRunning {
			batch.Status = BatchCancelled
			now := time.Now()
			batch.CompletedAt = &now
			if resp, exists := bp.results[batch.ID]; exists {
				resp.Status = BatchCancelled
				resp.CompletedAt = &now
			}
		}
	}

	log.Println("Batch processor shutdown complete")
	return nil
}

// BatchStatusInfo represents status information for a batch
type BatchStatusInfo struct {
	BatchID     string        `json:"batch_id"`
	ModelID     string        `json:"model_id"`
	Status      BatchStatus   `json:"status"`
	Progress    float64       `json:"progress"`
	Total       int           `json:"total"`
	Succeeded   int           `json:"succeeded"`
	Failed      int           `json:"failed"`
	CreatedAt   time.Time     `json:"created_at"`
	StartedAt   *time.Time    `json:"started_at"`
	CompletedAt *time.Time    `json:"completed_at"`
	Duration    time.Duration `json:"duration"`
}

// BatchFilter represents filter options for listing batches
type BatchFilter struct {
	ModelID   string      `json:"model_id,omitempty"`
	Status    BatchStatus `json:"status,omitempty"`
	StartTime time.Time   `json:"start_time,omitempty"`
	EndTime   time.Time   `json:"end_time,omitempty"`
	Page      int         `json:"page"`
	PerPage   int         `json:"per_page"`
}

// DynamicBatcher handles dynamic batching of requests
type DynamicBatcher struct {
	config      *BatchConfig
	pendingReqs []*InferenceRequest
	batchChan   chan *BatchRequest
	mu          sync.Mutex
}

// NewDynamicBatcher creates a new dynamic batcher
func NewDynamicBatcher(config *BatchConfig) *DynamicBatcher {
	db := &DynamicBatcher{
		config:      config,
		pendingReqs: make([]*InferenceRequest, 0),
		batchChan:   make(chan *BatchRequest, 100),
	}

	// Start batching routine
	go db.batchRoutine()

	return db
}

// AddRequest adds a request to the dynamic batcher
func (db *DynamicBatcher) AddRequest(req *InferenceRequest) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	db.pendingReqs = append(db.pendingReqs, req)
	return nil
}

// batchRoutine periodically creates batches from pending requests
func (db *DynamicBatcher) batchRoutine() {
	ticker := time.NewTicker(db.config.DynamicBatchWindow)
	defer ticker.Stop()

	for range ticker.C {
		db.createBatch()
	}
}

// createBatch creates a batch from pending requests
func (db *DynamicBatcher) createBatch() {
	db.mu.Lock()
	defer db.mu.Unlock()

	if len(db.pendingReqs) == 0 {
		return
	}

	// Group requests by model
	modelGroups := make(map[string][]*InferenceRequest)
	for _, req := range db.pendingReqs {
		modelGroups[req.ModelID] = append(modelGroups[req.ModelID], req)
	}

	// Create batches for each model
	for modelID, requests := range modelGroups {
		batch := &BatchRequest{
			ID:       uuid.New().String(),
			ModelID:  modelID,
			Requests: requests,
			Priority: PriorityNormal,
		}

		select {
		case db.batchChan <- batch:
			// Remove batched requests from pending
			db.pendingReqs = db.pendingReqs[len(requests):]
		default:
			// Channel full, try again later
			return
		}
	}
}

// GetBatchChan returns the channel for completed batches
func (db *DynamicBatcher) GetBatchChan() <-chan *BatchRequest {
	return db.batchChan
}
