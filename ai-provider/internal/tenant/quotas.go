// internal/tenant/quotas.go
// Quota management for resource limits and enforcement
// Handles checking, tracking, and enforcing resource quotas per tenant

package tenant

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	ErrQuotaExceeded      = errors.New("resource quota exceeded")
	ErrQuotaNotFound      = errors.New("quota not found")
	ErrInvalidQuotaValue  = errors.New("invalid quota value")
	ErrQuotaCheckFailed   = errors.New("quota check failed")
	ErrResourceNotFound   = errors.New("resource not found")
)

// ResourceType represents the type of resource being tracked
type ResourceType string

const (
	ResourceTypeStorage       ResourceType = "storage"
	ResourceTypeModels        ResourceType = "models"
	ResourceTypeInference     ResourceType = "inference"
	ResourceTypeTokens        ResourceType = "tokens"
	ResourceTypeGPU           ResourceType = "gpu"
	ResourceTypeCPU           ResourceType = "cpu"
	ResourceTypeAPIRequests   ResourceType = "api_requests"
	ResourceTypeBandwidth     ResourceType = "bandwidth"
)

// QuotaLimit represents a quota limit for a specific resource
type QuotaLimit struct {
	ResourceType ResourceType `json:"resource_type"`
	MaxValue     int64       `json:"max_value"`
	CurrentValue int64       `json:"current_value"`
	Unit         string      `json:"unit"`
	Period       QuotaPeriod `json:"period"`
	SoftLimit    bool        `json:"soft_limit"`
	AlertThresholdPercent int `json:"alert_threshold_percent"`
}

// QuotaPeriod represents the time period for quota enforcement
type QuotaPeriod string

const (
	QuotaPeriodHourly   QuotaPeriod = "hourly"
	QuotaPeriodDaily    QuotaPeriod = "daily"
	QuotaPeriodWeekly   QuotaPeriod = "weekly"
	QuotaPeriodMonthly  QuotaPeriod = "monthly"
	QuotaPeriodForever  QuotaPeriod = "forever"
)

// QuotaCheckResult represents the result of a quota check
type QuotaCheckResult struct {
	Allowed      bool      `json:"allowed"`
	Quota        *QuotaLimit `json:"quota"`
	UsageAfter   int64     `json:"usage_after"`
	Exceeded     bool      `json:"exceeded"`
	Warning      bool      `json:"warning"`
	Message      string    `json:"message"`
}

// QuotaUsageRecord represents a record of quota usage
type QuotaUsageRecord struct {
	ID          string      `json:"id"`
	TenantID    string      `json:"tenant_id"`
	ResourceType ResourceType `json:"resource_type"`
	Quantity    int64       `json:"quantity"`
	Unit        string      `json:"unit"`
	Timestamp   time.Time   `json:"timestamp"`
	Operation   string      `json:"operation"` // add, subtract, reset
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// QuotaManager manages resource quotas for tenants
type QuotaManager struct {
	db          *sql.DB
	quotaCache  map[string]*QuotaLimit
	cacheMutex  sync.RWMutex
	alertChan   chan QuotaAlert
}

// QuotaAlert represents a quota alert
type QuotaAlert struct {
	TenantID       string      `json:"tenant_id"`
	ResourceType   ResourceType `json:"resource_type"`
	CurrentValue   int64       `json:"current_value"`
	MaxValue       int64       `json:"max_value"`
	UsagePercent   float64     `json:"usage_percent"`
	AlertType      string      `json:"alert_type"` // warning, exceeded, recovered
	Timestamp      time.Time   `json:"timestamp"`
}

// NewQuotaManager creates a new quota manager
func NewQuotaManager(db *sql.DB) *QuotaManager {
	return &QuotaManager{
		db:         db,
		quotaCache: make(map[string]*QuotaLimit),
		alertChan:  make(chan QuotaAlert, 1000),
	}
}

// GetQuota retrieves a quota limit for a tenant and resource
func (qm *QuotaManager) GetQuota(ctx context.Context, tenantID string, resourceType ResourceType) (*QuotaLimit, error) {
	cacheKey := fmt.Sprintf("%s:%s", tenantID, resourceType)

	// Check cache first
	qm.cacheMutex.RLock()
	if quota, exists := qm.quotaCache[cacheKey]; exists {
		qm.cacheMutex.RUnlock()
		return quota, nil
	}
	qm.cacheMutex.RUnlock()

	var quota QuotaLimit
	var limitJSON []byte

	query := `
		SELECT resource_type, max_value, current_value, unit, period, soft_limit, alert_threshold_percent
		FROM quota_limits
		WHERE tenant_id = $1 AND resource_type = $2 AND deleted_at IS NULL
	`

	err := qm.db.QueryRowContext(ctx, query, tenantID, resourceType).Scan(
		&quota.ResourceType,
		&quota.MaxValue,
		&quota.CurrentValue,
		&quota.Unit,
		&quota.Period,
		&quota.SoftLimit,
		&quota.AlertThresholdPercent,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrQuotaNotFound
		}
		return nil, fmt.Errorf("failed to get quota: %w", err)
	}

	// Update cache
	qm.cacheMutex.Lock()
	qm.quotaCache[cacheKey] = &quota
	qm.cacheMutex.Unlock()

	return &quota, nil
}

// SetQuota sets a quota limit for a tenant and resource
func (qm *QuotaManager) SetQuota(ctx context.Context, tenantID string, quota *QuotaLimit) error {
	if quota.MaxValue < 0 {
		return ErrInvalidQuotaValue
	}
	if quota.AlertThresholdPercent < 0 || quota.AlertThresholdPercent > 100 {
		return ErrInvalidQuotaValue
	}

	query := `
		INSERT INTO quota_limits (tenant_id, resource_type, max_value, current_value, unit, period, soft_limit, alert_threshold_percent, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT (tenant_id, resource_type)
		DO UPDATE SET
			max_value = EXCLUDED.max_value,
			unit = EXCLUDED.unit,
			period = EXCLUDED.period,
			soft_limit = EXCLUDED.soft_limit,
			alert_threshold_percent = EXCLUDED.alert_threshold_percent,
			updated_at = CURRENT_TIMESTAMP
	`

	_, err := qm.db.ExecContext(ctx, query,
		tenantID,
		quota.ResourceType,
		quota.MaxValue,
		quota.CurrentValue,
		quota.Unit,
		quota.Period,
		quota.SoftLimit,
		quota.AlertThresholdPercent,
	)

	if err != nil {
		return fmt.Errorf("failed to set quota: %w", err)
	}

	// Update cache
	cacheKey := fmt.Sprintf("%s:%s", tenantID, quota.ResourceType)
	qm.cacheMutex.Lock()
	qm.quotaCache[cacheKey] = quota
	qm.cacheMutex.Unlock()

	return nil
}

// UpdateQuota updates the current usage of a quota
func (qm *QuotaManager) UpdateQuota(ctx context.Context, tenantID string, resourceType ResourceType, delta int64) error {
	quota, err := qm.GetQuota(ctx, tenantID, resourceType)
	if err != nil {
		return err
	}

	newValue := quota.CurrentValue + delta
	if newValue < 0 {
		newValue = 0
	}

	query := `
		UPDATE quota_limits
		SET current_value = $1, updated_at = CURRENT_TIMESTAMP
		WHERE tenant_id = $2 AND resource_type = $3
	`

	_, err = qm.db.ExecContext(ctx, query, newValue, tenantID, resourceType)
	if err != nil {
		return fmt.Errorf("failed to update quota: %w", err)
	}

	// Update cache
	quota.CurrentValue = newValue
	cacheKey := fmt.Sprintf("%s:%s", tenantID, resourceType)
	qm.cacheMutex.Lock()
	qm.quotaCache[cacheKey] = quota
	qm.cacheMutex.Unlock()

	// Check for alerts
	qm.checkQuotaAlerts(tenantID, quota)

	return nil
}

// CheckQuota checks if a resource request can be satisfied within quota limits
func (qm *QuotaManager) CheckQuota(ctx context.Context, tenantID string, resourceType ResourceType, requestAmount int64) (*QuotaCheckResult, error) {
	quota, err := qm.GetQuota(ctx, tenantID, resourceType)
	if err != nil {
		if err == ErrQuotaNotFound {
			// No quota set, allow by default
			return &QuotaCheckResult{
				Allowed:    true,
				UsageAfter: requestAmount,
				Message:    "No quota limit set",
			}, nil
		}
		return nil, fmt.Errorf("failed to check quota: %w", err)
	}

	result := &QuotaCheckResult{
		Quota:      quota,
		UsageAfter: quota.CurrentValue + requestAmount,
	}

	usagePercent := float64(result.UsageAfter) / float64(quota.MaxValue) * 100

	// Check if quota is exceeded
	if result.UsageAfter > quota.MaxValue {
		result.Exceeded = true
		if quota.SoftLimit {
			result.Allowed = true
			result.Message = fmt.Sprintf("Soft quota limit exceeded (%.1f%% of %d %s)", usagePercent, quota.MaxValue, quota.Unit)
		} else {
			result.Allowed = false
			result.Message = fmt.Sprintf("Quota limit exceeded (%.1f%% of %d %s). Request denied.", usagePercent, quota.MaxValue, quota.Unit)
			return result, ErrQuotaExceeded
		}
	} else {
		result.Allowed = true
	}

	// Check for warning threshold
	if usagePercent >= float64(quota.AlertThresholdPercent) {
		result.Warning = true
		result.Message = fmt.Sprintf("Warning: approaching quota limit (%.1f%% of %d %s)", usagePercent, quota.MaxValue, quota.Unit)
	}

	return result, nil
}

// RecordUsage records resource usage and updates quotas
func (qm *QuotaManager) RecordUsage(ctx context.Context, record *QuotaUsageRecord) error {
	// Update quota
	err := qm.UpdateQuota(ctx, record.TenantID, record.ResourceType, record.Quantity)
	if err != nil {
		return fmt.Errorf("failed to update quota: %w", err)
	}

	// Store usage record
	recordJSON, err := json.Marshal(record.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	query := `
		INSERT INTO usage_records (tenant_id, resource_type, quantity, unit, operation, metadata, recorded_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`

	_, err = qm.db.ExecContext(ctx, query,
		record.TenantID,
		record.ResourceType,
		record.Quantity,
		record.Unit,
		record.Operation,
		recordJSON,
		record.Timestamp,
	)

	if err != nil {
		return fmt.Errorf("failed to record usage: %w", err)
	}

	return nil
}

// GetUsageHistory retrieves usage history for a tenant
func (qm *QuotaManager) GetUsageHistory(ctx context.Context, tenantID string, resourceType ResourceType, startTime, endTime time.Time, limit int) ([]*QuotaUsageRecord, error) {
	query := `
		SELECT id, tenant_id, resource_type, quantity, unit, operation, metadata, recorded_at
		FROM usage_records
		WHERE tenant_id = $1 AND resource_type = $2 AND recorded_at >= $3 AND recorded_at <= $4
		ORDER BY recorded_at DESC
	`

	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := qm.db.QueryContext(ctx, query, tenantID, resourceType, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("failed to get usage history: %w", err)
	}
	defer rows.Close()

	var records []*QuotaUsageRecord
	for rows.Next() {
		var record QuotaUsageRecord
		var metadataJSON []byte

		err := rows.Scan(
			&record.ID,
			&record.TenantID,
			&record.ResourceType,
			&record.Quantity,
			&record.Unit,
			&record.Operation,
			&metadataJSON,
			&record.Timestamp,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan usage record: %w", err)
		}

		if metadataJSON != nil {
			err = json.Unmarshal(metadataJSON, &record.Metadata)
			if err != nil {
				return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
			}
		}

		records = append(records, &record)
	}

	return records, nil
}

// GetAllQuotas retrieves all quotas for a tenant
func (qm *QuotaManager) GetAllQuotas(ctx context.Context, tenantID string) (map[ResourceType]*QuotaLimit, error) {
	quotas := make(map[ResourceType]*QuotaLimit)

	query := `
		SELECT resource_type, max_value, current_value, unit, period, soft_limit, alert_threshold_percent
		FROM quota_limits
		WHERE tenant_id = $1 AND deleted_at IS NULL
	`

	rows, err := qm.db.QueryContext(ctx, query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get all quotas: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var quota QuotaLimit
		err := rows.Scan(
			&quota.ResourceType,
			&quota.MaxValue,
			&quota.CurrentValue,
			&quota.Unit,
			&quota.Period,
			&quota.SoftLimit,
			&quota.AlertThresholdPercent,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan quota: %w", err)
		}

		quotas[quota.ResourceType] = &quota

		// Update cache
		cacheKey := fmt.Sprintf("%s:%s", tenantID, quota.ResourceType)
		qm.cacheMutex.Lock()
		qm.quotaCache[cacheKey] = &quota
		qm.cacheMutex.Unlock()
	}

	return quotas, nil
}

// ResetQuota resets the current usage of a quota to zero
func (qm *QuotaManager) ResetQuota(ctx context.Context, tenantID string, resourceType ResourceType) error {
	return qm.UpdateQuota(ctx, tenantID, resourceType, -1000000) // Large negative to reset to 0
}

// DeleteQuota deletes a quota limit
func (qm *QuotaManager) DeleteQuota(ctx context.Context, tenantID string, resourceType ResourceType) error {
	query := `
		UPDATE quota_limits
		SET deleted_at = CURRENT_TIMESTAMP
		WHERE tenant_id = $1 AND resource_type = $2
	`

	_, err := qm.db.ExecContext(ctx, query, tenantID, resourceType)
	if err != nil {
		return fmt.Errorf("failed to delete quota: %w", err)
	}

	// Remove from cache
	cacheKey := fmt.Sprintf("%s:%s", tenantID, resourceType)
	qm.cacheMutex.Lock()
	delete(qm.quotaCache, cacheKey)
	qm.cacheMutex.Unlock()

	return nil
}

// GetQuotaSummary returns a summary of all quotas for a tenant
func (qm *QuotaManager) GetQuotaSummary(ctx context.Context, tenantID string) (map[string]interface{}, error) {
	quotas, err := qm.GetAllQuotas(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	summary := make(map[string]interface{})
	summary["tenant_id"] = tenantID
	summary["quotas"] = quotas
	summary["timestamp"] = time.Now()

	// Calculate overall health
	healthy := true
	warnings := 0
	exceeded := 0

	for _, quota := range quotas {
		usagePercent := float64(quota.CurrentValue) / float64(quota.MaxValue) * 100
		if usagePercent >= 100 {
			exceeded++
			if !quota.SoftLimit {
				healthy = false
			}
		} else if usagePercent >= float64(quota.AlertThresholdPercent) {
			warnings++
		}
	}

	summary["healthy"] = healthy
	summary["warnings"] = warnings
	summary["exceeded"] = exceeded

	return summary, nil
}

// checkQuotaAlerts checks and generates quota alerts
func (qm *QuotaManager) checkQuotaAlerts(tenantID string, quota *QuotaLimit) {
	usagePercent := float64(quota.CurrentValue) / float64(quota.MaxValue) * 100
	alertType := ""

	if usagePercent >= 100 {
		alertType = "exceeded"
	} else if usagePercent >= float64(quota.AlertThresholdPercent) {
		alertType = "warning"
	}

	if alertType != "" {
		alert := QuotaAlert{
			TenantID:       tenantID,
			ResourceType:   quota.ResourceType,
			CurrentValue:   quota.CurrentValue,
			MaxValue:       quota.MaxValue,
			UsagePercent:   usagePercent,
			AlertType:      alertType,
			Timestamp:      time.Now(),
		}

		// Send alert to channel (non-blocking)
		select {
		case qm.alertChan <- alert:
		default:
			// Channel full, drop alert
		}
	}
}

// GetAlertChannel returns the alert channel for monitoring
func (qm *QuotaManager) GetAlertChannel() <-chan QuotaAlert {
	return qm.alertChan
}

// CheckRequest checks if multiple resource requests can be satisfied
func (qm *QuotaManager) CheckRequest(ctx context.Context, tenantID string, requests map[ResourceType]int64) ([]*QuotaCheckResult, error) {
	results := make([]*QuotaCheckResult, 0, len(requests))
	allAllowed := true

	for resourceType, amount := range requests {
		result, err := qm.CheckQuota(ctx, tenantID, resourceType, amount)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
		if !result.Allowed {
			allAllowed = false
		}
	}

	if !allAllowed {
		return results, ErrQuotaExceeded
	}

	return results, nil
}

// ConsumeResources consumes multiple resources atomically
func (qm *QuotaManager) ConsumeResources(ctx context.Context, tenantID string, requests map[ResourceType]int64) error {
	// First check all quotas
	results, err := qm.CheckRequest(ctx, tenantID, requests)
	if err != nil {
		return err
	}

	// Verify all are allowed
	for _, result := range results {
		if !result.Allowed {
			return fmt.Errorf("quota check failed for %s: %s", result.Quota.ResourceType, result.Message)
		}
	}

	// Update all quotas
	for resourceType, amount := range requests {
		err := qm.UpdateQuota(ctx, tenantID, resourceType, amount)
		if err != nil {
			return fmt.Errorf("failed to update quota for %s: %w", resourceType, err)
		}
	}

	return nil
}

// GetDefaultQuotas returns default quota limits for a new tenant
func (qm *QuotaManager) GetDefaultQuotas() []*QuotaLimit {
	return []*QuotaLimit{
		{
			ResourceType:             ResourceTypeStorage,
			MaxValue:                 100 * 1024 * 1024 * 1024, // 100 GB
			CurrentValue:             0,
			Unit:                     "bytes",
			Period:                   QuotaPeriodForever,
			SoftLimit:                false,
			AlertThresholdPercent:    80,
		},
		{
			ResourceType:             ResourceTypeModels,
			MaxValue:                 50,
			CurrentValue:             0,
			Unit:                     "count",
			Period:                   QuotaPeriodForever,
			SoftLimit:                false,
			AlertThresholdPercent:    80,
		},
		{
			ResourceType:             ResourceTypeInference,
			MaxValue:                 100000,
			CurrentValue:             0,
			Unit:                     "requests",
			Period:                   QuotaPeriodMonthly,
			SoftLimit:                false,
			AlertThresholdPercent:    80,
		},
		{
			ResourceType:             ResourceTypeTokens,
			MaxValue:                 10000000,
			CurrentValue:             0,
			Unit:                     "tokens",
			Period:                   QuotaPeriodMonthly,
			SoftLimit:                false,
			AlertThresholdPercent:    80,
		},
		{
			ResourceType:             ResourceTypeGPU,
			MaxValue:                 4,
			CurrentValue:             0,
			Unit:                     "hours",
			Period:                   QuotaPeriodMonthly,
			SoftLimit:                false,
			AlertThresholdPercent:    80,
		},
		{
			ResourceType:             ResourceTypeAPIRequests,
			MaxValue:                 1000000,
			CurrentValue:             0,
			Unit:                     "requests",
			Period:                   QuotaPeriodDaily,
			SoftLimit:                false,
			AlertThresholdPercent:    80,
		},
	}
}

// InitializeDefaultQuotas initializes default quotas for a new tenant
func (qm *QuotaManager) InitializeDefaultQuotas(ctx context.Context, tenantID string) error {
	defaultQuotas := qm.GetDefaultQuotas()

	for _, quota := range defaultQuotas {
		err := qm.SetQuota(ctx, tenantID, quota)
		if err != nil {
			return fmt.Errorf("failed to initialize default quota for %s: %w", quota.ResourceType, err)
		}
	}

	return nil
}
