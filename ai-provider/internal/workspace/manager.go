// internal/workspace/manager.go
// Workspace management and organization
// Handles workspace creation, management, and lifecycle

package workspace

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

var (
	ErrWorkspaceNotFound       = errors.New("workspace not found")
	ErrWorkspaceAlreadyExists  = errors.New("workspace already exists")
	ErrInvalidWorkspace        = errors.New("invalid workspace data")
	ErrWorkspaceCannotDelete   = errors.New("cannot delete workspace with active resources")
	ErrInvalidOrganizationID   = errors.New("invalid organization ID")
	ErrMaxWorkspacesExceeded   = errors.New("maximum workspaces limit exceeded")
)

// WorkspaceStatus represents the status of a workspace
type WorkspaceStatus string

const (
	WorkspaceStatusActive    WorkspaceStatus = "active"
	WorkspaceStatusArchived WorkspaceStatus = "archived"
	WorkspaceStatusSuspended WorkspaceStatus = "suspended"
	WorkspaceStatusDeleted  WorkspaceStatus = "deleted"
)

// WorkspaceType represents the type of workspace
type WorkspaceType string

const (
	WorkspaceTypePersonal  WorkspaceType = "personal"
	WorkspaceTypeTeam      WorkspaceType = "team"
	WorkspaceTypeProject   WorkspaceType = "project"
	WorkspaceTypeShared    WorkspaceType = "shared"
)

// Workspace represents a tenant workspace
type Workspace struct {
	ID             uuid.UUID          `json:"id" db:"id"`
	TenantID       uuid.UUID          `json:"tenant_id" db:"tenant_id"`
	OrganizationID uuid.UUID          `json:"organization_id" db:"organization_id"`
	TeamID         *uuid.UUID         `json:"team_id,omitempty" db:"team_id"`
	OwnerID        uuid.UUID          `json:"owner_id" db:"owner_id"`

	// Workspace details
	Name           string             `json:"name" db:"name"`
	Slug           string             `json:"slug" db:"slug"`
	Description    string             `json:"description" db:"description"`
	Type           WorkspaceType       `json:"type" db:"type"`
	Status         WorkspaceStatus    `json:"status" db:"status"`

	// Workspace configuration
	Settings       WorkspaceSettings   `json:"settings" db:"settings"`
	Visibility     string             `json:"visibility" db:"visibility"` // private, organization, public

	// Resource tracking
	ModelCount     int                `json:"model_count" db:"model_count"`
	DatasetCount   int                `json:"dataset_count" db:"dataset_count"`
	PipelineCount  int                `json:"pipeline_count" db:"pipeline_count"`

	// Timestamps
	CreatedAt      time.Time          `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time          `json:"updated_at" db:"updated_at"`
	LastActivityAt time.Time          `json:"last_activity_at" db:"last_activity_at"`
	DeletedAt      *time.Time         `json:"deleted_at,omitempty" db:"deleted_at"`

	// Metadata
	Tags           []string           `json:"tags" db:"tags"`
	Metadata       map[string]interface{} `json:"metadata,omitempty" db:"metadata"`
}

// WorkspaceSettings represents workspace-level configuration
type WorkspaceSettings struct {
	// Resource defaults
	DefaultModelID     string                 `json:"default_model_id"`
	DefaultComputeType string                 `json:"default_compute_type"`

	// Feature flags
	Features          map[string]bool          `json:"features"`

	// Preferences
	AutoSaveEnabled   bool                    `json:"auto_save_enabled"`
	AutoSaveInterval  int                     `json:"auto_save_interval"` // minutes
	VersionControl     bool                    `json:"version_control"`

	// Limits
	MaxModels         int                     `json:"max_models"`
	MaxDatasets       int                     `json:"max_datasets"`
	MaxPipelines      int                     `json:"max_pipelines"`

	// Notifications
	NotificationsEnabled bool                  `json:"notifications_enabled"`
	NotificationChannels []string              `json:"notification_channels"`

	// Custom settings
	CustomSettings    map[string]interface{}   `json:"custom_settings"`
}

// CreateWorkspaceRequest represents a request to create a new workspace
type CreateWorkspaceRequest struct {
	TenantID       uuid.UUID    `json:"tenant_id"`
	OrganizationID uuid.UUID    `json:"organization_id"`
	TeamID         *uuid.UUID   `json:"team_id,omitempty"`
	OwnerID        uuid.UUID    `json:"owner_id"`
	Name           string       `json:"name"`
	Slug           string       `json:"slug"`
	Description    string       `json:"description,omitempty"`
	Type           WorkspaceType `json:"type"`
	Visibility     string       `json:"visibility"`
	Settings       WorkspaceSettings `json:"settings,omitempty"`
	Tags           []string     `json:"tags,omitempty"`
}

// UpdateWorkspaceRequest represents a request to update a workspace
type UpdateWorkspaceRequest struct {
	Name           *string            `json:"name,omitempty"`
	Slug           *string            `json:"slug,omitempty"`
	Description    *string            `json:"description,omitempty"`
	Type           *WorkspaceType     `json:"type,omitempty"`
	Status         *WorkspaceStatus   `json:"status,omitempty"`
	Visibility     *string            `json:"visibility,omitempty"`
	Settings       *WorkspaceSettings `json:"settings,omitempty"`
	Tags           *[]string          `json:"tags,omitempty"`
}

// ListWorkspacesOptions represents options for listing workspaces
type ListWorkspacesOptions struct {
	TenantID       *uuid.UUID
	OrganizationID *uuid.UUID
	TeamID         *uuid.UUID
	OwnerID        *uuid.UUID
	Type           *WorkspaceType
	Status         *WorkspaceStatus
	Visibility     *string
	Limit          int
	Offset         int
	Search         string
	SortBy         string
	SortOrder      string
}

// Manager manages workspaces
type Manager struct {
	db *sql.DB
}

// NewManager creates a new workspace manager
func NewManager(db *sql.DB) *Manager {
	return &Manager{
		db: db,
	}
}

// CreateWorkspace creates a new workspace
func (m *Manager) CreateWorkspace(ctx context.Context, req CreateWorkspaceRequest) (*Workspace, error) {
	if req.TenantID == uuid.Nil {
		return nil, ErrInvalidTenantID
	}
	if req.OrganizationID == uuid.Nil {
		return nil, ErrInvalidOrganizationID
	}
	if req.OwnerID == uuid.Nil {
		return nil, errors.New("owner_id is required")
	}
	if req.Name == "" {
		return nil, ErrInvalidWorkspace
	}
	if req.Slug == "" {
		return nil, ErrInvalidWorkspace
	}

	// Check if workspace with slug already exists for this organization
	existing, err := m.GetWorkspaceBySlug(ctx, req.OrganizationID, req.Slug)
	if err == nil && existing != nil {
		return nil, ErrWorkspaceAlreadyExists
	}

	// Check if tenant has exceeded workspace limit
	workspaceCount, err := m.GetWorkspaceCount(ctx, req.OrganizationID)
	if err == nil && workspaceCount >= 50 { // Default limit
		return nil, ErrMaxWorkspacesExceeded
	}

	// Apply default settings if not provided
	settings := req.Settings
	if settings.Features == nil {
		settings.Features = make(map[string]bool)
	}
	if settings.CustomSettings == nil {
		settings.CustomSettings = make(map[string]interface{})
	}
	if settings.DefaultModelID == "" {
		settings.DefaultModelID = "default"
	}
	if settings.DefaultComputeType == "" {
		settings.DefaultComputeType = "cpu"
	}
	if settings.MaxModels == 0 {
		settings.MaxModels = 100
	}
	if settings.MaxDatasets == 0 {
		settings.MaxDatasets = 1000
	}
	if settings.MaxPipelines == 0 {
		settings.MaxPipelines = 50
	}

	workspace := &Workspace{
		ID:             uuid.New(),
		TenantID:       req.TenantID,
		OrganizationID: req.OrganizationID,
		TeamID:         req.TeamID,
		OwnerID:        req.OwnerID,
		Name:           req.Name,
		Slug:           req.Slug,
		Description:    req.Description,
		Type:           req.Type,
		Status:         WorkspaceStatusActive,
		Settings:       settings,
		Visibility:     req.Visibility,
		ModelCount:     0,
		DatasetCount:   0,
		PipelineCount:  0,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		LastActivityAt: time.Now(),
		Tags:           req.Tags,
		Metadata:       make(map[string]interface{}),
	}

	settingsJSON, err := json.Marshal(workspace.Settings)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal settings: %w", err)
	}

	tagsJSON, err := json.Marshal(workspace.Tags)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal tags: %w", err)
	}

	metadataJSON, err := json.Marshal(workspace.Metadata)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal metadata: %w", err)
	}

	query := `
		INSERT INTO workspaces (id, tenant_id, organization_id, team_id, owner_id,
			name, slug, description, type, status, settings, visibility,
			model_count, dataset_count, pipeline_count,
			created_at, updated_at, last_activity_at, tags, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12,
			$13, $14, $15, $16, $17, $18, $19, $20)
		RETURNING id, tenant_id, organization_id, team_id, owner_id,
			name, slug, description, type, status, settings, visibility,
			model_count, dataset_count, pipeline_count,
			created_at, updated_at, last_activity_at, tags, metadata, deleted_at
	`

	err = m.db.QueryRowContext(ctx, query,
		workspace.ID,
		workspace.TenantID,
		workspace.OrganizationID,
		workspace.TeamID,
		workspace.OwnerID,
		workspace.Name,
		workspace.Slug,
		workspace.Description,
		workspace.Type,
		workspace.Status,
		settingsJSON,
		workspace.Visibility,
		workspace.ModelCount,
		workspace.DatasetCount,
		workspace.PipelineCount,
		workspace.CreatedAt,
		workspace.UpdatedAt,
		workspace.LastActivityAt,
		tagsJSON,
		metadataJSON,
	).Scan(
		&workspace.ID,
		&workspace.TenantID,
		&workspace.OrganizationID,
		&workspace.TeamID,
		&workspace.OwnerID,
		&workspace.Name,
		&workspace.Slug,
		&workspace.Description,
		&workspace.Type,
		&workspace.Status,
		&settingsJSON,
		&workspace.Visibility,
		&workspace.ModelCount,
		&workspace.DatasetCount,
		&workspace.PipelineCount,
		&workspace.CreatedAt,
		&workspace.UpdatedAt,
		&workspace.LastActivityAt,
		&tagsJSON,
		&metadataJSON,
		&workspace.DeletedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create workspace: %w", err)
	}

	// Unmarshal JSON fields
	json.Unmarshal(settingsJSON, &workspace.Settings)
	json.Unmarshal(tagsJSON, &workspace.Tags)
	json.Unmarshal(metadataJSON, &workspace.Metadata)

	return workspace, nil
}

// GetWorkspace retrieves a workspace by ID
func (m *Manager) GetWorkspace(ctx context.Context, id uuid.UUID) (*Workspace, error) {
	if id == uuid.Nil {
		return nil, ErrInvalidWorkspace
	}

	var workspace Workspace
	var settingsJSON, tagsJSON, metadataJSON []byte

	query := `
		SELECT id, tenant_id, organization_id, team_id, owner_id,
			name, slug, description, type, status, settings, visibility,
			model_count, dataset_count, pipeline_count,
			created_at, updated_at, last_activity_at, tags, metadata, deleted_at
		FROM workspaces
		WHERE id = $1 AND deleted_at IS NULL
	`

	err := m.db.QueryRowContext(ctx, query, id).Scan(
		&workspace.ID,
		&workspace.TenantID,
		&workspace.OrganizationID,
		&workspace.TeamID,
		&workspace.OwnerID,
		&workspace.Name,
		&workspace.Slug,
		&workspace.Description,
		&workspace.Type,
		&workspace.Status,
		&settingsJSON,
		&workspace.Visibility,
		&workspace.ModelCount,
		&workspace.DatasetCount,
		&workspace.PipelineCount,
		&workspace.CreatedAt,
		&workspace.UpdatedAt,
		&workspace.LastActivityAt,
		&tagsJSON,
		&metadataJSON,
		&workspace.DeletedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrWorkspaceNotFound
		}
		return nil, fmt.Errorf("failed to get workspace: %w", err)
	}

	// Unmarshal JSON fields
	if settingsJSON != nil {
		json.Unmarshal(settingsJSON, &workspace.Settings)
	}
	if tagsJSON != nil {
		json.Unmarshal(tagsJSON, &workspace.Tags)
	}
	if metadataJSON != nil {
		json.Unmarshal(metadataJSON, &workspace.Metadata)
	}

	return &workspace, nil
}

// GetWorkspaceBySlug retrieves a workspace by organization ID and slug
func (m *Manager) GetWorkspaceBySlug(ctx context.Context, orgID uuid.UUID, slug string) (*Workspace, error) {
	if orgID == uuid.Nil {
		return nil, ErrInvalidOrganizationID
	}
	if slug == "" {
		return nil, ErrInvalidWorkspace
	}

	var workspace Workspace
	var settingsJSON, tagsJSON, metadataJSON []byte

	query := `
		SELECT id, tenant_id, organization_id, team_id, owner_id,
			name, slug, description, type, status, settings, visibility,
			model_count, dataset_count, pipeline_count,
			created_at, updated_at, last_activity_at, tags, metadata, deleted_at
		FROM workspaces
		WHERE organization_id = $1 AND slug = $2 AND deleted_at IS NULL
	`

	err := m.db.QueryRowContext(ctx, query, orgID, slug).Scan(
		&workspace.ID,
		&workspace.TenantID,
		&workspace.OrganizationID,
		&workspace.TeamID,
		&workspace.OwnerID,
		&workspace.Name,
		&workspace.Slug,
		&workspace.Description,
		&workspace.Type,
		&workspace.Status,
		&settingsJSON,
		&workspace.Visibility,
		&workspace.ModelCount,
		&workspace.DatasetCount,
		&workspace.PipelineCount,
		&workspace.CreatedAt,
		&workspace.UpdatedAt,
		&workspace.LastActivityAt,
		&tagsJSON,
		&metadataJSON,
		&workspace.DeletedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrWorkspaceNotFound
		}
		return nil, fmt.Errorf("failed to get workspace by slug: %w", err)
	}

	if settingsJSON != nil {
		json.Unmarshal(settingsJSON, &workspace.Settings)
	}
	if tagsJSON != nil {
		json.Unmarshal(tagsJSON, &workspace.Tags)
	}
	if metadataJSON != nil {
		json.Unmarshal(metadataJSON, &workspace.Metadata)
	}

	return &workspace, nil
}

// UpdateWorkspace updates a workspace
func (m *Manager) UpdateWorkspace(ctx context.Context, id uuid.UUID, req UpdateWorkspaceRequest) (*Workspace, error) {
	if id == uuid.Nil {
		return nil, ErrInvalidWorkspace
	}

	workspace, err := m.GetWorkspace(ctx, id)
	if err != nil {
		return nil, err
	}

	// Apply updates
	if req.Name != nil {
		workspace.Name = *req.Name
	}
	if req.Slug != nil {
		workspace.Slug = *req.Slug
	}
	if req.Description != nil {
		workspace.Description = *req.Description
	}
	if req.Type != nil {
		workspace.Type = *req.Type
	}
	if req.Status != nil {
		if !isValidWorkspaceStatus(*req.Status) {
			return nil, errors.New("invalid workspace status")
		}
		workspace.Status = *req.Status
	}
	if req.Visibility != nil {
		workspace.Visibility = *req.Visibility
	}
	if req.Settings != nil {
		workspace.Settings = *req.Settings
	}
	if req.Tags != nil {
		workspace.Tags = *req.Tags
	}
	workspace.UpdatedAt = time.Now()
	workspace.LastActivityAt = time.Now()

	settingsJSON, err := json.Marshal(workspace.Settings)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal settings: %w", err)
	}

	tagsJSON, err := json.Marshal(workspace.Tags)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal tags: %w", err)
	}

	metadataJSON, err := json.Marshal(workspace.Metadata)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal metadata: %w", err)
	}

	query := `
		UPDATE workspaces
		SET name = $1, slug = $2, description = $3, type = $4, status = $5,
			settings = $6, visibility = $7, tags = $8, metadata = $9,
			updated_at = $10, last_activity_at = $11
		WHERE id = $12
		RETURNING id, tenant_id, organization_id, team_id, owner_id,
			name, slug, description, type, status, settings, visibility,
			model_count, dataset_count, pipeline_count,
			created_at, updated_at, last_activity_at, tags, metadata, deleted_at
	`

	err = m.db.QueryRowContext(ctx, query,
		workspace.Name,
		workspace.Slug,
		workspace.Description,
		workspace.Type,
		workspace.Status,
		settingsJSON,
		workspace.Visibility,
		tagsJSON,
		metadataJSON,
		workspace.UpdatedAt,
		workspace.LastActivityAt,
		workspace.ID,
	).Scan(
		&workspace.ID,
		&workspace.TenantID,
		&workspace.OrganizationID,
		&workspace.TeamID,
		&workspace.OwnerID,
		&workspace.Name,
		&workspace.Slug,
		&workspace.Description,
		&workspace.Type,
		&workspace.Status,
		&settingsJSON,
		&workspace.Visibility,
		&workspace.ModelCount,
		&workspace.DatasetCount,
		&workspace.PipelineCount,
		&workspace.CreatedAt,
		&workspace.UpdatedAt,
		&workspace.LastActivityAt,
		&tagsJSON,
		&metadataJSON,
		&workspace.DeletedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to update workspace: %w", err)
	}

	json.Unmarshal(settingsJSON, &workspace.Settings)
	json.Unmarshal(tagsJSON, &workspace.Tags)
	json.Unmarshal(metadataJSON, &workspace.Metadata)

	return workspace, nil
}

// DeleteWorkspace deletes a workspace
func (m *Manager) DeleteWorkspace(ctx context.Context, id uuid.UUID) error {
	if id == uuid.Nil {
		return ErrInvalidWorkspace
	}

	// Check if workspace has active resources
	hasResources, err := m.workspaceHasActiveResources(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to check workspace resources: %w", err)
	}
	if hasResources {
		return ErrWorkspaceCannotDelete
	}

	// Soft delete
	query := `
		UPDATE workspaces
		SET deleted_at = CURRENT_TIMESTAMP, status = 'deleted', updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
	`

	result, err := m.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete workspace: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return ErrWorkspaceNotFound
	}

	return nil
}

// ListWorkspaces lists workspaces with optional filters
func (m *Manager) ListWorkspaces(ctx context.Context, opts ListWorkspacesOptions) ([]*Workspace, int64, error) {
	workspaces := []*Workspace{}
	var total int64

	// Build query with dynamic filters
	baseQuery := `
		SELECT id, tenant_id, organization_id, team_id, owner_id,
			name, slug, description, type, status, settings, visibility,
			model_count, dataset_count, pipeline_count,
			created_at, updated_at, last_activity_at, tags, metadata, deleted_at
		FROM workspaces
		WHERE deleted_at IS NULL
	`
	countQuery := `
		SELECT COUNT(*)
		FROM workspaces
		WHERE deleted_at IS NULL
	`

	args := []interface{}{}
	argPos := 1

	if opts.TenantID != nil {
		baseQuery += fmt.Sprintf(" AND tenant_id = $%d", argPos)
		countQuery += fmt.Sprintf(" AND tenant_id = $%d", argPos)
		args = append(args, *opts.TenantID)
		argPos++
	}

	if opts.OrganizationID != nil {
		baseQuery += fmt.Sprintf(" AND organization_id = $%d", argPos)
		countQuery += fmt.Sprintf(" AND organization_id = $%d", argPos)
		args = append(args, *opts.OrganizationID)
		argPos++
	}

	if opts.TeamID != nil {
		baseQuery += fmt.Sprintf(" AND team_id = $%d", argPos)
		countQuery += fmt.Sprintf(" AND team_id = $%d", argPos)
		args = append(args, *opts.TeamID)
		argPos++
	}

	if opts.OwnerID != nil {
		baseQuery += fmt.Sprintf(" AND owner_id = $%d", argPos)
		countQuery += fmt.Sprintf(" AND owner_id = $%d", argPos)
		args = append(args, *opts.OwnerID)
		argPos++
	}

	if opts.Type != nil {
		baseQuery += fmt.Sprintf(" AND type = $%d", argPos)
		countQuery += fmt.Sprintf(" AND type = $%d", argPos)
		args = append(args, *opts.Type)
		argPos++
	}

	if opts.Status != nil {
		baseQuery += fmt.Sprintf(" AND status = $%d", argPos)
		countQuery += fmt.Sprintf(" AND status = $%d", argPos)
		args = append(args, *opts.Status)
		argPos++
	}

	if opts.Visibility != nil {
		baseQuery += fmt.Sprintf(" AND visibility = $%d", argPos)
		countQuery += fmt.Sprintf(" AND visibility = $%d", argPos)
		args = append(args, *opts.Visibility)
		argPos++
	}

	if opts.Search != "" {
		baseQuery += fmt.Sprintf(" AND (name ILIKE $%d OR slug ILIKE $%d OR description ILIKE $%d)", argPos, argPos+1, argPos+2)
		countQuery += fmt.Sprintf(" AND (name ILIKE $%d OR slug ILIKE $%d OR description ILIKE $%d)", argPos, argPos+1, argPos+2)
		searchPattern := "%" + opts.Search + "%"
		args = append(args, searchPattern, searchPattern, searchPattern)
		argPos += 3
	}

	// Get total count
	err := m.db.QueryRowContext(ctx, countQuery, args...[:argPos-1]...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count workspaces: %w", err)
	}

	// Add sorting
	sortColumn := "created_at"
	sortOrder := "DESC"
	if opts.SortBy != "" {
		sortColumn = opts.SortBy
	}
	if opts.SortOrder != "" {
		sortOrder = opts.SortOrder
	}
	baseQuery += fmt.Sprintf(" ORDER BY %s %s", sortColumn, sortOrder)

	// Add pagination
	if opts.Limit > 0 {
		baseQuery += fmt.Sprintf(" LIMIT $%d", argPos)
		args = append(args, opts.Limit)
		argPos++
	}
	if opts.Offset > 0 {
		baseQuery += fmt.Sprintf(" OFFSET $%d", argPos)
		args = append(args, opts.Offset)
		argPos++
	}

	rows, err := m.db.QueryContext(ctx, baseQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list workspaces: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var workspace Workspace
		var settingsJSON, tagsJSON, metadataJSON []byte

		err := rows.Scan(
			&workspace.ID,
			&workspace.TenantID,
			&workspace.OrganizationID,
			&workspace.TeamID,
			&workspace.OwnerID,
			&workspace.Name,
			&workspace.Slug,
			&workspace.Description,
			&workspace.Type,
			&workspace.Status,
			&settingsJSON,
			&workspace.Visibility,
			&workspace.ModelCount,
			&workspace.DatasetCount,
			&workspace.PipelineCount,
			&workspace.CreatedAt,
			&workspace.UpdatedAt,
			&workspace.LastActivityAt,
			&tagsJSON,
			&metadataJSON,
			&workspace.DeletedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan workspace: %w", err)
		}

		if settingsJSON != nil {
			json.Unmarshal(settingsJSON, &workspace.Settings)
		}
		if tagsJSON != nil {
			json.Unmarshal(tagsJSON, &workspace.Tags)
		}
		if metadataJSON != nil {
			json.Unmarshal(metadataJSON, &workspace.Metadata)
		}

		workspaces = append(workspaces, &workspace)
	}

	if err = rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating workspaces: %w", err)
	}

	return workspaces, total, nil
}

// GetWorkspacesByOrganization retrieves all workspaces for an organization
func (m *Manager) GetWorkspacesByOrganization(ctx context.Context, orgID uuid.UUID) ([]*Workspace, error) {
	if orgID == uuid.Nil {
		return nil, ErrInvalidOrganizationID
	}

	var workspaces []*Workspace

	query := `
		SELECT id, tenant_id, organization_id, team_id, owner_id,
			name, slug, description, type, status, settings, visibility,
			model_count, dataset_count, pipeline_count,
			created_at, updated_at, last_activity_at, tags, metadata, deleted_at
		FROM workspaces
		WHERE organization_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
	`

	rows, err := m.db.QueryContext(ctx, query, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspaces by organization: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var workspace Workspace
		var settingsJSON, tagsJSON, metadataJSON []byte

		err := rows.Scan(
			&workspace.ID,
			&workspace.TenantID,
			&workspace.OrganizationID,
			&workspace.TeamID,
			&workspace.OwnerID,
			&workspace.Name,
			&workspace.Slug,
			&workspace.Description,
			&workspace.Type,
			&workspace.Status,
			&settingsJSON,
			&workspace.Visibility,
			&workspace.ModelCount,
			&workspace.DatasetCount,
			&workspace.PipelineCount,
			&workspace.CreatedAt,
			&workspace.UpdatedAt,
			&workspace.LastActivityAt,
			&tagsJSON,
			&metadataJSON,
			&workspace.DeletedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan workspace: %w", err)
		}

		if settingsJSON != nil {
			json.Unmarshal(settingsJSON, &workspace.Settings)
		}
		if tagsJSON != nil {
			json.Unmarshal(tagsJSON, &workspace.Tags)
		}
		if metadataJSON != nil {
			json.Unmarshal(metadataJSON, &workspace.Metadata)
		}

		workspaces = append(workspaces, &workspace)
	}

	return workspaces, nil
}

// UpdateActivity updates the last activity timestamp for a workspace
func (m *Manager) UpdateActivity(ctx context.Context, id uuid.UUID) error {
	query := `
		UPDATE workspaces
		SET last_activity_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
	`

	_, err := m.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to update workspace activity: %w", err)
	}

	return nil
}

// GetWorkspaceCount returns the count of active workspaces for an organization
func (m *Manager) GetWorkspaceCount(ctx context.Context, orgID uuid.UUID) (int, error) {
	var count int

	query := `
		SELECT COUNT(*)
		FROM workspaces
		WHERE organization_id = $1 AND status = 'active' AND deleted_at IS NULL
	`

	err := m.db.QueryRowContext(ctx, query, orgID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to get workspace count: %w", err)
	}

	return count, nil
}

func (m *Manager) workspaceHasActiveResources(ctx context.Context, id uuid.UUID) (bool, error) {
	// Check for active models
	var modelCount int
	err := m.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM workspace_models WHERE workspace_id = $1 AND deleted_at IS NULL",
		id,
	).Scan(&modelCount)
	if err != nil {
		return false, err
	}

	if modelCount > 0 {
		return true, nil
	}

	// Check for active datasets
	var datasetCount int
	err = m.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM workspace_datasets WHERE workspace_id = $1 AND deleted_at IS NULL",
		id,
	).Scan(&datasetCount)
	if err != nil {
		return false, err
	}

	if datasetCount > 0 {
		return true, nil
	}

	// Check for active pipelines
	var pipelineCount int
	err = m.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM workspace_pipelines WHERE workspace_id = $1 AND deleted_at IS NULL",
		id,
	).Scan(&pipelineCount)
	if err != nil {
		return false, err
	}

	return pipelineCount > 0, nil
}

// isValidWorkspaceStatus checks if a workspace status is valid
func isValidWorkspaceStatus(status WorkspaceStatus) bool {
	switch status {
	case WorkspaceStatusActive, WorkspaceStatusArchived, WorkspaceStatusSuspended, WorkspaceStatusDeleted:
		return true
	default:
		return false
	}
}

// CloneWorkspace creates a copy of an existing workspace
func (m *Manager) CloneWorkspace(ctx context.Context, sourceID uuid.UUID, newName, newSlug string, newOwnerID uuid.UUID) (*Workspace, error) {
	source, err := m.GetWorkspace(ctx, sourceID)
	if err != nil {
		return nil, err
	}

	cloneReq := CreateWorkspaceRequest{
		TenantID:       source.TenantID,
		OrganizationID: source.OrganizationID,
		TeamID:         source.TeamID,
		OwnerID:        newOwnerID,
		Name:           newName,
		Slug:           newSlug,
		Description:    fmt.Sprintf("Clone of %s", source.Name),
		Type:           source.Type,
		Visibility:     source.Visibility,
		Settings:       source.Settings,
		Tags:           source.Tags,
	}

	return m.CreateWorkspace(ctx, cloneReq)
}

// ArchiveWorkspace archives a workspace
func (m *Manager) ArchiveWorkspace(ctx context.Context, id uuid.UUID) (*Workspace, error) {
	return m.updateWorkspaceStatus(ctx, id, WorkspaceStatusArchived)
}

// RestoreWorkspace restores an archived workspace
func (m *Manager) RestoreWorkspace(ctx context.Context, id uuid.UUID) (*Workspace, error) {
	return m.updateWorkspaceStatus(ctx, id, WorkspaceStatusActive)
}

func (m *Manager) updateWorkspaceStatus(ctx context.Context, id uuid.UUID, status WorkspaceStatus) (*Workspace, error) {
	if id == uuid.Nil {
		return nil, ErrInvalidWorkspace
	}

	if !isValidWorkspaceStatus(status) {
		return nil, errors.New("invalid workspace status")
	}

	workspace, err := m.GetWorkspace(ctx, id)
	if err != nil {
		return nil, err
	}

	if workspace.Status == status {
		return workspace, nil
	}

	query := `
		UPDATE workspaces
		SET status = $1, updated_at = CURRENT_TIMESTAMP
		WHERE id = $2
		RETURNING id, tenant_id, organization_id, team_id, owner_id,
			name, slug, description, type, status, settings, visibility,
			model_count, dataset_count, pipeline_count,
			created_at, updated_at, last_activity_at, tags, metadata, deleted_at
	`

	var settingsJSON, tagsJSON, metadataJSON []byte
	err = m.db.QueryRowContext(ctx, query, status, id).Scan(
		&workspace.ID,
		&workspace.TenantID,
		&workspace.OrganizationID,
		&workspace.TeamID,
		&workspace.OwnerID,
		&workspace.Name,
		&workspace.Slug,
		&workspace.Description,
		&workspace.Type,
		&workspace.Status,
		&settingsJSON,
		&workspace.Visibility,
		&workspace.ModelCount,
		&workspace.DatasetCount,
		&workspace.PipelineCount,
		&workspace.CreatedAt,
		&workspace.UpdatedAt,
		&workspace.LastActivityAt,
		&tagsJSON,
		&metadataJSON,
		&workspace.DeletedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to update workspace status: %w", err)
	}

	if settingsJSON != nil {
		json.Unmarshal(settingsJSON, &workspace.Settings)
	}
	if tagsJSON != nil {
		json.Unmarshal(tagsJSON, &workspace.Tags)
	}
	if metadataJSON != nil {
		json.Unmarshal(metadataJSON, &workspace.Metadata)
	}

	return workspace, nil
}
