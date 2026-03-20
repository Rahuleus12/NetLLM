package inference

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// GPUManager manages GPU devices and resources
type GPUManager struct {
	devices    map[int]*GPUInfo
	allocations map[int][]*ResourceAllocation
	mu         sync.RWMutex
	config     *GPUManagerConfig
	monitor    *GPUMonitor
}

// GPUManagerConfig represents configuration for the GPU manager
type GPUManagerConfig struct {
	EnableMonitoring    bool          `json:"enable_monitoring"`
	MonitorInterval     time.Duration `json:"monitor_interval"`
	MemoryBuffer        int64         `json:"memory_buffer"` // bytes to reserve
	MaxUtilization      float64       `json:"max_utilization"` // max GPU utilization (0-1)
	EnableGPULayers     bool          `json:"enable_gpu_layers"`
	DefaultGPULayers    int           `json:"default_gpu_layers"`
}

// NewGPUManager creates a new GPU manager instance
func NewGPUManager() *GPUManager {
	config := &GPUManagerConfig{
		EnableMonitoring: true,
		MonitorInterval:  5 * time.Second,
		MemoryBuffer:     500 * 1024 * 1024, // 500MB buffer
		MaxUtilization:   0.9,               // 90% max utilization
		EnableGPULayers:  true,
		DefaultGPULayers: -1, // -1 means all layers
	}

	manager := &GPUManager{
		devices:     make(map[int]*GPUInfo),
		allocations: make(map[int][]*ResourceAllocation),
		config:      config,
	}

	// Initialize GPU detection
	if err := manager.detectGPUs(); err != nil {
		log.Printf("Warning: GPU detection failed: %v", err)
	}

	// Start monitoring if enabled
	if config.EnableMonitoring && len(manager.devices) > 0 {
		manager.monitor = NewGPUMonitor(manager, config.MonitorInterval)
		manager.monitor.Start()
	}

	return manager
}

// NewGPUManagerWithConfig creates a new GPU manager with custom configuration
func NewGPUManagerWithConfig(config *GPUManagerConfig) *GPUManager {
	if config == nil {
		config = &GPUManagerConfig{
			EnableMonitoring: true,
			MonitorInterval:  5 * time.Second,
			MemoryBuffer:     500 * 1024 * 1024,
			MaxUtilization:   0.9,
			EnableGPULayers:  true,
			DefaultGPULayers: -1,
		}
	}

	manager := &GPUManager{
		devices:     make(map[int]*GPUInfo),
		allocations: make(map[int][]*ResourceAllocation),
		config:      config,
	}

	// Initialize GPU detection
	if err := manager.detectGPUs(); err != nil {
		log.Printf("Warning: GPU detection failed: %v", err)
	}

	// Start monitoring if enabled
	if config.EnableMonitoring && len(manager.devices) > 0 {
		manager.monitor = NewGPUMonitor(manager, config.MonitorInterval)
		manager.monitor.Start()
	}

	return manager
}

// detectGPUs detects available GPU devices
func (g *GPUManager) detectGPUs() error {
	g.mu.Lock()
	defer g.mu.Unlock()

	// In a real implementation, this would use CUDA/ROCm APIs
	// or call nvidia-smi/amd-smi to detect GPUs
	// For now, we'll implement a placeholder that can be extended

	// Try to detect NVIDIA GPUs
	nvidiaGPUs, err := g.detectNVIDIAGPUs()
	if err == nil && len(nvidiaGPUs) > 0 {
		for _, gpu := range nvidiaGPUs {
			g.devices[gpu.ID] = gpu
		}
		log.Printf("Detected %d NVIDIA GPU(s)", len(nvidiaGPUs))
	}

	// Try to detect AMD GPUs (if needed)
	// amdGPUs, err := g.detectAMDGPUs()
	// if err == nil && len(amdGPUs) > 0 {
	//     for _, gpu := range amdGPUs {
	//         g.devices[gpu.ID] = gpu
	//     }
	//     log.Printf("Detected %d AMD GPU(s)", len(amdGPUs))
	// }

	if len(g.devices) == 0 {
		log.Println("No GPU devices detected")
	}

	return nil
}

// detectNVIDIAGPUs detects NVIDIA GPU devices
func (g *GPUManager) detectNVIDIAGPUs() ([]*GPUInfo, error) {
	// Placeholder implementation
	// In production, this would use NVML (NVIDIA Management Library)
	// or execute nvidia-smi commands to get GPU information

	var gpus []*GPUInfo

	// Example: Query nvidia-smi for GPU information
	// This is a simplified placeholder - real implementation would parse nvidia-smi output
	// or use go-nvml package for direct NVML bindings

	// For development/testing, we can simulate GPU detection
	// In production, replace with actual GPU detection logic

	return gpus, nil
}

// GetDevice retrieves information about a specific GPU device
func (g *GPUManager) GetDevice(deviceID int) (*GPUInfo, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	gpu, exists := g.devices[deviceID]
	if !exists {
		return nil, fmt.Errorf("GPU device %d not found", deviceID)
	}

	return gpu, nil
}

// ListDevices lists all available GPU devices
func (g *GPUManager) ListDevices() []*GPUInfo {
	g.mu.RLock()
	defer g.mu.RUnlock()

	devices := make([]*GPUInfo, 0, len(g.devices))
	for _, gpu := range g.devices {
		devices = append(devices, gpu)
	}

	return devices
}

// GetAvailableDevices returns GPU devices with sufficient available memory
func (g *GPUManager) GetAvailableDevices(requiredMemory int64) []*GPUInfo {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var available []*GPUInfo
	for _, gpu := range g.devices {
		availableMemory := g.getAvailableMemory(gpu)
		if availableMemory >= requiredMemory {
			available = append(available, gpu)
		}
	}

	return available
}

// AllocateGPU allocates GPU resources for an instance
func (g *GPUManager) AllocateGPU(deviceID int, allocation *ResourceAllocation) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	gpu, exists := g.devices[deviceID]
	if !exists {
		return ErrNoAvailableGPUError()
	}

	// Check available memory
	availableMemory := g.getAvailableMemory(gpu)
	if availableMemory < allocation.GPUMemoryBytes {
		return ErrInsufficientGPUMemoryError(deviceID, allocation.GPUMemoryBytes, availableMemory)
	}

	// Check utilization
	if gpu.Utilization > int(g.config.MaxUtilization*100) {
		return NewError(ErrGPUAllocationFailed, "GPU utilization too high").
			WithMetadata("device_id", deviceID).
			WithMetadata("utilization", gpu.Utilization).
			WithRetryable(true)
	}

	// Add allocation
	allocation.DeviceID = deviceID
	allocation.DeviceType = DeviceGPU
	allocation.AllocatedAt = time.Now()

	g.allocations[deviceID] = append(g.allocations[deviceID], allocation)

	// Update GPU memory usage
	gpu.MemoryUsed += allocation.GPUMemoryBytes

	log.Printf("Allocated GPU %d: %d bytes for instance %s",
		deviceID, allocation.GPUMemoryBytes, allocation.InstanceID)

	return nil
}

// ReleaseGPU releases GPU resources
func (g *GPUManager) ReleaseGPU(allocation *ResourceAllocation) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if allocation.DeviceType != DeviceGPU {
		return nil // Not a GPU allocation
	}

	deviceID := allocation.DeviceID
	allocations, exists := g.allocations[deviceID]
	if !exists {
		return nil // No allocations for this device
	}

	// Find and remove the allocation
	for i, alloc := range allocations {
		if alloc.InstanceID == allocation.InstanceID {
			// Remove allocation from list
			g.allocations[deviceID] = append(allocations[:i], allocations[i+1:]...)

			// Update GPU memory usage
			if gpu, exists := g.devices[deviceID]; exists {
				gpu.MemoryUsed -= allocation.GPUMemoryBytes
				if gpu.MemoryUsed < 0 {
					gpu.MemoryUsed = 0
				}
			}

			log.Printf("Released GPU %d: %d bytes for instance %s",
				deviceID, allocation.GPUMemoryBytes, allocation.InstanceID)

			return nil
		}
	}

	return nil
}

// getAvailableMemory calculates available memory for a GPU
func (g *GPUManager) getAvailableMemory(gpu *GPUInfo) int64 {
	// Calculate total allocated memory
	var allocatedMemory int64
	if allocations, exists := g.allocations[gpu.ID]; exists {
		for _, alloc := range allocations {
			allocatedMemory += alloc.GPUMemoryBytes
		}
	}

	// Available = Total - Used - Buffer
	available := gpu.MemoryTotal - allocatedMemory - g.config.MemoryBuffer
	if available < 0 {
		available = 0
	}

	return available
}

// GetGPUUsage returns current GPU usage statistics
func (g *GPUManager) GetGPUUsage(deviceID int) (*GPUUsageStats, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	gpu, exists := g.devices[deviceID]
	if !exists {
		return nil, fmt.Errorf("GPU device %d not found", deviceID)
	}

	// Calculate allocation statistics
	var allocatedMemory int64
	var instanceCount int
	if allocations, exists := g.allocations[deviceID]; exists {
		instanceCount = len(allocations)
		for _, alloc := range allocations {
			allocatedMemory += alloc.GPUMemoryBytes
		}
	}

	stats := &GPUUsageStats{
		DeviceID:         deviceID,
		DeviceName:       gpu.Name,
		MemoryTotal:      gpu.MemoryTotal,
		MemoryUsed:       gpu.MemoryUsed,
		MemoryAllocated:  allocatedMemory,
		MemoryAvailable:  g.getAvailableMemory(gpu),
		Utilization:      gpu.Utilization,
		Temperature:      gpu.Temperature,
		PowerUsage:       gpu.PowerUsage,
		PowerCap:         gpu.PowerCap,
		InstanceCount:    instanceCount,
		UpdatedAt:        time.Now(),
	}

	return stats, nil
}

// GetAllGPUUsage returns usage statistics for all GPUs
func (g *GPUManager) GetAllGPUUsage() map[int]*GPUUsageStats {
	g.mu.RLock()
	defer g.mu.RUnlock()

	stats := make(map[int]*GPUUsageStats)
	for deviceID := range g.devices {
		if usage, err := g.GetGPUUsage(deviceID); err == nil {
			stats[deviceID] = usage
		}
	}

	return stats
}

// SelectBestGPU selects the best GPU for a given memory requirement
func (g *GPUManager) SelectBestGPU(requiredMemory int64) (int, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var bestDeviceID int = -1
	var bestScore float64 = -1

	for _, gpu := range g.devices {
		availableMemory := g.getAvailableMemory(gpu)

		// Skip if insufficient memory
		if availableMemory < requiredMemory {
			continue
		}

		// Skip if utilization too high
		if gpu.Utilization > int(g.config.MaxUtilization*100) {
			continue
		}

		// Calculate score based on available memory and utilization
		// Higher score = better choice
		memoryScore := float64(availableMemory) / float64(gpu.MemoryTotal)
		utilizationScore := 1.0 - (float64(gpu.Utilization) / 100.0)
		score := (memoryScore * 0.6) + (utilizationScore * 0.4)

		if score > bestScore {
			bestScore = score
			bestDeviceID = gpu.ID
		}
	}

	if bestDeviceID == -1 {
		return -1, ErrNoAvailableGPUError()
	}

	return bestDeviceID, nil
}

// UpdateGPUInfo updates GPU information (called by monitor)
func (g *GPUManager) UpdateGPUInfo(deviceID int, info *GPUInfo) {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.devices[deviceID] = info
}

// HasGPUAvailable checks if any GPU is available
func (g *GPUManager) HasGPUAvailable() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()

	return len(g.devices) > 0
}

// ReserveGPUMemory reserves GPU memory for a specific device
func (g *GPUManager) ReserveGPUMemory(deviceID int, size int64) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	gpu, exists := g.devices[deviceID]
	if !exists {
		return fmt.Errorf("GPU device %d not found", deviceID)
	}

	availableMemory := g.getAvailableMemory(gpu)
	if size > availableMemory {
		return ErrInsufficientGPUMemoryError(deviceID, size, availableMemory)
	}

	// Memory is reserved by creating a temporary allocation
	// This will be tracked through the allocations map
	gpu.MemoryUsed += size

	log.Printf("Reserved %dMB on GPU %d", size/1024/1024, deviceID)

	return nil
}

// ReleaseGPUMemory releases reserved GPU memory for a specific device
func (g *GPUManager) ReleaseGPUMemory(deviceID int, size int64) {
	g.mu.Lock()
	defer g.mu.Unlock()

	gpu, exists := g.devices[deviceID]
	if !exists {
		return
	}

	gpu.MemoryUsed -= size
	if gpu.MemoryUsed < 0 {
		gpu.MemoryUsed = 0
	}

	log.Printf("Released %dMB on GPU %d", size/1024/1024, deviceID)
}

// GetAverageGPUUsage returns the average GPU utilization across all devices
func (g *GPUManager) GetAverageGPUUsage() float64 {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if len(g.devices) == 0 {
		return 0.0
	}

	var totalUtilization float64
	for _, gpu := range g.devices {
		totalUtilization += float64(gpu.Utilization)
	}

	return totalUtilization / float64(len(g.devices))
}

// GetTotalGPUMemoryUsed returns the total GPU memory used across all devices
func (g *GPUManager) GetTotalGPUMemoryUsed() int64 {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var totalUsed int64
	for _, gpu := range g.devices {
		totalUsed += gpu.MemoryUsed
	}

	return totalUsed
}

// GetTotalGPUMemory returns the total GPU memory available across all devices
func (g *GPUManager) GetTotalGPUMemory() int64 {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var totalMemory int64
	for _, gpu := range g.devices {
		totalMemory += gpu.MemoryTotal
	}

	return totalMemory
}

// GetGPUInfo returns information about all GPU devices
func (g *GPUManager) GetGPUInfo() []*GPUInfo {
	g.mu.RLock()
	defer g.mu.RUnlock()

	gpus := make([]*GPUInfo, 0, len(g.devices))
	for _, gpu := range g.devices {
		gpus = append(gpus, gpu)
	}

	return gpus
}

// GetGPUCount returns the number of available GPUs
func (g *GPUManager) GetGPUCount() int {
	g.mu.RLock()
	defer g.mu.RUnlock()

	return len(g.devices)
}

// Shutdown shuts down the GPU manager
func (g *GPUManager) Shutdown() error {
	if g.monitor != nil {
		g.monitor.Stop()
	}

	log.Println("GPU manager shutdown complete")
	return nil
}

// GPUUsageStats represents detailed GPU usage statistics
type GPUUsageStats struct {
	DeviceID        int       `json:"device_id"`
	DeviceName      string    `json:"device_name"`
	MemoryTotal     int64     `json:"memory_total"`
	MemoryUsed      int64     `json:"memory_used"`
	MemoryAllocated int64     `json:"memory_allocated"`
	MemoryAvailable int64     `json:"memory_available"`
	Utilization     int       `json:"utilization"`
	Temperature     int       `json:"temperature"`
	PowerUsage      int       `json:"power_usage"`
	PowerCap        int       `json:"power_cap"`
	InstanceCount   int       `json:"instance_count"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// GPUMonitor monitors GPU devices periodically
type GPUMonitor struct {
	manager  *GPUManager
	interval time.Duration
	stopChan chan struct{}
	running  bool
	mu       sync.Mutex
}

// NewGPUMonitor creates a new GPU monitor
func NewGPUMonitor(manager *GPUManager, interval time.Duration) *GPUMonitor {
	return &GPUMonitor{
		manager:  manager,
		interval: interval,
		stopChan: make(chan struct{}),
	}
}

// Start starts the GPU monitor
func (m *GPUMonitor) Start() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		return
	}

	m.running = true
	go m.monitorRoutine()
	log.Println("GPU monitor started")
}

// Stop stops the GPU monitor
func (m *GPUMonitor) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return
	}

	close(m.stopChan)
	m.running = false
	log.Println("GPU monitor stopped")
}

// monitorRoutine periodically monitors GPU devices
func (m *GPUMonitor) monitorRoutine() {
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.updateGPUInfo()
		case <-m.stopChan:
			return
		}
	}
}

// updateGPUInfo updates GPU information for all devices
func (m *GPUMonitor) updateGPUInfo() {
	// In a real implementation, this would query nvidia-smi or use NVML
	// to get updated GPU information (utilization, memory, temperature, etc.)

	// Placeholder: Update GPU info from actual hardware
	// For each device in m.manager.devices, query current stats

	// Example structure (would be implemented with actual GPU API calls):
	// for deviceID := range m.manager.devices {
	//     info, err := m.queryGPUInfo(deviceID)
	//     if err != nil {
	//         log.Printf("Failed to query GPU %d: %v", deviceID, err)
	//         continue
	//     }
	//     m.manager.UpdateGPUInfo(deviceID, info)
	// }
}

// queryGPUInfo queries actual GPU information (placeholder)
func (m *GPUMonitor) queryGPUInfo(deviceID int) (*GPUInfo, error) {
	// Placeholder implementation
	// In production, this would use NVML or nvidia-smi to query GPU stats

	info := &GPUInfo{
		ID: deviceID,
		// Would be populated with actual data
	}

	return info, nil
}
