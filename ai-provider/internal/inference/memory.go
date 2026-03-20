package inference

import (
	"fmt"
	"log"
	"runtime"
	"sync"
	"time"
)

// MemoryManager handles memory allocation, tracking, and optimization
type MemoryManager struct {
	totalMemory     int64
	usedMemory      int64
	reservedMemory  int64
	memoryPools     map[string]*MemoryPool
	allocations     map[string]*MemoryAllocation
	mu              sync.RWMutex
	config          *MemoryConfig
	stats           *MemoryStats
	gcTicker        *time.Ticker
	stopGC          chan struct{}
}

// MemoryConfig represents configuration for memory management
type MemoryConfig struct {
	TotalMemoryBytes    int64         `json:"total_memory_bytes"`
	MaxMemoryUsage      float64       `json:"max_memory_usage"`      // 0.0-1.0
	ReservedMemoryBytes int64         `json:"reserved_memory_bytes"`
	PoolSizeBytes       int64         `json:"pool_size_bytes"`
	EnablePooling       bool          `json:"enable_pooling"`
	GCInterval          time.Duration `json:"gc_interval"`
	GCThreshold         float64       `json:"gc_threshold"`         // 0.0-1.0
	EnableOptimization  bool          `json:"enable_optimization"`
}

// MemoryPool represents a pool of pre-allocated memory
type MemoryPool struct {
	ID         string    `json:"id"`
	Size       int64     `json:"size"`
	Used       int64     `json:"used"`
	Available  int64     `json:"available"`
	Blocks     []*MemoryBlock `json:"blocks"`
	CreatedAt  time.Time `json:"created_at"`
	mu         sync.Mutex
}

// MemoryBlock represents a block of memory in a pool
type MemoryBlock struct {
	ID       string    `json:"id"`
	Size     int64     `json:"size"`
	InUse    bool      `json:"in_use"`
	AllocatedAt time.Time `json:"allocated_at"`
}

// MemoryAllocation represents a memory allocation
type MemoryAllocation struct {
	ID          string    `json:"id"`
	InstanceID  string    `json:"instance_id"`
	Size        int64     `json:"size"`
	PoolID      string    `json:"pool_id,omitempty"`
	AllocatedAt time.Time `json:"allocated_at"`
}

// MemoryStats represents memory statistics
type MemoryStats struct {
	TotalMemory      int64   `json:"total_memory"`
	UsedMemory       int64   `json:"used_memory"`
	FreeMemory       int64   `json:"free_memory"`
	ReservedMemory   int64   `json:"reserved_memory"`
	AvailableMemory  int64   `json:"available_memory"`
	UsagePercent     float64 `json:"usage_percent"`
	PoolCount        int     `json:"pool_count"`
	ActiveAllocations int    `json:"active_allocations"`
	TotalAllocations int64   `json:"total_allocations"`
	TotalFrees       int64   `json:"total_frees"`
	GCRuns           int64   `json:"gc_runs"`
	LastGCRun        time.Time `json:"last_gc_run"`
	PeakUsage        int64   `json:"peak_usage"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// NewMemoryManager creates a new memory manager instance
func NewMemoryManager() *MemoryManager {
	// Get system memory info
	var sysMem uint64
	memInfo := &runtime.MemStats{}
	runtime.ReadMemStats(memInfo)
	sysMem = memInfo.Sys

	config := &MemoryConfig{
		TotalMemoryBytes:    int64(sysMem),
		MaxMemoryUsage:      0.8, // 80% max usage
		ReservedMemoryBytes: 512 * 1024 * 1024, // 512MB reserved
		PoolSizeBytes:       256 * 1024 * 1024, // 256MB pools
		EnablePooling:       true,
		GCInterval:          30 * time.Second,
		GCThreshold:         0.75, // GC at 75% usage
		EnableOptimization:  true,
	}

	mm := &MemoryManager{
		totalMemory:    config.TotalMemoryBytes,
		reservedMemory: config.ReservedMemoryBytes,
		memoryPools:    make(map[string]*MemoryPool),
		allocations:    make(map[string]*MemoryAllocation),
		config:         config,
		stats:          &MemoryStats{
			TotalMemory:    config.TotalMemoryBytes,
			ReservedMemory: config.ReservedMemoryBytes,
			UpdatedAt:      time.Now(),
		},
		stopGC: make(chan struct{}),
	}

	// Start garbage collection routine
	if config.GCInterval > 0 {
		mm.gcTicker = time.NewTicker(config.GCInterval)
		go mm.gcRoutine()
	}

	log.Printf("Memory manager initialized: total=%dMB, reserved=%dMB",
		config.TotalMemoryBytes/1024/1024, config.ReservedMemoryBytes/1024/1024)

	return mm
}

// Allocate allocates memory for an instance
func (mm *MemoryManager) Allocate(instanceID string, size int64) (*MemoryAllocation, error) {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	// Check if we have enough available memory
	available := mm.getAvailableMemory()
	if size > available {
		return nil, ErrInsufficientMemoryError(size, available)
	}

	// Try to allocate from pool first
	var allocation *MemoryAllocation
	if mm.config.EnablePooling {
		allocation = mm.allocateFromPool(instanceID, size)
	}

	// If pooling failed or disabled, allocate directly
	if allocation == nil {
		allocation = mm.allocateDirect(instanceID, size)
	}

	if allocation == nil {
		return nil, NewError(ErrResourceExhausted, "Failed to allocate memory")
	}

	// Track allocation
	mm.allocations[allocation.ID] = allocation
	mm.usedMemory += size
	mm.updateStats()

	log.Printf("Allocated %dMB for instance %s (allocation: %s)",
		size/1024/1024, instanceID, allocation.ID)

	return allocation, nil
}

// allocateFromPool attempts to allocate from a memory pool
func (mm *MemoryManager) allocateFromPool(instanceID string, size int64) *MemoryAllocation {
	// Find a pool with enough space
	for _, pool := range mm.memoryPools {
		pool.mu.Lock()
		if pool.Available >= size {
			// Find or create a block
			block := mm.findOrCreateBlock(pool, size)
			if block != nil {
				block.InUse = true
				block.AllocatedAt = time.Now()
				pool.Used += block.Size
				pool.Available -= block.Size
				pool.mu.Unlock()

				allocation := &MemoryAllocation{
					ID:         generateID("mem"),
					InstanceID: instanceID,
					Size:       block.Size,
					PoolID:     pool.ID,
					AllocatedAt: time.Now(),
				}
				return allocation
			}
		}
		pool.mu.Unlock()
	}

	return nil
}

// allocateDirect allocates memory directly (not from pool)
func (mm *MemoryManager) allocateDirect(instanceID string, size int64) *MemoryAllocation {
	return &MemoryAllocation{
		ID:          generateID("mem"),
		InstanceID:  instanceID,
		Size:        size,
		AllocatedAt: time.Now(),
	}
}

// findOrCreateBlock finds an available block or creates a new one
func (mm *MemoryManager) findOrCreateBlock(pool *MemoryPool, size int64) *MemoryBlock {
	// Try to find an available block
	for _, block := range pool.Blocks {
		if !block.InUse && block.Size >= size {
			return block
		}
	}

	// Create a new block if pool has space
	if pool.Available >= size {
		block := &MemoryBlock{
			ID:   generateID("blk"),
			Size: size,
		}
		pool.Blocks = append(pool.Blocks, block)
		return block
	}

	return nil
}

// Release releases memory allocated to an instance
func (mm *MemoryManager) Release(allocationID string) error {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	allocation, exists := mm.allocations[allocationID]
	if !exists {
		return fmt.Errorf("allocation %s not found", allocationID)
	}

	// If from pool, release back to pool
	if allocation.PoolID != "" {
		mm.releaseToPool(allocation)
	}

	// Update memory tracking
	mm.usedMemory -= allocation.Size
	delete(mm.allocations, allocationID)
	mm.stats.TotalFrees++
	mm.updateStats()

	log.Printf("Released %dMB from allocation %s", allocation.Size/1024/1024, allocationID)

	return nil
}

// releaseToPool releases memory back to a pool
func (mm *MemoryManager) releaseToPool(allocation *MemoryAllocation) {
	pool, exists := mm.memoryPools[allocation.PoolID]
	if !exists {
		return
	}

	pool.mu.Lock()
	defer pool.mu.Unlock()

	// Find and release the block
	for _, block := range pool.Blocks {
		if block.InUse && block.Size == allocation.Size {
			block.InUse = false
			pool.Used -= block.Size
			pool.Available += block.Size
			break
		}
	}
}

// CreatePool creates a new memory pool
func (mm *MemoryManager) CreatePool(poolID string, size int64) (*MemoryPool, error) {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	if _, exists := mm.memoryPools[poolID]; exists {
		return nil, fmt.Errorf("pool %s already exists", poolID)
	}

	// Check if we have enough memory for the pool
	available := mm.getAvailableMemory()
	if size > available {
		return nil, ErrInsufficientMemoryError(size, available)
	}

	pool := &MemoryPool{
		ID:        poolID,
		Size:      size,
		Available: size,
		Blocks:    make([]*MemoryBlock, 0),
		CreatedAt: time.Now(),
	}

	mm.memoryPools[poolID] = pool
	mm.usedMemory += size
	mm.stats.PoolCount++
	mm.updateStats()

	log.Printf("Created memory pool %s: size=%dMB", poolID, size/1024/1024)

	return pool, nil
}

// DestroyPool destroys a memory pool
func (mm *MemoryManager) DestroyPool(poolID string) error {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	pool, exists := mm.memoryPools[poolID]
	if !exists {
		return fmt.Errorf("pool %s not found", poolID)
	}

	pool.mu.Lock()
	defer pool.mu.Unlock()

	// Check if pool has in-use blocks
	for _, block := range pool.Blocks {
		if block.InUse {
			return fmt.Errorf("pool %s has in-use blocks", poolID)
		}
	}

	// Release pool memory
	mm.usedMemory -= pool.Size
	delete(mm.memoryPools, poolID)
	mm.stats.PoolCount--
	mm.updateStats()

	log.Printf("Destroyed memory pool %s", poolID)

	return nil
}

// GetAvailableMemory returns the amount of available memory
func (mm *MemoryManager) GetAvailableMemory() int64 {
	mm.mu.RLock()
	defer mm.mu.RUnlock()
	return mm.getAvailableMemory()
}

// getAvailableMemory returns available memory (must be called with lock held)
func (mm *MemoryManager) getAvailableMemory() int64 {
	maxUsable := int64(float64(mm.totalMemory) * mm.config.MaxMemoryUsage)
	available := maxUsable - mm.usedMemory - mm.reservedMemory
	if available < 0 {
		return 0
	}
	return available
}

// GetStats returns current memory statistics
func (mm *MemoryManager) GetStats() *MemoryStats {
	mm.mu.RLock()
	defer mm.mu.RUnlock()
	return mm.stats
}

// GetUsage returns the current memory usage percentage
func (mm *MemoryManager) GetUsage() float64 {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	if mm.totalMemory == 0 {
		return 0
	}
	return float64(mm.usedMemory) / float64(mm.totalMemory)
}

// ReserveMemory reserves memory for future use
func (mm *MemoryManager) ReserveMemory(size int64) error {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	available := mm.getAvailableMemory()
	if size > available {
		return ErrInsufficientMemoryError(size, available)
	}

	mm.reservedMemory += size
	mm.updateStats()

	log.Printf("Reserved %dMB of memory", size/1024/1024)

	return nil
}

// ReleaseMemory releases reserved memory
func (mm *MemoryManager) ReleaseMemory(size int64) {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	mm.reservedMemory -= size
	if mm.reservedMemory < 0 {
		mm.reservedMemory = 0
	}
	mm.updateStats()

	log.Printf("Released %dMB of reserved memory", size/1024/1024)
}

// GetUsedMemory returns the amount of used memory
func (mm *MemoryManager) GetUsedMemory() int64 {
	mm.mu.RLock()
	defer mm.mu.RUnlock()
	return mm.usedMemory
}

// GetTotalMemory returns the total amount of memory
func (mm *MemoryManager) GetTotalMemory() int64 {
	mm.mu.RLock()
	defer mm.mu.RUnlock()
	return mm.totalMemory
}

// GetCPUUsage returns the current CPU usage percentage
func (mm *MemoryManager) GetCPUUsage() float64 {
	// In a real implementation, this would query actual CPU usage
	// For now, return a placeholder value
	return 0.0
}

// GetMemoryUsagePercent returns memory usage as a percentage
func (mm *MemoryManager) GetMemoryUsagePercent() float64 {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	if mm.totalMemory == 0 {
		return 0
	}
	return (float64(mm.usedMemory) / float64(mm.totalMemory)) * 100
}

// Optimize performs memory optimization
func (mm *MemoryManager) Optimize() error {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	log.Println("Running memory optimization...")

	// Defragment pools
	for _, pool := range mm.memoryPools {
		mm.defragmentPool(pool)
	}

	// Run garbage collection
	runtime.GC()

	// Update stats
	mm.updateStats()

	log.Println("Memory optimization complete")

	return nil
}

// defragmentPool defragments a memory pool
func (mm *MemoryManager) defragmentPool(pool *MemoryPool) {
	pool.mu.Lock()
	defer pool.mu.Unlock()

	// Remove unused blocks
	var activeBlocks []*MemoryBlock
	for _, block := range pool.Blocks {
		if block.InUse {
			activeBlocks = append(activeBlocks, block)
		}
	}

	pool.Blocks = activeBlocks
	pool.Available = pool.Size - pool.Used
}

// gcRoutine periodically runs garbage collection
func (mm *MemoryManager) gcRoutine() {
	for {
		select {
		case <-mm.gcTicker.C:
			mm.runGC()
		case <-mm.stopGC:
			return
		}
	}
}

// runGC runs garbage collection if needed
func (mm *MemoryManager) runGC() {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	usage := float64(mm.usedMemory) / float64(mm.totalMemory)

	if usage >= mm.config.GCThreshold {
		log.Printf("Running GC: usage=%.2f%%, threshold=%.2f%%",
			usage*100, mm.config.GCThreshold*100)

		// Run Go's garbage collector
		runtime.GC()

		// Update stats
		mm.stats.GCRuns++
		mm.stats.LastGCRun = time.Now()

		// Read actual memory usage after GC
		memInfo := &runtime.MemStats{}
		runtime.ReadMemStats(memInfo)

		log.Printf("GC complete: allocated=%dMB", memInfo.Alloc/1024/1024)
	}
}

// updateStats updates memory statistics
func (mm *MemoryManager) updateStats() {
	mm.stats.UsedMemory = mm.usedMemory
	mm.stats.FreeMemory = mm.totalMemory - mm.usedMemory
	mm.stats.AvailableMemory = mm.getAvailableMemory()

	if mm.totalMemory > 0 {
		mm.stats.UsagePercent = float64(mm.usedMemory) / float64(mm.totalMemory) * 100
	}

	if mm.usedMemory > mm.stats.PeakUsage {
		mm.stats.PeakUsage = mm.usedMemory
	}

	mm.stats.ActiveAllocations = len(mm.allocations)
	mm.stats.TotalAllocations++
	mm.stats.UpdatedAt = time.Now()
}

// SetMaxMemoryUsage sets the maximum memory usage threshold
func (mm *MemoryManager) SetMaxMemoryUsage(maxUsage float64) error {
	if maxUsage < 0 || maxUsage > 1 {
		return fmt.Errorf("max usage must be between 0 and 1")
	}

	mm.mu.Lock()
	defer mm.mu.Unlock()
	mm.config.MaxMemoryUsage = maxUsage
	return nil
}

// SetGCThreshold sets the garbage collection threshold
func (mm *MemoryManager) SetGCThreshold(threshold float64) error {
	if threshold < 0 || threshold > 1 {
		return fmt.Errorf("threshold must be between 0 and 1")
	}

	mm.mu.Lock()
	defer mm.mu.Unlock()
	mm.config.GCThreshold = threshold
	return nil
}

// ListAllocations lists all active memory allocations
func (mm *MemoryManager) ListAllocations() []*MemoryAllocation {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	allocations := make([]*MemoryAllocation, 0, len(mm.allocations))
	for _, alloc := range mm.allocations {
		allocations = append(allocations, alloc)
	}
	return allocations
}

// ListPools lists all memory pools
func (mm *MemoryManager) ListPools() []*MemoryPool {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	pools := make([]*MemoryPool, 0, len(mm.memoryPools))
	for _, pool := range mm.memoryPools {
		pools = append(pools, pool)
	}
	return pools
}

// GetInstanceMemory returns the total memory allocated to an instance
func (mm *MemoryManager) GetInstanceMemory(instanceID string) int64 {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	var total int64
	for _, alloc := range mm.allocations {
		if alloc.InstanceID == instanceID {
			total += alloc.Size
		}
	}
	return total
}

// ReleaseInstanceMemory releases all memory for an instance
func (mm *MemoryManager) ReleaseInstanceMemory(instanceID string) error {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	var toRelease []string
	for allocID, alloc := range mm.allocations {
		if alloc.InstanceID == instanceID {
			toRelease = append(toRelease, allocID)
		}
	}

	for _, allocID := range toRelease {
		mm.Release(allocID)
	}

	log.Printf("Released all memory for instance %s: %d allocations",
		instanceID, len(toRelease))

	return nil
}

// Shutdown shuts down the memory manager
func (mm *MemoryManager) Shutdown() error {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	// Stop GC routine
	if mm.gcTicker != nil {
		mm.gcTicker.Stop()
	}
	close(mm.stopGC)

	// Release all allocations
	for allocID := range mm.allocations {
		mm.Release(allocID)
	}

	// Destroy all pools
	for poolID := range mm.memoryPools {
		mm.DestroyPool(poolID)
	}

	log.Println("Memory manager shutdown complete")
	return nil
}

// Helper function to generate unique IDs
func generateID(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}
