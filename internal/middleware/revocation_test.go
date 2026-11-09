package middleware_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/victorradael/condoguard/api/internal/middleware"
	pkgjwt "github.com/victorradael/condoguard/api/pkg/jwt"
)

func backends(t *testing.T) map[string]middleware.Revocations {
	t.Helper()
	mr := miniredis.RunT(t)
	return map[string]middleware.Revocations{
		"redis":  middleware.NewRedisRevocations(redis.NewClient(&redis.Options{Addr: mr.Addr()}), "test:revoked:"),
		"memory": middleware.NewMemoryRevocations(),
	}
}

func TestRevocations_OneTokenAndWholeUser(t *testing.T) {
	ctx := context.Background()
	for name, r := range backends(t) {
		t.Run(name, func(t *testing.T) {
			now := time.Now()
			if gone, err := r.IsRevoked(ctx, "jti-1", "u1", now); err != nil || gone {
				t.Fatalf("fresh token: revoked=%v err=%v", gone, err)
			}
			if err := r.Revoke(ctx, "jti-1", now.Add(time.Minute)); err != nil {
				t.Fatal(err)
			}
			if gone, _ := r.IsRevoked(ctx, "jti-1", "u1", now); !gone {
				t.Error("jti-1 must be revoked")
			}
			if gone, _ := r.IsRevoked(ctx, "jti-2", "u1", now); gone {
				t.Error("another token of the same user must not be revoked by a single revoke")
			}
			// Expired entries cost nothing and block nothing.
			if err := r.Revoke(ctx, "old", now.Add(-time.Minute)); err != nil {
				t.Fatal(err)
			}
			if gone, _ := r.IsRevoked(ctx, "old", "u1", now); gone {
				t.Error("an already expired token needs no entry")
			}

			at := now.Add(time.Second)
			if err := r.RevokeUser(ctx, "u1", at, 15*time.Minute); err != nil {
				t.Fatal(err)
			}
			if gone, _ := r.IsRevoked(ctx, "jti-2", "u1", at.Add(-time.Millisecond)); !gone {
				t.Error("a token issued before the user revocation must be revoked")
			}
			if gone, _ := r.IsRevoked(ctx, "jti-3", "u1", at.Add(time.Millisecond)); gone {
				t.Error("a token issued after the user revocation (a new login) must work")
			}
			if gone, _ := r.IsRevoked(ctx, "jti-2", "u2", at.Add(-time.Hour)); gone {
				t.Error("other users are not affected")
			}
		})
	}
}

func TestRedisRevocations_UnreadableEntryFailsClosed(t *testing.T) {
	mr := miniredis.RunT(t)
	r := middleware.NewRedisRevocations(redis.NewClient(&redis.Options{Addr: mr.Addr()}), "p:")
	_ = mr.Set("p:user:u1", "garbage")
	if gone, err := r.IsRevoked(context.Background(), "j", "u1", time.Now()); err != nil || !gone {
		t.Errorf("garbage entry: revoked=%v err=%v, want revoked", gone, err)
	}
}

type brokenRevocations struct{}

func (brokenRevocations) Revoke(context.Context, string, time.Time) error { return nil }
func (brokenRevocations) RevokeUser(context.Context, string, time.Time, time.Duration) error {
	return nil
}
func (brokenRevocations) IsRevoked(context.Context, string, string, time.Time) (bool, error) {
	return false, errors.New("redis down")
}

func TestAuthenticateWith_Revocation(t *testing.T) {
	svc := pkgjwt.MustNewService("dGVzdC1zZWNyZXQta2V5LWZvci11bml0LXRlc3RpbmctMzJi")
	revoked := middleware.NewMemoryRevocations()
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if middleware.TokenIDFromContext(r.Context()) == "" {
			t.Error("the token id must reach the handler")
		}
		w.WriteHeader(http.StatusNoContent)
	})
	h := middleware.AuthenticateWith(svc, revoked)(next)
	call := func(tok string, hh http.Handler) int {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		rec := httptest.NewRecorder()
		hh.ServeHTTP(rec, req)
		return rec.Code
	}

	tok, _ := svc.GenerateToken("u1", "c1", []string{"morador"})
	if got := call(tok, h); got != http.StatusNoContent {
		t.Fatalf("valid token: %d", got)
	}
	claims, _ := svc.ValidateToken(tok)
	_ = revoked.Revoke(context.Background(), claims.ID, claims.ExpiresAt.Time)
	if got := call(tok, h); got != http.StatusUnauthorized {
		t.Errorf("revoked token: got %d, want 401", got)
	}

	other, _ := svc.GenerateToken("u2", "c1", []string{"morador"})
	_ = revoked.RevokeUser(context.Background(), "u2", time.Now().Add(time.Millisecond), time.Minute)
	if got := call(other, h); got != http.StatusUnauthorized {
		t.Errorf("token older than the user revocation: got %d, want 401", got)
	}
	time.Sleep(5 * time.Millisecond)
	fresh, _ := svc.GenerateToken("u2", "c1", []string{"morador"})
	if got := call(fresh, h); got != http.StatusNoContent {
		t.Errorf("token issued after the revocation: got %d, want 204", got)
	}

	if got := call(tok, middleware.AuthenticateWith(svc, brokenRevocations{})(next)); got != http.StatusServiceUnavailable {
		t.Errorf("revocation list down: got %d, want 503 (fail closed)", got)
	}
	// Without a list the middleware is the plain Authenticate.
	if got := call(tok, middleware.AuthenticateWith(svc, nil)(next)); got != http.StatusNoContent {
		t.Errorf("no list configured: got %d", got)
	}
	// Step tokens never pass as sessions.
	step, _ := svc.GenerateStepToken("u1", "c1", pkgjwt.PurposeMFAVerify, time.Minute)
	if got := call(step, h); got != http.StatusUnauthorized {
		t.Errorf("step token as session: got %d, want 401", got)
	}
}
