package analytics

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var (
	ErrAnalyticsNotFound    = errors.New("analytics data not found")
	ErrInvalidTimeRange     = errors.New("invalid time range")
	ErrInsufficientData     = errors.New("insufficient data for analysis")
	ErrInvalidGranularity   = errors.New("invalid granularity")
	ErrQueryFailed          = errors.New("analytics query failed")
	ErrAggregationFailed    = errors.New("data aggregation failed")
)

type Granularity string

const (
	GranularityMinute  Granularity = "minute"
	GranularityHour    Granularity = "hour"
	GranularityDay     Granularity = "day"
	GranularityWeek    Granularity = "week"
	GranularityMonth   Granularity = "month"
	GranularityQuarter Granularity = "quarter"
	GranularityYear    Granularity = "year"
)

type MetricType string

const (
	MetricTypeUsage       MetricType = "usage"
	MetricTypePerformance MetricType = "performance"
	MetricTypeCost        MetricType = "cost"
	MetricTypeError       MetricType = "error"
	MetricTypeAvailability MetricType = "availability"
	MetricTypeLatency     MetricType = "latency"
	MetricTypeThroughput  MetricType = "throughput"
	MetricTypeSuccess     MetricType = "success"
)

type AggregationType string

const (
	AggregationSum   AggregationType = "sum"
	AggregationAvg   AggregationType = "avg"
	AggregationMin   AggregationType = "min"
	AggregationMax   AggregationType = "max"
	AggregationCount AggregationType = "count"
	AggregationP95   AggregationType = "p95"
	AggregationP99   AggregationType = "p99"
)

type TimeRange struct {
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
}

type AnalyticsQuery struct {
	ID             uuid.UUID          `json:"id"`
	TenantID       uuid.UUID          `json:"tenant_id"`
	Name           string             `json:"name"`
	Description    string             `json:"description"`
	MetricType     MetricType         `json:"metric_type"`
	TimeRange      TimeRange          `json:"time_range"`
	Granularity    Granularity        `json:"granularity"`
	Aggregations   []AggregationType  `json:"aggregations"`
	Filters        []QueryFilter      `json:"filters"`
	GroupBy        []string           `json:"group_by"`
	OrderBy        string             `json:"order_by"`
	Limit          int                `json:"limit"`
	IncludeMissing bool               `json:"include_missing"`
	CreatedAt      time.Time          `json:"created_at" gorm:"autoCreateTime"`
}

type QueryFilter struct {
	Field    string      `json:"field"`
	Operator string      `json:"operator"`
	Value    interface{} `json:"value"`
}

type AnalyticsResult struct {
	QueryID       uuid.UUID           `json:"query_id"`
	ExecutedAt    time.Time           `json:"executed_at"`
	TimeRange     TimeRange           `json:"time_range"`
	Granularity   Granularity         `json:"granularity"`
	Series        []MetricSeries      `json:"series"`
	Summary       ResultSummary       `json:"summary"`
	Metadata      ResultMetadata      `json:"metadata"`
	ExecutionTime time.Duration       `json:"execution_time"`
	FromCache     bool                `json:"from_cache"`
}

type MetricSeries struct {
	Name       string            `json:"name"`
	MetricType MetricType        `json:"metric_type"`
	Labels     map[string]string `json:"labels"`
	Points     []MetricPoint     `json:"points"`
	Statistics SeriesStatistics  `json:"statistics"`
}

type MetricPoint struct {
	Timestamp time.Time               `json:"timestamp"`
	Value     float64                 `json:"value"`
	Count     int64                   `json:"count"`
	Metadata  map[string]interface{}  `json:"metadata,omitempty"`
}

type SeriesStatistics struct {
	Sum       float64 `json:"sum"`
	Avg       float64 `json:"avg"`
	Min       float64 `json:"min"`
	Max       float64 `json:"max"`
	Count     int64   `json:"count"`
	StdDev    float64 `json:"std_dev"`
	Variance  float64 `json:"variance"`
	Median    float64 `json:"median"`
	P95       float64 `json:"p95"`
	P99       float64 `json:"p99"`
	LastValue float64 `json:"last_value"`
	FirstValue float64 `json:"first_value"`
	Growth    float64 `json:"growth"`
	Trend     string  `json:"trend"`
}

type ResultSummary struct {
	TotalDataPoints int64             `json:"total_data_points"`
	TotalSeries     int               `json:"total_series"`
	TimeRange       TimeRange         `json:"time_range"`
	Aggregations    map[string]float64 `json:"aggregations"`
	TopResults      []TopResult       `json:"top_results"`
	Comparisons     []PeriodComparison `json:"comparisons"`
}

type TopResult struct {
	Rank    int               `json:"rank"`
	Name    string            `json:"name"`
	Value   float64           `json:"value"`
	Labels  map[string]string `json:"labels"`
	Percent float64           `json:"percent"`
}

type PeriodComparison struct {
	Label         string  `json:"label"`
	CurrentValue  float64 `json:"current_value"`
	PreviousValue float64 `json:"previous_value"`
	Change        float64 `json:"change"`
	ChangePercent float64 `json:"change_percent"`
}

type ResultMetadata struct {
	QueryHash       string        `json:"query_hash"`
	DataSources     []string      `json:"data_sources"`
	PartitionsHit   int           `json:"partitions_hit"`
	RowsScanned     int64         `json:"rows_scanned"`
	RowsReturned    int64         `json:"rows_returned"`
	CacheHit        bool          `json:"cache_hit"`
	QueryOptimized  bool          `json:"query_optimized"`
	SamplingRate    float64       `json:"sampling_rate"`
	ExecutionPlan   string        `json:"execution_plan"`
}

type AnalyticsCache struct {
	cache map[string]*CachedResult
	mu    sync.RWMutex
	ttl   time.Duration
}

type CachedResult struct {
	Result    *AnalyticsResult `json:"result"`
	CachedAt  time.Time        `json:"cached_at"`
	ExpiresAt time.Time        `json:"expires_at"`
	HitCount  int              `json:"hit_count"`
}

type Engine struct {
	db           *gorm.DB
	cache        *AnalyticsCache
	dataSources  map[string]DataSource
	aggregators  map[AggregationType]Aggregator
}

type DataSource interface {
	Query(ctx context.Context, query *AnalyticsQuery) (*AnalyticsResult, error)
	Name() string
	IsAvailable() bool
}

type Aggregator interface {
	Aggregate(values []float64) float64
	Type() AggregationType
}

func NewEngine(db *gorm.DB) *Engine {
	engine := &Engine{
		db: db,
		cache: &AnalyticsCache{
			cache: make(map[string]*CachedResult),
			ttl:   5 * time.Minute,
		},
		dataSources: make(map[string]DataSource),
		aggregators: make(map[AggregationType]Aggregator),
	}

	engine.initializeAggregators()

	return engine
}

func (e *Engine) initializeAggregators() {
	e.aggregators[AggregationSum] = &SumAggregator{}
	e.aggregators[AggregationAvg] = &AvgAggregator{}
	e.aggregators[AggregationMin] = &MinAggregator{}
	e.aggregators[AggregationMax] = &MaxAggregator{}
	e.aggregators[AggregationCount] = &CountAggregator{}
	e.aggregators[AggregationP95] = &PercentileAggregator{percentile: 0.95}
	e.aggregators[AggregationP99] = &PercentileAggregator{percentile: 0.99}
}

func (e *Engine) Query(ctx context.Context, query *AnalyticsQuery) (*AnalyticsResult, error) {
	if err := e.validateQuery(query); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrQueryFailed, err)
	}

	cacheKey := e.generateCacheKey(query)
	if cached := e.getCachedResult(cacheKey); cached != nil {
		cached.HitCount++
		cached.Result.FromCache = true
		return cached.Result, nil
	}

	startTime := time.Now()

	result := &AnalyticsResult{
		QueryID:     query.ID,
		ExecutedAt:  startTime,
		TimeRange:   query.TimeRange,
		Granularity: query.Granularity,
		Series:      make([]MetricSeries, 0),
		Metadata:    ResultMetadata{},
	}

	dataPoints, err := e.fetchDataPoints(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to fetch data points: %v", ErrQueryFailed, err)
	}

	if len(dataPoints) == 0 {
		return result, nil
	}

	groupedData := e.groupDataPoints(dataPoints, query.GroupBy)

	for groupKey, points := range groupedData {
		series := e.buildSeries(groupKey, points, query)
		result.Series = append(result.Series, series)
	}

	result.Summary = e.buildSummary(result.Series, query)
	result.Metadata = e.buildMetadata(query, dataPoints)
	result.ExecutionTime = time.Since(startTime)

	e.cacheResult(cacheKey, result)

	return result, nil
}

func (e *Engine) validateQuery(query *AnalyticsQuery) error {
	if query.TimeRange.StartTime.IsZero() || query.TimeRange.EndTime.IsZero() {
		return ErrInvalidTimeRange
	}

	if query.TimeRange.StartTime.After(query.TimeRange.EndTime) {
		return ErrInvalidTimeRange
	}

	if !isValidGranularity(query.Granularity) {
		return ErrInvalidGranularity
	}

	if len(query.Aggregations) == 0 {
		query.Aggregations = []AggregationType{AggregationAvg}
	}

	return nil
}

func (e *Engine) fetchDataPoints(ctx context.Context, query *AnalyticsQuery) ([]MetricPoint, error) {
	var rawData []struct {
		Timestamp time.Time
		Value     float64
		Labels    map[string]string
		Count     int64
	}

	dbQuery := e.db.WithContext(ctx).Table("analytics_metrics").
		Select("timestamp, value, labels, count").
		Where("tenant_id = ?", query.TenantID).
		Where("metric_type = ?", query.MetricType).
		Where("timestamp >= ? AND timestamp <= ?", query.TimeRange.StartTime, query.TimeRange.EndTime)

	for _, filter := range query.Filters {
		dbQuery = e.applyFilter(dbQuery, filter)
	}

	if err := dbQuery.Find(&rawData).Error; err != nil {
		return nil, err
	}

	points := make([]MetricPoint, len(rawData))
	for i, data := range rawData {
		points[i] = MetricPoint{
			Timestamp: data.Timestamp,
			Value:     data.Value,
			Count:     data.Count,
			Metadata:  data.Labels,
		}
	}

	return points, nil
}

func (e *Engine) applyFilter(dbQuery *gorm.DB, filter QueryFilter) *gorm.DB {
	switch filter.Operator {
	case "=":
		return dbQuery.Where("labels->>? = ?", filter.Field, filter.Value)
	case "!=":
		return dbQuery.Where("labels->>? != ?", filter.Field, filter.Value)
	case ">":
		return dbQuery.Where("(labels->>?)::float > ?", filter.Field, filter.Value)
	case "<":
		return dbQuery.Where("(labels->>?)::float < ?", filter.Field, filter.Value)
	case ">=":
		return dbQuery.Where("(labels->>?)::float >= ?", filter.Field, filter.Value)
	case "<=":
		return dbQuery.Where("(labels->>?)::float <= ?", filter.Field, filter.Value)
	case "in":
		return dbQuery.Where("labels->>? IN ?", filter.Field, filter.Value)
	case "like":
		return dbQuery.Where("labels->>? LIKE ?", filter.Field, filter.Value)
	default:
		return dbQuery
	}
}

func (e *Engine) groupDataPoints(points []MetricPoint, groupBy []string) map[string][]MetricPoint {
	grouped := make(map[string][]MetricPoint)

	for _, point := range points {
		key := e.generateGroupKey(point, groupBy)
		grouped[key] = append(grouped[key], point)
	}

	return grouped
}

func (e *Engine) generateGroupKey(point MetricPoint, groupBy []string) string {
	if len(groupBy) == 0 {
		return "default"
	}

	key := ""
	for _, field := range groupBy {
		if val, ok := point.Metadata[field]; ok {
			key += fmt.Sprintf("%v|", val)
		}
	}

	if key == "" {
		return "default"
	}

	return key
}

func (e *Engine) buildSeries(groupKey string, points []MetricPoint, query *AnalyticsQuery) MetricSeries {
	aggregatedPoints := e.aggregatePoints(points, query.Granularity, query.Aggregations)

	statistics := e.calculateStatistics(aggregatedPoints)

	labels := make(map[string]string)
	if len(points) > 0 && points[0].Metadata != nil {
		for k, v := range points[0].Metadata {
			if strVal, ok := v.(string); ok {
				labels[k] = strVal
			}
		}
	}

	return MetricSeries{
		Name:       groupKey,
		MetricType: query.MetricType,
		Labels:     labels,
		Points:     aggregatedPoints,
		Statistics: statistics,
	}
}

func (e *Engine) aggregatePoints(points []MetricPoint, granularity Granularity, aggregations []AggregationType) []MetricPoint {
	buckets := e.bucketByGranularity(points, granularity)

	result := make([]MetricPoint, 0, len(buckets))

	for timestamp, bucket := range buckets {
		values := make([]float64, len(bucket))
		for i, p := range bucket {
			values[i] = p.Value
		}

		aggregatedValue := 0.0
		for _, aggType := range aggregations {
			if aggregator, exists := e.aggregators[aggType]; exists {
				aggregatedValue = aggregator.Aggregate(values)
				break
			}
		}

		result = append(result, MetricPoint{
			Timestamp: timestamp,
			Value:     aggregatedValue,
			Count:     int64(len(bucket)),
		})
	}

	return e.sortPointsByTime(result)
}

func (e *Engine) bucketByGranularity(points []MetricPoint, granularity Granularity) map[time.Time][]MetricPoint {
	buckets := make(map[time.Time][]MetricPoint)

	for _, point := range points {
		bucketTime := e.truncateToGranularity(point.Timestamp, granularity)
		buckets[bucketTime] = append(buckets[bucketTime], point)
	}

	return buckets
}

func (e *Engine) truncateToGranularity(t time.Time, granularity Granularity) time.Time {
	switch granularity {
	case GranularityMinute:
		return t.Truncate(time.Minute)
	case GranularityHour:
		return t.Truncate(time.Hour)
	case GranularityDay:
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	case GranularityWeek:
		weekday := t.Weekday()
		return time.Date(t.Year(), t.Month(), t.Day()-int(weekday), 0, 0, 0, 0, t.Location())
	case GranularityMonth:
		return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
	case GranularityQuarter:
		quarter := (int(t.Month()) - 1) / 3
		return time.Date(t.Year(), time.Month(quarter*3+1), 1, 0, 0, 0, 0, t.Location())
	case GranularityYear:
		return time.Date(t.Year(), 1, 1, 0, 0, 0, 0, t.Location())
	default:
		return t.Truncate(time.Hour)
	}
}

func (e *Engine) sortPointsByTime(points []MetricPoint) []MetricPoint {
	sorted := make([]MetricPoint, len(points))
	copy(sorted, points)

	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i].Timestamp.After(sorted[j].Timestamp) {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	return sorted
}

func (e *Engine) calculateStatistics(points []MetricPoint) SeriesStatistics {
	if len(points) == 0 {
		return SeriesStatistics{}
	}

	values := make([]float64, len(points))
	for i, p := range points {
		values[i] = p.Value
	}

	stats := SeriesStatistics{
		Count:     int64(len(values)),
		FirstValue: values[0],
		LastValue: values[len(values)-1],
	}

	stats.Sum = e.sum(values)
	stats.Avg = stats.Sum / float64(stats.Count)
	stats.Min = e.min(values)
	stats.Max = e.max(values)
	stats.Variance = e.variance(values, stats.Avg)
	stats.StdDev = math.Sqrt(stats.Variance)
	stats.Median = e.percentile(values, 0.50)
	stats.P95 = e.percentile(values, 0.95)
	stats.P99 = e.percentile(values, 0.99)

	if stats.FirstValue != 0 {
		stats.Growth = ((stats.LastValue - stats.FirstValue) / stats.FirstValue) * 100
	}

	if stats.Growth > 5 {
		stats.Trend = "increasing"
	} else if stats.Growth < -5 {
		stats.Trend = "decreasing"
	} else {
		stats.Trend = "stable"
	}

	return stats
}

func (e *Engine) sum(values []float64) float64 {
	var sum float64
	for _, v := range values {
		sum += v
	}
	return sum
}

func (e *Engine) min(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	min := values[0]
	for _, v := range values {
		if v < min {
			min = v
		}
	}
	return min
}

func (e *Engine) max(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	max := values[0]
	for _, v := range values {
		if v > max {
			max = v
		}
	}
	return max
}

func (e *Engine) variance(values []float64, mean float64) float64 {
	if len(values) == 0 {
		return 0
	}
	var variance float64
	for _, v := range values {
		variance += math.Pow(v-mean, 2)
	}
	return variance / float64(len(values))
}

func (e *Engine) percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}

	sorted := make([]float64, len(values))
	copy(sorted, values)

	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i] > sorted[j] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	index := p * float64(len(sorted)-1)
	lower := int(math.Floor(index))
	upper := int(math.Ceil(index))

	if lower == upper {
		return sorted[lower]
	}

	return sorted[lower] + (sorted[upper]-sorted[lower])*(index-float64(lower))
}

func (e *Engine) buildSummary(series []MetricSeries, query *AnalyticsQuery) ResultSummary {
	summary := ResultSummary{
		TotalSeries:  len(series),
		TimeRange:    query.TimeRange,
		Aggregations: make(map[string]float64),
		TopResults:   make([]TopResult, 0),
		Comparisons:  make([]PeriodComparison, 0),
	}

	totalPoints := int64(0)
	for _, s := range series {
		totalPoints += int64(len(s.Points))
	}
	summary.TotalDataPoints = totalPoints

	for _, aggType := range query.Aggregations {
		aggValues := make([]float64, 0)
		for _, s := range series {
			switch aggType {
			case AggregationSum:
				aggValues = append(aggValues, s.Statistics.Sum)
			case AggregationAvg:
				aggValues = append(aggValues, s.Statistics.Avg)
			case AggregationMax:
				aggValues = append(aggValues, s.Statistics.Max)
			case AggregationMin:
				aggValues = append(aggValues, s.Statistics.Min)
			}
		}

		if len(aggValues) > 0 {
			if aggregator, exists := e.aggregators[aggType]; exists {
				summary.Aggregations[string(aggType)] = aggregator.Aggregate(aggValues)
			}
		}
	}

	return summary
}

func (e *Engine) buildMetadata(query *AnalyticsQuery, points []MetricPoint) ResultMetadata {
	return ResultMetadata{
		QueryHash:      e.generateCacheKey(query),
		DataSources:    []string{"primary"},
		RowsScanned:    int64(len(points)),
		RowsReturned:   int64(len(points)),
		CacheHit:       false,
		QueryOptimized: true,
		SamplingRate:   1.0,
	}
}

func (e *Engine) generateCacheKey(query *AnalyticsQuery) string {
	return fmt.Sprintf("%s-%s-%s-%s-%v",
		query.TenantID,
		query.MetricType,
		query.TimeRange.StartTime.Format(time.RFC3339),
		query.TimeRange.EndTime.Format(time.RFC3339),
		query.Granularity,
	)
}

func (e *Engine) getCachedResult(key string) *CachedResult {
	e.cache.mu.RLock()
	defer e.cache.mu.RUnlock()

	cached, exists := e.cache.cache[key]
	if !exists {
		return nil
	}

	if time.Now().After(cached.ExpiresAt) {
		return nil
	}

	return cached
}

func (e *Engine) cacheResult(key string, result *AnalyticsResult) {
	e.cache.mu.Lock()
	defer e.cache.mu.Unlock()

	e.cache.cache[key] = &CachedResult{
		Result:    result,
		CachedAt:  time.Now(),
		ExpiresAt: time.Now().Add(e.cache.ttl),
		HitCount:  0,
	}
}

func (e *Engine) ClearCache() {
	e.cache.mu.Lock()
	defer e.cache.mu.Unlock()
	e.cache.cache = make(map[string]*CachedResult)
}

func (e *Engine) GetCacheStats() map[string]interface{} {
	e.cache.mu.RLock()
	defer e.cache.mu.RUnlock()

	totalHits := 0
	for _, cached := range e.cache.cache {
		totalHits += cached.HitCount
	}

	return map[string]interface{}{
		"total_entries": len(e.cache.cache),
		"total_hits":    totalHits,
		"cache_ttl":     e.cache.ttl.String(),
	}
}

func (e *Engine) RegisterDataSource(name string, source DataSource) {
	e.dataSources[name] = source
}

func (e *Engine) SetCacheTTL(ttl time.Duration) {
	e.cache.mu.Lock()
	defer e.cache.mu.Unlock()
	e.cache.ttl = ttl
}

type SumAggregator struct{}

func (a *SumAggregator) Aggregate(values []float64) float64 {
	var sum float64
	for _, v := range values {
		sum += v
	}
	return sum
}

func (a *SumAggregator) Type() AggregationType {
	return AggregationSum
}

type AvgAggregator struct{}

func (a *AvgAggregator) Aggregate(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	var sum float64
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

func (a *AvgAggregator) Type() AggregationType {
	return AggregationAvg
}

type MinAggregator struct{}

func (a *MinAggregator) Aggregate(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	min := values[0]
	for _, v := range values {
		if v < min {
			min = v
		}
	}
	return min
}

func (a *MinAggregator) Type() AggregationType {
	return AggregationMin
}

type MaxAggregator struct{}

func (a *MaxAggregator) Aggregate(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	max := values[0]
	for _, v := range values {
		if v > max {
			max = v
		}
	}
	return max
}

func (a *MaxAggregator) Type() AggregationType {
	return AggregationMax
}

type CountAggregator struct{}

func (a *CountAggregator) Aggregate(values []float64) float64 {
	return float64(len(values))
}

func (a *CountAggregator) Type() AggregationType {
	return AggregationCount
}

type PercentileAggregator struct {
	percentile float64
}

func (a *PercentileAggregator) Aggregate(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	sorted := make([]float64, len(values))
	copy(sorted, values)

	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i] > sorted[j] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	index := a.percentile * float64(len(sorted)-1)
	lower := int(math.Floor(index))
	upper := int(math.Ceil(index))

	if lower == upper {
		return sorted[lower]
	}

	return sorted[lower] + (sorted[upper]-sorted[lower])*(index-float64(lower))
}

func (a *PercentileAggregator) Type() AggregationType {
	if a.percentile == 0.95 {
		return AggregationP95
	}
	return AggregationP99
}

func isValidGranularity(granularity Granularity) bool {
	switch granularity {
	case GranularityMinute, GranularityHour, GranularityDay, GranularityWeek, GranularityMonth, GranularityQuarter, GranularityYear:
		return true
	default:
		return false
	}
}
