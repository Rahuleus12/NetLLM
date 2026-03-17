package models

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"
)

// ConfigManager manages model configurations
type ConfigManager interface {
	// Configuration CRUD
	GetConfig(ctx context.Context, modelID string) (*ModelConfig, error)
	UpdateConfig(ctx context.Context, modelID string, config *ModelConfig) error
	ValidateConfig(ctx context.Context, config *ModelConfig) error
	ResetConfig(ctx context.Context, modelID string) error

	// Template management
	CreateTemplate(ctx context.Context, template *ConfigTemplate) error
	GetTemplate(ctx context.Context, id string) (*ConfigTemplate, error)
	GetTemplateByName(ctx context.Context, name string) (*ConfigTemplate, error)
	ListTemplates(ctx context.Context, category string) ([]*ConfigTemplate, error)
	UpdateTemplate(ctx context.Context, template *ConfigTemplate) error
	DeleteTemplate(ctx context.Context, id string) error
	ApplyTemplate(ctx context.Context, modelID string, templateName string, override *ModelConfig) error

	// Configuration utilities
	MergeConfigs(base *ModelConfig, override *ModelConfig) *ModelConfig
	ValidateAgainstSchema(config *ModelConfig, schema *ConfigSchema) error
	GetDefaultConfig(format ModelFormat) *ModelConfig
}

// ConfigSchema represents a configuration validation schema
type ConfigSchema struct {
	Name        string                 `json:"name"`
	Version     string                 `json:"version"`
	Fields      map[string]FieldSchema `json:"fields"`
	Required    []string               `json:"required"`
	Constraints map[string]interface{} `json:"constraints"`
}

// FieldSchema represents schema for a configuration field
type FieldSchema struct {
	Type        string      `json:"type"`
	Min         interface{} `json:"min,omitempty"`
	Max         interface{} `json:"max,omitempty"`
	Default     interface{} `json:"default,omitempty"`
	Description string      `json:"description"`
	Enum        []string    `json:"enum,omitempty"`
	Pattern     string      `json:"pattern,omitempty"`
}

// ConfigManagerImpl implements the ConfigManager interface
type ConfigManagerImpl struct {
	db        *sql.DB
	registry  ModelRegistry
	templates map[string]*ConfigTemplate
	schemas   map[string]*ConfigSchema
	mu        sync.RWMutex
}

// NewConfigManager creates a new configuration manager
func NewConfigManager(db *sql.DB, registry ModelRegistry) *ConfigManagerImpl {
	cm := &ConfigManagerImpl{
		db:        db,
		registry:  registry,
		templates: make(map[string]*ConfigTemplate),
		schemas:   make(map[string]*ConfigSchema),
	}

	// Initialize default templates
	cm.initializeDefaultTemplates()

	// Initialize configuration schemas
	cm.initializeSchemas()

	return cm
}

// GetConfig retrieves the configuration for a model
func (cm *ConfigManagerImpl) GetConfig(ctx context.Context, modelID string) (*ModelConfig, error) {
	model, err := cm.registry.Get(ctx, modelID)
	if err != nil {
		return nil, NewConfigurationError(modelID, "", err, "failed to get model")
	}

	// Return a copy to prevent unintended modifications
	config := model.Config
	return &config, nil
}

// UpdateConfig updates the configuration for a model
func (cm *ConfigManagerImpl) UpdateConfig(ctx context.Context, modelID string, config *ModelConfig) error {
	// Validate the configuration
	if err := cm.ValidateConfig(ctx, config); err != nil {
		return NewConfigurationError(modelID, "", err, "configuration validation failed")
	}

	// Get the current model
	model, err := cm.registry.Get(ctx, modelID)
	if err != nil {
		return NewConfigurationError(modelID, "", err, "failed to get model")
	}

	// Update the configuration
	model.Config = *config

	// Save to database
	if err := cm.updateModelConfig(ctx, modelID, config); err != nil {
		return NewConfigurationError(modelID, "", err, "failed to update configuration in database")
	}

	// Update the model in registry
	if err := cm.registry.Update(ctx, model); err != nil {
		return NewConfigurationError(modelID, "", err, "failed to update model")
	}

	log.Printf("Updated configuration for model %s", modelID)
	return nil
}

// ValidateConfig validates a model configuration
func (cm *ConfigManagerImpl) ValidateConfig(ctx context.Context, config *ModelConfig) error {
	if config == nil {
		return errors.New("configuration is nil")
	}

	// Validate context length
	if config.ContextLength <= 0 {
		return NewConfigurationError("", "context_length", nil, "context length must be positive")
	}
	if config.ContextLength > 1000000 {
		return NewConfigurationError("", "context_length", nil, "context length exceeds maximum (1000000)")
	}

	// Validate temperature
	if config.Temperature < 0 || config.Temperature > 2.0 {
		return NewConfigurationError("", "temperature", nil, "temperature must be between 0 and 2.0")
	}

	// Validate max tokens
	if config.MaxTokens <= 0 {
		return NewConfigurationError("", "max_tokens", nil, "max tokens must be positive")
	}
	if config.MaxTokens > config.ContextLength {
		return NewConfigurationError("", "max_tokens", nil, "max tokens cannot exceed context length")
	}

	// Validate top_p
	if config.TopP < 0 || config.TopP > 1.0 {
		return NewConfigurationError("", "top_p", nil, "top_p must be between 0 and 1.0")
	}

	// Validate top_k
	if config.TopK < 0 {
		return NewConfigurationError("", "top_k", nil, "top_k must be non-negative")
	}

	// Validate frequency penalty
	if config.FrequencyPenalty < -2.0 || config.FrequencyPenalty > 2.0 {
		return NewConfigurationError("", "frequency_penalty", nil, "frequency penalty must be between -2.0 and 2.0")
	}

	// Validate presence penalty
	if config.PresencePenalty < -2.0 || config.PresencePenalty > 2.0 {
		return NewConfigurationError("", "presence_penalty", nil, "presence penalty must be between -2.0 and 2.0")
	}

	// Validate repeat penalty
	if config.RepeatPenalty < 0 {
		return NewConfigurationError("", "repeat_penalty", nil, "repeat penalty must be non-negative")
	}

	return nil
}

// ResetConfig resets a model's configuration to defaults
func (cm *ConfigManagerImpl) ResetConfig(ctx context.Context, modelID string) error {
	model, err := cm.registry.Get(ctx, modelID)
	if err != nil {
		return NewConfigurationError(modelID, "", err, "failed to get model")
	}

	// Get default config for the model's format
	defaultConfig := cm.GetDefaultConfig(model.Format)

	// Update the configuration
	return cm.UpdateConfig(ctx, modelID, defaultConfig)
}

// CreateTemplate creates a new configuration template
func (cm *ConfigManagerImpl) CreateTemplate(ctx context.Context, template *ConfigTemplate) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Validate template
	if err := cm.validateTemplate(template); err != nil {
		return NewConfigurationError("", "", err, "template validation failed")
	}

	// Set timestamps
	now := time.Now()
	template.CreatedAt = now
	template.UpdatedAt = now

	// Generate ID if not provided
	if template.ID == "" {
		template.ID = generateTemplateID(template.Name)
	}

	// Check if template already exists
	if _, exists := cm.templates[template.ID]; exists {
		return NewConfigurationError("", "", ErrTemplateNotFound, "template already exists")
	}

	// Save to database
	if err := cm.saveTemplate(ctx, template); err != nil {
		return NewConfigurationError("", "", err, "failed to save template")
	}

	// Cache the template
	cm.templates[template.ID] = template

	log.Printf("Created configuration template: %s", template.Name)
	return nil
}

// GetTemplate retrieves a template by ID
func (cm *ConfigManagerImpl) GetTemplate(ctx context.Context, id string) (*ConfigTemplate, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	// Check cache first
	if template, exists := cm.templates[id]; exists {
		return template, nil
	}

	// Load from database
	template, err := cm.loadTemplate(ctx, id)
	if err != nil {
		return nil, NewConfigurationError("", "", err, "failed to load template")
	}

	// Cache it
	cm.templates[id] = template

	return template, nil
}

// GetTemplateByName retrieves a template by name
func (cm *ConfigManagerImpl) GetTemplateByName(ctx context.Context, name string) (*ConfigTemplate, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	// Search in cache
	for _, template := range cm.templates {
		if template.Name == name {
			return template, nil
		}
	}

	// Load from database
	template, err := cm.loadTemplateByName(ctx, name)
	if err != nil {
		return nil, NewConfigurationError("", "", err, "failed to load template by name")
	}

	// Cache it
	cm.templates[template.ID] = template

	return template, nil
}

// ListTemplates lists all configuration templates
func (cm *ConfigManagerImpl) ListTemplates(ctx context.Context, category string) ([]*ConfigTemplate, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	// If we have templates in cache, filter and return
	if len(cm.templates) > 0 {
		templates := make([]*ConfigTemplate, 0)
		for _, template := range cm.templates {
			if category == "" || template.Category == category {
				templates = append(templates, template)
			}
		}
		return templates, nil
	}

	// Load from database
	templates, err := cm.loadTemplates(ctx, category)
	if err != nil {
		return nil, NewConfigurationError("", "", err, "failed to load templates")
	}

	// Cache them
	for _, template := range templates {
		cm.templates[template.ID] = template
	}

	return templates, nil
}

// UpdateTemplate updates an existing template
func (cm *ConfigManagerImpl) UpdateTemplate(ctx context.Context, template *ConfigTemplate) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Validate template
	if err := cm.validateTemplate(template); err != nil {
		return NewConfigurationError("", "", err, "template validation failed")
	}

	// Update timestamp
	template.UpdatedAt = time.Now()

	// Update in database
	if err := cm.updateTemplateInDB(ctx, template); err != nil {
		return NewConfigurationError("", "", err, "failed to update template in database")
	}

	// Update cache
	cm.templates[template.ID] = template

	log.Printf("Updated configuration template: %s", template.Name)
	return nil
}

// DeleteTemplate deletes a template
func (cm *ConfigManagerImpl) DeleteTemplate(ctx context.Context, id string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Check if template exists
	if _, exists := cm.templates[id]; !exists {
		return NewConfigurationError("", "", ErrTemplateNotFound, "template not found")
	}

	// Delete from database
	if err := cm.deleteTemplateFromDB(ctx, id); err != nil {
		return NewConfigurationError("", "", err, "failed to delete template from database")
	}

	// Remove from cache
	delete(cm.templates, id)

	log.Printf("Deleted configuration template: %s", id)
	return nil
}

// ApplyTemplate applies a configuration template to a model
func (cm *ConfigManagerImpl) ApplyTemplate(ctx context.Context, modelID string, templateName string, override *ModelConfig) error {
	// Get the template
	template, err := cm.GetTemplateByName(ctx, templateName)
	if err != nil {
		return NewConfigurationError(modelID, "", err, "failed to get template")
	}

	// Start with template configuration
	config := template.Config

	// Apply overrides if provided
	if override != nil {
		config = *cm.MergeConfigs(&config, override)
	}

	// Update the model's configuration
	return cm.UpdateConfig(ctx, modelID, &config)
}

// MergeConfigs merges two configurations, with override taking precedence
func (cm *ConfigManagerImpl) MergeConfigs(base *ModelConfig, override *ModelConfig) *ModelConfig {
	merged := *base // Copy base

	if override == nil {
		return &merged
	}

	// Override non-zero values
	if override.ContextLength != 0 {
		merged.ContextLength = override.ContextLength
	}
	if override.Temperature != 0 {
		merged.Temperature = override.Temperature
	}
	if override.MaxTokens != 0 {
		merged.MaxTokens = override.MaxTokens
	}
	if override.TopP != 0 {
		merged.TopP = override.TopP
	}
	if override.TopK != 0 {
		merged.TopK = override.TopK
	}
	if override.FrequencyPenalty != 0 {
		merged.FrequencyPenalty = override.FrequencyPenalty
	}
	if override.PresencePenalty != 0 {
		merged.PresencePenalty = override.PresencePenalty
	}
	if override.RepeatPenalty != 0 {
		merged.RepeatPenalty = override.RepeatPenalty
	}
	if len(override.StopTokens) > 0 {
		merged.StopTokens = override.StopTokens
	}
	if len(override.CustomParams) > 0 {
		if merged.CustomParams == nil {
			merged.CustomParams = make(map[string]interface{})
		}
		for k, v := range override.CustomParams {
			merged.CustomParams[k] = v
		}
	}

	return &merged
}

// ValidateAgainstSchema validates a configuration against a schema
func (cm *ConfigManagerImpl) ValidateAgainstSchema(config *ModelConfig, schema *ConfigSchema) error {
	if schema == nil {
		return nil
	}

	// Check required fields
	for _, field := range schema.Required {
		if !cm.isFieldSet(config, field) {
			return NewConfigurationError("", field, nil, fmt.Sprintf("required field '%s' is not set", field))
		}
	}

	// Validate each field against its schema
	for fieldName, fieldSchema := range schema.Fields {
		value := cm.getFieldValue(config, fieldName)
		if err := cm.validateField(fieldName, value, fieldSchema); err != nil {
			return err
		}
	}

	return nil
}

// GetDefaultConfig returns the default configuration for a model format
func (cm *ConfigManagerImpl) GetDefaultConfig(format ModelFormat) *ModelConfig {
	baseConfig := DefaultModelConfig()

	// Format-specific defaults
	switch format {
	case FormatGGUF:
		baseConfig.ContextLength = 4096
		baseConfig.MaxTokens = 2048
	case FormatONNX:
		baseConfig.ContextLength = 2048
		baseConfig.MaxTokens = 1024
	case FormatPyTorch:
		baseConfig.ContextLength = 2048
		baseConfig.MaxTokens = 512
	case FormatTensorFlow:
		baseConfig.ContextLength = 2048
		baseConfig.MaxTokens = 512
	default:
		// Use base defaults
	}

	return &baseConfig
}

// Private helper methods

// validateTemplate validates a configuration template
func (cm *ConfigManagerImpl) validateTemplate(template *ConfigTemplate) error {
	if template.Name == "" {
		return errors.New("template name is required")
	}

	// Validate the configuration
	if err := cm.ValidateConfig(context.Background(), &template.Config); err != nil {
		return fmt.Errorf("invalid template configuration: %w", err)
	}

	return nil
}

// updateModelConfig updates the model configuration in the database
func (cm *ConfigManagerImpl) updateModelConfig(ctx context.Context, modelID string, config *ModelConfig) error {
	query := `
		UPDATE models SET
			context_length = $2,
			temperature = $3,
			max_tokens = $4,
			top_p = $5,
			top_k = $6,
			frequency_penalty = $7,
			presence_penalty = $8,
			repeat_penalty = $9,
			updated_at = $10
		WHERE id = $1
	`

	_, err := cm.db.ExecContext(ctx, query,
		modelID,
		config.ContextLength,
		config.Temperature,
		config.MaxTokens,
		config.TopP,
		config.TopK,
		config.FrequencyPenalty,
		config.PresencePenalty,
		config.RepeatPenalty,
		time.Now(),
	)

	return err
}

// saveTemplate saves a template to the database
func (cm *ConfigManagerImpl) saveTemplate(ctx context.Context, template *ConfigTemplate) error {
	query := `
		INSERT INTO config_templates (
			id, name, description, category, is_default,
			context_length, temperature, max_tokens, top_p, top_k,
			frequency_penalty, presence_penalty, repeat_penalty,
			created_at, updated_at, created_by
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
	`

	_, err := cm.db.ExecContext(ctx, query,
		template.ID,
		template.Name,
		template.Description,
		template.Category,
		template.IsDefault,
		template.Config.ContextLength,
		template.Config.Temperature,
		template.Config.MaxTokens,
		template.Config.TopP,
		template.Config.TopK,
		template.Config.FrequencyPenalty,
		template.Config.PresencePenalty,
		template.Config.RepeatPenalty,
		template.CreatedAt,
		template.UpdatedAt,
		template.CreatedBy,
	)

	return err
}

// loadTemplate loads a template from the database by ID
func (cm *ConfigManagerImpl) loadTemplate(ctx context.Context, id string) (*ConfigTemplate, error) {
	query := `
		SELECT id, name, description, category, is_default,
			   context_length, temperature, max_tokens, top_p, top_k,
			   frequency_penalty, presence_penalty, repeat_penalty,
			   created_at, updated_at, created_by
		FROM config_templates
		WHERE id = $1
	`

	template := &ConfigTemplate{}
	err := cm.db.QueryRowContext(ctx, query, id).Scan(
		&template.ID,
		&template.Name,
		&template.Description,
		&template.Category,
		&template.IsDefault,
		&template.Config.ContextLength,
		&template.Config.Temperature,
		&template.Config.MaxTokens,
		&template.Config.TopP,
		&template.Config.TopK,
		&template.Config.FrequencyPenalty,
		&template.Config.PresencePenalty,
		&template.Config.RepeatPenalty,
		&template.CreatedAt,
		&template.UpdatedAt,
		&template.CreatedBy,
	)

	if err != nil {
		return nil, err
	}

	return template, nil
}

// loadTemplateByName loads a template from the database by name
func (cm *ConfigManagerImpl) loadTemplateByName(ctx context.Context, name string) (*ConfigTemplate, error) {
	query := `
		SELECT id, name, description, category, is_default,
			   context_length, temperature, max_tokens, top_p, top_k,
			   frequency_penalty, presence_penalty, repeat_penalty,
			   created_at, updated_at, created_by
		FROM config_templates
		WHERE name = $1
	`

	template := &ConfigTemplate{}
	err := cm.db.QueryRowContext(ctx, query, name).Scan(
		&template.ID,
		&template.Name,
		&template.Description,
		&template.Category,
		&template.IsDefault,
		&template.Config.ContextLength,
		&template.Config.Temperature,
		&template.Config.MaxTokens,
		&template.Config.TopP,
		&template.Config.TopK,
		&template.Config.FrequencyPenalty,
		&template.Config.PresencePenalty,
		&template.Config.RepeatPenalty,
		&template.CreatedAt,
		&template.UpdatedAt,
		&template.CreatedBy,
	)

	if err != nil {
		return nil, err
	}

	return template, nil
}

// loadTemplates loads templates from the database
func (cm *ConfigManagerImpl) loadTemplates(ctx context.Context, category string) ([]*ConfigTemplate, error) {
	var query string
	var args []interface{}

	if category != "" {
		query = `
			SELECT id, name, description, category, is_default,
				   context_length, temperature, max_tokens, top_p, top_k,
				   frequency_penalty, presence_penalty, repeat_penalty,
				   created_at, updated_at, created_by
			FROM config_templates
			WHERE category = $1
			ORDER BY name
		`
		args = []interface{}{category}
	} else {
		query = `
			SELECT id, name, description, category, is_default,
				   context_length, temperature, max_tokens, top_p, top_k,
				   frequency_penalty, presence_penalty, repeat_penalty,
				   created_at, updated_at, created_by
			FROM config_templates
			ORDER BY category, name
		`
		args = []interface{}{}
	}

	rows, err := cm.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	templates := make([]*ConfigTemplate, 0)
	for rows.Next() {
		template := &ConfigTemplate{}
		err := rows.Scan(
			&template.ID,
			&template.Name,
			&template.Description,
			&template.Category,
			&template.IsDefault,
			&template.Config.ContextLength,
			&template.Config.Temperature,
			&template.Config.MaxTokens,
			&template.Config.TopP,
			&template.Config.TopK,
			&template.Config.FrequencyPenalty,
			&template.Config.PresencePenalty,
			&template.Config.RepeatPenalty,
			&template.CreatedAt,
			&template.UpdatedAt,
			&template.CreatedBy,
		)
		if err != nil {
			return nil, err
		}
		templates = append(templates, template)
	}

	return templates, nil
}

// updateTemplateInDB updates a template in the database
func (cm *ConfigManagerImpl) updateTemplateInDB(ctx context.Context, template *ConfigTemplate) error {
	query := `
		UPDATE config_templates SET
			name = $2,
			description = $3,
			category = $4,
			is_default = $5,
			context_length = $6,
			temperature = $7,
			max_tokens = $8,
			top_p = $9,
			top_k = $10,
			frequency_penalty = $11,
			presence_penalty = $12,
			repeat_penalty = $13,
			updated_at = $14
		WHERE id = $1
	`

	_, err := cm.db.ExecContext(ctx, query,
		template.ID,
		template.Name,
		template.Description,
		template.Category,
		template.IsDefault,
		template.Config.ContextLength,
		template.Config.Temperature,
		template.Config.MaxTokens,
		template.Config.TopP,
		template.Config.TopK,
		template.Config.FrequencyPenalty,
		template.Config.PresencePenalty,
		template.Config.RepeatPenalty,
		template.UpdatedAt,
	)

	return err
}

// deleteTemplateFromDB deletes a template from the database
func (cm *ConfigManagerImpl) deleteTemplateFromDB(ctx context.Context, id string) error {
	query := `DELETE FROM config_templates WHERE id = $1`
	_, err := cm.db.ExecContext(ctx, query, id)
	return err
}

// isFieldSet checks if a configuration field is set
func (cm *ConfigManagerImpl) isFieldSet(config *ModelConfig, field string) bool {
	switch field {
	case "context_length":
		return config.ContextLength != 0
	case "temperature":
		return config.Temperature != 0
	case "max_tokens":
		return config.MaxTokens != 0
	case "top_p":
		return config.TopP != 0
	case "top_k":
		return config.TopK != 0
	default:
		return false
	}
}

// getFieldValue gets the value of a configuration field
func (cm *ConfigManagerImpl) getFieldValue(config *ModelConfig, field string) interface{} {
	switch field {
	case "context_length":
		return config.ContextLength
	case "temperature":
		return config.Temperature
	case "max_tokens":
		return config.MaxTokens
	case "top_p":
		return config.TopP
	case "top_k":
		return config.TopK
	case "frequency_penalty":
		return config.FrequencyPenalty
	case "presence_penalty":
		return config.PresencePenalty
	case "repeat_penalty":
		return config.RepeatPenalty
	default:
		return nil
	}
}

// validateField validates a single field against its schema
func (cm *ConfigManagerImpl) validateField(fieldName string, value interface{}, schema FieldSchema) error {
	if value == nil {
		if schema.Default != nil {
			return nil // Has default, OK
		}
		return nil // Optional field, OK
	}

	// Type validation
	switch schema.Type {
	case "int":
		if _, ok := value.(int); !ok {
			return NewConfigurationError("", fieldName, nil, fmt.Sprintf("field '%s' must be an integer", fieldName))
		}
		intVal := value.(int)
		if schema.Min != nil {
			if min, ok := schema.Min.(int); ok && intVal < min {
				return NewConfigurationError("", fieldName, nil, fmt.Sprintf("field '%s' must be at least %d", fieldName, min))
			}
		}
		if schema.Max != nil {
			if max, ok := schema.Max.(int); ok && intVal > max {
				return NewConfigurationError("", fieldName, nil, fmt.Sprintf("field '%s' must be at most %d", fieldName, max))
			}
		}
	case "float":
		if _, ok := value.(float64); !ok {
			return NewConfigurationError("", fieldName, nil, fmt.Sprintf("field '%s' must be a number", fieldName))
		}
		floatVal := value.(float64)
		if schema.Min != nil {
			if min, ok := schema.Min.(float64); ok && floatVal < min {
				return NewConfigurationError("", fieldName, nil, fmt.Sprintf("field '%s' must be at least %f", fieldName, min))
			}
		}
		if schema.Max != nil {
			if max, ok := schema.Max.(float64); ok && floatVal > max {
				return NewConfigurationError("", fieldName, nil, fmt.Sprintf("field '%s' must be at most %f", fieldName, max))
			}
		}
	case "string":
		if _, ok := value.(string); !ok {
			return NewConfigurationError("", fieldName, nil, fmt.Sprintf("field '%s' must be a string", fieldName))
		}
	}

	return nil
}

// initializeDefaultTemplates creates default configuration templates
func (cm *ConfigManagerImpl) initializeDefaultTemplates() {
	templates := []*ConfigTemplate{
		{
			ID:          "default",
			Name:        "Default",
			Description: "Default balanced configuration",
			Category:    "general",
			IsDefault:   true,
			Config: ModelConfig{
				ContextLength:     2048,
				Temperature:       0.7,
				MaxTokens:         512,
				TopP:             0.9,
				TopK:             40,
				FrequencyPenalty: 0.0,
				PresencePenalty:  0.0,
				RepeatPenalty:    1.0,
			},
		},
		{
			ID:          "creative",
			Name:        "Creative Writing",
			Description: "Configuration optimized for creative writing",
			Category:    "creative",
			IsDefault:   false,
			Config: ModelConfig{
				ContextLength:     4096,
				Temperature:       0.9,
				MaxTokens:         1024,
				TopP:             0.95,
				TopK:             50,
				FrequencyPenalty: 0.5,
				PresencePenalty:  0.5,
				RepeatPenalty:    1.2,
			},
		},
		{
			ID:          "precise",
			Name:        "Precise",
			Description: "Configuration for precise, factual responses",
			Category:    "professional",
			IsDefault:   false,
			Config: ModelConfig{
				ContextLength:     2048,
				Temperature:       0.3,
				MaxTokens:         256,
				TopP:             0.8,
				TopK:             20,
				FrequencyPenalty: 0.0,
				PresencePenalty:  0.0,
				RepeatPenalty:    1.0,
			},
		},
		{
			ID:          "chat",
			Name:        "Chat",
			Description: "Configuration optimized for conversational AI",
			Category:    "conversational",
			IsDefault:   false,
			Config: ModelConfig{
				ContextLength:     4096,
				Temperature:       0.8,
				MaxTokens:         512,
				TopP:             0.9,
				TopK:             40,
				FrequencyPenalty: 0.3,
				PresencePenalty:  0.3,
				RepeatPenalty:    1.1,
			},
		},
		{
			ID:          "code",
			Name:        "Code Generation",
			Description: "Configuration optimized for code generation",
			Category:    "technical",
			IsDefault:   false,
			Config: ModelConfig{
				ContextLength:     8192,
				Temperature:       0.2,
				MaxTokens:         2048,
				TopP:             0.95,
				TopK:             50,
				FrequencyPenalty: 0.0,
				PresencePenalty:  0.0,
				RepeatPenalty:    1.0,
			},
		},
	}

	// Cache default templates
	for _, template := range templates {
		cm.templates[template.ID] = template
	}
}

// initializeSchemas initializes configuration schemas
func (cm *ConfigManagerImpl) initializeSchemas() {
	schemas := map[string]*ConfigSchema{
		"default": {
			Name:    "default",
			Version: "1.0",
			Fields: map[string]FieldSchema{
				"context_length": {
					Type:        "int",
					Min:         1,
					Max:         1000000,
					Default:     2048,
					Description: "Maximum context length for the model",
				},
				"temperature": {
					Type:        "float",
					Min:         0.0,
					Max:         2.0,
					Default:     0.7,
					Description: "Sampling temperature",
				},
				"max_tokens": {
					Type:        "int",
					Min:         1,
					Max:         10000,
					Default:     512,
					Description: "Maximum tokens to generate",
				},
				"top_p": {
					Type:        "float",
					Min:         0.0,
					Max:         1.0,
					Default:     0.9,
					Description: "Nucleus sampling probability",
				},
				"top_k": {
					Type:        "int",
					Min:         0,
					Max:         1000,
					Default:     40,
					Description: "Top-k sampling parameter",
				},
			},
		},
	}

	cm.schemas = schemas
}

// generateTemplateID generates a unique ID for a template
func generateTemplateID(name string) string {
	timestamp := time.Now().Unix()
	return fmt.Sprintf("template-%s-%d", name, timestamp)
}

// ExportConfig exports a model configuration to JSON
func (cm *ConfigManagerImpl) ExportConfig(ctx context.Context, modelID string) (string, error) {
	config, err := cm.GetConfig(ctx, modelID)
	if err != nil {
		return "", err
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return "", NewConfigurationError(modelID, "", err, "failed to export configuration")
	}

	return string(data), nil
}

// ImportConfig imports a model configuration from JSON
func (cm *ConfigManagerImpl) ImportConfig(ctx context.Context, modelID string, jsonData string) error {
	var config ModelConfig
	if err := json.Unmarshal([]byte(jsonData), &config); err != nil {
		return NewConfigurationError(modelID, "", err, "failed to parse configuration JSON")
	}

	return cm.UpdateConfig(ctx, modelID, &config)
}
