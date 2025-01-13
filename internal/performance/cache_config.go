package performance

import (
    "time"
    "github.com/victorradael/condoguard/internal/cache"
)

// CacheConfig defines caching strategies for different resources
type CacheConfig struct {
    // Default cache duration
    DefaultTTL time.Duration

    // Resource-specific cache durations
    ResourceTTL map[string]time.Duration

    // Cache implementation
    Cache cache.Cache
}

// NewDefaultCacheConfig returns a CacheConfig with default settings
func NewDefaultCacheConfig() *CacheConfig {
    return &CacheConfig{
        DefaultTTL: 5 * time.Minute,
        ResourceTTL: map[string]time.Duration{
            "users":         15 * time.Minute,
            "residents":     10 * time.Minute,
            "notifications": 1 * time.Minute,
            "expenses":      5 * time.Minute,
        },
        Cache: cache.NewRedisCache(),
    }
}

// GetCacheDuration returns the cache duration for a specific resource
func (c *CacheConfig) GetCacheDuration(resource string) time.Duration {
    if duration, ok := c.ResourceTTL[resource]; ok {
        return duration
    }
    return c.DefaultTTL
} 