package tenant

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
	ErrTenantNotFound       = errors.New("tenant not found")
	ErrTenantAlreadyExists  = errors.New("tenant already exists")
	ErrTenantInvalid        = errors.New("invalid tenant data")
	ErrTenantCannotDelete   = errors.New("cannot delete tenant with active resources")
	ErrTenantQuotaExceeded  = errors.New("tenant quota exceeded")
	ErrTenantNotActive      = errors.New("tenant is not active")
	ErrInvalidTenantID      = errors.New("invalid tenant ID")
	ErrInvalidTenantStatus  = errors.New("invalid tenant status")
)

type TenantStatus string

const (
	TenantStatusActive    TenantStatus = "active"
	TenantStatusSuspended TenantStatus = "suspended"
	TenantStatusPending   TenantStatus = "pending"
	TenantStatusDeleted   TenantStatus = "deleted"
)

type Tenant struct {
	ID          uuid.UUID      `json:"id" db:"id"`
	Name        string         `json:"name" db:"name"`
	Slug        string         `json:"slug" db:"slug"`
	PlanID      *uuid.UUID     `json:"plan_id,omitempty" db:"plan_id"`
	Status      TenantStatus   `json:"status" db:"status"`
	Settings    TenantSettings `json:"settings" db:"settings"`
	CreatedAt   time.Time      `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at" db:"updated_at"`
	DeletedAt   *time.Time     `json:"deleted_at,omitempty" db:"deleted_at"`
}

type TenantSettings struct {
	MaxModels           int               `json:"max_models"`
	MaxStorageGB        int               `json:"max_storage_gb"`
	MaxAPIRequestsPerDay int              `json:"max_api_requests_per_day"`
	MaxConcurrentJobs   int               `json:"max_concurrent_jobs"`
	AllowedRegions      []string          `json:"allowed_regions"`
	CustomDomains       []string          `json:"custom_domains"`
	FeatureFlags        map[string]bool   `json:"feature_flags"`
	ResourceQuotas      ResourceQuotas    `json:"resource_quotas"`
}

type ResourceQuotas struct {
	CPUQuota       int `json:"cpu_quota"`
	MemoryQuotaMB  int `json:"memory_quota_mb"`
	StorageQuotaGB int `json:"storage_quota_gb"`
	NetworkQuotaMB int `json:"network_quota_mb"`
}

type CreateTenantRequest struct {
	Name     string         `json:"name"`
	Slug     string         `json:"slug"`
	PlanID   *uuid.UUID     `json:"plan_id,omitempty"`
	Settings TenantSettings `json:"settings,omitempty"`
}

type UpdateTenantRequest struct {
	Name     *string         `json:"name,omitempty"`
	Slug     *string         `json:"slug,omitempty"`
	Status   *TenantStatus   `json:"status,omitempty"`
	Settings *TenantSettings `json:"settings,omitempty"`
}

type ListTenantsOptions struct {
	Status   *TenantStatus
	PlanID   *uuid.UUID
	Limit    int
	Offset   int
	Search   string
}

type Manager struct {
	db *sql.DB
}

func NewManager(db *sql.DB) *Manager {
	return &Manager{
		db: db,
	}
}

func (m *Manager) CreateTenant(ctx context.Context, req CreateTenantRequest) (*Tenant, error) {
	if req.Name == "" {
		return nil, ErrTenantInvalid
	}
	if req.Slug == "" {
		return nil, ErrTenantInvalid
	}

	// Check if tenant with slug already exists
	existing, err := m.GetTenantBySlug(ctx, req.Slug)
	if err == nil && existing != nil {
		return nil, ErrTenantAlreadyExists
	}

	// Apply default settings if not provided
	settings := req.Settings
	if settings.FeatureFlags == nil {
		settings.FeatureFlags = make(map[string]bool)
	}
	if settings.AllowedRegions == nil {
		settings.AllowedRegions = []string{"us-east-1", "us-west-2"}
	}

	settingsJSON, err := json.Marshal(settings)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal settings: %w", err)
	}

	tenant := &Tenant{
		ID:        uuid.New(),
		Name:      req.Name,
		Slug:      req.Slug,
		PlanID:    req.PlanID,
		Status:    TenantStatusPending,
		Settings:  settings,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	query := `
		INSERT INTO tenants (id, name, slug, plan_id, status, settings, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, name, slug, plan_id, status, settings, created_at, updated_at, deleted_at
	`

	err = m.db.QueryRowContext(ctx, query,
		tenant.ID,
		tenant.Name,
		tenant.Slug,
		tenant.PlanID,
		tenant.Status,
		settingsJSON,
		tenant.CreatedAt,
		tenant.UpdatedAt,
	).Scan(
		&tenant.ID,
		&tenant.Name,
		&tenant.Slug,
		&tenant.PlanID,
		&tenant.Status,
		&tenant.Settings,
		&tenant.CreatedAt,
		&tenant.UpdatedAt,
		&tenant.DeletedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create tenant: %w", err)
	}

	// Activate tenant immediately for now (in production, might require verification)
	_, err = m.ActivateTenant(ctx, tenant.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to activate tenant: %w", err)
	}

	return tenant, nil
}

func (m *Manager) GetTenant(ctx context.Context, id uuid.UUID) (*Tenant, error) {
	if id == uuid.Nil {
		return nil, ErrInvalidTenantID
	}

	var tenant Tenant
	var settingsJSON []byte

	query := `
		SELECT id, name, slug, plan_id, status, settings, created_at, updated_at, deleted_at
		FROM tenants
		WHERE id = $1 AND deleted_at IS NULL
	`

	err := m.db.QueryRowContext(ctx, query, id).Scan(
		&tenant.ID,
		&tenant.Name,
		&tenant.Slug,
		&tenant.PlanID,
		&tenant.Status,
		&settingsJSON,
		&tenant.CreatedAt,
		&tenant.UpdatedAt,
		&tenant.DeletedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrTenantNotFound
		}
		return nil, fmt.Errorf("failed to get tenant: %w", err)
	}

	err = json.Unmarshal(settingsJSON, &tenant.Settings)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal settings: %w", err)
	}

	return &tenant, nil
}

func (m *Manager) GetTenantBySlug(ctx context.Context, slug string) (*Tenant, error) {
	var tenant Tenant
	var settingsJSON []byte

	query := `
		SELECT id, name, slug, plan_id, status, settings, created_at, updated_at, deleted_at
		FROM tenants
		WHERE slug = $1 AND deleted_at IS NULL
	`

	err := m.db.QueryRowContext(ctx, query, slug).Scan(
		&tenant.ID,
		&tenant.Name,
		&tenant.Slug,
		&tenant.PlanID,
		&tenant.Status,
		&settingsJSON,
		&tenant.CreatedAt,
		&tenant.UpdatedAt,
		&tenant.DeletedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrTenantNotFound
		}
		return nil, fmt.Errorf("failed to get tenant by slug: %w", err)
	}

	err = json.Unmarshal(settingsJSON, &tenant.Settings)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal settings: %w", err)
	}

	return &tenant, nil
}

func (m *Manager) UpdateTenant(ctx context.Context, id uuid.UUID, req UpdateTenantRequest) (*Tenant, error) {
	if id == uuid.Nil {
		return nil, ErrInvalidTenantID
	}

	tenant, err := m.GetTenant(ctx, id)
	if err != nil {
		return nil, err
	}

	// Update fields if provided
	if req.Name != nil {
		tenant.Name = *req.Name
	}
	if req.Slug != nil {
		tenant.Slug = *req.Slug
	}
	if req.Status != nil {
		if !isValidTenantStatus(*req.Status) {
			return nil, ErrInvalidTenantStatus
		}
		tenant.Status = *req.Status
	}
	if req.Settings != nil {
		tenant.Settings = *req.Settings
	}
	tenant.UpdatedAt = time.Now()

	settingsJSON, err := json.Marshal(tenant.Settings)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal settings: %w", err)
	}

	query := `
		UPDATE tenants
		SET name = $1, slug = $2, status = $3, settings = $4, updated_at = $5
		WHERE id = $6
		RETURNING id, name, slug, plan_id, status, settings, created_at, updated_at, deleted_at
	`

	err = m.db.QueryRowContext(ctx, query,
		tenant.Name,
		tenant.Slug,
		tenant.Status,
		settingsJSON,
		tenant.UpdatedAt,
		tenant.ID,
	).Scan(
		&tenant.ID,
		&tenant.Name,
		&tenant.Slug,
		&tenant.PlanID,
		&tenant.Status,
		&settingsJSON,
		&tenant.CreatedAt,
		&tenant.UpdatedAt,
		&tenant.DeletedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to update tenant: %w", err)
	}

	err = json.Unmarshal(settingsJSON, &tenant.Settings)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal settings: %w", err)
	}

	return tenant, nil
}

func (m *Manager) DeleteTenant(ctx context.Context, id uuid.UUID) error {
	if id == uuid.Nil {
		return ErrInvalidTenantID
	}

	// Check if tenant has any active resources
	hasResources, err := m.tenantHasActiveResources(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to check tenant resources: %w", err)
	}
	if hasResources {
		return ErrTenantCannotDelete
	}

	// Soft delete tenant
	query := `
		UPDATE tenants
		SET deleted_at = CURRENT_TIMESTAMP, status = 'deleted', updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
	`

	result, err := m.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete tenant: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return ErrTenantNotFound
	}

	return nil
}

func (m *Manager) ListTenants(ctx context.Context, opts ListTenantsOptions) ([]*Tenant, int64, error) {
	tenants := []*Tenant{}
	var total int64

	// Build query with dynamic filters
	baseQuery := "SELECT id, name, slug, plan_id, status, settings, created_at, updated_at, deleted_at FROM tenants WHERE deleted_at IS NULL"
	countQuery := "SELECT COUNT(*) FROM tenants WHERE deleted_at IS NULL"

	args := []interface{}{}
	argPos := 1

	if opts.Status != nil {
		baseQuery += fmt.Sprintf(" AND status = $%d", argPos)
		countQuery += fmt.Sprintf(" AND status = $%d", argPos)
		args = append(args, *opts.Status)
		argPos++
	}

	if opts.PlanID != nil {
		baseQuery += fmt.Sprintf(" AND plan_id = $%d", argPos)
		countQuery += fmt.Sprintf(" AND plan_id = $%d", argPos)
		args = append(args, *opts.PlanID)
		argPos++
	}

	if opts.Search != "" {
		baseQuery += fmt.Sprintf(" AND (name ILIKE $%d OR slug ILIKE $%d)", argPos, argPos+1)
		countQuery += fmt.Sprintf(" AND (name ILIKE $%d OR slug ILIKE $%d)", argPos, argPos+1)
		searchPattern := "%" + opts.Search + "%"
		args = append(args, searchPattern, searchPattern)
		argPos += 2
	}

	// Get total count
	err := m.db.QueryRowContext(ctx, countQuery, args...[:argPos-1]...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count tenants: %w", err)
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
		return nil, 0, fmt.Errorf("failed to list tenants: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var tenant Tenant
		var settingsJSON []byte

		err := rows.Scan(
			&tenant.ID,
			&tenant.Name,
			&tenant.Slug,
			&tenant.PlanID,
			&tenant.Status,
			&settingsJSON,
			&tenant.CreatedAt,
			&tenant.UpdatedAt,
			&tenant.DeletedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan tenant: %w", err)
		}

		err = json.Unmarshal(settingsJSON, &tenant.Settings)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to unmarshal settings: %w", err)
		}

		tenants = append(tenants, &tenant)
	}

	if err = rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating tenants: %w", err)
	}

	return tenants, total, nil
}

func (m *Manager) ActivateTenant(ctx context.Context, id uuid.UUID) (*Tenant, error) {
	return m.updateTenantStatus(ctx, id, TenantStatusActive)
}

func (m *Manager) SuspendTenant(ctx context.Context, id uuid.UUID) (*Tenant, error) {
	return m.updateTenantStatus(ctx, id, TenantStatusSuspended)
}

func (m *Manager) updateTenantStatus(ctx context.Context, id uuid.UUID, status TenantStatus) (*Tenant, error) {
	if id == uuid.Nil {
		return nil, ErrInvalidTenantID
	}

	if !isValidTenantStatus(status) {
		return nil, ErrInvalidTenantStatus
	}

	tenant, err := m.GetTenant(ctx, id)
	if err != nil {
		return nil, err
	}

	if tenant.Status == status {
		return tenant, nil
	}

	query := `
		UPDATE tenants
		SET status = $1, updated_at = CURRENT_TIMESTAMP
		WHERE id = $2
		RETURNING id, name, slug, plan_id, status, settings, created_at, updated_at, deleted_at
	`

	var settingsJSON []byte
	err = m.db.QueryRowContext(ctx, query, status, id).Scan(
		&tenant.ID,
		&tenant.Name,
		&tenant.Slug,
		&tenant.PlanID,
		&tenant.Status,
		&settingsJSON,
		&tenant.CreatedAt,
		&tenant.UpdatedAt,
		&tenant.DeletedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to update tenant status: %w", err)
	}

	err = json.Unmarshal(settingsJSON, &tenant.Settings)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal settings: %w", err)
	}

	return tenant, nil
}

func (m *Manager) tenantHasActiveResources(ctx context.Context, tenantID uuid.UUID) (bool, error) {
	// Check for active models
	var modelCount int
	err := m.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM models WHERE tenant_id = $1 AND deleted_at IS NULL",
		tenantID,
	).Scan(&modelCount)
	if err != nil {
		return false, err
	}

	if modelCount > 0 {
		return true, nil
	}

	// Check for active organizations
	var orgCount int
	err = m.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM organizations WHERE tenant_id = $1 AND deleted_at IS NULL",
		tenantID,
	).Scan(&orgCount)
	if err != nil {
		return false, err
	}

	return orgCount > 0, nil
}

func isValidTenantStatus(status TenantStatus) bool {
	switch status {
	case TenantStatusActive, TenantStatusSuspended, TenantStatusPending, TenantStatusDeleted:
		return true
	default:
		return false
	}
}

// ProvisionTenant provisions a new tenant with default resources
func (m *Manager) ProvisionTenant(ctx context.Context, id uuid.UUID) error {
	tenant, err := m.GetTenant(ctx, id)
	if err != nil {
		return err
	}

	// Create default organization
	orgID := uuid.New()
	_, err = m.db.ExecContext(ctx, `
		INSERT INTO organizations (id, tenant_id, name, slug, settings, created_at)
		VALUES ($1, $2, $3, $4, $5, CURRENT_TIMESTAMP)
	`, orgID, tenant.ID, "Default", "default", "{}")
	if err != nil {
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "23505" {
			// Unique violation - organization might already exist
			return nil
		}
		return fmt.Errorf("failed to create default organization: %w", err)
	}

	// Initialize usage tracking
	_, err = m.db.ExecContext(ctx, `
		INSERT INTO usage_records (tenant_id, resource_type, quantity, unit, recorded_at)
		VALUES ($1, 'storage', 0, 'bytes', CURRENT_TIMESTAMP)
	`, tenant.ID)
	if err != nil {
		return fmt.Errorf("failed to initialize usage tracking: %w", err)
	}

	return nil
}
