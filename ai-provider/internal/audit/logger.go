// Package audit provides audit logging, event tracking, change tracking, and compliance reporting functionality.
package audit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// Audit errors
var (
	ErrAuditLogNotFound   = errors.New("audit log not found")
	ErrInvalidAuditEntry  = errors.New("invalid audit entry")
	ErrAuditStorageFailed = errors.New("failed to store audit entry")
)

// AuditLevel represents the severity level of an audit event
type AuditLevel string

const (
	AuditLevelInfo    AuditLevel = "info"
	AuditLevelWarning AuditLevel = "warning"
	AuditLevelError   AuditLevel = "error"
	AuditLevelCritical AuditLevel = "critical"
)

// AuditCategory represents the category of an audit event
type AuditCategory string

const (
	CategoryAuthentication AuditCategory = "authentication"
	CategoryAuthorization  AuditCategory = "authorization"
	CategoryDataAccess     AuditCategory = "data_access"
	CategoryDataModification AuditCategory = "data_modification"
	CategoryConfiguration  AuditCategory = "configuration"
	CategorySecurity       AuditCategory = "security"
	CategorySystem         AuditCategory = "system"
	CategoryAPI            AuditCategory = "api"
	CategoryUserManagement AuditCategory = "user_management"
	CategoryBilling        AuditCategory = "billing"
)

// AuditEntry represents a single audit log entry
type AuditEntry struct {
	ID           string                 `json:"id"`
	Timestamp    time.Time              `json:"timestamp"`
	Level        AuditLevel             `json:"level"`
	Category     AuditCategory          `json:"category"`
	Action       string                 `json:"action"`
	ResourceType string                 `json:"resource_type"`
	ResourceID   string                 `json:"resource_id,omitempty"`
	UserID       string                 `json:"user_id,omitempty"`
	UserEmail    string                 `json:"user_email,omitempty"`
	SessionID    string                 `json:"session_id,omitempty"`
	IPAddress    string                 `json:"ip_address,omitempty"`
	UserAgent    string                 `json:"user_agent,omitempty"`
	RequestID    string                 `json:"request_id,omitempty"`
	Method       string                 `json:"method,omitempty"`
	Path         string                 `json:"path,omitempty"`
	StatusCode   int                    `json:"status_code,omitempty"`
	Duration     time.Duration          `json:"duration,omitempty"`
	Details      map[string]interface{} `json:"details,omitempty"`
	Changes      *ChangeSet             `json:"changes,omitempty"`
	Metadata     map[string]string      `json:"metadata,omitempty"`
	Success      bool                   `json:"success"`
	ErrorMessage string                 `json:"error_message,omitempty"`
}

// ChangeSet represents a set of changes made to a resource
type ChangeSet struct {
	Before interface{}            `json:"before,omitempty"`
	After  interface{}            `json:"after,omitempty"`
	Diff   map[string]FieldChange `json:"diff,omitempty"`
}

// FieldChange represents a change to a single field
type FieldChange struct {
	OldValue interface{} `json:"old_value,omitempty"`
	NewValue interface{} `json:"new_value,omitempty"`
}

// AuditFilter represents filters for querying audit logs
type AuditFilter struct {
	UserID       string        `json:"user_id,omitempty"`
	ResourceType string        `json:"resource_type,omitempty"`
	ResourceID   string        `json:"resource_id,omitempty"`
	Category     AuditCategory `json:"category,omitempty"`
	Level        AuditLevel    `json:"level,omitempty"`
	Action       string        `json:"action,omitempty"`
	Success      *bool         `json:"success,omitempty"`
	StartTime    *time.Time    `json:"start_time,omitempty"`
	EndTime      *time.Time    `json:"end_time,omitempty"`
	IPAddress    string        `json:"ip_address,omitempty"`
	Search       string        `json:"search,omitempty"`
	Limit        int           `json:"limit,omitempty"`
	Offset       int           `json:"offset,omitempty"`
}

// AuditStats represents statistics about audit logs
type AuditStats struct {
	TotalEntries    int64                   `json:"total_entries"`
	EntriesByLevel  map[AuditLevel]int64    `json:"entries_by_level"`
	EntriesByCategory map[AuditCategory]int64 `json:"entries_by_category"`
	FailedActions   int64                   `json:"failed_actions"`
	UniqueUsers     int64                   `json:"unique_users"`
	UniqueIPs       int64                   `json:"unique_ips"`
	TimeRange       TimeRange               `json:"time_range"`
}

// TimeRange represents a time range
type TimeRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// AuditStore defines the interface for audit log storage
type AuditStore interface {
	Store(ctx context.Context, entry *AuditEntry) error
	Get(ctx context.Context, id string) (*AuditEntry, error)
	List(ctx context.Context, filter AuditFilter) ([]*AuditEntry, int64, error)
	Delete(ctx context.Context, id string) error
	DeleteByFilter(ctx context.Context, filter AuditFilter) (int64, error)
	GetStats(ctx context.Context, filter AuditFilter) (*AuditStats, error)
}

// MemoryAuditStore implements AuditStore using in-memory storage
type MemoryAuditStore struct {
	mu      sync.RWMutex
	entries map[string]*AuditEntry
	index   []*AuditEntry
}

// NewMemoryAuditStore creates a new in-memory audit store
func NewMemoryAuditStore() *MemoryAuditStore {
	return &MemoryAuditStore{
		entries: make(map[string]*AuditEntry),
		index:   make([]*AuditEntry, 0),
	}
}

// Store stores an audit entry
func (s *MemoryAuditStore) Store(ctx context.Context, entry *AuditEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.entries[entry.ID] = entry
	s.index = append(s.index, entry)

	return nil
}

// Get retrieves an audit entry by ID
func (s *MemoryAuditStore) Get(ctx context.Context, id string) (*AuditEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entry, exists := s.entries[id]
	if !exists {
		return nil, ErrAuditLogNotFound
	}

	return entry, nil
}

// List retrieves audit entries based on filter
func (s *MemoryAuditStore) List(ctx context.Context, filter AuditFilter) ([]*AuditEntry, int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []*AuditEntry

	for _, entry := range s.index {
		if !s.matchesFilter(entry, filter) {
			continue
		}
		results = append(results, entry)
	}

	total := int64(len(results))

	// Apply pagination
	if filter.Offset > 0 && filter.Offset < len(results) {
		results = results[filter.Offset:]
	}
	if filter.Limit > 0 && filter.Limit < len(results) {
		results = results[:filter.Limit]
	}

	return results, total, nil
}

// matchesFilter checks if an entry matches the filter criteria
func (s *MemoryAuditStore) matchesFilter(entry *AuditEntry, filter AuditFilter) bool {
	if filter.UserID != "" && entry.UserID != filter.UserID {
		return false
	}
	if filter.ResourceType != "" && entry.ResourceType != filter.ResourceType {
		return false
	}
	if filter.ResourceID != "" && entry.ResourceID != filter.ResourceID {
		return false
	}
	if filter.Category != "" && entry.Category != filter.Category {
		return false
	}
	if filter.Level != "" && entry.Level != filter.Level {
		return false
	}
	if filter.Action != "" && entry.Action != filter.Action {
		return false
	}
	if filter.Success != nil && entry.Success != *filter.Success {
		return false
	}
	if filter.IPAddress != "" && entry.IPAddress != filter.IPAddress {
		return false
	}
	if filter.StartTime != nil && entry.Timestamp.Before(*filter.StartTime) {
		return false
	}
	if filter.EndTime != nil && entry.Timestamp.After(*filter.EndTime) {
		return false
	}
	return true
}

// Delete deletes an audit entry
func (s *MemoryAuditStore) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.entries[id]; !exists {
		return ErrAuditLogNotFound
	}

	delete(s.entries, id)

	// Remove from index
	for i, entry := range s.index {
		if entry.ID == id {
			s.index = append(s.index[:i], s.index[i+1:]...)
			break
		}
	}

	return nil
}

// DeleteByFilter deletes entries matching a filter
func (s *MemoryAuditStore) DeleteByFilter(ctx context.Context, filter AuditFilter) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var count int64
	var newIndex []*AuditEntry

	for _, entry := range s.index {
		if s.matchesFilter(entry, filter) {
			delete(s.entries, entry.ID)
			count++
		} else {
			newIndex = append(newIndex, entry)
		}
	}

	s.index = newIndex
	return count, nil
}

// GetStats returns statistics about audit logs
func (s *MemoryAuditStore) GetStats(ctx context.Context, filter AuditFilter) (*AuditStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := &AuditStats{
		EntriesByLevel:    make(map[AuditLevel]int64),
		EntriesByCategory: make(map[AuditCategory]int64),
	}

	users := make(map[string]bool)
	ips := make(map[string]bool)

	for _, entry := range s.index {
		if !s.matchesFilter(entry, filter) {
			continue
		}

		stats.TotalEntries++
		stats.EntriesByLevel[entry.Level]++
		stats.EntriesByCategory[entry.Category]++

		if !entry.Success {
			stats.FailedActions++
		}

		if entry.UserID != "" {
			users[entry.UserID] = true
		}
		if entry.IPAddress != "" {
			ips[entry.IPAddress] = true
		}

		// Update time range
		if stats.TimeRange.Start.IsZero() || entry.Timestamp.Before(stats.TimeRange.Start) {
			stats.TimeRange.Start = entry.Timestamp
		}
		if stats.TimeRange.End.IsZero() || entry.Timestamp.After(stats.TimeRange.End) {
			stats.TimeRange.End = entry.Timestamp
		}
	}

	stats.UniqueUsers = int64(len(users))
	stats.UniqueIPs = int64(len(ips))

	return stats, nil
}

// AuditConfig holds audit logger configuration
type AuditConfig struct {
	// Enabled determines if audit logging is active
	Enabled bool `json:"enabled"`

	// Level determines the minimum level to log
	Level AuditLevel `json:"level"`

	// IncludeRequestBody includes request body in logs
	IncludeRequestBody bool `json:"include_request_body"`

	// IncludeResponseBody includes response body in logs
	IncludeResponseBody bool `json:"include_response_body"`

	// IncludeHeaders includes headers in logs
	IncludeHeaders bool `json:"include_headers"`

	// SensitiveHeaders lists headers to exclude from logging
	SensitiveHeaders []string `json:"sensitive_headers"`

	// SensitiveFields lists fields to mask in logs
	SensitiveFields []string `json:"sensitive_fields"`

	// RetentionDays is the number of days to retain logs
	RetentionDays int `json:"retention_days"`

	// Output is where to write logs (stdout, stderr, or file path)
	Output string `json:"output"`

	// Format is the output format (json or text)
	Format string `json:"format"`
}

// DefaultAuditConfig returns default audit configuration
func DefaultAuditConfig() *AuditConfig {
	return &AuditConfig{
		Enabled:           true,
		Level:             AuditLevelInfo,
		IncludeRequestBody:  false,
		IncludeResponseBody: false,
		IncludeHeaders:     false,
		SensitiveHeaders:   []string{"Authorization", "Cookie", "X-API-Key"},
		SensitiveFields:    []string{"password", "secret", "token", "api_key"},
		RetentionDays:      90,
		Output:             "stdout",
		Format:             "json",
	}
}

// AuditLogger handles audit logging
type AuditLogger struct {
	store  AuditStore
	config *AuditConfig
	output io.Writer
	mu     sync.Mutex
}

// NewAuditLogger creates a new audit logger
func NewAuditLogger(store AuditStore, config *AuditConfig) (*AuditLogger, error) {
	if config == nil {
		config = DefaultAuditConfig()
	}

	logger := &AuditLogger{
		store:  store,
		config: config,
	}

	// Set output
	switch config.Output {
	case "stdout", "":
		logger.output = os.Stdout
	case "stderr":
		logger.output = os.Stderr
	default:
		file, err := os.OpenFile(config.Output, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return nil, fmt.Errorf("failed to open audit log file: %w", err)
		}
		logger.output = file
	}

	return logger, nil
}

// Log creates and stores an audit entry
func (l *AuditLogger) Log(ctx context.Context, entry *AuditEntry) error {
	if !l.config.Enabled {
		return nil
	}

	// Check level
	if !l.shouldLog(entry.Level) {
		return nil
	}

	// Set ID if not provided
	if entry.ID == "" {
		entry.ID = generateAuditID()
	}

	// Set timestamp if not provided
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}

	// Mask sensitive data
	entry = l.maskSensitiveData(entry)

	// Store entry
	if err := l.store.Store(ctx, entry); err != nil {
		return fmt.Errorf("failed to store audit entry: %w", err)
	}

	// Write to output
	if err := l.writeEntry(entry); err != nil {
		return fmt.Errorf("failed to write audit entry: %w", err)
	}

	return nil
}

// shouldLog checks if the entry should be logged based on level
func (l *AuditLogger) shouldLog(level AuditLevel) bool {
	levels := map[AuditLevel]int{
		AuditLevelInfo:     0,
		AuditLevelWarning:  1,
		AuditLevelError:    2,
		AuditLevelCritical: 3,
	}

	return levels[level] >= levels[l.config.Level]
}

// maskSensitiveData masks sensitive fields in the audit entry
func (l *AuditLogger) maskSensitiveData(entry *AuditEntry) *AuditEntry {
	if entry.Details == nil {
		return entry
	}

	// Create a copy to avoid modifying the original
	details := make(map[string]interface{})
	for k, v := range entry.Details {
		if l.isSensitiveField(k) {
			details[k] = "***MASKED***"
		} else {
			details[k] = v
		}
	}
	entry.Details = details

	return entry
}

// isSensitiveField checks if a field is sensitive
func (l *AuditLogger) isSensitiveField(field string) bool {
	for _, sensitive := range l.config.SensitiveFields {
		if field == sensitive {
			return true
		}
	}
	return false
}

// writeEntry writes an entry to the output
func (l *AuditLogger) writeEntry(entry *AuditEntry) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	var data []byte
	var err error

	if l.config.Format == "json" {
		data, err = json.Marshal(entry)
	} else {
		data = []byte(l.formatTextEntry(entry))
	}

	if err != nil {
		return err
	}

	_, err = l.output.Write(append(data, '\n'))
	return err
}

// formatTextEntry formats an entry as text
func (l *AuditLogger) formatTextEntry(entry *AuditEntry) string {
	return fmt.Sprintf("[%s] [%s] [%s] %s - User: %s, Resource: %s/%s, Success: %v",
		entry.Timestamp.Format(time.RFC3339),
		entry.Level,
		entry.Category,
		entry.Action,
		entry.UserID,
		entry.ResourceType,
		entry.ResourceID,
		entry.Success,
	)
}

// LogAuthentication logs an authentication event
func (l *AuditLogger) LogAuthentication(ctx context.Context, userID, action string, success bool, details map[string]interface{}) error {
	entry := &AuditEntry{
		Level:        AuditLevelInfo,
		Category:     CategoryAuthentication,
		Action:       action,
		UserID:       userID,
		ResourceType: "user",
		ResourceID:   userID,
		Success:      success,
		Details:      details,
	}

	if !success {
		entry.Level = AuditLevelWarning
	}

	return l.Log(ctx, entry)
}

// LogDataAccess logs a data access event
func (l *AuditLogger) LogDataAccess(ctx context.Context, userID, resourceType, resourceID, action string, success bool) error {
	entry := &AuditEntry{
		Level:        AuditLevelInfo,
		Category:     CategoryDataAccess,
		Action:       action,
		UserID:       userID,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Success:      success,
	}

	return l.Log(ctx, entry)
}

// LogDataModification logs a data modification event
func (l *AuditLogger) LogDataModification(ctx context.Context, userID, resourceType, resourceID, action string, changes *ChangeSet) error {
	entry := &AuditEntry{
		Level:        AuditLevelInfo,
		Category:     CategoryDataModification,
		Action:       action,
		UserID:       userID,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Success:      true,
		Changes:      changes,
	}

	return l.Log(ctx, entry)
}

// LogAPIRequest logs an API request
func (l *AuditLogger) LogAPIRequest(ctx context.Context, userID, method, path string, statusCode int, duration time.Duration, details map[string]interface{}) error {
	entry := &AuditEntry{
		Level:      AuditLevelInfo,
		Category:   CategoryAPI,
		Action:     "api_request",
		UserID:     userID,
		Method:     method,
		Path:       path,
		StatusCode: statusCode,
		Duration:   duration,
		Success:    statusCode >= 200 && statusCode < 400,
		Details:    details,
	}

	if statusCode >= 400 {
		entry.Level = AuditLevelWarning
	}
	if statusCode >= 500 {
		entry.Level = AuditLevelError
	}

	return l.Log(ctx, entry)
}

// LogSecurityEvent logs a security event
func (l *AuditLogger) LogSecurityEvent(ctx context.Context, action string, level AuditLevel, details map[string]interface{}) error {
	entry := &AuditEntry{
		Level:    level,
		Category: CategorySecurity,
		Action:   action,
		Success:  level != AuditLevelCritical && level != AuditLevelError,
		Details:  details,
	}

	return l.Log(ctx, entry)
}

// Get retrieves an audit entry by ID
func (l *AuditLogger) Get(ctx context.Context, id string) (*AuditEntry, error) {
	return l.store.Get(ctx, id)
}

// List retrieves audit entries based on filter
func (l *AuditLogger) List(ctx context.Context, filter AuditFilter) ([]*AuditEntry, int64, error) {
	return l.store.List(ctx, filter)
}

// GetStats returns statistics about audit logs
func (l *AuditLogger) GetStats(ctx context.Context, filter AuditFilter) (*AuditStats, error) {
	return l.store.GetStats(ctx, filter)
}

// Delete deletes an audit entry
func (l *AuditLogger) Delete(ctx context.Context, id string) error {
	return l.store.Delete(ctx, id)
}

// DeleteByFilter deletes entries matching a filter
func (l *AuditLogger) DeleteByFilter(ctx context.Context, filter AuditFilter) (int64, error) {
	return l.store.DeleteByFilter(ctx, filter)
}

// CleanupOldEntries removes entries older than retention period
func (l *AuditLogger) CleanupOldEntries(ctx context.Context) (int64, error) {
	if l.config.RetentionDays <= 0 {
		return 0, nil
	}

	cutoff := time.Now().AddDate(0, 0, -l.config.RetentionDays)
	filter := AuditFilter{
		EndTime: &cutoff,
	}

	return l.store.DeleteByFilter(ctx, filter)
}

// generateAuditID generates a unique audit entry ID
func generateAuditID() string {
	return fmt.Sprintf("audit_%d", time.Now().UnixNano())
}

// AuditContextKey is used to store audit info in context
type AuditContextKey string

const (
	AuditKeyUserID    AuditContextKey = "user_id"
	AuditKeySessionID AuditContextKey = "session_id"
	AuditKeyRequestID AuditContextKey = "request_id"
	AuditKeyIPAddress AuditContextKey = "ip_address"
	AuditKeyUserAgent AuditContextKey = "user_agent"
)

// GetUserIDFromContext retrieves user ID from context
func GetUserIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(AuditKeyUserID).(string); ok {
		return v
	}
	return ""
}

// GetSessionIDFromContext retrieves session ID from context
func GetSessionIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(AuditKeySessionID).(string); ok {
		return v
	}
	return ""
}

// GetRequestIDFromContext retrieves request ID from context
func GetRequestIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(AuditKeyRequestID).(string); ok {
		return v
	}
	return ""
}

// GetIPAddressFromContext retrieves IP address from context
func GetIPAddressFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(AuditKeyIPAddress).(string); ok {
		return v
	}
	return ""
}

// GetUserAgentFromContext retrieves user agent from context
func GetUserAgentFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(AuditKeyUserAgent).(string); ok {
		return v
	}
	return ""
}

// WithAuditContext adds audit context values to a context
func WithAuditContext(ctx context.Context, userID, sessionID, requestID, ipAddress, userAgent string) context.Context {
	ctx = context.WithValue(ctx, AuditKeyUserID, userID)
	ctx = context.WithValue(ctx, AuditKeySessionID, sessionID)
	ctx = context.WithValue(ctx, AuditKeyRequestID, requestID)
	ctx = context.WithValue(ctx, AuditKeyIPAddress, ipAddress)
	ctx = context.WithValue(ctx, AuditKeyUserAgent, userAgent)
	return ctx
}

// NewEntry creates a new audit entry with context values
func (l *AuditLogger) NewEntry(ctx context.Context, level AuditLevel, category AuditCategory, action string) *AuditEntry {
	return &AuditEntry{
		ID:        generateAuditID(),
		Timestamp: time.Now(),
		Level:     level,
		Category:  category,
		Action:    action,
		UserID:    GetUserIDFromContext(ctx),
		SessionID: GetSessionIDFromContext(ctx),
		RequestID: GetRequestIDFromContext(ctx),
		IPAddress: GetIPAddressFromContext(ctx),
		UserAgent: GetUserAgentFromContext(ctx),
		Details:   make(map[string]interface{}),
		Metadata:  make(map[string]string),
	}
}

// AddDetail adds a detail to the audit entry
func (e *AuditEntry) AddDetail(key string, value interface{}) *AuditEntry {
	if e.Details == nil {
		e.Details = make(map[string]interface{})
	}
	e.Details[key] = value
	return e
}

// AddMetadata adds metadata to the audit entry
func (e *AuditEntry) AddMetadata(key, value string) *AuditEntry {
	if e.Metadata == nil {
		e.Metadata = make(map[string]string)
	}
	e.Metadata[key] = value
	return e
}

// SetResource sets the resource information
func (e *AuditEntry) SetResource(resourceType, resourceID string) *AuditEntry {
	e.ResourceType = resourceType
	e.ResourceID = resourceID
	return e
}

// SetUser sets the user information
func (e *AuditEntry) SetUser(userID, email string) *AuditEntry {
	e.UserID = userID
	e.UserEmail = email
	return e
}

// SetRequest sets the request information
func (e *AuditEntry) SetRequest(method, path string, statusCode int, duration time.Duration) *AuditEntry {
	e.Method = method
	e.Path = path
	e.StatusCode = statusCode
	e.Duration = duration
	e.Success = statusCode >= 200 && statusCode < 400
	return e
}

// SetChanges sets the change set
func (e *AuditEntry) SetChanges(before, after interface{}) *AuditEntry {
	e.Changes = &ChangeSet{
		Before: before,
		After:  after,
	}
	return e
}

// SetError sets the error information
func (e *AuditEntry) SetError(err error) *AuditEntry {
	e.Success = false
	if err != nil {
		e.ErrorMessage = err.Error()
	}
	return e
}

// Log logs the entry using the provided logger
func (e *AuditEntry) Log(ctx context.Context, logger *AuditLogger) error {
	return logger.Log(ctx, e)
}
