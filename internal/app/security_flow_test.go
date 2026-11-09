package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
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
	"github.com/victorradael/condoguard/api/pkg/mailer"
	"github.com/victorradael/condoguard/api/pkg/totp"
)

// The account-security flows of FAB-9, end to end over HTTP on PostgreSQL:
// e-mail verification, password recovery, mandatory 2FA, recovery codes,
// session revocation and the síndico's help (link to pass on, 2FA reset).

const (
	flowCondo = dbtest.CondoA
	flowPass  = "Corr3ct-Horse-Battery"
	flowKey   = "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="
	flowWeb   = "https://web.example"
)

type flow struct {
	t    *testing.T
	srv  *httptest.Server
	mail *mailer.Memory
	url  string
	now  time.Time
}

func newFlow(t *testing.T) *flow {
	t.Helper()
	d := dbtest.NewEnv(t)
	d.SeedCondo(flowCondo)
	f := &flow{t: t, mail: &mailer.Memory{}, url: d.AppURL}
	cfg := app.Config{
		DatabaseURL: d.AppURL, CondominiumID: flowCondo, LoginMaxAttempts: 50, LoginMaxAttemptsPerIP: 500, LoginLockoutWindow: time.Minute,
		RegisterMaxPerIP: 100, RegisterWindow: time.Minute, JWTSecret: testSecret, AccessTokenTTL: 5 * time.Minute, RefreshTokenTTL: time.Hour,
		CORSAllowedOrigins: []string{flowWeb}, TOTPKey: flowKey, TOTPIssuer: "Condoguard", WebBaseURL: flowWeb,
		Mail: f.mail, RequireVerifiedEmail: true,
	}
	a, err := app.New(context.Background(), cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), middleware.NewMetrics())
	if err != nil {
		t.Fatal(err)
	}
	f.srv = httptest.NewServer(a.Handler)
	t.Cleanup(func() { f.srv.Close(); a.Close() })
	return f
}

func (f *flow) call(method, path, token string, body any) (int, map[string]any) {
	f.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	r, _ := http.NewRequest(method, f.srv.URL+path, rd)
	r.Header.Set("Content-Type", "application/json")
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		f.t.Fatal(err)
	}
	defer resp.Body.Close()
	var m map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&m)
	return resp.StatusCode, m
}

func (f *flow) must(want int, method, path, token string, body any) map[string]any {
	f.t.Helper()
	code, m := f.call(method, path, token, body)
	if code != want {
		f.t.Fatalf("%s %s: expected %d, got %d: %v", method, path, want, code, m)
	}
	return m
}

var tokenRe = regexp.MustCompile(`token=([A-Za-z0-9_%-]+)`)

// lastToken waits for the e-mail to arrive (it is sent in the background) and
// returns the token in its link; n is how many e-mails to the address must exist.
func (f *flow) lastToken(to string, n int) string {
	f.t.Helper()
	for range 100 {
		if msgs := f.mail.To(to); len(msgs) >= n {
			m := tokenRe.FindStringSubmatch(msgs[len(msgs)-1].Body)
			if m == nil || !strings.Contains(msgs[len(msgs)-1].Body, flowWeb) {
				f.t.Fatalf("e-mail without a link to the web app: %q", msgs[len(msgs)-1].Body)
			}
			return m[1]
		}
		time.Sleep(20 * time.Millisecond)
	}
	f.t.Fatalf("no e-mail to %s", to)
	return ""
}

func (f *flow) verified(username, email string) {
	f.t.Helper()
	f.must(201, "POST", "/auth/register", "", map[string]any{"username": username, "email": email, "password": flowPass})
	f.must(204, "POST", "/auth/email/verify", "", map[string]any{"token": f.lastToken(email, 1)})
}

func (f *flow) login(email, pass string) (int, map[string]any) {
	return f.call("POST", "/auth/login", "", map[string]any{"email": email, "password": pass})
}

func (f *flow) createSindico(email string) {
	f.t.Helper()
	ctx := context.Background()
	pool, err := db.NewPool(ctx, f.url)
	if err != nil {
		f.t.Fatal(err)
	}
	defer pool.Close()
	_, err = user.NewService(user.NewPostgresRepository(pool)).Create(authz.AsOperator(tenant.WithCondominiumID(ctx, flowCondo)), user.CreateRequest{
		Username: strings.Split(email, "@")[0], Email: email, Password: flowPass, Roles: []string{authz.RoleSindico},
	})
	if err != nil {
		f.t.Fatal(err)
	}
}

func code(t *testing.T, secret string, plus int64) string {
	t.Helper()
	c, err := totp.Code(secret, totp.Step(time.Now())+plus)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestFlow_EmailVerification(t *testing.T) {
	f := newFlow(t)
	email := "ana@exemplo.test"
	f.must(201, "POST", "/auth/register", "", map[string]any{"username": "ana", "email": email, "password": flowPass})

	if code, body := f.login(email, flowPass); code != 403 || body["code"] != "email_not_verified" {
		t.Fatalf("login before verifying: %d %v", code, body)
	}
	tok := f.lastToken(email, 1)
	f.must(204, "POST", "/auth/email/verify", "", map[string]any{"token": tok})
	f.must(400, "POST", "/auth/email/verify", "", map[string]any{"token": tok}) // single use
	f.must(200, "POST", "/auth/login", "", map[string]any{"email": email, "password": flowPass})

	// Asking again never reveals whether the address has an account.
	f.must(202, "POST", "/auth/email/resend", "", map[string]any{"email": "ninguem@exemplo.test"})
	f.must(202, "POST", "/auth/email/resend", "", map[string]any{"email": email})
}

func TestFlow_PasswordRecovery(t *testing.T) {
	f := newFlow(t)
	email := "bia@exemplo.test"
	f.verified("bia", email)
	old := f.must(200, "POST", "/auth/login", "", map[string]any{"email": email, "password": flowPass})

	a := f.must(202, "POST", "/auth/password/forgot", "", map[string]any{"email": email})
	b := f.must(202, "POST", "/auth/password/forgot", "", map[string]any{"email": "ninguem@exemplo.test"})
	if a["message"] != b["message"] {
		t.Fatalf("the answer must not tell which e-mails exist: %v vs %v", a, b)
	}
	tok := f.lastToken(email, 2) // verification + reset
	f.must(422, "POST", "/auth/password/reset", "", map[string]any{"token": tok, "newPassword": "123"})
	f.must(204, "POST", "/auth/password/reset", "", map[string]any{"token": tok, "newPassword": "Nova-Senha-Segura-77"})
	f.must(400, "POST", "/auth/password/reset", "", map[string]any{"token": tok, "newPassword": "Outra-Senha-Segura-88"})

	if code, _ := f.login(email, flowPass); code != 401 {
		t.Fatalf("old password still works: %d", code)
	}
	f.must(200, "POST", "/auth/login", "", map[string]any{"email": email, "password": "Nova-Senha-Segura-77"})

	// The reset ended the sessions opened before it.
	f.must(401, "POST", "/auth/refresh", "", map[string]any{"refreshToken": old["refreshToken"]})
	f.must(401, "GET", "/users/me", old["accessToken"].(string), nil)
}

func TestFlow_MandatoryTwoFactorForSindico(t *testing.T) {
	f := newFlow(t)
	email := "sindico@exemplo.test"
	f.createSindico(email)
	// Created by the operator: the e-mail is not confirmed yet. Recovery confirms it.
	f.must(202, "POST", "/auth/password/forgot", "", map[string]any{"email": email})
	f.must(204, "POST", "/auth/password/reset", "", map[string]any{"token": f.lastToken(email, 1), "newPassword": flowPass})

	// Password alone never opens a session for this role: it must enrol first.
	ch := f.must(200, "POST", "/auth/login", "", map[string]any{"email": email, "password": flowPass})
	if ch["mfaRequired"] != true || ch["stage"] != "enroll" || ch["accessToken"] != nil {
		t.Fatalf("expected an enrol challenge, got %v", ch)
	}
	step := ch["mfaToken"].(string)
	// The step token opens nothing else.
	f.must(401, "GET", "/users/me", step, nil)

	setup := f.must(200, "POST", "/auth/2fa/setup", "", map[string]any{"mfaToken": step})
	secret := setup["secret"].(string)
	if !strings.HasPrefix(setup["otpauthUrl"].(string), "otpauth://totp/") {
		t.Fatalf("otpauth url: %v", setup["otpauthUrl"])
	}
	f.must(401, "POST", "/auth/2fa/enable", "", map[string]any{"mfaToken": step, "code": "000000"})
	done := f.must(200, "POST", "/auth/2fa/enable", "", map[string]any{"mfaToken": step, "code": code(t, secret, 0)})
	codes, _ := done["recoveryCodes"].([]any)
	if len(codes) != 10 || done["accessToken"] == nil {
		t.Fatalf("enrolment must finish the login and show 10 recovery codes once: %v", done)
	}
	access := done["accessToken"].(string)
	f.must(200, "GET", "/users/me", access, nil)

	// Next login: password, then the code. The same code works once.
	ch = f.must(200, "POST", "/auth/login", "", map[string]any{"email": email, "password": flowPass})
	if ch["stage"] != "verify" {
		t.Fatalf("expected verify, got %v", ch)
	}
	f.must(401, "POST", "/auth/2fa/verify", "", map[string]any{"mfaToken": ch["mfaToken"], "code": "123456"})
	next := code(t, secret, 1) // a step ahead of the one used at enrolment
	ok := f.must(200, "POST", "/auth/2fa/verify", "", map[string]any{"mfaToken": ch["mfaToken"], "code": next})
	if ok["accessToken"] == nil {
		t.Fatalf("no session after a valid code: %v", ok)
	}
	f.must(401, "POST", "/auth/2fa/verify", "", map[string]any{"mfaToken": ch["mfaToken"], "code": next}) // replay

	// A recovery code works once and then is gone.
	ch = f.must(200, "POST", "/auth/login", "", map[string]any{"email": email, "password": flowPass})
	rc := codes[0].(string)
	f.must(200, "POST", "/auth/2fa/verify", "", map[string]any{"mfaToken": ch["mfaToken"], "recoveryCode": rc})
	ch = f.must(200, "POST", "/auth/login", "", map[string]any{"email": email, "password": flowPass})
	f.must(401, "POST", "/auth/2fa/verify", "", map[string]any{"mfaToken": ch["mfaToken"], "recoveryCode": rc})

	// A role that requires 2FA cannot switch it off.
	f.must(403, "POST", "/auth/2fa/disable", access, map[string]any{"password": flowPass, "code": code(t, secret, 2)})
}

func TestFlow_OptionalTwoFactorAndSessionRevocation(t *testing.T) {
	f := newFlow(t)
	email := "caio@exemplo.test"
	f.verified("caio", email)
	s1 := f.must(200, "POST", "/auth/login", "", map[string]any{"email": email, "password": flowPass})
	if s1["mfaRequired"] != nil {
		t.Fatal("a morador is not forced into 2FA")
	}
	access := s1["accessToken"].(string)

	secret := f.must(200, "POST", "/auth/2fa/setup", access, nil)["secret"].(string)
	f.must(200, "POST", "/auth/2fa/enable", access, map[string]any{"code": code(t, secret, 0)})
	if ch := f.must(200, "POST", "/auth/login", "", map[string]any{"email": email, "password": flowPass}); ch["stage"] != "verify" {
		t.Fatalf("2FA on must ask for the code: %v", ch)
	}

	// Sign out everywhere: the access token already issued stops working at once.
	f.must(200, "GET", "/users/me", access, nil)
	f.must(204, "DELETE", "/auth/sessions", access, nil)
	f.must(401, "GET", "/users/me", access, nil)
	f.must(401, "POST", "/auth/refresh", "", map[string]any{"refreshToken": s1["refreshToken"]})
}

func TestFlow_LogoutRevokesTheAccessToken(t *testing.T) {
	f := newFlow(t)
	email := "dani@exemplo.test"
	f.verified("dani", email)
	s := f.must(200, "POST", "/auth/login", "", map[string]any{"email": email, "password": flowPass})
	access := s["accessToken"].(string)
	f.must(200, "GET", "/users/me", access, nil)
	f.must(204, "POST", "/auth/logout", access, map[string]any{"refreshToken": s["refreshToken"]})
	f.must(401, "GET", "/users/me", access, nil)
}

func TestFlow_SindicoHelpsAMemberAndCannotTouchAnAdministradora(t *testing.T) {
	f := newFlow(t)
	sind := "sind@exemplo.test"
	f.createSindico(sind)
	f.must(202, "POST", "/auth/password/forgot", "", map[string]any{"email": sind})
	f.must(204, "POST", "/auth/password/reset", "", map[string]any{"token": f.lastToken(sind, 1), "newPassword": flowPass})
	ch := f.must(200, "POST", "/auth/login", "", map[string]any{"email": sind, "password": flowPass})
	secret := f.must(200, "POST", "/auth/2fa/setup", "", map[string]any{"mfaToken": ch["mfaToken"]})["secret"].(string)
	tok := f.must(200, "POST", "/auth/2fa/enable", "", map[string]any{"mfaToken": ch["mfaToken"], "code": code(t, secret, 0)})["accessToken"].(string)

	// A morador who lost the e-mail: the síndico makes a link to pass on by WhatsApp.
	f.must(201, "POST", "/auth/register", "", map[string]any{"username": "edu", "email": "edu@exemplo.test", "password": flowPass})
	me := f.mustMember(t, tok, "edu@exemplo.test")
	link := f.must(201, "POST", "/users/"+me+"/account-links", tok, map[string]any{"purpose": "email_verify"})
	if !strings.HasPrefix(link["link"].(string), flowWeb+"/verificar-email?token=") || link["token"] == "" {
		t.Fatalf("link: %v", link)
	}
	f.must(204, "POST", "/auth/email/verify", "", map[string]any{"token": link["token"]})
	f.must(200, "POST", "/auth/login", "", map[string]any{"email": "edu@exemplo.test", "password": flowPass})
	f.must(422, "POST", "/users/"+me+"/account-links", tok, map[string]any{"purpose": "other"})

	// The audit trail records that a link was issued, never the token.
	var found bool
	logs := f.must(200, "GET", "/audit-log", tok, nil)
	raw, _ := json.Marshal(logs)
	if strings.Contains(string(raw), link["token"].(string)) {
		t.Fatal("the token leaked into the audit trail")
	}
	found = strings.Contains(string(raw), "issue_link")
	if !found {
		t.Fatalf("issuing a link must be audited: %s", raw)
	}

	// Takeover attempt: a síndico must not get a link for an administradora.
	adm := f.insertAdministradora(t)
	f.must(403, "POST", "/users/"+adm+"/account-links", tok, map[string]any{"purpose": "password_reset"})
	f.must(403, "DELETE", "/users/"+adm+"/2fa", tok, nil)

	// A morador cannot ask for any of this.
	m := f.must(200, "POST", "/auth/login", "", map[string]any{"email": "edu@exemplo.test", "password": flowPass})
	f.must(403, "POST", "/users/"+me+"/account-links", m["accessToken"].(string), map[string]any{"purpose": "password_reset"})

	// Lost phone and codes: the síndico resets the member's 2FA.
	f.must(204, "DELETE", "/users/"+me+"/2fa", tok, nil)
}

// mustMember finds the user id of an e-mail through the síndico's listing.
func (f *flow) mustMember(t *testing.T, token, email string) string {
	t.Helper()
	list := f.must(200, "GET", "/users?size=100", token, nil)
	items, _ := list["items"].([]any)
	for _, it := range items {
		if u, _ := it.(map[string]any); u["email"] == email {
			return u["id"].(string)
		}
	}
	t.Fatalf("%s not in %v", email, list)
	return ""
}

func (f *flow) insertAdministradora(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	pool, err := db.NewPool(ctx, f.url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	u, err := user.NewService(user.NewPostgresRepository(pool)).Create(authz.AsOperator(tenant.WithCondominiumID(ctx, flowCondo)), user.CreateRequest{
		Username: "adm", Email: "adm@exemplo.test", Password: flowPass, Roles: []string{authz.RoleAdministradora},
	})
	if err != nil {
		t.Fatal(err)
	}
	return u.ID
}
