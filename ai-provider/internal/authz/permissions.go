package authz

import (
	"errors"
	"fmt"
	"strings"
	"sync"
)

// Permission-related errors
var (
	ErrPermissionDenied      = errors.New("permission denied")
	ErrPermissionNotFound    = errors.New("permission not found")
	ErrPermissionInvalid     = errors.New("invalid permission")
	ErrPermissionDuplicate   = errors.New("duplicate permission")
	ErrPermissionDependency  = errors.New("permission dependency not satisfied")
)

// Permission represents a single permission in the system
type Permission struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Resource    string            `json:"resource"`
	Action      string            `json:"action"`
	Conditions  []Condition       `json:"conditions,omitempty"`
	DependsOn   []string          `json:"depends_on,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// Condition represents a condition that must be met for a permission to apply
type Condition struct {
	Type     string                 `json:"type"`
	Key      string                 `json:"key"`
	Operator string                 `json:"operator"` // eq, ne, in, not_in, gt, lt, gte, lte, exists
	Value    interface{}            `json:"value"`
	Params   map[string]interface{} `json:"params,omitempty"`
}

// PermissionSet represents a collection of permissions
type PermissionSet struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Permissions []string          `json:"permissions"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// PermissionManager manages permissions in the system
type PermissionManager struct {
	permissions map[string]*Permission
	sets        map[string]*PermissionSet
	mu          sync.RWMutex
}

// NewPermissionManager creates a new permission manager
func NewPermissionManager() *PermissionManager {
	return &PermissionManager{
		permissions: make(map[string]*Permission),
		sets:        make(map[string]*PermissionSet),
	}
}

// RegisterPermission registers a new permission
func (m *PermissionManager) RegisterPermission(permission *Permission) error {
	if permission == nil {
		return ErrPermissionInvalid
	}

	if permission.ID == "" {
		permission.ID = generatePermissionID(permission.Resource, permission.Action)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.permissions[permission.ID]; exists {
		return ErrPermissionDuplicate
	}

	// Validate dependencies
	for _, depID := range permission.DependsOn {
		if _, exists := m.permissions[depID]; !exists {
			return fmt.Errorf("%w: %s", ErrPermissionDependency, depID)
		}
	}

	m.permissions[permission.ID] = permission
	return nil
}

// GetPermission retrieves a permission by ID
func (m *PermissionManager) GetPermission(id string) (*Permission, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	permission, exists := m.permissions[id]
	if !exists {
		return nil, ErrPermissionNotFound
	}

	return permission, nil
}

// GetPermissionByResourceAction retrieves a permission by resource and action
func (m *PermissionManager) GetPermissionByResourceAction(resource, action string) (*Permission, error) {
	id := generatePermissionID(resource, action)
	return m.GetPermission(id)
}

// ListPermissions lists all registered permissions
func (m *PermissionManager) ListPermissions() []*Permission {
	m.mu.RLock()
	defer m.mu.RUnlock()

	permissions := make([]*Permission, 0, len(m.permissions))
	for _, p := range m.permissions {
		permissions = append(permissions, p)
	}

	return permissions
}

// ListPermissionsByResource lists all permissions for a specific resource
func (m *PermissionManager) ListPermissionsByResource(resource string) []*Permission {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var permissions []*Permission
	for _, p := range m.permissions {
		if p.Resource == resource {
			permissions = append(permissions, p)
		}
	}

	return permissions
}

// DeletePermission removes a permission
func (m *PermissionManager) DeletePermission(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.permissions[id]; !exists {
		return ErrPermissionNotFound
	}

	// Check if any other permission depends on this one
	for _, p := range m.permissions {
		for _, depID := range p.DependsOn {
			if depID == id {
				return fmt.Errorf("cannot delete permission %s: permission %s depends on it", id, p.ID)
			}
		}
	}

	delete(m.permissions, id)
	return nil
}

// CreatePermissionSet creates a new permission set
func (m *PermissionManager) CreatePermissionSet(set *PermissionSet) error {
	if set == nil || set.ID == "" {
		return errors.New("invalid permission set")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Validate all permissions in the set exist
	for _, permID := range set.Permissions {
		if _, exists := m.permissions[permID]; !exists {
			return fmt.Errorf("permission %s not found", permID)
		}
	}

	m.sets[set.ID] = set
	return nil
}

// GetPermissionSet retrieves a permission set by ID
func (m *PermissionManager) GetPermissionSet(id string) (*PermissionSet, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	set, exists := m.sets[id]
	if !exists {
		return nil, errors.New("permission set not found")
	}

	return set, nil
}

// ListPermissionSets lists all permission sets
func (m *PermissionManager) ListPermissionSets() []*PermissionSet {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sets := make([]*PermissionSet, 0, len(m.sets))
	for _, s := range m.sets {
		sets = append(sets, s)
	}

	return sets
}

// DeletePermissionSet removes a permission set
func (m *PermissionManager) DeletePermissionSet(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.sets[id]; !exists {
		return errors.New("permission set not found")
	}

	delete(m.sets, id)
	return nil
}

// PermissionChecker checks if permissions are granted
type PermissionChecker struct {
	manager *PermissionManager
}

// NewPermissionChecker creates a new permission checker
func NewPermissionChecker(manager *PermissionManager) *PermissionChecker {
	return &PermissionChecker{
		manager: manager,
	}
}

// Check checks if a user with the given permissions can perform an action on a resource
func (c *PermissionChecker) Check(userPermissions []string, resource, action string, context map[string]interface{}) error {
	permissionID := generatePermissionID(resource, action)

	// Check if user has the direct permission
	hasPermission := false
	for _, p := range userPermissions {
		if p == permissionID || p == PermissionAll {
			hasPermission = true
			break
		}
	}

	if !hasPermission {
		return ErrPermissionDenied
	}

	// Get the permission and check conditions
	permission, err := c.manager.GetPermission(permissionID)
	if err != nil {
		// Permission exists in user's list but not in manager - allow it
		return nil
	}

	// Evaluate conditions
	for _, condition := range permission.Conditions {
		if !c.evaluateCondition(condition, context) {
			return ErrPermissionDenied
		}
	}

	// Check dependencies
	for _, depID := range permission.DependsOn {
		hasDep := false
		for _, p := range userPermissions {
			if p == depID {
				hasDep = true
				break
			}
		}
		if !hasDep {
			return fmt.Errorf("%w: missing dependency %s", ErrPermissionDependency, depID)
		}
	}

	return nil
}

// HasPermission checks if a permission ID exists in the user's permissions
func (c *PermissionChecker) HasPermission(userPermissions []string, permissionID string) bool {
	for _, p := range userPermissions {
		if p == permissionID || p == PermissionAll {
			return true
		}
	}
	return false
}

// HasAnyPermission checks if user has any of the specified permissions
func (c *PermissionChecker) HasAnyPermission(userPermissions []string, permissionIDs ...string) bool {
	for _, p := range userPermissions {
		if p == PermissionAll {
			return true
		}
		for _, id := range permissionIDs {
			if p == id {
				return true
			}
		}
	}
	return false
}

// HasAllPermissions checks if user has all of the specified permissions
func (c *PermissionChecker) HasAllPermissions(userPermissions []string, permissionIDs ...string) bool {
	for _, id := range permissionIDs {
		found := false
		for _, p := range userPermissions {
			if p == id || p == PermissionAll {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// evaluateCondition evaluates a condition against the context
func (c *PermissionChecker) evaluateCondition(condition Condition, context map[string]interface{}) bool {
	value, exists := context[condition.Key]
	if !exists {
		return condition.Operator == "not_exists" || condition.Operator == "ne"
	}

	switch condition.Operator {
	case "eq", "==":
		return value == condition.Value
	case "ne", "!=":
		return value != condition.Value
	case "in":
		if arr, ok := condition.Value.([]interface{}); ok {
			for _, v := range arr {
				if v == value {
					return true
				}
			}
			return false
		}
		if arr, ok := condition.Value.([]string); ok {
			if strVal, ok := value.(string); ok {
				for _, v := range arr {
					if v == strVal {
						return true
					}
				}
			}
			return false
		}
		return false
	case "not_in":
		if arr, ok := condition.Value.([]interface{}); ok {
			for _, v := range arr {
				if v == value {
					return false
				}
			}
			return true
		}
		if arr, ok := condition.Value.([]string); ok {
			if strVal, ok := value.(string); ok {
				for _, v := range arr {
					if v == strVal {
						return false
					}
				}
			}
			return true
		}
		return true
	case "gt", ">":
		return compareNumbers(value, condition.Value) > 0
	case "lt", "<":
		return compareNumbers(value, condition.Value) < 0
	case "gte", ">=":
		return compareNumbers(value, condition.Value) >= 0
	case "lte", "<=":
		return compareNumbers(value, condition.Value) <= 0
	case "exists":
		return exists
	case "not_exists":
		return !exists
	case "contains":
		if strVal, ok := value.(string); ok {
			if strCond, ok := condition.Value.(string); ok {
				return strings.Contains(strVal, strCond)
			}
		}
		return false
	case "starts_with":
		if strVal, ok := value.(string); ok {
			if strCond, ok := condition.Value.(string); ok {
				return strings.HasPrefix(strVal, strCond)
			}
		}
		return false
	case "ends_with":
		if strVal, ok := value.(string); ok {
			if strCond, ok := condition.Value.(string); ok {
				return strings.HasSuffix(strVal, strCond)
			}
		}
		return false
	default:
		return false
	}
}

// compareNumbers compares two numeric values
func compareNumbers(a, b interface{}) int {
	aFloat := toFloat64(a)
	bFloat := toFloat64(b)

	if aFloat < bFloat {
		return -1
	} else if aFloat > bFloat {
		return 1
	}
	return 0
}

// toFloat64 converts a value to float64
func toFloat64(v interface{}) float64 {
	switch val := v.(type) {
	case int:
		return float64(val)
	case int8:
		return float64(val)
	case int16:
		return float64(val)
	case int32:
		return float64(val)
	case int64:
		return float64(val)
	case uint:
		return float64(val)
	case uint8:
		return float64(val)
	case uint16:
		return float64(val)
	case uint32:
		return float64(val)
	case uint64:
		return float64(val)
	case float32:
		return float64(val)
	case float64:
		return val
	default:
		return 0
	}
}

// generatePermissionID generates a permission ID from resource and action
func generatePermissionID(resource, action string) string {
	return resource + ":" + action
}

// ParsePermissionID parses a permission ID into resource and action
func ParsePermissionID(id string) (resource, action string, err error) {
	parts := strings.SplitN(id, ":", 2)
	if len(parts) != 2 {
		return "", "", ErrPermissionInvalid
	}
	return parts[0], parts[1], nil
}

// Standard permission actions
const (
	ActionCreate = "create"
	ActionRead   = "read"
	ActionUpdate = "update"
	ActionDelete = "delete"
	ActionList   = "list"
	ActionExport = "export"
	ActionImport = "import"
	ActionManage = "manage" // Full control
)

// PermissionAll is a wildcard permission that grants all permissions
const PermissionAll = "*"

// Standard resources
const (
	ResourceUser       = "user"
	ResourceRole       = "role"
	ResourcePermission = "permission"
	ResourceAPIKey     = "api_key"
	ResourceModel      = "model"
	ResourceInference  = "inference"
	ResourceAudit      = "audit"
	ResourceConfig     = "config"
	ResourceBilling    = "billing"
	ResourceWorkspace  = "workspace"
	ResourceOrg        = "organization"
)

// StandardPermissions returns a list of standard permissions
func StandardPermissions() []*Permission {
	return []*Permission{
		// User permissions
		{ID: "user:read", Name: "Read Users", Resource: ResourceUser, Action: ActionRead, Description: "View user information"},
		{ID: "user:create", Name: "Create Users", Resource: ResourceUser, Action: ActionCreate, Description: "Create new users"},
		{ID: "user:update", Name: "Update Users", Resource: ResourceUser, Action: ActionUpdate, Description: "Update user information"},
		{ID: "user:delete", Name: "Delete Users", Resource: ResourceUser, Action: ActionDelete, Description: "Delete users"},
		{ID: "user:manage", Name: "Manage Users", Resource: ResourceUser, Action: ActionManage, Description: "Full control over users"},

		// Role permissions
		{ID: "role:read", Name: "Read Roles", Resource: ResourceRole, Action: ActionRead, Description: "View roles"},
		{ID: "role:create", Name: "Create Roles", Resource: ResourceRole, Action: ActionCreate, Description: "Create new roles"},
		{ID: "role:update", Name: "Update Roles", Resource: ResourceRole, Action: ActionUpdate, Description: "Update roles"},
		{ID: "role:delete", Name: "Delete Roles", Resource: ResourceRole, Action: ActionDelete, Description: "Delete roles"},
		{ID: "role:manage", Name: "Manage Roles", Resource: ResourceRole, Action: ActionManage, Description: "Full control over roles"},

		// Model permissions
		{ID: "model:read", Name: "Read Models", Resource: ResourceModel, Action: ActionRead, Description: "View models"},
		{ID: "model:create", Name: "Create Models", Resource: ResourceModel, Action: ActionCreate, Description: "Create new models"},
		{ID: "model:update", Name: "Update Models", Resource: ResourceModel, Action: ActionUpdate, Description: "Update models"},
		{ID: "model:delete", Name: "Delete Models", Resource: ResourceModel, Action: ActionDelete, Description: "Delete models"},
		{ID: "model:manage", Name: "Manage Models", Resource: ResourceModel, Action: ActionManage, Description: "Full control over models"},

		// Inference permissions
		{ID: "inference:read", Name: "Read Inferences", Resource: ResourceInference, Action: ActionRead, Description: "View inference history"},
		{ID: "inference:create", Name: "Run Inference", Resource: ResourceInference, Action: ActionCreate, Description: "Run model inference"},

		// Audit permissions
		{ID: "audit:read", Name: "Read Audit Logs", Resource: ResourceAudit, Action: ActionRead, Description: "View audit logs"},
		{ID: "audit:export", Name: "Export Audit Logs", Resource: ResourceAudit, Action: ActionExport, Description: "Export audit logs"},

		// API Key permissions
		{ID: "api_key:read", Name: "Read API Keys", Resource: ResourceAPIKey, Action: ActionRead, Description: "View API keys"},
		{ID: "api_key:create", Name: "Create API Keys", Resource: ResourceAPIKey, Action: ActionCreate, Description: "Create new API keys"},
		{ID: "api_key:delete", Name: "Delete API Keys", Resource: ResourceAPIKey, Action: ActionDelete, Description: "Delete/revoke API keys"},

		// Billing permissions
		{ID: "billing:read", Name: "Read Billing", Resource: ResourceBilling, Action: ActionRead, Description: "View billing information"},
		{ID: "billing:manage", Name: "Manage Billing", Resource: ResourceBilling, Action: ActionManage, Description: "Manage billing"},

		// Workspace permissions
		{ID: "workspace:read", Name: "Read Workspaces", Resource: ResourceWorkspace, Action: ActionRead, Description: "View workspaces"},
		{ID: "workspace:create", Name: "Create Workspaces", Resource: ResourceWorkspace, Action: ActionCreate, Description: "Create workspaces"},
		{ID: "workspace:update", Name: "Update Workspaces", Resource: ResourceWorkspace, Action: ActionUpdate, Description: "Update workspaces"},
		{ID: "workspace:delete", Name: "Delete Workspaces", Resource: ResourceWorkspace, Action: ActionDelete, Description: "Delete workspaces"},

		// Organization permissions
		{ID: "organization:read", Name: "Read Organizations", Resource: ResourceOrg, Action: ActionRead, Description: "View organizations"},
		{ID: "organization:update", Name: "Update Organizations", Resource: ResourceOrg, Action: ActionUpdate, Description: "Update organizations"},
		{ID: "organization:manage", Name: "Manage Organizations", Resource: ResourceOrg, Action: ActionManage, Description: "Full control over organizations"},
	}
}

// RegisterStandardPermissions registers all standard permissions
func (m *PermissionManager) RegisterStandardPermissions() error {
	for _, p := range StandardPermissions() {
		if err := m.RegisterPermission(p); err != nil {
			if !errors.Is(err, ErrPermissionDuplicate) {
				return err
			}
		}
	}
	return nil
}

// PermissionFilter is used to filter permissions
type PermissionFilter struct {
	Resource   string   `json:"resource,omitempty"`
	Action     string   `json:"action,omitempty"`
	Actions    []string `json:"actions,omitempty"`
	PermissionIDs []string `json:"permission_ids,omitempty"`
}

// FilterPermissions filters a list of permissions based on the filter criteria
func (m *PermissionManager) FilterPermissions(permissions []string, filter PermissionFilter) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []string

	for _, permID := range permissions {
		// If specific permission IDs are requested, check those
		if len(filter.PermissionIDs) > 0 {
			found := false
			for _, id := range filter.PermissionIDs {
				if permID == id {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		// Parse the permission ID
		resource, action, err := ParsePermissionID(permID)
		if err != nil {
			continue
		}

		// Filter by resource
		if filter.Resource != "" && resource != filter.Resource {
			continue
		}

		// Filter by single action
		if filter.Action != "" && action != filter.Action {
			continue
		}

		// Filter by multiple actions
		if len(filter.Actions) > 0 {
			found := false
			for _, a := range filter.Actions {
				if action == a {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		result = append(result, permID)
	}

	return result
}

// MergePermissions merges multiple permission lists, removing duplicates
func MergePermissions(permissionLists ...[]string) []string {
	seen := make(map[string]struct{})
	var result []string

	for _, list := range permissionLists {
		for _, perm := range list {
			if _, exists := seen[perm]; !exists {
				seen[perm] = struct{}{}
				result = append(result, perm)
			}
		}
	}

	return result
}

// SubtractPermissions removes permissions from a list
func SubtractPermissions(permissions []string, subtract []string) []string {
	subtractSet := make(map[string]struct{})
	for _, p := range subtract {
		subtractSet[p] = struct{}{}
	}

	var result []string
	for _, p := range permissions {
		if _, exists := subtractSet[p]; !exists {
			result = append(result, p)
		}
	}

	return result
}
