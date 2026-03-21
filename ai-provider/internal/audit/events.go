C:/Users/Administrator/projects/go/Netllm/ai-provider/internal/audit/events.go
package audit

import (
	"encoding/json"
	"fmt"
	"time"
)

// EventType represents the type of audit event
type EventType string

const (
	// Authentication events
	EventAuthLogin         EventType = "auth.login"
	EventAuthLogout        EventType = "auth.logout"
	EventAuthLoginFailed   EventType = "auth.login_failed"
	EventAuthRefresh       EventType = "auth.refresh"
	EventAuthPasswordReset EventType = "auth.password_reset"
	EventAuthPasswordChange EventType = "auth.password_change"
	EventAuthMFAEnabled    EventType = "auth.mfa_enabled"
	EventAuthMFADisabled   EventType = "auth.mfa_disabled"
	EventAuthMFAChallenge  EventType = "auth.mfa_challenge"
	EventAuthOAuthLogin    EventType = "auth.oauth_login"
	EventAuthAPIKeyCreated EventType = "auth.api_key_created"
	EventAuthAPIKeyRevoked EventType = "auth.api_key_revoked"
	EventAuthAPIKeyUsed    EventType = "auth.api_key_used"

	// User events
	EventUserCreated  EventType = "user.created"
	EventUserUpdated  EventType = "user.updated"
	EventUserDeleted  EventType = "user.deleted"
	EventUserViewed   EventType = "user.viewed"
	EventUserEnabled  EventType = "user.enabled"
	EventUserDisabled EventType = "user.disabled"
	EventUserLocked   EventType = "user.locked"
	EventUserUnlocked EventType = "user.unlocked"

	// Role events
	EventRoleCreated  EventType = "role.created"
	EventRoleUpdated  EventType = "role.updated"
	EventRoleDeleted  EventType = "role.deleted"
	EventRoleAssigned EventType = "role.assigned"
	EventRoleRevoked  EventType = "role.revoked"

	// Permission events
	EventPermissionGranted   EventType = "permission.granted"
	EventPermissionRevoked   EventType = "permission.revoked"
	EventPermissionChecked   EventType = "permission.checked"
	EventPermissionDenied    EventType = "permission.denied"

	// Model events
	EventModelCreated   EventType = "model.created"
	EventModelUpdated   EventType = "model.updated"
	EventModelDeleted   EventType = "model.deleted"
	EventModelDeployed  EventType = "model.deployed"
	EventModelUndeployed EventType = "model.undeployed"
	EventModelViewed    EventType = "model.viewed"
	EventModelExported  EventType = "model.exported"
	EventModelImported  EventType = "model.imported"

	// Inference events
	EventInferenceStarted  EventType = "inference.started"
	EventInferenceCompleted EventType = "inference.completed"
	EventInferenceFailed   EventType = "inference.failed"
	EventInferenceCanceled EventType = "inference.canceled"

	// API events
	EventAPIRequest  EventType = "api.request"
	EventAPIResponse EventType = "api.response"
	EventAPIError    EventType = "api.error"
	EventAPIRateLimited EventType = "api.rate_limited"

	// Organization events
	EventOrgCreated  EventType = "org.created"
	EventOrgUpdated  EventType = "org.updated"
	EventOrgDeleted  EventType = "org.deleted"
	EventOrgMemberAdded   EventType = "org.member_added"
	EventOrgMemberRemoved EventType = "org.member_removed"
	EventOrgMemberRoleChanged EventType = "org.member_role_changed"

	// Workspace events
	EventWorkspaceCreated  EventType = "workspace.created"
	EventWorkspaceUpdated  EventType = "workspace.updated"
	EventWorkspaceDeleted  EventType = "workspace.deleted"
	EventWorkspaceArchived EventType = "workspace.archived"

	// Billing events
	EventBillingPlanChanged  EventType = "billing.plan_changed"
	EventBillingPaymentProcessed EventType = "billing.payment_processed"
	EventBillingPaymentFailed EventType = "billing.payment_failed"
	EventBillingInvoiceGenerated EventType = "billing.invoice_generated"
	EventBillingUsageThreshold EventType = "billing.usage_threshold"

	// Configuration events
	EventConfigChanged EventType = "config.changed"
	EventConfigExported EventType = "config.exported"
	EventConfigImported EventType = "config.imported"

	// Security events
	EventSecurityAlert      EventType = "security.alert"
	EventSecurityBreach     EventType = "security.breach"
	EventSecurityScan       EventType = "security.scan"
	EventSecurityVulnerability EventType = "security.vulnerability"
	EventSecurityIncident   EventType = "security.incident"
	EventSuspiciousActivity EventType = "security.suspicious_activity"
	EventBruteForceAttempt  EventType = "security.brute_force"
	EventIPBlocked          EventType = "security.ip_blocked"
	EventIPUnblocked        EventType = "security.ip_unblocked"

	// Data events
	EventDataExported  EventType = "data.exported"
	EventDataImported  EventType = "data.imported"
	EventDataDeleted   EventType = "data.deleted"
	EventDataAccessed  EventType = "data.accessed"
	EventDataModified  EventType = "data.modified"
	EventDataBreached  EventType = "data.breached"

	// System events
	EventSystemStartup  EventType = "system.startup"
	EventSystemShutdown EventType = "system.shutdown"
	EventSystemError    EventType = "system.error"
	EventSystemConfig   EventType = "system.config"
	EventSystemHealth   EventType = "system.health"
	EventSystemBackup   EventType = "system.backup"
	EventSystemRestore  EventType = "system.restore"

	// Compliance events
	EventComplianceReportGenerated EventType = "compliance.report_generated"
	EventComplianceViolation EventType = "compliance.violation"
	EventComplianceRemediation EventType = "compliance.remediation"
	EventGDPRDataRequest EventType = "compliance.gdpr_data_request"
	EventGDPRDataDeletion EventType = "compliance.gdpr_data_deletion"
	EventConsentGiven    EventType = "compliance.consent_given"
	EventConsentWithdrawn EventType = "compliance.consent_withdrawn"

	// Integration events
	EventIntegrationConnected    EventType = "integration.connected"
	EventIntegrationDisconnected EventType = "integration.disconnected"
	EventIntegrationSync         EventType = "integration.sync"
	EventIntegrationError        EventType = "integration.error"
	EventWebhookSent             EventType = "webhook.sent"
	EventWebhookFailed           EventType = "webhook.failed"
)

// EventCategory represents the category of an event
type EventCategory string

const (
	CategoryAuthentication EventCategory = "authentication"
	CategoryAuthorization  EventCategory = "authorization"
	CategoryUser          EventCategory = "user"
	CategoryModel         EventCategory = "model"
	CategoryInference     EventCategory = "inference"
	CategoryAPI           EventCategory = "api"
	CategoryOrganization  EventCategory = "organization"
	CategoryWorkspace     EventCategory = "workspace"
	CategoryBilling       EventCategory = "billing"
	CategoryConfiguration EventCategory = "configuration"
	CategorySecurity      EventCategory = "security"
	CategoryData          EventCategory = "data"
	CategorySystem        EventCategory = "system"
	CategoryCompliance    EventCategory = "compliance"
	CategoryIntegration   EventCategory = "integration"
)

// EventSeverity represents the severity level of an event
type EventSeverity string

const (
	SeverityDebug   EventSeverity = "debug"
	SeverityInfo    EventSeverity = "info"
	SeverityNotice  EventSeverity = "notice"
	SeverityWarning EventSeverity = "warning"
	SeverityError   EventSeverity = "error"
	SeverityCritical EventSeverity = "critical"
	SeverityAlert   EventSeverity = "alert"
	SeverityEmergency EventSeverity = "emergency"
)

// EventStatus represents the outcome of an event
type EventStatus string

const (
	StatusSuccess EventStatus = "success"
	StatusFailure EventStatus = "failure"
	StatusPending EventStatus = "pending"
	StatusUnknown EventStatus = "unknown"
)

// Event represents an audit event
type Event struct {
	// Unique identifier for the event
	ID string `json:"id"`

	// Type of event
	Type EventType `json:"type"`

	// Category of event
	Category EventCategory `json:"category"`

	// Severity level
	Severity EventSeverity `json:"severity"`

	// Status of the event
	Status EventStatus `json:"status"`

	// Human-readable message
	Message string `json:"message"`

	// Description with more details
	Description string `json:"description,omitempty"`

	// Timestamp when the event occurred
	Timestamp time.Time `json:"timestamp"`

	// Actor who triggered the event
	Actor Actor `json:"actor"`

	// Resource affected by the event
	Resource Resource `json:"resource"`

	// Additional metadata
	Metadata map[string]interface{} `json:"metadata,omitempty"`

	// Request information
	Request *RequestInfo `json:"request,omitempty"`

	// Response information
	Response *ResponseInfo `json:"response,omitempty"`

	// Changes made (for update events)
	Changes []*Change `json:"changes,omitempty"`

	// Tags for categorization and filtering
	Tags []string `json:"tags,omitempty"`

	// Correlation ID for tracing related events
	CorrelationID string `json:"correlation_id,omitempty"`

	// Session ID associated with the event
	SessionID string `json:"session_id,omitempty"`

	// Organization ID
	OrganizationID string `json:"organization_id,omitempty"`

	// Workspace ID
	WorkspaceID string `json:"workspace_id,omitempty"`

	// Retention period for this event
	RetentionDays int `json:"retention_days,omitempty"`

	// Compliance tags (e.g., "gdpr", "hipaa", "pci-dss")
	ComplianceTags []string `json:"compliance_tags,omitempty"`
}

// Actor represents the entity that triggered an event
type Actor struct {
	// User ID
	ID string `json:"id,omitempty"`

	// User email
	Email string `json:"email,omitempty"`

	// Username
	Username string `json:"username,omitempty"`

	// Type of actor (user, system, service, api_key)
	Type string `json:"type"`

	// Actor name for display
	Name string `json:"name,omitempty"`

	// IP address of the actor
	IPAddress string `json:"ip_address,omitempty"`

	// User agent string
	UserAgent string `json:"user_agent,omitempty"`

	// API key ID if using API key authentication
	APIKeyID string `json:"api_key_id,omitempty"`

	// Role of the actor
	Role string `json:"role,omitempty"`
}

// Resource represents a resource affected by an event
type Resource struct {
	// Resource type
	Type string `json:"type"`

	// Resource ID
	ID string `json:"id"`

	// Resource name
	Name string `json:"name,omitempty"`

	// Resource attributes
	Attributes map[string]interface{} `json:"attributes,omitempty"`
}

// RequestInfo contains HTTP request information
type RequestInfo struct {
	// HTTP method
	Method string `json:"method"`

	// Request path
	Path string `json:"path"`

	// Query parameters
	Query string `json:"query,omitempty"`

	// Request headers (sanitized)
	Headers map[string]string `json:"headers,omitempty"`

	// Request body size
	BodySize int64 `json:"body_size,omitempty"`

	// Content type
	ContentType string `json:"content_type,omitempty"`
}

// ResponseInfo contains HTTP response information
type ResponseInfo struct {
	// HTTP status code
	StatusCode int `json:"status_code"`

	// Response headers (sanitized)
	Headers map[string]string `json:"headers,omitempty"`

	// Response body size
	BodySize int64 `json:"body_size,omitempty"`

	// Response time in milliseconds
	ResponseTime int64 `json:"response_time_ms,omitempty"`
}

// Change represents a field change in an update event
type Change struct {
	// Field name
	Field string `json:"field"`

	// Old value
	OldValue interface{} `json:"old_value,omitempty"`

	// New value
	NewValue interface{} `json:"new_value,omitempty"`

	// Type of change (create, update, delete)
	Type string `json:"type"`
}

// ToJSON serializes the event to JSON
func (e *Event) ToJSON() ([]byte, error) {
	return json.Marshal(e)
}

// FromJSON deserializes the event from JSON
func (e *Event) FromJSON(data []byte) error {
	return json.Unmarshal(data, e)
}

// IsHighSeverity returns true if the event is high severity
func (e *Event) IsHighSeverity() bool {
	return e.Severity == SeverityError ||
		e.Severity == SeverityCritical ||
		e.Severity == SeverityAlert ||
		e.Severity == SeverityEmergency
}

// IsSecurityEvent returns true if this is a security-related event
func (e *Event) IsSecurityEvent() bool {
	return e.Category == CategorySecurity ||
		e.Type == EventAuthLoginFailed ||
		e.Type == EventAuthMFAChallenge ||
		e.Type == EventPermissionDenied ||
		e.Type == EventSuspiciousActivity ||
		e.Type == EventBruteForceAttempt
}

// IsComplianceEvent returns true if this is a compliance-related event
func (e *Event) IsComplianceEvent() bool {
	return e.Category == CategoryCompliance ||
		e.Type == EventGDPRDataRequest ||
		e.Type == EventGDPRDataDeletion ||
		e.Type == EventConsentGiven ||
		e.Type == EventConsentWithdrawn ||
		len(e.ComplianceTags) > 0
}

// EventBuilder provides a fluent interface for building events
type EventBuilder struct {
	event *Event
}

// NewEventBuilder creates a new event builder
func NewEventBuilder(eventType EventType) *EventBuilder {
	return &EventBuilder{
		event: &Event{
			ID:        generateEventID(),
			Type:      eventType,
			Timestamp: time.Now(),
			Severity:  SeverityInfo,
			Status:    StatusSuccess,
			Metadata:  make(map[string]interface{}),
			Tags:      []string{},
		},
	}
}

// WithCategory sets the event category
func (b *EventBuilder) WithCategory(category EventCategory) *EventBuilder {
	b.event.Category = category
	return b
}

// WithSeverity sets the event severity
func (b *EventBuilder) WithSeverity(severity EventSeverity) *EventBuilder {
	b.event.Severity = severity
	return b
}

// WithStatus sets the event status
func (b *EventBuilder) WithStatus(status EventStatus) *EventBuilder {
	b.event.Status = status
	return b
}

// WithMessage sets the event message
func (b *EventBuilder) WithMessage(message string) *EventBuilder {
	b.event.Message = message
	return b
}

// WithDescription sets the event description
func (b *EventBuilder) WithDescription(description string) *EventBuilder {
	b.event.Description = description
	return b
}

// WithActor sets the actor information
func (b *EventBuilder) WithActor(actor Actor) *EventBuilder {
	b.event.Actor = actor
	return b
}

// WithResource sets the resource information
func (b *EventBuilder) WithResource(resource Resource) *EventBuilder {
	b.event.Resource = resource
	return b
}

// WithMetadata adds metadata to the event
func (b *EventBuilder) WithMetadata(key string, value interface{}) *EventBuilder {
	b.event.Metadata[key] = value
	return b
}

// WithRequest sets the request information
func (b *EventBuilder) WithRequest(request *RequestInfo) *EventBuilder {
	b.event.Request = request
	return b
}

// WithResponse sets the response information
func (b *EventBuilder) WithResponse(response *ResponseInfo) *EventBuilder {
	b.event.Response = response
	return b
}

// WithChange adds a change to the event
func (b *EventBuilder) WithChange(change *Change) *EventBuilder {
	b.event.Changes = append(b.event.Changes, change)
	return b
}

// WithTag adds a tag to the event
func (b *EventBuilder) WithTag(tag string) *EventBuilder {
	b.event.Tags = append(b.event.Tags, tag)
	return b
}

// WithCorrelationID sets the correlation ID
func (b *EventBuilder) WithCorrelationID(id string) *EventBuilder {
	b.event.CorrelationID = id
	return b
}

// WithSessionID sets the session ID
func (b *EventBuilder) WithSessionID(id string) *EventBuilder {
	b.event.SessionID = id
	return b
}

// WithOrganizationID sets the organization ID
func (b *EventBuilder) WithOrganizationID(id string) *EventBuilder {
	b.event.OrganizationID = id
	return b
}

// WithWorkspaceID sets the workspace ID
func (b *EventBuilder) WithWorkspaceID(id string) *EventBuilder {
	b.event.WorkspaceID = id
	return b
}

// WithRetention sets the retention period
func (b *EventBuilder) WithRetention(days int) *EventBuilder {
	b.event.RetentionDays = days
	return b
}

// WithComplianceTag adds a compliance tag
func (b *EventBuilder) WithComplianceTag(tag string) *EventBuilder {
	b.event.ComplianceTags = append(b.event.ComplianceTags, tag)
	return b
}

// Build returns the built event
func (b *EventBuilder) Build() *Event {
	// Set default category based on event type if not set
	if b.event.Category == "" {
		b.event.Category = GetEventCategory(b.event.Type)
	}

	return b.event
}

// GetEventCategory returns the default category for an event type
func GetEventCategory(eventType EventType) EventCategory {
	prefix := string(eventType)
	if idx := indexByte(prefix, '.'); idx > 0 {
		prefix = prefix[:idx]
	}

	switch prefix {
	case "auth":
		return CategoryAuthentication
	case "user":
		return CategoryUser
	case "role":
		return CategoryAuthorization
	case "permission":
		return CategoryAuthorization
	case "model":
		return CategoryModel
	case "inference":
		return CategoryInference
	case "api":
		return CategoryAPI
	case "org":
		return CategoryOrganization
	case "workspace":
		return CategoryWorkspace
	case "billing":
		return CategoryBilling
	case "config":
		return CategoryConfiguration
	case "security":
		return CategorySecurity
	case "data":
		return CategoryData
	case "system":
		return CategorySystem
	case "compliance":
		return CategoryCompliance
	case "integration", "webhook":
		return CategoryIntegration
	default:
		return CategorySystem
	}
}

// indexByte finds the index of a byte in a string
func indexByte(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}

// generateEventID generates a unique event ID
func generateEventID() string {
	return fmt.Sprintf("evt_%d", time.Now().UnixNano())
}

// StandardEventDefinitions contains standard event definitions
var StandardEventDefinitions = map[EventType]EventDefinition{
	EventAuthLogin: {
		Type:        EventAuthLogin,
		Category:    CategoryAuthentication,
		Severity:    SeverityInfo,
		Description: "User logged in successfully",
		Tags:        []string{"authentication", "login"},
	},
	EventAuthLoginFailed: {
		Type:        EventAuthLoginFailed,
		Category:    CategoryAuthentication,
		Severity:    SeverityWarning,
		Description: "User login attempt failed",
		Tags:        []string{"authentication", "login", "failed"},
	},
	EventAuthLogout: {
		Type:        EventAuthLogout,
		Category:    CategoryAuthentication,
		Severity:    SeverityInfo,
		Description: "User logged out",
		Tags:        []string{"authentication", "logout"},
	},
	EventUserCreated: {
		Type:        EventUserCreated,
		Category:    CategoryUser,
		Severity:    SeverityInfo,
		Description: "New user account created",
		Tags:        []string{"user", "create"},
	},
	EventUserDeleted: {
		Type:        EventUserDeleted,
		Category:    CategoryUser,
		Severity:    SeverityWarning,
		Description: "User account deleted",
		Tags:        []string{"user", "delete"},
	},
	EventSecurityAlert: {
		Type:        EventSecurityAlert,
		Category:    CategorySecurity,
		Severity:    SeverityCritical,
		Description: "Security alert triggered",
		Tags:        []string{"security", "alert"},
	},
	EventSuspiciousActivity: {
		Type:        EventSuspiciousActivity,
		Category:    CategorySecurity,
		Severity:    SeverityWarning,
		Description: "Suspicious activity detected",
		Tags:        []string{"security", "suspicious"},
	},
	EventGDPRDataRequest: {
		Type:           EventGDPRDataRequest,
		Category:       CategoryCompliance,
		Severity:       SeverityNotice,
		Description:    "GDPR data request received",
		Tags:           []string{"compliance", "gdpr"},
		ComplianceTags: []string{"gdpr"},
	},
}

// EventDefinition defines the properties of an event type
type EventDefinition struct {
	Type           EventType     `json:"type"`
	Category       EventCategory `json:"category"`
	Severity       EventSeverity `json:"severity"`
	Description    string        `json:"description"`
	Tags           []string      `json:"tags"`
	ComplianceTags []string      `json:"compliance_tags,omitempty"`
	RetentionDays  int           `json:"retention_days,omitempty"`
}

// GetEventDefinition returns the definition for an event type
func GetEventDefinition(eventType EventType) (EventDefinition, bool) {
	def, ok := StandardEventDefinitions[eventType]
	return def, ok
}

// EventFilter is used to filter events
type EventFilter struct {
	Types       []EventType   `json:"types,omitempty"`
	Categories  []EventCategory `json:"categories,omitempty"`
	Severities  []EventSeverity `json:"severities,omitempty"`
	Statuses    []EventStatus `json:"statuses,omitempty"`
	UserIDs     []string      `json:"user_ids,omitempty"`
	ResourceIDs []string      `json:"resource_ids,omitempty"`
	ResourceTypes []string    `json:"resource_types,omitempty"`
	OrganizationIDs []string  `json:"organization_ids,omitempty"`
	WorkspaceIDs []string     `json:"workspace_ids,omitempty"`
	Tags        []string      `json:"tags,omitempty"`
	ComplianceTags []string   `json:"compliance_tags,omitempty"`
	StartTime   *time.Time    `json:"start_time,omitempty"`
	EndTime     *time.Time    `json:"end_time,omitempty"`
	SearchQuery string        `json:"search_query,omitempty"`
}

// Matches checks if an event matches the filter
func (f *EventFilter) Matches(event *Event) bool {
	// Check event types
	if len(f.Types) > 0 {
		found := false
		for _, t := range f.Types {
			if event.Type == t {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Check categories
	if len(f.Categories) > 0 {
		found := false
		for _, c := range f.Categories {
			if event.Category == c {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Check severities
	if len(f.Severities) > 0 {
		found := false
		for _, s := range f.Severities {
			if event.Severity == s {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Check time range
	if f.StartTime != nil && event.Timestamp.Before(*f.StartTime) {
		return false
	}
	if f.EndTime != nil && event.Timestamp.After(*f.EndTime) {
		return false
	}

	// Check user IDs
	if len(f.UserIDs) > 0 {
		found := false
		for _, id := range f.UserIDs {
			if event.Actor.ID == id {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Check resource IDs
	if len(f.ResourceIDs) > 0 {
		found := false
		for _, id := range f.ResourceIDs {
			if event.Resource.ID == id {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Check organization ID
	if len(f.OrganizationIDs) > 0 {
		found := false
		for _, id := range f.OrganizationIDs {
			if event.OrganizationID == id {
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

// EventSummary represents a summary of events
type EventSummary struct {
	TotalCount       int64                   `json:"total_count"`
	CountByType      map[EventType]int64     `json:"count_by_type"`
	CountByCategory  map[EventCategory]int64 `json:"count_by_category"`
	CountBySeverity  map[EventSeverity]int64 `json:"count_by_severity"`
	CountByStatus    map[EventStatus]int64   `json:"count_by_status"`
	FirstEventTime   *time.Time              `json:"first_event_time,omitempty"`
	LastEventTime    *time.Time              `json:"last_event_time,omitempty"`
	UniqueActors    int64                   `json:"unique_actors"`
	UniqueResources  int64                   `json:"unique_resources"`
}

// NewEventSummary creates a new event summary
func NewEventSummary() *EventSummary {
	return &EventSummary{
		CountByType:     make(map[EventType]int64),
		CountByCategory: make(map[EventCategory]int64),
		CountBySeverity: make(map[EventSeverity]int64),
		CountByStatus:   make(map[EventStatus]int64),
	}
}

// AddEvent adds an event to the summary
func (s *EventSummary) AddEvent(event *Event) {
	s.TotalCount++
	s.CountByType[event.Type]++
	s.CountByCategory[event.Category]++
	s.CountBySeverity[event.Severity]++
	s.CountByStatus[event.Status]++

	if s.FirstEventTime == nil || event.Timestamp.Before(*s.FirstEventTime) {
		t := event.Timestamp
		s.FirstEventTime = &t
	}
	if s.LastEventTime == nil || event.Timestamp.After(*s.LastEventTime) {
		t := event.Timestamp
		s.LastEventTime = &t
	}
}
