// internal/usage/tracker.go
// Usage tracking for tenant resources
// Handles real-time usage tracking, recording, and aggregation

package usage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

var (
	ErrUsageRecordNotFound = errors.New("usage record not found")
	ErrInvalidUsageData   = errors.New("invalid usage data")
	ErrInvalidResourceType = errors.New("invalid resource type")
)

// UsageRecord represents a single usage event
type UsageRecord struct {
	ID           uuid.UUID    `json:"id" db:"id"`
	TenantID     string       `json:"tenant_id" db:"tenant_id"`
	ResourceType ResourceType `json:"resource_type" db:"resource_type"`
	ResourceID   string       `json:"resource_id,omitempty" db:"resource_id"`
	Quantity     int64        `json:"quantity" db:"quantity"`
	Unit         string       `json:"unit" db:"unit"`
	Operation    string       `json:"operation" db:"operation"` // add, subtract, set

	// Context information
	WorkspaceID  *string      `json:"workspace_id,omitempty" db:"workspace_id"`
	UserID       *string      `json:"user_id,omitempty" db:"user_id"`
	SessionID    *string      `json:"session_id,omitempty" db:"session_id"`

	// Additional metadata
	Metadata     json.RawMessage `json:"metadata,omitempty" db:"metadata"`

	// Timestamps
	RecordedAt   time.Time    `json:"recorded_at" db:"recorded_at"`
	CreatedAt    time.Time    `json:"created_at" db:"created_at"`
}

// UsageSummary represents aggregated usage statistics
type UsageSummary struct {
	TenantID     string                 `json:"tenant_id"`
	ResourceType ResourceType            `json:"resource_type"`
	TotalQuantity int64                 `json:"total_quantity"`
	RecordCount   int64                 `json:"record_count"`
	FirstRecord  time.Time              `json:"first_record"`
	LastRecord   time.Time              `json:"last_record"`
	BreakdownBy  map[string]interface{} `json:"breakdown_by"`
}

// RecordUsageRequest represents a request to record usage
type RecordUsageRequest struct {
	TenantID     string       `json:"tenant_id"`
	ResourceType ResourceType `json:"resource_type"`
	ResourceID   string       `json:"resource_id,omitempty"`
	Quantity     int64        `json:"quantity"`
	Unit         string       `json:"unit"`
	Operation    string       `json:"operation"`
	WorkspaceID  *string      `json:"workspace_id,omitempty"`
	UserID       *string      `json:"user_id,omitempty"`
	SessionID    *string      `json:"session_id,omitempty"`
	Metadata     interface{}   `json:"metadata,omitempty"`
}

// ListUsageOptions represents options for listing usage records
type ListUsageOptions struct {
	TenantID     *string
	ResourceType *ResourceType
	ResourceID   *string
	StartTime    *time.Time
	EndTime      *time.Time
	Limit        int
	Offset       int
}

// Tracker manages usage tracking
type Tracker struct {
	db          *sql.DB
	recordQueue chan UsageRecord
	bufferSize  int
	wg          sync.WaitGroup
}

// NewTracker creates a new usage tracker
func NewTracker(db *sql.DB, bufferSize int) *Tracker {
	return &Tracker{
		db:          db,
		recordQueue: make(chan UsageRecord, bufferSize),
		bufferSize:  bufferSize,
	}
}

// Start starts the background goroutine for processing records
func (t *Tracker) Start() {
	t.wg.Add(1)
	go t.processRecords()
}

// Stop stops the background goroutine
func (t *Tracker) Stop() {
	close(t.recordQueue)
	t.wg.Wait()
}

// RecordUsage records a usage event
func (t *Tracker) RecordUsage(ctx context.Context, req RecordUsageRequest) (*UsageRecord, error) {
	if req.TenantID == "" {
		return nil, ErrInvalidUsageData
	}
	if !isValidResourceType(req.ResourceType) {
		return nil, ErrInvalidResourceType
	}
	if req.Quantity <= 0 {
		return nil, ErrInvalidUsageData
	}

	record := &UsageRecord{
		ID:           uuid.New(),
		TenantID:     req.TenantID,
		ResourceType: req.ResourceType,
		ResourceID:   req.ResourceID,
		Quantity:     req.Quantity,
		Unit:         req.Unit,
		Operation:    req.Operation,
		WorkspaceID:  req.WorkspaceID,
		UserID:       req.UserID,
		SessionID:    req.SessionID,
		RecordedAt:   time.Now(),
		CreatedAt:    time.Now(),
	}

	// Marshal metadata
	if req.Metadata != nil {
		metadataJSON, err := json.Marshal(req.Metadata)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal metadata: %w", err)
		}
		record.Metadata = metadataJSON
	}

	// Insert record
	query := `
		INSERT INTO usage_records (id, tenant_id, resource_type, resource_id,
			quantity, unit, operation, workspace_id, user_id, session_id,
			metadata, recorded_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING id, tenant_id, resource_type, resource_id, quantity, unit,
			operation, workspace_id, user_id, session_id, metadata,
			recorded_at, created_at
	`

	err := t.db.QueryRowContext(ctx, query,
		record.ID,
		record.TenantID,
		record.ResourceType,
		record.ResourceID,
		record.Quantity,
		record.Unit,
		record.Operation,
		record.WorkspaceID,
		record.UserID,
		record.SessionID,
		record.Metadata,
		record.RecordedAt,
		record.CreatedAt,
	).Scan(
		&record.ID,
		&record.TenantID,
		&record.ResourceType,
		&record.ResourceID,
		&record.Quantity,
		&record.Unit,
		&record.Operation,
		&record.WorkspaceID,
		&record.UserID,
		&record.SessionID,
		&record.Metadata,
		&record.RecordedAt,
		&record.CreatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to record usage: %w", err)
	}

	return record, nil
}

// RecordUsageAsync records usage asynchronously
func (t *Tracker) RecordUsageAsync(record UsageRecord) {
	t.recordQueue <- record
}

// processRecords processes records from the queue
func (t *Tracker) processRecords() {
	defer t.wg.Done()

	for record := range t.recordQueue {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

		// Prepare metadata JSON
		var metadataJSON []byte
		if record.Metadata != nil {
			metadataJSON = record.Metadata
		} else {
			metadataJSON = []byte("{}")
		}

		query := `
			INSERT INTO usage_records (id, tenant_id, resource_type, resource_id,
				quantity, unit, operation, workspace_id, user_id, session_id,
				metadata, recorded_at, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		`

		_, err := t.db.ExecContext(ctx, query,
			record.ID,
			record.TenantID,
			record.ResourceType,
			record.ResourceID,
			record.Quantity,
			record.Unit,
			record.Operation,
			record.WorkspaceID,
			record.UserID,
			record.SessionID,
			metadataJSON,
			record.RecordedAt,
			record.CreatedAt,
		)

		if err != nil {
			// Log error but continue processing
			fmt.Printf("Failed to record usage async: %v\n", err)
		}

		cancel()
	}
}

// GetUsageRecord retrieves a usage record by ID
func (t *Tracker) GetUsageRecord(ctx context.Context, id uuid.UUID) (*UsageRecord, error) {
	if id == uuid.Nil {
		return nil, ErrInvalidUsageData
	}

	var record UsageRecord

	query := `
		SELECT id, tenant_id, resource_type, resource_id, quantity, unit,
			operation, workspace_id, user_id, session_id, metadata,
			recorded_at, created_at
		FROM usage_records
		WHERE id = $1
	`

	err := t.db.QueryRowContext(ctx, query, id).Scan(
		&record.ID,
		&record.TenantID,
		&record.ResourceType,
		&record.ResourceID,
		&record.Quantity,
		&record.Unit,
		&record.Operation,
		&record.WorkspaceID,
		&record.UserID,
		&record.SessionID,
		&record.Metadata,
		&record.RecordedAt,
		&record.CreatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrUsageRecordNotFound
		}
		return nil, fmt.Errorf("failed to get usage record: %w", err)
	}

	return &record, nil
}

// ListUsageRecords lists usage records with optional filters
func (t *Tracker) ListUsageRecords(ctx context.Context, opts ListUsageOptions) ([]*UsageRecord, int64, error) {
	records := []*UsageRecord{}
	var total int64

	// Build query with dynamic filters
	baseQuery := `
		SELECT id, tenant_id, resource_type, resource_id, quantity, unit,
			operation, workspace_id, user_id, session_id, metadata,
			recorded_at, created_at
		FROM usage_records
		WHERE 1=1
	`
	countQuery := `SELECT COUNT(*) FROM usage_records WHERE 1=1`

	args := []interface{}{}
	argPos := 1

	if opts.TenantID != nil {
		baseQuery += fmt.Sprintf(" AND tenant_id = $%d", argPos)
		countQuery += fmt.Sprintf(" AND tenant_id = $%d", argPos)
		args = append(args, *opts.TenantID)
		argPos++
	}

	if opts.ResourceType != nil {
		baseQuery += fmt.Sprintf(" AND resource_type = $%d", argPos)
		countQuery += fmt.Sprintf(" AND resource_type = $%d", argPos)
		args = append(args, *opts.ResourceType)
		argPos++
	}

	if opts.ResourceID != nil {
		baseQuery += fmt.Sprintf(" AND resource_id = $%d", argPos)
		countQuery += fmt.Sprintf(" AND resource_id = $%d", argPos)
		args = append(args, *opts.ResourceID)
		argPos++
	}

	if opts.StartTime != nil {
		baseQuery += fmt.Sprintf(" AND recorded_at >= $%d", argPos)
		countQuery += fmt.Sprintf(" AND recorded_at >= $%d", argPos)
		args = append(args, *opts.StartTime)
		argPos++
	}

	if opts.EndTime != nil {
		baseQuery += fmt.Sprintf(" AND recorded_at <= $%d", argPos)
		countQuery += fmt.Sprintf(" AND recorded_at <= $%d", argPos)
		args = append(args, *opts.EndTime)
		argPos++
	}

	// Get total count
	err := t.db.QueryRowContext(ctx, countQuery, args...[:argPos-1]...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count usage records: %w", err)
	}

	// Add pagination
	if opts.Limit > 0 {
		baseQuery += fmt.Sprintf(" LIMIT $%d", argPos)
		args = append(args, opts.Limit)
		argPos++
	}
	if opts.Offset > 0 {
		baseQuery += fmt.Sprintf(" OFFSET $%d", argPos)
		args = append(args, opts.Offset)
		argPos++
	}

	baseQuery += " ORDER BY recorded_at DESC"

	rows, err := t.db.QueryContext(ctx, baseQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list usage records: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var record UsageRecord

		err := rows.Scan(
			&record.ID,
			&record.TenantID,
			&record.ResourceType,
			&record.ResourceID,
			&record.Quantity,
			&record.Unit,
			&record.Operation,
			&record.WorkspaceID,
			&record.UserID,
			&record.SessionID,
			&record.Metadata,
			&record.RecordedAt,
			&record.CreatedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan usage record: %w", err)
		}

		records = append(records, &record)
	}

	if err = rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating usage records: %w", err)
	}

	return records, total, nil
}

// GetUsageSummary retrieves aggregated usage statistics
func (t *Tracker) GetUsageSummary(ctx context.Context, tenantID string, resourceType ResourceType, startTime, endTime time.Time) (*UsageSummary, error) {
	if tenantID == "" {
		return nil, ErrInvalidUsageData
	}

	summary := &UsageSummary{
		TenantID:     tenantID,
		ResourceType: resourceType,
		BreakdownBy:  make(map[string]interface{}),
	}

	query := `
		SELECT
			SUM(quantity) as total_quantity,
			COUNT(*) as record_count,
			MIN(recorded_at) as first_record,
			MAX(recorded_at) as last_record
		FROM usage_records
		WHERE tenant_id = $1
			AND resource_type = $2
			AND recorded_at >= $3
			AND recorded_at <= $4
	`

	err := t.db.QueryRowContext(ctx, query, tenantID, resourceType, startTime, endTime).Scan(
		&summary.TotalQuantity,
		&summary.RecordCount,
		&summary.FirstRecord,
		&summary.LastRecord,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get usage summary: %w", err)
	}

	// Get breakdown by resource ID
	breakdownQuery := `
		SELECT resource_id, SUM(quantity), COUNT(*)
		FROM usage_records
		WHERE tenant_id = $1 AND resource_type = $2
			AND recorded_at >= $3 AND recorded_at <= $4
		GROUP BY resource_id
		ORDER BY SUM(quantity) DESC
		LIMIT 10
	`

	rows, err := t.db.QueryContext(ctx, breakdownQuery, tenantID, resourceType, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("failed to get usage breakdown: %w", err)
	}
	defer rows.Close()

	topResources := []map[string]interface{}{}
	for rows.Next() {
		var resourceID string
		var totalQuantity int64
		var count int64

		err := rows.Scan(&resourceID, &totalQuantity, &count)
		if err != nil {
			continue
		}

		topResources = append(topResources, map[string]interface{}{
			"resource_id":     resourceID,
			"total_quantity":  totalQuantity,
			"record_count":    count,
		})
	}

	summary.BreakdownBy["top_resources"] = topResources

	return summary, nil
}

// GetTenantUsage retrieves usage summary for a tenant across all resources
func (t *Tracker) GetTenantUsage(ctx context.Context, tenantID string, startTime, endTime time.Time) (map[ResourceType]*UsageSummary, error) {
	if tenantID == "" {
		return nil, ErrInvalidUsageData
	}

	summaries := make(map[ResourceType]*UsageSummary)

	query := `
		SELECT
			resource_type,
			SUM(quantity) as total_quantity,
			COUNT(*) as record_count,
			MIN(recorded_at) as first_record,
			MAX(recorded_at) as last_record
		FROM usage_records
		WHERE tenant_id = $1
			AND recorded_at >= $2
			AND recorded_at <= $3
		GROUP BY resource_type
		ORDER BY total_quantity DESC
	`

	rows, err := t.db.QueryContext(ctx, query, tenantID, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("failed to get tenant usage: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var resourceType ResourceType
		var summary UsageSummary

		err := rows.Scan(
			&resourceType,
			&summary.TotalQuantity,
			&summary.RecordCount,
			&summary.FirstRecord,
			&summary.LastRecord,
		)
		if err != nil {
			continue
		}

		summary.TenantID = tenantID
		summary.ResourceType = resourceType
		summary.BreakdownBy = make(map[string]interface{})

		summaries[resourceType] = &summary
	}

	return summaries, nil
}

// DeleteUsageRecords deletes usage records older than specified time
func (t *Tracker) DeleteUsageRecords(ctx context.Context, beforeTime time.Time) (int64, error) {
	query := `
		DELETE FROM usage_records
		WHERE recorded_at < $1
		RETURNING COUNT(*)
	`

	var deletedCount int64
	err := t.db.QueryRowContext(ctx, query, beforeTime).Scan(&deletedCount)
	if err != nil {
		return 0, fmt.Errorf("failed to delete usage records: %w", err)
	}

	return deletedCount, nil
}

// AggregateUsage aggregates usage data by time period
func (t *Tracker) AggregateUsage(ctx context.Context, tenantID string, resourceType ResourceType, startTime, endTime time.Time, period string) ([]map[string]interface{}, error) {
	if tenantID == "" {
		return nil, ErrInvalidUsageData
	}

	var dateFormat string
	switch period {
	case "hourly":
		dateFormat = "YYYY-MM-DD HH24:00:00"
	case "daily":
		dateFormat = "YYYY-MM-DD"
	case "weekly":
		dateFormat = "YYYY-WW"
	case "monthly":
		dateFormat = "YYYY-MM"
	default:
		dateFormat = "YYYY-MM-DD"
	}

	query := `
		SELECT
			DATE_TRUNC($1, recorded_at) as period,
			SUM(quantity) as total_quantity,
			COUNT(*) as record_count
		FROM usage_records
		WHERE tenant_id = $2
			AND resource_type = $3
			AND recorded_at >= $4
			AND recorded_at <= $5
		GROUP BY DATE_TRUNC($1, recorded_at)
		ORDER BY period ASC
	`

	rows, err := t.db.QueryContext(ctx, query, dateFormat, tenantID, resourceType, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate usage: %w", err)
	}
	defer rows.Close()

	aggregated := []map[string]interface{}{}
	for rows.Next() {
		var period time.Time
		var totalQuantity float64
		var recordCount int64

		err := rows.Scan(&period, &totalQuantity, &recordCount)
		if err != nil {
			continue
		}

		aggregated = append(aggregated, map[string]interface{}{
			"period":         period,
			"total_quantity":  totalQuantity,
			"record_count":    recordCount,
		})
	}

	return aggregated, nil
}

// BatchRecordUsage records multiple usage events in a single transaction
func (t *Tracker) BatchRecordUsage(ctx context.Context, requests []RecordUsageRequest) error {
	if len(requests) == 0 {
		return nil
	}

	tx, err := t.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	query := `
		INSERT INTO usage_records (id, tenant_id, resource_type, resource_id,
			quantity, unit, operation, workspace_id, user_id, session_id,
			metadata, recorded_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`

	for _, req := range requests {
		if req.TenantID == "" {
			return ErrInvalidUsageData
		}
		if !isValidResourceType(req.ResourceType) {
			return ErrInvalidResourceType
		}

		record := UsageRecord{
			ID:           uuid.New(),
			TenantID:     req.TenantID,
			ResourceType: req.ResourceType,
			ResourceID:   req.ResourceID,
			Quantity:     req.Quantity,
			Unit:         req.Unit,
			Operation:    req.Operation,
			WorkspaceID:  req.WorkspaceID,
			UserID:       req.UserID,
			SessionID:    req.SessionID,
			RecordedAt:   time.Now(),
			CreatedAt:    time.Now(),
		}

		// Marshal metadata
		var metadataJSON []byte
		if req.Metadata != nil {
			metadataJSON, _ = json.Marshal(req.Metadata)
		} else {
			metadataJSON = []byte("{}")
		}

		_, err := tx.ExecContext(ctx, query,
			record.ID,
			record.TenantID,
			record.ResourceType,
			record.ResourceID,
			record.Quantity,
			record.Unit,
			record.Operation,
			record.WorkspaceID,
			record.UserID,
			record.SessionID,
			metadataJSON,
			record.RecordedAt,
			record.CreatedAt,
		)

		if err != nil {
			return fmt.Errorf("failed to batch record usage: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// isValidResourceType checks if a resource type is valid
func isValidResourceType(resourceType ResourceType) bool {
	switch resourceType {
	case ResourceTypeStorage,
		ResourceTypeModels,
		ResourceTypeInference,
		ResourceTypeTokens,
		ResourceTypeGPU,
		ResourceTypeCPU,
		ResourceTypeAPIRequests,
		ResourceTypeBandwidth:
		return true
	default:
		return false
	}
}

// GetUsageByWorkspace retrieves usage for a specific workspace
func (t *Tracker) GetUsageByWorkspace(ctx context.Context, tenantID string, workspaceID string, startTime, endTime time.Time) (map[ResourceType]*UsageSummary, error) {
	if tenantID == "" || workspaceID == "" {
		return nil, ErrInvalidUsageData
	}

	summaries := make(map[ResourceType]*UsageSummary)

	query := `
		SELECT
			resource_type,
			SUM(quantity) as total_quantity,
			COUNT(*) as record_count,
			MIN(recorded_at) as first_record,
			MAX(recorded_at) as last_record
		FROM usage_records
		WHERE tenant_id = $1 AND workspace_id = $2
			AND recorded_at >= $3
			AND recorded_at <= $4
		GROUP BY resource_type
		ORDER BY total_quantity DESC
	`

	rows, err := t.db.QueryContext(ctx, query, tenantID, workspaceID, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace usage: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var resourceType ResourceType
		var summary UsageSummary

		err := rows.Scan(
			&resourceType,
			&summary.TotalQuantity,
			&summary.RecordCount,
			&summary.FirstRecord,
			&summary.LastRecord,
		)
		if err != nil {
			continue
		}

		summary.TenantID = tenantID
		summary.ResourceType = resourceType
		summary.BreakdownBy = make(map[string]interface{})
		summary.BreakdownBy["workspace_id"] = workspaceID

		summaries[resourceType] = &summary
	}

	return summaries, nil
}

// GetCurrentUsage retrieves current usage (since beginning of current period)
func (t *Tracker) GetCurrentUsage(ctx context.Context, tenantID string) (map[ResourceType]int64, error) {
	if tenantID == "" {
		return nil, ErrInvalidUsageData
	}

	// Get current month start
	now := time.Now()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	usage := make(map[ResourceType]int64)

	query := `
		SELECT resource_type, SUM(quantity)
		FROM usage_records
		WHERE tenant_id = $1 AND recorded_at >= $2
		GROUP BY resource_type
	`

	rows, err := t.db.QueryContext(ctx, query, tenantID, monthStart)
	if err != nil {
		return nil, fmt.Errorf("failed to get current usage: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var resourceType ResourceType
		var totalQuantity int64

		err := rows.Scan(&resourceType, &totalQuantity)
		if err != nil {
			continue
		}

		usage[resourceType] = totalQuantity
	}

	return usage, nil
}
