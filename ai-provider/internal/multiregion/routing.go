package multiregion

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"
)

// RoutingStrategy defines the strategy for routing requests
type RoutingStrategy string

const (
	RoutingGeoLatency    RoutingStrategy = "geo_latency"
	RoutingRoundRobin    RoutingStrategy = "round_robin"
	RoutingWeighted      RoutingStrategy = "weighted"
	RoutingLeastLoad     RoutingStrategy = "least_load"
	RoutingFailover      RoutingStrategy = "failover"
	RoutingLatencyBased  RoutingStrategy = "latency_based"
	RoutingRandom        RoutingStrategy = "random"
)

// RoutingPolicy represents a routing policy
type RoutingPolicy struct {
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	Strategy        RoutingStrategy   `json:"strategy"`
	Priority        int               `json:"priority"`
	Enabled         bool              `json:"enabled"`
	Conditions      map[string]string `json:"conditions"`
	RegionWeights   map[string]int    `json:"region_weights"`
	FallbackRegions []string          `json:"fallback_regions"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
}

// GeoLocation represents a geographic location
type GeoLocation struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Country   string  `json:"country"`
	City      string  `json:"city"`
	Region    string  `json:"region"`
	ISP       string  `json:"isp"`
	Timezone  string  `json:"timezone"`
}

// RoutingRequest represents a routing request
type RoutingRequest struct {
	ID           string       `json:"id"`
	ClientIP     string       `json:"client_ip"`
	GeoLocation  *GeoLocation `json:"geo_location"`
	ServiceType  string       `json:"service_type"`
	Headers      map[string]string `json:"headers"`
	Timestamp    time.Time    `json:"timestamp"`
	SessionID    string       `json:"session_id"`
	UserID       string       `json:"user_id"`
	Priority     int          `json:"priority"`
}

// RoutingDecision represents a routing decision
type RoutingDecision struct {
	RequestID      string        `json:"request_id"`
	SelectedRegion string        `json:"selected_region"`
	Strategy       RoutingStrategy `json:"strategy"`
	Latency        time.Duration `json:"latency"`
	Reason         string        `json:"reason"`
	Alternatives   []string      `json:"alternatives"`
	Timestamp      time.Time     `json:"timestamp"`
	Score          float64       `json:"score"`
}

// RegionMetrics represents metrics for a region
type RegionMetrics struct {
	RegionID        string        `json:"region_id"`
	Latency         time.Duration `json:"latency"`
	Load            int           `json:"load"`
	Capacity        int           `json:"capacity"`
	HealthScore     int           `json:"health_score"`
	RequestCount    int64         `json:"request_count"`
	ErrorRate       float64       `json:"error_rate"`
	LastUpdate      time.Time     `json:"last_update"`
	ActiveRequests  int           `json:"active_requests"`
	AvgResponseTime time.Duration `json:"avg_response_time"`
}

// RoutingRule represents a routing rule
type RoutingRule struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Priority    int               `json:"priority"`
	Conditions  map[string]string `json:"conditions"`
	TargetRegion string           `json:"target_region"`
	Enabled     bool              `json:"enabled"`
	CreatedAt   time.Time         `json:"created_at"`
}

// GeoRoutingConfig contains configuration for geo-routing
type GeoRoutingConfig struct {
	DefaultStrategy    RoutingStrategy `json:"default_strategy"`
	HealthCheckEnabled bool            `json:"health_check_enabled"`
	MinHealthScore     int             `json:"min_health_score"`
	LatencyWeight      float64         `json:"latency_weight"`
	LoadWeight         float64         `json:"load_weight"`
	HealthWeight       float64         `json:"health_weight"`
	UpdateInterval     time.Duration   `json:"update_interval"`
	MaxLatency         time.Duration   `json:"max_latency"`
	EnableStickySession bool           `json:"enable_sticky_session"`
	SessionTTL         time.Duration   `json:"session_ttl"`
}

// DefaultGeoRoutingConfig returns default geo-routing configuration
func DefaultGeoRoutingConfig() *GeoRoutingConfig {
	return &GeoRoutingConfig{
		DefaultStrategy:    RoutingGeoLatency,
		HealthCheckEnabled: true,
		MinHealthScore:     70,
		LatencyWeight:      0.5,
		LoadWeight:         0.3,
		HealthWeight:       0.2,
		UpdateInterval:     30 * time.Second,
		MaxLatency:         200 * time.Millisecond,
		EnableStickySession: true,
		SessionTTL:         30 * time.Minute,
	}
}

// GeoRouter manages geo-based routing
type GeoRouter struct {
	config        *GeoRoutingConfig
	regions       map[string]*Region
	metrics       map[string]*RegionMetrics
	policies      map[string]*RoutingPolicy
	rules         map[string]*RoutingRule
	sessions      map[string]string
	decisions     []*RoutingDecision
	mu            sync.RWMutex
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup
	decisionChan  chan *RoutingDecision
}

// NewGeoRouter creates a new geo-router
func NewGeoRouter(config *GeoRoutingConfig) *GeoRouter {
	if config == nil {
		config = DefaultGeoRoutingConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &GeoRouter{
		config:       config,
		regions:      make(map[string]*Region),
		metrics:      make(map[string]*RegionMetrics),
		policies:     make(map[string]*RoutingPolicy),
		rules:        make(map[string]*RoutingRule),
		sessions:     make(map[string]string),
		decisions:    make([]*RoutingDecision, 0),
		ctx:          ctx,
		cancel:       cancel,
		decisionChan: make(chan *RoutingDecision, 1000),
	}
}

// RegisterRegion registers a region for routing
func (gr *GeoRouter) RegisterRegion(region *Region) error {
	if region == nil {
		return errors.New("region cannot be nil")
	}

	gr.mu.Lock()
	defer gr.mu.Unlock()

	if _, exists := gr.regions[region.ID]; exists {
		return fmt.Errorf("region %s already registered", region.ID)
	}

	gr.regions[region.ID] = region
	gr.metrics[region.ID] = &RegionMetrics{
		RegionID:   region.ID,
		Capacity:   region.Capacity,
		LastUpdate: time.Now(),
	}

	return nil
}

// UnregisterRegion unregisters a region from routing
func (gr *GeoRouter) UnregisterRegion(regionID string) error {
	gr.mu.Lock()
	defer gr.mu.Unlock()

	if _, exists := gr.regions[regionID]; !exists {
		return fmt.Errorf("region %s not found", regionID)
	}

	delete(gr.regions, regionID)
	delete(gr.metrics, regionID)

	return nil
}

// AddRoutingPolicy adds a routing policy
func (gr *GeoRouter) AddRoutingPolicy(policy *RoutingPolicy) error {
	if policy == nil {
		return errors.New("policy cannot be nil")
	}

	gr.mu.Lock()
	defer gr.mu.Unlock()

	if _, exists := gr.policies[policy.ID]; exists {
		return fmt.Errorf("policy %s already exists", policy.ID)
	}

	policy.CreatedAt = time.Now()
	policy.UpdatedAt = time.Now()
	gr.policies[policy.ID] = policy

	return nil
}

// RemoveRoutingPolicy removes a routing policy
func (gr *GeoRouter) RemoveRoutingPolicy(policyID string) error {
	gr.mu.Lock()
	defer gr.mu.Unlock()

	if _, exists := gr.policies[policyID]; !exists {
		return fmt.Errorf("policy %s not found", policyID)
	}

	delete(gr.policies, policyID)
	return nil
}

// AddRoutingRule adds a routing rule
func (gr *GeoRouter) AddRoutingRule(rule *RoutingRule) error {
	if rule == nil {
		return errors.New("rule cannot be nil")
	}

	gr.mu.Lock()
	defer gr.mu.Unlock()

	if _, exists := gr.rules[rule.ID]; exists {
		return fmt.Errorf("rule %s already exists", rule.ID)
	}

	rule.CreatedAt = time.Now()
	gr.rules[rule.ID] = rule

	return nil
}

// RemoveRoutingRule removes a routing rule
func (gr *GeoRouter) RemoveRoutingRule(ruleID string) error {
	gr.mu.Lock()
	defer gr.mu.Unlock()

	if _, exists := gr.rules[ruleID]; !exists {
		return fmt.Errorf("rule %s not found", ruleID)
	}

	delete(gr.rules, ruleID)
	return nil
}

// RouteRequest routes a request to the appropriate region
func (gr *GeoRouter) RouteRequest(request *RoutingRequest) (*RoutingDecision, error) {
	if request == nil {
		return nil, errors.New("request cannot be nil")
	}

	// Check for sticky session
	if gr.config.EnableStickySession && request.SessionID != "" {
		if regionID, exists := gr.getStickySession(request.SessionID); exists {
			if gr.isRegionAvailable(regionID) {
				return gr.createDecision(request, regionID, "sticky_session", 1.0), nil
			}
		}
	}

	// Apply custom routing rules first
	if regionID := gr.applyRoutingRules(request); regionID != "" {
		return gr.createDecision(request, regionID, "custom_rule", 1.0), nil
	}

	// Apply routing policies
	if regionID := gr.applyRoutingPolicies(request); regionID != "" {
		return gr.createDecision(request, regionID, "policy", 1.0), nil
	}

	// Use default routing strategy
	regionID, reason, score := gr.routeByStrategy(request)
	if regionID == "" {
		return nil, errors.New("no available region for routing")
	}

	decision := gr.createDecision(request, regionID, reason, score)

	// Store sticky session
	if gr.config.EnableStickySession && request.SessionID != "" {
		gr.setStickySession(request.SessionID, regionID)
	}

	// Record decision
	gr.recordDecision(decision)

	return decision, nil
}

// routeByStrategy routes based on the configured strategy
func (gr *GeoRouter) routeByStrategy(request *RoutingRequest) (string, string, float64) {
	switch gr.config.DefaultStrategy {
	case RoutingGeoLatency:
		return gr.routeByGeoLatency(request)
	case RoutingRoundRobin:
		return gr.routeRoundRobin()
	case RoutingWeighted:
		return gr.routeWeighted()
	case RoutingLeastLoad:
		return gr.routeLeastLoad()
	case RoutingFailover:
		return gr.routeFailover()
	case RoutingLatencyBased:
		return gr.routeByLatency()
	case RoutingRandom:
		return gr.routeRandom()
	default:
		return gr.routeByGeoLatency(request)
	}
}

// routeByGeoLatency routes based on geographic location and latency
func (gr *GeoRouter) routeByGeoLatency(request *RoutingRequest) (string, string, float64) {
	gr.mu.RLock()
	defer gr.mu.RUnlock()

	if request.GeoLocation == nil {
		return gr.routeByLatency()
	}

	type regionScore struct {
		id    string
		score float64
	}

	scores := make([]regionScore, 0)

	for regionID, region := range gr.regions {
		if !gr.isRegionAvailableUnsafe(regionID) {
			continue
		}

		metrics := gr.metrics[regionID]
		score := gr.calculateRegionScore(region, metrics, request.GeoLocation)
		scores = append(scores, regionScore{id: regionID, score: score})
	}

	if len(scores) == 0 {
		return "", "no_available_regions", 0
	}

	// Find best score
	best := scores[0]
	for _, s := range scores {
		if s.score > best.score {
			best = s
		}
	}

	return best.id, "geo_latency", best.score
}

// calculateRegionScore calculates a score for a region
func (gr *GeoRouter) calculateRegionScore(region *Region, metrics *RegionMetrics, geoLocation *GeoLocation) float64 {
	// Calculate distance-based latency
	distance := gr.calculateDistance(geoLocation, region)
	distanceScore := 1.0 - (distance / 20000.0) // Normalize to 0-1 (earth circumference ~40,000 km)

	// Calculate load score
	loadScore := 1.0 - (float64(metrics.Load) / float64(region.Capacity))

	// Calculate health score
	healthScore := float64(region.HealthScore) / 100.0

	// Calculate latency score
	latencyScore := 1.0 - (float64(metrics.Latency) / float64(gr.config.MaxLatency))
	if latencyScore < 0 {
		latencyScore = 0
	}

	// Combine scores with weights
	totalScore := (distanceScore * gr.config.LatencyWeight) +
		(loadScore * gr.config.LoadWeight) +
		(healthScore * gr.config.HealthWeight) +
		(latencyScore * 0.2)

	return totalScore
}

// calculateDistance calculates distance between two geographic points
func (gr *GeoRouter) calculateDistance(geoLocation *GeoLocation, region *Region) float64 {
	// Simplified distance calculation (Haversine formula)
	// In production, would use proper geo libraries
	lat1 := geoLocation.Latitude * math.Pi / 180
	lat2 := 0.0 // Would get from region metadata
	lon1 := geoLocation.Longitude * math.Pi / 180
	lon2 := 0.0 // Would get from region metadata

	dlat := lat2 - lat1
	dlon := lon2 - lon1

	a := math.Sin(dlat/2)*math.Sin(dlat/2) + math.Cos(lat1)*math.Cos(lat2)*math.Sin(dlon/2)*math.Sin(dlon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return 6371 * c // Earth radius in km
}

// routeByLatency routes based on latency only
func (gr *GeoRouter) routeByLatency() (string, string, float64) {
	gr.mu.RLock()
	defer gr.mu.RUnlock()

	var bestRegion string
	bestLatency := time.Duration(math.MaxInt64)

	for regionID, metrics := range gr.metrics {
		if !gr.isRegionAvailableUnsafe(regionID) {
			continue
		}

		if metrics.Latency < bestLatency {
			bestLatency = metrics.Latency
			bestRegion = regionID
		}
	}

	if bestRegion == "" {
		return "", "no_available_regions", 0
	}

	score := 1.0 - (float64(bestLatency) / float64(gr.config.MaxLatency))
	return bestRegion, "latency_based", score
}

// routeRoundRobin routes using round-robin strategy
func (gr *GeoRouter) routeRoundRobin() (string, string, float64) {
	gr.mu.RLock()
	defer gr.mu.RUnlock()

	availableRegions := gr.getAvailableRegionsUnsafe()
	if len(availableRegions) == 0 {
		return "", "no_available_regions", 0
	}

	// Simple round-robin (in production, would maintain state)
	regionID := availableRegions[0]
	return regionID, "round_robin", 1.0
}

// routeWeighted routes based on weights
func (gr *GeoRouter) routeWeighted() (string, string, float64) {
	gr.mu.RLock()
	defer gr.mu.RUnlock()

	availableRegions := gr.getAvailableRegionsUnsafe()
	if len(availableRegions) == 0 {
		return "", "no_available_regions", 0
	}

	// Calculate total weight
	totalWeight := 0
	for _, regionID := range availableRegions {
		if region, exists := gr.regions[regionID]; exists {
			totalWeight += region.Priority
		}
	}

	if totalWeight == 0 {
		return availableRegions[0], "weighted", 1.0
	}

	// Weighted selection (simplified)
	regionID := availableRegions[0]
	return regionID, "weighted", float64(gr.regions[regionID].Priority) / float64(totalWeight)
}

// routeLeastLoad routes to region with least load
func (gr *GeoRouter) routeLeastLoad() (string, string, float64) {
	gr.mu.RLock()
	defer gr.mu.RUnlock()

	var bestRegion string
	minLoad := math.MaxInt64

	for regionID, region := range gr.regions {
		if !gr.isRegionAvailableUnsafe(regionID) {
			continue
		}

		loadPercentage := (region.CurrentLoad * 100) / region.Capacity
		if loadPercentage < minLoad {
			minLoad = loadPercentage
			bestRegion = regionID
		}
	}

	if bestRegion == "" {
		return "", "no_available_regions", 0
	}

	score := 1.0 - (float64(minLoad) / 100.0)
	return bestRegion, "least_load", score
}

// routeFailover routes using failover strategy
func (gr *GeoRouter) routeFailover() (string, string, float64) {
	gr.mu.RLock()
	defer gr.mu.RUnlock()

	// Try primary regions first
	for _, region := range gr.regions {
		if region.Priority == 100 && gr.isRegionAvailableUnsafe(region.ID) {
			return region.ID, "failover_primary", 1.0
		}
	}

	// Fall back to secondary regions
	for _, region := range gr.regions {
		if gr.isRegionAvailableUnsafe(region.ID) {
			return region.ID, "failover_secondary", 0.8
		}
	}

	return "", "no_available_regions", 0
}

// routeRandom routes randomly
func (gr *GeoRouter) routeRandom() (string, string, float64) {
	gr.mu.RLock()
	defer gr.mu.RUnlock()

	availableRegions := gr.getAvailableRegionsUnsafe()
	if len(availableRegions) == 0 {
		return "", "no_available_regions", 0
	}

	// Simple random selection (first available)
	regionID := availableRegions[0]
	return regionID, "random", 1.0
}

// applyRoutingRules applies custom routing rules
func (gr *GeoRouter) applyRoutingRules(request *RoutingRequest) string {
	gr.mu.RLock()
	defer gr.mu.RUnlock()

	for _, rule := range gr.rules {
		if !rule.Enabled {
			continue
		}

		if gr.matchesConditions(request, rule.Conditions) {
			return rule.TargetRegion
		}
	}

	return ""
}

// applyRoutingPolicies applies routing policies
func (gr *GeoRouter) applyRoutingPolicies(request *RoutingRequest) string {
	gr.mu.RLock()
	defer gr.mu.RUnlock()

	for _, policy := range gr.policies {
		if !policy.Enabled {
			continue
		}

		if gr.matchesConditions(request, policy.Conditions) {
			// Apply policy strategy
			regionID, _, _ := gr.routeByStrategy(request)
			return regionID
		}
	}

	return ""
}

// matchesConditions checks if a request matches conditions
func (gr *GeoRouter) matchesConditions(request *RoutingRequest, conditions map[string]string) bool {
	for key, value := range conditions {
		switch key {
		case "country":
			if request.GeoLocation == nil || request.GeoLocation.Country != value {
				return false
			}
		case "service_type":
			if request.ServiceType != value {
				return false
			}
		case "user_id":
			if request.UserID != value {
				return false
			}
		}
	}
	return true
}

// isRegionAvailable checks if a region is available
func (gr *GeoRouter) isRegionAvailable(regionID string) bool {
	gr.mu.RLock()
	defer gr.mu.RUnlock()
	return gr.isRegionAvailableUnsafe(regionID)
}

// isRegionAvailableUnsafe checks if a region is available without locking
func (gr *GeoRouter) isRegionAvailableUnsafe(regionID string) bool {
	region, exists := gr.regions[regionID]
	if !exists {
		return false
	}

	if region.GetStatus() != RegionActive {
		return false
	}

	if gr.config.HealthCheckEnabled && region.HealthScore < gr.config.MinHealthScore {
		return false
	}

	return true
}

// getAvailableRegionsUnsafe returns available regions without locking
func (gr *GeoRouter) getAvailableRegionsUnsafe() []string {
	regions := make([]string, 0)
	for regionID := range gr.regions {
		if gr.isRegionAvailableUnsafe(regionID) {
			regions = append(regions, regionID)
		}
	}
	return regions
}

// createDecision creates a routing decision
func (gr *GeoRouter) createDecision(request *RoutingRequest, regionID, reason string, score float64) *RoutingDecision {
	decision := &RoutingDecision{
		RequestID:      request.ID,
		SelectedRegion: regionID,
		Strategy:       gr.config.DefaultStrategy,
		Reason:         reason,
		Alternatives:   gr.getAvailableRegionsUnsafe(),
		Timestamp:      time.Now(),
		Score:          score,
	}

	if metrics, exists := gr.metrics[regionID]; exists {
		decision.Latency = metrics.Latency
	}

	return decision
}

// recordDecision records a routing decision
func (gr *GeoRouter) recordDecision(decision *RoutingDecision) {
	gr.mu.Lock()
	defer gr.mu.Unlock()

	gr.decisions = append(gr.decisions, decision)

	// Keep only last 1000 decisions
	if len(gr.decisions) > 1000 {
		gr.decisions = gr.decisions[len(gr.decisions)-1000:]
	}

	select {
	case gr.decisionChan <- decision:
	default:
		// Channel full, drop event
	}
}

// getStickySession gets sticky session region
func (gr *GeoRouter) getStickySession(sessionID string) (string, bool) {
	gr.mu.RLock()
	defer gr.mu.RUnlock()
	regionID, exists := gr.sessions[sessionID]
	return regionID, exists
}

// setStickySession sets sticky session region
func (gr *GeoRouter) setStickySession(sessionID, regionID string) {
	gr.mu.Lock()
	defer gr.mu.Unlock()
	gr.sessions[sessionID] = regionID
}

// UpdateRegionMetrics updates metrics for a region
func (gr *GeoRouter) UpdateRegionMetrics(regionID string, metrics *RegionMetrics) error {
	gr.mu.Lock()
	defer gr.mu.Unlock()

	if _, exists := gr.regions[regionID]; !exists {
		return fmt.Errorf("region %s not found", regionID)
	}

	metrics.RegionID = regionID
	metrics.LastUpdate = time.Now()
	gr.metrics[regionID] = metrics

	return nil
}

// GetRegionMetrics returns metrics for a region
func (gr *GeoRouter) GetRegionMetrics(regionID string) (*RegionMetrics, error) {
	gr.mu.RLock()
	defer gr.mu.RUnlock()

	metrics, exists := gr.metrics[regionID]
	if !exists {
		return nil, fmt.Errorf("metrics for region %s not found", regionID)
	}

	return metrics, nil
}

// GetRoutingDecisions returns recent routing decisions
func (gr *GeoRouter) GetRoutingDecisions(limit int) []*RoutingDecision {
	gr.mu.RLock()
	defer gr.mu.RUnlock()

	if limit <= 0 || limit > len(gr.decisions) {
		limit = len(gr.decisions)
	}

	decisions := make([]*RoutingDecision, limit)
	copy(decisions, gr.decisions[len(gr.decisions)-limit:])

	return decisions
}

// GetRoutingStats returns routing statistics
func (gr *GeoRouter) GetRoutingStats() map[string]interface{} {
	gr.mu.RLock()
	defer gr.mu.RUnlock()

	totalRegions := len(gr.regions)
	activeRegions := 0
	healthyRegions := 0
	totalRequests := int64(0)
	avgLatency := time.Duration(0)

	for regionID, region := range gr.regions {
		if region.GetStatus() == RegionActive {
			activeRegions++
		}
		if region.HealthScore >= gr.config.MinHealthScore {
			healthyRegions++
		}
		if metrics, exists := gr.metrics[regionID]; exists {
			totalRequests += metrics.RequestCount
			avgLatency += metrics.Latency
		}
	}

	if totalRegions > 0 {
		avgLatency = avgLatency / time.Duration(totalRegions)
	}

	return map[string]interface{}{
		"total_regions":      totalRegions,
		"active_regions":     activeRegions,
		"healthy_regions":    healthyRegions,
		"total_requests":     totalRequests,
		"average_latency":    avgLatency.String(),
		"total_decisions":    len(gr.decisions),
		"total_policies":     len(gr.policies),
		"total_rules":        len(gr.rules),
		"sticky_sessions":    len(gr.sessions),
		"default_strategy":   gr.config.DefaultStrategy,
	}
}

// Start starts the geo-router
func (gr *GeoRouter) Start() error {
	gr.wg.Add(1)
	go gr.monitorRegions()

	return nil
}

// Stop stops the geo-router
func (gr *GeoRouter) Stop() error {
	gr.cancel()
	gr.wg.Wait()
	close(gr.decisionChan)
	return nil
}

// monitorRegions monitors region health and metrics
func (gr *GeoRouter) monitorRegions() {
	defer gr.wg.Done()

	ticker := time.NewTicker(gr.config.UpdateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-gr.ctx.Done():
			return
		case <-ticker.C:
			gr.updateRegionMetrics()
		}
	}
}

// updateRegionMetrics updates metrics for all regions
func (gr *GeoRouter) updateRegionMetrics() {
	gr.mu.Lock()
	defer gr.mu.Unlock()

	for regionID, region := range gr.regions {
		if metrics, exists := gr.metrics[regionID]; exists {
			metrics.Load = region.CurrentLoad
			metrics.HealthScore = region.HealthScore
			metrics.LastUpdate = time.Now()
		}
	}
}

// ClearStickySessions clears all sticky sessions
func (gr *GeoRouter) ClearStickySessions() {
	gr.mu.Lock()
	defer gr.mu.Unlock()
	gr.sessions = make(map[string]string)
}
