package parity_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/victorradael/condoguard/api/internal/authz"
	"github.com/victorradael/condoguard/api/internal/db"
	"github.com/victorradael/condoguard/api/internal/tenant"
	"github.com/victorradael/condoguard/api/internal/user"
	pkgjwt "github.com/victorradael/condoguard/api/pkg/jwt"
)

// Tests of the 1b foundation (FAB-9): roles per condominium, the permission
// matrix over HTTP, the audit trail and privilege escalation.

// userWithRole has a síndico of condoA create an account with the role and
// logs it in.
func userWithRole(t *testing.T, srv *httptest.Server, admin, role string) string {
	t.Helper()
	email := role + "@exemplo.test"
	created := create(t, srv, admin, "/users", map[string]any{
		"username": role, "email": email, "password": password, "roles": []string{role},
	})
	if roles, _ := created["roles"].([]any); len(roles) != 1 || roles[0] != role {
		t.Fatalf("create %s: roles %v", role, created["roles"])
	}
	return login(t, srv, email)
}

// administradora creates the account the way cmd/createadmin does.
func (e *env) createAs(condoID, email, role string) {
	e.t.Helper()
	ctx := context.Background()
	pool, err := db.NewPool(ctx, e.dbURL)
	if err != nil {
		e.t.Fatal(err)
	}
	defer pool.Close()
	_, err = user.NewService(user.NewPostgresRepository(pool)).Create(authz.AsOperator(tenant.WithCondominiumID(ctx, condoID)), user.CreateRequest{
		Username: strings.Split(email, "@")[0], Email: email, Password: password, Roles: []string{role},
	})
	if err != nil {
		e.t.Fatalf("create %s: %v", role, err)
	}
}

func status(t *testing.T, srv *httptest.Server, method, path, token string, body any) int {
	t.Helper()
	resp := do(t, srv, req{method: method, path: path, token: token, body: body})
	return resp.StatusCode
}

// The permission matrix, role by role, against the real HTTP API.
func TestMatrix_OverHTTP(t *testing.T) {
	e := newEnv(t)
	srv := e.server(condoA)
	sindico := adminToken(t, e, srv, condoA)

	roles := []string{authz.RoleSindico, authz.RoleConselho, authz.RolePorteiro, authz.RoleMorador}
	tokens := map[string]string{authz.RoleSindico: sindico}
	for _, r := range roles[1:] {
		tokens[r] = userWithRole(t, srv, sindico, r)
	}
	e.createAs(condoA, "administradora@exemplo.test", authz.RoleAdministradora)
	tokens[authz.RoleAdministradora] = login(t, srv, "administradora@exemplo.test")

	// A unit and an expense of someone else, to read and change.
	unit := create(t, srv, sindico, "/residents", map[string]any{"unitNumber": "900", "floor": 9, "condominiumId": condoA, "ownerId": "someone-else"})
	unitID := unit["id"].(string)
	exp := create(t, srv, sindico, "/expenses", map[string]any{"description": "Elevador", "amountCents": 5000, "dueDate": "2026-12-10T00:00:00Z", "residentId": unitID})
	expID := exp["id"].(string)
	notif := create(t, srv, sindico, "/notifications", map[string]any{"message": "Obra", "residentIds": []string{unitID}})
	notifID := notif["id"].(string)

	type probe struct {
		name          string
		method, path  string
		body          func(role string) any
		allowed       map[string]bool // roles that pass authorization
		allowedStatus int
	}
	admins := map[string]bool{authz.RoleAdministradora: true, authz.RoleSindico: true}
	readers := map[string]bool{authz.RoleAdministradora: true, authz.RoleSindico: true, authz.RoleConselho: true}
	probes := []probe{
		{name: "list users", method: "GET", path: "/users", allowed: readers, allowedStatus: 200},
		{name: "read a user", method: "GET", path: "/users/{self}", allowed: readers, allowedStatus: 200},
		{name: "create user", method: "POST", path: "/users", allowed: admins, allowedStatus: 201,
			body: func(r string) any {
				return map[string]any{"username": "n-" + r, "email": "n-" + r + "@exemplo.test", "password": password, "roles": []string{"morador"}}
			}},
		{name: "audit trail", method: "GET", path: "/audit-log", allowed: readers, allowedStatus: 200},
		{name: "create unit", method: "POST", path: "/residents", allowed: admins, allowedStatus: 201,
			body: func(r string) any {
				return map[string]any{"unitNumber": "u-" + r, "floor": 1, "condominiumId": condoA, "ownerId": "x"}
			}},
		{name: "update unit", method: "PUT", path: "/residents/" + unitID, allowed: admins, allowedStatus: 200, body: func(string) any { return map[string]any{"floor": 8} }},
		{name: "create shop", method: "POST", path: "/shop-owners", allowed: admins, allowedStatus: 201,
			body: func(r string) any {
				cnpj := map[string]string{authz.RoleAdministradora: "11222333000181", authz.RoleSindico: "11444777000161"}[r]
				return map[string]any{"shopName": "Loja " + r, "cnpj": cnpj, "floor": 0, "ownerId": "x"}
			}},
		{name: "create expense", method: "POST", path: "/expenses", allowed: admins, allowedStatus: 201,
			body: func(string) any {
				return map[string]any{"description": "Taxa", "amountCents": 100, "dueDate": "2026-12-10T00:00:00Z", "residentId": unitID}
			}},
		{name: "update expense", method: "PUT", path: "/expenses/" + expID, allowed: admins, allowedStatus: 200, body: func(string) any { return map[string]any{"description": "Elevador 2"} }},
		{name: "create notification", method: "POST", path: "/notifications", allowed: admins, allowedStatus: 201,
			body: func(string) any { return map[string]any{"message": "Aviso", "residentIds": []string{unitID}} }},
		{name: "update notification", method: "PUT", path: "/notifications/" + notifID, allowed: admins, allowedStatus: 200, body: func(string) any { return map[string]any{"message": "Obra 2"} }},
	}
	selfID := map[string]string{}
	for r, tok := range tokens {
		resp := do(t, srv, req{method: http.MethodGet, path: "/users/me", token: tok})
		expect(t, resp, http.StatusOK, "me "+r)
		selfID[r], _ = decode(t, resp)["id"].(string)
	}

	for _, p := range probes {
		for _, role := range append(roles, authz.RoleAdministradora) {
			path := strings.ReplaceAll(p.path, "{self}", selfID[role])
			var body any
			if p.body != nil {
				body = p.body(role)
			}
			got := status(t, srv, p.method, path, tokens[role], body)
			want := http.StatusForbidden
			if p.allowed[role] {
				want = p.allowedStatus
			}
			if got != want {
				t.Errorf("%s as %s: got %d, want %d", p.name, role, got, want)
			}
		}
	}

	// Reads that every role may make, each with its own scope.
	for _, role := range append(roles, authz.RoleAdministradora) {
		for _, path := range []string{"/residents", "/shop-owners", "/expenses", "/notifications", "/users/me", "/condominium"} {
			if got := status(t, srv, http.MethodGet, path, tokens[role], nil); got != http.StatusOK {
				t.Errorf("GET %s as %s: got %d, want 200", path, role, got)
			}
		}
	}

	// Scope of the reads: the unit and its expense belong to someone else.
	type seen struct{ residents, expenses, notifications float64 }
	want := map[string]seen{
		authz.RoleAdministradora: {residents: -1, expenses: -1, notifications: -1}, // everything: just >= 1
		authz.RoleSindico:        {residents: -1, expenses: -1, notifications: -1},
		authz.RoleConselho:       {residents: -1, expenses: -1, notifications: -1},
		authz.RolePorteiro:       {residents: -1, expenses: 0, notifications: 0}, // units yes, finances no
		authz.RoleMorador:        {residents: 0, expenses: 0, notifications: 0},
	}
	for role, w := range want {
		for path, n := range map[string]float64{"/residents": w.residents, "/expenses": w.expenses, "/notifications": w.notifications} {
			total, _ := decode(t, do(t, srv, req{method: http.MethodGet, path: path, token: tokens[role]}))["total"].(float64)
			if n == -1 && total < 1 || n == 0 && total != 0 {
				t.Errorf("GET %s as %s: total %v, want %s", path, role, total, map[float64]string{-1: ">= 1", 0: "0"}[n])
			}
		}
	}

	// Deleting: only the managers; the others get 403 and the record stays.
	for _, role := range []string{authz.RoleConselho, authz.RolePorteiro, authz.RoleMorador} {
		for _, path := range []string{"/residents/" + unitID, "/expenses/" + expID, "/notifications/" + notifID, "/users/" + selfID[authz.RoleMorador]} {
			if got := status(t, srv, http.MethodDelete, path, tokens[role], nil); got != http.StatusForbidden {
				t.Errorf("DELETE %s as %s: got %d, want 403", path, role, got)
			}
		}
	}
	for _, path := range []string{"/residents/" + unitID, "/expenses/" + expID, "/notifications/" + notifID} {
		if got := status(t, srv, http.MethodGet, path, sindico, nil); got != http.StatusOK {
			t.Errorf("%s must survive the forbidden deletes, got %d", path, got)
		}
	}

	// The retired role names grant nothing.
	forged, _ := pkgjwt.MustNewService(testSecret).GenerateToken(selfID[authz.RoleMorador], condoA, []string{"ROLE_ADMIN"})
	if got := status(t, srv, http.MethodGet, "/users", forged, nil); got != http.StatusForbidden {
		t.Errorf("a token with the retired ROLE_ADMIN must not manage users, got %d", got)
	}
}

// A síndico manages the condominium but cannot reach above their own level.
func TestEscalation_SindicoCannotTouchAdministradora(t *testing.T) {
	e := newEnv(t)
	srv := e.server(condoA)
	sindico := adminToken(t, e, srv, condoA)
	e.createAs(condoA, "adm@exemplo.test", authz.RoleAdministradora)
	adm := login(t, srv, "adm@exemplo.test")

	resp := do(t, srv, req{method: http.MethodGet, path: "/users/me", token: adm})
	admID, _ := decode(t, resp)["id"].(string)
	resp = do(t, srv, req{method: http.MethodGet, path: "/users/me", token: sindico})
	sindicoID, _ := decode(t, resp)["id"].(string)

	// Creating or promoting to administradora, themselves included.
	expect(t, do(t, srv, req{method: http.MethodPost, path: "/users", token: sindico, body: map[string]any{
		"username": "x", "email": "x@exemplo.test", "password": password, "roles": []string{"administradora"},
	}}), http.StatusForbidden, "síndico creates administradora")
	expect(t, do(t, srv, req{method: http.MethodPut, path: "/users/" + sindicoID, token: sindico, body: map[string]any{"roles": []string{"administradora"}}}),
		http.StatusForbidden, "síndico promotes themselves")
	// Touching an administradora.
	expect(t, do(t, srv, req{method: http.MethodPut, path: "/users/" + admID, token: sindico, body: map[string]any{"roles": []string{"morador"}}}),
		http.StatusForbidden, "síndico demotes administradora")
	expect(t, do(t, srv, req{method: http.MethodPut, path: "/users/" + admID, token: sindico, body: map[string]any{"username": "owned"}}),
		http.StatusForbidden, "síndico renames administradora")
	expect(t, do(t, srv, req{method: http.MethodDelete, path: "/users/" + admID, token: sindico}), http.StatusForbidden, "síndico removes administradora")
	expect(t, do(t, srv, req{method: http.MethodGet, path: "/users/me", token: adm}), http.StatusOK, "administradora still there")

	// The administradora can do all of it.
	created := create(t, srv, adm, "/users", map[string]any{"username": "co", "email": "co@exemplo.test", "password": password, "roles": []string{"administradora"}})
	expect(t, do(t, srv, req{method: http.MethodDelete, path: "/users/" + created["id"].(string), token: adm}), http.StatusNoContent, "administradora removes administradora")
	// Unknown roles are refused whoever asks.
	expect(t, do(t, srv, req{method: http.MethodPost, path: "/users", token: adm, body: map[string]any{
		"username": "y", "email": "y@exemplo.test", "password": password, "roles": []string{"ROLE_ADMIN"},
	}}), http.StatusUnprocessableEntity, "retired role name")
}

// One person, two condominiums, two sets of roles.
func TestMemberships_RolesPerCondominium(t *testing.T) {
	e := newEnv(t)
	srv := e.server(condoA)
	adminA := adminToken(t, e, srv, condoA)

	// Maria lives in A (morador) and is síndica of B.
	register(t, srv, "maria", "maria@exemplo.test")
	if _, err := e.db.Admin.Exec(context.Background(),
		`INSERT INTO memberships (user_id, condominium_id, roles) SELECT id, $1, '{sindico}' FROM users WHERE email = 'maria@exemplo.test'`, condoB); err != nil {
		t.Fatal(err)
	}

	// Without saying which condominium, the login lists them (after the password check).
	resp := do(t, srv, req{method: http.MethodPost, path: "/auth/login", body: map[string]any{"email": "maria@exemplo.test", "password": password}})
	expect(t, resp, http.StatusConflict, "login without condominiumId")
	list, _ := decode(t, resp)["condominiums"].([]any)
	if len(list) != 2 {
		t.Fatalf("expected the 2 condominiums of Maria, got %v", list)
	}
	// A wrong password gets no list.
	resp = do(t, srv, req{method: http.MethodPost, path: "/auth/login", body: map[string]any{"email": "maria@exemplo.test", "password": password + "x"}})
	expect(t, resp, http.StatusUnauthorized, "wrong password")

	loginTo := func(condo string) (string, int) {
		lr := do(t, srv, req{method: http.MethodPost, path: "/auth/login", body: map[string]any{"email": "maria@exemplo.test", "password": password, "condominiumId": condo}})
		if lr.StatusCode != http.StatusOK {
			return "", lr.StatusCode
		}
		return decode(t, lr)["accessToken"].(string), lr.StatusCode
	}
	inA, _ := loginTo(condoA)
	inB, _ := loginTo(condoB)
	if _, code := loginTo("c0000000-0000-4000-8000-00000000000c"); code != http.StatusUnauthorized {
		t.Errorf("a condominium Maria does not belong to: got %d, want 401", code)
	}

	if got := status(t, srv, http.MethodGet, "/users", inA, nil); got != http.StatusForbidden {
		t.Errorf("Maria is only a morador in A: GET /users got %d, want 403", got)
	}
	if got := status(t, srv, http.MethodGet, "/users", inB, nil); got != http.StatusOK {
		t.Errorf("Maria is síndica in B: GET /users got %d, want 200", got)
	}
	for tok, want := range map[string]string{inA: condoA, inB: condoB} {
		vr := do(t, srv, req{method: http.MethodGet, path: "/condominium", token: tok})
		expect(t, vr, http.StatusOK, "current condominium")
		if id := decode(t, vr)["id"]; id != want {
			t.Errorf("session in %s reports %v", want, id)
		}
	}
	// B's list holds Maria only; A's síndico does not see B's members, nor does
	// removing Maria from A take her out of B.
	if total := decode(t, do(t, srv, req{method: http.MethodGet, path: "/users", token: inB}))["total"]; total != float64(1) {
		t.Errorf("B has %v members, want 1", total)
	}
	resp = do(t, srv, req{method: http.MethodGet, path: "/users/me", token: inA})
	mariaID, _ := decode(t, resp)["id"].(string)
	expect(t, do(t, srv, req{method: http.MethodDelete, path: "/users/" + mariaID, token: adminA}), http.StatusNoContent, "A removes Maria")
	if _, code := loginTo(condoA); code != http.StatusUnauthorized {
		t.Errorf("Maria no longer belongs to A: got %d, want 401", code)
	}
	// Alone now, she logs in to B without naming it.
	resp = do(t, srv, req{method: http.MethodPost, path: "/auth/login", body: map[string]any{"email": "maria@exemplo.test", "password": password}})
	expect(t, resp, http.StatusOK, "Maria is still in B")
	expect(t, do(t, srv, req{method: http.MethodGet, path: "/users/me", token: inB}), http.StatusOK, "her session in B is intact")
}

// ── audit trail ──────────────────────────────────────────────────────────────

type auditEntry struct {
	Action, Resource, ResourceID, IP, UserAgent, RequestID, ActorUserID string
	ActorRoles                                                          []string
	Before, After                                                       map[string]any
}

func auditLog(t *testing.T, srv *httptest.Server, token, query string) (entries []auditEntry, total float64) {
	t.Helper()
	resp := do(t, srv, req{method: http.MethodGet, path: "/audit-log?pageSize=100" + query, token: token})
	expect(t, resp, http.StatusOK, "audit log")
	body := decode(t, resp)
	total, _ = body["total"].(float64)
	items, _ := body["items"].([]any)
	for _, it := range items {
		m := it.(map[string]any)
		str := func(k string) string { s, _ := m[k].(string); return s }
		en := auditEntry{Action: str("action"), Resource: str("resource"), ResourceID: str("resourceId"), IP: str("ip"),
			UserAgent: str("userAgent"), RequestID: str("requestId"), ActorUserID: str("actorUserId")}
		for _, r := range m["actorRoles"].([]any) {
			en.ActorRoles = append(en.ActorRoles, r.(string))
		}
		en.Before, _ = m["before"].(map[string]any)
		en.After, _ = m["after"].(map[string]any)
		entries = append(entries, en)
	}
	return entries, total
}

// Every change to a cadastro writes an entry: who, what, when, from where,
// before and after.
func TestAudit_EveryCadastroChangeIsRecorded(t *testing.T) {
	e := newEnv(t)
	srv := e.server(condoA)
	admin := adminToken(t, e, srv, condoA)
	me := decode(t, do(t, srv, req{method: http.MethodGet, path: "/users/me", token: admin}))
	adminID := me["id"].(string)

	call := func(method, path string, body any) map[string]any {
		resp := do(t, srv, req{method: method, path: path, token: admin, body: body})
		if resp.StatusCode >= 300 {
			t.Fatalf("%s %s: %d", method, path, resp.StatusCode)
		}
		if resp.StatusCode == http.StatusNoContent {
			return nil
		}
		return decode(t, resp)
	}

	unit := call("POST", "/residents", map[string]any{"unitNumber": "101", "floor": 1, "condominiumId": condoA, "ownerId": adminID})
	unitID := unit["id"].(string)
	call("PUT", "/residents/"+unitID, map[string]any{"floor": 4})
	shop := call("POST", "/shop-owners", map[string]any{"shopName": "Farmácia", "cnpj": "11222333000181", "floor": 0, "ownerId": adminID})
	shopID := shop["id"].(string)
	call("PUT", "/shop-owners/"+shopID, map[string]any{"shopName": "Farmácia Central"})
	exp := call("POST", "/expenses", map[string]any{"description": "Água", "amountCents": 1000, "dueDate": "2026-12-10T00:00:00Z", "residentId": unitID})
	expID := exp["id"].(string)
	call("PUT", "/expenses/"+expID, map[string]any{"amountCents": 2500})
	notif := call("POST", "/notifications", map[string]any{"message": "Assembleia", "residentIds": []string{unitID}})
	notifID := notif["id"].(string)
	call("PUT", "/notifications/"+notifID, map[string]any{"message": "Assembleia adiada"})
	usr := call("POST", "/users", map[string]any{"username": "joao", "email": "joao@exemplo.test", "password": password, "roles": []string{"morador"}})
	usrID := usr["id"].(string)
	call("PUT", "/users/"+usrID, map[string]any{"username": "joao2", "roles": []string{"porteiro"}})

	for _, d := range []string{"/notifications/" + notifID, "/expenses/" + expID, "/shop-owners/" + shopID, "/residents/" + unitID, "/users/" + usrID} {
		call("DELETE", d, nil)
	}

	type key struct{ resource, action string }
	got := map[key]int{}
	entries, total := auditLog(t, srv, admin, "")
	if int(total) != len(entries) {
		t.Fatalf("total %v but %d items", total, len(entries))
	}
	for _, en := range entries {
		got[key{en.Resource, en.Action}]++
	}
	// 5 resources × (create, update, delete) + the admin created by the fixture is outside the API.
	for _, res := range []string{"resident", "shop_owner", "expense", "notification", "user"} {
		for _, act := range []string{"create", "update", "delete"} {
			if got[key{res, act}] < 1 {
				t.Errorf("no audit entry for %s %s (have %v)", act, res, got)
			}
		}
	}

	// Who, from where, before and after.
	upd, _ := auditLog(t, srv, admin, "&resource=resident&resourceId="+unitID)
	if len(upd) != 3 {
		t.Fatalf("expected 3 entries for the unit (delete, update, create — newest first), got %d", len(upd))
	}
	del, mid, cre := upd[0], upd[1], upd[2]
	if del.Action != "delete" || mid.Action != "update" || cre.Action != "create" {
		t.Fatalf("order: %s, %s, %s", del.Action, mid.Action, cre.Action)
	}
	if mid.ActorUserID != adminID || len(mid.ActorRoles) != 1 || mid.ActorRoles[0] != authz.RoleSindico {
		t.Errorf("actor: %s %v", mid.ActorUserID, mid.ActorRoles)
	}
	if mid.IP == "" || mid.RequestID == "" || !strings.Contains(mid.UserAgent, "Go-http-client") {
		t.Errorf("origin missing: ip=%q requestId=%q ua=%q", mid.IP, mid.RequestID, mid.UserAgent)
	}
	if mid.Before["floor"] != float64(1) || mid.After["floor"] != float64(4) {
		t.Errorf("update must carry before/after: %v → %v", mid.Before, mid.After)
	}
	if cre.Before != nil || cre.After["unitNumber"] != "101" {
		t.Errorf("create: before=%v after=%v", cre.Before, cre.After)
	}
	if del.After != nil || del.Before["floor"] != float64(4) {
		t.Errorf("delete: before=%v after=%v", del.Before, del.After)
	}

	// No secret ever reaches the trail.
	raw, _ := auditLog(t, srv, admin, "&resource=user")
	for _, en := range raw {
		for _, side := range []map[string]any{en.Before, en.After} {
			for k, v := range side {
				if strings.Contains(strings.ToLower(k), "password") || strings.Contains(fmt.Sprint(v), "$2a$") {
					t.Errorf("secret in audit entry: %s=%v", k, v)
				}
			}
		}
	}
}

// A change that fails leaves no entry, and the trail cannot be edited over HTTP.
func TestAudit_FailedChangesLeaveNoEntryAndTheTrailIsReadOnly(t *testing.T) {
	e := newEnv(t)
	srv := e.server(condoA)
	admin := adminToken(t, e, srv, condoA)
	create(t, srv, admin, "/residents", map[string]any{"unitNumber": "101", "floor": 1, "condominiumId": condoA, "ownerId": "x"})
	_, before := auditLog(t, srv, admin, "")

	expect(t, do(t, srv, req{method: http.MethodPost, path: "/residents", token: admin, body: map[string]any{"unitNumber": "101", "floor": 1, "condominiumId": condoA, "ownerId": "x"}}),
		http.StatusConflict, "duplicate unit")
	expect(t, do(t, srv, req{method: http.MethodPost, path: "/residents", token: admin, body: map[string]any{"floor": 1}}), http.StatusUnprocessableEntity, "invalid unit")
	expect(t, do(t, srv, req{method: http.MethodDelete, path: "/residents/00000000-0000-4000-8000-000000000000", token: admin}), http.StatusNotFound, "unknown unit")
	if _, after := auditLog(t, srv, admin, ""); after != before {
		t.Errorf("failed changes wrote audit entries: %v → %v", before, after)
	}

	for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		if got := status(t, srv, m, "/audit-log", admin, map[string]any{}); got != http.StatusMethodNotAllowed && got != http.StatusNotFound {
			t.Errorf("%s /audit-log: got %d, the trail must be read-only", m, got)
		}
	}
}

// The trail of one condominium is invisible to another, and reads need audit.read.
func TestAudit_PerCondominiumAndOnlyForAuditors(t *testing.T) {
	e := newEnv(t)
	srvA, srvB := e.server(condoA), e.server(condoB)
	adminA, adminB := adminToken(t, e, srvA, condoA), adminToken(t, e, srvB, condoB)
	create(t, srvA, adminA, "/residents", map[string]any{"unitNumber": "101", "floor": 1, "condominiumId": condoA, "ownerId": "x"})
	create(t, srvA, adminA, "/residents", map[string]any{"unitNumber": "102", "floor": 1, "condominiumId": condoA, "ownerId": "x"})

	if entries, total := auditLog(t, srvB, adminB, ""); total != 0 || len(entries) != 0 {
		t.Errorf("B sees %v entries of A", total)
	}
	if _, total := auditLog(t, srvA, adminA, ""); total != 2 {
		t.Errorf("A has %v entries, want 2", total)
	}
	res, _ := resident(t, srvA, "ana")
	porteiro := userWithRole(t, srvA, adminA, authz.RolePorteiro)
	for name, tok := range map[string]string{"morador": res, "porteiro": porteiro} {
		if got := status(t, srvA, http.MethodGet, "/audit-log", tok, nil); got != http.StatusForbidden {
			t.Errorf("%s reads the trail: got %d, want 403", name, got)
		}
	}
	if got := status(t, srvA, http.MethodGet, "/audit-log", "", nil); got != http.StatusUnauthorized {
		t.Errorf("anonymous reads the trail: got %d, want 401", got)
	}
	if got := status(t, srvA, http.MethodGet, "/audit-log?page=0", adminA, nil); got != http.StatusBadRequest {
		t.Errorf("invalid page: got %d, want 400", got)
	}
}
