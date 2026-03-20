package inference

import (
	"context"
	"fmt"
	"time"
)

// InstanceState represents the state of a model instance
type InstanceState string

const (
	StateLoading    InstanceState = "loading"
	StateActive     InstanceState = "active"
	StateIdle       InstanceState = "idle"
	StateBusy       InstanceState = "busy"
	StateUnloading  InstanceState = "unloading"
	StateStopped    InstanceState = "stopped"
	StateError      InstanceState = "error"
	StateRestarting InstanceState = "restarting"
)

// InferenceMode represents the mode of inference
type InferenceMode string

const (
	ModeSync      InferenceMode = "sync"
	ModeStreaming InferenceMode = "streaming"
	ModeBatch     InferenceMode = "batch"
)

// RequestPriority represents the priority of an inference request
type RequestPriority int

const (
	PriorityLow      RequestPriority = 1
	PriorityNormal   RequestPriority = 5
	PriorityHigh     RequestPriority = 10
	PriorityCritical RequestPriority = 15
)

// DeviceType represents the type of compute device
type DeviceType string

const (
	DeviceCPU DeviceType = "cpu"
	DeviceGPU DeviceType = "gpu"
)

// InstanceStatus represents the runtime status of a model instance
type InstanceStatus struct {
	InstanceID    string         `json:"instance_id"`
	ModelID       string         `json:"model_id"`
	State         InstanceState  `json:"state"`
	Device        DeviceType     `json:"device"`
	DeviceID      int            `json:"device_id,omitempty"`
	Port          int            `json:"port,omitempty"`
	LoadedAt      time.Time      `json:"loaded_at"`
	LastUsed      time.Time      `json:"last_used"`
	RequestCount  int64          `json:"request_count"`
	ErrorCount    int64          `json:"error_count"`
	MemoryUsed    int64          `json:"memory_used"`     // bytes
	MemoryTotal   int64          `json:"memory_total"`    // bytes
	CPUUsage      float64        `json:"cpu_usage"`       // percentage
	GPUUsage      float64        `json:"gpu_usage"`       // percentage
	GPUMemoryUsed int64          `json:"gpu_memory_used"` // bytes
	GPUMemoryTotal int64         `json:"gpu_memory_total"` // bytes
	Uptime        time.Duration  `json:"uptime"`
	AvgLatency    time.Duration  `json:"avg_latency"`
	QueueSize     int            `json:"queue_size"`
}

// InstanceConfig represents configuration for a model instance
type InstanceConfig struct {
	ModelID           string        `json:"model_id"`
	ModelPath         string        `json:"model_path"`
	Format            string        `json:"format"`
	ContextLength     int           `json:"context_length"`
	MaxTokens         int           `json:"max_tokens"`
	Temperature       float64       `json:"temperature"`
	TopP              float64       `json:"top_p"`
	TopK              int           `json:"top_k"`
	Device            DeviceType    `json:"device"`
	DeviceID          int           `json:"device_id"`
	GPULayers         int           `json:"gpu_layers"`
	Threads           int           `json:"threads"`
	BatchSize         int           `json:"batch_size"`
	MaxQueueSize      int           `json:"max_queue_size"`
	Timeout           time.Duration `json:"timeout"`
	IdleTimeout       time.Duration `json:"idle_timeout"`
	EnableCache       bool          `json:"enable_cache"`
	CacheSize         int           `json:"cache_size"` // MB
	Port              int           `json:"port,omitempty"`
	AutoRestart       bool          `json:"auto_restart"`
}

// InferenceRequest represents a request for inference
type InferenceRequest struct {
	ID               string                 `json:"id"`
	ModelID          string                 `json:"model_id"`
	Mode             InferenceMode          `json:"mode"`
	Priority         RequestPriority        `json:"priority"`
	Prompt           string                 `json:"prompt"`
	Messages         []ChatMessage          `json:"messages,omitempty"`
	Stream           bool                   `json:"stream"`
	MaxTokens        int                    `json:"max_tokens"`
	Temperature      float64                `json:"temperature"`
	TopP             float64                `json:"top_p"`
	TopK             int                    `json:"top_k"`
	Stop             []string               `json:"stop,omitempty"`
	FrequencyPenalty float64                `json:"frequency_penalty"`
	PresencePenalty  float64                `json:"presence_penalty"`
	User             string                 `json:"user,omitempty"`
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
	Context          context.Context        `json:"-"`
	CreatedAt        time.Time              `json:"created_at"`
	Timeout          time.Duration          `json:"timeout"`
}

// ChatMessage represents a single message in a chat conversation
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	Name    string `json:"name,omitempty"`
}

// InferenceResponse represents a response from inference
type InferenceResponse struct {
	ID                string                 `json:"id"`
	RequestID         string                 `json:"request_id"`
	ModelID           string                 `json:"model_id"`
	InstanceID        string                 `json:"instance_id"`
	Content           string                 `json:"content"`
	FinishReason      string                 `json:"finish_reason"`
	InputTokens       int                    `json:"input_tokens"`
	OutputTokens      int                    `json:"output_tokens"`
	TotalTokens       int                    `json:"total_tokens"`
	Latency           time.Duration          `json:"latency"`
	TimeToFirstToken  time.Duration          `json:"time_to_first_token"`
	TokensPerSecond   float64                `json:"tokens_per_second"`
	Probabilities     []TokenProbability     `json:"probabilities,omitempty"`
	Alternatives      []AlternativeResponse  `json:"alternatives,omitempty"`
	Metadata          map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt         time.Time              `json:"created_at"`
}

// TokenProbability represents the probability of a token
type TokenProbability struct {
	Token       string  `json:"token"`
	Probability float64 `json:"probability"`
	LogProb     float64 `json:"log_prob"`
}

// AlternativeResponse represents an alternative completion
type AlternativeResponse struct {
	Content      string  `json:"content"`
	Probability  float64 `json:"probability"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
}

// StreamChunk represents a chunk in a streaming response
type StreamChunk struct {
	ID           string        `json:"id"`
	RequestID    string        `json:"request_id"`
	ModelID      string        `json:"model_id"`
	InstanceID   string        `json:"instance_id"`
	Content      string        `json:"content"`
	Delta        string        `json:"delta"`
	InputTokens  int           `json:"input_tokens"`
	OutputTokens int           `json:"output_tokens"`
	FinishReason string        `json:"finish_reason"`
	Latency      time.Duration `json:"latency"`
	CreatedAt    time.Time     `json:"created_at"`
	Error        *StreamError  `json:"error,omitempty"`
}

// StreamError represents an error in streaming
type StreamError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Type    string `json:"type"`
}

// Error implements the error interface for StreamError
func (e *StreamError) Error() string {
	return fmt.Sprintf("stream error [%d]: %s", e.Code, e.Message)
}

// BatchRequest represents a batch inference request
type BatchRequest struct {
	ID          string              `json:"id"`
	ModelID     string              `json:"model_id"`
	Requests    []InferenceRequest  `json:"requests"`
	Priority    RequestPriority     `json:"priority"`
	CreatedAt   time.Time           `json:"created_at"`
	StartedAt   *time.Time          `json:"started_at"`
	CompletedAt *time.Time          `json:"completed_at"`
	Status      BatchStatus         `json:"status"`
	Progress    float64             `json:"progress"` // 0-100
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// BatchStatus represents the status of a batch request
type BatchStatus string

const (
	BatchPending   BatchStatus = "pending"
	BatchRunning   BatchStatus = "running"
	BatchComplete  BatchStatus = "complete"
	BatchPartial   BatchStatus = "partial"
	BatchFailed    BatchStatus = "failed"
	BatchCancelled BatchStatus = "cancelled"
)

// BatchResponse represents the response for a batch request
type BatchResponse struct {
	ID          string                        `json:"id"`
	BatchID     string                        `json:"batch_id"`
	ModelID     string                        `json:"model_id"`
	Status      BatchStatus                   `json:"status"`
	Results     []BatchResult                 `json:"results"`
	Total       int                           `json:"total"`
	Succeeded   int                           `json:"succeeded"`
	Failed      int                           `json:"failed"`
	Duration    time.Duration                 `json:"duration"`
	CreatedAt   time.Time                     `json:"created_at"`
	CompletedAt *time.Time                    `json:"completed_at"`
	Metadata    map[string]interface{}        `json:"metadata,omitempty"`
}

// BatchResult represents a single result in a batch
type BatchResult struct {
	Index      int                `json:"index"`
	RequestID  string             `json:"request_id"`
	Success    bool               `json:"success"`
	Response   *InferenceResponse `json:"response,omitempty"`
	Error      *BatchError        `json:"error,omitempty"`
	Duration   time.Duration      `json:"duration"`
}

// BatchError represents an error in a batch result
type BatchError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Type    string `json:"type"`
}

// ResourceAllocation represents resource allocation for an instance
type ResourceAllocation struct {
	InstanceID      string     `json:"instance_id"`
	DeviceType      DeviceType `json:"device_type"`
	DeviceID        int        `json:"device_id"`
	CPUThreads      int        `json:"cpu_threads"`
	MemoryBytes     int64      `json:"memory_bytes"`
	GPUMemoryBytes  int64      `json:"gpu_memory_bytes"`
	GPULayers       int        `json:"gpu_layers"`
	AllocatedAt     time.Time  `json:"allocated_at"`
}

// ResourceUsage represents current resource usage
type ResourceUsage struct {
	CPUUsage      float64   `json:"cpu_usage"`      // percentage
	MemoryUsed    int64     `json:"memory_used"`    // bytes
	MemoryTotal   int64     `json:"memory_total"`   // bytes
	GPUUsage      float64   `json:"gpu_usage"`      // percentage
	GPUMemoryUsed int64     `json:"gpu_memory_used"` // bytes
	GPUMemoryTotal int64    `json:"gpu_memory_total"` // bytes
	ActiveInstances int     `json:"active_instances"`
	QueuedRequests  int     `json:"queued_requests"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// GPUInfo represents information about a GPU device
type GPUInfo struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	Vendor       string `json:"vendor"`
	MemoryTotal  int64  `json:"memory_total"`  // bytes
	MemoryUsed   int64  `json:"memory_used"`   // bytes
	MemoryFree   int64  `json:"memory_free"`   // bytes
	Temperature  int    `json:"temperature"`   // celsius
	Utilization  int    `json:"utilization"`   // percentage
	PowerUsage   int    `json:"power_usage"`   // watts
	PowerCap     int    `json:"power_cap"`     // watts
	DriverVersion string `json:"driver_version"`
	CUDAVersion  string `json:"cuda_version,omitempty"`
}

// PerformanceMetrics represents performance metrics for inference
type PerformanceMetrics struct {
	InstanceID         string        `json:"instance_id"`
	ModelID            string        `json:"model_id"`
	RequestCount       int64         `json:"request_count"`
	SuccessCount       int64         `json:"success_count"`
	ErrorCount         int64         `json:"error_count"`
	AvgLatency         time.Duration `json:"avg_latency"`
	MinLatency         time.Duration `json:"min_latency"`
	MaxLatency         time.Duration `json:"max_latency"`
	P50Latency         time.Duration `json:"p50_latency"`
	P95Latency         time.Duration `json:"p95_latency"`
	P99Latency         time.Duration `json:"p99_latency"`
	AvgTokensPerSecond float64       `json:"avg_tokens_per_second"`
	TotalInputTokens   int64         `json:"total_input_tokens"`
	TotalOutputTokens  int64         `json:"total_output_tokens"`
	CacheHits          int64         `json:"cache_hits"`
	CacheMisses        int64         `json:"cache_misses"`
	CacheHitRate       float64       `json:"cache_hit_rate"`
	QueueWaitTime      time.Duration `json:"queue_wait_time"`
	Uptime             time.Duration `json:"uptime"`
	CPUUsage           float64       `json:"cpu_usage"`
	GPUUsage           float64       `json:"gpu_usage"`
	GPUMemoryUsed      int64         `json:"gpu_memory_used"`
	GPUMemoryTotal     int64         `json:"gpu_memory_total"`
	UpdatedAt          time.Time     `json:"updated_at"`
}

// CacheEntry represents an entry in the inference cache
type CacheEntry struct {
	Key         string                 `json:"key"`
	RequestHash string                 `json:"request_hash"`
	Response    *InferenceResponse     `json:"response"`
	CreatedAt   time.Time              `json:"created_at"`
	ExpiresAt   time.Time              `json:"expires_at"`
	HitCount    int64                  `json:"hit_count"`
	Size        int64                  `json:"size"` // bytes
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// SchedulerStats represents statistics for the scheduler
type SchedulerStats struct {
	TotalRequests      int64         `json:"total_requests"`
	QueuedRequests     int64         `json:"queued_requests"`
	ProcessingRequests int64         `json:"processing_requests"`
	CompletedRequests  int64         `json:"completed_requests"`
	FailedRequests     int64         `json:"failed_requests"`
	AvgQueueTime       time.Duration `json:"avg_queue_time"`
	AvgProcessTime     time.Duration `json:"avg_process_time"`
	ActiveInstances    int           `json:"active_instances"`
	IdleInstances      int           `json:"idle_instances"`
	TotalCapacity      int           `json:"total_capacity"`
	LoadBalance        float64       `json:"load_balance"` // 0-1
	UpdatedAt          time.Time     `json:"updated_at"`
}

// InferenceFilter represents filter options for listing inferences
type InferenceFilter struct {
	ModelID   string    `json:"model_id,omitempty"`
	InstanceID string   `json:"instance_id,omitempty"`
	Status    string    `json:"status,omitempty"`
	StartTime time.Time `json:"start_time,omitempty"`
	EndTime   time.Time `json:"end_time,omitempty"`
	Page      int       `json:"page"`
	PerPage   int       `json:"per_page"`
}

// LoadModelRequest represents a request to load a model
type LoadModelRequest struct {
	ModelID       string        `json:"model_id"`
	Device        DeviceType    `json:"device"`
	DeviceID      int           `json:"device_id,omitempty"`
	GPULayers     int           `json:"gpu_layers,omitempty"`
	Threads       int           `json:"threads,omitempty"`
	BatchSize     int           `json:"batch_size,omitempty"`
	MaxQueueSize  int           `json:"max_queue_size,omitempty"`
	Timeout       time.Duration `json:"timeout,omitempty"`
	IdleTimeout   time.Duration `json:"idle_timeout,omitempty"`
	EnableCache   bool          `json:"enable_cache"`
	CacheSize     int           `json:"cache_size,omitempty"` // MB
}

// UnloadModelRequest represents a request to unload a model
type UnloadModelRequest struct {
	InstanceID string        `json:"instance_id"`
	Force      bool          `json:"force"`
	Timeout    time.Duration `json:"timeout"`
}

// ListInstancesFilter represents filter options for listing instances
type ListInstancesFilter struct {
	ModelID string        `json:"model_id,omitempty"`
	State   InstanceState `json:"state,omitempty"`
	Device  DeviceType    `json:"device,omitempty"`
	Page    int           `json:"page"`
	PerPage int           `json:"per_page"`
}
