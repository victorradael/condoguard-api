package parity_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/victorradael/condoguard/api/internal/authz"
	"github.com/victorradael/condoguard/api/internal/seed"
	"github.com/victorradael/condoguard/api/pkg/mailer"
)

// Tests of the 1c feature (FAB-10): blocks, units, people, links with history,
// invitations and the CSV import, over HTTP against PostgreSQL with RLS.

// api wraps one server and one token to keep the tests short.
type api struct {
	t     *testing.T
	srv   *httptest.Server
	token string
}

func (a api) call(method, path string, body any, want int) map[string]any {
	a.t.Helper()
	resp := do(a.t, a.srv, req{method: method, path: path, body: body, token: a.token})
	expect(a.t, resp, want, method+" "+path)
	if resp.StatusCode == http.StatusNoContent {
		return nil
	}
	return decode(a.t, resp)
}

func (a api) status(method, path string, body any) int {
	a.t.Helper()
	return do(a.t, a.srv, req{method: method, path: path, body: body, token: a.token}).StatusCode
}

func (a api) items(path string) []map[string]any {
	a.t.Helper()
	raw, _ := a.call(http.MethodGet, path, nil, http.StatusOK)["items"].([]any)
	out := make([]map[string]any, len(raw))
	for i, r := range raw {
		out[i], _ = r.(map[string]any)
	}
	return out
}

func id(m map[string]any) string { s, _ := m["id"].(string); return s }

// tenantFixture is a condominium with a síndico, one block with units 101 and 102.
type tenantFixture struct {
	e       *env
	srv     *httptest.Server
	mail    *mailer.Memory
	sindico api
	block   string
	unit1   string
	unit2   string
}

func newTenant(t *testing.T) *tenantFixture {
	t.Helper()
	e := newEnv(t)
	mail := &mailer.Memory{}
	srv := e.serverWith(condoA, mail)
	f := &tenantFixture{e: e, srv: srv, mail: mail}
	f.sindico = api{t, srv, adminToken(t, e, srv, condoA)}
	f.block = id(f.sindico.call("POST", "/blocks", map[string]any{"name": "Torre A"}, http.StatusCreated))
	f.unit1 = id(f.sindico.call("POST", "/units", map[string]any{"unitNumber": "101", "floor": 1, "blockId": f.block}, http.StatusCreated))
	f.unit2 = id(f.sindico.call("POST", "/units", map[string]any{"unitNumber": "102", "floor": 1, "blockId": f.block}, http.StatusCreated))
	return f
}

// accountFor invites the person by link, accepts it and returns the logged-in api.
func (f *tenantFixture) accountFor(personID, email string) api {
	f.t().Helper()
	inv := f.sindico.call("POST", "/people/"+personID+"/invitations", map[string]any{"channel": "link"}, http.StatusCreated)
	token, _ := inv["token"].(string)
	resp := do(f.t(), f.srv, req{method: "POST", path: "/auth/invitations/accept", body: map[string]any{"token": token, "username": email, "password": password}})
	expect(f.t(), resp, http.StatusCreated, "accept")
	return api{f.e.t, f.srv, login(f.t(), f.srv, email)}
}

func (f *tenantFixture) t() *testing.T { return f.e.t }

func (f *tenantFixture) person(name, cpf, email string) string {
	f.t().Helper()
	body := map[string]any{"fullName": name, "cpf": cpf, "email": email, "phone": "(11) 90000-1234"}
	return id(f.sindico.call("POST", "/people", body, http.StatusCreated))
}

func (f *tenantFixture) link(unit, person, kind string) map[string]any {
	f.t().Helper()
	return f.sindico.call("POST", "/units/"+unit+"/links", map[string]any{"personId": person, "kind": kind, "startsOn": "2020-01-01"}, http.StatusCreated)
}

const (
	cpfAna  = "529.982.247-25"
	cpfBia  = "111.444.777-35"
	cpfCaio = "390.533.447-05"
)

func TestPeople_BlocksAndUnits(t *testing.T) {
	f := newTenant(t)
	s := f.sindico

	// The same number lives in another block; twice in one block is a conflict.
	b2 := id(s.call("POST", "/blocks", map[string]any{"name": "Torre B"}, http.StatusCreated))
	s.call("POST", "/units", map[string]any{"unitNumber": "101", "blockId": b2}, http.StatusCreated)
	s.call("POST", "/units", map[string]any{"unitNumber": "101", "blockId": f.block}, http.StatusConflict)
	s.call("POST", "/blocks", map[string]any{"name": "Torre A"}, http.StatusConflict)
	s.call("POST", "/blocks", map[string]any{"name": "  "}, http.StatusUnprocessableEntity)

	// A block with units cannot be deleted; an empty one can.
	s.call("DELETE", "/blocks/"+f.block, nil, http.StatusConflict)
	empty := id(s.call("POST", "/blocks", map[string]any{"name": "Torre C"}, http.StatusCreated))
	s.call("DELETE", "/blocks/"+empty, nil, http.StatusNoContent)

	unit := s.call("GET", "/units/"+f.unit1, nil, http.StatusOK)
	if unit["blockName"] != "Torre A" || unit["blockId"] != f.block {
		t.Errorf("unit does not carry its block: %v", unit)
	}
	if got := s.items("/units?blockId=" + b2); len(got) != 1 {
		t.Errorf("filter by block: %v", got)
	}
	s.call("PUT", "/blocks/"+b2, map[string]any{"name": "Torre Bravo"}, http.StatusOK)
	if blocks := s.items("/blocks"); len(blocks) != 2 || blocks[1]["name"] != "Torre Bravo" {
		t.Errorf("blocks: %v", blocks)
	}

	// Another condominium's block is not usable.
	srvB := f.e.server(condoB)
	adminB := api{t, srvB, adminToken(t, f.e, srvB, condoB)}
	foreign := id(adminB.call("POST", "/blocks", map[string]any{"name": "Torre A"}, http.StatusCreated))
	s.call("POST", "/units", map[string]any{"unitNumber": "777", "blockId": foreign}, http.StatusUnprocessableEntity)
	s.call("GET", "/blocks/"+foreign, nil, http.StatusNotFound)
}

func TestPeople_LinksTransfersAndHistory(t *testing.T) {
	f := newTenant(t)
	s := f.sindico
	ana := f.person("Ana Souza", cpfAna, "ana@exemplo.test")
	bia := f.person("Bia Lima", cpfBia, "bia@exemplo.test")
	caio := f.person("Caio Reis", cpfCaio, "caio@exemplo.test")

	owner := f.link(f.unit1, ana, "proprietario")
	if owner["active"] != true {
		t.Errorf("owner link should be active: %v", owner)
	}
	// Two principal owners in the same period: the database refuses.
	s.call("POST", "/units/"+f.unit1+"/links", map[string]any{"personId": bia, "kind": "proprietario", "startsOn": "2021-01-01"}, http.StatusConflict)
	// A co-owner is fine.
	f.link(f.unit1, bia, "coproprietario")
	// The same person twice with the same kind and period: conflict.
	s.call("POST", "/units/"+f.unit1+"/links", map[string]any{"personId": bia, "kind": "coproprietario", "startsOn": "2021-01-01"}, http.StatusConflict)
	s.call("POST", "/units/"+f.unit1+"/links", map[string]any{"personId": caio, "kind": "amigo"}, http.StatusUnprocessableEntity)
	s.call("POST", "/units/"+f.unit1+"/links", map[string]any{"personId": caio, "kind": "morador", "startsOn": "2024-05-01", "endsOn": "2024-05-01"}, http.StatusUnprocessableEntity)

	// The owner changes on 2023-06-15: the old owner and the co-owner end, the new one starts.
	res := s.call("POST", "/units/"+f.unit1+"/transfers", map[string]any{"kind": "proprietario", "effectiveDate": "2023-06-15", "personId": caio}, http.StatusCreated)
	ended, _ := res["ended"].([]any)
	if len(ended) != 2 {
		t.Fatalf("expected the owner and the co-owner to end, got %v", res["ended"])
	}
	created, _ := res["created"].(map[string]any)
	if created["startsOn"] != "2023-06-15" || created["personName"] != "Caio Reis" || created["active"] != true {
		t.Errorf("new owner: %v", created)
	}
	// A swap dated before the current owner started is refused (the history cannot fold on itself).
	s.call("POST", "/units/"+f.unit1+"/transfers", map[string]any{"kind": "proprietario", "effectiveDate": "2022-01-01", "personId": ana}, http.StatusConflict)

	// Tenants swap too, and the history keeps everyone, newest first.
	s.call("POST", "/units/"+f.unit1+"/transfers", map[string]any{"kind": "inquilino", "effectiveDate": "2024-01-10", "person": map[string]any{"fullName": "Dora Alves", "cpf": "168.995.350-09"}}, http.StatusCreated)
	s.call("POST", "/units/"+f.unit1+"/transfers", map[string]any{"kind": "inquilino", "effectiveDate": "2025-02-01", "personId": bia}, http.StatusCreated)
	hist := s.items("/units/" + f.unit1 + "/links")
	if len(hist) != 5 {
		t.Fatalf("history: expected 5 lines, got %d: %v", len(hist), hist)
	}
	if hist[0]["personName"] != "Bia Lima" || hist[0]["kind"] != "inquilino" || hist[0]["active"] != true {
		t.Errorf("newest first: %v", hist[0])
	}
	active := s.items("/units/" + f.unit1 + "/links?active=true")
	if len(active) != 2 {
		t.Errorf("active: expected owner + tenant, got %v", active)
	}
	s.call("POST", "/units/"+f.unit1+"/transfers", map[string]any{"kind": "morador", "personId": ana}, http.StatusUnprocessableEntity)

	// A person who has links cannot be deleted; one who has none can.
	s.call("DELETE", "/people/"+ana, nil, http.StatusConflict)
	loose := f.person("Edu Pires", "", "edu@exemplo.test")
	s.call("DELETE", "/people/"+loose, nil, http.StatusNoContent)

	// Ending a link on its own start day makes no sense.
	linkID := id(f.link(f.unit2, ana, "morador"))
	s.call("PUT", "/links/"+linkID+"/end", map[string]any{"endsOn": "2020-01-01"}, http.StatusUnprocessableEntity)
	s.call("PUT", "/links/"+linkID+"/end", map[string]any{"endsOn": "2022-01-01"}, http.StatusOK)

	// The trail has the swaps.
	audit := s.items("/audit-log?resource=unit_transfer")
	if len(audit) != 3 {
		t.Errorf("audit trail of the swaps: %v", audit)
	}
}

func TestPeople_DataIsShownByNeed(t *testing.T) {
	f := newTenant(t)
	s := f.sindico
	ana := f.person("Ana Souza", cpfAna, "ana@exemplo.test")
	bia := f.person("Bia Lima", cpfBia, "bia@exemplo.test")
	f.link(f.unit1, ana, "proprietario")
	f.link(f.unit2, bia, "proprietario")

	tokenFor := func(role string) api { return api{t, f.srv, userWithRole(t, f.srv, s.token, role)} }
	conselho, porteiro := tokenFor(authz.RoleConselho), tokenFor(authz.RolePorteiro)

	// síndico: everything.
	full := s.call("GET", "/people/"+ana, nil, http.StatusOK)
	if full["cpf"] != "529.982.247-25" || full["email"] != "ana@exemplo.test" || full["phone"] == nil {
		t.Errorf("síndico should see it all: %v", full)
	}
	// conselho: contact data and a masked CPF.
	c := conselho.call("GET", "/people/"+ana, nil, http.StatusOK)
	if c["cpf"] != "***.982.247-**" || c["email"] != "ana@exemplo.test" {
		t.Errorf("conselho: %v", c)
	}
	// porteiro: who lives where, nothing personal.
	p := porteiro.call("GET", "/people/"+ana, nil, http.StatusOK)
	if p["fullName"] != "Ana Souza" || p["cpf"] != nil || p["email"] != nil || p["phone"] != nil || p["hasAccount"] != nil {
		t.Errorf("porteiro sees too much: %v", p)
	}
	if units, _ := p["units"].([]any); len(units) != 1 {
		t.Errorf("porteiro should see the unit of the person: %v", p)
	}
	for _, it := range porteiro.items("/people") {
		if it["cpf"] != nil || it["email"] != nil || it["phone"] != nil {
			t.Errorf("porteiro list leaks: %v", it)
		}
	}
	// Only people.write holders change anything.
	for _, who := range []api{conselho, porteiro} {
		who.call("POST", "/people", map[string]any{"fullName": "X"}, http.StatusForbidden)
		who.call("PUT", "/people/"+ana, map[string]any{"fullName": "X"}, http.StatusForbidden)
		who.call("DELETE", "/people/"+ana, nil, http.StatusForbidden)
		who.call("POST", "/units/"+f.unit1+"/links", map[string]any{"personId": ana, "kind": "morador"}, http.StatusForbidden)
		who.call("POST", "/units/"+f.unit1+"/transfers", map[string]any{"kind": "inquilino", "personId": ana}, http.StatusForbidden)
		who.call("POST", "/people/"+ana+"/invitations", nil, http.StatusForbidden)
		who.call("POST", "/blocks", map[string]any{"name": "X"}, http.StatusForbidden)
		who.call("POST", "/imports", nil, http.StatusForbidden)
	}
	// The conselho reads the whole history; the porteiro only who is there now.
	f.link(f.unit1, bia, "morador")
	if got := conselho.items("/units/" + f.unit1 + "/links"); len(got) != 2 {
		t.Errorf("conselho history: %v", got)
	}
}

func TestPeople_ResidentSeesOnlyTheirHousehold(t *testing.T) {
	f := newTenant(t)
	s := f.sindico
	ana := f.person("Ana Souza", cpfAna, "ana@exemplo.test")
	bia := f.person("Bia Lima", cpfBia, "bia@exemplo.test")
	caio := f.person("Caio Reis", cpfCaio, "caio@exemplo.test")
	f.link(f.unit1, ana, "proprietario")
	f.link(f.unit1, bia, "morador")
	f.link(f.unit2, caio, "proprietario") // someone else's home

	anaAPI := f.accountFor(ana, "ana@exemplo.test")

	// Her own data in full, the household with contact but no CPF, nobody else.
	self := anaAPI.call("GET", "/people/"+ana, nil, http.StatusOK)
	if self["cpf"] != "529.982.247-25" {
		t.Errorf("own CPF: %v", self)
	}
	mate := anaAPI.call("GET", "/people/"+bia, nil, http.StatusOK)
	if mate["cpf"] != nil || mate["email"] != "bia@exemplo.test" {
		t.Errorf("household member: %v", mate)
	}
	anaAPI.call("GET", "/people/"+caio, nil, http.StatusNotFound)
	list := anaAPI.items("/people")
	if len(list) != 2 {
		t.Errorf("list should hold her household only: %v", list)
	}
	for _, it := range list {
		if it["fullName"] == "Caio Reis" || it["cpf"] != nil && it["id"] != ana {
			t.Errorf("list leaks: %v", it)
		}
	}
	if got := anaAPI.items("/people?unitId=" + f.unit2); len(got) != 0 {
		t.Errorf("asking for another unit must show nothing: %v", got)
	}
	// Units: hers yes, the other one is "not found", like a unit that does not exist.
	anaAPI.call("GET", "/units/"+f.unit1, nil, http.StatusOK)
	anaAPI.call("GET", "/units/"+f.unit2, nil, http.StatusNotFound)
	anaAPI.call("GET", "/units/"+f.unit2+"/links", nil, http.StatusNotFound)
	anaAPI.call("POST", "/units/"+f.unit2+"/links", map[string]any{"person": map[string]any{"fullName": "Intruso"}, "kind": "morador"}, http.StatusNotFound)
	if got := anaAPI.items("/units"); len(got) != 1 {
		t.Errorf("her units: %v", got)
	}

	// She is responsible for the unit: she adds a dependent (new person) and ends it.
	dep := anaAPI.call("POST", "/units/"+f.unit1+"/links", map[string]any{
		"kind": "dependente", "person": map[string]any{"fullName": "Duda Souza"},
	}, http.StatusCreated)
	if dep["personName"] != "Duda Souza" || dep["kind"] != "dependente" {
		t.Errorf("dependent: %v", dep)
	}
	// ...but cannot name people by id, create owners, tenants or touch the owner link.
	anaAPI.call("POST", "/units/"+f.unit1+"/links", map[string]any{"personId": caio, "kind": "morador"}, http.StatusForbidden)
	anaAPI.call("POST", "/units/"+f.unit1+"/links", map[string]any{"person": map[string]any{"fullName": "Novo Dono"}, "kind": "proprietario"}, http.StatusForbidden)
	anaAPI.call("POST", "/units/"+f.unit1+"/transfers", map[string]any{"kind": "proprietario", "personId": caio}, http.StatusForbidden)
	hist := anaAPI.items("/units/" + f.unit1 + "/links")
	ownerLink := ""
	for _, l := range hist {
		if l["kind"] == "proprietario" {
			ownerLink = id(l)
		}
	}
	anaAPI.call("PUT", "/links/"+ownerLink+"/end", nil, http.StatusForbidden)
	// Added today, so it can be taken back; a link that already started is only ended.
	anaAPI.call("DELETE", "/links/"+id(dep), nil, http.StatusNoContent)
	anaAPI.call("DELETE", "/links/"+ownerLink, nil, http.StatusForbidden)
	dep2 := anaAPI.call("POST", "/units/"+f.unit1+"/links", map[string]any{"kind": "dependente", "startsOn": "2020-01-01", "person": map[string]any{"fullName": "Duda Souza"}}, http.StatusCreated)
	anaAPI.call("DELETE", "/links/"+id(dep2), nil, http.StatusConflict)
	anaAPI.call("PUT", "/links/"+id(dep2)+"/end", nil, http.StatusOK)
	// A CPF already registered is not revealed to a non-writer, only refused.
	anaAPI.call("POST", "/units/"+f.unit1+"/links", map[string]any{"kind": "morador", "person": map[string]any{"fullName": "Caio", "cpf": cpfCaio}}, http.StatusConflict)

	// A resident who is not responsible (Bia, "morador") cannot register anyone.
	biaAPI := f.accountFor(bia, "bia@exemplo.test")
	biaAPI.call("POST", "/units/"+f.unit1+"/links", map[string]any{"kind": "dependente", "person": map[string]any{"fullName": "Eli"}}, http.StatusForbidden)
	biaAPI.call("GET", "/units/"+f.unit1+"/links", nil, http.StatusOK)

	// My profile and my units.
	prof := anaAPI.call("GET", "/me/profile", nil, http.StatusOK)
	units, _ := prof["units"].([]any)
	if len(units) != 1 {
		t.Fatalf("profile units: %v", prof)
	}
	if u, _ := units[0].(map[string]any); u["canManageHousehold"] != true || u["unitNumber"] != "101" || u["blockName"] != "Torre A" {
		t.Errorf("my unit: %v", u)
	}
	if mine := biaAPI.call("GET", "/me/units", nil, http.StatusOK); mine["items"].([]any)[0].(map[string]any)["canManageHousehold"] != false {
		t.Errorf("a plain resident does not manage the household: %v", mine)
	}
	upd := anaAPI.call("PUT", "/me/profile", map[string]any{"fullName": "Ana S. Souza", "phone": "11 91111-2222"}, http.StatusOK)
	if p, _ := upd["person"].(map[string]any); p["fullName"] != "Ana S. Souza" || p["phone"] != "11911112222" || p["cpf"] != "529.982.247-25" {
		t.Errorf("profile update: %v", upd)
	}
	// The síndico has no person record: no profile.
	s.call("GET", "/me/profile", nil, http.StatusNotFound)

	// When the ownership changes the old owner loses access to the unit at once.
	s.call("POST", "/units/"+f.unit1+"/transfers", map[string]any{"kind": "proprietario", "effectiveDate": time.Now().Format("2006-01-02"), "personId": caio}, http.StatusCreated)
	anaAPI.call("GET", "/units/"+f.unit1, nil, http.StatusNotFound)
	anaAPI.call("GET", "/people/"+bia, nil, http.StatusNotFound)
}

var tokenRE = regexp.MustCompile(`[A-Z2-7]{4}(-[A-Z2-7]{4}){7}`)

func TestPeople_InvitationFlow(t *testing.T) {
	f := newTenant(t)
	s := f.sindico
	ana := f.person("Ana Souza", cpfAna, "ana@exemplo.test")
	noMail := f.person("Sem Email", "", "")
	f.link(f.unit1, ana, "proprietario")

	s.call("POST", "/people/"+noMail+"/invitations", nil, http.StatusUnprocessableEntity)

	// Channel e-mail: the token goes to the person only.
	inv := s.call("POST", "/people/"+ana+"/invitations", map[string]any{"channel": "email"}, http.StatusCreated)
	if inv["token"] != nil || inv["link"] != nil {
		t.Fatalf("an e-mail invitation must not hand the token to the síndico: %v", inv)
	}
	msgs := f.mail.To("ana@exemplo.test")
	if len(msgs) != 1 || !strings.Contains(msgs[0].Body, "https://app.exemplo.test/convite?token=") {
		t.Fatalf("invitation e-mail: %v", msgs)
	}
	token := tokenRE.FindString(msgs[0].Body)
	if token == "" {
		t.Fatalf("no token in the e-mail: %s", msgs[0].Body)
	}

	info := do(t, f.srv, req{method: "POST", path: "/auth/invitations/inspect", body: map[string]any{"token": token}})
	expect(t, info, http.StatusOK, "inspect")
	if m := decode(t, info); m["personName"] != "Ana Souza" || m["accountExists"] != false || m["email"] != "ana@exemplo.test" {
		t.Errorf("inspect: %v", m)
	}
	expect(t, do(t, f.srv, req{method: "POST", path: "/auth/invitations/inspect", body: map[string]any{"token": "AAAA-AAAA-AAAA-AAAA-AAAA-AAAA-AAAA-AAAA"}}), http.StatusBadRequest, "unknown token")
	expect(t, do(t, f.srv, req{method: "POST", path: "/auth/invitations/inspect", body: map[string]any{"token": "x"}}), http.StatusBadRequest, "malformed token")

	// A weak password does not burn the invitation.
	expect(t, do(t, f.srv, req{method: "POST", path: "/auth/invitations/accept", body: map[string]any{"token": token, "password": "123"}}), http.StatusUnprocessableEntity, "weak password")
	resp := do(t, f.srv, req{method: "POST", path: "/auth/invitations/accept", body: map[string]any{"token": token, "username": "Ana", "password": password}})
	expect(t, resp, http.StatusCreated, "accept")
	if m := decode(t, resp); m["condominiumId"] != condoA || m["verifyEmail"] != false {
		t.Errorf("accept result: %v", m)
	}
	// Single use.
	expect(t, do(t, f.srv, req{method: "POST", path: "/auth/invitations/accept", body: map[string]any{"token": token, "username": "Ana2", "password": password}}), http.StatusBadRequest, "second use")

	// The account works, is a morador, and is tied to the person.
	anaAPI := api{t, f.srv, login(t, f.srv, "ana@exemplo.test")}
	me := anaAPI.call("GET", "/users/me", nil, http.StatusOK)
	if roles, _ := me["roles"].([]any); len(roles) != 1 || roles[0] != authz.RoleMorador {
		t.Errorf("roles: %v", me)
	}
	if p := s.call("GET", "/people/"+ana, nil, http.StatusOK); p["hasAccount"] != true {
		t.Errorf("person should show an account: %v", p)
	}
	s.call("POST", "/people/"+ana+"/invitations", nil, http.StatusConflict)
	list := s.items("/people/" + ana + "/invitations")
	if len(list) != 1 || list[0]["status"] != "accepted" {
		t.Errorf("invitation status: %v", list)
	}

	// Channel link: returned once; a new invitation revokes the previous one;
	// an expired one stops working; the e-mail is not proven so a confirmation is sent.
	bia := f.person("Bia Lima", cpfBia, "bia@exemplo.test")
	first := s.call("POST", "/people/"+bia+"/invitations", map[string]any{"channel": "link"}, http.StatusCreated)
	second := s.call("POST", "/people/"+bia+"/invitations", map[string]any{"channel": "link"}, http.StatusCreated)
	if first["token"] == second["token"] || !strings.HasPrefix(second["link"].(string), "https://app.exemplo.test/convite?token=") {
		t.Fatalf("link invitations: %v / %v", first, second)
	}
	expect(t, do(t, f.srv, req{method: "POST", path: "/auth/invitations/accept", body: map[string]any{"token": first["token"], "username": "Bia", "password": password}}), http.StatusBadRequest, "revoked")
	if _, err := f.e.db.Admin.Exec(context.Background(), `UPDATE invitations SET expires_at = now() - interval '1 minute' WHERE person_id = $1 AND revoked_at IS NULL`, bia); err != nil {
		t.Fatal(err)
	}
	expect(t, do(t, f.srv, req{method: "POST", path: "/auth/invitations/accept", body: map[string]any{"token": second["token"], "username": "Bia", "password": password}}), http.StatusBadRequest, "expired")
	third := s.call("POST", "/people/"+bia+"/invitations", map[string]any{"channel": "link"}, http.StatusCreated)
	resp = do(t, f.srv, req{method: "POST", path: "/auth/invitations/accept", body: map[string]any{"token": third["token"], "username": "Bia", "password": password}})
	expect(t, resp, http.StatusCreated, "accept link")
	if m := decode(t, resp); m["verifyEmail"] != true {
		t.Errorf("a link invitation must ask for e-mail confirmation: %v", m)
	}
	var got []mailer.Message
	for i := 0; i < 50 && len(got) == 0; i++ { // the confirmation goes out in the background
		time.Sleep(20 * time.Millisecond)
		got = f.mail.To("bia@exemplo.test")
	}
	if len(got) != 1 || !strings.Contains(got[0].Subject, "Confirme") {
		t.Errorf("confirmation e-mail: %v", got)
	}

	// An e-mail that already has an account asks for its password instead of creating one.
	caio := f.person("Caio Reis", cpfCaio, "ana@exemplo.test") // same login as Ana, another condominium person
	inv3 := s.call("POST", "/people/"+caio+"/invitations", map[string]any{"channel": "link"}, http.StatusCreated)
	info = do(t, f.srv, req{method: "POST", path: "/auth/invitations/inspect", body: map[string]any{"token": inv3["token"]}})
	expect(t, info, http.StatusOK, "inspect existing")
	if m := decode(t, info); m["accountExists"] != true {
		t.Errorf("inspect should say the account exists: %v", m)
	}
	expect(t, do(t, f.srv, req{method: "POST", path: "/auth/invitations/accept", body: map[string]any{"token": inv3["token"], "password": "Wrong-Pass-1234"}}), http.StatusUnauthorized, "wrong password")
	// In the same condominium that account is already tied to Ana's person, so Caio cannot take it;
	// the refused attempt rolls back and leaves the invitation as it was.
	expect(t, do(t, f.srv, req{method: "POST", path: "/auth/invitations/accept", body: map[string]any{"token": inv3["token"], "password": password}}), http.StatusConflict, "account tied to another person")
	expect(t, do(t, f.srv, req{method: "POST", path: "/auth/invitations/inspect", body: map[string]any{"token": inv3["token"]}}), http.StatusOK, "still pending")
}

func TestPeople_Import(t *testing.T) {
	f := newTenant(t)
	s := f.sindico
	postCSV := func(path, body string, want int) map[string]any {
		t.Helper()
		resp := do(t, f.srv, req{method: "POST", path: path, token: s.token, rawBody: strings.NewReader(body)})
		expect(t, resp, want, "POST "+path)
		return decode(t, resp)
	}
	good := "bloco;unidade;andar;papel;nome;cpf;email;telefone;inicio\n" +
		"Torre A;101;1;proprietario;Ana Souza;529.982.247-25;ana@exemplo.test;(11) 90000-1111;2020-01-01\n" + // existing unit
		"Torre A;201;2;proprietario;Bia Lima;111.444.777-35;bia@exemplo.test;11 90000-2222;15/03/2021\n" +
		"Torre A;201;2;dependente;Duda Lima;;;;\n" +
		"Torre B;11;1;inquilino;Caio Reis;390.533.447-05;;;2022-05-01\n" +
		"Torre B;12;1;;;;;;\n" // a unit alone

	prev := postCSV("/imports/preview", good, http.StatusOK)
	if prev["valid"] != true || prev["newBlocks"] != float64(1) || prev["newUnits"] != float64(3) || prev["newPeople"] != float64(4) || prev["newLinks"] != float64(4) {
		t.Fatalf("preview: %v", prev)
	}
	if units := s.items("/units"); len(units) != 2 {
		t.Fatalf("a preview must not save anything, units: %d", len(units))
	}

	rep := postCSV("/imports", good, http.StatusCreated)
	if rep["valid"] != true {
		t.Fatalf("import: %v", rep)
	}
	if units := s.items("/units?pageSize=100"); len(units) != 5 {
		t.Errorf("units after import: %d", len(units))
	}
	if people := s.items("/people?pageSize=100"); len(people) != 4 {
		t.Errorf("people after import: %d", len(people))
	}
	// Importing the same file again changes nothing (people with CPF or e-mail are matched).
	again := postCSV("/imports", good, http.StatusCreated)
	if again["newLinks"] != float64(0) || again["newPeople"] != float64(0) || again["newUnits"] != float64(0) || again["unchanged"].(float64) < 4 {
		t.Errorf("re-import: %v", again)
	}
	if people := s.items("/people?pageSize=100"); len(people) != 4 {
		t.Errorf("re-import duplicated people: %d", len(people))
	}

	// A file with errors reports every bad line and saves nothing.
	bad := "unidade,papel,nome,cpf,email,telefone,inicio\n" +
		"301,proprietario,Fulano de Tal,123.456.789-00,,,\n" + // bad CPF
		"302,sindico,Beltrano,,,,\n" + // unknown role
		"303,morador,,,,,\n" + // no name
		"304,morador,Ciclano,,nao-e-email,,\n" + // bad e-mail
		",morador,Sem Unidade,,,,\n" + // no unit
		"305,proprietario,Dono Um,,,,2021-01-01\n" +
		"305,proprietario,Dono Dois,,,,2022-01-01\n" + // second principal owner
		"306,morador,Certo,,,,31/02/2021\n" + // impossible date
		"307,morador,Bom,,bom@exemplo.test,,2021-01-01\n"
	resp := postCSV("/imports", bad, http.StatusUnprocessableEntity)
	report, _ := resp["report"].(map[string]any)
	errs, _ := report["errors"].([]any)
	lines := map[float64]string{}
	for _, e := range errs {
		m := e.(map[string]any)
		lines[m["line"].(float64)] = m["field"].(string)
	}
	want := map[float64]string{2: "cpf", 3: "papel", 4: "nome", 5: "email", 6: "unidade", 8: "papel", 9: "inicio"}
	for line, field := range want {
		if lines[line] != field {
			t.Errorf("line %v: expected an error on %q, got %q (all: %v)", line, field, lines[line], lines)
		}
	}
	if len(lines) != len(want) {
		t.Errorf("unexpected error lines: %v", lines)
	}
	if units := s.items("/units?pageSize=100"); len(units) != 5 {
		t.Errorf("a failed import must save nothing, units: %d", len(units))
	}
	// Garbage in the header and in the encoding.
	postCSV("/imports/preview", "nome;cpf\nAna;1\n", http.StatusUnprocessableEntity)
	postCSV("/imports/preview", "", http.StatusUnprocessableEntity)
	postCSV("/imports/preview", "unidade\n\xff\xfe\n", http.StatusUnprocessableEntity)
}

func TestPeople_CrossTenantIsolation(t *testing.T) {
	f := newTenant(t)
	ana := f.person("Ana Souza", cpfAna, "ana@exemplo.test")
	f.link(f.unit1, ana, "proprietario")

	srvB := f.e.server(condoB)
	adminB := api{t, srvB, adminToken(t, f.e, srvB, condoB)}
	for _, p := range []string{"/people/" + ana, "/units/" + f.unit1, "/blocks/" + f.block, "/units/" + f.unit1 + "/links"} {
		adminB.call("GET", p, nil, http.StatusNotFound)
	}
	adminB.call("PUT", "/people/"+ana, map[string]any{"fullName": "Hack"}, http.StatusNotFound)
	adminB.call("DELETE", "/people/"+ana, nil, http.StatusNotFound)
	adminB.call("POST", "/people/"+ana+"/invitations", nil, http.StatusNotFound)
	adminB.call("POST", "/units/"+f.unit1+"/links", map[string]any{"personId": ana, "kind": "morador"}, http.StatusNotFound)
	adminB.call("POST", "/units/"+f.unit1+"/transfers", map[string]any{"kind": "inquilino", "personId": ana}, http.StatusNotFound)
	if got := adminB.items("/people"); len(got) != 0 {
		t.Errorf("condominium B sees people of A: %v", got)
	}
	// The same CPF may exist in another condominium.
	adminB.call("POST", "/people", map[string]any{"fullName": "Ana Souza", "cpf": cpfAna}, http.StatusCreated)
}

// ── the fictitious data of FAB-10 through the API ────────────────────────────

const seedPassword = "Demo-senha-de-teste-2026"

func TestSeedPoC_SpreadsheetImportsWithoutErrors(t *testing.T) {
	f := newTenant(t) // condominium A: a síndico and two units
	ds, err := seed.Generate(seed.Profiles["poc"], seed.DefaultSeed)
	if err != nil {
		t.Fatal(err)
	}
	var sheet strings.Builder
	if err = ds.WriteImportCSV(&sheet); err != nil {
		t.Fatal(err)
	}
	// Condominium B is empty: the spreadsheet is the initial import.
	srvB := f.e.server(condoB)
	adminB := api{t, srvB, adminToken(t, f.e, srvB, condoB)}
	postB := func(path, body string, want int) map[string]any {
		t.Helper()
		resp := do(t, srvB, req{method: "POST", path: path, token: adminB.token, rawBody: strings.NewReader(body)})
		expect(t, resp, want, "POST "+path)
		return decode(t, resp)
	}

	prev := postB("/imports/preview", sheet.String(), http.StatusOK)
	if prev["valid"] != true || prev["errorCount"] != float64(0) || prev["newUnits"] != float64(120) || prev["newBlocks"] != float64(3) {
		t.Fatalf("preview of the PoC spreadsheet: %v", prev)
	}
	rep := postB("/imports", sheet.String(), http.StatusCreated)
	if rep["errorCount"] != float64(0) || rep["newUnits"] != float64(120) {
		t.Fatalf("import of the PoC spreadsheet: %v", rep)
	}
	units := 0
	for page := 1; ; page++ {
		got := adminB.call("GET", "/units?pageSize=100&sort=number&page="+strconv.Itoa(page), nil, http.StatusOK)
		items, _ := got["items"].([]any)
		units += len(items)
		if len(items) < 100 {
			break
		}
	}
	if units != 120 {
		t.Errorf("units after the import: %d", units)
	}
	current := 0
	for _, l := range ds.Links {
		if l.Ends == nil {
			current++
		}
	}
	if rep["newLinks"] != float64(current) {
		t.Errorf("links: expected %d (the current ones), got %v", current, rep["newLinks"])
	}

	// The spoiled spreadsheet: a report by line, nothing saved.
	var broken strings.Builder
	if err = ds.WriteBrokenCSV(&broken); err != nil {
		t.Fatal(err)
	}
	post := func(path, body string, want int) map[string]any {
		t.Helper()
		resp := do(t, f.srv, req{method: "POST", path: path, token: f.sindico.token, rawBody: strings.NewReader(body)})
		expect(t, resp, want, "POST "+path)
		return decode(t, resp)
	}
	resp := post("/imports/preview", broken.String(), http.StatusOK) // condominium A: the units are new there too
	if resp["valid"] != false {
		t.Fatalf("the spoiled spreadsheet must not be valid: %v", resp)
	}
	errs, _ := resp["errors"].([]any)
	if len(errs) < 6 {
		t.Errorf("expected at least 6 problems reported by line, got %d: %v", len(errs), errs)
	}
	for _, e := range errs {
		m := e.(map[string]any)
		if m["line"].(float64) < 2 || m["message"] == "" {
			t.Errorf("report line: %v", m)
		}
	}
	post("/imports", broken.String(), http.StatusUnprocessableEntity)
	if got := f.sindico.items("/units?pageSize=100"); len(got) != 2 {
		t.Errorf("a refused import must save nothing, units: %d", len(got))
	}
}

func TestSeedPoC_ResidentSeesNothingOfOtherUnits(t *testing.T) {
	e := newEnv(t)
	ds, err := seed.Generate(seed.Profiles["poc"], seed.DefaultSeed)
	if err != nil {
		t.Fatal(err)
	}
	if err = seed.Load(context.Background(), e.db.Pool, ds, seedPassword); err != nil {
		t.Fatal(err)
	}
	srv := e.server(ds.CondominiumID.String())
	loginAs := func(email string) api {
		resp := do(t, srv, req{method: "POST", path: "/auth/login", body: map[string]any{"email": email, "password": seedPassword}})
		expect(t, resp, http.StatusOK, "login "+email)
		tok, _ := decode(t, resp)["accessToken"].(string)
		return api{t, srv, tok}
	}
	morador, sindico, porteiro := loginAs("morador@exemplo.test"), loginAs("sindico@exemplo.test"), loginAs("porteiro@exemplo.test")
	inquilino := loginAs("inquilino@exemplo.test")

	// Her household: the people with an active link in her unit.
	profile := morador.call("GET", "/me/profile", nil, http.StatusOK)
	selfID := profile["person"].(map[string]any)["id"].(string)
	mine := morador.call("GET", "/me/units", nil, http.StatusOK)["items"].([]any)
	if len(mine) != 1 {
		t.Fatalf("morador@ should live in one unit: %v", mine)
	}
	unitID := mine[0].(map[string]any)["unitId"].(string)
	household := map[string]bool{}
	for _, l := range sindico.items("/units/" + unitID + "/links?active=true") {
		household[l["personId"].(string)] = true
	}

	// Every person of the condominium (300): the resident reaches exactly the household, never the CPF of others.
	seen := 0
	for page := 1; ; page++ {
		got := sindico.call("GET", "/people?pageSize=100&page="+strconv.Itoa(page), nil, http.StatusOK)["items"].([]any)
		for _, it := range got {
			pid := it.(map[string]any)["id"].(string)
			status := morador.status("GET", "/people/"+pid, nil)
			if household[pid] {
				if status != http.StatusOK {
					t.Errorf("household member %s: got %d", pid, status)
				}
				seen++
			} else if status != http.StatusNotFound {
				t.Errorf("person %s of another unit: expected 404, got %d", pid, status)
			}
		}
		if len(got) < 100 {
			break
		}
	}
	if seen != len(household) {
		t.Errorf("household members reached: %d of %d", seen, len(household))
	}
	listed := morador.items("/people?pageSize=100")
	if len(listed) != len(household) {
		t.Errorf("the resident lists %d people, her household has %d", len(listed), len(household))
	}
	for _, it := range listed {
		if it["cpf"] != nil && it["id"] != selfID {
			t.Errorf("another person's CPF in the resident's list: %v", it)
		}
	}
	if got := morador.items("/units?pageSize=100"); len(got) != 1 {
		t.Errorf("the resident should see one unit, sees %d", len(got))
	}

	// The porteiro sees names and units of everyone, nothing personal.
	list := porteiro.items("/people?pageSize=100")
	if len(list) != 100 {
		t.Fatalf("porteiro list: %d", len(list))
	}
	for _, it := range list {
		if it["cpf"] != nil || it["email"] != nil || it["phone"] != nil || it["hasAccount"] != nil {
			t.Errorf("porteiro sees personal data: %v", it)
		}
	}
	// The tenant (inquilino) is responsible for another unit and cannot reach into the first one.
	unit2 := inquilino.call("GET", "/me/units", nil, http.StatusOK)["items"].([]any)[0].(map[string]any)["unitId"].(string)
	inquilino.call("GET", "/units/"+unitID+"/links", nil, http.StatusNotFound)
	inquilino.call("POST", "/units/"+unitID+"/links", map[string]any{"kind": "dependente", "person": map[string]any{"fullName": "Intruso"}}, http.StatusNotFound)
	inquilino.call("POST", "/units/"+unit2+"/links", map[string]any{"kind": "dependente", "person": map[string]any{"fullName": "Filho do Inquilino"}}, http.StatusCreated)
}
