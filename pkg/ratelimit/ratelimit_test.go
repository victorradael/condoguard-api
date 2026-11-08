package ratelimit

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// exercise runs the shared contract against a Limiter (max 3 failures).
// advance moves the limiter's clock forward.
func exercise(t *testing.T, l Limiter, advance func(time.Duration)) {
	t.Helper()
	ctx := context.Background()

	for i := 0; i < 2; i++ {
		if err := l.Fail(ctx, "k"); err != nil {
			t.Fatal(err)
		}
	}
	if blocked, _, _ := l.Blocked(ctx, "k"); blocked {
		t.Fatal("2 failures must not block")
	}
	_ = l.Fail(ctx, "k")
	blocked, retry, err := l.Blocked(ctx, "k")
	if err != nil || !blocked {
		t.Fatalf("3 failures must block (err %v)", err)
	}
	if retry <= 0 || retry > time.Minute {
		t.Errorf("retry-after must be within the window, got %v", retry)
	}
	if blocked, _, _ := l.Blocked(ctx, "other"); blocked {
		t.Error("keys are independent")
	}

	if err := l.Reset(ctx, "k"); err != nil {
		t.Fatal(err)
	}
	if blocked, _, _ := l.Blocked(ctx, "k"); blocked {
		t.Error("reset must unblock")
	}

	for i := 0; i < 3; i++ {
		_ = l.Fail(ctx, "w")
	}
	advance(time.Minute + time.Second)
	if blocked, _, _ := l.Blocked(ctx, "w"); blocked {
		t.Error("window expiry must unblock")
	}
}

func TestMemoryLimiter(t *testing.T) {
	now := time.Now()
	l := newMemory(3, time.Minute, func() time.Time { return now })
	exercise(t, l, func(d time.Duration) { now = now.Add(d) })
}

func TestRedisLimiter_Miniredis(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	l := NewRedis(client, "test:", 3, time.Minute)
	exercise(t, l, mr.FastForward)

	// The window starts at the first failure and is not extended by later ones.
	ctx := context.Background()
	_ = l.Fail(ctx, "ttl")
	mr.FastForward(30 * time.Second)
	_ = l.Fail(ctx, "ttl")
	if ttl := mr.TTL("test:ttl"); ttl > 30*time.Second {
		t.Errorf("later failures must not extend the window, ttl %v", ttl)
	}
}

// Runs against a real Redis when TEST_REDIS_URL is set (CI service).
func TestRedisLimiter_Real(t *testing.T) {
	url := os.Getenv("TEST_REDIS_URL")
	if url == "" {
		t.Skip("TEST_REDIS_URL not set — skipping real Redis test")
	}
	opts, err := redis.ParseURL(url)
	if err != nil {
		t.Fatal(err)
	}
	client := redis.NewClient(opts)
	defer client.Close()

	prefix := "test:" + t.Name() + ":" + time.Now().Format(time.RFC3339Nano) + ":"
	l := NewRedis(client, prefix, 2, time.Minute)
	ctx := context.Background()
	defer client.Del(ctx, prefix+"k")

	_ = l.Fail(ctx, "k")
	if blocked, _, _ := l.Blocked(ctx, "k"); blocked {
		t.Fatal("1 failure must not block")
	}
	if err = l.Fail(ctx, "k"); err != nil {
		t.Fatal(err)
	}
	blocked, retry, err := l.Blocked(ctx, "k")
	if err != nil || !blocked || retry <= 0 {
		t.Fatalf("expected blocked with retry-after, got %v %v %v", blocked, retry, err)
	}
	_ = l.Reset(ctx, "k")
	if blocked, _, _ := l.Blocked(ctx, "k"); blocked {
		t.Error("reset must unblock")
	}
}
