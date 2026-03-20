package inference

import (
	"context"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"
)

// ResourceScheduler manages resource allocation and scheduling for model instances
type ResourceScheduler struct {
	gpuManager *GPUManager
	memManager *MemoryManager

	// Resource tracking
	allocations   map[string]*ResourceAllocation
	instanceLoads map[string]int // instance_id -> request count

	// Priority queues
	priorityQueues map[RequestPriority][]*InferenceRequest

	// Statistics
	stats *SchedulerStats

	mu sync.RWMutex
}

// NewResourceScheduler creates a new resource scheduler
func NewResourceScheduler(gpuManager *GPUManager, memManager *MemoryManager) *ResourceScheduler {
	scheduler := &ResourceScheduler{
		gpuManager:     gpuManager,
		memManager:     memManager,
		allocations:    make(map[string]*ResourceAllocation),
		instanceLoads:  make(map[string]int),
		priorityQueues: make(map[RequestPriority][]*InferenceRequest),
		stats: &SchedulerStats{
			UpdatedAt: time.Now(),
		},
	}

	// Initialize priority queues
	for priority := PriorityLow; priority <= PriorityCritical; priority++ {
		scheduler.priorityQueues[priority] = make([]*InferenceRequest, 0)
	}

	// Start background routines
	go scheduler.monitorResources()
	go scheduler.processPriorityQueues()

	return scheduler
}

// AllocateResources allocates resources for a model instance
func (s *ResourceScheduler) AllocateResources(ctx context.Context, config *InstanceConfig) (*ResourceAllocation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Estimate resource requirements
	requiredMemory := s.estimateMemoryRequirement(config)
	requiredGPUMemory := int64(0)
	if config.Device == DeviceGPU {
		requiredGPUMemory = s.estimateGPUMemoryRequirement(config)
	}

	// Check available resources
	availableMem := s.memManager.GetAvailableMemory()
	if availableMem < requiredMemory {
		return nil, ErrInsufficientMemoryError(requiredMemory, availableMem)
	}

	// Allocate GPU resources if needed
	var gpuDeviceID int
	var err error
	if config.Device == DeviceGPU {
		gpuDeviceID, err = s.allocateGPU(config.DeviceID, requiredGPUMemory)
		if err != nil {
			return nil, err
		}
	}

	// Allocate CPU resources
	cpuThreads := config.Threads
	if cpuThreads == 0 {
		cpuThreads = 4 // default
	}

	// Create allocation
	allocation := &ResourceAllocation{
		InstanceID:     generateAllocationID(),
		DeviceType:     config.Device,
		DeviceID:       gpuDeviceID,
		CPUThreads:     cpuThreads,
		MemoryBytes:    requiredMemory,
		GPUMemoryBytes: requiredGPUMemory,
		GPULayers:      config.GPULayers,
		AllocatedAt:    time.Now(),
	}

	// Reserve resources
	s.memManager.ReserveMemory(requiredMemory)
	if config.Device == DeviceGPU {
		s.gpuManager.ReserveGPUMemory(gpuDeviceID, requiredGPUMemory)
	}

	// Track allocation
	s.allocations[allocation.InstanceID] = allocation
	s.instanceLoads[allocation.InstanceID] = 0

	// Update stats
	s.stats.ActiveInstances++
	s.stats.UpdatedAt = time.Now()

	log.Printf("Allocated resources for instance %s: CPU=%d threads, Memory=%d bytes, GPU=%d bytes",
		allocation.InstanceID, cpuThreads, requiredMemory, requiredGPUMemory)

	return allocation, nil
}

// ReleaseResources releases allocated resources
func (s *ResourceScheduler) ReleaseResources(allocation *ResourceAllocation) error {
	if allocation == nil {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Release memory
	s.memManager.ReleaseMemory(allocation.MemoryBytes)

	// Release GPU memory if allocated
	if allocation.DeviceType == DeviceGPU {
		s.gpuManager.ReleaseGPUMemory(allocation.DeviceID, allocation.GPUMemoryBytes)
	}

	// Remove tracking
	delete(s.allocations, allocation.InstanceID)
	delete(s.instanceLoads, allocation.InstanceID)

	// Update stats
	s.stats.ActiveInstances--
	s.stats.UpdatedAt = time.Now()

	log.Printf("Released resources for instance %s", allocation.InstanceID)

	return nil
}

// SelectInstance selects the best instance for a request based on load balancing
func (s *ResourceScheduler) SelectInstance(modelID string, instances []*ModelInstance) (*ModelInstance, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(instances) == 0 {
		return nil, ErrNoAvailableInstanceError(modelID)
	}

	// Filter ready instances
	var readyInstances []*ModelInstance
	for _, instance := range instances {
		if instance.IsReady() {
			readyInstances = append(readyInstances, instance)
		}
	}

	if len(readyInstances) == 0 {
		return nil, ErrNoAvailableInstanceError(modelID)
	}

	// Sort instances by load (least loaded first)
	sort.Slice(readyInstances, func(i, j int) bool {
		loadI := s.instanceLoads[readyInstances[i].ID]
		loadJ := s.instanceLoads[readyInstances[j].ID]
		return loadI < loadJ
	})

	// Select least loaded instance
	selectedInstance := readyInstances[0]

	// Increment load counter
	s.instanceLoads[selectedInstance.ID]++

	return selectedInstance, nil
}

// QueueRequest queues a request with priority
func (s *ResourceScheduler) QueueRequest(req *InferenceRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Validate priority
	if req.Priority < PriorityLow || req.Priority > PriorityCritical {
		return NewError(ErrPriorityInvalid, "Invalid priority value").
			WithMetadata("priority", req.Priority)
	}

	// Add to priority queue
	s.priorityQueues[req.Priority] = append(s.priorityQueues[req.Priority], req)

	// Update stats
	s.stats.TotalRequests++
	s.stats.QueuedRequests++
	s.stats.UpdatedAt = time.Now()

	return nil
}

// GetNextRequest gets the next highest priority request
func (s *ResourceScheduler) GetNextRequest() *InferenceRequest {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check queues from highest to lowest priority
	for priority := PriorityCritical; priority >= PriorityLow; priority-- {
		queue := s.priorityQueues[priority]
		if len(queue) > 0 {
			// Dequeue the first request
			req := queue[0]
			s.priorityQueues[priority] = queue[1:]

			// Update stats
			s.stats.QueuedRequests--
			s.stats.ProcessingRequests++
			s.stats.UpdatedAt = time.Now()

			return req
		}
	}

	return nil
}

// CompleteRequest marks a request as completed
func (s *ResourceScheduler) CompleteRequest(req *InferenceRequest, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Decrement instance load
	if req.ModelID != "" {
		// Find instance and decrement load (this is a simplified version)
		for instanceID, load := range s.instanceLoads {
			if load > 0 {
				s.instanceLoads[instanceID] = load - 1
				break
			}
		}
	}

	// Update stats
	s.stats.ProcessingRequests--
	s.stats.CompletedRequests++
	if err != nil {
		s.stats.FailedRequests++
	}
	s.stats.UpdatedAt = time.Now()
}

// GetStats returns current scheduler statistics
func (s *ResourceScheduler) GetStats() *SchedulerStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Create a copy
	stats := *s.stats

	// Calculate averages and derived metrics
	if stats.TotalRequests > 0 {
		totalTime := stats.AvgQueueTime * time.Duration(stats.TotalRequests)
		stats.AvgQueueTime = totalTime / time.Duration(stats.TotalRequests)
	}

	stats.IdleInstances = stats.ActiveInstances - s.countBusyInstances()
	stats.LoadBalance = s.calculateLoadBalance()

	return &stats
}

// GetResourceUsage returns current resource usage
func (s *ResourceScheduler) GetResourceUsage() *ResourceUsage {
	s.mu.RLock()
	defer s.mu.RUnlock()

	usage := &ResourceUsage{
		CPUUsage:        s.memManager.GetCPUUsage(),
		MemoryUsed:      s.memManager.GetUsedMemory(),
		MemoryTotal:     s.memManager.GetTotalMemory(),
		GPUUsage:        s.gpuManager.GetAverageGPUUsage(),
		GPUMemoryUsed:   s.gpuManager.GetTotalGPUMemoryUsed(),
		GPUMemoryTotal:  s.gpuManager.GetTotalGPUMemory(),
		ActiveInstances: len(s.allocations),
		QueuedRequests:  s.getTotalQueuedRequests(),
		UpdatedAt:       time.Now(),
	}

	return usage
}

// SetResourceQuota sets resource quotas for an instance
func (s *ResourceScheduler) SetResourceQuota(instanceID string, quota *ResourceQuota) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	allocation, exists := s.allocations[instanceID]
	if !exists {
		return ErrInstanceNotFoundError(instanceID)
	}

	// Apply quota limits
	if quota.MaxMemoryBytes > 0 {
		allocation.MemoryBytes = min(allocation.MemoryBytes, quota.MaxMemoryBytes)
	}

	if quota.MaxGPUMemoryBytes > 0 && allocation.DeviceType == DeviceGPU {
		allocation.GPUMemoryBytes = min(allocation.GPUMemoryBytes, quota.MaxGPUMemoryBytes)
	}

	return nil
}

// monitorResources periodically monitors resource usage
func (s *ResourceScheduler) monitorResources() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		s.checkResourceUsage()
	}
}

// checkResourceUsage checks current resource usage and takes action if needed
func (s *ResourceScheduler) checkResourceUsage() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check memory usage
	memUsage := s.memManager.GetMemoryUsagePercent()
	if memUsage > 90 {
		log.Printf("Warning: Memory usage is high: %.2f%%", memUsage)
	}

	// Check GPU usage
	for _, gpu := range s.gpuManager.GetGPUInfo() {
		if gpu.Utilization > 90 {
			log.Printf("Warning: GPU %d utilization is high: %d%%", gpu.ID, gpu.Utilization)
		}
	}

	// Check queue sizes
	totalQueued := s.getTotalQueuedRequestsLocked()
	if totalQueued > 100 {
		log.Printf("Warning: High number of queued requests: %d", totalQueued)
	}
}

// processPriorityQueues processes requests from priority queues
func (s *ResourceScheduler) processPriorityQueues() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		// This would typically dispatch requests to available instances
		// For now, it's a placeholder for the priority queue processing logic
		req := s.GetNextRequest()
		if req != nil {
			// Would dispatch to instance here
			_ = req
		}
	}
}

// Helper methods

// estimateMemoryRequirement estimates the memory needed for a model instance
func (s *ResourceScheduler) estimateMemoryRequirement(config *InstanceConfig) int64 {
	// Base memory requirement (simplified estimation)
	baseMemory := int64(1024 * 1024 * 1024) // 1 GB base

	// Add context length factor
	contextMemory := int64(config.ContextLength) * 1024 // ~1KB per context token

	// Add batch size factor
	batchMemory := int64(config.BatchSize) * 512 * 1024 // ~512KB per batch item

	return baseMemory + contextMemory + batchMemory
}

// estimateGPUMemoryRequirement estimates GPU memory needed
func (s *ResourceScheduler) estimateGPUMemoryRequirement(config *InstanceConfig) int64 {
	// Estimate based on GPU layers and context length
	layerMemory := int64(config.GPULayers) * 100 * 1024 * 1024 // ~100MB per layer
	contextMemory := int64(config.ContextLength) * 2 * 1024 // ~2KB per context token

	return layerMemory + contextMemory
}

// allocateGPU allocates a GPU device
func (s *ResourceScheduler) allocateGPU(preferredDevice int, requiredMemory int64) (int, error) {
	gpus := s.gpuManager.GetGPUInfo()
	if len(gpus) == 0 {
		return 0, ErrNoAvailableGPUError()
	}

	// Try preferred device first
	if preferredDevice >= 0 && preferredDevice < len(gpus) {
		gpu := gpus[preferredDevice]
		if gpu.MemoryFree >= requiredMemory {
			return preferredDevice, nil
		}
	}

	// Find best available GPU
	for _, gpu := range gpus {
		if gpu.MemoryFree >= requiredMemory {
			return gpu.ID, nil
		}
	}

	return 0, ErrInsufficientGPUMemoryError(0, requiredMemory, 0)
}

// countBusyInstances counts the number of busy instances
func (s *ResourceScheduler) countBusyInstances() int {
	busy := 0
	for _, load := range s.instanceLoads {
		if load > 0 {
			busy++
		}
	}
	return busy
}

// calculateLoadBalance calculates how balanced the load is (0-1, where 1 is perfectly balanced)
func (s *ResourceScheduler) calculateLoadBalance() float64 {
	if len(s.instanceLoads) == 0 {
		return 1.0
	}

	// Calculate standard deviation of loads
	var sum int
	for _, load := range s.instanceLoads {
		sum += load
	}

	mean := float64(sum) / float64(len(s.instanceLoads))
	if mean == 0 {
		return 1.0
	}

	var variance float64
	for _, load := range s.instanceLoads {
		diff := float64(load) - mean
		variance += diff * diff
	}
	variance /= float64(len(s.instanceLoads))

	stdDev := sqrt(variance)
	coefficientOfVariation := stdDev / mean

	// Convert to balance score (lower CV = better balance)
	balance := 1.0 - min(coefficientOfVariation, 1.0)

	return balance
}

// getTotalQueuedRequests gets total queued requests across all priorities
func (s *ResourceScheduler) getTotalQueuedRequests() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.getTotalQueuedRequestsLocked()
}

// getTotalQueuedRequestsLocked gets total queued requests (must hold lock)
func (s *ResourceScheduler) getTotalQueuedRequestsLocked() int {
	total := 0
	for _, queue := range s.priorityQueues {
		total += len(queue)
	}
	return total
}

// ResourceQuota represents resource quotas for an instance
type ResourceQuota struct {
	MaxMemoryBytes    int64 `json:"max_memory_bytes"`
	MaxGPUMemoryBytes int64 `json:"max_gpu_memory_bytes"`
	MaxCPUThreads     int   `json:"max_cpu_threads"`
	MaxRequests       int   `json:"max_requests"`
}

// generateAllocationID generates a unique allocation ID
func generateAllocationID() string {
	return fmt.Sprintf("alloc_%d", time.Now().UnixNano())
}

// Math helper functions

func min(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func sqrt(x float64) float64 {
	// Newton's method for square root
	if x == 0 {
		return 0
	}
