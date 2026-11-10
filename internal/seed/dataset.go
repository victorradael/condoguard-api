// Package seed generates the fictitious data of the demonstration condominium
// (Brazilian names, fictitious CPFs, e-mails at exemplo.test, fictitious phones)
// and loads it into PostgreSQL. The same seed always generates the same data:
// every id and timestamp comes from the seed, not from the clock or the database.
// Everything it writes carries is_demo, so reset-demo removes only that.
package seed

import (
	"fmt"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/victorradael/condoguard/api/internal/authz"
	"github.com/victorradael/condoguard/api/internal/people"
)

// DefaultSeed is the seed used when none is given.
const DefaultSeed int64 = 20261110

// Profile sizes a data set.
type Profile struct {
	Name           string
	Blocks         []string
	FloorsPerBlock int
	UnitsPerFloor  int
	People         int // total, including the people who only appear in the history
	OwnerSwaps     int // units whose owner changed once
	TenantSwaps    int // units whose tenant changed once
	LeasedUnits    int // units with a current tenant
	CoOwnerUnits   int // units with a co-owner
}

// Profiles by name. "poc" is the condominium of the proof of concept (see FAB-3,
// metas): 3 blocks, 120 units, ~300 people, ~15 owner and ~25 tenant swaps.
var Profiles = map[string]Profile{
	"seed": {Name: "seed", Blocks: []string{"Bloco A"}, FloorsPerBlock: 2, UnitsPerFloor: 4, People: 16,
		OwnerSwaps: 1, TenantSwaps: 1, LeasedUnits: 3, CoOwnerUnits: 1},
	"poc": {Name: "poc", Blocks: []string{"Bloco A", "Bloco B", "Bloco C"}, FloorsPerBlock: 10, UnitsPerFloor: 4, People: 300,
		OwnerSwaps: 15, TenantSwaps: 25, LeasedUnits: 40, CoOwnerUnits: 25},
}

// Units is the number of units of the profile.
func (p Profile) Units() int { return len(p.Blocks) * p.FloorsPerBlock * p.UnitsPerFloor }

// Block of the data set.
type Block struct {
	ID   uuid.UUID
	Name string
}

// Unit of the data set.
type Unit struct {
	ID        uuid.UUID
	Block     int // index in Dataset.Blocks
	Number    string
	Floor     int
	CreatedAt time.Time
}

// Person of the data set. CPF, Email and Phone may be empty.
type Person struct {
	ID                uuid.UUID
	Name              string
	CPF, Email, Phone string
	CreatedAt         time.Time
	account           bool
}

// Link of the data set.
type Link struct {
	ID     uuid.UUID
	Unit   int
	Person int
	Kind   string
	Starts time.Time
	Ends   *time.Time
}

// Account is a test login, one per role.
type Account struct {
	Email    string
	Username string
	Role     string
	Person   int // index in Dataset.People, -1 for none
}

// Dataset is everything the generator produces.
type Dataset struct {
	Seed            int64
	Profile         Profile
	AdministratorID uuid.UUID
	Administrator   string
	CondominiumID   uuid.UUID
	Condominium     string
	Blocks          []Block
	Units           []Unit
	People          []Person
	Links           []Link
	Accounts        []Account
	OwnerSwapUnits  []int
	TenantSwapUnits []int
}

// Base of the generated timestamps: fixed, so runs do not depend on the clock.
var baseTime = time.Date(2026, 1, 5, 12, 0, 0, 0, time.UTC)

func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

type gen struct {
	rng      *rand.Rand
	seed     int64
	ds       *Dataset
	cpfs     map[string]bool
	names    map[string]int
	phoneSeq int
}

func (g *gen) id(kind string, n int) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(fmt.Sprintf("condoguard-demo/%d/%s/%d", g.seed, kind, n)))
}

// between returns a day between a and b (inclusive).
func (g *gen) between(a, b time.Time) time.Time {
	n := int(b.Sub(a).Hours() / 24)
	if n <= 0 {
		return a
	}
	return a.AddDate(0, 0, g.rng.IntN(n+1))
}

var asciiFold = strings.NewReplacer("á", "a", "à", "a", "â", "a", "ã", "a", "é", "e", "ê", "e", "í", "i", "ó", "o", "ô", "o", "õ", "o",
	"ú", "u", "ç", "c", "Á", "A", "É", "E", "Í", "I", "Ó", "O", "Ú", "U", "Ç", "C", "Â", "A", "Ê", "E", "Ô", "O", "Ã", "A", "Õ", "O")

func slug(s string) string { return strings.ToLower(asciiFold.Replace(s)) }

// newPerson makes a fictitious person. withCPF/withEmail say which data it has.
func (g *gen) newPerson(withCPF, withEmail bool) int {
	idx := len(g.ds.People)
	var name string
	for try := 0; ; try++ {
		name = firstNames[g.rng.IntN(len(firstNames))] + " " + surnames[g.rng.IntN(len(surnames))]
		if g.rng.IntN(3) > 0 {
			name += " " + surnames[g.rng.IntN(len(surnames))]
		}
		if g.names[name] == 0 || try > 20 {
			break
		}
	}
	g.names[name]++
	p := Person{ID: g.id("person", idx), Name: name, CreatedAt: baseTime.Add(time.Duration(idx) * time.Minute)}
	if withCPF {
		for {
			nine := fmt.Sprintf("%09d", g.rng.IntN(1_000_000_000))
			cpf := people.CPFWithCheckDigits(nine)
			if people.ValidCPF(cpf) && !g.cpfs[cpf] {
				g.cpfs[cpf] = true
				p.CPF = cpf
				break
			}
		}
	}
	if withEmail {
		parts := strings.Fields(slug(name))
		p.Email = fmt.Sprintf("%s.%s.%d@exemplo.test", parts[0], parts[len(parts)-1], idx+1)
	}
	// Phones are fictitious: 90000-XXXX is not assigned to anyone.
	g.phoneSeq++
	if g.rng.IntN(10) > 0 {
		p.Phone = fmt.Sprintf("%d90000%04d", []int{11, 21, 31, 41}[g.rng.IntN(4)], g.phoneSeq%10000)
	}
	g.ds.People = append(g.ds.People, p)
	return idx
}

func (g *gen) link(unit, person int, kind string, starts time.Time, ends *time.Time) {
	g.ds.Links = append(g.ds.Links, Link{ID: g.id("link", len(g.ds.Links)), Unit: unit, Person: person, Kind: kind, Starts: starts, Ends: ends})
}

// Generate builds the data set of the profile. It is a pure function of (profile, seed).
func Generate(profile Profile, seed int64) (*Dataset, error) {
	nUnits := profile.Units()
	minPeople := nUnits + profile.OwnerSwaps + profile.CoOwnerUnits + profile.LeasedUnits + profile.TenantSwaps
	switch {
	case nUnits < 4:
		return nil, fmt.Errorf("seed: the profile needs at least 4 units")
	case profile.OwnerSwaps > nUnits-2 || profile.CoOwnerUnits > nUnits || profile.LeasedUnits > nUnits-2 || profile.TenantSwaps > profile.LeasedUnits:
		return nil, fmt.Errorf("seed: the swaps, co-owners and leased units do not fit in %d units", nUnits)
	case profile.People < minPeople:
		return nil, fmt.Errorf("seed: %d people is too few for the profile (needs at least %d)", profile.People, minPeople)
	}
	g := &gen{rng: rand.New(rand.NewPCG(uint64(seed), uint64(seed)^0x9E3779B97F4A7C15)), seed: seed, cpfs: map[string]bool{}, names: map[string]int{}}
	ds := &Dataset{Seed: seed, Profile: profile, Administrator: "Administradora Exemplo Ltda (demo)", Condominium: "Residencial Aurora (demo)"}
	g.ds = ds
	ds.AdministratorID, ds.CondominiumID = g.id("administrator", 0), g.id("condominium", 0)

	for i, name := range profile.Blocks {
		ds.Blocks = append(ds.Blocks, Block{ID: g.id("block", i), Name: name})
	}
	for b := range profile.Blocks {
		for floor := 1; floor <= profile.FloorsPerBlock; floor++ {
			for k := 1; k <= profile.UnitsPerFloor; k++ {
				i := len(ds.Units)
				ds.Units = append(ds.Units, Unit{ID: g.id("unit", i), Block: b, Number: fmt.Sprint(floor*100 + k), Floor: floor,
					CreatedAt: baseTime.Add(-time.Hour + time.Duration(i)*time.Second)})
			}
		}
	}

	// Which units get which story. Units 0 and 1 are kept simple (they hold the test accounts).
	pick := func(n int, exclude ...int) []int {
		skip := map[int]bool{}
		for _, e := range exclude {
			skip[e] = true
		}
		var out []int
		for _, u := range g.rng.Perm(nUnits) {
			if len(out) == n {
				break
			}
			if !skip[u] {
				out = append(out, u)
			}
		}
		return out
	}
	ownerSwap := map[int]bool{}
	for _, u := range pick(profile.OwnerSwaps, 0, 1) {
		ownerSwap[u] = true
	}
	leased := map[int]bool{1: true}
	for _, u := range pick(profile.LeasedUnits-1, 0, 1) {
		leased[u] = true
	}
	tenantSwap := map[int]bool{}
	leasedList := make([]int, 0, len(leased))
	for u := 0; u < nUnits; u++ {
		if leased[u] {
			leasedList = append(leasedList, u)
		}
	}
	for _, i := range g.rng.Perm(len(leasedList))[:profile.TenantSwaps] {
		tenantSwap[leasedList[i]] = true
	}
	coOwned := map[int]bool{}
	for _, u := range pick(profile.CoOwnerUnits) {
		coOwned[u] = true
	}

	ownerOf := make([]int, nUnits) // current owner (person index)
	ownerSince := make([]time.Time, nUnits)
	homeSince := make([]time.Time, nUnits) // when the current household moved in
	for u := 0; u < nUnits; u++ {
		start := g.between(day(2016, 1, 1), day(2021, 12, 31))
		if ownerSwap[u] {
			prev := g.newPerson(true, true)
			swap := g.between(day(2022, 6, 1), day(2025, 9, 30))
			end := swap
			g.link(u, prev, people.KindOwner, start, &end)
			ds.OwnerSwapUnits = append(ds.OwnerSwapUnits, u)
			start = swap
		}
		o := g.newPerson(true, true)
		ownerOf[u], ownerSince[u], homeSince[u] = o, start, start
		g.link(u, o, people.KindOwner, start, nil)
		if coOwned[u] {
			c := g.newPerson(true, true)
			g.link(u, c, people.KindCoOwner, start, nil)
		}
	}
	for u := 0; u < nUnits; u++ {
		if !leased[u] {
			continue
		}
		from := ownerSince[u]
		start := g.between(maxDay(from, day(2020, 1, 1)), day(2025, 12, 31))
		if tenantSwap[u] {
			prev := g.newPerson(true, true)
			swap := g.between(maxDay(start, day(2022, 1, 1)).AddDate(0, 0, 30), day(2026, 6, 30))
			if swap.Before(start.AddDate(0, 0, 30)) {
				swap = start.AddDate(0, 0, 30)
			}
			end := swap
			g.link(u, prev, people.KindTenant, start, &end)
			ds.TenantSwapUnits = append(ds.TenantSwapUnits, u)
			start = swap
		}
		t := g.newPerson(true, true)
		g.link(u, t, people.KindTenant, start, nil)
		homeSince[u] = start
	}
	// The rest of the household: residents (adults, with contact) and dependents (children, often without CPF or e-mail).
	for len(ds.People) < profile.People {
		u := g.rng.IntN(nUnits)
		since := maxDay(homeSince[u], day(2018, 1, 1))
		start := g.between(since, day(2026, 6, 30))
		if g.rng.IntN(100) < 55 {
			g.link(u, g.newPerson(g.rng.IntN(100) < 70, g.rng.IntN(100) < 80), people.KindResident, start, nil)
		} else {
			g.link(u, g.newPerson(g.rng.IntN(100) < 35, false), people.KindDependent, start, nil)
		}
	}

	// Test accounts, one per role. The resident accounts are tied to people of the data set.
	roleOwner := ownerOf[0]
	var tenantOfUnit1 int
	for _, l := range ds.Links {
		if l.Unit == 1 && l.Kind == people.KindTenant && l.Ends == nil {
			tenantOfUnit1 = l.Person
		}
	}
	ds.Accounts = []Account{
		{Email: "administradora@exemplo.test", Username: "administradora", Role: authz.RoleAdministradora, Person: -1},
		{Email: "sindico@exemplo.test", Username: "sindico", Role: authz.RoleSindico, Person: -1},
		{Email: "conselho@exemplo.test", Username: "conselho", Role: authz.RoleConselho, Person: -1},
		{Email: "porteiro@exemplo.test", Username: "porteiro", Role: authz.RolePorteiro, Person: -1},
		{Email: "morador@exemplo.test", Username: "morador", Role: authz.RoleMorador, Person: roleOwner},
		{Email: "inquilino@exemplo.test", Username: "inquilino", Role: authz.RoleMorador, Person: tenantOfUnit1},
	}
	for _, a := range ds.Accounts {
		if a.Person >= 0 {
			ds.People[a.Person].Email = a.Email
			ds.People[a.Person].account = true
		}
	}
	return ds, nil
}

func maxDay(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

// Counts summarises a data set (for the report and the tests).
type Counts struct {
	Blocks, Units, People, Links, OwnerSwaps, TenantSwaps, Accounts int
}

// Counts returns the sizes of the data set.
func (d *Dataset) Counts() Counts {
	return Counts{Blocks: len(d.Blocks), Units: len(d.Units), People: len(d.People), Links: len(d.Links),
		OwnerSwaps: len(d.OwnerSwapUnits), TenantSwaps: len(d.TenantSwapUnits), Accounts: len(d.Accounts)}
}
