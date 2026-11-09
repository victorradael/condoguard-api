package middleware

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// Revocations is the list of access tokens that must stop working before they
// expire: one token (logout) or everything a user was issued up to a moment
// (password reset, role change, "sign out everywhere"). Entries only live as
// long as the tokens they cover could, so the list stays small.
type Revocations interface {
	// Revoke blocks one access token (by its jti) until it would have expired anyway.
	Revoke(ctx context.Context, jti string, until time.Time) error
	// RevokeUser blocks every access token of the user issued before at.
	// ttl is how long such tokens can still live (the access-token lifetime).
	RevokeUser(ctx context.Context, userID string, at time.Time, ttl time.Duration) error
	// IsRevoked reports whether the token was blocked by either rule.
	IsRevoked(ctx context.Context, jti, userID string, issuedAt time.Time) (bool, error)
}

// userMargin keeps the per-user entry slightly longer than the tokens it covers.
const userMargin = time.Minute

// ── Redis ─────────────────────────────────────────────────────────────────────

type redisRevocations struct {
	c      *redis.Client
	prefix string
}

// NewRedisRevocations stores the list in Redis under prefix.
func NewRedisRevocations(c *redis.Client, prefix string) Revocations {
	return &redisRevocations{c: c, prefix: prefix}
}

func (r *redisRevocations) Revoke(ctx context.Context, jti string, until time.Time) error {
	ttl := time.Until(until)
	if ttl <= 0 {
		return nil // already expired
	}
	return r.c.Set(ctx, r.prefix+"jti:"+jti, "1", ttl).Err()
}

func (r *redisRevocations) RevokeUser(ctx context.Context, userID string, at time.Time, ttl time.Duration) error {
	return r.c.Set(ctx, r.prefix+"user:"+userID, strconv.FormatInt(at.UnixMilli(), 10), ttl+userMargin).Err()
}

func (r *redisRevocations) IsRevoked(ctx context.Context, jti, userID string, issuedAt time.Time) (bool, error) {
	// One round trip for both keys.
	vals, err := r.c.MGet(ctx, r.prefix+"jti:"+jti, r.prefix+"user:"+userID).Result()
	if err != nil {
		return false, err
	}
	if vals[0] != nil {
		return true, nil
	}
	if s, ok := vals[1].(string); ok {
		ms, perr := strconv.ParseInt(s, 10, 64)
		if perr != nil {
			return true, nil // unreadable entry: fail closed
		}
		return issuedAt.UnixMilli() < ms, nil
	}
	return false, nil
}

// ── In memory (dev without Redis, tests) ──────────────────────────────────────

type memoryRevocations struct {
	mu    sync.Mutex
	jtis  map[string]time.Time
	users map[string]time.Time
}

// NewMemoryRevocations keeps the list in process memory (single instance only).
func NewMemoryRevocations() Revocations {
	return &memoryRevocations{jtis: map[string]time.Time{}, users: map[string]time.Time{}}
}

func (m *memoryRevocations) Revoke(_ context.Context, jti string, until time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	for k, t := range m.jtis { // forget what expired
		if t.Before(now) {
			delete(m.jtis, k)
		}
	}
	m.jtis[jti] = until
	return nil
}

func (m *memoryRevocations) RevokeUser(_ context.Context, userID string, at time.Time, _ time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.users[userID] = at
	return nil
}

func (m *memoryRevocations) IsRevoked(_ context.Context, jti, userID string, issuedAt time.Time) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if until, ok := m.jtis[jti]; ok && until.After(time.Now()) {
		return true, nil
	}
	if at, ok := m.users[userID]; ok && issuedAt.Before(at) {
		return true, nil
	}
	return false, nil
}
