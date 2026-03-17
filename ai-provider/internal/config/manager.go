package config

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/spf13/viper"
)

// Manager handles application configuration
type Manager struct {
	viper  *viper.Viper
	config *Config
	mu     sync.RWMutex
}

// Config represents the complete application configuration
type Config struct {
	System    SystemConfig    `mapstructure:"system"`
	Compute   ComputeConfig   `mapstructure:"compute"`
	Models    ModelsConfig    `mapstructure:"models"`
	Storage   StorageConfig   `mapstructure:"storage"`
	API       APIConfig       `mapstructure:"api"`
	Logging   LoggingConfig   `mapstructure:"logging"`
	Monitoring MonitoringConfig `mapstructure:"monitoring"`
	Container ContainerConfig `mapstructure:"container"`
	Security  SecurityConfig  `mapstructure:"security"`
}

// SystemConfig holds system-level configuration
type SystemConfig struct {
	Host           string        `mapstructure:"host"`
	Port           int           `mapstructure:"port"`
	Workers        int           `mapstructure:"workers"`
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`
	ReadTimeout    time.Duration `mapstructure:"read_timeout"`
	WriteTimeout   time.Duration `mapstructure:"write_timeout"`
}

// ComputeConfig holds compute resource configuration
type ComputeConfig struct {
	GPUEnabled  bool     `mapstructure:"gpu_enabled"`
	GPUDevices  []int    `mapstructure:"gpu_devices"`
	CPUThreads  int      `mapstructure:"cpu_threads"`
	MemoryLimit string   `mapstructure:"memory_limit"`
	BatchSize   int      `mapstructure:"batch_size"`
}

// ModelsConfig holds model management configuration
type ModelsConfig struct {
	MaxConcurrent  int           `mapstructure:"max_concurrent"`
	AutoScale      bool          `mapstructure:"auto_scale"`
	ScaleThreshold float64       `mapstructure:"scale_threshold"`
	IdleTimeout    time.Duration `mapstructure:"idle_timeout"`
	RegistryPath   string        `mapstructure:"registry_path"`
	DownloadPath   string        `mapstructure:"download_path"`
	TempPath       string        `mapstructure:"temp_path"`
}

// StorageConfig holds storage configuration
type StorageConfig struct {
	ModelsPath string        `mapstructure:"models_path"`
	CacheSize  string        `mapstructure:"cache_size"`
	Database   DatabaseConfig `mapstructure:"database"`
	Cache      CacheConfig   `mapstructure:"cache"`
}

// DatabaseConfig holds database connection configuration
type DatabaseConfig struct {
	Type          string `mapstructure:"type"`
	Host          string `mapstructure:"host"`
	Port          int    `mapstructure:"port"`
	Name          string `mapstructure:"name"`
	User          string `mapstructure:"user"`
	Password      string `mapstructure:"password"`
	SSLMode       string `mapstructure:"sslmode"`
	MaxConnections int   `mapstructure:"max_connections"`
}

// CacheConfig holds cache configuration
type CacheConfig struct {
	Type     string `mapstructure:"type"`
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
	PoolSize int    `mapstructure:"pool_size"`
}

// APIConfig holds API configuration
type APIConfig struct {
	RateLimit      int      `mapstructure:"rate_limit"`
	AuthEnabled    bool     `mapstructure:"auth_enabled"`
	CORSOrigins    []string `mapstructure:"cors_origins"`
	JWTSecret      string   `mapstructure:"jwt_secret"`
	APIKeyHeader   string   `mapstructure:"api_key_header"`
	MaxRequestSize int64    `mapstructure:"max_request_size"`
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level       string   `mapstructure:"level"`
	Format      string   `mapstructure:"format"`
	File        string   `mapstructure:"file"`
	MaxSize     int      `mapstructure:"max_size"`
	MaxBackups  int      `mapstructure:"max_backups"`
	MaxAge      int      `mapstructure:"max_age"`
	Compress    bool     `mapstructure:"compress"`
	OutputPaths []string `mapstructure:"output_paths"`
}

// MonitoringConfig holds monitoring configuration
type MonitoringConfig struct {
	PrometheusEnabled  bool          `mapstructure:"prometheus_enabled"`
	MetricsInterval    time.Duration `mapstructure:"metrics_interval"`
	MetricsPath        string        `mapstructure:"metrics_path"`
	HealthCheckPath    string        `mapstructure:"health_check_path"`
	HealthCheckInterval time.Duration `mapstructure:"health_check_interval"`
}

// ContainerConfig holds container runtime configuration
type ContainerConfig struct {
	Runtime       string            `mapstructure:"runtime"`
	Network       string            `mapstructure:"network"`
	BaseImage     string            `mapstructure:"base_image"`
	ModelTemplate string            `mapstructure:"model_template"`
	ResourceLimits ResourceLimitsConfig `mapstructure:"resource_limits"`
}

// ResourceLimitsConfig holds container resource limits
type ResourceLimitsConfig struct {
	CPU    int    `mapstructure:"cpu"`
	Memory string `mapstructure:"memory"`
	GPU    int    `mapstructure:"gpu"`
}

// SecurityConfig holds security configuration
type SecurityConfig struct {
	TLSEnabled    bool     `mapstructure:"tls_enabled"`
	CertFile      string   `mapstructure:"cert_file"`
	KeyFile       string   `mapstructure:"key_file"`
	AllowedHosts  []string `mapstructure:"allowed_hosts"`
	TrustedProxies []string `mapstructure:"trusted_proxies"`
}

// NewManager creates a new configuration manager
func NewManager() *Manager {
	return &Manager{
		viper: viper.New(),
	}
}

// Load loads configuration from file and environment variables
func (m *Manager) Load(configPath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Set default values
	m.setDefaults()

	// Configure Viper
	m.viper.SetConfigName("config")
	m.viper.SetConfigType("yaml")

	if configPath != "" {
		m.viper.SetConfigFile(configPath)
	} else {
		m.viper.AddConfigPath("./configs")
		m.viper.AddConfigPath("/etc/ai-provider")
		m.viper.AddConfigPath(".")
	}

	// Enable environment variable override
	m.viper.SetEnvPrefix("AI_PROVIDER")
	m.viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	m.viper.AutomaticEnv()

	// Read configuration file
	if err := m.viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return fmt.Errorf("error reading config file: %w", err)
		}
		// Config file not found, will use defaults and env vars
	}

	// Unmarshal configuration
	var config Config
	if err := m.viper.Unmarshal(&config); err != nil {
		return fmt.Errorf("error unmarshaling config: %w", err)
	}

	// Validate configuration
	if err := m.validate(&config); err != nil {
		return fmt.Errorf("config validation failed: %w", err)
	}

	m.config = &config
	return nil
}

// setDefaults sets default configuration values
func (m *Manager) setDefaults() {
	// System defaults
	m.viper.SetDefault("system.host", "0.0.0.0")
	m.viper.SetDefault("system.port", 8080)
	m.viper.SetDefault("system.workers", 4)
	m.viper.SetDefault("system.shutdown_timeout", "30s")
	m.viper.SetDefault("system.read_timeout", "30s")
	m.viper.SetDefault("system.write_timeout", "30s")

	// Compute defaults
	m.viper.SetDefault("compute.gpu_enabled", false)
	m.viper.SetDefault("compute.gpu_devices", []int{0})
	m.viper.SetDefault("compute.cpu_threads", 8)
	m.viper.SetDefault("compute.memory_limit", "16GB")
	m.viper.SetDefault("compute.batch_size", 32)

	// Models defaults
	m.viper.SetDefault("models.max_concurrent", 5)
	m.viper.SetDefault("models.auto_scale", true)
	m.viper.SetDefault("models.scale_threshold", 0.8)
	m.viper.SetDefault("models.idle_timeout", "300s")
	m.viper.SetDefault("models.registry_path", "./configs/models")
	m.viper.SetDefault("models.download_path", "/models")
	m.viper.SetDefault("models.temp_path", "/tmp/ai-provider")

	// Storage defaults
	m.viper.SetDefault("storage.models_path", "/models")
	m.viper.SetDefault("storage.cache_size", "50GB")
	m.viper.SetDefault("storage.database.type", "postgres")
	m.viper.SetDefault("storage.database.host", "localhost")
	m.viper.SetDefault("storage.database.port", 5432)
	m.viper.SetDefault("storage.database.name", "aiprovider")
	m.viper.SetDefault("storage.database.user", "admin")
	m.viper.SetDefault("storage.database.password", "secret")
	m.viper.SetDefault("storage.database.sslmode", "disable")
	m.viper.SetDefault("storage.database.max_connections", 20)
	m.viper.SetDefault("storage.cache.type", "redis")
	m.viper.SetDefault("storage.cache.host", "localhost")
	m.viper.SetDefault("storage.cache.port", 6379)
	m.viper.SetDefault("storage.cache.password", "")
	m.viper.SetDefault("storage.cache.db", 0)
	m.viper.SetDefault("storage.cache.pool_size", 10)

	// API defaults
	m.viper.SetDefault("api.rate_limit", 100)
	m.viper.SetDefault("api.auth_enabled", false)
	m.viper.SetDefault("api.cors_origins", []string{"*"})
	m.viper.SetDefault("api.jwt_secret", "change-me-in-production")
	m.viper.SetDefault("api.api_key_header", "X-API-Key")
	m.viper.SetDefault("api.max_request_size", 10485760) // 10MB

	// Logging defaults
	m.viper.SetDefault("logging.level", "INFO")
	m.viper.SetDefault("logging.format", "json")
	m.viper.SetDefault("logging.file", "/var/log/ai-provider.log")
	m.viper.SetDefault("logging.max_size", 100)
	m.viper.SetDefault("logging.max_backups", 5)
	m.viper.SetDefault("logging.max_age", 30)
	m.viper.SetDefault("logging.compress", true)
	m.viper.SetDefault("logging.output_paths", []string{"stdout"})

	// Monitoring defaults
	m.viper.SetDefault("monitoring.prometheus_enabled", true)
	m.viper.SetDefault("monitoring.metrics_interval", "15s")
	m.viper.SetDefault("monitoring.metrics_path", "/metrics")
	m.viper.SetDefault("monitoring.health_check_path", "/health")
	m.viper.SetDefault("monitoring.health_check_interval", "30s")

	// Container defaults
	m.viper.SetDefault("container.runtime", "docker")
	m.viper.SetDefault("container.network", "ai-provider-network")
	m.viper.SetDefault("container.base_image", "ai-provider-core:latest")
	m.viper.SetDefault("container.model_template", "model-{name}-{version}")
	m.viper.SetDefault("container.resource_limits.cpu", 2)
	m.viper.SetDefault("container.resource_limits.memory", "4GB")
	m.viper.SetDefault("container.resource_limits.gpu", 1)

	// Security defaults
	m.viper.SetDefault("security.tls_enabled", false)
	m.viper.SetDefault("security.cert_file", "/etc/ssl/certs/server.crt")
	m.viper.SetDefault("security.key_file", "/etc/ssl/private/server.key")
	m.viper.SetDefault("security.allowed_hosts", []string{})
	m.viper.SetDefault("security.trusted_proxies", []string{})
}

// validate validates the configuration
func (m *Manager) validate(config *Config) error {
	// Validate system configuration
	if config.System.Port < 1 || config.System.Port > 65535 {
		return fmt.Errorf("invalid port number: %d", config.System.Port)
	}
	if config.System.Workers < 1 {
		return fmt.Errorf("workers must be at least 1")
	}

	// Validate compute configuration
	if config.Compute.CPUThreads < 1 {
		return fmt.Errorf("CPU threads must be at least 1")
	}

	// Validate models configuration
	if config.Models.MaxConcurrent < 1 {
		return fmt.Errorf("max concurrent models must be at least 1")
	}
	if config.Models.ScaleThreshold < 0 || config.Models.ScaleThreshold > 1 {
		return fmt.Errorf("scale threshold must be between 0 and 1")
	}

	// Validate API configuration
	if config.API.RateLimit < 1 {
		return fmt.Errorf("rate limit must be at least 1")
	}

	// Validate logging level
	validLogLevels := map[string]bool{"DEBUG": true, "INFO": true, "WARN": true, "ERROR": true}
	if !validLogLevels[strings.ToUpper(config.Logging.Level)] {
		return fmt.Errorf("invalid log level: %s", config.Logging.Level)
	}

	return nil
}

// Get returns the current configuration
func (m *Manager) Get() *Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.config
}

// GetString gets a string configuration value
func (m *Manager) GetString(key string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.viper.GetString(key)
}

// GetInt gets an integer configuration value
func (m *Manager) GetInt(key string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.viper.GetInt(key)
}

// GetBool gets a boolean configuration value
func (m *Manager) GetBool(key string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.viper.GetBool(key)
}

// GetDuration gets a duration configuration value
func (m *Manager) GetDuration(key string) time.Duration {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.viper.GetDuration(key)
}

// GetStringSlice gets a string slice configuration value
func (m *Manager) GetStringSlice(key string) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.viper.GetStringSlice(key)
}

// Set sets a configuration value at runtime
func (m *Manager) Set(key string, value interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.viper.Set(key, value)

	// Re-unmarshal to update the config struct
	var config Config
	if err := m.viper.Unmarshal(&config); err != nil {
		return fmt.Errorf("error unmarshaling config: %w", err)
	}

	// Validate new configuration
	if err := m.validate(&config); err != nil {
		return fmt.Errorf("config validation failed: %w", err)
	}

	m.config = &config
	return nil
}

// Reload reloads the configuration from the file
func (m *Manager) Reload() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.viper.ReadInConfig(); err != nil {
		return fmt.Errorf("error reading config file: %w", err)
	}

	var config Config
	if err := m.viper.Unmarshal(&config); err != nil {
		return fmt.Errorf("error unmarshaling config: %w", err)
	}

	if err := m.validate(&config); err != nil {
		return fmt.Errorf("config validation failed: %w", err)
	}

	m.config = &config
	return nil
}

// GetDatabaseConnectionString returns the database connection string
func (m *Manager) GetDatabaseConnectionString() string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cfg := m.config.Storage.Database
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.Name, cfg.SSLMode,
	)
}

// GetRedisAddr returns the Redis address
func (m *Manager) GetRedisAddr() string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cfg := m.config.Storage.Cache
	return fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
}

// IsAuthEnabled returns whether authentication is enabled
func (m *Manager) IsAuthEnabled() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.config.API.AuthEnabled
}

// IsPrometheusEnabled returns whether Prometheus metrics are enabled
func (m *Manager) IsPrometheusEnabled() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.config.Monitoring.PrometheusEnabled
}

// IsGPUEnabled returns whether GPU acceleration is enabled
func (m *Manager) IsGPUEnabled() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.config.Compute.GPUEnabled
}
