package models

import (
	"errors"
	"fmt"
)

// Model-specific error types for comprehensive error handling

// Common errors
var (
	// ErrModelNotFound indicates that the requested model was not found
	ErrModelNotFound = errors.New("model not found")

	// ErrModelAlreadyExists indicates that a model with the same identifier already exists
	ErrModelAlreadyExists = errors.New("model already exists")

	// ErrModelInvalid indicates that the model data is invalid
	ErrModelInvalid = errors.New("invalid model data")

	// ErrModelOperationFailed indicates a general model operation failure
	ErrModelOperationFailed = errors.New("model operation failed")
)

// Download errors
var (
	// ErrDownloadFailed indicates that the model download failed
	ErrDownloadFailed = errors.New("download failed")

	// ErrDownloadInProgress indicates that a download is already in progress
	ErrDownloadInProgress = errors.New("download already in progress")

	// ErrDownloadNotActive indicates that no active download exists
	ErrDownloadNotActive = errors.New("no active download")

	// ErrDownloadCancelled indicates that the download was cancelled
	ErrDownloadCancelled = errors.New("download cancelled")

	// ErrDownloadTimeout indicates that the download timed out
	ErrDownloadTimeout = errors.New("download timeout")

	// ErrInvalidDownloadSource indicates that the download source is invalid
	ErrInvalidDownloadSource = errors.New("invalid download source")
)

// Validation errors
var (
	// ErrValidationFailed indicates that model validation failed
	ErrValidationFailed = errors.New("validation failed")

	// ErrChecksumMismatch indicates that the checksum verification failed
	ErrChecksumMismatch = errors.New("checksum mismatch")

	// ErrInvalidModelFormat indicates that the model format is invalid or unsupported
	ErrInvalidModelFormat = errors.New("invalid model format")

	// ErrModelCorrupted indicates that the model file is corrupted
	ErrModelCorrupted = errors.New("model file corrupted")

	// ErrModelIntegrityCheckFailed indicates that the integrity check failed
	ErrModelIntegrityCheckFailed = errors.New("model integrity check failed")
)

// Version errors
var (
	// ErrVersionNotFound indicates that the requested version was not found
	ErrVersionNotFound = errors.New("version not found")

	// ErrVersionAlreadyExists indicates that the version already exists
	ErrVersionAlreadyExists = errors.New("version already exists")

	// ErrInvalidVersion indicates that the version string is invalid
	ErrInvalidVersion = errors.New("invalid version format")

	// ErrVersionConflict indicates a version conflict (e.g., trying to delete active version)
	ErrVersionConflict = errors.New("version conflict")
)

// Configuration errors
var (
	// ErrConfigurationInvalid indicates that the model configuration is invalid
	ErrConfigurationInvalid = errors.New("invalid configuration")

	// ErrConfigurationNotFound indicates that the configuration was not found
	ErrConfigurationNotFound = errors.New("configuration not found")

	// ErrTemplateNotFound indicates that the configuration template was not found
	ErrTemplateNotFound = errors.New("configuration template not found")

	// ErrTemplateInvalid indicates that the configuration template is invalid
	ErrTemplateInvalid = errors.New("invalid configuration template")
)

// Container errors
var (
	// ErrContainerNotFound indicates that the container was not found
	ErrContainerNotFound = errors.New("container not found")

	// ErrContainerFailed indicates that container operation failed
	ErrContainerFailed = errors.New("container operation failed")

	// ErrContainerTimeout indicates that container operation timed out
	ErrContainerTimeout = errors.New("container operation timeout")

	// ErrContainerAlreadyRunning indicates that the container is already running
	ErrContainerAlreadyRunning = errors.New("container already running")

	// ErrContainerNotRunning indicates that the container is not running
	ErrContainerNotRunning = errors.New("container not running")
)

// Storage errors
var (
	// ErrStorageFull indicates that storage is full
	ErrStorageFull = errors.New("storage full")

	// ErrFileNotFound indicates that the model file was not found
	ErrFileNotFound = errors.New("model file not found")

	// ErrFileAccessDenied indicates access denied to model file
	ErrFileAccessDenied = errors.New("file access denied")

	// ErrPathInvalid indicates that the file path is invalid
	ErrPathInvalid = errors.New("invalid file path")
)

// ModelError represents a detailed model error with context
type ModelError struct {
	ModelID   string
	Operation string
	Err       error
	Message   string
}

// Error implements the error interface
func (e *ModelError) Error() string {
	if e.ModelID != "" {
		return fmt.Sprintf("model error [%s]: %s - %s", e.ModelID, e.Operation, e.Message)
	}
	return fmt.Sprintf("model error: %s - %s", e.Operation, e.Message)
}

// Unwrap returns the underlying error
func (e *ModelError) Unwrap() error {
	return e.Err
}

// NewModelError creates a new ModelError
func NewModelError(modelID, operation string, err error, message string) *ModelError {
	return &ModelError{
		ModelID:   modelID,
		Operation: operation,
		Err:       err,
		Message:   message,
	}
}

// DownloadError represents a download-specific error with details
type DownloadError struct {
	ModelID  string
	URL      string
	Err      error
	Message  string
	Progress float64
}

// Error implements the error interface
func (e *DownloadError) Error() string {
	return fmt.Sprintf("download error [%s]: %s (progress: %.1f%%) - %s",
		e.ModelID, e.URL, e.Progress, e.Message)
}

// Unwrap returns the underlying error
func (e *DownloadError) Unwrap() error {
	return e.Err
}

// NewDownloadError creates a new DownloadError
func NewDownloadError(modelID, url string, err error, message string, progress float64) *DownloadError {
	return &DownloadError{
		ModelID:  modelID,
		URL:      url,
		Err:      err,
		Message:  message,
		Progress: progress,
	}
}

// ValidationError represents a validation-specific error with details
type ValidationError struct {
	ModelID string
	Check   string
	Err     error
	Message string
	Details map[string]interface{}
}

// Error implements the error interface
func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation error [%s]: %s check failed - %s",
		e.ModelID, e.Check, e.Message)
}

// Unwrap returns the underlying error
func (e *ValidationError) Unwrap() error {
	return e.Err
}

// NewValidationError creates a new ValidationError
func NewValidationError(modelID, check string, err error, message string) *ValidationError {
	return &ValidationError{
		ModelID: modelID,
		Check:   check,
		Err:     err,
		Message: message,
		Details: make(map[string]interface{}),
	}
}

// VersionError represents a version-specific error with details
type VersionError struct {
	ModelID string
	Version string
	Err     error
	Message string
}

// Error implements the error interface
func (e *VersionError) Error() string {
	return fmt.Sprintf("version error [%s@%s]: %s", e.ModelID, e.Version, e.Message)
}

// Unwrap returns the underlying error
func (e *VersionError) Unwrap() error {
	return e.Err
}

// NewVersionError creates a new VersionError
func NewVersionError(modelID, version string, err error, message string) *VersionError {
	return &VersionError{
		ModelID: modelID,
		Version: version,
		Err:     err,
		Message: message,
	}
}

// ConfigurationError represents a configuration-specific error with details
type ConfigurationError struct {
	ModelID    string
	ConfigKey  string
	Err        error
	Message    string
	Suggestion string
}

// Error implements the error interface
func (e *ConfigurationError) Error() string {
	if e.ConfigKey != "" {
		return fmt.Sprintf("configuration error [%s]: key '%s' - %s",
			e.ModelID, e.ConfigKey, e.Message)
	}
	return fmt.Sprintf("configuration error [%s]: %s", e.ModelID, e.Message)
}

// Unwrap returns the underlying error
func (e *ConfigurationError) Unwrap() error {
	return e.Err
}

// NewConfigurationError creates a new ConfigurationError
func NewConfigurationError(modelID, configKey string, err error, message string) *ConfigurationError {
	return &ConfigurationError{
		ModelID:   modelID,
		ConfigKey: configKey,
		Err:       err,
		Message:   message,
	}
}

// ContainerError represents a container-specific error with details
type ContainerError struct {
	ModelID     string
	ContainerID string
	Err         error
	Message     string
	State       string
}

// Error implements the error interface
func (e *ContainerError) Error() string {
	if e.ContainerID != "" {
		return fmt.Sprintf("container error [%s]: container %s (%s) - %s",
			e.ModelID, e.ContainerID, e.State, e.Message)
	}
	return fmt.Sprintf("container error [%s]: %s", e.ModelID, e.Message)
}

// Unwrap returns the underlying error
func (e *ContainerError) Unwrap() error {
	return e.Err
}

// NewContainerError creates a new ContainerError
func NewContainerError(modelID, containerID string, err error, message string) *ContainerError {
	return &ContainerError{
		ModelID:     modelID,
		ContainerID: containerID,
		Err:         err,
		Message:     message,
	}
}

// IsModelError checks if an error is a ModelError
func IsModelError(err error) bool {
	var modelErr *ModelError
	return errors.As(err, &modelErr)
}

// IsDownloadError checks if an error is a DownloadError
func IsDownloadError(err error) bool {
	var downloadErr *DownloadError
	return errors.As(err, &downloadErr)
}

// IsValidationError checks if an error is a ValidationError
func IsValidationError(err error) bool {
	var validationErr *ValidationError
	return errors.As(err, &validationErr)
}

// IsVersionError checks if an error is a VersionError
func IsVersionError(err error) bool {
	var versionErr *VersionError
	return errors.As(err, &versionErr)
}

// IsConfigurationError checks if an error is a ConfigurationError
func IsConfigurationError(err error) bool {
	var configErr *ConfigurationError
	return errors.As(err, &configErr)
}

// IsContainerError checks if an error is a ContainerError
func IsContainerError(err error) bool {
	var containerErr *ContainerError
	return errors.As(err, &containerErr)
}

// WrapError wraps an error with additional context
func WrapError(err error, operation, message string) error {
	return fmt.Errorf("%s: %s: %w", operation, message, err)
}

// WrapModelError wraps an error with model context
func WrapModelError(modelID string, err error, operation, message string) error {
	return NewModelError(modelID, operation, err, message)
}
