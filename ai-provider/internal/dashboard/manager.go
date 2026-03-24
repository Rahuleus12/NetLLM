package dashboard

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var (
	ErrDashboardNotFound      = errors.New("dashboard not found")
	ErrDashboardAlreadyExists = errors.New("dashboard already exists")
	ErrInvalidDashboard       = errors.New("invalid dashboard configuration")
	ErrWidgetNotFound         = errors.New("widget not found")
	ErrUnauthorizedAccess     = errors.New("unauthorized dashboard access")
	ErrInvalidTimeRange       = errors.New("invalid time range")
	ErrSharingFailed          = errors.New("failed to share dashboard")
)

type DashboardStatus string

const (
	StatusActive    DashboardStatus = "active"
	StatusArchived  DashboardStatus = "archived"
	StatusDraft     DashboardStatus = "draft"
	StatusDeleted   DashboardStatus = "deleted"
)

type DashboardType string

const (
	TypeSystem    DashboardType = "system"
	TypeCustom    DashboardType = "custom"
	TypeTemplate  DashboardType = "template"
)

type RefreshInterval string

const (
	Refresh5s    RefreshInterval = "5s"
	Refresh10s   RefreshInterval = "10s"
	Refresh30s   RefreshInterval = "30s"
	Refresh1m    RefreshInterval = "1m"
	Refresh5m    RefreshInterval = "5m"
	Refresh15m   RefreshInterval = "15m"
	Refresh30m   RefreshInterval = "30m"
	Refresh1h    RefreshInterval = "1h"
	RefreshManual RefreshInterval = "manual"
)

type Dashboard struct {
	ID          uuid.UUID       `json:"id" gorm:"type:uuid;primary_key;default:gen_random_uuid()"`
	TenantID    uuid.UUID       `json:"tenant_id" gorm:"type:uuid;not null;index"`
	OrgID       uuid.UUID       `json:"org_id" gorm:"type:uuid;not null;index"`
	WorkspaceID *uuid.UUID      `json:"workspace_id,omitempty" gorm:"type:uuid;index"`
	Name        string          `json:"name" gorm:"not null;index"`
	Slug        string          `json:"slug" gorm:"unique;not null;index"`
	Description string          `json:"description"`
	Type        DashboardType   `json:"type" gorm:"not null;default:'custom'"`
	Status      DashboardStatus `json:"status" gorm:"not null;default:'active'"`
	Config      DashboardConfig `json:"config" gorm:"type:jsonb"`
	Layout      DashboardLayout `json:"layout" gorm:"type:jsonb"`
	Settings    DashboardSettings `json:"settings" gorm:"type:jsonb"`
	Tags        []string        `json:"tags" gorm:"type:text[]"`
	OwnerID     uuid.UUID       `json:"owner_id" gorm:"type:uuid;not null;index"`
	CreatedBy   uuid.UUID       `json:"created_by" gorm:"type:uuid;not null"`
	UpdatedBy   uuid.UUID       `json:"updated_by" gorm:"type:uuid"`
	CreatedAt   time.Time       `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt   time.Time       `json:"updated_at" gorm:"autoUpdateTime"`
	DeletedAt   gorm.DeletedAt  `json:"deleted_at,omitempty" gorm:"index"`
	Version     int             `json:"version" gorm:"not null;default:1"`
	IsPublic    bool            `json:"is_public" gorm:"default:false"`
	IsDefault   bool            `json:"is_default" gorm:"default:false"`
}

type DashboardConfig struct {
	TimeRange       TimeRangeConfig   `json:"time_range"`
	RefreshInterval RefreshInterval   `json:"refresh_interval"`
	Theme           string            `json:"theme"`
	Timezone        string            `json:"timezone"`
	Variables       []DashboardVariable `json:"variables"`
	Annotations     []Annotation      `json:"annotations"`
}

type TimeRangeConfig struct {
	Type      string    `json:"type"` // "relative" or "absolute"
	From      time.Time `json:"from,omitempty"`
	To        time.Time `json:"to,omitempty"`
	Relative  string    `json:"relative,omitempty"` // "last1h", "last24h", "last7d", etc.
}

type DashboardVariable struct {
	Name         string      `json:"name"`
	Type         string      `json:"type"` // "query", "constant", "textbox", "custom"
	Label        string      `json:"label"`
	Default      interface{} `json:"default"`
	Options      []string    `json:"options,omitempty"`
	Query        string      `json:"query,omitempty"`
	Refresh      string      `json:"refresh"` // "never", "on_dashboard_load", "on_time_range_change"
	MultiSelect  bool        `json:"multi_select"`
	IncludeAll   bool        `json:"include_all"`
	Description  string      `json:"description"`
}

type Annotation struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Enabled     bool      `json:"enabled"`
	Datasource  string    `json:"datasource"`
	Query       string    `json:"query"`
	Color       string    `json:"color"`
	Icon        string    `json:"icon"`
	ShowLine    bool      `json:"show_line"`
	TimeFormat  string    `json:"time_format"`
}

type DashboardLayout struct {
	Type       string         `json:"type"` // "grid", "flex", "absolute"
	Columns    int            `json:"columns"`
	RowHeight  int            `json:"row_height"`
	Gap        int            `json:"gap"`
	Widgets    []WidgetLayout `json:"widgets"`
	Responsive bool           `json:"responsive"`
}

type WidgetLayout struct {
	WidgetID uuid.UUID `json:"widget_id"`
	X        int       `json:"x"`
	Y        int       `json:"y"`
	Width    int       `json:"width"`
	Height   int       `json:"height"`
	MinW     int       `json:"min_w,omitempty"`
	MinH     int       `json:"min_h,omitempty"`
	MaxW     int       `json:"max_w,omitempty"`
	MaxH     int       `json:"max_h,omitempty"`
	Static   bool      `json:"static"`
}

type DashboardSettings struct {
	Editable        bool   `json:"editable"`
	HideControls    bool   `json:"hide_controls"`
	AutoFit         bool   `json:"auto_fit"`
	ShowTimePicker  bool   `json:"show_time_picker"`
	ShowRefresh     bool   `json:"show_refresh"`
	ShowVariables   bool   `json:"show_variables"`
	ShowAnnotations bool   `json:"show_annotations"`
	TitleSize       string `json:"title_size"`
	DescriptionSize string `json:"description_size"`
}

type CreateDashboardRequest struct {
	TenantID    uuid.UUID       `json:"tenant_id" binding:"required"`
	OrgID       uuid.UUID       `json:"org_id" binding:"required"`
	WorkspaceID *uuid.UUID      `json:"workspace_id,omitempty"`
	Name        string          `json:"name" binding:"required"`
	Slug        string          `json:"slug" binding:"required"`
	Description string          `json:"description"`
	Type        DashboardType   `json:"type"`
	Config      DashboardConfig `json:"config"`
	Layout      DashboardLayout `json:"layout"`
	Settings    DashboardSettings `json:"settings"`
	Tags        []string        `json:"tags"`
	OwnerID     uuid.UUID       `json:"owner_id" binding:"required"`
	IsPublic    bool            `json:"is_public"`
	IsDefault   bool            `json:"is_default"`
}

type UpdateDashboardRequest struct {
	Name        *string           `json:"name,omitempty"`
	Description *string           `json:"description,omitempty"`
	Status      *DashboardStatus  `json:"status,omitempty"`
	Config      *DashboardConfig  `json:"config,omitempty"`
	Layout      *DashboardLayout  `json:"layout,omitempty"`
	Settings    *DashboardSettings `json:"settings,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	IsPublic    *bool             `json:"is_public,omitempty"`
	IsDefault   *bool             `json:"is_default,omitempty"`
	UpdatedBy   uuid.UUID         `json:"updated_by" binding:"required"`
}

type ListDashboardsOptions struct {
	TenantID    uuid.UUID
	OrgID       *uuid.UUID
	WorkspaceID *uuid.UUID
	Type        *DashboardType
	Status      *DashboardStatus
	OwnerID     *uuid.UUID
	Tags        []string
	IsPublic    *bool
	IsDefault   *bool
	Search      string
	Limit       int
	Offset      int
	OrderBy     string
	OrderDir    string
}

type DashboardSummary struct {
	ID          uuid.UUID       `json:"id"`
	Name        string          `json:"name"`
	Slug        string          `json:"slug"`
	Description string          `json:"description"`
	Type        DashboardType   `json:"type"`
	Status      DashboardStatus `json:"status"`
	OwnerID     uuid.UUID       `json:"owner_id"`
	WidgetCount int             `json:"widget_count"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	IsPublic    bool            `json:"is_public"`
	IsDefault   bool            `json:"is_default"`
}

type Manager struct {
	db *gorm.DB
}

func NewManager(db *gorm.DB) *Manager {
	return &Manager{db: db}
}

func (m *Manager) CreateDashboard(ctx context.Context, req *CreateDashboardRequest) (*Dashboard, error) {
	if err := m.validateDashboardRequest(req); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidDashboard, err)
	}

	dashboard := &Dashboard{
		TenantID:    req.TenantID,
		OrgID:       req.OrgID,
		WorkspaceID: req.WorkspaceID,
		Name:        req.Name,
		Slug:        req.Slug,
		Description: req.Description,
		Type:        req.Type,
		Status:      StatusDraft,
		Config:      req.Config,
		Layout:      req.Layout,
		Settings:    req.Settings,
		Tags:        req.Tags,
		OwnerID:     req.OwnerID,
		CreatedBy:   req.OwnerID,
		UpdatedBy:   req.OwnerID,
		IsPublic:    req.IsPublic,
		IsDefault:   req.IsDefault,
		Version:     1,
	}

	if dashboard.Type == "" {
		dashboard.Type = TypeCustom
	}

	if dashboard.Config.RefreshInterval == "" {
		dashboard.Config.RefreshInterval = Refresh30s
	}

	if dashboard.Config.Timezone == "" {
		dashboard.Config.Timezone = "UTC"
	}

	if dashboard.Config.Theme == "" {
		dashboard.Config.Theme = "light"
	}

	if err := m.db.WithContext(ctx).Create(dashboard).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return nil, ErrDashboardAlreadyExists
		}
		return nil, fmt.Errorf("failed to create dashboard: %w", err)
	}

	return dashboard, nil
}

func (m *Manager) GetDashboard(ctx context.Context, id uuid.UUID) (*Dashboard, error) {
	var dashboard Dashboard
	err := m.db.WithContext(ctx).
		Where("id = ?", id).
		Where("status != ?", StatusDeleted).
		First(&dashboard).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrDashboardNotFound
		}
		return nil, fmt.Errorf("failed to get dashboard: %w", err)
	}

	return &dashboard, nil
}

func (m *Manager) GetDashboardBySlug(ctx context.Context, tenantID uuid.UUID, slug string) (*Dashboard, error) {
	var dashboard Dashboard
	err := m.db.WithContext(ctx).
		Where("tenant_id = ? AND slug = ?", tenantID, slug).
		Where("status != ?", StatusDeleted).
		First(&dashboard).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrDashboardNotFound
		}
		return nil, fmt.Errorf("failed to get dashboard by slug: %w", err)
	}

	return &dashboard, nil
}

func (m *Manager) UpdateDashboard(ctx context.Context, id uuid.UUID, req *UpdateDashboardRequest) (*Dashboard, error) {
	dashboard, err := m.GetDashboard(ctx, id)
	if err != nil {
		return nil, err
	}

	updates := make(map[string]interface{})

	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.Status != nil {
		if !isValidDashboardStatus(*req.Status) {
			return nil, fmt.Errorf("%w: invalid status", ErrInvalidDashboard)
		}
		updates["status"] = *req.Status
	}
	if req.Config != nil {
		updates["config"] = *req.Config
	}
	if req.Layout != nil {
		updates["layout"] = *req.Layout
	}
	if req.Settings != nil {
		updates["settings"] = *req.Settings
	}
	if req.Tags != nil {
		updates["tags"] = req.Tags
	}
	if req.IsPublic != nil {
		updates["is_public"] = *req.IsPublic
	}
	if req.IsDefault != nil {
		updates["is_default"] = *req.IsDefault
	}

	updates["updated_by"] = req.UpdatedBy
	updates["version"] = dashboard.Version + 1

	if err := m.db.WithContext(ctx).Model(dashboard).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("failed to update dashboard: %w", err)
	}

	return m.GetDashboard(ctx, id)
}

func (m *Manager) DeleteDashboard(ctx context.Context, id uuid.UUID, deletedBy uuid.UUID) error {
	dashboard, err := m.GetDashboard(ctx, id)
	if err != nil {
		return err
	}

	now := time.Now()
	updates := map[string]interface{}{
		"status":     StatusDeleted,
		"updated_by": deletedBy,
		"deleted_at": now,
	}

	if err := m.db.WithContext(ctx).Model(dashboard).Updates(updates).Error; err != nil {
		return fmt.Errorf("failed to delete dashboard: %w", err)
	}

	return nil
}

func (m *Manager) ListDashboards(ctx context.Context, opts *ListDashboardsOptions) ([]DashboardSummary, int64, error) {
	query := m.db.WithContext(ctx).Model(&Dashboard{}).Where("status != ?", StatusDeleted)

	if !opts.TenantID.IsZero() {
		query = query.Where("tenant_id = ?", opts.TenantID)
	}
	if opts.OrgID != nil {
		query = query.Where("org_id = ?", *opts.OrgID)
	}
	if opts.WorkspaceID != nil {
		query = query.Where("workspace_id = ?", *opts.WorkspaceID)
	}
	if opts.Type != nil {
		query = query.Where("type = ?", *opts.Type)
	}
	if opts.Status != nil {
		query = query.Where("status = ?", *opts.Status)
	}
	if opts.OwnerID != nil {
		query = query.Where("owner_id = ?", *opts.OwnerID)
	}
	if opts.IsPublic != nil {
		query = query.Where("is_public = ?", *opts.IsPublic)
	}
	if opts.IsDefault != nil {
		query = query.Where("is_default = ?", *opts.IsDefault)
	}
	if len(opts.Tags) > 0 {
		query = query.Where("tags && ?", opts.Tags)
	}
	if opts.Search != "" {
		searchPattern := "%" + opts.Search + "%"
		query = query.Where("name ILIKE ? OR description ILIKE ?", searchPattern, searchPattern)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count dashboards: %w", err)
	}

	orderBy := "created_at"
	if opts.OrderBy != "" {
		orderBy = opts.OrderBy
	}
	orderDir := "DESC"
	if opts.OrderDir != "" {
		orderDir = opts.OrderDir
	}
	query = query.Order(fmt.Sprintf("%s %s", orderBy, orderDir))

	if opts.Limit > 0 {
		query = query.Limit(opts.Limit)
	}
	if opts.Offset > 0 {
		query = query.Offset(opts.Offset)
	}

	var dashboards []Dashboard
	if err := query.Find(&dashboards).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to list dashboards: %w", err)
	}

	summaries := make([]DashboardSummary, len(dashboards))
	for i, d := range dashboards {
		widgetCount := 0
		if d.Layout.Widgets != nil {
			widgetCount = len(d.Layout.Widgets)
		}
		summaries[i] = DashboardSummary{
			ID:          d.ID,
			Name:        d.Name,
			Slug:        d.Slug,
			Description: d.Description,
			Type:        d.Type,
			Status:      d.Status,
			OwnerID:     d.OwnerID,
			WidgetCount: widgetCount,
			CreatedAt:   d.CreatedAt,
			UpdatedAt:   d.UpdatedAt,
			IsPublic:    d.IsPublic,
			IsDefault:   d.IsDefault,
		}
	}

	return summaries, total, nil
}

func (m *Manager) DuplicateDashboard(ctx context.Context, id uuid.UUID, newName string, ownerID uuid.UUID) (*Dashboard, error) {
	original, err := m.GetDashboard(ctx, id)
	if err != nil {
		return nil, err
	}

	newSlug := generateUniqueSlug(original.Slug, ownerID)

	duplicate := &Dashboard{
		TenantID:    original.TenantID,
		OrgID:       original.OrgID,
		WorkspaceID: original.WorkspaceID,
		Name:        newName,
		Slug:        newSlug,
		Description: original.Description,
		Type:        TypeCustom,
		Status:      StatusDraft,
		Config:      original.Config,
		Layout:      original.Layout,
		Settings:    original.Settings,
		Tags:        original.Tags,
		OwnerID:     ownerID,
		CreatedBy:   ownerID,
		UpdatedBy:   ownerID,
		IsPublic:    false,
		IsDefault:   false,
		Version:     1,
	}

	if err := m.db.WithContext(ctx).Create(duplicate).Error; err != nil {
		return nil, fmt.Errorf("failed to duplicate dashboard: %w", err)
	}

	return duplicate, nil
}

func (m *Manager) SetDefaultDashboard(ctx context.Context, id uuid.UUID, updatedBy uuid.UUID) error {
	dashboard, err := m.GetDashboard(ctx, id)
	if err != nil {
		return err
	}

	tx := m.db.WithContext(ctx).Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	if err := tx.Model(&Dashboard{}).
		Where("tenant_id = ? AND org_id = ? AND is_default = ?", dashboard.TenantID, dashboard.OrgID, true).
		Update("is_default", false).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to unset default dashboard: %w", err)
	}

	if err := tx.Model(dashboard).Updates(map[string]interface{}{
		"is_default": true,
		"updated_by": updatedBy,
	}).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to set default dashboard: %w", err)
	}

	return tx.Commit().Error
}

func (m *Manager) GetDefaultDashboard(ctx context.Context, tenantID, orgID uuid.UUID) (*Dashboard, error) {
	var dashboard Dashboard
	err := m.db.WithContext(ctx).
		Where("tenant_id = ? AND org_id = ? AND is_default = ? AND status != ?",
			tenantID, orgID, true, StatusDeleted).
		First(&dashboard).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrDashboardNotFound
		}
		return nil, fmt.Errorf("failed to get default dashboard: %w", err)
	}

	return &dashboard, nil
}

func (m *Manager) GetDashboardsByOwner(ctx context.Context, ownerID uuid.UUID) ([]DashboardSummary, error) {
	var dashboards []Dashboard
	err := m.db.WithContext(ctx).
		Where("owner_id = ? AND status != ?", ownerID, StatusDeleted).
		Order("created_at DESC").
		Find(&dashboards).Error

	if err != nil {
		return nil, fmt.Errorf("failed to get dashboards by owner: %w", err)
	}

	summaries := make([]DashboardSummary, len(dashboards))
	for i, d := range dashboards {
		widgetCount := 0
		if d.Layout.Widgets != nil {
			widgetCount = len(d.Layout.Widgets)
		}
		summaries[i] = DashboardSummary{
			ID:          d.ID,
			Name:        d.Name,
			Slug:        d.Slug,
			Description: d.Description,
			Type:        d.Type,
			Status:      d.Status,
			OwnerID:     d.OwnerID,
			WidgetCount: widgetCount,
			CreatedAt:   d.CreatedAt,
			UpdatedAt:   d.UpdatedAt,
			IsPublic:    d.IsPublic,
			IsDefault:   d.IsDefault,
		}
	}

	return summaries, nil
}

func (m *Manager) ArchiveDashboard(ctx context.Context, id uuid.UUID, updatedBy uuid.UUID) error {
	status := StatusArchived
	req := &UpdateDashboardRequest{
		Status:    &status,
		UpdatedBy: updatedBy,
	}
	_, err := m.UpdateDashboard(ctx, id, req)
	return err
}

func (m *Manager) RestoreDashboard(ctx context.Context, id uuid.UUID, updatedBy uuid.UUID) error {
	status := StatusActive
	req := &UpdateDashboardRequest{
		Status:    &status,
		UpdatedBy: updatedBy,
	}
	_, err := m.UpdateDashboard(ctx, id, req)
	return err
}

func (m *Manager) validateDashboardRequest(req *CreateDashboardRequest) error {
	if req.Name == "" {
		return fmt.Errorf("name is required")
	}
	if req.Slug == "" {
		return fmt.Errorf("slug is required")
	}
	if req.TenantID.IsZero() {
		return fmt.Errorf("tenant_id is required")
	}
	if req.OrgID.IsZero() {
		return fmt.Errorf("org_id is required")
	}
	if req.OwnerID.IsZero() {
		return fmt.Errorf("owner_id is required")
	}

	if req.Type != "" && !isValidDashboardType(req.Type) {
		return fmt.Errorf("invalid dashboard type")
	}

	if len(req.Layout.Widgets) > 0 {
		for _, w := range req.Layout.Widgets {
			if w.WidgetID.IsZero() {
				return fmt.Errorf("widget_id is required for all widgets")
			}
		}
	}

	return nil
}

func isValidDashboardStatus(status DashboardStatus) bool {
	switch status {
	case StatusActive, StatusArchived, StatusDraft, StatusDeleted:
		return true
	default:
		return false
	}
}

func isValidDashboardType(dashboardType DashboardType) bool {
	switch dashboardType {
	case TypeSystem, TypeCustom, TypeTemplate:
		return true
	default:
		return false
	}
}

func generateUniqueSlug(baseSlug string, ownerID uuid.UUID) string {
	timestamp := time.Now().Unix()
	hash := sha256.Sum256([]byte(fmt.Sprintf("%s-%s-%d", baseSlug, ownerID, timestamp)))
	return fmt.Sprintf("%s-%x", baseSlug, hash[:4])
}

func (m *Manager) ExportDashboard(ctx context.Context, id uuid.UUID) ([]byte, error) {
	dashboard, err := m.GetDashboard(ctx, id)
	if err != nil {
		return nil, err
	}

	export := struct {
		Dashboard
		ExportVersion string `json:"export_version"`
		ExportedAt    time.Time `json:"exported_at"`
	}{
		Dashboard:     *dashboard,
		ExportVersion: "1.0",
		ExportedAt:    time.Now(),
	}

	data, err := json.MarshalIndent(export, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to export dashboard: %w", err)
	}

	return data, nil
}

func (m *Manager) ImportDashboard(ctx context.Context, data []byte, ownerID uuid.UUID) (*Dashboard, error) {
	var import struct {
		Dashboard
		ExportVersion string `json:"export_version"`
		ExportedAt    time.Time `json:"exported_at"`
	}

	if err := json.Unmarshal(data, &import); err != nil {
		return nil, fmt.Errorf("failed to parse dashboard import: %w", err)
	}

	newSlug := generateUniqueSlug(import.Slug, ownerID)

	dashboard := &Dashboard{
		TenantID:    import.TenantID,
		OrgID:       import.OrgID,
		WorkspaceID: import.WorkspaceID,
		Name:        import.Name,
		Slug:        newSlug,
		Description: import.Description,
		Type:        TypeCustom,
		Status:      StatusDraft,
		Config:      import.Config,
		Layout:      import.Layout,
		Settings:    import.Settings,
		Tags:        import.Tags,
		OwnerID:     ownerID,
		CreatedBy:   ownerID,
		UpdatedBy:   ownerID,
		IsPublic:    false,
		IsDefault:   false,
		Version:     1,
	}

	if err := m.db.WithContext(ctx).Create(dashboard).Error; err != nil {
		return nil, fmt.Errorf("failed to import dashboard: %w", err)
	}

	return dashboard, nil
}
