package authz

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// ACL errors
var (
	ErrACLMultipleMatches = errors.New("multiple ACL entries match")
)

// AccessType represents the type of access
type AccessType string

const (
	AccessTypeAllow AccessType = "allow"
	AccessTypeDeny  AccessType = "deny"
)

// ACE (Access Control Entry) represents a single entry in an ACL
type ACE struct {
	ID          string     `json:"id"`
	Principal   string     `json:"principal"`   // user ID, role name, or group ID
	PrincipalType PrincipalType `json:"principal_type"` // user, role, group
	Resource    string     `json:"resource"`    // resource identifier or pattern
	ResourceType string    `json:"resource_type"` // type of resource
	Permissions []string   `json:"permissions"` // specific permissions
	Access      AccessType `json:"access"`      // allow or deny
	Priority    int        `json:"priority"`    // higher priority = evaluated first
	Inherited   bool       `json:"inherited"`   // whether this ACE is inherited
	CreatedAt   time.Time  `json:"created_at"`
	CreatedBy   string     `json:"created_by"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
}

// ACL (Access Control List) represents a list of ACEs for a resource
type ACL struct {
	ResourceID   string `json:"resource_id"`
	ResourceType string `json:"resource_type"`
	Entries      []*ACE `json:"entries"`
	InheritFrom  string `json:"inherit_from,omitempty"` // parent resource ID to inherit from
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// ACLStore defines the interface for ACL storage
type ACLStore interface {
	// ACL operations
	CreateACL(ctx context.Context, acl *ACL) error
	GetACL(ctx context.Context, resourceID, resourceType string) (*ACL, error)
	UpdateACL(ctx context.Context, acl *ACL) error
	DeleteACL(ctx context.Context, resourceID, resourceType string) error

	// ACE operations
	AddACE(ctx context.Context, resourceID, resourceType string, ace *ACE) error
	UpdateACE(ctx context.Context, resourceID, resourceType string, ace *ACE) error
	RemoveACE(ctx context.Context, resourceID, resourceType, aceID string) error

	// Query operations
	GetACLsByPrincipal(ctx context.Context, principal string, principalType PrincipalType) ([]*ACL, error)
	GetACLsByResource(ctx context.Context, resourceType string) ([]*ACL, error)
}

// MemoryACLStore implements ACLStore using in-memory storage
type MemoryACLStore struct {
	mu       sync.RWMutex
	acls     map[string]*ACL // key: resourceID:resourceType
	byPrincipal map[string]map[string]struct{} // principal -> ACL keys
}

// NewMemoryACLStore creates a new in-memory ACL store
func NewMemoryACLStore() *MemoryACLStore {
	return &MemoryACLStore{
		acls:        make(map[string]*ACL),
		byPrincipal: make(map[string]map[string]struct{}),
	}
}

// aclKey generates a unique key for an ACL
func aclKey(resourceID, resourceType string) string {
	return fmt.Sprintf("%s:%s", resourceType, resourceID)
}

// CreateACL creates a new ACL
func (s *MemoryACLStore) CreateACL(ctx context.Context, acl *ACL) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := aclKey(acl.ResourceID, acl.ResourceType)
	if _, exists := s.acls[key]; exists {
		return ErrACLAlreadyExists
	}

	now := time.Now()
	acl.CreatedAt = now
	acl.UpdatedAt = now
	s.acls[key] = acl

	return nil
}

// GetACL retrieves an ACL by resource ID and type
func (s *MemoryACLStore) GetACL(ctx context.Context, resourceID, resourceType string) (*ACL, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := aclKey(resourceID, resourceType)
	acl, exists := s.acls[key]
	if !exists {
		return nil, ErrACLNotFound
	}

	return acl, nil
}

// UpdateACL updates an existing ACL
func (s *MemoryACLStore) UpdateACL(ctx context.Context, acl *ACL) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := aclKey(acl.ResourceID, acl.ResourceType)
	if _, exists := s.acls[key]; !exists {
		return ErrACLNotFound
	}

	acl.UpdatedAt = time.Now()
	s.acls[key] = acl

	return nil
}

// DeleteACL deletes an ACL
func (s *MemoryACLStore) DeleteACL(ctx context.Context, resourceID, resourceType string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := aclKey(resourceID, resourceType)
	delete(s.acls, key)

	return nil
}

// AddACE adds an ACE to an ACL
func (s *MemoryACLStore) AddACE(ctx context.Context, resourceID, resourceType string, ace *ACE) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := aclKey(resourceID, resourceType)
	acl, exists := s.acls[key]
	if !exists {
		return ErrACLNotFound
	}

	// Check for duplicate ACE ID
	for _, existing := range acl.Entries {
		if existing.ID == ace.ID {
			return ErrACEAlreadyExists
		}
	}

	acl.Entries = append(acl.Entries, ace)
	acl.UpdatedAt = time.Now()

	// Update principal index
	s.indexACE(ace, key)

	return nil
}

// UpdateACE updates an ACE in an ACL
func (s *MemoryACLStore) UpdateACE(ctx context.Context, resourceID, resourceType string, ace *ACE) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := aclKey(resourceID, resourceType)
	acl, exists := s.acls[key]
	if !exists {
		return ErrACLNotFound
	}

	for i, existing := range acl.Entries {
		if existing.ID == ace.ID {
			acl.Entries[i] = ace
			acl.UpdatedAt = time.Now()
			return nil
		}
	}

	return ErrACENotFound
}

// RemoveACE removes an ACE from an ACL
func (s *MemoryACLStore) RemoveACE(ctx context.Context, resourceID, resourceType, aceID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := aclKey(resourceID, resourceType)
	acl, exists := s.acls[key]
	if !exists {
		return ErrACLNotFound
	}

	for i, existing := range acl.Entries {
		if existing.ID == aceID {
			// Remove from slice
			acl.Entries = append(acl.Entries[:i], acl.Entries[i+1:]...)
			acl.UpdatedAt = time.Now()
			return nil
		}
	}

	return ErrACENotFound
}

// GetACLsByPrincipal retrieves all ACLs that have entries for a principal
func (s *MemoryACLStore) GetACLsByPrincipal(ctx context.Context, principal string, principalType PrincipalType) ([]*ACL, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	principalKey := fmt.Sprintf("%s:%s", principalType, principal)
	aclKeys, exists := s.byPrincipal[principalKey]
	if !exists {
		return []*ACL{}, nil
	}

	var acls []*ACL
	for key := range aclKeys {
		if acl, exists := s.acls[key]; exists {
			acls = append(acls, acl)
		}
	}

	return acls, nil
}

// GetACLsByResource retrieves all ACLs for a resource type
func (s *MemoryACLStore) GetACLsByResource(ctx context.Context, resourceType string) ([]*ACL, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var acls []*ACL
	for key, acl := range s.acls {
		if acl.ResourceType == resourceType {
			_ = key
			acls = append(acls, acl)
		}
	}

	return acls, nil
}

// indexACE adds an ACE to the principal index
func (s *MemoryACLStore) indexACE(ace *ACE, aclKey string) {
	principalKey := fmt.Sprintf("%s:%s", ace.PrincipalType, ace.Principal)
	if _, exists := s.byPrincipal[principalKey]; !exists {
		s.byPrincipal[principalKey] = make(map[string]struct{})
	}
	s.byPrincipal[principalKey][aclKey] = struct{}{}
}

// ACLService provides ACL management functionality
type ACLService struct {
	store ACLStore
}

// NewACLService creates a new ACL service
func NewACLService(store ACLStore) *ACLService {
	return &ACLService{
		store: store,
	}
}

// CreateACL creates a new ACL for a resource
func (s *ACLService) CreateACL(ctx context.Context, resourceID, resourceType, inheritFrom string) (*ACL, error) {
	acl := &ACL{
		ResourceID:   resourceID,
		ResourceType: resourceType,
		Entries:      []*ACE{},
		InheritFrom:  inheritFrom,
	}

	if err := s.store.CreateACL(ctx, acl); err != nil {
		return nil, fmt.Errorf("failed to create ACL: %w", err)
	}

	return acl, nil
}

// GetACL retrieves an ACL for a resource
func (s *ACLService) GetACL(ctx context.Context, resourceID, resourceType string) (*ACL, error) {
	return s.store.GetACL(ctx, resourceID, resourceType)
}

// DeleteACL deletes an ACL for a resource
func (s *ACLService) DeleteACL(ctx context.Context, resourceID, resourceType string) error {
	return s.store.DeleteACL(ctx, resourceID, resourceType)
}

// GrantAccess grants access to a principal for a resource
func (s *ACLService) GrantAccess(ctx context.Context, req *GrantAccessRequest) (*ACE, error) {
	acl, err := s.store.GetACL(ctx, req.ResourceID, req.ResourceType)
	if err != nil {
		if errors.Is(err, ErrACLNotFound) {
			// Create ACL if it doesn't exist
			acl, err = s.CreateACL(ctx, req.ResourceID, req.ResourceType, "")
			if err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}

	ace := &ACE{
		ID:           generateACEID(),
		Principal:    req.Principal,
		PrincipalType: req.PrincipalType,
		Resource:     req.ResourceID,
		ResourceType: req.ResourceType,
		Permissions:  req.Permissions,
		Access:       AccessTypeAllow,
		Priority:     req.Priority,
		CreatedAt:    time.Now(),
		CreatedBy:    req.CreatedBy,
		ExpiresAt:    req.ExpiresAt,
	}

	if err := s.store.AddACE(ctx, acl.ResourceID, acl.ResourceType, ace); err != nil {
		return nil, fmt.Errorf("failed to grant access: %w", err)
	}

	return ace, nil
}

// RevokeAccess revokes access from a principal for a resource
func (s *ACLService) RevokeAccess(ctx context.Context, resourceID, resourceType, aceID string) error {
	return s.store.RemoveACE(ctx, resourceID, resourceType, aceID)
}

// DenyAccess explicitly denies access to a principal for a resource
func (s *ACLService) DenyAccess(ctx context.Context, req *GrantAccessRequest) (*ACE, error) {
	acl, err := s.store.GetACL(ctx, req.ResourceID, req.ResourceType)
	if err != nil {
		if errors.Is(err, ErrACLNotFound) {
			acl, err = s.CreateACL(ctx, req.ResourceID, req.ResourceType, "")
			if err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}

	ace := &ACE{
		ID:           generateACEID(),
		Principal:    req.Principal,
		PrincipalType: req.PrincipalType,
		Resource:     req.ResourceID,
		ResourceType: req.ResourceType,
		Permissions:  req.Permissions,
		Access:       AccessTypeDeny,
		Priority:     req.Priority,
		CreatedAt:    time.Now(),
		CreatedBy:    req.CreatedBy,
		ExpiresAt:    req.ExpiresAt,
	}

	if err := s.store.AddACE(ctx, acl.ResourceID, acl.ResourceType, ace); err != nil {
		return nil, fmt.Errorf("failed to deny access: %w", err)
	}

	return ace, nil
}

// GrantAccessRequest represents a request to grant access
type GrantAccessRequest struct {
	ResourceID   string      `json:"resource_id"`
	ResourceType string      `json:"resource_type"`
	Principal    string      `json:"principal"`
	PrincipalType PrincipalType `json:"principal_type"`
	Permissions  []string    `json:"permissions"`
	Priority     int         `json:"priority"`
	CreatedBy    string      `json:"created_by"`
	ExpiresAt    *time.Time  `json:"expires_at,omitempty"`
}

// CheckAccess checks if a principal has access to a resource
func (s *ACLService) CheckAccess(ctx context.Context, req *CheckAccessRequest) (*AccessDecision, error) {
	// Get the ACL for the resource
	acl, err := s.store.GetACL(ctx, req.ResourceID, req.ResourceType)
	if err != nil {
		if errors.Is(err, ErrACLNotFound) {
			// No ACL means no access by default
			return &AccessDecision{
				Allowed: false,
				Reason:  "no ACL found for resource",
			}, nil
		}
		return nil, err
	}

	// Collect all applicable ACEs (including inherited)
	allACEs, err := s.collectACEs(ctx, acl)
	if err != nil {
		return nil, err
	}

	// Evaluate ACEs by priority (highest first)
	decision := &AccessDecision{
		Allowed: false,
		Reason:  "no matching ACE found",
	}

	// Sort ACEs by priority
	sortedACEs := sortACEsByPriority(allACEs)

	for _, ace := range sortedACEs {
		// Check if ACE is expired
		if ace.ExpiresAt != nil && time.Now().After(*ace.ExpiresAt) {
			continue
		}

		// Check if ACE applies to the principal
		if !s.aceMatchesPrincipal(ace, req.Principal, req.PrincipalType, req.Roles) {
			continue
		}

		// Check if ACE applies to the requested permission
		if !s.aceMatchesPermission(ace, req.Permission) {
			continue
		}

		// ACE matches - make decision based on access type
		decision.MatchedACE = ace
		if ace.Access == AccessTypeDeny {
			decision.Allowed = false
			decision.Reason = "explicit deny"
			return decision, nil
		}
		decision.Allowed = true
		decision.Reason = "explicit allow"
		return decision, nil
	}

	return decision, nil
}

// CheckAccessRequest represents a request to check access
type CheckAccessRequest struct {
	ResourceID   string       `json:"resource_id"`
	ResourceType string       `json:"resource_type"`
	Principal    string       `json:"principal"`
	PrincipalType PrincipalType `json:"principal_type"`
	Permission   string       `json:"permission"`
	Roles        []string     `json:"roles,omitempty"`
}

// AccessDecision represents an access decision
type AccessDecision struct {
	Allowed    bool   `json:"allowed"`
	Reason     string `json:"reason"`
	MatchedACE *ACE   `json:"matched_ace,omitempty"`
}

// collectACEs collects all ACEs including inherited ones
func (s *ACLService) collectACEs(ctx context.Context, acl *ACL) ([]*ACE, error) {
	var allACEs []*ACE
	allACEs = append(allACEs, acl.Entries...)

	// Collect inherited ACEs
	if acl.InheritFrom != "" {
		parentACL, err := s.store.GetACL(ctx, acl.InheritFrom, acl.ResourceType)
		if err == nil {
			parentACEs, err := s.collectACEs(ctx, parentACL)
			if err != nil {
				return nil, err
			}
			// Mark as inherited
			for _, ace := range parentACEs {
				aceCopy := *ace
				aceCopy.Inherited = true
				allACEs = append(allACEs, &aceCopy)
			}
		}
	}

	return allACEs, nil
}

// aceMatchesPrincipal checks if an ACE matches a principal
func (s *ACLService) aceMatchesPrincipal(ace *ACE, principal string, principalType PrincipalType, roles []string) bool {
	// Direct match
	if ace.Principal == principal && ace.PrincipalType == principalType {
		return true
	}

	// Role match
	if ace.PrincipalType == PrincipalTypeRole {
		for _, role := range roles {
			if ace.Principal == role {
				return true
			}
		}
	}

	// Group match (if principalType is group and principal is in group)
	// This would require group membership lookup in a real implementation

	return false
}

// aceMatchesPermission checks if an ACE matches a permission
func (s *ACLService) aceMatchesPermission(ace *ACE, permission string) bool {
	for _, p := range ace.Permissions {
		if p == "*" || p == permission {
			return true
		}
		// Wildcard matching (e.g., "read:*" matches "read:models")
		if matchWildcard(p, permission) {
			return true
		}
	}
	return false
}

// matchWildcard checks if a pattern matches a permission with wildcard support
func matchWildcard(pattern, permission string) bool {
	if pattern == "*" {
		return true
	}

	// Simple wildcard matching
	if len(pattern) > 0 && pattern[len(pattern)-1] == '*' {
		prefix := pattern[:len(pattern)-1]
		if len(permission) >= len(prefix) && permission[:len(prefix)] == prefix {
			return true
		}
	}

	return false
}

// sortACEsByPriority sorts ACEs by priority (highest first)
func sortACEsByPriority(aces []*ACE) []*ACE {
	// Simple bubble sort for small lists
	result := make([]*ACE, len(aces))
	copy(result, aces)

	for i := 0; i < len(result); i++ {
		for j := i + 1; j < len(result); j++ {
			if result[j].Priority > result[i].Priority {
				result[i], result[j] = result[j], result[i]
			}
		}
	}

	return result
}

// generateACEID generates a unique ACE ID
func generateACEID() string {
	return fmt.Sprintf("ace_%d", time.Now().UnixNano())
}

// GetPrincipalAccess gets all access entries for a principal
func (s *ACLService) GetPrincipalAccess(ctx context.Context, principal string, principalType PrincipalType) ([]*ACLEntrySummary, error) {
	acls, err := s.store.GetACLsByPrincipal(ctx, principal, principalType)
	if err != nil {
		return nil, err
	}

	var summaries []*ACLEntrySummary
	for _, acl := range acls {
		for _, ace := range acl.Entries {
			if ace.Principal == principal && ace.PrincipalType == principalType {
				summaries = append(summaries, &ACLEntrySummary{
					ACE:          ace,
					ResourceID:   acl.ResourceID,
					ResourceType: acl.ResourceType,
				})
			}
		}
	}

	return summaries, nil
}

// ACLEntrySummary represents an ACE with resource information
type ACLEntrySummary struct {
	ACE          *ACE  `json:"ace"`
	ResourceID   string `json:"resource_id"`
	ResourceType string `json:"resource_type"`
}

// SetInheritance sets the inheritance for an ACL
func (s *ACLService) SetInheritance(ctx context.Context, resourceID, resourceType, inheritFrom string) error {
	acl, err := s.store.GetACL(ctx, resourceID, resourceType)
	if err != nil {
		return err
	}

	acl.InheritFrom = inheritFrom
	return s.store.UpdateACL(ctx, acl)
}
