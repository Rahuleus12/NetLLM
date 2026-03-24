package analytics

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
)

var (
	ErrTrendNotFound       = errors.New("trend not found")
	ErrInsufficientTrendData = errors.New("insufficient data for trend analysis")
	ErrInvalidTrendMethod  = errors.New("invalid trend detection method")
)

type TrendDirection string

const (
	TrendDirectionUp       TrendDirection = "up"
	TrendDirectionDown     TrendDirection = "down"
	TrendDirectionStable   TrendDirection = "stable"
	TrendDirectionVolatile TrendDirection = "volatile"
)

type TrendMethod string

const (
	TrendMethodLinear      TrendMethod = "linear"
	TrendMethodExponential TrendMethod = "exponential"
	TrendMethodPolynomial  TrendMethod = "polynomial"
	TrendMethodMovingAvg   TrendMethod = "moving_average"
	TrendMethodSeasonal    TrendMethod = "seasonal"
)

type TrendPattern string

const (
	PatternSeasonal     TrendPattern = "seasonal"
	PatternCyclical     TrendPattern = "cyclical"
	PatternSpike        TrendPattern = "spike"
	PatternDrop         TrendPattern = "drop"
	PatternPlateau      TrendPattern = "plateau"
	PatternStepChange   TrendPattern = "step_change"
	PatternOscillation  TrendPattern = "oscillation"
)

type TrendAnalysis struct {
	ID              uuid.UUID           `json:"id"`
	TenantID        uuid.UUID           `json:"tenant_id"`
	MetricType      MetricType          `json:"metric_type"`
	TimeRange       TimeRange           `json:"time_range"`
	Method          TrendMethod         `json:"method"`
	Direction       TrendDirection      `json:"direction"`
	Strength        float64             `json:"strength"` // 0-1
	Confidence      float64             `json:"confidence"` // 0-1
	Slope           float64             `json:"slope"`
	Intercept       float64             `json:"intercept"`
	R2Score         float64             `json:"r2_score"`
	Patterns        []DetectedPattern   `json:"patterns"`
	ChangePoints    []ChangePoint       `json:"change_points"`
	Seasonality     *SeasonalityInfo    `json:"seasonality,omitempty"`
	Forecast        []TrendForecast     `json:"forecast"`
	Metadata        TrendMetadata       `json:"metadata"`
	CreatedAt       time.Time           `json:"created_at" gorm:"autoCreateTime"`
}

type DetectedPattern struct {
	Type        TrendPattern `json:"type"`
	StartTime   time.Time    `json:"start_time"`
	EndTime     time.Time    `json:"end_time"`
	Strength    float64      `json:"strength"`
	Description string       `json:"description"`
	Magnitude   float64      `json:"magnitude"`
}

type ChangePoint struct {
	Timestamp   time.Time `json:"timestamp"`
	Type        string    `json:"type"` // "increase", "decrease", "level_shift"
	Magnitude   float64   `json:"magnitude"`
	Confidence  float64   `json:"confidence"`
	Description string    `json:"description"`
}

type SeasonalityInfo struct {
	HasSeasonality bool            `json:"has_seasonality"`
	Period         time.Duration   `json:"period"`
	Strength       float64         `json:"strength"`
	Peaks          []time.Time     `json:"peaks"`
	Valleys        []time.Time     `json:"valleys"`
	Pattern        string          `json:"pattern"` // "daily", "weekly", "monthly", "yearly"
}

type TrendForecast struct {
	Timestamp   time.Time `json:"timestamp"`
	Value       float64   `json:"value"`
	LowerBound  float64   `json:"lower_bound"`
	UpperBound  float64   `json:"upper_bound"`
	Confidence  float64   `json:"confidence"`
}

type TrendMetadata struct {
	DataPoints     int64   `json:"data_points"`
	TimeSpan       string  `json:"time_span"`
	AverageValue   float64 `json:"average_value"`
	Variance       float64 `json:"variance"`
	CoefficientVar float64 `json:"coefficient_of_variation"`
}

type TrendRequest struct {
	TenantID   uuid.UUID  `json:"tenant_id" binding:"required"`
	MetricType MetricType `json:"metric_type" binding:"required"`
	TimeRange  TimeRange  `json:"time_range" binding:"required"`
	Method     TrendMethod `json:"method"`
	ForecastPeriods int   `json:"forecast_periods"`
}

type TrendAnalyzer struct {
	engine *Engine
}

func NewTrendAnalyzer(engine *Engine) *TrendAnalyzer {
	return &TrendAnalyzer{engine: engine}
}

func (ta *TrendAnalyzer) AnalyzeTrend(ctx context.Context, req *TrendRequest) (*TrendAnalysis, error) {
	if err := ta.validateRequest(req); err != nil {
		return nil, err
	}

	query := &AnalyticsQuery{
		TenantID:   req.TenantID,
		MetricType: req.MetricType,
		TimeRange:  req.TimeRange,
		Granularity: ta.determineGranularity(req.TimeRange),
		Aggregations: []AggregationType{AggregationAvg},
	}

	result, err := ta.engine.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query data for trend analysis: %w", err)
	}

	if len(result.Series) == 0 || len(result.Series[0].Points) < 3 {
		return nil, ErrInsufficientTrendData
	}

	points := result.Series[0].Points

	analysis := &TrendAnalysis{
		ID:         uuid.New(),
		TenantID:   req.TenantID,
		MetricType: req.MetricType,
		TimeRange:  req.TimeRange,
		Method:     req.Method,
		Patterns:   make([]DetectedPattern, 0),
		ChangePoints: make([]ChangePoint, 0),
		Forecast:   make([]TrendForecast, 0),
		CreatedAt:  time.Now(),
	}

	if analysis.Method == "" {
		analysis.Method = TrendMethodLinear
	}

	values := ta.extractValues(points)
	timestamps := ta.extractTimestamps(points)

	analysis.Direction = ta.detectTrendDirection(values)
	analysis.Slope, analysis.Intercept = ta.calculateLinearRegression(timestamps, values)
	analysis.R2Score = ta.calculateR2Score(timestamps, values, analysis.Slope, analysis.Intercept)
	analysis.Strength = ta.calculateTrendStrength(analysis.R2Score)
	analysis.Confidence = ta.calculateConfidence(len(points), analysis.R2Score)

	analysis.Patterns = ta.detectPatterns(points, values)
	analysis.ChangePoints = ta.detectChangePoints(points, values)
	analysis.Seasonality = ta.detectSeasonality(points, values)
	analysis.Metadata = ta.buildMetadata(points, values)

	if req.ForecastPeriods > 0 {
		analysis.Forecast = ta.generateForecast(timestamps, values, analysis.Slope, analysis.Intercept, req.ForecastPeriods)
	}

	return analysis, nil
}

func (ta *TrendAnalyzer) validateRequest(req *TrendRequest) error {
	if req.TenantID.IsZero() {
		return fmt.Errorf("tenant_id is required")
	}
	if req.TimeRange.StartTime.IsZero() || req.TimeRange.EndTime.IsZero() {
		return ErrInvalidTimeRange
	}
	if req.TimeRange.StartTime.After(req.TimeRange.EndTime) {
		return ErrInvalidTimeRange
	}
	return nil
}

func (ta *TrendAnalyzer) determineGranularity(timeRange TimeRange) Granularity {
	duration := timeRange.EndTime.Sub(timeRange.StartTime)

	switch {
	case duration <= time.Hour:
		return GranularityMinute
	case duration <= 24*time.Hour:
		return GranularityHour
	case duration <= 7*24*time.Hour:
		return GranularityHour
	case duration <= 30*24*time.Hour:
		return GranularityDay
	case duration <= 365*24*time.Hour:
		return GranularityDay
	default:
		return GranularityWeek
	}
}

func (ta *TrendAnalyzer) extractValues(points []MetricPoint) []float64 {
	values := make([]float64, len(points))
	for i, p := range points {
		values[i] = p.Value
	}
	return values
}

func (ta *TrendAnalyzer) extractTimestamps(points []MetricPoint) []float64 {
	timestamps := make([]float64, len(points))
	if len(points) > 0 {
		baseTime := points[0].Timestamp.Unix()
		for i, p := range points {
			timestamps[i] = float64(p.Timestamp.Unix() - baseTime)
		}
	}
	return timestamps
}

func (ta *TrendAnalyzer) detectTrendDirection(values []float64) TrendDirection {
	if len(values) < 2 {
		return TrendDirectionStable
	}

	increasing := 0
	decreasing := 0

	for i := 1; i < len(values); i++ {
		if values[i] > values[i-1] {
			increasing++
		} else if values[i] < values[i-1] {
			decreasing++
		}
	}

	total := float64(len(values) - 1)
	incRatio := float64(increasing) / total
	decRatio := float64(decreasing) / total

	if incRatio > 0.6 {
		return TrendDirectionUp
	} else if decRatio > 0.6 {
		return TrendDirectionDown
	} else if incRatio > 0.4 && decRatio > 0.4 {
		return TrendDirectionVolatile
	}

	return TrendDirectionStable
}

func (ta *TrendAnalyzer) calculateLinearRegression(x, y []float64) (slope, intercept float64) {
	if len(x) != len(y) || len(x) == 0 {
		return 0, 0
	}

	n := float64(len(x))
	sumX := 0.0
	sumY := 0.0
	sumXY := 0.0
	sumX2 := 0.0

	for i := 0; i < len(x); i++ {
		sumX += x[i]
		sumY += y[i]
		sumXY += x[i] * y[i]
		sumX2 += x[i] * x[i]
	}

	denominator := n*sumX2 - sumX*sumX
	if denominator == 0 {
		return 0, sumY / n
	}

	slope = (n*sumXY - sumX*sumY) / denominator
	intercept = (sumY - slope*sumX) / n

	return slope, intercept
}

func (ta *TrendAnalyzer) calculateR2Score(x, y []float64, slope, intercept float64) float64 {
	if len(x) == 0 {
		return 0
	}

	yMean := 0.0
	for _, val := range y {
		yMean += val
	}
	yMean /= float64(len(y))

	ssTotal := 0.0
	ssResidual := 0.0

	for i := 0; i < len(x); i++ {
		predicted := slope*x[i] + intercept
		ssTotal += math.Pow(y[i]-yMean, 2)
		ssResidual += math.Pow(y[i]-predicted, 2)
	}

	if ssTotal == 0 {
		return 1.0
	}

	return 1.0 - (ssResidual / ssTotal)
}

func (ta *TrendAnalyzer) calculateTrendStrength(r2Score float64) float64 {
	return math.Max(0, math.Min(1, r2Score))
}

func (ta *TrendAnalyzer) calculateConfidence(dataPoints int, r2Score float64) float64 {
	pointsScore := math.Min(1.0, float64(dataPoints)/30.0)
	return (pointsScore * 0.4) + (r2Score * 0.6)
}

func (ta *TrendAnalyzer) detectPatterns(points []MetricPoint, values []float64) []DetectedPattern {
	patterns := make([]DetectedPattern, 0)

	if len(values) < 5 {
		return patterns
	}

	if pattern := ta.detectSpikePattern(points, values); pattern != nil {
		patterns = append(patterns, *pattern)
	}

	if pattern := ta.detectDropPattern(points, values); pattern != nil {
		patterns = append(patterns, *pattern)
	}

	if pattern := ta.detectPlateauPattern(points, values); pattern != nil {
		patterns = append(patterns, *pattern)
	}

	if pattern := ta.detectStepChangePattern(points, values); pattern != nil {
		patterns = append(patterns, *pattern)
	}

	if pattern := ta.detectOscillationPattern(points, values); pattern != nil {
		patterns = append(patterns, *pattern)
	}

	return patterns
}

func (ta *TrendAnalyzer) detectSpikePattern(points []MetricPoint, values []float64) *DetectedPattern {
	mean := ta.calculateMean(values)
	stdDev := ta.calculateStdDev(values, mean)

	for i := 1; i < len(values)-1; i++ {
		if values[i] > mean+2*stdDev {
			if values[i] > values[i-1] && values[i] > values[i+1] {
				return &DetectedPattern{
					Type:        PatternSpike,
					StartTime:   points[i].Timestamp,
					EndTime:     points[i].Timestamp,
					Strength:    (values[i] - mean) / stdDev,
					Description: fmt.Sprintf("Spike detected at %s with value %.2f", points[i].Timestamp.Format(time.RFC3339), values[i]),
					Magnitude:   values[i] - mean,
				}
			}
		}
	}

	return nil
}

func (ta *TrendAnalyzer) detectDropPattern(points []MetricPoint, values []float64) *DetectedPattern {
	mean := ta.calculateMean(values)
	stdDev := ta.calculateStdDev(values, mean)

	for i := 1; i < len(values)-1; i++ {
		if values[i] < mean-2*stdDev {
			if values[i] < values[i-1] && values[i] < values[i+1] {
				return &DetectedPattern{
					Type:        PatternDrop,
					StartTime:   points[i].Timestamp,
					EndTime:     points[i].Timestamp,
					Strength:    (mean - values[i]) / stdDev,
					Description: fmt.Sprintf("Drop detected at %s with value %.2f", points[i].Timestamp.Format(time.RFC3339), values[i]),
					Magnitude:   mean - values[i],
				}
			}
		}
	}

	return nil
}

func (ta *TrendAnalyzer) detectPlateauPattern(points []MetricPoint, values []float64) *DetectedPattern {
	if len(values) < 5 {
		return nil
	}

	variance := ta.calculateVariance(values)
	mean := ta.calculateMean(values)

	if variance < 0.01*mean {
		return &DetectedPattern{
			Type:        PatternPlateau,
			StartTime:   points[0].Timestamp,
			EndTime:     points[len(points)-1].Timestamp,
			Strength:    1.0 - (variance / mean),
			Description: "Plateau detected with stable values",
			Magnitude:   0,
		}
	}

	return nil
}

func (ta *TrendAnalyzer) detectStepChangePattern(points []MetricPoint, values []float64) *DetectedPattern {
	if len(values) < 10 {
		return nil
	}

	midPoint := len(values) / 2
	firstHalf := values[:midPoint]
	secondHalf := values[midPoint:]

	firstMean := ta.calculateMean(firstHalf)
	secondMean := ta.calculateMean(secondHalf)

	overallMean := ta.calculateMean(values)
	threshold := overallMean * 0.2

	if math.Abs(secondMean-firstMean) > threshold {
		return &DetectedPattern{
			Type:        PatternStepChange,
			StartTime:   points[midPoint-1].Timestamp,
			EndTime:     points[midPoint].Timestamp,
			Strength:    math.Abs(secondMean-firstMean) / overallMean,
			Description: fmt.Sprintf("Step change detected from %.2f to %.2f", firstMean, secondMean),
			Magnitude:   secondMean - firstMean,
		}
	}

	return nil
}

func (ta *TrendAnalyzer) detectOscillationPattern(points []MetricPoint, values []float64) *DetectedPattern {
	if len(values) < 6 {
		return nil
	}

	oscillations := 0
	for i := 1; i < len(values)-1; i++ {
		if (values[i] > values[i-1] && values[i] > values[i+1]) ||
			(values[i] < values[i-1] && values[i] < values[i+1]) {
			oscillations++
		}
	}

	oscillationRatio := float64(oscillations) / float64(len(values)-2)
	if oscillationRatio > 0.6 {
		return &DetectedPattern{
			Type:        PatternOscillation,
			StartTime:   points[0].Timestamp,
			EndTime:     points[len(points)-1].Timestamp,
			Strength:    oscillationRatio,
			Description: fmt.Sprintf("Oscillation pattern detected with %.1f%% oscillation rate", oscillationRatio*100),
			Magnitude:   ta.calculateStdDev(values, ta.calculateMean(values)),
		}
	}

	return nil
}

func (ta *TrendAnalyzer) detectChangePoints(points []MetricPoint, values []float64) []ChangePoint {
	changePoints := make([]ChangePoint, 0)

	if len(values) < 10 {
		return changePoints
	}

	windowSize := 5
	for i := windowSize; i < len(values)-windowSize; i++ {
		beforeWindow := values[i-windowSize : i]
		afterWindow := values[i : i+windowSize]

		beforeMean := ta.calculateMean(beforeWindow)
		afterMean := ta.calculateMean(afterWindow)

		changeMagnitude := math.Abs(afterMean - beforeMean)
		threshold := beforeMean * 0.15

		if changeMagnitude > threshold {
			changeType := "increase"
			if afterMean < beforeMean {
				changeType = "decrease"
			}

			changePoints = append(changePoints, ChangePoint{
				Timestamp:   points[i].Timestamp,
				Type:        changeType,
				Magnitude:   changeMagnitude,
				Confidence:  math.Min(1.0, changeMagnitude/threshold),
				Description: fmt.Sprintf("%s of %.2f detected", changeType, changeMagnitude),
			})
		}
	}

	return changePoints
}

func (ta *TrendAnalyzer) detectSeasonality(points []MetricPoint, values []float64) *SeasonalityInfo {
	if len(values) < 24 {
		return nil
	}

	autocorr := ta.calculateAutocorrelation(values, 24)

	if autocorr > 0.7 {
		peaks := ta.findPeaks(points, values)
		valleys := ta.findValleys(points, values)

		period := time.Duration(24) * time.Hour
		if len(points) > 1 {
			period = points[1].Timestamp.Sub(points[0].Timestamp) * 24
		}

		return &SeasonalityInfo{
			HasSeasonality: true,
			Period:         period,
			Strength:       autocorr,
			Peaks:          peaks,
			Valleys:        valleys,
			Pattern:        "daily",
		}
	}

	return nil
}

func (ta *TrendAnalyzer) calculateAutocorrelation(values []float64, lag int) float64 {
	if len(values) <= lag {
		return 0
	}

	mean := ta.calculateMean(values)
	variance := ta.calculateVariance(values)

	if variance == 0 {
		return 0
	}

	sum := 0.0
	for i := 0; i < len(values)-lag; i++ {
		sum += (values[i] - mean) * (values[i+lag] - mean)
	}

	return sum / (float64(len(values)-lag) * variance)
}

func (ta *TrendAnalyzer) findPeaks(points []MetricPoint, values []float64) []time.Time {
	peaks := make([]time.Time, 0)
	mean := ta.calculateMean(values)

	for i := 1; i < len(values)-1; i++ {
		if values[i] > values[i-1] && values[i] > values[i+1] && values[i] > mean {
			peaks = append(peaks, points[i].Timestamp)
		}
	}

	return peaks
}

func (ta *TrendAnalyzer) findValleys(points []MetricPoint, values []float64) []time.Time {
	valleys := make([]time.Time, 0)
	mean := ta.calculateMean(values)

	for i := 1; i < len(values)-1; i++ {
		if values[i] < values[i-1] && values[i] < values[i+1] && values[i] < mean {
			valleys = append(valleys, points[i].Timestamp)
		}
	}

	return valleys
}

func (ta *TrendAnalyzer) generateForecast(timestamps, values []float64, slope, intercept float64, periods int) []TrendForecast {
	forecasts := make([]TrendForecast, periods)

	if len(timestamps) == 0 {
		return forecasts
	}

	lastTimestamp := timestamps[len(timestamps)-1]
	interval := 3600.0

	if len(timestamps) > 1 {
		interval = timestamps[len(timestamps)-1] - timestamps[len(timestamps)-2]
	}

	stdDev := ta.calculateStdDev(values, ta.calculateMean(values))

	for i := 0; i < periods; i++ {
		futureTimestamp := lastTimestamp + float64(i+1)*interval
		predictedValue := slope*futureTimestamp + intercept

		uncertainty := stdDev * math.Sqrt(1.0 + float64(i+1)*0.1)

		forecasts[i] = TrendForecast{
			Timestamp:   time.Now().Add(time.Duration((i+1)*int(interval)) * time.Second),
			Value:       predictedValue,
			LowerBound:  predictedValue - 1.96*uncertainty,
			UpperBound:  predictedValue + 1.96*uncertainty,
			Confidence:  math.Max(0.1, 1.0-float64(i+1)*0.05),
		}
	}

	return forecasts
}

func (ta *TrendAnalyzer) buildMetadata(points []MetricPoint, values []float64) TrendMetadata {
	if len(values) == 0 {
		return TrendMetadata{}
	}

	mean := ta.calculateMean(values)
	variance := ta.calculateVariance(values)
	stdDev := math.Sqrt(variance)

	timeSpan := ""
	if len(points) > 0 {
		duration := points[len(points)-1].Timestamp.Sub(points[0].Timestamp)
		timeSpan = duration.String()
	}

	coefVar := 0.0
	if mean != 0 {
		coefVar = (stdDev / mean) * 100
	}

	return TrendMetadata{
		DataPoints:     int64(len(points)),
		TimeSpan:       timeSpan,
		AverageValue:   mean,
		Variance:       variance,
		CoefficientVar: coefVar,
	}
}

func (ta *TrendAnalyzer) calculateMean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

func (ta *TrendAnalyzer) calculateStdDev(values []float64, mean float64) float64 {
	return math.Sqrt(ta.calculateVariance(values))
}

func (ta *TrendAnalyzer) calculateVariance(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	mean := ta.calculateMean(values)
	variance := 0.0
	for _, v := range values {
		variance += math.Pow(v-mean, 2)
	}
	return variance / float64(len(values))
}
