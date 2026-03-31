package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"

	"github.com/ai-provider/internal/inference"
)

// InferenceHandlers handles inference-related API requests
type InferenceHandlers struct {
	executor *inference.InferenceExecutor
}

// NewInferenceHandlers creates a new inference handlers instance
func NewInferenceHandlers(executor *inference.InferenceExecutor) *InferenceHandlers {
	return &InferenceHandlers{
		executor: executor,
	}
}

// InferenceRequestBody represents the API request body for inference
type InferenceRequestBody struct {
	Model            string                  `json:"model"`
	Prompt           string                  `json:"prompt,omitempty"`
	Messages         []inference.ChatMessage `json:"messages,omitempty"`
	Stream           bool                    `json:"stream,omitempty"`
	MaxTokens        int                     `json:"max_tokens,omitempty"`
	Temperature      float64                 `json:"temperature,omitempty"`
	TopP             float64                 `json:"top_p,omitempty"`
	TopK             int                     `json:"top_k,omitempty"`
	Stop             []string                `json:"stop,omitempty"`
	FrequencyPenalty float64                 `json:"frequency_penalty,omitempty"`
	PresencePenalty  float64                 `json:"presence_penalty,omitempty"`
	User             string                  `json:"user,omitempty"`
	Metadata         map[string]interface{}  `json:"metadata,omitempty"`
}

// BatchInferenceRequestBody represents the API request body for batch inference
type BatchInferenceRequestBody struct {
	Model    string                  `json:"model"`
	Requests []InferenceRequestBody  `json:"requests"`
	Priority int                     `json:"priority,omitempty"`
	Metadata map[string]interface{}  `json:"metadata,omitempty"`
}

// AsyncInferenceResponse represents the response returned for async inference
type AsyncInferenceResponse struct {
	ID        string    `json:"id"`
	ModelID   string    `json:"model_id"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// CancelResponse represents the response for a cancelled request
type CancelResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// ActiveRequestsResponse represents the response for listing active requests
type ActiveRequestsResponse struct {
	Data  []inference.RequestStatus `json:"data"`
	Count int                       `json:"count"`
}

// ExecuteInference handles POST /api/v1/inference/{model_id}
// Performs synchronous inference on the specified model.
//
// Request body:
//
//	{
//	  "prompt": "string or use messages",
//	  "messages": [{"role": "user", "content": "hello"}],
//	  "max_tokens": 512,
//	  "temperature": 0.7,
//	  "top_p": 0.9,
//	  "top_k": 40,
//	  "stop": ["\n"],
//	  "frequency_penalty": 0.0,
//	  "presence_penalty": 0.0,
//	  "user": "optional-user-id",
//	  "metadata": {}
//	}
func (h *InferenceHandlers) ExecuteInference(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	modelID := vars["model_id"]
	if modelID == "" {
		h.respondInferenceError(w, http.StatusBadRequest, "model_id is required", "invalid_request")
		return
	}

	var reqBody InferenceRequestBody
	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
		h.respondInferenceError(w, http.StatusBadRequest, "invalid request body: "+err.Error(), "invalid_request")
		return
	}

	req := h.buildInferenceRequest(modelID, reqBody, inference.ModeSync, 60*time.Second)

	if req.Prompt == "" && len(req.Messages) == 0 {
		h.respondInferenceError(w, http.StatusBadRequest, "either 'prompt' or 'messages' must be provided", "invalid_request")
		return
	}

	response, err := h.executor.Execute(r.Context(), req)
	if err != nil {
		log.Printf("Inference error for model %s: %v", modelID, err)
		h.respondInferenceError(w, http.StatusInternalServerError, "inference failed: "+err.Error(), "inference_error")
		return
	}

	h.respondInferenceJSON(w, http.StatusOK, response)
}

// ExecuteInferenceAsync handles POST /api/v1/inference/{model_id}/async
// Starts an asynchronous inference request and returns a request ID for polling.
//
// Response: 202 Accepted with { "id": "...", "model_id": "...", "status": "processing" }
func (h *InferenceHandlers) ExecuteInferenceAsync(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	modelID := vars["model_id"]
	if modelID == "" {
		h.respondInferenceError(w, http.StatusBadRequest, "model_id is required", "invalid_request")
		return
	}

	var reqBody InferenceRequestBody
	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
		h.respondInferenceError(w, http.StatusBadRequest, "invalid request body: "+err.Error(), "invalid_request")
		return
	}

	req := h.buildInferenceRequest(modelID, reqBody, inference.ModeSync, 120*time.Second)

	if req.Prompt == "" && len(req.Messages) == 0 {
		h.respondInferenceError(w, http.StatusBadRequest, "either 'prompt' or 'messages' must be provided", "invalid_request")
		return
	}

	resultChan, err := h.executor.ExecuteAsync(r.Context(), req)
	if err != nil {
		log.Printf("Async inference submission error for model %s: %v", modelID, err)
		h.respondInferenceError(w, http.StatusInternalServerError, "failed to submit inference request: "+err.Error(), "inference_error")
		return
	}

	h.respondInferenceJSON(w, http.StatusAccepted, AsyncInferenceResponse{
		ID:        req.ID,
		ModelID:   modelID,
		Status:    "processing",
		CreatedAt: req.CreatedAt,
	})

	// Consume the result in background to prevent goroutine leaks
	go func() {
		select {
		case <-resultChan:
		case <-time.After(5 * time.Minute):
		}
	}()
}

// GetInferenceStatus handles GET /api/v1/inference/requests/{request_id}
// Returns the status of an async inference request
func (h *InferenceHandlers) GetInferenceStatus(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	requestID := vars["request_id"]
	if requestID == "" {
		h.respondInferenceError(w, http.StatusBadRequest, "request_id is required", "invalid_request")
		return
	}

	status, err := h.executor.GetRequestStatus(requestID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			h.respondInferenceError(w, http.StatusNotFound, "request not found", "not_found")
			return
		}
		h.respondInferenceError(w, http.StatusInternalServerError, "failed to get request status: "+err.Error(), "internal_error")
		return
	}

	h.respondInferenceJSON(w, http.StatusOK, status)
}

// CancelInference handles DELETE /api/v1/inference/requests/{request_id}
// Cancels a pending or running inference request
func (h *InferenceHandlers) CancelInference(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	requestID := vars["request_id"]
	if requestID == "" {
		h.respondInferenceError(w, http.StatusBadRequest, "request_id is required", "invalid_request")
		return
	}

	if err := h.executor.CancelRequest(requestID); err != nil {
		if strings.Contains(err.Error(), "not found") {
			h.respondInferenceError(w, http.StatusNotFound, "request not found", "not_found")
			return
		}
		h.respondInferenceError(w, http.StatusInternalServerError, "failed to cancel request: "+err.Error(), "internal_error")
		return
	}

	h.respondInferenceJSON(w, http.StatusOK, CancelResponse{
		ID:     requestID,
		Status: "cancelled",
	})
}

// ExecuteBatch handles POST /api/v1/inference/batch
// Submits a batch of inference requests for parallel processing.
//
// Request body:
//
//	{
//	  "model": "model-id",
//	  "requests": [
//	    {"prompt": "...", "max_tokens": 256},
//	    {"messages": [{"role": "user", "content": "..."}]}
//	  ],
//	  "priority": 5,
//	  "metadata": {}
//	}
func (h *InferenceHandlers) ExecuteBatch(w http.ResponseWriter, r *http.Request) {
	var reqBody BatchInferenceRequestBody
	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
		h.respondInferenceError(w, http.StatusBadRequest, "invalid request body: "+err.Error(), "invalid_request")
		return
	}

	if reqBody.Model == "" {
		h.respondInferenceError(w, http.StatusBadRequest, "model is required", "invalid_request")
		return
	}
	if len(reqBody.Requests) == 0 {
		h.respondInferenceError(w, http.StatusBadRequest, "at least one request is required", "invalid_request")
		return
	}
	if len(reqBody.Requests) > 100 {
		h.respondInferenceError(w, http.StatusBadRequest, "maximum 100 requests per batch", "invalid_request")
		return
	}

	requests := make([]inference.InferenceRequest, 0, len(reqBody.Requests))
	for i, req := range reqBody.Requests {
		if req.Prompt == "" && len(req.Messages) == 0 {
			h.respondInferenceError(w, http.StatusBadRequest,
				fmt.Sprintf("request %d: either 'prompt' or 'messages' must be provided", i), "invalid_request")
			return
		}

		requests = append(requests, inference.InferenceRequest{
			ID:               uuid.New().String(),
			ModelID:          reqBody.Model,
			Mode:             inference.ModeBatch,
			Priority:         inference.RequestPriority(defaultInt(reqBody.Priority, int(inference.PriorityNormal))),
			Prompt:           req.Prompt,
			Messages:         req.Messages,
			MaxTokens:        defaultInt(req.MaxTokens, 512),
			Temperature:      defaultFloat(req.Temperature, 0.7),
			TopP:             defaultFloat(req.TopP, 0.9),
			TopK:             defaultInt(req.TopK, 40),
			Stop:             req.Stop,
			FrequencyPenalty: req.FrequencyPenalty,
			PresencePenalty:  req.PresencePenalty,
			User:             req.User,
			Metadata:         reqBody.Metadata,
			CreatedAt:        time.Now(),
			Timeout:          120 * time.Second,
		})
	}

	batchResponse, err := h.executor.ExecuteBatch(r.Context(), reqBody.Model, requests)
	if err != nil {
		log.Printf("Batch inference error for model %s: %v", reqBody.Model, err)
		h.respondInferenceError(w, http.StatusInternalServerError, "batch inference failed: "+err.Error(), "inference_error")
		return
	}

	h.respondInferenceJSON(w, http.StatusOK, batchResponse)
}

// GetExecutorStats handles GET /api/v1/inference/stats
// Returns inference executor statistics including throughput, latency, and queue metrics
func (h *InferenceHandlers) GetExecutorStats(w http.ResponseWriter, r *http.Request) {
	stats := h.executor.GetStats()
	h.respondInferenceJSON(w, http.StatusOK, stats)
}

// GetActiveRequests handles GET /api/v1/inference/requests
// Returns a list of all currently active inference requests
func (h *InferenceHandlers) GetActiveRequests(w http.ResponseWriter, r *http.Request) {
	requests := h.executor.GetActiveRequests()
	h.respondInferenceJSON(w, http.StatusOK, ActiveRequestsResponse{
		Data:  requests,
		Count: len(requests),
	})
}

// StreamInference handles POST /api/v1/inference/{model_id}/stream
// Performs streaming inference using Server-Sent Events (SSE).
//
// The client receives events in this order:
//  1. "chunk" events containing partial content
//  2. "done" event signalling completion
//  3. "error" event if something goes wrong
//
// Example SSE output:
//
//	event: chunk
//	data: {"id":"...","content":"Hello","delta":"Hello","output_tokens":1}
//
//	event: done
//	data: {"request_id":"..."}
func (h *InferenceHandlers) StreamInference(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	modelID := vars["model_id"]
	if modelID == "" {
		h.respondInferenceError(w, http.StatusBadRequest, "model_id is required", "invalid_request")
		return
	}

	var reqBody InferenceRequestBody
	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
		h.respondInferenceError(w, http.StatusBadRequest, "invalid request body: "+err.Error(), "invalid_request")
		return
	}

	if reqBody.Prompt == "" && len(reqBody.Messages) == 0 {
		h.respondInferenceError(w, http.StatusBadRequest, "either 'prompt' or 'messages' must be provided", "invalid_request")
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Ensure we can flush
	flusher, ok := w.(http.Flusher)
	if !ok {
		h.respondInferenceError(w, http.StatusInternalServerError, "streaming not supported", "internal_error")
		return
	}

	// Send initial connection event
	fmt.Fprintf(w, "event: connected\ndata: {\"model_id\":\"%s\",\"request_id\":\"%s\"}\n\n", modelID, uuid.New().String())
	flusher.Flush()

	req := h.buildInferenceRequest(modelID, reqBody, inference.ModeStreaming, 120*time.Second)

	response, err := h.executor.Execute(r.Context(), req)
	if err != nil {
		errorData, _ := json.Marshal(map[string]string{
			"message": err.Error(),
			"type":    "inference_error",
		})
		fmt.Fprintf(w, "event: error\ndata: %s\n\n", errorData)
		flusher.Flush()
		return
	}

	// Send the response as a stream chunk
	chunk := inference.StreamChunk{
		ID:           response.ID,
		RequestID:    response.RequestID,
		ModelID:      response.ModelID,
		InstanceID:   response.InstanceID,
		Content:      response.Content,
		Delta:        response.Content,
		InputTokens:  response.InputTokens,
		OutputTokens: response.OutputTokens,
		FinishReason: response.FinishReason,
		Latency:      response.Latency,
		CreatedAt:    time.Now(),
	}

	chunkData, _ := json.Marshal(chunk)
	fmt.Fprintf(w, "event: chunk\ndata: %s\n\n", chunkData)
	flusher.Flush()

	// Send usage statistics
	usageData, _ := json.Marshal(map[string]interface{}{
		"request_id":     req.ID,
		"input_tokens":   response.InputTokens,
		"output_tokens":  response.OutputTokens,
		"total_tokens":   response.TotalTokens,
		"latency_ms":     response.Latency.Milliseconds(),
		"tokens_per_sec": response.TokensPerSecond,
	})
	fmt.Fprintf(w, "event: usage\ndata: %s\n\n", usageData)
	flusher.Flush()

	// Send done event
	fmt.Fprintf(w, "event: done\ndata: {\"request_id\":\"%s\",\"finish_reason\":\"%s\"}\n\n", req.ID, response.FinishReason)
	flusher.Flush()
}

// RegisterInferenceRoutes registers all inference routes on the given router
func (h *InferenceHandlers) RegisterInferenceRoutes(router *mux.Router) {
	// Inference execution
	router.HandleFunc("/api/v1/inference/{model_id}", h.ExecuteInference).Methods("POST")
	router.HandleFunc("/api/v1/inference/{model_id}/async", h.ExecuteInferenceAsync).Methods("POST")
	router.HandleFunc("/api/v1/inference/{model_id}/stream", h.StreamInference).Methods("POST")

	// Batch inference
	router.HandleFunc("/api/v1/inference/batch", h.ExecuteBatch).Methods("POST")

	// Request management
	router.HandleFunc("/api/v1/inference/requests", h.GetActiveRequests).Methods("GET")
	router.HandleFunc("/api/v1/inference/requests/{request_id}", h.GetInferenceStatus).Methods("GET")
	router.HandleFunc("/api/v1/inference/requests/{request_id}", h.CancelInference).Methods("DELETE")

	// Statistics
	router.HandleFunc("/api/v1/inference/stats", h.GetExecutorStats).Methods("GET")
}

// buildInferenceRequest creates an InferenceRequest from the API request body
func (h *InferenceHandlers) buildInferenceRequest(modelID string, body InferenceRequestBody, mode inference.InferenceMode, timeout time.Duration) *inference.InferenceRequest {
	return &inference.InferenceRequest{
		ID:               uuid.New().String(),
		ModelID:          modelID,
		Mode:             mode,
		Priority:         inference.PriorityNormal,
		Prompt:           body.Prompt,
		Messages:         body.Messages,
		Stream:           body.Stream,
		MaxTokens:        defaultInt(body.MaxTokens, 512),
		Temperature:      defaultFloat(body.Temperature, 0.7),
		TopP:             defaultFloat(body.TopP, 0.9),
		TopK:             defaultInt(body.TopK, 40),
		Stop:             body.Stop,
		FrequencyPenalty: body.FrequencyPenalty,
		PresencePenalty:  body.PresencePenalty,
		User:             body.User,
		Metadata:         body.Metadata,
		CreatedAt:        time.Now(),
		Timeout:          timeout,
	}
}

// respondInferenceJSON sends a JSON response
func (h *InferenceHandlers) respondInferenceJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(data)
}

// respondInferenceError sends an error response in OpenAI-compatible format
func (h *InferenceHandlers) respondInferenceError(w http.ResponseWriter, statusCode int, message string, errType string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]interface{}{
			"message": message,
			"type":    errType,
			"code":    fmt.Sprintf("%d", statusCode),
		},
	})
}

// defaultInt returns val if non-zero, otherwise def
func defaultInt(val, def int) int {
	if val == 0 {
		return def
	}
	return val
}

// defaultFloat returns val if non-zero, otherwise def
func defaultFloat(val, def float64) float64 {
	if val == 0 {
		return def
	}
	return val
}
