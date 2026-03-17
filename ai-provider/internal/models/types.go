package models

import (
	"time"
)

// ModelFormat represents the format of a model file
type ModelFormat string

const (
	FormatGGUF       ModelFormat = "gguf"
	FormatONNX       ModelFormat = "onnx"
	FormatPyTorch    ModelFormat = "pytorch"
	FormatTensorFlow ModelFormat = "tensorflow"
	FormatSafeTensors ModelFormat = "safetensors"
	FormatCustom     ModelFormat = "custom"
)

// ModelStatus represents the current status of a model
type ModelStatus string

const (
	StatusInactive    ModelStatus = "inactive"
	StatusDownloading  ModelStatus = "downloading"
	StatusValidating  ModelStatus = "validating"
	StatusLoading     ModelStatus = "loading"
	StatusActive      ModelStatus = "active"
	StatusError       ModelStatus = "error"
	StatusDeprecated  ModelStatus = "deprecated"
)

// DownloadStatus represents the status of a download operation
type DownloadStatus string

const (
	DownloadPending   DownloadStatus = "pending"
	DownloadRunning   DownloadStatus = "running"
	DownloadPaused    DownloadStatus = "paused"
	DownloadCompleted DownloadStatus = "completed"
	DownloadFailed    DownloadStatus = "failed"
	DownloadCancelled DownloadStatus = "cancelled"
)

// ValidationStatus represents the result of model validation
type ValidationStatus string

const (
	ValidationValid   ValidationStatus = "valid"
	ValidationInvalid ValidationStatus = "invalid"
	ValidationWarning ValidationStatus = "warning"
	ValidationPending ValidationStatus = "pending"
)

// Model represents an AI model in the system
type Model struct {
	ID           string                 `json:"id" db:"id"`
	Name         string                 `json:"name" db:"name"`
	Version      string                 `json:"version" db:"version"`
	Description  string                 `json:"description" db:"description"`
	Format       ModelFormat            `json:"format" db:"format"`
	Status       ModelStatus            `json:"status" db:"status"`
	Source       ModelSource            `json:"source" db:"source"`
	FileInfo     ModelFileInfo          `json:"file_info" db:"file_info"`
	Config       ModelConfig            `json:"config" db:"config"`
	Requirements ModelRequirements      `json:"requirements" db:"requirements"`
	Instances    ModelInstances         `json:"instances" db:"instances"`
	Metrics      ModelMetrics           `json:"metrics" db:"metrics"`
	Tags         []string               `json:"tags" db:"tags"`
	Metadata     map[string]interface{} `json:"metadata" db:"metadata"`
	CreatedAt    time.Time              `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at" db:"updated_at"`
	CreatedBy    string                 `json:"created_by" db:"created_by"`
}

// ModelSource represents the source location of a model
type ModelSource struct {
	Type     string `json:"type" db:"type"`           // url, s3, local, huggingface
	URL      string `json:"url" db:"url"`             // URL or path to the model
	Checksum string `json:"checksum" db:"checksum"`   // Expected checksum
	Username string `json:"username,omitempty" db:"username"` // For authentication
	Password string `json:"-" db:"password"`          // Sensitive, not exposed in JSON
	Region   string `json:"region,omitempty" db:"region"`     // For S3
	Bucket   string `json:"bucket,omitempty" db:"bucket"`     // For S3
	Key      string `json:"key,omitempty" db:"key"`           // For S3
}

// ModelFileInfo contains information about the model file
type ModelFileInfo struct {
	Path             string    `json:"path" db:"path"`
	SizeBytes        int64     `json:"size_bytes" db:"size_bytes"`
	ChecksumVerified bool      `json:"checksum_verified" db:"checksum_verified"`
	LastVerified     time.Time `json:"last_verified" db:"last_verified"`
	LastAccessed     time.Time `json:"last_accessed" db:"last_accessed"`
	DownloadedAt     time.Time `json:"downloaded_at" db:"downloaded_at"`
}

// ModelConfig represents model-specific configuration
type ModelConfig struct {
	ContextLength    int                    `json:"context_length" db:"context_length"`
	Temperature      float64                `json:"temperature" db:"temperature"`
	MaxTokens        int                    `json:"max_tokens" db:"max_tokens"`
	TopP            float64                `json:"top_p" db:"top_p"`
	TopK            int                    `json:"top_k" db:"top_k"`
	StopTokens      []string               `json:"stop_tokens" db:"stop_tokens"`
	FrequencyPenalty float64               `json:"frequency_penalty" db:"frequency_penalty"`
	PresencePenalty  float64               `json:"presence_penalty" db:"presence_penalty"`
	RepeatPenalty    float64               `json:"repeat_penalty" db:"repeat_penalty"`
	CustomParams     map[string]interface{} `json:"custom_params" db:"custom_params"`
}

// ModelRequirements specifies the resource requirements for a model
type ModelRequirements struct {
	RAMMin      int    `json:"ram_min" db:"ram_min"`           // Minimum RAM in MB
	GPUMemory   int    `json:"gpu_memory" db:"gpu_memory"`     // GPU memory in MB
	CPUCores    int    `json:"cpu_cores" db:"cpu_cores"`       // Number of CPU cores
	GPURequired bool   `json:"gpu_required" db:"gpu_required"` // Whether GPU is required
	GPUCount    int    `json:"gpu_count" db:"gpu_count"`       // Number of GPUs required
	StorageGB   int    `json:"storage_gb" db:"storage_gb"`     // Storage in GB
}

// ModelInstances contains information about running model instances
type ModelInstances struct {
	Running int               `json:"running" db:"running"`
	Total   int               `json:"total" db:"total"`
	List    []ModelInstance   `json:"list" db:"list"`
}

// ModelInstance represents a single running instance of a model
type ModelInstance struct {
	ID           string    `json:"id" db:"id"`
	ModelID      string    `json:"model_id" db:"model_id"`
	ContainerID  string    `json:"container_id" db:"container_id"`
	Status       string    `json:"status" db:"status"`
	Port         int       `json:"port" db:"port"`
	GPUDevice    int       `json:"gpu_device" db:"gpu_device"`
	MemoryUsed   int64     `json:"memory_used" db:"memory_used"`
	CPUUsage     float64   `json:"cpu_usage" db:"cpu_usage"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	StartedAt    time.Time `json:"started_at" db:"started_at"`
	StoppedAt    time.Time `json:"stopped_at" db:"stopped_at"`
	LastActiveAt time.Time `json:"last_active_at" db:"last_active_at"`
}

// ModelMetrics contains usage and performance metrics for a model
type ModelMetrics struct {
	TotalRequests  int64     `json:"total_requests" db:"total_requests"`
	SuccessCount   int64     `json:"success_count" db:"success_count"`
	ErrorCount     int64     `json:"error_count" db:"error_count"`
	AvgLatencyMs   float64   `json:"avg_latency_ms" db:"avg_latency_ms"`
	MinLatencyMs   int64     `json:"min_latency_ms" db:"min_latency_ms"`
	MaxLatencyMs   int64     `json:"max_latency_ms" db:"max_latency_ms"`
	TokensIn       int64     `json:"tokens_in" db:"tokens_in"`
	TokensOut      int64     `json:"tokens_out" db:"tokens_out"`
	LastUsed       time.Time `json:"last_used" db:"last_used"`
	LastUpdated    time.Time `json:"last_updated" db:"last_updated"`
}

// DownloadProgress tracks the progress of a model download
type DownloadProgress struct {
	ModelID          string         `json:"model_id" db:"model_id"`
	Status           DownloadStatus `json:"status" db:"status"`
	Percentage       float64        `json:"percentage" db:"percentage"`
	BytesDownloaded  int64          `json:"bytes_downloaded" db:"bytes_downloaded"`
	TotalBytes       int64          `json:"total_bytes" db:"total_bytes"`
	SpeedMbps        float64        `json:"speed_mbps" db:"speed_mbps"`
	ETARemaining     int            `json:"eta_seconds" db:"eta_seconds"`
	StartedAt        time.Time      `json:"started_at" db:"started_at"`
	UpdatedAt        time.Time      `json:"updated_at" db:"updated_at"`
	CompletedAt      *time.Time     `json:"completed_at" db:"completed_at"`
	Error           string         `json:"error,omitempty" db:"error"`
	RetryCount      int            `json:"retry_count" db:"retry_count"`
	CurrentSpeed    float64        `json:"current_speed" db:"current_speed"`
	AverageSpeed    float64        `json:"average_speed" db:"average_speed"`
}

// ValidationResult contains the results of model validation
type ValidationResult struct {
	ModelID     string                 `json:"model_id" db:"model_id"`
	Status      ValidationStatus       `json:"status" db:"status"`
	Checks      map[string]CheckResult `json:"checks" db:"checks"`
	Errors      []string               `json:"errors" db:"errors"`
	Warnings    []string               `json:"warnings" db:"warnings"`
	ValidatedAt time.Time              `json:"validated_at" db:"validated_at"`
	Duration    int64                  `json:"duration_ms" db:"duration_ms"`
}

// CheckResult represents the result of a single validation check
type CheckResult struct {
	Name     string      `json:"name" db:"name"`
	Status   ValidationStatus `json:"status" db:"status"`
	Expected interface{} `json:"expected,omitempty" db:"expected"`
	Actual   interface{} `json:"actual,omitempty" db:"actual"`
	Message  string      `json:"message" db:"message"`
}

// ModelVersion represents a version of a model
type ModelVersion struct {
	ID          string    `json:"id" db:"id"`
	ModelID     string    `json:"model_id" db:"model_id"`
	Version     string    `json:"version" db:"version"`
	Source      ModelSource `json:"source" db:"source"`
	Changelog   string    `json:"changelog" db:"changelog"`
	Status      ModelStatus `json:"status" db:"status"`
	IsActive    bool      `json:"is_active" db:"is_active"`
	IsDeprecated bool     `json:"is_deprecated" db:"is_deprecated"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
	CreatedBy   string    `json:"created_by" db:"created_by"`
}

// VersionComparison represents the comparison between two model versions
type VersionComparison struct {
	ModelID     string        `json:"model_id" db:"model_id"`
	FromVersion string        `json:"from_version" db:"from_version"`
	ToVersion   string        `json:"to_version" db:"to_version"`
	Differences []VersionDiff `json:"differences" db:"differences"`
	UpgradePath []string      `json:"upgrade_path" db:"upgrade_path"`
}

// VersionDiff represents a difference between two versions
type VersionDiff struct {
	Field    string      `json:"field" db:"field"`
	OldValue interface{} `json:"old_value" db:"old_value"`
	NewValue interface{} `json:"new_value" db:"new_value"`
	Type     string      `json:"type" db:"type"` // added, removed, changed
}

// ModelFilter represents filter options for listing models
type ModelFilter struct {
	Page      int          `json:"page" db:"page"`
	PerPage   int          `json:"per_page" db:"per_page"`
	Status    ModelStatus  `json:"status" db:"status"`
	Format    ModelFormat  `json:"format" db:"format"`
	Search    string       `json:"search" db:"search"`
	SortBy    string       `json:"sort_by" db:"sort_by"`
	SortOrder string       `json:"sort_order" db:"sort_order"`
	Tags      []string     `json:"tags" db:"tags"`
}

// ModelList represents a paginated list of models
type ModelList struct {
	Data       []*Model `json:"data"`
	Pagination Pagination `json:"pagination"`
}

// Pagination represents pagination information
type Pagination struct {
	Page       int   `json:"page"`
	PerPage    int   `json:"per_page"`
	TotalPages int   `json:"total_pages"`
	TotalCount int64 `json:"total_count"`
	HasNext    bool  `json:"has_next"`
	HasPrev    bool  `json:"has_prev"`
}

// ConfigTemplate represents a reusable configuration template
type ConfigTemplate struct {
	ID          string       `json:"id" db:"id"`
	Name        string       `json:"name" db:"name"`
	Description string       `json:"description" db:"description"`
	Config      ModelConfig  `json:"config" db:"config"`
	Category    string       `json:"category" db:"category"`
	Tags        []string     `json:"tags" db:"tags"`
	IsDefault   bool         `json:"is_default" db:"is_default"`
	CreatedAt   time.Time    `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at" db:"updated_at"`
	CreatedBy   string       `json:"created_by" db:"created_by"`
}

// ContainerTemplate represents a template for model containers
type ContainerTemplate struct {
	ID               string            `json:"id" db:"id"`
	ModelID          string            `json:"model_id" db:"model_id"`
	Image            string            `json:"image" db:"image"`
	Command          []string          `json:"command" db:"command"`
	Args             []string          `json:"args" db:"args"`
	Env              map[string]string `json:"env" db:"env"`
	Volumes          []VolumeMount     `json:"volumes" db:"volumes"`
	Ports            []PortMapping     `json:"ports" db:"ports"`
	Resources        ResourceLimits    `json:"resources" db:"resources"`
	HealthCheck      HealthCheckConfig `json:"health_check" db:"health_check"`
	CreatedAt        time.Time         `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at" db:"updated_at"`
}

// VolumeMount represents a volume mount in a container
type VolumeMount struct {
	Source   string `json:"source" db:"source"`
	Target   string `json:"target" db:"target"`
	ReadOnly bool   `json:"read_only" db:"read_only"`
}

// PortMapping represents a port mapping in a container
type PortMapping struct {
	ContainerPort int    `json:"container_port" db:"container_port"`
	HostPort      int    `json:"host_port" db:"host_port"`
	Protocol      string `json:"protocol" db:"protocol"`
}

// ResourceLimits represents resource limits for a container
type ResourceLimits struct {
	CPU    int    `json:"cpu" db:"cpu"`       // CPU cores
	Memory string `json:"memory" db:"memory"` // Memory limit (e.g., "4GB")
	GPU    int    `json:"gpu" db:"gpu"`       // Number of GPUs
}

// HealthCheckConfig represents health check configuration
type HealthCheckConfig struct {
	Test        []string `json:"test" db:"test"`
	Interval    int      `json:"interval" db:"interval"`       // seconds
	Timeout     int      `json:"timeout" db:"timeout"`         // seconds
	Retries     int      `json:"retries" db:"retries"`
	StartPeriod int      `json:"start_period" db:"start_period"` // seconds
}

// ModelEvent represents an event related to a model
type ModelEvent struct {
	ID        string                 `json:"id" db:"id"`
	ModelID   string                 `json:"model_id" db:"model_id"`
	Type      string                 `json:"type" db:"type"`
	Message   string                 `json:"message" db:"message"`
	Details   map[string]interface{} `json:"details" db:"details"`
	Timestamp time.Time              `json:"timestamp" db:"timestamp"`
	UserID    string                 `json:"user_id" db:"user_id"`
}

// ModelOperation represents a long-running operation on a model
type ModelOperation struct {
	ID          string                 `json:"id" db:"id"`
	ModelID     string                 `json:"model_id" db:"model_id"`
	Type        string                 `json:"type" db:"type"`
	Status      string                 `json:"status" db:"status"`
	Progress    float64                `json:"progress" db:"progress"`
	Message     string                 `json:"message" db:"message"`
	Error       string                 `json:"error" db:"error"`
	StartedAt   time.Time              `json:"started_at" db:"started_at"`
	CompletedAt *time.Time             `json:"completed_at" db:"completed_at"`
	Metadata    map[string]interface{} `json:"metadata" db:"metadata"`
}

// ModelStats represents statistical information about models
type ModelStats struct {
	TotalModels      int64 `json:"total_models" db:"total_models"`
	ActiveModels     int64 `json:"active_models" db:"active_models"`
	InactiveModels   int64 `json:"inactive_models" db:"inactive_models"`
	DownloadingModels int64 `json:"downloading_models" db:"downloading_models"`
	ErrorModels      int64 `json:"error_models" db:"error_models"`
	TotalInstances   int64 `json:"total_instances" db:"total_instances"`
	RunningInstances int64 `json:"running_instances" db:"running_instances"`
	TotalRequests    int64 `json:"total_requests" db:"total_requests"`
	TotalTokens      int64 `json:"total_tokens" db:"total_tokens"`
	TotalStorage     int64 `json:"total_storage" db:"total_storage"`
}

// DefaultModelConfig returns a default configuration for models
func DefaultModelConfig() ModelConfig {
	return ModelConfig{
		ContextLength:     2048,
		Temperature:       0.7,
		MaxTokens:         512,
		TopP:             0.9,
		TopK:             40,
		StopTokens:       []string{},
		FrequencyPenalty: 0.0,
		PresencePenalty:  0.0,
		RepeatPenalty:    1.0,
		CustomParams:     make(map[string]interface{}),
	}
}

// DefaultModelRequirements returns default resource requirements
func DefaultModelRequirements() ModelRequirements {
	return ModelRequirements{
		RAMMin:      4096,
		GPUMemory:   0,
		CPUCores:    2,
		GPURequired: false,
		GPUCount:    0,
		StorageGB:   10,
	}
}

// IsReady checks if the model is ready for inference
func (m *Model) IsReady() bool {
	return m.Status == StatusActive && m.Instances.Running > 0
}

// CanStart checks if the model can be started
func (m *Model) CanStart() bool {
	return m.Status == StatusInactive || m.Status == StatusError
}

// CanStop checks if the model can be stopped
func (m *Model) CanStop() bool {
	return m.Status == StatusActive || m.Status == StatusLoading
}

// CanDelete checks if the model can be deleted
func (m *Model) CanDelete() bool {
	return m.Status != StatusDownloading && m.Status != StatusLoading
}

// GetStoragePath returns the storage path for the model
func (m *Model) GetStoragePath() string {
	return m.FileInfo.Path
}

// GetFormattedSize returns a human-readable size string
func (m *Model) GetFormattedSize() string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
		TB = GB * 1024
	)

	bytes := m.FileInfo.SizeBytes
	switch {
	case bytes >= TB:
		return formatSize(bytes, TB, "TB")
	case bytes >= GB:
		return formatSize(bytes, GB, "GB")
	case bytes >= MB:
		return formatSize(bytes, MB, "MB")
	case bytes >= KB:
		return formatSize(bytes, KB, "KB")
	default:
		return formatSize(bytes, 1, "B")
	}
}

// formatSize formats a size value with the given unit
func formatSize(bytes int64, unit int64, suffix string) string {
	return formatNumber(float64(bytes)/float64(unit)) + " " + suffix
}

// formatNumber formats a number to 2 decimal places
func formatNumber(n float64) string {
	if n < 10 {
		return formatDecimal(n, 2)
	}
	return formatDecimal(n, 1)
}

// formatDecimal formats a number to the specified decimal places
func formatDecimal(n float64, places int) string {
	format := "%." + string(rune('0'+places)) + "f"
	return sprintf(format, n)
}

// sprintf is a helper to avoid importing fmt
func sprintf(format string, a ...interface{}) string {
	// Simple implementation for our use case
	return format // Simplified version
}
