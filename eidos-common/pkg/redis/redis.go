// Package redis provides a comprehensive Redis client wrapper with support for:
// - Connection pool management
// - Cluster and Sentinel mode
// - Pipeline batch operations
// - Pub/Sub messaging
// - Lua script execution
// - Distributed locks
// - Rate limiting
// - Balance management with atomic operations
package redis

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// Global client instance (optional singleton pattern)
var defaultClient *Client

// SetDefaultClient sets the default client instance
func SetDefaultClient(c *Client) {
	defaultClient = c
}

// GetDefaultClient returns the default client instance
func GetDefaultClient() *Client {
	return defaultClient
}

// MustGetDefaultClient returns the default client or panics if not set
func MustGetDefaultClient() *Client {
	if defaultClient == nil {
		panic("redis: default client not initialized")
	}
	return defaultClient
}

// NewClientFromConfig creates a client from config.RedisConfig
func NewClientFromConfig(cfg *Config) (*Client, error) {
	return NewClient(cfg)
}

// NewSimpleClient creates a simple single-node client
func NewSimpleClient(addr, password string, db int) (*Client, error) {
	cfg := DefaultConfig()
	cfg.Mode = ModeSingle
	cfg.Addresses = []string{addr}
	cfg.Password = password
	cfg.DB = db
	return NewClient(cfg)
}

// NewClusterClient creates a cluster client
func NewClusterClient(addrs []string, password string) (*Client, error) {
	cfg := DefaultConfig()
	cfg.Mode = ModeCluster
	cfg.Addresses = addrs
	cfg.Password = password
	return NewClient(cfg)
}

// NewSentinelClient creates a sentinel client
func NewSentinelClient(sentinelAddrs []string, masterName, password string, db int) (*Client, error) {
	cfg := DefaultConfig()
	cfg.Mode = ModeSentinel
	cfg.Addresses = sentinelAddrs
	cfg.MasterName = masterName
	cfg.Password = password
	cfg.DB = db
	return NewClient(cfg)
}

// Helper functions for common operations

// IsNilError checks if the error is a nil error (key not found)
func IsNilError(err error) bool {
	return err == redis.Nil
}

// IsExist checks if the error means the key already exists
func IsExist(err error) bool {
	// Redis doesn't have a specific "already exists" error
	// This is typically handled by the result of SET NX
	return false
}

// WithTimeout creates a context with the specified timeout
func WithTimeout(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, timeout)
}

// Key builders for common patterns

// KeyBuilder builds Redis keys with a prefix
type KeyBuilder struct {
	prefix string
}

// NewKeyBuilder creates a new key builder
func NewKeyBuilder(prefix string) *KeyBuilder {
	return &KeyBuilder{prefix: prefix}
}

// Build builds a key with the given parts
func (b *KeyBuilder) Build(parts ...string) string {
	key := b.prefix
	for _, part := range parts {
		if key != "" {
			key += ":"
		}
		key += part
	}
	return key
}

// WithSuffix returns a new builder with an additional suffix
func (b *KeyBuilder) WithSuffix(suffix string) *KeyBuilder {
	return &KeyBuilder{prefix: b.prefix + ":" + suffix}
}

// Cache helper

// CacheOptions cache options
type CacheOptions struct {
	TTL           time.Duration
	NullValueTTL  time.Duration // TTL for caching null values (防止缓存穿透)
	EnableNullCache bool
}

// DefaultCacheOptions returns default cache options
func DefaultCacheOptions() *CacheOptions {
	return &CacheOptions{
		TTL:            5 * time.Minute,
		NullValueTTL:   1 * time.Minute,
		EnableNullCache: true,
	}
}

// CacheHelper provides common caching patterns
type CacheHelper struct {
	client  *Client
	options *CacheOptions
}

// NewCacheHelper creates a cache helper
func NewCacheHelper(client *Client, opts *CacheOptions) *CacheHelper {
	if opts == nil {
		opts = DefaultCacheOptions()
	}
	return &CacheHelper{
		client:  client,
		options: opts,
	}
}

// GetOrSet gets a value from cache or sets it using the loader function
func (h *CacheHelper) GetOrSet(ctx context.Context, key string, loader func() (string, error)) (string, error) {
	// Try to get from cache
	val, err := h.client.Get(ctx, key)
	if err == nil {
		if h.options.EnableNullCache && val == "__NULL__" {
			return "", nil // Cached null value
		}
		return val, nil
	}

	if !IsNilError(err) {
		return "", err
	}

	// Load from source
	val, err = loader()
	if err != nil {
		return "", err
	}

	// Cache the result
	if val == "" && h.options.EnableNullCache {
		// Cache null value to prevent cache penetration
		h.client.Set(ctx, key, "__NULL__", h.options.NullValueTTL)
	} else if val != "" {
		h.client.Set(ctx, key, val, h.options.TTL)
	}

	return val, nil
}

// Delete deletes a cached value
func (h *CacheHelper) Delete(ctx context.Context, keys ...string) error {
	_, err := h.client.Del(ctx, keys...)
	return err
}

// SetWithTTL sets a value with a specific TTL
func (h *CacheHelper) SetWithTTL(ctx context.Context, key, value string, ttl time.Duration) error {
	return h.client.Set(ctx, key, value, ttl)
}

// Health check helper

// HealthChecker provides health check functionality
type HealthChecker struct {
	client *Client
}

// NewHealthChecker creates a health checker
func NewHealthChecker(client *Client) *HealthChecker {
	return &HealthChecker{client: client}
}

// Check performs a health check
func (h *HealthChecker) Check(ctx context.Context) error {
	return h.client.Ping(ctx)
}

// CheckWithTimeout performs a health check with timeout
func (h *HealthChecker) CheckWithTimeout(timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return h.Check(ctx)
}

// IsHealthy returns true if the client is healthy
func (h *HealthChecker) IsHealthy() bool {
	return h.client.IsHealthy()
}

// GetMetrics returns the client metrics
func (h *HealthChecker) GetMetrics() *Metrics {
	return h.client.GetMetrics()
}
