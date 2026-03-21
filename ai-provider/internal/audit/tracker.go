package audit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"
)

// Change tracking errors
var (
	ErrChangeNotFound      = errors.New("change not found")
	ErrInvalidChangeType   = errors.New("invalid change type")
	ErrChangeStoreFailed   = errors.New("failed to store change")
	ErrInvalidDiff         = errors.New("invalid diff")
	ErrSnapshotNotFound    = errors.New("snapshot not found")
)

// ChangeType represents the type of change
type ChangeType string

const (
	ChangeTypeCreate  ChangeType = "create"
	ChangeTypeUpdate  ChangeType = "update"
	ChangeTypeDelete  ChangeType = "delete"
	ChangeTypeRestore ChangeType = "restore"
)

// String returns the string representation of the change type
func (ct ChangeType) String() string {
	return string(ct)
}

// IsValid checks if the change type is valid
func (ct ChangeType) IsValid() bool {
	switch ct {
	case ChangeTypeCreate, ChangeTypeUpdate, ChangeTypeDelete, ChangeTypeRestore:
		return true
	default:
		return false
	}
}

// FieldDiff represents a diff for a single field
type FieldDiff struct {
	Field    string      `json:"field"`
	OldValue interface{} `json:"old_value,omitempty"`
	NewValue interface{} `json:"new_value,omitempty"`
	Type     string      `json:"type"` // "added", "removed", "modified", "unchanged"
}

// Change represents a tracked change to a resource
type Change struct {
	ID           string            `json:"id"`
	ResourceType string            `json:"resource_type"`
	ResourceID   string            `json:"resource_id"`
	ChangeType   ChangeType        `json:"change_type"`
	FieldDiffs   []FieldDiff       `json:"field_diffs,omitempty"`
	OldSnapshot  map[string]interface{} `json:"old_snapshot,omitempty"`
	NewSnapshot  map[string]interface{} `json:"new_snapshot,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
	ChangedBy    string            `json:"changed_by"`
	ChangedAt    time.Time         `json:"changed_at"`
	Reason       string            `json:"reason,omitempty"`
	SessionID    string            `json:"session_id,omitempty"`
	RequestID    string            `json:"request_id,omitempty"`
	IPAddress    string            `json:"ip_address,omitempty"`
	UserAgent    string            `json:"user_agent,omitempty"`
}

// ChangeSummary represents a summary of a change
type ChangeSummary struct {
	ID           string    `json:"id"`
	ResourceType string    `json:"resource_type"`
	ResourceID   string    `json:"resource_id"`
	ChangeType   ChangeType `json:"change_type"`
	FieldsCount  int       `json:"fields_count"`
	ChangedBy    string    `json:"changed_by"`
	ChangedAt    time.Time `json:"changed_at"`
	Summary      string    `json:"summary"`
}

// GetSummary generates a human-readable summary of the change
func (c *Change) GetSummary() string {
	var summary strings.Builder

	switch c.ChangeType {
	case ChangeTypeCreate:
		summary.WriteString("Created ")
	case ChangeTypeUpdate:
		summary.WriteString("Updated ")
	case ChangeTypeDelete:
		summary.WriteString("Deleted ")
	case ChangeTypeRestore:
		summary.WriteString("Restored ")
	}

	summary.WriteString(c.ResourceType)
	summary.WriteString(" (")
	summary.WriteString(c.ResourceID)
	summary.WriteString(")")

	if len(c.FieldDiffs) > 0 {
		summary.WriteString(" - Modified fields: ")
		for i, diff := range c.FieldDiffs {
			if i > 0 {
				summary.WriteString(", ")
			}
			summary.WriteString(diff.Field)
		}
	}

	return summary.String()
}

// ToSummary converts the change to a summary
func (c *Change) ToSummary() *ChangeSummary {
	return &ChangeSummary{
		ID:           c.ID,
		ResourceType: c.ResourceType,
		ResourceID:   c.ResourceID,
		ChangeType:   c.ChangeType,
		FieldsCount:  len(c.FieldDiffs),
		ChangedBy:    c.ChangedBy,
		ChangedAt:    c.ChangedAt,
		Summary:      c.GetSummary(),
	}
}

// ChangeStore defines the interface for change storage
type ChangeStore interface {
	// Store stores a change record
	Store(ctx context.Context, change *Change) error

	// Get retrieves a change by ID
	Get(ctx context.Context, id string) (*Change, error)

	// GetByResource retrieves changes for a resource
	GetByResource(ctx context.Context, resourceType, resourceID string, opts *ChangeQueryOptions) ([]*Change, error)

	// GetByUser retrieves changes made by a user
	GetByUser(ctx context.Context, userID string, opts *ChangeQueryOptions) ([]*Change, error)

	// List retrieves changes based on filter
	List(ctx context.Context, opts *ChangeQueryOptions) ([]*Change, error)

	// Delete deletes a change record
	Delete(ctx context.Context, id string) error

	// DeleteByResource deletes all changes for a resource
	DeleteByResource(ctx context.Context, resourceType, resourceID string) error
}

// ChangeQueryOptions defines options for querying changes
type ChangeQueryOptions struct {
	ResourceType string    `json:"resource_type,omitempty"`
	ResourceID   string    `json:"resource_id,omitempty"`
	ChangeType   ChangeType `json:"change_type,omitempty"`
	ChangedBy    string    `json:"changed_by,omitempty"`
	StartTime    time.Time `json:"start_time,omitempty"`
	EndTime      time.Time `json:"end_time,omitempty"`
	Fields       []string  `json:"fields,omitempty"` // Filter by specific fields changed
	Limit        int       `json:"limit,omitempty"`
	Offset       int       `json:"offset,omitempty"`
	OrderBy      string    `json:"order_by,omitempty"` // "changed_at", "resource_type", etc.
	OrderDesc    bool      `json:"order_desc,omitempty"`
}

// MemoryChangeStore is an in-memory implementation of ChangeStore
type MemoryChangeStore struct {
	mu             sync.RWMutex
	changes        map[string]*Change
	byResource     map[string]map[string]struct{} // resourceType:resourceID -> change IDs
	byUser         map[string]map[string]struct{} // userID -> change IDs
	byTime         []*Change // sorted by time
}

// NewMemoryChangeStore creates a new in-memory change store
func NewMemoryChangeStore() *MemoryChangeStore {
	return &MemoryChangeStore{
		changes:    make(map[string]*Change),
		byResource: make(map[string]map[string]struct{}),
		byUser:     make(map[string]map[string]struct{}),
		byTime:     make([]*Change, 0),
	}
}

// Store stores a change record
func (s *MemoryChangeStore) Store(ctx context.Context, change *Change) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.changes[change.ID] = change

	// Index by resource
	resourceKey := fmt.Sprintf("%s:%s", change.ResourceType, change.ResourceID)
	if _, exists := s.byResource[resourceKey]; !exists {
		s.byResource[resourceKey] = make(map[string]struct{})
	}
	s.byResource[resourceKey][change.ID] = struct{}{}

	// Index by user
	if change.ChangedBy != "" {
		if _, exists := s.byUser[change.ChangedBy]; !exists {
			s.byUser[change.ChangedBy] = make(map[string]struct{})
		}
		s.byUser[change.ChangedBy][change.ID] = struct{}{}
	}

	// Add to time index (insert sorted)
	s.insertByTime(change)

	return nil
}

// insertByTime inserts a change into the time-sorted slice
func (s *MemoryChangeStore) insertByTime(change *Change) {
	// Simple append for now - in production use a proper sorted insert
	s.byTime = append(s.byTime, change)
}

// Get retrieves a change by ID
func (s *MemoryChangeStore) Get(ctx context.Context, id string) (*Change, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	change, exists := s.changes[id]
	if !exists {
		return nil, ErrChangeNotFound
	}

	return change, nil
}

// GetByResource retrieves changes for a resource
func (s *MemoryChangeStore) GetByResource(ctx context.Context, resourceType, resourceID string, opts *ChangeQueryOptions) ([]*Change, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	resourceKey := fmt.Sprintf("%s:%s", resourceType, resourceID)
	changeIDs, exists := s.byResource[resourceKey]
	if !exists {
		return []*Change{}, nil
	}

	var changes []*Change
	for id := range changeIDs {
		if change, exists := s.changes[id]; exists {
			changes = append(changes, change)
		}
	}

	return s.applyQueryOptions(changes, opts), nil
}

// GetByUser retrieves changes made by a user
func (s *MemoryChangeStore) GetByUser(ctx context.Context, userID string, opts *ChangeQueryOptions) ([]*Change, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	changeIDs, exists := s.byUser[userID]
	if !exists {
		return []*Change{}, nil
	}

	var changes []*Change
	for id := range changeIDs {
		if change, exists := s.changes[id]; exists {
			changes = append(changes, change)
		}
	}

	return s.applyQueryOptions(changes, opts), nil
}

// List retrieves changes based on filter
func (s *MemoryChangeStore) List(ctx context.Context, opts *ChangeQueryOptions) ([]*Change, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var changes []*Change
	for _, change := range s.byTime {
		changes = append(changes, change)
	}

	return s.applyQueryOptions(changes, opts), nil
}

// Delete deletes a change record
func (s *MemoryChangeStore) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	change, exists := s.changes[id]
	if !exists {
		return nil
	}

	// Remove from main map
	delete(s.changes, id)

	// Remove from resource index
	resourceKey := fmt.Sprintf("%s:%s", change.ResourceType, change.ResourceID)
	if changeIDs, exists := s.byResource[resourceKey]; exists {
		delete(changeIDs, id)
	}

	// Remove from user index
	if change.ChangedBy != "" {
		if changeIDs, exists := s.byUser[change.ChangedBy]; exists {
			delete(changeIDs, id)
		}
	}

	// Remove from time index
	for i, c := range s.byTime {
		if c.ID == id {
			s.byTime = append(s.byTime[:i], s.byTime[i+1:]...)
			break
		}
	}

	return nil
}

// DeleteByResource deletes all changes for a resource
func (s *MemoryChangeStore) DeleteByResource(ctx context.Context, resourceType, resourceID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	resourceKey := fmt.Sprintf("%s:%s", resourceType, resourceID)
	changeIDs, exists := s.byResource[resourceKey]
	if !exists {
		return nil
	}

	for id := range changeIDs {
		delete(s.changes, id)
	}

	delete(s.byResource, resourceKey)

	// Rebuild time index
	s.byTime = make([]*Change, 0)
	for _, change := range s.changes {
		s.byTime = append(s.byTime, change)
	}

	return nil
}

// applyQueryOptions applies query options to filter and sort changes
func (s *MemoryChangeStore) applyQueryOptions(changes []*Change, opts *ChangeQueryOptions) []*Change {
	if opts == nil {
		return changes
	}

	var result []*Change

	for _, change := range changes {
		// Apply filters
		if opts.ChangeType != "" && change.ChangeType != opts.ChangeType {
			continue
		}

		if !opts.StartTime.IsZero() && change.ChangedAt.Before(opts.StartTime) {
			continue
		}

		if !opts.EndTime.IsZero() && change.ChangedAt.After(opts.EndTime) {
			continue
		}

		if len(opts.Fields) > 0 {
			// Check if any of the specified fields were changed
			fieldChanged := false
			for _, diff := range change.FieldDiffs {
				for _, field := range opts.Fields {
					if diff.Field == field {
						fieldChanged = true
						break
					}
				}
				if fieldChanged {
					break
				}
			}
			if !fieldChanged {
				continue
			}
		}

		result = append(result, change)
	}

	// Sort
	if opts.OrderBy != "" {
		result = sortChanges(result, opts.OrderBy, opts.OrderDesc)
	} else {
		// Default sort by time descending
		result = sortChanges(result, "changed_at", true)
	}

	// Apply pagination
	if opts.Offset > 0 && opts.Offset < len(result) {
		result = result[opts.Offset:]
	}

	if opts.Limit > 0 && opts.Limit < len(result) {
		result = result[:opts.Limit]
	}

	return result
}

// sortChanges sorts changes by the specified field
func sortChanges(changes []*Change, orderBy string, desc bool) []*Change {
	// Simple bubble sort for small lists
	result := make([]*Change, len(changes))
	copy(result, changes)

	for i := 0; i < len(result); i++ {
		for j := i + 1; j < len(result); j++ {
			var shouldSwap bool
			switch orderBy {
			case "changed_at":
				if desc {
					shouldSwap = result[j].ChangedAt.After(result[i].ChangedAt)
				} else {
					shouldSwap = result[j].ChangedAt.Before(result[i].ChangedAt)
				}
			case "resource_type":
				if desc {
					shouldSwap = result[j].ResourceType > result[i].ResourceType
				} else {
					shouldSwap = result[j].ResourceType < result[i].ResourceType
				}
			case "change_type":
				if desc {
					shouldSwap = result[j].ChangeType > result[i].ChangeType
				} else {
					shouldSwap = result[j].ChangeType < result[i].ChangeType
				}
			}

			if shouldSwap {
				result[i], result[j] = result[j], result[i]
			}
		}
	}

	return result
}

// ChangeTracker provides change tracking functionality
type ChangeTracker struct {
	store        ChangeStore
	snapshotter  Snapshotter
	idGenerator  IDGenerator
}

// Snapshotter defines the interface for taking snapshots
type Snapshotter interface {
	// TakeSnapshot takes a snapshot of a resource
	TakeSnapshot(ctx context.Context, resourceType, resourceID string) (map[string]interface{}, error)

	// GetSnapshot retrieves a stored snapshot
	GetSnapshot(ctx context.Context, resourceType, resourceID string, at time.Time) (map[string]interface{}, error)
}

// IDGenerator generates unique IDs
type IDGenerator interface {
	Generate() string
}

// DefaultIDGenerator is the default ID generator
type DefaultIDGenerator struct{}

// Generate generates a unique ID
func (g *DefaultIDGenerator) Generate() string {
	return fmt.Sprintf("chg_%d", time.Now().UnixNano())
}

// NewChangeTracker creates a new change tracker
func NewChangeTracker(store ChangeStore, snapshotter Snapshotter) *ChangeTracker {
	return &ChangeTracker{
		store:       store,
		snapshotter: snapshotter,
		idGenerator: &DefaultIDGenerator{},
	}
}

// TrackCreate tracks the creation of a resource
func (t *ChangeTracker) TrackCreate(ctx context.Context, req *TrackChangeRequest) (*Change, error) {
	if !req.ChangeType.IsValid() {
		req.ChangeType = ChangeTypeCreate
	}

	change := &Change{
		ID:           t.idGenerator.Generate(),
		ResourceType: req.ResourceType,
		ResourceID:   req.ResourceID,
		ChangeType:   ChangeTypeCreate,
		NewSnapshot:  req.NewState,
		FieldDiffs:   t.generateCreateDiffs(req.NewState),
		Metadata:     req.Metadata,
		ChangedBy:    req.ChangedBy,
		ChangedAt:    time.Now(),
		Reason:       req.Reason,
		SessionID:    req.SessionID,
		RequestID:    req.RequestID,
		IPAddress:    req.IPAddress,
		UserAgent:    req.UserAgent,
	}

	if err := t.store.Store(ctx, change); err != nil {
		return nil, fmt.Errorf("failed to store change: %w", err)
	}

	return change, nil
}

// TrackUpdate tracks an update to a resource
func (t *ChangeTracker) TrackUpdate(ctx context.Context, req *TrackChangeRequest) (*Change, error) {
	if req.OldState == nil && req.NewState == nil {
		return nil, ErrInvalidChangeType
	}

	fieldDiffs := t.Diff(req.OldState, req.NewState)
	if len(fieldDiffs) == 0 {
		// No changes detected
		return nil, nil
	}

	change := &Change{
		ID:           t.idGenerator.Generate(),
		ResourceType: req.ResourceType,
		ResourceID:   req.ResourceID,
		ChangeType:   ChangeTypeUpdate,
		OldSnapshot:  req.OldState,
		NewSnapshot:  req.NewState,
		FieldDiffs:   fieldDiffs,
		Metadata:     req.Metadata,
		ChangedBy:    req.ChangedBy,
		ChangedAt:    time.Now(),
		Reason:       req.Reason,
		SessionID:    req.SessionID,
		RequestID:    req.RequestID,
		IPAddress:    req.IPAddress,
		UserAgent:    req.UserAgent,
	}

	if err := t.store.Store(ctx, change); err != nil {
		return nil, fmt.Errorf("failed to store change: %w", err)
	}

	return change, nil
}

// TrackDelete tracks the deletion of a resource
func (t *ChangeTracker) TrackDelete(ctx context.Context, req *TrackChangeRequest) (*Change, error) {
	change := &Change{
		ID:           t.idGenerator.Generate(),
		ResourceType: req.ResourceType,
		ResourceID:   req.ResourceID,
		ChangeType:   ChangeTypeDelete,
		OldSnapshot:  req.OldState,
		FieldDiffs:   t.generateDeleteDiffs(req.OldState),
		Metadata:     req.Metadata,
		ChangedBy:    req.ChangedBy,
		ChangedAt:    time.Now(),
		Reason:       req.Reason,
		SessionID:    req.SessionID,
		RequestID:    req.RequestID,
		IPAddress:    req.IPAddress,
		UserAgent:    req.UserAgent,
	}

	if err := t.store.Store(ctx, change); err != nil {
		return nil, fmt.Errorf("failed to store change: %w", err)
	}

	return change, nil
}

// TrackChangeRequest represents a request to track a change
type TrackChangeRequest struct {
	ResourceType string                 `json:"resource_type"`
	ResourceID   string                 `json:"resource_id"`
	ChangeType   ChangeType             `json:"change_type"`
	OldState     map[string]interface{} `json:"old_state,omitempty"`
	NewState     map[string]interface{} `json:"new_state,omitempty"`
	Metadata     map[string]string      `json:"metadata,omitempty"`
	ChangedBy    string                 `json:"changed_by"`
	Reason       string                 `json:"reason,omitempty"`
	SessionID    string                 `json:"session_id,omitempty"`
	RequestID    string                 `json:"request_id,omitempty"`
	IPAddress    string                 `json:"ip_address,omitempty"`
	UserAgent    string                 `json:"user_agent,omitempty"`
}

// Track tracks a change based on the request
func (t *ChangeTracker) Track(ctx context.Context, req *TrackChangeRequest) (*Change, error) {
	switch req.ChangeType {
	case ChangeTypeCreate:
		return t.TrackCreate(ctx, req)
	case ChangeTypeUpdate:
		return t.TrackUpdate(ctx, req)
	case ChangeTypeDelete:
		return t.TrackDelete(ctx, req)
	case ChangeTypeRestore:
		return t.TrackCreate(ctx, req) // Treat restore as create
	default:
		return nil, ErrInvalidChangeType
	}
}

// Diff generates field diffs between two states
func (t *ChangeTracker) Diff(oldState, newState map[string]interface{}) []FieldDiff {
	if oldState == nil && newState == nil {
		return nil
	}

	var diffs []FieldDiff

	if oldState == nil {
		// All fields in newState are new
		for field, value := range newState {
			diffs = append(diffs, FieldDiff{
				Field:    field,
				OldValue: nil,
				NewValue: value,
				Type:     "added",
			})
		}
		return diffs
	}

	if newState == nil {
		// All fields in oldState were removed
		for field, value := range oldState {
			diffs = append(diffs, FieldDiff{
				Field:    field,
				OldValue: value,
				NewValue: nil,
				Type:     "removed",
			})
		}
		return diffs
	}

	// Check for modified and added fields
	allFields := make(map[string]bool)
	for field := range oldState {
		allFields[field] = true
	}
	for field := range newState {
		allFields[field] = true
	}

	for field := range allFields {
		oldValue, oldExists := oldState[field]
		newValue, newExists := newState[field]

		if !oldExists && newExists {
			diffs = append(diffs, FieldDiff{
				Field:    field,
				OldValue: nil,
				NewValue: newValue,
				Type:     "added",
			})
		} else if oldExists && !newExists {
			diffs = append(diffs, FieldDiff{
				Field:    field,
				OldValue: oldValue,
				NewValue: nil,
				Type:     "removed",
			})
		} else if !reflect.DeepEqual(oldValue, newValue) {
			diffs = append(diffs, FieldDiff{
				Field:    field,
				OldValue: oldValue,
				NewValue: newValue,
				Type:     "modified",
			})
		}
	}

	return diffs
}

// generateCreateDiffs generates diffs for a create operation
func (t *ChangeTracker) generateCreateDiffs(state map[string]interface{}) []FieldDiff {
	var diffs []FieldDiff
	for field, value := range state {
		diffs = append(diffs, FieldDiff{
			Field:    field,
			OldValue: nil,
			NewValue: value,
			Type:     "added",
		})
	}
	return diffs
}

// generateDeleteDiffs generates diffs for a delete operation
func (t *ChangeTracker) generateDeleteDiffs(state map[string]interface{}) []FieldDiff {
	var diffs []FieldDiff
	for field, value := range state {
		diffs = append(diffs, FieldDiff{
			Field:    field,
			OldValue: value,
			NewValue: nil,
			Type:     "removed",
		})
	}
	return diffs
}

// GetChange retrieves a change by ID
func (t *ChangeTracker) GetChange(ctx context.Context, id string) (*Change, error) {
	return t.store.Get(ctx, id)
}

// GetResourceHistory retrieves the change history for a resource
func (t *ChangeTracker) GetResourceHistory(ctx context.Context, resourceType, resourceID string, opts *ChangeQueryOptions) ([]*Change, error) {
	return t.store.GetByResource(ctx, resourceType, resourceID, opts)
}

// GetUserChanges retrieves changes made by a user
func (t *ChangeTracker) GetUserChanges(ctx context.Context, userID string, opts *ChangeQueryOptions) ([]*Change, error) {
	return t.store.GetByUser(ctx, userID, opts)
}

// ListChanges retrieves changes based on filter
func (t *ChangeTracker) ListChanges(ctx context.Context, opts *ChangeQueryOptions) ([]*Change, error) {
	return t.store.List(ctx, opts)
}

// GetChangeCount returns the count of changes for a resource
func (t *ChangeTracker) GetChangeCount(ctx context.Context, resourceType, resourceID string) (int, error) {
	changes, err := t.store.GetByResource(ctx, resourceType, resourceID, nil)
	if err != nil {
		return 0, err
	}
	return len(changes), nil
}

// Revert attempts to revert a change
func (t *ChangeTracker) Revert(ctx context.Context, changeID string, changedBy string) (*Change, error) {
	change, err := t.store.Get(ctx, changeID)
	if err != nil {
		return nil, err
	}

	if change.ChangeType == ChangeTypeDelete {
		// Restore deleted resource
		return t.TrackCreate(ctx, &TrackChangeRequest{
			ResourceType: change.ResourceType,
			ResourceID:   change.ResourceID,
			NewState:     change.OldSnapshot,
			ChangedBy:    changedBy,
			Reason:       fmt.Sprintf("Reverting change %s", changeID),
		})
	}

	if change.ChangeType == ChangeTypeCreate {
		// Delete created resource
		return t.TrackDelete(ctx, &TrackChangeRequest{
			ResourceType: change.ResourceType,
			ResourceID:   change.ResourceID,
			OldState:     change.NewSnapshot,
			ChangedBy:    changedBy,
			Reason:       fmt.Sprintf("Reverting change %s", changeID),
		})
	}

	// For updates, revert to old state
	return t.TrackUpdate(ctx, &TrackChangeRequest{
		ResourceType: change.ResourceType,
		ResourceID:   change.ResourceID,
		OldState:     change.NewSnapshot,
		NewState:     change.OldSnapshot,
		ChangedBy:    changedBy,
		Reason:       fmt.Sprintf("Reverting change %s", changeID),
	})
}

// GetChangesBetween returns changes between two points in time
func (t *ChangeTracker) GetChangesBetween(ctx context.Context, resourceType, resourceID string, start, end time.Time) ([]*Change, error) {
	opts := &ChangeQueryOptions{
		StartTime: start,
		EndTime:   end,
		OrderBy:   "changed_at",
		OrderDesc: false,
	}
	return t.store.GetByResource(ctx, resourceType, resourceID, opts)
}

// GetChangeAt returns the state of a resource at a specific point in time
func (t *ChangeTracker) GetChangeAt(ctx context.Context, resourceType, resourceID string, at time.Time) (map[string]interface{}, error) {
	// Get all changes up to the specified time
	opts := &ChangeQueryOptions{
		EndTime:  at,
		OrderBy:  "changed_at",
		OrderDesc: false,
	}

	changes, err := t.store.GetByResource(ctx, resourceType, resourceID, opts)
	if err != nil {
		return nil, err
	}

	if len(changes) == 0 {
		return nil, ErrSnapshotNotFound
	}

	// Apply changes in order to reconstruct the state
	var state map[string]interface{}
	for _, change := range changes {
		switch change.ChangeType {
		case ChangeTypeCreate:
			state = change.NewSnapshot
		case ChangeTypeUpdate:
			if state == nil {
				state = make(map[string]interface{})
			}
			for _, diff := range change.FieldDiffs {
				state[diff.Field] = diff.NewValue
			}
		case ChangeTypeDelete:
			state = nil
		case ChangeTypeRestore:
			state = change.NewSnapshot
		}
	}

	return state, nil
}

// ToJSON converts a change to JSON
func (c *Change) ToJSON() ([]byte, error) {
	return json.Marshal(c)
}

// FromJSON deserializes a change from JSON
func (c *Change) FromJSON(data []byte) error {
	return json.Unmarshal(data, c)
}

// DiffStats represents statistics about changes
type DiffStats struct {
	TotalChanges   int            `json:"total_changes"`
	ByType         map[ChangeType]int `json:"by_type"`
	ByField        map[string]int `json:"by_field"`
	TopChangers    []UserChangeCount `json:"top_changers"`
	ChangesPerDay  []DailyChangeCount `json:"changes_per_day"`
}

// UserChangeCount represents change count per user
type UserChangeCount struct {
	UserID string `json:"user_id"`
	Count  int    `json:"count"`
}

// DailyChangeCount represents change count per day
type DailyChangeCount struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

// GetDiffStats returns statistics about changes
func (t *ChangeTracker) GetDiffStats(ctx context.Context, opts *ChangeQueryOptions) (*DiffStats, error) {
	changes, err := t.store.List(ctx, opts)
	if err != nil {
		return nil, err
	}

	stats := &DiffStats{
		TotalChanges: len(changes),
		ByType:       make(map[ChangeType]int),
		ByField:      make(map[string]int),
	}

	userCounts := make(map[string]int)
	dayCounts := make(map[string]int)

	for _, change := range changes {
		// Count by type
		stats.ByType[change.ChangeType]++

		// Count by field
		for _, diff := range change.FieldDiffs {
			stats.ByField[diff.Field]++
		}

		// Count by user
		if change.ChangedBy != "" {
			userCounts[change.ChangedBy]++
		}

		// Count by day
		day := change.ChangedAt.Format("2006-01-02")
		dayCounts[day]++
	}

	// Get top changers
	for userID, count := range userCounts {
		stats.TopChangers = append(stats.TopChangers, UserChangeCount{
			UserID: userID,
			Count:  count,
		})
	}

	// Get changes per day
	for day, count := range dayCounts {
		stats.ChangesPerDay = append(stats.ChangesPerDay, DailyChangeCount{
			Date:  day,
			Count: count,
		})
	}

	return stats, nil
}

// CleanupOldChanges removes changes older than the specified duration
func (t *ChangeTracker) CleanupOldChanges(ctx context.Context, olderThan time.Duration) (int, error) {
	cutoff := time.Now().Add(-olderThan)
	changes, err := t.store.List(ctx, &ChangeQueryOptions{
		EndTime: cutoff,
	})
	if err != nil {
		return 0, err
	}

	count := 0
	for _, change := range changes {
		if err := t.store.Delete(ctx, change.ID); err != nil {
			continue
		}
		count++
	}

	return count, nil
}
