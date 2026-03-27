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

// DeploymentManager handles automated deployment operations
type DeploymentManager struct {
	config      *DeploymentConfig
	logger      *zap.SugaredLogger
	history     *DeploymentHistory
	validator   *DeploymentValidator
	healthCheck *HealthChecker
	rollback    *RollbackManager
	metrics     *DeploymentMetrics
	mu          sync.RWMutex
	activeDep   *ActiveDeployment
}

// DeploymentConfig holds deployment configuration
type DeploymentConfig struct {
	Enabled              bool                   `yaml:"enabled" json:"enabled"`
	Strategy             DeploymentStrategy     `yaml:"strategy" json:"strategy"`
	Timeout              time.Duration          `yaml:"timeout" json:"timeout"`
	RollbackOnFailure    bool                   `yaml:"rollback_on_failure" json:"rollback_on_failure"`
	HealthCheckInterval  time.Duration          `yaml:"health_check_interval" json:"health_check_interval"`
	HealthCheckTimeout   time.Duration          `yaml:"health_check_timeout" json:"health_check_timeout"`
	MaxRetryAttempts     int                    `yaml:"max_retry_attempts" json:"max_retry_attempts"`
	RetryDelay           time.Duration          `yaml:"retry_delay" json:"retry_delay"`
	DryRun               bool                   `yaml:"dry_run" json:"dry_run"`
	PreDeployHooks       []DeploymentHook       `yaml:"pre_deploy_hooks" json:"pre_deploy_hooks"`
	PostDeployHooks      []DeploymentHook       `yaml:"post_deploy_hooks" json:"post_deploy_hooks"`
	PreRollbackHooks     []DeploymentHook       `yaml:"pre_rollback_hooks" json:"pre_rollback_hooks"`
	PostRollbackHooks    []DeploymentHook       `yaml:"post_rollback_hooks" json:"post_rollback_hooks"`
	NotificationConfig   NotificationConfig     `yaml:"notification_config" json:"notification_config"`
	CanaryConfig         *CanaryConfig          `yaml:"canary_config,omitempty" json:"canary_config,omitempty"`
	BlueGreenConfig      *BlueGreenConfig       `yaml:"blue_green_config,omitempty" json:"blue_green_config,omitempty"`
	RollingConfig        *RollingConfig         `yaml:"rolling_config,omitempty" json:"rolling_config,omitempty"`
}

// DeploymentStrategy defines the deployment strategy type
type DeploymentStrategy string

const (
	StrategyRolling    DeploymentStrategy = "rolling"
	StrategyBlueGreen  DeploymentStrategy = "blue-green"
	StrategyCanary     DeploymentStrategy = "canary"
	StrategyRecreate   DeploymentStrategy = "recreate"
)

// DeploymentHook represents a deployment hook
type DeploymentHook struct {
	Name        string            `yaml:"name" json:"name"`
	Type        HookType          `yaml:"type" json:"type"`
	Command     string            `yaml:"command" json:"command"`
	Args        []string          `yaml:"args" json:"args"`
	Env         map[string]string `yaml:"env" json:"env"`
	Timeout     time.Duration     `yaml:"timeout" json:"timeout"`
	IgnoreError bool              `yaml:"ignore_error" json:"ignore_error"`
}

// HookType defines the type of deployment hook
type HookType string

const (
	HookTypeExec    HookType = "exec"
	HookTypeHTTP    HookType = "http"
	HookTypeWebhook HookType = "webhook"
)

// CanaryConfig holds canary deployment configuration
type CanaryConfig struct {
	InitialReplicas      int           `yaml:"initial_replicas" json:"initial_replicas"`
	IncrementReplicas    int           `yaml:"increment_replicas" json:"increment_replicas"`
	IncrementInterval    time.Duration `yaml:"increment_interval" json:"increment_interval"`
	SuccessThreshold     float64       `yaml:"success_threshold" json:"success_threshold"`
	FailureThreshold     float64       `yaml:"failure_threshold" json:"failure_threshold"`
	AnalysisDuration     time.Duration `yaml:"analysis_duration" json:"analysis_duration"`
	Metrics              []string      `yaml:"metrics" json:"metrics"`
	PauseOnFailure       bool          `yaml:"pause_on_failure" json:"pause_on_failure"`
	AutoPromote          bool          `yaml:"auto_promote" json:"auto_promote"`
	ManualGate           bool          `yaml:"manual_gate" json:"manual_gate"`
}

// BlueGreenConfig holds blue-green deployment configuration
type BlueGreenConfig struct {
	ActiveColor      string        `yaml:"active_color" json:"active_color"`
	WaitForPromotion bool          `yaml:"wait_for_promotion" json:"wait_for_promotion"`
	AutoPromote      bool          `yaml:"auto_promote" json:"auto_promote"`
	PromotionDelay   time.Duration `yaml:"promotion_delay" json:"promotion_delay"`
	KeepOldVersion   bool          `yaml:"keep_old_version" json:"keep_old_version"`
	HealthCheckDelay time.Duration `yaml:"health_check_delay" json:"health_check_delay"`
}

// RollingConfig holds rolling deployment configuration
type RollingConfig struct {
	MaxUnavailable   int           `yaml:"max_unavailable" json:"max_unavailable"`
	MaxSurge         int           `yaml:"max_surge" json:"max_surge"`
	Interval         time.Duration `yaml:"interval" json:"interval"`
	PauseBetweenPods bool          `yaml:"pause_between_pods" json:"pause_between_pods"`
}

// Deployment represents a deployment instance
type Deployment struct {
	ID                 string                 `json:"id"`
	Name               string                 `json:"name"`
	Namespace          string                 `json:"namespace"`
	Strategy           DeploymentStrategy     `json:"strategy"`
	Image              string                 `json:"image"`
	Tag                string                 `json:"tag"`
	Replicas           int                    `json:"replicas"`
	Status             DeploymentStatus       `json:"status"`
	Phase              DeploymentPhase        `json:"phase"`
	Progress           float64                `json:"progress"`
	StartTime          time.Time              `json:"start_time"`
	EndTime            *time.Time             `json:"end_time,omitempty"`
	Duration           time.Duration          `json:"duration"`
	PreviousVersion    string                 `json:"previous_version"`
	CurrentVersion     string                 `json:"current_version"`
	RollbackVersion    string                 `json:"rollback_version,omitempty"`
	HealthStatus       HealthStatus           `json:"health_status"`
	HealthChecks       []HealthCheckResult    `json:"health_checks"`
	Errors             []DeploymentError      `json:"errors,omitempty"`
	Warnings           []string               `json:"warnings,omitempty"`
	Metadata           map[string]interface{} `json:"metadata,omitempty"`
	Labels             map[string]string      `json:"labels,omitempty"`
	Annotations        map[string]string      `json:"annotations,omitempty"`
	CanaryStatus       *CanaryStatus          `json:"canary_status,omitempty"`
	BlueGreenStatus    *BlueGreenStatus       `json:"blue_green_status,omitempty"`
	RollingStatus      *RollingStatus         `json:"rolling_status,omitempty"`
}

// DeploymentStatus defines the status of a deployment
type DeploymentStatus string

const (
	StatusPending    DeploymentStatus = "pending"
	StatusRunning    DeploymentStatus = "running"
	StatusSucceeded  DeploymentStatus = "succeeded"
	StatusFailed     DeploymentStatus = "failed"
	StatusCancelled  DeploymentStatus = "cancelled"
	StatusRollback   DeploymentStatus = "rollback"
	StatusPaused     DeploymentStatus = "paused"
)

// DeploymentPhase defines the current phase of deployment
type DeploymentPhase string

const (
	PhaseInitializing   DeploymentPhase = "initializing"
	PhasePreHooks       DeploymentPhase = "pre_hooks"
	PhaseDeploying      DeploymentPhase = "deploying"
	PhaseHealthCheck    DeploymentPhase = "health_check"
	PhasePostHooks      DeploymentPhase = "post_hooks"
	PhaseCompleted      DeploymentPhase = "completed"
	PhaseRollingBack    DeploymentPhase = "rolling_back"
	PhaseFailed         DeploymentPhase = "failed"
)

// HealthStatus represents the health status of a deployment
type HealthStatus struct {
	Healthy          bool              `json:"healthy"`
	ReadyReplicas    int               `json:"ready_replicas"`
	TotalReplicas    int               `json:"total_replicas"`
	AvailableReplicas int              `json:"available_replicas"`
	LastCheckTime    time.Time         `json:"last_check_time"`
	Checks           []HealthCheck     `json:"checks"`
}

// HealthCheck represents a health check configuration
type HealthCheck struct {
	Name     string        `json:"name"`
	Type     string        `json:"type"` // http, tcp, exec, grpc
	Endpoint string        `json:"endpoint,omitempty"`
	Port     int           `json:"port,omitempty"`
	Path     string        `json:"path,omitempty"`
	Timeout  time.Duration `json:"timeout"`
	Interval time.Duration `json:"interval"`
}

// HealthCheckResult represents the result of a health check
type HealthCheckResult struct {
	Name      string        `json:"name"`
	Status    string        `json:"status"` // passed, failed, warning
	Message   string        `json:"message"`
	Timestamp time.Time     `json:"timestamp"`
	Duration  time.Duration `json:"duration"`
}

// DeploymentError represents an error during deployment
type DeploymentError struct {
	Time    time.Time `json:"time"`
	Message string    `json:"message"`
	Code    string    `json:"code,omitempty"`
	Details string    `json:"details,omitempty"`
}

// CanaryStatus holds status for canary deployment
type CanaryStatus struct {
	CurrentWeight     float64   `json:"current_weight"`
	TargetWeight      float64   `json:"target_weight"`
	StableReplicas    int       `json:"stable_replicas"`
	CanaryReplicas    int       `json:"canary_replicas"`
	AnalysisResult    string    `json:"analysis_result"`
	Promoted          bool      `json:"promoted"`
	LastAnalysisTime  time.Time `json:"last_analysis_time"`
	SuccessRate       float64   `json:"success_rate"`
	ErrorRate         float64   `json:"error_rate"`
}

// BlueGreenStatus holds status for blue-green deployment
type BlueGreenStatus struct {
	ActiveColor    string    `json:"active_color"`
	PreviewColor   string    `json:"preview_color"`
	ActiveReady    bool      `json:"active_ready"`
	PreviewReady   bool      `json:"preview_ready"`
	Promoted       bool      `json:"promoted"`
	PromotionTime  time.Time `json:"promotion_time,omitempty"`
}

// RollingStatus holds status for rolling deployment
type RollingStatus struct {
	UpdatedReplicas   int `json:"updated_replicas"`
	ReadyReplicas     int `json:"ready_replicas"`
	AvailableReplicas int `json:"available_replicas"`
	UnavailableReplicas int `json:"unavailable_replicas"`
}

// ActiveDeployment tracks currently active deployment
type ActiveDeployment struct {
	Deployment *Deployment
	CancelFunc context.CancelFunc
	StartTime  time.Time
}

// DeploymentHistory manages deployment history
type DeploymentHistory struct {
	deployments map[string]*Deployment
	maxHistory  int
	mu          sync.RWMutex
}

// NewDeploymentHistory creates a new deployment history
func NewDeploymentHistory(maxHistory int) *DeploymentHistory {
	return &DeploymentHistory{
		deployments: make(map[string]*Deployment),
		maxHistory:  maxHistory,
	}
}

// Add adds a deployment to history
func (h *DeploymentHistory) Add(deployment *Deployment) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.deployments[deployment.ID] = deployment

	// Cleanup old deployments if needed
	if len(h.deployments) > h.maxHistory {
		h.cleanup()
	}
}

// Get retrieves a deployment from history
func (h *DeploymentHistory) Get(id string) (*Deployment, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	deployment, exists := h.deployments[id]
	return deployment, exists
}

// List returns all deployments
func (h *DeploymentHistory) List() []*Deployment {
	h.mu.RLock()
	defer h.mu.RUnlock()

	deployments := make([]*Deployment, 0, len(h.deployments))
	for _, d := range h.deployments {
		deployments = append(deployments, d)
	}
	return deployments
}

// cleanup removes old deployments
func (h *DeploymentHistory) cleanup() {
	// Keep only the most recent deployments
	if len(h.deployments) <= h.maxHistory {
		return
	}

	// Sort by time and remove oldest
	type deploymentWithTime struct {
		id   string
		time time.Time
	}

	var sorted []deploymentWithTime
	for id, d := range h.deployments {
		sorted = append(sorted, deploymentWithTime{id: id, time: d.StartTime})
	}

	// Simple sort by time
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i].time.Before(sorted[j].time) {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	// Remove oldest entries
	for i := h.maxHistory; i < len(sorted); i++ {
		delete(h.deployments, sorted[i].id)
	}
}

// DeploymentValidator validates deployment configurations
type DeploymentValidator struct {
	logger *zap.SugaredLogger
}

// NewDeploymentValidator creates a new deployment validator
func NewDeploymentValidator(logger *zap.SugaredLogger) *DeploymentValidator {
	return &DeploymentValidator{
		logger: logger,
	}
}

// Validate validates a deployment request
func (v *DeploymentValidator) Validate(deployment *Deployment) error {
	var errors []string

	if deployment.Name == "" {
		errors = append(errors, "deployment name is required")
	}

	if deployment.Image == "" {
		errors = append(errors, "image is required")
	}

	if deployment.Tag == "" {
		errors = append(errors, "tag is required")
	}

	if deployment.Replicas < 0 {
		errors = append(errors, "replicas must be non-negative")
	}

	if deployment.Strategy == "" {
		errors = append(errors, "deployment strategy is required")
	}

	// Validate strategy-specific configurations
	switch deployment.Strategy {
	case StrategyCanary:
		if deployment.CanaryStatus == nil {
			errors = append(errors, "canary configuration is required for canary strategy")
		}
	case StrategyBlueGreen:
		if deployment.BlueGreenStatus == nil {
			errors = append(errors, "blue-green configuration is required for blue-green strategy")
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("validation failed: %v", errors)
	}

	return nil
}

// HealthChecker performs health checks on deployments
type HealthChecker struct {
	config *DeploymentConfig
	logger *zap.SugaredLogger
}

// NewHealthChecker creates a new health checker
func NewHealthChecker(config *DeploymentConfig, logger *zap.SugaredLogger) *HealthChecker {
	return &HealthChecker{
		config: config,
		logger: logger,
	}
}

// Check performs health checks on a deployment
func (hc *HealthChecker) Check(ctx context.Context, deployment *Deployment) (*HealthStatus, error) {
	status := &HealthStatus{
		Healthy:       false,
		TotalReplicas: deployment.Replicas,
		LastCheckTime: time.Now(),
		Checks:        []HealthCheck{},
	}

	// Simulate health check logic
	// In production, this would:
	// 1. Check pod readiness
	// 2. Check service endpoints
	// 3. Run application-level health checks
	// 4. Verify database connectivity
	// 5. Check cache connectivity

	// For now, return a simulated healthy status
	status.Healthy = true
	status.ReadyReplicas = deployment.Replicas
	status.AvailableReplicas = deployment.Replicas

	deployment.HealthStatus = *status
	deployment.HealthChecks = []HealthCheckResult{
		{
			Name:      "pod-readiness",
			Status:    "passed",
			Message:   "All pods are ready",
			Timestamp: time.Now(),
			Duration:  100 * time.Millisecond,
		},
		{
			Name:      "service-endpoint",
			Status:    "passed",
			Message:   "Service endpoints are available",
			Timestamp: time.Now(),
			Duration:  50 * time.Millisecond,
		},
		{
			Name:      "database-connectivity",
			Status:    "passed",
			Message:   "Database connection is healthy",
			Timestamp: time.Now(),
			Duration:  25 * time.Millisecond,
		},
	}

	return status, nil
}

// RollbackManager handles deployment rollbacks
type RollbackManager struct {
	config *DeploymentConfig
	logger *zap.SugaredLogger
}

// NewRollbackManager creates a new rollback manager
func NewRollbackManager(config *DeploymentConfig, logger *zap.SugaredLogger) *RollbackManager {
	return &RollbackManager{
		config: config,
		logger: logger,
	}
}

// Rollback performs a rollback to a previous version
func (rm *RollbackManager) Rollback(ctx context.Context, deployment *Deployment, targetVersion string) error {
	rm.logger.Infow("Starting rollback",
		"deployment_id", deployment.ID,
		"current_version", deployment.CurrentVersion,
		"target_version", targetVersion,
	)

	// Update deployment status
	deployment.Status = StatusRollback
	deployment.Phase = PhaseRollingBack
	deployment.RollbackVersion = targetVersion

	// Execute pre-rollback hooks
	if err := rm.executeHooks(ctx, deployment, rm.config.PreRollbackHooks); err != nil {
		rm.logger.Errorw("Pre-rollback hooks failed", "error", err)
		return fmt.Errorf("pre-rollback hooks failed: %w", err)
	}

	// Perform rollback
	// In production, this would:
	// 1. Update the deployment to use the previous image version
	// 2. Wait for rollout to complete
	// 3. Verify health checks pass

	deployment.CurrentVersion = targetVersion

	// Execute post-rollback hooks
	if err := rm.executeHooks(ctx, deployment, rm.config.PostRollbackHooks); err != nil {
		rm.logger.Warnw("Post-rollback hooks failed", "error", err)
		// Don't fail the rollback if post-hooks fail
	}

	rm.logger.Infow("Rollback completed successfully",
		"deployment_id", deployment.ID,
		"target_version", targetVersion,
	)

	return nil
}

// executeHooks executes deployment hooks
func (rm *RollbackManager) executeHooks(ctx context.Context, deployment *Deployment, hooks []DeploymentHook) error {
	for _, hook := range hooks {
		rm.logger.Infow("Executing hook", "hook_name", hook.Name, "type", hook.Type)

		// In production, this would execute the actual hook command
		// For now, we simulate it
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
			// Simulate hook execution
		}

		rm.logger.Infow("Hook completed", "hook_name", hook.Name)
	}
	return nil
}

// DeploymentMetrics tracks deployment metrics
type DeploymentMetrics struct {
	TotalDeployments    int64
	SuccessfulDeploys   int64
	FailedDeploys       int64
	Rollbacks           int64
	AverageDuration     time.Duration
	TotalDuration       time.Duration
	mu                  sync.Mutex
}

// NewDeploymentMetrics creates new deployment metrics
func NewDeploymentMetrics() *DeploymentMetrics {
	return &DeploymentMetrics{}
}

// RecordDeployment records a deployment metric
func (m *DeploymentMetrics) RecordDeployment(deployment *Deployment) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.TotalDeployments++
	m.TotalDuration += deployment.Duration

	if deployment.Status == StatusSucceeded {
		m.SuccessfulDeploys++
	} else if deployment.Status == StatusFailed {
		m.FailedDeploys++
	}

	// Calculate average
	m.AverageDuration = time.Duration(int64(m.TotalDuration) / m.TotalDeployments)
}

// RecordRollback records a rollback metric
func (m *DeploymentMetrics) RecordRollback() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Rollbacks++
}

// GetMetrics returns current metrics
func (m *DeploymentMetrics) GetMetrics() map[string]interface{} {
	m.mu.Lock()
	defer m.mu.Unlock()

	return map[string]interface{}{
		"total_deployments":  m.TotalDeployments,
		"successful_deploys": m.SuccessfulDeploys,
		"failed_deploys":     m.FailedDeploys,
		"rollbacks":          m.Rollbacks,
		"average_duration":   m.AverageDuration.String(),
	}
}

// NewDeploymentManager creates a new deployment manager
func NewDeploymentManager(config *DeploymentConfig, logger *zap.SugaredLogger) *DeploymentManager {
	return &DeploymentManager{
		config:      config,
		logger:      logger,
		history:     NewDeploymentHistory(100),
		validator:   NewDeploymentValidator(logger),
		healthCheck: NewHealthChecker(config, logger),
		rollback:    NewRollbackManager(config, logger),
		metrics:     NewDeploymentMetrics(),
	}
}

// Deploy starts a new deployment
func (dm *DeploymentManager) Deploy(ctx context.Context, req *DeploymentRequest) (*Deployment, error) {
	// Create deployment object
	deployment := &Deployment{
		ID:              uuid.New().String(),
		Name:            req.Name,
		Namespace:       req.Namespace,
		Strategy:        dm.config.Strategy,
		Image:           req.Image,
		Tag:             req.Tag,
		Replicas:        req.Replicas,
		Status:          StatusPending,
		Phase:           PhaseInitializing,
		Progress:        0,
		StartTime:       time.Now(),
		PreviousVersion: req.PreviousVersion,
		CurrentVersion:  fmt.Sprintf("%s:%s", req.Image, req.Tag),
		Metadata:        req.Metadata,
		Labels:          req.Labels,
		Annotations:     req.Annotations,
	}

	// Validate deployment
	if err := dm.validator.Validate(deployment); err != nil {
		deployment.Status = StatusFailed
		deployment.Phase = PhaseFailed
		deployment.Errors = append(deployment.Errors, DeploymentError{
			Time:    time.Now(),
			Message: err.Error(),
			Code:    "VALIDATION_ERROR",
		})
		return deployment, err
	}

	// Check if there's already an active deployment
	dm.mu.Lock()
	if dm.activeDep != nil && dm.activeDep.Deployment.Status == StatusRunning {
		dm.mu.Unlock()
		return nil, fmt.Errorf("deployment %s is already in progress", dm.activeDep.Deployment.ID)
	}

	// Set as active deployment
	ctx, cancel := context.WithCancel(ctx)
	dm.activeDep = &ActiveDeployment{
		Deployment: deployment,
		CancelFunc: cancel,
		StartTime:  time.Now(),
	}
	dm.mu.Unlock()

	// Add to history
	dm.history.Add(deployment)

	// Run deployment in goroutine
	go dm.runDeployment(ctx, deployment)

	dm.logger.Infow("Deployment started",
		"deployment_id", deployment.ID,
		"name", deployment.Name,
		"strategy", deployment.Strategy,
		"image", deployment.CurrentVersion,
	)

	return deployment, nil
}

// DeploymentRequest represents a deployment request
type DeploymentRequest struct {
	Name            string                 `json:"name"`
	Namespace       string                 `json:"namespace"`
	Image           string                 `json:"image"`
	Tag             string                 `json:"tag"`
	Replicas        int                    `json:"replicas"`
	PreviousVersion string                 `json:"previous_version,omitempty"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
	Labels          map[string]string      `json:"labels,omitempty"`
	Annotations     map[string]string      `json:"annotations,omitempty"`
}

// runDeployment executes the deployment process
func (dm *DeploymentManager) runDeployment(ctx context.Context, deployment *Deployment) {
	defer dm.cleanupActiveDeployment()

	var err error

	// Update status
	deployment.Status = StatusRunning

	// Execute deployment based on strategy
	switch deployment.Strategy {
	case StrategyRolling:
		err = dm.executeRollingDeployment(ctx, deployment)
	case StrategyBlueGreen:
		err = dm.executeBlueGreenDeployment(ctx, deployment)
	case StrategyCanary:
		err = dm.executeCanaryDeployment(ctx, deployment)
	case StrategyRecreate:
		err = dm.executeRecreateDeployment(ctx, deployment)
	default:
		err = fmt.Errorf("unsupported deployment strategy: %s", deployment.Strategy)
	}

	// Handle completion
	if err != nil {
		dm.logger.Errorw("Deployment failed",
			"deployment_id", deployment.ID,
			"error", err,
		)

		deployment.Status = StatusFailed
		deployment.Phase = PhaseFailed
		deployment.Errors = append(deployment.Errors, DeploymentError{
			Time:    time.Now(),
			Message: err.Error(),
			Code:    "DEPLOYMENT_ERROR",
		})

		// Record metrics
		dm.metrics.RecordDeployment(deployment)

		// Rollback if configured
		if dm.config.RollbackOnFailure && deployment.PreviousVersion != "" {
			dm.logger.Infow("Initiating automatic rollback",
				"deployment_id", deployment.ID,
				"previous_version", deployment.PreviousVersion,
			)

			if rollbackErr := dm.rollback.Rollback(ctx, deployment, deployment.PreviousVersion); rollbackErr != nil {
				dm.logger.Errorw("Rollback failed", "error", rollbackErr)
				deployment.Errors = append(deployment.Errors, DeploymentError{
					Time:    time.Now(),
					Message: fmt.Sprintf("Rollback failed: %v", rollbackErr),
					Code:    "ROLLBACK_ERROR",
				})
			} else {
				deployment.Status = StatusRollback
				dm.metrics.RecordRollback()
			}
		}
	} else {
		now := time.Now()
		deployment.Status = StatusSucceeded
		deployment.Phase = PhaseCompleted
		deployment.EndTime = &now
		deployment.Duration = now.Sub(deployment.StartTime)
		deployment.Progress = 100

		dm.logger.Infow("Deployment succeeded",
			"deployment_id", deployment.ID,
			"duration", deployment.Duration,
		)

		dm.metrics.RecordDeployment(deployment)
	}
}

// executeRollingDeployment executes a rolling deployment
func (dm *DeploymentManager) executeRollingDeployment(ctx context.Context, deployment *Deployment) error {
	dm.logger.Infow("Executing rolling deployment", "deployment_id", deployment.ID)

	deployment.Phase = PhasePreHooks
	deployment.Progress = 10

	// Execute pre-deploy hooks
	if err := dm.executeHooks(ctx, deployment, dm.config.PreDeployHooks); err != nil {
		return fmt.Errorf("pre-deploy hooks failed: %w", err)
	}

	deployment.Phase = PhaseDeploying
	deployment.Progress = 20

	// Initialize rolling status
	deployment.RollingStatus = &RollingStatus{
		UpdatedReplicas:     0,
		ReadyReplicas:       0,
		AvailableReplicas:   0,
		UnavailableReplicas: deployment.Replicas,
	}

	// Simulate rolling update
	for i := 0; i < deployment.Replicas; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Update one replica at a time
		deployment.RollingStatus.UpdatedReplicas++
		deployment.RollingStatus.UnavailableReplicas--

		// Wait for replica to become ready
		time.Sleep(2 * time.Second)

		deployment.RollingStatus.ReadyReplicas++
		deployment.RollingStatus.AvailableReplicas++

		// Update progress
		deployment.Progress = 20 + (float64(i+1) / float64(deployment.Replicas) * 60)

		dm.logger.Infow("Rolling update progress",
			"deployment_id", deployment.ID,
			"updated", deployment.RollingStatus.UpdatedReplicas,
			"total", deployment.Replicas,
		)
	}

	deployment.Phase = PhaseHealthCheck
	deployment.Progress = 80

	// Run health checks
	_, err := dm.healthCheck.Check(ctx, deployment)
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}

	deployment.Phase = PhasePostHooks
	deployment.Progress = 90

	// Execute post-deploy hooks
	if err := dm.executeHooks(ctx, deployment, dm.config.PostDeployHooks); err != nil {
		return fmt.Errorf("post-deploy hooks failed: %w", err)
	}

	return nil
}

// executeBlueGreenDeployment executes a blue-green deployment
func (dm *DeploymentManager) executeBlueGreenDeployment(ctx context.Context, deployment *Deployment) error {
	dm.logger.Infow("Executing blue-green deployment", "deployment_id", deployment.ID)

	// Initialize blue-green status
	deployment.BlueGreenStatus = &BlueGreenStatus{
		ActiveColor:  "blue",
		PreviewColor: "green",
		ActiveReady:  true,
		PreviewReady: false,
		Promoted:     false,
	}

	deployment.Phase = PhaseDeploying
	deployment.Progress = 30

	// Deploy to preview environment
	dm.logger.Infow("Deploying to preview environment", "color", deployment.BlueGreenStatus.PreviewColor)

	// Simulate deployment
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(5 * time.Second):
		// Deployment complete
	}

	deployment.BlueGreenStatus.PreviewReady = true
	deployment.Progress = 50

	deployment.Phase = PhaseHealthCheck

	// Run health checks on preview
	_, err := dm.healthCheck.Check(ctx, deployment)
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}

	deployment.Progress = 70

	// Promote if auto-promote is enabled or wait for manual promotion
	if dm.config.BlueGreenConfig.AutoPromote {
		dm.logger.Infow("Auto-promoting deployment", "deployment_id", deployment.ID)

		// Switch traffic to preview
		deployment.BlueGreenStatus.ActiveColor = "green"
		deployment.BlueGreenStatus.PreviewColor = "blue"
		deployment.BlueGreenStatus.Promoted = true
		deployment.BlueGreenStatus.PromotionTime = time.Now()
	}

	deployment.Phase = PhasePostHooks
	deployment.Progress = 90

	// Execute post-deploy hooks
	if err := dm.executeHooks(ctx, deployment, dm.config.PostDeployHooks); err != nil {
		return fmt.Errorf("post-deploy hooks failed: %w", err)
	}

	return nil
}

// executeCanaryDeployment executes a canary deployment
func (dm *DeploymentManager) executeCanaryDeployment(ctx context.Context, deployment *Deployment) error {
	dm.logger.Infow("Executing canary deployment", "deployment_id", deployment.ID)

	// Initialize canary status
	deployment.CanaryStatus = &CanaryStatus{
		CurrentWeight:  0,
		TargetWeight:   100,
		StableReplicas: deployment.Replicas,
		CanaryReplicas: 0,
		Promoted:       false,
	}

	deployment.Phase = PhaseDeploying

	// Gradually increase canary weight
	weights := []float64{10, 25, 50, 75, 100}

	for _, weight := range weights {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		deployment.CanaryStatus.CurrentWeight = weight
		deployment.CanaryReplicas = int(float64(deployment.Replicas) * weight / 100)

		deployment.Progress = weight * 0.8

		dm.logger.Infow("Canary deployment progress",
			"deployment_id", deployment.ID,
			"weight", weight,
			"canary_replicas", deployment.CanaryReplicas,
		)

		// Wait and analyze metrics
		deployment.Phase = PhaseHealthCheck
		_, err := dm.healthCheck.Check(ctx, deployment)
		if err != nil {
			deployment.CanaryStatus.AnalysisResult = "failed"
			return fmt.Errorf("canary analysis failed at weight %.0f: %w", weight, err)
		}

		// Simulate analysis
		time.Sleep(2 * time.Second)

		deployment.Phase = PhaseDeploying
	}

	deployment.CanaryStatus.Promoted = true
	deployment.CanaryStatus.AnalysisResult = "success"

	deployment.Phase = PhasePostHooks
	deployment.Progress = 90

	// Execute post-deploy hooks
	if err := dm.executeHooks(ctx, deployment, dm.config.PostDeployHooks); err != nil {
		return fmt.Errorf("post-deploy hooks failed: %w", err)
	}

	return nil
}

// executeRecreateDeployment executes a recreate deployment
func (dm *DeploymentManager) executeRecreateDeployment(ctx context.Context, deployment *Deployment) error {
	dm.logger.Infow("Executing recreate deployment", "deployment_id", deployment.ID)

	deployment.Phase = PhaseDeploying
	deployment.Progress = 20

	// Scale down all existing replicas
	dm.logger.Infow("Scaling down existing replicas")
	time.Sleep(2 * time.Second)

	deployment.Progress = 40

	// Deploy new version
	dm.logger.Infow("Deploying new version")
	time.Sleep(3 * time.Second)

	deployment.Progress = 70

	deployment.Phase = PhaseHealthCheck

	// Run health checks
	_, err := dm.healthCheck.Check(ctx, deployment)
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}

	deployment.Phase = PhasePostHooks
	deployment.Progress = 90

	// Execute post-deploy hooks
	if err := dm.executeHooks(ctx, deployment, dm.config.PostDeployHooks); err != nil {
		return fmt.Errorf("post-deploy hooks failed: %w", err)
	}

	return nil
}

// executeHooks executes deployment hooks
func (dm *DeploymentManager) executeHooks(ctx context.Context, deployment *Deployment, hooks []DeploymentHook) error {
	for _, hook := range hooks {
		dm.logger.Infow("Executing hook",
			"deployment_id", deployment.ID,
			"hook_name", hook.Name,
			"type", hook.Type,
		)

		// In production, this would execute the actual hook
		// For now, we simulate it
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
			// Simulate hook execution
		}

		dm.logger.Infow("Hook completed",
			"deployment_id", deployment.ID,
			"hook_name", hook.Name,
		)
	}
	return nil
}

// cleanupActiveDeployment cleans up the active deployment
func (dm *DeploymentManager) cleanupActiveDeployment() {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	dm.activeDep = nil
}

// Cancel cancels an active deployment
func (dm *DeploymentManager) Cancel(deploymentID string) error {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if dm.activeDep == nil {
		return fmt.Errorf("no active deployment")
	}

	if dm.activeDep.Deployment.ID != deploymentID {
		return fmt.Errorf("deployment %s is not the active deployment", deploymentID)
	}

	dm.activeDep.Deployment.Status = StatusCancelled
	dm.activeDep.CancelFunc()

	dm.logger.Infow("Deployment cancelled", "deployment_id", deploymentID)

	return nil
}

// GetDeployment retrieves a deployment by ID
func (dm *DeploymentManager) GetDeployment(deploymentID string) (*Deployment, bool) {
	return dm.history.Get(deploymentID)
}

// ListDeployments lists all deployments
func (dm *DeploymentManager) ListDeployments() []*Deployment {
	return dm.history.List()
}

// GetStatus returns the status of a deployment
func (dm *DeploymentManager) GetStatus(deploymentID string) (*DeploymentStatusResponse, error) {
	deployment, exists := dm.history.Get(deploymentID)
	if !exists {
		return nil, fmt.Errorf("deployment %s not found", deploymentID)
	}

	return &DeploymentStatusResponse{
		ID:            deployment.ID,
		Name:          deployment.Name,
		Status:        deployment.Status,
		Phase:         deployment.Phase,
		Progress:      deployment.Progress,
		CurrentVersion: deployment.CurrentVersion,
		HealthStatus:  deployment.HealthStatus,
		Duration:      deployment.Duration,
		Errors:        deployment.Errors,
		Warnings:      deployment.Warnings,
	}, nil
}

// DeploymentStatusResponse represents a deployment status response
type DeploymentStatusResponse struct {
	ID             string              `json:"id"`
	Name           string              `json:"name"`
	Status         DeploymentStatus    `json:"status"`
	Phase          DeploymentPhase     `json:"phase"`
	Progress       float64             `json:"progress"`
	CurrentVersion string              `json:"current_version"`
	HealthStatus   HealthStatus        `json:"health_status"`
	Duration       time.Duration       `json:"duration"`
	Errors         []DeploymentError   `json:"errors,omitempty"`
	Warnings       []string            `json:"warnings,omitempty"`
}

// RollbackDeployment rolls back a deployment
func (dm *DeploymentManager) RollbackDeployment(deploymentID string, targetVersion string) error {
	deployment, exists := dm.history.Get(deploymentID)
	if !exists {
		return fmt.Errorf("deployment %s not found", deploymentID)
	}

	ctx := context.Background()

	dm.metrics.RecordRollback()

	return dm.rollback.Rollback(ctx, deployment, targetVersion)
}

// GetMetrics returns deployment metrics
func (dm *DeploymentManager) GetMetrics() map[string]interface{} {
	return dm.metrics.GetMetrics()
}

// Export exports deployment history to JSON
func (dm *DeploymentManager) Export() ([]byte, error) {
	deployments := dm.history.List()

	export := struct {
		Deployments []*Deployment `json:"deployments"`
		Metrics     map[string]interface{} `json:"metrics"`
		ExportTime  time.Time `json:"export_time"`
	}{
		Deployments: deployments,
		Metrics:     dm.metrics.GetMetrics(),
		ExportTime:  time.Now(),
	}

	return json.MarshalIndent(export, "", "  ")
}

// GetActiveDeployment returns the currently active deployment
func (dm *DeploymentManager) GetActiveDeployment() *Deployment {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	if dm.activeDep == nil {
		return nil
	}

	return dm.activeDep.Deployment
}

// Pause pauses a deployment
func (dm *DeploymentManager) Pause(deploymentID string) error {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if dm.activeDep == nil || dm.activeDep.Deployment.ID != deploymentID {
		return fmt.Errorf("deployment %s is not active", deploymentID)
	}

	dm.activeDep.Deployment.Status = StatusPaused
	dm.logger.Infow("Deployment paused", "deployment_id", deploymentID)

	return nil
}

// Resume resumes a paused deployment
func (dm *DeploymentManager) Resume(deploymentID string) error {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if dm.activeDep == nil || dm.activeDep.Deployment.ID != deploymentID {
		return fmt.Errorf("deployment %s is not active", deploymentID)
	}

	if dm.activeDep.Deployment.Status != StatusPaused {
		return fmt.Errorf("deployment %s is not paused", deploymentID)
	}

	dm.activeDep.Deployment.Status = StatusRunning
	dm.logger.Infow("Deployment resumed", "deployment_id", deploymentID)

	return nil
}
