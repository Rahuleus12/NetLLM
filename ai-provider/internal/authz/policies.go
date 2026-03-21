// Package authz provides authorization functionality including RBAC, permissions, and policy-based access control
package authz

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Policy-related errors
var (
	ErrPolicyNotFound      = errors.New("policy not found")
	ErrPolicyInvalid       = errors.New("invalid policy")
	ErrPolicyConflict      = errors.New("policy conflict")
	ErrPolicyEvaluation    = errors.New("policy evaluation failed")
	ErrConditionNotMet     = errors.New("condition not met")
	ErrInvalidEffect       = errors.New("invalid effect")
	ErrInvalidCondition    = errors.New("invalid condition")
)

// Effect represents the effect of a policy
type Effect string

const (
	EffectAllow Effect = "allow"
	EffectDeny  Effect = "deny"
)

// IsValid checks if the effect is valid
func (e Effect) IsValid() bool {
	return e == EffectAllow || e == EffectDeny
}

// ConditionOperator represents a condition operator
type ConditionOperator string

const (
	OpEquals              ConditionOperator = "equals"
	OpNotEquals           ConditionOperator = "not_equals"
	OpContains            ConditionOperator = "contains"
	OpNotContains         ConditionOperator = "not_contains"
	OpStartsWith          ConditionOperator = "starts_with"
	OpEndsWith            ConditionOperator = "ends_with"
	OpGreaterThan         ConditionOperator = "greater_than"
	OpGreaterThanOrEqual  ConditionOperator = "greater_than_or_equal"
	OpLessThan            ConditionOperator = "less_than"
	OpLessThanOrEqual     ConditionOperator = "less_than_or_equal"
	OpIn                  ConditionOperator = "in"
	OpNotIn               ConditionOperator = "not_in"
	OpExists              ConditionOperator = "exists"
	OpNotExists           ConditionOperator = "not_exists"
	OpMatches             ConditionOperator = "matches" // Regex match
	OpIPAddress           ConditionOperator = "ip_address"
	OpIPAddressRange      ConditionOperator = "ip_address_range"
	OpBefore              ConditionOperator = "before" // Time comparison
	OpAfter               ConditionOperator = "after"  // Time comparison
)

// Condition represents a single condition in a policy
type Condition struct {
	Key      string            `json:"key"`
	Operator ConditionOperator `json:"operator"`
	Value    interface{}       `json:"value"`
}

// Evaluate evaluates the condition against the given context
func (c *Condition) Evaluate(ctx context) (bool, error) {
	contextValue, exists := getNestedValue(ctx, c.Key)

	switch c.Operator {
	case OpExists:
		return exists, nil
	case OpNotExists:
		return !exists, nil
	case OpEquals:
		if !exists {
			return false, nil
		}
		return compareEquals(contextValue, c.Value), nil
	case OpNotEquals:
		if !exists {
			return true, nil
		}
		return !compareEquals(contextValue, c.Value), nil
	case OpContains:
		if !exists {
			return false, nil
		}
		return containsValue(contextValue, c.Value), nil
	case OpNotContains:
		if !exists {
			return true, nil
		}
		return !containsValue(contextValue, c.Value), nil
	case OpStartsWith:
		if !exists {
			return false, nil
		}
		return startsWith(contextValue, c.Value), nil
	case OpEndsWith:
		if !exists {
			return false, nil
		}
		return endsWith(contextValue, c.Value), nil
	case OpIn:
		if !exists {
			return false, nil
		}
		return valueInList(contextValue, c.Value), nil
	case OpNotIn:
		if !exists {
			return true, nil
		}
		return !valueInList(contextValue, c.Value), nil
	case OpGreaterThan:
		if !exists {
			return false, nil
		}
		return compareNumbers(contextValue, c.Value, ">")
	case OpGreaterThanOrEqual:
		if !exists {
			return false, nil
		}
		return compareNumbers(contextValue, c.Value, ">=")
	case OpLessThan:
		if !exists {
			return false, nil
		}
		return compareNumbers(contextValue, c.Value, "<")
	case OpLessThanOrEqual:
		if !exists {
			return false, nil
		}
		return compareNumbers(contextValue, c.Value, "<=")
	case OpBefore:
		if !exists {
			return false, nil
		}
		return compareTime(contextValue, c.Value, "before")
	case OpAfter:
		if !exists {
			return false, nil
		}
		return compareTime(contextValue, c.Value, "after")
	case OpMatches:
		if !exists {
			return false, nil
		}
		return matchRegex(contextValue, c.Value)
	default:
		return false, ErrInvalidCondition
	}
}

// Statement represents a policy statement
type Statement struct {
	ID          string       `json:"id"`
	SID         string       `json:"sid,omitempty"` // Statement ID for reference
	Effect      Effect       `json:"effect"`
	Actions     []string     `json:"actions"`
	Resources   []string     `json:"resources"`
	Conditions  []Condition  `json:"conditions,omitempty"`
	Description string       `json:"description,omitempty"`
}

// Evaluate evaluates the statement against the given request
func (s *Statement) Evaluate(req *PolicyRequest) (bool, *Effect, error) {
	// Check if action matches
	actionMatches := false
	for _, action := range s.Actions {
		if matchPattern(action, req.Action) {
			actionMatches = true
			break
		}
	}
	if !actionMatches {
		return false, nil, nil
	}

	// Check if resource matches
	resourceMatches := false
	for _, resource := range s.Resources {
		if matchPattern(resource, req.Resource) {
			resourceMatches = true
			break
		}
	}
	if !resourceMatches {
		return false, nil, nil
	}

	// Evaluate conditions
	ctx := buildContext(req)
	for _, cond := range s.Conditions {
		met, err := cond.Evaluate(ctx)
		if err != nil {
			return false, nil, fmt.Errorf("condition evaluation failed: %w", err)
		}
		if !met {
			return false, nil, nil
		}
	}

	return true, &s.Effect, nil
}

// Policy represents an access policy
type Policy struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Version     string      `json:"version"`
	Statements  []Statement `json:"statements"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
	Tags        []string    `json:"tags,omitempty"`
	Priority    int         `json:"priority,omitempty"` // Higher priority policies are evaluated first
	Enabled     bool        `json:"enabled"`
}

// PolicyRequest represents a policy evaluation request
type PolicyRequest struct {
	Principal string            `json:"principal"` // User ID, role, or group
	Action    string            `json:"action"`
	Resource  string            `json:"resource"`
	Context   map[string]interface{} `json:"context,omitempty"`
}

// PolicyResult represents the result of policy evaluation
type PolicyResult struct {
	Allowed      bool     `json:"allowed"`
	Denied       bool     `json:"denied"`
	MatchedPolicies []string `json:"matched_policies,omitempty"`
	DeniedBy     string   `json:"denied_by,omitempty"`
	AllowedBy    string   `json:"allowed_by,omitempty"`
	Reason       string   `json:"reason,omitempty"`
}

// PolicyCombiningAlgorithm defines how to combine multiple policy results
type PolicyCombiningAlgorithm string

const (
	AlgorithmAllowOverride  PolicyCombiningAlgorithm = "allow_override"
	AlgorithmDenyOverride   PolicyCombiningAlgorithm = "deny_override"
	AlgorithmFirstMatch     PolicyCombiningAlgorithm = "first_match"
	AlgorithmAllAllow       PolicyCombiningAlgorithm = "all_allow"
	AlgorithmAllDeny        PolicyCombiningAlgorithm = "all_deny"
)

// PolicyStore defines the interface for policy storage
type PolicyStore interface {
	Create(ctx context.Context, policy *Policy) error
	Get(ctx context.Context, id string) (*Policy, error)
	Update(ctx context.Context, policy *Policy) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context, filter PolicyFilter) ([]*Policy, error)
	GetByPrincipal(ctx context.Context, principal string) ([]*Policy, error)
}

// PolicyFilter defines filters for listing policies
type PolicyFilter struct {
	Tags      []string `json:"tags,omitempty"`
	Enabled   *bool    `json:"enabled,omitempty"`
	Action    string   `json:"action,omitempty"`
	Resource  string   `json:"resource,omitempty"`
}

// MemoryPolicyStore is an in-memory implementation of PolicyStore
type MemoryPolicyStore struct {
	mu       sync.RWMutex
	policies map[string]*Policy
	byTag    map[string]map[string]struct{}
}

// NewMemoryPolicyStore creates a new in-memory policy store
func NewMemoryPolicyStore() *MemoryPolicyStore {
	return &MemoryPolicyStore{
		policies: make(map[string]*Policy),
		byTag:    make(map[string]map[string]struct{}),
	}
}

// Create stores a new policy
func (s *MemoryPolicyStore) Create(ctx context.Context, policy *Policy) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.policies[policy.ID]; exists {
		return ErrPolicyConflict
	}

	s.policies[policy.ID] = policy

	for _, tag := range policy.Tags {
		if _, exists := s.byTag[tag]; !exists {
			s.byTag[tag] = make(map[string]struct{})
		}
		s.byTag[tag][policy.ID] = struct{}{}
	}

	return nil
}

// Get retrieves a policy by ID
func (s *MemoryPolicyStore) Get(ctx context.Context, id string) (*Policy, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	policy, exists := s.policies[id]
	if !exists {
		return nil, ErrPolicyNotFound
	}

	return policy, nil
}

// Update updates an existing policy
func (s *MemoryPolicyStore) Update(ctx context.Context, policy *Policy) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.policies[policy.ID]; !exists {
		return ErrPolicyNotFound
	}

	// Remove old tag associations
	if old := s.policies[policy.ID]; old != nil {
		for _, tag := range old.Tags {
			if ids, exists := s.byTag[tag]; exists {
				delete(ids, policy.ID)
			}
		}
	}

	s.policies[policy.ID] = policy

	// Add new tag associations
	for _, tag := range policy.Tags {
		if _, exists := s.byTag[tag]; !exists {
			s.byTag[tag] = make(map[string]struct{})
		}
		s.byTag[tag][policy.ID] = struct{}{}
	}

	return nil
}

// Delete removes a policy
func (s *MemoryPolicyStore) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	policy, exists := s.policies[id]
	if !exists {
		return nil
	}

	for _, tag := range policy.Tags {
		if ids, exists := s.byTag[tag]; exists {
			delete(ids, id)
		}
	}

	delete(s.policies, id)
	return nil
}

// List retrieves policies based on filter
func (s *MemoryPolicyStore) List(ctx context.Context, filter PolicyFilter) ([]*Policy, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*Policy

	for _, policy := range s.policies {
		if filter.Enabled != nil && policy.Enabled != *filter.Enabled {
			continue
		}

		if len(filter.Tags) > 0 {
			matched := false
			for _, tag := range filter.Tags {
				for _, pTag := range policy.Tags {
					if tag == pTag {
						matched = true
						break
					}
				}
				if matched {
					break
				}
			}
			if !matched {
				continue
			}
		}

		result = append(result, policy)
	}

	return result, nil
}

// GetByPrincipal retrieves policies for a principal
func (s *MemoryPolicyStore) GetByPrincipal(ctx context.Context, principal string) ([]*Policy, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*Policy
	for _, policy := range s.policies {
		if !policy.Enabled {
			continue
		}
		for _, stmt := range policy.Statements {
			for _, resource := range stmt.Resources {
				if matchPattern(resource, principal) || resource == "*" {
					result = append(result, policy)
					break
				}
			}
		}
	}
	return result, nil
}

// PolicyEngine manages policy evaluation
type PolicyEngine struct {
	store     PolicyStore
	algorithm PolicyCombiningAlgorithm
}

// NewPolicyEngine creates a new policy engine
func NewPolicyEngine(store PolicyStore, algorithm PolicyCombiningAlgorithm) *PolicyEngine {
	if algorithm == "" {
		algorithm = AlgorithmDenyOverride
	}
	return &PolicyEngine{
		store:     store,
		algorithm: algorithm,
	}
}

// Evaluate evaluates policies for a given request
func (e *PolicyEngine) Evaluate(ctx context.Context, req *PolicyRequest) (*PolicyResult, error) {
	policies, err := e.store.GetByPrincipal(ctx, req.Principal)
	if err != nil {
		return nil, fmt.Errorf("failed to get policies: %w", err)
	}

	return e.EvaluatePolicies(req, policies)
}

// EvaluatePolicies evaluates a specific set of policies
func (e *PolicyEngine) EvaluatePolicies(req *PolicyRequest, policies []*Policy) (*PolicyResult, error) {
	result := &PolicyResult{
		MatchedPolicies: []string{},
	}

	var allowEffects []*Policy
	var denyEffects []*Policy

	for _, policy := range policies {
		if !policy.Enabled {
			continue
		}

		for _, stmt := range policy.Statements {
			matches, effect, err := stmt.Evaluate(req)
			if err != nil {
				return nil, fmt.Errorf("statement evaluation failed: %w", err)
			}

			if matches {
				result.MatchedPolicies = append(result.MatchedPolicies, policy.ID)
				if *effect == EffectAllow {
					allowEffects = append(allowEffects, policy)
				} else {
					denyEffects = append(denyEffects, policy)
				}
			}
		}
	}

	// Apply combining algorithm
	switch e.algorithm {
	case AlgorithmAllowOverride:
		if len(allowEffects) > 0 {
			result.Allowed = true
			result.AllowedBy = allowEffects[0].ID
		}
		if len(denyEffects) > 0 {
			result.Denied = true
			result.DeniedBy = denyEffects[0].ID
		}
		// Allow overrides deny
		if result.Allowed {
			result.Denied = false
		}

	case AlgorithmDenyOverride:
		if len(allowEffects) > 0 {
			result.Allowed = true
			result.AllowedBy = allowEffects[0].ID
		}
		if len(denyEffects) > 0 {
			result.Denied = true
			result.DeniedBy = denyEffects[0].ID
		}
		// Deny overrides allow
		if result.Denied {
			result.Allowed = false
		}

	case AlgorithmFirstMatch:
		if len(denyEffects) > 0 {
			result.Denied = true
			result.DeniedBy = denyEffects[0].ID
		} else if len(allowEffects) > 0 {
			result.Allowed = true
			result.AllowedBy = allowEffects[0].ID
		}

	case AlgorithmAllAllow:
		if len(denyEffects) == 0 && len(allowEffects) > 0 {
			result.Allowed = true
			result.AllowedBy = "all_policies"
		} else if len(denyEffects) > 0 {
			result.Denied = true
			result.DeniedBy = denyEffects[0].ID
		}

	case AlgorithmAllDeny:
		if len(allowEffects) == 0 && len(denyEffects) > 0 {
			result.Denied = true
			result.DeniedBy = "all_policies"
		} else if len(allowEffects) > 0 {
			result.Allowed = true
			result.AllowedBy = allowEffects[0].ID
		}
	}

	if result.Allowed {
		result.Reason = "Access allowed by policy"
	} else if result.Denied {
		result.Reason = "Access denied by policy"
	} else {
		result.Reason = "No matching policy found"
	}

	return result, nil
}

// CreatePolicy creates a new policy
func (e *PolicyEngine) CreatePolicy(ctx context.Context, policy *Policy) error {
	if policy.ID == "" {
		return ErrPolicyInvalid
	}

	for _, stmt := range policy.Statements {
		if !stmt.Effect.IsValid() {
			return ErrInvalidEffect
		}
	}

	policy.CreatedAt = time.Now()
	policy.UpdatedAt = time.Now()
	if policy.Version == "" {
		policy.Version = "2024-01-01"
	}

	return e.store.Create(ctx, policy)
}

// UpdatePolicy updates an existing policy
func (e *PolicyEngine) UpdatePolicy(ctx context.Context, policy *Policy) error {
	for _, stmt := range policy.Statements {
		if !stmt.Effect.IsValid() {
			return ErrInvalidEffect
		}
	}

	policy.UpdatedAt = time.Now()
	return e.store.Update(ctx, policy)
}

// DeletePolicy deletes a policy
func (e *PolicyEngine) DeletePolicy(ctx context.Context, id string) error {
	return e.store.Delete(ctx, id)
}

// GetPolicy retrieves a policy by ID
func (e *PolicyEngine) GetPolicy(ctx context.Context, id string) (*Policy, error) {
	return e.store.Get(ctx, id)
}

// ListPolicies lists policies with optional filtering
func (e *PolicyEngine) ListPolicies(ctx context.Context, filter PolicyFilter) ([]*Policy, error) {
	return e.store.List(ctx, filter)
}

// ToJSON serializes a policy to JSON
func (p *Policy) ToJSON() ([]byte, error) {
	return json.Marshal(p)
}

// FromJSON deserializes a policy from JSON
func (p *Policy) FromJSON(data []byte) error {
	return json.Unmarshal(data, p)
}

// Helper functions

func buildContext(req *PolicyRequest) context {
	ctx := make(context)
	ctx["principal"] = req.Principal
	ctx["action"] = req.Action
	ctx["resource"] = req.Resource
	for k, v := range req.Context {
		ctx[k] = v
	}
	return ctx
}

type context map[string]interface{}

func getNestedValue(ctx context, key string) (interface{}, bool) {
	parts := strings.Split(key, ".")
	var current interface{} = ctx
	for _, part := range parts {
		switch v := current.(type) {
		case map[string]interface{}:
			val, exists := v[part]
			if !exists {
				return nil, false
			}
			current = val
		case context:
			val, exists := v[part]
			if !exists {
				return nil, false
			}
			current = val
		default:
			return nil, false
		}
	}
	return current, true
}

func compareEquals(a, b interface{}) bool {
	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
}

func containsValue(container, value interface{}) bool {
	containerStr := fmt.Sprintf("%v", container)
	valueStr := fmt.Sprintf("%v", value)
	return strings.Contains(containerStr, valueStr)
}

func startsWith(str, prefix interface{}) bool {
	return strings.HasPrefix(fmt.Sprintf("%v", str), fmt.Sprintf("%v", prefix))
}

func endsWith(str, suffix interface{}) bool {
	return strings.HasSuffix(fmt.Sprintf("%v", str), fmt.Sprintf("%v", suffix))
}

func valueInList(value, list interface{}) bool {
	switch v := list.(type) {
	case []string:
		for _, item := range v {
			if compareEquals(value, item) {
				return true
			}
		}
	case []interface{}:
		for _, item := range v {
			if compareEquals(value, item) {
				return true
			}
		}
	}
	return false
}

func compareNumbers(a, b interface{}, op string) (bool, error) {
	aFloat, ok := toFloat64(a)
	if !ok {
		return false, nil
	}
	bFloat, ok := toFloat64(b)
	if !ok {
		return false, nil
	}

	switch op {
	case ">":
		return aFloat > bFloat, nil
	case ">=":
		return aFloat >= bFloat, nil
	case "<":
		return aFloat < bFloat, nil
	case "<=":
		return aFloat <= bFloat, nil
	}
	return false, nil
}

func toFloat64(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case float32:
		return float64(n), true
	case float64:
		return n, true
	}
	return 0, false
}

func compareTime(a, b interface{}, op string) (bool, error) {
	var aTime, bTime time.Time
	var err error

	switch v := a.(type) {
	case time.Time:
		aTime = v
	case string:
		aTime, err = time.Parse(time.RFC3339, v)
		if err != nil {
			return false, nil
		}
	}

	switch v := b.(type) {
	case time.Time:
		bTime = v
	case string:
		bTime, err = time.Parse(time.RFC3339, v)
		if err != nil {
			return false, nil
		}
	}

	switch op {
	case "before":
		return aTime.Before(bTime), nil
	case "after":
		return aTime.After(bTime), nil
	}
	return false, nil
}

func matchRegex(value, pattern interface{}) (bool, error) {
	// Simplified regex matching - in production use regexp package
	patternStr := fmt.Sprintf("%v", pattern)
	valueStr := fmt.Sprintf("%v", value)

	if patternStr == "*" {
		return true, nil
	}

	return strings.Contains(valueStr, strings.Trim(patternStr, ".*")), nil
}

func matchPattern(pattern, value string) bool {
	if pattern == "*" {
		return true
	}
	if pattern == value {
		return true
	}
	if strings.Contains(pattern, "*") {
		parts := strings.Split(pattern, "*")
		if len(parts) == 2 {
			return strings.HasPrefix(value, parts[0]) && strings.HasSuffix(value, parts[1])
		}
	}
	return false
}
