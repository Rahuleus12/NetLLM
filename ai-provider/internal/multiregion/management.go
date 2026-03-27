package multiregion

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// RegionType defines the type of region
type RegionType string

const (
	RegionTypePrimary   RegionType = "primary"
	RegionTypeSecondary RegionType = "secondary"
	RegionTypeEdge      RegionType = "edge"
	RegionTypeDR        RegionType = "disaster_recovery"
)

// RegionLifecycleState represents the lifecycle state of a region
type RegionLifecycleState string

const (
	LifecycleCreating    RegionLifecycleState = "creating"
	LifecycleActive      RegionLifecycleState = "active"
	LifecycleUpdating    RegionLifecycleState = "updating"
	LifecycleDeactivating RegionLifecycleState = "deactivating"
	LifecycleInactive    RegionLifecycleState = "inactive"
	LifecycleDeleting    RegionLifecycleState = "deleting"
	LifecycleError       RegionLifecycleState = "error"
)

// RegionConfig represents region configuration
type RegionConfig struct {
	ID               string                 `json:"id"`
	Name             string                 `json:"name"`
	Code             string                 `json:"code"`
	Type             RegionType             `json:"type"`
	Endpoint         string                 `json:"endpoint"`
	APIKey           string                 `json:"api_key"`
	Priority         int                    `json:"priority"`
	Capacity         int                    `json:"capacity"`
	Enabled          bool                   `json:"enabled"`
	AutoScale        bool                   `json:"auto_scale"`
	MinCapacity      int                    `json:"min_capacity"`
	MaxCapacity      int                    `json:"max_capacity"`
	ScaleThreshold   int                    `json:"scale_threshold"`
	Zones            []string               `json:"zones"`
	Metadata         map[string]interface{} `json:"metadata"`
	Tags             map[string]string      `json:"tags"`
	CreatedAt        time.Time              `json:"created_at"`
	UpdatedAt        time.Time              `json:"updated_at"`
}

// CapacityPlan represents a capacity plan for a region
type CapacityPlan struct {
	RegionID          string    `json:"region_id"`
	CurrentCapacity   int       `json:"current_capacity"`
	TargetCapacity    int       `json:"target_capacity"`
	ProjectedLoad     int       `json:"projected_load"`
	UtilizationRate   float64   `json:"utilization_rate"`
	ScaleUpThreshold  float64   `json:"scale_up_threshold"`
	ScaleDownThreshold float64  `json:"scale_down_threshold"`
	LastUpdated       time.Time `json:"last_updated"`
	Recommendations   []string  `json:"recommendations"`
}

// RegionEvent represents a region lifecycle event
type RegionEvent struct {
	ID        string               `json:"id"`
	RegionID  string               `json:"region_id"`
	Type      string               `json:"type"`
	State     RegionLifecycleState `json:"state"`
	Message   string               `json:"message"`
	Timestamp time.Time            `json:"timestamp"`
	UserID    string               `json:"user_id"`
	Metadata  map[string]interface{} `json:"metadata"`
}

// RegionManager manages region lifecycle and configuration
type RegionManager struct {
	regions      map[string]*RegionConfig
	plans        map[string]*CapacityPlan
	events       []*RegionEvent
	mu           sync.RWMutex
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	eventChan    chan *RegionEvent
}

// NewRegionManager creates a new region manager
func NewRegionManager() *RegionManager {
	ctx, cancel := context.WithCancel(context.Background())

	return &RegionManager{
		regions:    make(map[string]*RegionConfig),
		plans:      make(map[string]*CapacityPlan),
		events:     make([]*RegionEvent, 0),
		ctx:        ctx,
		cancel:     cancel,
		eventChan:  make(chan *RegionEvent, 100),
	}
}

// CreateRegion creates a new region
func (rm *RegionManager) CreateRegion(config *RegionConfig) error {
	if config == nil {
		return errors.New("region config cannot be nil")
	}

	if config.ID == "" {
		return errors.New("region ID is required")
	}

	rm.mu.Lock()
	defer rm.mu.Unlock()

	if _, exists := rm.regions[config.ID]; exists {
		return fmt.Errorf("region %s already exists", config.ID)
	}

	// Set timestamps
	config.CreatedAt = time.Now()
	config.UpdatedAt = time.Now()

	// Initialize defaults
	if config.Capacity == 0 {
		config.Capacity = 100
	}
	if config.MinCapacity == 0 {
		config.MinCapacity = 10
	}
	if config.MaxCapacity == 0 {
		config.MaxCapacity = 1000
	}
	if config.Metadata == nil {
		config.Metadata = make(map[string]interface{})
	}
	if config.Tags == nil {
		config.Tags = make(map[string]string)
	}

	rm.regions[config.ID] = config

	// Create capacity plan
	rm.plans[config.ID] = &CapacityPlan{
		RegionID:          config.ID,
		CurrentCapacity:   config.Capacity,
		TargetCapacity:    config.Capacity,
		ScaleUpThreshold:  80.0,
		ScaleDownThreshold: 30.0,
		LastUpdated:       time.Now(),
		Recommendations:   make([]string, 0),
	}

	// Emit event
	rm.emitEvent(config.ID, "region_created", LifecycleCreating, "Region created successfully", "system", nil)

	return nil
}

// UpdateRegion updates an existing region
func (rm *RegionManager) UpdateRegion(config *RegionConfig) error {
	if config == nil {
		return errors.New("region config cannot be nil")
	}

	rm.mu.Lock()
	defer rm.mu.Unlock()

	existing, exists := rm.regions[config.ID]
	if !exists {
		return fmt.Errorf("region %s not found", config.ID)
	}

	// Preserve creation timestamp
	config.CreatedAt = existing.CreatedAt
	config.UpdatedAt = time.Now()

	rm.regions[config.ID] = config

	// Update capacity plan
	if plan, exists := rm.plans[config.ID]; exists {
		plan.CurrentCapacity = config.Capacity
		plan.TargetCapacity = config.Capacity
		plan.LastUpdated = time.Now()
	}

	// Emit event
	rm.emitEvent(config.ID, "region_updated", LifecycleUpdating, "Region updated successfully", "system", nil)

	return nil
}

// DeleteRegion deletes a region
func (rm *RegionManager) DeleteRegion(regionID string) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if _, exists := rm.regions[regionID]; !exists {
		return fmt.Errorf("region %s not found", regionID)
	}

	// Emit event before deletion
	rm.emitEvent(regionID, "region_deleted", LifecycleDeleting, "Region deleted successfully", "system", nil)

	delete(rm.regions, regionID)
	delete(rm.plans, regionID)

	return nil
}

// GetRegion retrieves a region by ID
func (rm *RegionManager) GetRegion(regionID string) (*RegionConfig, error) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	config, exists := rm.regions[regionID]
	if !exists {
		return nil, fmt.Errorf("region %s not found", regionID)
	}

	return config, nil
}

// GetAllRegions returns all regions
func (rm *RegionManager) GetAllRegions() []*RegionConfig {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	regions := make([]*RegionConfig, 0, len(rm.regions))
	for _, config := range rm.regions {
		regions = append(regions, config)
	}

	return regions
}

// GetRegionsByType returns regions of a specific type
func (rm *RegionManager) GetRegionsByType(regionType RegionType) []*RegionConfig {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	regions := make([]*RegionConfig, 0)
	for _, config := range rm.regions {
		if config.Type == regionType {
			regions = append(regions, config)
		}
	}

	return regions
}

// ActivateRegion activates a region
func (rm *RegionManager) ActivateRegion(regionID string) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	config, exists := rm.regions[regionID]
	if !exists {
		return fmt.Errorf("region %s not found", regionID)
	}

	config.Enabled = true
	config.UpdatedAt = time.Now()

	rm.emitEvent(regionID, "region_activated", LifecycleActive, "Region activated successfully", "system", nil)

	return nil
}

// DeactivateRegion deactivates a region
func (rm *RegionManager) DeactivateRegion(regionID string) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	config, exists := rm.regions[regionID]
	if !exists {
		return fmt.Errorf("region %s not found", regionID)
	}

	config.Enabled = false
	config.UpdatedAt = time.Now()

	rm.emitEvent(regionID, "region_deactivated", LifecycleInactive, "Region deactivated successfully", "system", nil)

	return nil
}

// UpdateCapacity updates region capacity
func (rm *RegionManager) UpdateCapacity(regionID string, capacity int) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	config, exists := rm.regions[regionID]
	if !exists {
		return fmt.Errorf("region %s not found", regionID)
	}

	if capacity < config.MinCapacity || capacity > config.MaxCapacity {
		return fmt.Errorf("capacity %d is out of bounds (min: %d, max: %d)",
			capacity, config.MinCapacity, config.MaxCapacity)
	}

	config.Capacity = capacity
	config.UpdatedAt = time.Now()

	// Update capacity plan
	if plan, exists := rm.plans[regionID]; exists {
		plan.TargetCapacity = capacity
		plan.LastUpdated = time.Now()
	}

	rm.emitEvent(regionID, "capacity_updated", LifecycleActive,
		fmt.Sprintf("Capacity updated to %d", capacity), "system", nil)

	return nil
}

// GetCapacityPlan returns capacity plan for a region
func (rm *RegionManager) GetCapacityPlan(regionID string) (*CapacityPlan, error) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	plan, exists := rm.plans[regionID]
	if !exists {
		return nil, fmt.Errorf("capacity plan for region %s not found", regionID)
	}

	return plan, nil
}

// UpdateCapacityPlan updates capacity plan
func (rm *RegionManager) UpdateCapacityPlan(plan *CapacityPlan) error {
	if plan == nil {
		return errors.New("capacity plan cannot be nil")
	}

	rm.mu.Lock()
	defer rm.mu.Unlock()

	if _, exists := rm.regions[plan.RegionID]; !exists {
		return fmt.Errorf("region %s not found", plan.RegionID)
	}

	plan.LastUpdated = time.Now()
	rm.plans[plan.RegionID] = plan

	return nil
}

// ScaleRegion scales region capacity automatically
func (rm *RegionManager) ScaleRegion(regionID string, currentLoad int) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	config, exists := rm.regions[regionID]
	if !exists {
		return fmt.Errorf("region %s not found", regionID)
	}

	if !config.AutoScale {
		return errors.New("auto-scaling is disabled for this region")
	}

	plan, exists := rm.plans[regionID]
	if !exists {
		return fmt.Errorf("capacity plan for region %s not found", regionID)
	}

	// Calculate utilization
	utilization := float64(currentLoad) / float64(config.Capacity) * 100
	plan.UtilizationRate = utilization
	plan.ProjectedLoad = currentLoad

	var newCapacity int
	var action string

	// Scale up
	if utilization >= plan.ScaleUpThreshold {
		newCapacity = int(float64(config.Capacity) * 1.5)
		if newCapacity > config.MaxCapacity {
			newCapacity = config.MaxCapacity
		}
		action = "scale_up"
		plan.Recommendations = append(plan.Recommendations,
			fmt.Sprintf("Scale up recommended: utilization at %.2f%%", utilization))
	}

	// Scale down
	if utilization <= plan.ScaleDownThreshold {
		newCapacity = int(float64(config.Capacity) * 0.8)
		if newCapacity < config.MinCapacity {
			newCapacity = config.MinCapacity
		}
		action = "scale_down"
		plan.Recommendations = append(plan.Recommendations,
			fmt.Sprintf("Scale down recommended: utilization at %.2f%%", utilization))
	}

	// Apply scaling
	if newCapacity > 0 && newCapacity != config.Capacity {
		config.Capacity = newCapacity
		config.UpdatedAt = time.Now()
		plan.TargetCapacity = newCapacity
		plan.LastUpdated = time.Now()

		rm.emitEvent(regionID, action, LifecycleUpdating,
			fmt.Sprintf("Region scaled to capacity %d", newCapacity), "auto-scaler", nil)
	}

	return nil
}

// GetRegionEvents returns events for a region
func (rm *RegionManager) GetRegionEvents(regionID string, limit int) []*RegionEvent {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	events := make([]*RegionEvent, 0)
	count := 0

	for i := len(rm.events) - 1; i >= 0 && count < limit; i-- {
		if rm.events[i].RegionID == regionID {
			events = append(events, rm.events[i])
			count++
		}
	}

	return events
}

// GetAllEvents returns all events
func (rm *RegionManager) GetAllEvents(limit int) []*RegionEvent {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	if limit <= 0 || limit > len(rm.events) {
		limit = len(rm.events)
	}

	events := make([]*RegionEvent, limit)
	start := len(rm.events) - limit
	if start < 0 {
		start = 0
	}
	copy(events, rm.events[start:])

	return events
}

// emitEvent emits a region event
func (rm *RegionManager) emitEvent(regionID, eventType string, state RegionLifecycleState, message, userID string, metadata map[string]interface{}) {
	event := &RegionEvent{
		ID:        fmt.Sprintf("event-%d", time.Now().UnixNano()),
		RegionID:  regionID,
		Type:      eventType,
		State:     state,
		Message:   message,
		Timestamp: time.Now(),
		UserID:    userID,
		Metadata:  metadata,
	}

	rm.events = append(rm.events, event)

	// Keep only last 1000 events
	if len(rm.events) > 1000 {
		rm.events = rm.events[len(rm.events)-1000:]
	}

	select {
	case rm.eventChan <- event:
	default:
		// Channel full, drop event
	}
}

// GetStats returns region management statistics
func (rm *RegionManager) GetStats() map[string]interface{} {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	totalRegions := len(rm.regions)
	activeRegions := 0
	enabledRegions := 0
	autoScalingRegions := 0
	totalCapacity := 0

	for _, config := range rm.regions {
		if config.Enabled {
			enabledRegions++
		}
		if config.AutoScale {
			autoScalingRegions++
		}
		totalCapacity += config.Capacity
	}

	return map[string]interface{}{
		"total_regions":          totalRegions,
		"active_regions":         activeRegions,
		"enabled_regions":        enabledRegions,
		"auto_scaling_regions":   autoScalingRegions,
		"total_capacity":         totalCapacity,
		"total_plans":            len(rm.plans),
		"total_events":           len(rm.events),
	}
}

// Start starts the region manager
func (rm *RegionManager) Start() error {
	rm.wg.Add(1)
	go rm.processEvents()

	return nil
}

// Stop stops the region manager
func (rm *RegionManager) Stop() error {
	rm.cancel()
	rm.wg.Wait()
	close(rm.eventChan)
	return nil
}

// processEvents processes region events
func (rm *RegionManager) processEvents() {
	defer rm.wg.Done()

	for {
		select {
		case <-rm.ctx.Done():
			return
		case event := <-rm.eventChan:
			if event == nil {
				return
			}
			// Process event (could send to monitoring, logging, etc.)
			rm.handleEvent(event)
		}
	}
}

// handleEvent handles a region event
func (rm *RegionManager) handleEvent(event *RegionEvent) {
	// Could be extended to send notifications, update monitoring, etc.
}
