package models

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// ModelListResult represents the result of a model list operation
type ModelListResult struct {
	Models     []*Model `json:"data"`
	TotalCount int64    `json:"total_count"`
	Page       int      `json:"page"`
	PerPage    int      `json:"per_page"`
	TotalPages int      `json:"total_pages"`
}

// ModelRegistry defines the interface for model registry operations
type ModelRegistry interface {
	// CRUD operations
	Create(ctx context.Context, model *Model) error
	Get(ctx context.Context, id string) (*Model, error)
	GetByName(ctx context.Context, name, version string) (*Model, error)
	Update(ctx context.Context, model *Model) error
	Delete(ctx context.Context, id string) error

	// Listing and search
	List(ctx context.Context, filter *ModelFilter) (*ModelListResult, error)
	Search(ctx context.Context, query string) ([]*Model, error)

	// Status management
	UpdateStatus(ctx context.Context, id string, status ModelStatus) error

	// Batch operations
	CreateBatch(ctx context.Context, models []*Model) error
	DeleteBatch(ctx context.Context, ids []string) error

	// Utility methods
	Exists(ctx context.Context, id string) (bool, error)
	Count(ctx context.Context, status ModelStatus) (int64, error)
}

// Registry implements the ModelRegistry interface
type Registry struct {
	db    *sql.DB
	cache *redis.Client
	mu    sync.RWMutex
}

// NewRegistry creates a new model registry instance
func NewRegistry(db *sql.DB, cache *redis.Client) *Registry {
	return &Registry{
		db:    db,
		cache: cache,
	}
}

// Create adds a new model to the registry
func (r *Registry) Create(ctx context.Context, model *Model) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Validate model
	if err := r.validateModel(model); err != nil {
		return fmt.Errorf("model validation failed: %w", err)
	}

	// Set timestamps
	now := time.Now()
	model.CreatedAt = now
	model.UpdatedAt = now

	// Set default status if not specified
	if model.Status == "" {
		model.Status = StatusInactive
	}

	// Insert into database
	query := `
		INSERT INTO models (
			id, name, version, description, format, source, status,
			context_length, temperature, max_tokens, top_p, top_k,
			ram_min, gpu_memory, cpu_cores, gpu_required,
			file_path, file_size, checksum_verified,
			created_at, updated_at, created_by
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7,
			$8, $9, $10, $11, $12,
			$13, $14, $15, $16,
			$17, $18, $19,
			$20, $21, $22
		)
	`

	_, err := r.db.ExecContext(ctx, query,
		model.ID, model.Name, model.Version, model.Description, model.Format, model.Source.URL, model.Status,
		model.Config.ContextLength, model.Config.Temperature, model.Config.MaxTokens, model.Config.TopP, model.Config.TopK,
		model.Requirements.RAMMin, model.Requirements.GPUMemory, model.Requirements.CPUCores, model.Requirements.GPURequired,
		model.FileInfo.Path, model.FileInfo.SizeBytes, model.FileInfo.ChecksumVerified,
		model.CreatedAt, model.UpdatedAt, model.CreatedBy,
	)

	if err != nil {
		return fmt.Errorf("failed to create model in database: %w", err)
	}

	// Cache the model
	if err := r.cacheModel(ctx, model); err != nil {
		log.Printf("Warning: failed to cache model: %v", err)
	}

	log.Printf("Created model: %s (v%s)", model.Name, model.Version)
	return nil
}

// Get retrieves a model by ID
func (r *Registry) Get(ctx context.Context, id string) (*Model, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Try cache first
	model, err := r.getCachedModel(ctx, id)
	if err == nil && model != nil {
		return model, nil
	}

	// Fetch from database
	query := `
		SELECT
			id, name, version, description, format, source, status,
			context_length, temperature, max_tokens, top_p, top_k,
			ram_min, gpu_memory, cpu_cores, gpu_required,
			file_path, file_size, checksum_verified,
			created_at, updated_at, created_by
		FROM models
		WHERE id = $1
	`

	model = &Model{}
	err = r.db.QueryRowContext(ctx, query, id).Scan(
		&model.ID, &model.Name, &model.Version, &model.Description, &model.Format, &model.Source.URL, &model.Status,
		&model.Config.ContextLength, &model.Config.Temperature, &model.Config.MaxTokens, &model.Config.TopP, &model.Config.TopK,
		&model.Requirements.RAMMin, &model.Requirements.GPUMemory, &model.Requirements.CPUCores, &model.Requirements.GPURequired,
		&model.FileInfo.Path, &model.FileInfo.SizeBytes, &model.FileInfo.ChecksumVerified,
		&model.CreatedAt, &model.UpdatedAt, &model.CreatedBy,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("model not found: %s", id)
		}
		return nil, fmt.Errorf("failed to get model: %w", err)
	}

	// Cache for future requests
	if err := r.cacheModel(ctx, model); err != nil {
		log.Printf("Warning: failed to cache model: %v", err)
	}

	return model, nil
}

// GetByName retrieves a model by name and version
func (r *Registry) GetByName(ctx context.Context, name, version string) (*Model, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	query := `
		SELECT
			id, name, version, description, format, source, status,
			context_length, temperature, max_tokens, top_p, top_k,
			ram_min, gpu_memory, cpu_cores, gpu_required,
			file_path, file_size, checksum_verified,
			created_at, updated_at, created_by
		FROM models
		WHERE name = $1 AND version = $2
	`

	model := &Model{}
	err := r.db.QueryRowContext(ctx, query, name, version).Scan(
		&model.ID, &model.Name, &model.Version, &model.Description, &model.Format, &model.Source.URL, &model.Status,
		&model.Config.ContextLength, &model.Config.Temperature, &model.Config.MaxTokens, &model.Config.TopP, &model.Config.TopK,
		&model.Requirements.RAMMin, &model.Requirements.GPUMemory, &model.Requirements.CPUCores, &model.Requirements.GPURequired,
		&model.FileInfo.Path, &model.FileInfo.SizeBytes, &model.FileInfo.ChecksumVerified,
		&model.CreatedAt, &model.UpdatedAt, &model.CreatedBy,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("model not found: %s (v%s)", name, version)
		}
		return nil, fmt.Errorf("failed to get model: %w", err)
	}

	return model, nil
}

// Update modifies an existing model
func (r *Registry) Update(ctx context.Context, model *Model) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Update timestamp
	model.UpdatedAt = time.Now()

	query := `
		UPDATE models SET
			name = $2, version = $3, description = $4, format = $5, source = $6, status = $7,
			context_length = $8, temperature = $9, max_tokens = $10, top_p = $11, top_k = $12,
			ram_min = $13, gpu_memory = $14, cpu_cores = $15, gpu_required = $16,
			file_path = $17, file_size = $18, checksum_verified = $19,
			updated_at = $20
		WHERE id = $1
	`

	result, err := r.db.ExecContext(ctx, query,
		model.ID, model.Name, model.Version, model.Description, model.Format, model.Source.URL, model.Status,
		model.Config.ContextLength, model.Config.Temperature, model.Config.MaxTokens, model.Config.TopP, model.Config.TopK,
		model.Requirements.RAMMin, model.Requirements.GPUMemory, model.Requirements.CPUCores, model.Requirements.GPURequired,
		model.FileInfo.Path, model.FileInfo.SizeBytes, model.FileInfo.ChecksumVerified,
		model.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to update model: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("model not found: %s", model.ID)
	}

	// Invalidate cache
	if err := r.invalidateCache(ctx, model.ID); err != nil {
		log.Printf("Warning: failed to invalidate cache: %v", err)
	}

	// Re-cache the updated model
	if err := r.cacheModel(ctx, model); err != nil {
		log.Printf("Warning: failed to cache model: %v", err)
	}

	log.Printf("Updated model: %s (v%s)", model.Name, model.Version)
	return nil
}

// Delete removes a model from the registry
func (r *Registry) Delete(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Get model first to check if it exists
	model, err := r.Get(ctx, id)
	if err != nil {
		return err
	}

	// Delete from database
	query := `DELETE FROM models WHERE id = $1`
	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete model: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("model not found: %s", id)
	}

	// Invalidate cache
	if err := r.invalidateCache(ctx, id); err != nil {
		log.Printf("Warning: failed to invalidate cache: %v", err)
	}

	log.Printf("Deleted model: %s (v%s)", model.Name, model.Version)
	return nil
}

// List retrieves models with filtering and pagination
func (r *Registry) List(ctx context.Context, filter *ModelFilter) (*ModelListResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Set defaults
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PerPage <= 0 {
		filter.PerPage = 20
	}
	if filter.PerPage > 100 {
		filter.PerPage = 100
	}
	if filter.SortBy == "" {
		filter.SortBy = "created_at"
	}
	if filter.SortOrder == "" {
		filter.SortOrder = "DESC"
	}

	// Build query
	baseQuery := "FROM models WHERE 1=1"
	args := []interface{}{}
	argIndex := 1

	// Add filters
	if filter.Status != "" {
		baseQuery += fmt.Sprintf(" AND status = $%d", argIndex)
		args = append(args, filter.Status)
		argIndex++
	}

	if filter.Format != "" {
		baseQuery += fmt.Sprintf(" AND format = $%d", argIndex)
		args = append(args, filter.Format)
		argIndex++
	}

	if filter.Search != "" {
		baseQuery += fmt.Sprintf(" AND (name ILIKE $%d OR description ILIKE $%d)", argIndex, argIndex)
		searchTerm := "%" + filter.Search + "%"
		args = append(args, searchTerm)
		argIndex++
	}

	// Get total count
	countQuery := "SELECT COUNT(*) " + baseQuery
	var totalCount int64
	err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&totalCount)
	if err != nil {
		return nil, fmt.Errorf("failed to count models: %w", err)
	}

	// Calculate pagination
	totalPages := int(totalCount) / filter.PerPage
	if int(totalCount)%filter.PerPage > 0 {
		totalPages++
	}

	// Get models
	offset := (filter.Page - 1) * filter.PerPage
	query := fmt.Sprintf(`
		SELECT
			id, name, version, description, format, source, status,
			context_length, temperature, max_tokens, top_p, top_k,
			ram_min, gpu_memory, cpu_cores, gpu_required,
			file_path, file_size, checksum_verified,
			created_at, updated_at, created_by
		%s
		ORDER BY %s %s
		LIMIT $%d OFFSET $%d
	`, baseQuery, filter.SortBy, filter.SortOrder, argIndex, argIndex+1)

	args = append(args, filter.PerPage, offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list models: %w", err)
	}
	defer rows.Close()

	models := []*Model{}
	for rows.Next() {
		model := &Model{}
		err := rows.Scan(
			&model.ID, &model.Name, &model.Version, &model.Description, &model.Format, &model.Source.URL, &model.Status,
			&model.Config.ContextLength, &model.Config.Temperature, &model.Config.MaxTokens, &model.Config.TopP, &model.Config.TopK,
			&model.Requirements.RAMMin, &model.Requirements.GPUMemory, &model.Requirements.CPUCores, &model.Requirements.GPURequired,
			&model.FileInfo.Path, &model.FileInfo.SizeBytes, &model.FileInfo.ChecksumVerified,
			&model.CreatedAt, &model.UpdatedAt, &model.CreatedBy,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan model: %w", err)
		}
		models = append(models, model)
	}

	return &ModelListResult{
		Models:     models,
		TotalCount: totalCount,
		Page:       filter.Page,
		PerPage:    filter.PerPage,
		TotalPages: totalPages,
	}, nil
}

// Search performs a full-text search on models
func (r *Registry) Search(ctx context.Context, query string) ([]*Model, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	searchQuery := `
		SELECT
			id, name, version, description, format, source, status,
			context_length, temperature, max_tokens, top_p, top_k,
			ram_min, gpu_memory, cpu_cores, gpu_required,
			file_path, file_size, checksum_verified,
			created_at, updated_at, created_by
		FROM models
		WHERE name ILIKE $1 OR description ILIKE $1
		ORDER BY created_at DESC
		LIMIT 50
	`

	searchTerm := "%" + query + "%"
	rows, err := r.db.QueryContext(ctx, searchQuery, searchTerm)
	if err != nil {
		return nil, fmt.Errorf("failed to search models: %w", err)
	}
	defer rows.Close()

	models := []*Model{}
	for rows.Next() {
		model := &Model{}
		err := rows.Scan(
			&model.ID, &model.Name, &model.Version, &model.Description, &model.Format, &model.Source.URL, &model.Status,
			&model.Config.ContextLength, &model.Config.Temperature, &model.Config.MaxTokens, &model.Config.TopP, &model.Config.TopK,
			&model.Requirements.RAMMin, &model.Requirements.GPUMemory, &model.Requirements.CPUCores, &model.Requirements.GPURequired,
			&model.FileInfo.Path, &model.FileInfo.SizeBytes, &model.FileInfo.ChecksumVerified,
			&model.CreatedAt, &model.UpdatedAt, &model.CreatedBy,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan model: %w", err)
		}
		models = append(models, model)
	}

	return models, nil
}

// UpdateStatus updates the status of a model
func (r *Registry) UpdateStatus(ctx context.Context, id string, status ModelStatus) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	query := `UPDATE models SET status = $2, updated_at = $3 WHERE id = $1`
	result, err := r.db.ExecContext(ctx, query, id, status, time.Now())
	if err != nil {
		return fmt.Errorf("failed to update model status: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("model not found: %s", id)
	}

	// Invalidate cache
	if err := r.invalidateCache(ctx, id); err != nil {
		log.Printf("Warning: failed to invalidate cache: %v", err)
	}

	log.Printf("Updated model status: %s -> %s", id, status)
	return nil
}

// CreateBatch creates multiple models in a single transaction
func (r *Registry) CreateBatch(ctx context.Context, models []*Model) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	query := `
		INSERT INTO models (
			id, name, version, description, format, source, status,
			context_length, temperature, max_tokens, top_p, top_k,
			ram_min, gpu_memory, cpu_cores, gpu_required,
			file_path, file_size, checksum_verified,
			created_at, updated_at, created_by
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7,
			$8, $9, $10, $11, $12,
			$13, $14, $15, $16,
			$17, $18, $19,
			$20, $21, $22
		)
	`

	stmt, err := tx.PrepareContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, model := range models {
		if err := r.validateModel(model); err != nil {
			return fmt.Errorf("model validation failed for %s: %w", model.Name, err)
		}

		now := time.Now()
		model.CreatedAt = now
		model.UpdatedAt = now

		if model.Status == "" {
			model.Status = StatusInactive
		}

		_, err := stmt.ExecContext(ctx,
			model.ID, model.Name, model.Version, model.Description, model.Format, model.Source.URL, model.Status,
			model.Config.ContextLength, model.Config.Temperature, model.Config.MaxTokens, model.Config.TopP, model.Config.TopK,
			model.Requirements.RAMMin, model.Requirements.GPUMemory, model.Requirements.CPUCores, model.Requirements.GPURequired,
			model.FileInfo.Path, model.FileInfo.SizeBytes, model.FileInfo.ChecksumVerified,
			model.CreatedAt, model.UpdatedAt, model.CreatedBy,
		)

		if err != nil {
			return fmt.Errorf("failed to create model %s: %w", model.Name, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	log.Printf("Created %d models in batch", len(models))
	return nil
}

// DeleteBatch deletes multiple models in a single transaction
func (r *Registry) DeleteBatch(ctx context.Context, ids []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	query := `DELETE FROM models WHERE id = $1`
	stmt, err := tx.PrepareContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, id := range ids {
		_, err := stmt.ExecContext(ctx, id)
		if err != nil {
			return fmt.Errorf("failed to delete model %s: %w", id, err)
		}

		// Invalidate cache
		if err := r.invalidateCache(ctx, id); err != nil {
			log.Printf("Warning: failed to invalidate cache for %s: %v", id, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	log.Printf("Deleted %d models in batch", len(ids))
	return nil
}

// Exists checks if a model exists
func (r *Registry) Exists(ctx context.Context, id string) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM models WHERE id = $1)`
	var exists bool
	err := r.db.QueryRowContext(ctx, query, id).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check model existence: %w", err)
	}
	return exists, nil
}

// Count returns the count of models with optional status filter
func (r *Registry) Count(ctx context.Context, status ModelStatus) (int64, error) {
	var query string
	var args []interface{}

	if status != "" {
		query = `SELECT COUNT(*) FROM models WHERE status = $1`
		args = []interface{}{status}
	} else {
		query = `SELECT COUNT(*) FROM models`
		args = []interface{}{}
	}

	var count int64
	err := r.db.QueryRowContext(ctx, query, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count models: %w", err)
	}
	return count, nil
}

// Helper methods

// validateModel validates a model before creation/update
func (r *Registry) validateModel(model *Model) error {
	if model.ID == "" {
		return errors.New("model ID is required")
	}
	if model.Name == "" {
		return errors.New("model name is required")
	}
	if model.Version == "" {
		return errors.New("model version is required")
	}
	if model.Format == "" {
		return errors.New("model format is required")
	}

	// Validate format
	validFormats := map[ModelFormat]bool{
		FormatGGUF:       true,
		FormatONNX:       true,
		FormatPyTorch:    true,
		FormatTensorFlow: true,
		FormatSafeTensors: true,
	}
	if !validFormats[model.Format] {
		return fmt.Errorf("invalid model format: %s", model.Format)
	}

	// Validate status if provided
	if model.Status != "" {
		validStatuses := map[ModelStatus]bool{
			StatusInactive:   true,
			StatusDownloading: true,
			StatusValidating: true,
			StatusLoading:    true,
			StatusActive:     true,
			StatusError:      true,
			StatusDeprecated: true,
		}
		if !validStatuses[model.Status] {
			return fmt.Errorf("invalid model status: %s", model.Status)
		}
	}

	return nil
}

// cacheModel caches a model in Redis
func (r *Registry) cacheModel(ctx context.Context, model *Model) error {
	if r.cache == nil {
		return nil
	}

	key := r.getCacheKey(model.ID)
	// In a real implementation, you would serialize the model to JSON
	// and store it in Redis with an expiration time
	// For now, we'll just log it
	log.Printf("Caching model: %s", key)
	return nil
}

// getCachedModel retrieves a model from cache
func (r *Registry) getCachedModel(ctx context.Context, id string) (*Model, error) {
	if r.cache == nil {
		return nil, errors.New("cache not available")
	}

	key := r.getCacheKey(id)
	// In a real implementation, you would get the model from Redis
	// and deserialize it from JSON
	// For now, we'll return nil to indicate cache miss
	log.Printf("Cache miss for model: %s", key)
	return nil, errors.New("cache miss")
}

// invalidateCache removes a model from cache
func (r *Registry) invalidateCache(ctx context.Context, id string) error {
	if r.cache == nil {
		return nil
	}

	key := r.getCacheKey(id)
	// In a real implementation, you would delete the key from Redis
	log.Printf("Invalidating cache for model: %s", key)
	return nil
}

// getCacheKey generates a cache key for a model
func (r *Registry) getCacheKey(id string) string {
	return fmt.Sprintf("model:%s", id)
}

// GetModelsByStatus retrieves all models with a specific status
func (r *Registry) GetModelsByStatus(ctx context.Context, status ModelStatus) ([]*Model, error) {
	filter := &ModelFilter{
		Status:  status,
		Page:    1,
		PerPage: 1000, // Large limit for internal use
	}
	result, err := r.List(ctx, filter)
	if err != nil {
		return nil, err
	}
	return result.Models, nil
}

// GetActiveModels retrieves all active models
func (r *Registry) GetActiveModels(ctx context.Context) ([]*Model, error) {
	return r.GetModelsByStatus(ctx, StatusActive)
}

// GetModelVersions retrieves all versions of a model by name
func (r *Registry) GetModelVersions(ctx context.Context, name string) ([]*Model, error) {
	query := `
		SELECT
			id, name, version, description, format, source, status,
			context_length, temperature, max_tokens, top_p, top_k,
			ram_min, gpu_memory, cpu_cores, gpu_required,
			file_path, file_size, checksum_verified,
			created_at, updated_at, created_by
		FROM models
		WHERE name = $1
		ORDER BY created_at DESC
	`

	rows, err := r.db.QueryContext(ctx, query, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get model versions: %w", err)
	}
	defer rows.Close()

	models := []*Model{}
	for rows.Next() {
		model := &Model{}
		err := rows.Scan(
			&model.ID, &model.Name, &model.Version, &model.Description, &model.Format, &model.Source.URL, &model.Status,
			&model.Config.ContextLength, &model.Config.Temperature, &model.Config.MaxTokens, &model.Config.TopP, &model.Config.TopK,
			&model.Requirements.RAMMin, &model.Requirements.GPUMemory, &model.Requirements.CPUCores, &model.Requirements.GPURequired,
			&model.FileInfo.Path, &model.FileInfo.SizeBytes, &model.FileInfo.ChecksumVerified,
			&model.CreatedAt, &model.UpdatedAt, &model.CreatedBy,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan model: %w", err)
		}
		models = append(models, model)
	}

	return models, nil
}
