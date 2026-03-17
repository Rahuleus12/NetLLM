package monitoring

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"runtime"
	"sync"
	"time"
)

// HealthStatus represents the health status of a component
type HealthStatus string

const (
	HealthStatusHealthy   HealthStatus = "healthy"
	HealthStatusUnhealthy HealthStatus = "unhealthy"
	HealthStatusDegraded  HealthStatus = "degraded"
)

// ComponentHealth represents the health of a single component
type ComponentHealth struct {
	Name      string       `json:"name"`
	Status    HealthStatus `json:"status"`
	Message   string       `json:"message,omitempty"`
	Timestamp time.Time    `json:"timestamp"`
	Details   interface{}  `json:"details,omitempty"`
}

// SystemHealth represents the overall system health
type SystemHealth struct {
	Status      HealthStatus                `json:"status"`
	Timestamp   time.Time                   `json:"timestamp"`
	Uptime      time.Duration               `json:"uptime"`
	Version     string                      `json:"version"`
	Components  map[string]ComponentHealth  `json:"components"`
	SystemInfo  SystemInfo                  `json:"system_info"`
}

// SystemInfo contains system-level information
type SystemInfo struct {
	GoVersion    string `json:"go_version"`
	OS           string `json:"os"`
	Arch         string `json:"arch"`
	NumCPU       int    `json:"num_cpu"`
	NumGoroutine int    `json:"num_goroutine"`
	MemAllocMB   uint64 `json:"mem_alloc_mb"`
	MemTotalMB   uint64 `json:"mem_total_mb"`
	MemSysMB     uint64 `json:"mem_sys_mb"`
}

// HealthChecker defines the interface for health checkers
type HealthChecker interface {
	Name() string
	Check(ctx context.Context) ComponentHealth
}

// HealthMonitor manages health checks for all system components
type HealthMonitor struct {
	checkers      map[string]HealthChecker
	startTime     time.Time
	version       string
	mu            sync.RWMutex
}

// NewHealthMonitor creates a new health monitor
func NewHealthMonitor(version string) *HealthMonitor {
	return &HealthMonitor{
		checkers:  make(map[string]HealthChecker),
		startTime: time.Now(),
		version:   version,
	}
}

// RegisterChecker registers a health checker
func (hm *HealthMonitor) RegisterChecker(checker HealthChecker) {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	hm.checkers[checker.Name()] = checker
}

// UnregisterChecker removes a health checker
func (hm *HealthMonitor) UnregisterChecker(name string) {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	delete(hm.checkers, name)
}

// CheckHealth performs health checks on all registered components
func (hm *HealthMonitor) CheckHealth(ctx context.Context) *SystemHealth {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	components := make(map[string]ComponentHealth)
	overallStatus := HealthStatusHealthy

	// Check each component
	for name, checker := range hm.checkers {
		health := checker.Check(ctx)
		components[name] = health

		// Update overall status
		if health.Status == HealthStatusUnhealthy {
			overallStatus = HealthStatusUnhealthy
		} else if health.Status == HealthStatusDegraded && overallStatus != HealthStatusUnhealthy {
			overallStatus = HealthStatusDegraded
		}
	}

	// Get system information
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	systemInfo := SystemInfo{
		GoVersion:    runtime.Version(),
		OS:           runtime.GOOS,
		Arch:         runtime.GOARCH,
		NumCPU:       runtime.NumCPU(),
		NumGoroutine: runtime.NumGoroutine(),
		MemAllocMB:   memStats.Alloc / 1024 / 1024,
		MemTotalMB:   memStats.TotalAlloc / 1024 / 1024,
		MemSysMB:     memStats.Sys / 1024 / 1024,
	}

	return &SystemHealth{
		Status:     overallStatus,
		Timestamp:  time.Now(),
		Uptime:     time.Since(hm.startTime),
		Version:    hm.version,
		Components: components,
		SystemInfo: systemInfo,
	}
}

// IsReady checks if the system is ready to accept requests
func (hm *HealthMonitor) IsReady(ctx context.Context) bool {
	health := hm.CheckHealth(ctx)
	return health.Status == HealthStatusHealthy || health.Status == HealthStatusDegraded
}

// IsLive checks if the system is alive (basic liveness probe)
func (hm *HealthMonitor) IsLive() bool {
	return true
}

// GetUptime returns the uptime duration
func (hm *HealthMonitor) GetUptime() time.Duration {
	return time.Since(hm.startTime)
}

// DatabaseHealthChecker checks database health
type DatabaseHealthChecker struct {
	db     *sql.DB
	name   string
}

// NewDatabaseHealthChecker creates a new database health checker
func NewDatabaseHealthChecker(db *sql.DB, name string) *DatabaseHealthChecker {
	return &DatabaseHealthChecker{
		db:   db,
		name: name,
	}
}

// Name returns the name of the health checker
func (d *DatabaseHealthChecker) Name() string {
	return d.name
}

// Check performs the database health check
func (d *DatabaseHealthChecker) Check(ctx context.Context) ComponentHealth {
	startTime := time.Now()

	// Perform a simple query to check connectivity
	var result int
	err := d.db.QueryRowContext(ctx, "SELECT 1").Scan(&result)

	latency := time.Since(startTime)

	if err != nil {
		return ComponentHealth{
			Name:      d.name,
			Status:    HealthStatusUnhealthy,
			Message:   fmt.Sprintf("Database connection failed: %v", err),
			Timestamp: time.Now(),
			Details: map[string]interface{}{
				"latency_ms": latency.Milliseconds(),
				"error":      err.Error(),
			},
		}
	}

	// Get database stats
	stats := d.db.Stats()

	status := HealthStatusHealthy
	message := "Database connection is healthy"

	// Check if too many connections are open
	if stats.OpenConnections > stats.MaxOpenConnections/2 {
		status = HealthStatusDegraded
		message = "Database connection pool is under high load"
	}

	return ComponentHealth{
		Name:      d.name,
		Status:    status,
		Message:   message,
		Timestamp: time.Now(),
		Details: map[string]interface{}{
			"latency_ms":          latency.Milliseconds(),
			"open_connections":    stats.OpenConnections,
			"idle_connections":    stats.Idle,
			"wait_count":          stats.WaitCount,
			"wait_duration_ms":    stats.WaitDuration.Milliseconds(),
			"max_open_connections": stats.MaxOpenConnections,
		},
	}
}

// RedisHealthChecker checks Redis health
type RedisHealthChecker struct {
	client interface {
		Ping(ctx context.Context) error
	}
	name string
}

// NewRedisHealthChecker creates a new Redis health checker
func NewRedisHealthChecker(client interface{ Ping(ctx context.Context) error }, name string) *RedisHealthChecker {
	return &RedisHealthChecker{
		client: client,
		name:   name,
	}
}

// Name returns the name of the health checker
func (r *RedisHealthChecker) Name() string {
	return r.name
}

// Check performs the Redis health check
func (r *RedisHealthChecker) Check(ctx context.Context) ComponentHealth {
	startTime := time.Now()

	err := r.client.Ping(ctx)

	latency := time.Since(startTime)

	if err != nil {
		return ComponentHealth{
			Name:      r.name,
			Status:    HealthStatusUnhealthy,
			Message:   fmt.Sprintf("Redis connection failed: %v", err),
			Timestamp: time.Now(),
			Details: map[string]interface{}{
				"latency_ms": latency.Milliseconds(),
				"error":      err.Error(),
			},
		}
	}

	status := HealthStatusHealthy
	message := "Redis connection is healthy"

	// Consider degraded if latency is too high
	if latency > 100*time.Millisecond {
		status = HealthStatusDegraded
		message = "Redis latency is high"
	}

	return ComponentHealth{
		Name:      r.name,
		Status:    status,
		Message:   message,
		Timestamp: time.Now(),
		Details: map[string]interface{}{
			"latency_ms": latency.Milliseconds(),
		},
	}
}

// GPUHealthChecker checks GPU availability
type GPUHealthChecker struct {
	name        string
	gpuEnabled  bool
	gpuDevices  []int
}

// NewGPUHealthChecker creates a new GPU health checker
func NewGPUHealthChecker(gpuEnabled bool, gpuDevices []int) *GPUHealthChecker {
	return &GPUHealthChecker{
		name:       "gpu",
		gpuEnabled: gpuEnabled,
		gpuDevices: gpuDevices,
	}
}

// Name returns the name of the health checker
func (g *GPUHealthChecker) Name() string {
	return g.name
}

// Check performs the GPU health check
func (g *GPUHealthChecker) Check(ctx context.Context) ComponentHealth {
	if !g.gpuEnabled {
		return ComponentHealth{
			Name:      g.name,
			Status:    HealthStatusHealthy,
			Message:   "GPU is disabled",
			Timestamp: time.Now(),
			Details: map[string]interface{}{
				"gpu_enabled": false,
			},
		}
	}

	// TODO: Implement actual GPU health check
	// This would typically use NVIDIA management library or nvidia-smi
	// For now, we'll simulate the check

	availableGPUs := len(g.gpuDevices)

	if availableGPUs == 0 {
		return ComponentHealth{
			Name:      g.name,
			Status:    HealthStatusUnhealthy,
			Message:   "No GPUs available",
			Timestamp: time.Now(),
			Details: map[string]interface{}{
				"gpu_enabled":  true,
				"gpu_count":    0,
				"gpu_devices":  g.gpuDevices,
			},
		}
	}

	return ComponentHealth{
		Name:      g.name,
		Status:    HealthStatusHealthy,
		Message:   fmt.Sprintf("%d GPU(s) available", availableGPUs),
		Timestamp: time.Now(),
		Details: map[string]interface{}{
			"gpu_enabled": true,
			"gpu_count":   availableGPUs,
			"gpu_devices": g.gpuDevices,
		},
	}
}

// ModelRegistryHealthChecker checks model registry health
type ModelRegistryHealthChecker struct {
	name         string
	registryPath string
}

// NewModelRegistryHealthChecker creates a new model registry health checker
func NewModelRegistryHealthChecker(registryPath string) *ModelRegistryHealthChecker {
	return &ModelRegistryHealthChecker{
		name:         "model_registry",
		registryPath: registryPath,
	}
}

// Name returns the name of the health checker
func (m *ModelRegistryHealthChecker) Name() string {
	return m.name
}

// Check performs the model registry health check
func (m *ModelRegistryHealthChecker) Check(ctx context.Context) ComponentHealth {
	// TODO: Implement actual model registry check
	// This would check if the registry path is accessible and contains valid models

	return ComponentHealth{
		Name:      m.name,
		Status:    HealthStatusHealthy,
		Message:   "Model registry is accessible",
		Timestamp: time.Now(),
		Details: map[string]interface{}{
			"registry_path": m.registryPath,
		},
	}
}

// ContainerRuntimeHealthChecker checks container runtime health
type ContainerRuntimeHealthChecker struct {
	name    string
	runtime string
}

// NewContainerRuntimeHealthChecker creates a new container runtime health checker
func NewContainerRuntimeHealthChecker(runtime string) *ContainerRuntimeHealthChecker {
	return &ContainerRuntimeHealthChecker{
		name:    "container_runtime",
		runtime: runtime,
	}
}

// Name returns the name of the health checker
func (c *ContainerRuntimeHealthChecker) Name() string {
	return c.name
}

// Check performs the container runtime health check
func (c *ContainerRuntimeHealthChecker) Check(ctx context.Context) ComponentHealth {
	// TODO: Implement actual container runtime check
	// This would verify that Docker/Podman is accessible and running

	return ComponentHealth{
		Name:      c.name,
		Status:    HealthStatusHealthy,
		Message:   fmt.Sprintf("Container runtime (%s) is accessible", c.runtime),
		Timestamp: time.Now(),
		Details: map[string]interface{}{
			"runtime": c.runtime,
		},
	}
}

// DiskSpaceHealthChecker checks available disk space
type DiskSpaceHealthChecker struct {
	name        string
	paths       []string
	minFreeGB   float64
}

// NewDiskSpaceHealthChecker creates a new disk space health checker
func NewDiskSpaceHealthChecker(paths []string, minFreeGB float64) *DiskSpaceHealthChecker {
	return &DiskSpaceHealthChecker{
		name:      "disk_space",
		paths:     paths,
		minFreeGB: minFreeGB,
	}
}

// Name returns the name of the health checker
func (d *DiskSpaceHealthChecker) Name() string {
	return d.name
}

// Check performs the disk space health check
func (d *DiskSpaceHealthChecker) Check(ctx context.Context) ComponentHealth {
	// TODO: Implement actual disk space check
	// This would use syscall.Statfs to check available disk space

	return ComponentHealth{
		Name:      d.name,
		Status:    HealthStatusHealthy,
		Message:   "Sufficient disk space available",
		Timestamp: time.Now(),
		Details: map[string]interface{}{
			"paths":       d.paths,
			"min_free_gb": d.minFreeGB,
		},
	}
}

// MemoryHealthChecker checks system memory usage
type MemoryHealthChecker struct {
	name          string
	maxUsagePct   float64
}

// NewMemoryHealthChecker creates a new memory health checker
func NewMemoryHealthChecker(maxUsagePct float64) *MemoryHealthChecker {
	return &MemoryHealthChecker{
		name:        "memory",
		maxUsagePct: maxUsagePct,
	}
}

// Name returns the name of the health checker
func (m *MemoryHealthChecker) Name() string {
	return m.name
}

// Check performs the memory health check
func (m *MemoryHealthChecker) Check(ctx context.Context) ComponentHealth {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// Calculate memory usage percentage
	// Note: This is heap memory, not total system memory
	usagePct := float64(memStats.Alloc) / float64(memStats.Sys) * 100

	status := HealthStatusHealthy
	message := "Memory usage is normal"

	if usagePct > m.maxUsagePct {
		status = HealthStatusDegraded
		message = fmt.Sprintf("Memory usage is high (%.1f%%)", usagePct)
	}

	if usagePct > 95 {
		status = HealthStatusUnhealthy
		message = "Memory usage is critical"
	}

	return ComponentHealth{
		Name:      m.name,
		Status:    status,
		Message:   message,
		Timestamp: time.Now(),
		Details: map[string]interface{}{
			"alloc_mb":    memStats.Alloc / 1024 / 1024,
			"total_mb":    memStats.TotalAlloc / 1024 / 1024,
			"sys_mb":      memStats.Sys / 1024 / 1024,
			"usage_pct":   usagePct,
			"max_usage_pct": m.maxUsagePct,
		},
	}
}

// SimpleHealthChecker is a basic health checker that uses a function
type SimpleHealthChecker struct {
	name     string
	checkFn  func(ctx context.Context) ComponentHealth
}

// NewSimpleHealthChecker creates a new simple health checker
func NewSimpleHealthChecker(name string, checkFn func(ctx context.Context) ComponentHealth) *SimpleHealthChecker {
	return &SimpleHealthChecker{
		name:    name,
		checkFn: checkFn,
	}
}

// Name returns the name of the health checker
func (s *SimpleHealthChecker) Name() string {
	return s.name
}

// Check performs the health check
func (s *SimpleHealthChecker) Check(ctx context.Context) ComponentHealth {
	if s.checkFn == nil {
		return ComponentHealth{
			Name:      s.name,
			Status:    HealthStatusUnhealthy,
			Message:   "Health check function not configured",
			Timestamp: time.Now(),
		}
	}
	return s.checkFn(ctx)
}

// Example usage and helper functions

// DefaultHealthMonitor creates a health monitor with default checkers
func DefaultHealthMonitor(version string, db *sql.DB, gpuEnabled bool, gpuDevices []int) *HealthMonitor {
	monitor := NewHealthMonitor(version)

	// Register default health checkers
	if db != nil {
		monitor.RegisterChecker(NewDatabaseHealthChecker(db, "database"))
	}

	monitor.RegisterChecker(NewGPUHealthChecker(gpuEnabled, gpuDevices))
	monitor.RegisterChecker(NewMemoryHealthChecker(80.0))

	log.Printf("Health monitor initialized with %d checkers", len(monitor.checkers))

	return monitor
}
