// internal/usage/analytics.go
// Usage analytics and insights
// Handles usage analytics, trends analysis, and pattern detection

package usage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

var (
	ErrAnalyticsNotFound      = errors.New("analytics not found")
	ErrInvalidTimeRange     = errors.New("invalid time range")
	ErrInsufficientData     = errors.New("insufficient data for analytics")
)

// TimeRange represents a time range for analytics
type TimeRange struct {
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
}

// UsageTrend represents a usage trend over time
type UsageTrend struct {
	ResourceType ResourceType     `json:"resource_type"`
	Period       string           `json:"period"`       // hourly, daily, weekly, monthly
	DataPoints   []TrendDataPoint `json:"data_points"`
	Trend        string           `json:"trend"`        // increasing, decreasing, stable
	GrowthRate   float64          `json:"growth_rate"`  // percentage
}

// TrendDataPoint represents a single data point in a trend
type TrendDataPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
	Count     int64     `json:"count"`
}

// UsagePattern represents detected usage patterns
type UsagePattern struct {
	Type            string    `json:"type"`             // peak, seasonal, anomalous
	Description     string    `json:"description"`
	StartTime       time.Time `json:"start_time"`
	EndTime         time.Time `json:"end_time"`
	Confidence      float64   `json:"confidence"`
	ResourceType    ResourceType `json:"resource_type"`
	AverageUsage    float64   `json:"average_usage"`
	NormalRange    []float64 `json:"normal_range"`
}

// UsageInsight represents a generated insight
type UsageInsight struct {
	ID          string         `json:"id"`
	Type        string         `json:"type"`        // efficiency, cost, capacity, security
	Title       string         `json:"title"`
	Description string         `json:"description"`
	Severity    string         `json:"severity"`    // info, warning, critical
	TenantID    string         `json:"tenant_id"`
	ResourceType ResourceType    `json:"resource_type"`
	Metrics    map[string]interface{} `json:"metrics"`
	Recommendations []string   `json:"recommendations"`
	CreatedAt   time.Time      `json:"created_at"`
	ExpiresAt   time.Time      `json:"expires_at"`
}

// AnalyticsOptions represents options for analytics queries
type AnalyticsOptions struct {
	TenantID       string
	ResourceTypes  []ResourceType
	TimeRange      TimeRange
	Granularity    string         // hourly, daily, weekly, monthly
	GroupBy       []string       // tenant, resource_type, workspace, user
	CompareWith    *TimeRange     // optional comparison period
}

// AnalyticsManager handles usage analytics
type AnalyticsManager struct {
	db *sql.DB
}

// NewAnalyticsManager creates a new analytics manager
func NewAnalyticsManager(db *sql.DB) *AnalyticsManager {
	return &AnalyticsManager{
		db: db,
	}
}

// GetUsageTrend retrieves usage trends for a resource
func (am *AnalyticsManager) GetUsageTrend(ctx context.Context, tenantID string, resourceType ResourceType, timeRange TimeRange, granularity string) (*UsageTrend, error) {
	if tenantID == "" {
		return nil, errors.New("tenant_id is required")
	}
	if timeRange.StartTime.After(timeRange.EndTime) {
		return nil, ErrInvalidTimeRange
	}

	// Validate granularity
	validGranularities := map[string]bool{
		"hourly":  true,
		"daily":   true,
		"weekly":  true,
		"monthly": true,
	}
	if !validGranularities[granularity] {
		granularity = "daily"
	}

	// Build date truncation based on granularity
	var dateFormat string
	var interval time.Duration
	switch granularity {
	case "hourly":
		dateFormat = "YYYY-MM-DD HH24:00:00"
		interval = time.Hour
	case "daily":
		dateFormat = "YYYY-MM-DD"
		interval = time.Hour * 24
	case "weekly":
		dateFormat = "YYYY-WW"
		interval = time.Hour * 24 * 7
	case "monthly":
		dateFormat = "YYYY-MM"
		interval = time.Hour * 24 * 30
	}

	trend := &UsageTrend{
		ResourceType: resourceType,
		Period:       granularity,
		DataPoints:   []TrendDataPoint{},
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

	rows, err := am.db.QueryContext(ctx, query, dateFormat, tenantID, resourceType, timeRange.StartTime, timeRange.EndTime)
	if err != nil {
		return nil, fmt.Errorf("failed to get usage trend: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var dp TrendDataPoint
		var totalQuantity sql.NullFloat64
		var count int64

		err := rows.Scan(&dp.Timestamp, &totalQuantity, &count)
		if err != nil {
			return nil, fmt.Errorf("failed to scan trend data point: %w", err)
		}

		if totalQuantity.Valid {
			dp.Value = totalQuantity.Float64
		}
		dp.Count = count

		trend.DataPoints = append(trend.DataPoints, dp)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating trend data: %w", err)
	}

	// Calculate trend and growth rate
	if len(trend.DataPoints) >= 2 {
		trend.Trend, trend.GrowthRate = am.calculateTrend(trend.DataPoints)
	} else {
		trend.Trend = "stable"
		trend.GrowthRate = 0
	}

	return trend, nil
}

// DetectUsagePatterns detects usage patterns in historical data
func (am *AnalyticsManager) DetectUsagePatterns(ctx context.Context, tenantID string, resourceType ResourceType, timeRange TimeRange) ([]UsagePattern, error) {
	if tenantID == "" {
		return nil, errors.New("tenant_id is required")
	}

	patterns := []UsagePattern{}

	// Detect peak usage patterns
	peakPattern, err := am.detectPeakUsage(ctx, tenantID, resourceType, timeRange)
	if err == nil && peakPattern != nil {
		patterns = append(patterns, *peakPattern)
	}

	// Detect anomalous usage
	anomalyPattern, err := am.detectAnomalies(ctx, tenantID, resourceType, timeRange)
	if err == nil && anomalyPattern != nil {
		patterns = append(patterns, *anomalyPattern)
	}

	return patterns, nil
}

// GenerateInsights generates usage insights
func (am *AnalyticsManager) GenerateInsights(ctx context.Context, tenantID string, timeRange TimeRange) ([]UsageInsight, error) {
	if tenantID == "" {
		return nil, errors.New("tenant_id is required")
	}

	insights := []UsageInsight{}

	// Generate efficiency insights
	efficiencyInsights, err := am.generateEfficiencyInsights(ctx, tenantID, timeRange)
	if err == nil {
		insights = append(insights, efficiencyInsights...)
	}

	// Generate capacity insights
	capacityInsights, err := am.generateCapacityInsights(ctx, tenantID, timeRange)
	if err == nil {
		insights = append(insights, capacityInsights...)
	}

	// Generate cost insights
	costInsights, err := am.generateCostInsights(ctx, tenantID, timeRange)
	if err == nil {
		insights = append(insights, costInsights...)
	}

	return insights, nil
}

// GetUsageSummary returns a summary of usage statistics
func (am *AnalyticsManager) GetUsageSummary(ctx context.Context, tenantID string, timeRange TimeRange) (map[string]interface{}, error) {
	if tenantID == "" {
		return nil, errors.New("tenant_id is required")
	}

	summary := make(map[string]interface{})

	// Get total usage by resource type
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

	rows, err := am.db.QueryContext(ctx, query, tenantID, timeRange.StartTime, timeRange.EndTime)
	if err != nil {
		return nil, fmt.Errorf("failed to get usage summary: %w", err)
	}
	defer rows.Close()

	resources := []map[string]interface{}{}
	for rows.Next() {
		var resourceType string
		var totalQuantity float64
		var recordCount int64
		var firstRecord, lastRecord time.Time

		err := rows.Scan(&resourceType, &totalQuantity, &recordCount, &firstRecord, &lastRecord)
		if err != nil {
			return nil, fmt.Errorf("failed to scan summary row: %w", err)
		}

		resources = append(resources, map[string]interface{}{
			"resource_type":   resourceType,
			"total_quantity": totalQuantity,
			"record_count":   recordCount,
			"first_record":   firstRecord,
			"last_record":    lastRecord,
		})
	}

	summary["by_resource_type"] = resources
	summary["time_range"] = timeRange
	summary["total_resources"] = len(resources)

	// Get top workspaces by usage
	topWorkspaces, err := am.getTopWorkspaces(ctx, tenantID, timeRange, 5)
	if err == nil {
		summary["top_workspaces"] = topWorkspaces
	}

	// Get top users by usage
	topUsers, err := am.getTopUsers(ctx, tenantID, timeRange, 5)
	if err == nil {
		summary["top_users"] = topUsers
	}

	return summary, nil
}

// CompareUsage compares usage between two time periods
func (am *AnalyticsManager) CompareUsage(ctx context.Context, tenantID string, currentRange, previousRange TimeRange) (map[string]interface{}, error) {
	if tenantID == "" {
		return nil, errors.New("tenant_id is required")
	}

	comparison := make(map[string]interface{})

	// Get current period usage
	currentUsage, err := am.getPeriodUsage(ctx, tenantID, currentRange)
	if err != nil {
		return nil, fmt.Errorf("failed to get current period usage: %w", err)
	}

	// Get previous period usage
	previousUsage, err := am.getPeriodUsage(ctx, tenantID, previousRange)
	if err != nil {
		return nil, fmt.Errorf("failed to get previous period usage: %w", err)
	}

	// Calculate change
	change := make(map[string]interface{})
	for resourceType, current := range currentUsage {
		previous := previousUsage[resourceType]
		if previous > 0 {
			changePercent := ((current - previous) / previous) * 100
			change[string(resourceType)] = map[string]interface{}{
				"current":       current,
				"previous":      previous,
				"change":        current - previous,
				"change_percent": changePercent,
				"trend":         getTrendFromChange(changePercent),
			}
		}
	}

	comparison["current_range"] = currentRange
	comparison["previous_range"] = previousRange
	comparison["change"] = change

	return comparison, nil
}

// calculateTrend calculates the trend direction and growth rate
func (am *AnalyticsManager) calculateTrend(dataPoints []TrendDataPoint) (string, float64) {
	if len(dataPoints) < 2 {
		return "stable", 0
	}

	first := dataPoints[0].Value
	last := dataPoints[len(dataPoints)-1].Value

	if first == 0 {
		return "stable", 0
	}

	growthRate := ((last - first) / first) * 100

	// Determine trend
	if growthRate > 10 {
		return "increasing", growthRate
	} else if growthRate < -10 {
		return "decreasing", growthRate
	} else {
		return "stable", growthRate
	}
}

// detectPeakUsage detects peak usage periods
func (am *AnalyticsManager) detectPeakUsage(ctx context.Context, tenantID string, resourceType ResourceType, timeRange TimeRange) (*UsagePattern, error) {
	query := `
		SELECT
			AVG(quantity) as avg_quantity,
			STDDEV(quantity) as std_quantity,
			MAX(quantity) as max_quantity,
			MIN(quantity) as min_quantity
		FROM usage_records
		WHERE tenant_id = $1
			AND resource_type = $2
			AND recorded_at >= $3
			AND recorded_at <= $4
	`

	var avgQuantity, stdQuantity, maxQuantity, minQuantity float64
	err := am.db.QueryRowContext(ctx, query, tenantID, resourceType, timeRange.StartTime, timeRange.EndTime).Scan(
		&avgQuantity, &stdQuantity, &maxQuantity, &minQuantity,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to detect peak usage: %w", err)
	}

	// If standard deviation is high, there's significant variation
	if stdQuantity > avgQuantity*0.3 {
		return &UsagePattern{
			Type:         "peak",
			Description:  "Usage shows significant variation with peak periods",
			StartTime:    timeRange.StartTime,
			EndTime:      timeRange.EndTime,
			Confidence:   0.85,
			ResourceType: resourceType,
			AverageUsage: avgQuantity,
			NormalRange:  []float64{avgQuantity - stdQuantity, avgQuantity + stdQuantity},
		}, nil
	}

	return nil, nil
}

// detectAnomalies detects anomalous usage patterns
func (am *AnalyticsManager) detectAnomalies(ctx context.Context, tenantID string, resourceType ResourceType, timeRange TimeRange) (*UsagePattern, error) {
	query := `
		SELECT
			AVG(quantity) as avg_quantity,
			STDDEV(quantity) as std_quantity,
			COUNT(*) as total_records,
			COUNT(*) FILTER (WHERE quantity > 3 * (SELECT AVG(quantity) FROM usage_records WHERE tenant_id = $1 AND resource_type = $2)) as anomaly_count
		FROM usage_records
		WHERE tenant_id = $1
			AND resource_type = $2
			AND recorded_at >= $3
			AND recorded_at <= $4
	`

	var avgQuantity, stdQuantity float64
	var totalRecords, anomalyCount int64
	err := am.db.QueryRowContext(ctx, query, tenantID, resourceType, timeRange.StartTime, timeRange.EndTime).Scan(
		&avgQuantity, &stdQuantity, &totalRecords, &anomalyCount,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to detect anomalies: %w", err)
	}

	// If more than 5% of records are anomalies
	if float64(anomalyCount)/float64(totalRecords) > 0.05 {
		return &UsagePattern{
			Type:         "anomalous",
			Description:  "Detected unusually high or low usage values",
			StartTime:    timeRange.StartTime,
			EndTime:      timeRange.EndTime,
			Confidence:   0.75,
			ResourceType: resourceType,
			AverageUsage: avgQuantity,
			NormalRange:  []float64{avgQuantity - 2*stdQuantity, avgQuantity + 2*stdQuantity},
		}, nil
	}

	return nil, nil
}

// generateEfficiencyInsights generates efficiency-related insights
func (am *AnalyticsManager) generateEfficiencyInsights(ctx context.Context, tenantID string, timeRange TimeRange) ([]UsageInsight, error) {
	insights := []UsageInsight{}

	// Check for underutilized resources
	query := `
		SELECT resource_type, COUNT(*) as record_count, AVG(quantity) as avg_quantity
		FROM usage_records
		WHERE tenant_id = $1
			AND recorded_at >= $2
			AND recorded_at <= $3
		GROUP BY resource_type
		HAVING COUNT(*) < 10
	`

	rows, err := am.db.QueryContext(ctx, query, tenantID, timeRange.StartTime, timeRange.EndTime)
	if err != nil {
		return nil, fmt.Errorf("failed to generate efficiency insights: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var resourceType string
		var recordCount int64
		var avgQuantity float64

		err := rows.Scan(&resourceType, &recordCount, &avgQuantity)
		if err != nil {
			continue
		}

		insight := UsageInsight{
			ID:          generateInsightID(),
			Type:        "efficiency",
			Title:       "Low Resource Utilization",
			Description:  fmt.Sprintf("Resource type %s has been used only %d times in the selected period", resourceType, recordCount),
			Severity:    "info",
			TenantID:    tenantID,
			ResourceType: ResourceType(resourceType),
			Metrics: map[string]interface{}{
				"record_count":  recordCount,
				"avg_quantity":  avgQuantity,
				"time_range":    timeRange,
			},
			Recommendations: []string{
				"Consider if this resource type is still needed",
				"Review access patterns for this resource",
				"Archive or remove unused resources",
			},
			CreatedAt: time.Now(),
			ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
		}

		insights = append(insights, insight)
	}

	return insights, nil
}

// generateCapacityInsights generates capacity-related insights
func (am *AnalyticsManager) generateCapacityInsights(ctx context.Context, tenantID string, timeRange TimeRange) ([]UsageInsight, error) {
	insights := []UsageInsight{}

	// Get quota information and compare with usage
	query := `
		SELECT
			q.resource_type,
			q.max_value,
			q.current_value,
			(q.current_value::float / q.max_value::float) * 100 as usage_percent
		FROM quota_limits q
		WHERE q.tenant_id = $1
			AND (q.current_value / q.max_value) > 0.8
	`

	rows, err := am.db.QueryContext(ctx, query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to generate capacity insights: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var resourceType string
		var maxValue, currentValue float64
		var usagePercent float64

		err := rows.Scan(&resourceType, &maxValue, &currentValue, &usagePercent)
		if err != nil {
			continue
		}

		severity := "warning"
		if usagePercent >= 90 {
			severity = "critical"
		}

		insight := UsageInsight{
			ID:          generateInsightID(),
			Type:        "capacity",
			Title:       "High Resource Utilization",
			Description:  fmt.Sprintf("Resource type %s is at %.1f%% capacity", resourceType, usagePercent),
			Severity:    severity,
			TenantID:    tenantID,
			ResourceType: ResourceType(resourceType),
			Metrics: map[string]interface{}{
				"current_usage":    currentValue,
				"max_quota":       maxValue,
				"usage_percent":   usagePercent,
				"remaining":       maxValue - currentValue,
			},
			Recommendations: []string{
				"Monitor usage closely",
				"Consider increasing quotas",
				"Review and optimize resource usage",
			},
			CreatedAt: time.Now(),
			ExpiresAt: time.Now().Add(24 * time.Hour),
		}

		insights = append(insights, insight)
	}

	return insights, nil
}

// generateCostInsights generates cost-related insights
func (am *AnalyticsManager) generateCostInsights(ctx context.Context, tenantID string, timeRange TimeRange) ([]UsageInsight, error) {
	insights := []UsageInsight{}

	// Identify most expensive resource types
	query := `
		SELECT
			resource_type,
			SUM(quantity) as total_usage,
			COUNT(*) as record_count
		FROM usage_records
		WHERE tenant_id = $1
			AND recorded_at >= $2
			AND recorded_at <= $3
		GROUP BY resource_type
		ORDER BY total_usage DESC
		LIMIT 3
	`

	rows, err := am.db.QueryContext(ctx, query, tenantID, timeRange.StartTime, timeRange.EndTime)
	if err != nil {
		return nil, fmt.Errorf("failed to generate cost insights: %w", err)
	}
	defer rows.Close()

	topResources := []map[string]interface{}{}
	for rows.Next() {
		var resourceType string
		var totalUsage float64
		var recordCount int64

		err := rows.Scan(&resourceType, &totalUsage, &recordCount)
		if err != nil {
			continue
		}

		topResources = append(topResources, map[string]interface{}{
			"resource_type": resourceType,
			"total_usage":   totalUsage,
			"record_count":  recordCount,
		})
	}

	if len(topResources) > 0 {
		insight := UsageInsight{
			ID:          generateInsightID(),
			Type:        "cost",
			Title:       "Resource Cost Analysis",
			Description:  "Top resource types by usage volume",
			Severity:    "info",
			TenantID:    tenantID,
			ResourceType: "",
			Metrics: map[string]interface{}{
				"top_resources": topResources,
				"time_range":    timeRange,
			},
			Recommendations: []string{
				"Review usage patterns of top resources",
				"Consider optimizing high-usage resources",
				"Implement cost monitoring alerts",
			},
			CreatedAt: time.Now(),
			ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
		}

		insights = append(insights, insight)
	}

	return insights, nil
}

// getPeriodUsage gets total usage for a time period
func (am *AnalyticsManager) getPeriodUsage(ctx context.Context, tenantID string, timeRange TimeRange) (map[ResourceType]float64, error) {
	query := `
		SELECT
			resource_type,
			SUM(quantity) as total_quantity
		FROM usage_records
		WHERE tenant_id = $1
			AND recorded_at >= $2
			AND recorded_at <= $3
		GROUP BY resource_type
	`

	rows, err := am.db.QueryContext(ctx, query, tenantID, timeRange.StartTime, timeRange.EndTime)
	if err != nil {
		return nil, fmt.Errorf("failed to get period usage: %w", err)
	}
	defer rows.Close()

	usage := make(map[ResourceType]float64)
	for rows.Next() {
		var resourceType ResourceType
		var totalQuantity float64

		err := rows.Scan(&resourceType, &totalQuantity)
		if err != nil {
			continue
		}

		usage[resourceType] = totalQuantity
	}

	return usage, nil
}

// getTopWorkspaces gets top workspaces by usage
func (am *AnalyticsManager) getTopWorkspaces(ctx context.Context, tenantID string, timeRange TimeRange, limit int) ([]map[string]interface{}, error) {
	query := `
		SELECT
			uw.workspace_id,
			SUM(ur.quantity) as total_quantity,
			COUNT(*) as record_count
		FROM usage_records ur
		JOIN workspace_usage uw ON ur.id = uw.usage_record_id
		WHERE ur.tenant_id = $1
			AND ur.recorded_at >= $2
			AND ur.recorded_at <= $3
		GROUP BY uw.workspace_id
		ORDER BY total_quantity DESC
		LIMIT $4
	`

	rows, err := am.db.QueryContext(ctx, query, tenantID, timeRange.StartTime, timeRange.EndTime, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get top workspaces: %w", err)
	}
	defer rows.Close()

	workspaces := []map[string]interface{}{}
	for rows.Next() {
		var workspaceID string
		var totalQuantity float64
		var recordCount int64

		err := rows.Scan(&workspaceID, &totalQuantity, &recordCount)
		if err != nil {
			continue
		}

		workspaces = append(workspaces, map[string]interface{}{
			"workspace_id":  workspaceID,
			"total_usage":   totalQuantity,
			"record_count":  recordCount,
		})
	}

	return workspaces, nil
}

// getTopUsers gets top users by usage
func (am *AnalyticsManager) getTopUsers(ctx context.Context, tenantID string, timeRange TimeRange, limit int) ([]map[string]interface{}, error) {
	query := `
		SELECT
			uu.user_id,
			SUM(ur.quantity) as total_quantity,
			COUNT(*) as record_count
		FROM usage_records ur
		JOIN user_usage uu ON ur.id = uu.usage_record_id
		WHERE ur.tenant_id = $1
			AND ur.recorded_at >= $2
			AND ur.recorded_at <= $3
		GROUP BY uu.user_id
		ORDER BY total_quantity DESC
		LIMIT $4
	`

	rows, err := am.db.QueryContext(ctx, query, tenantID, timeRange.StartTime, timeRange.EndTime, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get top users: %w", err)
	}
	defer rows.Close()

	users := []map[string]interface{}{}
	for rows.Next() {
		var userID string
		var totalQuantity float64
		var recordCount int64

		err := rows.Scan(&userID, &totalQuantity, &recordCount)
		if err != nil {
			continue
		}

		users = append(users, map[string]interface{}{
			"user_id":      userID,
			"total_usage":   totalQuantity,
			"record_count":  recordCount,
		})
	}

	return users, nil
}

// getTrendFromChange returns trend string based on change percentage
func getTrendFromChange(changePercent float64) string {
	if changePercent > 20 {
		return "significant_increase"
	} else if changePercent > 5 {
		return "moderate_increase"
	} else if changePercent > -5 {
		return "stable"
	} else if changePercent > -20 {
		return "moderate_decrease"
	} else {
		return "significant_decrease"
	}
}

// generateInsightID generates a unique insight ID
func generateInsightID() string {
	return fmt.Sprintf("insight-%d", time.Now().UnixNano())
}
