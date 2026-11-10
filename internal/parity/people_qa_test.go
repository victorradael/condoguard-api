package parity_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/victorradael/condoguard/api/internal/authz"
)

// Independent verification of the 1c (FAB-21): privilege escalation by the
// responsible resident, invitations, data shown by need, input hardening, the
// audit trail and the response headers. The happy paths live in people_test.go;
// these are the attacks.

func today() string { return time.Now().Format("2006-01-02") }

// An invitation is tied to the e-mail it was issued for. When the síndico fixes a
// wrong e-mail, the pending invitation to the old address must stop working:
// otherwise whoever holds that address takes over the person (and their home).
func TestQA_InvitationDiesWhenTheEmailChanges(t *testing.T) {
	f := newTenant(t)
	s := f.sindico
	ana := f.person("Ana Souza", cpfAna, "errado@exemplo.test")
	f.link(f.unit1, ana, "proprietario")
	inv := s.call("POST", "/people/"+ana+"/invitations", map[string]any{"channel": "link"}, http.StatusCreated)

	s.call("PUT", "/people/"+ana, map[string]any{"fullName": "Ana Souza", "cpf": cpfAna, "email": "ana@exemplo.test"}, http.StatusOK)

	resp := do(t, f.srv, req{method: "POST", path: "/auth/invitations/accept", body: map[string]any{"token": inv["token"], "username": "intruso", "password": password}})
	if resp.StatusCode == http.StatusCreated {
		t.Errorf("the invitation to the old e-mail still works after the e-mail changed (account %v)", decode(t, resp)["email"])
	}
}

// Ending a link is a one-way street for the responsible resident: they cannot
// re-open a link that already ended (bringing an ex-housemate back) nor rewrite
// years of history by ending it in the past.
func TestQA_ResponsibleCannotReopenOrBackdateLinks(t *testing.T) {
	f := newTenant(t)
	s := f.sindico
	ana := f.person("Ana Souza", cpfAna, "ana@exemplo.test")
	bia := f.person("Bia Lima", cpfBia, "bia@exemplo.test")
	f.link(f.unit1, ana, "proprietario")
	biaLink := id(f.link(f.unit1, bia, "morador"))
	anaAPI := f.accountFor(ana, "ana@exemplo.test")

	t.Run("reopen", func(t *testing.T) {
		// The síndico ended Bia's stay yesterday.
		yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
		s.call("PUT", "/links/"+biaLink+"/end", map[string]any{"endsOn": yesterday}, http.StatusOK)
		st := anaAPI.status("PUT", "/links/"+biaLink+"/end", map[string]any{"endsOn": "2199-01-01"})
		if st == http.StatusOK {
			t.Errorf("the responsible resident re-opened a link the síndico had ended (status %d)", st)
		}
		for _, l := range s.items("/units/" + f.unit1 + "/links") {
			if id(l) == biaLink && l["active"] == true {
				t.Errorf("the ended link is active again: %v", l)
			}
		}
	})

	t.Run("backdate", func(t *testing.T) {
		cleo := anaAPI.call("POST", "/units/"+f.unit1+"/links", map[string]any{"kind": "morador", "startsOn": "2020-01-01",
			"person": map[string]any{"fullName": "Cleo Souza"}}, http.StatusCreated)
		st := anaAPI.status("PUT", "/links/"+id(cleo)+"/end", map[string]any{"endsOn": "2020-01-02"})
		if st == http.StatusOK {
			t.Errorf("the responsible resident rewrote the history: a link from 2020 now ended on 2020-01-02")
		}
	})
}

// A tenant is responsible for the household too, but only for residents and
// dependents of their own unit, and only while the tenancy lasts.
func TestQA_TenantResponsibleLimits(t *testing.T) {
	f := newTenant(t)
	s := f.sindico
	owner := f.person("Dono", cpfAna, "dono@exemplo.test")
	tenant := f.person("Inquilino", cpfBia, "inq@exemplo.test")
	other := f.person("Vizinho", cpfCaio, "viz@exemplo.test")
	ownerLink := id(f.link(f.unit2, owner, "proprietario"))
	tenantLink := id(f.link(f.unit2, tenant, "inquilino"))
	otherLink := id(f.link(f.unit1, other, "proprietario"))
	inq := f.accountFor(tenant, "inq@exemplo.test")

	kid := inq.call("POST", "/units/"+f.unit2+"/links", map[string]any{"kind": "dependente", "person": map[string]any{"fullName": "Filho"}}, http.StatusCreated)
	for _, kind := range []string{"proprietario", "coproprietario", "inquilino"} {
		inq.call("POST", "/units/"+f.unit2+"/links", map[string]any{"kind": kind, "person": map[string]any{"fullName": "Novo " + kind}}, http.StatusForbidden)
	}
	inq.call("POST", "/units/"+f.unit2+"/links", map[string]any{"kind": "morador", "personId": other}, http.StatusForbidden)
	inq.call("PUT", "/links/"+ownerLink+"/end", nil, http.StatusForbidden)
	inq.call("DELETE", "/links/"+ownerLink, nil, http.StatusForbidden)
	inq.call("PUT", "/links/"+tenantLink+"/end", nil, http.StatusForbidden) // not even their own tenancy
	inq.call("PUT", "/links/"+otherLink+"/end", nil, http.StatusNotFound)   // another home
	inq.call("DELETE", "/links/"+otherLink, nil, http.StatusNotFound)
	inq.call("POST", "/units/"+f.unit2+"/transfers", map[string]any{"kind": "inquilino", "person": map[string]any{"fullName": "X"}}, http.StatusForbidden)
	inq.call("POST", "/people", map[string]any{"fullName": "X"}, http.StatusForbidden)
	inq.call("PUT", "/people/"+owner, map[string]any{"fullName": "X"}, http.StatusForbidden)
	inq.call("POST", "/people/"+owner+"/invitations", map[string]any{"channel": "link"}, http.StatusForbidden)
	inq.call("GET", "/people/"+owner+"/invitations", nil, http.StatusForbidden)
	inq.call("GET", "/audit-log", nil, http.StatusForbidden)
	resp := do(t, f.srv, req{method: "POST", path: "/imports/preview", token: inq.token, rawBody: strings.NewReader("unidade\n999\n")})
	expect(t, resp, http.StatusForbidden, "import by a resident")

	// The tenancy ends today: from now on the ex-tenant manages nothing.
	s.call("PUT", "/links/"+tenantLink+"/end", map[string]any{"endsOn": today()}, http.StatusOK)
	if st := inq.status("POST", "/units/"+f.unit2+"/links", map[string]any{"kind": "dependente", "person": map[string]any{"fullName": "Outro"}}); st == http.StatusCreated {
		t.Error("an ex-tenant still adds people to the unit")
	}
	if st := inq.status("PUT", "/links/"+id(kid)+"/end", nil); st == http.StatusOK {
		t.Error("an ex-tenant still ends links of the unit")
	}
	if st := inq.status("GET", "/people/"+owner, nil); st != http.StatusNotFound {
		t.Errorf("an ex-tenant still reads the owner: %d", st)
	}
}

// A dependent does not get the unit (rule of 00005): the same must hold for the
// people and the links of that unit.
func TestQA_DependentWithAccountSeesNoHousehold(t *testing.T) {
	f := newTenant(t)
	ana := f.person("Ana Souza", cpfAna, "ana@exemplo.test")
	duda := f.person("Duda Souza", cpfBia, "duda@exemplo.test")
	f.link(f.unit1, ana, "proprietario")
	f.link(f.unit1, duda, "dependente")
	dudaAPI := f.accountFor(duda, "duda@exemplo.test")

	unit := dudaAPI.status("GET", "/units/"+f.unit1, nil)
	links := dudaAPI.status("GET", "/units/"+f.unit1+"/links", nil)
	person := dudaAPI.status("GET", "/people/"+ana, nil)
	listed := len(dudaAPI.items("/people"))
	if unit == http.StatusNotFound && (links != http.StatusNotFound || person != http.StatusNotFound || listed > 1) {
		t.Errorf("a dependent cannot see the unit (%d) but sees its links (%d), the owner (%d) and %d people", unit, links, person, listed)
	}
}

// An account of another condominium (here: the síndico of B) accepting an
// invitation in A becomes a morador of A, nothing more, and stays síndico of B only.
func TestQA_AcceptWithExistingAccountGrantsOnlyMorador(t *testing.T) {
	f := newTenant(t)
	s := f.sindico
	srvB := f.e.server(condoB)
	_ = adminToken(t, f.e, srvB, condoB)
	emailB := "sindico-" + condoB + "@exemplo.test"
	p := f.person("Síndico de B", cpfAna, emailB)
	f.link(f.unit1, p, "morador")
	inv := s.call("POST", "/people/"+p+"/invitations", map[string]any{"channel": "link"}, http.StatusCreated)
	accept := func(pass string) int {
		return do(t, f.srv, req{method: "POST", path: "/auth/invitations/accept", body: map[string]any{"token": inv["token"], "password": pass}}).StatusCode
	}
	if st := accept("Wrong-Pass-1234"); st != http.StatusUnauthorized {
		t.Fatalf("wrong password: %d", st)
	}
	if st := accept(password); st != http.StatusCreated {
		t.Fatalf("accept: %d", st)
	}
	resp := do(t, f.srv, req{method: "POST", path: "/auth/login", body: map[string]any{"email": emailB, "password": password, "condominiumId": condoA}})
	expect(t, resp, http.StatusOK, "login in A")
	tok, _ := decode(t, resp)["accessToken"].(string)
	me := api{t, f.srv, tok}.call("GET", "/users/me", nil, http.StatusOK)
	if roles, _ := me["roles"].([]any); len(roles) != 1 || roles[0] != authz.RoleMorador {
		t.Errorf("roles in A: %v", me["roles"])
	}
	api{t, f.srv, tok}.call("POST", "/people", map[string]any{"fullName": "X"}, http.StatusForbidden)
}

// Brute force on the public invitation routes is limited per IP. inspect shares
// the per-IP login bucket (LOGIN_MAX_ATTEMPTS_PER_IP, off in this harness), so
// only accept is exercised here.
func TestQA_InvitationRoutesAreRateLimited(t *testing.T) {
	f := newTenant(t)
	for _, path := range []string{"/auth/invitations/accept"} {
		limited := false
		for i := 0; i < 40 && !limited; i++ {
			resp := do(t, f.srv, req{method: "POST", path: path, body: map[string]any{"token": "AAAA-AAAA-AAAA-AAAA-AAAA-AAAA-AAAA-AAA" + string(rune('A'+i%26)), "password": password}})
			if resp.StatusCode == http.StatusTooManyRequests {
				limited = true
				if resp.Header.Get("Retry-After") == "" {
					t.Errorf("%s: 429 without Retry-After", path)
				}
			}
		}
		if !limited {
			t.Errorf("%s: 40 guesses from one IP and no 429", path)
		}
	}
}

// Hostile input answers 4xx, never 500, and never widens a search.
func TestQA_InputHardening(t *testing.T) {
	f := newTenant(t)
	s := f.sindico
	ana := f.person("Ana Souza", cpfAna, "ana@exemplo.test")
	f.link(f.unit1, ana, "proprietario")

	for name, body := range map[string]map[string]any{
		"NUL in the name":  {"fullName": "Ana\x00Souza"},
		"NUL in the phone": {"fullName": "Ana", "phone": "11\x009999"},
		"huge name":        {"fullName": strings.Repeat("a", 5000)},
		"name not a text":  {"fullName": map[string]any{"$ne": ""}},
	} {
		if st := s.status("POST", "/people", body); st >= 500 || st < 400 {
			t.Errorf("%s: %d", name, st)
		}
	}
	if st := s.status("POST", "/blocks", map[string]any{"name": "Torre\x00"}); st >= 500 || st < 400 {
		t.Errorf("NUL in a block name: %d", st)
	}
	for _, q := range []string{"%", "_", "' OR '1'='1", `\`, "Ana%' --", "%%%%"} {
		got := s.call("GET", "/people?q="+urlQuery(q), nil, http.StatusOK)
		if got["total"].(float64) != 0 {
			t.Errorf("q=%q matched %v people: the search is not literal", q, got["total"])
		}
	}
	for _, path := range []string{"/people?q=" + strings.Repeat("a", 500), "/people?unitId=x", "/people?pageSize=100000", "/people?page=-1", "/units?blockId=%27"} {
		if st := s.status("GET", path, nil); st >= 500 {
			t.Errorf("GET %s: %d", path, st)
		}
	}
	if st := s.status("POST", "/units/"+f.unit1+"/links", map[string]any{"personId": ana, "kind": "morador", "startsOn": "99999-01-01"}); st >= 500 || st < 400 {
		t.Errorf("absurd date: %d", st)
	}
	resp := do(t, f.srv, req{method: "POST", path: "/units/" + f.unit1 + "/links", token: s.token, rawBody: strings.NewReader(`{"personId":` + strings.Repeat("[", 100000) + `}`)})
	if resp.StatusCode >= 500 {
		t.Errorf("deeply nested JSON: %d", resp.StatusCode)
	}
}

func urlQuery(s string) string {
	r := strings.NewReplacer("%", "%25", "'", "%27", " ", "%20", "\\", "%5C", "_", "_")
	return r.Replace(s)
}

// The CSV upload: size, row count, encoding and broken bytes are refused cleanly;
// a resident cannot use it. Formula-looking cells are only stored (reported to FAB-10).
func TestQA_ImportUploadLimits(t *testing.T) {
	f := newTenant(t)
	post := func(body io.Reader, length int64) *http.Response {
		hr, _ := http.NewRequest("POST", f.srv.URL+"/imports/preview", body)
		hr.Header.Set("Authorization", "Bearer "+f.sindico.token)
		hr.Header.Set("Content-Type", "text/csv")
		hr.ContentLength = length
		resp, err := http.DefaultClient.Do(hr)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = resp.Body.Close() })
		return resp
	}
	big := "unidade;nome\n" + strings.Repeat("101;"+strings.Repeat("a", 100)+"\n", 11000) // ~1.1 MiB
	if st := post(strings.NewReader(big), int64(len(big))).StatusCode; st != http.StatusRequestEntityTooLarge {
		t.Errorf("1.1 MiB with Content-Length: %d", st)
	}
	if st := post(io.MultiReader(strings.NewReader(big)), -1).StatusCode; st < 400 || st >= 500 {
		t.Errorf("1.1 MiB streamed without length: %d", st)
	}
	var many strings.Builder
	many.WriteString("unidade\n")
	for i := 0; i < 5001; i++ {
		many.WriteString("u")
		many.WriteString(strings.Repeat("1", i%7+1))
		many.WriteString("\n")
	}
	if st := post(strings.NewReader(many.String()), int64(many.Len())).StatusCode; st != http.StatusUnprocessableEntity {
		t.Errorf("5001 rows: %d", st)
	}
	for name, body := range map[string]string{
		"NUL byte":       "unidade;nome\n101;Ana\x00Souza\n",
		"UTF-16":         "\xff\xfeu\x00n\x00i\x00d\x00a\x00d\x00e\x00\n\x00",
		"unclosed quote": "unidade;nome\n101;\"Ana\n",
		"wide row":       "unidade;nome\n101;" + strings.Repeat(";", 200000) + "\n",
	} {
		if st := post(strings.NewReader(body), int64(len(body))).StatusCode; st >= 500 {
			t.Errorf("%s: %d", name, st)
		}
	}
}

// Personal data and one-time tokens are never cached, and the trail keeps no
// token nor full CPF.
func TestQA_NoStoreAndTrailWithoutSecrets(t *testing.T) {
	f := newTenant(t)
	s := f.sindico
	ana := f.person("Ana Souza", cpfAna, "ana@exemplo.test")
	f.link(f.unit1, ana, "proprietario")
	inv := s.call("POST", "/people/"+ana+"/invitations", map[string]any{"channel": "link"}, http.StatusCreated)
	token, _ := inv["token"].(string)

	check := func(what string, resp *http.Response) {
		t.Helper()
		if cc := resp.Header.Get("Cache-Control"); !strings.Contains(cc, "no-store") {
			t.Errorf("%s: Cache-Control %q", what, cc)
		}
	}
	for _, path := range []string{"/people", "/people/" + ana, "/people/" + ana + "/invitations", "/units/" + f.unit1 + "/links", "/me/units", "/blocks", "/units"} {
		check("GET "+path, do(t, f.srv, req{method: "GET", path: path, token: s.token}))
	}
	check("POST invitation", do(t, f.srv, req{method: "POST", path: "/people/" + ana + "/invitations", token: s.token, body: map[string]any{"channel": "link"}}))
	check("inspect", do(t, f.srv, req{method: "POST", path: "/auth/invitations/inspect", body: map[string]any{"token": token}}))
	check("accept", do(t, f.srv, req{method: "POST", path: "/auth/invitations/accept", body: map[string]any{"token": token, "password": password}}))

	resp := do(t, f.srv, req{method: "GET", path: "/audit-log?pageSize=100", token: s.token})
	expect(t, resp, http.StatusOK, "audit")
	trail, _ := io.ReadAll(resp.Body)
	for _, secret := range []string{token, strings.ReplaceAll(token, "-", ""), "52998224725", "529.982.247-25"} {
		if secret != "" && strings.Contains(string(trail), secret) {
			t.Errorf("the audit trail holds %q", secret)
		}
	}
}

// Deleting people and blocks that something still points to is a 409, never a 500.
func TestQA_DeleteWithDependentsIsAConflict(t *testing.T) {
	f := newTenant(t)
	s := f.sindico
	invited := f.person("Convidada", "", "conv@exemplo.test")
	s.call("POST", "/people/"+invited+"/invitations", map[string]any{"channel": "link"}, http.StatusCreated)
	if st := s.status("DELETE", "/people/"+invited, nil); st >= 500 {
		t.Errorf("person with an invitation: %d", st)
	}
	withAccount := f.person("Com Conta", cpfCaio, "conta@exemplo.test")
	f.link(f.unit2, withAccount, "morador")
	_ = f.accountFor(withAccount, "conta@exemplo.test")
	if st := s.status("DELETE", "/people/"+withAccount, nil); st != http.StatusConflict {
		t.Errorf("person with links and an account: %d", st)
	}
	if st := s.status("DELETE", "/units/"+f.unit2, nil); st >= 500 {
		t.Errorf("unit with links: %d", st)
	}
	if st := s.status("DELETE", "/blocks/"+f.block, nil); st != http.StatusConflict {
		t.Errorf("block with units: %d", st)
	}
}

// Ids of another condominium are "not found" on every write route of 1c.
func TestQA_CrossTenantWritesByID(t *testing.T) {
	f := newTenant(t)
	ana := f.person("Ana Souza", cpfAna, "ana@exemplo.test")
	link := id(f.link(f.unit1, ana, "proprietario"))
	srvB := f.e.server(condoB)
	b := api{t, srvB, adminToken(t, f.e, srvB, condoB)}
	b.call("PUT", "/links/"+link+"/end", nil, http.StatusNotFound)
	b.call("DELETE", "/links/"+link, nil, http.StatusNotFound)
	b.call("GET", "/people/"+ana+"/invitations", nil, http.StatusNotFound)
	b.call("PUT", "/blocks/"+f.block, map[string]any{"name": "x"}, http.StatusNotFound)
	b.call("DELETE", "/blocks/"+f.block, nil, http.StatusNotFound)
	b.call("PUT", "/units/"+f.unit1, map[string]any{"unitNumber": "1", "floor": 1}, http.StatusNotFound)
	b.call("DELETE", "/units/"+f.unit1, nil, http.StatusNotFound)
	bUnit := id(b.call("POST", "/units", map[string]any{"unitNumber": "1", "floor": 1}, http.StatusCreated))
	// A person of A cannot be linked into B by id, nor A's block used by B.
	if st := b.status("POST", "/units/"+bUnit+"/links", map[string]any{"personId": ana, "kind": "morador"}); st == http.StatusCreated {
		t.Error("B linked a person of A")
	}
	if st := b.status("POST", "/units/"+bUnit+"/transfers", map[string]any{"kind": "proprietario", "personId": ana}); st == http.StatusCreated {
		t.Error("B transferred a unit to a person of A")
	}
	if st := b.status("PUT", "/units/"+bUnit, map[string]any{"unitNumber": "1", "floor": 1, "blockId": f.block}); st == http.StatusOK {
		t.Error("B moved its unit into A's block")
	}
	if n := countRows(t, f, "SELECT count(*) FROM unit_links WHERE condominium_id <> person_condo(person_id)"); n != 0 {
		t.Errorf("%d links cross condominiums", n)
	}
}

func countRows(t *testing.T, f *tenantFixture, q string) int {
	t.Helper()
	q = strings.Replace(q, "person_condo(person_id)", "(SELECT p.condominium_id FROM people p WHERE p.id = unit_links.person_id)", 1)
	var n int
	if err := f.e.db.Admin.QueryRow(context.Background(), q).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
