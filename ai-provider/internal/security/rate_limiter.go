// Package security provides security-related functionality including rate limiting,
// input validation, sanitization, and CSRF protection.
package security

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Rate limiter errors
var (
	ErrRateLimitExceeded   = errors.New("rate limit exceeded")
	ErrInvalidRateLimit    = errors.New("invalid rate limit configuration")
	ErrRateLimiterNotFound = errors.New("rate limiter not found")
)

// RateLimitConfig holds configuration for rate limiting
type RateLimitConfig struct {
	RequestsPerSecond float64       `json:"requests_per_second"`
	BurstSize         int           `json:"burst_size"`
	WindowDuration    time.Duration `json:"window_duration"`
	KeyFunc           KeyFunc       `json:"-"`
	OnLimitExceeded   LimitHandler  `json:"-"`
}

// KeyFunc is a function that extracts a rate limit key from a context
type KeyFunc func(ctx context.Context) string

// LimitHandler is called when a rate limit is exceeded
type LimitHandler func(ctx context.Context, key string, limit *RateLimitInfo)

// RateLimitInfo contains information about the current rate limit state
type RateLimitInfo struct {
	Key          string        `json:"key"`
	Limit        int           `json:"limit"`
	Remaining    int           `json:"remaining"`
	ResetAt      time.Time     `json:"reset_at"`
	RetryAfter   time.Duration `json:"retry_after"`
	WindowStart  time.Time     `json:"window_start"`
	TotalRequests int64        `json:"total_requests"`
}

// TokenBucket represents a token bucket rate limiter
type TokenBucket struct {
	rate       float64 // tokens per second
	burst      int     // maximum tokens
	tokens     float64 // current tokens
	lastUpdate time.Time
	mu         sync.Mutex
}

// NewTokenBucket creates a new token bucket rate limiter
func NewTokenBucket(rate float64, burst int) *TokenBucket {
	return &TokenBucket{
		rate:       rate,
		burst:      burst,
		tokens:     float64(burst),
		lastUpdate: time.Now(),
	}
}

// Allow checks if a request is allowed under the rate limit
func (tb *TokenBucket) Allow() bool {
	return tb.AllowN(1)
}

// AllowN checks if n requests are allowed under the rate limit
func (tb *TokenBucket) AllowN(n int) bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(tb.lastUpdate).Seconds()
	tb.lastUpdate = now

	// Add tokens based on elapsed time
	tb.tokens += elapsed * tb.rate
	if tb.tokens > float64(tb.burst) {
		tb.tokens = float64(tb.burst)
	}

	// Check if we have enough tokens
	if tb.tokens >= float64(n) {
		tb.tokens -= float64(n)
		return true
	}

	return false
}

// Wait blocks until a request is allowed or the context is cancelled
func (tb *TokenBucket) Wait(ctx context.Context) error {
	return tb.WaitN(ctx, 1)
}

// WaitN blocks until n requests are allowed or the context is cancelled
func (tb *TokenBucket) WaitN(ctx context.Context, n int) error {
	for {
		if tb.AllowN(n) {
			return nil
		}

		// Calculate wait time
		tb.mu.Lock()
		waitTime := time.Duration(float64(n)/tb.rate) * time.Second
		tb.mu.Unlock()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waitTime / 10):
			// Check again
		}
	}
}

// GetTokens returns the current number of tokens
func (tb *TokenBucket) GetTokens() float64 {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(tb.lastUpdate).Seconds()

	tokens := tb.tokens + elapsed*tb.rate
	if tokens > float64(tb.burst) {
		tokens = float64(tb.burst)
	}

	return tokens
}

// FixedWindowRateLimiter implements fixed window rate limiting
type FixedWindowRateLimiter struct {
	windows    map[string]*fixedWindow
	windowSize time.Duration
	limit      int
	mu         sync.RWMutex
}

type fixedWindow struct {
	count     int
	startTime time.Time
}

// NewFixedWindowRateLimiter creates a new fixed window rate limiter
func NewFixedWindowRateLimiter(limit int, windowSize time.Duration) *FixedWindowRateLimiter {
	return &FixedWindowRateLimiter{
		windows:    make(map[string]*fixedWindow),
		windowSize: windowSize,
		limit:      limit,
	}
}

// Allow checks if a request is allowed for the given key
func (rl *FixedWindowRateLimiter) Allow(key string) bool {
	return rl.AllowN(key, 1)
}

// AllowN checks if n requests are allowed for the given key
func (rl *FixedWindowRateLimiter) AllowN(key string, n int) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	window, exists := rl.windows[key]

	if !exists || now.Sub(window.startTime) >= rl.windowSize {
		// Create new window
		rl.windows[key] = &fixedWindow{
			count:     n,
			startTime: now,
		}
		return n <= rl.limit
	}

	// Check if adding n would exceed limit
	if window.count+n > rl.limit {
		return false
	}

	window.count += n
	return true
}

// GetInfo returns rate limit info for a key
func (rl *FixedWindowRateLimiter) GetInfo(key string) *RateLimitInfo {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	window, exists := rl.windows[key]
	if !exists {
		return &RateLimitInfo{
			Key:       key,
			Limit:     rl.limit,
			Remaining: rl.limit,
		}
	}

	now := time.Now()
	if now.Sub(window.startTime) >= rl.windowSize {
		return &RateLimitInfo{
			Key:       key,
			Limit:     rl.limit,
			Remaining: rl.limit,
		}
	}

	remaining := rl.limit - window.count
	if remaining < 0 {
		remaining = 0
	}

	return &RateLimitInfo{
		Key:         key,
		Limit:       rl.limit,
		Remaining:   remaining,
		WindowStart: window.startTime,
		ResetAt:     window.startTime.Add(rl.windowSize),
		TotalRequests: int64(window.count),
	}
}

// Reset resets the rate limit for a key
func (rl *FixedWindowRateLimiter) Reset(key string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.windows, key)
}

// Cleanup removes expired windows
func (rl *FixedWindowRateLimiter) Cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	for key, window := range rl.windows {
		if now.Sub(window.startTime) >= rl.windowSize {
			delete(rl.windows, key)
		}
	}
}

// SlidingWindowRateLimiter implements sliding window rate limiting
type SlidingWindowRateLimiter struct {
	windows    map[string]*slidingWindow
	windowSize time.Duration
	limit      int
	mu         sync.RWMutex
}

type slidingWindow struct {
	timestamps []time.Time
}

// NewSlidingWindowRateLimiter creates a new sliding window rate limiter
func NewSlidingWindowRateLimiter(limit int, windowSize time.Duration) *SlidingWindowRateLimiter {
	return &SlidingWindowRateLimiter{
		windows:    make(map[string]*slidingWindow),
		windowSize: windowSize,
		limit:      limit,
	}
}

// Allow checks if a request is allowed for the given key
func (rl *SlidingWindowRateLimiter) Allow(key string) bool {
	return rl.AllowN(key, 1)
}

// AllowN checks if n requests are allowed for the given key
func (rl *SlidingWindowRateLimiter) AllowN(key string, n int) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	windowStart := now.Add(-rl.windowSize)

	window, exists := rl.windows[key]
	if !exists {
		window = &slidingWindow{
			timestamps: make([]time.Time, 0),
		}
		rl.windows[key] = window
	}

	// Filter out old timestamps
	validIdx := 0
	for _, ts := range window.timestamps {
		if ts.After(windowStart) {
			window.timestamps[validIdx] = ts
			validIdx++
		}
	}
	window.timestamps = window.timestamps[:validIdx]

	// Check if adding n would exceed limit
	if len(window.timestamps)+n > rl.limit {
		return false
	}

	// Add new timestamps
	for i := 0; i < n; i++ {
		window.timestamps = append(window.timestamps, now)
	}

	return true
}

// GetInfo returns rate limit info for a key
func (rl *SlidingWindowRateLimiter) GetInfo(key string) *RateLimitInfo {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	window, exists := rl.windows[key]
	if !exists {
		return &RateLimitInfo{
			Key:       key,
			Limit:     rl.limit,
			Remaining: rl.limit,
		}
	}

	now := time.Now()
	windowStart := now.Add(-rl.windowSize)

	count := 0
	var oldestTime time.Time
	for _, ts := range window.timestamps {
		if ts.After(windowStart) {
			count++
			if oldestTime.IsZero() || ts.Before(oldestTime) {
				oldestTime = ts
			}
		}
	}

	remaining := rl.limit - count
	if remaining < 0 {
		remaining = 0
	}

	return &RateLimitInfo{
		Key:          key,
		Limit:        rl.limit,
		Remaining:    remaining,
		WindowStart:  windowStart,
		ResetAt:      now.Add(rl.windowSize),
		TotalRequests: int64(count),
	}
}

// Reset resets the rate limit for a key
func (rl *SlidingWindowRateLimiter) Reset(key string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.windows, key)
}

// LeakyBucket implements leaky bucket rate limiting
type LeakyBucket struct {
	capacity   int
	available  int
	rate       time.Duration // time per request
	lastLeak   time.Time
	mu         sync.Mutex
}

// NewLeakyBucket creates a new leaky bucket rate limiter
func NewLeakyBucket(capacity int, rate time.Duration) *LeakyBucket {
	return &LeakyBucket{
		capacity:  capacity,
		available: capacity,
		rate:      rate,
		lastLeak:  time.Now(),
	}
}

// Allow checks if a request is allowed
func (lb *LeakyBucket) Allow() bool {
	return lb.AllowN(1)
}

// AllowN checks if n requests are allowed
func (lb *LeakyBucket) AllowN(n int) bool {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	// Leak requests based on elapsed time
	now := time.Now()
	elapsed := now.Sub(lb.lastLeak)
	leaked := int(elapsed / lb.rate)

	lb.available += leaked
	if lb.available > lb.capacity {
		lb.available = lb.capacity
	}
	lb.lastLeak = now

	// Check if we have enough capacity
	if lb.available >= n {
		lb.available -= n
		return true
	}

	return false
}

// RateLimiterType defines the type of rate limiter
type RateLimiterType string

const (
	RateLimiterTokenBucket    RateLimiterType = "token_bucket"
	RateLimiterFixedWindow    RateLimiterType = "fixed_window"
	RateLimiterSlidingWindow  RateLimiterType = "sliding_window"
	RateLimiterLeakyBucket    RateLimiterType = "leaky_bucket"
)

// RateLimiter interface for all rate limiter types
type RateLimiter interface {
	Allow(key string) bool
	AllowN(key string, n int) bool
	GetInfo(key string) *RateLimitInfo
	Reset(key string)
}

// UniversalRateLimiter provides a unified interface for rate limiting
type UniversalRateLimiter struct {
	typ          RateLimiterType
	tokenBucket  map[string]*TokenBucket
	fixedWindow  *FixedWindowRateLimiter
	slidingWindow *SlidingWindowRateLimiter
	leakyBucket  map[string]*LeakyBucket
	config       *RateLimitConfig
	mu           sync.RWMutex
}

// NewUniversalRateLimiter creates a new universal rate limiter
func NewUniversalRateLimiter(typ RateLimiterType, config *RateLimitConfig) (*UniversalRateLimiter, error) {
	if config == nil {
		return nil, ErrInvalidRateLimit
	}

	rl := &UniversalRateLimiter{
		typ:    typ,
		config: config,
	}

	switch typ {
	case RateLimiterTokenBucket:
		rl.tokenBucket = make(map[string]*TokenBucket)
	case RateLimiterFixedWindow:
		rl.fixedWindow = NewFixedWindowRateLimiter(config.BurstSize, config.WindowDuration)
	case RateLimiterSlidingWindow:
		rl.slidingWindow = NewSlidingWindowRateLimiter(config.BurstSize, config.WindowDuration)
	case RateLimiterLeakyBucket:
		rl.leakyBucket = make(map[string]*LeakyBucket)
	default:
		return nil, ErrInvalidRateLimit
	}

	return rl, nil
}

// Allow checks if a request is allowed for the given key
func (rl *UniversalRateLimiter) Allow(key string) bool {
	return rl.AllowN(key, 1)
}

// AllowN checks if n requests are allowed for the given key
func (rl *UniversalRateLimiter) AllowN(key string, n int) bool {
	switch rl.typ {
	case RateLimiterTokenBucket:
		rl.mu.Lock()
		tb, exists := rl.tokenBucket[key]
		if !exists {
			tb = NewTokenBucket(rl.config.RequestsPerSecond, rl.config.BurstSize)
			rl.tokenBucket[key] = tb
		}
		rl.mu.Unlock()
		return tb.AllowN(n)

	case RateLimiterFixedWindow:
		return rl.fixedWindow.AllowN(key, n)

	case RateLimiterSlidingWindow:
		return rl.slidingWindow.AllowN(key, n)

	case RateLimiterLeakyBucket:
		rl.mu.Lock()
		lb, exists := rl.leakyBucket[key]
		if !exists {
			lb = NewLeakyBucket(rl.config.BurstSize, time.Second/time.Duration(rl.config.RequestsPerSecond))
			rl.leakyBucket[key] = lb
		}
		rl.mu.Unlock()
		return lb.AllowN(n)

	default:
		return false
	}
}

// GetInfo returns rate limit info for a key
func (rl *UniversalRateLimiter) GetInfo(key string) *RateLimitInfo {
	switch rl.typ {
	case RateLimiterTokenBucket:
		rl.mu.RLock()
		tb, exists := rl.tokenBucket[key]
		rl.mu.RUnlock()
		if !exists {
			return &RateLimitInfo{
				Key:       key,
				Limit:     rl.config.BurstSize,
				Remaining: rl.config.BurstSize,
			}
		}
		tokens := int(tb.GetTokens())
		return &RateLimitInfo{
			Key:       key,
			Limit:     rl.config.BurstSize,
			Remaining: tokens,
		}

	case RateLimiterFixedWindow:
		return rl.fixedWindow.GetInfo(key)

	case RateLimiterSlidingWindow:
		return rl.slidingWindow.GetInfo(key)

	default:
		return &RateLimitInfo{
			Key:   key,
			Limit: rl.config.BurstSize,
		}
	}
}

// Reset resets the rate limit for a key
func (rl *UniversalRateLimiter) Reset(key string) {
	switch rl.typ {
	case RateLimiterTokenBucket:
		rl.mu.Lock()
		delete(rl.tokenBucket, key)
		rl.mu.Unlock()

	case RateLimiterFixedWindow:
		rl.fixedWindow.Reset(key)

	case RateLimiterSlidingWindow:
		rl.slidingWindow.Reset(key)

	case RateLimiterLeakyBucket:
		rl.mu.Lock()
		delete(rl.leakyBucket, key)
		rl.mu.Unlock()
	}
}

// RateLimitMiddleware provides HTTP middleware for rate limiting
type RateLimitMiddleware struct {
	limiter RateLimiter
	keyFunc KeyFunc
	handler LimitHandler
}

// NewRateLimitMiddleware creates a new rate limiting middleware
func NewRateLimitMiddleware(limiter RateLimiter, keyFunc KeyFunc, handler LimitHandler) *RateLimitMiddleware {
	return &RateLimitMiddleware{
		limiter: limiter,
		keyFunc: keyFunc,
		handler: handler,
	}
}

// Check checks if a request is allowed
func (m *RateLimitMiddleware) Check(ctx context.Context) (*RateLimitInfo, error) {
	key := m.keyFunc(ctx)
	if key == "" {
		key = "default"
	}

	if !m.limiter.Allow(key) {
		info := m.limiter.GetInfo(key)
		if m.handler != nil {
			m.handler(ctx, key, info)
		}
		return info, ErrRateLimitExceeded
	}

	return m.limiter.GetInfo(key), nil
}

// DefaultKeyFunc extracts the client IP as the rate limit key
func DefaultKeyFunc(ctx context.Context) string {
	if ip, ok := ctx.Value("client_ip").(string); ok {
		return ip
	}
	return "unknown"
}

// CompositeKeyFunc creates a key from multiple values
func CompositeKeyFunc(keys ...string) KeyFunc {
	return func(ctx context.Context) string {
		result := ""
		for _, key := range keys {
			var val string
			switch key {
			case "ip":
				if ip, ok := ctx.Value("client_ip").(string); ok {
					val = ip
				}
			case "user_id":
				if userID, ok := ctx.Value("user_id").(string); ok {
					val = userID
				}
			case "api_key":
				if apiKey, ok := ctx.Value("api_key").(string); ok {
					val = apiKey
				}
			default:
				if v, ok := ctx.Value(key).(string); ok {
					val = v
				}
			}
			if val != "" {
				if result != "" {
					result += ":"
				}
				result += val
			}
		}
		return result
	}
}

// RateLimitService provides rate limiting with multiple named limiters
type RateLimitService struct {
	limiters map[string]RateLimiter
	configs  map[string]*RateLimitConfig
	mu       sync.RWMutex
}

// NewRateLimitService creates a new rate limit service
func NewRateLimitService() *RateLimitService {
	return &RateLimitService{
		limiters: make(map[string]RateLimiter),
		configs:  make(map[string]*RateLimitConfig),
	}
}

// RegisterLimiter registers a named rate limiter
func (s *RateLimitService) RegisterLimiter(name string, config *RateLimitConfig) error {
	if config == nil || config.BurstSize <= 0 {
		return ErrInvalidRateLimit
	}

	limiter, err := NewUniversalRateLimiter(RateLimiterSlidingWindow, config)
	if err != nil {
		return err
	}

	s.mu.Lock()
	s.limiters[name] = limiter
	s.configs[name] = config
	s.mu.Unlock()

	return nil
}

// Allow checks if a request is allowed for a named limiter
func (s *RateLimitService) Allow(name, key string) bool {
	s.mu.RLock()
	limiter, exists := s.limiters[name]
	s.mu.RUnlock()

	if !exists {
		return true // No limiter = allow all
	}

	return limiter.Allow(key)
}

// AllowN checks if n requests are allowed for a named limiter
func (s *RateLimitService) AllowN(name, key string, n int) bool {
	s.mu.RLock()
	limiter, exists := s.limiters[name]
	s.mu.RUnlock()

	if !exists {
		return true
	}

	return limiter.AllowN(key, n)
}

// GetInfo returns rate limit info for a named limiter
func (s *RateLimitService) GetInfo(name, key string) (*RateLimitInfo, error) {
	s.mu.RLock()
	limiter, exists := s.limiters[name]
	s.mu.RUnlock()

	if !exists {
		return nil, ErrRateLimiterNotFound
	}

	return limiter.GetInfo(key), nil
}

// Reset resets a rate limiter for a key
func (s *RateLimitService) Reset(name, key string) error {
	s.mu.RLock()
	limiter, exists := s.limiters[name]
	s.mu.RUnlock()

	if !exists {
		return ErrRateLimiterNotFound
	}

	limiter.Reset(key)
	return nil
}

// RemoveLimiter removes a named rate limiter
func (s *RateLimitService) RemoveLimiter(name string) {
	s.mu.Lock()
	delete(s.limiters, name)
	delete(s.configs, name)
	s.mu.Unlock()
}

// ListLimiters lists all registered limiters
func (s *RateLimitService) ListLimiters() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	names := make([]string, 0, len(s.limiters))
	for name := range s.limiters {
		names = append(names, name)
	}
	return names
}

// CleanupRoutine starts a background cleanup routine
func (s *RateLimitService) CleanupRoutine(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.mu.RLock()
			for _, limiter := range s.limiters {
				if ul, ok := limiter.(*UniversalRateLimiter); ok {
					if ul.fixedWindow != nil {
						ul.fixedWindow.Cleanup()
					}
					if ul.slidingWindow != nil {
						// Sliding window cleanup happens automatically
					}
				}
			}
			s.mu.RUnlock()
		}
	}
}

// RateLimit presets for common use cases
var (
	// StrictRateLimit: 10 requests per second
	StrictRateLimit = &RateLimitConfig{
		RequestsPerSecond: 10,
		BurstSize:         20,
		WindowDuration:    time.Second,
	}

	// StandardRateLimit: 100 requests per second
	StandardRateLimit = &RateLimitConfig{
		RequestsPerSecond: 100,
		BurstSize:         200,
		WindowDuration:    time.Second,
	}

	// RelaxedRateLimit: 1000 requests per second
	RelaxedRateLimit = &RateLimitConfig{
		RequestsPerSecond: 1000,
		BurstSize:         2000,
		WindowDuration:    time.Second,
	}

	// APIRateLimit: 60 requests per minute
	APIRateLimit = &RateLimitConfig{
		RequestsPerSecond: 1,
		BurstSize:         60,
		WindowDuration:    time.Minute,
	}

	// AuthRateLimit: 5 requests per minute (for login attempts)
	AuthRateLimit = &RateLimitConfig{
		RequestsPerSecond: 0.1,
		BurstSize:         5,
		WindowDuration:    time.Minute,
	}
)
