// internal/organization/teams.go
// Team management within organizations
// Handles team creation, member management, and team-level permissions

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
	ErrTeamNotFound       = errors.New("team not found")
	ErrTeamAlreadyExists  = errors.New("team already exists")
	ErrInvalidTeam        = errors.New("invalid team data")
	ErrTeamCannotDelete   = errors.New("cannot delete team with active members")
	ErrMemberAlreadyInTeam = errors.New("member already in team")
	ErrMemberNotInTeam    = errors.New("member not in team")
)

// TeamStatus represents the status of a team
type TeamStatus string

const (
	TeamStatusActive    TeamStatus = "active"
	TeamStatusArchived TeamStatus = "archived"
	TeamStatusDeleted  TeamStatus = "deleted"
)

// Team represents a team within an organization
type Team struct {
	ID             uuid.UUID    `json:"id" db:"id"`
	OrganizationID uuid.UUID    `json:"organization_id" db:"organization_id"`
	Name           string       `json:"name" db:"name"`
	Slug           string       `json:"slug" db:"slug"`
	Description    string       `json:"description" db:"description"`
	Status         TeamStatus   `json:"status" db:"status"`

	// Team settings
	Settings       TeamSettings `json:"settings" db:"settings"`
	ParentTeamID   *uuid.UUID   `json:"parent_team_id,omitempty" db:"parent_team_id"`

	// Contact information
	ContactEmail   string       `json:"contact_email,omitempty" db:"contact_email"`
	ContactName    string       `json:"contact_name,omitempty" db:"contact_name"`

	// Timestamps
	CreatedAt      time.Time    `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at" db:"updated_at"`
	DeletedAt      *time.Time   `json:"deleted_at,omitempty" db:"deleted_at"`
}

// TeamSettings represents team-level settings
type TeamSettings struct {
	// Visibility
	IsPrivate      bool   `json:"is_private"`

	// Permissions
	AllowSelfJoin  bool   `json:"allow_self_join"`
	RequireApproval bool  `json:"require_approval"`

	// Defaults
	DefaultRole    MemberRole `json:"default_role"`

	// Limits
	MaxMembers     int    `json:"max_members"`
	MaxSubteams    int    `json:"max_subteams"`

	// Features
	Features       map[string]bool `json:"features"`

	// Custom settings
	CustomSettings map[string]interface{} `json:"custom_settings"`
}

// TeamMember represents a member's association with a team
type TeamMember struct {
	ID             uuid.UUID  `json:"id" db:"id"`
	TeamID         uuid.UUID  `json:"team_id" db:"team_id"`
	MemberID       uuid.UUID  `json:"member_id" db:"member_id"`
	Role           MemberRole `json:"role" db:"role"`

	// Status and metadata
	Status         string     `json:"status" db:"status"` // active, pending, removed
	JoinedAt       time.Time  `json:"joined_at" db:"joined_at"`
	UpdatedAt      time.Time  `json:"updated_at" db:"updated_at"`
	LeftAt         *time.Time `json:"left_at,omitempty" db:"left_at"`

	// Invitation tracking
	InvitedBy      *uuid.UUID `json:"invited_by,omitempty" db:"invited_by"`
	InvitedAt      *time.Time `json:"invited_at,omitempty" db:"invited_at"`
}

// CreateTeamRequest represents a request to create a new team
type CreateTeamRequest struct {
	OrganizationID uuid.UUID  `json:"organization_id"`
	Name           string     `json:"name"`
	Slug           string     `json:"slug"`
	Description    string     `json:"description,omitempty"`
	ParentTeamID   *uuid.UUID `json:"parent_team_id,omitempty"`
	ContactEmail   string     `json:"contact_email,omitempty"`
	ContactName    string     `json:"contact_name,omitempty"`
	Settings       TeamSettings `json:"settings,omitempty"`
}

// UpdateTeamRequest represents a request to update a team
type UpdateTeamRequest struct {
	Name          *string     `json:"name,omitempty"`
	Slug          *string     `json:"slug,omitempty"`
	Description   *string     `json:"description,omitempty"`
	Status        *TeamStatus `json:"status,omitempty"`
	ContactEmail  *string     `json:"contact_email,omitempty"`
	ContactName   *string     `json:"contact_name,omitempty"`
	Settings      *TeamSettings `json:"settings,omitempty"`
}

// AddTeamMemberRequest represents a request to add a member to a team
type AddTeamMemberRequest struct {
	MemberID     uuid.UUID  `json:"member_id"`
	Role         MemberRole `json:"role"`
}

// ListTeamsOptions represents options for listing teams
type ListTeamsOptions struct {
	OrganizationID *uuid.UUID
	ParentTeamID  *uuid.UUID
	Status        *TeamStatus
	Limit         int
	Offset        int
	Search        string
}

// TeamManager manages teams within organizations
type TeamManager struct {
	db *sql.DB
}

// NewTeamManager creates a new team manager
func NewTeamManager(db *sql.DB) *TeamManager {
	return &TeamManager{
		db: db,
	}
}

// CreateTeam creates a new team
func (tm *TeamManager) CreateTeam(ctx context.Context, req CreateTeamRequest) (*Team, error) {
	if req.OrganizationID == uuid.Nil {
		return nil, ErrInvalidTeam
	}
	if req.Name == "" {
		return nil, ErrInvalidTeam
	}
	if req.Slug == "" {
		return nil, ErrInvalidTeam
	}

	// Check if team with slug already exists in this organization
	existing, err := tm.GetTeamBySlug(ctx, req.OrganizationID, req.Slug)
	if err == nil && existing != nil {
		return nil, ErrTeamAlreadyExists
	}

	// Apply default settings if not provided
	settings := req.Settings
	if settings.Features == nil {
		settings.Features = make(map[string]bool)
	}
	if settings.CustomSettings == nil {
		settings.CustomSettings = make(map[string]interface{})
	}

	team := &Team{
		ID:             uuid.New(),
		OrganizationID: req.OrganizationID,
		Name:           req.Name,
		Slug:           req.Slug,
		Description:    req.Description,
		Status:         TeamStatusActive,
		Settings:       settings,
		ParentTeamID:   req.ParentTeamID,
		ContactEmail:   req.ContactEmail,
		ContactName:    req.ContactName,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	settingsJSON, err := json.Marshal(team.Settings)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal settings: %w", err)
	}

	query := `
		INSERT INTO teams (id, organization_id, name, slug, description, status,
			settings, parent_team_id, contact_email, contact_name, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id, organization_id, name, slug, description, status, settings,
			parent_team_id, contact_email, contact_name, created_at, updated_at, deleted_at
	`

	err = tm.db.QueryRowContext(ctx, query,
		team.ID,
		team.OrganizationID,
		team.Name,
		team.Slug,
		team.Description,
		team.Status,
		settingsJSON,
		team.ParentTeamID,
		team.ContactEmail,
		team.ContactName,
		team.CreatedAt,
		team.UpdatedAt,
	).Scan(
		&team.ID,
		&team.OrganizationID,
		&team.Name,
		&team.Slug,
		&team.Description,
		&team.Status,
		&settingsJSON,
		&team.ParentTeamID,
		&team.ContactEmail,
		&team.ContactName,
		&team.CreatedAt,
		&team.UpdatedAt,
		&team.DeletedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create team: %w", err)
	}

	// Unmarshal settings
	json.Unmarshal(settingsJSON, &team.Settings)

	return team, nil
}

// GetTeam retrieves a team by ID
func (tm *TeamManager) GetTeam(ctx context.Context, id uuid.UUID) (*Team, error) {
	if id == uuid.Nil {
		return nil, ErrInvalidTeam
	}

	var team Team
	var settingsJSON []byte

	query := `
		SELECT id, organization_id, name, slug, description, status, settings,
			parent_team_id, contact_email, contact_name, created_at, updated_at, deleted_at
		FROM teams
		WHERE id = $1 AND deleted_at IS NULL
	`

	err := tm.db.QueryRowContext(ctx, query, id).Scan(
		&team.ID,
		&team.OrganizationID,
		&team.Name,
		&team.Slug,
		&team.Description,
		&team.Status,
		&settingsJSON,
		&team.ParentTeamID,
		&team.ContactEmail,
		&team.ContactName,
		&team.CreatedAt,
		&team.UpdatedAt,
		&team.DeletedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrTeamNotFound
		}
		return nil, fmt.Errorf("failed to get team: %w", err)
	}

	// Unmarshal settings
	if settingsJSON != nil {
		json.Unmarshal(settingsJSON, &team.Settings)
	}

	return &team, nil
}

// GetTeamBySlug retrieves a team by organization ID and slug
func (tm *TeamManager) GetTeamBySlug(ctx context.Context, orgID uuid.UUID, slug string) (*Team, error) {
	var team Team
	var settingsJSON []byte

	query := `
		SELECT id, organization_id, name, slug, description, status, settings,
			parent_team_id, contact_email, contact_name, created_at, updated_at, deleted_at
		FROM teams
		WHERE organization_id = $1 AND slug = $2 AND deleted_at IS NULL
	`

	err := tm.db.QueryRowContext(ctx, query, orgID, slug).Scan(
		&team.ID,
		&team.OrganizationID,
		&team.Name,
		&team.Slug,
		&team.Description,
		&team.Status,
		&settingsJSON,
		&team.ParentTeamID,
		&team.ContactEmail,
		&team.ContactName,
		&team.CreatedAt,
		&team.UpdatedAt,
		&team.DeletedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrTeamNotFound
		}
		return nil, fmt.Errorf("failed to get team by slug: %w", err)
	}

	if settingsJSON != nil {
		json.Unmarshal(settingsJSON, &team.Settings)
	}

	return &team, nil
}

// UpdateTeam updates a team
func (tm *TeamManager) UpdateTeam(ctx context.Context, id uuid.UUID, req UpdateTeamRequest) (*Team, error) {
	if id == uuid.Nil {
		return nil, ErrInvalidTeam
	}

	team, err := tm.GetTeam(ctx, id)
	if err != nil {
		return nil, err
	}

	// Apply updates
	if req.Name != nil {
		team.Name = *req.Name
	}
	if req.Slug != nil {
		team.Slug = *req.Slug
	}
	if req.Description != nil {
		team.Description = *req.Description
	}
	if req.Status != nil {
		if !isValidTeamStatus(*req.Status) {
			return nil, errors.New("invalid team status")
		}
		team.Status = *req.Status
	}
	if req.ContactEmail != nil {
		team.ContactEmail = *req.ContactEmail
	}
	if req.ContactName != nil {
		team.ContactName = *req.ContactName
	}
	if req.Settings != nil {
		team.Settings = *req.Settings
	}
	team.UpdatedAt = time.Now()

	settingsJSON, err := json.Marshal(team.Settings)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal settings: %w", err)
	}

	query := `
		UPDATE teams
		SET name = $1, slug = $2, description = $3, status = $4, settings = $5,
			contact_email = $6, contact_name = $7, updated_at = $8
		WHERE id = $9
		RETURNING id, organization_id, name, slug, description, status, settings,
			parent_team_id, contact_email, contact_name, created_at, updated_at, deleted_at
	`

	err = tm.db.QueryRowContext(ctx, query,
		team.Name,
		team.Slug,
		team.Description,
		team.Status,
		settingsJSON,
		team.ContactEmail,
		team.ContactName,
		team.UpdatedAt,
		team.ID,
	).Scan(
		&team.ID,
		&team.OrganizationID,
		&team.Name,
		&team.Slug,
		&team.Description,
		&team.Status,
		&settingsJSON,
		&team.ParentTeamID,
		&team.ContactEmail,
		&team.ContactName,
		&team.CreatedAt,
		&team.UpdatedAt,
		&team.DeletedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to update team: %w", err)
	}

	json.Unmarshal(settingsJSON, &team.Settings)

	return team, nil
}

// DeleteTeam deletes a team
func (tm *TeamManager) DeleteTeam(ctx context.Context, id uuid.UUID) error {
	if id == uuid.Nil {
		return ErrInvalidTeam
	}

	// Check if team has active members
	members, err := tm.ListTeamMembers(ctx, id)
	if err == nil && len(members) > 0 {
		return ErrTeamCannotDelete
	}

	// Soft delete
	query := `
		UPDATE teams
		SET deleted_at = CURRENT_TIMESTAMP, status = 'deleted', updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
	`

	result, err := tm.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete team: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return ErrTeamNotFound
	}

	return nil
}

// ListTeams lists teams with optional filters
func (tm *TeamManager) ListTeams(ctx context.Context, opts ListTeamsOptions) ([]*Team, int64, error) {
	teams := []*Team{}
	var total int64

	// Build query with dynamic filters
	baseQuery := `
		SELECT id, organization_id, name, slug, description, status, settings,
			parent_team_id, contact_email, contact_name, created_at, updated_at, deleted_at
		FROM teams
		WHERE deleted_at IS NULL
	`
	countQuery := `
		SELECT COUNT(*)
		FROM teams
		WHERE deleted_at IS NULL
	`

	args := []interface{}{}
	argPos := 1

	if opts.OrganizationID != nil {
		baseQuery += fmt.Sprintf(" AND organization_id = $%d", argPos)
		countQuery += fmt.Sprintf(" AND organization_id = $%d", argPos)
		args = append(args, *opts.OrganizationID)
		argPos++
	}

	if opts.ParentTeamID != nil {
		baseQuery += fmt.Sprintf(" AND parent_team_id = $%d", argPos)
		countQuery += fmt.Sprintf(" AND parent_team_id = $%d", argPos)
		args = append(args, *opts.ParentTeamID)
		argPos++
	}

	if opts.Status != nil {
		baseQuery += fmt.Sprintf(" AND status = $%d", argPos)
		countQuery += fmt.Sprintf(" AND status = $%d", argPos)
		args = append(args, *opts.Status)
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
		return nil, 0, fmt.Errorf("failed to count teams: %w", err)
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

	rows, err := tm.db.QueryContext(ctx, baseQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list teams: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var team Team
		var settingsJSON []byte

		err := rows.Scan(
			&team.ID,
			&team.OrganizationID,
			&team.Name,
			&team.Slug,
			&team.Description,
			&team.Status,
			&settingsJSON,
			&team.ParentTeamID,
			&team.ContactEmail,
			&team.ContactName,
			&team.CreatedAt,
			&team.UpdatedAt,
			&team.DeletedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan team: %w", err)
		}

		if settingsJSON != nil {
			json.Unmarshal(settingsJSON, &team.Settings)
		}

		teams = append(teams, &team)
	}

	if err = rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating teams: %w", err)
	}

	return teams, total, nil
}

// GetTeamsByOrganization retrieves all teams for an organization
func (tm *TeamManager) GetTeamsByOrganization(ctx context.Context, orgID uuid.UUID) ([]*Team, error) {
	if orgID == uuid.Nil {
		return nil, ErrInvalidTeam
	}

	var teams []*Team

	query := `
		SELECT id, organization_id, name, slug, description, status, settings,
			parent_team_id, contact_email, contact_name, created_at, updated_at, deleted_at
		FROM teams
		WHERE organization_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
	`

	rows, err := tm.db.QueryContext(ctx, query, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to get teams by organization: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var team Team
		var settingsJSON []byte

		err := rows.Scan(
			&team.ID,
			&team.OrganizationID,
			&team.Name,
			&team.Slug,
			&team.Description,
			&team.Status,
			&settingsJSON,
			&team.ParentTeamID,
			&team.ContactEmail,
			&team.ContactName,
			&team.CreatedAt,
			&team.UpdatedAt,
			&team.DeletedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan team: %w", err)
		}

		if settingsJSON != nil {
			json.Unmarshal(settingsJSON, &team.Settings)
		}

		teams = append(teams, &team)
	}

	return teams, nil
}

// AddMemberToTeam adds a member to a team
func (tm *TeamManager) AddMemberToTeam(ctx context.Context, teamID, memberID uuid.UUID, req AddTeamMemberRequest, addedBy uuid.UUID) (*TeamMember, error) {
	if teamID == uuid.Nil || memberID == uuid.Nil {
		return nil, ErrInvalidTeam
	}

	if !isValidRole(req.Role) {
		return nil, ErrInvalidRole
	}

	// Check if member is already in team
	existing, err := tm.GetTeamMember(ctx, teamID, memberID)
	if err == nil && existing != nil {
		return nil, ErrMemberAlreadyInTeam
	}

	teamMember := &TeamMember{
		ID:        uuid.New(),
		TeamID:    teamID,
		MemberID:  memberID,
		Role:      req.Role,
		Status:    "active",
		JoinedAt:  time.Now(),
		UpdatedAt: time.Now(),
		InvitedBy: &addedBy,
		InvitedAt: &time.Now(),
	}

	query := `
		INSERT INTO team_members (id, team_id, member_id, role, status,
			joined_at, updated_at, invited_by, invited_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, team_id, member_id, role, status, joined_at, updated_at, left_at, invited_by, invited_at
	`

	err = tm.db.QueryRowContext(ctx, query,
		teamMember.ID,
		teamMember.TeamID,
		teamMember.MemberID,
		teamMember.Role,
		teamMember.Status,
		teamMember.JoinedAt,
		teamMember.UpdatedAt,
		teamMember.InvitedBy,
		teamMember.InvitedAt,
	).Scan(
		&teamMember.ID,
		&teamMember.TeamID,
		&teamMember.MemberID,
		&teamMember.Role,
		&teamMember.Status,
		&teamMember.JoinedAt,
		&teamMember.UpdatedAt,
		&teamMember.LeftAt,
		&teamMember.InvitedBy,
		&teamMember.InvitedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to add member to team: %w", err)
	}

	return teamMember, nil
}

// GetTeamMember retrieves a team member
func (tm *TeamManager) GetTeamMember(ctx context.Context, teamID, memberID uuid.UUID) (*TeamMember, error) {
	var teamMember TeamMember

	query := `
		SELECT id, team_id, member_id, role, status, joined_at, updated_at, left_at, invited_by, invited_at
		FROM team_members
		WHERE team_id = $1 AND member_id = $2 AND status != 'removed'
	`

	err := tm.db.QueryRowContext(ctx, query, teamID, memberID).Scan(
		&teamMember.ID,
		&teamMember.TeamID,
		&teamMember.MemberID,
		&teamMember.Role,
		&teamMember.Status,
		&teamMember.JoinedAt,
		&teamMember.UpdatedAt,
		&teamMember.LeftAt,
		&teamMember.InvitedBy,
		&teamMember.InvitedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrMemberNotInTeam
		}
		return nil, fmt.Errorf("failed to get team member: %w", err)
	}

	return &teamMember, nil
}

// ListTeamMembers lists all members of a team
func (tm *TeamManager) ListTeamMembers(ctx context.Context, teamID uuid.UUID) ([]*TeamMember, error) {
	var members []*TeamMember

	query := `
		SELECT id, team_id, member_id, role, status, joined_at, updated_at, left_at, invited_by, invited_at
		FROM team_members
		WHERE team_id = $1 AND status != 'removed'
		ORDER BY joined_at ASC
	`

	rows, err := tm.db.QueryContext(ctx, query, teamID)
	if err != nil {
		return nil, fmt.Errorf("failed to list team members: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var member TeamMember

		err := rows.Scan(
			&member.ID,
			&member.TeamID,
			&member.MemberID,
			&member.Role,
			&member.Status,
			&member.JoinedAt,
			&member.UpdatedAt,
			&member.LeftAt,
			&member.InvitedBy,
			&member.InvitedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan team member: %w", err)
		}

		members = append(members, &member)
	}

	return members, nil
}

// RemoveMemberFromTeam removes a member from a team
func (tm *TeamManager) RemoveMemberFromTeam(ctx context.Context, teamID, memberID uuid.UUID) error {
	query := `
		UPDATE team_members
		SET status = 'removed', left_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		WHERE team_id = $1 AND member_id = $2
	`

	result, err := tm.db.ExecContext(ctx, query, teamID, memberID)
	if err != nil {
		return fmt.Errorf("failed to remove member from team: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return ErrMemberNotInTeam
	}

	return nil
}

// UpdateTeamMemberRole updates a member's role in a team
func (tm *TeamManager) UpdateTeamMemberRole(ctx context.Context, teamID, memberID uuid.UUID, newRole MemberRole) error {
	if !isValidRole(newRole) {
		return ErrInvalidRole
	}

	query := `
		UPDATE team_members
		SET role = $1, updated_at = CURRENT_TIMESTAMP
		WHERE team_id = $2 AND member_id = $3 AND status != 'removed'
	`

	result, err := tm.db.ExecContext(ctx, query, newRole, teamID, memberID)
	if err != nil {
		return fmt.Errorf("failed to update team member role: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return ErrMemberNotInTeam
	}

	return nil
}

// GetTeamCount returns the count of active teams in an organization
func (tm *TeamManager) GetTeamCount(ctx context.Context, orgID uuid.UUID) (int, error) {
	var count int

	query := `
		SELECT COUNT(*)
		FROM teams
		WHERE organization_id = $1 AND status = 'active' AND deleted_at IS NULL
	`

	err := tm.db.QueryRowContext(ctx, query, orgID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to get team count: %w", err)
	}

	return count, nil
}

// GetTeamMemberCount returns the count of active members in a team
func (tm *TeamManager) GetTeamMemberCount(ctx context.Context, teamID uuid.UUID) (int, error) {
	var count int

	query := `
		SELECT COUNT(*)
		FROM team_members
		WHERE team_id = $1 AND status = 'active'
	`

	err := tm.db.QueryRowContext(ctx, query, teamID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to get team member count: %w", err)
	}

	return count, nil
}

// isValidTeamStatus checks if a team status is valid
func isValidTeamStatus(status TeamStatus) bool {
	switch status {
	case TeamStatusActive, TeamStatusArchived, TeamStatusDeleted:
		return true
	default:
		return false
	}
}
