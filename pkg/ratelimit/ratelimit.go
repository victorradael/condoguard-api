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

// Limiter tracks attempts for a key. Every attempt is reserved up front and
// counts as a failure unless it is released or the key is reset, so concurrent
// attempts can't all pass the check before any of them is counted.
type Limiter interface {
	// Reserve atomically records one attempt for key. If the key already
	// reached the configured maximum, nothing is recorded and Reserve reports
	// false with how long the lockout lasts.
	Reserve(ctx context.Context, key string) (bool, time.Duration, error)
	// Release gives back an attempt reserved for key that should not count
	// (e.g. the request was rejected before checking credentials).
	Release(ctx context.Context, key string) error
	// Reset clears the attempts of key (e.g. after a successful login).
	Reset(ctx context.Context, key string) error
}

// ── Redis ─────────────────────────────────────────────────────────────────────

// reserveScript checks and increments the counter in one step, starting the
// window on the first attempt so a key can never be left without a TTL.
// Returns {1, 0} when reserved, {0, pttl} when locked out.
var reserveScript = redis.NewScript(`
local n = tonumber(redis.call("GET", KEYS[1]) or "0")
if n >= tonumber(ARGV[2]) then
  return {0, redis.call("PTTL", KEYS[1])}
end
n = redis.call("INCR", KEYS[1])
if n == 1 then
  redis.call("PEXPIRE", KEYS[1], ARGV[1])
end
return {1, 0}
`)

// releaseScript decrements an existing counter, keeping its TTL.
var releaseScript = redis.NewScript(`
local n = tonumber(redis.call("GET", KEYS[1]) or "0")
if n > 1 then
  return redis.call("DECR", KEYS[1])
end
redis.call("DEL", KEYS[1])
return 0
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

func (l *redisLimiter) Reserve(ctx context.Context, key string) (bool, time.Duration, error) {
	res, err := reserveScript.Run(ctx, l.client, []string{l.prefix + key}, l.window.Milliseconds(), l.max).Int64Slice()
	if err != nil {
		return false, 0, err
	}
	if len(res) == 2 && res[0] == 1 {
		return true, 0, nil
	}
	ttl := l.window
	if len(res) == 2 && res[1] > 0 {
		ttl = time.Duration(res[1]) * time.Millisecond
	}
	return false, ttl, nil
}

func (l *redisLimiter) Release(ctx context.Context, key string) error {
	return releaseScript.Run(ctx, l.client, []string{l.prefix + key}).Err()
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

func (l *memoryLimiter) Reserve(_ context.Context, key string) (bool, time.Duration, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.live(key)
	if !ok {
		e = entry{expires: l.now().Add(l.window)}
	}
	if e.count >= l.max {
		return false, e.expires.Sub(l.now()), nil
	}
	e.count++
	l.entries[key] = e
	return true, 0, nil
}

func (l *memoryLimiter) Release(_ context.Context, key string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.live(key)
	if !ok {
		return nil
	}
	if e.count <= 1 {
		delete(l.entries, key)
		return nil
	}
	e.count--
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
