package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// ValidationError represents a configuration validation error
type ValidationError struct {
	Field   string
	Message string
	Value   interface{}
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation error for field '%s': %s (value: %v)", e.Field, e.Message, e.Value)
}

// ValidationErrors is a collection of validation errors
type ValidationErrors []*ValidationError

func (e ValidationErrors) Error() string {
	if len(e) == 0 {
		return "no validation errors"
	}

	var messages []string
	for _, err := range e {
		messages = append(messages, err.Error())
	}
	return strings.Join(messages, "; ")
}

// Validator validates configuration settings
type Validator struct {
	errors ValidationErrors
	config *Config
}

// Config and related types are defined in manager.go

// NewValidator creates a new configuration validator
func NewValidator(cfg *Config) *Validator {
	return &Validator{
		config: cfg,
		errors: make(ValidationErrors, 0),
	}
}

// Validate performs all validation checks
func (v *Validator) Validate() ValidationErrors {
	v.validateSystem()
	v.validateCompute()
	v.validateModels()
	v.validateStorage()
	v.validateAPI()
	v.validateLogging()
	v.validateMonitoring()
	v.validateContainer()
	v.validateSecurity()

	return v.errors
}

// addError adds a validation error
func (v *Validator) addError(field, message string, value interface{}) {
	v.errors = append(v.errors, &ValidationError{
		Field:   field,
		Message: message,
		Value:   value,
	})
}

// validateSystem validates system configuration
func (v *Validator) validateSystem() {
	// Validate host
	if v.config.System.Host == "" {
		v.addError("system.host", "host cannot be empty", v.config.System.Host)
	}

	// Validate port
	if v.config.System.Port < 1 || v.config.System.Port > 65535 {
		v.addError("system.port", "port must be between 1 and 65535", v.config.System.Port)
	}

	// Validate workers
	if v.config.System.Workers < 1 {
		v.addError("system.workers", "workers must be at least 1", v.config.System.Workers)
	}

	// Validate timeouts
	if v.config.System.ShutdownTimeout < 1*time.Second {
		v.addError("system.shutdown_timeout", "shutdown timeout must be at least 1 second", v.config.System.ShutdownTimeout)
	}

	if v.config.System.ReadTimeout < 1*time.Second {
		v.addError("system.read_timeout", "read timeout must be at least 1 second", v.config.System.ReadTimeout)
	}

	if v.config.System.WriteTimeout < 1*time.Second {
		v.addError("system.write_timeout", "write timeout must be at least 1 second", v.config.System.WriteTimeout)
	}
}

// validateCompute validates compute configuration
func (v *Validator) validateCompute() {
	// Validate GPU devices if GPU is enabled
	if v.config.Compute.GPUEnabled {
		if len(v.config.Compute.GPUDevices) == 0 {
			v.addError("compute.gpu_devices", "at least one GPU device must be specified when GPU is enabled", v.config.Compute.GPUDevices)
		}

		for _, device := range v.config.Compute.GPUDevices {
			if device < 0 {
				v.addError("compute.gpu_devices", "GPU device ID cannot be negative", device)
			}
		}
	}

	// Validate CPU threads
	if v.config.Compute.CPUThreads < 1 {
		v.addError("compute.cpu_threads", "CPU threads must be at least 1", v.config.Compute.CPUThreads)
	}

	// Validate memory limit
	if v.config.Compute.MemoryLimit != "" {
		if !isValidMemorySize(v.config.Compute.MemoryLimit) {
			v.addError("compute.memory_limit", "invalid memory limit format (use format like 16GB, 512MB)", v.config.Compute.MemoryLimit)
		}
	}

	// Validate batch size
	if v.config.Compute.BatchSize < 1 {
		v.addError("compute.batch_size", "batch size must be at least 1", v.config.Compute.BatchSize)
	}
}

// validateModels validates model configuration
func (v *Validator) validateModels() {
	// Validate max concurrent models
	if v.config.Models.MaxConcurrent < 1 {
		v.addError("models.max_concurrent", "max concurrent models must be at least 1", v.config.Models.MaxConcurrent)
	}

	// Validate scale threshold
	if v.config.Models.ScaleThreshold < 0 || v.config.Models.ScaleThreshold > 1 {
		v.addError("models.scale_threshold", "scale threshold must be between 0 and 1", v.config.Models.ScaleThreshold)
	}

	// Validate idle timeout
	if v.config.Models.IdleTimeout < 0 {
		v.addError("models.idle_timeout", "idle timeout cannot be negative", v.config.Models.IdleTimeout)
	}

	// Validate paths
	if v.config.Models.RegistryPath != "" {
		if err := validateDirectoryPath(v.config.Models.RegistryPath); err != nil {
			v.addError("models.registry_path", err.Error(), v.config.Models.RegistryPath)
		}
	}

	if v.config.Models.DownloadPath != "" {
		if err := validateDirectoryPath(v.config.Models.DownloadPath); err != nil {
			v.addError("models.download_path", err.Error(), v.config.Models.DownloadPath)
		}
	}

	if v.config.Models.TempPath != "" {
		if err := validateDirectoryPath(v.config.Models.TempPath); err != nil {
			v.addError("models.temp_path", err.Error(), v.config.Models.TempPath)
		}
	}
}

// validateStorage validates storage configuration
func (v *Validator) validateStorage() {
	// Validate models path
	if v.config.Storage.ModelsPath != "" {
		if err := validateDirectoryPath(v.config.Storage.ModelsPath); err != nil {
			v.addError("storage.models_path", err.Error(), v.config.Storage.ModelsPath)
		}
	}

	// Validate cache size
	if v.config.Storage.CacheSize != "" {
		if !isValidMemorySize(v.config.Storage.CacheSize) {
			v.addError("storage.cache_size", "invalid cache size format (use format like 50GB, 512MB)", v.config.Storage.CacheSize)
		}
	}

	// Validate database configuration
	v.validateDatabase()

	// Validate cache configuration
	v.validateCache()
}

// validateDatabase validates database configuration
func (v *Validator) validateDatabase() {
	db := v.config.Storage.Database

	// Validate database type
	validDBTypes := []string{"postgres", "mysql", "sqlite", "mongodb"}
	if !contains(validDBTypes, db.Type) {
		v.addError("storage.database.type", "unsupported database type", db.Type)
	}

	// Validate host
	if db.Host == "" {
		v.addError("storage.database.host", "database host cannot be empty", db.Host)
	}

	// Validate port
	if db.Port < 1 || db.Port > 65535 {
		v.addError("storage.database.port", "database port must be between 1 and 65535", db.Port)
	}

	// Validate database name
	if db.Name == "" {
		v.addError("storage.database.name", "database name cannot be empty", db.Name)
	}

	// Validate user
	if db.User == "" {
		v.addError("storage.database.user", "database user cannot be empty", db.User)
	}

	// Validate SSL mode
	validSSLModes := []string{"disable", "require", "verify-ca", "verify-full"}
	if !contains(validSSLModes, db.SSLMode) {
		v.addError("storage.database.sslmode", "invalid SSL mode", db.SSLMode)
	}

	// Validate max connections
	if db.MaxConnections < 1 {
		v.addError("storage.database.max_connections", "max connections must be at least 1", db.MaxConnections)
	}
}

// validateCache validates cache configuration
func (v *Validator) validateCache() {
	cache := v.config.Storage.Cache

	// Validate cache type
	validCacheTypes := []string{"redis", "memcached", "memory"}
	if !contains(validCacheTypes, cache.Type) {
		v.addError("storage.cache.type", "unsupported cache type", cache.Type)
	}

	// Validate host
	if cache.Host == "" {
		v.addError("storage.cache.host", "cache host cannot be empty", cache.Host)
	}

	// Validate port
	if cache.Port < 1 || cache.Port > 65535 {
		v.addError("storage.cache.port", "cache port must be between 1 and 65535", cache.Port)
	}

	// Validate DB
	if cache.DB < 0 {
		v.addError("storage.cache.db", "cache database number cannot be negative", cache.DB)
	}

	// Validate pool size
	if cache.PoolSize < 1 {
		v.addError("storage.cache.pool_size", "pool size must be at least 1", cache.PoolSize)
	}
}

// validateAPI validates API configuration
func (v *Validator) validateAPI() {
	// Validate rate limit
	if v.config.API.RateLimit < 0 {
		v.addError("api.rate_limit", "rate limit cannot be negative", v.config.API.RateLimit)
	}

	// Validate CORS origins
	for _, origin := range v.config.API.CORSOrigins {
		if origin != "*" {
			if _, err := url.Parse(origin); err != nil {
				v.addError("api.cors_origins", "invalid CORS origin URL", origin)
			}
		}
	}

	// Validate JWT secret if auth is enabled
	if v.config.API.AuthEnabled {
		if v.config.API.JWTSecret == "" || v.config.API.JWTSecret == "your-secret-key-change-in-production" {
			v.addError("api.jwt_secret", "JWT secret must be set and changed from default when auth is enabled", v.config.API.JWTSecret)
		}
		if len(v.config.API.JWTSecret) < 32 {
			v.addError("api.jwt_secret", "JWT secret should be at least 32 characters for security", len(v.config.API.JWTSecret))
		}
	}

	// Validate API key header
	if v.config.API.APIKeyHeader == "" {
		v.addError("api.api_key_header", "API key header name cannot be empty", v.config.API.APIKeyHeader)
	}

	// Validate max request size (if set, must be positive)
	if v.config.API.MaxRequestSize < 0 {
		v.addError("api.max_request_size", "max request size cannot be negative", v.config.API.MaxRequestSize)
	}
}

// validateLogging validates logging configuration
func (v *Validator) validateLogging() {
	// Validate log level
	validLogLevels := []string{"DEBUG", "INFO", "WARN", "ERROR", "FATAL"}
	if !contains(validLogLevels, strings.ToUpper(v.config.Logging.Level)) {
		v.addError("logging.level", "invalid log level", v.config.Logging.Level)
	}

	// Validate log format
	validLogFormats := []string{"json", "text"}
	if !contains(validLogFormats, strings.ToLower(v.config.Logging.Format)) {
		v.addError("logging.format", "invalid log format", v.config.Logging.Format)
	}

	// Validate log file path
	if v.config.Logging.File != "" {
		dir := filepath.Dir(v.config.Logging.File)
		if err := validateDirectoryPath(dir); err != nil {
			v.addError("logging.file", "log file directory is not accessible", dir)
		}
	}

	// Validate max size (if set, must be positive)
	if v.config.Logging.MaxSize < 0 {
		v.addError("logging.max_size", "max size cannot be negative", v.config.Logging.MaxSize)
	}

	// Validate max backups
	if v.config.Logging.MaxBackups < 0 {
		v.addError("logging.max_backups", "max backups cannot be negative", v.config.Logging.MaxBackups)
	}

	// Validate max age
	if v.config.Logging.MaxAge < 0 {
		v.addError("logging.max_age", "max age cannot be negative", v.config.Logging.MaxAge)
	}

	// Validate output paths
	for _, path := range v.config.Logging.OutputPaths {
		if path != "stdout" && path != "stderr" {
			if err := validateDirectoryPath(filepath.Dir(path)); err != nil {
				v.addError("logging.output_paths", "output path directory is not accessible", path)
			}
		}
	}
}

// validateMonitoring validates monitoring configuration
func (v *Validator) validateMonitoring() {
	// Validate metrics interval
	if v.config.Monitoring.MetricsInterval < 1*time.Second {
		v.addError("monitoring.metrics_interval", "metrics interval must be at least 1 second", v.config.Monitoring.MetricsInterval)
	}

	// Validate metrics path
	if v.config.Monitoring.MetricsPath == "" {
		v.addError("monitoring.metrics_path", "metrics path cannot be empty", v.config.Monitoring.MetricsPath)
	}

	if !strings.HasPrefix(v.config.Monitoring.MetricsPath, "/") {
		v.addError("monitoring.metrics_path", "metrics path must start with /", v.config.Monitoring.MetricsPath)
	}

	// Validate health check path
	if v.config.Monitoring.HealthCheckPath == "" {
		v.addError("monitoring.health_check_path", "health check path cannot be empty", v.config.Monitoring.HealthCheckPath)
	}

	if !strings.HasPrefix(v.config.Monitoring.HealthCheckPath, "/") {
		v.addError("monitoring.health_check_path", "health check path must start with /", v.config.Monitoring.HealthCheckPath)
	}

	// Validate health check interval
	if v.config.Monitoring.HealthCheckInterval < 1*time.Second {
		v.addError("monitoring.health_check_interval", "health check interval must be at least 1 second", v.config.Monitoring.HealthCheckInterval)
	}
}

// validateContainer validates container configuration
func (v *Validator) validateContainer() {
	// Validate runtime
	validRuntimes := []string{"docker", "podman", "containerd"}
	if !contains(validRuntimes, v.config.Container.Runtime) {
		v.addError("container.runtime", "unsupported container runtime", v.config.Container.Runtime)
	}

	// Validate network name
	if v.config.Container.Network == "" {
		v.addError("container.network", "network name cannot be empty", v.config.Container.Network)
	}

	// Validate base image
	if v.config.Container.BaseImage == "" {
		v.addError("container.base_image", "base image cannot be empty", v.config.Container.BaseImage)
	}

	// Validate model template
	if v.config.Container.ModelTemplate == "" {
		v.addError("container.model_template", "model template cannot be empty", v.config.Container.ModelTemplate)
	}

	// Validate resource limits
	v.validateResourceLimits()
}

// validateResourceLimits validates resource limit configuration
func (v *Validator) validateResourceLimits() {
	limits := v.config.Container.ResourceLimits

	// Validate CPU limit
	if limits.CPU < 0 {
		v.addError("container.resource_limits.cpu", "CPU limit cannot be negative", limits.CPU)
	}

	// Validate memory limit
	if limits.Memory != "" {
		if !isValidMemorySize(limits.Memory) {
			v.addError("container.resource_limits.memory", "invalid memory limit format", limits.Memory)
		}
	}

	// Validate GPU limit
	if limits.GPU < 0 {
		v.addError("container.resource_limits.gpu", "GPU limit cannot be negative", limits.GPU)
	}
}

// validateSecurity validates security configuration
func (v *Validator) validateSecurity() {
	// Validate TLS configuration
	if v.config.Security.TLSEnabled {
		if v.config.Security.CertFile == "" {
			v.addError("security.cert_file", "certificate file path is required when TLS is enabled", v.config.Security.CertFile)
		} else {
			if _, err := os.Stat(v.config.Security.CertFile); os.IsNotExist(err) {
				v.addError("security.cert_file", "certificate file does not exist", v.config.Security.CertFile)
			}
		}

		if v.config.Security.KeyFile == "" {
			v.addError("security.key_file", "key file path is required when TLS is enabled", v.config.Security.KeyFile)
		} else {
			if _, err := os.Stat(v.config.Security.KeyFile); os.IsNotExist(err) {
				v.addError("security.key_file", "key file does not exist", v.config.Security.KeyFile)
			}
		}
	}

	// Validate allowed hosts
	for _, host := range v.config.Security.AllowedHosts {
		if host == "" {
			v.addError("security.allowed_hosts", "host cannot be empty", host)
		}
	}

	// Validate trusted proxies
	for _, proxy := range v.config.Security.TrustedProxies {
		if proxy == "" {
			v.addError("security.trusted_proxies", "proxy cannot be empty", proxy)
		}
		// Could add IP/CIDR validation here
	}
}

// Helper functions

// isValidMemorySize validates memory size format (e.g., 16GB, 512MB)
func isValidMemorySize(size string) bool {
	matched, _ := regexp.MatchString(`^\d+(KB|MB|GB|TB)$`, strings.ToUpper(size))
	return matched
}

// validateDirectoryPath checks if a directory path is valid and accessible
func validateDirectoryPath(path string) error {
	// Check if path is absolute or relative
	if !filepath.IsAbs(path) {
		// Convert to absolute path
		absPath, err := filepath.Abs(path)
		if err != nil {
			return fmt.Errorf("failed to resolve absolute path: %w", err)
		}
		path = absPath
	}

	// Check if directory exists
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		// Try to create the directory
		if err := os.MkdirAll(path, 0755); err != nil {
			return fmt.Errorf("directory does not exist and cannot be created: %w", err)
		}
		return nil
	}

	if err != nil {
		return fmt.Errorf("cannot access directory: %w", err)
	}

	if !info.IsDir() {
		return fmt.Errorf("path is not a directory")
	}

	return nil
}

// contains checks if a string slice contains a specific string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if strings.EqualFold(s, item) {
			return true
		}
	}
	return false
}

// ValidateConfig is a convenience function that validates a configuration
func ValidateConfig(cfg *Config) error {
	validator := NewValidator(cfg)
	errors := validator.Validate()

	if len(errors) > 0 {
		return errors
	}

	return nil
}
