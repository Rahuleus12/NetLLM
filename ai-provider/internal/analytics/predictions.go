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
	ErrPredictionNotFound     = errors.New("prediction not found")
	ErrInsufficientPredictionData = errors.New("insufficient data for prediction")
	ErrInvalidPredictionMethod = errors.New("invalid prediction method")
	ErrPredictionHorizonTooFar = errors.New("prediction horizon too far")
)

type PredictionMethod string

const (
	PredictionMethodLinear          PredictionMethod = "linear"
	PredictionMethodExponentialSmoothing PredictionMethod = "exponential_smoothing"
	PredictionMethodMovingAverage   PredictionMethod = "moving_average"
	PredictionMethodARIMA          PredictionMethod = "arima"
	PredictionMethodProphet        PredictionMethod = "prophet"
	PredictionMethodNeuralNetwork  PredictionMethod = "neural_network"
)

type PredictionType string

const (
	PredictionTypePoint    PredictionType = "point"
	PredictionTypeInterval PredictionType = "interval"
	PredictionTypeDistribution PredictionType = "distribution"
)

type Prediction struct {
	ID             uuid.UUID       `json:"id"`
	TenantID       uuid.UUID       `json:"tenant_id"`
	MetricType     MetricType      `json:"metric_type"`
	Method         PredictionMethod `json:"method"`
	Type           PredictionType  `json:"type"`
	TrainingRange  TimeRange       `json:"training_range"`
	PredictionStart time.Time      `json:"prediction_start"`
	PredictionEnd  time.Time       `json:"prediction_end"`
	Predictions    []PredictedValue `json:"predictions"`
	Accuracy       PredictionAccuracy `json:"accuracy"`
	Model          PredictionModel `json:"model"`
	Metadata       PredictionMetadata `json:"metadata"`
	CreatedAt      time.Time       `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt      time.Time       `json:"updated_at" gorm:"autoUpdateTime"`
}

type PredictedValue struct {
	Timestamp      time.Time `json:"timestamp"`
	Value          float64   `json:"value"`
	LowerBound     float64   `json:"lower_bound"`
	UpperBound     float64   `json:"upper_bound"`
	Confidence     float64   `json:"confidence"`
	PredictionType string    `json:"prediction_type"`
}

type PredictionAccuracy struct {
	MAE      float64 `json:"mae"`       // Mean Absolute Error
	MSE      float64 `json:"mse"`       // Mean Squared Error
	RMSE     float64 `json:"rmse"`      // Root Mean Squared Error
	MAPE     float64 `json:"mape"`      // Mean Absolute Percentage Error
	R2Score  float64 `json:"r2_score"`  // R² Score
	AIC      float64 `json:"aic"`       // Akaike Information Criterion
	BIC      float64 `json:"bic"`       // Bayesian Information Criterion
}

type PredictionModel struct {
	Type          string                 `json:"type"`
	Parameters    map[string]interface{} `json:"parameters"`
	Features      []string               `json:"features"`
	TrainingSize  int                    `json:"training_size"`
	ValidationSize int                   `json:"validation_size"`
	Version       string                 `json:"version"`
}

type PredictionMetadata struct {
	DataPoints       int64   `json:"data_points"`
	Horizon          string  `json:"horizon"`
	Granularity      string  `json:"granularity"`
	Seasonality      bool    `json:"seasonality"`
	TrendComponent   float64 `json:"trend_component"`
	SeasonalComponent float64 `json:"seasonal_component"`
	ResidualComponent float64 `json:"residual_component"`
}

type PredictionRequest struct {
	TenantID       uuid.UUID       `json:"tenant_id" binding:"required"`
	MetricType     MetricType      `json:"metric_type" binding:"required"`
	TrainingRange  TimeRange       `json:"training_range" binding:"required"`
	Method         PredictionMethod `json:"method"`
	PredictionHorizon time.Duration `json:"prediction_horizon"`
	Granularity    Granularity     `json:"granularity"`
	ConfidenceLevel float64        `json:"confidence_level"`
}

type PredictionEngine struct {
	engine *Engine
}

func NewPredictionEngine(engine *Engine) *PredictionEngine {
	return &PredictionEngine{engine: engine}
}

func (pe *PredictionEngine) GeneratePrediction(ctx context.Context, req *PredictionRequest) (*Prediction, error) {
	if err := pe.validateRequest(req); err != nil {
		return nil, err
	}

	if req.Method == "" {
		req.Method = PredictionMethodLinear
	}

	if req.ConfidenceLevel == 0 {
		req.ConfidenceLevel = 0.95
	}

	query := &AnalyticsQuery{
		TenantID:     req.TenantID,
		MetricType:   req.MetricType,
		TimeRange:    req.TrainingRange,
		Granularity:  req.Granularity,
		Aggregations: []AggregationType{AggregationAvg},
	}

	result, err := pe.engine.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query data for prediction: %w", err)
	}

	if len(result.Series) == 0 || len(result.Series[0].Points) < 5 {
		return nil, ErrInsufficientPredictionData
	}

	points := result.Series[0].Points
	values := pe.extractValues(points)
	timestamps := pe.extractTimestamps(points)

	prediction := &Prediction{
		ID:            uuid.New(),
		TenantID:      req.TenantID,
		MetricType:    req.MetricType,
		Method:        req.Method,
		Type:          PredictionTypeInterval,
		TrainingRange: req.TrainingRange,
		PredictionStart: req.TrainingRange.EndTime,
		PredictionEnd: req.TrainingRange.EndTime.Add(req.PredictionHorizon),
		Predictions:   make([]PredictedValue, 0),
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	switch req.Method {
	case PredictionMethodLinear:
		prediction.Predictions, prediction.Model = pe.linearPrediction(timestamps, values, points, req)
	case PredictionMethodMovingAverage:
		prediction.Predictions, prediction.Model = pe.movingAveragePrediction(values, points, req)
	case PredictionMethodExponentialSmoothing:
		prediction.Predictions, prediction.Model = pe.exponentialSmoothingPrediction(values, points, req)
	default:
		prediction.Predictions, prediction.Model = pe.linearPrediction(timestamps, values, points, req)
	}

	prediction.Accuracy = pe.calculateAccuracy(values, prediction.Predictions)
	prediction.Metadata = pe.buildMetadata(points, values, req)

	return prediction, nil
}

func (pe *PredictionEngine) validateRequest(req *PredictionRequest) error {
	if req.TenantID.IsZero() {
		return fmt.Errorf("tenant_id is required")
	}
	if req.TrainingRange.StartTime.IsZero() || req.TrainingRange.EndTime.IsZero() {
		return ErrInvalidTimeRange
	}
	if req.TrainingRange.StartTime.After(req.TrainingRange.EndTime) {
		return ErrInvalidTimeRange
	}
	if req.PredictionHorizon <= 0 {
		return fmt.Errorf("prediction_horizon must be positive")
	}
	if req.PredictionHorizon > 365*24*time.Hour {
		return ErrPredictionHorizonTooFar
	}
	return nil
}

func (pe *PredictionEngine) extractValues(points []MetricPoint) []float64 {
	values := make([]float64, len(points))
	for i, p := range points {
		values[i] = p.Value
	}
	return values
}

func (pe *PredictionEngine) extractTimestamps(points []MetricPoint) []float64 {
	timestamps := make([]float64, len(points))
	if len(points) > 0 {
		baseTime := points[0].Timestamp.Unix()
		for i, p := range points {
			timestamps[i] = float64(p.Timestamp.Unix() - baseTime)
		}
	}
	return timestamps
}

func (pe *PredictionEngine) linearPrediction(timestamps, values []float64, points []MetricPoint, req *PredictionRequest) ([]PredictedValue, PredictionModel) {
	slope, intercept := pe.calculateLinearRegression(timestamps, values)

	numPredictions := int(req.PredictionHorizon / pe.estimateInterval(points))
	if numPredictions == 0 {
		numPredictions = 1
	}
	if numPredictions > 100 {
		numPredictions = 100
	}

	predictions := make([]PredictedValue, numPredictions)
	lastTimestamp := timestamps[len(timestamps)-1]
	interval := 3600.0
	if len(timestamps) > 1 {
		interval = timestamps[len(timestamps)-1] - timestamps[len(timestamps)-2]
	}

	stdDev := pe.calculateStdDev(values)
	zScore := 1.96
	if req.ConfidenceLevel == 0.99 {
		zScore = 2.576
	} else if req.ConfidenceLevel == 0.90 {
		zScore = 1.645
	}

	for i := 0; i < numPredictions; i++ {
		futureTimestamp := lastTimestamp + float64(i+1)*interval
		predictedValue := slope*futureTimestamp + intercept

		uncertainty := stdDev * math.Sqrt(1.0+float64(i+1)*0.15)
		margin := zScore * uncertainty

		predictions[i] = PredictedValue{
			Timestamp:      points[len(points)-1].Timestamp.Add(time.Duration(i+1) * time.Hour),
			Value:          predictedValue,
			LowerBound:     predictedValue - margin,
			UpperBound:     predictedValue + margin,
			Confidence:     math.Max(0.1, req.ConfidenceLevel-float64(i+1)*0.03),
			PredictionType: "forecast",
		}
	}

	model := PredictionModel{
		Type:         "linear_regression",
		Parameters:   map[string]interface{}{"slope": slope, "intercept": intercept},
		Features:     []string{"time"},
		TrainingSize: len(values),
		Version:      "1.0",
	}

	return predictions, model
}

func (pe *PredictionEngine) movingAveragePrediction(values []float64, points []MetricPoint, req *PredictionRequest) ([]PredictedValue, PredictionModel) {
	windowSize := 5
	if len(values) < windowSize {
		windowSize = len(values)
	}

	recentValues := values[len(values)-windowSize:]
	avgValue := 0.0
	for _, v := range recentValues {
		avgValue += v
	}
	avgValue /= float64(windowSize)

	stdDev := pe.calculateStdDev(values)
	zScore := 1.96

	numPredictions := int(req.PredictionHorizon / pe.estimateInterval(points))
	if numPredictions == 0 {
		numPredictions = 1
	}
	if numPredictions > 100 {
		numPredictions = 100
	}

	predictions := make([]PredictedValue, numPredictions)
	for i := 0; i < numPredictions; i++ {
		uncertainty := stdDev * math.Sqrt(1.0+float64(i+1)*0.2)
		margin := zScore * uncertainty

		predictions[i] = PredictedValue{
			Timestamp:      points[len(points)-1].Timestamp.Add(time.Duration(i+1) * time.Hour),
			Value:          avgValue,
			LowerBound:     avgValue - margin,
			UpperBound:     avgValue + margin,
			Confidence:     math.Max(0.1, req.ConfidenceLevel-float64(i+1)*0.04),
			PredictionType: "forecast",
		}
	}

	model := PredictionModel{
		Type:         "moving_average",
		Parameters:   map[string]interface{}{"window_size": windowSize, "average": avgValue},
		TrainingSize: len(values),
		Version:      "1.0",
	}

	return predictions, model
}

func (pe *PredictionEngine) exponentialSmoothingPrediction(values []float64, points []MetricPoint, req *PredictionRequest) ([]PredictedValue, PredictionModel) {
	alpha := 0.3
	smoothedValue := values[0]
	for i := 1; i < len(values); i++ {
		smoothedValue = alpha*values[i] + (1-alpha)*smoothedValue
	}

	trend := 0.0
	if len(values) > 1 {
		recentTrend := (values[len(values)-1] - values[len(values)-2])
		trend = alpha*recentTrend + (1-alpha)*0
	}

	stdDev := pe.calculateStdDev(values)
	zScore := 1.96

	numPredictions := int(req.PredictionHorizon / pe.estimateInterval(points))
	if numPredictions == 0 {
		numPredictions = 1
	}
	if numPredictions > 100 {
		numPredictions = 100
	}

	predictions := make([]PredictedValue, numPredictions)
	for i := 0; i < numPredictions; i++ {
		predictedValue := smoothedValue + trend*float64(i+1)
		uncertainty := stdDev * math.Sqrt(1.0+float64(i+1)*0.12)
		margin := zScore * uncertainty

		predictions[i] = PredictedValue{
			Timestamp:      points[len(points)-1].Timestamp.Add(time.Duration(i+1) * time.Hour),
			Value:          predictedValue,
			LowerBound:     predictedValue - margin,
			UpperBound:     predictedValue + margin,
			Confidence:     math.Max(0.1, req.ConfidenceLevel-float64(i+1)*0.025),
			PredictionType: "forecast",
		}
	}

	model := PredictionModel{
		Type:         "exponential_smoothing",
		Parameters:   map[string]interface{}{"alpha": alpha, "level": smoothedValue, "trend": trend},
		TrainingSize: len(values),
		Version:      "1.0",
	}

	return predictions, model
}

func (pe *PredictionEngine) calculateLinearRegression(x, y []float64) (slope, intercept float64) {
	if len(x) != len(y) || len(x) == 0 {
		return 0, 0
	}

	n := float64(len(x))
	sumX, sumY, sumXY, sumX2 := 0.0, 0.0, 0.0, 0.0

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

func (pe *PredictionEngine) calculateAccuracy(actual []float64, predictions []PredictedValue) PredictionAccuracy {
	if len(predictions) == 0 || len(actual) < 2 {
		return PredictionAccuracy{}
	}

	mae := 0.0
	mse := 0.0
	mape := 0.0
	ssRes := 0.0
	ssTot := 0.0
	mean := pe.calculateMean(actual)

	comparisonCount := 0
	for i := 0; i < len(predictions) && i < len(actual); i++ {
		err := math.Abs(predictions[i].Value - actual[i])
		mae += err
		mse += err * err

		if actual[i] != 0 {
			mape += math.Abs(err / actual[i])
		}

		ssRes += math.Pow(actual[i]-predictions[i].Value, 2)
		ssTot += math.Pow(actual[i]-mean, 2)
		comparisonCount++
	}

	if comparisonCount == 0 {
		return PredictionAccuracy{}
	}

	mae /= float64(comparisonCount)
	mse /= float64(comparisonCount)
	rmse := math.Sqrt(mse)
	mape = (mape / float64(comparisonCount)) * 100

	r2Score := 0.0
	if ssTot > 0 {
		r2Score = 1 - (ssRes / ssTot)
	}

	n := float64(comparisonCount)
	k := 2.0
	aic := n*math.Log(mse) + 2*k
	bic := n*math.Log(mse) + k*math.Log(n)

	return PredictionAccuracy{
		MAE:     mae,
		MSE:     mse,
		RMSE:    rmse,
		MAPE:    mape,
		R2Score: r2Score,
		AIC:     aic,
		BIC:     bic,
	}
}

func (pe *PredictionEngine) buildMetadata(points []MetricPoint, values []float64, req *PredictionRequest) PredictionMetadata {
	var trendComponent, seasonalComponent, residualComponent float64

	if len(values) > 1 {
		trendComponent = (values[len(values)-1] - values[0]) / float64(len(values))
	}

	seasonalComponent = 0.0
	residualComponent = pe.calculateStdDev(values)

	return PredictionMetadata{
		DataPoints:        int64(len(points)),
		Horizon:           req.PredictionHorizon.String(),
		Granularity:       string(req.Granularity),
		Seasonality:       false,
		TrendComponent:    trendComponent,
		SeasonalComponent: seasonalComponent,
		ResidualComponent: residualComponent,
	}
}

func (pe *PredictionEngine) estimateInterval(points []MetricPoint) time.Duration {
	if len(points) < 2 {
		return time.Hour
	}
	return points[len(points)-1].Timestamp.Sub(points[len(points)-2].Timestamp)
}

func (pe *PredictionEngine) calculateMean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

func (pe *PredictionEngine) calculateStdDev(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	mean := pe.calculateMean(values)
	variance := 0.0
	for _, v := range values {
		variance += math.Pow(v-mean, 2)
	}
	variance /= float64(len(values))
	return math.Sqrt(variance)
}
