package health

import (
    "context"
    "github.com/go-redis/redis/v8"
    "go.mongodb.org/mongo-driver/mongo"
    "go.mongodb.org/mongo-driver/mongo/readpref"
    "runtime"
)

// DatabaseCheck creates a health check for MongoDB
func DatabaseCheck(db *mongo.Client) func(context.Context) Check {
    return func(ctx context.Context) Check {
        check := Check{
            Name:   "database",
            Status: StatusUp,
            Details: map[string]interface{}{
                "type": "mongodb",
            },
        }

        if err := db.Ping(ctx, readpref.Primary()); err != nil {
            check.Status = StatusDown
            check.Error = err.Error()
        }

        return check
    }
}

// RedisCheck creates a health check for Redis
func RedisCheck(client *redis.Client) func(context.Context) Check {
    return func(ctx context.Context) Check {
        check := Check{
            Name:   "redis",
            Status: StatusUp,
            Details: map[string]interface{}{
                "type": "redis",
            },
        }

        if err := client.Ping(ctx).Err(); err != nil {
            check.Status = StatusDown
            check.Error = err.Error()
        }

        return check
    }
}

// SystemCheck creates a health check for system resources
func SystemCheck() func(context.Context) Check {
    return func(ctx context.Context) Check {
        var m runtime.MemStats
        runtime.ReadMemStats(&m)

        check := Check{
            Name:   "system",
            Status: StatusUp,
            Details: map[string]interface{}{
                "goroutines": runtime.NumGoroutine(),
                "memory": map[string]interface{}{
                    "alloc":      m.Alloc,
                    "totalAlloc": m.TotalAlloc,
                    "sys":        m.Sys,
                    "numGC":      m.NumGC,
                },
            },
        }

        // Set status to DEGRADED if memory usage is high
        if m.Sys > 1<<30 { // 1GB
            check.Status = StatusDegraded
            check.Details["warning"] = "High memory usage"
        }

        return check
    }
} 