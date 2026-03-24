package dashboard

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var (
	ErrShareNotFound       = errors.New("dashboard share not found")
	ErrShareExpired        = errors.New("dashboard share has expired")
	ErrInvalidPermission   = errors.New("invalid permission level")
	ErrUnauthorizedShare   = errors.New("unauthorized to share dashboard")
	ErrShareLimitExceeded  = errors.New("share limit exceeded")
)

type PermissionLevel string

const (
	PermissionView   PermissionLevel = "view"
	PermissionEdit   PermissionLevel = "edit"
	PermissionAdmin  PermissionLevel = "admin"
	PermissionOwner  PermissionLevel = "owner"
)

type ShareType string

const (
	ShareTypeUser    ShareType = "user"
	ShareTypeTeam    ShareType = "team"
	ShareTypeOrg     ShareType = "organization"
	ShareTypePublic  ShareType = "public"
	ShareTypeLink    ShareType = "link"
)

type DashboardShare struct {
	ID           uuid.UUID       `json:"id" gorm:"type:uuid;primary_key;default:gen_random_uuid()"`
	DashboardID  uuid.UUID       `json:"dashboard_id" gorm:"type:uuid;not null;index"`
	ShareType    ShareType       `json:"share_type" gorm:"not null;index"`
	SharedWith   *uuid.UUID      `json:"shared_with,omitempty" gorm:"type:uuid;index"`
	SharedBy     uuid.UUID       `json:"shared_by" gorm:"type:uuid;not null;index"`
	Permission   PermissionLevel `json:"permission" gorm:"not null;default:'view'"`
	ShareToken   string          `json:"share_token" gorm:"unique;index"`
	Password     *string         `json:"password,omitempty"`
	ExpiresAt    *time.Time      `json:"expires_at,omitempty"`
	MaxAccess    *int            `json:"max_access,omitempty"`
	AccessCount  int             `json:"access_count" gorm:"default:0"`
	IsActive     bool            `json:"is_active" gorm:"default:true"`
	AllowDownload bool           `json:"allow_download" gorm:"default:true"`
	AllowShare   bool            `json:"allow_share" gorm:"default:false"`
	CreatedAt    time.Time       `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt    time.Time       `json:"updated_at" gorm:"autoUpdateTime"`
	LastAccessAt *time.Time      `json:"last_access_at,omitempty"`
	Metadata     map[string]interface{} `json:"metadata" gorm:"type:jsonb"`
}

type SharePermission struct {
	ID          uuid.UUID       `json:"id" gorm:"type:uuid;primary_key;default:gen_random_uuid()"`
	DashboardID uuid.UUID       `json:"dashboard_id" gorm:"type:uuid;not null;index"`
	UserID      uuid.UUID       `json:"user_id" gorm:"type:uuid;not null;index"`
	Permission  PermissionLevel `json:"permission" gorm:"not null"`
	Inherited   bool            `json:"inherited" gorm:"default:false"`
	Source      string          `json:"source"` // "direct", "team", "org"
	GrantedBy   uuid.UUID       `json:"granted_by" gorm:"type:uuid;not null"`
	GrantedAt   time.Time       `json:"granted_at" gorm:"autoCreateTime"`
	ExpiresAt   *time.Time      `json:"expires_at,omitempty"`
}

type CreateShareRequest struct {
	DashboardID   uuid.UUID       `json:"dashboard_id" binding:"required"`
	ShareType     ShareType       `json:"share_type" binding:"required"`
	SharedWith    *uuid.UUID      `json:"shared_with,omitempty"`
	Permission    PermissionLevel `json:"permission" binding:"required"`
	Password      *string         `json:"password,omitempty"`
	ExpiresAt     *time.Time      `json:"expires_at,omitempty"`
	MaxAccess     *int            `json:"max_access,omitempty"`
	AllowDownload bool            `json:"allow_download"`
	AllowShare    bool            `json:"allow_share"`
	SharedBy      uuid.UUID       `json:"shared_by" binding:"required"`
}

type UpdateShareRequest struct {
	Permission    *PermissionLevel `json:"permission,omitempty"`
	Password      *string          `json:"password,omitempty"`
	ExpiresAt     *time.Time       `json:"expires_at,omitempty"`
	MaxAccess     *int             `json:"max_access,omitempty"`
	IsActive      *bool            `json:"is_active,omitempty"`
	AllowDownload *bool            `json:"allow_download,omitempty"`
	AllowShare    *bool            `json:"allow_share,omitempty"`
}

type ShareAccessLog struct {
	ID          uuid.UUID `json:"id" gorm:"type:uuid;primary_key;default:gen_random_uuid()"`
	ShareID     uuid.UUID `json:"share_id" gorm:"type:uuid;not null;index"`
	UserID      *uuid.UUID `json:"user_id,omitempty" gorm:"type:uuid;index"`
	IPAddress   string    `json:"ip_address"`
	UserAgent   string    `json:"user_agent"`
	AccessedAt  time.Time `json:"accessed_at" gorm:"autoCreateTime;index"`
	Action      string    `json:"action"` // "view", "edit", "download"
}

type SharingManager struct {
	db *gorm.DB
}

func NewSharingManager(db *gorm.DB) *SharingManager {
	return &SharingManager{db: db}
}

func (sm *SharingManager) CreateShare(ctx context.Context, req *CreateShareRequest) (*DashboardShare, error) {
	if err := sm.validateShareRequest(req); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidPermission, err)
	}

	shareToken := sm.generateShareToken()

	share := &DashboardShare{
		DashboardID:   req.DashboardID,
		ShareType:     req.ShareType,
		SharedWith:    req.SharedWith,
		SharedBy:      req.SharedBy,
		Permission:    req.Permission,
		ShareToken:    shareToken,
		Password:      req.Password,
		ExpiresAt:     req.ExpiresAt,
		MaxAccess:     req.MaxAccess,
		AllowDownload: req.AllowDownload,
		AllowShare:    req.AllowShare,
		IsActive:      true,
		Metadata:      make(map[string]interface{}),
	}

	if err := sm.db.WithContext(ctx).Create(share).Error; err != nil {
		return nil, fmt.Errorf("failed to create share: %w", err)
	}

	if req.ShareType == ShareTypeUser && req.SharedWith != nil {
		permission := &SharePermission{
			DashboardID: req.DashboardID,
			UserID:      *req.SharedWith,
			Permission:  req.Permission,
			Inherited:   false,
			Source:      "direct",
			GrantedBy:   req.SharedBy,
			ExpiresAt:   req.ExpiresAt,
		}
		if err := sm.db.WithContext(ctx).Create(permission).Error; err != nil {
			return nil, fmt.Errorf("failed to create permission: %w", err)
		}
	}

	return share, nil
}

func (sm *SharingManager) GetShare(ctx context.Context, id uuid.UUID) (*DashboardShare, error) {
	var share DashboardShare
	err := sm.db.WithContext(ctx).
		Where("id = ?", id).
		First(&share).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrShareNotFound
		}
		return nil, fmt.Errorf("failed to get share: %w", err)
	}

	return &share, nil
}

func (sm *SharingManager) GetShareByToken(ctx context.Context, token string) (*DashboardShare, error) {
	var share DashboardShare
	err := sm.db.WithContext(ctx).
		Where("share_token = ? AND is_active = ?", token, true).
		First(&share).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrShareNotFound
		}
		return nil, fmt.Errorf("failed to get share by token: %w", err)
	}

	if share.ExpiresAt != nil && time.Now().After(*share.ExpiresAt) {
		return nil, ErrShareExpired
	}

	if share.MaxAccess != nil && share.AccessCount >= *share.MaxAccess {
		return nil, ErrShareLimitExceeded
	}

	return &share, nil
}

func (sm *SharingManager) UpdateShare(ctx context.Context, id uuid.UUID, req *UpdateShareRequest) (*DashboardShare, error) {
	share, err := sm.GetShare(ctx, id)
	if err != nil {
		return nil, err
	}

	updates := make(map[string]interface{})

	if req.Permission != nil {
		if !isValidPermission(*req.Permission) {
			return nil, ErrInvalidPermission
		}
		updates["permission"] = *req.Permission
	}
	if req.Password != nil {
		updates["password"] = *req.Password
	}
	if req.ExpiresAt != nil {
		updates["expires_at"] = *req.ExpiresAt
	}
	if req.MaxAccess != nil {
		updates["max_access"] = *req.MaxAccess
	}
	if req.IsActive != nil {
		updates["is_active"] = *req.IsActive
	}
	if req.AllowDownload != nil {
		updates["allow_download"] = *req.AllowDownload
	}
	if req.AllowShare != nil {
		updates["allow_share"] = *req.AllowShare
	}

	if len(updates) > 0 {
		if err := sm.db.WithContext(ctx).Model(share).Updates(updates).Error; err != nil {
			return nil, fmt.Errorf("failed to update share: %w", err)
		}
	}

	return sm.GetShare(ctx, id)
}

func (sm *SharingManager) DeleteShare(ctx context.Context, id uuid.UUID) error {
	result := sm.db.WithContext(ctx).Delete(&DashboardShare{}, "id = ?", id)
	if result.Error != nil {
		return fmt.Errorf("failed to delete share: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrShareNotFound
	}
	return nil
}

func (sm *SharingManager) RevokeShare(ctx context.Context, id uuid.UUID) error {
	isActive := false
	req := &UpdateShareRequest{
		IsActive: &isActive,
	}
	_, err := sm.UpdateShare(ctx, id, req)
	return err
}

func (sm *SharingManager) GetSharesByDashboard(ctx context.Context, dashboardID uuid.UUID) ([]DashboardShare, error) {
	var shares []DashboardShare
	err := sm.db.WithContext(ctx).
		Where("dashboard_id = ?", dashboardID).
		Order("created_at DESC").
		Find(&shares).Error

	if err != nil {
		return nil, fmt.Errorf("failed to get shares: %w", err)
	}

	return shares, nil
}

func (sm *SharingManager) GetUserPermissions(ctx context.Context, dashboardID, userID uuid.UUID) (*SharePermission, error) {
	var permission SharePermission
	err := sm.db.WithContext(ctx).
		Where("dashboard_id = ? AND user_id = ?", dashboardID, userID).
		First(&permission).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get user permissions: %w", err)
	}

	if permission.ExpiresAt != nil && time.Now().After(*permission.ExpiresAt) {
		return nil, nil
	}

	return &permission, nil
}

func (sm *SharingManager) CheckPermission(ctx context.Context, dashboardID, userID uuid.UUID, required PermissionLevel) (bool, error) {
	permission, err := sm.GetUserPermissions(ctx, dashboardID, userID)
	if err != nil {
		return false, err
	}

	if permission == nil {
		return false, nil
	}

	return hasPermissionLevel(permission.Permission, required), nil
}

func (sm *SharingManager) LogShareAccess(ctx context.Context, shareID uuid.UUID, userID *uuid.UUID, ipAddress, userAgent, action string) error {
	log := &ShareAccessLog{
		ShareID:    shareID,
		UserID:     userID,
		IPAddress:  ipAddress,
		UserAgent:  userAgent,
		Action:     action,
	}

	if err := sm.db.WithContext(ctx).Create(log).Error; err != nil {
		return fmt.Errorf("failed to log share access: %w", err)
	}

	now := time.Now()
	updates := map[string]interface{}{
		"access_count":  gorm.Expr("access_count + 1"),
		"last_access_at": now,
	}

	if err := sm.db.WithContext(ctx).
		Model(&DashboardShare{}).
		Where("id = ?", shareID).
		Updates(updates).Error; err != nil {
		return fmt.Errorf("failed to update share access: %w", err)
	}

	return nil
}

func (sm *SharingManager) GetShareAccessLogs(ctx context.Context, shareID uuid.UUID, limit, offset int) ([]ShareAccessLog, error) {
	var logs []ShareAccessLog
	query := sm.db.WithContext(ctx).
		Where("share_id = ?", shareID).
		Order("accessed_at DESC")

	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}

	if err := query.Find(&logs).Error; err != nil {
		return nil, fmt.Errorf("failed to get access logs: %w", err)
	}

	return logs, nil
}

func (sm *SharingManager) CreatePublicShare(ctx context.Context, dashboardID uuid.UUID, createdBy uuid.UUID, expiresAt *time.Time) (*DashboardShare, error) {
	req := &CreateShareRequest{
		DashboardID: dashboardID,
		ShareType:   ShareTypePublic,
		Permission:  PermissionView,
		ExpiresAt:   expiresAt,
		SharedBy:    createdBy,
	}

	return sm.CreateShare(ctx, req)
}

func (sm *SharingManager) ValidateShareAccess(ctx context.Context, token, password string) (*DashboardShare, error) {
	share, err := sm.GetShareByToken(ctx, token)
	if err != nil {
		return nil, err
	}

	if share.Password != nil && *share.Password != password {
		return nil, ErrUnauthorizedShare
	}

	if !share.IsActive {
		return nil, ErrShareExpired
	}

	return share, nil
}

func (sm *SharingManager) generateShareToken() string {
	timestamp := time.Now().Unix()
	random := uuid.New().String()
	data := fmt.Sprintf("%d-%s", timestamp, random)
	hash := sha256.Sum256([]byte(data))
	return base64.URLEncoding.EncodeToString(hash[:16])
}

func (sm *SharingManager) validateShareRequest(req *CreateShareRequest) error {
	if !isValidPermission(req.Permission) {
		return fmt.Errorf("invalid permission level")
	}

	if req.ShareType == ShareTypeUser && req.SharedWith == nil {
		return fmt.Errorf("shared_with is required for user share type")
	}

	return nil
}

func isValidPermission(permission PermissionLevel) bool {
	switch permission {
	case PermissionView, PermissionEdit, PermissionAdmin, PermissionOwner:
		return true
	default:
		return false
	}
}

func hasPermissionLevel(userPermission, requiredPermission PermissionLevel) bool {
	permissionLevels := map[PermissionLevel]int{
		PermissionView:  1,
		PermissionEdit:  2,
		PermissionAdmin: 3,
		PermissionOwner: 4,
	}

	userLevel := permissionLevels[userPermission]
	requiredLevel := permissionLevels[requiredPermission]

	return userLevel >= requiredLevel
}
