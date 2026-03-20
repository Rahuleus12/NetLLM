package inference

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ResponseFormat represents the format for response output
type ResponseFormat string

const (
	FormatOpenAI ResponseFormat = "openai"
	FormatJSON   ResponseFormat = "json"
	FormatText   ResponseFormat = "text"
	FormatCustom ResponseFormat = "custom"
)

// FormatterConfig represents configuration for response formatting
type FormatterConfig struct {
	DefaultFormat    ResponseFormat `json:"default_format"`
	IncludeMetadata  bool           `json:"include_metadata"`
	IncludeTiming    bool           `json:"include_timing"`
	PrettyPrint      bool           `json:"pretty_print"`
	IncludeTokens    bool           `json:"include_tokens"`
	StreamDelimiter  string         `json:"stream_delimiter"`
	CustomTemplate   string         `json:"custom_template"`
}

// ResponseFormatter handles formatting of inference responses
type ResponseFormatter struct {
	config *FormatterConfig
}

// NewResponseFormatter creates a new response formatter
func NewResponseFormatter(config *FormatterConfig) *ResponseFormatter {
	if config == nil {
		config = &FormatterConfig{
			DefaultFormat:   FormatOpenAI,
			IncludeMetadata: true,
			IncludeTiming:   true,
			PrettyPrint:     false,
			IncludeTokens:   true,
			StreamDelimiter: "\n",
		}
	}

	return &ResponseFormatter{
		config: config,
	}
}

// FormatResponse formats an inference response based on the specified format
func (f *ResponseFormatter) FormatResponse(resp *InferenceResponse, format ResponseFormat) ([]byte, error) {
	if format == "" {
		format = f.config.DefaultFormat
	}

	switch format {
	case FormatOpenAI:
		return f.formatOpenAI(resp)
	case FormatJSON:
		return f.formatJSON(resp)
	case FormatText:
		return f.formatText(resp)
	case FormatCustom:
		return f.formatCustom(resp)
	default:
		return f.formatOpenAI(resp)
	}
}

// formatOpenAI formats response in OpenAI-compatible format
func (f *ResponseFormatter) formatOpenAI(resp *InferenceResponse) ([]byte, error) {
	openAIResp := &OpenAIResponse{
		ID:      resp.ID,
		Object:  "chat.completion",
		Created: resp.CreatedAt.Unix(),
		Model:   resp.ModelID,
		Choices: []OpenAIChoice{
			{
				Index: 0,
				Message: OpenAIMessage{
					Role:    "assistant",
					Content: resp.Content,
				},
				FinishReason: resp.FinishReason,
			},
		},
		Usage: OpenAIUsage{
			PromptTokens:     resp.InputTokens,
			CompletionTokens: resp.OutputTokens,
			TotalTokens:      resp.TotalTokens,
		},
	}

	if f.config.IncludeTiming {
		openAIResp.Latency = resp.Latency.String()
		openAIResp.TimeToFirstToken = resp.TimeToFirstToken.String()
		openAIResp.TokensPerSecond = resp.TokensPerSecond
	}

	if f.config.IncludeMetadata && resp.Metadata != nil {
		openAIResp.Metadata = resp.Metadata
	}

	if f.config.PrettyPrint {
		return json.MarshalIndent(openAIResp, "", "  ")
	}

	return json.Marshal(openAIResp)
}

// formatJSON formats response in standard JSON format
func (f *ResponseFormatter) formatJSON(resp *InferenceResponse) ([]byte, error) {
	output := map[string]interface{}{
		"id":             resp.ID,
		"request_id":     resp.RequestID,
		"model_id":       resp.ModelID,
		"instance_id":    resp.InstanceID,
		"content":        resp.Content,
		"finish_reason":  resp.FinishReason,
		"created_at":     resp.CreatedAt,
	}

	if f.config.IncludeTokens {
		output["input_tokens"] = resp.InputTokens
		output["output_tokens"] = resp.OutputTokens
		output["total_tokens"] = resp.TotalTokens
	}

	if f.config.IncludeTiming {
		output["latency"] = resp.Latency.String()
		output["time_to_first_token"] = resp.TimeToFirstToken.String()
		output["tokens_per_second"] = resp.TokensPerSecond
	}

	if f.config.IncludeMetadata {
		output["metadata"] = resp.Metadata
	}

	if resp.Probabilities != nil {
		output["probabilities"] = resp.Probabilities
	}

	if resp.Alternatives != nil {
		output["alternatives"] = resp.Alternatives
	}

	if f.config.PrettyPrint {
		return json.MarshalIndent(output, "", "  ")
	}

	return json.Marshal(output)
}

// formatText formats response as plain text
func (f *ResponseFormatter) formatText(resp *InferenceResponse) ([]byte, error) {
	var builder strings.Builder

	builder.WriteString(resp.Content)

	if f.config.IncludeTokens {
		builder.WriteString(fmt.Sprintf("\n\n---\nTokens: %d input, %d output, %d total",
			resp.InputTokens, resp.OutputTokens, resp.TotalTokens))
	}

	if f.config.IncludeTiming {
		builder.WriteString(fmt.Sprintf("\nLatency: %v", resp.Latency))
		builder.WriteString(fmt.Sprintf("\nTokens/sec: %.2f", resp.TokensPerSecond))
	}

	builder.WriteString("\n")

	return []byte(builder.String()), nil
}

// formatCustom formats response using a custom template
func (f *ResponseFormatter) formatCustom(resp *InferenceResponse) ([]byte, error) {
	// Simple template replacement - could be enhanced with text/template
	output := f.config.CustomTemplate

	output = strings.ReplaceAll(output, "{content}", resp.Content)
	output = strings.ReplaceAll(output, "{id}", resp.ID)
	output = strings.ReplaceAll(output, "{model_id}", resp.ModelID)
	output = strings.ReplaceAll(output, "{finish_reason}", resp.FinishReason)
	output = strings.ReplaceAll(output, "{input_tokens}", fmt.Sprintf("%d", resp.InputTokens))
	output = strings.ReplaceAll(output, "{output_tokens}", fmt.Sprintf("%d", resp.OutputTokens))
	output = strings.ReplaceAll(output, "{total_tokens}", fmt.Sprintf("%d", resp.TotalTokens))
	output = strings.ReplaceAll(output, "{latency}", resp.Latency.String())
	output = strings.ReplaceAll(output, "{tokens_per_second}", fmt.Sprintf("%.2f", resp.TokensPerSecond))

	return []byte(output), nil
}

// FormatStreamChunk formats a streaming chunk
func (f *ResponseFormatter) FormatStreamChunk(chunk *StreamChunk, format ResponseFormat) ([]byte, error) {
	if format == "" {
		format = f.config.DefaultFormat
	}

	switch format {
	case FormatOpenAI:
		return f.formatStreamChunkOpenAI(chunk)
	case FormatJSON:
		return f.formatStreamChunkJSON(chunk)
	case FormatText:
		return f.formatStreamChunkText(chunk)
	default:
		return f.formatStreamChunkOpenAI(chunk)
	}
}

// formatStreamChunkOpenAI formats a streaming chunk in OpenAI format
func (f *ResponseFormatter) formatStreamChunkOpenAI(chunk *StreamChunk) ([]byte, error) {
	openAIChunk := &OpenAIStreamChunk{
		ID:      chunk.ID,
		Object:  "chat.completion.chunk",
		Created: chunk.CreatedAt.Unix(),
		Model:   chunk.ModelID,
		Choices: []OpenAIStreamChoice{
			{
				Index: 0,
				Delta: OpenAIMessage{
					Role:    "assistant",
					Content: chunk.Delta,
				},
				FinishReason: chunk.FinishReason,
			},
		},
	}

	if chunk.Error != nil {
		openAIChunk.Error = chunk.Error
	}

	data, err := json.Marshal(openAIChunk)
	if err != nil {
		return nil, err
	}

	return []byte(fmt.Sprintf("data: %s%s", string(data), f.config.StreamDelimiter)), nil
}

// formatStreamChunkJSON formats a streaming chunk in JSON format
func (f *ResponseFormatter) formatStreamChunkJSON(chunk *StreamChunk) ([]byte, error) {
	output := map[string]interface{}{
		"id":            chunk.ID,
		"request_id":    chunk.RequestID,
		"model_id":      chunk.ModelID,
		"instance_id":   chunk.InstanceID,
		"content":       chunk.Content,
		"delta":         chunk.Delta,
		"finish_reason": chunk.FinishReason,
		"created_at":    chunk.CreatedAt,
	}

	if chunk.Error != nil {
		output["error"] = chunk.Error
	}

	if f.config.IncludeTokens {
		output["input_tokens"] = chunk.InputTokens
		output["output_tokens"] = chunk.OutputTokens
	}

	if f.config.IncludeTiming {
		output["latency"] = chunk.Latency.String()
	}

	data, err := json.Marshal(output)
	if err != nil {
		return nil, err
	}

	return []byte(fmt.Sprintf("%s%s", string(data), f.config.StreamDelimiter)), nil
}

// formatStreamChunkText formats a streaming chunk as plain text
func (f *ResponseFormatter) formatStreamChunkText(chunk *StreamChunk) ([]byte, error) {
	return []byte(chunk.Delta), nil
}

// FormatBatchResponse formats a batch response
func (f *ResponseFormatter) FormatBatchResponse(resp *BatchResponse, format ResponseFormat) ([]byte, error) {
	if format == "" {
		format = f.config.DefaultFormat
	}

	switch format {
	case FormatJSON:
		return f.formatBatchJSON(resp)
	default:
		return f.formatBatchJSON(resp)
	}
}

// formatBatchJSON formats a batch response in JSON
func (f *ResponseFormatter) formatBatchJSON(resp *BatchResponse) ([]byte, error) {
	output := map[string]interface{}{
		"id":           resp.ID,
		"batch_id":     resp.BatchID,
		"model_id":     resp.ModelID,
		"status":       resp.Status,
		"total":        resp.Total,
		"succeeded":    resp.Succeeded,
		"failed":       resp.Failed,
		"duration":     resp.Duration.String(),
		"created_at":   resp.CreatedAt,
	}

	if resp.CompletedAt != nil {
		output["completed_at"] = resp.CompletedAt
	}

	if resp.Metadata != nil {
		output["metadata"] = resp.Metadata
	}

	// Format individual results
	formattedResults := make([]map[string]interface{}, len(resp.Results))
	for i, result := range resp.Results {
		formattedResult := map[string]interface{}{
			"index":      result.Index,
			"request_id": result.RequestID,
			"success":    result.Success,
			"duration":   result.Duration.String(),
		}

		if result.Success && result.Response != nil {
			formattedResult["content"] = result.Response.Content
			formattedResult["finish_reason"] = result.Response.FinishReason
			if f.config.IncludeTokens {
				formattedResult["input_tokens"] = result.Response.InputTokens
				formattedResult["output_tokens"] = result.Response.OutputTokens
				formattedResult["total_tokens"] = result.Response.TotalTokens
			}
		}

		if result.Error != nil {
			formattedResult["error"] = result.Error
		}

		formattedResults[i] = formattedResult
	}

	output["results"] = formattedResults

	if f.config.PrettyPrint {
		return json.MarshalIndent(output, "", "  ")
	}

	return json.Marshal(output)
}

// FormatError formats an error response
func (f *ResponseFormatter) FormatError(err *InferenceError, format ResponseFormat) ([]byte, error) {
	if format == "" {
		format = f.config.DefaultFormat
	}

	errorResp := map[string]interface{}{
		"error": map[string]interface{}{
			"code":       err.Code,
			"message":    err.Message,
			"details":    err.Details,
			"retryable":  err.Retryable,
			"timestamp":  err.Timestamp,
		},
	}

	if err.ModelID != "" {
		errorResp["error"].(map[string]interface{})["model_id"] = err.ModelID
	}

	if err.InstanceID != "" {
		errorResp["error"].(map[string]interface{})["instance_id"] = err.InstanceID
	}

	if err.RequestID != "" {
		errorResp["error"].(map[string]interface{})["request_id"] = err.RequestID
	}

	if err.Metadata != nil {
		errorResp["error"].(map[string]interface{})["metadata"] = err.Metadata
	}

	if f.config.PrettyPrint {
		return json.MarshalIndent(errorResp, "", "  ")
	}

	return json.Marshal(errorResp)
}

// OpenAI-compatible response types

// OpenAIResponse represents an OpenAI-compatible response
type OpenAIResponse struct {
	ID                string                 `json:"id"`
	Object            string                 `json:"object"`
	Created           int64                  `json:"created"`
	Model             string                 `json:"model"`
	Choices           []OpenAIChoice         `json:"choices"`
	Usage             OpenAIUsage            `json:"usage"`
	Latency           string                 `json:"latency,omitempty"`
	TimeToFirstToken  string                 `json:"time_to_first_token,omitempty"`
	TokensPerSecond   float64                `json:"tokens_per_second,omitempty"`
	Metadata          map[string]interface{} `json:"metadata,omitempty"`
}

// OpenAIChoice represents a choice in an OpenAI response
type OpenAIChoice struct {
	Index        int          `json:"index"`
	Message      OpenAIMessage `json:"message"`
	FinishReason string       `json:"finish_reason"`
}

// OpenAIMessage represents a message in OpenAI format
type OpenAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	Name    string `json:"name,omitempty"`
}

// OpenAIUsage represents token usage in OpenAI format
type OpenAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// OpenAIStreamChunk represents a streaming chunk in OpenAI format
type OpenAIStreamChunk struct {
	ID      string               `json:"id"`
	Object  string               `json:"object"`
	Created int64                `json:"created"`
	Model   string               `json:"model"`
	Choices []OpenAIStreamChoice `json:"choices"`
	Error   *StreamError         `json:"error,omitempty"`
}

// OpenAIStreamChoice represents a choice in an OpenAI stream chunk
type OpenAIStreamChoice struct {
	Index        int          `json:"index"`
	Delta        OpenAIMessage `json:"delta"`
	FinishReason string       `json:"finish_reason"`
}

// CountTokens estimates the number of tokens in a text
func (f *ResponseFormatter) CountTokens(text string) int {
	// Simple estimation: ~4 characters per token on average
	// This is a rough approximation - real implementation would use a proper tokenizer
	if len(text) == 0 {
		return 0
	}

	// Count words and punctuation as a better approximation
	words := strings.Fields(text)
	tokenCount := 0

	for _, word := range words {
		// Long words might be split into multiple tokens
		if len(word) > 6 {
			tokenCount += 2
		} else {
			tokenCount++
		}
	}

	return tokenCount
}

// CountMessageTokens estimates tokens for chat messages
func (f *ResponseFormatter) CountMessageTokens(messages []ChatMessage) int {
	total := 0

	for _, msg := range messages {
		// Add tokens for role and content
		total += f.CountTokens(msg.Role)
		total += f.CountTokens(msg.Content)

		// Add overhead for message structure (~4 tokens per message)
		total += 4
	}

	return total
}

// CalculateTokensPerSecond calculates tokens per second
func (f *ResponseFormatter) CalculateTokensPerSecond(outputTokens int, duration time.Duration) float64 {
	if duration == 0 || outputTokens == 0 {
		return 0
	}

	seconds := duration.Seconds()
	if seconds == 0 {
		return 0
	}

	return float64(outputTokens) / seconds
}

// GetFinalStreamChunk returns the final chunk for a stream
func (f *ResponseFormatter) GetFinalStreamChunk(requestID, modelID, instanceID string, format ResponseFormat) ([]byte, error) {
	chunk := &StreamChunk{
		ID:           requestID,
		RequestID:    requestID,
		ModelID:      modelID,
		InstanceID:   instanceID,
		Content:      "",
		Delta:        "",
		FinishReason: "stop",
		CreatedAt:    time.Now(),
	}

	return f.FormatStreamChunk(chunk, format)
}

// GetStreamDoneMessage returns the done message for SSE streams
func (f *ResponseFormatter) GetStreamDoneMessage() []byte {
	return []byte(fmt.Sprintf("data: [DONE]%s", f.config.StreamDelimiter))
}
