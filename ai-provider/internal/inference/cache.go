package inference

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"
)

// InferenceCache provides caching for inference results
type InferenceCache struct {
	entries    map[string]*CacheEntry
	mu         sync.RWMutex
	config     *CacheConfig
	stats      *CacheStats
	evictor    *CacheEvictor
	enabled    bool
}

// CacheConfig represents configuration for the inference cache
type CacheConfig struct {
	Enabled          bool          `json:"enabled"`
	MaxSizeBytes     int64         `json:"max_size_bytes"`
	DefaultTTL       time.Duration `json:"default_ttl"`
	MaxEntries       int           `json:"max_entries"`
	EvictionPolicy   string        `json:"eviction_policy"` // lru, lfu, fifo
	EvictionInterval time.Duration `json:"eviction_interval"`
	EnableWarmup     bool          `json:"enable_warmup"`
	HashAlgorithm    string        `json:"hash_algorithm"`
}

// CacheStats represents cache statistics
type CacheStats struct {
	TotalRequests   int64     `json:"total_requests"`
	Hits            int64     `json:"hits"`
	Misses          int64     `json:"misses"`
	HitRate         float64   `json:"hit_rate"`
	Evictions       int64     `json:"evictions"`
	Expirations     int64     `json:"expirations"`
	TotalSize       int64     `json:"total_size"`
	EntryCount      int       `json:"entry_count"`
	AverageLatency  time.Duration `json:"average_latency"`
	MemoryUsage     int64     `json:"memory_usage"`
	LastEviction    time.Time `json:"last_eviction"`
	LastCleanup     time.Time `json:"last_cleanup"`
	UpdatedAt       time.Time `json:"updated_at"`
	mu              sync.RWMutex
}

// CacheEvictor handles cache eviction
type CacheEvictor struct {
	cache     *InferenceCache
	policy    string
	interval  time.Duration
	stopChan  chan struct{}
	running   bool
	mu        sync.Mutex
}

// NewInferenceCache creates a new inference cache instance
func NewInferenceCache(config *CacheConfig) *InferenceCache {
	if config == nil {
		config = &CacheConfig{
			Enabled:          true,
			MaxSizeBytes:     1024 * 1024 * 1024, // 1GB
			DefaultTTL:       1 * time.Hour,
			MaxEntries:       10000,
			EvictionPolicy:   "lru",
			EvictionInterval: 5 * time.Minute,
			EnableWarmup:     false,
			HashAlgorithm:    "sha256",
		}
	}

	cache := &InferenceCache{
		entries: make(map[string]*CacheEntry),
		config:  config,
		stats:   &CacheStats{UpdatedAt: time.Now()},
		enabled: config.Enabled,
	}

	// Start eviction routine
	if config.Enabled && config.EvictionInterval > 0 {
		cache.evictor = NewCacheEvictor(cache, config.EvictionPolicy, config.EvictionInterval)
		cache.evictor.Start()
	}

	log.Printf("Inference cache initialized: enabled=%v, max_size=%dMB, max_entries=%d",
		config.Enabled, config.MaxSizeBytes/1024/1024, config.MaxEntries)

	return cache
}

// Get retrieves a cached result
func (c *InferenceCache) Get(ctx context.Context, req *InferenceRequest) (*InferenceResponse, error) {
	if !c.enabled {
		return nil, ErrCacheMissError("")
	}

	key, err := c.generateKey(req)
	if err != nil {
		return nil, err
	}

	c.mu.RLock()
	entry, exists := c.entries[key]
	c.mu.RUnlock()

	// Update stats
	c.stats.mu.Lock()
	c.stats.TotalRequests++
	c.stats.mu.Unlock()

	if !exists {
		c.recordMiss()
		return nil, ErrCacheMissError(key)
	}

	// Check if entry has expired
	if time.Now().After(entry.ExpiresAt) {
		c.mu.Lock()
		delete(c.entries, key)
		c.mu.Unlock()

		c.recordMiss()
		c.stats.mu.Lock()
		c.stats.Expirations++
		c.stats.mu.Unlock()

		return nil, ErrCacheMissError(key)
	}

	// Update hit count and last accessed time
	c.mu.Lock()
	entry.HitCount++
	entry.Metadata["last_accessed"] = time.Now()
	c.mu.Unlock()

	// Record cache hit
	c.recordHit()

	// Return a copy of the response to avoid race conditions
	response := *entry.Response
	return &response, nil
}

// Set stores a result in the cache
func (c *InferenceCache) Set(ctx context.Context, req *InferenceRequest, resp *InferenceResponse) error {
	if !c.enabled {
		return nil
	}

	key, err := c.generateKey(req)
	if err != nil {
		return err
	}

	// Estimate entry size
	size := c.estimateEntrySize(req, resp)

	// Check if we need to make room
	if err := c.ensureCapacity(size); err != nil {
		return err
	}

	// Create cache entry
	entry := &CacheEntry{
		Key:         key,
		RequestHash: key,
		Response:    resp,
		CreatedAt:   time.Now(),
		ExpiresAt:   time.Now().Add(c.config.DefaultTTL),
		HitCount:    0,
		Size:        size,
		Metadata: map[string]interface{}{
			"model_id":    req.ModelID,
			"request_id":  req.ID,
			"last_accessed": time.Now(),
		},
	}

	// Store entry
	c.mu.Lock()
	c.entries[key] = entry
	c.mu.Unlock()

	// Update stats
	c.stats.mu.Lock()
	c.stats.EntryCount = len(c.entries)
	c.stats.TotalSize += size
	c.stats.UpdatedAt = time.Now()
	c.stats.mu.Unlock()

	log.Printf("Cached result for request %s: key=%s, size=%d bytes", req.ID, key, size)

	return nil
}

// Delete removes an entry from the cache
func (c *InferenceCache) Delete(key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, exists := c.entries[key]
	if !exists {
		return ErrCacheMissError(key)
	}

	// Update stats
	c.stats.mu.Lock()
	c.stats.TotalSize -= entry.Size
	c.stats.EntryCount--
	c.stats.mu.Unlock()

	delete(c.entries, key)

	return nil
}

// Clear clears all entries from the cache
func (c *InferenceCache) Clear() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	count := len(c.entries)
	c.entries = make(map[string]*CacheEntry)

	// Reset stats
	c.stats.mu.Lock()
	c.stats.TotalSize = 0
	c.stats.EntryCount = 0
	c.stats.UpdatedAt = time.Now()
	c.stats.mu.Unlock()

	log.Printf("Cleared cache: removed %d entries", count)

	return nil
}

// GetStats returns current cache statistics
func (c *InferenceCache) GetStats() *CacheStats {
	c.stats.mu.RLock()
	stats := *c.stats
	c.stats.mu.RUnlock()

	// Calculate hit rate
	if stats.TotalRequests > 0 {
		stats.HitRate = float64(stats.Hits) / float64(stats.TotalRequests)
	}

	c.mu.RLock()
	stats.EntryCount = len(c.entries)
	c.mu.RUnlock()

	return &stats
}

// GetEntry retrieves a cache entry by key
func (c *InferenceCache) GetEntry(key string) (*CacheEntry, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.entries[key]
	if !exists {
		return nil, ErrCacheMissError(key)
	}

	return entry, nil
}

// ListEntries lists all cache entries
func (c *InferenceCache) ListEntries(limit int) []*CacheEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entries := make([]*CacheEntry, 0, len(c.entries))
	count := 0

	for _, entry := range c.entries {
		entries = append(entries, entry)
		count++
		if limit > 0 && count >= limit {
			break
		}
	}

	return entries
}

// SetTTL sets a custom TTL for a specific entry
func (c *InferenceCache) SetTTL(key string, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, exists := c.entries[key]
	if !exists {
		return ErrCacheMissError(key)
	}

	entry.ExpiresAt = time.Now().Add(ttl)
	return nil
}

// Invalidate invalidates cache entries matching a pattern
func (c *InferenceCache) Invalidate(modelID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var toDelete []string
	for key, entry := range c.entries {
		if modelID != "" {
			if entry.Metadata["model_id"] == modelID {
				toDelete = append(toDelete, key)
			}
		} else {
			// Invalidate all
			toDelete = append(toDelete, key)
		}
	}

	// Delete entries
	for _, key := range toDelete {
		entry := c.entries[key]
		c.stats.mu.Lock()
		c.stats.TotalSize -= entry.Size
		c.stats.mu.Unlock()
		delete(c.entries, key)
	}

	log.Printf("Invalidated %d cache entries for model %s", len(toDelete), modelID)

	return nil
}

// Warmup warms up the cache with pre-computed results
func (c *InferenceCache) Warmup(ctx context.Context, requests []*InferenceRequest, executor func(*InferenceRequest) (*InferenceResponse, error)) error {
	if !c.config.EnableWarmup {
		return NewError(ErrCacheDisabled, "Cache warmup is disabled")
	}

	log.Printf("Starting cache warmup with %d requests", len(requests))

	var warmed int
	var failed int

	for _, req := range requests {
		// Check if already cached
		key, err := c.generateKey(req)
		if err != nil {
			failed++
			continue
		}

		c.mu.RLock()
		_, exists := c.entries[key]
		c.mu.RUnlock()

		if exists {
			continue // Already cached
		}

		// Execute request
		resp, err := executor(req)
		if err != nil {
			log.Printf("Cache warmup failed for request %s: %v", req.ID, err)
			failed++
			continue
		}

		// Cache result
		if err := c.Set(ctx, req, resp); err != nil {
			log.Printf("Failed to cache warmup result: %v", err)
			failed++
			continue
		}

		warmed++
	}

	log.Printf("Cache warmup complete: warmed=%d, failed=%d", warmed, failed)

	return nil
}

// generateKey generates a unique cache key for a request
func (c *InferenceCache) generateKey(req *InferenceRequest) (string, error) {
	// Create a hashable representation of the request
	keyData := map[string]interface{}{
		"model_id":    req.ModelID,
		"prompt":      req.Prompt,
		"max_tokens":  req.MaxTokens,
		"temperature": req.Temperature,
		"top_p":       req.TopP,
		"top_k":       req.TopK,
		"stop":        req.Stop,
		"frequency_penalty": req.FrequencyPenalty,
		"presence_penalty":  req.PresencePenalty,
	}

	// Include messages if present
	if len(req.Messages) > 0 {
		keyData["messages"] = req.Messages
	}

	// Serialize to JSON
	data, err := json.Marshal(keyData)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	// Generate hash
	hash := sha256.Sum256(data)
	return fmt.Sprintf("%x", hash), nil
}

// estimateEntrySize estimates the size of a cache entry
func (c *InferenceCache) estimateEntrySize(req *InferenceRequest, resp *InferenceResponse) int64 {
	size := int64(0)

	// Estimate request size
	size += int64(len(req.Prompt))
	size += int64(len(req.Messages) * 100) // rough estimate
	size += int64(500) // metadata overhead

	// Estimate response size
	size += int64(len(resp.Content))
	size += int64(resp.TotalTokens * 10) // rough estimate for tokens
	size += int64(500) // metadata overhead

	return size
}

// ensureCapacity ensures there's enough capacity in the cache
func (c *InferenceCache) ensureCapacity(requiredSize int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check max entries
	if len(c.entries) >= c.config.MaxEntries {
		if err := c.evict(1); err != nil {
			return err
		}
	}

	// Check max size
	currentSize := c.getCurrentSize()
	if currentSize+requiredSize > c.config.MaxSizeBytes {
		// Need to evict entries to make room
		toFree := (currentSize + requiredSize) - c.config.MaxSizeBytes
		if err := c.evictToFree(toFree); err != nil {
			return ErrCacheFullError(c.config.MaxSizeBytes)
		}
	}

	return nil
}

// getCurrentSize gets the current total size of cached entries
func (c *InferenceCache) getCurrentSize() int64 {
	var total int64
	for _, entry := range c.entries {
		total += entry.Size
	}
	return total
}

// evict evicts a specific number of entries
func (c *InferenceCache) evict(count int) error {
	if count <= 0 {
		return nil
	}

	// Get entries sorted by eviction policy
	entries := c.getSortedEntries()

	// Evict the specified number
	evicted := 0
	for _, entry := range entries {
		if evicted >= count {
			break
		}

		delete(c.entries, entry.Key)
		c.stats.mu.Lock()
		c.stats.TotalSize -= entry.Size
		c.stats.Evictions++
		c.stats.mu.Unlock()

		evicted++
	}

	return nil
}

// evictToFree evicts entries to free up a specific amount of space
func (c *InferenceCache) evictToFree(required int64) error {
	entries := c.getSortedEntries()

	var freed int64
	for _, entry := range entries {
		if freed >= required {
			break
		}

		delete(c.entries, entry.Key)
		c.stats.mu.Lock()
		c.stats.TotalSize -= entry.Size
		c.stats.Evictions++
		c.stats.mu.Unlock()

		freed += entry.Size
	}

	return nil
}

// getSortedEntries returns entries sorted by eviction policy
func (c *InferenceCache) getSortedEntries() []*CacheEntry {
	entries := make([]*CacheEntry, 0, len(c.entries))
	for _, entry := range c.entries {
		entries = append(entries, entry)
	}

	// Sort based on eviction policy
	switch c.config.EvictionPolicy {
	case "lru":
		// Sort by last accessed time (oldest first)
		entries = sortByLastAccessed(entries)
	case "lfu":
		// Sort by hit count (lowest first)
		entries = sortByHitCount(entries)
	case "fifo":
		// Sort by creation time (oldest first)
		entries = sortByCreationTime(entries)
	default:
		// Default to LRU
		entries = sortByLastAccessed(entries)
	}

	return entries
}

// recordHit records a cache hit
func (c *InferenceCache) recordHit() {
	c.stats.mu.Lock()
	defer c.stats.mu.Unlock()
	c.stats.Hits++
	c.stats.UpdatedAt = time.Now()
}

// recordMiss records a cache miss
func (c *InferenceCache) recordMiss() {
	c.stats.mu.Lock()
	defer c.stats.mu.Unlock()
	c.stats.Misses++
	c.stats.UpdatedAt = time.Now()
}

// Cleanup removes expired entries
func (c *InferenceCache) Cleanup() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var toDelete []string
	now := time.Now()

	for key, entry := range c.entries {
		if now.After(entry.ExpiresAt) {
			toDelete = append(toDelete, key)
		}
	}

	// Delete expired entries
	for _, key := range toDelete {
		entry := c.entries[key]
		c.stats.mu.Lock()
		c.stats.TotalSize -= entry.Size
		c.stats.Expirations++
		c.stats.mu.Unlock()
		delete(c.entries, key)
	}

	if len(toDelete) > 0 {
		log.Printf("Cache cleanup: removed %d expired entries", len(toDelete))
		c.stats.mu.Lock()
		c.stats.LastCleanup = now
		c.stats.mu.Unlock()
	}

	return nil
}

// Shutdown shuts down the cache
func (c *InferenceCache) Shutdown() error {
	if c.evictor != nil {
		c.evictor.Stop()
	}

	c.Clear()

	log.Println("Inference cache shutdown complete")
	return nil
}

// NewCacheEvictor creates a new cache evictor
func NewCacheEvictor(cache *InferenceCache, policy string, interval time.Duration) *CacheEvictor {
	return &CacheEvictor{
		cache:    cache,
		policy:   policy,
		interval: interval,
		stopChan: make(chan struct{}),
	}
}

// Start starts the eviction routine
func (e *CacheEvictor) Start() {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.running {
		return
	}

	e.running = true
	go e.evictionRoutine()

	log.Printf("Cache evictor started: policy=%s, interval=%v", e.policy, e.interval)
}

// Stop stops the eviction routine
func (e *CacheEvictor) Stop() {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.running {
		return
	}

	close(e.stopChan)
	e.running = false

	log.Println("Cache evictor stopped")
}

// evictionRoutine periodically runs eviction
func (e *CacheEvictor) evictionRoutine() {
	ticker := time.NewTicker(e.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			e.runEviction()
		case <-e.stopChan:
			return
		}
	}
}

// runEviction runs the eviction process
func (e *CacheEvictor) runEviction() {
	// Run cleanup first
	e.cache.Cleanup()

	// Check if eviction is needed
	stats := e.cache.GetStats()
	if stats.TotalSize > int64(float64(e.cache.config.MaxSizeBytes)*0.9) {
		// Cache is 90% full, evict 10% of entries
		toEvict := int(float64(stats.EntryCount) * 0.1)
		if toEvict > 0 {
			e.cache.evict(toEvict)
			e.cache.stats.mu.Lock()
			e.cache.stats.LastEviction = time.Now()
			e.cache.stats.mu.Unlock()
		}
	}
}

// Sorting helper functions

func sortByLastAccessed(entries []*CacheEntry) []*CacheEntry {
	// Simple bubble sort (for small datasets)
	n := len(entries)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			timeJ := getLastAccessedTime(entries[j])
			timeJ1 := getLastAccessedTime(entries[j+1])
			if timeJ.Before(timeJ1) {
				entries[j], entries[j+1] = entries[j+1], entries[j]
			}
		}
	}
	return entries
}

func sortByHitCount(entries []*CacheEntry) []*CacheEntry {
	n := len(entries)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			if entries[j].HitCount > entries[j+1].HitCount {
				entries[j], entries[j+1] = entries[j+1], entries[j]
			}
		}
	}
	return entries
}

func sortByCreationTime(entries []*CacheEntry) []*CacheEntry {
	n := len(entries)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			if entries[j].CreatedAt.After(entries[j+1].CreatedAt) {
				entries[j], entries[j+1] = entries[j+1], entries[j]
			}
		}
	}
	return entries
}

func getLastAccessedTime(entry *CacheEntry) time.Time {
	if lastAccessed, ok := entry.Metadata["last_accessed"].(time.Time); ok {
		return lastAccessed
	}
	return entry.CreatedAt
}
