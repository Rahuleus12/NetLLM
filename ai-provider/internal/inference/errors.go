package inference

import (
	"errors"
	"fmt"
	"time"
)

// ErrorCode represents a specific error code for inference operations
type ErrorCode string

const (
	// Model Loading Errors
	ErrModelNotFound         ErrorCode = "MODEL_NOT_FOUND"
	ErrModelAlreadyLoaded    ErrorCode = "MODEL_ALREADY_LOADED"
	ErrModelLoadFailed       ErrorCode = "MODEL_LOAD_FAILED"
	ErrModelUnloadFailed     ErrorCode = "MODEL_UNLOAD_FAILED"
	ErrModelFormatUnsupported ErrorCode = "MODEL_FORMAT_UNSUPPORTED"
	ErrModelCorrupted        ErrorCode = "MODEL_CORRUPTED"
	ErrModelPathInvalid      ErrorCode = "MODEL_PATH_INVALID"
	ErrModelConfigInvalid    ErrorCode = "MODEL_CONFIG_INVALID"

	// Instance Errors
	ErrInstanceNotFound      ErrorCode = "INSTANCE_NOT_FOUND"
	ErrInstanceAlreadyExists ErrorCode = "INSTANCE_ALREADY_EXISTS"
	ErrInstanceNotReady      ErrorCode = "INSTANCE_NOT_READY"
	ErrInstanceFailed        ErrorCode = "INSTANCE_FAILED"
	ErrInstanceTimeout       ErrorCode = "INSTANCE_TIMEOUT"
	ErrInstanceBusy          ErrorCode = "INSTANCE_BUSY"
	ErrInstanceStopped       ErrorCode = "INSTANCE_STOPPED"
	ErrMaxInstancesReached   ErrorCode = "MAX_INSTANCES_REACHED"

	// Inference Execution Errors
	ErrInferenceFailed       ErrorCode = "INFERENCE_FAILED"
	ErrInferenceTimeout      ErrorCode = "INFERENCE_TIMEOUT"
	ErrInferenceCancelled    ErrorCode = "INFERENCE_CANCELLED"
	ErrRequestQueueFull      ErrorCode = "REQUEST_QUEUE_FULL"
	ErrRequestInvalid        ErrorCode = "REQUEST_INVALID"
	ErrPromptTooLong         ErrorCode = "PROMPT_TOO_LONG"
	ErrTokenLimitExceeded    ErrorCode = "TOKEN_LIMIT_EXCEEDED"
	ErrContextLengthExceeded ErrorCode = "CONTEXT_LENGTH_EXCEEDED"
	ErrGenerationFailed      ErrorCode = "GENERATION_FAILED"
	ErrStreamingFailed       ErrorCode = "STREAMING_FAILED"

	// Resource Errors
	ErrInsufficientMemory    ErrorCode = "INSUFFICIENT_MEMORY"
	ErrInsufficientGPUMemory ErrorCode = "INSUFFICIENT_GPU_MEMORY"
	ErrNoAvailableGPU        ErrorCode = "NO_AVAILABLE_GPU"
	ErrGPUAllocationFailed   ErrorCode = "GPU_ALLOCATION_FAILED"
	ErrCPUAllocationFailed   ErrorCode = "CPU_ALLOCATION_FAILED"
	ErrResourceExhausted     ErrorCode = "RESOURCE_EXHAUSTED"
	ErrResourceQuotaExceeded ErrorCode = "RESOURCE_QUOTA_EXCEEDED"

	// Batch Processing Errors
	ErrBatchNotFound      ErrorCode = "BATCH_NOT_FOUND"
	ErrBatchCreationFailed ErrorCode = "BATCH_CREATION_FAILED"
	ErrBatchExecutionFailed ErrorCode = "BATCH_EXECUTION_FAILED"
	ErrBatchTimeout       ErrorCode = "BATCH_TIMEOUT"
	ErrBatchCancelled     ErrorCode = "BATCH_CANCELLED"
	ErrBatchSizeExceeded  ErrorCode = "BATCH_SIZE_EXCEEDED"
	ErrBatchInvalid       ErrorCode = "BATCH_INVALID"

	// Cache Errors
	ErrCacheMiss          ErrorCode = "CACHE_MISS"
	ErrCacheFull          ErrorCode = "CACHE_FULL"
	ErrCacheDisabled      ErrorCode = "CACHE_DISABLED"
	ErrCacheCorrupted     ErrorCode = "CACHE_CORRUPTED"

	// Scheduler Errors
	ErrSchedulerNotReady      ErrorCode = "SCHEDULER_NOT_READY"
	ErrSchedulerOverloaded    ErrorCode = "SCHEDULER_OVERLOADED"
	ErrNoAvailableInstance    ErrorCode = "NO_AVAILABLE_INSTANCE"
	ErrSchedulingFailed       ErrorCode = "SCHEDULING_FAILED"
	ErrPriorityInvalid        ErrorCode = "PRIORITY_INVALID"

	// Runtime Errors
	ErrRuntimeNotInitialized ErrorCode = "RUNTIME_NOT_INITIALIZED"
	ErrRuntimeError          ErrorCode = "RUNTIME_ERROR"
	ErrRuntimeCrash          ErrorCode = "RUNTIME_CRASH"
	ErrRuntimeTimeout        ErrorCode = "RUNTIME_TIMEOUT"
	ErrRuntimeUnsupported    ErrorCode = "RUNTIME_UNSUPPORTED"

	// Configuration Errors
	ErrConfigNotFound     ErrorCode = "CONFIG_NOT_FOUND"
	ErrConfigInvalid      ErrorCode = "CONFIG_INVALID"
	ErrConfigLoadFailed   ErrorCode = "CONFIG_LOAD_FAILED"
	ErrConfigSaveFailed   ErrorCode = "CONFIG_SAVE_FAILED"

	// General Errors
	ErrInternal           ErrorCode = "INTERNAL_ERROR"
	ErrNotImplemented     ErrorCode = "NOT_IMPLEMENTED"
	ErrServiceUnavailable ErrorCode = "SERVICE_UNAVAILABLE"
	ErrMaintenanceMode    ErrorCode = "MAINTENANCE_MODE"
)

// InferenceError represents a structured error for inference operations
type InferenceError struct {
	Code       ErrorCode               `json:"code"`
	Message    string                  `json:"message"`
	Details    string                  `json:"details,omitempty"`
	ModelID    string                  `json:"model_id,omitempty"`
	InstanceID string                  `json:"instance_id,omitempty"`
	RequestID  string                  `json:"request_id,omitempty"`
	Retryable  bool                    `json:"retryable"`
	Metadata   map[string]interface{}  `json:"metadata,omitempty"`
	Timestamp  time.Time               `json:"timestamp"`
	Cause      error                   `json:"-"`
}

// Error implements the error interface
func (e *InferenceError) Error() string {
	if e.Details != "" {
		return fmt.Sprintf("[%s] %s: %s", e.Code, e.Message, e.Details)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap returns the underlying cause of the error
func (e *InferenceError) Unwrap() error {
	return e.Cause
}

// NewError creates a new InferenceError
func NewError(code ErrorCode, message string) *InferenceError {
	return &InferenceError{
		Code:      code,
		Message:   message,
		Retryable: isRetryable(code),
		Timestamp: time.Now(),
		Metadata:  make(map[string]interface{}),
	}
}

// WrapError wraps an existing error with inference error context
func WrapError(code ErrorCode, message string, cause error) *InferenceError {
	return &InferenceError{
		Code:      code,
		Message:   message,
		Cause:     cause,
		Retryable: isRetryable(code),
		Timestamp: time.Now(),
		Metadata:  make(map[string]interface{}),
	}
}

// WithDetails adds details to the error
func (e *InferenceError) WithDetails(details string) *InferenceError {
	e.Details = details
	return e
}

// WithModelID adds model ID to the error
func (e *InferenceError) WithModelID(modelID string) *InferenceError {
	e.ModelID = modelID
	return e
}

// WithInstanceID adds instance ID to the error
func (e *InferenceError) WithInstanceID(instanceID string) *InferenceError {
	e.InstanceID = instanceID
	return e
}

// WithRequestID adds request ID to the error
func (e *InferenceError) WithRequestID(requestID string) *InferenceError {
	e.RequestID = requestID
	return e
}

// WithMetadata adds metadata to the error
func (e *InferenceError) WithMetadata(key string, value interface{}) *InferenceError {
	if e.Metadata == nil {
		e.Metadata = make(map[string]interface{})
	}
	e.Metadata[key] = value
	return e
}

// WithRetryable sets whether the error is retryable
func (e *InferenceError) WithRetryable(retryable bool) *InferenceError {
	e.Retryable = retryable
	return e
}

// isRetryable determines if an error code represents a retryable error
func isRetryable(code ErrorCode) bool {
	retryableCodes := map[ErrorCode]bool{
		ErrInstanceBusy:          true,
		ErrInstanceTimeout:       true,
		ErrInferenceTimeout:      true,
		ErrRequestQueueFull:      true,
		ErrNoAvailableGPU:        true,
		ErrNoAvailableInstance:   true,
		ErrSchedulerOverloaded:   true,
		ErrResourceExhausted:     true,
		ErrRuntimeTimeout:        true,
		ErrServiceUnavailable:    true,
	}
	return retryableCodes[code]
}

// Model Loading Error Constructors

// ErrModelNotFound creates a model not found error
func ErrModelNotFoundError(modelID string) *InferenceError {
	return NewError(ErrModelNotFound, "Model not found").
		WithModelID(modelID).
		WithDetails(fmt.Sprintf("The model with ID '%s' does not exist", modelID))
}

// ErrModelAlreadyLoadedError creates a model already loaded error
func ErrModelAlreadyLoadedError(modelID, instanceID string) *InferenceError {
	return NewError(ErrModelAlreadyLoaded, "Model is already loaded").
		WithModelID(modelID).
		WithInstanceID(instanceID).
		WithDetails(fmt.Sprintf("Model '%s' is already loaded in instance '%s'", modelID, instanceID))
}

// ErrModelLoadFailedError creates a model load failed error
func ErrModelLoadFailedError(modelID string, cause error) *InferenceError {
	return WrapError(ErrModelLoadFailed, "Failed to load model", cause).
		WithModelID(modelID).
		WithRetryable(false)
}

// ErrModelFormatUnsupportedError creates an unsupported format error
func ErrModelFormatUnsupportedError(format string) *InferenceError {
	return NewError(ErrModelFormatUnsupported, "Unsupported model format").
		WithDetails(fmt.Sprintf("The model format '%s' is not supported", format))
}

// Instance Error Constructors

// ErrInstanceNotFoundError creates an instance not found error
func ErrInstanceNotFoundError(instanceID string) *InferenceError {
	return NewError(ErrInstanceNotFound, "Instance not found").
		WithInstanceID(instanceID).
		WithDetails(fmt.Sprintf("The instance with ID '%s' does not exist", instanceID))
}

// ErrInstanceNotReadyError creates an instance not ready error
func ErrInstanceNotReadyError(instanceID string, state InstanceState) *InferenceError {
	return NewError(ErrInstanceNotReady, "Instance is not ready").
		WithInstanceID(instanceID).
		WithDetails(fmt.Sprintf("Instance '%s' is in state '%s'", instanceID, state)).
		WithRetryable(true)
}

// ErrMaxInstancesReachedError creates a max instances reached error
func ErrMaxInstancesReachedError(max int) *InferenceError {
	return NewError(ErrMaxInstancesReached, "Maximum number of instances reached").
		WithDetails(fmt.Sprintf("Cannot create more than %d instances", max)).
		WithMetadata("max_instances", max)
}

// Inference Execution Error Constructors

// ErrInferenceFailedError creates an inference failed error
func ErrInferenceFailedError(requestID string, cause error) *InferenceError {
	return WrapError(ErrInferenceFailed, "Inference execution failed", cause).
		WithRequestID(requestID).
		WithRetryable(false)
}

// ErrInferenceTimeoutError creates an inference timeout error
func ErrInferenceTimeoutError(requestID string, timeout time.Duration) *InferenceError {
	return NewError(ErrInferenceTimeout, "Inference request timed out").
		WithRequestID(requestID).
		WithDetails(fmt.Sprintf("Request exceeded timeout of %v", timeout)).
		WithRetryable(true).
		WithMetadata("timeout", timeout.String())
}

// ErrRequestQueueFullError creates a request queue full error
func ErrRequestQueueFullError(instanceID string, queueSize int) *InferenceError {
	return NewError(ErrRequestQueueFull, "Request queue is full").
		WithInstanceID(instanceID).
		WithDetails(fmt.Sprintf("Instance '%s' has reached maximum queue size of %d", instanceID, queueSize)).
		WithRetryable(true).
		WithMetadata("queue_size", queueSize)
}

// ErrPromptTooLongError creates a prompt too long error
func ErrPromptTooLongError(tokenCount, maxTokens int) *InferenceError {
	return NewError(ErrPromptTooLong, "Prompt exceeds maximum length").
		WithDetails(fmt.Sprintf("Prompt has %d tokens, maximum is %d", tokenCount, maxTokens)).
		WithMetadata("token_count", tokenCount).
		WithMetadata("max_tokens", maxTokens)
}

// Resource Error Constructors

// ErrInsufficientMemoryError creates an insufficient memory error
func ErrInsufficientMemoryError(required, available int64) *InferenceError {
	return NewError(ErrInsufficientMemory, "Insufficient memory").
		WithDetails(fmt.Sprintf("Required: %d bytes, Available: %d bytes", required, available)).
		WithMetadata("required_bytes", required).
		WithMetadata("available_bytes", available).
		WithRetryable(true)
}

// ErrInsufficientGPUMemoryError creates an insufficient GPU memory error
func ErrInsufficientGPUMemoryError(gpuID int, required, available int64) *InferenceError {
	return NewError(ErrInsufficientGPUMemory, "Insufficient GPU memory").
		WithDetails(fmt.Sprintf("GPU %d: Required %d bytes, Available: %d bytes", gpuID, required, available)).
		WithMetadata("gpu_id", gpuID).
		WithMetadata("required_bytes", required).
		WithMetadata("available_bytes", available).
		WithRetryable(true)
}

// ErrNoAvailableGPUError creates a no available GPU error
func ErrNoAvailableGPUError() *InferenceError {
	return NewError(ErrNoAvailableGPU, "No available GPU devices").
		WithDetails("All GPU devices are either unavailable or have insufficient memory").
		WithRetryable(true)
}

// Batch Error Constructors

// ErrBatchNotFoundError creates a batch not found error
func ErrBatchNotFoundError(batchID string) *InferenceError {
	return NewError(ErrBatchNotFound, "Batch not found").
		WithDetails(fmt.Sprintf("The batch with ID '%s' does not exist", batchID)).
		WithMetadata("batch_id", batchID)
}

// ErrBatchSizeExceededError creates a batch size exceeded error
func ErrBatchSizeExceededError(size, maxSize int) *InferenceError {
	return NewError(ErrBatchSizeExceeded, "Batch size exceeds maximum").
		WithDetails(fmt.Sprintf("Batch size %d exceeds maximum allowed size of %d", size, maxSize)).
		WithMetadata("size", size).
		WithMetadata("max_size", maxSize)
}

// Cache Error Constructors

// ErrCacheMissError creates a cache miss error
func ErrCacheMissError(key string) *InferenceError {
	return NewError(ErrCacheMiss, "Cache miss").
		WithDetails(fmt.Sprintf("No cache entry found for key '%s'", key)).
		WithMetadata("cache_key", key)
}

// ErrCacheFullError creates a cache full error
func ErrCacheFullError(size int64) *InferenceError {
	return NewError(ErrCacheFull, "Cache is full").
		WithDetails(fmt.Sprintf("Cache has reached maximum size of %d bytes", size)).
		WithMetadata("cache_size", size)
}

// Scheduler Error Constructors

// ErrNoAvailableInstanceError creates a no available instance error
func ErrNoAvailableInstanceError(modelID string) *InferenceError {
	return NewError(ErrNoAvailableInstance, "No available instance").
		WithModelID(modelID).
		WithDetails(fmt.Sprintf("No available instance found for model '%s'", modelID)).
		WithRetryable(true)
}

// ErrSchedulerOverloadedError creates a scheduler overloaded error
func ErrSchedulerOverloadedError(queueSize int) *InferenceError {
	return NewError(ErrSchedulerOverloaded, "Scheduler is overloaded").
		WithDetails(fmt.Sprintf("Scheduler has %d requests in queue", queueSize)).
		WithRetryable(true).
		WithMetadata("queue_size", queueSize)
}

// Runtime Error Constructors

// ErrRuntimeNotInitializedError creates a runtime not initialized error
func ErrRuntimeNotInitializedError(runtime string) *InferenceError {
	return NewError(ErrRuntimeNotInitialized, "Runtime not initialized").
		WithDetails(fmt.Sprintf("The '%s' runtime has not been initialized", runtime)).
		WithMetadata("runtime", runtime)
}

// ErrRuntimeError creates a generic runtime error
func ErrRuntimeError(runtime string, cause error) *InferenceError {
	return WrapError(ErrRuntimeError, "Runtime error occurred", cause).
		WithMetadata("runtime", runtime).
		WithRetryable(false)
}

// Helper functions for error checking

// IsModelError checks if an error is a model-related error
func IsModelError(err error) bool {
	var inferenceErr *InferenceError
	if errors.As(err, &inferenceErr) {
		switch inferenceErr.Code {
		case ErrModelNotFound, ErrModelLoadFailed, ErrModelFormatUnsupported,
		     ErrModelCorrupted, ErrModelPathInvalid, ErrModelConfigInvalid:
			return true
		}
	}
	return false
}

// IsInstanceError checks if an error is an instance-related error
func IsInstanceError(err error) bool {
	var inferenceErr *InferenceError
	if errors.As(err, &inferenceErr) {
		switch inferenceErr.Code {
		case ErrInstanceNotFound, ErrInstanceNotReady, ErrInstanceFailed,
		     ErrInstanceTimeout, ErrInstanceBusy, ErrMaxInstancesReached:
			return true
		}
	}
	return false
}

// IsResourceError checks if an error is a resource-related error
func IsResourceError(err error) bool {
	var inferenceErr *InferenceError
	if errors.As(err, &inferenceErr) {
		switch inferenceErr.Code {
		case ErrInsufficientMemory, ErrInsufficientGPUMemory, ErrNoAvailableGPU,
		     ErrGPUAllocationFailed, ErrCPUAllocationFailed, ErrResourceExhausted:
			return true
		}
	}
	return false
}

// IsRetryable checks if an error is retryable
func IsRetryable(err error) bool {
	var inferenceErr *InferenceError
	if errors.As(err, &inferenceErr) {
		return inferenceErr.Retryable
	}
	return false
}

// GetErrorCode extracts the error code from an error
func GetErrorCode(err error) ErrorCode {
	var inferenceErr *InferenceError
	if errors.As(err, &inferenceErr) {
		return inferenceErr.Code
	}
	return ErrInternal
}
