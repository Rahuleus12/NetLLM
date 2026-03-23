// internal/billing/plans.go
// Billing plans and subscriptions
// Handles plan management, pricing, feature configuration, and trial periods

package billing

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
	ErrPlanNotFound         = errors.New("plan not found")
	ErrPlanAlreadyExists  = errors.New("plan already exists")
	ErrInvalidPlan         = errors.New("invalid plan data")
	ErrCannotDeleteDefault = errors.New("cannot delete default plan")
	ErrCannotDeleteActive  = errors.New("cannot delete plan with active subscriptions")
)

// PlanType represents the type of billing plan
type PlanType string

const (
	PlanTypeStandard  PlanType = "standard"
	PlanTypeEnterprise PlanType = "enterprise"
	PlanTypeCustom    PlanType = "custom"
)

// PlanStatus represents the status of a plan
type PlanStatus string

const (
	PlanStatusActive   PlanStatus = "active"
	PlanStatusArchived PlanStatus = "archived"
	PlanStatusDeprecated PlanStatus = "deprecated"
)

// BillingInterval represents billing frequency
type BillingInterval string

const (
	BillingIntervalMonthly  BillingInterval = "monthly"
	BillingIntervalQuarterly BillingInterval = "quarterly"
	BillingIntervalAnnually BillingInterval = "annually"
)

// Plan represents a billing plan
type Plan struct {
	ID              uuid.UUID   `json:"id" db:"id"`
	Name            string      `json:"name" db:"name"`
	Slug            string      `json:"slug" db:"slug"`
	Description     string      `json:"description" db:"description"`
	Type            PlanType    `json:"type" db:"type"`
	Status          PlanStatus  `json:"status" db:"status"`

	// Pricing
	BasePrice       float64            `json:"base_price" db:"base_price"`
	Currency        string             `json:"currency" db:"currency"`
	Interval        BillingInterval     `json:"interval" db:"interval"`
	TrialDays       int                `json:"trial_days" db:"trial_days"`

	// Features
	Features        json.RawMessage    `json:"features" db:"features"`
	Metadata        json.RawMessage    `json:"metadata" db:"metadata"`

	// Limits
	MaxModels       int                `json:"max_models" db:"max_models"`
	MaxStorageGB   int                `json:"max_storage_gb" db:"max_storage_gb"`
	MaxAPIRequests  int                `json:"max_api_requests" db:"max_api_requests"`
	MaxGPUs         int                `json:"max_gpus" db:"max_gpus"`
	MaxUsers        int                `json:"max_users" db:"max_users"`

	// Visibility
	IsPublic        bool              `json:"is_public" db:"is_public"`
	IsDefault       bool              `json:"is_default" db:"is_default"`

	// Tier/Ranking
	Tier            string            `json:"tier" db:"tier"` // starter, basic, pro, premium, enterprise

	// Timestamps
	CreatedAt       time.Time         `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at" db:"updated_at"`
	DeletedAt       *time.Time        `json:"deleted_at,omitempty" db:"deleted_at"`
}

// PlanFeatures represents configurable features for a plan
type PlanFeatures struct {
	// Model Features
	ModelUpload         bool   `json:"model_upload"`
	ModelDownload       bool   `json:"model_download"`
	ModelHosting        bool   `json:"model_hosting"`
	ModelVersioning    bool   `json:"model_versioning"`

	// Inference Features
	InferenceAPI        bool   `json:"inference_api"`
	BatchInference     bool   `json:"batch_inference"`
	StreamingInference bool   `json:"streaming_inference"`
	GPUAcceleration     bool   `json:"gpu_acceleration"`
	AutoScaling        bool   `json:"auto_scaling"`

	// Usage Features
	UsageAnalytics      bool   `json:"usage_analytics"`
	UsageReports       bool   `json:"usage_reports"`
	CostManagement     bool   `json:"cost_management"`
	QuotaManagement     bool   `json:"quota_management"`

	// Support Features
	EmailSupport        bool   `json:"email_support"`
	ChatSupport         bool   `json:"chat_support"`
	PrioritySupport    bool   `json:"priority_support"`
	PhoneSupport       bool   `json:"phone_support"`
	DedicatedSupport   bool   `json:"dedicated_support"`

	// Integration Features
	APIAccess           bool   `json:"api_access"`
	Webhooks            bool   `json:"webhooks"`
	SSO                 bool   `json:"sso"`
	CustomDomain        bool   `json:"custom_domain"`

	// Advanced Features
	AdvancedSecurity     bool   `json:"advanced_security"`
	AuditLogs            bool   `json:"audit_logs"`
	CustomBranding      bool   `json:"custom_branding"`
	WhiteLabel          bool   `json:"white_label"`
}

// CreatePlanRequest represents a request to create a new plan
type CreatePlanRequest struct {
	Name             string            `json:"name"`
	Slug             string            `json:"slug"`
	Description      string            `json:"description"`
	Type             PlanType         `json:"type"`
	BasePrice        float64           `json:"base_price"`
	Currency         string            `json:"currency"`
	Interval         BillingInterval   `json:"interval"`
	TrialDays        int               `json:"trial_days"`
	Features         *PlanFeatures      `json:"features"`
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
	MaxModels        int               `json:"max_models"`
	MaxStorageGB     int               `json:"max_storage_gb"`
	MaxAPIRequests   int               `json:"max_api_requests"`
	MaxGPUs          int               `json:"max_gpus"`
	MaxUsers         int               `json:"max_users"`
	IsPublic         bool              `json:"is_public"`
	IsDefault        bool              `json:"is_default"`
	Tier             string            `json:"tier"`
}

// UpdatePlanRequest represents a request to update a plan
type UpdatePlanRequest struct {
	Name             *string            `json:"name,omitempty"`
	Slug             *string            `json:"slug,omitempty"`
	Description      *string            `json:"description,omitempty"`
	Type             *PlanType         `json:"type,omitempty"`
	Status           *PlanStatus        `json:"status,omitempty"`
	BasePrice        *float64           `json:"base_price,omitempty"`
	Currency         *string            `json:"currency,omitempty"`
	Interval         *BillingInterval   `json:"interval,omitempty"`
	TrialDays        *int               `json:"trial_days,omitempty"`
	Features         *PlanFeatures      `json:"features,omitempty"`
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
	MaxModels        *int               `json:"max_models,omitempty"`
	MaxStorageGB     *int               `json:"max_storage_gb,omitempty"`
	MaxAPIRequests   *int               `json:"max_api_requests,omitempty"`
	MaxGPUs          *int               `json:"max_gpus,omitempty"`
	MaxUsers         *int               `json:"max_users,omitempty"`
	IsPublic         *bool              `json:"is_public,omitempty"`
	IsDefault        *bool              `json:"is_default,omitempty"`
	Tier             *string            `json:"tier,omitempty"`
}

// ListPlansOptions represents options for listing plans
type ListPlansOptions struct {
	Type         *PlanType
	Status       *PlanStatus
	IsPublic     *bool
	IsDefault    *bool
	Tier         *string
	MinPrice     *float64
	MaxPrice     *float64
	Limit        int
	Offset       int
	Search       string
}

// PlanManager manages billing plans
type PlanManager struct {
	db *sql.DB
}

// NewPlanManager creates a new plan manager
func NewPlanManager(db *sql.DB) *PlanManager {
	return &PlanManager{
		db: db,
	}
}

// CreatePlan creates a new billing plan
func (pm *PlanManager) CreatePlan(ctx context.Context, req CreatePlanRequest) (*Plan, error) {
	if req.Name == "" {
		return nil, ErrInvalidPlan
	}
	if req.Slug == "" {
		return nil, ErrInvalidPlan
	}
	if req.BasePrice <= 0 {
		return nil, errors.New("base_price must be positive")
	}

	// Check if plan with slug already exists
	existing, err := pm.GetPlanBySlug(ctx, req.Slug)
	if err == nil && existing != nil {
		return nil, ErrPlanAlreadyExists
	}

	// Serialize features
	featuresJSON := []byte("{}")
	if req.Features != nil {
		featuresJSON, err = json.Marshal(req.Features)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal features: %w", err)
		}
	}

	// Serialize metadata
	metadataJSON := []byte("{}")
	if req.Metadata != nil {
		metadataJSON, err = json.Marshal(req.Metadata)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal metadata: %w", err)
		}
	}

	// Set default values
	if req.Currency == "" {
		req.Currency = "USD"
	}
	if req.Interval == "" {
		req.Interval = BillingIntervalMonthly
	}
	if req.MaxModels <= 0 {
		req.MaxModels = 50
	}
	if req.MaxStorageGB <= 0 {
		req.MaxStorageGB = 100
	}
	if req.MaxAPIRequests <= 0 {
		req.MaxAPIRequests = 100000
	}
	if req.MaxUsers <= 0 {
		req.MaxUsers = 10
	}
	if req.Tier == "" {
		req.Tier = "basic"
	}

	plan := &Plan{
		ID:           uuid.New(),
		Name:         req.Name,
		Slug:         req.Slug,
		Description:  req.Description,
		Type:         req.Type,
		Status:       PlanStatusActive,
		BasePrice:    req.BasePrice,
		Currency:     req.Currency,
		Interval:     req.Interval,
		TrialDays:    req.TrialDays,
		Features:     featuresJSON,
		Metadata:     metadataJSON,
		MaxModels:    req.MaxModels,
		MaxStorageGB: req.MaxStorageGB,
		MaxAPIRequests: req.MaxAPIRequests,
		MaxGPUs:      req.MaxGPUs,
		MaxUsers:     req.MaxUsers,
		IsPublic:     req.IsPublic,
		IsDefault:    req.IsDefault,
		Tier:         req.Tier,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	query := `
		INSERT INTO plans (id, name, slug, description, type, status,
			base_price, currency, interval, trial_days, features, metadata,
			max_models, max_storage_gb, max_api_requests, max_gpus, max_users,
			is_public, is_default, tier, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)
		RETURNING id, name, slug, description, type, status, base_price, currency,
			interval, trial_days, features, metadata, max_models, max_storage_gb,
			max_api_requests, max_gpus, max_users, is_public, is_default, tier,
			created_at, updated_at, deleted_at
	`

	err = pm.db.QueryRowContext(ctx, query,
		plan.ID,
		plan.Name,
		plan.Slug,
		plan.Description,
		plan.Type,
		plan.Status,
		plan.BasePrice,
		plan.Currency,
		plan.Interval,
		plan.TrialDays,
		plan.Features,
		plan.Metadata,
		plan.MaxModels,
		plan.MaxStorageGB,
		plan.MaxAPIRequests,
		plan.MaxGPUs,
		plan.MaxUsers,
		plan.IsPublic,
		plan.IsDefault,
		plan.Tier,
		plan.CreatedAt,
		plan.UpdatedAt,
	).Scan(
		&plan.ID,
		&plan.Name,
		&plan.Slug,
		&plan.Description,
		&plan.Type,
		&plan.Status,
		&plan.BasePrice,
		&plan.Currency,
		&plan.Interval,
		&plan.TrialDays,
		&plan.Features,
		&plan.Metadata,
		&plan.MaxModels,
		&plan.MaxStorageGB,
		&plan.MaxAPIRequests,
		&plan.MaxGPUs,
		&plan.MaxUsers,
		&plan.IsPublic,
		&plan.IsDefault,
		&plan.Tier,
		&plan.CreatedAt,
		&plan.UpdatedAt,
		&plan.DeletedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create plan: %w", err)
	}

	return plan, nil
}

// GetPlan retrieves a plan by ID
func (pm *PlanManager) GetPlan(ctx context.Context, id uuid.UUID) (*Plan, error) {
	if id == uuid.Nil {
		return nil, ErrInvalidPlan
	}

	var plan Plan

	query := `
		SELECT id, name, slug, description, type, status, base_price, currency,
			interval, trial_days, features, metadata, max_models, max_storage_gb,
			max_api_requests, max_gpus, max_users, is_public, is_default, tier,
			created_at, updated_at, deleted_at
		FROM plans
		WHERE id = $1 AND deleted_at IS NULL
	`

	err := pm.db.QueryRowContext(ctx, query, id).Scan(
		&plan.ID,
		&plan.Name,
		&plan.Slug,
		&plan.Description,
		&plan.Type,
		&plan.Status,
		&plan.BasePrice,
		&plan.Currency,
		&plan.Interval,
		&plan.TrialDays,
		&plan.Features,
		&plan.Metadata,
		&plan.MaxModels,
		&plan.MaxStorageGB,
		&plan.MaxAPIRequests,
		&plan.MaxGPUs,
		&plan.MaxUsers,
		&plan.IsPublic,
		&plan.IsDefault,
		&plan.Tier,
		&plan.CreatedAt,
		&plan.UpdatedAt,
		&plan.DeletedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrPlanNotFound
		}
		return nil, fmt.Errorf("failed to get plan: %w", err)
	}

	return &plan, nil
}

// GetPlanBySlug retrieves a plan by slug
func (pm *PlanManager) GetPlanBySlug(ctx context.Context, slug string) (*Plan, error) {
	if slug == "" {
		return nil, ErrInvalidPlan
	}

	var plan Plan

	query := `
		SELECT id, name, slug, description, type, status, base_price, currency,
			interval, trial_days, features, metadata, max_models, max_storage_gb,
			max_api_requests, max_gpus, max_users, is_public, is_default, tier,
			created_at, updated_at, deleted_at
		FROM plans
		WHERE slug = $1 AND deleted_at IS NULL
	`

	err := pm.db.QueryRowContext(ctx, query, slug).Scan(
		&plan.ID,
		&plan.Name,
		&plan.Slug,
		&plan.Description,
		&plan.Type,
		&plan.Status,
		&plan.BasePrice,
		&plan.Currency,
		&plan.Interval,
		&plan.TrialDays,
		&plan.Features,
		&plan.Metadata,
		&plan.MaxModels,
		&plan.MaxStorageGB,
		&plan.MaxAPIRequests,
		&plan.MaxGPUs,
		&plan.MaxUsers,
		&plan.IsPublic,
		&plan.IsDefault,
		&plan.Tier,
		&plan.CreatedAt,
		&plan.UpdatedAt,
		&plan.DeletedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrPlanNotFound
		}
		return nil, fmt.Errorf("failed to get plan by slug: %w", err)
	}

	return &plan, nil
}

// GetDefaultPlan retrieves the default plan
func (pm *PlanManager) GetDefaultPlan(ctx context.Context) (*Plan, error) {
	var plan Plan

	query := `
		SELECT id, name, slug, description, type, status, base_price, currency,
			interval, trial_days, features, metadata, max_models, max_storage_gb,
			max_api_requests, max_gpus, max_users, is_public, is_default, tier,
			created_at, updated_at, deleted_at
		FROM plans
		WHERE is_default = true AND deleted_at IS NULL AND status = 'active'
		ORDER BY created_at DESC
		LIMIT 1
	`

	err := pm.db.QueryRowContext(ctx, query).Scan(
		&plan.ID,
		&plan.Name,
		&plan.Slug,
		&plan.Description,
		&plan.Type,
		&plan.Status,
		&plan.BasePrice,
		&plan.Currency,
		&plan.Interval,
		&plan.TrialDays,
		&plan.Features,
		&plan.Metadata,
		&plan.MaxModels,
		&plan.MaxStorageGB,
		&plan.MaxAPIRequests,
		&plan.MaxGPUs,
		&plan.MaxUsers,
		&plan.IsPublic,
		&plan.IsDefault,
		&plan.Tier,
		&plan.CreatedAt,
		&plan.UpdatedAt,
		&plan.DeletedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrPlanNotFound
		}
		return nil, fmt.Errorf("failed to get default plan: %w", err)
	}

	return &plan, nil
}

// UpdatePlan updates a plan
func (pm *PlanManager) UpdatePlan(ctx context.Context, id uuid.UUID, req UpdatePlanRequest) (*Plan, error) {
	if id == uuid.Nil {
		return nil, ErrInvalidPlan
	}

	plan, err := pm.GetPlan(ctx, id)
	if err != nil {
		return nil, err
	}

	// Apply updates
	if req.Name != nil {
		plan.Name = *req.Name
	}
	if req.Slug != nil {
		plan.Slug = *req.Slug
	}
	if req.Description != nil {
		plan.Description = *req.Description
	}
	if req.Type != nil {
		plan.Type = *req.Type
	}
	if req.Status != nil {
		if !isValidPlanStatus(*req.Status) {
			return nil, errors.New("invalid plan status")
		}
		plan.Status = *req.Status
	}
	if req.BasePrice != nil {
		if *req.BasePrice <= 0 {
			return nil, errors.New("base_price must be positive")
		}
		plan.BasePrice = *req.BasePrice
	}
	if req.Currency != nil {
		plan.Currency = *req.Currency
	}
	if req.Interval != nil {
		plan.Interval = *req.Interval
	}
	if req.TrialDays != nil {
		plan.TrialDays = *req.TrialDays
	}
	if req.Features != nil {
		featuresJSON, err := json.Marshal(req.Features)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal features: %w", err)
		}
		plan.Features = featuresJSON
	}
	if req.Metadata != nil {
		metadataJSON, err := json.Marshal(req.Metadata)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal metadata: %w", err)
		}
		plan.Metadata = metadataJSON
	}
	if req.MaxModels != nil {
		plan.MaxModels = *req.MaxModels
	}
	if req.MaxStorageGB != nil {
		plan.MaxStorageGB = *req.MaxStorageGB
	}
	if req.MaxAPIRequests != nil {
		plan.MaxAPIRequests = *req.MaxAPIRequests
	}
	if req.MaxGPUs != nil {
		plan.MaxGPUs = *req.MaxGPUs
	}
	if req.MaxUsers != nil {
		plan.MaxUsers = *req.MaxUsers
	}
	if req.IsPublic != nil {
		plan.IsPublic = *req.IsPublic
	}
	if req.IsDefault != nil {
		plan.IsDefault = *req.IsDefault
	}
	if req.Tier != nil {
		plan.Tier = *req.Tier
	}
	plan.UpdatedAt = time.Now()

	query := `
		UPDATE plans
		SET name = $1, slug = $2, description = $3, type = $4, status = $5,
			base_price = $6, currency = $7, interval = $8, trial_days = $9,
			features = $10, metadata = $11, max_models = $12, max_storage_gb = $13,
			max_api_requests = $14, max_gpus = $15, max_users = $16,
			is_public = $17, is_default = $18, tier = $19, updated_at = $20
		WHERE id = $21
		RETURNING id, name, slug, description, type, status, base_price, currency,
			interval, trial_days, features, metadata, max_models, max_storage_gb,
			max_api_requests, max_gpus, max_users, is_public, is_default, tier,
			created_at, updated_at, deleted_at
	`

	err = pm.db.QueryRowContext(ctx, query,
		plan.Name,
		plan.Slug,
		plan.Description,
		plan.Type,
		plan.Status,
		plan.BasePrice,
		plan.Currency,
		plan.Interval,
		plan.TrialDays,
		plan.Features,
		plan.Metadata,
		plan.MaxModels,
		plan.MaxStorageGB,
		plan.MaxAPIRequests,
		plan.MaxGPUs,
		plan.MaxUsers,
		plan.IsPublic,
		plan.IsDefault,
		plan.Tier,
		plan.UpdatedAt,
		plan.ID,
	).Scan(
		&plan.ID,
		&plan.Name,
		&plan.Slug,
		&plan.Description,
		&plan.Type,
		&plan.Status,
		&plan.BasePrice,
		&plan.Currency,
		&plan.Interval,
		&plan.TrialDays,
		&plan.Features,
		&plan.Metadata,
		&plan.MaxModels,
		&plan.MaxStorageGB,
		&plan.MaxAPIRequests,
		&plan.MaxGPUs,
		&plan.MaxUsers,
		&plan.IsPublic,
		&plan.IsDefault,
		&plan.Tier,
		&plan.CreatedAt,
		&plan.UpdatedAt,
		&plan.DeletedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to update plan: %w", err)
	}

	return &plan, nil
}

// DeletePlan deletes a plan
func (pm *PlanManager) DeletePlan(ctx context.Context, id uuid.UUID) error {
	if id == uuid.Nil {
		return ErrInvalidPlan
	}

	// Check if it's the default plan
	plan, err := pm.GetPlan(ctx, id)
	if err != nil {
		return err
	}

	if plan.IsDefault {
		return ErrCannotDeleteDefault
	}

	// Check if there are active subscriptions
	hasActiveSubscriptions, err := pm.planHasActiveSubscriptions(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to check for active subscriptions: %w", err)
	}
	if hasActiveSubscriptions {
		return ErrCannotDeleteActive
	}

	// Soft delete
	query := `
		UPDATE plans
		SET deleted_at = CURRENT_TIMESTAMP, status = 'deleted', updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
	`

	result, err := pm.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete plan: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return ErrPlanNotFound
	}

	return nil
}

// ListPlans lists plans with optional filters
func (pm *PlanManager) ListPlans(ctx context.Context, opts ListPlansOptions) ([]*Plan, int64, error) {
	plans := []*Plan{}
	var total int64

	// Build query with dynamic filters
	baseQuery := `
		SELECT id, name, slug, description, type, status, base_price, currency,
			interval, trial_days, features, metadata, max_models, max_storage_gb,
			max_api_requests, max_gpus, max_users, is_public, is_default, tier,
			created_at, updated_at, deleted_at
		FROM plans
		WHERE deleted_at IS NULL
	`
	countQuery := `SELECT COUNT(*) FROM plans WHERE deleted_at IS NULL`

	args := []interface{}{}
	argPos := 1

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

	if opts.MinPrice != nil {
		baseQuery += fmt.Sprintf(" AND base_price >= $%d", argPos)
		countQuery += fmt.Sprintf(" AND base_price >= $%d", argPos)
		args = append(args, *opts.MinPrice)
		argPos++
	}

	if opts.MaxPrice != nil {
		baseQuery += fmt.Sprintf(" AND base_price <= $%d", argPos)
		countQuery += fmt.Sprintf(" AND base_price <= $%d", argPos)
		args = append(args, *opts.MaxPrice)
		argPos++
	}

	if opts.Tier != nil {
		baseQuery += fmt.Sprintf(" AND tier = $%d", argPos)
		countQuery += fmt.Sprintf(" AND tier = $%d", argPos)
		args = append(args, *opts.Tier)
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
	err := pm.db.QueryRowContext(ctx, countQuery, args...[:argPos-1]...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count plans: %w", err)
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

	baseQuery += " ORDER BY tier ASC, base_price ASC"

	rows, err := pm.db.QueryContext(ctx, baseQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list plans: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var plan Plan

		err := rows.Scan(
			&plan.ID,
			&plan.Name,
			&plan.Slug,
			&plan.Description,
			&plan.Type,
			&plan.Status,
			&plan.BasePrice,
			&plan.Currency,
			&plan.Interval,
			&plan.TrialDays,
			&plan.Features,
			&plan.Metadata,
			&plan.MaxModels,
			&plan.MaxStorageGB,
			&plan.MaxAPIRequests,
			&plan.MaxGPUs,
			&plan.MaxUsers,
			&plan.IsPublic,
			&plan.IsDefault,
			&plan.Tier,
			&plan.CreatedAt,
			&plan.UpdatedAt,
			&plan.DeletedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan plan: %w", err)
		}

		plans = append(plans, &plan)
	}

	if err = rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating plans: %w", err)
	}

	return plans, total, nil
}

// GetPublicPlans retrieves all publicly available plans
func (pm *PlanManager) GetPublicPlans(ctx context.Context) ([]*Plan, error) {
	plans := []*Plan{}

	query := `
		SELECT id, name, slug, description, type, status, base_price, currency,
			interval, trial_days, features, metadata, max_models, max_storage_gb,
			max_api_requests, max_gpus, max_users, is_public, is_default, tier,
			created_at, updated_at, deleted_at
		FROM plans
		WHERE is_public = true AND deleted_at IS NULL AND status = 'active'
		ORDER BY base_price ASC
	`

	rows, err := pm.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get public plans: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var plan Plan

		err := rows.Scan(
			&plan.ID,
			&plan.Name,
			&plan.Slug,
			&plan.Description,
			&plan.Type,
			&plan.Status,
			&plan.BasePrice,
			&plan.Currency,
			&plan.Interval,
			&plan.TrialDays,
			&plan.Features,
			&plan.Metadata,
			&plan.MaxModels,
			&plan.MaxStorageGB,
			&plan.MaxAPIRequests,
			&plan.MaxGPUs,
			&plan.MaxUsers,
			&plan.IsPublic,
			&plan.IsDefault,
			&plan.Tier,
			&plan.CreatedAt,
			&plan.UpdatedAt,
			&plan.DeletedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan plan: %w", err)
		}

		plans = append(plans, &plan)
	}

	return plans, nil
}

// GetPlanFeatures retrieves features for a plan
func (pm *PlanManager) GetPlanFeatures(ctx context.Context, id uuid.UUID) (*PlanFeatures, error) {
	plan, err := pm.GetPlan(ctx, id)
	if err != nil {
		return nil, err
	}

	var features PlanFeatures
	if plan.Features != nil {
		err = json.Unmarshal(plan.Features, &features)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal features: %w", err)
		}
	}

	return &features, nil
}

// ComparePlans compares features and pricing between plans
func (pm *PlanManager) ComparePlans(ctx context.Context, planIDs []uuid.UUID) ([]map[string]interface{}, error) {
	if len(planIDs) == 0 {
		return nil, errors.New("no plan IDs provided")
	}

	plans := make([]*Plan, 0, len(planIDs))
	for i, id := range planIDs {
		plan, err := pm.GetPlan(ctx, id)
		if err != nil {
			return nil, err
		}
		plans[i] = plan
	}

	comparisons := []map[string]interface{}{}
	for i, planA := range plans {
		for j, planB := range plans {
			if i >= j {
				continue
			}

			comparison := map[string]interface{}{
				"plan_a_id":      planA.ID.String(),
				"plan_b_id":      planB.ID.String(),
				"plan_a_name":    planA.Name,
				"plan_b_name":    planB.Name,
				"plan_a_price":   planA.BasePrice,
				"plan_b_price":   planB.BasePrice,
				"price_diff":     planB.BasePrice - planA.BasePrice,
				"price_percent_diff": ((planB.BasePrice - planA.BasePrice) / planA.BasePrice) * 100,
			}

			// Compare features
			var featuresA, featuresB PlanFeatures
			json.Unmarshal(planA.Features, &featuresA)
			json.Unmarshal(planB.Features, &featuresB)

			featureComparison := map[string]bool{
				"model_upload":         featuresB.ModelUpload >= featuresA.ModelUpload,
				"model_download":       featuresB.ModelDownload >= featuresA.ModelDownload,
				"model_hosting":        featuresB.ModelHosting >= featuresA.ModelHosting,
				"model_versioning":     featuresB.ModelVersioning >= featuresA.ModelVersioning,
				"inference_api":        featuresB.InferenceAPI >= featuresA.InferenceAPI,
				"batch_inference":      featuresB.BatchInference >= featuresA.BatchInference,
				"streaming_inference": featuresB.StreamingInference >= featuresA.StreamingInference,
				"gpu_acceleration":     featuresB.GPUAcceleration >= featuresA.GPUAcceleration,
				"auto_scaling":         featuresB.AutoScaling >= featuresA.AutoScaling,
				"usage_analytics":      featuresB.UsageAnalytics >= featuresA.UsageAnalytics,
				"usage_reports":        featuresB.UsageReports >= featuresA.UsageReports,
				"cost_management":      featuresB.CostManagement >= featuresA.CostManagement,
				"quota_management":      featuresB.QuotaManagement >= featuresA.QuotaManagement,
				"email_support":        featuresB.EmailSupport >= featuresA.EmailSupport,
				"chat_support":         featuresB.ChatSupport >= featuresA.ChatSupport,
				"priority_support":     featuresB.PrioritySupport >= featuresA.PrioritySupport,
				"phone_support":       featuresB.PhoneSupport >= featuresA.PhoneSupport,
				"dedicated_support":    featuresB.DedicatedSupport >= featuresA.DedicatedSupport,
				"api_access":           featuresB.APIAccess >= featuresA.APIAccess,
				"webhooks":            featuresB.Webhooks >= featuresA.Webhooks,
				"sso":                 featuresB.SSO >= featuresA.SSO,
				"custom_domain":        featuresB.CustomDomain >= featuresA.CustomDomain,
				"advanced_security":    featuresB.AdvancedSecurity >= featuresA.AdvancedSecurity,
				"audit_logs":          featuresB.AuditLogs >= featuresA.AuditLogs,
				"custom_branding":     featuresB.CustomBranding >= featuresA.CustomBranding,
				"white_label":         featuresB.WhiteLabel >= featuresA.WhiteLabel,
			}

			comparison["feature_comparison"] = featureComparison

			// Score comparison
			featuresAScore := pm.calculateFeatureScore(featuresA)
			featuresBScore := pm.calculateFeatureScore(featuresB)
			comparison["score_difference"] = featuresBScore - featuresAScore

			comparisons = append(comparisons, comparison)
		}
	}

	return comparisons, nil
}

// CalculateMonthlyPrice calculates monthly price for plans with different intervals
func (pm *PlanManager) CalculateMonthlyPrice(ctx context.Context, id uuid.UUID) (float64, error) {
	plan, err := pm.GetPlan(ctx, id)
	if err != nil {
		return 0, err
	}

	switch plan.Interval {
	case BillingIntervalMonthly:
		return plan.BasePrice, nil
	case BillingIntervalQuarterly:
		return plan.BasePrice / 3, nil
	case BillingIntervalAnnually:
		return plan.BasePrice / 12, nil
	default:
		return 0, errors.New("invalid billing interval")
	}
}

// planHasActiveSubscriptions checks if a plan has active subscriptions
func (pm *PlanManager) planHasActiveSubscriptions(ctx context.Context, id uuid.UUID) (bool, error) {
	if id == uuid.Nil {
		return false, ErrInvalidPlan
	}

	var count int

	query := `
		SELECT COUNT(*)
		FROM subscriptions
		WHERE plan_id = $1 AND status = 'active' AND deleted_at IS NULL
	`

	err := pm.db.QueryRowContext(ctx, query, id).Scan(&count)
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// SetDefaultPlan sets a plan as the default
func (pm *PlanManager) SetDefaultPlan(ctx context.Context, id uuid.UUID) error {
	if id == uuid.Nil {
		return ErrInvalidPlan
	}

	plan, err := pm.GetPlan(ctx, id)
	if err != nil {
		return err
	}

	// Remove default from all other plans
	_, err = pm.db.ExecContext(ctx, `
		UPDATE plans SET is_default = false WHERE is_default = true AND deleted_at IS NULL
	`)
	if err != nil {
		return fmt.Errorf("failed to clear default plans: %w", err)
	}

	// Set new default
	query := `
		UPDATE plans
		SET is_default = true, updated_at = CURRENT_TIMESTAMP
		WHERE id = $1 AND deleted_at IS NULL
	`

	_, err = pm.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to set default plan: %w", err)
	}

	return nil
}

// calculateFeatureScore calculates a numeric score for plan features
func (pm *PlanManager) calculateFeatureScore(features PlanFeatures) int {
	score := 0

	if features.ModelUpload {
		score += 10
	}
	if features.ModelDownload {
		score += 10
	}
	if features.ModelHosting {
		score += 20
	}
	if features.ModelVersioning {
		score += 10
	}
	if features.InferenceAPI {
		score += 30
	}
	if features.BatchInference {
		score += 20
	}
	if features.StreamingInference {
		score += 25
	}
	if features.GPUAcceleration {
		score += 30
	}
	if features.AutoScaling {
		score += 25
	}
	if features.UsageAnalytics {
		score += 15
	}
	if features.UsageReports {
		score += 15
	}
	if features.CostManagement {
		score += 15
	}
	if features.QuotaManagement {
		score += 15
	}
	if features.EmailSupport {
		score += 5
	}
	if features.ChatSupport {
		score += 10
	}
	if features.PrioritySupport {
		score += 15
	}
	if features.PhoneSupport {
		score += 20
	}
	if features.DedicatedSupport {
		score += 50
	}
	if features.APIAccess {
		score += 20
	}
	if features.Webhooks {
		score += 15
	}
	if features.SSO {
		score += 15
	}
	if features.CustomDomain {
		score += 10
	}
	if features.AdvancedSecurity {
		score += 20
	}
	if features.AuditLogs {
		score += 10
	}
	if features.CustomBranding {
		score += 15
	}
	if features.WhiteLabel {
		score += 25
	}

	return score
}

// isValidPlanStatus checks if a plan status is valid
func isValidPlanStatus(status PlanStatus) bool {
	switch status {
	case PlanStatusActive, PlanStatusArchived, PlanStatusDeprecated:
		return true
	default:
		return false
	}
}

// CreateDefaultPlan creates a default plan for new installations
func (pm *PlanManager) CreateDefaultPlan(ctx context.Context) (*Plan, error) {
	req := CreatePlanRequest{
		Name:            "Free Tier",
		Slug:            "free",
		Description:     "Free tier for getting started",
		Type:            PlanTypeStandard,
		BasePrice:       0,
		Currency:         "USD",
		Interval:        BillingIntervalMonthly,
		TrialDays:       0,
		Features:        &PlanFeatures{
			ModelUpload:         false,
			ModelDownload:       true,
			ModelHosting:        false,
			ModelVersioning:     false,
			InferenceAPI:        true,
			BatchInference:     false,
			StreamingInference: false,
			GPUAcceleration:     false,
			AutoScaling:         false,
			UsageAnalytics:      false,
			UsageReports:       false,
			CostManagement:     false,
			QuotaManagement:     true,
			EmailSupport:        false,
			ChatSupport:         false,
			PrioritySupport:     false,
			PhoneSupport:       false,
			DedicatedSupport:   false,
			APIAccess:           true,
			Webhooks:            false,
			SSO:                 false,
			CustomDomain:        false,
			AdvancedSecurity:     false,
			AuditLogs:            true,
			CustomBranding:      false,
			WhiteLabel:          false,
		},
		MaxModels:        1,
		MaxStorageGB:     1,
		MaxAPIRequests:   1000,
		MaxGPUs:          0,
		MaxUsers:         1,
		IsPublic:         true,
		IsDefault:        true,
		Tier:             "starter",
	}

	return pm.CreatePlan(ctx, req)
}
