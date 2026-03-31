package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"runtime/debug"
	"time"

	"github.com/gorilla/mux"
)

// SystemHandlers handles system-level API requests (health, readiness, info, diagnostics)
type SystemHandlers struct {
	version   string
	buildTime string
	gitCommit string
	startTime time.Time
}

// NewSystemHandlers creates a new system handlers instance
func NewSystemHandlers(version, buildTime, gitCommit string) *SystemHandlers {
	return &SystemHandlers{
		version:   version,
		buildTime: buildTime,
		gitCommit: gitCommit,
		startTime: time.Now(),
	}
}

// HealthResponse represents a detailed health check response
type HealthResponse struct {
	Status    string                    `json:"status"`
	Timestamp string                    `json:"timestamp"`
	Uptime    string                    `json:"uptime"`
	Version   string                    `json:"version"`
	Checks    map[string]ComponentCheck `json:"checks,omitempty"`
}

// ComponentCheck represents the health status of a single component
type ComponentCheck struct {
	Status   string        `json:"status"`
	Duration string        `json:"duration,omitempty"`
	Message  string        `json:"message,omitempty"`
	Details  interface{}   `json:"details,omitempty"`
}

// ReadinessResponse represents the readiness check response
type ReadinessResponse struct {
	Ready     bool     `json:"ready"`
	Timestamp string   `json:"timestamp"`
	Reasons   []string `json:"reasons,omitempty"`
}

// VersionResponse represents the version information response
type VersionResponse struct {
	Version   string `json:"version"`
	BuildTime string `json:"build_time"`
	GitCommit string `json:"git_commit"`
	GoVersion string `json:"go_version"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
}

// SystemInfoResponse represents detailed system information
type SystemInfoResponse struct {
	Version   string            `json:"version"`
	Go        GoInfo            `json:"go"`
	Runtime   RuntimeInfo       `json:"runtime"`
	Host      HostInfo          `json:"host"`
	Uptime    string            `json:"uptime"`
	StartTime string            `json:"start_time"`
	Build     BuildInfo         `json:"build"`
}

// GoInfo holds Go runtime information
type GoInfo struct {
	Version  string `json:"version"`
	OS       string `json:"os"`
	Arch     string `json:"arch"`
	MaxProcs int    `json:"max_procs"`
	NumCPU   int    `json:"num_cpu"`
}

// RuntimeInfo holds memory and goroutine information
type RuntimeInfo struct {
	Goroutines    int              `json:"goroutines"`
	MemoryStats   MemoryStatsInfo  `json:"memory"`
	GCStats       GCStatsInfo      `json:"gc"`
	CGOCalls      int64            `json:"cgo_calls"`
}

// MemoryStatsInfo holds memory statistics in human-readable format
type MemoryStatsInfo struct {
	Alloc       string `json:"alloc"`
	TotalAlloc  string `json:"total_alloc"`
	Sys         string `json:"sys"`
	HeapAlloc   string `json:"heap_alloc"`
	HeapSys     string `json:"heap_sys"`
	HeapInUse   string `json:"heap_in_use"`
	HeapObjects string `json:"heap_objects"`
	StackInUse  string `json:"stack_in_use"`
	StackSys    string `json:"stack_sys"`
}

// GCStatsInfo holds garbage collector statistics
type GCStatsInfo struct {
	NumGC        uint32 `json:"num_gc"`
	LastGC       string `json:"last_gc"`
	PauseTotal   string `json:"pause_total"`
	NextGC       string `json:"next_gc"`
	GCPausePercent float64 `json:"gc_pause_percent"`
}

// HostInfo holds host system information
type HostInfo struct {
	Hostname string `json:"hostname,omitempty"`
	OS       string `json:"os"`
	Platform string `json:"platform"`
}

// BuildInfo holds build information
type BuildInfo struct {
	Version   string `json:"version"`
	BuildTime string `json:"build_time"`
	GitCommit string `json:"git_commit"`
}

// DiagnosticsResponse represents a full diagnostics snapshot
type DiagnosticsResponse struct {
	Timestamp  string            `json:"timestamp"`
	Version    VersionResponse   `json:"version"`
	System     SystemInfoResponse `json:"system"`
	Health     HealthResponse    `json:"health"`
	Config     interface{}       `json:"config,omitempty"`
}

// MetricsResponse represents collected metrics
type MetricsResponse struct {
	Timestamp      string              `json:"timestamp"`
	Uptime         string              `json:"uptime"`
	RequestMetrics RequestMetricsInfo  `json:"requests"`
	MemoryMetrics  MemoryMetricsInfo   `json:"memory"`
	RuntimeMetrics RuntimeMetricsInfo  `json:"runtime"`
}

// RequestMetricsInfo holds request-related metrics
type RequestMetricsInfo struct {
	ActiveRequests  int `json:"active_requests"`
	QueuedRequests  int `json:"queued_requests"`
}

// MemoryMetricsInfo holds memory-related metrics in bytes
type MemoryMetricsInfo struct {
	AllocBytes     uint64  `json:"alloc_bytes"`
	SysBytes       uint64  `json:"sys_bytes"`
	HeapAllocBytes uint64  `json:"heap_alloc_bytes"`
	HeapSysBytes   uint64  `json:"heap_sys_bytes"`
	HeapInUseBytes uint64  `json:"heap_in_use_bytes"`
	StackInUseBytes uint64 `json:"stack_in_use_bytes"`
	GCCount        uint32  `json:"gc_count"`
	Goroutines     int     `json:"goroutines"`
}

// RuntimeMetricsInfo holds runtime metrics
type RuntimeMetricsInfo struct {
	Goroutines int    `json:"goroutines"`
	NumCPU     int    `json:"num_cpu"`
	MaxProcs   int    `json:"max_procs"`
	CGOCalls   int64  `json:"cgo_calls"`
	UptimeSec  int64  `json:"uptime_sec"`
}

// Health handles GET /health
// Returns a detailed health check including component status
func (h *SystemHandlers) Health(w http.ResponseWriter, r *http.Request) {
	checks := make(map[string]ComponentCheck)

	// Check Go runtime health
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	checks["runtime"] = ComponentCheck{
		Status:   "healthy",
		Duration: "0ms",
		Details: map[string]interface{}{
			"goroutines": runtime.NumGoroutine(),
			"heap_alloc": formatBytes(memStats.HeapAlloc),
			"gc_count":   memStats.NumGC,
		},
	}

	// Overall status
	overallStatus := "healthy"
	for _, check := range checks {
		if check.Status != "healthy" {
			overallStatus = "degraded"
			break
		}
	}

	response := HealthResponse{
		Status:    overallStatus,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Uptime:    h.formatDuration(time.Since(h.startTime)),
		Version:   h.version,
		Checks:    checks,
	}

	statusCode := http.StatusOK
	if overallStatus == "degraded" {
		statusCode = http.StatusServiceUnavailable
	}

	h.respondJSON(w, statusCode, response)
}

// Readiness handles GET /ready
// Returns whether the service is ready to accept traffic
func (h *SystemHandlers) Readiness(w http.ResponseWriter, r *http.Request) {
	var reasons []string
	ready := true

	// Check if the service has been running for at least 1 second
	if time.Since(h.startTime) < time.Second {
		reasons = append(reasons, "service still initializing")
		ready = false
	}

	// Check goroutine count (if excessively high, consider not ready)
	if runtime.NumGoroutine() > 10000 {
		reasons = append(reasons, fmt.Sprintf("high goroutine count: %d", runtime.NumGoroutine()))
		ready = false
	}

	// Check memory pressure
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	if memStats.Sys > 10*1024*1024*1024 { // > 10GB system memory
		reasons = append(reasons, "high memory pressure")
		ready = false
	}

	response := ReadinessResponse{
		Ready:     ready,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Reasons:   reasons,
	}

	statusCode := http.StatusOK
	if !ready {
		statusCode = http.StatusServiceUnavailable
	}

	h.respondJSON(w, statusCode, response)
}

// Version handles GET /version
// Returns version information about the running service
func (h *SystemHandlers) Version(w http.ResponseWriter, r *http.Request) {
	response := VersionResponse{
		Version:   h.version,
		BuildTime: h.buildTime,
		GitCommit: h.gitCommit,
		GoVersion: runtime.Version(),
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
	}

	h.respondJSON(w, http.StatusOK, response)
}

// SystemInfo handles GET /api/v1/system/info
// Returns detailed system information including Go runtime, memory, and build info
func (h *SystemHandlers) SystemInfo(w http.ResponseWriter, r *http.Request) {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	uptime := time.Since(h.startTime)

	response := SystemInfoResponse{
		Version: h.version,
		Go: GoInfo{
			Version:  runtime.Version(),
			OS:       runtime.GOOS,
			Arch:     runtime.GOARCH,
			MaxProcs: runtime.GOMAXPROCS(0),
			NumCPU:   runtime.NumCPU(),
		},
		Runtime: RuntimeInfo{
			Goroutines: runtime.NumGoroutine(),
			MemoryStats: MemoryStatsInfo{
				Alloc:       formatBytes(memStats.Alloc),
				TotalAlloc:  formatBytes(memStats.TotalAlloc),
				Sys:         formatBytes(memStats.Sys),
				HeapAlloc:   formatBytes(memStats.HeapAlloc),
				HeapSys:     formatBytes(memStats.HeapSys),
				HeapInUse:   formatBytes(memStats.HeapInuse),
				HeapObjects: fmt.Sprintf("%d", memStats.HeapObjects),
				StackInUse:  formatBytes(memStats.StackInuse),
				StackSys:    formatBytes(memStats.StackSys),
			},
			GCStats: GCStatsInfo{
				NumGC:        memStats.NumGC,
				LastGC:       time.Unix(0, int64(memStats.LastGC)).Format(time.RFC3339),
				PauseTotal:   fmt.Sprintf("%.2fms", float64(memStats.PauseTotalNs)/1e6),
				NextGC:       formatBytes(memStats.NextGC),
				GCPausePercent: float64(memStats.PauseTotalNs) / float64(uptime.Nanoseconds()) * 100,
			},
			CGOCalls: runtime.NumCgoCall(),
		},
		Host: HostInfo{
			Hostname: "",
			OS:       runtime.GOOS,
			Platform: runtime.GOOS + "/" + runtime.GOARCH,
		},
		Uptime:    h.formatDuration(uptime),
		StartTime: h.startTime.UTC().Format(time.RFC3339),
		Build: BuildInfo{
			Version:   h.version,
			BuildTime: h.buildTime,
			GitCommit: h.gitCommit,
		},
	}

	h.respondJSON(w, http.StatusOK, response)
}

// Diagnostics handles GET /api/v1/system/diagnostics
// Returns a comprehensive diagnostics snapshot for troubleshooting
func (h *SystemHandlers) Diagnostics(w http.ResponseWriter, r *http.Request) {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	uptime := time.Since(h.startTime)

	// Collect build info from Go modules
	buildInfo := make(map[string]string)
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range bi.Settings {
			buildInfo[setting.Key] = setting.Value
		}
	}

	response := DiagnosticsResponse{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Version: VersionResponse{
			Version:   h.version,
			BuildTime: h.buildTime,
			GitCommit: h.gitCommit,
			GoVersion: runtime.Version(),
			OS:        runtime.GOOS,
			Arch:      runtime.GOARCH,
		},
		System: SystemInfoResponse{
			Version: h.version,
			Go: GoInfo{
				Version:  runtime.Version(),
				OS:       runtime.GOOS,
				Arch:     runtime.GOARCH,
				MaxProcs: runtime.GOMAXPROCS(0),
				NumCPU:   runtime.NumCPU(),
			},
			Runtime: RuntimeInfo{
				Goroutines: runtime.NumGoroutine(),
				MemoryStats: MemoryStatsInfo{
					Alloc:       formatBytes(memStats.Alloc),
					TotalAlloc:  formatBytes(memStats.TotalAlloc),
					Sys:         formatBytes(memStats.Sys),
					HeapAlloc:   formatBytes(memStats.HeapAlloc),
					HeapSys:     formatBytes(memStats.HeapSys),
					HeapInUse:   formatBytes(memStats.HeapInuse),
					HeapObjects: fmt.Sprintf("%d", memStats.HeapObjects),
					StackInUse:  formatBytes(memStats.StackInuse),
					StackSys:    formatBytes(memStats.StackSys),
				},
				GCStats: GCStatsInfo{
					NumGC:        memStats.NumGC,
					LastGC:       time.Unix(0, int64(memStats.LastGC)).Format(time.RFC3339),
					PauseTotal:   fmt.Sprintf("%.2fms", float64(memStats.PauseTotalNs)/1e6),
					NextGC:       formatBytes(memStats.NextGC),
					GCPausePercent: float64(memStats.PauseTotalNs) / float64(uptime.Nanoseconds()) * 100,
				},
				CGOCalls: runtime.NumCgoCall(),
			},
			Host: HostInfo{
				OS:       runtime.GOOS,
				Platform: runtime.GOOS + "/" + runtime.GOARCH,
			},
			Uptime:    h.formatDuration(uptime),
			StartTime: h.startTime.UTC().Format(time.RFC3339),
			Build: BuildInfo{
				Version:   h.version,
				BuildTime: h.buildTime,
				GitCommit: h.gitCommit,
			},
		},
		Health: HealthResponse{
			Status:    "healthy",
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Uptime:    h.formatDuration(uptime),
			Version:   h.version,
		},
		Config: map[string]interface{}{
			"go_max_procs":     runtime.GOMAXPROCS(0),
			"num_cpu":          runtime.NumCPU(),
			"go_version":       runtime.Version(),
			"build_settings":   buildInfo,
			"cgocalls":         runtime.NumCgoCall(),
			"goroutines":       runtime.NumGoroutine(),
		},
	}

	h.respondJSON(w, http.StatusOK, response)
}

// Metrics handles GET /api/v1/system/metrics
// Returns system and runtime metrics in a structured format
func (h *SystemHandlers) Metrics(w http.ResponseWriter, r *http.Request) {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	uptime := time.Since(h.startTime)

	response := MetricsResponse{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Uptime:    h.formatDuration(uptime),
		MemoryMetrics: MemoryMetricsInfo{
			AllocBytes:     memStats.Alloc,
			SysBytes:       memStats.Sys,
			HeapAllocBytes: memStats.HeapAlloc,
			HeapSysBytes:   memStats.HeapSys,
			HeapInUseBytes: memStats.HeapInuse,
			StackInUseBytes: memStats.StackInuse,
			GCCount:        memStats.NumGC,
			Goroutines:     uint64(runtime.NumGoroutine()),
		},
		RuntimeMetrics: RuntimeMetricsInfo{
			Goroutines: runtime.NumGoroutine(),
			NumCPU:     runtime.NumCPU(),
			MaxProcs:   runtime.GOMAXPROCS(0),
			CGOCalls:   runtime.NumCgoCall(),
			UptimeSec:  int64(uptime.Seconds()),
		},
	}

	h.respondJSON(w, http.StatusOK, response)
}

// Ping handles GET /ping
// Returns a simple pong response for lightweight health checking
func (h *SystemHandlers) Ping(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"message":"pong","timestamp":"%s"}`, time.Now().UTC().Format(time.RFC3339))
}

// RegisterSystemRoutes registers all system routes on the given router
func (h *SystemHandlers) RegisterSystemRoutes(router *mux.Router) {
	// Basic health endpoints (no /api/v1 prefix, commonly used by load balancers)
	router.HandleFunc("/health", h.Health).Methods("GET")
	router.HandleFunc("/ready", h.Readiness).Methods("GET")
	router.HandleFunc("/version", h.Version).Methods("GET")
	router.HandleFunc("/ping", h.Ping).Methods("GET")

	// Detailed system endpoints under /api/v1/system
	api := router.PathPrefix("/api/v1/system").Subrouter()
	api.HandleFunc("/info", h.SystemInfo).Methods("GET")
	api.HandleFunc("/diagnostics", h.Diagnostics).Methods("GET")
	api.HandleFunc("/metrics", h.Metrics).Methods("GET")
}

// formatDuration formats a duration into a human-readable string
func (h *SystemHandlers) formatDuration(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd%dh%dm%ds", days, hours, minutes, seconds)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh%dm%ds", hours, minutes, seconds)
	}
	if minutes > 0 {
		return fmt.Sprintf("%dm%ds", minutes, seconds)
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}

// respondJSON sends a JSON response
func (h *SystemHandlers) respondJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(data)
}

// formatBytes formats byte count into human-readable string
func formatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
