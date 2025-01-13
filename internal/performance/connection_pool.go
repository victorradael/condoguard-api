package performance

import (
    "context"
    "time"
    "go.mongodb.org/mongo-driver/mongo"
    "go.mongodb.org/mongo-driver/mongo/options"
)

// ConnectionPoolConfig defines database connection pool settings
type ConnectionPoolConfig struct {
    MaxPoolSize     uint64
    MinPoolSize     uint64
    MaxConnIdleTime time.Duration
    MaxConnLifetime time.Duration
}

// NewDefaultConnectionPoolConfig returns connection pool configuration with optimal defaults
func NewDefaultConnectionPoolConfig() *ConnectionPoolConfig {
    return &ConnectionPoolConfig{
        MaxPoolSize:     100,
        MinPoolSize:     10,
        MaxConnIdleTime: 30 * time.Second,
        MaxConnLifetime: 5 * time.Minute,
    }
}

// ConfigureConnectionPool applies connection pool settings to MongoDB client options
func (c *ConnectionPoolConfig) ConfigureConnectionPool(opts *options.ClientOptions) *options.ClientOptions {
    return opts.
        SetMaxPoolSize(c.MaxPoolSize).
        SetMinPoolSize(c.MinPoolSize).
        SetMaxConnIdleTime(c.MaxConnIdleTime).
        SetMaxConnLifetime(c.MaxConnLifetime)
} 