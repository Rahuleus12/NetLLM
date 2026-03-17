package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gorilla/mux"
	"ai-provider/internal/models"
)

// ModelHandlers handles model-related API requests
type ModelHandlers struct {
	manager *models.ModelManager
}

// NewModelHandlers creates a new model handlers instance
func NewModelHandlers(manager *models.ModelManager) *ModelHandlers {
	return &ModelHandlers{
		manager: manager,
	}
}

// ListModels handles GET /api/v1/models
func (h *ModelHandlers) ListModels(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	filter := &models.ModelFilter{
		Page:      1,
		PerPage:   20,
		SortBy:    "created_at",
		SortOrder: "DESC",
	}

	// Parse pagination
	if page := r.URL.Query().Get("page"); page != "" {
		if p, err := strconv.Atoi(page); err == nil && p > 0 {
			filter.Page = p
		}
	}

	if perPage := r.URL.Query().Get("per_page"); perPage != "" {
		if pp, err := strconv.Atoi(perPage); err == nil && pp > 0 && pp <= 100 {
			filter.PerPage = pp
		}
	}

	// Parse filters
	if status := r.URL.Query().Get("status"); status != "" {
		filter.Status = models.ModelStatus(status)
	}

	if format := r.URL.Query().Get("format"); format != "" {
		filter.Format = models.ModelFormat(format)
	}

	if search := r.URL.Query().Get("search"); search != "" {
		filter.Search = search
	}

	if sortBy := r.URL.Query().Get("sort_by"); sortBy != "" {
		filter.SortBy = sortBy
	}

	if sortOrder := r.URL.Query().Get("sort_order"); sortOrder != "" {
		filter.SortOrder = strings.ToUpper(sortOrder)
	}

	// Get models
	result, err := h.manager.ListModels(r.Context(), filter)
	if err != nil {
		h.respondError(w, http.StatusInternalServerError, "failed to list models", err)
		return
	}

	h.respondJSON(w, http.StatusOK, result)
}

// RegisterModel handles POST /api/v1/models
func (h *ModelHandlers) RegisterModel(w http.ResponseWriter, r *http.Request) {
	var req models.RegisterModelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, http.StatusBadRequest, "invalid request body", err)
		return
	}

	// Validate request
	if req.Name == "" {
		h.respondError(w, http.StatusBadRequest, "model name is required", nil)
		return
	}

	if req.Version == "" {
		h.respondError(w, http.StatusBadRequest, "model version is required", nil)
		return
	}

	if req.Format == "" {
		h.respondError(w, http.StatusBadRequest, "model format is required", nil)
		return
	}

	// Register model
	model, err := h.manager.RegisterModel(r.Context(), &req)
	if err != nil {
		h.respondError(w, http.StatusInternalServerError, "failed to register model", err)
		return
	}

	h.respondJSON(w, http.StatusCreated, model)
}

// GetModel handles GET /api/v1/models/{id}
func (h *ModelHandlers) GetModel(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	modelID := vars["id"]

	if modelID == "" {
		h.respondError(w, http.StatusBadRequest, "model ID is required", nil)
		return
	}

	model, err := h.manager.GetModel(r.Context(), modelID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			h.respondError(w, http.StatusNotFound, "model not found", err)
			return
		}
		h.respondError(w, http.StatusInternalServerError, "failed to get model", err)
		return
	}

	h.respondJSON(w, http.StatusOK, model)
}

// UpdateModel handles PUT /api/v1/models/{id}
func (h *ModelHandlers) UpdateModel(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	modelID := vars["id"]

	if modelID == "" {
		h.respondError(w, http.StatusBadRequest, "model ID is required", nil)
		return
	}

	var req models.UpdateModelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, http.StatusBadRequest, "invalid request body", err)
		return
	}

	model, err := h.manager.UpdateModel(r.Context(), modelID, &req)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			h.respondError(w, http.StatusNotFound, "model not found", err)
			return
		}
		h.respondError(w, http.StatusInternalServerError, "failed to update model", err)
		return
	}

	h.respondJSON(w, http.StatusOK, model)
}

// DeleteModel handles DELETE /api/v1/models/{id}
func (h *ModelHandlers) DeleteModel(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	modelID := vars["id"]

	if modelID == "" {
		h.respondError(w, http.StatusBadRequest, "model ID is required", nil)
		return
	}

	// Check for force parameter
	force := r.URL.Query().Get("force") == "true"

	if err := h.manager.DeleteModel(r.Context(), modelID, force); err != nil {
		if strings.Contains(err.Error(), "not found") {
			h.respondError(w, http.StatusNotFound, "model not found", err)
			return
		}
		h.respondError(w, http.StatusInternalServerError, "failed to delete model", err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// StartDownload handles POST /api/v1/models/{id}/download
func (h *ModelHandlers) StartDownload(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	modelID := vars["id"]

	if modelID == "" {
		h.respondError(w, http.StatusBadRequest, "model ID is required", nil)
		return
	}

	if err := h.manager.StartDownload(r.Context(), modelID); err != nil {
		h.respondError(w, http.StatusInternalServerError, "failed to start download", err)
		return
	}

	h.respondJSON(w, http.StatusAccepted, map[string]interface{}{
		"message":  "Download started",
		"model_id": modelID,
	})
}

// GetDownloadProgress handles GET /api/v1/models/{id}/download
func (h *ModelHandlers) GetDownloadProgress(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	modelID := vars["id"]

	if modelID == "" {
		h.respondError(w, http.StatusBadRequest, "model ID is required", nil)
		return
	}

	progress, err := h.manager.GetDownloadProgress(r.Context(), modelID)
	if err != nil {
		h.respondError(w, http.StatusNotFound, "download not found", err)
		return
	}

	h.respondJSON(w, http.StatusOK, progress)
}

// CancelDownload handles DELETE /api/v1/models/{id}/download
func (h *ModelHandlers) CancelDownload(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	modelID := vars["id"]

	if modelID == "" {
		h.respondError(w, http.StatusBadRequest, "model ID is required", nil)
		return
	}

	if err := h.manager.CancelDownload(r.Context(), modelID); err != nil {
		h.respondError(w, http.StatusInternalServerError, "failed to cancel download", err)
		return
	}

	h.respondJSON(w, http.StatusOK, map[string]interface{}{
		"message":  "Download cancelled",
		"model_id": modelID,
	})
}

// ResumeDownload handles POST /api/v1/models/{id}/download/resume
func (h *ModelHandlers) ResumeDownload(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	modelID := vars["id"]

	if modelID == "" {
		h.respondError(w, http.StatusBadRequest, "model ID is required", nil)
		return
	}

	// TODO: Implement resume download in download manager
	h.respondError(w, http.StatusNotImplemented, "resume download not yet implemented", nil)
}

// ValidateModel handles POST /api/v1/models/{id}/validate
func (h *ModelHandlers) ValidateModel(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	modelID := vars["id"]

	if modelID == "" {
		h.respondError(w, http.StatusBadRequest, "model ID is required", nil)
		return
	}

	if err := h.manager.ValidateModel(r.Context(), modelID); err != nil {
		h.respondError(w, http.StatusInternalServerError, "validation failed", err)
		return
	}

	h.respondJSON(w, http.StatusOK, map[string]interface{}{
		"message":  "Model validation completed",
		"model_id": modelID,
	})
}

// ActivateModel handles POST /api/v1/models/{id}/activate
func (h *ModelHandlers) ActivateModel(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	modelID := vars["id"]

	if modelID == "" {
		h.respondError(w, http.StatusBadRequest, "model ID is required", nil)
		return
	}

	if err := h.manager.ActivateModel(r.Context(), modelID); err != nil {
		h.respondError(w, http.StatusInternalServerError, "failed to activate model", err)
		return
	}

	h.respondJSON(w, http.StatusOK, map[string]interface{}{
		"message":  "Model activated successfully",
		"model_id": modelID,
	})
}

// DeactivateModel handles POST /api/v1/models/{id}/deactivate
func (h *ModelHandlers) DeactivateModel(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	modelID := vars["id"]

	if modelID == "" {
		h.respondError(w, http.StatusBadRequest, "model ID is required", nil)
		return
	}

	if err := h.manager.DeactivateModel(r.Context(), modelID); err != nil {
		h.respondError(w, http.StatusInternalServerError, "failed to deactivate model", err)
		return
	}

	h.respondJSON(w, http.StatusOK, map[string]interface{}{
		"message":  "Model deactivated successfully",
		"model_id": modelID,
	})
}

// GetModelStats handles GET /api/v1/models/stats
func (h *ModelHandlers) GetModelStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.manager.GetModelStats(r.Context())
	if err != nil {
		h.respondError(w, http.StatusInternalServerError, "failed to get model statistics", err)
		return
	}

	h.respondJSON(w, http.StatusOK, stats)
}

// SearchModels handles GET /api/v1/models/search
func (h *ModelHandlers) SearchModels(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		h.respondError(w, http.StatusBadRequest, "search query is required", nil)
		return
	}

	models, err := h.manager.SearchModels(r.Context(), query)
	if err != nil {
		h.respondError(w, http.StatusInternalServerError, "search failed", err)
		return
	}

	h.respondJSON(w, http.StatusOK, map[string]interface{}{
		"data":  models,
		"query": query,
		"count": len(models),
	})
}

// GetModelVersions handles GET /api/v1/models/{id}/versions
func (h *ModelHandlers) GetModelVersions(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	modelID := vars["id"]

	if modelID == "" {
		h.respondError(w, http.StatusBadRequest, "model ID is required", nil)
		return
	}

	// TODO: Implement version listing
	h.respondError(w, http.StatusNotImplemented, "version listing not yet implemented", nil)
}

// Helper functions

// respondJSON sends a JSON response
func (h *ModelHandlers) respondJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(data)
}

// respondError sends an error response
func (h *ModelHandlers) respondError(w http.ResponseWriter, statusCode int, message string, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	errorResponse := map[string]interface{}{
		"error": map[string]interface{}{
			"message": message,
			"code":    statusCode,
		},
	}

	if err != nil {
		errorResponse["error"].(map[string]interface{})["details"] = err.Error()
	}

	json.NewEncoder(w).Encode(errorResponse)
}

// RegisterRoutes registers model routes on the router
func (h *ModelHandlers) RegisterRoutes(router *mux.Router) {
	// Model management routes
	router.HandleFunc("/api/v1/models", h.ListModels).Methods("GET")
	router.HandleFunc("/api/v1/models", h.RegisterModel).Methods("POST")
	router.HandleFunc("/api/v1/models/search", h.SearchModels).Methods("GET")
	router.HandleFunc("/api/v1/models/stats", h.GetModelStats).Methods("GET")
	router.HandleFunc("/api/v1/models/{id}", h.GetModel).Methods("GET")
	router.HandleFunc("/api/v1/models/{id}", h.UpdateModel).Methods("PUT")
	router.HandleFunc("/api/v1/models/{id}", h.DeleteModel).Methods("DELETE")

	// Download routes
	router.HandleFunc("/api/v1/models/{id}/download", h.StartDownload).Methods("POST")
	router.HandleFunc("/api/v1/models/{id}/download", h.GetDownloadProgress).Methods("GET")
	router.HandleFunc("/api/v1/models/{id}/download", h.CancelDownload).Methods("DELETE")
	router.HandleFunc("/api/v1/models/{id}/download/resume", h.ResumeDownload).Methods("POST")

	// Validation and activation routes
	router.HandleFunc("/api/v1/models/{id}/validate", h.ValidateModel).Methods("POST")
	router.HandleFunc("/api/v1/models/{id}/activate", h.ActivateModel).Methods("POST")
	router.HandleFunc("/api/v1/models/{id}/deactivate", h.DeactivateModel).Methods("POST")

	// Version routes
	router.HandleFunc("/api/v1/models/{id}/versions", h.GetModelVersions).Methods("GET")
}
