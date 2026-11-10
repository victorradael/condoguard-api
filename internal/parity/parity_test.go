// Package parity contains end-to-end tests of the HTTP contract and of the
// security fixes of FAB-8 (1a): each test drives the full stack — middleware,
// handlers, services and PostgreSQL — and proves that the attack fails.
//
// These tests run only when TEST_DATABASE_URL is set (i.e. in the integration
// CI job). No mocks. Each test gets its own freshly migrated database; two
// servers bound to different condominiums may share it to test isolation.
//
// Run: TEST_DATABASE_URL=postgres://... go test ./internal/parity/... -v -count=1
package parity_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/victorradael/condoguard/api/internal/app"
	"github.com/victorradael/condoguard/api/internal/authz"
	"github.com/victorradael/condoguard/api/internal/db"
	"github.com/victorradael/condoguard/api/internal/db/dbtest"
	"github.com/victorradael/condoguard/api/internal/middleware"
	"github.com/victorradael/condoguard/api/internal/tenant"
	"github.com/victorradael/condoguard/api/internal/user"
	pkgjwt "github.com/victorradael/condoguard/api/pkg/jwt"
	"github.com/victorradael/condoguard/api/pkg/mailer"
	"github.com/victorradael/condoguard/api/pkg/openapi"
)

const (
	testSecret   = "dGVzdC1zZWNyZXQta2V5LWZvci11bml0LXRlc3RpbmctMzJi"
	metricsToken = "metrics-token-for-tests-0123456789abcdef"
	webOrigin    = "http://localhost:3000"
	evilOrigin   = "https://evil.example"
	password     = "Corr3ct-Horse-Battery"
	condoA       = dbtest.CondoA
	condoB       = dbtest.CondoB
	maxAttempts  = 3
	maxSignUps   = 12
)

// ── test environment ──────────────────────────────────────────────────────────

// env is one migrated database shared by one server per condominium. The
// servers connect as a role that owns nothing and has no BYPASSRLS, like the
// API in production, so Row-Level Security applies to every request.
type env struct {
	t     *testing.T
	dbURL string
	db    *dbtest.Env
}

func newEnv(t *testing.T) *env {
	t.Helper()
	d := dbtest.NewEnv(t)
	d.SeedCondo(condoA)
	d.SeedCondo(condoB)
	return &env{t: t, dbURL: d.AppURL, db: d}
}

// server starts the API with condoID as the sign-up/login tenant.
func (e *env) server(condoID string) *httptest.Server {
	e.t.Helper()
	return e.serverWith(condoID, nil)
}

// serverWith is server with the e-mails going to mail (nil keeps the default).
func (e *env) serverWith(condoID string, mail mailer.Sender) *httptest.Server {
	e.t.Helper()
	cfg := app.Config{
		Mail:               mail,
		WebBaseURL:         "https://app.exemplo.test",
		DatabaseURL:        e.dbURL,
		MigrateOnStart:     false, // dbtest.NewEnv already migrated
		CondominiumID:      condoID,
		LoginMaxAttempts:   maxAttempts,
		LoginLockoutWindow: time.Minute,
		RegisterMaxPerIP:   maxSignUps,
		RegisterWindow:     time.Minute,
		JWTSecret:          testSecret,
		AccessTokenTTL:     5 * time.Minute,
		RefreshTokenTTL:    time.Hour,
		CORSAllowedOrigins: []string{webOrigin},
		CookieSecure:       false, // httptest serves plain HTTP
		MetricsToken:       metricsToken,
	}
	a, err := app.New(context.Background(), cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), middleware.NewMetrics())
	if err != nil {
		e.t.Fatalf("app: %v", err)
	}
	srv := httptest.NewServer(a.Handler)
	e.t.Cleanup(func() {
		srv.Close()
		a.Close()
	})
	return srv
}

// createAdmin creates a sindico account the way cmd/createadmin does:
// public sign-up never grants it.
func (e *env) createAdmin(condoID, email string) {
	e.t.Helper()
	ctx := context.Background()
	pool, err := db.NewPool(ctx, e.dbURL)
	if err != nil {
		e.t.Fatal(err)
	}
	defer pool.Close()
	_, err = user.NewService(user.NewPostgresRepository(pool)).Create(authz.AsOperator(tenant.WithCondominiumID(ctx, condoID)), user.CreateRequest{
		Username: strings.Split(email, "@")[0],
		Email:    email,
		Password: password,
		Roles:    []string{authz.RoleSindico},
	})
	if err != nil {
		e.t.Fatalf("create admin: %v", err)
	}
}

type req struct {
	method, path string
	body         any
	rawBody      io.Reader
	token        string
	origin       string
	cookies      []*http.Cookie
}

func do(t *testing.T, srv *httptest.Server, r req) *http.Response {
	t.Helper()
	body := r.rawBody
	if r.body != nil {
		b, _ := json.Marshal(r.body)
		body = bytes.NewReader(b)
	}
	hr, err := http.NewRequest(r.method, srv.URL+r.path, body)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		hr.Header.Set("Content-Type", "application/json")
	}
	if r.token != "" {
		hr.Header.Set("Authorization", "Bearer "+r.token)
	}
	if r.origin != "" {
		hr.Header.Set("Origin", r.origin)
	}
	for _, c := range r.cookies {
		hr.AddCookie(c)
	}
	resp, err := http.DefaultClient.Do(hr)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func decode(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	return m
}

func expect(t *testing.T, resp *http.Response, want int, what string) {
	t.Helper()
	if resp.StatusCode != want {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("%s: expected %d, got %d: %s", what, want, resp.StatusCode, b)
	}
}

func register(t *testing.T, srv *httptest.Server, username, email string) {
	t.Helper()
	expect(t, do(t, srv, req{method: http.MethodPost, path: "/auth/register", body: map[string]any{
		"username": username, "email": email, "password": password,
	}}), http.StatusCreated, "register "+email)
}

// login runs the app flow and returns the access token.
func login(t *testing.T, srv *httptest.Server, email string) string {
	t.Helper()
	resp := do(t, srv, req{method: http.MethodPost, path: "/auth/login", body: map[string]any{"email": email, "password": password}})
	expect(t, resp, http.StatusOK, "login "+email)
	tok, _ := decode(t, resp)["accessToken"].(string)
	if tok == "" {
		t.Fatalf("login %s: no accessToken", email)
	}
	return tok
}

// resident registers and logs in a morador and returns its token and id.
func resident(t *testing.T, srv *httptest.Server, name string) (token, id string) {
	t.Helper()
	email := name + "@exemplo.test"
	register(t, srv, name, email)
	token = login(t, srv, email)
	resp := do(t, srv, req{method: http.MethodGet, path: "/users/me", token: token})
	expect(t, resp, http.StatusOK, "me")
	id, _ = decode(t, resp)["id"].(string)
	return token, id
}

func adminToken(t *testing.T, e *env, srv *httptest.Server, condoID string) string {
	t.Helper()
	email := "sindico-" + condoID + "@exemplo.test"
	e.createAdmin(condoID, email)
	return login(t, srv, email)
}

func create(t *testing.T, srv *httptest.Server, token, path string, body map[string]any) map[string]any {
	t.Helper()
	resp := do(t, srv, req{method: http.MethodPost, path: path, body: body, token: token})
	expect(t, resp, http.StatusCreated, "create "+path)
	return decode(t, resp)
}

func cookie(resp *http.Response, name string) *http.Cookie {
	for _, c := range resp.Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// ── Item 1: sign-up never grants client-sent roles ───────────────────────────

func TestSecurity1_Register_ClientRolesAreIgnored(t *testing.T) {
	srv := newEnv(t).server(condoA)

	expect(t, do(t, srv, req{method: http.MethodPost, path: "/auth/register", body: map[string]any{
		"username": "mallory", "email": "mallory@exemplo.test", "password": password,
		"roles": []string{"sindico"}, "role": "admin",
	}}), http.StatusCreated, "register")

	resp := do(t, srv, req{method: http.MethodPost, path: "/auth/login", body: map[string]any{"email": "mallory@exemplo.test", "password": password}})
	expect(t, resp, http.StatusOK, "login")
	body := decode(t, resp)
	roles, _ := body["roles"].([]any)
	if len(roles) != 1 || roles[0] != authz.RoleMorador {
		t.Fatalf("sign-up must create a resident only, got roles %v", roles)
	}
	token, _ := body["accessToken"].(string)

	expect(t, do(t, srv, req{method: http.MethodGet, path: "/users", token: token}), http.StatusForbidden, "GET /users as resident")
	expect(t, do(t, srv, req{method: http.MethodPost, path: "/users", token: token, body: map[string]any{
		"username": "evil", "email": "evil@exemplo.test", "password": password, "roles": []string{"sindico"},
	}}), http.StatusForbidden, "POST /users as resident")
}

func TestSecurity1_AdminCanCreateAdmins(t *testing.T) {
	e := newEnv(t)
	srv := e.server(condoA)
	admin := adminToken(t, e, srv, condoA)

	created := create(t, srv, admin, "/users", map[string]any{
		"username": "sub", "email": "sub@exemplo.test", "password": password, "roles": []string{"sindico"},
	})
	if roles, _ := created["roles"].([]any); len(roles) != 1 || roles[0] != authz.RoleSindico {
		t.Errorf("admin must be able to grant sindico, got %v", created["roles"])
	}
}

// ── Item 2: tokens signed with any other secret are rejected ─────────────────

func TestSecurity2_TokenSignedWithOtherSecret_Returns401(t *testing.T) {
	srv := newEnv(t).server(condoA)

	// The old default secret, base64-padded to the minimum length.
	forger := pkgjwt.MustNewService("c2VjcmV0c2VjcmV0c2VjcmV0c2VjcmV0c2VjcmV0c2VjcmV0")
	forged, err := forger.GenerateToken("attacker", condoA, []string{authz.RoleSindico})
	if err != nil {
		t.Fatal(err)
	}
	expect(t, do(t, srv, req{method: http.MethodGet, path: "/users", token: forged}), http.StatusUnauthorized, "forged admin token")
}

// ── Item 3: residents only see their own units, shops and expenses ───────────

func TestSecurity3_OwnerAuthorization(t *testing.T) {
	e := newEnv(t)
	srv := e.server(condoA)
	admin := adminToken(t, e, srv, condoA)
	ana, anaID := resident(t, srv, "ana")
	bia, _ := resident(t, srv, "bia")

	unit := create(t, srv, admin, "/residents", map[string]any{"unitNumber": "101", "floor": 1, "condominiumId": condoA, "ownerId": anaID})
	shop := create(t, srv, admin, "/shop-owners", map[string]any{"shopName": "Padaria", "cnpj": "11222333000181", "floor": 0, "ownerId": anaID})
	exp := create(t, srv, admin, "/expenses", map[string]any{"description": "Condomínio", "amountCents": 50000, "dueDate": "2026-12-10T00:00:00Z", "residentId": unit["id"]})

	paths := []string{"/residents/" + unit["id"].(string), "/shop-owners/" + shop["id"].(string), "/expenses/" + exp["id"].(string)}
	for _, p := range paths {
		expect(t, do(t, srv, req{method: http.MethodGet, path: p, token: ana}), http.StatusOK, "owner GET "+p)
		expect(t, do(t, srv, req{method: http.MethodGet, path: p, token: bia}), http.StatusNotFound, "other resident GET "+p)
		expect(t, do(t, srv, req{method: http.MethodDelete, path: p, token: ana}), http.StatusForbidden, "owner DELETE "+p)
	}
	expect(t, do(t, srv, req{method: http.MethodPut, path: paths[2], token: ana, body: map[string]any{"amountCents": 1}}), http.StatusForbidden, "owner edits expense")

	for _, list := range []string{"/residents", "/shop-owners", "/expenses"} {
		resp := do(t, srv, req{method: http.MethodGet, path: list, token: bia})
		expect(t, resp, http.StatusOK, "list "+list)
		if total := decode(t, resp)["total"]; total != float64(0) {
			t.Errorf("other resident lists %s: expected total 0, got %v", list, total)
		}
	}

	// A resident cannot create units or expenses for themselves.
	expect(t, do(t, srv, req{method: http.MethodPost, path: "/residents", token: bia, body: map[string]any{
		"unitNumber": "102", "floor": 1, "condominiumId": condoA, "ownerId": "x",
	}}), http.StatusForbidden, "resident creates unit")
}

// ── Item 4: data of another condominium is never visible ─────────────────────

func TestSecurity4_CondominiumIsolation(t *testing.T) {
	e := newEnv(t)
	srvA, srvB := e.server(condoA), e.server(condoB)
	adminA := adminToken(t, e, srvA, condoA)
	adminB := adminToken(t, e, srvB, condoB)
	_, anaID := resident(t, srvA, "ana")

	unit := create(t, srvA, adminA, "/residents", map[string]any{"unitNumber": "101", "floor": 1, "condominiumId": condoA, "ownerId": anaID})
	exp := create(t, srvA, adminA, "/expenses", map[string]any{"description": "Água", "amountCents": 1000, "dueDate": "2026-12-10T00:00:00Z", "residentId": unit["id"]})
	notif := create(t, srvA, adminA, "/notifications", map[string]any{"message": "Assembleia", "residentIds": []string{unit["id"].(string)}})

	// The tenant comes from the token, so the server the request hits does not matter.
	for _, srv := range []*httptest.Server{srvA, srvB} {
		for _, p := range []string{"/residents/" + unit["id"].(string), "/expenses/" + exp["id"].(string), "/notifications/" + notif["id"].(string), "/users/" + anaID} {
			expect(t, do(t, srv, req{method: http.MethodGet, path: p, token: adminB}), http.StatusNotFound, "admin of B reads "+p)
			expect(t, do(t, srv, req{method: http.MethodDelete, path: p, token: adminB}), http.StatusNotFound, "admin of B deletes "+p)
		}
		for _, list := range []string{"/residents", "/expenses", "/notifications", "/users"} {
			resp := do(t, srv, req{method: http.MethodGet, path: list, token: adminB})
			expect(t, resp, http.StatusOK, "list "+list)
			// Admin B sees only their own account in /users.
			want := float64(0)
			if list == "/users" {
				want = 1
			}
			if total := decode(t, resp)["total"]; total != want {
				t.Errorf("admin of B lists %s: expected total %v, got %v", list, want, total)
			}
		}
	}

	// An expense that condo B files against a unit id of condo A never reaches
	// condo A: its owner and its admin still see only their own expense.
	// (Unit ids are not foreign keys yet — composite FKs land with RLS in 1b.)
	create(t, srvB, adminB, "/expenses", map[string]any{
		"description": "Golpe", "amountCents": 1000, "dueDate": "2026-12-10T00:00:00Z", "residentId": unit["id"],
	})
	ana := login(t, srvA, "ana@exemplo.test")
	for who, tok := range map[string]string{"owner": ana, "admin": adminA} {
		resp := do(t, srvA, req{method: http.MethodGet, path: "/expenses", token: tok})
		expect(t, resp, http.StatusOK, who+" lists expenses")
		if total := decode(t, resp)["total"]; total != float64(1) {
			t.Errorf("%s of A: expected only A's expense, got total %v", who, total)
		}
	}

	// The e-mail is the login and is unique across condominiums. The account
	// belongs to A, so a login through B's server (whose default condominium
	// only matters for sign-up) opens a session in A, never in B.
	expect(t, do(t, srvB, req{method: http.MethodPost, path: "/auth/register", body: map[string]any{
		"username": "ana", "email": "ana@exemplo.test", "password": password,
	}}), http.StatusConflict, "same e-mail in another condominium")
	viaB := login(t, srvB, "ana@exemplo.test")
	resp := do(t, srvB, req{method: http.MethodGet, path: "/condominium", token: viaB})
	expect(t, resp, http.StatusOK, "current condominium")
	if id := decode(t, resp)["id"]; id != condoA {
		t.Errorf("a login of A's account must open a session in A, got %v", id)
	}
}

// ── Item 5: author from the token, read state per recipient ──────────────────

func TestSecurity5_Notifications(t *testing.T) {
	e := newEnv(t)
	srv := e.server(condoA)
	admin := adminToken(t, e, srv, condoA)
	ana, anaID := resident(t, srv, "ana")
	caio, caioID := resident(t, srv, "caio")
	bia, _ := resident(t, srv, "bia")

	u1 := create(t, srv, admin, "/residents", map[string]any{"unitNumber": "101", "floor": 1, "condominiumId": condoA, "ownerId": anaID})
	u2 := create(t, srv, admin, "/residents", map[string]any{"unitNumber": "102", "floor": 1, "condominiumId": condoA, "ownerId": caioID})

	adminResp := do(t, srv, req{method: http.MethodGet, path: "/users/me", token: admin})
	adminID, _ := decode(t, adminResp)["id"].(string)

	n := create(t, srv, admin, "/notifications", map[string]any{
		"message": "Manutenção do elevador", "createdById": "forged-author",
		"residentIds": []string{u1["id"].(string), u2["id"].(string)},
	})
	if n["createdById"] != adminID {
		t.Fatalf("createdById must come from the token (%s), got %v", adminID, n["createdById"])
	}
	id := n["id"].(string)

	// Residents cannot send notifications.
	expect(t, do(t, srv, req{method: http.MethodPost, path: "/notifications", token: ana, body: map[string]any{
		"message": "spam", "residentIds": []string{u2["id"].(string)},
	}}), http.StatusForbidden, "resident sends notification")

	// Ana reads it (twice: idempotent).
	for i := range 2 {
		resp := do(t, srv, req{method: http.MethodPut, path: "/notifications/" + id + "/read", token: ana})
		expect(t, resp, http.StatusOK, "ana marks read")
		if decode(t, resp)["read"] != true {
			t.Fatalf("call %d: notification must be read for ana", i+1)
		}
	}
	// Caio, the other recipient, still has it unread.
	resp := do(t, srv, req{method: http.MethodGet, path: "/notifications/" + id, token: caio})
	expect(t, resp, http.StatusOK, "caio reads")
	if decode(t, resp)["read"] != false {
		t.Error("read state must be per recipient: caio has not read it")
	}
	// Bia is not a recipient: she can neither see nor mark it.
	expect(t, do(t, srv, req{method: http.MethodGet, path: "/notifications/" + id, token: bia}), http.StatusNotFound, "non-recipient reads")
	expect(t, do(t, srv, req{method: http.MethodPut, path: "/notifications/" + id + "/read", token: bia}), http.StatusNotFound, "non-recipient marks read")
}

// ── Item 6: CORS allow-list, never "*" with credentials ──────────────────────

func TestSecurity6_CORS(t *testing.T) {
	srv := newEnv(t).server(condoA)

	preflight := func(origin string) *http.Response {
		r, _ := http.NewRequest(http.MethodOptions, srv.URL+"/auth/login", nil)
		r.Header.Set("Origin", origin)
		r.Header.Set("Access-Control-Request-Method", "POST")
		resp, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = resp.Body.Close() })
		return resp
	}

	ok := preflight(webOrigin)
	expect(t, ok, http.StatusNoContent, "preflight from the web origin")
	if got := ok.Header.Get("Access-Control-Expose-Headers"); !strings.Contains(got, "Retry-After") {
		t.Errorf("the web must be able to read Retry-After, got Expose-Headers %q", got)
	}
	if ok.Header.Get("Access-Control-Allow-Origin") != webOrigin || ok.Header.Get("Access-Control-Allow-Credentials") != "true" {
		t.Errorf("allowed origin must be echoed with credentials, got %q", ok.Header.Get("Access-Control-Allow-Origin"))
	}

	bad := preflight(evilOrigin)
	expect(t, bad, http.StatusForbidden, "preflight from another origin")
	if acao := bad.Header.Get("Access-Control-Allow-Origin"); acao != "" {
		t.Errorf("unknown origin must get no Access-Control-Allow-Origin, got %q", acao)
	}

	health := do(t, srv, req{method: http.MethodGet, path: "/health", origin: evilOrigin})
	if acao := health.Header.Get("Access-Control-Allow-Origin"); acao == "*" || acao == evilOrigin {
		t.Errorf("simple request from another origin got Access-Control-Allow-Origin %q", acao)
	}
}

// ── Item 7: e-mail login, attempt limit, password policy, body limit ─────────

func TestSecurity7_LoginAndInputLimits(t *testing.T) {
	srv := newEnv(t).server(condoA)
	register(t, srv, "ana", "ana@exemplo.test")

	// Login is by e-mail only.
	expect(t, do(t, srv, req{method: http.MethodPost, path: "/auth/login", body: map[string]any{"username": "ana", "password": password}}),
		http.StatusUnprocessableEntity, "login by username")
	login(t, srv, "ANA@Exemplo.test") // e-mail is case-insensitive

	// E-mail is unique, ignoring case.
	expect(t, do(t, srv, req{method: http.MethodPost, path: "/auth/register", body: map[string]any{
		"username": "ana2", "email": "Ana@Exemplo.TEST", "password": password,
	}}), http.StatusConflict, "duplicate e-mail")

	// Weak passwords are rejected.
	for _, weak := range []string{"short1!", "alllowercaseletters", "ana@exemplo.test", "12345678901234"} {
		expect(t, do(t, srv, req{method: http.MethodPost, path: "/auth/register", body: map[string]any{
			"username": "weak", "email": "weak@exemplo.test", "password": weak,
		}}), http.StatusUnprocessableEntity, "weak password "+weak)
	}

	// After maxAttempts failures even the right password is refused.
	for i := range maxAttempts {
		expect(t, do(t, srv, req{method: http.MethodPost, path: "/auth/login", body: map[string]any{"email": "ana@exemplo.test", "password": "wrong"}}),
			http.StatusUnauthorized, "failed attempt "+string(rune('1'+i)))
	}
	expect(t, do(t, srv, req{method: http.MethodPost, path: "/auth/login", body: map[string]any{"email": "ana@exemplo.test", "password": password}}),
		http.StatusTooManyRequests, "login after max failures")

	// Sign-ups are limited per IP, which slows down e-mail enumeration (409).
	// Every attempt counts: ana, the duplicate and the 4 weak passwords above.
	for i := 2 + 4; i < maxSignUps; i++ {
		do(t, srv, req{method: http.MethodPost, path: "/auth/register", body: map[string]any{
			"username": "probe", "email": "ana@exemplo.test", "password": password,
		}})
	}
	limited := do(t, srv, req{method: http.MethodPost, path: "/auth/register", body: map[string]any{
		"username": "late", "email": "late@exemplo.test", "password": password,
	}})
	expect(t, limited, http.StatusTooManyRequests, "sign-up after the per-IP limit")
	if limited.Header.Get("Retry-After") == "" {
		t.Error("429 must carry Retry-After")
	}

	// Bodies above the limit are refused before they are parsed.
	huge := `{"username":"x","email":"x@exemplo.test","password":"` + strings.Repeat("a", int(middleware.DefaultMaxBodyBytes)) + `"}`
	expect(t, do(t, srv, req{method: http.MethodPost, path: "/auth/register", rawBody: strings.NewReader(huge)}),
		http.StatusRequestEntityTooLarge, "oversized body")
}

// ── Item 8: web flow with httpOnly cookies, refresh rotation and revocation ──

func TestSecurity8_CookieFlow(t *testing.T) {
	srv := newEnv(t).server(condoA)
	register(t, srv, "ana", "ana@exemplo.test")

	resp := do(t, srv, req{method: http.MethodPost, path: "/auth/login", origin: webOrigin, body: map[string]any{
		"email": "ana@exemplo.test", "password": password, "useCookies": true,
	}})
	expect(t, resp, http.StatusOK, "cookie login")
	access, refresh := cookie(resp, middleware.AccessCookie), cookie(resp, middleware.RefreshCookie)

	// Login CSRF: another site can't sign the browser into an account.
	for _, origin := range []string{"", evilOrigin} {
		csrf := do(t, srv, req{method: http.MethodPost, path: "/auth/login", origin: origin, body: map[string]any{
			"email": "ana@exemplo.test", "password": password, "useCookies": true,
		}})
		expect(t, csrf, http.StatusForbidden, "cookie login from origin "+origin)
		if len(csrf.Cookies()) != 0 {
			t.Errorf("cookie login from %q must set no cookie", origin)
		}
	}
	if access == nil || refresh == nil || !access.HttpOnly || !refresh.HttpOnly {
		t.Fatalf("login must set httpOnly %s and %s cookies", middleware.AccessCookie, middleware.RefreshCookie)
	}
	body := decode(t, resp)
	if body["accessToken"] != nil || body["refreshToken"] != nil {
		t.Fatalf("cookie flow must keep tokens out of the body: %v", body)
	}

	// The access cookie authenticates reads.
	expect(t, do(t, srv, req{method: http.MethodGet, path: "/users/me", cookies: []*http.Cookie{access}}), http.StatusOK, "me with cookie")

	// Cookie-authenticated writes from another origin are refused (CSRF).
	expect(t, do(t, srv, req{method: http.MethodPost, path: "/notifications", origin: evilOrigin, cookies: []*http.Cookie{access},
		body: map[string]any{"message": "x", "residentIds": []string{"r"}}}), http.StatusForbidden, "cross-origin write with cookie")

	// Refresh rotates the refresh cookie.
	rot := do(t, srv, req{method: http.MethodPost, path: "/auth/refresh", origin: webOrigin, cookies: []*http.Cookie{refresh}})
	expect(t, rot, http.StatusOK, "refresh")
	newRefresh := cookie(rot, middleware.RefreshCookie)
	if newRefresh == nil || newRefresh.Value == refresh.Value {
		t.Fatal("refresh must rotate the refresh token")
	}

	// Reusing the old refresh token revokes the whole session.
	expect(t, do(t, srv, req{method: http.MethodPost, path: "/auth/refresh", origin: webOrigin, cookies: []*http.Cookie{refresh}}),
		http.StatusUnauthorized, "reused refresh token")
	expect(t, do(t, srv, req{method: http.MethodPost, path: "/auth/refresh", origin: webOrigin, cookies: []*http.Cookie{newRefresh}}),
		http.StatusUnauthorized, "rotated token after reuse was detected")

	// Logout revokes the refresh token of a fresh session.
	resp = do(t, srv, req{method: http.MethodPost, path: "/auth/login", origin: webOrigin, body: map[string]any{
		"email": "ana@exemplo.test", "password": password, "useCookies": true,
	}})
	expect(t, resp, http.StatusOK, "second cookie login")
	refresh = cookie(resp, middleware.RefreshCookie)
	expect(t, do(t, srv, req{method: http.MethodPost, path: "/auth/logout", origin: webOrigin, cookies: []*http.Cookie{refresh}}),
		http.StatusNoContent, "logout")
	expect(t, do(t, srv, req{method: http.MethodPost, path: "/auth/refresh", origin: webOrigin, cookies: []*http.Cookie{refresh}}),
		http.StatusUnauthorized, "refresh after logout")
}

func TestSecurity8_AppFlow_BearerAndRefreshInBody(t *testing.T) {
	srv := newEnv(t).server(condoA)
	register(t, srv, "ana", "ana@exemplo.test")

	resp := do(t, srv, req{method: http.MethodPost, path: "/auth/login", body: map[string]any{"email": "ana@exemplo.test", "password": password}})
	expect(t, resp, http.StatusOK, "app login")
	if cookie(resp, middleware.AccessCookie) != nil {
		t.Error("app flow must not set cookies")
	}
	body := decode(t, resp)
	rt, _ := body["refreshToken"].(string)

	resp = do(t, srv, req{method: http.MethodPost, path: "/auth/refresh", body: map[string]any{"refreshToken": rt}})
	expect(t, resp, http.StatusOK, "app refresh")
	tok, _ := decode(t, resp)["accessToken"].(string)
	expect(t, do(t, srv, req{method: http.MethodGet, path: "/users/me", token: tok}), http.StatusOK, "me with refreshed bearer")
}

// Changing a user's roles revokes their refresh tokens: the old roles can't be
// carried into a new access token.
func TestSecurity8_RoleChangeRevokesRefreshTokens(t *testing.T) {
	e := newEnv(t)
	srv := e.server(condoA)
	admin := adminToken(t, e, srv, condoA)
	register(t, srv, "ana", "ana@exemplo.test")

	resp := do(t, srv, req{method: http.MethodPost, path: "/auth/login", body: map[string]any{"email": "ana@exemplo.test", "password": password}})
	expect(t, resp, http.StatusOK, "login")
	body := decode(t, resp)
	rt, _ := body["refreshToken"].(string)
	me := do(t, srv, req{method: http.MethodGet, path: "/users/me", token: body["accessToken"].(string)})
	id, _ := decode(t, me)["id"].(string)

	// Renaming keeps the session.
	expect(t, do(t, srv, req{method: http.MethodPut, path: "/users/" + id, token: admin, body: map[string]any{"username": "ana2"}}), http.StatusOK, "rename")
	resp = do(t, srv, req{method: http.MethodPost, path: "/auth/refresh", body: map[string]any{"refreshToken": rt}})
	expect(t, resp, http.StatusOK, "refresh after rename")
	rt, _ = decode(t, resp)["refreshToken"].(string)

	expect(t, do(t, srv, req{method: http.MethodPut, path: "/users/" + id, token: admin, body: map[string]any{"roles": []string{"sindico"}}}), http.StatusOK, "promote")
	expect(t, do(t, srv, req{method: http.MethodPost, path: "/auth/refresh", body: map[string]any{"refreshToken": rt}}), http.StatusUnauthorized, "refresh after role change")
}

// ── Web ↔ API contract ───────────────────────────────────────────────────────

func TestContract_ProtectedRoutes_NoToken_Returns401(t *testing.T) {
	srv := newEnv(t).server(condoA)
	for _, path := range []string{"/users", "/users/me", "/residents", "/shop-owners", "/shopOwners", "/expenses", "/notifications"} {
		expect(t, do(t, srv, req{method: http.MethodGet, path: path}), http.StatusUnauthorized, path)
	}
}

func TestContract_ShopOwners_BothPrefixes(t *testing.T) {
	e := newEnv(t)
	srv := e.server(condoA)
	admin := adminToken(t, e, srv, condoA)
	shop := create(t, srv, admin, "/shop-owners", map[string]any{"shopName": "Farmácia", "cnpj": "11222333000181", "floor": 0, "ownerId": "u"})
	for _, p := range []string{"/shop-owners/", "/shopOwners/"} {
		expect(t, do(t, srv, req{method: http.MethodGet, path: p + shop["id"].(string), token: admin}), http.StatusOK, p)
	}
}

func TestContract_GetByID_NonExistent_Returns404(t *testing.T) {
	e := newEnv(t)
	srv := e.server(condoA)
	admin := adminToken(t, e, srv, condoA)
	for _, base := range []string{"/users/", "/residents/", "/shop-owners/", "/expenses/", "/notifications/"} {
		for _, id := range []string{"000000000000000000000000", "2b1e4f1c-8a0f-4c1e-9d43-0d8f5a8b9e10"} {
			expect(t, do(t, srv, req{method: http.MethodGet, path: base + id, token: admin}), http.StatusNotFound, base+id)
		}
	}
}

func TestContract_Lists_ArePaginated(t *testing.T) {
	e := newEnv(t)
	srv := e.server(condoA)
	admin := adminToken(t, e, srv, condoA)
	for i := range 3 {
		create(t, srv, admin, "/residents", map[string]any{"unitNumber": string(rune('A' + i)), "floor": i, "condominiumId": condoA, "ownerId": "u"})
	}

	resp := do(t, srv, req{method: http.MethodGet, path: "/residents?page=2&pageSize=2", token: admin})
	expect(t, resp, http.StatusOK, "page 2")
	body := decode(t, resp)
	items, _ := body["items"].([]any)
	if body["page"] != float64(2) || body["pageSize"] != float64(2) || body["total"] != float64(3) || len(items) != 1 {
		t.Errorf("unexpected page envelope: %v", body)
	}
	expect(t, do(t, srv, req{method: http.MethodGet, path: "/residents?pageSize=100000", token: admin}), http.StatusBadRequest, "oversized page")
}

func TestContract_Metrics_RequiresToken(t *testing.T) {
	srv := newEnv(t).server(condoA)
	expect(t, do(t, srv, req{method: http.MethodGet, path: "/metrics"}), http.StatusUnauthorized, "anonymous /metrics")
	expect(t, do(t, srv, req{method: http.MethodGet, path: "/metrics", token: metricsToken}), http.StatusOK, "/metrics with token")
}

func TestContract_AllResponses_HaveRequestIDHeader(t *testing.T) {
	srv := newEnv(t).server(condoA)
	for _, path := range []string{"/health", "/metrics", "/residents"} {
		if resp := do(t, srv, req{method: http.MethodGet, path: path}); resp.Header.Get("X-Request-Id") == "" {
			t.Errorf("path %s: expected X-Request-Id header", path)
		}
	}
}

func TestContract_SecurityHeaders(t *testing.T) {
	e := newEnv(t)
	srv := e.server(condoA)
	admin := adminToken(t, e, srv, condoA)

	health := do(t, srv, req{method: http.MethodGet, path: "/health"})
	if got := health.Header.Get("Content-Security-Policy"); got != middleware.APIContentSecurityPolicy {
		t.Errorf("API CSP = %q", got)
	}
	if health.Header.Get("Strict-Transport-Security") == "" {
		t.Error("expected HSTS")
	}

	// /docs gets its own CSP: only the pinned Swagger UI files, no inline code but its own.
	docs := do(t, srv, req{method: http.MethodGet, path: "/docs", token: admin})
	expect(t, docs, http.StatusOK, "/docs as admin")
	if got := docs.Header.Get("Content-Security-Policy"); got != openapi.SwaggerUICSP {
		t.Errorf("/docs CSP = %q", got)
	}

	// A forged request ID is replaced, not echoed into the logs.
	r, _ := http.NewRequest(http.MethodGet, srv.URL+"/health", nil)
	r.Header.Set("X-Request-Id", "forged id\twith tabs")
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if got := resp.Header.Get("X-Request-Id"); got == "" || strings.Contains(got, " ") {
		t.Errorf("forged X-Request-Id must be replaced, got %q", got)
	}
}
