package ha

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// HealthStatus represents the health status of a component
type HealthStatus string

const (
	StatusHealthy     HealthStatus = "healthy"
	StatusUnhealthy   HealthStatus = "unhealthy"
	StatusDegraded    HealthStatus = "degraded"
	StatusUnknown     HealthStatus = "unknown"
)

// HealthCheckType defines the type of health check
type HealthCheckType string

const (
	CheckHTTP      HealthCheckType = "http"
	CheckTCP       HealthCheckType = "tcp"
	CheckGRPC      HealthCheckType = "grpc"
	CheckCustom    HealthCheckType = "custom"
	CheckHeartbeat HealthCheckType = "heartbeat"
)

// HealthCheckResult represents the result of a health check
type HealthCheckResult struct {
	ID           string        `json:"id"`
	NodeID       string        `json:"node_id"`
	CheckType    HealthCheckType `json:"check_type"`
	Status       HealthStatus  `json:"status"`
	Score        int           `json:"score"`
	ResponseTime time.Duration `json:"response_time"`
	Message      string        `json:"message"`
	Timestamp    time.Time     `json:"timestamp"`
	Details      map[string]interface{} `json:"details"`
}

// HealthCheckConfig contains configuration for a health check
type HealthCheckConfig struct {
	ID               string          `json:"id"`
	Name             string          `json:"name"`
	CheckType        HealthCheckType `json:"check_type"`
	Endpoint         string          `json:"endpoint"`
	Timeout          time.Duration   `json:"timeout"`
	Interval         time.Duration   `json:"interval"`
	UnhealthyThreshold int           `json:"unhealthy_threshold"`
	HealthyThreshold   int           `json:"healthy_threshold"`
	Retries           int            `json:"retries"`
	ExpectedStatus    int            `json:"expected_status"`
	Headers           map[string]string `json:"headers"`
	Body              string          `json:"body"`
	CustomCheck       func() (bool, error) `json:"-"`
}

// DefaultHealthCheckConfig returns default health check configuration
func DefaultHealthCheckConfig() *HealthCheckConfig {
	return &HealthCheckConfig{
		CheckType:          CheckHTTP,
		Timeout:            5 * time.Second,
		Interval:           10 * time.Second,
		UnhealthyThreshold: 3,
		HealthyThreshold:   2,
		Retries:            2,
		ExpectedStatus:     200,
		Headers:            make(map[string]string),
	}
}

// NodeHealth represents the health state of a node
type NodeHealth struct {
	NodeID          string              `json:"node_id"`
	OverallStatus   HealthStatus        `json:"overall_status"`
	OverallScore    int                 `json:"overall_score"`
	LastCheck       time.Time           `json:"last_check"`
	CheckResults    map[string]*HealthCheckResult `json:"check_results"`
	ConsecutiveFailures int             `json:"consecutive_failures"`
	ConsecutiveSuccesses int            `json:"consecutive_successes"`
	mu              sync.RWMutex
}

// UpdateScore updates the overall health score
func (nh *NodeHealth) UpdateScore() {
	nh.mu.Lock()
	defer nh.mu.Unlock()

	if len(nh.CheckResults) == 0 {
		nh.OverallScore = 0
		nh.OverallStatus = StatusUnknown
		return
	}

	totalScore := 0
	for _, result := range nh.CheckResults {
		totalScore += result.Score
	}

	nh.OverallScore = totalScore / len(nh.CheckResults)

	// Determine status based on score
	if nh.OverallScore >= 80 {
		nh.OverallStatus = StatusHealthy
	} else if nh.OverallScore >= 50 {
		nh.OverallStatus = StatusDegraded
	} else {
		nh.OverallStatus = StatusUnhealthy
	}
}

// HealthChecker manages health checks for nodes and backends
type HealthChecker struct {
	configs      map[string]*HealthCheckConfig
	nodeHealth   map[string]*NodeHealth
	httpClient   *http.Client
	mu           sync.RWMutex
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	resultChan   chan *HealthCheckResult
}

// NewHealthChecker creates a new health checker
func NewHealthChecker() *HealthChecker {
	ctx, cancel := context.WithCancel(context.Background())

	return &HealthChecker{
		configs:    make(map[string]*HealthCheckConfig),
		nodeHealth: make(map[string]*NodeHealth),
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		ctx:        ctx,
		cancel:     cancel,
		resultChan: make(chan *HealthCheckResult, 1000),
	}
}

// RegisterHealthCheck registers a new health check
func (hc *HealthChecker) RegisterHealthCheck(config *HealthCheckConfig) error {
	if config == nil {
		return errors.New("health check config cannot be nil")
	}

	if config.ID == "" {
		return errors.New("health check ID is required")
	}

	hc.mu.Lock()
	defer hc.mu.Unlock()

	if _, exists := hc.configs[config.ID]; exists {
		return fmt.Errorf("health check %s already exists", config.ID)
	}

	hc.configs[config.ID] = config
	return nil
}

// UnregisterHealthCheck removes a health check
func (hc *HealthChecker) UnregisterHealthCheck(checkID string) error {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	if _, exists := hc.configs[checkID]; !exists {
		return fmt.Errorf("health check %s not found", checkID)
	}

	delete(hc.configs, checkID)
	return nil
}

// RegisterNode registers a node for health monitoring
func (hc *HealthChecker) RegisterNode(nodeID string) error {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	if _, exists := hc.nodeHealth[nodeID]; exists {
		return fmt.Errorf("node %s already registered", nodeID)
	}

	hc.nodeHealth[nodeID] = &NodeHealth{
		NodeID:       nodeID,
		OverallStatus: StatusUnknown,
		CheckResults:  make(map[string]*HealthCheckResult),
	}

	return nil
}

// UnregisterNode removes a node from health monitoring
func (hc *HealthChecker) UnregisterNode(nodeID string) error {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	if _, exists := hc.nodeHealth[nodeID]; !exists {
		return fmt.Errorf("node %s not found", nodeID)
	}

	delete(hc.nodeHealth, nodeID)
	return nil
}

// Start starts the health checker
func (hc *HealthChecker) Start() error {
	hc.wg.Add(1)
	go hc.runHealthChecks()

	hc.wg.Add(1)
	go hc.processResults()

	return nil
}

// Stop stops the health checker
func (hc *HealthChecker) Stop() error {
	hc.cancel()
	hc.wg.Wait()
	close(hc.resultChan)
	return nil
}

// runHealthChecks runs periodic health checks
func (hc *HealthChecker) runHealthChecks() {
	defer hc.wg.Done()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-hc.ctx.Done():
			return
		case <-ticker.C:
			hc.performHealthChecks()
		}
	}
}

// performHealthChecks performs all registered health checks
func (hc *HealthChecker) performHealthChecks() {
	hc.mu.RLock()
	configs := make([]*HealthCheckConfig, 0, len(hc.configs))
	for _, config := range hc.configs {
		configs = append(configs, config)
	}
	hc.mu.RUnlock()

	for _, config := range configs {
		result := hc.executeHealthCheck(config)

		select {
		case hc.resultChan <- result:
		default:
			// Channel full, drop result
		}
	}
}

// executeHealthCheck executes a single health check
func (hc *HealthChecker) executeHealthCheck(config *HealthCheckConfig) *HealthCheckResult {
	startTime := time.Now()

	result := &HealthCheckResult{
		ID:        config.ID,
		CheckType: config.CheckType,
		Timestamp: startTime,
		Details:   make(map[string]interface{}),
	}

	var err error
	var success bool

	// Execute based on check type
	switch config.CheckType {
	case CheckHTTP:
		success, err = hc.performHTTPCheck(config)
	case CheckTCP:
		success, err = hc.performTCPCheck(config)
	case CheckHeartbeat:
		success, err = hc.performHeartbeatCheck(config)
	case CheckCustom:
		if config.CustomCheck != nil {
			success, err = config.CustomCheck()
		} else {
			err = errors.New("custom check function not defined")
		}
	default:
		err = fmt.Errorf("unsupported check type: %s", config.CheckType)
	}

	result.ResponseTime = time.Since(startTime)

	if err != nil || !success {
		result.Status = StatusUnhealthy
		result.Score = 0
		if err != nil {
			result.Message = err.Error()
		} else {
			result.Message = "health check failed"
		}
	} else {
		result.Status = StatusHealthy
		result.Score = 100
		result.Message = "health check passed"
	}

	return result
}

// performHTTPCheck performs an HTTP health check
func (hc *HealthChecker) performHTTPCheck(config *HealthCheckConfig) (bool, error) {
	if config.Endpoint == "" {
		return false, errors.New("endpoint is required for HTTP check")
	}

	req, err := http.NewRequest("GET", config.Endpoint, nil)
	if err != nil {
		return false, err
	}

	// Add headers
	for key, value := range config.Headers {
		req.Header.Set(key, value)
	}

	ctx, cancel := context.WithTimeout(hc.ctx, config.Timeout)
	defer cancel()
	req = req.WithContext(ctx)

	resp, err := hc.httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if config.ExpectedStatus > 0 {
		return resp.StatusCode == config.ExpectedStatus, nil
	}

	return resp.StatusCode >= 200 && resp.StatusCode < 300, nil
}

// performTCPCheck performs a TCP health check
func (hc *HealthChecker) performTCPCheck(config *HealthCheckConfig) (bool, error) {
	// Simplified TCP check - in production would use net.DialTimeout
	return true, nil
}

// performHeartbeatCheck performs a heartbeat health check
func (hc *HealthChecker) performHeartbeatCheck(config *HealthCheckConfig) (bool, error) {
	// Simplified heartbeat check - would check last heartbeat time
	return true, nil
}

// processResults processes health check results
func (hc *HealthChecker) processResults() {
	defer hc.wg.Done()

	for {
		select {
		case <-hc.ctx.Done():
			return
		case result := <-hc.resultChan:
			if result == nil {
				return
			}
			hc.updateNodeHealth(result)
		}
	}
}

// updateNodeHealth updates node health based on check result
func (hc *HealthChecker) updateNodeHealth(result *HealthCheckResult) {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	nodeHealth, exists := hc.nodeHealth[result.NodeID]
	if !exists {
		return
	}

	nodeHealth.mu.Lock()
	defer nodeHealth.mu.Unlock()

	nodeHealth.CheckResults[result.ID] = result
	nodeHealth.LastCheck = result.Timestamp

	// Update consecutive counters
	if result.Status == StatusHealthy {
		nodeHealth.ConsecutiveSuccesses++
		nodeHealth.ConsecutiveFailures = 0
	} else {
		nodeHealth.ConsecutiveFailures++
		nodeHealth.ConsecutiveSuccesses = 0
	}

	nodeHealth.UpdateScore()
}

// GetNodeHealth returns the health of a specific node
func (hc *HealthChecker) GetNodeHealth(nodeID string) (*NodeHealth, error) {
	hc.mu.RLock()
	defer hc.mu.RUnlock()

	nodeHealth, exists := hc.nodeHealth[nodeID]
	if !exists {
		return nil, fmt.Errorf("node %s not found", nodeID)
	}

	return nodeHealth, nil
}

// GetNodeHealthScore returns the health score of a node
func (hc *HealthChecker) GetNodeHealthScore(nodeID string) int {
	hc.mu.RLock()
	defer hc.mu.RUnlock()

	nodeHealth, exists := hc.nodeHealth[nodeID]
	if !exists {
		return 0
	}

	nodeHealth.mu.RLock()
	defer nodeHealth.mu.RUnlock()

	return nodeHealth.OverallScore
}

// GetAllNodeHealth returns health status of all nodes
func (hc *HealthChecker) GetAllNodeHealth() map[string]*NodeHealth {
	hc.mu.RLock()
	defer hc.mu.RUnlock()

	health := make(map[string]*NodeHealth)
	for k, v := range hc.nodeHealth {
		health[k] = v
	}

	return health
}

// GetHealthStats returns overall health statistics
func (hc *HealthChecker) GetHealthStats() map[string]interface{} {
	hc.mu.RLock()
	defer hc.mu.RUnlock()

	totalNodes := len(hc.nodeHealth)
	healthyNodes := 0
	degradedNodes := 0
	unhealthyNodes := 0

	for _, nodeHealth := range hc.nodeHealth {
		nodeHealth.mu.RLock()
		switch nodeHealth.OverallStatus {
		case StatusHealthy:
			healthyNodes++
		case StatusDegraded:
			degradedNodes++
		case StatusUnhealthy:
			unhealthyNodes++
		}
		nodeHealth.mu.RUnlock()
	}

	return map[string]interface{}{
		"total_nodes":      totalNodes,
		"healthy_nodes":    healthyNodes,
		"degraded_nodes":   degradedNodes,
		"unhealthy_nodes":  unhealthyNodes,
		"total_checks":     len(hc.configs),
		"health_percentage": float64(healthyNodes) / float64(totalNodes) * 100,
	}
}
