package auth_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/victorradael/condoguard/api/internal/auth"
	"github.com/victorradael/condoguard/api/internal/middleware"
	"github.com/victorradael/condoguard/api/internal/tenant"
	pkgjwt "github.com/victorradael/condoguard/api/pkg/jwt"
	"github.com/victorradael/condoguard/api/pkg/password"
)

const (
	secret    = "dGVzdC1zZWNyZXQta2V5LWZvci11bml0LXRlc3RpbmctMzJi"
	webOrigin = "http://localhost:3000"
	goodPass  = "S3cr3t!Pass9"
)

// ── helpers ──────────────────────────────────────────────────────────────────

type fixture struct {
	h      http.Handler
	repo   auth.Repository
	jwtSvc *pkgjwt.Service
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	return newFixtureWith(t, auth.NewInMemoryRepository(), auth.NewInMemoryTokenStore())
}

func newFixtureWith(t *testing.T, repo auth.Repository, tokens auth.TokenStore) *fixture {
	t.Helper()
	jwtSvc := pkgjwt.MustNewService(secret)
	h := auth.NewHandler(auth.NewService(repo, tokens, jwtSvc, 0),
		auth.CookieOptions{Secure: true, AllowedOrigins: []string{webOrigin}})
	return &fixture{h: tenant.Middleware("condo-a")(h), repo: repo, jwtSvc: jwtSvc}
}

func (f *fixture) do(t *testing.T, path string, body any, mutate ...func(*http.Request)) *httptest.ResponseRecorder {
	t.Helper()
	var b []byte
	if body != nil {
		b, _ = json.Marshal(body)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	for _, m := range mutate {
		m(req)
	}
	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, req)
	return rec
}

func fromOrigin(origin string) func(*http.Request) {
	return func(r *http.Request) { r.Header.Set("Origin", origin) }
}

func (f *fixture) register(t *testing.T, email string) {
	t.Helper()
	if rec := f.do(t, "/auth/register", map[string]any{"username": "alice", "email": email, "password": goodPass}); rec.Code != http.StatusCreated {
		t.Fatalf("register: expected 201, got %d — %s", rec.Code, rec.Body)
	}
}

func decodePair(t *testing.T, rec *httptest.ResponseRecorder) auth.TokenPair {
	t.Helper()
	var p auth.TokenPair
	if err := json.NewDecoder(rec.Body).Decode(&p); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return p
}

func cookie(rec *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func ctxA() context.Context { return tenant.WithCondominiumID(context.Background(), "condo-a") }

// ── POST /auth/register ───────────────────────────────────────────────────────

func TestRegister_Success_Returns201(t *testing.T) {
	f := newFixture(t)
	rec := f.do(t, "/auth/register", map[string]any{"username": "alice", "email": "alice@example.com", "password": goodPass})
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d — %s", rec.Code, rec.Body)
	}
	var resp map[string]string
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if resp["message"] == "" {
		t.Error("expected message in response body")
	}
}

// Item 1: public sign-up never grants the roles the client asks for.
func TestRegister_Security_RolesFromClientAreIgnored(t *testing.T) {
	f := newFixture(t)
	rec := f.do(t, "/auth/register", map[string]any{
		"username": "mallory", "email": "mallory@example.com", "password": goodPass,
		"roles": []string{"ROLE_ADMIN"},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}
	u, err := f.repo.FindByEmail(ctxA(), "mallory@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(u.Roles) != 1 || u.Roles[0] != "ROLE_USER" {
		t.Fatalf("expected [ROLE_USER], got %v", u.Roles)
	}

	pair := decodePair(t, f.do(t, "/auth/login", map[string]string{"email": "mallory@example.com", "password": goodPass}))
	claims, err := f.jwtSvc.ValidateToken(pair.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range claims.Roles {
		if r == "ROLE_ADMIN" {
			t.Fatal("token must not carry ROLE_ADMIN")
		}
	}
}

func TestRegister_DuplicateEmail_Returns409_CaseInsensitive(t *testing.T) {
	f := newFixture(t)
	f.register(t, "alice@example.com")
	rec := f.do(t, "/auth/register", map[string]any{"username": "alice2", "email": " Alice@Example.COM ", "password": goodPass})
	if rec.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", rec.Code)
	}
}

func TestRegister_InvalidInput_Returns422(t *testing.T) {
	f := newFixture(t)
	for name, body := range map[string]map[string]any{
		"missing email":    {"username": "a", "password": goodPass},
		"invalid email":    {"username": "a", "email": "not-an-email", "password": goodPass},
		"missing username": {"email": "a@example.com", "password": goodPass},
		"missing password": {"username": "a", "email": "a@example.com"},
		"short password":   {"username": "a", "email": "a@example.com", "password": "abc123"},
		"no digits":        {"username": "a", "email": "a@example.com", "password": "abcdefghijklmn"},
		"has e-mail":       {"username": "a", "email": "joaquim@example.com", "password": "joaquim12345"},
	} {
		t.Run(name, func(t *testing.T) {
			if rec := f.do(t, "/auth/register", body); rec.Code != http.StatusUnprocessableEntity {
				t.Errorf("expected 422, got %d — %s", rec.Code, rec.Body)
			}
		})
	}
}

func TestRegister_PasswordIsHashed_EmailNormalised(t *testing.T) {
	f := newFixture(t)
	f.register(t, " Carol@Example.com")
	u, err := f.repo.FindByEmail(ctxA(), "carol@example.com")
	if err != nil {
		t.Fatalf("user not found under the normalised e-mail: %v", err)
	}
	if u.Password == goodPass || !password.Verify(goodPass, u.Password) {
		t.Error("password must be stored as a bcrypt hash")
	}
}

// ── POST /auth/login ──────────────────────────────────────────────────────────

func TestLogin_AppFlow_ReturnsTokenPair(t *testing.T) {
	f := newFixture(t)
	f.register(t, "dave@example.com")

	rec := f.do(t, "/auth/login", map[string]string{"email": "DAVE@example.com", "password": goodPass})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — %s", rec.Code, rec.Body)
	}
	if len(rec.Result().Cookies()) != 0 {
		t.Error("the app flow must not set cookies")
	}
	p := decodePair(t, rec)
	if p.AccessToken == "" || p.RefreshToken == "" || p.TokenType != "Bearer" || p.ExpiresIn != 900 {
		t.Fatalf("unexpected pair: %+v", p)
	}
	claims, err := f.jwtSvc.ValidateToken(p.AccessToken)
	if err != nil || claims.CondominiumID != "condo-a" {
		t.Fatalf("token: %+v, %v", claims, err)
	}
}

// Item 8: in the web flow the tokens only travel in httpOnly cookies.
func TestLogin_Security_CookieFlowKeepsTokensOutOfTheBody(t *testing.T) {
	f := newFixture(t)
	f.register(t, "erin@example.com")

	rec := f.do(t, "/auth/login", map[string]any{"email": "erin@example.com", "password": goodPass, "useCookies": true}, fromOrigin(webOrigin))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	access, refresh := cookie(rec, middleware.AccessCookie), cookie(rec, middleware.RefreshCookie)
	if access == nil || refresh == nil {
		t.Fatalf("expected both cookies, got %v", rec.Result().Cookies())
	}
	if strings.Contains(body, access.Value) || strings.Contains(body, refresh.Value) || strings.Contains(body, "accessToken") {
		t.Errorf("tokens must not be in the body: %s", body)
	}
	for _, c := range []*http.Cookie{access, refresh} {
		if !c.HttpOnly || !c.Secure || c.SameSite == http.SameSiteNoneMode || c.MaxAge <= 0 {
			t.Errorf("cookie %s: insecure attributes %+v", c.Name, c)
		}
	}
	if refresh.Path != "/auth" || refresh.SameSite != http.SameSiteStrictMode {
		t.Errorf("refresh cookie must be Path=/auth, SameSite=Strict: %+v", refresh)
	}
	if _, err := f.jwtSvc.ValidateToken(access.Value); err != nil {
		t.Errorf("access cookie is not a valid token: %v", err)
	}
}

func TestLogin_WrongPasswordOrUnknownUser_Returns401(t *testing.T) {
	f := newFixture(t)
	f.register(t, "frank@example.com")
	for _, body := range []map[string]string{
		{"email": "frank@example.com", "password": "wrong-passw0rd"},
		{"email": "ghost@example.com", "password": goodPass},
		{"email": "not-an-email", "password": goodPass},
	} {
		if rec := f.do(t, "/auth/login", body); rec.Code != http.StatusUnauthorized {
			t.Errorf("%v: expected 401, got %d", body, rec.Code)
		}
	}
}

// Login is by e-mail: the username is not an identifier anymore.
func TestLogin_ByUsername_Returns422(t *testing.T) {
	f := newFixture(t)
	f.register(t, "gina@example.com")
	if rec := f.do(t, "/auth/login", map[string]string{"username": "alice", "password": goodPass}); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d", rec.Code)
	}
}

// ── POST /auth/refresh and /auth/logout (item 8) ──────────────────────────────

func TestRefresh_AppFlow_RotatesTheToken(t *testing.T) {
	f := newFixture(t)
	f.register(t, "hank@example.com")
	first := decodePair(t, f.do(t, "/auth/login", map[string]string{"email": "hank@example.com", "password": goodPass}))

	rec := f.do(t, "/auth/refresh", map[string]string{"refreshToken": first.RefreshToken})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — %s", rec.Code, rec.Body)
	}
	second := decodePair(t, rec)
	if second.RefreshToken == "" || second.RefreshToken == first.RefreshToken || second.AccessToken == "" {
		t.Fatalf("expected a new pair, got %+v", second)
	}
}

// Reusing an exchanged refresh token means it leaked: it fails, and the whole
// session (including the newer token) is revoked.
func TestRefresh_Security_ReuseRevokesTheSession(t *testing.T) {
	f := newFixture(t)
	f.register(t, "ivy@example.com")
	first := decodePair(t, f.do(t, "/auth/login", map[string]string{"email": "ivy@example.com", "password": goodPass}))
	second := decodePair(t, f.do(t, "/auth/refresh", map[string]string{"refreshToken": first.RefreshToken}))

	if rec := f.do(t, "/auth/refresh", map[string]string{"refreshToken": first.RefreshToken}); rec.Code != http.StatusUnauthorized {
		t.Fatalf("reused token: expected 401, got %d", rec.Code)
	}
	if rec := f.do(t, "/auth/refresh", map[string]string{"refreshToken": second.RefreshToken}); rec.Code != http.StatusUnauthorized {
		t.Fatalf("token of the revoked session: expected 401, got %d", rec.Code)
	}
}

func TestRefresh_InvalidToken_Returns401(t *testing.T) {
	f := newFixture(t)
	for _, body := range []any{nil, map[string]string{"refreshToken": "made-up"}} {
		if rec := f.do(t, "/auth/refresh", body); rec.Code != http.StatusUnauthorized {
			t.Errorf("%v: expected 401, got %d", body, rec.Code)
		}
	}
}

func TestLogout_RevokesTheRefreshToken(t *testing.T) {
	f := newFixture(t)
	f.register(t, "jack@example.com")
	p := decodePair(t, f.do(t, "/auth/login", map[string]string{"email": "jack@example.com", "password": goodPass}))

	if rec := f.do(t, "/auth/logout", map[string]string{"refreshToken": p.RefreshToken}); rec.Code != http.StatusNoContent {
		t.Fatalf("logout: expected 204, got %d", rec.Code)
	}
	if rec := f.do(t, "/auth/refresh", map[string]string{"refreshToken": p.RefreshToken}); rec.Code != http.StatusUnauthorized {
		t.Fatalf("refresh after logout: expected 401, got %d", rec.Code)
	}
	if rec := f.do(t, "/auth/logout", map[string]string{"refreshToken": "unknown"}); rec.Code != http.StatusNoContent {
		t.Errorf("logout with unknown token: expected 204, got %d", rec.Code)
	}
}

// Login CSRF: session cookies are only issued to an allowed origin, so a
// cross-site form cannot sign the victim's browser into another account.
func TestLogin_Security_CookieLoginNeedsAllowedOrigin(t *testing.T) {
	f := newFixture(t)
	f.register(t, "mallory@example.com")
	body := map[string]any{"email": "mallory@example.com", "password": goodPass, "useCookies": true}

	for _, origin := range []string{"", "https://evil.example", "null"} {
		rec := f.do(t, "/auth/login", body, fromOrigin(origin))
		if rec.Code != http.StatusForbidden {
			t.Errorf("cookie login from %q: expected 403, got %d", origin, rec.Code)
		}
		if len(rec.Result().Cookies()) != 0 {
			t.Errorf("cookie login from %q must set no cookie", origin)
		}
	}
	// The app flow (tokens in the body, no cookies) needs no Origin.
	if rec := f.do(t, "/auth/login", map[string]any{"email": "mallory@example.com", "password": goodPass}); rec.Code != http.StatusOK {
		t.Errorf("app login: expected 200, got %d", rec.Code)
	}
}

func TestCookieFlow_RefreshAndLogout(t *testing.T) {
	f := newFixture(t)
	f.register(t, "kim@example.com")
	login := f.do(t, "/auth/login", map[string]any{"email": "kim@example.com", "password": goodPass, "useCookies": true}, fromOrigin(webOrigin))
	refresh := cookie(login, middleware.RefreshCookie)

	withCookie := func(c *http.Cookie, origin string) func(*http.Request) {
		return func(r *http.Request) {
			r.AddCookie(c)
			if origin != "" {
				r.Header.Set("Origin", origin)
			}
		}
	}

	// CSRF: the cookie is only honoured from an allowed origin.
	for _, origin := range []string{"", "https://evil.example"} {
		if rec := f.do(t, "/auth/refresh", nil, withCookie(refresh, origin)); rec.Code != http.StatusForbidden {
			t.Fatalf("refresh from %q: expected 403, got %d", origin, rec.Code)
		}
	}

	rec := f.do(t, "/auth/refresh", nil, withCookie(refresh, webOrigin))
	if rec.Code != http.StatusOK {
		t.Fatalf("refresh: expected 200, got %d — %s", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), "refreshToken") {
		t.Error("cookie refresh must not return tokens in the body")
	}
	newRefresh := cookie(rec, middleware.RefreshCookie)
	if newRefresh == nil || newRefresh.Value == refresh.Value {
		t.Fatal("expected a rotated refresh cookie")
	}

	rec = f.do(t, "/auth/logout", nil, withCookie(newRefresh, webOrigin))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("logout: expected 204, got %d", rec.Code)
	}
	for _, name := range []string{middleware.AccessCookie, middleware.RefreshCookie} {
		if c := cookie(rec, name); c == nil || c.MaxAge >= 0 {
			t.Errorf("logout must expire cookie %s, got %+v", name, c)
		}
	}
	if rec := f.do(t, "/auth/refresh", nil, withCookie(newRefresh, webOrigin)); rec.Code != http.StatusUnauthorized {
		t.Errorf("refresh after logout: expected 401, got %d", rec.Code)
	}
}

// ── Item 4: sessions are scoped to the condominium ────────────────────────────

func TestRefresh_Security_TokenOfAnotherCondominiumIsRejected(t *testing.T) {
	repo, tokens := auth.NewInMemoryRepository(), auth.NewInMemoryTokenStore()
	f := newFixtureWith(t, repo, tokens)
	f.register(t, "lia@example.com")
	p := decodePair(t, f.do(t, "/auth/login", map[string]string{"email": "lia@example.com", "password": goodPass}))

	jwtSvc := pkgjwt.MustNewService(secret)
	other := tenant.Middleware("condo-b")(auth.NewHandler(auth.NewService(repo, tokens, jwtSvc, 0), auth.CookieOptions{}))
	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", strings.NewReader(`{"refreshToken":"`+p.RefreshToken+`"}`))
	rec := httptest.NewRecorder()
	other.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
	if rec := f.do(t, "/auth/login", map[string]string{"email": "lia@example.com", "password": goodPass}); rec.Code != http.StatusOK {
		t.Errorf("the original session's condominium still logs in: %d", rec.Code)
	}
}
