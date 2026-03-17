package monitoring

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics holds all Prometheus metrics for the AI Provider
type Metrics struct {
	// HTTP metrics
	HTTPRequestsTotal    *prometheus.CounterVec
	HTTPRequestDuration  *prometheus.HistogramVec
	HTTPRequestsInFlight *prometheus.GaugeVec
	HTTPRequestSize      *prometheus.HistogramVec
	HTTPResponseSize     *prometheus.HistogramVec

	// Model metrics
	ModelInferenceTotal     *prometheus.CounterVec
	ModelInferenceDuration  *prometheus.HistogramVec
	ModelInferenceErrors    *prometheus.CounterVec
	ModelInferenceTokens    *prometheus.CounterVec
	ModelLoadDuration       *prometheus.HistogramVec
	ModelUnloadDuration     *prometheus.HistogramVec
	ModelsActive            *prometheus.GaugeVec
	ModelsLoading           *prometheus.GaugeVec

	// Container metrics
	ContainersRunning    *prometheus.GaugeVec
	ContainersStopped    *prometheus.GaugeVec
	ContainerRestarts    *prometheus.CounterVec
	ContainerMemoryUsage *prometheus.GaugeVec
	ContainerCPUUsage    *prometheus.GaugeVec

	// Resource metrics
	CPUUtilization    *prometheus.GaugeVec
	MemoryUtilization *prometheus.GaugeVec
	GPUUtilization    *prometheus.GaugeVec
	GPUMemoryUsage    *prometheus.GaugeVec
	GPUTemperature    *prometheus.GaugeVec

	// Database metrics
	DatabaseConnections     *prometheus.GaugeVec
	DatabaseQueriesTotal    *prometheus.CounterVec
	DatabaseQueryDuration   *prometheus.HistogramVec
	DatabaseErrors          *prometheus.CounterVec
	DatabaseConnectionsIdle *prometheus.GaugeVec

	// Cache metrics
	CacheHitsTotal   *prometheus.CounterVec
	CacheMissesTotal *prometheus.CounterVec
	CacheLatency     *prometheus.HistogramVec
	CacheSize        *prometheus.GaugeVec
	CacheEvictions   *prometheus.CounterVec

	// Queue metrics
	QueueLength      *prometheus.GaugeVec
	QueueWaitTime    *prometheus.HistogramVec
	QueueProcessingTime *prometheus.HistogramVec
	QueueItemsProcessed *prometheus.CounterVec

	// System metrics
	Uptime            prometheus.GaugeFunc
	Goroutines        prometheus.GaugeFunc
	VersionInfo       *prometheus.GaugeVec
	ConfigReloadTotal *prometheus.CounterVec

	// Registry
	registry *prometheus.Registry
}

// MetricsConfig holds configuration for metrics
type MetricsConfig struct {
	Namespace          string
	Subsystem          string
	EnableDefaultMetrics bool
}

// NewMetrics creates and initializes all metrics
func NewMetrics(cfg *MetricsConfig) *Metrics {
	if cfg.Namespace == "" {
		cfg.Namespace = "ai_provider"
	}

	m := &Metrics{
		registry: prometheus.NewRegistry(),
	}

	// Initialize HTTP metrics
	m.initHTTPMetrics(cfg)

	// Initialize model metrics
	m.initModelMetrics(cfg)

	// Initialize container metrics
	m.initContainerMetrics(cfg)

	// Initialize resource metrics
	m.initResourceMetrics(cfg)

	// Initialize database metrics
	m.initDatabaseMetrics(cfg)

	// Initialize cache metrics
	m.initCacheMetrics(cfg)

	// Initialize queue metrics
	m.initQueueMetrics(cfg)

	// Initialize system metrics
	m.initSystemMetrics(cfg)

	// Register all metrics
	m.registerMetrics()

	log.Println("Prometheus metrics initialized successfully")
	return m
}

// initHTTPMetrics initializes HTTP-related metrics
func (m *Metrics) initHTTPMetrics(cfg *MetricsConfig) {
	m.HTTPRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: cfg.Namespace,
			Subsystem: cfg.Subsystem,
			Name:      "http_requests_total",
			Help:      "Total number of HTTP requests",
		},
		[]string{"method", "path", "status"},
	)

	m.HTTPRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: cfg.Namespace,
			Subsystem: cfg.Subsystem,
			Name:      "http_request_duration_seconds",
			Help:      "Duration of HTTP requests in seconds",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)

	m.HTTPRequestsInFlight = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: cfg.Namespace,
			Subsystem: cfg.Subsystem,
			Name:      "http_requests_in_flight",
			Help:      "Number of HTTP requests currently being processed",
		},
		[]string{"method", "path"},
	)

	m.HTTPRequestSize = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: cfg.Namespace,
			Subsystem: cfg.Subsystem,
			Name:      "http_request_size_bytes",
			Help:      "Size of HTTP requests in bytes",
			Buckets:   prometheus.ExponentialBuckets(100, 10, 7),
		},
		[]string{"method", "path"},
	)

	m.HTTPResponseSize = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: cfg.Namespace,
			Subsystem: cfg.Subsystem,
			Name:      "http_response_size_bytes",
			Help:      "Size of HTTP responses in bytes",
			Buckets:   prometheus.ExponentialBuckets(100, 10, 7),
		},
		[]string{"method", "path"},
	)
}

// initModelMetrics initializes model-related metrics
func (m *Metrics) initModelMetrics(cfg *MetricsConfig) {
	m.ModelInferenceTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: cfg.Namespace,
			Subsystem: "model",
			Name:      "inference_requests_total",
			Help:      "Total number of model inference requests",
		},
		[]string{"model_id", "model_name", "model_version", "status"},
	)

	m.ModelInferenceDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: cfg.Namespace,
			Subsystem: "model",
			Name:      "inference_duration_seconds",
			Help:      "Duration of model inference in seconds",
			Buckets:   prometheus.ExponentialBuckets(0.001, 2, 15),
		},
		[]string{"model_id", "model_name", "model_version"},
	)

	m.ModelInferenceErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: cfg.Namespace,
			Subsystem: "model",
			Name:      "inference_errors_total",
			Help:      "Total number of model inference errors",
		},
		[]string{"model_id", "model_name", "error_type"},
	)

	m.ModelInferenceTokens = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: cfg.Namespace,
			Subsystem: "model",
			Name:      "inference_tokens_total",
			Help:      "Total number of tokens processed",
		},
		[]string{"model_id", "model_name", "token_type"},
	)

	m.ModelLoadDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: cfg.Namespace,
			Subsystem: "model",
			Name:      "load_duration_seconds",
			Help:      "Duration of model loading in seconds",
			Buckets:   prometheus.ExponentialBuckets(0.1, 2, 15),
		},
		[]string{"model_id", "model_name"},
	)

	m.ModelUnloadDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: cfg.Namespace,
			Subsystem: "model",
			Name:      "unload_duration_seconds",
			Help:      "Duration of model unloading in seconds",
			Buckets:   prometheus.ExponentialBuckets(0.01, 2, 10),
		},
		[]string{"model_id", "model_name"},
	)

	m.ModelsActive = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: cfg.Namespace,
			Subsystem: "model",
			Name:      "active_count",
			Help:      "Number of active models",
		},
		[]string{"model_id", "model_name"},
	)

	m.ModelsLoading = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: cfg.Namespace,
			Subsystem: "model",
			Name:      "loading_count",
			Help:      "Number of models currently being loaded",
		},
		[]string{"model_id", "model_name"},
	)
}

// initContainerMetrics initializes container-related metrics
func (m *Metrics) initContainerMetrics(cfg *MetricsConfig) {
	m.ContainersRunning = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: cfg.Namespace,
			Subsystem: "container",
			Name:      "running_count",
			Help:      "Number of running containers",
		},
		[]string{"model_id", "model_name"},
	)

	m.ContainersStopped = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: cfg.Namespace,
			Subsystem: "container",
			Name:      "stopped_count",
			Help:      "Number of stopped containers",
		},
		[]string{"model_id", "model_name"},
	)

	m.ContainerRestarts = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: cfg.Namespace,
			Subsystem: "container",
			Name:      "restarts_total",
			Help:      "Total number of container restarts",
		},
		[]string{"model_id", "model_name", "reason"},
	)

	m.ContainerMemoryUsage = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: cfg.Namespace,
			Subsystem: "container",
			Name:      "memory_usage_bytes",
			Help:      "Memory usage of containers in bytes",
		},
		[]string{"model_id", "container_id"},
	)

	m.ContainerCPUUsage = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: cfg.Namespace,
			Subsystem: "container",
			Name:      "cpu_usage_percent",
			Help:      "CPU usage percentage of containers",
		},
		[]string{"model_id", "container_id"},
	)
}

// initResourceMetrics initializes system resource metrics
func (m *Metrics) initResourceMetrics(cfg *MetricsConfig) {
	m.CPUUtilization = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: cfg.Namespace,
			Subsystem: "system",
			Name:      "cpu_utilization_percent",
			Help:      "CPU utilization percentage",
		},
		[]string{"core"},
	)

	m.MemoryUtilization = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: cfg.Namespace,
			Subsystem: "system",
			Name:      "memory_utilization_percent",
			Help:      "Memory utilization percentage",
		},
		[]string{"type"},
	)

	m.GPUUtilization = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: cfg.Namespace,
			Subsystem: "gpu",
			Name:      "utilization_percent",
			Help:      "GPU utilization percentage",
		},
		[]string{"gpu_id", "gpu_name"},
	)

	m.GPUMemoryUsage = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: cfg.Namespace,
			Subsystem: "gpu",
			Name:      "memory_usage_bytes",
			Help:      "GPU memory usage in bytes",
		},
		[]string{"gpu_id", "gpu_name"},
	)

	m.GPUTemperature = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: cfg.Namespace,
			Subsystem: "gpu",
			Name:      "temperature_celsius",
			Help:      "GPU temperature in celsius",
		},
		[]string{"gpu_id", "gpu_name"},
	)
}

// initDatabaseMetrics initializes database-related metrics
func (m *Metrics) initDatabaseMetrics(cfg *MetricsConfig) {
	m.DatabaseConnections = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: cfg.Namespace,
			Subsystem: "database",
			Name:      "connections_active",
			Help:      "Number of active database connections",
		},
		[]string{"database"},
	)

	m.DatabaseQueriesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: cfg.Namespace,
			Subsystem: "database",
			Name:      "queries_total",
			Help:      "Total number of database queries",
		},
		[]string{"database", "operation", "table"},
	)

	m.DatabaseQueryDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: cfg.Namespace,
			Subsystem: "database",
			Name:      "query_duration_seconds",
			Help:      "Duration of database queries in seconds",
			Buckets:   prometheus.ExponentialBuckets(0.001, 2, 15),
		},
		[]string{"database", "operation"},
	)

	m.DatabaseErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: cfg.Namespace,
			Subsystem: "database",
			Name:      "errors_total",
			Help:      "Total number of database errors",
		},
		[]string{"database", "error_type"},
	)

	m.DatabaseConnectionsIdle = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: cfg.Namespace,
			Subsystem: "database",
			Name:      "connections_idle",
			Help:      "Number of idle database connections",
		},
		[]string{"database"},
	)
}

// initCacheMetrics initializes cache-related metrics
func (m *Metrics) initCacheMetrics(cfg *MetricsConfig) {
	m.CacheHitsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: cfg.Namespace,
			Subsystem: "cache",
			Name:      "hits_total",
			Help:      "Total number of cache hits",
		},
		[]string{"cache_type"},
	)

	m.CacheMissesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: cfg.Namespace,
			Subsystem: "cache",
			Name:      "misses_total",
			Help:      "Total number of cache misses",
		},
		[]string{"cache_type"},
	)

	m.CacheLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: cfg.Namespace,
			Subsystem: "cache",
			Name:      "latency_seconds",
			Help:      "Cache operation latency in seconds",
			Buckets:   prometheus.ExponentialBuckets(0.0001, 2, 15),
		},
		[]string{"cache_type", "operation"},
	)

	m.CacheSize = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: cfg.Namespace,
			Subsystem: "cache",
			Name:      "size_bytes",
			Help:      "Cache size in bytes",
		},
		[]string{"cache_type"},
	)

	m.CacheEvictions = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: cfg.Namespace,
			Subsystem: "cache",
			Name:      "evictions_total",
			Help:      "Total number of cache evictions",
		},
		[]string{"cache_type", "reason"},
	)
}

// initQueueMetrics initializes queue-related metrics
func (m *Metrics) initQueueMetrics(cfg *MetricsConfig) {
	m.QueueLength = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: cfg.Namespace,
			Subsystem: "queue",
			Name:      "length",
			Help:      "Current queue length",
		},
		[]string{"queue_name", "priority"},
	)

	m.QueueWaitTime = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: cfg.Namespace,
			Subsystem: "queue",
			Name:      "wait_time_seconds",
			Help:      "Time items spend waiting in queue",
			Buckets:   prometheus.ExponentialBuckets(0.01, 2, 15),
		},
		[]string{"queue_name"},
	)

	m.QueueProcessingTime = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: cfg.Namespace,
			Subsystem: "queue",
			Name:      "processing_time_seconds",
			Help:      "Time spent processing queue items",
			Buckets:   prometheus.ExponentialBuckets(0.01, 2, 15),
		},
		[]string{"queue_name"},
	)

	m.QueueItemsProcessed = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: cfg.Namespace,
			Subsystem: "queue",
			Name:      "items_processed_total",
			Help:      "Total number of queue items processed",
		},
		[]string{"queue_name", "status"},
	)
}

// initSystemMetrics initializes system-level metrics
func (m *Metrics) initSystemMetrics(cfg *MetricsConfig) {
	// Uptime metric (will be set dynamically)
	m.Uptime = prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{
			Namespace: cfg.Namespace,
			Subsystem: "system",
			Name:      "uptime_seconds",
			Help:      "System uptime in seconds",
		},
		func() float64 {
			return float64(time.Since(startTime).Seconds())
		},
	)

	// Goroutines count
	m.Goroutines = prometheus.NewGaugeFunc(
		prometheus.GaugeOpts{
			Namespace: cfg.Namespace,
			Subsystem: "system",
			Name:      "goroutines",
			Help:      "Number of goroutines",
		},
		func() float64 {
			return float64(getGoroutineCount())
		},
	)

	// Version information
	m.VersionInfo = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: cfg.Namespace,
			Subsystem: "system",
			Name:      "version_info",
			Help:      "System version information",
		},
		[]string{"version", "build_time", "git_commit"},
	)

	m.ConfigReloadTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: cfg.Namespace,
			Subsystem: "system",
			Name:      "config_reloads_total",
			Help:      "Total number of configuration reloads",
		},
		[]string{"status"},
	)
}

// registerMetrics registers all metrics with the Prometheus registry
func (m *Metrics) registerMetrics() {
	// Register HTTP metrics
	m.registry.MustRegister(m.HTTPRequestsTotal)
	m.registry.MustRegister(m.HTTPRequestDuration)
	m.registry.MustRegister(m.HTTPRequestsInFlight)
	m.registry.MustRegister(m.HTTPRequestSize)
	m.registry.MustRegister(m.HTTPResponseSize)

	// Register model metrics
	m.registry.MustRegister(m.ModelInferenceTotal)
	m.registry.MustRegister(m.ModelInferenceDuration)
	m.registry.MustRegister(m.ModelInferenceErrors)
	m.registry.MustRegister(m.ModelInferenceTokens)
	m.registry.MustRegister(m.ModelLoadDuration)
	m.registry.MustRegister(m.ModelUnloadDuration)
	m.registry.MustRegister(m.ModelsActive)
	m.registry.MustRegister(m.ModelsLoading)

	// Register container metrics
	m.registry.MustRegister(m.ContainersRunning)
	m.registry.MustRegister(m.ContainersStopped)
	m.registry.MustRegister(m.ContainerRestarts)
	m.registry.MustRegister(m.ContainerMemoryUsage)
	m.registry.MustRegister(m.ContainerCPUUsage)

	// Register resource metrics
	m.registry.MustRegister(m.CPUUtilization)
	m.registry.MustRegister(m.MemoryUtilization)
	m.registry.MustRegister(m.GPUUtilization)
	m.registry.MustRegister(m.GPUMemoryUsage)
	m.registry.MustRegister(m.GPUTemperature)

	// Register database metrics
	m.registry.MustRegister(m.DatabaseConnections)
	m.registry.MustRegister(m.DatabaseQueriesTotal)
	m.registry.MustRegister(m.DatabaseQueryDuration)
	m.registry.MustRegister(m.DatabaseErrors)
	m.registry.MustRegister(m.DatabaseConnectionsIdle)

	// Register cache metrics
	m.registry.MustRegister(m.CacheHitsTotal)
	m.registry.MustRegister(m.CacheMissesTotal)
	m.registry.MustRegister(m.CacheLatency)
	m.registry.MustRegister(m.CacheSize)
	m.registry.MustRegister(m.CacheEvictions)

	// Register queue metrics
	m.registry.MustRegister(m.QueueLength)
	m.registry.MustRegister(m.QueueWaitTime)
	m.registry.MustRegister(m.QueueProcessingTime)
	m.registry.MustRegister(m.QueueItemsProcessed)

	// Register system metrics
	m.registry.MustRegister(m.Uptime)
	m.registry.MustRegister(m.Goroutines)
	m.registry.MustRegister(m.VersionInfo)
	m.registry.MustRegister(m.ConfigReloadTotal)
}

// Handler returns an HTTP handler for the Prometheus metrics endpoint
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

// RecordHTTPRequest records an HTTP request metric
func (m *Metrics) RecordHTTPRequest(method, path, status string, duration time.Duration, requestSize, responseSize int64) {
	m.HTTPRequestsTotal.WithLabelValues(method, path, status).Inc()
	m.HTTPRequestDuration.WithLabelValues(method, path).Observe(duration.Seconds())
	m.HTTPRequestSize.WithLabelValues(method, path).Observe(float64(requestSize))
	m.HTTPResponseSize.WithLabelValues(method, path).Observe(float64(responseSize))
}

// IncrementInFlightRequest increments the in-flight request counter
func (m *Metrics) IncrementInFlightRequest(method, path string) {
	m.HTTPRequestsInFlight.WithLabelValues(method, path).Inc()
}

// DecrementInFlightRequest decrements the in-flight request counter
func (m *Metrics) DecrementInFlightRequest(method, path string) {
	m.HTTPRequestsInFlight.WithLabelValues(method, path).Dec()
}

// RecordModelInference records a model inference metric
func (m *Metrics) RecordModelInference(modelID, modelName, modelVersion, status string, duration time.Duration, inputTokens, outputTokens int) {
	m.ModelInferenceTotal.WithLabelValues(modelID, modelName, modelVersion, status).Inc()
	m.ModelInferenceDuration.WithLabelValues(modelID, modelName, modelVersion).Observe(duration.Seconds())

	if inputTokens > 0 {
		m.ModelInferenceTokens.WithLabelValues(modelID, modelName, "input").Add(float64(inputTokens))
	}
	if outputTokens > 0 {
		m.ModelInferenceTokens.WithLabelValues(modelID, modelName, "output").Add(float64(outputTokens))
	}
}

// RecordModelError records a model error
func (m *Metrics) RecordModelError(modelID, modelName, errorType string) {
	m.ModelInferenceErrors.WithLabelValues(modelID, modelName, errorType).Inc()
}

// RecordModelLoad records a model loading duration
func (m *Metrics) RecordModelLoad(modelID, modelName string, duration time.Duration) {
	m.ModelLoadDuration.WithLabelValues(modelID, modelName).Observe(duration.Seconds())
}

// RecordModelUnload records a model unloading duration
func (m *Metrics) RecordModelUnload(modelID, modelName string, duration time.Duration) {
	m.ModelUnloadDuration.WithLabelValues(modelID, modelName).Observe(duration.Seconds())
}

// SetActiveModels sets the number of active models
func (m *Metrics) SetActiveModels(modelID, modelName string, count int) {
	m.ModelsActive.WithLabelValues(modelID, modelName).Set(float64(count))
}

// SetLoadingModels sets the number of loading models
func (m *Metrics) SetLoadingModels(modelID, modelName string, count int) {
	m.ModelsLoading.WithLabelValues(modelID, modelName).Set(float64(count))
}

// UpdateContainerMetrics updates container-related metrics
func (m *Metrics) UpdateContainerMetrics(modelID, modelName, containerID string, running int, memoryUsage int64, cpuUsage float64) {
	m.ContainersRunning.WithLabelValues(modelID, modelName).Set(float64(running))
	m.ContainerMemoryUsage.WithLabelValues(modelID, containerID).Set(float64(memoryUsage))
	m.ContainerCPUUsage.WithLabelValues(modelID, containerID).Set(cpuUsage)
}

// RecordContainerRestart records a container restart
func (m *Metrics) RecordContainerRestart(modelID, modelName, reason string) {
	m.ContainerRestarts.WithLabelValues(modelID, modelName, reason).Inc()
}

// UpdateResourceMetrics updates system resource metrics
func (m *Metrics) UpdateResourceMetrics(cpuUtil float64, memUtil float64, gpuUtils map[int]float64) {
	m.CPUUtilization.WithLabelValues("total").Set(cpuUtil)
	m.MemoryUtilization.WithLabelValues("total").Set(memUtil)

	for gpuID, util := range gpuUtils {
		m.GPUUtilization.WithLabelValues(fmt.Sprintf("%d", gpuID), "").Set(util)
	}
}

// UpdateGPUMetrics updates GPU-specific metrics
func (m *Metrics) UpdateGPUMetrics(gpuID int, gpuName string, memoryUsage int64, temperature float64) {
	gpuIDStr := fmt.Sprintf("%d", gpuID)
	m.GPUMemoryUsage.WithLabelValues(gpuIDStr, gpuName).Set(float64(memoryUsage))
	m.GPUTemperature.WithLabelValues(gpuIDStr, gpuName).Set(temperature)
}

// RecordDatabaseQuery records a database query metric
func (m *Metrics) RecordDatabaseQuery(database, operation, table string, duration time.Duration, err error) {
	m.DatabaseQueriesTotal.WithLabelValues(database, operation, table).Inc()
	m.DatabaseQueryDuration.WithLabelValues(database, operation).Observe(duration.Seconds())

	if err != nil {
		m.DatabaseErrors.WithLabelValues(database, "query_error").Inc()
	}
}

// UpdateDatabaseConnections updates database connection metrics
func (m *Metrics) UpdateDatabaseConnections(database string, active, idle int) {
	m.DatabaseConnections.WithLabelValues(database).Set(float64(active))
	m.DatabaseConnectionsIdle.WithLabelValues(database).Set(float64(idle))
}

// RecordCacheHit records a cache hit
func (m *Metrics) RecordCacheHit(cacheType string) {
	m.CacheHitsTotal.WithLabelValues(cacheType).Inc()
}

// RecordCacheMiss records a cache miss
func (m *Metrics) RecordCacheMiss(cacheType string) {
	m.CacheMissesTotal.WithLabelValues(cacheType).Inc()
}

// RecordCacheOperation records a cache operation with latency
func (m *Metrics) RecordCacheOperation(cacheType, operation string, duration time.Duration) {
	m.CacheLatency.WithLabelValues(cacheType, operation).Observe(duration.Seconds())
}

// UpdateCacheSize updates cache size metric
func (m *Metrics) UpdateCacheSize(cacheType string, size int64) {
	m.CacheSize.WithLabelValues(cacheType).Set(float64(size))
}

// RecordCacheEviction records a cache eviction
func (m *Metrics) RecordCacheEviction(cacheType, reason string) {
	m.CacheEvictions.WithLabelValues(cacheType, reason).Inc()
}

// UpdateQueueLength updates queue length metric
func (m *Metrics) UpdateQueueLength(queueName, priority string, length int) {
	m.QueueLength.WithLabelValues(queueName, priority).Set(float64(length))
}

// RecordQueueProcessing records queue item processing
func (m *Metrics) RecordQueueProcessing(queueName string, waitTime, processingTime time.Duration, status string) {
	m.QueueWaitTime.WithLabelValues(queueName).Observe(waitTime.Seconds())
	m.QueueProcessingTime.WithLabelValues(queueName).Observe(processingTime.Seconds())
	m.QueueItemsProcessed.WithLabelValues(queueName, status).Inc()
}

// SetVersionInfo sets the version information metric
func (m *Metrics) SetVersionInfo(version, buildTime, gitCommit string) {
	m.VersionInfo.WithLabelValues(version, buildTime, gitCommit).Set(1)
}

// RecordConfigReload records a configuration reload
func (m *Metrics) RecordConfigReload(success bool) {
	status := "success"
	if !success {
		status = "failure"
	}
	m.ConfigReloadTotal.WithLabelValues(status).Inc()
}

// GetRegistry returns the Prometheus registry
func (m *Metrics) GetRegistry() *prometheus.Registry {
	return m.registry
}

// Helper variables and functions
var startTime = time.Now()

func getGoroutineCount() int {
	// This will be implemented using runtime.NumGoroutine()
	return 0 // placeholder
}

// ResponseWriter wrapper for capturing status code
type responseWriterWrapper struct {
	http.ResponseWriter
	statusCode int
	written    bool
}

func (w *responseWriterWrapper) WriteHeader(code int) {
	if !w.written {
		w.statusCode = code
		w.written = true
		w.ResponseWriter.WriteHeader(code)
	}
}

func (w *responseWriterWrapper) Write(b []byte) (int, error) {
	if !w.written {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
}

// Middleware returns an HTTP middleware for collecting metrics
func (m *Metrics) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		path := r.URL.Path
		method := r.Method

		// Wrap response writer to capture status code
		wrappedW := &responseWriterWrapper{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
			written:        false,
		}

		// Track in-flight requests
		m.IncrementInFlightRequest(method, path)
		defer m.DecrementInFlightRequest(method, path)

		// Call next handler
		next.ServeHTTP(wrappedW, r)

		// Record metrics
		duration := time.Since(start)
		status := strconv.Itoa(wrappedW.statusCode)
		m.RecordHTTPRequest(method, path, status, duration, r.ContentLength, 0)
	})
}
