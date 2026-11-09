package ratelimit

import (
	"context"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// exercise runs the shared contract against a Limiter (max 3 attempts).
// advance moves the limiter's clock forward.
func exercise(t *testing.T, l Limiter, advance func(time.Duration)) {
	t.Helper()
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		ok, _, err := l.Reserve(ctx, "k")
		if err != nil || !ok {
			t.Fatalf("attempt %d must be reserved (err %v)", i+1, err)
		}
	}
	ok, retry, err := l.Reserve(ctx, "k")
	if err != nil || ok {
		t.Fatalf("4th attempt must be refused (err %v)", err)
	}
	if retry <= 0 || retry > time.Minute {
		t.Errorf("retry-after must be within the window, got %v", retry)
	}
	if ok, _, _ := l.Reserve(ctx, "other"); !ok {
		t.Error("keys are independent")
	}

	// Release gives one attempt back.
	if err := l.Release(ctx, "k"); err != nil {
		t.Fatal(err)
	}
	if ok, _, _ := l.Reserve(ctx, "k"); !ok {
		t.Error("released attempt must be available again")
	}
	if ok, _, _ := l.Reserve(ctx, "k"); ok {
		t.Error("only one attempt was released")
	}
	// Releasing an unknown key is a no-op.
	if err := l.Release(ctx, "none"); err != nil {
		t.Fatal(err)
	}

	if err := l.Reset(ctx, "k"); err != nil {
		t.Fatal(err)
	}
	if ok, _, _ := l.Reserve(ctx, "k"); !ok {
		t.Error("reset must unblock")
	}

	for i := 0; i < 3; i++ {
		_, _, _ = l.Reserve(ctx, "w")
	}
	advance(time.Minute + time.Second)
	if ok, _, _ := l.Reserve(ctx, "w"); !ok {
		t.Error("window expiry must unblock")
	}
}

// concurrent checks that parallel attempts on one key never exceed the max.
func concurrent(t *testing.T, l Limiter, maxAttempts int) {
	t.Helper()
	var reserved atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if ok, _, err := l.Reserve(context.Background(), "race"); err == nil && ok {
				reserved.Add(1)
			}
		}()
	}
	wg.Wait()
	if got := reserved.Load(); got != int64(maxAttempts) {
		t.Errorf("200 parallel attempts: %d reserved, want %d", got, maxAttempts)
	}
}

func TestMemoryLimiter(t *testing.T) {
	now := time.Now()
	var mu sync.Mutex
	l := newMemory(3, time.Minute, func() time.Time { mu.Lock(); defer mu.Unlock(); return now })
	exercise(t, l, func(d time.Duration) { mu.Lock(); now = now.Add(d); mu.Unlock() })
	concurrent(t, NewMemory(5, time.Minute), 5)
}

func TestRedisLimiter_Miniredis(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	l := NewRedis(client, "test:", 3, time.Minute)
	exercise(t, l, mr.FastForward)
	concurrent(t, NewRedis(client, "test:c:", 5, time.Minute), 5)

	// The window starts at the first attempt and is not extended by later ones.
	ctx := context.Background()
	_, _, _ = l.Reserve(ctx, "ttl")
	mr.FastForward(30 * time.Second)
	_, _, _ = l.Reserve(ctx, "ttl")
	_ = l.Release(ctx, "ttl")
	if ttl := mr.TTL("test:ttl"); ttl <= 0 || ttl > 30*time.Second {
		t.Errorf("later attempts must not extend or drop the window, ttl %v", ttl)
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
	defer client.Del(ctx, prefix+"k", prefix+"race")

	for i := 0; i < 2; i++ {
		if ok, _, err := l.Reserve(ctx, "k"); err != nil || !ok {
			t.Fatalf("attempt %d must be reserved (err %v)", i+1, err)
		}
	}
	ok, retry, err := l.Reserve(ctx, "k")
	if err != nil || ok || retry <= 0 {
		t.Fatalf("expected refused with retry-after, got %v %v %v", ok, retry, err)
	}
	_ = l.Reset(ctx, "k")
	if ok, _, _ := l.Reserve(ctx, "k"); !ok {
		t.Error("reset must unblock")
	}
	concurrent(t, NewRedis(client, prefix, 5, time.Minute), 5)
}
