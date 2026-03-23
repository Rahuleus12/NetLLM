// internal/organization/members.go
// Member management with roles and permissions
// Handles organization members, their roles, and access control

package organization

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
	ErrMemberNotFound       = errors.New("member not found")
	ErrMemberAlreadyExists  = errors.New("member already exists")
	ErrInvalidRole          = errors.New("invalid role")
	ErrInvalidMemberStatus  = errors.New("invalid member status")
	ErrCannotRemoveOwner    = errors.New("cannot remove organization owner")
	ErrCannotChangeOwnerRole = errors.New("cannot change owner role")
	ErrPermissionDenied     = errors.New("permission denied")
	ErrInviteNotFound       = errors.New("invitation not found")
	ErrInviteExpired        = errors.New("invitation expired")
	ErrInviteAlreadyAccepted = errors.New("invitation already accepted")
)

// MemberStatus represents the status of a member
type MemberStatus string

const (
	MemberStatusActive    MemberStatus = "active"
	MemberStatusPending   MemberStatus = "pending"
	MemberStatusSuspended MemberStatus = "suspended"
	MemberStatusInvited   MemberStatus = "invited"
)

// MemberRole represents the role of a member in an organization
type MemberRole string

const (
	RoleOwner      MemberRole = "owner"
	RoleAdmin      MemberRole = "admin"
	RoleMember     MemberRole = "member"
	RoleViewer     MemberRole = "viewer"
	RoleDeveloper  MemberRole = "developer"
	RoleBilling    MemberRole = "billing"
)

// Permission represents a specific permission
type Permission string

const (
	// Organization permissions
	PermissionOrgView        Permission = "org:view"
	PermissionOrgEdit        Permission = "org:edit"
	PermissionOrgDelete      Permission = "org:delete"
	PermissionOrgManageSettings Permission = "org:manage_settings"

	// Member permissions
	PermissionMemberView     Permission = "member:view"
	PermissionMemberAdd      Permission = "member:add"
	PermissionMemberRemove   Permission = "member:remove"
	PermissionMemberEdit     Permission = "member:edit"

	// Team permissions
	PermissionTeamView       Permission = "team:view"
	PermissionTeamCreate     Permission = "team:create"
	PermissionTeamEdit       Permission = "team:edit"
	PermissionTeamDelete     Permission = "team:delete"

	// Model permissions
	PermissionModelView      Permission = "model:view"
	PermissionModelUpload    Permission = "model:upload"
	PermissionModelDelete    Permission = "model:delete"
	PermissionModelDeploy    Permission = "model:deploy"

	// Inference permissions
	PermissionInferenceRun   Permission = "inference:run"
	PermissionInferenceView  Permission = "inference:view"
	PermissionInferenceCancel Permission = "inference:cancel"

	// Billing permissions
	PermissionBillingView    Permission = "billing:view"
	PermissionBillingEdit    Permission = "billing:edit"

	// Admin permissions
	PermissionAdminAccess     Permission = "admin:access"
	PermissionAuditLogs      Permission = "audit:logs"
)

// Member represents an organization member
type Member struct {
	ID            uuid.UUID    `json:"id" db:"id"`
	OrganizationID uuid.UUID    `json:"organization_id" db:"organization_id"`
	UserID        uuid.UUID    `json:"user_id" db:"user_id"`
	TeamID        *uuid.UUID   `json:"team_id,omitempty" db:"team_id"`
	Role          MemberRole   `json:"role" db:"role"`
	Status        MemberStatus `json:"status" db:"status"`

	// Profile information
	Email         string       `json:"email" db:"email"`
	FirstName     string       `json:"first_name" db:"first_name"`
	LastName      string       `json:"last_name" db:"last_name"`
	AvatarURL     string       `json:"avatar_url,omitempty" db:"avatar_url"`

	// Timestamps
	JoinedAt      time.Time    `json:"joined_at" db:"joined_at"`
	UpdatedAt     time.Time    `json:"updated_at" db:"updated_at"`
	LastLoginAt   *time.Time   `json:"last_login_at,omitempty" db:"last_login_at"`

	// Invitation tracking
	InvitedBy     *uuid.UUID   `json:"invited_by,omitempty" db:"invited_by"`
	InvitedAt     *time.Time   `json:"invited_at,omitempty" db:"invited_at"`
}

// MemberInvitation represents an invitation to join an organization
type MemberInvitation struct {
	ID             uuid.UUID    `json:"id" db:"id"`
	OrganizationID uuid.UUID    `json:"organization_id" db:"organization_id"`
	Email          string       `json:"email" db:"email"`
	Role           MemberRole   `json:"role" db:"role"`
	TeamID         *uuid.UUID   `json:"team_id,omitempty" db:"team_id"`

	// Invitation details
	InvitedBy      uuid.UUID    `json:"invited_by" db:"invited_by"`
	Token          string       `json:"token" db:"token"`

	// Status and expiration
	Status         string       `json:"status" db:"status"` // pending, accepted, declined, expired
	ExpiresAt      time.Time    `json:"expires_at" db:"expires_at"`
	AcceptedAt     *time.Time   `json:"accepted_at,omitempty" db:"accepted_at"`
	DeclinedAt     *time.Time   `json:"declined_at,omitempty" db:"declined_at"`

	// Timestamps
	CreatedAt      time.Time    `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at" db:"updated_at"`
}

// CreateMemberRequest represents a request to create a new member
type CreateMemberRequest struct {
	UserID        uuid.UUID  `json:"user_id"`
	TeamID        *uuid.UUID `json:"team_id,omitempty"`
	Role          MemberRole `json:"role"`
	Email         string     `json:"email"`
	FirstName     string     `json:"first_name"`
	LastName      string     `json:"last_name"`
}

// UpdateMemberRequest represents a request to update a member
type UpdateMemberRequest struct {
	Role          *MemberRole   `json:"role,omitempty"`
	TeamID        *uuid.UUID    `json:"team_id,omitempty"`
	Status        *MemberStatus `json:"status,omitempty"`
	AvatarURL     *string       `json:"avatar_url,omitempty"`
}

// CreateInvitationRequest represents a request to create an invitation
type CreateInvitationRequest struct {
	Email         string     `json:"email"`
	Role          MemberRole `json:"role"`
	TeamID        *uuid.UUID `json:"team_id,omitempty"`
	ExpiresInDays int        `json:"expires_in_days"`
}

// MemberManager manages organization members
type MemberManager struct {
	db *sql.DB
}

// NewMemberManager creates a new member manager
func NewMemberManager(db *sql.DB) *MemberManager {
	return &MemberManager{
		db: db,
	}
}

// AddMember adds a new member to an organization
func (mm *MemberManager) AddMember(ctx context.Context, orgID uuid.UUID, req CreateMemberRequest, addedBy uuid.UUID) (*Member, error) {
	if req.UserID == uuid.Nil {
		return nil, errors.New("user_id is required")
	}

	if !isValidRole(req.Role) {
		return nil, ErrInvalidRole
	}

	// Check if user is already a member
	existing, err := mm.GetMemberByUserID(ctx, orgID, req.UserID)
	if err == nil && existing != nil {
		return nil, ErrMemberAlreadyExists
	}

	member := &Member{
		ID:            uuid.New(),
		OrganizationID: orgID,
		UserID:        req.UserID,
		TeamID:        req.TeamID,
		Role:          req.Role,
		Status:        MemberStatusActive,
		Email:         req.Email,
		FirstName:     req.FirstName,
		LastName:      req.LastName,
		JoinedAt:      time.Now(),
		UpdatedAt:     time.Now(),
	}

	query := `
		INSERT INTO members (id, organization_id, user_id, team_id, role, status,
			email, first_name, last_name, avatar_url, joined_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id, organization_id, user_id, team_id, role, status,
			email, first_name, last_name, avatar_url, joined_at, updated_at,
			last_login_at, invited_by, invited_at
	`

	err = mm.db.QueryRowContext(ctx, query,
		member.ID,
		member.OrganizationID,
		member.UserID,
		member.TeamID,
		member.Role,
		member.Status,
		member.Email,
		member.FirstName,
		member.LastName,
		member.AvatarURL,
		member.JoinedAt,
		member.UpdatedAt,
	).Scan(
		&member.ID,
		&member.OrganizationID,
		&member.UserID,
		&member.TeamID,
		&member.Role,
		&member.Status,
		&member.Email,
		&member.FirstName,
		&member.LastName,
		&member.AvatarURL,
		&member.JoinedAt,
		&member.UpdatedAt,
		&member.LastLoginAt,
		&member.InvitedBy,
		&member.InvitedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to add member: %w", err)
	}

	return member, nil
}

// GetMember retrieves a member by ID
func (mm *MemberManager) GetMember(ctx context.Context, memberID uuid.UUID) (*Member, error) {
	if memberID == uuid.Nil {
		return nil, errors.New("invalid member ID")
	}

	var member Member

	query := `
		SELECT id, organization_id, user_id, team_id, role, status,
			email, first_name, last_name, avatar_url, joined_at, updated_at,
			last_login_at, invited_by, invited_at
		FROM members
		WHERE id = $1 AND deleted_at IS NULL
	`

	err := mm.db.QueryRowContext(ctx, query, memberID).Scan(
		&member.ID,
		&member.OrganizationID,
		&member.UserID,
		&member.TeamID,
		&member.Role,
		&member.Status,
		&member.Email,
		&member.FirstName,
		&member.LastName,
		&member.AvatarURL,
		&member.JoinedAt,
		&member.UpdatedAt,
		&member.LastLoginAt,
		&member.InvitedBy,
		&member.InvitedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrMemberNotFound
		}
		return nil, fmt.Errorf("failed to get member: %w", err)
	}

	return &member, nil
}

// GetMemberByUserID retrieves a member by user ID and organization
func (mm *MemberManager) GetMemberByUserID(ctx context.Context, orgID, userID uuid.UUID) (*Member, error) {
	var member Member

	query := `
		SELECT id, organization_id, user_id, team_id, role, status,
			email, first_name, last_name, avatar_url, joined_at, updated_at,
			last_login_at, invited_by, invited_at
		FROM members
		WHERE organization_id = $1 AND user_id = $2 AND deleted_at IS NULL
	`

	err := mm.db.QueryRowContext(ctx, query, orgID, userID).Scan(
		&member.ID,
		&member.OrganizationID,
		&member.UserID,
		&member.TeamID,
		&member.Role,
		&member.Status,
		&member.Email,
		&member.FirstName,
		&member.LastName,
		&member.AvatarURL,
		&member.JoinedAt,
		&member.UpdatedAt,
		&member.LastLoginAt,
		&member.InvitedBy,
		&member.InvitedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrMemberNotFound
		}
		return nil, fmt.Errorf("failed to get member by user ID: %w", err)
	}

	return &member, nil
}

// ListMembers lists all members of an organization
func (mm *MemberManager) ListMembers(ctx context.Context, orgID uuid.UUID, role *MemberRole, teamID *uuid.UUID) ([]*Member, error) {
	query := `
		SELECT id, organization_id, user_id, team_id, role, status,
			email, first_name, last_name, avatar_url, joined_at, updated_at,
			last_login_at, invited_by, invited_at
		FROM members
		WHERE organization_id = $1 AND deleted_at IS NULL
	`
	args := []interface{}{orgID}
	argPos := 2

	if role != nil {
		query += fmt.Sprintf(" AND role = $%d", argPos)
		args = append(args, *role)
		argPos++
	}

	if teamID != nil {
		query += fmt.Sprintf(" AND team_id = $%d", argPos)
		args = append(args, *teamID)
		argPos++
	}

	query += " ORDER BY joined_at ASC"

	rows, err := mm.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list members: %w", err)
	}
	defer rows.Close()

	var members []*Member
	for rows.Next() {
		var member Member

		err := rows.Scan(
			&member.ID,
			&member.OrganizationID,
			&member.UserID,
			&member.TeamID,
			&member.Role,
			&member.Status,
			&member.Email,
			&member.FirstName,
			&member.LastName,
			&member.AvatarURL,
			&member.JoinedAt,
			&member.UpdatedAt,
			&member.LastLoginAt,
			&member.InvitedBy,
			&member.InvitedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan member: %w", err)
		}

		members = append(members, &member)
	}

	return members, nil
}

// UpdateMember updates a member's information
func (mm *MemberManager) UpdateMember(ctx context.Context, memberID uuid.UUID, req UpdateMemberRequest) (*Member, error) {
	member, err := mm.GetMember(ctx, memberID)
	if err != nil {
		return nil, err
	}

	// Cannot change owner role
	if req.Role != nil && member.Role == RoleOwner {
		return nil, ErrCannotChangeOwnerRole
	}

	// Apply updates
	if req.Role != nil {
		if !isValidRole(*req.Role) {
			return nil, ErrInvalidRole
		}
		member.Role = *req.Role
	}
	if req.TeamID != nil {
		member.TeamID = req.TeamID
	}
	if req.Status != nil {
		if !isValidMemberStatus(*req.Status) {
			return nil, ErrInvalidMemberStatus
		}
		member.Status = *req.Status
	}
	if req.AvatarURL != nil {
		member.AvatarURL = *req.AvatarURL
	}
	member.UpdatedAt = time.Now()

	query := `
		UPDATE members
		SET role = $1, team_id = $2, status = $3, avatar_url = $4, updated_at = $5
		WHERE id = $6
		RETURNING id, organization_id, user_id, team_id, role, status,
			email, first_name, last_name, avatar_url, joined_at, updated_at,
			last_login_at, invited_by, invited_at
	`

	err = mm.db.QueryRowContext(ctx, query,
		member.Role,
		member.TeamID,
		member.Status,
		member.AvatarURL,
		member.UpdatedAt,
		member.ID,
	).Scan(
		&member.ID,
		&member.OrganizationID,
		&member.UserID,
		&member.TeamID,
		&member.Role,
		&member.Status,
		&member.Email,
		&member.FirstName,
		&member.LastName,
		&member.AvatarURL,
		&member.JoinedAt,
		&member.UpdatedAt,
		&member.LastLoginAt,
		&member.InvitedBy,
		&member.InvitedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to update member: %w", err)
	}

	return member, nil
}

// RemoveMember removes a member from an organization
func (mm *MemberManager) RemoveMember(ctx context.Context, memberID uuid.UUID) error {
	member, err := mm.GetMember(ctx, memberID)
	if err != nil {
		return err
	}

	// Cannot remove owner
	if member.Role == RoleOwner {
		return ErrCannotRemoveOwner
	}

	query := `
		UPDATE members
		SET deleted_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
	`

	result, err := mm.db.ExecContext(ctx, query, memberID)
	if err != nil {
		return fmt.Errorf("failed to remove member: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return ErrMemberNotFound
	}

	return nil
}

// InviteMember invites a user to join an organization
func (mm *MemberManager) InviteMember(ctx context.Context, orgID uuid.UUID, req CreateInvitationRequest, invitedBy uuid.UUID) (*MemberInvitation, error) {
	if req.Email == "" {
		return nil, errors.New("email is required")
	}

	if !isValidRole(req.Role) {
		return nil, ErrInvalidRole
	}

	expiresIn := time.Duration(req.ExpiresInDays) * 24 * time.Hour
	if expiresIn == 0 {
		expiresIn = 7 * 24 * time.Hour // Default 7 days
	}

	token := generateInvitationToken()

	invitation := &MemberInvitation{
		ID:             uuid.New(),
		OrganizationID: orgID,
		Email:          req.Email,
		Role:           req.Role,
		TeamID:         req.TeamID,
		InvitedBy:      invitedBy,
		Token:          token,
		Status:         "pending",
		ExpiresAt:      time.Now().Add(expiresIn),
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	query := `
		INSERT INTO member_invitations (id, organization_id, email, role, team_id,
			invited_by, token, status, expires_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id, organization_id, email, role, team_id, invited_by, token,
			status, expires_at, accepted_at, declined_at, created_at, updated_at
	`

	err := mm.db.QueryRowContext(ctx, query,
		invitation.ID,
		invitation.OrganizationID,
		invitation.Email,
		invitation.Role,
		invitation.TeamID,
		invitation.InvitedBy,
		invitation.Token,
		invitation.Status,
		invitation.ExpiresAt,
		invitation.CreatedAt,
		invitation.UpdatedAt,
	).Scan(
		&invitation.ID,
		&invitation.OrganizationID,
		&invitation.Email,
		&invitation.Role,
		&invitation.TeamID,
		&invitation.InvitedBy,
		&invitation.Token,
		&invitation.Status,
		&invitation.ExpiresAt,
		&invitation.AcceptedAt,
		&invitation.DeclinedAt,
		&invitation.CreatedAt,
		&invitation.UpdatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create invitation: %w", err)
	}

	return invitation, nil
}

// AcceptInvitation accepts an invitation to join an organization
func (mm *MemberManager) AcceptInvitation(ctx context.Context, token string, userID uuid.UUID) (*Member, error) {
	invitation, err := mm.GetInvitationByToken(ctx, token)
	if err != nil {
		return nil, err
	}

	if invitation.Status != "pending" {
		return nil, ErrInviteAlreadyAccepted
	}

	if time.Now().After(invitation.ExpiresAt) {
		return nil, ErrInviteExpired
	}

	// Create member
	member := &Member{
		ID:            uuid.New(),
		OrganizationID: invitation.OrganizationID,
		UserID:        userID,
		TeamID:        invitation.TeamID,
		Role:          invitation.Role,
		Status:        MemberStatusActive,
		Email:         invitation.Email,
		FirstName:     "", // Will be filled from user profile
		LastName:      "",
		JoinedAt:      time.Now(),
		UpdatedAt:     time.Now(),
		InvitedBy:     &invitation.InvitedBy,
		InvitedAt:     &invitation.CreatedAt,
	}

	// Start transaction
	tx, err := mm.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	// Create member
	query := `
		INSERT INTO members (id, organization_id, user_id, team_id, role, status,
			email, first_name, last_name, avatar_url, joined_at, updated_at,
			invited_by, invited_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		RETURNING id, organization_id, user_id, team_id, role, status,
			email, first_name, last_name, avatar_url, joined_at, updated_at,
			last_login_at, invited_by, invited_at
	`

	err = tx.QueryRowContext(ctx, query,
		member.ID,
		member.OrganizationID,
		member.UserID,
		member.TeamID,
		member.Role,
		member.Status,
		member.Email,
		member.FirstName,
		member.LastName,
		member.AvatarURL,
		member.JoinedAt,
		member.UpdatedAt,
		member.InvitedBy,
		member.InvitedAt,
	).Scan(
		&member.ID,
		&member.OrganizationID,
		&member.UserID,
		&member.TeamID,
		&member.Role,
		&member.Status,
		&member.Email,
		&member.FirstName,
		&member.LastName,
		&member.AvatarURL,
		&member.JoinedAt,
		&member.UpdatedAt,
		&member.LastLoginAt,
		member.InvitedBy,
		member.InvitedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create member from invitation: %w", err)
	}

	// Update invitation status
	now := time.Now()
	_, err = tx.ExecContext(ctx, `
		UPDATE member_invitations
		SET status = 'accepted', accepted_at = $1, updated_at = $2
		WHERE id = $3
	`, now, now, invitation.ID)

	if err != nil {
		return nil, fmt.Errorf("failed to update invitation: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return member, nil
}

// GetInvitationByToken retrieves an invitation by token
func (mm *MemberManager) GetInvitationByToken(ctx context.Context, token string) (*MemberInvitation, error) {
	var invitation MemberInvitation

	query := `
		SELECT id, organization_id, email, role, team_id, invited_by, token,
			status, expires_at, accepted_at, declined_at, created_at, updated_at
		FROM member_invitations
		WHERE token = $1 AND deleted_at IS NULL
	`

	err := mm.db.QueryRowContext(ctx, query, token).Scan(
		&invitation.ID,
		&invitation.OrganizationID,
		&invitation.Email,
		&invitation.Role,
		&invitation.TeamID,
		&invitation.InvitedBy,
		&invitation.Token,
		&invitation.Status,
		&invitation.ExpiresAt,
		&invitation.AcceptedAt,
		&invitation.DeclinedAt,
		&invitation.CreatedAt,
		&invitation.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrInviteNotFound
		}
		return nil, fmt.Errorf("failed to get invitation: %w", err)
	}

	return &invitation, nil
}

// HasPermission checks if a member has a specific permission
func (mm *MemberManager) HasPermission(ctx context.Context, memberID uuid.UUID, permission Permission) (bool, error) {
	member, err := mm.GetMember(ctx, memberID)
	if err != nil {
		return false, err
	}

	// Get permissions for role
	permissions := getPermissionsForRole(member.Role)

	for _, p := range permissions {
		if p == permission {
			return true, nil
		}
	}

	return false, nil
}

// HasAnyPermission checks if a member has any of the specified permissions
func (mm *MemberManager) HasAnyPermission(ctx context.Context, memberID uuid.UUID, permissions []Permission) (bool, error) {
	member, err := mm.GetMember(ctx, memberID)
	if err != nil {
		return false, err
	}

	// Get permissions for role
	rolePermissions := getPermissionsForRole(member.Role)

	rolePermMap := make(map[Permission]bool)
	for _, p := range rolePermissions {
		rolePermMap[p] = true
	}

	for _, p := range permissions {
		if rolePermMap[p] {
			return true, nil
		}
	}

	return false, nil
}

// GetMemberPermissions returns all permissions for a member
func (mm *MemberManager) GetMemberPermissions(ctx context.Context, memberID uuid.UUID) ([]Permission, error) {
	member, err := mm.GetMember(ctx, memberID)
	if err != nil {
		return nil, err
	}

	return getPermissionsForRole(member.Role), nil
}

// isValidRole checks if a role is valid
func isValidRole(role MemberRole) bool {
	switch role {
	case RoleOwner, RoleAdmin, RoleMember, RoleViewer, RoleDeveloper, RoleBilling:
		return true
	default:
		return false
	}
}

// isValidMemberStatus checks if a member status is valid
func isValidMemberStatus(status MemberStatus) bool {
	switch status {
	case MemberStatusActive, MemberStatusPending, MemberStatusSuspended, MemberStatusInvited:
		return true
	default:
		return false
	}
}

// getPermissionsForRole returns all permissions for a given role
func getPermissionsForRole(role MemberRole) []Permission {
	switch role {
	case RoleOwner:
		return []Permission{
			PermissionOrgView, PermissionOrgEdit, PermissionOrgDelete, PermissionOrgManageSettings,
			PermissionMemberView, PermissionMemberAdd, PermissionMemberRemove, PermissionMemberEdit,
			PermissionTeamView, PermissionTeamCreate, PermissionTeamEdit, PermissionTeamDelete,
			PermissionModelView, PermissionModelUpload, PermissionModelDelete, PermissionModelDeploy,
			PermissionInferenceRun, PermissionInferenceView, PermissionInferenceCancel,
			PermissionBillingView, PermissionBillingEdit,
			PermissionAdminAccess, PermissionAuditLogs,
		}
	case RoleAdmin:
		return []Permission{
			PermissionOrgView, PermissionOrgEdit, PermissionOrgManageSettings,
			PermissionMemberView, PermissionMemberAdd, PermissionMemberRemove, PermissionMemberEdit,
			PermissionTeamView, PermissionTeamCreate, PermissionTeamEdit, PermissionTeamDelete,
			PermissionModelView, PermissionModelUpload, PermissionModelDelete, PermissionModelDeploy,
			PermissionInferenceRun, PermissionInferenceView, PermissionInferenceCancel,
			PermissionBillingView, PermissionBillingEdit,
			PermissionAdminAccess, PermissionAuditLogs,
		}
	case RoleDeveloper:
		return []Permission{
			PermissionOrgView,
			PermissionMemberView,
			PermissionTeamView,
			PermissionModelView, PermissionModelUpload, PermissionModelDeploy,
			PermissionInferenceRun, PermissionInferenceView, PermissionInferenceCancel,
		}
	case RoleBilling:
		return []Permission{
			PermissionOrgView,
			PermissionMemberView,
			PermissionTeamView,
			PermissionBillingView, PermissionBillingEdit,
		}
	case RoleMember:
		return []Permission{
			PermissionOrgView,
			PermissionMemberView,
			PermissionTeamView,
			PermissionModelView,
			PermissionInferenceRun, PermissionInferenceView,
		}
	case RoleViewer:
		return []Permission{
			PermissionOrgView,
			PermissionMemberView,
			PermissionTeamView,
			PermissionModelView,
			PermissionInferenceView,
		}
	default:
		return []Permission{}
	}
}

// generateInvitationToken generates a unique invitation token
func generateInvitationToken() string {
	return uuid.New().String()
}

// UpdateLastLogin updates the last login time for a member
func (mm *MemberManager) UpdateLastLogin(ctx context.Context, memberID uuid.UUID) error {
	query := `
		UPDATE members
		SET last_login_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
	`

	_, err := mm.db.ExecContext(ctx, query, memberID)
	if err != nil {
		return fmt.Errorf("failed to update last login: %w", err)
	}

	return nil
}

// GetMemberCount returns the count of active members in an organization
func (mm *MemberManager) GetMemberCount(ctx context.Context, orgID uuid.UUID) (int, error) {
	var count int

	query := `
		SELECT COUNT(*)
		FROM members
		WHERE organization_id = $1 AND status = 'active' AND deleted_at IS NULL
	`

	err := mm.db.QueryRowContext(ctx, query, orgID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to get member count: %w", err)
	}

	return count, nil
}
