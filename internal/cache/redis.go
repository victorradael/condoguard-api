package cache

import (
    "context"
    "encoding/json"
    "time"
    "github.com/go-redis/redis/v8"
    "github.com/victorradael/condoguard/internal/config"
)

type RedisCache struct {
    client *redis.Client
    ctx    context.Context
}

func NewRedisCache() *RedisCache {
    client := redis.NewClient(&redis.Options{
        Addr:     config.GetEnv("REDIS_ADDR", "localhost:6379"),
        Password: config.GetEnv("REDIS_PASSWORD", ""),
        DB:       0,
    })

    return &RedisCache{
        client: client,
        ctx:    context.Background(),
    }
}

func (c *RedisCache) Set(key string, value interface{}, expiration time.Duration) error {
    json, err := json.Marshal(value)
    if err != nil {
        return err
    }

    return c.client.Set(c.ctx, key, json, expiration).Err()
}

func (c *RedisCache) Get(key string, dest interface{}) error {
    val, err := c.client.Get(c.ctx, key).Result()
    if err != nil {
        return err
    }

    return json.Unmarshal([]byte(val), dest)
}

func (c *RedisCache) Delete(key string) error {
    return c.client.Del(c.ctx, key).Err()
}

func (c *RedisCache) Clear() error {
    return c.client.FlushAll(c.ctx).Err()
} 