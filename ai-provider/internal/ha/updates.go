package ha

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// UpdateStrategy defines the strategy for performing updates
type UpdateStrategy string

const (
	StrategyRolling   UpdateStrategy = "rolling"
	StrategyBlueGreen UpdateStrategy = "blue_green"
	StrategyCanary    UpdateStrategy = "canary"
)

// UpdateStatus represents the status of an update
type UpdateStatus string

const (
	UpdatePending    UpdateStatus = "pending"
	UpdateInProgress UpdateStatus = "in_progress"
	UpdateCompleted  UpdateStatus = "completed"
	UpdateFailed     UpdateStatus = "failed"
	UpdateRolledBack UpdateStatus = "rolled_back"
)

// UpdateConfig contains configuration for updates
type UpdateConfig struct {
	Strategy           UpdateStrategy `json:"strategy"`
	BatchSize          int            `json:"batch_size"`
	BatchInterval      time.Duration  `json:"batch_interval"`
	HealthCheckWait    time.Duration  `json:"health_check_wait"`
	RollbackOnFailure  bool           `json:"rollback_on_failure"`
	MaxUnavailable     int            `json:"max_unavailable"`
	Timeout            time.Duration  `json:"timeout"`
	CanaryPercentage   int            `json:"canary_percentage"`
	CanaryWaitTime     time.Duration  `json:"canary_wait_time"`
}

// DefaultUpdateConfig returns default update configuration
func DefaultUpdateConfig() *UpdateConfig {
	return &UpdateConfig{
		Strategy:          StrategyRolling,
		BatchSize:         1,
		BatchInterval:     30 * time.Second,
		HealthCheckWait:   60 * time.Second,
		RollbackOnFailure: true,
		MaxUnavailable:    1,
		Timeout:           30 * time.Minute,
		CanaryPercentage:  10,
		CanaryWaitTime:    10 * time.Minute,
	}
}

// VersionInfo represents version information
type VersionInfo struct {
	Version   string    `json:"version"`
	Image     string    `json:"image"`
	Timestamp time.Time `json:"timestamp"`
	Checksum  string    `json:"checksum"`
}

// UpdateEvent represents an update event
type UpdateEvent struct {
	ID          string        `json:"id"`
	Timestamp   time.Time     `json:"timestamp"`
	NodeID      string        `json:"node_id"`
	OldVersion  string        `json:"old_version"`
	NewVersion  string        `json:"new_version"`
	Status      UpdateStatus  `json:"status"`
	Message     string        `json:"message"`
	Duration    time.Duration `json:"duration"`
}

// UpdateManager manages zero-downtime updates
type UpdateManager struct {
	config         *UpdateConfig
	currentVersion *VersionInfo
	targetVersion  *VersionInfo
	status         UpdateStatus
	events         []*UpdateEvent
	nodes          map[string]*Node
	failoverMgr    *FailoverManager
	healthChecker  *HealthChecker
	mu             sync.RWMutex
	ctx            context.Context
	cancel         context.CancelFunc
	wg             sync.WaitGroup
}

// NewUpdateManager creates a new update manager
func NewUpdateManager(config *UpdateConfig) *UpdateManager {
	if config == nil {
		config = DefaultUpdateConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &UpdateManager{
		config:  config,
		status:  UpdatePending,
		events:  make([]*UpdateEvent, 0),
		nodes:   make(map[string]*Node),
		ctx:     ctx,
		cancel:  cancel,
	}
}

// SetFailoverManager sets the failover manager
func (um *UpdateManager) SetFailoverManager(fm *FailoverManager) {
	um.failoverMgr = fm
}

// SetHealthChecker sets the health checker
func (um *UpdateManager) SetHealthChecker(hc *HealthChecker) {
	um.healthChecker = hc
}

// SetCurrentVersion sets the current version
func (um *UpdateManager) SetCurrentVersion(version *VersionInfo) error {
	if version == nil {
		return errors.New("version cannot be nil")
	}

	um.mu.Lock()
	defer um.mu.Unlock()

	um.currentVersion = version
	return nil
}

// GetCurrentVersion returns the current version
func (um *UpdateManager) GetCurrentVersion() *VersionInfo {
	um.mu.RLock()
	defer um.mu.RUnlock()
	return um.currentVersion
}

// RegisterNode registers a node for updates
func (um *UpdateManager) RegisterNode(node *Node) error {
	if node == nil {
		return errors.New("node cannot be nil")
	}

	um.mu.Lock()
	defer um.mu.Unlock()

	um.nodes[node.ID] = node
	return nil
}

// UnregisterNode removes a node from update management
func (um *UpdateManager) UnregisterNode(nodeID string) error {
	um.mu.Lock()
	defer um.mu.Unlock()

	if _, exists := um.nodes[nodeID]; !exists {
		return fmt.Errorf("node %s not found", nodeID)
	}

	delete(um.nodes, nodeID)
	return nil
}

// StartUpdate starts an update to a new version
func (um *UpdateManager) StartUpdate(targetVersion *VersionInfo) error {
	if targetVersion == nil {
		return errors.New("target version cannot be nil")
	}

	um.mu.Lock()
	defer um.mu.Unlock()

	if um.status == UpdateInProgress {
		return errors.New("update already in progress")
	}

	um.targetVersion = targetVersion
	um.status = UpdateInProgress

	// Start update based on strategy
	switch um.config.Strategy {
	case StrategyRolling:
		go um.performRollingUpdate()
	case StrategyBlueGreen:
		go um.performBlueGreenUpdate()
	case StrategyCanary:
		go um.performCanaryUpdate()
	default:
		um.status = UpdateFailed
		return fmt.Errorf("unsupported update strategy: %s", um.config.Strategy)
	}

	return nil
}

// performRollingUpdate performs a rolling update
func (um *UpdateManager) performRollingUpdate() {
	defer um.wg.Done()
	um.wg.Add(1)

	startTime := time.Now()

	nodes := um.getNodeList()
	batchCount := (len(nodes) + um.config.BatchSize - 1) / um.config.BatchSize

	for batch := 0; batch < batchCount; batch++ {
		select {
		case <-um.ctx.Done():
			um.status = UpdateFailed
			return
		default:
		}

		start := batch * um.config.BatchSize
		end := start + um.config.BatchSize
		if end > len(nodes) {
			end = len(nodes)
		}

		batchNodes := nodes[start:end]

		// Update batch
		for _, node := range batchNodes {
			if err := um.updateNode(node); err != nil {
				event := &UpdateEvent{
					ID:         fmt.Sprintf("update-%d", time.Now().Unix()),
					Timestamp:  time.Now(),
					NodeID:     node.ID,
					OldVersion: um.currentVersion.Version,
					NewVersion: um.targetVersion.Version,
					Status:     UpdateFailed,
					Message:    err.Error(),
					Duration:   time.Since(startTime),
				}
				um.events = append(um.events, event)

				if um.config.RollbackOnFailure {
					um.performRollback()
					return
				}
			}
		}

		// Wait for health checks
		time.Sleep(um.config.HealthCheckWait)

		// Verify batch health
		if !um.verifyBatchHealth(batchNodes) {
			if um.config.RollbackOnFailure {
				um.performRollback()
				return
			}
		}

		// Wait between batches
		if batch < batchCount-1 {
			time.Sleep(um.config.BatchInterval)
		}
	}

	um.status = UpdateCompleted
	um.currentVersion = um.targetVersion

	event := &UpdateEvent{
		ID:         fmt.Sprintf("update-%d", time.Now().Unix()),
		Timestamp:  time.Now(),
		OldVersion: um.currentVersion.Version,
		NewVersion: um.targetVersion.Version,
		Status:     UpdateCompleted,
		Message:    "Update completed successfully",
		Duration:   time.Since(startTime),
	}
	um.events = append(um.events, event)
}

// performBlueGreenUpdate performs a blue-green update
func (um *UpdateManager) performBlueGreenUpdate() {
	// Simplified blue-green deployment
	// In production, would manage two complete environments
	um.performRollingUpdate()
}

// performCanaryUpdate performs a canary update
func (um *UpdateManager) performCanaryUpdate() {
	defer um.wg.Done()
	um.wg.Add(1)

	startTime := time.Now()
	nodes := um.getNodeList()

	// Calculate canary nodes
	canaryCount := (len(nodes) * um.config.CanaryPercentage) / 100
	if canaryCount == 0 {
		canaryCount = 1
	}

	// Update canary nodes first
	canaryNodes := nodes[:canaryCount]
	for _, node := range canaryNodes {
		if err := um.updateNode(node); err != nil {
			um.status = UpdateFailed
			return
		}
	}

	// Wait and monitor canary
	time.Sleep(um.config.CanaryWaitTime)

	// Verify canary health
	if !um.verifyBatchHealth(canaryNodes) {
		if um.config.RollbackOnFailure {
			um.performRollback()
		}
		return
	}

	// Update remaining nodes
	remainingNodes := nodes[canaryCount:]
	for _, node := range remainingNodes {
		if err := um.updateNode(node); err != nil {
			if um.config.RollbackOnFailure {
				um.performRollback()
			}
			return
		}
	}

	um.status = UpdateCompleted
	um.currentVersion = um.targetVersion

	event := &UpdateEvent{
		ID:         fmt.Sprintf("update-%d", time.Now().Unix()),
		Timestamp:  time.Now(),
		OldVersion: um.currentVersion.Version,
		NewVersion: um.targetVersion.Version,
		Status:     UpdateCompleted,
		Message:    "Canary update completed successfully",
		Duration:   time.Since(startTime),
	}
	um.events = append(um.events, event)
}

// updateNode updates a single node
func (um *UpdateManager) updateNode(node *Node) error {
	// In production, would:
	// 1. Cordon the node (stop scheduling new work)
	// 2. Drain existing connections
	// 3. Update the node
	// 4. Perform health checks
	// 5. Uncordon the node

	// Simulate update
	node.SetState(StateRecovering)

	event := &UpdateEvent{
		ID:         fmt.Sprintf("update-%d", time.Now().Unix()),
		Timestamp:  time.Now(),
		NodeID:     node.ID,
		OldVersion: um.currentVersion.Version,
		NewVersion: um.targetVersion.Version,
		Status:     UpdateCompleted,
		Message:    "Node updated successfully",
	}
	um.events = append(um.events, event)

	node.SetState(StateActive)
	return nil
}

// performRollback performs a rollback to the previous version
func (um *UpdateManager) performRollback() {
	startTime := time.Now()

	// Swap versions
	targetVersion := um.currentVersion
	um.targetVersion = targetVersion

	// Perform rolling update back
	nodes := um.getNodeList()
	for _, node := range nodes {
		um.updateNode(node)
	}

	um.status = UpdateRolledBack

	event := &UpdateEvent{
		ID:         fmt.Sprintf("rollback-%d", time.Now().Unix()),
		Timestamp:  time.Now(),
		OldVersion: um.targetVersion.Version,
		NewVersion: targetVersion.Version,
		Status:     UpdateRolledBack,
		Message:    "Rollback completed",
		Duration:   time.Since(startTime),
	}
	um.events = append(um.events, event)
}

// verifyBatchHealth verifies health of a batch of nodes
func (um *UpdateManager) verifyBatchHealth(nodes []*Node) bool {
	if um.healthChecker == nil {
		return true
	}

	for _, node := range nodes {
		healthScore := um.healthChecker.GetNodeHealthScore(node.ID)
		if healthScore < 70 {
			return false
		}
	}

	return true
}

// getNodeList returns list of all nodes
func (um *UpdateManager) getNodeList() []*Node {
	um.mu.RLock()
	defer um.mu.RUnlock()

	nodes := make([]*Node, 0, len(um.nodes))
	for _, node := range um.nodes {
		nodes = append(nodes, node)
	}

	return nodes
}

// GetStatus returns current update status
func (um *UpdateManager) GetStatus() UpdateStatus {
	um.mu.RLock()
	defer um.mu.RUnlock()
	return um.status
}

// GetEvents returns all update events
func (um *UpdateManager) GetEvents() []*UpdateEvent {
	um.mu.RLock()
	defer um.mu.RUnlock()

	events := make([]*UpdateEvent, len(um.events))
	copy(events, um.events)
	return events
}

// CancelUpdate cancels an ongoing update
func (um *UpdateManager) CancelUpdate() error {
	um.mu.Lock()
	defer um.mu.Unlock()

	if um.status != UpdateInProgress {
		return errors.New("no update in progress")
	}

	um.cancel()
	um.status = UpdateFailed

	return nil
}

// GetUpdateStats returns update statistics
func (um *UpdateManager) GetUpdateStats() map[string]interface{} {
	um.mu.RLock()
	defer um.mu.RUnlock()

	totalEvents := len(um.events)
	completedUpdates := 0
	failedUpdates := 0
	rollbacks := 0

	for _, event := range um.events {
		switch event.Status {
		case UpdateCompleted:
			completedUpdates++
		case UpdateFailed:
			failedUpdates++
		case UpdateRolledBack:
			rollbacks++
		}
	}

	return map[string]interface{}{
		"current_version":     um.currentVersion,
		"target_version":      um.targetVersion,
		"status":              um.status,
		"strategy":            um.config.Strategy,
		"total_nodes":         len(um.nodes),
		"total_events":        totalEvents,
		"completed_updates":   completedUpdates,
		"failed_updates":      failedUpdates,
		"rollbacks":           rollbacks,
	}
}

// Stop stops the update manager
func (um *UpdateManager) Stop() error {
	um.cancel()
	um.wg.Wait()
	return nil
}
