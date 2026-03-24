package plugins

import (
	"context"
	"encoding/json"
	"time"
)

// PluginStatus represents the current status of a plugin
type PluginStatus string

const (
	StatusInstalled   PluginStatus = "installed"
	StatusEnabled     PluginStatus = "enabled"
	StatusDisabled    PluginStatus = "disabled"
	StatusRunning     PluginStatus = "running"
	StatusStopped     PluginStatus = "stopped"
	StatusError       PluginStatus = "error"
	StatusInstalling  PluginStatus = "installing"
	StatusUninstalling PluginStatus = "uninstalling"
)

// PluginType represents the type of plugin
type PluginType string

const (
	PluginTypeModel       PluginType = "model"
	PluginTypeInference   PluginType = "inference"
	PluginTypeStorage     PluginType = "storage"
	PluginTypeAuth        PluginType = "auth"
	PluginTypeMonitoring  PluginType = "monitoring"
	PluginTypeIntegration PluginType = "integration"
	PluginTypeCLI         PluginType = "cli"
	PluginTypeCustom      PluginType = "custom"
)

// Plugin represents a plugin instance
type Plugin struct {
	ID             string                 `json:"id"`
	Name           string                 `json:"name"`
	Version        string                 `json:"version"`
	Description    string                 `json:"description"`
	Type           PluginType             `json:"type"`
	Status         PluginStatus           `json:"status"`
	Enabled        bool                   `json:"enabled"`
	Manifest       *PluginManifest        `json:"manifest"`
	Config         map[string]interface{} `json:"config"`
	Permissions    []string               `json:"permissions"`
	Dependencies   []string               `json:"dependencies"`
	Path           string                 `json:"path"`
	Checksum       string                 `json:"checksum"`
	InstalledAt    time.Time              `json:"installed_at"`
	UpdatedAt      time.Time              `json:"updated_at"`
	LastStartedAt  *time.Time             `json:"last_started_at"`
	LastStoppedAt  *time.Time             `json:"last_stopped_at"`
	Error          string                 `json:"error,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

// PluginManifest describes the plugin metadata and configuration
type PluginManifest struct {
	APIVersion  string            `json:"api_version"`
	Name        string            `json:"name"`
	Version     string            `json:"version"`
	Description string            `json:"description"`
	Author      string            `json:"author"`
	License     string            `json:"license"`
	Homepage    string            `json:"homepage"`
	Repository  string            `json:"repository"`
	Type        PluginType        `json:"type"`
	Main        string            `json:"main"`
	Entrypoint  string            `json:"entrypoint"`
	Hooks       []HookDefinition  `json:"hooks"`
	Config      ConfigDefinition  `json:"config"`
	Permissions []string          `json:"permissions"`
	Requires    []Requirement     `json:"requires"`
	Provides    []string          `json:"provides"`
	Tags        []string          `json:"tags"`
	MinVersion  string            `json:"min_version"`
	MaxVersion  string            `json:"max_version"`
}

// HookDefinition defines a plugin hook
type HookDefinition struct {
	Name        string   `json:"name"`
	Type        HookType `json:"type"`
	Priority    int      `json:"priority"`
	Description string   `json:"description"`
}

// HookType represents the type of hook
type HookType string

const (
	HookPreStart      HookType = "pre_start"
	HookPostStart     HookType = "post_start"
	HookPreStop       HookType = "pre_stop"
	HookPostStop      HookType = "post_stop"
	HookPreInstall    HookType = "pre_install"
	HookPostInstall   HookType = "post_install"
	HookPreUninstall  HookType = "pre_uninstall"
	HookPostUninstall HookType = "post_uninstall"
	HookPreUpdate     HookType = "pre_update"
	HookPostUpdate    HookType = "post_update"
	HookPreEnable     HookType = "pre_enable"
	HookPostEnable    HookType = "post_enable"
	HookPreDisable    HookType = "pre_disable"
	HookPostDisable   HookType = "post_disable"
)

// ConfigDefinition defines plugin configuration schema
type ConfigDefinition struct {
	Schema  map[string]ConfigField `json:"schema"`
	Default map[string]interface{} `json:"default"`
}

// ConfigField defines a configuration field
type ConfigField struct {
	Type        string      `json:"type"`
	Required    bool        `json:"required"`
	Default     interface{} `json:"default"`
	Description string      `json:"description"`
	Validation  string      `json:"validation"`
	Options     []string    `json:"options"`
	Sensitive   bool        `json:"sensitive"`
}

// Requirement defines a plugin requirement
type Requirement struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Type    string `json:"type"`
}

// PluginInterface defines the interface that all plugins must implement
type PluginInterface interface {
	// Initialize initializes the plugin
	Initialize(ctx context.Context, config map[string]interface{}) error

	// Start starts the plugin
	Start(ctx context.Context) error

	// Stop stops the plugin
	Stop(ctx context.Context) error

	// Configure configures the plugin
	Configure(ctx context.Context, config map[string]interface{}) error

	// GetInfo returns plugin information
	GetInfo() *PluginInfo

	// HealthCheck performs a health check
	HealthCheck(ctx context.Context) error

	// Cleanup cleans up plugin resources
	Cleanup(ctx context.Context) error
}

// PluginInfo contains runtime plugin information
type PluginInfo struct {
	ID      string                 `json:"id"`
	Name    string                 `json:"name"`
	Version string                 `json:"version"`
	Type    PluginType             `json:"type"`
	Status  PluginStatus           `json:"status"`
	Metrics map[string]interface{} `json:"metrics"`
}

// PluginEvent represents a plugin event
type PluginEvent struct {
	ID        string                 `json:"id"`
	PluginID  string                 `json:"plugin_id"`
	Type      EventType              `json:"type"`
	Timestamp time.Time              `json:"timestamp"`
	Data      map[string]interface{} `json:"data"`
	Error     string                 `json:"error,omitempty"`
}

// EventType represents plugin event types
type EventType string

const (
	EventInstalled   EventType = "installed"
	EventUninstalled EventType = "uninstalled"
	EventEnabled     EventType = "enabled"
	EventDisabled    EventType = "disabled"
	EventStarted     EventType = "started"
	EventStopped     EventType = "stopped"
	EventError       EventType = "error"
	EventUpdated     EventType = "updated"
	EventConfigured  EventType = "configured"
)

// PluginInstallOptions defines options for plugin installation
type PluginInstallOptions struct {
	Source      string                 `json:"source"`
	Version     string                 `json:"version"`
	Config      map[string]interface{} `json:"config"`
	Verify      bool                   `json:"verify"`
	EnableAfter bool                   `json:"enable_after"`
	Force       bool                   `json:"force"`
}

// PluginUpdateOptions defines options for plugin updates
type PluginUpdateOptions struct {
	Version string                 `json:"version"`
	Config  map[string]interface{} `json:"config"`
	Force   bool                   `json:"force"`
}

// PluginFilter defines filters for listing plugins
type PluginFilter struct {
	Status   []PluginStatus `json:"status"`
	Type     []PluginType   `json:"type"`
	Enabled  *bool          `json:"enabled"`
	Tags     []string       `json:"tags"`
	Search   string         `json:"search"`
	Page     int            `json:"page"`
	PageSize int            `json:"page_size"`
}

// PluginList represents a paginated list of plugins
type PluginList struct {
	Plugins    []Plugin `json:"plugins"`
	Total      int      `json:"total"`
	Page       int      `json:"page"`
	PageSize   int      `json:"page_size"`
	TotalPages int      `json:"total_pages"`
}

// MarketplacePlugin represents a plugin in the marketplace
type MarketplacePlugin struct {
	ID             string            `json:"id"`
	Name           string            `json:"name"`
	Version        string            `json:"version"`
	Description    string            `json:"description"`
	Author         string            `json:"author"`
	Downloads      int64             `json:"downloads"`
	Rating         float64           `json:"rating"`
	Reviews        int64             `json:"reviews"`
	Tags           []string          `json:"tags"`
	Categories     []string          `json:"categories"`
	Homepage       string            `json:"homepage"`
	Repository     string            `json:"repository"`
	Versions       []string          `json:"versions"`
	LatestVersion  string            `json:"latest_version"`
	PublishedAt    time.Time         `json:"published_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
	Verified       bool              `json:"verified"`
	Official       bool              `json:"official"`
	Screenshots    []string          `json:"screenshots"`
	Documentation  string            `json:"documentation"`
	License        string            `json:"license"`
	Compatibility  map[string]string `json:"compatibility"`
}

// MarketplaceSearchFilter defines filters for marketplace search
type MarketplaceSearchFilter struct {
	Query      string   `json:"query"`
	Type       string   `json:"type"`
	Category   string   `json:"category"`
	Tags       []string `json:"tags"`
	Verified   *bool    `json:"verified"`
	Official   *bool    `json:"official"`
	SortBy     string   `json:"sort_by"`
	SortOrder  string   `json:"sort_order"`
	Page       int      `json:"page"`
	PageSize   int      `json:"page_size"`
}

// PluginSandboxConfig defines sandbox configuration
type PluginSandboxConfig struct {
	Enabled           bool           `json:"enabled"`
	NetworkAccess     bool           `json:"network_access"`
	FileSystemAccess  []string       `json:"filesystem_access"`
	EnvironmentVars   []string       `json:"environment_vars"`
	ResourceLimits    ResourceLimits `json:"resource_limits"`
	AllowedSyscalls   []string       `json:"allowed_syscalls"`
	SeccompProfile    string         `json:"seccomp_profile"`
	AppArmorProfile   string         `json:"apparmor_profile"`
}

// ResourceLimits defines resource limits for plugins
type ResourceLimits struct {
	MaxMemoryMB     int64  `json:"max_memory_mb"`
	MaxCPUPercent   int    `json:"max_cpu_percent"`
	MaxGoroutines   int    `json:"max_goroutines"`
	MaxFileSizeMB   int64  `json:"max_file_size_mb"`
	MaxNetworkConns int    `json:"max_network_conns"`
	TimeoutSeconds  int    `json:"timeout_seconds"`
}

// PluginLog represents a plugin log entry
type PluginLog struct {
	ID        string    `json:"id"`
	PluginID  string    `json:"plugin_id"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
	Metadata  map[string]interface{} `json:"metadata"`
}

// PluginMetric represents a plugin metric
type PluginMetric struct {
	PluginID  string                 `json:"plugin_id"`
	Name      string                 `json:"name"`
	Value     interface{}            `json:"value"`
	Type      string                 `json:"type"`
	Timestamp time.Time              `json:"timestamp"`
	Tags      map[string]string      `json:"tags"`
}

// PluginHookResult represents the result of a hook execution
type PluginHookResult struct {
	PluginID string                 `json:"plugin_id"`
	Success  bool                   `json:"success"`
	Error    string                 `json:"error,omitempty"`
	Data     map[string]interface{} `json:"data,omitempty"`
	Duration time.Duration          `json:"duration"`
}

// PluginAPIRequest represents an API request to a plugin
type PluginAPIRequest struct {
	PluginID string                 `json:"plugin_id"`
	Method   string                 `json:"method"`
	Path     string                 `json:"path"`
	Headers  map[string]string      `json:"headers"`
	Body     json.RawMessage        `json:"body"`
	Params   map[string]interface{} `json:"params"`
	Context  context.Context        `json:"-"`
}

// PluginAPIResponse represents an API response from a plugin
type PluginAPIResponse struct {
	StatusCode int                    `json:"status_code"`
	Headers    map[string]string      `json:"headers"`
	Body       json.RawMessage        `json:"body"`
	Data       map[string]interface{} `json:"data,omitempty"`
	Error      string                 `json:"error,omitempty"`
}

// PluginPermission represents plugin permissions
type PluginPermission struct {
	Resource string `json:"resource"`
	Action   string `json:"action"`
	Scope    string `json:"scope"`
}

// PluginDependency represents a plugin dependency
type PluginDependency struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	Required  bool   `json:"required"`
	Installed bool   `json:"installed"`
}

// PluginValidationResult represents plugin validation result
type PluginValidationResult struct {
	Valid    bool     `json:"valid"`
	Errors   []string `json:"errors"`
	Warnings []string `json:"warnings"`
}

// PluginInstallResult represents plugin installation result
type PluginInstallResult struct {
	PluginID string    `json:"plugin_id"`
	Success  bool      `json:"success"`
	Error    string    `json:"error,omitempty"`
	Warnings []string  `json:"warnings,omitempty"`
	Duration time.Duration `json:"duration"`
}
