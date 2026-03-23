// internal/workspace/resources.go
// Resource organization within workspaces
// Handles workspace resources, their organization, and management

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
	ErrResourceNotFound       = errors.New("resource not found")
	ErrResourceAlreadyExists = errors.New("resource already exists")
	ErrInvalidResource       = errors.New("invalid resource data")
	ErrResourceNotInWorkspace = errors.New("resource not in workspace")
	ErrCannotRemoveResource   = errors.New("cannot remove resource with dependencies")
	ErrInvalidResourceType    = errors.New("invalid resource type")
)

// ResourceType represents the type of workspace resource
type ResourceType string

const (
	ResourceTypeModel     ResourceType = "model"
	ResourceTypeDataset   ResourceType = "dataset"
	ResourceTypeNotebook  ResourceType = "notebook"
	ResourceTypeAPI       ResourceType = "api"
	ResourceTypePipeline  ResourceType = "pipeline"
	ResourceTypeWorkflow  ResourceType = "workflow"
	ResourceTypeDocument  ResourceType = "document"
	ResourceTypeImage     ResourceType = "image"
	ResourceTypeSecret    ResourceType = "secret"
)

// ResourceStatus represents the status of a resource
type ResourceStatus string

const (
	ResourceStatusActive     ResourceStatus = "active"
	ResourceStatusArchived  ResourceStatus = "archived"
	ResourceStatusDraft     ResourceStatus = "draft"
	ResourceStatusDeleted   ResourceStatus = "deleted"
	ResourceStatusError     ResourceStatus = "error"
	ResourceStatusProcessing ResourceStatus = "processing"
)

// Resource represents a workspace resource
type Resource struct {
	ID           uuid.UUID        `json:"id" db:"id"`
	WorkspaceID  uuid.UUID        `json:"workspace_id" db:"workspace_id"`
	Type         ResourceType     `json:"type" db:"type"`
	Name         string           `json:"name" db:"name"`
	Slug         string           `json:"slug" db:"slug"`
	Description  string           `json:"description" db:"description"`
	Status       ResourceStatus   `json:"status" db:"status"`

	// Resource configuration
	Config       json.RawMessage  `json:"config,omitempty" db:"config"`
	Metadata     json.RawMessage  `json:"metadata,omitempty" db:"metadata"`

	// Resource references
	ExternalID   *string          `json:"external_id,omitempty" db:"external_id"`
	ExternalURL  *string          `json:"external_url,omitempty" db:"external_url"`

	// Ownership
	CreatedBy    uuid.UUID        `json:"created_by" db:"created_by"`
	UpdatedBy    uuid.UUID        `json:"updated_by" db:"updated_by"`

	// Timestamps
	CreatedAt    time.Time        `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time        `json:"updated_at" db:"updated_at"`
	DeletedAt    *time.Time       `json:"deleted_at,omitempty" db:"deleted_at"`
	LastAccessedAt *time.Time     `json:"last_accessed_at,omitempty" db:"last_accessed_at"`
}

// ResourceFolder represents a folder for organizing resources
type ResourceFolder struct {
	ID           uuid.UUID      `json:"id" db:"id"`
	WorkspaceID  uuid.UUID      `json:"workspace_id" db:"workspace_id"`
	Name         string         `json:"name" db:"name"`
	Path         string         `json:"path" db:"path"`
	ParentID     *uuid.UUID     `json:"parent_id,omitempty" db:"parent_id"`
	Description  string         `json:"description" db:"description"`

	// Folder settings
	IsPublic     bool           `json:"is_public" db:"is_public"`
	SortOrder    int            `json:"sort_order" db:"sort_order"`

	// Timestamps
	CreatedAt    time.Time      `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at" db:"updated_at"`
	DeletedAt    *time.Time     `json:"deleted_at,omitempty" db:"deleted_at"`
}

// ResourceTag represents a tag that can be applied to resources
type ResourceTag struct {
	ID           uuid.UUID      `json:"id" db:"id"`
	WorkspaceID  uuid.UUID      `json:"workspace_id" db:"workspace_id"`
	Name         string         `json:"name" db:"name"`
	Color        string         `json:"color" db:"color"`
	Description  string         `json:"description" db:"description"`

	// Timestamps
	CreatedAt    time.Time      `json:"created_at" db:"created_at"`
}

// ResourceAssociation represents a relationship between resources
type ResourceAssociation struct {
	ID           uuid.UUID      `json:"id" db:"id"`
	ResourceID   uuid.UUID      `json:"resource_id" db:"resource_id"`
	RelatedID    uuid.UUID      `json:"related_id" db:"related_id"`
	Type         string         `json:"type" db:"type"` // depends_on, uses, produces, references
	Metadata     json.RawMessage `json:"metadata,omitempty" db:"metadata"`
	CreatedAt    time.Time      `json:"created_at" db:"created_at"`
}

// CreateResourceRequest represents a request to create a new resource
type CreateResourceRequest struct {
	WorkspaceID  uuid.UUID      `json:"workspace_id"`
	Type         ResourceType   `json:"type"`
	Name         string         `json:"name"`
	Slug         string         `json:"slug"`
	Description  string         `json:"description,omitempty"`
	Config       interface{}    `json:"config,omitempty"`
	Metadata     interface{}    `json:"metadata,omitempty"`
	ExternalID   *string        `json:"external_id,omitempty"`
	ExternalURL  *string        `json:"external_url,omitempty"`
}

// UpdateResourceRequest represents a request to update a resource
type UpdateResourceRequest struct {
	Name         *string         `json:"name,omitempty"`
	Slug         *string         `json:"slug,omitempty"`
	Description  *string         `json:"description,omitempty"`
	Status       *ResourceStatus `json:"status,omitempty"`
	Config       interface{}     `json:"config,omitempty"`
	Metadata     interface{}     `json:"metadata,omitempty"`
	ExternalURL  *string         `json:"external_url,omitempty"`
}

// ListResourcesOptions represents options for listing resources
type ListResourcesOptions struct {
	WorkspaceID  *uuid.UUID
	Type         *ResourceType
	Status       *ResourceStatus
	Tag          *string
	FolderID     *uuid.UUID
	CreatedBy    *uuid.UUID
	Limit        int
	Offset       int
	Search       string
}

// ResourceManager manages workspace resources
type ResourceManager struct {
	db *sql.DB
}

// NewResourceManager creates a new resource manager
func NewResourceManager(db *sql.DB) *ResourceManager {
	return &ResourceManager{
		db: db,
	}
}

// CreateResource creates a new workspace resource
func (rm *ResourceManager) CreateResource(ctx context.Context, req CreateResourceRequest, createdBy uuid.UUID) (*Resource, error) {
	if req.WorkspaceID == uuid.Nil {
		return nil, ErrInvalidResource
	}
	if !isValidResourceType(req.Type) {
		return nil, ErrInvalidResourceType
	}
	if req.Name == "" {
		return nil, ErrInvalidResource
	}
	if req.Slug == "" {
		return nil, ErrInvalidResource
	}

	// Check if resource with slug already exists in this workspace
	existing, err := rm.GetResourceBySlug(ctx, req.WorkspaceID, req.Slug)
	if err == nil && existing != nil {
		return nil, ErrResourceAlreadyExists
	}

	resource := &Resource{
		ID:          uuid.New(),
		WorkspaceID: req.WorkspaceID,
		Type:        req.Type,
		Name:        req.Name,
		Slug:        req.Slug,
		Description: req.Description,
		Status:      ResourceStatusActive,
		CreatedBy:   createdBy,
		UpdatedBy:   createdBy,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		ExternalID:  req.ExternalID,
		ExternalURL: req.ExternalURL,
	}

	// Marshal config and metadata
	if req.Config != nil {
		configJSON, err := json.Marshal(req.Config)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal config: %w", err)
		}
		resource.Config = configJSON
	}

	if req.Metadata != nil {
		metadataJSON, err := json.Marshal(req.Metadata)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal metadata: %w", err)
		}
		resource.Metadata = metadataJSON
	}

	query := `
		INSERT INTO resources (id, workspace_id, type, name, slug, description, status,
			config, metadata, external_id, external_url, created_by, updated_by,
			created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		RETURNING id, workspace_id, type, name, slug, description, status, config,
			metadata, external_id, external_url, created_by, updated_by, created_at,
			updated_at, deleted_at, last_accessed_at
	`

	err = rm.db.QueryRowContext(ctx, query,
		resource.ID,
		resource.WorkspaceID,
		resource.Type,
		resource.Name,
		resource.Slug,
		resource.Description,
		resource.Status,
		resource.Config,
		resource.Metadata,
		resource.ExternalID,
		resource.ExternalURL,
		resource.CreatedBy,
		resource.UpdatedBy,
		resource.CreatedAt,
		resource.UpdatedAt,
	).Scan(
		&resource.ID,
		&resource.WorkspaceID,
		&resource.Type,
		&resource.Name,
		&resource.Slug,
		&resource.Description,
		&resource.Status,
		&resource.Config,
		&resource.Metadata,
		&resource.ExternalID,
		&resource.ExternalURL,
		&resource.CreatedBy,
		&resource.UpdatedBy,
		&resource.CreatedAt,
		&resource.UpdatedAt,
		&resource.DeletedAt,
		&resource.LastAccessedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	return resource, nil
}

// GetResource retrieves a resource by ID
func (rm *ResourceManager) GetResource(ctx context.Context, id uuid.UUID) (*Resource, error) {
	if id == uuid.Nil {
		return nil, ErrInvalidResource
	}

	var resource Resource

	query := `
		SELECT id, workspace_id, type, name, slug, description, status, config,
			metadata, external_id, external_url, created_by, updated_by, created_at,
			updated_at, deleted_at, last_accessed_at
		FROM resources
		WHERE id = $1 AND deleted_at IS NULL
	`

	err := rm.db.QueryRowContext(ctx, query, id).Scan(
		&resource.ID,
		&resource.WorkspaceID,
		&resource.Type,
		&resource.Name,
		&resource.Slug,
		&resource.Description,
		&resource.Status,
		&resource.Config,
		&resource.Metadata,
		&resource.ExternalID,
		&resource.ExternalURL,
		&resource.CreatedBy,
		&resource.UpdatedBy,
		&resource.CreatedAt,
		&resource.UpdatedAt,
		&resource.DeletedAt,
		&resource.LastAccessedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrResourceNotFound
		}
		return nil, fmt.Errorf("failed to get resource: %w", err)
	}

	return &resource, nil
}

// GetResourceBySlug retrieves a resource by workspace ID and slug
func (rm *ResourceManager) GetResourceBySlug(ctx context.Context, workspaceID uuid.UUID, slug string) (*Resource, error) {
	if workspaceID == uuid.Nil {
		return nil, ErrInvalidResource
	}

	var resource Resource

	query := `
		SELECT id, workspace_id, type, name, slug, description, status, config,
			metadata, external_id, external_url, created_by, updated_by, created_at,
			updated_at, deleted_at, last_accessed_at
		FROM resources
		WHERE workspace_id = $1 AND slug = $2 AND deleted_at IS NULL
	`

	err := rm.db.QueryRowContext(ctx, query, workspaceID, slug).Scan(
		&resource.ID,
		&resource.WorkspaceID,
		&resource.Type,
		&resource.Name,
		&resource.Slug,
		&resource.Description,
		&resource.Status,
		&resource.Config,
		&resource.Metadata,
		&resource.ExternalID,
		&resource.ExternalURL,
		&resource.CreatedBy,
		&resource.UpdatedBy,
		&resource.CreatedAt,
		&resource.UpdatedAt,
		&resource.DeletedAt,
		&resource.LastAccessedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrResourceNotFound
		}
		return nil, fmt.Errorf("failed to get resource by slug: %w", err)
	}

	return &resource, nil
}

// UpdateResource updates a resource
func (rm *ResourceManager) UpdateResource(ctx context.Context, id uuid.UUID, req UpdateResourceRequest, updatedBy uuid.UUID) (*Resource, error) {
	if id == uuid.Nil {
		return nil, ErrInvalidResource
	}

	resource, err := rm.GetResource(ctx, id)
	if err != nil {
		return nil, err
	}

	// Apply updates
	if req.Name != nil {
		resource.Name = *req.Name
	}
	if req.Slug != nil {
		resource.Slug = *req.Slug
	}
	if req.Description != nil {
		resource.Description = *req.Description
	}
	if req.Status != nil {
		if !isValidResourceStatus(*req.Status) {
			return nil, errors.New("invalid resource status")
		}
		resource.Status = *req.Status
	}
	if req.ExternalURL != nil {
		resource.ExternalURL = req.ExternalURL
	}
	if req.Config != nil {
		configJSON, err := json.Marshal(req.Config)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal config: %w", err)
		}
		resource.Config = configJSON
	}
	if req.Metadata != nil {
		metadataJSON, err := json.Marshal(req.Metadata)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal metadata: %w", err)
		}
		resource.Metadata = metadataJSON
	}
	resource.UpdatedBy = updatedBy
	resource.UpdatedAt = time.Now()

	query := `
		UPDATE resources
		SET name = $1, slug = $2, description = $3, status = $4, config = $5,
			metadata = $6, external_url = $7, updated_by = $8, updated_at = $9
		WHERE id = $10
		RETURNING id, workspace_id, type, name, slug, description, status, config,
			metadata, external_id, external_url, created_by, updated_by, created_at,
			updated_at, deleted_at, last_accessed_at
	`

	err = rm.db.QueryRowContext(ctx, query,
		resource.Name,
		resource.Slug,
		resource.Description,
		resource.Status,
		resource.Config,
		resource.Metadata,
		resource.ExternalURL,
		resource.UpdatedBy,
		resource.UpdatedAt,
		resource.ID,
	).Scan(
		&resource.ID,
		&resource.WorkspaceID,
		&resource.Type,
		&resource.Name,
		&resource.Slug,
		&resource.Description,
		&resource.Status,
		&resource.Config,
		&resource.Metadata,
		&resource.ExternalID,
		&resource.ExternalURL,
		&resource.CreatedBy,
		&resource.UpdatedBy,
		&resource.CreatedAt,
		&resource.UpdatedAt,
		&resource.DeletedAt,
		&resource.LastAccessedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to update resource: %w", err)
	}

	return resource, nil
}

// DeleteResource deletes a resource
func (rm *ResourceManager) DeleteResource(ctx context.Context, id uuid.UUID) error {
	if id == uuid.Nil {
		return ErrInvalidResource
	}

	// Check if resource has dependencies
	hasDependencies, err := rm.resourceHasDependencies(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to check resource dependencies: %w", err)
	}
	if hasDependencies {
		return ErrCannotRemoveResource
	}

	// Soft delete
	query := `
		UPDATE resources
		SET deleted_at = CURRENT_TIMESTAMP, status = 'deleted', updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
	`

	result, err := rm.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete resource: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return ErrResourceNotFound
	}

	return nil
}

// ListResources lists resources with optional filters
func (rm *ResourceManager) ListResources(ctx context.Context, opts ListResourcesOptions) ([]*Resource, int64, error) {
	resources := []*Resource{}
	var total int64

	// Build query with dynamic filters
	baseQuery := `
		SELECT id, workspace_id, type, name, slug, description, status, config,
			metadata, external_id, external_url, created_by, updated_by, created_at,
			updated_at, deleted_at, last_accessed_at
		FROM resources
		WHERE deleted_at IS NULL
	`
	countQuery := `
		SELECT COUNT(*)
		FROM resources
		WHERE deleted_at IS NULL
	`

	args := []interface{}{}
	argPos := 1

	if opts.WorkspaceID != nil {
		baseQuery += fmt.Sprintf(" AND workspace_id = $%d", argPos)
		countQuery += fmt.Sprintf(" AND workspace_id = $%d", argPos)
		args = append(args, *opts.WorkspaceID)
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

	if opts.CreatedBy != nil {
		baseQuery += fmt.Sprintf(" AND created_by = $%d", argPos)
		countQuery += fmt.Sprintf(" AND created_by = $%d", argPos)
		args = append(args, *opts.CreatedBy)
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
	err := rm.db.QueryRowContext(ctx, countQuery, args...[:argPos-1]...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count resources: %w", err)
	}

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

	baseQuery += " ORDER BY created_at DESC"

	rows, err := rm.db.QueryContext(ctx, baseQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list resources: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var resource Resource

		err := rows.Scan(
			&resource.ID,
			&resource.WorkspaceID,
			&resource.Type,
			&resource.Name,
			&resource.Slug,
			&resource.Description,
			&resource.Status,
			&resource.Config,
			&resource.Metadata,
			&resource.ExternalID,
			&resource.ExternalURL,
			&resource.CreatedBy,
			&resource.UpdatedBy,
			&resource.CreatedAt,
			&resource.UpdatedAt,
			&resource.DeletedAt,
			&resource.LastAccessedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan resource: %w", err)
		}

		resources = append(resources, &resource)
	}

	if err = rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating resources: %w", err)
	}

	return resources, total, nil
}

// GetResourcesByWorkspace retrieves all resources for a workspace
func (rm *ResourceManager) GetResourcesByWorkspace(ctx context.Context, workspaceID uuid.UUID) ([]*Resource, error) {
	if workspaceID == uuid.Nil {
		return nil, ErrInvalidResource
	}

	var resources []*Resource

	query := `
		SELECT id, workspace_id, type, name, slug, description, status, config,
			metadata, external_id, external_url, created_by, updated_by, created_at,
			updated_at, deleted_at, last_accessed_at
		FROM resources
		WHERE workspace_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
	`

	rows, err := rm.db.QueryContext(ctx, query, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get resources by workspace: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var resource Resource

		err := rows.Scan(
			&resource.ID,
			&resource.WorkspaceID,
			&resource.Type,
			&resource.Name,
			&resource.Slug,
			&resource.Description,
			&resource.Status,
			&resource.Config,
			&resource.Metadata,
			&resource.ExternalID,
			&resource.ExternalURL,
			&resource.CreatedBy,
			&resource.UpdatedBy,
			&resource.CreatedAt,
			&resource.UpdatedAt,
			&resource.DeletedAt,
			&resource.LastAccessedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan resource: %w", err)
		}

		resources = append(resources, &resource)
	}

	return resources, nil
}

// UpdateLastAccessed updates the last accessed time for a resource
func (rm *ResourceManager) UpdateLastAccessed(ctx context.Context, id uuid.UUID) error {
	query := `
		UPDATE resources
		SET last_accessed_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
	`

	_, err := rm.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to update last accessed: %w", err)
	}

	return nil
}

// CreateFolder creates a new resource folder
func (rm *ResourceManager) CreateFolder(ctx context.Context, workspaceID uuid.UUID, name, path string, parentID *uuid.UUID, createdBy uuid.UUID) (*ResourceFolder, error) {
	if workspaceID == uuid.Nil {
		return nil, ErrInvalidResource
	}
	if name == "" {
		return nil, ErrInvalidResource
	}

	folder := &ResourceFolder{
		ID:          uuid.New(),
		WorkspaceID: workspaceID,
		Name:        name,
		Path:        path,
		ParentID:    parentID,
		Description: "",
		IsPublic:    false,
		SortOrder:   0,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	query := `
		INSERT INTO resource_folders (id, workspace_id, name, path, parent_id, description,
			is_public, sort_order, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, workspace_id, name, path, parent_id, description, is_public,
			sort_order, created_at, updated_at, deleted_at
	`

	err := rm.db.QueryRowContext(ctx, query,
		folder.ID,
		folder.WorkspaceID,
		folder.Name,
		folder.Path,
		folder.ParentID,
		folder.Description,
		folder.IsPublic,
		folder.SortOrder,
		folder.CreatedAt,
		folder.UpdatedAt,
	).Scan(
		&folder.ID,
		&folder.WorkspaceID,
		&folder.Name,
		&folder.Path,
		&folder.ParentID,
		&folder.Description,
		&folder.IsPublic,
		&folder.SortOrder,
		&folder.CreatedAt,
		&folder.UpdatedAt,
		&folder.DeletedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create folder: %w", err)
	}

	return folder, nil
}

// ListFolders lists all folders for a workspace
func (rm *ResourceManager) ListFolders(ctx context.Context, workspaceID uuid.UUID) ([]*ResourceFolder, error) {
	if workspaceID == uuid.Nil {
		return nil, ErrInvalidResource
	}

	var folders []*ResourceFolder

	query := `
		SELECT id, workspace_id, name, path, parent_id, description, is_public,
			sort_order, created_at, updated_at, deleted_at
		FROM resource_folders
		WHERE workspace_id = $1 AND deleted_at IS NULL
		ORDER BY path ASC
	`

	rows, err := rm.db.QueryContext(ctx, query, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to list folders: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var folder ResourceFolder

		err := rows.Scan(
			&folder.ID,
			&folder.WorkspaceID,
			&folder.Name,
			&folder.Path,
			&folder.ParentID,
			&folder.Description,
			&folder.IsPublic,
			&folder.SortOrder,
			&folder.CreatedAt,
			&folder.UpdatedAt,
			&folder.DeletedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan folder: %w", err)
		}

		folders = append(folders, &folder)
	}

	return folders, nil
}

// CreateTag creates a new resource tag
func (rm *ResourceManager) CreateTag(ctx context.Context, workspaceID uuid.UUID, name, color, description string) (*ResourceTag, error) {
	if workspaceID == uuid.Nil {
		return nil, ErrInvalidResource
	}
	if name == "" {
		return nil, ErrInvalidResource
	}

	tag := &ResourceTag{
		ID:          uuid.New(),
		WorkspaceID: workspaceID,
		Name:        name,
		Color:       color,
		Description: description,
		CreatedAt:   time.Now(),
	}

	query := `
		INSERT INTO resource_tags (id, workspace_id, name, color, description, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, workspace_id, name, color, description, created_at
	`

	err := rm.db.QueryRowContext(ctx, query,
		tag.ID,
		tag.WorkspaceID,
		tag.Name,
		tag.Color,
		tag.Description,
		tag.CreatedAt,
	).Scan(
		&tag.ID,
		&tag.WorkspaceID,
		&tag.Name,
		&tag.Color,
		&tag.Description,
		&tag.CreatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create tag: %w", err)
	}

	return tag, nil
}

// ListTags lists all tags for a workspace
func (rm *ResourceManager) ListTags(ctx context.Context, workspaceID uuid.UUID) ([]*ResourceTag, error) {
	if workspaceID == uuid.Nil {
		return nil, ErrInvalidResource
	}

	var tags []*ResourceTag

	query := `
		SELECT id, workspace_id, name, color, description, created_at
		FROM resource_tags
		WHERE workspace_id = $1
		ORDER BY name ASC
	`

	rows, err := rm.db.QueryContext(ctx, query, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to list tags: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var tag ResourceTag

		err := rows.Scan(
			&tag.ID,
			&tag.WorkspaceID,
			&tag.Name,
			&tag.Color,
			&tag.Description,
			&tag.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan tag: %w", err)
		}

		tags = append(tags, &tag)
	}

	return tags, nil
}

// AddResourceAssociation creates an association between two resources
func (rm *ResourceManager) AddResourceAssociation(ctx context.Context, resourceID, relatedID uuid.UUID, associationType string, metadata interface{}) error {
	if resourceID == uuid.Nil || relatedID == uuid.Nil {
		return ErrInvalidResource
	}

	var metadataJSON json.RawMessage
	if metadata != nil {
		var err error
		metadataJSON, err = json.Marshal(metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}
	}

	query := `
		INSERT INTO resource_associations (id, resource_id, related_id, type, metadata, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`

	_, err := rm.db.ExecContext(ctx, query,
		uuid.New(),
		resourceID,
		relatedID,
		associationType,
		metadataJSON,
		time.Now(),
	)

	if err != nil {
		return fmt.Errorf("failed to add resource association: %w", err)
	}

	return nil
}

// GetResourceAssociations retrieves all associations for a resource
func (rm *ResourceManager) GetResourceAssociations(ctx context.Context, resourceID uuid.UUID) ([]*ResourceAssociation, error) {
	if resourceID == uuid.Nil {
		return nil, ErrInvalidResource
	}

	var associations []*ResourceAssociation

	query := `
		SELECT id, resource_id, related_id, type, metadata, created_at
		FROM resource_associations
		WHERE resource_id = $1 OR related_id = $1
		ORDER BY created_at DESC
	`

	rows, err := rm.db.QueryContext(ctx, query, resourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get resource associations: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var association ResourceAssociation

		err := rows.Scan(
			&association.ID,
			&association.ResourceID,
			&association.RelatedID,
			&association.Type,
			&association.Metadata,
			&association.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan resource association: %w", err)
		}

		associations = append(associations, &association)
	}

	return associations, nil
}

func (rm *ResourceManager) resourceHasDependencies(ctx context.Context, id uuid.UUID) (bool, error) {
	var count int

	query := `
		SELECT COUNT(*)
		FROM resource_associations
		WHERE resource_id = $1 OR related_id = $1
	`

	err := rm.db.QueryRowContext(ctx, query, id).Scan(&count)
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

func isValidResourceType(resourceType ResourceType) bool {
	switch resourceType {
	case ResourceTypeModel, ResourceTypeDataset, ResourceTypeNotebook,
		ResourceTypeAPI, ResourceTypePipeline, ResourceTypeWorkflow,
		ResourceTypeDocument, ResourceTypeImage, ResourceTypeSecret:
		return true
	default:
		return false
	}
}

func isValidResourceStatus(status ResourceStatus) bool {
	switch status {
	case ResourceStatusActive, ResourceStatusArchived, ResourceStatusDraft,
		ResourceStatusDeleted, ResourceStatusError, ResourceStatusProcessing:
		return true
	default:
		return false
	}
}
