package ha

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

// LoadBalanceAlgorithm defines the type of load balancing algorithm
type LoadBalanceAlgorithm string

const (
	AlgorithmRoundRobin        LoadBalanceAlgorithm = "round_robin"
	AlgorithmWeightedRoundRobin LoadBalanceAlgorithm = "weighted_round_robin"
	AlgorithmLeastConnections  LoadBalanceAlgorithm = "least_connections"
	AlgorithmHealthBased       LoadBalanceAlgorithm = "health_based"
	AlgorithmRandom            LoadBalanceAlgorithm = "random"
	AlgorithmIPHash            LoadBalanceAlgorithm = "ip_hash"
)

// Backend represents a backend server
type Backend struct {
	ID             string        `json:"id"`
	Address        string        `json:"address"`
	Port           int           `json:"port"`
	Weight         int           `json:"weight"`
	MaxConnections int           `json:"max_connections"`
	CurrentConns   int64         `json:"current_connections"`
	HealthScore    int           `json:"health_score"`
	IsActive       bool          `json:"is_active"`
	Region         string        `json:"region"`
	Zone           string        `json:"zone"`
	LastUsed       time.Time     `json:"last_used"`
	RequestCount   int64         `json:"request_count"`
	mu             sync.RWMutex
}

// IncrementConnections increments the connection count
func (b *Backend) IncrementConnections() int64 {
	return atomic.AddInt64(&b.CurrentConns, 1)
}

// DecrementConnections decrements the connection count
func (b *Backend) DecrementConnections() int64 {
	return atomic.AddInt64(&b.CurrentConns, -1)
}

// GetConnections returns current connection count
func (b *Backend) GetConnections() int64 {
	return atomic.LoadInt64(&b.CurrentConns)
}

// IncrementRequestCount increments the request counter
func (b *Backend) IncrementRequestCount() int64 {
	return atomic.AddInt64(&b.RequestCount, 1)
}

// CanAcceptConnections checks if backend can accept more connections
func (b *Backend) CanAcceptConnections() bool {
	if b.MaxConnections == 0 {
		return true
	}
	return b.GetConnections() < int64(b.MaxConnections)
}

// LoadBalancerConfig contains configuration for load balancer
type LoadBalancerConfig struct {
	Algorithm          LoadBalanceAlgorithm `json:"algorithm"`
	HealthCheckEnabled bool                 `json:"health_check_enabled"`
	MinHealthScore     int                  `json:"min_health_score"`
	RetryAttempts      int                  `json:"retry_attempts"`
	RetryDelay         time.Duration        `json:"retry_delay"`
	StickySession      bool                 `json:"sticky_session"`
	SessionTTL         time.Duration        `json:"session_ttl"`
}

// DefaultLoadBalancerConfig returns default configuration
func DefaultLoadBalancerConfig() *LoadBalancerConfig {
	return &LoadBalancerConfig{
		Algorithm:          AlgorithmRoundRobin,
		HealthCheckEnabled: true,
		MinHealthScore:     70,
		RetryAttempts:      3,
		RetryDelay:         100 * time.Millisecond,
		StickySession:      false,
		SessionTTL:         30 * time.Minute,
	}
}

// LoadBalancer manages load balancing across backends
type LoadBalancer struct {
	config        *LoadBalancerConfig
	backends      map[string]*Backend
	backendList   []*Backend
	algorithm     LoadBalanceAlgorithm
	healthChecker *HealthChecker
	mu            sync.RWMutex
	roundRobinIdx uint64
	sessionCache  map[string]*Backend
	ctx           context.Context
	cancel        context.CancelFunc
}

// NewLoadBalancer creates a new load balancer
func NewLoadBalancer(config *LoadBalancerConfig) *LoadBalancer {
	if config == nil {
		config = DefaultLoadBalancerConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &LoadBalancer{
		config:       config,
		backends:     make(map[string]*Backend),
		backendList:  make([]*Backend, 0),
		algorithm:    config.Algorithm,
		sessionCache: make(map[string]*Backend),
		ctx:          ctx,
		cancel:       cancel,
	}
}

// SetHealthChecker sets the health checker
func (lb *LoadBalancer) SetHealthChecker(hc *HealthChecker) {
	lb.healthChecker = hc
}

// AddBackend adds a backend server
func (lb *LoadBalancer) AddBackend(backend *Backend) error {
	if backend == nil {
		return errors.New("backend cannot be nil")
	}

	lb.mu.Lock()
	defer lb.mu.Unlock()

	if _, exists := lb.backends[backend.ID]; exists {
		return fmt.Errorf("backend %s already exists", backend.ID)
	}

	lb.backends[backend.ID] = backend
	lb.backendList = append(lb.backendList, backend)

	return nil
}

// RemoveBackend removes a backend server
func (lb *LoadBalancer) RemoveBackend(backendID string) error {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	if _, exists := lb.backends[backendID]; !exists {
		return fmt.Errorf("backend %s not found", backendID)
	}

	delete(lb.backends, backendID)

	// Remove from list
	for i, b := range lb.backendList {
		if b.ID == backendID {
			lb.backendList = append(lb.backendList[:i], lb.backendList[i+1:]...)
			break
		}
	}

	// Remove from session cache
	for sessionID, backend := range lb.sessionCache {
		if backend.ID == backendID {
			delete(lb.sessionCache, sessionID)
		}
	}

	return nil
}

// GetBackend retrieves a backend by ID
func (lb *LoadBalancer) GetBackend(backendID string) (*Backend, error) {
	lb.mu.RLock()
	defer lb.mu.RUnlock()

	backend, exists := lb.backends[backendID]
	if !exists {
		return nil, fmt.Errorf("backend %s not found", backendID)
	}

	return backend, nil
}

// GetAllBackends returns all backends
func (lb *LoadBalancer) GetAllBackends() []*Backend {
	lb.mu.RLock()
	defer lb.mu.RUnlock()

	backends := make([]*Backend, len(lb.backendList))
	copy(backends, lb.backendList)
	return backends
}

// SelectBackend selects a backend based on the load balancing algorithm
func (lb *LoadBalancer) SelectBackend(sessionID ...string) (*Backend, error) {
	lb.mu.RLock()
	defer lb.mu.RUnlock()

	// Check sticky session
	if lb.config.StickySession && len(sessionID) > 0 {
		if backend, exists := lb.sessionCache[sessionID[0]]; exists {
			if backend.IsActive && backend.HealthScore >= lb.config.MinHealthScore {
				return backend, nil
			}
			// Remove unhealthy sticky session
			delete(lb.sessionCache, sessionID[0])
		}
	}

	// Get healthy backends
	healthyBackends := lb.getHealthyBackends()
	if len(healthyBackends) == 0 {
		return nil, errors.New("no healthy backends available")
	}

	// Select based on algorithm
	var selected *Backend
	switch lb.algorithm {
	case AlgorithmRoundRobin:
		selected = lb.selectRoundRobin(healthyBackends)
	case AlgorithmWeightedRoundRobin:
		selected = lb.selectWeightedRoundRobin(healthyBackends)
	case AlgorithmLeastConnections:
		selected = lb.selectLeastConnections(healthyBackends)
	case AlgorithmHealthBased:
		selected = lb.selectHealthBased(healthyBackends)
	case AlgorithmRandom:
		selected = lb.selectRandom(healthyBackends)
	default:
		selected = lb.selectRoundRobin(healthyBackends)
	}

	// Store in session cache if sticky session is enabled
	if lb.config.StickySession && len(sessionID) > 0 && selected != nil {
		lb.sessionCache[sessionID[0]] = selected
	}

	return selected, nil
}

// getHealthyBackends returns list of healthy backends
func (lb *LoadBalancer) getHealthyBackends() []*Backend {
	healthy := make([]*Backend, 0)
	for _, backend := range lb.backendList {
		if backend.IsActive && backend.HealthScore >= lb.config.MinHealthScore && backend.CanAcceptConnections() {
			healthy = append(healthy, backend)
		}
	}
	return healthy
}

// selectRoundRobin selects backend using round-robin algorithm
func (lb *LoadBalancer) selectRoundRobin(backends []*Backend) *Backend {
	if len(backends) == 0 {
		return nil
	}

	idx := atomic.AddUint64(&lb.roundRobinIdx, 1) - 1
	return backends[idx%uint64(len(backends))]
}

// selectWeightedRoundRobin selects backend using weighted round-robin
func (lb *LoadBalancer) selectWeightedRoundRobin(backends []*Backend) *Backend {
	if len(backends) == 0 {
		return nil
	}

	// Calculate total weight
	totalWeight := 0
	for _, b := range backends {
		totalWeight += b.Weight
	}

	if totalWeight == 0 {
		return lb.selectRoundRobin(backends)
	}

	// Select based on weight
	r := rand.Intn(totalWeight)
	currentWeight := 0
	for _, b := range backends {
		currentWeight += b.Weight
		if r < currentWeight {
			return b
		}
	}

	return backends[0]
}

// selectLeastConnections selects backend with least connections
func (lb *LoadBalancer) selectLeastConnections(backends []*Backend) *Backend {
	if len(backends) == 0 {
		return nil
	}

	var selected *Backend
	minConns := int64(-1)

	for _, b := range backends {
		conns := b.GetConnections()
		if minConns == -1 || conns < minConns {
			minConns = conns
			selected = b
		}
	}

	return selected
}

// selectHealthBased selects backend based on health score
func (lb *LoadBalancer) selectHealthBased(backends []*Backend) *Backend {
	if len(backends) == 0 {
		return nil
	}

	var selected *Backend
	maxScore := 0

	for _, b := range backends {
		if b.HealthScore > maxScore {
			maxScore = b.HealthScore
			selected = b
		}
	}

	return selected
}

// selectRandom selects a random backend
func (lb *LoadBalancer) selectRandom(backends []*Backend) *Backend {
	if len(backends) == 0 {
		return nil
	}

	return backends[rand.Intn(len(backends))]
}

// ReleaseBackend releases a backend after use
func (lb *LoadBalancer) ReleaseBackend(backendID string) error {
	backend, err := lb.GetBackend(backendID)
	if err != nil {
		return err
	}

	backend.DecrementConnections()
	return nil
}

// UpdateBackendHealth updates backend health score
func (lb *LoadBalancer) UpdateBackendHealth(backendID string, healthScore int) error {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	backend, exists := lb.backends[backendID]
	if !exists {
		return fmt.Errorf("backend %s not found", backendID)
	}

	backend.HealthScore = healthScore
	backend.IsActive = healthScore >= lb.config.MinHealthScore

	return nil
}

// SetAlgorithm changes the load balancing algorithm
func (lb *LoadBalancer) SetAlgorithm(algorithm LoadBalanceAlgorithm) {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	lb.algorithm = algorithm
}

// GetStats returns load balancer statistics
func (lb *LoadBalancer) GetStats() map[string]interface{} {
	lb.mu.RLock()
	defer lb.mu.RUnlock()

	totalRequests := int64(0)
	totalConnections := int64(0)
	activeBackends := 0

	for _, b := range lb.backends {
		totalRequests += b.RequestCount
		totalConnections += b.GetConnections()
		if b.IsActive {
			activeBackends++
		}
	}

	return map[string]interface{}{
		"algorithm":          lb.algorithm,
		"total_backends":     len(lb.backends),
		"active_backends":    activeBackends,
		"total_requests":     totalRequests,
		"total_connections":  totalConnections,
		"sticky_sessions":    len(lb.sessionCache),
		"min_health_score":   lb.config.MinHealthScore,
	}
}

// GetBackendStats returns statistics for a specific backend
func (lb *LoadBalancer) GetBackendStats(backendID string) (map[string]interface{}, error) {
	backend, err := lb.GetBackend(backendID)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"id":                backend.ID,
		"address":           backend.Address,
		"port":              backend.Port,
		"weight":            backend.Weight,
		"current_connections": backend.GetConnections(),
		"max_connections":   backend.MaxConnections,
		"health_score":      backend.HealthScore,
		"is_active":         backend.IsActive,
		"request_count":     backend.RequestCount,
		"region":            backend.Region,
		"zone":              backend.Zone,
	}, nil
}

// Stop stops the load balancer
func (lb *LoadBalancer) Stop() error {
	lb.cancel()
	return nil
}

// ClearSessionCache clears the session cache
func (lb *LoadBalancer) ClearSessionCache() {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	lb.sessionCache = make(map[string]*Backend)
}
