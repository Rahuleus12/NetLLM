// internal/usage/reporter.go
// Usage reporting functionality
// Handles report generation, aggregation, and export

package usage

import (
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

var (
	ErrReportNotFound       = errors.New("report not found")
	ErrInvalidReport       = errors.New("invalid report data")
	ErrReportGenerationFailed = errors.New("report generation failed")
)

// ReportType represents the type of usage report
type ReportType string

const (
	ReportTypeUsage      ReportType = "usage"
	ReportTypeBilling    ReportType = "billing"
	ReportTypePerformance ReportType = "performance"
	ReportTypeQuota      ReportType = "quota"
	ReportTypeSummary    ReportType = "summary"
)

// ReportFormat represents the format of the report
type ReportFormat string

const (
	ReportFormatJSON  ReportFormat = "json"
	ReportFormatCSV   ReportFormat = "csv"
	ReportFormatPDF   ReportFormat = "pdf"
	ReportFormatHTML  ReportFormat = "html"
)

// ReportPeriod represents the time period for a report
type ReportPeriod string

const (
	ReportPeriodHourly  ReportPeriod = "hourly"
	ReportPeriodDaily   ReportPeriod = "daily"
	ReportPeriodWeekly  ReportPeriod = "weekly"
	ReportPeriodMonthly ReportPeriod = "monthly"
	ReportPeriodCustom  ReportPeriod = "custom"
)

// ReportStatus represents the status of a report
type ReportStatus string

const (
	ReportStatusPending   ReportStatus = "pending"
	ReportStatusGenerating ReportStatus = "generating"
	ReportStatusCompleted ReportStatus = "completed"
	ReportStatusFailed    ReportStatus = "failed"
)

// UsageReport represents a generated usage report
type UsageReport struct {
	ID          uuid.UUID      `json:"id" db:"id"`
	TenantID    uuid.UUID      `json:"tenant_id" db:"tenant_id"`
	Type        ReportType    `json:"type" db:"type"`
	Period      ReportPeriod  `json:"period" db:"period"`
	Format      ReportFormat   `json:"format" db:"format"`
	Status      ReportStatus  `json:"status" db:"status"`

	// Report parameters
	StartTime   time.Time     `json:"start_time" db:"start_time"`
	EndTime     time.Time     `json:"end_time" db:"end_time"`
	ResourceType string        `json:"resource_type,omitempty" db:"resource_type"`

	// Report data
	Data        json.RawMessage `json:"data,omitempty" db:"data"`
	Summary     ReportSummary  `json:"summary" db:"summary"`

	// File reference
	FileURL     string        `json:"file_url,omitempty" db:"file_url"`
	FileSize    int64         `json:"file_size,omitempty" db:"file_size"`

	// Metadata
	GeneratedBy uuid.UUID      `json:"generated_by" db:"generated_by"`
	Parameters  map[string]interface{} `json:"parameters,omitempty" db:"parameters"`

	// Timestamps
	CreatedAt   time.Time     `json:"created_at" db:"created_at"`
	CompletedAt *time.Time    `json:"completed_at,omitempty" db:"completed_at"`
}

// ReportSummary represents summary statistics in a report
type ReportSummary struct {
	TotalRecords      int64                 `json:"total_records"`
	TotalQuantity     int64                 `json:"total_quantity"`
	AveragePerDay     float64               `json:"average_per_day"`
	PeakUsage        PeakUsage             `json:"peak_usage"`
	BreakdownByType  map[string]int64      `json:"breakdown_by_type"`
	BreakdownByUser  map[string]int64      `json:"breakdown_by_user"`
	TrendAnalysis    map[string]interface{} `json:"trend_analysis"`
}

// PeakUsage represents peak usage information
type PeakUsage struct {
	Timestamp   time.Time `json:"timestamp"`
	Quantity   int64     `json:"quantity"`
	ResourceID string    `json:"resource_id,omitempty"`
	UserID     string    `json:"user_id,omitempty"`
}

// GenerateReportRequest represents a request to generate a new report
type GenerateReportRequest struct {
	TenantID     uuid.UUID      `json:"tenant_id"`
	Type         ReportType    `json:"type"`
	Period       ReportPeriod  `json:"period"`
	Format       ReportFormat   `json:"format"`
	StartTime    time.Time     `json:"start_time"`
	EndTime      time.Time     `json:"end_time"`
	ResourceType string        `json:"resource_type,omitempty"`
	Parameters   map[string]interface{} `json:"parameters,omitempty"`
}

// ListReportsOptions represents options for listing reports
type ListReportsOptions struct {
	TenantID    *uuid.UUID
	Type        *ReportType
	Period      *ReportPeriod
	Status      *ReportStatus
	StartTime   *time.Time
	EndTime     *time.Time
	Limit       int
	Offset      int
}

// Reporter manages usage reports
type Reporter struct {
	db *sql.DB
}

// NewReporter creates a new reporter
func NewReporter(db *sql.DB) *Reporter {
	return &Reporter{
		db: db,
	}
}

// GenerateReport generates a new usage report
func (r *Reporter) GenerateReport(ctx context.Context, req GenerateReportRequest, generatedBy uuid.UUID) (*UsageReport, error) {
	if req.TenantID == uuid.Nil {
		return nil, ErrInvalidReport
	}
	if req.StartTime.After(req.EndTime) {
		return nil, errors.New("start_time must be before end_time")
	}
	if !isValidReportType(req.Type) {
		return nil, ErrInvalidReport
	}
	if !isValidReportFormat(req.Format) {
		return nil, ErrInvalidReport
	}

	report := &UsageReport{
		ID:          uuid.New(),
		TenantID:    req.TenantID,
		Type:        req.Type,
		Period:      req.Period,
		Format:      req.Format,
		Status:      ReportStatusPending,
		StartTime:   req.StartTime,
		EndTime:     req.EndTime,
		ResourceType: req.ResourceType,
		GeneratedBy: generatedBy,
		Parameters:  req.Parameters,
		CreatedAt:   time.Now(),
	}

	// Insert report
	dataJSON := []byte("{}")
	summaryJSON, err := json.Marshal(ReportSummary{})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal summary: %w", err)
	}

	parametersJSON, err := json.Marshal(req.Parameters)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal parameters: %w", err)
	}

	query := `
		INSERT INTO usage_reports (id, tenant_id, type, period, format, status,
			start_time, end_time, resource_type, data, summary, generated_by,
			parameters, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		RETURNING id, tenant_id, type, period, format, status, start_time, end_time,
			resource_type, data, summary, file_url, file_size, generated_by,
			parameters, created_at, completed_at
	`

	err = r.db.QueryRowContext(ctx, query,
		report.ID,
		report.TenantID,
		report.Type,
		report.Period,
		report.Format,
		report.Status,
		report.StartTime,
		report.EndTime,
		report.ResourceType,
		dataJSON,
		summaryJSON,
		report.GeneratedBy,
		parametersJSON,
		report.CreatedAt,
	).Scan(
		&report.ID,
		&report.TenantID,
		&report.Type,
		&report.Period,
		&report.Format,
		&report.Status,
		&report.StartTime,
		&report.EndTime,
		&report.ResourceType,
		&dataJSON,
		&summaryJSON,
		&report.FileURL,
		&report.FileSize,
		&report.GeneratedBy,
		&parametersJSON,
		&report.CreatedAt,
		&report.CompletedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create report: %w", err)
	}

	// Generate report data
	err = r.generateReportData(ctx, report)
	if err != nil {
		r.updateReportStatus(ctx, report.ID, ReportStatusFailed, err.Error())
		return nil, fmt.Errorf("failed to generate report data: %w", err)
	}

	// Mark as completed
	now := time.Now()
	report.CompletedAt = &now
	err = r.updateReportStatus(ctx, report.ID, ReportStatusCompleted, "")
	if err != nil {
		return nil, fmt.Errorf("failed to update report status: %w", err)
	}

	return report, nil
}

// GetReport retrieves a report by ID
func (r *Reporter) GetReport(ctx context.Context, id uuid.UUID) (*UsageReport, error) {
	if id == uuid.Nil {
		return nil, ErrInvalidReport
	}

	var report UsageReport
	var dataJSON, summaryJSON, parametersJSON []byte

	query := `
		SELECT id, tenant_id, type, period, format, status, start_time, end_time,
			resource_type, data, summary, file_url, file_size, generated_by,
			parameters, created_at, completed_at
		FROM usage_reports
		WHERE id = $1
	`

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&report.ID,
		&report.TenantID,
		&report.Type,
		&report.Period,
		&report.Format,
		&report.Status,
		&report.StartTime,
		&report.EndTime,
		&report.ResourceType,
		&dataJSON,
		&summaryJSON,
		&report.FileURL,
		&report.FileSize,
		&report.GeneratedBy,
		&parametersJSON,
		&report.CreatedAt,
		&report.CompletedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrReportNotFound
		}
		return nil, fmt.Errorf("failed to get report: %w", err)
	}

	// Unmarshal JSON fields
	if dataJSON != nil {
		report.Data = dataJSON
	}
	if summaryJSON != nil {
		json.Unmarshal(summaryJSON, &report.Summary)
	}
	if parametersJSON != nil {
		json.Unmarshal(parametersJSON, &report.Parameters)
	}

	return &report, nil
}

// ListReports lists reports with optional filters
func (r *Reporter) ListReports(ctx context.Context, opts ListReportsOptions) ([]*UsageReport, int64, error) {
	reports := []*UsageReport{}
	var total int64

	// Build query with dynamic filters
	baseQuery := `
		SELECT id, tenant_id, type, period, format, status, start_time, end_time,
			resource_type, data, summary, file_url, file_size, generated_by,
			parameters, created_at, completed_at
		FROM usage_reports
		WHERE 1=1
	`
	countQuery := `SELECT COUNT(*) FROM usage_reports WHERE 1=1`

	args := []interface{}{}
	argPos := 1

	if opts.TenantID != nil {
		baseQuery += fmt.Sprintf(" AND tenant_id = $%d", argPos)
		countQuery += fmt.Sprintf(" AND tenant_id = $%d", argPos)
		args = append(args, *opts.TenantID)
		argPos++
	}

	if opts.Type != nil {
		baseQuery += fmt.Sprintf(" AND type = $%d", argPos)
		countQuery += fmt.Sprintf(" AND type = $%d", argPos)
		args = append(args, *opts.Type)
		argPos++
	}

	if opts.Period != nil {
		baseQuery += fmt.Sprintf(" AND period = $%d", argPos)
		countQuery += fmt.Sprintf(" AND period = $%d", argPos)
		args = append(args, *opts.Period)
		argPos++
	}

	if opts.Status != nil {
		baseQuery += fmt.Sprintf(" AND status = $%d", argPos)
		countQuery += fmt.Sprintf(" AND status = $%d", argPos)
		args = append(args, *opts.Status)
		argPos++
	}

	if opts.StartTime != nil {
		baseQuery += fmt.Sprintf(" AND created_at >= $%d", argPos)
		countQuery += fmt.Sprintf(" AND created_at >= $%d", argPos)
		args = append(args, *opts.StartTime)
		argPos++
	}

	if opts.EndTime != nil {
		baseQuery += fmt.Sprintf(" AND created_at <= $%d", argPos)
		countQuery += fmt.Sprintf(" AND created_at <= $%d", argPos)
		args = append(args, *opts.EndTime)
		argPos++
	}

	// Get total count
	err := r.db.QueryRowContext(ctx, countQuery, args...[:argPos-1]...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count reports: %w", err)
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

	baseQuery += " ORDER BY created_at DESC"

	rows, err := r.db.QueryContext(ctx, baseQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list reports: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var report UsageReport
		var dataJSON, summaryJSON, parametersJSON []byte

		err := rows.Scan(
			&report.ID,
			&report.TenantID,
			&report.Type,
			&report.Period,
			&report.Format,
			&report.Status,
			&report.StartTime,
			&report.EndTime,
			&report.ResourceType,
			&dataJSON,
			&summaryJSON,
			&report.FileURL,
			&report.FileSize,
			&report.GeneratedBy,
			&parametersJSON,
			&report.CreatedAt,
			&report.CompletedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan report: %w", err)
		}

		if dataJSON != nil {
			report.Data = dataJSON
		}
		if summaryJSON != nil {
			json.Unmarshal(summaryJSON, &report.Summary)
		}
		if parametersJSON != nil {
			json.Unmarshal(parametersJSON, &report.Parameters)
		}

		reports = append(reports, &report)
	}

	if err = rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating reports: %w", err)
	}

	return reports, total, nil
}

// DeleteReport deletes a report
func (r *Reporter) DeleteReport(ctx context.Context, id uuid.UUID) error {
	if id == uuid.Nil {
		return ErrInvalidReport
	}

	query := `DELETE FROM usage_reports WHERE id = $1`

	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete report: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return ErrReportNotFound
	}

	return nil
}

// ExportReport exports a report to a file
func (r *Reporter) ExportReport(ctx context.Context, reportID uuid.UUID, format ReportFormat) ([]byte, error) {
	report, err := r.GetReport(ctx, reportID)
	if err != nil {
		return nil, err
	}

	switch format {
	case ReportFormatJSON:
		return r.exportToJSON(report)
	case ReportFormatCSV:
		return r.exportToCSV(report)
	default:
		return nil, fmt.Errorf("unsupported format: %s", format)
	}
}

// generateReportData generates the data for a report
func (r *Reporter) generateReportData(ctx context.Context, report *UsageReport) error {
	report.Status = ReportStatusGenerating

	// Update status to generating
	err := r.updateReportStatus(ctx, report.ID, ReportStatusGenerating, "")
	if err != nil {
		return err
	}

	// Generate summary based on report type
	summary, err := r.generateSummary(ctx, report)
	if err != nil {
		return err
	}

	// Serialize data and summary
	dataJSON, err := json.Marshal(summary)
	if err != nil {
		return fmt.Errorf("failed to marshal data: %w", err)
	}

	summaryJSON, err := json.Marshal(summary)
	if err != nil {
		return fmt.Errorf("failed to marshal summary: %w", err)
	}

	// Update report with data
	query := `
		UPDATE usage_reports
		SET data = $1, summary = $2
		WHERE id = $3
	`

	_, err = r.db.ExecContext(ctx, query, dataJSON, summaryJSON, report.ID)
	if err != nil {
		return fmt.Errorf("failed to update report data: %w", err)
	}

	return nil
}

// generateSummary generates summary statistics for a report
func (r *Reporter) generateSummary(ctx context.Context, report *UsageReport) (*ReportSummary, error) {
	summary := &ReportSummary{
		BreakdownByType: make(map[string]int64),
		BreakdownByUser: make(map[string]int64),
		TrendAnalysis:   make(map[string]interface{}),
	}

	// Query usage records for the period
	query := `
		SELECT resource_type, SUM(quantity), COUNT(*)
		FROM usage_records
		WHERE tenant_id = $1 AND recorded_at >= $2 AND recorded_at <= $3
		GROUP BY resource_type
	`

	rows, err := r.db.QueryContext(ctx, query, report.TenantID, report.StartTime, report.EndTime)
	if err != nil {
		return nil, fmt.Errorf("failed to query usage records: %w", err)
	}
	defer rows.Close()

	totalQuantity := int64(0)
	totalRecords := int64(0)

	for rows.Next() {
		var resourceType string
		var quantity int64
		var count int64

		err := rows.Scan(&resourceType, &quantity, &count)
		if err != nil {
			return nil, fmt.Errorf("failed to scan usage record: %w", err)
		}

		summary.BreakdownByType[resourceType] = quantity
		totalQuantity += quantity
		totalRecords += count
	}

	summary.TotalQuantity = totalQuantity
	summary.TotalRecords = totalRecords

	// Calculate average per day
	days := report.EndTime.Sub(report.StartTime).Hours() / 24
	if days > 0 {
		summary.AveragePerDay = float64(totalQuantity) / days
	}

	// Find peak usage
	peak, err := r.findPeakUsage(ctx, report.TenantID, report.StartTime, report.EndTime)
	if err == nil {
		summary.PeakUsage = *peak
	}

	return summary, nil
}

// findPeakUsage finds the peak usage within a time period
func (r *Reporter) findPeakUsage(ctx context.Context, tenantID uuid.UUID, startTime, endTime time.Time) (*PeakUsage, error) {
	query := `
		SELECT quantity, recorded_at, resource_type
		FROM usage_records
		WHERE tenant_id = $1 AND recorded_at >= $2 AND recorded_at <= $3
		ORDER BY quantity DESC
		LIMIT 1
	`

	var peak PeakUsage
	err := r.db.QueryRowContext(ctx, query, tenantID, startTime, endTime).Scan(
		&peak.Quantity,
		&peak.Timestamp,
		&peak.ResourceID,
	)

	if err != nil {
		return nil, err
	}

	return &peak, nil
}

// updateReportStatus updates the status of a report
func (r *Reporter) updateReportStatus(ctx context.Context, id uuid.UUID, status ReportStatus, errorMessage string) error {
	query := `
		UPDATE usage_reports
		SET status = $1, updated_at = CURRENT_TIMESTAMP
		WHERE id = $2
	`

	result, err := r.db.ExecContext(ctx, query, status, id)
	if err != nil {
		return fmt.Errorf("failed to update report status: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return ErrReportNotFound
	}

	return nil
}

// exportToJSON exports a report to JSON format
func (r *Reporter) exportToJSON(report *UsageReport) ([]byte, error) {
	return json.MarshalIndent(report, "", "  ")
}

// exportToCSV exports a report to CSV format
func (r *Reporter) exportToCSV(report *UsageReport) ([]byte, error) {
	var output [][]string

	// Header
	output = append(output, []string{"Tenant ID", "Type", "Period", "Start Time", "End Time", "Total Quantity", "Average Per Day"})

	// Data row
	summary := report.Summary
	output = append(output, []string{
		report.TenantID.String(),
		string(report.Type),
		string(report.Period),
		report.StartTime.Format(time.RFC3339),
		report.EndTime.Format(time.RFC3339),
		fmt.Sprintf("%d", summary.TotalQuantity),
		fmt.Sprintf("%.2f", summary.AveragePerDay),
	})

	// Convert to CSV
	var result []byte
	writer := csv.NewWriter(bytesWriter{&result})
	err := writer.WriteAll(output)
	if err != nil {
		return nil, fmt.Errorf("failed to write CSV: %w", err)
	}

	return result, nil
}

// bytesWriter is a helper for writing to []byte
type bytesWriter struct {
	data *[]byte
}

func (bw bytesWriter) Write(p []byte) (n int, err error) {
	*bw.data = append(*bw.data, p...)
	return len(p), nil
}

func isValidReportType(reportType ReportType) bool {
	switch reportType {
	case ReportTypeUsage, ReportTypeBilling, ReportTypePerformance, ReportTypeQuota, ReportTypeSummary:
		return true
	default:
		return false
	}
}

func isValidReportFormat(format ReportFormat) bool {
	switch format {
	case ReportFormatJSON, ReportFormatCSV, ReportFormatPDF, ReportFormatHTML:
		return true
	default:
		return false
	}
}

// ScheduleReport schedules a report to be generated periodically
func (r *Reporter) ScheduleReport(ctx context.Context, req GenerateReportRequest, schedule string, generatedBy uuid.UUID) error {
	// In a production system, this would integrate with a job scheduler
	// For now, we'll just generate the report immediately
	_, err := r.GenerateReport(ctx, req, generatedBy)
	return err
}

// GetScheduledReports retrieves all scheduled reports for a tenant
func (r *Reporter) GetScheduledReports(ctx context.Context, tenantID uuid.UUID) ([]*UsageReport, error) {
	opts := ListReportsOptions{
		TenantID: &tenantID,
		Status:   func() *ReportStatus { s := ReportStatusPending; return &s }(),
	}

	reports, _, err := r.ListReports(ctx, opts)
	return reports, err
}
