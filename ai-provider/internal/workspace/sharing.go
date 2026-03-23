// internal/workspace/sharing.go
// Shared resources between workspaces
// Handles resource sharing, permissions, and access control

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
	ErrShareNotFound       = errors.New("share not found")
	ErrShareAlreadyExists = errors.New("share already exists")
	ErrInvalidShare       = errors.New("invalid share data")
	ErrAccessDenied       = errors.New("access denied")
	ErrShareExpired       = errors.New("share has expired")
	ErrCannotShare        = errors.New("cannot share this resource")
)

// SharePermission represents the permission level for a shared resource
type SharePermission string

const (
	SharePermissionRead   SharePermission = "read"
	SharePermissionWrite  SharePermission = "write"
	SharePermissionAdmin  SharePermission = "admin"
)

// ShareStatus represents the status of a share
type ShareStatus string

const (
	ShareStatusActive   ShareStatus = "active"
	ShareStatusPending  ShareStatus = "pending"
	ShareStatusRevoked  ShareStatus = "revoked"
	ShareStatusExpired  ShareStatus = "expired"
)

// ShareType represents the type of resource being shared
type ShareType string

const (
	ShareTypeWorkspace  ShareType = "workspace"
	ShareTypeResource   ShareType = "resource"
	ShareTypeFolder     ShareType = "folder"
)

// WorkspaceShare represents a share between workspaces
type WorkspaceShare struct {
	ID             uuid.UUID        `json:"id" db:"id"`
	SourceID       uuid.UUID        `json:"source_id" db:"source_id"`
	SourceWorkspaceID uuid.UUID     `json:"source_workspace_id" db:"source_workspace_id"`
	TargetID       uuid.UUID        `json:"target_id" db:"target_id"`
	TargetWorkspaceID uuid.UUID     `json:"target_workspace_id" db:"target_workspace_id"`

	Type           ShareType        `json:"type" db:"type"`
	Permission     SharePermission `json:"permission" db:"permission"`
	Status         ShareStatus     `json:"status" db:"status"`

	// Metadata
	Description    string          `json:"description" db:"description"`
	Metadata       json.RawMessage `json:"metadata,omitempty" db:"metadata"`

	// Access control
	SharedBy       uuid.UUID       `json:"shared_by" db:"shared_by"`
	ApprovedBy     *uuid.UUID      `json:"approved_by,omitempty" db:"approved_by"`

	// Expiration
	ExpiresAt      *time.Time      `json:"expires_at,omitempty" db:"expires_at"`

	// Timestamps
	CreatedAt      time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at" db:"updated_at"`
	RevokedAt      *time.Time      `json:"revoked_at,omitempty" db:"revoked_at"`
	LastAccessedAt *time.Time      `json:"last_accessed_at,omitempty" db:"last_accessed_at"`
}

// CreateShareRequest represents a request to create a new share
type CreateShareRequest struct {
	SourceID       uuid.UUID       `json:"source_id"`
	SourceWorkspaceID uuid.UUID    `json:"source_workspace_id"`
	TargetID       uuid.UUID       `json:"target_id"`
	TargetWorkspaceID uuid.UUID    `json:"target_workspace_id"`
	Type           ShareType       `json:"type"`
	Permission     SharePermission `json:"permission"`
	Description    string         `json:"description,omitempty"`
	Metadata       interface{}    `json:"metadata,omitempty"`
	ExpiresIn      *int           `json:"expires_in,omitempty"` // hours
}

// UpdateShareRequest represents a request to update a share
type UpdateShareRequest struct {
	Permission     *SharePermission `json:"permission,omitempty"`
	Status         *ShareStatus     `json:"status,omitempty"`
	Description    *string          `json:"description,omitempty"`
	Metadata       interface{}      `json:"metadata,omitempty"`
	ExpiresAt      *time.Time       `json:"expires_at,omitempty"`
}

// ListSharesOptions represents options for listing shares
type ListSharesOptions struct {
	SourceWorkspaceID *uuid.UUID
	TargetWorkspaceID *uuid.UUID
	SourceID         *uuid.UUID
	TargetID         *uuid.UUID
	Type             *ShareType
	Permission       *SharePermission
	Status           *ShareStatus
	Limit            int
	Offset           int
}

// SharingManager manages workspace shares
type SharingManager struct {
	db *sql.DB
}

// NewSharingManager creates a new sharing manager
func NewSharingManager(db *sql.DB) *SharingManager {
	return &SharingManager{
		db: db,
	}
}

// CreateShare creates a new share between workspaces
func (sm *SharingManager) CreateShare(ctx context.Context, req CreateShareRequest, sharedBy uuid.UUID) (*WorkspaceShare, error) {
	if req.SourceWorkspaceID == uuid.Nil {
		return nil, ErrInvalidShare
	}
	if req.TargetWorkspaceID == uuid.Nil {
		return nil, ErrInvalidShare
	}
	if !isValidSharePermission(req.Permission) {
		return nil, ErrInvalidShare
	}
	if !isValidShareType(req.Type) {
		return nil, ErrInvalidShare
	}

	// Check if share already exists
	existing, err := sm.GetShare(ctx, req.SourceID, req.TargetID, req.Type)
	if err == nil && existing != nil {
		return nil, ErrShareAlreadyExists
	}

	var expiresAt *time.Time
	if req.ExpiresIn != nil {
		exp := time.Now().Add(time.Duration(*req.ExpiresIn) * time.Hour)
		expiresAt = &exp
	}

	share := &WorkspaceShare{
		ID:               uuid.New(),
		SourceID:         req.SourceID,
		SourceWorkspaceID: req.SourceWorkspaceID,
		TargetID:         req.TargetID,
		TargetWorkspaceID: req.TargetWorkspaceID,
		Type:             req.Type,
		Permission:       req.Permission,
		Status:           ShareStatusActive,
		Description:      req.Description,
		SharedBy:         sharedBy,
		ExpiresAt:        expiresAt,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	// Marshal metadata
	if req.Metadata != nil {
		metadataJSON, err := json.Marshal(req.Metadata)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal metadata: %w", err)
		}
		share.Metadata = metadataJSON
	}

	query := `
		INSERT INTO workspace_shares (id, source_id, source_workspace_id, target_id,
			target_workspace_id, type, permission, status, description, metadata,
			shared_by, expires_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		RETURNING id, source_id, source_workspace_id, target_id, target_workspace_id,
			type, permission, status, description, metadata, shared_by, approved_by,
			expires_at, created_at, updated_at, revoked_at, last_accessed_at
	`

	err = sm.db.QueryRowContext(ctx, query,
		share.ID,
		share.SourceID,
		share.SourceWorkspaceID,
		share.TargetID,
		share.TargetWorkspaceID,
		share.Type,
		share.Permission,
		share.Status,
		share.Description,
		share.Metadata,
		share.SharedBy,
		share.ExpiresAt,
		share.CreatedAt,
		share.UpdatedAt,
	).Scan(
		&share.ID,
		&share.SourceID,
		&share.SourceWorkspaceID,
		&share.TargetID,
		&share.TargetWorkspaceID,
		&share.Type,
		&share.Permission,
		&share.Status,
		&share.Description,
		&share.Metadata,
		&share.SharedBy,
		&share.ApprovedBy,
		&share.ExpiresAt,
		&share.CreatedAt,
		&share.UpdatedAt,
		&share.RevokedAt,
		&share.LastAccessedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create share: %w", err)
	}

	return share, nil
}

// GetShare retrieves a share by source, target, and type
func (sm *SharingManager) GetShare(ctx context.Context, sourceID, targetID uuid.UUID, shareType ShareType) (*WorkspaceShare, error) {
	if sourceID == uuid.Nil || targetID == uuid.Nil {
		return nil, ErrInvalidShare
	}

	var share WorkspaceShare

	query := `
		SELECT id, source_id, source_workspace_id, target_id, target_workspace_id,
			type, permission, status, description, metadata, shared_by, approved_by,
			expires_at, created_at, updated_at, revoked_at, last_accessed_at
		FROM workspace_shares
		WHERE source_id = $1 AND target_id = $2 AND type = $3 AND status != 'deleted'
	`

	err := sm.db.QueryRowContext(ctx, query, sourceID, targetID, shareType).Scan(
		&share.ID,
		&share.SourceID,
		&share.SourceWorkspaceID,
		&share.TargetID,
		&share.TargetWorkspaceID,
		&share.Type,
		&share.Permission,
		&share.Status,
		&share.Description,
		&share.Metadata,
		&share.SharedBy,
		&share.ApprovedBy,
		&share.ExpiresAt,
		&share.CreatedAt,
		&share.UpdatedAt,
		&share.RevokedAt,
		&share.LastAccessedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrShareNotFound
		}
		return nil, fmt.Errorf("failed to get share: %w", err)
	}

	// Check if share has expired
	if share.ExpiresAt != nil && time.Now().After(*share.ExpiresAt) {
		return nil, ErrShareExpired
	}

	return &share, nil
}

// UpdateShare updates a share
func (sm *SharingManager) UpdateShare(ctx context.Context, id uuid.UUID, req UpdateShareRequest, updatedBy uuid.UUID) (*WorkspaceShare, error) {
	if id == uuid.Nil {
		return nil, ErrInvalidShare
	}

	share, err := sm.GetShareByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// Check if share is active
	if share.Status != ShareStatusActive {
		return nil, ErrAccessDenied
	}

	// Apply updates
	if req.Permission != nil {
		if !isValidSharePermission(*req.Permission) {
			return nil, ErrInvalidShare
		}
		share.Permission = *req.Permission
	}
	if req.Status != nil {
		if !isValidShareStatus(*req.Status) {
			return nil, errors.New("invalid share status")
		}
		share.Status = *req.Status
	}
	if req.Description != nil {
		share.Description = *req.Description
	}
	if req.ExpiresAt != nil {
		share.ExpiresAt = req.ExpiresAt
	}
	if req.Metadata != nil {
		metadataJSON, err := json.Marshal(req.Metadata)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal metadata: %w", err)
		}
		share.Metadata = metadataJSON
	}
	share.UpdatedAt = time.Now()

	query := `
		UPDATE workspace_shares
		SET permission = $1, status = $2, description = $3, metadata = $4,
			expires_at = $5, updated_at = $6
		WHERE id = $7
		RETURNING id, source_id, source_workspace_id, target_id, target_workspace_id,
			type, permission, status, description, metadata, shared_by, approved_by,
			expires_at, created_at, updated_at, revoked_at, last_accessed_at
	`

	err = sm.db.QueryRowContext(ctx, query,
		share.Permission,
		share.Status,
		share.Description,
		share.Metadata,
		share.ExpiresAt,
		share.UpdatedAt,
		share.ID,
	).Scan(
		&share.ID,
		&share.SourceID,
		&share.SourceWorkspaceID,
		&share.TargetID,
		&share.TargetWorkspaceID,
		&share.Type,
		&share.Permission,
		&share.Status,
		&share.Description,
		&share.Metadata,
		&share.SharedBy,
		&share.ApprovedBy,
		&share.ExpiresAt,
		&share.CreatedAt,
		&share.UpdatedAt,
		&share.RevokedAt,
		&share.LastAccessedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to update share: %w", err)
	}

	return share, nil
}

// RevokeShare revokes a share
func (sm *SharingManager) RevokeShare(ctx context.Context, id uuid.UUID) error {
	if id == uuid.Nil {
		return ErrInvalidShare
	}

	now := time.Now()

	query := `
		UPDATE workspace_shares
		SET status = 'revoked', revoked_at = $1, updated_at = $1
		WHERE id = $2
	`

	result, err := sm.db.ExecContext(ctx, query, now, id)
	if err != nil {
		return fmt.Errorf("failed to revoke share: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return ErrShareNotFound
	}

	return nil
}

// DeleteShare deletes a share permanently
func (sm *SharingManager) DeleteShare(ctx context.Context, id uuid.UUID) error {
	if id == uuid.Nil {
		return ErrInvalidShare
	}

	query := `DELETE FROM workspace_shares WHERE id = $1`

	result, err := sm.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete share: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return ErrShareNotFound
	}

	return nil
}

// ListShares lists shares with optional filters
func (sm *SharingManager) ListShares(ctx context.Context, opts ListSharesOptions) ([]*WorkspaceShare, int64, error) {
	shares := []*WorkspaceShare{}
	var total int64

	// Build query with dynamic filters
	baseQuery := `
		SELECT id, source_id, source_workspace_id, target_id, target_workspace_id,
			type, permission, status, description, metadata, shared_by, approved_by,
			expires_at, created_at, updated_at, revoked_at, last_accessed_at
		FROM workspace_shares
		WHERE status != 'deleted'
	`
	countQuery := `SELECT COUNT(*) FROM workspace_shares WHERE status != 'deleted'`

	args := []interface{}{}
	argPos := 1

	if opts.SourceWorkspaceID != nil {
		baseQuery += fmt.Sprintf(" AND source_workspace_id = $%d", argPos)
		countQuery += fmt.Sprintf(" AND source_workspace_id = $%d", argPos)
		args = append(args, *opts.SourceWorkspaceID)
		argPos++
	}

	if opts.TargetWorkspaceID != nil {
		baseQuery += fmt.Sprintf(" AND target_workspace_id = $%d", argPos)
		countQuery += fmt.Sprintf(" AND target_workspace_id = $%d", argPos)
		args = append(args, *opts.TargetWorkspaceID)
		argPos++
	}

	if opts.SourceID != nil {
		baseQuery += fmt.Sprintf(" AND source_id = $%d", argPos)
		countQuery += fmt.Sprintf(" AND source_id = $%d", argPos)
		args = append(args, *opts.SourceID)
		argPos++
	}

	if opts.TargetID != nil {
		baseQuery += fmt.Sprintf(" AND target_id = $%d", argPos)
		countQuery += fmt.Sprintf(" AND target_id = $%d", argPos)
		args = append(args, *opts.TargetID)
		argPos++
	}

	if opts.Type != nil {
		baseQuery += fmt.Sprintf(" AND type = $%d", argPos)
		countQuery += fmt.Sprintf(" AND type = $%d", argPos)
		args = append(args, *opts.Type)
		argPos++
	}

	if opts.Permission != nil {
		baseQuery += fmt.Sprintf(" AND permission = $%d", argPos)
		countQuery += fmt.Sprintf(" AND permission = $%d", argPos)
		args = append(args, *opts.Permission)
		argPos++
	}

	if opts.Status != nil {
		baseQuery += fmt.Sprintf(" AND status = $%d", argPos)
		countQuery += fmt.Sprintf(" AND status = $%d", argPos)
		args = append(args, *opts.Status)
		argPos++
	}

	// Get total count
	err := sm.db.QueryRowContext(ctx, countQuery, args...[:argPos-1]...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count shares: %w", err)
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

	rows, err := sm.db.QueryContext(ctx, baseQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list shares: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var share WorkspaceShare

		err := rows.Scan(
			&share.ID,
			&share.SourceID,
			&share.SourceWorkspaceID,
			&share.TargetID,
			&share.TargetWorkspaceID,
			&share.Type,
			&share.Permission,
			&share.Status,
			&share.Description,
			&share.Metadata,
			&share.SharedBy,
			&share.ApprovedBy,
			&share.ExpiresAt,
			&share.CreatedAt,
			&share.UpdatedAt,
			&share.RevokedAt,
			&share.LastAccessedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan share: %w", err)
		}

		shares = append(shares, &share)
	}

	if err = rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating shares: %w", err)
	}

	return shares, total, nil
}

// GetSharesByWorkspace retrieves all shares for a workspace (both incoming and outgoing)
func (sm *SharingManager) GetSharesByWorkspace(ctx context.Context, workspaceID uuid.UUID) ([]*WorkspaceShare, error) {
	if workspaceID == uuid.Nil {
		return nil, ErrInvalidShare
	}

	var shares []*WorkspaceShare

	query := `
		SELECT id, source_id, source_workspace_id, target_id, target_workspace_id,
			type, permission, status, description, metadata, shared_by, approved_by,
			expires_at, created_at, updated_at, revoked_at, last_accessed_at
		FROM workspace_shares
		WHERE source_workspace_id = $1 OR target_workspace_id = $1
		AND status != 'deleted'
		ORDER BY created_at DESC
	`

	rows, err := sm.db.QueryContext(ctx, query, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get shares by workspace: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var share WorkspaceShare

		err := rows.Scan(
			&share.ID,
			&share.SourceID,
			&share.SourceWorkspaceID,
			&share.TargetID,
			&share.TargetWorkspaceID,
			&share.Type,
			&share.Permission,
			&share.Status,
			&share.Description,
			&share.Metadata,
			&share.SharedBy,
			&share.ApprovedBy,
			&share.ExpiresAt,
			&share.CreatedAt,
			&share.UpdatedAt,
			&share.RevokedAt,
			&share.LastAccessedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan share: %w", err)
		}

		shares = append(shares, &share)
	}

	return shares, nil
}

// CheckAccess checks if a workspace has access to a shared resource
func (sm *SharingManager) CheckAccess(ctx context.Context, workspaceID, resourceID uuid.UUID, requiredPermission SharePermission) (bool, error) {
	if workspaceID == uuid.Nil || resourceID == uuid.Nil {
		return false, ErrInvalidShare
	}

	var count int
	var permissionLevel string

	query := `
		SELECT permission
		FROM workspace_shares
		WHERE target_workspace_id = $1 AND source_id = $2
		AND status = 'active'
		AND (expires_at IS NULL OR expires_at > CURRENT_TIMESTAMP)
	`

	err := sm.db.QueryRowContext(ctx, query, workspaceID, resourceID).Scan(&permissionLevel)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, fmt.Errorf("failed to check access: %w", err)
	}

	// Check permission hierarchy
	if requiredPermission == SharePermissionRead {
		return true, nil
	}
	if requiredPermission == SharePermissionWrite {
		return permissionLevel == string(SharePermissionWrite) || permissionLevel == string(SharePermissionAdmin), nil
	}
	if requiredPermission == SharePermissionAdmin {
		return permissionLevel == string(SharePermissionAdmin), nil
	}

	return false, nil
}

// UpdateLastAccessed updates the last accessed timestamp for a share
func (sm *SharingManager) UpdateLastAccessed(ctx context.Context, id uuid.UUID) error {
	query := `
		UPDATE workspace_shares
		SET last_accessed_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
	`

	_, err := sm.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to update last accessed: %w", err)
	}

	return nil
}

// GetShareByID retrieves a share by ID
func (sm *SharingManager) GetShareByID(ctx context.Context, id uuid.UUID) (*WorkspaceShare, error) {
	if id == uuid.Nil {
		return nil, ErrInvalidShare
	}

	var share WorkspaceShare

	query := `
		SELECT id, source_id, source_workspace_id, target_id, target_workspace_id,
			type, permission, status, description, metadata, shared_by, approved_by,
			expires_at, created_at, updated_at, revoked_at, last_accessed_at
		FROM workspace_shares
		WHERE id = $1 AND status != 'deleted'
	`

	err := sm.db.QueryRowContext(ctx, query, id).Scan(
		&share.ID,
		&share.SourceID,
		&share.SourceWorkspaceID,
		&share.TargetID,
		&share.TargetWorkspaceID,
		&share.Type,
		&share.Permission,
		&share.Status,
		&share.Description,
		&share.Metadata,
		&share.SharedBy,
		&share.ApprovedBy,
		&share.ExpiresAt,
		&share.CreatedAt,
		&share.UpdatedAt,
		&share.RevokedAt,
		&share.LastAccessedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrShareNotFound
		}
		return nil, fmt.Errorf("failed to get share by ID: %w", err)
	}

	return &share, nil
}

func isValidSharePermission(permission SharePermission) bool {
	switch permission {
	case SharePermissionRead, SharePermissionWrite, SharePermissionAdmin:
		return true
	default:
		return false
	}
}

func isValidShareStatus(status ShareStatus) bool {
	switch status {
	case ShareStatusActive, ShareStatusPending, ShareStatusRevoked, ShareStatusExpired:
		return true
	default:
		return false
	}
}

func isValidShareType(shareType ShareType) bool {
	switch shareType {
	case ShareTypeWorkspace, ShareTypeResource, ShareTypeFolder:
		return true
	default:
		return false
	}
}
