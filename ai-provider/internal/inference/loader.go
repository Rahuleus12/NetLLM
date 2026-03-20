package inference

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/ai-provider/internal/models"
	"github.com/ai-provider/internal/storage"
	"github.com/google/uuid"
)

// ModelLoader handles loading and unloading of AI models
type ModelLoader struct {
	db          *storage.Database
	modelManager *models.ModelManager
	instances   map[string]*ModelInstance
	mu          sync.RWMutex
	config      *LoaderConfig
	scheduler   *ResourceScheduler
	gpuManager  *GPUManager
	memManager  *MemoryManager
}

// LoaderConfig represents configuration for the model loader
type LoaderConfig struct {
	MaxInstances        int           `json:"max_instances"`
	DefaultTimeout      time.Duration `json:"default_timeout"`
	DefaultIdleTimeout  time.Duration `json:"default_idle_timeout"`
	EnableAutoUnload    bool          `json:"enable_auto_unload"`
	HealthCheckInterval time.Duration `json:"health_check_interval"`
	MaxRetries          int           `json:"max_retries"`
	RetryDelay          time.Duration `json:"retry_delay"`
}

// NewModelLoader creates a new model loader instance
func NewModelLoader(db *storage.Database, modelManager *models.ModelManager, config *LoaderConfig) *ModelLoader {
	if config == nil {
		config = &LoaderConfig{
			MaxInstances:        10,
			DefaultTimeout:      5 * time.Minute,
			DefaultIdleTimeout:  30 * time.Minute,
			EnableAutoUnload:    true,
			HealthCheckInterval: 30 * time.Second,
			MaxRetries:          3,
			RetryDelay:          5 * time.Second,
		}
	}

	loader := &ModelLoader{
		db:           db,
		modelManager: modelManager,
		instances:    make(map[string]*ModelInstance),
		config:       config,
	}

	// Initialize resource managers
	loader.gpuManager = NewGPUManager()
	loader.memManager = NewMemoryManager()
	loader.scheduler = NewResourceScheduler(loader.gpuManager, loader.memManager)

	// Start health check routine
	if config.HealthCheckInterval > 0 {
		go loader.healthCheckRoutine()
	}

	// Start auto-unload routine
	if config.EnableAutoUnload {
		go loader.autoUnloadRoutine()
	}

	return loader
}

// LoadModel loads a model into memory and creates a new instance
func (l *ModelLoader) LoadModel(ctx context.Context, req *LoadModelRequest) (*ModelInstance, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Check if we've reached max instances
	if len(l.instances) >= l.config.MaxInstances {
		return nil, ErrMaxInstancesReachedError(l.config.MaxInstances)
	}

	// Get model information from model manager
	model, err := l.modelManager.GetModel(ctx, req.ModelID)
	if err != nil {
		return nil, ErrModelNotFoundError(req.ModelID)
	}

	// Check if model file exists
	if model.FileInfo.Path == "" {
		return nil, NewError(ErrModelPathInvalid, "Model file path is empty").
			WithModelID(req.ModelID)
	}

	// Create instance configuration
	instanceConfig := l.createInstanceConfig(model, req)

	// Allocate resources
	allocation, err := l.scheduler.AllocateResources(ctx, instanceConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to allocate resources: %w", err)
	}

	// Create new instance
	instanceID := uuid.New().String()
	instance := &ModelInstance{
		ID:         instanceID,
		ModelID:    req.ModelID,
		Model:      model,
		Config:     instanceConfig,
		State:      StateLoading,
		Allocation: allocation,
		LoadedAt:   time.Now(),
		LastUsed:   time.Now(),
		metrics:    &PerformanceMetrics{},
		requests:   make(chan *InferenceRequest, instanceConfig.MaxQueueSize),
		stopChan:   make(chan struct{}),
	}

	// Load model based on format
	runtime, err := l.getRuntime(model.Format)
	if err != nil {
		l.scheduler.ReleaseResources(allocation)
		return nil, err
	}

	// Load the model using the appropriate runtime
	if err := runtime.Load(ctx, instance); err != nil {
		l.scheduler.ReleaseResources(allocation)
		return nil, ErrModelLoadFailedError(req.ModelID, err)
	}

	instance.runtime = runtime
	instance.State = StateActive

	// Store instance
	l.instances[instanceID] = instance

	// Record in database
	if err := l.recordInstanceCreation(ctx, instance); err != nil {
		log.Printf("Warning: failed to record instance creation: %v", err)
	}

	// Start request processor
	go instance.processRequests()

	log.Printf("Model loaded successfully: %s (instance: %s, device: %s)",
		req.ModelID, instanceID, req.Device)

	return instance, nil
}

// UnloadModel unloads a model instance from memory
func (l *ModelLoader) UnloadModel(ctx context.Context, req *UnloadModelRequest) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	instance, exists := l.instances[req.InstanceID]
	if !exists {
		return ErrInstanceNotFoundError(req.InstanceID)
	}

	// Check if instance is busy
	if instance.State == StateBusy && !req.Force {
		return NewError(ErrInstanceBusy, "Instance is currently processing requests").
			WithInstanceID(req.InstanceID).
			WithRetryable(true)
	}

	// Set timeout
	timeout := req.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Stop accepting new requests
	instance.State = StateUnloading

	// Wait for current requests to complete or timeout
	done := make(chan struct{})
	go func() {
		instance.waitForCompletion()
		close(done)
	}()

	select {
	case <-done:
		// Proceed with unload
	case <-ctx.Done():
		if !req.Force {
			return NewError(ErrInstanceTimeout, "Timeout waiting for instance to complete").
				WithInstanceID(req.InstanceID)
		}
		// Force unload
		log.Printf("Force unloading instance %s", req.InstanceID)
	}

	// Unload model from runtime
	if instance.runtime != nil {
		if err := instance.runtime.Unload(ctx, instance); err != nil {
			log.Printf("Warning: runtime unload error: %v", err)
		}
	}

	// Stop request processor
	close(instance.stopChan)

	// Release resources
	l.scheduler.ReleaseResources(instance.Allocation)

	// Remove from instances map
	delete(l.instances, req.InstanceID)

	// Record in database
	if err := l.recordInstanceTermination(ctx, instance); err != nil {
		log.Printf("Warning: failed to record instance termination: %v", err)
	}

	log.Printf("Model unloaded successfully: %s (instance: %s)",
		instance.ModelID, req.InstanceID)

	return nil
}

// GetInstance retrieves a model instance by ID
func (l *ModelLoader) GetInstance(instanceID string) (*ModelInstance, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	instance, exists := l.instances[instanceID]
	if !exists {
		return nil, ErrInstanceNotFoundError(instanceID)
	}

	return instance, nil
}

// ListInstances lists all loaded model instances
func (l *ModelLoader) ListInstances(filter *ListInstancesFilter) []*ModelInstance {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var result []*ModelInstance
	for _, instance := range l.instances {
		// Apply filters
		if filter != nil {
			if filter.ModelID != "" && instance.ModelID != filter.ModelID {
				continue
			}
			if filter.State != "" && instance.State != filter.State {
				continue
			}
			if filter.Device != "" && instance.Config.Device != filter.Device {
				continue
			}
		}
		result = append(result, instance)
	}

	return result
}

// GetInstanceStatus retrieves the status of a model instance
func (l *ModelLoader) GetInstanceStatus(instanceID string) (*InstanceStatus, error) {
	instance, err := l.GetInstance(instanceID)
	if err != nil {
		return nil, err
	}

	return instance.GetStatus(), nil
}

// ReloadModel reloads a model instance
func (l *ModelLoader) ReloadModel(ctx context.Context, instanceID string) error {
	instance, err := l.GetInstance(instanceID)
	if err != nil {
		return err
	}

	// Store current config
	config := instance.Config

	// Unload
	unloadReq := &UnloadModelRequest{
		InstanceID: instanceID,
		Force:      false,
		Timeout:    5 * time.Minute,
	}

	if err := l.UnloadModel(ctx, unloadReq); err != nil {
		return fmt.Errorf("failed to unload model for reload: %w", err)
	}

	// Load again with same config
	loadReq := &LoadModelRequest{
		ModelID:      config.ModelID,
		Device:       config.Device,
		DeviceID:     config.DeviceID,
		GPULayers:    config.GPULayers,
		Threads:      config.Threads,
		BatchSize:    config.BatchSize,
		MaxQueueSize: config.MaxQueueSize,
		Timeout:      config.Timeout,
		IdleTimeout:  config.IdleTimeout,
		EnableCache:  config.EnableCache,
		CacheSize:    config.CacheSize,
	}

	_, err = l.LoadModel(ctx, loadReq)
	return err
}

// createInstanceConfig creates an instance configuration from model and request
func (l *ModelLoader) createInstanceConfig(model *models.Model, req *LoadModelRequest) *InstanceConfig {
	config := &InstanceConfig{
		ModelID:       req.ModelID,
		ModelPath:     model.FileInfo.Path,
		Format:        string(model.Format),
		ContextLength: model.Config.ContextLength,
		MaxTokens:     model.Config.MaxTokens,
		Temperature:   model.Config.Temperature,
		TopP:         0.9,  // default values
		TopK:         40,
		Device:       req.Device,
		DeviceID:     req.DeviceID,
		Threads:      req.Threads,
		BatchSize:    req.BatchSize,
		MaxQueueSize: req.MaxQueueSize,
		Timeout:      req.Timeout,
		IdleTimeout:  req.IdleTimeout,
		EnableCache:  req.EnableCache,
		CacheSize:    req.CacheSize,
	}

	// Set defaults
	if config.Threads == 0 {
		config.Threads = 4
	}
	if config.BatchSize == 0 {
		config.BatchSize = 512
	}
	if config.MaxQueueSize == 0 {
		config.MaxQueueSize = 100
	}
	if config.Timeout == 0 {
		config.Timeout = l.config.DefaultTimeout
	}
	if config.IdleTimeout == 0 {
		config.IdleTimeout = l.config.DefaultIdleTimeout
	}

	// GPU-specific settings
	if req.Device == DeviceGPU {
		if config.GPULayers == 0 {
			// Default to offloading all layers to GPU
			config.GPULayers = -1 // -1 means all layers
		}
	}

	return config
}

// getRuntime returns the appropriate runtime for a model format
func (l *ModelLoader) getRuntime(format models.ModelFormat) (ModelRuntime, error) {
	switch format {
	case models.FormatGGUF:
		return NewGGUFRuntime(), nil
	case models.FormatONNX:
		return NewONNXRuntime(), nil
	case models.FormatPyTorch:
		return NewPyTorchRuntime(), nil
	default:
		return nil, ErrModelFormatUnsupportedError(string(format))
	}
}

// recordInstanceCreation records instance creation in the database
func (l *ModelLoader) recordInstanceCreation(ctx context.Context, instance *ModelInstance) error {
	db := l.db.GetDB()

	query := `
		INSERT INTO model_instances (id, model_id, status, port, gpu_device, memory_used)
		VALUES ($1, $2, $3, $4, $5, $6)
	`

	var gpuDevice *int
	if instance.Config.Device == DeviceGPU {
		gpuDevice = &instance.Config.DeviceID
	}

	_, err := db.ExecContext(ctx, query,
		instance.ID,
		instance.ModelID,
		string(instance.State),
		instance.Config.Port,
		gpuDevice,
		instance.Allocation.MemoryBytes,
	)

	return err
}

// recordInstanceTermination records instance termination in the database
func (l *ModelLoader) recordInstanceTermination(ctx context.Context, instance *ModelInstance) error {
	db := l.db.GetDB()

	query := `
		UPDATE model_instances
		SET status = $1, stopped_at = CURRENT_TIMESTAMP
		WHERE id = $2
	`

	_, err := db.ExecContext(ctx, query, string(StateStopped), instance.ID)
	return err
}

// healthCheckRoutine periodically checks the health of loaded instances
func (l *ModelLoader) healthCheckRoutine() {
	ticker := time.NewTicker(l.config.HealthCheckInterval)
	defer ticker.Stop()

	for range ticker.C {
		l.checkInstancesHealth()
	}
}

// checkInstancesHealth checks the health of all loaded instances
func (l *ModelLoader) checkInstancesHealth() {
	l.mu.RLock()
	defer l.mu.RUnlock()

	for _, instance := range l.instances {
		if err := instance.HealthCheck(); err != nil {
			log.Printf("Health check failed for instance %s: %v", instance.ID, err)

			// Mark instance as errored
			instance.State = StateError
			instance.metrics.ErrorCount++

			// Try to restart if needed
			if instance.Config.AutoRestart {
				go l.restartInstance(instance.ID)
			}
		}
	}
}

// autoUnloadRoutine periodically checks for idle instances and unloads them
func (l *ModelLoader) autoUnloadRoutine() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		l.unloadIdleInstances()
	}
}

// unloadIdleInstances unloads instances that have been idle for too long
func (l *ModelLoader) unloadIdleInstances() {
	l.mu.Lock()
	defer l.mu.Unlock()

	ctx := context.Background()

	for _, instance := range l.instances {
		if instance.State == StateIdle {
			idleTime := time.Since(instance.LastUsed)
			if idleTime > instance.Config.IdleTimeout {
				log.Printf("Auto-unloading idle instance %s (idle for %v)",
					instance.ID, idleTime)

				// Unload in background
				go func(instID string) {
					unloadReq := &UnloadModelRequest{
						InstanceID: instID,
						Force:      false,
						Timeout:    30 * time.Second,
					}
					if err := l.UnloadModel(ctx, unloadReq); err != nil {
						log.Printf("Failed to auto-unload instance %s: %v", instID, err)
					}
				}(instance.ID)
			}
		}
	}
}

// restartInstance attempts to restart a failed instance
func (l *ModelLoader) restartInstance(instanceID string) {
	l.mu.RLock()
	instance, exists := l.instances[instanceID]
	l.mu.RUnlock()

	if !exists {
		return
	}

	log.Printf("Attempting to restart instance %s", instanceID)

	ctx := context.Background()

	// Reload the instance
	if err := l.ReloadModel(ctx, instanceID); err != nil {
		log.Printf("Failed to restart instance %s: %v", instanceID, err)
	} else {
		log.Printf("Successfully restarted instance %s", instanceID)
	}
}

// GetStats returns statistics about the loader
func (l *ModelLoader) GetStats() *LoaderStats {
	l.mu.RLock()
	defer l.mu.RUnlock()

	stats := &LoaderStats{
		TotalInstances: len(l.instances),
		MaxInstances:   l.config.MaxInstances,
		ByState:       make(map[InstanceState]int),
		ByDevice:      make(map[DeviceType]int),
	}

	for _, instance := range l.instances {
		stats.ByState[instance.State]++
		stats.ByDevice[instance.Config.Device]++
		stats.TotalMemoryUsed += instance.Allocation.MemoryBytes
		stats.TotalGPUMemoryUsed += instance.Allocation.GPUMemoryBytes
	}

	return stats
}

// LoaderStats represents statistics for the model loader
type LoaderStats struct {
	TotalInstances      int                    `json:"total_instances"`
	MaxInstances        int                    `json:"max_instances"`
	ByState            map[InstanceState]int  `json:"by_state"`
	ByDevice           map[DeviceType]int     `json:"by_device"`
	TotalMemoryUsed    int64                  `json:"total_memory_used"`
	TotalGPUMemoryUsed int64                  `json:"total_gpu_memory_used"`
}

// Shutdown gracefully shuts down all loaded instances
func (l *ModelLoader) Shutdown(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	log.Printf("Shutting down model loader with %d instances", len(l.instances))

	var errors []error

	for _, instance := range l.instances {
		unloadReq := &UnloadModelRequest{
			InstanceID: instance.ID,
			Force:      true,
			Timeout:    30 * time.Second,
		}

		if err := l.UnloadModel(ctx, unloadReq); err != nil {
			errors = append(errors, fmt.Errorf("failed to unload instance %s: %w", instance.ID, err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("errors during shutdown: %v", errors)
	}

	log.Println("Model loader shutdown complete")
	return nil
}

// ModelRuntime interface for different model runtimes
type ModelRuntime interface {
	Load(ctx context.Context, instance *ModelInstance) error
	Unload(ctx context.Context, instance *ModelInstance) error
	Execute(ctx context.Context, instance *ModelInstance, req *InferenceRequest) (*InferenceResponse, error)
	ExecuteStream(ctx context.Context, instance *ModelInstance, req *InferenceRequest) (<-chan *StreamChunk, error)
	HealthCheck(instance *ModelInstance) error
	GetInfo() *RuntimeInfo
}

// RuntimeInfo represents information about a runtime
type RuntimeInfo struct {
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	Formats      []string `json:"formats"`
	Features     []string `json:"features"`
	GPUSupport   bool     `json:"gpu_support"`
	StreamingSupport bool `json:"streaming_support"`
}
