package authz

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// RBAC errors
var (
	ErrRoleNotFound       = errors.New("role not found")
	ErrRoleExists         = errors.New("role already exists")
	ErrPermissionNotFound = errors.New("permission not found")
	ErrPermissionDenied   = errors.New("permission denied")
	ErrInvalidRole        = errors.New("invalid role")
	ErrInvalidPermission  = errors.New("invalid permission")
	ErrCircularInheritance = errors.New("circular role inheritance detected")
)

// Permission represents a specific permission in the system
type Permission struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Resource    string    `json:"resource"`    // e.g., "models", "users", "apikeys"
	Action      string    `json:"action"`      // e.g., "read", "write", "delete", "admin"
	Scope       string    `json:"scope"`       // e.g., "own", "team", "organization", "global"
	CreatedAt   time.Time `json:"created_at"`
}

// String returns a string representation of the permission
func (p *Permission) String() string {
	return fmt.Sprintf("%s:%s:%s", p.Resource, p.Action, p.Scope)
}

// Role represents a collection of permissions
type Role struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Permissions []string      `json:"permissions"` // Permission IDs
	Parents     []string      `json:"parents,omitempty"` // Inherited role IDs
	IsSystem    bool          `json:"is_system"`    // System roles cannot be deleted
	IsDefault   bool          `json:"is_default"`   // Default role assigned to new users
	Priority    int           `json:"priority"`     // Higher priority = more permissions
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at"`
}

// RoleAssignment represents a role assigned to a user
type RoleAssignment struct {
	ID          string    `json:"id"`
	UserID      string    `json:"user_id"`
	RoleID      string    `json:"role_id"`
	ResourceID  string    `json:"resource_id,omitempty"` // Optional resource-specific assignment
	GrantedBy   string    `json:"granted_by"`
	GrantedAt   time.Time `json:"granted_at"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
}

// IsExpired checks if the role assignment has expired
func (ra *RoleAssignment) IsExpired() bool {
	if ra.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*ra.ExpiresAt)
}

// RoleRepository defines the interface for role storage
type RoleRepository interface {
	CreateRole(ctx context.Context, role *Role) error
	GetRole(ctx context.Context, id string) (*Role, error)
	GetRoleByName(ctx context.Context, name string) (*Role, error)
	ListRoles(ctx context.Context) ([]*Role, error)
	UpdateRole(ctx context.Context, role *Role) error
	DeleteRole(ctx context.Context, id string) error

	AssignRole(ctx context.Context, assignment *RoleAssignment) error
	RevokeRole(ctx context.Context, userID, roleID string) error
	GetUserRoles(ctx context.Context, userID string) ([]*RoleAssignment, error)
	GetRoleAssignments(ctx context.Context, roleID string) ([]*RoleAssignment, error)
}

// PermissionRepository defines the interface for permission storage
type PermissionRepository interface {
	CreatePermission(ctx context.Context, permission *Permission) error
	GetPermission(ctx context.Context, id string) (*Permission, error)
	GetPermissionByName(ctx context.Context, name string) (*Permission, error)
	ListPermissions(ctx context.Context) ([]*Permission, error)
	DeletePermission(ctx context.Context, id string) error
}

// RBACConfig holds RBAC configuration
type RBACConfig struct {
	// EnableCaching enables permission caching
	EnableCaching bool
	// CacheTTL is the time-to-live for cached permissions
	CacheTTL time.Duration
	// MaxRoleInheritanceDepth is the maximum depth of role inheritance
	MaxRoleInheritanceDepth int
}

// DefaultRBACConfig returns default RBAC configuration
func DefaultRBACConfig() *RBACConfig {
	return &RBACConfig{
		EnableCaching:           true,
		CacheTTL:                5 * time.Minute,
		MaxRoleInheritanceDepth: 10,
	}
}

// RBACManager manages role-based access control
type RBACManager struct {
	roleRepo       RoleRepository
	permissionRepo PermissionRepository
	config         *RBACConfig
	permissionCache sync.Map // userID -> cachedPermissions
}

// NewRBACManager creates a new RBAC manager
func NewRBACManager(roleRepo RoleRepository, permissionRepo PermissionRepository, config *RBACConfig) *RBACManager {
	if config == nil {
		config = DefaultRBACConfig()
	}
	return &RBACManager{
		roleRepo:       roleRepo,
		permissionRepo: permissionRepo,
		config:         config,
	}
}

// CreateRole creates a new role
func (m *RBACManager) CreateRole(ctx context.Context, role *Role) error {
	// Validate role
	if role.Name == "" {
		return ErrInvalidRole
	}

	// Check if role already exists
	existing, _ := m.roleRepo.GetRoleByName(ctx, role.Name)
	if existing != nil {
		return ErrRoleExists
	}

	// Validate permissions exist
	for _, permID := range role.Permissions {
		_, err := m.permissionRepo.GetPermission(ctx, permID)
		if err != nil {
			return fmt.Errorf("permission %s not found: %w", permID, err)
		}
	}

	// Validate parent roles exist and no circular inheritance
	if err := m.validateInheritance(ctx, role); err != nil {
		return err
	}

	now := time.Now()
	role.CreatedAt = now
	role.UpdatedAt = now

	return m.roleRepo.CreateRole(ctx, role)
}

// validateInheritance validates role inheritance for circular dependencies
func (m *RBACManager) validateInheritance(ctx context.Context, role *Role) error {
	if len(role.Parents) == 0 {
		return nil
	}

	visited := make(map[string]bool)
	return m.checkCircularInheritance(ctx, role.ID, role.Parents, visited, 0)
}

// checkCircularInheritance recursively checks for circular inheritance
func (m *RBACManager) checkCircularInheritance(ctx context.Context, roleID string, parents []string, visited map[string]bool, depth int) error {
	if depth > m.config.MaxRoleInheritanceDepth {
		return fmt.Errorf("role inheritance depth exceeded")
	}

	for _, parentID := range parents {
		if parentID == roleID {
			return ErrCircularInheritance
		}

		if visited[parentID] {
			continue
		}
		visited[parentID] = true

		parent, err := m.roleRepo.GetRole(ctx, parentID)
		if err != nil {
			return fmt.Errorf("parent role %s not found: %w", parentID, err)
		}

		if err := m.checkCircularInheritance(ctx, roleID, parent.Parents, visited, depth+1); err != nil {
			return err
		}
	}

	return nil
}

// GetRole retrieves a role by ID
func (m *RBACManager) GetRole(ctx context.Context, id string) (*Role, error) {
	return m.roleRepo.GetRole(ctx, id)
}

// GetRoleByName retrieves a role by name
func (m *RBACManager) GetRoleByName(ctx context.Context, name string) (*Role, error) {
	return m.roleRepo.GetRoleByName(ctx, name)
}

// ListRoles lists all roles
func (m *RBACManager) ListRoles(ctx context.Context) ([]*Role, error) {
	return m.roleRepo.ListRoles(ctx)
}

// UpdateRole updates a role
func (m *RBACManager) UpdateRole(ctx context.Context, role *Role) error {
	// Check if role exists
	existing, err := m.roleRepo.GetRole(ctx, role.ID)
	if err != nil {
		return ErrRoleNotFound
	}

	// System roles cannot be modified
	if existing.IsSystem {
		return fmt.Errorf("cannot modify system role")
	}

	// Validate permissions
	for _, permID := range role.Permissions {
		_, err := m.permissionRepo.GetPermission(ctx, permID)
		if err != nil {
			return fmt.Errorf("permission %s not found: %w", permID, err)
		}
	}

	// Validate inheritance
	if err := m.validateInheritance(ctx, role); err != nil {
		return err
	}

	role.UpdatedAt = time.Now()
	role.CreatedAt = existing.CreatedAt // Preserve created_at

	return m.roleRepo.UpdateRole(ctx, role)
}

// DeleteRole deletes a role
func (m *RBACManager) DeleteRole(ctx context.Context, id string) error {
	role, err := m.roleRepo.GetRole(ctx, id)
	if err != nil {
		return ErrRoleNotFound
	}

	if role.IsSystem {
		return fmt.Errorf("cannot delete system role")
	}

	return m.roleRepo.DeleteRole(ctx, id)
}

// AssignRole assigns a role to a user
func (m *RBACManager) AssignRole(ctx context.Context, userID, roleID, grantedBy string, expiresAt *time.Time) error {
	// Check if role exists
	_, err := m.roleRepo.GetRole(ctx, roleID)
	if err != nil {
		return ErrRoleNotFound
	}

	assignment := &RoleAssignment{
		ID:        generateID(),
		UserID:    userID,
		RoleID:    roleID,
		GrantedBy: grantedBy,
		GrantedAt: time.Now(),
		ExpiresAt: expiresAt,
	}

	// Invalidate cache
	m.invalidateCache(userID)

	return m.roleRepo.AssignRole(ctx, assignment)
}

// RevokeRole revokes a role from a user
func (m *RBACManager) RevokeRole(ctx context.Context, userID, roleID string) error {
	// Invalidate cache
	m.invalidateCache(userID)

	return m.roleRepo.RevokeRole(ctx, userID, roleID)
}

// GetUserRoles gets all roles assigned to a user
func (m *RBACManager) GetUserRoles(ctx context.Context, userID string) ([]*Role, error) {
	assignments, err := m.roleRepo.GetUserRoles(ctx, userID)
	if err != nil {
		return nil, err
	}

	roles := make([]*Role, 0, len(assignments))
	for _, assignment := range assignments {
		if assignment.IsExpired() {
			continue
		}

		role, err := m.roleRepo.GetRole(ctx, assignment.RoleID)
		if err != nil {
			continue
		}
		roles = append(roles, role)
	}

	return roles, nil
}

// GetAllUserPermissions gets all permissions for a user (including inherited)
func (m *RBACManager) GetAllUserPermissions(ctx context.Context, userID string) ([]*Permission, error) {
	// Check cache first
	if m.config.EnableCaching {
		if cached, ok := m.getFromCache(userID); ok {
			return cached, nil
		}
	}

	// Get user's roles
	roles, err := m.GetUserRoles(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Collect all permission IDs (including inherited)
	permissionIDs := make(map[string]bool)
	for _, role := range roles {
		m.collectPermissionIDs(ctx, role, permissionIDs, 0)
	}

	// Fetch permissions
	permissions := make([]*Permission, 0, len(permissionIDs))
	for permID := range permissionIDs {
		perm, err := m.permissionRepo.GetPermission(ctx, permID)
		if err != nil {
			continue
		}
		permissions = append(permissions, perm)
	}

	// Cache the result
	if m.config.EnableCaching {
		m.addToCache(userID, permissions)
	}

	return permissions, nil
}

// collectPermissionIDs collects all permission IDs from a role and its parents
func (m *RBACManager) collectPermissionIDs(ctx context.Context, role *Role, permissionIDs map[string]bool, depth int) {
	if depth > m.config.MaxRoleInheritanceDepth {
		return
	}

	// Add direct permissions
	for _, permID := range role.Permissions {
		permissionIDs[permID] = true
	}

	// Add inherited permissions
	for _, parentID := range role.Parents {
		parent, err := m.roleRepo.GetRole(ctx, parentID)
		if err != nil {
			continue
		}
		m.collectPermissionIDs(ctx, parent, permissionIDs, depth+1)
	}
}

// HasPermission checks if a user has a specific permission
func (m *RBACManager) HasPermission(ctx context.Context, userID, resource, action, scope string) (bool, error) {
	permissions, err := m.GetAllUserPermissions(ctx, userID)
	if err != nil {
		return false, err
	}

	for _, perm := range permissions {
		if perm.Resource == resource && perm.Action == action {
			// Check scope hierarchy: global > organization > team > own
			if scopeMatches(perm.Scope, scope) {
				return true, err
			}
		}
		// Admin action grants all actions on the resource
		if perm.Resource == resource && perm.Action == "admin" {
			return true, nil
		}
		// Global scope grants all scopes
		if perm.Resource == resource && perm.Action == action && perm.Scope == "global" {
			return true, nil
		}
	}

	return false, nil
}

// HasAnyPermission checks if a user has any of the specified permissions
func (m *RBACManager) HasAnyPermission(ctx context.Context, userID string, permissions []PermissionCheck) (bool, error) {
	for _, check := range permissions {
		has, err := m.HasPermission(ctx, userID, check.Resource, check.Action, check.Scope)
		if err != nil {
			return false, err
		}
		if has {
			return true, nil
		}
	}
	return false, nil
}

// HasAllPermissions checks if a user has all of the specified permissions
func (m *RBACManager) HasAllPermissions(ctx context.Context, userID string, permissions []PermissionCheck) (bool, error) {
	for _, check := range permissions {
		has, err := m.HasPermission(ctx, userID, check.Resource, check.Action, check.Scope)
		if err != nil {
			return false, err
		}
		if !has {
			return false, nil
		}
	}
	return true, nil
}

// HasRole checks if a user has a specific role
func (m *RBACManager) HasRole(ctx context.Context, userID, roleName string) (bool, error) {
	roles, err := m.GetUserRoles(ctx, userID)
	if err != nil {
		return false, err
	}

	for _, role := range roles {
		if role.Name == roleName {
			return true, nil
		}
		// Check inherited roles
		if m.hasRoleInherited(ctx, role, roleName, make(map[string]bool)) {
			return true, nil
		}
	}

	return false, nil
}

// hasRoleInherited checks if a role inherits another role
func (m *RBACManager) hasRoleInherited(ctx context.Context, role *Role, targetRoleName string, visited map[string]bool) bool {
	if visited[role.ID] {
		return false
	}
	visited[role.ID] = true

	for _, parentID := range role.Parents {
		parent, err := m.roleRepo.GetRole(ctx, parentID)
		if err != nil {
			continue
		}

		if parent.Name == targetRoleName {
			return true
		}

		if m.hasRoleInherited(ctx, parent, targetRoleName, visited) {
			return true
		}
	}

	return false
}

// CreatePermission creates a new permission
func (m *RBACManager) CreatePermission(ctx context.Context, permission *Permission) error {
	if permission.Name == "" || permission.Resource == "" || permission.Action == "" {
		return ErrInvalidPermission
	}

	permission.CreatedAt = time.Now()
	return m.permissionRepo.CreatePermission(ctx, permission)
}

// GetPermission retrieves a permission by ID
func (m *RBACManager) GetPermission(ctx context.Context, id string) (*Permission, error) {
	return m.permissionRepo.GetPermission(ctx, id)
}

// ListPermissions lists all permissions
func (m *RBACManager) ListPermissions(ctx context.Context) ([]*Permission, error) {
	return m.permissionRepo.ListPermissions(ctx)
}

// DeletePermission deletes a permission
func (m *RBACManager) DeletePermission(ctx context.Context, id string) error {
	return m.permissionRepo.DeletePermission(ctx, id)
}

// PermissionCheck represents a permission check request
type PermissionCheck struct {
	Resource string `json:"resource"`
	Action   string `json:"action"`
	Scope    string `json:"scope"`
}

// scopeMatches checks if a permission scope matches a required scope
func scopeMatches(permScope, requiredScope string) bool {
	// Scope hierarchy: global > organization > team > own
	scopeHierarchy := map[string]int{
		"global":       4,
		"organization": 3,
		"team":         2,
		"own":          1,
	}

	permLevel, permOk := scopeHierarchy[permScope]
	reqLevel, reqOk := scopeHierarchy[requiredScope]

	if !permOk || !reqOk {
		return permScope == requiredScope
	}

	// Permission scope must be at least as high as required scope
	return permLevel >= reqLevel
}

// cachedPermissions holds cached permissions with expiry
type cachedPermissions struct {
	permissions []*Permission
	expiresAt   time.Time
}

// getFromCache retrieves permissions from cache
func (m *RBACManager) getFromCache(userID string) ([]*Permission, bool) {
	value, ok := m.permissionCache.Load(userID)
	if !ok {
		return nil, false
	}

	cached := value.(*cachedPermissions)
	if time.Now().After(cached.expiresAt) {
		m.permissionCache.Delete(userID)
		return nil, false
	}

	return cached.permissions, true
}

// addToCache adds permissions to cache
func (m *RBACManager) addToCache(userID string, permissions []*Permission) {
	m.permissionCache.Store(userID, &cachedPermissions{
		permissions: permissions,
		expiresAt:   time.Now().Add(m.config.CacheTTL),
	})
}

// invalidateCache invalidates the permission cache for a user
func (m *RBACManager) invalidateCache(userID string) {
	m.permissionCache.Delete(userID)
}

// generateID generates a unique ID
func generateID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// DefaultRoles returns the default system roles
func DefaultRoles() []*Role {
	now := time.Now()
	return []*Role{
		{
			ID:          "role-admin",
			Name:        "admin",
			Description: "Administrator with full access",
			Permissions: []string{"perm-admin-all"},
			IsSystem:    true,
			Priority:    100,
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		{
			ID:          "role-user",
			Name:        "user",
			Description: "Standard user with basic access",
			Permissions: []string{"perm-inference-read", "perm-models-read"},
			IsSystem:    true,
			IsDefault:   true,
			Priority:    1,
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		{
			ID:          "role-developer",
			Name:        "developer",
			Description: "Developer with inference and model management access",
			Permissions: []string{"perm-inference-all", "perm-models-all"},
			Parents:     []string{"role-user"},
			IsSystem:    true,
			Priority:    50,
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		{
			ID:          "role-viewer",
			Name:        "viewer",
			Description: "Read-only access",
			Permissions: []string{"perm-inference-read", "perm-models-read", "perm-audit-read"},
			IsSystem:    true,
			Priority:    10,
			CreatedAt:   now,
			UpdatedAt:   now,
		},
	}
}

// DefaultPermissions returns the default system permissions
func DefaultPermissions() []*Permission {
	now := time.Now()
	return []*Permission{
		{ID: "perm-admin-all", Name: "admin.all", Description: "Full administrative access", Resource: "*", Action: "*", Scope: "global", CreatedAt: now},
		{ID: "perm-inference-read", Name: "inference.read", Description: "Read inference access", Resource: "inference", Action: "read", Scope: "own", CreatedAt: now},
		{ID: "perm-inference-write", Name: "inference.write", Description: "Write inference access", Resource: "inference", Action: "write", Scope: "own", CreatedAt: now},
		{ID: "perm-inference-all", Name: "inference.all", Description: "Full inference access", Resource: "inference", Action: "*", Scope: "team", CreatedAt: now},
		{ID: "perm-models-read", Name: "models.read", Description: "Read models access", Resource: "models", Action: "read", Scope: "own", CreatedAt: now},
		{ID: "perm-models-write", Name: "models.write", Description: "Write models access", Resource: "models", Action: "write", Scope: "own", CreatedAt: now},
		{ID: "perm-models-all", Name: "models.all", Description: "Full models access", Resource: "models", Action: "*", Scope: "team", CreatedAt: now},
		{ID: "perm-users-read", Name: "users.read", Description: "Read users access", Resource: "users", Action: "read", Scope: "team", CreatedAt: now},
		{ID: "perm-users-write", Name: "users.write", Description: "Write users access", Resource: "users", Action: "write", Scope: "team", CreatedAt: now},
		{ID: "perm-users-all", Name: "users.all", Description: "Full users access", Resource: "users", Action: "*", Scope: "organization", CreatedAt: now},
		{ID: "perm-audit-read", Name: "audit.read", Description: "Read audit logs", Resource: "audit", Action: "read", Scope: "team", CreatedAt: now},
		{ID: "perm-apikeys-read", Name: "apikeys.read", Description: "Read API keys", Resource: "apikeys", Action: "read", Scope: "own", CreatedAt: now},
		{ID: "perm-apikeys-write", Name: "apikeys.write", Description: "Write API keys", Resource: "apikeys", Action: "write", Scope: "own", CreatedAt: now},
		{ID: "perm-billing-read", Name: "billing.read", Description: "Read billing info", Resource: "billing", Action: "read", Scope: "own", CreatedAt: now},
		{ID: "perm-billing-write", Name: "billing.write", Description: "Write billing info", Resource: "billing", Action: "write", Scope: "own", CreatedAt: now},
	}
}

// InitializeDefaultRolesAndPermissions initializes the default roles and permissions
func (m *RBACManager) InitializeDefaultRolesAndPermissions(ctx context.Context) error {
	// Create default permissions
	for _, perm := range DefaultPermissions() {
		_, err := m.permissionRepo.GetPermission(ctx, perm.ID)
		if err == nil {
			continue // Already exists
		}
		if err := m.permissionRepo.CreatePermission(ctx, perm); err != nil {
			return fmt.Errorf("failed to create permission %s: %w", perm.ID, err)
		}
	}

	// Create default roles
	for _, role := range DefaultRoles() {
		_, err := m.roleRepo.GetRole(ctx, role.ID)
		if err == nil {
			continue // Already exists
		}
		if err := m.roleRepo.CreateRole(ctx, role); err != nil {
			return fmt.Errorf("failed to create role %s: %w", role.ID, err)
		}
	}

	return nil
}
