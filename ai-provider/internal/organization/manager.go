// internal/organization/manager.go
// Organization management with CRUD operations and tenant association
// Handles organization creation, management, and lifecycle

package organization

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

var (
	ErrOrganizationNotFound       = errors.New("organization not found")
	ErrOrganizationAlreadyExists = errors.New("organization already exists")
	ErrInvalidOrganization       = errors.New("invalid organization data")
	ErrOrganizationCannotDelete   = errors.New("cannot delete organization with active resources")
	ErrInvalidTenantID          = errors.New("invalid tenant ID")
)

// OrganizationStatus represents the status of an organization
type OrganizationStatus string

const (
	OrgStatusActive    OrganizationStatus = "active"
	OrgStatusSuspended OrganizationStatus = "suspended"
	OrgStatusPending   OrganizationStatus = "pending"
	OrgStatusDeleted   OrganizationStatus = "deleted"
)

// Organization represents a tenant organization
type Organization struct {
	ID           uuid.UUID          `json:"id" db:"id"`
	TenantID     uuid.UUID          `json:"tenant_id" db:"tenant_id"`
	Name         string             `json:"name" db:"name"`
	Slug         string             `json:"slug" db:"slug"`
	Description  string             `json:"description" db:"description"`
	Status       OrganizationStatus `json:"status" db:"status"`
	Settings     OrganizationConfig `json:"settings" db:"settings"`

	// Organization details
	Website      string    `json:"website,omitempty" db:"website"`
	Industry     string    `json:"industry,omitempty" db:"industry"`
	CompanySize  string    `json:"company_size,omitempty" db:"company_size"`
	Country      string    `json:"country,omitempty" db:"country"`
	Timezone     string    `json:"timezone,omitempty" db:"timezone"`

	// Contact information
	ContactEmail string    `json:"contact_email,omitempty" db:"contact_email"`
	ContactPhone string    `json:"contact_phone,omitempty" db:"contact_email"`

	// Timestamps
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at" db:"updated_at"`
	DeletedAt   *time.Time `json:"deleted_at,omitempty" db:"deleted_at"`

	// Metadata
	Metadata    map[string]interface{} `json:"metadata,omitempty" db:"metadata"`
}

// OrganizationConfig represents organization-level configuration
type OrganizationConfig struct {
	// Feature flags
	Features map[string]bool `json:"features"`

	// Resource limits
	MaxWorkspaces   int `json:"max_workspaces"`
	MaxMembers      int `json:"max_members"`
	MaxTeams        int `json:"max_teams"`

	// Preferences
	DefaultWorkspaceSettings map[string]interface{} `json:"default_workspace_settings"`

	// Security settings
	TwoFactorEnabled bool   `json:"two_factor_enabled"`
	SSOEnabled       bool   `json:"sso_enabled"`
	AllowedDomains   []string `json:"allowed_domains"`
}

// CreateOrganizationRequest represents a request to create an organization
type CreateOrganizationRequest struct {
	TenantID    uuid.UUID `json:"tenant_id"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	Description string    `json:"description,omitempty"`
	Website     string    `json:"website,omitempty"`
	Industry    string    `json:"industry,omitempty"`
	CompanySize string    `json:"company_size,omitempty"`
	Country     string    `json:"country,omitempty"`
	Timezone    string    `json:"timezone,omitempty"`
	ContactEmail string   `json:"contact_email,omitempty"`
	ContactPhone string   `json:"contact_phone,omitempty"`
}

// UpdateOrganizationRequest represents a request to update an organization
type UpdateOrganizationRequest struct {
	Name         *string             `json:"name,omitempty"`
	Slug         *string             `json:"slug,omitempty"`
	Description  *string             `json:"description,omitempty"`
	Status       *OrganizationStatus `json:"status,omitempty"`
	Website      *string             `json:"website,omitempty"`
	Industry     *string             `json:"industry,omitempty"`
	CompanySize  *string             `json:"company_size,omitempty"`
	Country      *string             `json:"country,omitempty"`
	Timezone     *string             `json:"timezone,omitempty"`
	ContactEmail *string             `json:"contact_email,omitempty"`
	ContactPhone *string             `json:"contact_phone,omitempty"`
	Settings     *OrganizationConfig `json:"settings,omitempty"`
}

// ListOrganizationsOptions represents options for listing organizations
type ListOrganizationsOptions struct {
	TenantID *uuid.UUID
	Status   *OrganizationStatus
	Industry *string
	Limit    int
	Offset   int
	Search   string
}

// Manager manages organizations
type Manager struct {
	db *sql.DB
}

// NewManager creates a new organization manager
func NewManager(db *sql.DB) *Manager {
	return &Manager{
		db: db,
	}
}

// CreateOrganization creates a new organization
func (m *Manager) CreateOrganization(ctx context.Context, req CreateOrganizationRequest) (*Organization, error) {
	if req.TenantID == uuid.Nil {
		return nil, ErrInvalidTenantID
	}
	if req.Name == "" {
		return nil, ErrInvalidOrganization
	}
	if req.Slug == "" {
		return nil, ErrInvalidOrganization
	}

	// Check if organization with slug already exists for this tenant
	existing, err := m.GetOrganizationBySlug(ctx, req.TenantID, req.Slug)
	if err == nil && existing != nil {
		return nil, ErrOrganizationAlreadyExists
	}

	// Set defaults
	if req.Timezone == "" {
		req.Timezone = "UTC"
	}

	organization := &Organization{
		ID:          uuid.New(),
		TenantID:    req.TenantID,
		Name:        req.Name,
		Slug:        req.Slug,
		Description: req.Description,
		Status:      OrgStatusActive,
		Website:     req.Website,
		Industry:    req.Industry,
		CompanySize: req.CompanySize,
		Country:     req.Country,
		Timezone:    req.Timezone,
		ContactEmail: req.ContactEmail,
		ContactPhone: req.ContactPhone,
		Settings: OrganizationConfig{
			Features:                make(map[string]bool),
			MaxWorkspaces:           10,
			MaxMembers:              100,
			MaxTeams:                20,
			DefaultWorkspaceSettings: make(map[string]interface{}),
			TwoFactorEnabled:        false,
			SSOEnabled:             false,
			AllowedDomains:          []string{},
		},
		Metadata:    make(map[string]interface{}),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	settingsJSON, err := json.Marshal(organization.Settings)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal settings: %w", err)
	}

	metadataJSON, err := json.Marshal(organization.Metadata)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal metadata: %w", err)
	}

	query := `
		INSERT INTO organizations (id, tenant_id, name, slug, description, status,
			website, industry, company_size, country, timezone, contact_email, contact_phone,
			settings, metadata, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
		RETURNING id, tenant_id, name, slug, description, status, website, industry,
			company_size, country, timezone, contact_email, contact_phone, settings, metadata,
			created_at, updated_at, deleted_at
	`

	err = m.db.QueryRowContext(ctx, query,
		organization.ID,
		organization.TenantID,
		organization.Name,
		organization.Slug,
		organization.Description,
		organization.Status,
		organization.Website,
		organization.Industry,
		organization.CompanySize,
		organization.Country,
		organization.Timezone,
		organization.ContactEmail,
		organization.ContactPhone,
		settingsJSON,
		metadataJSON,
		organization.CreatedAt,
		organization.UpdatedAt,
	).Scan(
		&organization.ID,
		&organization.TenantID,
		&organization.Name,
		&organization.Slug,
		&organization.Description,
		&organization.Status,
		&organization.Website,
		&organization.Industry,
		&organization.CompanySize,
		&organization.Country,
		&organization.Timezone,
		&organization.ContactEmail,
		&organization.ContactPhone,
		&settingsJSON,
		&metadataJSON,
		&organization.CreatedAt,
		&organization.UpdatedAt,
		&organization.DeletedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create organization: %w", err)
	}

	// Unmarshal settings and metadata
	json.Unmarshal(settingsJSON, &organization.Settings)
	json.Unmarshal(metadataJSON, &organization.Metadata)

	return organization, nil
}

// GetOrganization retrieves an organization by ID
func (m *Manager) GetOrganization(ctx context.Context, id uuid.UUID) (*Organization, error) {
	if id == uuid.Nil {
		return nil, ErrInvalidOrganization
	}

	var organization Organization
	var settingsJSON, metadataJSON []byte

	query := `
		SELECT id, tenant_id, name, slug, description, status, website, industry,
			company_size, country, timezone, contact_email, contact_phone, settings,
			metadata, created_at, updated_at, deleted_at
		FROM organizations
		WHERE id = $1 AND deleted_at IS NULL
	`

	err := m.db.QueryRowContext(ctx, query, id).Scan(
		&organization.ID,
		&organization.TenantID,
		&organization.Name,
		&organization.Slug,
		&organization.Description,
		&organization.Status,
		&organization.Website,
		&organization.Industry,
		&organization.CompanySize,
		&organization.Country,
		&organization.Timezone,
		&organization.ContactEmail,
		&organization.ContactPhone,
		&settingsJSON,
		&metadataJSON,
		&organization.CreatedAt,
		&organization.UpdatedAt,
		&organization.DeletedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrOrganizationNotFound
		}
		return nil, fmt.Errorf("failed to get organization: %w", err)
	}

	// Unmarshal JSON fields
	if settingsJSON != nil {
		json.Unmarshal(settingsJSON, &organization.Settings)
	}
	if metadataJSON != nil {
		json.Unmarshal(metadataJSON, &organization.Metadata)
	}

	return &organization, nil
}

// GetOrganizationBySlug retrieves an organization by tenant ID and slug
func (m *Manager) GetOrganizationBySlug(ctx context.Context, tenantID uuid.UUID, slug string) (*Organization, error) {
	if tenantID == uuid.Nil {
		return nil, ErrInvalidTenantID
	}
	if slug == "" {
		return nil, ErrInvalidOrganization
	}

	var organization Organization
	var settingsJSON, metadataJSON []byte

	query := `
		SELECT id, tenant_id, name, slug, description, status, website, industry,
			company_size, country, timezone, contact_email, contact_phone, settings,
			metadata, created_at, updated_at, deleted_at
		FROM organizations
		WHERE tenant_id = $1 AND slug = $2 AND deleted_at IS NULL
	`

	err := m.db.QueryRowContext(ctx, query, tenantID, slug).Scan(
		&organization.ID,
		&organization.TenantID,
		&organization.Name,
		&organization.Slug,
		&organization.Description,
		&organization.Status,
		&organization.Website,
		&organization.Industry,
		&organization.CompanySize,
		&organization.Country,
		&organization.Timezone,
		&organization.ContactEmail,
		&organization.ContactPhone,
		&settingsJSON,
		&metadataJSON,
		&organization.CreatedAt,
		&organization.UpdatedAt,
		&organization.DeletedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrOrganizationNotFound
		}
		return nil, fmt.Errorf("failed to get organization by slug: %w", err)
	}

	if settingsJSON != nil {
		json.Unmarshal(settingsJSON, &organization.Settings)
	}
	if metadataJSON != nil {
		json.Unmarshal(metadataJSON, &organization.Metadata)
	}

	return &organization, nil
}

// UpdateOrganization updates an organization
func (m *Manager) UpdateOrganization(ctx context.Context, id uuid.UUID, req UpdateOrganizationRequest) (*Organization, error) {
	if id == uuid.Nil {
		return nil, ErrInvalidOrganization
	}

	organization, err := m.GetOrganization(ctx, id)
	if err != nil {
		return nil, err
	}

	// Apply updates
	if req.Name != nil {
		organization.Name = *req.Name
	}
	if req.Slug != nil {
		organization.Slug = *req.Slug
	}
	if req.Description != nil {
		organization.Description = *req.Description
	}
	if req.Status != nil {
		if !isValidOrganizationStatus(*req.Status) {
			return nil, errors.New("invalid organization status")
		}
		organization.Status = *req.Status
	}
	if req.Website != nil {
		organization.Website = *req.Website
	}
	if req.Industry != nil {
		organization.Industry = *req.Industry
	}
	if req.CompanySize != nil {
		organization.CompanySize = *req.CompanySize
	}
	if req.Country != nil {
		organization.Country = *req.Country
	}
	if req.Timezone != nil {
		organization.Timezone = *req.Timezone
	}
	if req.ContactEmail != nil {
		organization.ContactEmail = *req.ContactEmail
	}
	if req.ContactPhone != nil {
		organization.ContactPhone = *req.ContactPhone
	}
	if req.Settings != nil {
		organization.Settings = *req.Settings
	}
	organization.UpdatedAt = time.Now()

	settingsJSON, err := json.Marshal(organization.Settings)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal settings: %w", err)
	}

	metadataJSON, err := json.Marshal(organization.Metadata)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal metadata: %w", err)
	}

	query := `
		UPDATE organizations
		SET name = $1, slug = $2, description = $3, status = $4, website = $5,
			industry = $6, company_size = $7, country = $8, timezone = $9,
			contact_email = $10, contact_phone = $11, settings = $12, metadata = $13,
			updated_at = $14
		WHERE id = $15
		RETURNING id, tenant_id, name, slug, description, status, website, industry,
			company_size, country, timezone, contact_email, contact_phone, settings,
			metadata, created_at, updated_at, deleted_at
	`

	err = m.db.QueryRowContext(ctx, query,
		organization.Name,
		organization.Slug,
		organization.Description,
		organization.Status,
		organization.Website,
		organization.Industry,
		organization.CompanySize,
		organization.Country,
		organization.Timezone,
		organization.ContactEmail,
		organization.ContactPhone,
		settingsJSON,
		metadataJSON,
		organization.UpdatedAt,
		organization.ID,
	).Scan(
		&organization.ID,
		&organization.TenantID,
		&organization.Name,
		&organization.Slug,
		&organization.Description,
		&organization.Status,
		&organization.Website,
		&organization.Industry,
		&organization.CompanySize,
		&organization.Country,
		&organization.Timezone,
		&organization.ContactEmail,
		&organization.ContactPhone,
		&settingsJSON,
		&metadataJSON,
		&organization.CreatedAt,
		&organization.UpdatedAt,
		&organization.DeletedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to update organization: %w", err)
	}

	json.Unmarshal(settingsJSON, &organization.Settings)
	json.Unmarshal(metadataJSON, &organization.Metadata)

	return organization, nil
}

// DeleteOrganization deletes an organization
func (m *Manager) DeleteOrganization(ctx context.Context, id uuid.UUID) error {
	if id == uuid.Nil {
		return ErrInvalidOrganization
	}

	// Check if organization has active resources
	hasResources, err := m.organizationHasActiveResources(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to check organization resources: %w", err)
	}
	if hasResources {
		return ErrOrganizationCannotDelete
	}

	// Soft delete
	query := `
		UPDATE organizations
		SET deleted_at = CURRENT_TIMESTAMP, status = 'deleted', updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
	`

	result, err := m.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete organization: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return ErrOrganizationNotFound
	}

	return nil
}

// ListOrganizations lists organizations with optional filters
func (m *Manager) ListOrganizations(ctx context.Context, opts ListOrganizationsOptions) ([]*Organization, int64, error) {
	organizations := []*Organization{}
	var total int64

	// Build query with dynamic filters
	baseQuery := `
		SELECT id, tenant_id, name, slug, description, status, website, industry,
			company_size, country, timezone, contact_email, contact_phone, settings,
			metadata, created_at, updated_at, deleted_at
		FROM organizations
		WHERE deleted_at IS NULL
	`
	countQuery := `
		SELECT COUNT(*)
		FROM organizations
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

	if opts.Status != nil {
		baseQuery += fmt.Sprintf(" AND status = $%d", argPos)
		countQuery += fmt.Sprintf(" AND status = $%d", argPos)
		args = append(args, *opts.Status)
		argPos++
	}

	if opts.Industry != nil {
		baseQuery += fmt.Sprintf(" AND industry = $%d", argPos)
		countQuery += fmt.Sprintf(" AND industry = $%d", argPos)
		args = append(args, *opts.Industry)
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
		return nil, 0, fmt.Errorf("failed to count organizations: %w", err)
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

	rows, err := m.db.QueryContext(ctx, baseQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list organizations: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var organization Organization
		var settingsJSON, metadataJSON []byte

		err := rows.Scan(
			&organization.ID,
			&organization.TenantID,
			&organization.Name,
			&organization.Slug,
			&organization.Description,
			&organization.Status,
			&organization.Website,
			&organization.Industry,
			&organization.CompanySize,
			&organization.Country,
			&organization.Timezone,
			&organization.ContactEmail,
			&organization.ContactPhone,
			&settingsJSON,
			&metadataJSON,
			&organization.CreatedAt,
			&organization.UpdatedAt,
			&organization.DeletedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan organization: %w", err)
		}

		if settingsJSON != nil {
			json.Unmarshal(settingsJSON, &organization.Settings)
		}
		if metadataJSON != nil {
			json.Unmarshal(metadataJSON, &organization.Metadata)
		}

		organizations = append(organizations, &organization)
	}

	if err = rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating organizations: %w", err)
	}

	return organizations, total, nil
}

// ActivateOrganization activates an organization
func (m *Manager) ActivateOrganization(ctx context.Context, id uuid.UUID) (*Organization, error) {
	return m.updateOrganizationStatus(ctx, id, OrgStatusActive)
}

// SuspendOrganization suspends an organization
func (m *Manager) SuspendOrganization(ctx context.Context, id uuid.UUID) (*Organization, error) {
	return m.updateOrganizationStatus(ctx, id, OrgStatusSuspended)
}

func (m *Manager) updateOrganizationStatus(ctx context.Context, id uuid.UUID, status OrganizationStatus) (*Organization, error) {
	if id == uuid.Nil {
		return nil, ErrInvalidOrganization
	}

	if !isValidOrganizationStatus(status) {
		return nil, errors.New("invalid organization status")
	}

	organization, err := m.GetOrganization(ctx, id)
	if err != nil {
		return nil, err
	}

	if organization.Status == status {
		return organization, nil
	}

	query := `
		UPDATE organizations
		SET status = $1, updated_at = CURRENT_TIMESTAMP
		WHERE id = $2
		RETURNING id, tenant_id, name, slug, description, status, website, industry,
			company_size, country, timezone, contact_email, contact_phone, settings,
			metadata, created_at, updated_at, deleted_at
	`

	var settingsJSON, metadataJSON []byte
	err = m.db.QueryRowContext(ctx, query, status, id).Scan(
		&organization.ID,
		&organization.TenantID,
		&organization.Name,
		&organization.Slug,
		&organization.Description,
		&organization.Status,
		&organization.Website,
		&organization.Industry,
		&organization.CompanySize,
		&organization.Country,
		&organization.Timezone,
		&organization.ContactEmail,
		&organization.ContactPhone,
		&settingsJSON,
		&metadataJSON,
		&organization.CreatedAt,
		&organization.UpdatedAt,
		&organization.DeletedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to update organization status: %w", err)
	}

	if settingsJSON != nil {
		json.Unmarshal(settingsJSON, &organization.Settings)
	}
	if metadataJSON != nil {
		json.Unmarshal(metadataJSON, &organization.Metadata)
	}

	return organization, nil
}

// GetOrganizationsByTenant retrieves all organizations for a tenant
func (m *Manager) GetOrganizationsByTenant(ctx context.Context, tenantID uuid.UUID) ([]*Organization, error) {
	if tenantID == uuid.Nil {
		return nil, ErrInvalidTenantID
	}

	var organizations []*Organization

	query := `
		SELECT id, tenant_id, name, slug, description, status, website, industry,
			company_size, country, timezone, contact_email, contact_phone, settings,
			metadata, created_at, updated_at, deleted_at
		FROM organizations
		WHERE tenant_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
	`

	rows, err := m.db.QueryContext(ctx, query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get organizations by tenant: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var organization Organization
		var settingsJSON, metadataJSON []byte

		err := rows.Scan(
			&organization.ID,
			&organization.TenantID,
			&organization.Name,
			&organization.Slug,
			&organization.Description,
			&organization.Status,
			&organization.Website,
			&organization.Industry,
			&organization.CompanySize,
			&organization.Country,
			&organization.Timezone,
			&organization.ContactEmail,
			&organization.ContactPhone,
			&settingsJSON,
			&metadataJSON,
			&organization.CreatedAt,
			&organization.UpdatedAt,
			&organization.DeletedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan organization: %w", err)
		}

		if settingsJSON != nil {
			json.Unmarshal(settingsJSON, &organization.Settings)
		}
		if metadataJSON != nil {
			json.Unmarshal(metadataJSON, &organization.Metadata)
		}

		organizations = append(organizations, &organization)
	}

	return organizations, nil
}

func (m *Manager) organizationHasActiveResources(ctx context.Context, id uuid.UUID) (bool, error) {
	// Check for active workspaces
	var workspaceCount int
	err := m.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM workspaces WHERE organization_id = $1 AND deleted_at IS NULL",
		id,
	).Scan(&workspaceCount)
	if err != nil {
		return false, err
	}

	if workspaceCount > 0 {
		return true, nil
	}

	// Check for active teams
	var teamCount int
	err = m.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM teams WHERE organization_id = $1 AND deleted_at IS NULL",
		id,
	).Scan(&teamCount)
	if err != nil {
		return false, err
	}

	return teamCount > 0, nil
}

func isValidOrganizationStatus(status OrganizationStatus) bool {
	switch status {
	case OrgStatusActive, OrgStatusSuspended, OrgStatusPending, OrgStatusDeleted:
		return true
	default:
		return false
	}
}

// UpdateOrganizationSettings updates organization settings
func (m *Manager) UpdateOrganizationSettings(ctx context.Context, id uuid.UUID, settings OrganizationConfig) error {
	if id == uuid.Nil {
		return ErrInvalidOrganization
	}

	settingsJSON, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("failed to marshal settings: %w", err)
	}

	query := `
		UPDATE organizations
		SET settings = $1, updated_at = CURRENT_TIMESTAMP
		WHERE id = $2
	`

	_, err = m.db.ExecContext(ctx, query, settingsJSON, id)
	if err != nil {
		return fmt.Errorf("failed to update organization settings: %w", err)
	}

	return nil
}

// GetOrganizationSettings retrieves organization settings
func (m *Manager) GetOrganizationSettings(ctx context.Context, id uuid.UUID) (*OrganizationConfig, error) {
	org, err := m.GetOrganization(ctx, id)
	if err != nil {
		return nil, err
	}

	return &org.Settings, nil
}
