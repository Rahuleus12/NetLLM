// internal/workspace/templates.go
// Workspace templates for quick setup
// Handles workspace templates and template application

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
	ErrTemplateNotFound       = errors.New("template not found")
	ErrTemplateAlreadyExists  = errors.New("template already exists")
	ErrInvalidTemplate        = errors.New("invalid template data")
	ErrTemplateCannotDelete   = errors.New("cannot delete default template")
)

// TemplateCategory represents the category of a template
type TemplateCategory string

const (
	TemplateCategoryGeneral    TemplateCategory = "general"
	TemplateCategoryMachineLearning TemplateCategory = "machine_learning"
	TemplateCategoryDataScience   TemplateCategory = "data_science"
	TemplateCategoryDevelopment  TemplateCategory = "development"
	TemplateCategoryProduction  TemplateCategory = "production"
)

// WorkspaceTemplate represents a reusable workspace template
type WorkspaceTemplate struct {
	ID           uuid.UUID         `json:"id" db:"id"`
	Name         string            `json:"name" db:"name"`
	Slug         string            `json:"slug" db:"slug"`
	Description  string            `json:"description" db:"description"`
	Category     TemplateCategory  `json:"category" db:"category"`

	// Template configuration
	IsDefault    bool              `json:"is_default" db:"is_default"`
	IsPublic     bool              `json:"is_public" db:"is_public"`
	IsSystem     bool              `json:"is_system" db:"is_system"`

	// Workspace defaults
	DefaultSettings WorkspaceSettings `json:"default_settings" db:"default_settings"`
	ResourceDefaults map[string]interface{} `json:"resource_defaults" db:"resource_defaults"`

	// Template content
	Resources    []TemplateResource `json:"resources" db:"resources"`
	Folders      []TemplateFolder   `json:"folders" db:"folders"`

	// Metadata
	Tags         []string          `json:"tags" db:"tags"`
	Metadata     map[string]interface{} `json:"metadata,omitempty" db:"metadata"`

	// Ownership
	CreatedBy    uuid.UUID         `json:"created_by" db:"created_by"`
	OrganizationID *uuid.UUID      `json:"organization_id,omitempty" db:"organization_id"`

	// Timestamps
	CreatedAt    time.Time         `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at" db:"updated_at"`
}

// TemplateResource represents a resource in a template
type TemplateResource struct {
	Type         ResourceType     `json:"type"`
	Name         string           `json:"name"`
	Description  string           `json:"description"`
	Config       json.RawMessage  `json:"config"`
	Metadata     json.RawMessage  `json:"metadata"`
}

// TemplateFolder represents a folder in a template
type TemplateFolder struct {
	Name         string           `json:"name"`
	Path         string           `json:"path"`
	Description  string           `json:"description"`
	IsPublic     bool             `json:"is_public"`
}

// CreateTemplateRequest represents a request to create a new template
type CreateTemplateRequest struct {
	Name         string                  `json:"name"`
	Slug         string                  `json:"slug"`
	Description  string                  `json:"description"`
	Category     TemplateCategory         `json:"category"`
	IsPublic     bool                    `json:"is_public"`
	DefaultSettings WorkspaceSettings      `json:"default_settings"`
	ResourceDefaults map[string]interface{} `json:"resource_defaults"`
	Resources    []TemplateResource      `json:"resources"`
	Folders      []TemplateFolder        `json:"folders"`
	Tags         []string                `json:"tags"`
	OrganizationID *uuid.UUID           `json:"organization_id,omitempty"`
}

// UpdateTemplateRequest represents a request to update a template
type UpdateTemplateRequest struct {
	Name         *string                 `json:"name,omitempty"`
	Slug         *string                 `json:"slug,omitempty"`
	Description  *string                 `json:"description,omitempty"`
	Category     *TemplateCategory        `json:"category,omitempty"`
	IsPublic     *bool                   `json:"is_public,omitempty"`
	DefaultSettings *WorkspaceSettings    `json:"default_settings,omitempty"`
	ResourceDefaults map[string]interface{} `json:"resource_defaults,omitempty"`
	Resources    *[]TemplateResource     `json:"resources,omitempty"`
	Folders      *[]TemplateFolder       `json:"folders,omitempty"`
	Tags         *[]string               `json:"tags,omitempty"`
}

// ListTemplatesOptions represents options for listing templates
type ListTemplatesOptions struct {
	Category     *TemplateCategory
	IsPublic     *bool
	IsDefault    *bool
	OrganizationID *uuid.UUID
	Limit        int
	Offset       int
	Search       string
}

// TemplateManager manages workspace templates
type TemplateManager struct {
	db *sql.DB
}

// NewTemplateManager creates a new template manager
func NewTemplateManager(db *sql.DB) *TemplateManager {
	return &TemplateManager{
		db: db,
	}
}

// CreateTemplate creates a new workspace template
func (tm *TemplateManager) CreateTemplate(ctx context.Context, req CreateTemplateRequest, createdBy uuid.UUID) (*WorkspaceTemplate, error) {
	if req.Name == "" {
		return nil, ErrInvalidTemplate
	}
	if req.Slug == "" {
		return nil, ErrInvalidTemplate
	}
	if !isValidTemplateCategory(req.Category) {
		return nil, ErrInvalidTemplate
	}

	// Check if template with slug already exists
	existing, err := tm.GetTemplateBySlug(ctx, req.Slug)
	if err == nil && existing != nil {
		return nil, ErrTemplateAlreadyExists
	}

	template := &WorkspaceTemplate{
		ID:               uuid.New(),
		Name:             req.Name,
		Slug:             req.Slug,
		Description:      req.Description,
		Category:         req.Category,
		IsDefault:        false,
		IsPublic:         req.IsPublic,
		IsSystem:         false,
		DefaultSettings:   req.DefaultSettings,
		ResourceDefaults:  req.ResourceDefaults,
		Resources:        req.Resources,
		Folders:          req.Folders,
		Tags:             req.Tags,
		CreatedBy:        createdBy,
		OrganizationID:   req.OrganizationID,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	// Marshal JSON fields
	defaultSettingsJSON, err := json.Marshal(template.DefaultSettings)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal default settings: %w", err)
	}

	resourceDefaultsJSON, err := json.Marshal(template.ResourceDefaults)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal resource defaults: %w", err)
	}

	resourcesJSON, err := json.Marshal(template.Resources)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal resources: %w", err)
	}

	foldersJSON, err := json.Marshal(template.Folders)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal folders: %w", err)
	}

	tagsJSON, err := json.Marshal(template.Tags)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal tags: %w", err)
	}

	metadataJSON, err := json.Marshal(template.Metadata)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal metadata: %w", err)
	}

	query := `
		INSERT INTO workspace_templates (id, name, slug, description, category,
			is_default, is_public, is_system, default_settings, resource_defaults,
			resources, folders, tags, metadata, created_by, organization_id,
			created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
		RETURNING id, name, slug, description, category, is_default, is_public,
			is_system, default_settings, resource_defaults, resources, folders,
			tags, metadata, created_by, organization_id, created_at, updated_at
	`

	err = tm.db.QueryRowContext(ctx, query,
		template.ID,
		template.Name,
		template.Slug,
		template.Description,
		template.Category,
		template.IsDefault,
		template.IsPublic,
		template.IsSystem,
		defaultSettingsJSON,
		resourceDefaultsJSON,
		resourcesJSON,
		foldersJSON,
		tagsJSON,
		metadataJSON,
		template.CreatedBy,
		template.OrganizationID,
		template.CreatedAt,
		template.UpdatedAt,
	).Scan(
		&template.ID,
		&template.Name,
		&template.Slug,
		&template.Description,
		&template.Category,
		&template.IsDefault,
		&template.IsPublic,
		&template.IsSystem,
		&defaultSettingsJSON,
		&resourceDefaultsJSON,
		&resourcesJSON,
		&foldersJSON,
		&tagsJSON,
		&metadataJSON,
		&template.CreatedBy,
		&template.OrganizationID,
		&template.CreatedAt,
		&template.UpdatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create template: %w", err)
	}

	// Unmarshal JSON fields
	json.Unmarshal(defaultSettingsJSON, &template.DefaultSettings)
	json.Unmarshal(resourceDefaultsJSON, &template.ResourceDefaults)
	json.Unmarshal(resourcesJSON, &template.Resources)
	json.Unmarshal(foldersJSON, &template.Folders)
	json.Unmarshal(tagsJSON, &template.Tags)
	json.Unmarshal(metadataJSON, &template.Metadata)

	return template, nil
}

// GetTemplate retrieves a template by ID
func (tm *TemplateManager) GetTemplate(ctx context.Context, id uuid.UUID) (*WorkspaceTemplate, error) {
	if id == uuid.Nil {
		return nil, ErrInvalidTemplate
	}

	var template WorkspaceTemplate
	var defaultSettingsJSON, resourceDefaultsJSON, resourcesJSON, foldersJSON, tagsJSON, metadataJSON []byte

	query := `
		SELECT id, name, slug, description, category, is_default, is_public,
			is_system, default_settings, resource_defaults, resources, folders,
			tags, metadata, created_by, organization_id, created_at, updated_at
		FROM workspace_templates
		WHERE id = $1
	`

	err := tm.db.QueryRowContext(ctx, query, id).Scan(
		&template.ID,
		&template.Name,
		&template.Slug,
		&template.Description,
		&template.Category,
		&template.IsDefault,
		&template.IsPublic,
		&template.IsSystem,
		&defaultSettingsJSON,
		&resourceDefaultsJSON,
		&resourcesJSON,
		&foldersJSON,
		&tagsJSON,
		&metadataJSON,
		&template.CreatedBy,
		&template.OrganizationID,
		&template.CreatedAt,
		&template.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrTemplateNotFound
		}
		return nil, fmt.Errorf("failed to get template: %w", err)
	}

	// Unmarshal JSON fields
	if defaultSettingsJSON != nil {
		json.Unmarshal(defaultSettingsJSON, &template.DefaultSettings)
	}
	if resourceDefaultsJSON != nil {
		json.Unmarshal(resourceDefaultsJSON, &template.ResourceDefaults)
	}
	if resourcesJSON != nil {
		json.Unmarshal(resourcesJSON, &template.Resources)
	}
	if foldersJSON != nil {
		json.Unmarshal(foldersJSON, &template.Folders)
	}
	if tagsJSON != nil {
		json.Unmarshal(tagsJSON, &template.Tags)
	}
	if metadataJSON != nil {
		json.Unmarshal(metadataJSON, &template.Metadata)
	}

	return &template, nil
}

// GetTemplateBySlug retrieves a template by slug
func (tm *TemplateManager) GetTemplateBySlug(ctx context.Context, slug string) (*WorkspaceTemplate, error) {
	if slug == "" {
		return nil, ErrInvalidTemplate
	}

	var template WorkspaceTemplate
	var defaultSettingsJSON, resourceDefaultsJSON, resourcesJSON, foldersJSON, tagsJSON, metadataJSON []byte

	query := `
		SELECT id, name, slug, description, category, is_default, is_public,
			is_system, default_settings, resource_defaults, resources, folders,
			tags, metadata, created_by, organization_id, created_at, updated_at
		FROM workspace_templates
		WHERE slug = $1
	`

	err := tm.db.QueryRowContext(ctx, query, slug).Scan(
		&template.ID,
		&template.Name,
		&template.Slug,
		&template.Description,
		&template.Category,
		&template.IsDefault,
		&template.IsPublic,
		&template.IsSystem,
		&defaultSettingsJSON,
		&resourceDefaultsJSON,
		&resourcesJSON,
		&foldersJSON,
		&tagsJSON,
		&metadataJSON,
		&template.CreatedBy,
		&template.OrganizationID,
		&template.CreatedAt,
		&template.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrTemplateNotFound
		}
		return nil, fmt.Errorf("failed to get template by slug: %w", err)
	}

	if defaultSettingsJSON != nil {
		json.Unmarshal(defaultSettingsJSON, &template.DefaultSettings)
	}
	if resourceDefaultsJSON != nil {
		json.Unmarshal(resourceDefaultsJSON, &template.ResourceDefaults)
	}
	if resourcesJSON != nil {
		json.Unmarshal(resourcesJSON, &template.Resources)
	}
	if foldersJSON != nil {
		json.Unmarshal(foldersJSON, &template.Folders)
	}
	if tagsJSON != nil {
		json.Unmarshal(tagsJSON, &template.Tags)
	}
	if metadataJSON != nil {
		json.Unmarshal(metadataJSON, &template.Metadata)
	}

	return &template, nil
}

// UpdateTemplate updates a template
func (tm *TemplateManager) UpdateTemplate(ctx context.Context, id uuid.UUID, req UpdateTemplateRequest) (*WorkspaceTemplate, error) {
	if id == uuid.Nil {
		return nil, ErrInvalidTemplate
	}

	template, err := tm.GetTemplate(ctx, id)
	if err != nil {
		return nil, err
	}

	// Cannot update system templates
	if template.IsSystem {
		return nil, errors.New("cannot update system template")
	}

	// Apply updates
	if req.Name != nil {
		template.Name = *req.Name
	}
	if req.Slug != nil {
		template.Slug = *req.Slug
	}
	if req.Description != nil {
		template.Description = *req.Description
	}
	if req.Category != nil {
		if !isValidTemplateCategory(*req.Category) {
			return nil, errors.New("invalid template category")
		}
		template.Category = *req.Category
	}
	if req.IsPublic != nil {
		template.IsPublic = *req.IsPublic
	}
	if req.DefaultSettings != nil {
		template.DefaultSettings = *req.DefaultSettings
	}
	if req.ResourceDefaults != nil {
		template.ResourceDefaults = *req.ResourceDefaults
	}
	if req.Resources != nil {
		template.Resources = *req.Resources
	}
	if req.Folders != nil {
		template.Folders = *req.Folders
	}
	if req.Tags != nil {
		template.Tags = *req.Tags
	}
	template.UpdatedAt = time.Now()

	// Marshal JSON fields
	defaultSettingsJSON, err := json.Marshal(template.DefaultSettings)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal default settings: %w", err)
	}

	resourceDefaultsJSON, err := json.Marshal(template.ResourceDefaults)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal resource defaults: %w", err)
	}

	resourcesJSON, err := json.Marshal(template.Resources)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal resources: %w", err)
	}

	foldersJSON, err := json.Marshal(template.Folders)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal folders: %w", err)
	}

	tagsJSON, err := json.Marshal(template.Tags)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal tags: %w", err)
	}

	metadataJSON, err := json.Marshal(template.Metadata)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal metadata: %w", err)
	}

	query := `
		UPDATE workspace_templates
		SET name = $1, slug = $2, description = $3, category = $4, is_public = $5,
			default_settings = $6, resource_defaults = $7, resources = $8,
			folders = $9, tags = $10, metadata = $11, updated_at = $12
		WHERE id = $13
		RETURNING id, name, slug, description, category, is_default, is_public,
			is_system, default_settings, resource_defaults, resources, folders,
			tags, metadata, created_by, organization_id, created_at, updated_at
	`

	err = tm.db.QueryRowContext(ctx, query,
		template.Name,
		template.Slug,
		template.Description,
		template.Category,
		template.IsPublic,
		defaultSettingsJSON,
		resourceDefaultsJSON,
		resourcesJSON,
		foldersJSON,
		tagsJSON,
		metadataJSON,
		template.UpdatedAt,
		template.ID,
	).Scan(
		&template.ID,
		&template.Name,
		&template.Slug,
		&template.Description,
		&template.Category,
		&template.IsDefault,
		&template.IsPublic,
		&template.IsSystem,
		&defaultSettingsJSON,
		&resourceDefaultsJSON,
		&resourcesJSON,
		&foldersJSON,
		&tagsJSON,
		&metadataJSON,
		&template.CreatedBy,
		&template.OrganizationID,
		&template.CreatedAt,
		&template.UpdatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to update template: %w", err)
	}

	json.Unmarshal(defaultSettingsJSON, &template.DefaultSettings)
	json.Unmarshal(resourceDefaultsJSON, &template.ResourceDefaults)
	json.Unmarshal(resourcesJSON, &template.Resources)
	json.Unmarshal(foldersJSON, &template.Folders)
	json.Unmarshal(tagsJSON, &template.Tags)
	json.Unmarshal(metadataJSON, &template.Metadata)

	return template, nil
}

// DeleteTemplate deletes a template
func (tm *TemplateManager) DeleteTemplate(ctx context.Context, id uuid.UUID) error {
	if id == uuid.Nil {
		return ErrInvalidTemplate
	}

	// Check if it's a system or default template
	template, err := tm.GetTemplate(ctx, id)
	if err != nil {
		return err
	}

	if template.IsDefault || template.IsSystem {
		return ErrTemplateCannotDelete
	}

	query := `DELETE FROM workspace_templates WHERE id = $1`

	result, err := tm.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete template: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return ErrTemplateNotFound
	}

	return nil
}

// ListTemplates lists templates with optional filters
func (tm *TemplateManager) ListTemplates(ctx context.Context, opts ListTemplatesOptions) ([]*WorkspaceTemplate, int64, error) {
	templates := []*WorkspaceTemplate{}
	var total int64

	// Build query with dynamic filters
	baseQuery := `
		SELECT id, name, slug, description, category, is_default, is_public,
			is_system, default_settings, resource_defaults, resources, folders,
			tags, metadata, created_by, organization_id, created_at, updated_at
		FROM workspace_templates
		WHERE 1=1
	`
	countQuery := `SELECT COUNT(*) FROM workspace_templates WHERE 1=1`

	args := []interface{}{}
	argPos := 1

	if opts.Category != nil {
		baseQuery += fmt.Sprintf(" AND category = $%d", argPos)
		countQuery += fmt.Sprintf(" AND category = $%d", argPos)
		args = append(args, *opts.Category)
		argPos++
	}

	if opts.IsPublic != nil {
		baseQuery += fmt.Sprintf(" AND is_public = $%d", argPos)
		countQuery += fmt.Sprintf(" AND is_public = $%d", argPos)
		args = append(args, *opts.IsPublic)
		argPos++
	}

	if opts.IsDefault != nil {
		baseQuery += fmt.Sprintf(" AND is_default = $%d", argPos)
		countQuery += fmt.Sprintf(" AND is_default = $%d", argPos)
		args = append(args, *opts.IsDefault)
		argPos++
	}

	if opts.OrganizationID != nil {
		baseQuery += fmt.Sprintf(" AND organization_id = $%d", argPos)
		countQuery += fmt.Sprintf(" AND organization_id = $%d", argPos)
		args = append(args, *opts.OrganizationID)
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
	err := tm.db.QueryRowContext(ctx, countQuery, args...[:argPos-1]...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count templates: %w", err)
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

	baseQuery += " ORDER BY is_default DESC, created_at DESC"

	rows, err := tm.db.QueryContext(ctx, baseQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list templates: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var template WorkspaceTemplate
		var defaultSettingsJSON, resourceDefaultsJSON, resourcesJSON, foldersJSON, tagsJSON, metadataJSON []byte

		err := rows.Scan(
			&template.ID,
			&template.Name,
			&template.Slug,
			&template.Description,
			&template.Category,
			&template.IsDefault,
			&template.IsPublic,
			&template.IsSystem,
			&defaultSettingsJSON,
			&resourceDefaultsJSON,
			&resourcesJSON,
			&foldersJSON,
			&tagsJSON,
			&metadataJSON,
			&template.CreatedBy,
			&template.OrganizationID,
			&template.CreatedAt,
			&template.UpdatedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan template: %w", err)
		}

		if defaultSettingsJSON != nil {
			json.Unmarshal(defaultSettingsJSON, &template.DefaultSettings)
		}
		if resourceDefaultsJSON != nil {
			json.Unmarshal(resourceDefaultsJSON, &template.ResourceDefaults)
		}
		if resourcesJSON != nil {
			json.Unmarshal(resourcesJSON, &template.Resources)
		}
		if foldersJSON != nil {
			json.Unmarshal(foldersJSON, &template.Folders)
		}
		if tagsJSON != nil {
			json.Unmarshal(tagsJSON, &template.Tags)
		}
		if metadataJSON != nil {
			json.Unmarshal(metadataJSON, &template.Metadata)
		}

		templates = append(templates, &template)
	}

	if err = rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating templates: %w", err)
	}

	return templates, total, nil
}

// GetDefaultTemplates returns all default templates
func (tm *TemplateManager) GetDefaultTemplates(ctx context.Context) ([]*WorkspaceTemplate, error) {
	var templates []*WorkspaceTemplate

	query := `
		SELECT id, name, slug, description, category, is_default, is_public,
			is_system, default_settings, resource_defaults, resources, folders,
			tags, metadata, created_by, organization_id, created_at, updated_at
		FROM workspace_templates
		WHERE is_default = true OR is_system = true
		ORDER BY is_system DESC, name ASC
	`

	rows, err := tm.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get default templates: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var template WorkspaceTemplate
		var defaultSettingsJSON, resourceDefaultsJSON, resourcesJSON, foldersJSON, tagsJSON, metadataJSON []byte

		err := rows.Scan(
			&template.ID,
			&template.Name,
			&template.Slug,
			&template.Description,
			&template.Category,
			&template.IsDefault,
			&template.IsPublic,
			&template.IsSystem,
			&defaultSettingsJSON,
			&resourceDefaultsJSON,
			&resourcesJSON,
			&foldersJSON,
			&tagsJSON,
			&metadataJSON,
			&template.CreatedBy,
			&template.OrganizationID,
			&template.CreatedAt,
			&template.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan template: %w", err)
		}

		if defaultSettingsJSON != nil {
			json.Unmarshal(defaultSettingsJSON, &template.DefaultSettings)
		}
		if resourceDefaultsJSON != nil {
			json.Unmarshal(resourceDefaultsJSON, &template.ResourceDefaults)
		}
		if resourcesJSON != nil {
			json.Unmarshal(resourcesJSON, &template.Resources)
		}
		if foldersJSON != nil {
			json.Unmarshal(foldersJSON, &template.Folders)
		}
		if tagsJSON != nil {
			json.Unmarshal(tagsJSON, &template.Tags)
		}
		if metadataJSON != nil {
			json.Unmarshal(metadataJSON, &template.Metadata)
		}

		templates = append(templates, &template)
	}

	return templates, nil
}

// ApplyTemplate applies a template to a new workspace
func (tm *TemplateManager) ApplyTemplate(ctx context.Context, templateID uuid.UUID, workspaceID uuid.UUID) error {
	if templateID == uuid.Nil || workspaceID == uuid.Nil {
		return ErrInvalidTemplate
	}

	template, err := tm.GetTemplate(ctx, templateID)
	if err != nil {
		return err
	}

	// In a real implementation, this would create resources and folders
	// from the template in the new workspace
	// For now, we'll just update the workspace settings

	settingsJSON, err := json.Marshal(template.DefaultSettings)
	if err != nil {
		return fmt.Errorf("failed to marshal settings: %w", err)
	}

	query := `
		UPDATE workspaces
		SET settings = $1, updated_at = CURRENT_TIMESTAMP
		WHERE id = $2
	`

	_, err = tm.db.ExecContext(ctx, query, settingsJSON, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to apply template to workspace: %w", err)
	}

	return nil
}

func isValidTemplateCategory(category TemplateCategory) bool {
	switch category {
	case TemplateCategoryGeneral, TemplateCategoryMachineLearning,
		TemplateCategoryDataScience, TemplateCategoryDevelopment,
		TemplateCategoryProduction:
		return true
	default:
		return false
	}
