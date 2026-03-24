package integrations

import (
	"context"
	"time"
)

// IntegrationStatus represents the current status of an integration
type IntegrationStatus string

const (
	StatusActive      IntegrationStatus = "active"
	StatusInactive    IntegrationStatus = "inactive"
	StatusConnected   IntegrationStatus = "connected"
	StatusDisconnected IntegrationStatus = "disconnected"
	StatusError       IntegrationStatus = "error"
	StatusSyncing     IntegrationStatus = "syncing"
	StatusPending     IntegrationStatus = "pending"
	StatusConfiguring IntegrationStatus = "configuring"
)

// IntegrationType represents the type of integration
type IntegrationType string

const (
	TypeDatabase    IntegrationType = "database"
	TypeAPI         IntegrationType = "api"
	TypeCloudStorage IntegrationType = "cloud_storage"
	TypeMessaging   IntegrationType = "messaging"
	TypeMonitoring  IntegrationType = "monitoring"
	TypeVCS         IntegrationType = "vcs"
	TypeCI          IntegrationType = "ci"
	TypeCD          IntegrationType = "cd"
	TypeCRM         IntegrationType = "crm"
	TypeAnalytics   IntegrationType = "analytics"
	TypePayment     IntegrationType = "payment"
	TypeEmail       IntegrationType = "email"
	TypeCustom      IntegrationType = "custom"
)

// Integration represents an integration instance
type Integration struct {
	ID              string                 `json:"id"`
	Name            string                 `json:"name"`
	Type            IntegrationType        `json:"type"`
	Provider        string                 `json:"provider"`
	Status          IntegrationStatus      `json:"status"`
	Description     string                 `json:"description"`
	Config          map[string]interface{} `json:"config"`
	Credentials     *CredentialConfig      `json:"credentials"`
	TemplateID      string                 `json:"template_id"`
	Enabled         bool                   `json:"enabled"`
	AutoSync        bool                   `json:"auto_sync"`
	SyncInterval    int                    `json:"sync_interval"`
	LastSyncAt      *time.Time             `json:"last_sync_at"`
	NextSyncAt      *time.Time             `json:"next_sync_at"`
	LastSyncStatus  string                 `json:"last_sync_status"`
	SyncError       string                 `json:"sync_error"`
	Metadata        map[string]interface{} `json:"metadata"`
	Tags            []string               `json:"tags"`
	CreatedAt       time.Time              `json:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at"`
	CreatedBy       string                 `json:"created_by"`
	OrganizationID  string                 `json:"organization_id"`
	WorkspaceID     string                 `json:"workspace_id"`
	HealthStatus    *HealthStatus          `json:"health_status"`
	RateLimit       *RateLimitConfig       `json:"rate_limit"`
	RetryPolicy     *RetryPolicy           `json:"retry_policy"`
	Timeout         int                    `json:"timeout"`
}

// CredentialConfig stores integration credentials
type CredentialConfig struct {
	Type        string                 `json:"type"`
	AuthType    string                 `json:"auth_type"`
	Data        map[string]interface{} `json:"data"`
	Encrypted   bool                   `json:"encrypted"`
	ExpiresAt   *time.Time             `json:"expires_at"`
	RefreshToken string                `json:"refresh_token,omitempty"`
	TokenURL    string                 `json:"token_url,omitempty"`
}

// HealthStatus represents the health status of an integration
type HealthStatus struct {
	Status          string            `json:"status"`
	LastChecked     time.Time         `json:"last_checked"`
	ResponseTime    int64             `json:"response_time"`
	ErrorRate       float64           `json:"error_rate"`
	SuccessRate     float64           `json:"success_rate"`
	LastError       string            `json:"last_error"`
	LastErrorAt     *time.Time        `json:"last_error_at"`
	ConsecutiveFails int              `json:"consecutive_fails"`
	Metadata        map[string]interface{} `json:"metadata"`
}

// RateLimitConfig defines rate limiting configuration
type RateLimitConfig struct {
	Enabled       bool `json:"enabled"`
	RequestsPerMin int `json:"requests_per_min"`
	RequestsPerHour int `json:"requests_per_hour"`
	BurstLimit    int  `json:"burst_limit"`
	RetryAfter    int  `json:"retry_after"`
}

// RetryPolicy defines retry behavior
type RetryPolicy struct {
	MaxRetries     int           `json:"max_retries"`
	InitialDelay   int           `json:"initial_delay"`
	MaxDelay       int           `json:"max_delay"`
	Multiplier     float64       `json:"multiplier"`
	RetryOnErrors  []string      `json:"retry_on_errors"`
	RetryOnStatus  []int         `json:"retry_on_status"`
}

// Connector defines the interface for integration connectors
type Connector interface {
	// Initialize initializes the connector
	Initialize(ctx context.Context, config map[string]interface{}) error

	// Connect establishes connection to the integration
	Connect(ctx context.Context) error

	// Disconnect closes the connection
	Disconnect(ctx context.Context) error

	// TestConnection tests the connection
	TestConnection(ctx context.Context) error

	// Sync performs data synchronization
	Sync(ctx context.Context, opts SyncOptions) (*SyncResult, error)

	// Validate validates the configuration
	Validate(ctx context.Context, config map[string]interface{}) error

	// GetSchema returns the data schema
	GetSchema(ctx context.Context) (*DataSchema, error)

	// GetInfo returns connector information
	GetInfo() *ConnectorInfo

	// HealthCheck performs a health check
	HealthCheck(ctx context.Context) (*HealthStatus, error)

	// Discover discovers available resources
	Discover(ctx context.Context) ([]Resource, error)
}

// ConnectorInfo contains connector metadata
type ConnectorInfo struct {
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	Version         string            `json:"version"`
	Type            IntegrationType   `json:"type"`
	Provider        string            `json:"provider"`
	Description     string            `json:"description"`
	Documentation   string            `json:"documentation"`
	AuthTypes       []string          `json:"auth_types"`
	ConfigSchema    map[string]interface{} `json:"config_schema"`
	SupportedSyncs  []SyncType        `json:"supported_syncs"`
	Capabilities    []string          `json:"capabilities"`
	Icon            string            `json:"icon"`
	Category        string            `json:"category"`
	Tags            []string          `json:"tags"`
}

// IntegrationTemplate defines a reusable integration template
type IntegrationTemplate struct {
	ID              string                 `json:"id"`
	Name            string                 `json:"name"`
	Type            IntegrationType        `json:"type"`
	Provider        string                 `json:"provider"`
	Description     string                 `json:"description"`
	ConfigTemplate  map[string]interface{} `json:"config_template"`
	AuthConfig      *AuthConfigTemplate    `json:"auth_config"`
	DefaultConfig   map[string]interface{} `json:"default_config"`
	RequiredFields  []string               `json:"required_fields"`
	OptionalFields  []string               `json:"optional_fields"`
	Documentation   string                 `json:"documentation"`
	Examples        []TemplateExample      `json:"examples"`
	Icon            string                 `json:"icon"`
	Category        string                 `json:"category"`
	Tags            []string               `json:"tags"`
	Verified        bool                   `json:"verified"`
	Official        bool                   `json:"official"`
	Popularity      int                    `json:"popularity"`
	CreatedAt       time.Time              `json:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at"`
}

// AuthConfigTemplate defines authentication configuration template
type AuthConfigTemplate struct {
	AuthType    string                 `json:"auth_type"`
	Fields      []AuthField            `json:"fields"`
	OAuthConfig *OAuthConfigTemplate   `json:"oauth_config"`
}

// AuthField defines an authentication field
type AuthField struct {
	Name         string      `json:"name"`
	Type         string      `json:"type"`
	Label        string      `json:"label"`
	Required     bool        `json:"required"`
	Sensitive    bool        `json:"sensitive"`
	Default      interface{} `json:"default"`
	Description  string      `json:"description"`
	Placeholder  string      `json:"placeholder"`
	Validation   string      `json:"validation"`
}

// OAuthConfigTemplate defines OAuth configuration
type OAuthConfigTemplate struct {
	AuthorizeURL    string   `json:"authorize_url"`
	TokenURL        string   `json:"token_url"`
	Scopes          []string `json:"scopes"`
	AdditionalParams map[string]string `json:"additional_params"`
}

// TemplateExample provides an example configuration
type TemplateExample struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Config      map[string]interface{} `json:"config"`
}

// SyncOptions defines synchronization options
type SyncOptions struct {
	Direction       SyncDirection     `json:"direction"`
	Mode            SyncMode          `json:"mode"`
	Resources       []string          `json:"resources"`
	Incremental     bool              `json:"incremental"`
	LastSyncToken   string            `json:"last_sync_token"`
	Filters         map[string]interface{} `json:"filters"`
	TransformConfig *TransformConfig  `json:"transform_config"`
	BatchSize       int               `json:"batch_size"`
	Timeout         int               `json:"timeout"`
	DryRun          bool              `json:"dry_run"`
}

// SyncDirection defines the direction of synchronization
type SyncDirection string

const (
	SyncBidirectional SyncDirection = "bidirectional"
	SyncToExternal    SyncDirection = "to_external"
	SyncFromExternal  SyncDirection = "from_external"
)

// SyncMode defines the synchronization mode
type SyncMode string

const (
	SyncModeFull      SyncMode = "full"
	SyncModeIncremental SyncMode = "incremental"
	SyncModeDelta      SyncMode = "delta"
)

// SyncType defines types of synchronization
type SyncType string

const (
	SyncTypeData      SyncType = "data"
	SyncTypeSchema    SyncType = "schema"
	SyncTypeConfig    SyncType = "config"
	SyncTypeEvent     SyncType = "event"
	SyncTypeFile      SyncType = "file"
)

// SyncResult represents the result of a synchronization
type SyncResult struct {
	IntegrationID    string                 `json:"integration_id"`
	Success          bool                   `json:"success"`
	Status           string                 `json:"status"`
	StartedAt        time.Time              `json:"started_at"`
	CompletedAt      *time.Time             `json:"completed_at"`
	Duration         int64                  `json:"duration"`
	RecordsProcessed int64                  `json:"records_processed"`
	RecordsCreated   int64                  `json:"records_created"`
	RecordsUpdated   int64                  `json:"records_updated"`
	RecordsDeleted   int64                  `json:"records_deleted"`
	RecordsFailed    int64                  `json:"records_failed"`
	RecordsSkipped   int64                  `json:"records_skipped"`
	BytesTransferred int64                  `json:"bytes_transferred"`
	SyncToken        string                 `json:"sync_token"`
	NextSyncToken    string                 `json:"next_sync_token"`
	Errors           []SyncError            `json:"errors"`
	Warnings         []string               `json:"warnings"`
	Metadata         map[string]interface{} `json:"metadata"`
}

// SyncError represents a synchronization error
type SyncError struct {
	Resource  string                 `json:"resource"`
	RecordID  string                 `json:"record_id"`
	Operation string                 `json:"operation"`
	Error     string                 `json:"error"`
	Code      string                 `json:"code"`
	Timestamp time.Time              `json:"timestamp"`
	Metadata  map[string]interface{} `json:"metadata"`
}

// TransformConfig defines data transformation configuration
type TransformConfig struct {
	FieldMappings  []FieldMapping         `json:"field_mappings"`
	Transformations []Transformation      `json:"transformations"`
	Validations    []Validation          `json:"validations"`
	Filters        []Filter              `json:"filters"`
}

// FieldMapping defines field mapping between systems
type FieldMapping struct {
	SourceField    string      `json:"source_field"`
	TargetField    string      `json:"target_field"`
	Transformation string      `json:"transformation"`
	DefaultValue   interface{} `json:"default_value"`
	Required       bool        `json:"required"`
}

// Transformation defines a data transformation
type Transformation struct {
	Type        string                 `json:"type"`
	Field       string                 `json:"field"`
	Expression  string                 `json:"expression"`
	Params      map[string]interface{} `json:"params"`
}

// Validation defines a validation rule
type Validation struct {
	Field   string      `json:"field"`
	Rule    string      `json:"rule"`
	Value   interface{} `json:"value"`
	Message string      `json:"message"`
}

// Filter defines a data filter
type Filter struct {
	Field    string      `json:"field"`
	Operator string      `json:"operator"`
	Value    interface{} `json:"value"`
}

// DataSchema represents the schema of data
type DataSchema struct {
	Name       string            `json:"name"`
	Type       string            `json:"type"`
	Fields     []SchemaField     `json:"fields"`
	PrimaryKey []string          `json:"primary_key"`
	Indexes    []SchemaIndex     `json:"indexes"`
	Metadata   map[string]interface{} `json:"metadata"`
}

// SchemaField defines a field in the schema
type SchemaField struct {
	Name         string      `json:"name"`
	Type         string      `json:"type"`
	Nullable     bool        `json:"nullable"`
	Default      interface{} `json:"default"`
	Description  string      `json:"description"`
	Constraints  []string    `json:"constraints"`
}

// SchemaIndex defines an index in the schema
type SchemaIndex struct {
	Name    string   `json:"name"`
	Fields  []string `json:"fields"`
	Unique  bool     `json:"unique"`
}

// Resource represents a discoverable resource
type Resource struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Type        string                 `json:"type"`
	Description string                 `json:"description"`
	Schema      *DataSchema            `json:"schema"`
	Config      map[string]interface{} `json:"config"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// IntegrationEvent represents an integration event
type IntegrationEvent struct {
	ID             string                 `json:"id"`
	IntegrationID  string                 `json:"integration_id"`
	Type           EventType              `json:"type"`
	Timestamp      time.Time              `json:"timestamp"`
	Data           map[string]interface{} `json:"data"`
	Error          string                 `json:"error,omitempty"`
	Source         string                 `json:"source"`
	CorrelationID  string                 `json:"correlation_id"`
}

// EventType represents integration event types
type EventType string

const (
	EventConnected       EventType = "connected"
	EventDisconnected    EventType = "disconnected"
	EventSyncStarted     EventType = "sync_started"
	EventSyncCompleted   EventType = "sync_completed"
	EventSyncFailed      EventType = "sync_failed"
	EventHealthCheck     EventType = "health_check"
	EventConfigChanged   EventType = "config_changed"
	EventEnabled         EventType = "enabled"
	EventDisabled        EventType = "disabled"
	EventError           EventType = "error"
	EventDataReceived    EventType = "data_received"
	EventDataSent        EventType = "data_sent"
)

// IntegrationLog represents a log entry
type IntegrationLog struct {
	ID             string                 `json:"id"`
	IntegrationID  string                 `json:"integration_id"`
	Level          string                 `json:"level"`
	Message        string                 `json:"message"`
	Timestamp      time.Time              `json:"timestamp"`
	Context        map[string]interface{} `json:"context"`
	CorrelationID  string                 `json:"correlation_id"`
}

// IntegrationMetric represents integration metrics
type IntegrationMetric struct {
	IntegrationID      string            `json:"integration_id"`
	Timestamp          time.Time         `json:"timestamp"`
	RequestCount       int64             `json:"request_count"`
	SuccessCount       int64             `json:"success_count"`
	ErrorCount         int64             `json:"error_count"`
	AvgResponseTime    int64             `json:"avg_response_time"`
	DataTransferred    int64             `json:"data_transferred"`
	SyncCount          int64             `json:"sync_count"`
	LastSyncDuration   int64             `json:"last_sync_duration"`
	HealthScore        float64           `json:"health_score"`
	CustomMetrics      map[string]interface{} `json:"custom_metrics"`
}

// IntegrationFilter defines filters for listing integrations
type IntegrationFilter struct {
	Status   []IntegrationStatus `json:"status"`
	Type     []IntegrationType   `json:"type"`
	Provider []string            `json:"provider"`
	Enabled  *bool               `json:"enabled"`
	Tags     []string            `json:"tags"`
	Search   string              `json:"search"`
	Page     int                 `json:"page"`
	PageSize int                 `json:"page_size"`
}

// IntegrationList represents a paginated list of integrations
type IntegrationList struct {
	Integrations []Integration `json:"integrations"`
	Total        int           `json:"total"`
	Page         int           `json:"page"`
	PageSize     int           `json:"page_size"`
	TotalPages   int           `json:"total_pages"`
}

// CreateIntegrationRequest defines request for creating integration
type CreateIntegrationRequest struct {
	Name           string                 `json:"name"`
	Type           IntegrationType        `json:"type"`
	Provider       string                 `json:"provider"`
	Description    string                 `json:"description"`
	Config         map[string]interface{} `json:"config"`
	Credentials    *CredentialConfig      `json:"credentials"`
	TemplateID     string                 `json:"template_id"`
	AutoSync       bool                   `json:"auto_sync"`
	SyncInterval   int                    `json:"sync_interval"`
	Tags           []string               `json:"tags"`
	OrganizationID string                 `json:"organization_id"`
	WorkspaceID    string                 `json:"workspace_id"`
}

// UpdateIntegrationRequest defines request for updating integration
type UpdateIntegrationRequest struct {
	Name         string                 `json:"name"`
	Description  string                 `json:"description"`
	Config       map[string]interface{} `json:"config"`
	Credentials  *CredentialConfig      `json:"credentials"`
	AutoSync     *bool                  `json:"auto_sync"`
	SyncInterval *int                   `json:"sync_interval"`
	Tags         []string               `json:"tags"`
}

// IntegrationTestResult represents the result of testing an integration
type IntegrationTestResult struct {
	Success      bool                   `json:"success"`
	Message      string                 `json:"message"`
	ResponseTime int64                  `json:"response_time"`
	Details      map[string]interface{} `json:"details"`
	Errors       []string               `json:"errors"`
	Warnings     []string               `json:"warnings"`
}
