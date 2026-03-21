package audit

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Compliance errors
var (
	ErrComplianceReportNotFound = errors.New("compliance report not found")
	ErrInvalidComplianceRule    = errors.New("invalid compliance rule")
	ErrConsentNotFound          = errors.New("consent not found")
	ErrDataRetentionViolation   = errors.New("data retention violation")
	ErrGDPRViolation            = errors.New("GDPR violation detected")
)

// ComplianceFramework represents a compliance framework
type ComplianceFramework string

const (
	FrameworkGDPR       ComplianceFramework = "gdpr"
	FrameworkCCPA       ComplianceFramework = "ccpa"
	FrameworkHIPAA      ComplianceFramework = "hipaa"
	FrameworkSOC2       ComplianceFramework = "soc2"
	FrameworkISO27001   ComplianceFramework = "iso27001"
	FrameworkPCI_DSS    ComplianceFramework = "pci_dss"
)

// ComplianceStatus represents the status of compliance
type ComplianceStatus string

const (
	StatusCompliant    ComplianceStatus = "compliant"
	StatusNonCompliant ComplianceStatus = "non_compliant"
	StatusPending      ComplianceStatus = "pending"
	StatusUnknown      ComplianceStatus = "unknown"
)

// ConsentType represents types of user consent
type ConsentType string

const (
	ConsentTypeMarketing    ConsentType = "marketing"
	ConsentTypeAnalytics    ConsentType = "analytics"
	ConsentTypeThirdParty   ConsentType = "third_party"
	ConsentTypeCookies      ConsentType = "cookies"
	ConsentTypeDataProcessing ConsentType = "data_processing"
	ConsentTypeDataSharing  ConsentType = "data_sharing"
)

// ConsentStatus represents the status of consent
type ConsentStatus string

const (
	ConsentStatusGranted  ConsentStatus = "granted"
	ConsentStatusWithdrawn ConsentStatus = "withdrawn"
	ConsentStatusPending  ConsentStatus = "pending"
)

// Consent represents a user's consent record
type Consent struct {
	ID           string        `json:"id"`
	UserID       string        `json:"user_id"`
	ConsentType  ConsentType   `json:"consent_type"`
	Status       ConsentStatus `json:"status"`
	GrantedAt    *time.Time    `json:"granted_at,omitempty"`
	WithdrawnAt  *time.Time    `json:"withdrawn_at,omitempty"`
	Source       string        `json:"source"` // web, api, mobile
	IPAddress    string        `json:"ip_address,omitempty"`
	UserAgent    string        `json:"user_agent,omitempty"`
	Version      string        `json:"version"` // consent version
	Notes        string        `json:"notes,omitempty"`
	CreatedAt    time.Time     `json:"created_at"`
	UpdatedAt    time.Time     `json:"updated_at"`
}

// DataRetentionPolicy represents a data retention policy
type DataRetentionPolicy struct {
	ID             string        `json:"id"`
	Name           string        `json:"name"`
	Description    string        `json:"description"`
	ResourceType   string        `json:"resource_type"`
	RetentionDays  int           `json:"retention_days"`
	Action         string        `json:"action"` // delete, archive, anonymize
	Enabled        bool          `json:"enabled"`
	CreatedAt      time.Time     `json:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
}

// DataSubjectRequest represents a GDPR data subject request
type DataSubjectRequest struct {
	ID            string              `json:"id"`
	UserID        string              `json:"user_id"`
	Email         string              `json:"email"`
	RequestType   string              `json:"request_type"` // access, erasure, portability, rectification
	Status        string              `json:"status"` // pending, processing, completed, rejected
	Details       string              `json:"details,omitempty"`
	RequestDate   time.Time           `json:"request_date"`
	DueDate       time.Time           `json:"due_date"`
	CompletedDate *time.Time          `json:"completed_date,omitempty"`
	HandledBy     string              `json:"handled_by,omitempty"`
	Notes         string              `json:"notes,omitempty"`
	ResponseData  map[string]interface{} `json:"response_data,omitempty"`
	CreatedAt     time.Time           `json:"created_at"`
	UpdatedAt     time.Time           `json:"updated_at"`
}

// ComplianceRule represents a compliance rule
type ComplianceRule struct {
	ID          string             `json:"id"`
	Framework   ComplianceFramework `json:"framework"`
	Code        string             `json:"code"` // e.g., "GDPR-7.1" for right to erasure
	Name        string             `json:"name"`
	Description string             `json:"description"`
	Category    string             `json:"category"`
	Severity    string             `json:"severity"` // low, medium, high, critical
	Enabled     bool               `json:"enabled"`
	CheckFunc   ComplianceCheckFunc `json:"-"`
	CreatedAt   time.Time          `json:"created_at"`
}

// ComplianceCheckFunc is a function that checks compliance
type ComplianceCheckFunc func(ctx context.Context, rule *ComplianceRule) (*ComplianceCheckResult, error)

// ComplianceCheckResult represents the result of a compliance check
type ComplianceCheckResult struct {
	RuleID      string             `json:"rule_id"`
	Status      ComplianceStatus   `json:"status"`
	Score       int                `json:"score"` // 0-100
	Findings    []ComplianceFinding `json:"findings"`
	CheckedAt   time.Time          `json:"checked_at"`
	Duration    time.Duration      `json:"duration"`
}

// ComplianceFinding represents a finding in a compliance check
type ComplianceFinding struct {
	ID          string                 `json:"id"`
	Severity    string                 `json:"severity"`
	Title       string                 `json:"title"`
	Description string                 `json:"description"`
	Resource    string                 `json:"resource,omitempty"`
	Remediation string                 `json:"remediation,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// ComplianceReport represents a compliance report
type ComplianceReport struct {
	ID           string                  `json:"id"`
	Name         string                  `json:"name"`
	Description  string                  `json:"description"`
	Framework    ComplianceFramework     `json:"framework"`
	Status       ComplianceStatus        `json:"status"`
	Score        int                     `json:"score"` // Overall compliance score
	Results      []*ComplianceCheckResult `json:"results"`
	Summary      *ComplianceSummary      `json:"summary"`
	GeneratedAt  time.Time               `json:"generated_at"`
	GeneratedBy  string                  `json:"generated_by"`
	PeriodStart  time.Time               `json:"period_start"`
	PeriodEnd    time.Time               `json:"period_end"`
	NextReviewAt time.Time               `json:"next_review_at"`
}

// ComplianceSummary represents a summary of compliance status
type ComplianceSummary struct {
	TotalRules      int `json:"total_rules"`
	Compliant       int `json:"compliant"`
	NonCompliant    int `json:"non_compliant"`
	Pending         int `json:"pending"`
	CriticalIssues  int `json:"critical_issues"`
	HighIssues      int `json:"high_issues"`
	MediumIssues    int `json:"medium_issues"`
	LowIssues       int `json:"low_issues"`
}

// ConsentStore defines the interface for consent storage
type ConsentStore interface {
	CreateConsent(ctx context.Context, consent *Consent) error
	GetConsent(ctx context.Context, id string) (*Consent, error)
	GetConsentByUserAndType(ctx context.Context, userID string, consentType ConsentType) (*Consent, error)
	ListConsentsByUser(ctx context.Context, userID string) ([]*Consent, error)
	UpdateConsent(ctx context.Context, consent *Consent) error
	DeleteConsent(ctx context.Context, id string) error
}

// DataRetentionPolicyStore defines the interface for retention policy storage
type DataRetentionPolicyStore interface {
	CreatePolicy(ctx context.Context, policy *DataRetentionPolicy) error
	GetPolicy(ctx context.Context, id string) (*DataRetentionPolicy, error)
	GetPolicyByResourceType(ctx context.Context, resourceType string) (*DataRetentionPolicy, error)
	ListPolicies(ctx context.Context) ([]*DataRetentionPolicy, error)
	UpdatePolicy(ctx context.Context, policy *DataRetentionPolicy) error
	DeletePolicy(ctx context.Context, id string) error
}

// DataSubjectRequestStore defines the interface for DSR storage
type DataSubjectRequestStore interface {
	CreateRequest(ctx context.Context, req *DataSubjectRequest) error
	GetRequest(ctx context.Context, id string) (*DataSubjectRequest, error)
	GetRequestByUser(ctx context.Context, userID string) ([]*DataSubjectRequest, error)
	ListRequests(ctx context.Context, status string, limit, offset int) ([]*DataSubjectRequest, error)
	UpdateRequest(ctx context.Context, req *DataSubjectRequest) error
	DeleteRequest(ctx context.Context, id string) error
}

// ComplianceReportStore defines the interface for compliance report storage
type ComplianceReportStore interface {
	CreateReport(ctx context.Context, report *ComplianceReport) error
	GetReport(ctx context.Context, id string) (*ComplianceReport, error)
	ListReports(ctx context.Context, framework ComplianceFramework, limit, offset int) ([]*ComplianceReport, error)
	DeleteReport(ctx context.Context, id string) error
}

// MemoryConsentStore implements ConsentStore using in-memory storage
type MemoryConsentStore struct {
	mu       sync.RWMutex
	consents map[string]*Consent
	byUser   map[string]map[string]*Consent // userID -> consentID -> Consent
}

// NewMemoryConsentStore creates a new in-memory consent store
func NewMemoryConsentStore() *MemoryConsentStore {
	return &MemoryConsentStore{
		consents: make(map[string]*Consent),
		byUser:   make(map[string]map[string]*Consent),
	}
}

// CreateConsent creates a new consent record
func (s *MemoryConsentStore) CreateConsent(ctx context.Context, consent *Consent) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.consents[consent.ID] = consent

	if _, exists := s.byUser[consent.UserID]; !exists {
		s.byUser[consent.UserID] = make(map[string]*Consent)
	}
	s.byUser[consent.UserID][consent.ID] = consent

	return nil
}

// GetConsent retrieves a consent by ID
func (s *MemoryConsentStore) GetConsent(ctx context.Context, id string) (*Consent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	consent, exists := s.consents[id]
	if !exists {
		return nil, ErrConsentNotFound
	}
	return consent, nil
}

// GetConsentByUserAndType retrieves consent by user and type
func (s *MemoryConsentStore) GetConsentByUserAndType(ctx context.Context, userID string, consentType ConsentType) (*Consent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	userConsents, exists := s.byUser[userID]
	if !exists {
		return nil, ErrConsentNotFound
	}

	for _, consent := range userConsents {
		if consent.ConsentType == consentType {
			return consent, nil
		}
	}

	return nil, ErrConsentNotFound
}

// ListConsentsByUser lists all consents for a user
func (s *MemoryConsentStore) ListConsentsByUser(ctx context.Context, userID string) ([]*Consent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	userConsents, exists := s.byUser[userID]
	if !exists {
		return []*Consent{}, nil
	}

	consents := make([]*Consent, 0, len(userConsents))
	for _, consent := range userConsents {
		consents = append(consents, consent)
	}
	return consents, nil
}

// UpdateConsent updates a consent record
func (s *MemoryConsentStore) UpdateConsent(ctx context.Context, consent *Consent) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.consents[consent.ID]; !exists {
		return ErrConsentNotFound
	}

	consent.UpdatedAt = time.Now()
	s.consents[consent.ID] = consent
	s.byUser[consent.UserID][consent.ID] = consent

	return nil
}

// DeleteConsent deletes a consent record
func (s *MemoryConsentStore) DeleteConsent(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	consent, exists := s.consents[id]
	if !exists {
		return nil
	}

	delete(s.consents, id)
	if userConsents, exists := s.byUser[consent.UserID]; exists {
		delete(userConsents, id)
	}

	return nil
}

// ComplianceService provides compliance management functionality
type ComplianceService struct {
	consentStore   ConsentStore
	retentionStore DataRetentionPolicyStore
	dsrStore       DataSubjectRequestStore
	reportStore    ComplianceReportStore
	rules          map[string]*ComplianceRule
	auditLogger    *AuditLogger
	mu             sync.RWMutex
}

// NewComplianceService creates a new compliance service
func NewComplianceService(
	consentStore ConsentStore,
	retentionStore DataRetentionPolicyStore,
	dsrStore DataSubjectRequestStore,
	reportStore ComplianceReportStore,
	auditLogger *AuditLogger,
) *ComplianceService {
	return &ComplianceService{
		consentStore:   consentStore,
		retentionStore: retentionStore,
		dsrStore:       dsrStore,
		reportStore:    reportStore,
		rules:          make(map[string]*ComplianceRule),
		auditLogger:    auditLogger,
	}
}

// GrantConsent grants consent for a user
func (s *ComplianceService) GrantConsent(ctx context.Context, userID string, consentType ConsentType, source, ipAddr, userAgent, version string) (*Consent, error) {
	now := time.Now()
	consent := &Consent{
		ID:          generateComplianceID(),
		UserID:      userID,
		ConsentType: consentType,
		Status:      ConsentStatusGranted,
		GrantedAt:   &now,
		Source:      source,
		IPAddress:   ipAddr,
		UserAgent:   userAgent,
		Version:     version,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := s.consentStore.CreateConsent(ctx, consent); err != nil {
		return nil, err
	}

	// Log the consent grant
	if s.auditLogger != nil {
		s.auditLogger.Log(ctx, &AuditEvent{
			EventType:    "consent.granted",
			UserID:       userID,
			ResourceType: "consent",
			ResourceID:   consent.ID,
			Details: map[string]interface{}{
				"consent_type": consentType,
				"source":       source,
				"version":      version,
			},
			IPAddress: ipAddr,
		})
	}

	return consent, nil
}

// WithdrawConsent withdraws consent for a user
func (s *ComplianceService) WithdrawConsent(ctx context.Context, userID string, consentType ConsentType) error {
	consent, err := s.consentStore.GetConsentByUserAndType(ctx, userID, consentType)
	if err != nil {
		return err
	}

	now := time.Now()
	consent.Status = ConsentStatusWithdrawn
	consent.WithdrawnAt = &now
	consent.UpdatedAt = now

	if err := s.consentStore.UpdateConsent(ctx, consent); err != nil {
		return err
	}

	// Log the consent withdrawal
	if s.auditLogger != nil {
		s.auditLogger.Log(ctx, &AuditEvent{
			EventType:    "consent.withdrawn",
			UserID:       userID,
			ResourceType: "consent",
			ResourceID:   consent.ID,
			Details: map[string]interface{}{
				"consent_type": consentType,
			},
		})
	}

	return nil
}

// GetConsentStatus gets the consent status for a user and type
func (s *ComplianceService) GetConsentStatus(ctx context.Context, userID string, consentType ConsentType) (ConsentStatus, error) {
	consent, err := s.consentStore.GetConsentByUserAndType(ctx, userID, consentType)
	if err != nil {
		return ConsentStatusPending, nil // No consent = pending
	}
	return consent.Status, nil
}

// HasConsent checks if a user has granted consent
func (s *ComplianceService) HasConsent(ctx context.Context, userID string, consentType ConsentType) bool {
	status, _ := s.GetConsentStatus(ctx, userID, consentType)
	return status == ConsentStatusGranted
}

// ListUserConsents lists all consents for a user
func (s *ComplianceService) ListUserConsents(ctx context.Context, userID string) ([]*Consent, error) {
	return s.consentStore.ListConsentsByUser(ctx, userID)
}

// CreateDataSubjectRequest creates a new data subject request
func (s *ComplianceService) CreateDataSubjectRequest(ctx context.Context, userID, email, requestType, details string) (*DataSubjectRequest, error) {
	now := time.Now()
	// GDPR requires response within 30 days
	dueDate := now.AddDate(0, 0, 30)

	req := &DataSubjectRequest{
		ID:          generateComplianceID(),
		UserID:      userID,
		Email:       email,
		RequestType: requestType,
		Status:      "pending",
		Details:     details,
		RequestDate: now,
		DueDate:     dueDate,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := s.dsrStore.CreateRequest(ctx, req); err != nil {
		return nil, err
	}

	// Log the DSR
	if s.auditLogger != nil {
		s.auditLogger.Log(ctx, &AuditEvent{
			EventType:    "dsr.created",
			UserID:       userID,
			ResourceType: "data_subject_request",
			ResourceID:   req.ID,
			Details: map[string]interface{}{
				"request_type": requestType,
				"email":        email,
				"due_date":     dueDate,
			},
		})
	}

	return req, nil
}

// ProcessDataSubjectRequest processes a data subject request
func (s *ComplianceService) ProcessDataSubjectRequest(ctx context.Context, reqID, handlerID string, responseData map[string]interface{}) error {
	req, err := s.dsrStore.GetRequest(ctx, reqID)
	if err != nil {
		return err
	}

	now := time.Now()
	req.Status = "completed"
	req.CompletedDate = &now
	req.HandledBy = handlerID
	req.ResponseData = responseData
	req.UpdatedAt = now

	if err := s.dsrStore.UpdateRequest(ctx, req); err != nil {
		return err
	}

	// Log the processing
	if s.auditLogger != nil {
		s.auditLogger.Log(ctx, &AuditEvent{
			EventType:    "dsr.completed",
			UserID:       req.UserID,
			ResourceType: "data_subject_request",
			ResourceID:   req.ID,
			Details: map[string]interface{}{
				"request_type": req.RequestType,
				"handled_by":   handlerID,
			},
		})
	}

	return nil
}

// GetDataSubjectRequest retrieves a data subject request
func (s *ComplianceService) GetDataSubjectRequest(ctx context.Context, reqID string) (*DataSubjectRequest, error) {
	return s.dsrStore.GetRequest(ctx, reqID)
}

// ListDataSubjectRequests lists data subject requests
func (s *ComplianceService) ListDataSubjectRequests(ctx context.Context, status string, limit, offset int) ([]*DataSubjectRequest, error) {
	return s.dsrStore.ListRequests(ctx, status, limit, offset)
}

// RegisterComplianceRule registers a compliance rule
func (s *ComplianceService) RegisterComplianceRule(rule *ComplianceRule) error {
	if rule.ID == "" || rule.Framework == "" || rule.Code == "" {
		return ErrInvalidComplianceRule
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	rule.CreatedAt = time.Now()
	s.rules[rule.ID] = rule

	return nil
}

// GetComplianceRule retrieves a compliance rule
func (s *ComplianceService) GetComplianceRule(id string) (*ComplianceRule, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rule, exists := s.rules[id]
	if !exists {
		return nil, ErrComplianceReportNotFound
	}
	return rule, nil
}

// ListComplianceRules lists all compliance rules for a framework
func (s *ComplianceService) ListComplianceRules(framework ComplianceFramework) []*ComplianceRule {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var rules []*ComplianceRule
	for _, rule := range s.rules {
		if framework == "" || rule.Framework == framework {
			rules = append(rules, rule)
		}
	}
	return rules
}

// RunComplianceCheck runs a compliance check for a rule
func (s *ComplianceService) RunComplianceCheck(ctx context.Context, ruleID string) (*ComplianceCheckResult, error) {
	rule, err := s.GetComplianceRule(ruleID)
	if err != nil {
		return nil, err
	}

	start := time.Now()
	result := &ComplianceCheckResult{
		RuleID:    ruleID,
		CheckedAt: start,
		Findings:  []ComplianceFinding{},
	}

	if rule.CheckFunc != nil {
		checkResult, err := rule.CheckFunc(ctx, rule)
		if err != nil {
			result.Status = StatusUnknown
			result.Score = 0
		} else {
			result.Status = checkResult.Status
			result.Score = checkResult.Score
			result.Findings = checkResult.Findings
		}
	} else {
		// Default check - mark as pending
		result.Status = StatusPending
		result.Score = 0
	}

	result.Duration = time.Since(start)
	return result, nil
}

// GenerateComplianceReport generates a compliance report
func (s *ComplianceService) GenerateComplianceReport(ctx context.Context, framework ComplianceFramework, name, description, generatedBy string, periodStart, periodEnd time.Time) (*ComplianceReport, error) {
	rules := s.ListComplianceRules(framework)

	report := &ComplianceReport{
		ID:          generateComplianceID(),
		Name:        name,
		Description: description,
		Framework:   framework,
		Status:      StatusPending,
		Results:     make([]*ComplianceCheckResult, 0),
		Summary: &ComplianceSummary{
			TotalRules: len(rules),
		},
		GeneratedAt: time.Now(),
		GeneratedBy: generatedBy,
		PeriodStart: periodStart,
		PeriodEnd:   periodEnd,
	}

	// Run checks for all rules
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}

		result, err := s.RunComplianceCheck(ctx, rule.ID)
		if err != nil {
			continue
		}

		report.Results = append(report.Results, result)

		// Update summary
		switch result.Status {
		case StatusCompliant:
			report.Summary.Compliant++
		case StatusNonCompliant:
			report.Summary.NonCompliant++
		case StatusPending:
			report.Summary.Pending++
		}

		// Count issues by severity
		for _, finding := range result.Findings {
			switch finding.Severity {
			case "critical":
				report.Summary.CriticalIssues++
			case "high":
				report.Summary.HighIssues++
			case "medium":
				report.Summary.MediumIssues++
			case "low":
				report.Summary.LowIssues++
			}
		}
	}

	// Calculate overall score
	if report.Summary.TotalRules > 0 {
		report.Score = (report.Summary.Compliant * 100) / report.Summary.TotalRules
	}

	// Determine overall status
	if report.Summary.NonCompliant > 0 {
		report.Status = StatusNonCompliant
	} else if report.Summary.Pending > 0 {
		report.Status = StatusPending
	} else {
		report.Status = StatusCompliant
	}

	// Set next review date (quarterly)
	report.NextReviewAt = time.Now().AddDate(0, 3, 0)

	// Store the report
	if s.reportStore != nil {
		if err := s.reportStore.CreateReport(ctx, report); err != nil {
			return nil, err
		}
	}

	// Log report generation
	if s.auditLogger != nil {
		s.auditLogger.Log(ctx, &AuditEvent{
			EventType:    "compliance.report.generated",
			UserID:       generatedBy,
			ResourceType: "compliance_report",
			ResourceID:   report.ID,
			Details: map[string]interface{}{
				"framework": framework,
				"score":     report.Score,
				"status":    report.Status,
			},
		})
	}

	return report, nil
}

// GetComplianceReport retrieves a compliance report
func (s *ComplianceService) GetComplianceReport(ctx context.Context, id string) (*ComplianceReport, error) {
	return s.reportStore.GetReport(ctx, id)
}

// ListComplianceReports lists compliance reports
func (s *ComplianceService) ListComplianceReports(ctx context.Context, framework ComplianceFramework, limit, offset int) ([]*ComplianceReport, error) {
	return s.reportStore.ListReports(ctx, framework, limit, offset)
}

// CreateRetentionPolicy creates a data retention policy
func (s *ComplianceService) CreateRetentionPolicy(ctx context.Context, policy *DataRetentionPolicy) error {
	now := time.Now()
	policy.CreatedAt = now
	policy.UpdatedAt = now
	return s.retentionStore.CreatePolicy(ctx, policy)
}

// GetRetentionPolicy retrieves a retention policy
func (s *ComplianceService) GetRetentionPolicy(ctx context.Context, id string) (*DataRetentionPolicy, error) {
	return s.retentionStore.GetPolicy(ctx, id)
}

// GetRetentionPolicyByResourceType retrieves a retention policy by resource type
func (s *ComplianceService) GetRetentionPolicyByResourceType(ctx context.Context, resourceType string) (*DataRetentionPolicy, error) {
	return s.retentionStore.GetPolicyByResourceType(ctx, resourceType)
}

// ListRetentionPolicies lists all retention policies
func (s *ComplianceService) ListRetentionPolicies(ctx context.Context) ([]*DataRetentionPolicy, error) {
	return s.retentionStore.ListPolicies(ctx)
}

// UpdateRetentionPolicy updates a retention policy
func (s *ComplianceService) UpdateRetentionPolicy(ctx context.Context, policy *DataRetentionPolicy) error {
	policy.UpdatedAt = time.Now()
	return s.retentionStore.UpdatePolicy(ctx, policy)
}

// DeleteRetentionPolicy deletes a retention policy
func (s *ComplianceService) DeleteRetentionPolicy(ctx context.Context, id string) error {
	return s.retentionStore.DeletePolicy(ctx, id)
}

// EnforceRetentionPolicies enforces data retention policies
func (s *ComplianceService) EnforceRetentionPolicies(ctx context.Context) (map[string]int, error) {
	policies, err := s.retentionStore.ListPolicies(ctx)
	if err != nil {
		return nil, err
	}

	results := make(map[string]int)

	for _, policy := range policies {
		if !policy.Enabled {
			continue
		}

		// Log retention enforcement
		if s.auditLogger != nil {
			s.auditLogger.Log(ctx, &AuditEvent{
				EventType:    "retention.enforced",
				ResourceType: policy.ResourceType,
				Details: map[string]interface{}{
					"policy_id":      policy.ID,
					"retention_days": policy.RetentionDays,
					"action":         policy.Action,
				},
			})
		}

		results[policy.ResourceType] = 0
	}

	return results, nil
}

// GenerateGDPRReport generates a GDPR compliance report
func (s *ComplianceService) GenerateGDPRReport(ctx context.Context, generatedBy string) (*ComplianceReport, error) {
	return s.GenerateComplianceReport(ctx, FrameworkGDPR, "GDPR Compliance Report", "General Data Protection Regulation compliance assessment", generatedBy, time.Now().AddDate(0, -1, 0), time.Now())
}

// GenerateSOC2Report generates a SOC 2 compliance report
func (s *ComplianceService) GenerateSOC2Report(ctx context.Context, generatedBy string) (*ComplianceReport, error) {
	return s.GenerateComplianceReport(ctx, FrameworkSOC2, "SOC 2 Compliance Report", "SOC 2 Type II compliance assessment", generatedBy, time.Now().AddDate(0, -1, 0), time.Now())
}

// GetComplianceDashboard returns compliance dashboard data
func (s *ComplianceService) GetComplianceDashboard(ctx context.Context) (*ComplianceDashboard, error) {
	dashboard := &ComplianceDashboard{
		Frameworks: make(map[ComplianceFramework]*FrameworkStatus),
		GeneratedAt: time.Now(),
	}

	// Get status for each framework
	frameworks := []ComplianceFramework{FrameworkGDPR, FrameworkCCPA, FrameworkSOC2, FrameworkISO27001}

	for _, framework := range frameworks {
		rules := s.ListComplianceRules(framework)
		status := &FrameworkStatus{
			TotalRules: len(rules),
			Compliant:  0,
			LastCheck:  time.Now(),
		}

		for _, rule := range rules {
			if rule.Enabled {
				status.EnabledRules++
			}
		}

		dashboard.Frameworks[framework] = status
	}

	return dashboard, nil
}

// ComplianceDashboard represents compliance dashboard data
type ComplianceDashboard struct {
	Frameworks  map[ComplianceFramework]*FrameworkStatus `json:"frameworks"`
	GeneratedAt time.Time                                `json:"generated_at"`
	Alerts      []ComplianceAlert                        `json:"alerts,omitempty"`
}

// FrameworkStatus represents the status of a compliance framework
type FrameworkStatus struct {
	TotalRules   int           `json:"total_rules"`
	EnabledRules int           `json:"enabled_rules"`
	Compliant    int           `json:"compliant"`
	NonCompliant int           `json:"non_compliant"`
	LastCheck    time.Time     `json:"last_check"`
	NextCheck    time.Time     `json:"next_check"`
}

// ComplianceAlert represents a compliance alert
type ComplianceAlert struct {
	ID          string    `json:"id"`
	Severity    string    `json:"severity"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Framework   ComplianceFramework `json:"framework"`
	CreatedAt   time.Time `json:"created_at"`
	Acknowledged bool     `json:"acknowledged"`
}

// generateComplianceID generates a unique compliance ID
func generateComplianceID() string {
	return fmt.Sprintf("comp_%d", time.Now().UnixNano())
}

// DefaultGDPRRules returns default GDPR compliance rules
func DefaultGDPRRules() []*ComplianceRule {
	now := time.Now()
	return []*ComplianceRule{
		{
			ID:          "gdpr-consent",
			Framework:   FrameworkGDPR,
			Code:        "GDPR-7.1",
			Name:        "Consent Management",
			Description: "Ensure valid consent is obtained for data processing",
			Category:    "consent",
			Severity:    "high",
			Enabled:     true,
			CreatedAt:   now,
		},
		{
			ID:          "gdpr-erasure",
			Framework:   FrameworkGDPR,
			Code:        "GDPR-17",
			Name:        "Right to Erasure",
			Description: "Ensure data subjects can request deletion of their data",
			Category:    "data_rights",
			Severity:    "high",
			Enabled:     true,
			CreatedAt:   now,
		},
		{
			ID:          "gdpr-access",
			Framework:   FrameworkGDPR,
			Code:        "GDPR-15",
			Name:        "Right to Access",
			Description: "Ensure data subjects can access their personal data",
			Category:    "data_rights",
			Severity:    "high",
			Enabled:     true,
			CreatedAt:   now,
		},
		{
			ID:          "gdpr-portability",
			Framework:   FrameworkGDPR,
			Code:        "GDPR-20",
			Name:        "Data Portability",
			Description: "Ensure data subjects can receive their data in a portable format",
			Category:    "data_rights",
			Severity:    "medium",
			Enabled:     true,
			CreatedAt:   now,
		},
		{
			ID:          "gdpr-retention",
			Framework:   FrameworkGDPR,
			Code:        "GDPR-5.1",
			Name:        "Data Retention",
			Description: "Ensure data is not kept longer than necessary",
			Category:    "data_management",
			Severity:    "medium",
			Enabled:     true,
			CreatedAt:   now,
		},
		{
			ID:          "gdpr-audit",
			Framework:   FrameworkGDPR,
			Code:        "GDPR-30",
			Name:        "Records of Processing",
			Description: "Maintain records of processing activities",
			Category:    "accountability",
			Severity:    "medium",
			Enabled:     true,
			CreatedAt:   now,
		},
	}
}

// InitializeDefaultRules initializes default compliance rules
func (s *ComplianceService) InitializeDefaultRules() error {
	for _, rule := range DefaultGDPRRules() {
		if err := s.RegisterComplianceRule(rule); err != nil {
			return err
		}
	}
	return nil
}
