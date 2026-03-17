package models

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ModelManager orchestrates all model operations
type ModelManager struct {
	registry    *Registry
	downloadMgr *DownloadManager
	config      *ManagerConfig
	mu          sync.RWMutex
	eventChan   chan *ModelEvent
}

// ManagerConfig holds configuration for the model manager
type ManagerConfig struct {
	MaxConcurrentDownloads int           `json:"max_concurrent_downloads"`
	AutoValidate           bool          `json:"auto_validate"`
	AutoActivate           bool          `json:"auto_activate"`
	DownloadTimeout        time.Duration `json:"download_timeout"`
	ValidationTimeout      time.Duration `json:"validation_timeout"`
	ModelStoragePath       string        `json:"model_storage_path"`
	TempPath               string        `json:"temp_path"`
	EnableVersioning       bool          `json:"enable_versioning"`
	MaxVersionsPerModel    int           `json:"max_versions_per_model"`
}

// NewModelManager creates a new model manager instance
func NewModelManager(registry *Registry, downloadMgr *DownloadManager, config *ManagerConfig) *ModelManager {
	// Set defaults
	if config.MaxConcurrentDownloads == 0 {
		config.MaxConcurrentDownloads = 3
	}
	if config.DownloadTimeout == 0 {
		config.DownloadTimeout = 30 * time.Minute
	}
	if config.ValidationTimeout == 0 {
		config.ValidationTimeout = 5 * time.Minute
	}
	if config.ModelStoragePath == "" {
		config.ModelStoragePath = "/models"
	}
	if config.TempPath == "" {
		config.TempPath = "/tmp/ai-provider"
	}
	if config.MaxVersionsPerModel == 0 {
		config.MaxVersionsPerModel = 10
	}

	return &ModelManager{
		registry:    registry,
		downloadMgr: downloadMgr,
		config:      config,
		eventChan:   make(chan *ModelEvent, 100),
	}
}

// RegisterModel registers a new model in the system
func (mm *ModelManager) RegisterModel(ctx context.Context, req *RegisterModelRequest) (*Model, error) {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	// Generate model ID
	modelID := uuid.New().String()

	// Create model instance
	model := &Model{
		ID:          modelID,
		Name:        req.Name,
		Version:     req.Version,
		Description: req.Description,
		Format:      req.Format,
		Status:      StatusInactive,
		Source:      req.Source,
		Config:      req.Config,
		Requirements: req.Requirements,
		Tags:        req.Tags,
		Metadata:    req.Metadata,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		CreatedBy:   req.CreatedBy,
	}

	// Set defaults
	if model.Config.ContextLength == 0 {
		model.Config = DefaultModelConfig()
	}
	if model.Requirements.RAMMin == 0 {
		model.Requirements = DefaultModelRequirements()
	}

	// Set file path
	if req.Source.Type == "url" || req.Source.Type == "s3" {
		model.FileInfo.Path = fmt.Sprintf("%s/%s/%s/model.%s",
			mm.config.ModelStoragePath, model.Name, model.Version, model.Format)
	}

	// Register in the registry
	if err := mm.registry.Create(ctx, model); err != nil {
		return nil, fmt.Errorf("failed to register model: %w", err)
	}

	// Emit event
	mm.emitEvent(modelID, "model.registered", "Model registered successfully", nil)

	log.Printf("Model registered: %s (v%s) - %s", model.Name, model.Version, model.ID)

	// Auto-download if requested
	if req.AutoDownload && (req.Source.Type == "url" || req.Source.Type == "s3") {
		go func() {
			if err := mm.StartDownload(context.Background(), model.ID); err != nil {
				log.Printf("Failed to start auto-download for model %s: %v", model.ID, err)
			}
		}()
	}

	return model, nil
}

// GetModel retrieves a model by ID
func (mm *ModelManager) GetModel(ctx context.Context, modelID string) (*Model, error) {
	return mm.registry.Get(ctx, modelID)
}

// GetModelByName retrieves a model by name and version
func (mm *ModelManager) GetModelByName(ctx context.Context, name, version string) (*Model, error) {
	return mm.registry.GetByName(ctx, name, version)
}

// ListModels lists models with filtering
func (mm *ModelManager) ListModels(ctx context.Context, filter *ModelFilter) (*ModelListResult, error) {
	return mm.registry.List(ctx, filter)
}

// UpdateModel updates a model
func (mm *ModelManager) UpdateModel(ctx context.Context, modelID string, req *UpdateModelRequest) (*Model, error) {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	// Get existing model
	model, err := mm.registry.Get(ctx, modelID)
	if err != nil {
		return nil, err
	}

	// Update fields
	if req.Description != "" {
		model.Description = req.Description
	}
	if req.Config != nil {
		model.Config = *req.Config
	}
	if req.Requirements != nil {
		model.Requirements = *req.Requirements
	}
	if req.Tags != nil {
		model.Tags = req.Tags
	}
	if req.Metadata != nil {
		model.Metadata = req.Metadata
	}

	model.UpdatedAt = time.Now()

	// Save to registry
	if err := mm.registry.Update(ctx, model); err != nil {
		return nil, fmt.Errorf("failed to update model: %w", err)
	}

	// Emit event
	mm.emitEvent(modelID, "model.updated", "Model updated successfully", nil)

	log.Printf("Model updated: %s (v%s)", model.Name, model.Version)
	return model, nil
}

// DeleteModel deletes a model
func (mm *ModelManager) DeleteModel(ctx context.Context, modelID string, force bool) error {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	// Get model
	model, err := mm.registry.Get(ctx, modelID)
	if err != nil {
		return err
	}

	// Check if model can be deleted
	if !force && model.Status == StatusActive {
		return fmt.Errorf("cannot delete active model, use force=true to override")
	}

	// Cancel any active downloads
	if model.Status == StatusDownloading {
		mm.downloadMgr.CancelDownload(ctx, modelID)
	}

	// Delete from registry
	if err := mm.registry.Delete(ctx, modelID); err != nil {
		return fmt.Errorf("failed to delete model: %w", err)
	}

	// TODO: Delete model files from disk

	// Emit event
	mm.emitEvent(modelID, "model.deleted", "Model deleted successfully", map[string]interface{}{
		"force": force,
	})

	log.Printf("Model deleted: %s (v%s)", model.Name, model.Version)
	return nil
}

// StartDownload starts downloading a model
func (mm *ModelManager) StartDownload(ctx context.Context, modelID string) error {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	// Get model
	model, err := mm.registry.Get(ctx, modelID)
	if err != nil {
		return err
	}

	// Check if model can be downloaded
	if model.Status != StatusInactive && model.Status != StatusError {
		return fmt.Errorf("model cannot be downloaded in status: %s", model.Status)
	}

	// Update status to downloading
	if err := mm.registry.UpdateStatus(ctx, modelID, StatusDownloading); err != nil {
		return fmt.Errorf("failed to update model status: %w", err)
	}

	// Create download request
	downloadReq := &DownloadRequest{
		ModelID:      modelID,
		Source:       model.Source,
		DestPath:     model.FileInfo.Path,
		ExpectedSize: model.FileInfo.SizeBytes,
		Checksum:     model.Source.Checksum,
		Config: DownloadConfig{
			MaxThreads:    4,
			ChunkSize:     10 * 1024 * 1024,
			Timeout:       mm.config.DownloadTimeout,
			RetryAttempts: 3,
			RetryDelay:    5 * time.Second,
			ResumeEnabled: true,
			TempDir:       mm.config.TempPath,
		},
	}

	// Start download
	if err := mm.downloadMgr.StartDownload(ctx, downloadReq); err != nil {
		// Update status to error
		mm.registry.UpdateStatus(ctx, modelID, StatusError)
		return fmt.Errorf("failed to start download: %w", err)
	}

	// Monitor download progress
	go mm.monitorDownload(modelID)

	// Emit event
	mm.emitEvent(modelID, "model.download.started", "Model download started", nil)

	log.Printf("Download started for model: %s", modelID)
	return nil
}

// monitorDownload monitors download progress and handles completion
func (mm *ModelManager) monitorDownload(modelID string) {
	ctx := context.Background()

	// Stream progress updates
	progressChan := mm.downloadMgr.StreamProgress(ctx, modelID)

	for progress := range progressChan {
		switch progress.Status {
		case DownloadCompleted:
			// Download completed successfully
			log.Printf("Download completed for model: %s", modelID)

			// Get model to update file info
			model, err := mm.registry.Get(ctx, modelID)
			if err != nil {
				log.Printf("Failed to get model after download: %v", err)
				return
			}

			// Update file info
			model.FileInfo.SizeBytes = progress.TotalBytes
			model.FileInfo.DownloadedAt = time.Now()
			model.FileInfo.ChecksumVerified = progress.BytesDownloaded == progress.TotalBytes

			// Save updates
			if err := mm.registry.Update(ctx, model); err != nil {
				log.Printf("Failed to update model after download: %v", err)
			}

			// Auto-validate if configured
			if mm.config.AutoValidate {
				go func() {
					if err := mm.ValidateModel(ctx, modelID); err != nil {
						log.Printf("Auto-validation failed for model %s: %v", modelID, err)
					}
				}()
			} else {
				// Update status to inactive (ready to activate)
				mm.registry.UpdateStatus(ctx, modelID, StatusInactive)
			}

			// Emit event
			mm.emitEvent(modelID, "model.download.completed", "Model download completed", map[string]interface{}{
				"size_bytes": progress.TotalBytes,
			})

			return

		case DownloadFailed:
			// Download failed
			log.Printf("Download failed for model: %s - %s", modelID, progress.Error)

			// Update status to error
			mm.registry.UpdateStatus(ctx, modelID, StatusError)

			// Emit event
			mm.emitEvent(modelID, "model.download.failed", "Model download failed", map[string]interface{}{
				"error": progress.Error,
			})

			return

		case DownloadCancelled:
			// Download cancelled
			log.Printf("Download cancelled for model: %s", modelID)

			// Update status to inactive
			mm.registry.UpdateStatus(ctx, modelID, StatusInactive)

			// Emit event
			mm.emitEvent(modelID, "model.download.cancelled", "Model download cancelled", nil)

			return
		}
	}
}

// CancelDownload cancels a model download
func (mm *ModelManager) CancelDownload(ctx context.Context, modelID string) error {
	return mm.downloadMgr.CancelDownload(ctx, modelID)
}

// GetDownloadProgress gets download progress for a model
func (mm *ModelManager) GetDownloadProgress(ctx context.Context, modelID string) (*DownloadProgress, error) {
	return mm.downloadMgr.GetProgress(ctx, modelID)
}

// ValidateModel validates a model
func (mm *ModelManager) ValidateModel(ctx context.Context, modelID string) error {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	// Get model
	model, err := mm.registry.Get(ctx, modelID)
	if err != nil {
		return err
	}

	// Update status to validating
	if err := mm.registry.UpdateStatus(ctx, modelID, StatusValidating); err != nil {
		return fmt.Errorf("failed to update model status: %w", err)
	}

	// Emit event
	mm.emitEvent(modelID, "model.validation.started", "Model validation started", nil)

	// Perform validation
	validationResult := &ValidationResult{
		ModelID:     modelID,
		Status:      ValidationPending,
		Checks:      make(map[string]CheckResult),
		ValidatedAt: time.Now(),
	}

	// TODO: Implement actual validation logic
	// For now, we'll simulate validation
	time.Sleep(2 * time.Second)

	// Check if file exists
	if model.FileInfo.Path != "" {
		validationResult.Checks["file_exists"] = CheckResult{
			Name:    "file_exists",
			Status:  ValidationValid,
			Message: "Model file exists",
		}
	}

	// Check checksum if available
	if model.Source.Checksum != "" {
		validationResult.Checks["checksum"] = CheckResult{
			Name:    "checksum",
			Status:  ValidationValid,
			Message: "Checksum verification passed",
		}
	}

	// Check format
	validationResult.Checks["format"] = CheckResult{
		Name:    "format",
		Status:  ValidationValid,
		Message: fmt.Sprintf("Model format is valid: %s", model.Format),
	}

	// Determine overall status
	validationResult.Status = ValidationValid
	for _, check := range validationResult.Checks {
		if check.Status == ValidationInvalid {
			validationResult.Status = ValidationInvalid
			break
		}
	}

	// Update model status based on validation result
	var newStatus ModelStatus
	if validationResult.Status == ValidationValid {
		newStatus = StatusInactive
		if mm.config.AutoActivate {
			newStatus = StatusActive
		}
	} else {
		newStatus = StatusError
	}

	if err := mm.registry.UpdateStatus(ctx, modelID, newStatus); err != nil {
		log.Printf("Failed to update model status after validation: %v", err)
	}

	// Emit event
	eventMessage := "Model validation completed"
	if validationResult.Status == ValidationInvalid {
		eventMessage = "Model validation failed"
	}
	mm.emitEvent(modelID, "model.validation.completed", eventMessage, map[string]interface{}{
		"status": validationResult.Status,
	})

	log.Printf("Model validated: %s - Status: %s", modelID, validationResult.Status)
	return nil
}

// ActivateModel activates a model for inference
func (mm *ModelManager) ActivateModel(ctx context.Context, modelID string) error {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	// Get model
	model, err := mm.registry.Get(ctx, modelID)
	if err != nil {
		return err
	}

	// Check if model can be activated
	if model.Status != StatusInactive && model.Status != StatusError {
		return fmt.Errorf("model cannot be activated in status: %s", model.Status)
	}

	// Update status to loading
	if err := mm.registry.UpdateStatus(ctx, modelID, StatusLoading); err != nil {
		return fmt.Errorf("failed to update model status: %w", err)
	}

	// TODO: Implement actual model loading logic
	// This would involve:
	// 1. Loading model into memory
	// 2. Starting model container
	// 3. Setting up inference endpoint
	// 4. Running health checks

	// Simulate loading time
	time.Sleep(1 * time.Second)

	// Update status to active
	if err := mm.registry.UpdateStatus(ctx, modelID, StatusActive); err != nil {
		return fmt.Errorf("failed to update model status: %w", err)
	}

	// Emit event
	mm.emitEvent(modelID, "model.activated", "Model activated successfully", nil)

	log.Printf("Model activated: %s (v%s)", model.Name, model.Version)
	return nil
}

// DeactivateModel deactivates a model
func (mm *ModelManager) DeactivateModel(ctx context.Context, modelID string) error {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	// Get model
	model, err := mm.registry.Get(ctx, modelID)
	if err != nil {
		return err
	}

	// Check if model can be deactivated
	if model.Status != StatusActive && model.Status != StatusLoading {
		return fmt.Errorf("model cannot be deactivated in status: %s", model.Status)
	}

	// TODO: Implement actual model unloading logic
	// This would involve:
	// 1. Stopping inference endpoint
	// 2. Stopping model container
	// 3. Unloading model from memory
	// 4. Cleaning up resources

	// Update status to inactive
	if err := mm.registry.UpdateStatus(ctx, modelID, StatusInactive); err != nil {
		return fmt.Errorf("failed to update model status: %w", err)
	}

	// Emit event
	mm.emitEvent(modelID, "model.deactivated", "Model deactivated successfully", nil)

	log.Printf("Model deactivated: %s (v%s)", model.Name, model.Version)
	return nil
}

// GetModelStats returns statistics about models
func (mm *ModelManager) GetModelStats(ctx context.Context) (*ModelStats, error) {
	stats := &ModelStats{}

	// Get counts for different statuses
	var err error
	stats.TotalModels, err = mm.registry.Count(ctx, "")
	if err != nil {
		return nil, err
	}

	stats.ActiveModels, err = mm.registry.Count(ctx, StatusActive)
	if err != nil {
		return nil, err
	}

	stats.InactiveModels, err = mm.registry.Count(ctx, StatusInactive)
	if err != nil {
		return nil, err
	}

	stats.DownloadingModels, err = mm.registry.Count(ctx, StatusDownloading)
	if err != nil {
		return nil, err
	}

	stats.ErrorModels, err = mm.registry.Count(ctx, StatusError)
	if err != nil {
		return nil, err
	}

	// TODO: Calculate more detailed statistics
	// - Total instances
	// - Running instances
	// - Total requests
	// - Total tokens
	// - Total storage used

	return stats, nil
}

// SearchModels searches for models
func (mm *ModelManager) SearchModels(ctx context.Context, query string) ([]*Model, error) {
	return mm.registry.Search(ctx, query)
}

// GetModelVersions gets all versions of a model
func (mm *ModelManager) GetModelVersions(ctx context.Context, name string) ([]*Model, error) {
	return mm.registry.GetModelVersions(ctx, name)
}

// emitEvent emits a model event
func (mm *ModelManager) emitEvent(modelID, eventType, message string, details map[string]interface{}) {
	event := &ModelEvent{
		ID:        uuid.New().String(),
		ModelID:   modelID,
		Type:      eventType,
		Message:   message,
		Details:   details,
		Timestamp: time.Now(),
	}

	select {
	case mm.eventChan <- event:
	default:
		log.Printf("Warning: event channel full, dropping event: %s", eventType)
	}
}

// GetEventChannel returns the event channel for monitoring
func (mm *ModelManager) GetEventChannel() <-chan *ModelEvent {
	return mm.eventChan
}

// RegisterModelRequest represents a request to register a new model
type RegisterModelRequest struct {
	Name          string                 `json:"name"`
	Version       string                 `json:"version"`
	Description   string                 `json:"description"`
	Format        ModelFormat            `json:"format"`
	Source        ModelSource            `json:"source"`
	Config        ModelConfig            `json:"config"`
	Requirements  ModelRequirements      `json:"requirements"`
	Tags          []string               `json:"tags"`
	Metadata      map[string]interface{} `json:"metadata"`
	AutoDownload  bool                   `json:"auto_download"`
	AutoActivate  bool                   `json:"auto_activate"`
	CreatedBy     string                 `json:"created_by"`
}

// UpdateModelRequest represents a request to update a model
type UpdateModelRequest struct {
	Description   string                 `json:"description"`
	Config        *ModelConfig           `json:"config"`
	Requirements  *ModelRequirements     `json:"requirements"`
	Tags          []string               `json:"tags"`
	Metadata      map[string]interface{} `json:"metadata"`
}

// ManagerOption represents a functional option for configuring the manager
type ManagerOption func(*ModelManager)

// WithAutoValidate sets auto-validation option
func WithAutoValidate(autoValidate bool) ManagerOption {
	return func(mm *ModelManager) {
		mm.config.AutoValidate = autoValidate
	}
}

// WithAutoActivate sets auto-activation option
func WithAutoActivate(autoActivate bool) ManagerOption {
	return func(mm *ModelManager) {
		mm.config.AutoActivate = autoActivate
	}
}

// WithMaxConcurrentDownloads sets maximum concurrent downloads
func WithMaxConcurrentDownloads(max int) ManagerOption {
	return func(mm *ModelManager) {
		mm.config.MaxConcurrentDownloads = max
	}
}

// WithModelStoragePath sets the model storage path
func WithModelStoragePath(path string) ManagerOption {
	return func(mm *ModelManager) {
		mm.config.ModelStoragePath = path
	}
}

// WithTempPath sets the temporary path
func WithTempPath(path string) ManagerOption {
	return func(mm *ModelManager) {
		mm.config.TempPath = path
	}
}
