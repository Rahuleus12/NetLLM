// internal/tenant/config.go
// Tenant-specific configuration management
// Handles tenant-level settings, preferences, and feature flags

package tenant

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// TenantConfig represents tenant-specific configuration
type TenantConfig struct {
	// Core Settings
	TenantID     string                 `json:"tenant_id"`
	Name         string                 `json:"name"`
	Slug         string                 `json:"slug"`
	Environment  string                 `json:"environment"` // dev, staging, production
	Status       string                 `json:"status"`       // active, suspended, deleted

	// Feature Flags
	Features FeatureFlags `json:"features"`

	// Resource Limits
	Quotas ResourceQuotas `json:"quotas"`

	// Custom Settings
	Settings map[string]interface{} `json:"settings"`

	// Branding
	Branding BrandingConfig `json:"branding"`

	// Metadata
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	CreatedBy string    `json:"created_by"`
	UpdatedBy string    `json:"updated_by"`
}

// FeatureFlags represents enabled/disabled features for a tenant
type FeatureFlags struct {
	// Core Features
	ModelManagement bool `json:"model_management"`
	Inference       bool `json:"inference"`
	BatchInference  bool `json:"batch_inference"`
	Streaming       bool `json:"streaming"`

	// Advanced Features
	GPUAcceleration bool `json:"gpu_acceleration"`
	AutoScaling     bool `json:"auto_scaling"`
	Caching         bool `json:"caching"`
	Monitoring      bool `json:"monitoring"`

	// Enterprise Features
	MultiRegion      bool `json:"multi_region"`
	DedicatedSupport bool `json:"dedicated_support"`
	CustomBranding   bool `json:"custom_branding"`
	APIAccess        bool `json:"api_access"`
	Webhooks         bool `json:"webhooks"`
}

// ResourceQuotas represents resource limits for a tenant
type ResourceQuotas struct {
	// Model Limits
	MaxModels            int64 `json:"max_models"`
	MaxModelSizeGB       int64 `json:"max_model_size_gb"`
	TotalStorageGB       int64 `json:"total_storage_gb"`

	// Inference Limits
	MaxConcurrentRequests int `json:"max_concurrent_requests"`
	MaxBatchSize         int  `json:"max_batch_size"`
	MaxTokensPerRequest  int  `json:"max_tokens_per_request"`

	// Compute Limits
	MaxGPUs              int `json:"max_gpus"`
	MaxGPUHoursPerMonth  int64 `json:"max_gpu_hours_per_month"`
	MaxCPUPercentage     int  `json:"max_cpu_percentage"`

	// API Limits
	MaxAPIRequestsPerDay int `json:"max_api_requests_per_day"`
	MaxAPIRequestsPerMin int `json:"max_api_requests_per_min"`

	// Rate Limits
	RequestRateLimit int `json:"request_rate_limit"` // requests per second
	TokenRateLimit   int `json:"token_rate_limit"`   // tokens per second
}

// BrandingConfig represents tenant branding settings
type BrandingConfig struct {
	LogoURL      string `json:"logo_url"`
	PrimaryColor string `json:"primary_color"`
	CustomDomain string `json:"custom_domain"`
	CompanyName  string `json:"company_name"`
	SupportEmail string `json:"support_email"`
	WebsiteURL   string `json:"website_url"`
}

// ConfigManager manages tenant configurations
type ConfigManager struct {
	// Default configuration template
	defaultConfig *TenantConfig

	// Configuration validation rules
	validationRules map[string]ValidationRule
}

// ValidationRule defines how to validate a configuration field
type ValidationRule struct {
	Required bool
	Min      interface{}
	Max      interface{}
	Enum     []string
	Pattern  string
}

// NewConfigManager creates a new configuration manager
func NewConfigManager() *ConfigManager {
	return &ConfigManager{
		defaultConfig:   getDefaultConfig(),
		validationRules: getValidationRules(),
	}
}

// CreateConfig creates a new tenant configuration
func (cm *ConfigManager) CreateConfig(tenantID, name, slug, environment string, createdBy string) (*TenantConfig, error) {
	config := &TenantConfig{
		TenantID:    tenantID,
		Name:        name,
		Slug:        slug,
		Environment: environment,
		Status:      "active",
		Features:    cm.defaultConfig.Features,
		Quotas:      cm.defaultConfig.Quotas,
		Settings:    make(map[string]interface{}),
		Branding:    cm.defaultConfig.Branding,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		CreatedBy:   createdBy,
		UpdatedBy:   createdBy,
	}

	// Copy default settings
	for k, v := range cm.defaultConfig.Settings {
		config.Settings[k] = v
	}

	if err := cm.ValidateConfig(config); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	return config, nil
}

// GetDefaultConfig returns the default configuration template
func (cm *ConfigManager) GetDefaultConfig() *TenantConfig {
	configCopy := *cm.defaultConfig
	return &configCopy
}

// ValidateConfig validates a tenant configuration
func (cm *ConfigManager) ValidateConfig(config *TenantConfig) error {
	// Validate required fields
	if config.TenantID == "" {
		return errors.New("tenant_id is required")
	}
	if config.Name == "" {
		return errors.New("name is required")
	}
	if config.Slug == "" {
		return errors.New("slug is required")
	}

	// Validate status
	validStatuses := []string{"active", "suspended", "deleted"}
	if !contains(validStatuses, config.Status) {
		return fmt.Errorf("invalid status: %s", config.Status)
	}

	// Validate environment
	validEnvs := []string{"dev", "staging", "production"}
	if !contains(validEnvs, config.Environment) {
		return fmt.Errorf("invalid environment: %s", config.Environment)
	}

	// Validate quotas
	if config.Quotas.MaxModels < 0 {
		return errors.New("max_models cannot be negative")
	}
	if config.Quotas.MaxModelSizeGB < 0 {
		return errors.New("max_model_size_gb cannot be negative")
	}
	if config.Quotas.MaxConcurrentRequests < 0 {
		return errors.New("max_concurrent_requests cannot be negative")
	}
	if config.Quotas.MaxBatchSize < 0 {
		return errors.New("max_batch_size cannot be negative")
	}
	if config.Quotas.MaxTokensPerRequest < 0 {
		return errors.New("max_tokens_per_request cannot be negative")
	}
	if config.Quotas.MaxGPUs < 0 {
		return errors.New("max_gpus cannot be negative")
	}

	// Validate rate limits
	if config.Quotas.RequestRateLimit < 0 {
		return errors.New("request_rate_limit cannot be negative")
	}
	if config.Quotas.TokenRateLimit < 0 {
		return errors.New("token_rate_limit cannot be negative")
	}

	return nil
}

// UpdateConfig updates tenant configuration
func (cm *ConfigManager) UpdateConfig(config *TenantConfig, updates map[string]interface{}, updatedBy string) error {
	// Apply updates
	for key, value := range updates {
		switch key {
		case "name":
			if name, ok := value.(string); ok {
				config.Name = name
			}
		case "status":
			if status, ok := value.(string); ok {
				config.Status = status
			}
		case "environment":
			if env, ok := value.(string); ok {
				config.Environment = env
			}
		case "features":
			if features, ok := value.(FeatureFlags); ok {
				config.Features = features
			}
		case "quotas":
			if quotas, ok := value.(ResourceQuotas); ok {
				config.Quotas = quotas
			}
		case "branding":
			if branding, ok := value.(BrandingConfig); ok {
				config.Branding = branding
			}
		case "settings":
			if settings, ok := value.(map[string]interface{}); ok {
				// Merge settings
				for k, v := range settings {
					config.Settings[k] = v
				}
			}
		}
	}

	config.UpdatedAt = time.Now()
	config.UpdatedBy = updatedBy

	return cm.ValidateConfig(config)
}

// MergeConfigs merges two configurations with priority to override
func (cm *ConfigManager) MergeConfigs(base, override *TenantConfig) *TenantConfig {
	result := *base

	// Override fields if set in override config
	if override.Name != "" {
		result.Name = override.Name
	}
	if override.Slug != "" {
		result.Slug = override.Slug
	}
	if override.Environment != "" {
		result.Environment = override.Environment
	}
	if override.Status != "" {
		result.Status = override.Status
	}

	// Merge feature flags
	if override.Features.ModelManagement {
		result.Features.ModelManagement = true
	}
	if override.Features.Inference {
		result.Features.Inference = true
	}
	if override.Features.BatchInference {
		result.Features.BatchInference = true
	}
	if override.Features.Streaming {
		result.Features.Streaming = true
	}
	if override.Features.GPUAcceleration {
		result.Features.GPUAcceleration = true
	}
	if override.Features.AutoScaling {
		result.Features.AutoScaling = true
	}
	if override.Features.Caching {
		result.Features.Caching = true
	}
	if override.Features.Monitoring {
		result.Features.Monitoring = true
	}
	if override.Features.MultiRegion {
		result.Features.MultiRegion = true
	}
	if override.Features.DedicatedSupport {
		result.Features.DedicatedSupport = true
	}
	if override.Features.CustomBranding {
		result.Features.CustomBranding = true
	}
	if override.Features.APIAccess {
		result.Features.APIAccess = true
	}
	if override.Features.Webhooks {
		result.Features.Webhooks = true
	}

	// Merge quotas (use override values if non-zero)
	if override.Quotas.MaxModels > 0 {
		result.Quotas.MaxModels = override.Quotas.MaxModels
	}
	if override.Quotas.MaxModelSizeGB > 0 {
		result.Quotas.MaxModelSizeGB = override.Quotas.MaxModelSizeGB
	}
	if override.Quotas.TotalStorageGB > 0 {
		result.Quotas.TotalStorageGB = override.Quotas.TotalStorageGB
	}
	if override.Quotas.MaxConcurrentRequests > 0 {
		result.Quotas.MaxConcurrentRequests = override.Quotas.MaxConcurrentRequests
	}
	if override.Quotas.MaxBatchSize > 0 {
		result.Quotas.MaxBatchSize = override.Quotas.MaxBatchSize
	}
	if override.Quotas.MaxTokensPerRequest > 0 {
		result.Quotas.MaxTokensPerRequest = override.Quotas.MaxTokensPerRequest
	}
	if override.Quotas.MaxGPUs > 0 {
		result.Quotas.MaxGPUs = override.Quotas.MaxGPUs
	}
	if override.Quotas.MaxGPUHoursPerMonth > 0 {
		result.Quotas.MaxGPUHoursPerMonth = override.Quotas.MaxGPUHoursPerMonth
	}
	if override.Quotas.MaxCPUPercentage > 0 {
		result.Quotas.MaxCPUPercentage = override.Quotas.MaxCPUPercentage
	}
	if override.Quotas.MaxAPIRequestsPerDay > 0 {
		result.Quotas.MaxAPIRequestsPerDay = override.Quotas.MaxAPIRequestsPerDay
	}
	if override.Quotas.MaxAPIRequestsPerMin > 0 {
		result.Quotas.MaxAPIRequestsPerMin = override.Quotas.MaxAPIRequestsPerMin
	}
	if override.Quotas.RequestRateLimit > 0 {
		result.Quotas.RequestRateLimit = override.Quotas.RequestRateLimit
	}
	if override.Quotas.TokenRateLimit > 0 {
		result.Quotas.TokenRateLimit = override.Quotas.TokenRateLimit
	}

	// Merge branding
	if override.Branding.LogoURL != "" {
		result.Branding.LogoURL = override.Branding.LogoURL
	}
	if override.Branding.PrimaryColor != "" {
		result.Branding.PrimaryColor = override.Branding.PrimaryColor
	}
	if override.Branding.CustomDomain != "" {
		result.Branding.CustomDomain = override.Branding.CustomDomain
	}
	if override.Branding.CompanyName != "" {
		result.Branding.CompanyName = override.Branding.CompanyName
	}
	if override.Branding.SupportEmail != "" {
		result.Branding.SupportEmail = override.Branding.SupportEmail
	}
	if override.Branding.WebsiteURL != "" {
		result.Branding.WebsiteURL = override.Branding.WebsiteURL
	}

	// Merge settings
	if override.Settings != nil {
		for k, v := range override.Settings {
			result.Settings[k] = v
		}
	}

	return &result
}

// SerializeConfig serializes configuration to JSON
func (cm *ConfigManager) SerializeConfig(config *TenantConfig) ([]byte, error) {
	return json.MarshalIndent(config, "", "  ")
}

// DeserializeConfig deserializes configuration from JSON
func (cm *ConfigManager) DeserializeConfig(data []byte) (*TenantConfig, error) {
	var config TenantConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to deserialize config: %w", err)
	}

	if err := cm.ValidateConfig(&config); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	return &config, nil
}

// GetFeature returns the value of a feature flag
func (c *TenantConfig) GetFeature(feature string) bool {
	switch feature {
	case "model_management":
		return c.Features.ModelManagement
	case "inference":
		return c.Features.Inference
	case "batch_inference":
		return c.Features.BatchInference
	case "streaming":
		return c.Features.Streaming
	case "gpu_acceleration":
		return c.Features.GPUAcceleration
	case "auto_scaling":
		return c.Features.AutoScaling
	case "caching":
		return c.Features.Caching
	case "monitoring":
		return c.Features.Monitoring
	case "multi_region":
		return c.Features.MultiRegion
	case "dedicated_support":
		return c.Features.DedicatedSupport
	case "custom_branding":
		return c.Features.CustomBranding
	case "api_access":
		return c.Features.APIAccess
	case "webhooks":
		return c.Features.Webhooks
	default:
		return false
	}
}

// SetFeature sets the value of a feature flag
func (c *TenantConfig) SetFeature(feature string, enabled bool) {
	switch feature {
	case "model_management":
		c.Features.ModelManagement = enabled
	case "inference":
		c.Features.Inference = enabled
	case "batch_inference":
		c.Features.BatchInference = enabled
	case "streaming":
		c.Features.Streaming = enabled
	case "gpu_acceleration":
		c.Features.GPUAcceleration = enabled
	case "auto_scaling":
		c.Features.AutoScaling = enabled
	case "caching":
		c.Features.Caching = enabled
	case "monitoring":
		c.Features.Monitoring = enabled
	case "multi_region":
		c.Features.MultiRegion = enabled
	case "dedicated_support":
		c.Features.DedicatedSupport = enabled
	case "custom_branding":
		c.Features.CustomBranding = enabled
	case "api_access":
		c.Features.APIAccess = enabled
	case "webhooks":
		c.Features.Webhooks = enabled
	}
}

// GetSetting retrieves a custom setting value
func (c *TenantConfig) GetSetting(key string) (interface{}, bool) {
	val, exists := c.Settings[key]
	return val, exists
}

// SetSetting sets a custom setting value
func (c *TenantConfig) SetSetting(key string, value interface{}) {
	if c.Settings == nil {
		c.Settings = make(map[string]interface{})
	}
	c.Settings[key] = value
}

// getDefaultConfig returns the default tenant configuration
func getDefaultConfig() *TenantConfig {
	return &TenantConfig{
		Environment: "production",
		Status:      "active",
		Features: FeatureFlags{
			ModelManagement: true,
			Inference:       true,
			BatchInference:  true,
			Streaming:       true,
			GPUAcceleration: false,
			AutoScaling:     false,
			Caching:         true,
			Monitoring:      true,
			MultiRegion:     false,
			DedicatedSupport: false,
			CustomBranding:  false,
			APIAccess:       true,
			Webhooks:        false,
		},
		Quotas: ResourceQuotas{
			MaxModels:            50,
			MaxModelSizeGB:      100,
			TotalStorageGB:      500,
			MaxConcurrentRequests: 100,
			MaxBatchSize:         50,
			MaxTokensPerRequest:  8192,
			MaxGPUs:              4,
			MaxGPUHoursPerMonth:  720,
			MaxCPUPercentage:     80,
			MaxAPIRequestsPerDay: 10000,
			MaxAPIRequestsPerMin: 100,
			RequestRateLimit:     10,
			TokenRateLimit:       1000,
		},
		Settings: make(map[string]interface{}),
		Branding: BrandingConfig{
			LogoURL:      "",
			PrimaryColor: "#007bff",
			CustomDomain: "",
			CompanyName:  "",
			SupportEmail: "",
			WebsiteURL:   "",
		},
	}
}

// getValidationRules returns configuration validation rules
func getValidationRules() map[string]ValidationRule {
	return map[string]ValidationRule{
		"tenant_id":    {Required: true},
		"name":         {Required: true, Min: 1, Max: 255},
		"slug":         {Required: true, Min: 1, Max: 100, Pattern: "^[a-z0-9-]+$"},
		"environment":  {Required: true, Enum: []string{"dev", "staging", "production"}},
		"status":       {Required: true, Enum: []string{"active", "suspended", "deleted"}},
		"max_models":   {Required: true, Min: 0, Max: 1000},
		"max_gpus":     {Required: true, Min: 0, Max: 128},
	}
}

// contains checks if a string exists in a slice
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
