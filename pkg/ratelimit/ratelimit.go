// Package ratelimit counts failed attempts per key inside a fixed window.
//
// The Redis implementation is the one used in deployments; the in-memory one
// keeps the API usable (and tests hermetic) when REDIS_URL is not set.
package ratelimit

import (
	"context"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// Limiter tracks failures for a key. A key is blocked once it reaches the
// configured number of failures, until its window expires or Reset is called.
type Limiter interface {
	// Blocked reports whether key is locked out and, if so, for how long.
	Blocked(ctx context.Context, key string) (bool, time.Duration, error)
	// Fail records one failure for key.
	Fail(ctx context.Context, key string) error
	// Reset clears the failures of key (e.g. after a successful login).
	Reset(ctx context.Context, key string) error
}

// ── Redis ─────────────────────────────────────────────────────────────────────

// failScript increments the counter and starts the window on the first failure,
// atomically, so a crash between INCR and EXPIRE can't leave a key without TTL.
var failScript = redis.NewScript(`
local n = redis.call("INCR", KEYS[1])
if n == 1 then
  redis.call("PEXPIRE", KEYS[1], ARGV[1])
end
return n
`)

type redisLimiter struct {
	client *redis.Client
	prefix string
	max    int64
	window time.Duration
}

// NewRedis returns a Limiter storing counters in Redis under prefix+key.
func NewRedis(client *redis.Client, prefix string, maxFailures int, window time.Duration) Limiter {
	return &redisLimiter{client: client, prefix: prefix, max: int64(maxFailures), window: window}
}

func (l *redisLimiter) Blocked(ctx context.Context, key string) (bool, time.Duration, error) {
	k := l.prefix + key
	n, err := l.client.Get(ctx, k).Int64()
	if err == redis.Nil {
		return false, 0, nil
	}
	if err != nil {
		return false, 0, err
	}
	if n < l.max {
		return false, 0, nil
	}
	ttl, err := l.client.PTTL(ctx, k).Result()
	if err != nil {
		return false, 0, err
	}
	if ttl <= 0 {
		ttl = l.window
	}
	return true, ttl, nil
}

func (l *redisLimiter) Fail(ctx context.Context, key string) error {
	return failScript.Run(ctx, l.client, []string{l.prefix + key}, l.window.Milliseconds()).Err()
}

func (l *redisLimiter) Reset(ctx context.Context, key string) error {
	return l.client.Del(ctx, l.prefix+key).Err()
}

// ── In-memory ─────────────────────────────────────────────────────────────────

type entry struct {
	count   int
	expires time.Time
}

type memoryLimiter struct {
	mu      sync.Mutex
	entries map[string]entry
	max     int
	window  time.Duration
	now     func() time.Time
}

// NewMemory returns a process-local Limiter (single instance only).
func NewMemory(maxFailures int, window time.Duration) Limiter {
	return newMemory(maxFailures, window, time.Now)
}

func newMemory(maxFailures int, window time.Duration, now func() time.Time) *memoryLimiter {
	return &memoryLimiter{entries: make(map[string]entry), max: maxFailures, window: window, now: now}
}

func (l *memoryLimiter) Blocked(_ context.Context, key string) (bool, time.Duration, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.live(key)
	if !ok || e.count < l.max {
		return false, 0, nil
	}
	return true, e.expires.Sub(l.now()), nil
}

func (l *memoryLimiter) Fail(_ context.Context, key string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.live(key)
	if !ok {
		e = entry{expires: l.now().Add(l.window)}
	}
	e.count++
	l.entries[key] = e
	return nil
}

func (l *memoryLimiter) Reset(_ context.Context, key string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.entries, key)
	return nil
}

// live returns the entry for key if its window has not expired. Caller holds mu.
func (l *memoryLimiter) live(key string) (entry, bool) {
	e, ok := l.entries[key]
	if !ok {
		return entry{}, false
	}
	if !l.now().Before(e.expires) {
		delete(l.entries, key)
		return entry{}, false
	}
	return e, true
}
