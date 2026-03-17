package models

import (
	"context"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ValidationEngine provides comprehensive model validation
type ValidationEngine struct {
	registry ModelRegistry
	mu       sync.RWMutex
}

// NewValidationEngine creates a new validation engine
func NewValidationEngine(registry ModelRegistry) *ValidationEngine {
	return &ValidationEngine{
		registry: registry,
	}
}

// Validate performs comprehensive validation on a model
func (ve *ValidationEngine) Validate(ctx context.Context, modelID string) (*ValidationResult, error) {
	startTime := time.Now()

	// Get model from registry
	model, err := ve.registry.Get(ctx, modelID)
	if err != nil {
		return nil, fmt.Errorf("failed to get model: %w", err)
	}

	result := &ValidationResult{
		ModelID:     modelID,
		Status:      ValidationValid,
		Checks:      make(map[string]CheckResult),
		Errors:      []string{},
		Warnings:    []string{},
		ValidatedAt: time.Now(),
	}

	// Run all validation checks
	ve.validateChecksum(model, result)
	ve.validateFormat(model, result)
	ve.validateSize(model, result)
	ve.validateIntegrity(model, result)
	ve.validateRequirements(model, result)
	ve.validateConfiguration(model, result)

	// Determine overall status
	ve.determineOverallStatus(result)

	// Set duration
	result.Duration = time.Since(startTime).Milliseconds()

	log.Printf("Validation completed for model %s: %s (duration: %dms)",
		modelID, result.Status, result.Duration)

	return result, nil
}

// validateChecksum validates the model file checksum
func (ve *ValidationEngine) validateChecksum(model *Model, result *ValidationResult) {
	check := CheckResult{
		Name:   "checksum",
		Status: ValidationValid,
	}

	// Skip if no checksum provided
	if model.Source.Checksum == "" {
		check.Status = ValidationWarning
		check.Message = "No checksum provided for validation"
		result.Warnings = append(result.Warnings, "Model has no checksum to validate")
		result.Checks["checksum"] = check
		return
	}

	// Check if file exists
	if model.FileInfo.Path == "" {
		check.Status = ValidationInvalid
		check.Message = "Model file path not specified"
		result.Errors = append(result.Errors, "Model file path is empty")
		result.Checks["checksum"] = check
		return
	}

	if _, err := os.Stat(model.FileInfo.Path); os.IsNotExist(err) {
		check.Status = ValidationInvalid
		check.Message = "Model file does not exist"
		result.Errors = append(result.Errors, fmt.Sprintf("Model file not found: %s", model.FileInfo.Path))
		result.Checks["checksum"] = check
		return
	}

	// Calculate checksum
	actualChecksum, err := ve.calculateChecksum(model.FileInfo.Path, model.Source.Checksum)
	if err != nil {
		check.Status = ValidationInvalid
		check.Message = fmt.Sprintf("Failed to calculate checksum: %v", err)
		result.Errors = append(result.Errors, check.Message)
		result.Checks["checksum"] = check
		return
	}

	// Compare checksums
	expectedChecksum := strings.ToLower(strings.TrimPrefix(model.Source.Checksum, "sha256:"))
	expectedChecksum = strings.TrimPrefix(expectedChecksum, "md5:")
	expectedChecksum = strings.TrimPrefix(expectedChecksum, "sha1:")

	if actualChecksum != expectedChecksum {
		check.Status = ValidationInvalid
		check.Expected = model.Source.Checksum
		check.Actual = actualChecksum
		check.Message = "Checksum mismatch"
		result.Errors = append(result.Errors,
			fmt.Sprintf("Checksum mismatch: expected %s, got %s", model.Source.Checksum, actualChecksum))
	} else {
		check.Message = "Checksum verified successfully"
		check.Actual = actualChecksum
	}

	result.Checks["checksum"] = check
}

// validateFormat validates the model format
func (ve *ValidationEngine) validateFormat(model *Model, result *ValidationResult) {
	check := CheckResult{
		Name:   "format",
		Status: ValidationValid,
	}

	// Validate format is supported
	validFormats := map[ModelFormat]bool{
		FormatGGUF:       true,
		FormatONNX:       true,
		FormatPyTorch:    true,
		FormatTensorFlow: true,
		FormatSafeTensors: true,
		FormatCustom:     true,
	}

	if !validFormats[model.Format] {
		check.Status = ValidationInvalid
		check.Message = fmt.Sprintf("Unsupported model format: %s", model.Format)
		result.Errors = append(result.Errors, check.Message)
		result.Checks["format"] = check
		return
	}

	// Try to detect actual format from file
	if model.FileInfo.Path != "" {
		detectedFormat, err := ve.detectModelFormat(model.FileInfo.Path)
		if err == nil && detectedFormat != "" && detectedFormat != model.Format {
			check.Status = ValidationWarning
			check.Expected = model.Format
			check.Actual = detectedFormat
			check.Message = fmt.Sprintf("Detected format (%s) differs from specified format (%s)",
				detectedFormat, model.Format)
			result.Warnings = append(result.Warnings, check.Message)
		} else if err == nil {
			check.Message = "Format validated successfully"
		}
	} else {
		check.Message = "Format specified (file not available for detection)"
	}

	result.Checks["format"] = check
}

// validateSize validates the model file size
func (ve *ValidationEngine) validateSize(model *Model, result *ValidationResult) {
	check := CheckResult{
		Name:   "size",
		Status: ValidationValid,
	}

	// Check if file exists
	if model.FileInfo.Path == "" {
		check.Status = ValidationWarning
		check.Message = "Model file path not specified"
		result.Warnings = append(result.Warnings, "Cannot validate size: file path not specified")
		result.Checks["size"] = check
		return
	}

	fileInfo, err := os.Stat(model.FileInfo.Path)
	if os.IsNotExist(err) {
		check.Status = ValidationInvalid
		check.Message = "Model file does not exist"
		result.Errors = append(result.Errors, fmt.Sprintf("Model file not found: %s", model.FileInfo.Path))
		result.Checks["size"] = check
		return
	}

	if err != nil {
		check.Status = ValidationInvalid
		check.Message = fmt.Sprintf("Failed to get file info: %v", err)
		result.Errors = append(result.Errors, check.Message)
		result.Checks["size"] = check
		return
	}

	actualSize := fileInfo.Size()
	check.Actual = actualSize

	// Check if size matches expected
	if model.FileInfo.SizeBytes > 0 && actualSize != model.FileInfo.SizeBytes {
		check.Status = ValidationWarning
		check.Expected = model.FileInfo.SizeBytes
		check.Message = fmt.Sprintf("File size mismatch: expected %d bytes, got %d bytes",
			model.FileInfo.SizeBytes, actualSize)
		result.Warnings = append(result.Warnings, check.Message)
	} else {
		check.Message = fmt.Sprintf("File size: %d bytes", actualSize)
	}

	// Check minimum size (at least 1KB)
	if actualSize < 1024 {
		check.Status = ValidationWarning
		check.Message = fmt.Sprintf("File size is very small: %d bytes", actualSize)
		result.Warnings = append(result.Warnings, check.Message)
	}

	result.Checks["size"] = check
}

// validateIntegrity validates the model file integrity
func (ve *ValidationEngine) validateIntegrity(model *Model, result *ValidationResult) {
	check := CheckResult{
		Name:   "integrity",
		Status: ValidationValid,
	}

	// Check if file exists
	if model.FileInfo.Path == "" {
		check.Status = ValidationWarning
		check.Message = "Model file path not specified"
		result.Warnings = append(result.Warnings, "Cannot validate integrity: file path not specified")
		result.Checks["integrity"] = check
		return
	}

	// Open file and check for basic integrity
	file, err := os.Open(model.FileInfo.Path)
	if err != nil {
		check.Status = ValidationInvalid
		check.Message = fmt.Sprintf("Failed to open file: %v", err)
		result.Errors = append(result.Errors, check.Message)
		result.Checks["integrity"] = check
		return
	}
	defer file.Close()

	// Check if file is readable
	buffer := make([]byte, 1024)
	_, err = file.Read(buffer)
	if err != nil && err != io.EOF {
		check.Status = ValidationInvalid
		check.Message = fmt.Sprintf("File is not readable: %v", err)
		result.Errors = append(result.Errors, check.Message)
		result.Checks["integrity"] = check
		return
	}

	// Check for common file corruption indicators based on format
	if model.Format == FormatGGUF {
		if err := ve.validateGGUFIntegrity(model.FileInfo.Path, result); err != nil {
			check.Status = ValidationInvalid
			check.Message = fmt.Sprintf("GGUF integrity check failed: %v", err)
			result.Errors = append(result.Errors, check.Message)
			result.Checks["integrity"] = check
			return
		}
	}

	check.Message = "File integrity validated"
	result.Checks["integrity"] = check
}

// validateRequirements validates model resource requirements
func (ve *ValidationEngine) validateRequirements(model *Model, result *ValidationResult) {
	check := CheckResult{
		Name:   "requirements",
		Status: ValidationValid,
	}

	// Validate RAM requirement
	if model.Requirements.RAMMin <= 0 {
		check.Status = ValidationWarning
		check.Message = "Minimum RAM requirement not specified"
		result.Warnings = append(result.Warnings, "Model should specify minimum RAM requirement")
	}

	// Validate CPU cores
	if model.Requirements.CPUCores <= 0 {
		check.Status = ValidationWarning
		check.Message = "CPU cores requirement not specified"
		result.Warnings = append(result.Warnings, "Model should specify CPU cores requirement")
	}

	// Validate GPU requirement consistency
	if model.Requirements.GPURequired && model.Requirements.GPUMemory <= 0 {
		check.Status = ValidationWarning
		check.Message = "GPU required but GPU memory not specified"
		result.Warnings = append(result.Warnings, "GPU memory should be specified when GPU is required")
	}

	if check.Status == ValidationValid {
		check.Message = "Resource requirements are valid"
	}

	result.Checks["requirements"] = check
}

// validateConfiguration validates model configuration
func (ve *ValidationEngine) validateConfiguration(model *Model, result *ValidationResult) {
	check := CheckResult{
		Name:   "configuration",
		Status: ValidationValid,
	}

	// Validate context length
	if model.Config.ContextLength <= 0 {
		check.Status = ValidationWarning
		check.Message = "Context length not specified or invalid"
		result.Warnings = append(result.Warnings, "Model should specify a valid context length")
	}

	// Validate temperature
	if model.Config.Temperature < 0 || model.Config.Temperature > 2.0 {
		check.Status = ValidationWarning
		check.Message = fmt.Sprintf("Temperature value seems unusual: %.2f", model.Config.Temperature)
		result.Warnings = append(result.Warnings, check.Message)
	}

	// Validate max tokens
	if model.Config.MaxTokens <= 0 {
		check.Status = ValidationWarning
		check.Message = "Max tokens not specified or invalid"
		result.Warnings = append(result.Warnings, "Model should specify max tokens")
	}

	// Validate top_p
	if model.Config.TopP < 0 || model.Config.TopP > 1.0 {
		check.Status = ValidationWarning
		check.Message = fmt.Sprintf("TopP value out of range [0, 1]: %.2f", model.Config.TopP)
		result.Warnings = append(result.Warnings, check.Message)
	}

	// Validate top_k
	if model.Config.TopK < 0 {
		check.Status = ValidationWarning
		check.Message = fmt.Sprintf("TopK value is negative: %d", model.Config.TopK)
		result.Warnings = append(result.Warnings, check.Message)
	}

	if check.Status == ValidationValid {
		check.Message = "Model configuration is valid"
	}

	result.Checks["configuration"] = check
}

// calculateChecksum calculates the checksum of a file
func (ve *ValidationEngine) calculateChecksum(filePath, expectedChecksum string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	// Determine hash algorithm based on expected checksum
	var hash interface{}
	expectedChecksum = strings.ToLower(expectedChecksum)

	if strings.HasPrefix(expectedChecksum, "sha256:") || len(expectedChecksum) == 64 {
		hash = sha256.New()
	} else if strings.HasPrefix(expectedChecksum, "md5:") || len(expectedChecksum) == 32 {
		hash = md5.New()
	} else if strings.HasPrefix(expectedChecksum, "sha1:") || len(expectedChecksum) == 40 {
		hash = sha1.New()
	} else {
		// Default to SHA256
		hash = sha256.New()
	}

	if _, err := io.Copy(hash.(io.Writer), file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.(interface{ Sum([]byte) []byte }).Sum(nil)), nil
}

// detectModelFormat attempts to detect the model format from file
func (ve *ValidationEngine) detectModelFormat(filePath string) (ModelFormat, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	// Read first few bytes to check magic numbers
	buffer := make([]byte, 512)
	n, err := file.Read(buffer)
	if err != nil && err != io.EOF {
		return "", err
	}

	buffer = buffer[:n]

	// Check for GGUF magic number
	if len(buffer) >= 4 {
		// GGUF magic: "GGUF"
		if string(buffer[0:4]) == "GGUF" {
			return FormatGGUF, nil
		}

		// Check for ONNX (protobuf magic)
		if buffer[0] == 0x08 && buffer[1] == 0x01 {
			return FormatONNX, nil
		}

		// Check for PyTorch (PK zip for .pt files)
		if buffer[0] == 0x50 && buffer[1] == 0x4B { // PK
			return FormatPyTorch, nil
		}

		// Check for SafeTensors
		if len(buffer) >= 8 {
			// SafeTensors starts with a JSON-like header length
			// This is a simplified check
			if buffer[0] == '{' || (buffer[0] >= '0' && buffer[0] <= '9') {
				ext := strings.ToLower(filepath.Ext(filePath))
				if ext == ".safetensors" {
					return FormatSafeTensors, nil
				}
			}
		}
	}

	// Fall back to extension-based detection
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".gguf":
		return FormatGGUF, nil
	case ".onnx":
		return FormatONNX, nil
	case ".pt", ".pth", ".bin":
		return FormatPyTorch, nil
	case ".safetensors":
		return FormatSafeTensors, nil
	case ".pb":
		return FormatTensorFlow, nil
	default:
		return FormatCustom, nil
	}
}

// validateGGUFIntegrity performs GGUF-specific integrity checks
func (ve *ValidationEngine) validateGGUFIntegrity(filePath string, result *ValidationResult) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	// Read GGUF header
	header := make([]byte, 4)
	if _, err := file.Read(header); err != nil {
		return fmt.Errorf("failed to read GGUF header: %w", err)
	}

	// Verify GGUF magic number
	if string(header) != "GGUF" {
		return fmt.Errorf("invalid GGUF magic number: expected 'GGUF', got '%s'", string(header))
	}

	// Read version (3 bytes)
	version := make([]byte, 3)
	if _, err := file.Read(version); err != nil {
		return fmt.Errorf("failed to read GGUF version: %w", err)
	}

	// Basic validation passed
	return nil
}

// determineOverallStatus determines the overall validation status
func (ve *ValidationEngine) determineOverallStatus(result *ValidationResult) {
	hasInvalid := false
	hasWarning := false

	for _, check := range result.Checks {
		if check.Status == ValidationInvalid {
			hasInvalid = true
			break
		}
		if check.Status == ValidationWarning {
			hasWarning = true
		}
	}

	if hasInvalid {
		result.Status = ValidationInvalid
	} else if hasWarning {
		result.Status = ValidationWarning
	} else {
		result.Status = ValidationValid
	}
}

// ValidateBatch validates multiple models in parallel
func (ve *ValidationEngine) ValidateBatch(ctx context.Context, modelIDs []string) map[string]*ValidationResult {
	results := make(map[string]*ValidationResult)
	var wg sync.WaitGroup
	var mu sync.Mutex

	// Validate models in parallel (max 5 concurrent)
	semaphore := make(chan struct{}, 5)

	for _, modelID := range modelIDs {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			result, err := ve.Validate(ctx, id)
			if err != nil {
				result = &ValidationResult{
					ModelID: id,
					Status:  ValidationInvalid,
					Errors:  []string{fmt.Sprintf("Validation failed: %v", err)},
				}
			}

			mu.Lock()
			results[id] = result
			mu.Unlock()
		}(modelID)
	}

	wg.Wait()
	return results
}

// ValidateChecksum validates only the checksum of a model
func (ve *ValidationEngine) ValidateChecksum(ctx context.Context, modelID string) error {
	model, err := ve.registry.Get(ctx, modelID)
	if err != nil {
		return fmt.Errorf("failed to get model: %w", err)
	}

	if model.Source.Checksum == "" {
		return fmt.Errorf("no checksum provided for model")
	}

	if model.FileInfo.Path == "" {
		return fmt.Errorf("model file path not specified")
	}

	actualChecksum, err := ve.calculateChecksum(model.FileInfo.Path, model.Source.Checksum)
	if err != nil {
		return fmt.Errorf("failed to calculate checksum: %w", err)
	}

	expectedChecksum := strings.ToLower(strings.TrimPrefix(model.Source.Checksum, "sha256:"))
	expectedChecksum = strings.TrimPrefix(expectedChecksum, "md5:")
	expectedChecksum = strings.TrimPrefix(expectedChecksum, "sha1:")

	if actualChecksum != expectedChecksum {
		return fmt.Errorf("checksum mismatch: expected %s, got %s",
			model.Source.Checksum, actualChecksum)
	}

	return nil
}

// ValidateFormat validates only the format of a model
func (ve *ValidationEngine) ValidateFormat(ctx context.Context, modelID string) error {
	model, err := ve.registry.Get(ctx, modelID)
	if err != nil {
		return fmt.Errorf("failed to get model: %w", err)
	}

	if model.FileInfo.Path == "" {
		return fmt.Errorf("model file path not specified")
	}

	detectedFormat, err := ve.detectModelFormat(model.FileInfo.Path)
	if err != nil {
		return fmt.Errorf("failed to detect format: %w", err)
	}

	if detectedFormat != model.Format && detectedFormat != FormatCustom {
		return fmt.Errorf("format mismatch: expected %s, detected %s",
			model.Format, detectedFormat)
	}

	return nil
}

// ValidateIntegrity validates only the integrity of a model
func (ve *ValidationEngine) ValidateIntegrity(ctx context.Context, modelID string) error {
	model, err := ve.registry.Get(ctx, modelID)
	if err != nil {
		return fmt.Errorf("failed to get model: %w", err)
	}

	if model.FileInfo.Path == "" {
		return fmt.Errorf("model file path not specified")
	}

	// Check if file exists
	if _, err := os.Stat(model.FileInfo.Path); os.IsNotExist(err) {
		return fmt.Errorf("model file does not exist: %s", model.FileInfo.Path)
	}

	// Check if file is readable
	file, err := os.Open(model.FileInfo.Path)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Try to read first bytes
	buffer := make([]byte, 1024)
	if _, err := file.Read(buffer); err != nil && err != io.EOF {
		return fmt.Errorf("file is not readable: %w", err)
	}

	// Format-specific integrity checks
	if model.Format == FormatGGUF {
		if err := ve.validateGGUFIntegrity(model.FileInfo.Path, nil); err != nil {
			return fmt.Errorf("GGUF integrity check failed: %w", err)
		}
	}

	return nil
}

// GetValidationStats returns validation statistics
func (ve *ValidationEngine) GetValidationStats(ctx context.Context) (map[string]interface{}, error) {
	stats := map[string]interface{}{
		"total_validations":   0,
		"valid_models":        0,
		"invalid_models":      0,
		"warnings":            0,
		"average_duration_ms": int64(0),
	}

	// In a real implementation, you would query this from a validation log
	// For now, return empty stats
	return stats, nil
}

// QuickValidation performs a quick validation without deep checks
func (ve *ValidationEngine) QuickValidation(ctx context.Context, modelID string) error {
	model, err := ve.registry.Get(ctx, modelID)
	if err != nil {
		return fmt.Errorf("failed to get model: %w", err)
	}

	// Check if file exists
	if model.FileInfo.Path == "" {
		return fmt.Errorf("model file path not specified")
	}

	if _, err := os.Stat(model.FileInfo.Path); os.IsNotExist(err) {
		return fmt.Errorf("model file does not exist")
	}

	// Basic format check
	validFormats := map[ModelFormat]bool{
		FormatGGUF:       true,
		FormatONNX:       true,
		FormatPyTorch:    true,
		FormatTensorFlow: true,
		FormatSafeTensors: true,
		FormatCustom:     true,
	}

	if !validFormats[model.Format] {
		return fmt.Errorf("unsupported model format: %s", model.Format)
	}

	return nil
}

// IsModelValid checks if a model has passed validation
func (ve *ValidationEngine) IsModelValid(ctx context.Context, modelID string) bool {
	// Quick check - file exists and format is valid
	if err := ve.QuickValidation(ctx, modelID); err != nil {
		return false
	}

	// In a real implementation, you would check a validation cache
	// to see if the model was recently validated
	return true
}
