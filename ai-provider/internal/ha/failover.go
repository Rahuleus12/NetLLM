package ha

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// FailoverState represents the current state of a node
type FailoverState string

const (
	StateActive       FailoverState = "active"
	StateStandby      FailoverState = "standby"
	StateFailingOver  FailoverState = "failing_over"
	StateFailed       FailoverState = "failed"
	StateRecovering   FailoverState = "recovering"
	StateUnknown      FailoverState = "unknown"
)

// FailoverPolicy defines the strategy for failover
type FailoverPolicy string

const (
	PolicyAutomatic   FailoverPolicy = "automatic"
	PolicyManual      FailoverPolicy = "manual"
	PolicyQuorumBased FailoverPolicy = "quorum_based"
)

// Node represents a node in the HA cluster
type Node struct {
	ID            string        `json:"id"`
	Name          string        `json:"name"`
	Address       string        `json:"address"`
	Port          int           `json:"port"`
	State         FailoverState `json:"state"`
	Priority      int           `json:"priority"`
	Region        string        `json:"region"`
	Zone          string        `json:"zone"`
	LastHeartbeat time.Time     `json:"last_heartbeat"`
	HealthScore   int           `json:"health_score"`
	Metadata      map[string]interface{} `json:"metadata"`
	mu            sync.RWMutex
}

// UpdateHeartbeat updates the node's last heartbeat time
func (n *Node) UpdateHeartbeat() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.LastHeartbeat = time.Now()
}

// SetState safely updates the node's state
func (n *Node) SetState(state FailoverState) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.State = state
}

// GetState safely retrieves the node's state
func (n *Node) GetState() FailoverState {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.State
}

// FailoverConfig contains configuration for the failover system
type FailoverConfig struct {
	Policy               FailoverPolicy `json:"policy"`
	HeartbeatInterval    time.Duration  `json:"heartbeat_interval"`
	HeartbeatTimeout     time.Duration  `json:"heartbeat_timeout"`
	FailoverTimeout      time.Duration  `json:"failover_timeout"`
	RecoveryTimeout      time.Duration  `json:"recovery_timeout"`
	QuorumSize          int            `json:"quorum_size"`
	AutoFailoverEnabled bool           `json:"auto_failover_enabled"`
	MaxFailoverAttempts int            `json:"max_failover_attempts"`
	HealthCheckInterval time.Duration  `json:"health_check_interval"`
}

// DefaultFailoverConfig returns the default failover configuration
func DefaultFailoverConfig() *FailoverConfig {
	return &FailoverConfig{
		Policy:               PolicyAutomatic,
		HeartbeatInterval:    5 * time.Second,
		HeartbeatTimeout:     15 * time.Second,
		FailoverTimeout:      30 * time.Second,
		RecoveryTimeout:      60 * time.Second,
		QuorumSize:          3,
		AutoFailoverEnabled: true,
		MaxFailoverAttempts: 3,
		HealthCheckInterval: 10 * time.Second,
	}
}

// FailoverEvent represents a failover event
type FailoverEvent struct {
	ID             string        `json:"id"`
	Timestamp      time.Time     `json:"timestamp"`
	FromNode       string        `json:"from_node"`
	ToNode         string        `json:"to_node"`
	Reason         string        `json:"reason"`
	Success        bool          `json:"success"`
	Duration       time.Duration `json:"duration"`
	FailoverPolicy FailoverPolicy `json:"failover_policy"`
}

// FailoverManager manages the failover process
type FailoverManager struct {
	config        *FailoverConfig
	nodes         map[string]*Node
	activeNode    *Node
	events        []*FailoverEvent
	healthChecker *HealthChecker
	mu            sync.RWMutex
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup
	eventChan     chan *FailoverEvent
}

// NewFailoverManager creates a new failover manager
func NewFailoverManager(config *FailoverConfig) *FailoverManager {
	if config == nil {
		config = DefaultFailoverConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &FailoverManager{
		config:    config,
		nodes:     make(map[string]*Node),
		events:    make([]*FailoverEvent, 0),
		ctx:       ctx,
		cancel:    cancel,
		eventChan: make(chan *FailoverEvent, 100),
	}
}

// SetHealthChecker sets the health checker for the failover manager
func (fm *FailoverManager) SetHealthChecker(hc *HealthChecker) {
	fm.healthChecker = hc
}

// AddNode adds a node to the failover manager
func (fm *FailoverManager) AddNode(node *Node) error {
	if node == nil {
		return errors.New("node cannot be nil")
	}

	fm.mu.Lock()
	defer fm.mu.Unlock()

	if _, exists := fm.nodes[node.ID]; exists {
		return fmt.Errorf("node %s already exists", node.ID)
	}

	fm.nodes[node.ID] = node

	// If this is the first node or has highest priority, make it active
	if fm.activeNode == nil || node.Priority > fm.activeNode.Priority {
		if fm.activeNode != nil {
			fm.activeNode.SetState(StateStandby)
		}
		node.SetState(StateActive)
		fm.activeNode = node
	} else {
		node.SetState(StateStandby)
	}

	return nil
}

// RemoveNode removes a node from the failover manager
func (fm *FailoverManager) RemoveNode(nodeID string) error {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	node, exists := fm.nodes[nodeID]
	if !exists {
		return fmt.Errorf("node %s not found", nodeID)
	}

	// If removing active node, trigger failover
	if fm.activeNode != nil && fm.activeNode.ID == nodeID {
		if err := fm.performFailoverUnsafe(nodeID, "node removed"); err != nil {
			return fmt.Errorf("failed to failover after node removal: %w", err)
		}
	}

	delete(fm.nodes, nodeID)
	return nil
}

// GetNode retrieves a node by ID
func (fm *FailoverManager) GetNode(nodeID string) (*Node, error) {
	fm.mu.RLock()
	defer fm.mu.RUnlock()

	node, exists := fm.nodes[nodeID]
	if !exists {
		return nil, fmt.Errorf("node %s not found", nodeID)
	}

	return node, nil
}

// GetAllNodes returns all nodes
func (fm *FailoverManager) GetAllNodes() []*Node {
	fm.mu.RLock()
	defer fm.mu.RUnlock()

	nodes := make([]*Node, 0, len(fm.nodes))
	for _, node := range fm.nodes {
		nodes = append(nodes, node)
	}

	return nodes
}

// GetActiveNode returns the currently active node
func (fm *FailoverManager) GetActiveNode() *Node {
	fm.mu.RLock()
	defer fm.mu.RUnlock()
	return fm.activeNode
}

// Start starts the failover manager
func (fm *FailoverManager) Start() error {
	fm.wg.Add(1)
	go fm.monitorNodes()

	fm.wg.Add(1)
	go fm.processEvents()

	return nil
}

// Stop stops the failover manager
func (fm *FailoverManager) Stop() error {
	fm.cancel()
	fm.wg.Wait()
	close(fm.eventChan)
	return nil
}

// monitorNodes monitors node health and triggers failover if necessary
func (fm *FailoverManager) monitorNodes() {
	defer fm.wg.Done()

	ticker := time.NewTicker(fm.config.HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-fm.ctx.Done():
			return
		case <-ticker.C:
			fm.checkNodesHealth()
		}
	}
}

// checkNodesHealth checks the health of all nodes
func (fm *FailoverManager) checkNodesHealth() {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	now := time.Now()

	for _, node := range fm.nodes {
		// Check heartbeat timeout
		if now.Sub(node.LastHeartbeat) > fm.config.HeartbeatTimeout {
			if node.GetState() == StateActive {
				// Active node is unresponsive, trigger failover
				fm.performFailoverUnsafe(node.ID, "heartbeat timeout")
			} else {
				node.SetState(StateFailed)
			}
		}

		// Check health score if health checker is available
		if fm.healthChecker != nil {
			healthScore := fm.healthChecker.GetNodeHealthScore(node.ID)
			node.HealthScore = healthScore

			if healthScore < 50 && node.GetState() == StateActive {
				fm.performFailoverUnsafe(node.ID, "low health score")
			}
		}
	}
}

// performFailoverUnsafe performs failover without locking (must be called with lock held)
func (fm *FailoverManager) performFailoverUnsafe(fromNodeID, reason string) error {
	startTime := time.Now()

	// Find the best candidate for failover
	candidate := fm.selectFailoverCandidate()
	if candidate == nil {
		return errors.New("no available standby node for failover")
	}

	// Create failover event
	event := &FailoverEvent{
		ID:             fmt.Sprintf("failover-%d", time.Now().Unix()),
		Timestamp:      startTime,
		FromNode:       fromNodeID,
		ToNode:         candidate.ID,
		Reason:         reason,
		FailoverPolicy: fm.config.Policy,
	}

	// Check policy
	if fm.config.Policy == PolicyManual {
		event.Success = false
		event.Duration = time.Since(startTime)
		fm.events = append(fm.events, event)
		return errors.New("manual failover policy - automatic failover disabled")
	}

	// Check quorum if policy requires it
	if fm.config.Policy == PolicyQuorumBased {
		if !fm.checkQuorum() {
			event.Success = false
			event.Duration = time.Since(startTime)
			fm.events = append(fm.events, event)
			return errors.New("quorum not reached for failover")
		}
	}

	// Perform the failover
	oldActive := fm.activeNode
	if oldActive != nil {
		oldActive.SetState(StateStandby)
	}

	candidate.SetState(StateActive)
	fm.activeNode = candidate

	// Mark old node as failed if it was the one we're failing over from
	if oldNode, exists := fm.nodes[fromNodeID]; exists {
		oldNode.SetState(StateFailed)
	}

	event.Success = true
	event.Duration = time.Since(startTime)
	fm.events = append(fm.events, event)

	// Send event to channel
	select {
	case fm.eventChan <- event:
	default:
		// Channel full, drop event
	}

	return nil
}

// selectFailoverCandidate selects the best candidate for failover
func (fm *FailoverManager) selectFailoverCandidate() *Node {
	var candidate *Node
	var highestPriority int = -1

	for _, node := range fm.nodes {
		state := node.GetState()
		if state == StateStandby && node.HealthScore >= 70 {
			if node.Priority > highestPriority {
				highestPriority = node.Priority
				candidate = node
			}
		}
	}

	return candidate
}

// checkQuorum checks if quorum is reached for failover decision
func (fm *FailoverManager) checkQuorum() bool {
	healthyNodes := 0
	for _, node := range fm.nodes {
		if node.HealthScore >= 70 {
			healthyNodes++
		}
	}
	return healthyNodes >= fm.config.QuorumSize
}

// PerformManualFailover performs a manual failover to a specific node
func (fm *FailoverManager) PerformManualFailover(toNodeID string, reason string) error {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	targetNode, exists := fm.nodes[toNodeID]
	if !exists {
		return fmt.Errorf("target node %s not found", toNodeID)
	}

	if targetNode.GetState() != StateStandby {
		return fmt.Errorf("target node %s is not in standby state", toNodeID)
	}

	if targetNode.HealthScore < 70 {
		return fmt.Errorf("target node %s health score is too low: %d", toNodeID, targetNode.HealthScore)
	}

	var fromNodeID string
	if fm.activeNode != nil {
		fromNodeID = fm.activeNode.ID
		fm.activeNode.SetState(StateStandby)
	}

	targetNode.SetState(StateActive)
	fm.activeNode = targetNode

	event := &FailoverEvent{
		ID:             fmt.Sprintf("failover-%d", time.Now().Unix()),
		Timestamp:      time.Now(),
		FromNode:       fromNodeID,
		ToNode:         toNodeID,
		Reason:         reason,
		Success:        true,
		FailoverPolicy: PolicyManual,
	}

	fm.events = append(fm.events, event)

	return nil
}

// GetFailoverEvents returns all failover events
func (fm *FailoverManager) GetFailoverEvents() []*FailoverEvent {
	fm.mu.RLock()
	defer fm.mu.RUnlock()

	events := make([]*FailoverEvent, len(fm.events))
	copy(events, fm.events)
	return events
}

// processEvents processes failover events
func (fm *FailoverManager) processEvents() {
	defer fm.wg.Done()

	for {
		select {
		case <-fm.ctx.Done():
			return
		case event := <-fm.eventChan:
			if event == nil {
				return
			}
			// Process event (could send to monitoring, logging, etc.)
			fm.handleFailoverEvent(event)
		}
	}
}

// handleFailoverEvent handles a failover event
func (fm *FailoverManager) handleFailoverEvent(event *FailoverEvent) {
	// This could be extended to:
	// - Send notifications
	// - Update monitoring systems
	// - Log to external systems
	// - Trigger alerts
	// For now, we'll just ensure it's stored
}

// GetFailoverStats returns statistics about failovers
func (fm *FailoverManager) GetFailoverStats() map[string]interface{} {
	fm.mu.RLock()
	defer fm.mu.RUnlock()

	totalFailovers := len(fm.events)
	successfulFailovers := 0
	failedFailovers := 0
	avgDuration := time.Duration(0)

	for _, event := range fm.events {
		if event.Success {
			successfulFailovers++
		} else {
			failedFailovers++
		}
		avgDuration += event.Duration
	}

	if totalFailovers > 0 {
		avgDuration = avgDuration / time.Duration(totalFailovers)
	}

	return map[string]interface{}{
		"total_failovers":      totalFailovers,
		"successful_failovers": successfulFailovers,
		"failed_failovers":     failedFailovers,
		"average_duration":     avgDuration.String(),
		"active_nodes":         len(fm.nodes),
		"current_active_node":  fm.activeNode.ID,
		"policy":               fm.config.Policy,
	}
}
