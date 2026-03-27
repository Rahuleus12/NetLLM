package operations

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// MaintenanceManager handles maintenance mode operations
type MaintenanceManager struct {
	config      *MaintenanceConfig
	logger      *zap.SugaredLogger
	state       *MaintenanceState
	scheduler   *MaintenanceScheduler
	hooks       *MaintenanceHooks
	metrics     *MaintenanceMetrics
	notifier    *MaintenanceNotifier
	healthCheck *MaintenanceHealthChecker
	mu          sync.RWMutex
	running     bool
	cancelFunc  context.CancelFunc
}

// MaintenanceConfig holds maintenance configuration
type MaintenanceConfig struct {
	Enabled                    bool                   `yaml:"enabled" json:"enabled"`
	Mode                       MaintenanceMode        `yaml:"mode" json:"mode"`
	Message                    string                 `yaml:"message" json:"message"`
	GracePeriod                time.Duration          `yaml:"grace_period" json:"grace_period"`
	MaxDuration                time.Duration          `yaml:"max_duration" json:"max_duration"`
	AllowHealthChecks          bool                   `yaml:"allow_health_checks" json:"allow_health_checks"`
	AllowMetrics               bool                   `yaml:"allow_metrics" json:"allow_metrics"`
	DrainConnections           bool                   `yaml:"drain_connections" json:"drain_connections"`
	DrainTimeout               time.Duration          `yaml:"drain_timeout" json:"drain_timeout"`
	AutoEnable                 bool                   `yaml:"auto_enable" json:"auto_enable"`
	AutoDisable                bool                   `yaml:"auto_disable" json:"auto_disable"`
	ScheduledWindows           []MaintenanceWindow    `yaml:"scheduled_windows" json:"scheduled_windows"`
	PreMaintenanceHooks        []MaintenanceHook      `yaml:"pre_maintenance_hooks" json:"pre_maintenance_hooks"`
	PostMaintenanceHooks       []MaintenanceHook      `yaml:"post_maintenance_hooks" json:"post_maintenance_hooks"`
	EmergencyContacts          []string               `yaml:"emergency_contacts" json:"emergency_contacts"`
	NotificationConfig         NotificationConfig     `yaml:"notification_config" json:"notification_config"`
	AllowedIPs                 []string               `yaml:"allowed_ips" json:"allowed_ips"`
	AllowedPaths               []string               `yaml:"allowed_paths" json:"allowed_paths"`
	EnableReadonlyMode         bool                   `yaml:"enable_readonly_mode" json:"enable_readonly_mode"`
	DisableBackgroundJobs      bool                   `yaml:"disable_background_jobs" json:"disable_background_jobs"`
	DisableScheduledTasks      bool                   `yaml:"disable_scheduled_tasks" json:"disable_scheduled_tasks"`
	MaxConcurrentMaintenance   int                    `yaml:"max_concurrent_maintenance" json:"max_concurrent_maintenance"`
	RequireApproval            bool                   `yaml:"require_approval" json:"require_approval"`
	ApprovalTimeout            time.Duration          `yaml:"approval_timeout" json:"approval_timeout"`
}

// MaintenanceMode defines the type of maintenance
type MaintenanceMode string

const (
	MaintenanceModeScheduled   MaintenanceMode = "scheduled"
	MaintenanceModeEmergency   MaintenanceMode = "emergency"
	MaintenanceModeRolling     MaintenanceMode = "rolling"
	MaintenanceModeReadonly    MaintenanceMode = "readonly"
	MaintenanceModeDegraded    MaintenanceMode = "degraded"
)

// MaintenanceStatus defines the status of maintenance
type MaintenanceStatus string

const (
	MaintenanceStatusInactive    MaintenanceStatus = "inactive"
	MaintenanceStatusPending     MaintenanceStatus = "pending"
	MaintenanceStatusActivating  MaintenanceStatus = "activating"
	MaintenanceStatusActive      MaintenanceStatus = "active"
	MaintenanceStatusDraining    MaintenanceStatus = "draining"
	MaintenanceStatusDeactivating MaintenanceStatus = "deactivating"
	MaintenanceStatusCompleted   MaintenanceStatus = "completed"
	MaintenanceStatusFailed      MaintenanceStatus = "failed"
	MaintenanceStatusCancelled   MaintenanceStatus = "cancelled"
)

// MaintenanceWindow represents a scheduled maintenance window
type MaintenanceWindow struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	StartTime   time.Time     `json:"start_time"`
	EndTime     time.Time     `json:"end_time"`
	Duration    time.Duration `json:"duration"`
	Recurrence  string        `json:"recurrence,omitempty"` // cron expression for recurring maintenance
	Timezone    string        `json:"timezone"`
	Enabled     bool          `json:"enabled"`
	AutoStart   bool          `json:"auto_start"`
	AutoEnd     bool          `json:"auto_end"`
	Tags        []string      `json:"tags,omitempty"`
}

// MaintenanceHook represents a maintenance hook
type MaintenanceHook struct {
	Name        string            `yaml:"name" json:"name"`
	Type        HookType          `yaml:"type" json:"type"`
	Command     string            `yaml:"command" json:"command"`
	Args        []string          `yaml:"args" json:"args"`
	Env         map[string]string `yaml:"env" json:"env"`
	Timeout     time.Duration     `yaml:"timeout" json:"timeout"`
	IgnoreError bool              `yaml:"ignore_error" json:"ignore_error"`
}

// MaintenanceState represents the current maintenance state
type MaintenanceState struct {
	ID               string                 `json:"id"`
	Status           MaintenanceStatus      `json:"status"`
	Mode             MaintenanceMode        `json:"mode"`
	Reason           string                 `json:"reason"`
	Message          string                 `json:"message"`
	StartTime        time.Time              `json:"start_time"`
	EndTime          *time.Time             `json:"end_time,omitempty"`
	ExpectedDuration time.Duration          `json:"expected_duration"`
	ActualDuration   time.Duration          `json:"actual_duration"`
	Progress         float64                `json:"progress"`
	EnabledBy        string                 `json:"enabled_by"`
	DisabledBy       string                 `json:"disabled_by,omitempty"`
	ApprovedBy       string                 `json:"approved_by,omitempty"`
	Tasks            []MaintenanceTask      `json:"tasks,omitempty"`
	ActiveConnections int                   `json:"active_connections"`
	DrainedConnections int                  `json:"drained_connections"`
	Errors           []MaintenanceError     `json:"errors,omitempty"`
	Warnings         []string               `json:"warnings,omitempty"`
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
	Window           *MaintenanceWindow     `json:"window,omitempty"`
}

// MaintenanceTask represents a task performed during maintenance
type MaintenanceTask struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Status      TaskStatus        `json:"status"`
	StartTime   time.Time         `json:"start_time"`
	EndTime     *time.Time        `json:"end_time,omitempty"`
	Duration    time.Duration     `json:"duration"`
	Progress    float64           `json:"progress"`
	Error       string            `json:"error,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// TaskStatus defines the status of a maintenance task
type TaskStatus string

const (
	TaskStatusPending    TaskStatus = "pending"
	TaskStatusRunning    TaskStatus = "running"
	TaskStatusCompleted  TaskStatus = "completed"
	TaskStatusFailed     TaskStatus = "failed"
	TaskStatusSkipped    TaskStatus = "skipped"
	TaskStatusCancelled  TaskStatus = "cancelled"
)

// MaintenanceError represents an error during maintenance
type MaintenanceError struct {
	Time    time.Time `json:"time"`
	Message string    `json:"message"`
	Code    string    `json:"code,omitempty"`
	Details string    `json:"details,omitempty"`
}

// MaintenanceScheduler schedules maintenance windows
type MaintenanceScheduler struct {
	windows map[string]*MaintenanceWindow
	running bool
	mu      sync.RWMutex
	logger  *zap.SugaredLogger
}

// NewMaintenanceScheduler creates a new maintenance scheduler
func NewMaintenanceScheduler(logger *zap.SugaredLogger) *MaintenanceScheduler {
	return &MaintenanceScheduler{
		windows: make(map[string]*MaintenanceWindow),
		logger:  logger,
	}
}

// AddWindow adds a maintenance window
func (s *MaintenanceScheduler) AddWindow(window *MaintenanceWindow) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if window.ID == "" {
		window.ID = uuid.New().String()
	}

	s.windows[window.ID] = window
	s.logger.Infow("Added maintenance window",
		"window_id", window.ID,
		"name", window.Name,
		"start_time", window.StartTime,
		"duration", window.Duration,
	)
}

// RemoveWindow removes a maintenance window
func (s *MaintenanceScheduler) RemoveWindow(windowID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.windows, windowID)
	s.logger.Infow("Removed maintenance window", "window_id", windowID)
}

// GetWindow retrieves a maintenance window
func (s *MaintenanceScheduler) GetWindow(windowID string) (*MaintenanceWindow, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	window, exists := s.windows[windowID]
	if !exists {
		return nil, false
	}

	// Return a copy
	copy := *window
	return &copy, true
}

// ListWindows lists all maintenance windows
func (s *MaintenanceScheduler) ListWindows() []*MaintenanceWindow {
	s.mu.RLock()
	defer s.mu.RUnlock()

	windows := make([]*MaintenanceWindow, 0, len(s.windows))
	for _, window := range s.windows {
		copy := *window
		windows = append(windows, &copy)
	}

	return windows
}

// GetNextWindow returns the next scheduled maintenance window
func (s *MaintenanceScheduler) GetNextWindow() *MaintenanceWindow {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var nextWindow *MaintenanceWindow
	var nextTime time.Time

	for _, window := range s.windows {
		if !window.Enabled {
			continue
		}

		if nextWindow == nil || window.StartTime.Before(nextTime) {
			nextWindow = window
			nextTime = window.StartTime
		}
	}

	if nextWindow == nil {
		return nil
	}

	copy := *nextWindow
	return &copy
}

// GetActiveWindows returns currently active maintenance windows
func (s *MaintenanceScheduler) GetActiveWindows() []*MaintenanceWindow {
	s.mu.RLock()
	defer s.mu.RUnlock()

	now := time.Now()
	active := make([]*MaintenanceWindow, 0)

	for _, window := range s.windows {
		if !window.Enabled {
			continue
		}

		endTime := window.StartTime.Add(window.Duration)
		if now.After(window.StartTime) && now.Before(endTime) {
			copy := *window
			active = append(active, &copy)
		}
	}

	return active
}

// IsInMaintenanceWindow checks if current time is within a maintenance window
func (s *MaintenanceScheduler) IsInMaintenanceWindow() bool {
	return len(s.GetActiveWindows()) > 0
}

// MaintenanceHooks manages maintenance hooks
type MaintenanceHooks struct {
	logger *zap.SugaredLogger
}

// NewMaintenanceHooks creates a new maintenance hooks manager
func NewMaintenanceHooks(logger *zap.SugaredLogger) *MaintenanceHooks {
	return &MaintenanceHooks{
		logger: logger,
	}
}

// Execute executes maintenance hooks
func (h *MaintenanceHooks) Execute(ctx context.Context, hooks []MaintenanceHook, phase string) error {
	for _, hook := range hooks {
		h.logger.Infow("Executing maintenance hook",
			"hook_name", hook.Name,
			"phase", phase,
			"type", hook.Type,
		)

		// In production, this would execute the actual hook
		// For now, we simulate it
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
			// Simulate hook execution
		}

		h.logger.Infow("Maintenance hook completed",
			"hook_name", hook.Name,
			"phase", phase,
		)
	}

	return nil
}

// MaintenanceMetrics tracks maintenance metrics
type MaintenanceMetrics struct {
	TotalMaintenanceEvents    int64
	ScheduledMaintenance      int64
	EmergencyMaintenance      int64
	TotalDuration             time.Duration
	AverageDuration           time.Duration
	TotalConnectionDrains     int64
	FailedMaintenanceEvents   int64
	CancelledMaintenanceEvents int64
	mu                        sync.Mutex
}

// NewMaintenanceMetrics creates new maintenance metrics
func NewMaintenanceMetrics() *MaintenanceMetrics {
	return &MaintenanceMetrics{}
}

// RecordMaintenance records a maintenance event
func (m *MaintenanceMetrics) RecordMaintenance(state *MaintenanceState) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.TotalMaintenanceEvents++
	m.TotalDuration += state.ActualDuration

	switch state.Mode {
	case MaintenanceModeScheduled:
		m.ScheduledMaintenance++
	case MaintenanceModeEmergency:
		m.EmergencyMaintenance++
	}

	if state.Status == MaintenanceStatusFailed {
		m.FailedMaintenanceEvents++
	} else if state.Status == MaintenanceStatusCancelled {
		m.CancelledMaintenanceEvents++
	}

	// Calculate average
	m.AverageDuration = time.Duration(int64(m.TotalDuration) / m.TotalMaintenanceEvents)
}

// RecordDrain records connection drain
func (m *MaintenanceMetrics) RecordDrain(count int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.TotalConnectionDrains += int64(count)
}

// GetMetrics returns current metrics
func (m *MaintenanceMetrics) GetMetrics() map[string]interface{} {
	m.mu.Lock()
	defer m.mu.Unlock()

	return map[string]interface{}{
		"total_maintenance_events":     m.TotalMaintenanceEvents,
		"scheduled_maintenance":        m.ScheduledMaintenance,
		"emergency_maintenance":        m.EmergencyMaintenance,
		"total_duration":               m.TotalDuration.String(),
		"average_duration":             m.AverageDuration.String(),
		"total_connection_drains":      m.TotalConnectionDrains,
		"failed_maintenance_events":    m.FailedMaintenanceEvents,
		"cancelled_maintenance_events": m.CancelledMaintenanceEvents,
	}
}

// MaintenanceNotifier handles maintenance notifications
type MaintenanceNotifier struct {
	config *MaintenanceConfig
	logger *zap.SugaredLogger
}

// NewMaintenanceNotifier creates a new maintenance notifier
func NewMaintenanceNotifier(config *MaintenanceConfig, logger *zap.SugaredLogger) *MaintenanceNotifier {
	return &MaintenanceNotifier{
		config: config,
		logger: logger,
	}
}

// Notify sends maintenance notifications
func (n *MaintenanceNotifier) Notify(ctx context.Context, state *MaintenanceState, event string) error {
	if !n.config.NotificationConfig.Enabled {
		return nil
	}

	n.logger.Infow("Sending maintenance notification",
		"event", event,
		"status", state.Status,
		"mode", state.Mode,
	)

	// In production, this would send actual notifications
	// (email, Slack, PagerDuty, etc.)

	return nil
}

// MaintenanceHealthChecker performs health checks during maintenance
type MaintenanceHealthChecker struct {
	logger *zap.SugaredLogger
}

// NewMaintenanceHealthChecker creates a new health checker
func NewMaintenanceHealthChecker(logger *zap.SugaredLogger) *MaintenanceHealthChecker {
	return &MaintenanceHealthChecker{
		logger: logger,
	}
}

// Check performs health checks
func (hc *MaintenanceHealthChecker) Check(ctx context.Context) (*HealthCheckResult, error) {
	result := &HealthCheckResult{
		Status:    "healthy",
		Timestamp: time.Now(),
		Checks:    make(map[string]string),
	}

	// In production, this would check:
	// - Database connectivity
	// - Redis connectivity
	// - Storage availability
	// - External service dependencies

	result.Checks["database"] = "ok"
	result.Checks["cache"] = "ok"
	result.Checks["storage"] = "ok"

	return result, nil
}

// HealthCheckResult represents a health check result
type HealthCheckResult struct {
	Status    string            `json:"status"`
	Timestamp time.Time         `json:"timestamp"`
	Checks    map[string]string `json:"checks"`
}

// NewMaintenanceManager creates a new maintenance manager
func NewMaintenanceManager(config *MaintenanceConfig, logger *zap.SugaredLogger) *MaintenanceManager {
	return &MaintenanceManager{
		config:      config,
		logger:      logger,
		state:       &MaintenanceState{Status: MaintenanceStatusInactive},
		scheduler:   NewMaintenanceScheduler(logger),
		hooks:       NewMaintenanceHooks(logger),
		metrics:     NewMaintenanceMetrics(),
		notifier:    NewMaintenanceNotifier(config, logger),
		healthCheck: NewMaintenanceHealthChecker(logger),
	}
}

// Initialize initializes the maintenance manager
func (mm *MaintenanceManager) Initialize(ctx context.Context) error {
	mm.logger.Infow("Initializing maintenance manager")

	// Load scheduled windows
	for i := range mm.config.ScheduledWindows {
		mm.scheduler.AddWindow(&mm.config.ScheduledWindows[i])
	}

	// Start scheduler if auto-enable is enabled
	if mm.config.AutoEnable {
		go mm.runScheduler(ctx)
	}

	mm.logger.Infow("Maintenance manager initialized",
		"scheduled_windows", len(mm.scheduler.ListWindows()),
		"auto_enable", mm.config.AutoEnable,
	)

	return nil
}

// runScheduler runs the maintenance scheduler
func (mm *MaintenanceManager) runScheduler(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			mm.checkScheduledWindows(ctx)
		}
	}
}

// checkScheduledWindows checks for scheduled maintenance windows
func (mm *MaintenanceManager) checkScheduledWindows(ctx context.Context) {
	windows := mm.scheduler.GetActiveWindows()

	for _, window := range windows {
		if window.AutoStart && !mm.IsActive() {
			mm.logger.Infow("Auto-starting scheduled maintenance",
				"window_id", window.ID,
				"window_name", window.Name,
			)

			_, err := mm.Enable(ctx, &EnableMaintenanceRequest{
				Mode:     MaintenanceModeScheduled,
				Reason:   window.Name,
				Message:  window.Description,
				Duration: window.Duration,
				Window:   window,
			})

			if err != nil {
				mm.logger.Errorw("Failed to auto-start maintenance",
					"window_id", window.ID,
					"error", err,
				)
			}
		}
	}
}

// Enable enables maintenance mode
func (mm *MaintenanceManager) Enable(ctx context.Context, req *EnableMaintenanceRequest) (*MaintenanceState, error) {
	mm.mu.Lock()
	if mm.running {
		mm.mu.Unlock()
		return nil, fmt.Errorf("maintenance operation already in progress")
	}

	ctx, cancel := context.WithCancel(ctx)
	mm.cancelFunc = cancel
	mm.running = true
	mm.mu.Unlock()

	defer func() {
		mm.mu.Lock()
		mm.running = false
		mm.cancelFunc = nil
		mm.mu.Unlock()
	}()

	// Create maintenance state
	state := &MaintenanceState{
		ID:               uuid.New().String(),
		Status:           MaintenanceStatusPending,
		Mode:             req.Mode,
		Reason:           req.Reason,
		Message:          req.Message,
		StartTime:        time.Now(),
		ExpectedDuration: req.Duration,
		EnabledBy:        req.EnabledBy,
		ApprovedBy:       req.ApprovedBy,
		Tasks:            []MaintenanceTask{},
		Window:           req.Window,
		Metadata:         req.Metadata,
	}

	mm.state = state

	mm.logger.Infow("Enabling maintenance mode",
		"maintenance_id", state.ID,
		"mode", state.Mode,
		"reason", state.Reason,
		"duration", state.ExpectedDuration,
	)

	// Send notification
	if err := mm.notifier.Notify(ctx, state, "maintenance_starting"); err != nil {
		mm.logger.Warnw("Failed to send notification", "error", err)
	}

	// Execute pre-maintenance hooks
	state.Status = MaintenanceStatusActivating
	if err := mm.hooks.Execute(ctx, mm.config.PreMaintenanceHooks, "pre"); err != nil {
		state.Status = MaintenanceStatusFailed
		state.Errors = append(state.Errors, MaintenanceError{
			Time:    time.Now(),
			Message: err.Error(),
			Code:    "PRE_HOOK_FAILED",
		})
		return state, fmt.Errorf("pre-maintenance hooks failed: %w", err)
	}

	// Drain connections if configured
	if mm.config.DrainConnections {
		state.Status = MaintenanceStatusDraining
		if err := mm.drainConnections(ctx, state); err != nil {
			mm.logger.Warnw("Connection drain failed", "error", err)
			state.Warnings = append(state.Warnings, fmt.Sprintf("Connection drain: %v", err))
		}
	}

	// Disable background jobs if configured
	if mm.config.DisableBackgroundJobs {
		mm.logger.Infow("Disabling background jobs")
		// In production, this would signal background job workers to stop
	}

	// Disable scheduled tasks if configured
	if mm.config.DisableScheduledTasks {
		mm.logger.Infow("Disabling scheduled tasks")
		// In production, this would stop the task scheduler
	}

	// Enable readonly mode if configured
	if mm.config.EnableReadonlyMode {
		mm.logger.Infow("Enabling readonly mode")
		// In production, this would configure the database/API for readonly access
	}

	// Activate maintenance mode
	state.Status = MaintenanceStatusActive
	state.Progress = 0

	mm.logger.Infow("Maintenance mode enabled",
		"maintenance_id", state.ID,
		"status", state.Status,
	)

	// Send notification
	if err := mm.notifier.Notify(ctx, state, "maintenance_enabled"); err != nil {
		mm.logger.Warnw("Failed to send notification", "error", err)
	}

	// Start auto-disable timer if configured
	if mm.config.AutoDisable && state.ExpectedDuration > 0 {
		go mm.autoDisableTimer(ctx, state)
	}

	return state, nil
}

// EnableMaintenanceRequest represents a request to enable maintenance mode
type EnableMaintenanceRequest struct {
	Mode       MaintenanceMode        `json:"mode"`
	Reason     string                 `json:"reason"`
	Message    string                 `json:"message"`
	Duration   time.Duration          `json:"duration"`
	EnabledBy  string                 `json:"enabled_by"`
	ApprovedBy string                 `json:"approved_by,omitempty"`
	Window     *MaintenanceWindow     `json:"window,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// drainConnections drains active connections
func (mm *MaintenanceManager) drainConnections(ctx context.Context, state *MaintenanceState) error {
	mm.logger.Infow("Starting connection drain",
		"timeout", mm.config.DrainTimeout,
	)

	// In production, this would:
	// 1. Stop accepting new connections
	// 2. Wait for existing connections to complete
	// 3. Force close remaining connections after timeout

	// Simulate connection draining
	drained := 0
	active := 10 // Simulated active connections

	for i := 0; i < active; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
			drained++
			state.DrainedConnections = drained
			state.ActiveConnections = active - drained
		}
	}

	mm.metrics.RecordDrain(drained)

	mm.logger.Infow("Connection drain completed",
		"drained", drained,
	)

	return nil
}

// autoDisableTimer automatically disables maintenance after duration
func (mm *MaintenanceManager) autoDisableTimer(ctx context.Context, state *MaintenanceState) {
	timer := time.NewTimer(state.ExpectedDuration)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return
	case <-timer.C:
		mm.logger.Infow("Auto-disabling maintenance mode",
			"maintenance_id", state.ID,
		)

		_, err := mm.Disable(ctx, &DisableMaintenanceRequest{
			DisabledBy: "system",
			Reason:     "auto-disable after duration",
		})

		if err != nil {
			mm.logger.Errorw("Failed to auto-disable maintenance", "error", err)
		}
	}
}

// Disable disables maintenance mode
func (mm *MaintenanceManager) Disable(ctx context.Context, req *DisableMaintenanceRequest) (*MaintenanceState, error) {
	mm.mu.Lock()
	if mm.running {
		mm.mu.Unlock()
		return nil, fmt.Errorf("maintenance operation already in progress")
	}

	mm.running = true
	mm.mu.Unlock()

	defer func() {
		mm.mu.Lock()
		mm.running = false
		mm.mu.Unlock()
	}()

	state := mm.state

	if state.Status != MaintenanceStatusActive {
		return nil, fmt.Errorf("maintenance mode is not active (current status: %s)", state.Status)
	}

	mm.logger.Infow("Disabling maintenance mode",
		"maintenance_id", state.ID,
		"disabled_by", req.DisabledBy,
	)

	// Send notification
	if err := mm.notifier.Notify(ctx, state, "maintenance_ending"); err != nil {
		mm.logger.Warnw("Failed to send notification", "error", err)
	}

	// Update status
	state.Status = MaintenanceStatusDeactivating

	// Re-enable background jobs
	if mm.config.DisableBackgroundJobs {
		mm.logger.Infow("Re-enabling background jobs")
		// In production, this would restart background job workers
	}

	// Re-enable scheduled tasks
	if mm.config.DisableScheduledTasks {
		mm.logger.Infow("Re-enabling scheduled tasks")
		// In production, this would restart the task scheduler
	}

	// Disable readonly mode
	if mm.config.EnableReadonlyMode {
		mm.logger.Infow("Disabling readonly mode")
		// In production, this would restore read-write access
	}

	// Execute post-maintenance hooks
	if err := mm.hooks.Execute(ctx, mm.config.PostMaintenanceHooks, "post"); err != nil {
		mm.logger.Warnw("Post-maintenance hooks failed", "error", err)
		state.Warnings = append(state.Warnings, fmt.Sprintf("Post-hooks: %v", err))
	}

	// Update state
	now := time.Now()
	state.Status = MaintenanceStatusCompleted
	state.EndTime = &now
	state.ActualDuration = now.Sub(state.StartTime)
	state.DisabledBy = req.DisabledBy
	state.Progress = 100

	// Record metrics
	mm.metrics.RecordMaintenance(state)

	mm.logger.Infow("Maintenance mode disabled",
		"maintenance_id", state.ID,
		"duration", state.ActualDuration,
	)

	// Send notification
	if err := mm.notifier.Notify(ctx, state, "maintenance_completed"); err != nil {
		mm.logger.Warnw("Failed to send notification", "error", err)
	}

	return state, nil
}

// DisableMaintenanceRequest represents a request to disable maintenance mode
type DisableMaintenanceRequest struct {
	DisabledBy string `json:"disabled_by"`
	Reason     string `json:"reason,omitempty"`
}

// Cancel cancels an active maintenance operation
func (mm *MaintenanceManager) Cancel(reason string) error {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	if !mm.running {
		return fmt.Errorf("no maintenance operation in progress")
	}

	if mm.cancelFunc != nil {
		mm.cancelFunc()
	}

	mm.state.Status = MaintenanceStatusCancelled
	mm.state.Warnings = append(mm.state.Warnings, fmt.Sprintf("Cancelled: %s", reason))

	mm.logger.Infow("Maintenance operation cancelled",
		"maintenance_id", mm.state.ID,
		"reason", reason,
	)

	return nil
}

// IsActive returns whether maintenance mode is active
func (mm *MaintenanceManager) IsActive() bool {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	return mm.state.Status == MaintenanceStatusActive ||
		mm.state.Status == MaintenanceStatusActivating ||
		mm.state.Status == MaintenanceStatusDraining
}

// GetState returns the current maintenance state
func (mm *MaintenanceManager) GetState() *MaintenanceState {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	// Return a copy
	copy := *mm.state
	return &copy
}

// GetStatus returns the current maintenance status
func (mm *MaintenanceManager) GetStatus() MaintenanceStatus {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	return mm.state.Status
}

// UpdateProgress updates maintenance progress
func (mm *MaintenanceManager) UpdateProgress(progress float64, message string) {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	mm.state.Progress = progress
	if message != "" {
		mm.state.Message = message
	}

	mm.logger.Debugw("Updated maintenance progress",
		"maintenance_id", mm.state.ID,
		"progress", progress,
		"message", message,
	)
}

// AddTask adds a maintenance task
func (mm *MaintenanceManager) AddTask(task *MaintenanceTask) {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	if task.ID == "" {
		task.ID = uuid.New().String()
	}

	task.StartTime = time.Now()
	task.Status = TaskStatusRunning

	mm.state.Tasks = append(mm.state.Tasks, *task)

	mm.logger.Infow("Added maintenance task",
		"maintenance_id", mm.state.ID,
		"task_id", task.ID,
		"task_name", task.Name,
	)
}

// UpdateTask updates a maintenance task
func (mm *MaintenanceManager) UpdateTask(taskID string, status TaskStatus, progress float64, errMsg string) error {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	for i := range mm.state.Tasks {
		if mm.state.Tasks[i].ID == taskID {
			mm.state.Tasks[i].Status = status
			mm.state.Tasks[i].Progress = progress

			if errMsg != "" {
				mm.state.Tasks[i].Error = errMsg
			}

			if status == TaskStatusCompleted || status == TaskStatusFailed || status == TaskStatusCancelled {
				now := time.Now()
				mm.state.Tasks[i].EndTime = &now
				mm.state.Tasks[i].Duration = now.Sub(mm.state.Tasks[i].StartTime)
			}

			return nil
		}
	}

	return fmt.Errorf("task %s not found", taskID)
}

// ScheduleWindow schedules a maintenance window
func (mm *MaintenanceManager) ScheduleWindow(window *MaintenanceWindow) error {
	if window.StartTime.Before(time.Now()) {
		return fmt.Errorf("cannot schedule maintenance window in the past")
	}

	if window.Duration <= 0 {
		return fmt.Errorf("maintenance window duration must be positive")
	}

	mm.scheduler.AddWindow(window)

	mm.logger.Infow("Scheduled maintenance window",
		"window_id", window.ID,
		"name", window.Name,
		"start_time", window.StartTime,
		"duration", window.Duration,
	)

	return nil
}

// CancelWindow cancels a scheduled maintenance window
func (mm *MaintenanceManager) CancelWindow(windowID string) error {
	mm.scheduler.RemoveWindow(windowID)

	mm.logger.Infow("Cancelled maintenance window", "window_id", windowID)

	return nil
}

// ListWindows lists all scheduled maintenance windows
func (mm *MaintenanceManager) ListWindows() []*MaintenanceWindow {
	return mm.scheduler.ListWindows()
}

// GetNextWindow returns the next scheduled maintenance window
func (mm *MaintenanceManager) GetNextWindow() *MaintenanceWindow {
	return mm.scheduler.GetNextWindow()
}

// CheckHealth performs a health check during maintenance
func (mm *MaintenanceManager) CheckHealth(ctx context.Context) (*HealthCheckResult, error) {
	return mm.healthCheck.Check(ctx)
}

// GetMetrics returns maintenance metrics
func (mm *MaintenanceManager) GetMetrics() map[string]interface{} {
	return mm.metrics.GetMetrics()
}

// IsAllowed checks if a request is allowed during maintenance
func (mm *MaintenanceManager) IsAllowed(ip string, path string) bool {
	if !mm.IsActive() {
		return true
	}

	// Check allowed IPs
	if len(mm.config.AllowedIPs) > 0 {
		allowed := false
		for _, allowedIP := range mm.config.AllowedIPs {
			if ip == allowedIP {
				allowed = true
				break
			}
		}
		if !allowed {
			return false
		}
	}

	// Check allowed paths
	if len(mm.config.AllowedPaths) > 0 {
		allowed := false
		for _, allowedPath := range mm.config.AllowedPaths {
			if path == allowedPath {
				allowed = true
				break
			}
		}
		if !allowed {
			return false
		}
	}

	return true
}

// ShouldAllowHealthCheck returns whether health checks should be allowed
func (mm *MaintenanceManager) ShouldAllowHealthCheck() bool {
	return mm.config.AllowHealthChecks
}

// ShouldAllowMetrics returns whether metrics endpoint should be allowed
func (mm *MaintenanceManager) ShouldAllowMetrics() bool {
	return mm.config.AllowMetrics
}

// GetMessage returns the current maintenance message
func (mm *MaintenanceManager) GetMessage() string {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	return mm.state.Message
}

// SetMessage sets the maintenance message
func (mm *MaintenanceManager) SetMessage(message string) {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	mm.state.Message = message

	mm.logger.Infow("Updated maintenance message", "message", message)
}

// AddError adds an error to the maintenance state
func (mm *MaintenanceManager) AddError(err MaintenanceError) {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	mm.state.Errors = append(mm.state.Errors, err)

	mm.logger.Errorw("Added maintenance error",
		"maintenance_id", mm.state.ID,
		"error_code", err.Code,
		"error_message", err.Message,
	)
}

// AddWarning adds a warning to the maintenance state
func (mm *MaintenanceManager) AddWarning(warning string) {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	mm.state.Warnings = append(mm.state.Warnings, warning)

	mm.logger.Warnw("Added maintenance warning",
		"maintenance_id", mm.state.ID,
		"warning", warning,
	)
}

// Export exports maintenance state to JSON
func (mm *MaintenanceManager) Export() ([]byte, error) {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	export := struct {
		State     *MaintenanceState      `json:"state"`
		Windows   []*MaintenanceWindow   `json:"scheduled_windows"`
		Metrics   map[string]interface{} `json:"metrics"`
		ExportTime time.Time             `json:"export_time"`
	}{
		State:     mm.state,
		Windows:   mm.scheduler.ListWindows(),
		Metrics:   mm.metrics.GetMetrics(),
		ExportTime: time.Now(),
	}

	return json.MarshalIndent(export, "", "  ")
}

// Extend extends the maintenance duration
func (mm *MaintenanceManager) Extend(additionalDuration time.Duration) error {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	if mm.state.Status != MaintenanceStatusActive {
		return fmt.Errorf("maintenance mode is not active")
	}

	mm.state.ExpectedDuration += additionalDuration

	mm.logger.Infow("Extended maintenance duration",
		"maintenance_id", mm.state.ID,
		"additional_duration", additionalDuration,
		"new_total_duration", mm.state.ExpectedDuration,
	)

	return nil
}

// GetRemainingDuration returns the remaining maintenance duration
func (mm *MaintenanceManager) GetRemainingDuration() time.Duration {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	if mm.state.Status != MaintenanceStatusActive {
		return 0
	}

	elapsed := time.Since(mm.state.StartTime)
	remaining := mm.state.ExpectedDuration - elapsed

	if remaining < 0 {
		return 0
	}

	return remaining
}

// IsEmergency returns whether current maintenance is emergency
func (mm *MaintenanceManager) IsEmergency() bool {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	return mm.state.Mode == MaintenanceModeEmergency
}

// IsScheduled returns whether current maintenance is scheduled
func (mm *MaintenanceManager) IsScheduled() bool {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	return mm.state.Mode == MaintenanceModeScheduled
}

// GetMaintenanceID returns the current maintenance ID
func (mm *MaintenanceManager) GetMaintenanceID() string {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	return mm.state.ID
}

// GetStartTime returns the maintenance start time
func (mm *MaintenanceManager) GetStartTime() time.Time {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	return mm.state.StartTime
}

// GetDuration returns the actual maintenance duration
func (mm *MaintenanceManager) GetDuration() time.Duration {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	if mm.state.EndTime != nil {
		return mm.state.EndTime.Sub(mm.state.StartTime)
	}

	return time.Since(mm.state.StartTime)
}

// RequestApproval requests approval for maintenance (if required)
func (mm *MaintenanceManager) RequestApproval(ctx context.Context, req *EnableMaintenanceRequest) error {
	if !mm.config.RequireApproval {
		return nil
	}

	mm.logger.Infow("Requesting maintenance approval",
		"mode", req.Mode,
		"reason", req.Reason,
	)

	// In production, this would:
	// 1. Send approval request to designated approvers
	// 2. Wait for approval or timeout
	// 3. Return error if not approved

	// Simulate approval process
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(1 * time.Second):
		// Simulate approval
	}

	return nil
}
