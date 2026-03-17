package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

// CacheConfig holds Redis cache configuration
type CacheConfig struct {
	Host     string
	Port     int
	Password string
	DB       int
	PoolSize int
}

// Cache represents the Redis cache client
type Cache struct {
	config *CacheConfig
	client *redis.Client
}

// NewCache creates a new cache instance
func NewCache(cfg *CacheConfig) *Cache {
	return &Cache{
		config: cfg,
	}
}

// Connect establishes a connection to Redis
func (c *Cache) Connect(ctx context.Context) error {
	client := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", c.config.Host, c.config.Port),
		Password: c.config.Password,
		DB:       c.config.DB,
		PoolSize: c.config.PoolSize,
	})

	// Test the connection
	if err := client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("failed to connect to Redis: %w", err)
	}

	c.client = client
	log.Printf("Connected to Redis cache: %s:%d DB:%d", c.config.Host, c.config.Port, c.config.DB)

	return nil
}

// Close closes the Redis connection
func (c *Cache) Close() error {
	if c.client != nil {
		return c.client.Close()
	}
	return nil
}

// HealthCheck checks if Redis connection is healthy
func (c *Cache) HealthCheck(ctx context.Context) error {
	if c.client == nil {
		return fmt.Errorf("cache connection not initialized")
	}

	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	if err := c.client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("cache health check failed: %w", err)
	}

	return nil
}

// Get retrieves a value from cache
func (c *Cache) Get(ctx context.Context, key string) (string, error) {
	if c.client == nil {
		return "", fmt.Errorf("cache not initialized")
	}

	val, err := c.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", fmt.Errorf("key not found: %s", key)
	}
	if err != nil {
		return "", fmt.Errorf("failed to get from cache: %w", err)
	}

	return val, nil
}

// GetJSON retrieves a JSON value from cache and unmarshals it
func (c *Cache) GetJSON(ctx context.Context, key string, dest interface{}) error {
	val, err := c.Get(ctx, key)
	if err != nil {
		return err
	}

	if err := json.Unmarshal([]byte(val), dest); err != nil {
		return fmt.Errorf("failed to unmarshal JSON: %w", err)
	}

	return nil
}

// Set stores a value in cache
func (c *Cache) Set(ctx context.Context, key string, value string, expiration time.Duration) error {
	if c.client == nil {
		return fmt.Errorf("cache not initialized")
	}

	if err := c.client.Set(ctx, key, value, expiration).Err(); err != nil {
		return fmt.Errorf("failed to set cache: %w", err)
	}

	return nil
}

// SetJSON stores a JSON value in cache
func (c *Cache) SetJSON(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	return c.Set(ctx, key, string(data), expiration)
}

// Delete removes a key from cache
func (c *Cache) Delete(ctx context.Context, key string) error {
	if c.client == nil {
		return fmt.Errorf("cache not initialized")
	}

	if err := c.client.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("failed to delete from cache: %w", err)
	}

	return nil
}

// DeletePattern removes all keys matching a pattern
func (c *Cache) DeletePattern(ctx context.Context, pattern string) error {
	if c.client == nil {
		return fmt.Errorf("cache not initialized")
	}

	iter := c.client.Scan(ctx, 0, pattern, 0).Iterator()
	for iter.Next(ctx) {
		if err := c.client.Del(ctx, iter.Val()).Err(); err != nil {
			log.Printf("Failed to delete key %s: %v", iter.Val(), err)
		}
	}

	if err := iter.Err(); err != nil {
		return fmt.Errorf("failed to scan keys: %w", err)
	}

	return nil
}

// Exists checks if a key exists in cache
func (c *Cache) Exists(ctx context.Context, key string) (bool, error) {
	if c.client == nil {
		return false, fmt.Errorf("cache not initialized")
	}

	count, err := c.client.Exists(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("failed to check existence: %w", err)
	}

	return count > 0, nil
}

// Expire sets expiration time for a key
func (c *Cache) Expire(ctx context.Context, key string, expiration time.Duration) error {
	if c.client == nil {
		return fmt.Errorf("cache not initialized")
	}

	if err := c.client.Expire(ctx, key, expiration).Err(); err != nil {
		return fmt.Errorf("failed to set expiration: %w", err)
	}

	return nil
}

// TTL returns the remaining time to live of a key
func (c *Cache) TTL(ctx context.Context, key string) (time.Duration, error) {
	if c.client == nil {
		return 0, fmt.Errorf("cache not initialized")
	}

	ttl, err := c.client.TTL(ctx, key).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to get TTL: %w", err)
	}

	return ttl, nil
}

// Increment increments a key's value
func (c *Cache) Increment(ctx context.Context, key string) (int64, error) {
	if c.client == nil {
		return 0, fmt.Errorf("cache not initialized")
	}

	val, err := c.client.Incr(ctx, key).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to increment: %w", err)
	}

	return val, nil
}

// IncrementBy increments a key's value by a specific amount
func (c *Cache) IncrementBy(ctx context.Context, key string, value int64) (int64, error) {
	if c.client == nil {
		return 0, fmt.Errorf("cache not initialized")
	}

	val, err := c.client.IncrBy(ctx, key, value).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to increment by: %w", err)
	}

	return val, nil
}

// Decrement decrements a key's value
func (c *Cache) Decrement(ctx context.Context, key string) (int64, error) {
	if c.client == nil {
		return 0, fmt.Errorf("cache not initialized")
	}

	val, err := c.client.Decr(ctx, key).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to decrement: %w", err)
	}

	return val, nil
}

// DecrementBy decrements a key's value by a specific amount
func (c *Cache) DecrementBy(ctx context.Context, key string, value int64) (int64, error) {
	if c.client == nil {
		return 0, fmt.Errorf("cache not initialized")
	}

	val, err := c.client.DecrBy(ctx, key, value).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to decrement by: %w", err)
	}

	return val, nil
}

// HashSet sets a field in a hash
func (c *Cache) HashSet(ctx context.Context, key string, field string, value string) error {
	if c.client == nil {
		return fmt.Errorf("cache not initialized")
	}

	if err := c.client.HSet(ctx, key, field, value).Err(); err != nil {
		return fmt.Errorf("failed to set hash field: %w", err)
	}

	return nil
}

// HashGet gets a field from a hash
func (c *Cache) HashGet(ctx context.Context, key string, field string) (string, error) {
	if c.client == nil {
		return "", fmt.Errorf("cache not initialized")
	}

	val, err := c.client.HGet(ctx, key, field).Result()
	if err == redis.Nil {
		return "", fmt.Errorf("field not found: %s", field)
	}
	if err != nil {
		return "", fmt.Errorf("failed to get hash field: %w", err)
	}

	return val, nil
}

// HashGetAll gets all fields from a hash
func (c *Cache) HashGetAll(ctx context.Context, key string) (map[string]string, error) {
	if c.client == nil {
		return nil, fmt.Errorf("cache not initialized")
	}

	val, err := c.client.HGetAll(ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get all hash fields: %w", err)
	}

	return val, nil
}

// HashDelete deletes a field from a hash
func (c *Cache) HashDelete(ctx context.Context, key string, field string) error {
	if c.client == nil {
		return fmt.Errorf("cache not initialized")
	}

	if err := c.client.HDel(ctx, key, field).Err(); err != nil {
		return fmt.Errorf("failed to delete hash field: %w", err)
	}

	return nil
}

// ListPush pushes a value to a list
func (c *Cache) ListPush(ctx context.Context, key string, value string) error {
	if c.client == nil {
		return fmt.Errorf("cache not initialized")
	}

	if err := c.client.LPush(ctx, key, value).Err(); err != nil {
		return fmt.Errorf("failed to push to list: %w", err)
	}

	return nil
}

// ListPop pops a value from a list
func (c *Cache) ListPop(ctx context.Context, key string) (string, error) {
	if c.client == nil {
		return "", fmt.Errorf("cache not initialized")
	}

	val, err := c.client.RPop(ctx, key).Result()
	if err == redis.Nil {
		return "", fmt.Errorf("list is empty")
	}
	if err != nil {
		return "", fmt.Errorf("failed to pop from list: %w", err)
	}

	return val, nil
}

// ListRange gets a range of elements from a list
func (c *Cache) ListRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	if c.client == nil {
		return nil, fmt.Errorf("cache not initialized")
	}

	val, err := c.client.LRange(ctx, key, start, stop).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get list range: %w", err)
	}

	return val, nil
}

// ListLength gets the length of a list
func (c *Cache) ListLength(ctx context.Context, key string) (int64, error) {
	if c.client == nil {
		return 0, fmt.Errorf("cache not initialized")
	}

	val, err := c.client.LLen(ctx, key).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to get list length: %w", err)
	}

	return val, nil
}

// SetAdd adds a member to a set
func (c *Cache) SetAdd(ctx context.Context, key string, member string) error {
	if c.client == nil {
		return fmt.Errorf("cache not initialized")
	}

	if err := c.client.SAdd(ctx, key, member).Err(); err != nil {
		return fmt.Errorf("failed to add to set: %w", err)
	}

	return nil
}

// SetMembers gets all members of a set
func (c *Cache) SetMembers(ctx context.Context, key string) ([]string, error) {
	if c.client == nil {
		return nil, fmt.Errorf("cache not initialized")
	}

	val, err := c.client.SMembers(ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get set members: %w", err)
	}

	return val, nil
}

// SetIsMember checks if a value is a member of a set
func (c *Cache) SetIsMember(ctx context.Context, key string, member string) (bool, error) {
	if c.client == nil {
		return false, fmt.Errorf("cache not initialized")
	}

	val, err := c.client.SIsMember(ctx, key, member).Result()
	if err != nil {
		return false, fmt.Errorf("failed to check set membership: %w", err)
	}

	return val, nil
}

// SetRemove removes a member from a set
func (c *Cache) SetRemove(ctx context.Context, key string, member string) error {
	if c.client == nil {
		return fmt.Errorf("cache not initialized")
	}

	if err := c.client.SRem(ctx, key, member).Err(); err != nil {
		return fmt.Errorf("failed to remove from set: %w", err)
	}

	return nil
}

// FlushDB flushes the current database
func (c *Cache) FlushDB(ctx context.Context) error {
	if c.client == nil {
		return fmt.Errorf("cache not initialized")
	}

	if err := c.client.FlushDB(ctx).Err(); err != nil {
		return fmt.Errorf("failed to flush database: %w", err)
	}

	return nil
}

// Info gets Redis server information
func (c *Cache) Info(ctx context.Context, section ...string) (string, error) {
	if c.client == nil {
		return "", fmt.Errorf("cache not initialized")
	}

	info, err := c.client.Info(ctx, section...).Result()
	if err != nil {
		return "", fmt.Errorf("failed to get Redis info: %w", err)
	}

	return info, nil
}

// DBSize gets the number of keys in the database
func (c *Cache) DBSize(ctx context.Context) (int64, error) {
	if c.client == nil {
		return 0, fmt.Errorf("cache not initialized")
	}

	size, err := c.client.DBSize(ctx).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to get database size: %w", err)
	}

	return size, nil
}

// GetClient returns the underlying Redis client
func (c *Cache) GetClient() *redis.Client {
	return c.client
}

// CacheKeyBuilder helps build cache keys with consistent formatting
type CacheKeyBuilder struct {
	prefix string
}

// NewCacheKeyBuilder creates a new cache key builder
func NewCacheKeyBuilder(prefix string) *CacheKeyBuilder {
	return &CacheKeyBuilder{prefix: prefix}
}

// Build builds a cache key from parts
func (ckb *CacheKeyBuilder) Build(parts ...string) string {
	key := ckb.prefix
	for _, part := range parts {
		key += ":" + part
	}
	return key
}

// ModelKey returns a cache key for a model
func (ckb *CacheKeyBuilder) ModelKey(modelID string) string {
	return ckb.Build("model", modelID)
}

// ModelListKey returns a cache key for the model list
func (ckb *CacheKeyBuilder) ModelListKey() string {
	return ckb.Build("models", "list")
}

// ConfigKey returns a cache key for configuration
func (ckb *CacheKeyBuilder) ConfigKey(key string) string {
	return ckb.Build("config", key)
}

// SessionKey returns a cache key for a session
func (ckb *CacheKeyBuilder) SessionKey(sessionID string) string {
	return ckb.Build("session", sessionID)
}

// MetricsKey returns a cache key for metrics
func (ckb *CacheKeyBuilder) MetricsKey(metricType string) string {
	return ckb.Build("metrics", metricType)
}

// RateLimitKey returns a cache key for rate limiting
func (ckb *CacheKeyBuilder) RateLimitKey(identifier string) string {
	return ckb.Build("ratelimit", identifier)
}
