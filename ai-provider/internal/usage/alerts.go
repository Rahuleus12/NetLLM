// internal/usage/alerts.go
// Usage alerts and notifications
// Handles alert thresholds, triggers, and delivery

package usage

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
	ErrAlertNotFound       = errors.New("alert not found")
	ErrAlertAlreadyExists  = errors.New("alert already exists")
	ErrInvalidAlert       = errors.New("invalid alert data")
	ErrAlertDeliveryFailed = errors.New("alert delivery failed")
)

// AlertType represents type of usage alert
type AlertType string

const (
	AlertTypeQuotaExceeded  AlertType = "quota_exceeded"
	AlertTypeQuotaWarning   AlertType = "quota_warning"
	AlertTypeUsageAnomaly   AlertType = "usage_anomaly"
	AlertTypeCostThreshold  AlertType = "cost_threshold"
	AlertTypePatternChange  AlertType = "pattern_change"
)

// AlertStatus represents status of an alert
type AlertStatus string

const (
	AlertStatusPending   AlertStatus = "pending"
	AlertStatusTriggered AlertStatus = "triggered"
	AlertStatusResolved  AlertStatus = "resolved"
	AlertStatusIgnored   AlertStatus = "ignored"
)

// AlertSeverity represents severity level of an alert
type AlertSeverity string

const (
	AlertSeverityInfo     AlertSeverity = "info"
	AlertSeverityWarning  AlertSeverity = "warning"
	AlertSeverityCritical AlertSeverity = "critical"
)

// DeliveryChannel represents how alerts are delivered
type DeliveryChannel string

const (
	DeliveryChannelEmail   DeliveryChannel = "email"
	DeliveryChannelWebhook DeliveryChannel = "webhook"
	DeliveryChannelSlack   DeliveryChannel = "slack"
	DeliveryChannelSMS     DeliveryChannel = "sms"
)

// UsageAlert represents a configured usage alert
type UsageAlert struct {
	ID            uuid.UUID       `json:"id" db:"id"`
	TenantID      string          `json:"tenant_id" db:"tenant_id"`
	Type          AlertType       `json:"type" db:"type"`
	Name          string          `json:"name" db:"name"`
	Description   string          `json:"description" db:"description"`
	Status        AlertStatus     `json:"status" db:"status"`
	Severity      AlertSeverity   `json:"severity" db:"severity"`

	// Alert conditions
	ResourceType  string          `json:"resource_type" db:"resource_type"`
	Threshold     float64         `json:"threshold" db:"threshold"`
	ThresholdUnit string          `json:"threshold_unit" db:"threshold_unit"`
	TimeWindow    time.Duration   `json:"time_window" db:"time_window"`

	// Delivery configuration
	Enabled       bool            `json:"enabled" db:"enabled"`
	DeliveryChannels []DeliveryChannel `json:"delivery_channels" db:"delivery_channels"`
	Recipients    []string        `json:"recipients" db:"recipients"`
	WebhookURL    *string         `json:"webhook_url,omitempty" db:"webhook_url"`

	// Alert metadata
	LastTriggered *time.Time      `json:"last_triggered_at,omitempty" db:"last_triggered_at"`
	TriggerCount  int             `json:"trigger_count" db:"trigger_count"`
	ResolvedAt    *time.Time      `json:"resolved_at,omitempty" db:"resolved_at"`
	Metadata      map[string]interface{} `json:"metadata,omitempty" db:"metadata"`

	// Timestamps
	CreatedBy     uuid.UUID       `json:"created_by" db:"created_by"`
	CreatedAt     time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at" db:"updated_at"`
}

// AlertTrigger represents a triggered alert instance
type AlertTrigger struct {
	ID           uuid.UUID      `json:"id" db:"id"`
	AlertID      uuid.UUID      `json:"alert_id" db:"alert_id"`
	TenantID     string         `json:"tenant_id" db:"tenant_id"`
	Status       AlertStatus   `json:"status" db:"status"`

	// Trigger details
	ResourceType string         `json:"resource_type" db:"resource_type"`
	CurrentValue float64        `json:"current_value" db:"current_value"`
	Threshold    float64        `json:"threshold" db:"threshold"`
	ExceededBy  float64        `json:"exceeded_by"`
	Context     json.RawMessage `json:"context" db:"context"`

	// Delivery tracking
	DeliveryStatus   string                 `json:"delivery_status" db:"delivery_status"`
	DeliveryAttempts int                    `json:"delivery_attempts" db:"delivery_attempts"`
	DeliveryResults  map[string]interface{} `json:"delivery_results,omitempty" db:"delivery_results"`

	// Resolution
	ResolvedAt   *time.Time `json:"resolved_at,omitempty" db:"resolved_at"`
	ResolvedBy   *string    `json:"resolved_by,omitempty" db:"resolved_by"`
	ResolutionNote string     `json:"resolution_note,omitempty" db:"resolution_note"`

	// Timestamps
	TriggeredAt  time.Time `json:"triggered_at" db:"triggered_at"`
	AcknowledgedAt *time.Time `json:"acknowledged_at,omitempty" db:"acknowledged_at"`
}

// CreateAlertRequest represents a request to create a new alert
type CreateAlertRequest struct {
	TenantID        string                `json:"tenant_id"`
	Type            AlertType             `json:"type"`
	Name            string                `json:"name"`
	Description     string                `json:"description"`
	Severity        AlertSeverity         `json:"severity"`
	ResourceType    string                `json:"resource_type"`
	Threshold       float64               `json:"threshold"`
	ThresholdUnit   string                `json:"threshold_unit"`
	TimeWindow      time.Duration          `json:"time_window"`
	DeliveryChannels []DeliveryChannel     `json:"delivery_channels"`
	Recipients      []string              `json:"recipients"`
	WebhookURL      *string               `json:"webhook_url,omitempty"`
}

// UpdateAlertRequest represents a request to update an alert
type UpdateAlertRequest struct {
	Name            *string              `json:"name,omitempty"`
	Description     *string              `json:"description,omitempty"`
	Status          *AlertStatus         `json:"status,omitempty"`
	Severity        *AlertSeverity       `json:"severity,omitempty"`
	Threshold       *float64             `json:"threshold,omitempty"`
	ThresholdUnit   *string              `json:"threshold_unit,omitempty"`
	TimeWindow      *time.Duration       `json:"time_window,omitempty"`
	Enabled         *bool                `json:"enabled,omitempty"`
	DeliveryChannels []DeliveryChannel    `json:"delivery_channels,omitempty"`
	Recipients      *[]string            `json:"recipients,omitempty"`
	WebhookURL      *string              `json:"webhook_url,omitempty"`
	Metadata        *map[string]interface{} `json:"metadata,omitempty"`
}

// AlertManager manages usage alerts
type AlertManager struct {
	db *sql.DB
}

// NewAlertManager creates a new alert manager
func NewAlertManager(db *sql.DB) *AlertManager {
	return &AlertManager{
		db: db,
	}
}

// CreateAlert creates a new usage alert
func (am *AlertManager) CreateAlert(ctx context.Context, req CreateAlertRequest, createdBy uuid.UUID) (*UsageAlert, error) {
	if req.TenantID == "" {
		return nil, ErrInvalidAlert
	}
	if !isValidAlertType(req.Type) {
		return nil, ErrInvalidAlert
	}
	if req.Name == "" {
		return nil, ErrInvalidAlert
	}
	if req.Threshold <= 0 {
		return nil, errors.New("threshold must be positive")
	}

	alert := &UsageAlert{
		ID:               uuid.New(),
		TenantID:          req.TenantID,
		Type:              req.Type,
		Name:              req.Name,
		Description:       req.Description,
		Status:            AlertStatusPending,
		Severity:          req.Severity,
		ResourceType:      req.ResourceType,
		Threshold:         req.Threshold,
		ThresholdUnit:     req.ThresholdUnit,
		TimeWindow:        req.TimeWindow,
		Enabled:           true,
		DeliveryChannels:   req.DeliveryChannels,
		Recipients:        req.Recipients,
		WebhookURL:        req.WebhookURL,
		TriggerCount:      0,
		CreatedBy:         createdBy,
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}

	channelsJSON, err := json.Marshal(alert.DeliveryChannels)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal delivery channels: %w", err)
	}

	recipientsJSON, err := json.Marshal(alert.Recipients)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal recipients: %w", err)
	}

	metadataJSON, err := json.Marshal(alert.Metadata)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal metadata: %w", err)
	}

	query := `
		INSERT INTO usage_alerts (id, tenant_id, type, name, description, status, severity,
			resource_type, threshold, threshold_unit, time_window, enabled,
			delivery_channels, recipients, webhook_url, trigger_count,
			created_by, created_at, updated_at, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)
		RETURNING id, tenant_id, type, name, description, status, severity,
			resource_type, threshold, threshold_unit, time_window, enabled,
			delivery_channels, recipients, webhook_url, last_triggered_at,
			trigger_count, resolved_at, created_by, created_at, updated_at, metadata
	`

	err = am.db.QueryRowContext(ctx, query,
		alert.ID,
		alert.TenantID,
		alert.Type,
		alert.Name,
		alert.Description,
		alert.Status,
		alert.Severity,
		alert.ResourceType,
		alert.Threshold,
		alert.ThresholdUnit,
		alert.TimeWindow,
		alert.Enabled,
		channelsJSON,
		recipientsJSON,
		alert.WebhookURL,
		alert.TriggerCount,
		alert.CreatedBy,
		alert.CreatedAt,
		alert.UpdatedAt,
		metadataJSON,
	).Scan(
		&alert.ID,
		&alert.TenantID,
		&alert.Type,
		&alert.Name,
		&alert.Description,
		&alert.Status,
		&alert.Severity,
		&alert.ResourceType,
		&alert.Threshold,
		&alert.ThresholdUnit,
		&alert.TimeWindow,
		&alert.Enabled,
		&channelsJSON,
		&recipientsJSON,
		&alert.WebhookURL,
		&alert.LastTriggered,
		&alert.TriggerCount,
		&alert.ResolvedAt,
		&alert.CreatedBy,
		&alert.CreatedAt,
		&alert.UpdatedAt,
		&metadataJSON,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create alert: %w", err)
	}

	// Unmarshal JSON fields
	json.Unmarshal(channelsJSON, &alert.DeliveryChannels)
	json.Unmarshal(recipientsJSON, &alert.Recipients)
	json.Unmarshal(metadataJSON, &alert.Metadata)

	return alert, nil
}

// GetAlert retrieves an alert by ID
func (am *AlertManager) GetAlert(ctx context.Context, id uuid.UUID) (*UsageAlert, error) {
	if id == uuid.Nil {
		return nil, ErrInvalidAlert
	}

	var alert UsageAlert
	var channelsJSON, recipientsJSON, metadataJSON []byte

	query := `
		SELECT id, tenant_id, type, name, description, status, severity,
			resource_type, threshold, threshold_unit, time_window, enabled,
			delivery_channels, recipients, webhook_url, last_triggered_at,
			trigger_count, resolved_at, created_by, created_at, updated_at, metadata
		FROM usage_alerts
		WHERE id = $1
	`

	err := am.db.QueryRowContext(ctx, query, id).Scan(
		&alert.ID,
		&alert.TenantID,
		&alert.Type,
		&alert.Name,
		&alert.Description,
		&alert.Status,
		&alert.Severity,
		&alert.ResourceType,
		&alert.Threshold,
		&alert.ThresholdUnit,
		&alert.TimeWindow,
		&alert.Enabled,
		&channelsJSON,
		&recipientsJSON,
		&alert.WebhookURL,
		&alert.LastTriggered,
		&alert.TriggerCount,
		&alert.ResolvedAt,
		&alert.CreatedBy,
		&alert.CreatedAt,
		&alert.UpdatedAt,
		&metadataJSON,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrAlertNotFound
		}
		return nil, fmt.Errorf("failed to get alert: %w", err)
	}

	// Unmarshal JSON fields
	if channelsJSON != nil {
		json.Unmarshal(channelsJSON, &alert.DeliveryChannels)
	}
	if recipientsJSON != nil {
		json.Unmarshal(recipientsJSON, &alert.Recipients)
	}
	if metadataJSON != nil {
		json.Unmarshal(metadataJSON, &alert.Metadata)
	}

	return &alert, nil
}

// UpdateAlert updates an alert
func (am *AlertManager) UpdateAlert(ctx context.Context, id uuid.UUID, req UpdateAlertRequest) (*UsageAlert, error) {
	if id == uuid.Nil {
		return nil, ErrInvalidAlert
	}

	alert, err := am.GetAlert(ctx, id)
	if err != nil {
		return nil, err
	}

	// Apply updates
	if req.Name != nil {
		alert.Name = *req.Name
	}
	if req.Description != nil {
		alert.Description = *req.Description
	}
	if req.Status != nil {
		if !isValidAlertStatus(*req.Status) {
			return nil, errors.New("invalid alert status")
		}
		alert.Status = *req.Status
	}
	if req.Severity != nil {
		alert.Severity = *req.Severity
	}
	if req.Threshold != nil {
		if *req.Threshold <= 0 {
			return nil, errors.New("threshold must be positive")
		}
		alert.Threshold = *req.Threshold
	}
	if req.ThresholdUnit != nil {
		alert.ThresholdUnit = *req.ThresholdUnit
	}
	if req.TimeWindow != nil {
		alert.TimeWindow = *req.TimeWindow
	}
	if req.Enabled != nil {
		alert.Enabled = *req.Enabled
	}
	if req.DeliveryChannels != nil {
		alert.DeliveryChannels = *req.DeliveryChannels
	}
	if req.Recipients != nil {
		alert.Recipients = *req.Recipients
	}
	if req.WebhookURL != nil {
		alert.WebhookURL = req.WebhookURL
	}
	if req.Metadata != nil {
		alert.Metadata = *req.Metadata
	}
	alert.UpdatedAt = time.Now()

	// Marshal JSON fields
	channelsJSON, err := json.Marshal(alert.DeliveryChannels)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal delivery channels: %w", err)
	}

	recipientsJSON, err := json.Marshal(alert.Recipients)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal recipients: %w", err)
	}

	metadataJSON, err := json.Marshal(alert.Metadata)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal metadata: %w", err)
	}

	query := `
		UPDATE usage_alerts
		SET name = $1, description = $2, status = $3, severity = $4,
			threshold = $5, threshold_unit = $6, time_window = $7, enabled = $8,
			delivery_channels = $9, recipients = $10, webhook_url = $11,
			metadata = $12, updated_at = $13
		WHERE id = $14
		RETURNING id, tenant_id, type, name, description, status, severity,
			resource_type, threshold, threshold_unit, time_window, enabled,
			delivery_channels, recipients, webhook_url, last_triggered_at,
			trigger_count, resolved_at, created_by, created_at, updated_at, metadata
	`

	err = am.db.QueryRowContext(ctx, query,
		alert.Name,
		alert.Description,
		alert.Status,
		alert.Severity,
		alert.Threshold,
		alert.ThresholdUnit,
		alert.TimeWindow,
		alert.Enabled,
		channelsJSON,
		recipientsJSON,
		alert.WebhookURL,
		metadataJSON,
		alert.UpdatedAt,
		alert.ID,
	).Scan(
		&alert.ID,
		&alert.TenantID,
		&alert.Type,
		&alert.Name,
		&alert.Description,
		&alert.Status,
		&alert.Severity,
		&alert.ResourceType,
		&alert.Threshold,
		&alert.ThresholdUnit,
		&alert.TimeWindow,
		&alert.Enabled,
		&channelsJSON,
		&recipientsJSON,
		&alert.WebhookURL,
		&alert.LastTriggered,
		&alert.TriggerCount,
		&alert.ResolvedAt,
		&alert.CreatedBy,
		&alert.CreatedAt,
		&alert.UpdatedAt,
		&metadataJSON,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to update alert: %w", err)
	}

	json.Unmarshal(channelsJSON, &alert.DeliveryChannels)
	json.Unmarshal(recipientsJSON, &alert.Recipients)
	json.Unmarshal(metadataJSON, &alert.Metadata)

	return alert, nil
}

// DeleteAlert deletes an alert
func (am *AlertManager) DeleteAlert(ctx context.Context, id uuid.UUID) error {
	if id == uuid.Nil {
		return ErrInvalidAlert
	}

	query := `DELETE FROM usage_alerts WHERE id = $1`

	result, err := am.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete alert: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return ErrAlertNotFound
	}

	return nil
}

// ListAlerts lists alerts for a tenant
func (am *AlertManager) ListAlerts(ctx context.Context, tenantID string, status *AlertStatus) ([]*UsageAlert, error) {
	if tenantID == "" {
		return nil, errors.New("tenant_id is required")
	}

	var alerts []*UsageAlert

	query := `
		SELECT id, tenant_id, type, name, description, status, severity,
			resource_type, threshold, threshold_unit, time_window, enabled,
			delivery_channels, recipients, webhook_url, last_triggered_at,
			trigger_count, resolved_at, created_by, created_at, updated_at, metadata
		FROM usage_alerts
		WHERE tenant_id = $1
	`

	args := []interface{}{tenantID}
	argPos := 2

	if status != nil {
		query += fmt.Sprintf(" AND status = $%d", argPos)
		args = append(args, *status)
		argPos++
	}

	query += " ORDER BY created_at DESC"

	rows, err := am.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list alerts: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var alert UsageAlert
		var channelsJSON, recipientsJSON, metadataJSON []byte

		err := rows.Scan(
			&alert.ID,
			&alert.TenantID,
			&alert.Type,
			&alert.Name,
			&alert.Description,
			&alert.Status,
			&alert.Severity,
			&alert.ResourceType,
			&alert.Threshold,
			&alert.ThresholdUnit,
			&alert.TimeWindow,
			&alert.Enabled,
			&channelsJSON,
			&recipientsJSON,
			&alert.WebhookURL,
			&alert.LastTriggered,
			&alert.TriggerCount,
			&alert.ResolvedAt,
			&alert.CreatedBy,
			&alert.CreatedAt,
			&alert.UpdatedAt,
			&metadataJSON,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan alert: %w", err)
		}

		if channelsJSON != nil {
			json.Unmarshal(channelsJSON, &alert.DeliveryChannels)
		}
		if recipientsJSON != nil {
			json.Unmarshal(recipientsJSON, &alert.Recipients)
		}
		if metadataJSON != nil {
			json.Unmarshal(metadataJSON, &alert.Metadata)
		}

		alerts = append(alerts, &alert)
	}

	return alerts, nil
}

// TriggerAlert triggers an alert
func (am *AlertManager) TriggerAlert(ctx context.Context, alertID uuid.UUID, currentValue float64, context map[string]interface{}) (*AlertTrigger, error) {
	if alertID == uuid.Nil {
		return nil, ErrInvalidAlert
	}

	alert, err := am.GetAlert(ctx, alertID)
	if err != nil {
		return nil, err
	}

	if !alert.Enabled {
		return nil, errors.New("alert is disabled")
	}

	trigger := &AlertTrigger{
		ID:            uuid.New(),
		AlertID:       alertID,
		TenantID:      alert.TenantID,
		Status:        AlertStatusTriggered,
		ResourceType:  alert.ResourceType,
		CurrentValue:  currentValue,
		Threshold:     alert.Threshold,
		ExceededBy:    currentValue - alert.Threshold,
		TriggeredAt:   time.Now(),
	}

	// Serialize context
	contextJSON, err := json.Marshal(context)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal context: %w", err)
	}
	trigger.Context = contextJSON

	// Insert trigger
	resultsJSON, _ := json.Marshal(map[string]interface{}{})
	query := `
		INSERT INTO alert_triggers (id, alert_id, tenant_id, status, resource_type,
			current_value, threshold, exceeded_by, context, delivery_status,
			delivery_attempts, delivery_results, triggered_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		RETURNING id, alert_id, tenant_id, status, resource_type, current_value,
			threshold, exceeded_by, context, delivery_status, delivery_attempts,
			delivery_results, resolved_at, resolved_by, resolution_note,
			triggered_at, acknowledged_at
	`

	err = am.db.QueryRowContext(ctx, query,
		trigger.ID,
		trigger.AlertID,
		trigger.TenantID,
		trigger.Status,
		trigger.ResourceType,
		trigger.CurrentValue,
		trigger.Threshold,
		trigger.ExceededBy,
		trigger.Context,
		trigger.DeliveryStatus,
		trigger.DeliveryAttempts,
		resultsJSON,
		trigger.TriggeredAt,
	).Scan(
		&trigger.ID,
		&trigger.AlertID,
		&trigger.TenantID,
		&trigger.Status,
		&trigger.ResourceType,
		&trigger.CurrentValue,
		&trigger.Threshold,
		&trigger.ExceededBy,
		&trigger.Context,
		&trigger.DeliveryStatus,
		&trigger.DeliveryAttempts,
		&trigger.DeliveryResults,
		&trigger.ResolvedAt,
		&trigger.ResolvedBy,
		&trigger.ResolutionNote,
		&trigger.TriggeredAt,
		&trigger.AcknowledgedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create trigger: %w", err)
	}

	// Update alert
	now := time.Now()
	alert.LastTriggered = &now
	alert.TriggerCount++
	alert.Status = AlertStatusTriggered

	_, err = am.UpdateAlert(ctx, alert.ID, UpdateAlertRequest{
		Status: func() *AlertStatus { s := AlertStatusTriggered; return &s }(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to update alert status: %w", err)
	}

	// Deliver alert
	err = am.deliverAlert(ctx, alert, trigger)
	if err != nil {
		trigger.DeliveryStatus = "failed"
		trigger.DeliveryResults = map[string]interface{}{"error": err.Error()}
		am.updateTriggerDeliveryStatus(ctx, trigger.ID, trigger)
		return nil, ErrAlertDeliveryFailed
	}

	return trigger, nil
}

// ResolveTrigger resolves a triggered alert
func (am *AlertManager) ResolveTrigger(ctx context.Context, triggerID uuid.UUID, resolvedBy string, note string) error {
	if triggerID == uuid.Nil {
		return ErrInvalidAlert
	}

	now := time.Now()

	query := `
		UPDATE alert_triggers
		SET status = 'resolved', resolved_at = $1, resolved_by = $2, resolution_note = $3
		WHERE id = $4
	`

	result, err := am.db.ExecContext(ctx, query, now, resolvedBy, note, triggerID)
	if err != nil {
		return fmt.Errorf("failed to resolve trigger: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return ErrAlertNotFound
	}

	return nil
}

// GetActiveAlertTriggers retrieves active triggers for a tenant
func (am *AlertManager) GetActiveAlertTriggers(ctx context.Context, tenantID string) ([]*AlertTrigger, error) {
	if tenantID == "" {
		return nil, errors.New("tenant_id is required")
	}

	var triggers []*AlertTrigger

	query := `
		SELECT id, alert_id, tenant_id, status, resource_type, current_value,
			threshold, exceeded_by, context, delivery_status, delivery_attempts,
			delivery_results, resolved_at, resolved_by, resolution_note,
			triggered_at, acknowledged_at
		FROM alert_triggers
		WHERE tenant_id = $1 AND status = 'triggered'
		ORDER BY triggered_at DESC
	`

	rows, err := am.db.QueryContext(ctx, query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get active triggers: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var trigger AlertTrigger

		err := rows.Scan(
			&trigger.ID,
			&trigger.AlertID,
			&trigger.TenantID,
			&trigger.Status,
			&trigger.ResourceType,
			&trigger.CurrentValue,
			&trigger.Threshold,
			&trigger.ExceededBy,
			&trigger.Context,
			&trigger.DeliveryStatus,
			&trigger.DeliveryAttempts,
			&trigger.DeliveryResults,
			&trigger.ResolvedAt,
			&trigger.ResolvedBy,
			&trigger.ResolutionNote,
			&trigger.TriggeredAt,
			&trigger.AcknowledgedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan trigger: %w", err)
		}

		triggers = append(triggers, &trigger)
	}

	return triggers, nil
}

// deliverAlert delivers an alert through configured channels
func (am *AlertManager) deliverAlert(ctx context.Context, alert *UsageAlert, trigger *AlertTrigger) error {
	// In a real implementation, this would deliver through actual channels
	// For now, we'll just mark as delivered

	for _, channel := range alert.DeliveryChannels {
		switch channel {
		case DeliveryChannelEmail:
			// TODO: Implement email delivery
		case DeliveryChannelWebhook:
			// TODO: Implement webhook delivery
		case DeliveryChannelSlack:
			// TODO: Implement Slack delivery
		case DeliveryChannelSMS:
			// TODO: Implement SMS delivery
		}
	}

	trigger.DeliveryStatus = "delivered"
	trigger.DeliveryAttempts++

	return am.updateTriggerDeliveryStatus(ctx, trigger.ID, trigger)
}

// updateTriggerDeliveryStatus updates the delivery status of a trigger
func (am *AlertManager) updateTriggerDeliveryStatus(ctx context.Context, triggerID uuid.UUID, trigger *AlertTrigger) error {
	resultsJSON, err := json.Marshal(trigger.DeliveryResults)
	if err != nil {
		return fmt.Errorf("failed to marshal delivery results: %w", err)
	}

	query := `
		UPDATE alert_triggers
		SET delivery_status = $1, delivery_attempts = $2, delivery_results = $3
		WHERE id = $4
	`

	_, err = am.db.ExecContext(ctx, query, trigger.DeliveryStatus, trigger.DeliveryAttempts, resultsJSON, triggerID)
	if err != nil {
		return fmt.Errorf("failed to update trigger delivery status: %w", err)
	}

	return nil
}

// CheckThresholds checks all enabled alerts and triggers if thresholds are exceeded
func (am *AlertManager) CheckThresholds(ctx context.Context, tenantID string) error {
	if tenantID == "" {
		return errors.New("tenant_id is required")
	}

	// Get all enabled alerts for tenant
	alerts, err := am.ListAlerts(ctx, tenantID, nil)
	if err != nil {
		return fmt.Errorf("failed to list alerts: %w", err)
	}

	// Check each alert
	for _, alert := range alerts {
		if !alert.Enabled {
			continue
		}

		// Get current usage for resource type
		currentValue, err := am.getCurrentUsage(ctx, tenantID, alert.ResourceType)
		if err != nil {
			continue
		}

		// Check if threshold is exceeded
		if currentValue >= alert.Threshold {
			// Check if already triggered recently
			if alert.LastTriggered != nil && time.Since(*alert.LastTriggered) < alert.TimeWindow {
				continue
			}

			// Trigger alert
			_, err = am.TriggerAlert(ctx, alert.ID, currentValue, map[string]interface{}{
				"alert_name": alert.Name,
				"threshold": alert.Threshold,
				"timestamp": time.Now(),
			})
			if err != nil {
				// Log error but continue with other alerts
				continue
			}
		}
	}

	return nil
}

// getCurrentUsage gets current usage for a resource type
func (am *AlertManager) getCurrentUsage(ctx context.Context, tenantID, resourceType string) (float64, error) {
	var currentValue float64

	query := `
		SELECT SUM(quantity)
		FROM usage_records
		WHERE tenant_id = $1 AND resource_type = $2
			AND recorded_at >= CURRENT_TIMESTAMP - INTERVAL '24 hours'
	`

	err := am.db.QueryRowContext(ctx, query, tenantID, resourceType).Scan(&currentValue)
	if err != nil {
		return 0, err
	}

	return currentValue, nil
}

// isValidAlertType checks if an alert type is valid
func isValidAlertType(alertType AlertType) bool {
	switch alertType {
	case AlertTypeQuotaExceeded, AlertTypeQuotaWarning, AlertTypeUsageAnomaly,
		AlertTypeCostThreshold, AlertTypePatternChange:
		return true
	default:
		return false
	}
}

// isValidAlertStatus checks if an alert status is valid
func isValidAlertStatus(status AlertStatus) bool {
	switch status {
	case AlertStatusPending, AlertStatusTriggered, AlertStatusResolved, AlertStatusIgnored:
		return true
	default:
		return false
	}
}
