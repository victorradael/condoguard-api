package people

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/victorradael/condoguard/api/internal/audit"
	"github.com/victorradael/condoguard/api/internal/authz"
	"github.com/victorradael/condoguard/api/internal/db"
	"github.com/victorradael/condoguard/api/internal/db/dbgen"
	"github.com/victorradael/condoguard/api/internal/tenant"
	"github.com/victorradael/condoguard/api/pkg/civil"
	pkgemail "github.com/victorradael/condoguard/api/pkg/email"
	"github.com/victorradael/condoguard/api/pkg/page"
)

// Service holds the business rules of blocks, people and unit links.
type Service struct {
	pool  *pgxpool.Pool
	q     *dbgen.Queries
	trail audit.Trail
	inv   *invitations
}

// NewService creates the Service on the shared pool. Changes write their audit
// entry in the same transaction (WithAudit).
func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool, q: db.Queries(pool), trail: audit.NewTrail()}
}

// WithAudit makes every change write its audit entry in the same transaction.
func (s *Service) WithAudit(t audit.Trail) *Service {
	s.trail = t
	return s
}

// Audit resources.
const (
	resBlock      = "block"
	resPerson     = "person"
	resLink       = "unit_link"
	resTransfer   = "unit_transfer"
	resInvitation = "invitation"
	resImport     = "import"
)

const (
	maxName   = 200
	maxBlock  = 80
	maxImport = 5000 // rows per CSV file
)

// ── access levels ─────────────────────────────────────────────────────────────

// level says how much of a person the caller may see.
type level int

const (
	levelNone level = iota
	levelBasic
	levelHousehold
	levelContact
	levelFull
)

func levelOf(p authz.Principal) level {
	switch {
	case p.Can(authz.PermPeopleReadCPF):
		return levelFull
	case p.Can(authz.PermPeopleReadContact):
		return levelContact
	case p.Can(authz.PermPeopleReadAll):
		return levelBasic
	}
	return levelNone
}

func (s *Service) condo(ctx context.Context) (uuid.UUID, error) { return tenant.UUID(ctx) }

func parseID(id string) (uuid.UUID, bool) { return db.ParseID(id) }

func validationf(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrValidation, fmt.Sprintf(format, a...))
}

func conflictf(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrConflict, fmt.Sprintf(format, a...))
}

// ── blocks ────────────────────────────────────────────────────────────────────

func cleanName(s string, max int, what string) (string, error) {
	s = strings.Join(strings.Fields(s), " ")
	if strings.ContainsRune(s, 0) || !utf8.ValidString(s) {
		return "", validationf("%s has invalid characters", what)
	}
	if s == "" {
		return "", validationf("%s is required", what)
	}
	if utf8.RuneCountInString(s) > max {
		return "", validationf("%s is too long (max %d)", what, max)
	}
	return s, nil
}

func blockOf(id uuid.UUID, name string, units int64, at time.Time) *Block {
	return &Block{ID: id.String(), Name: name, UnitCount: units, CreatedAt: at}
}

// ListBlocks returns one page of blocks. Needs units.read_all.
func (s *Service) ListBlocks(ctx context.Context, pg page.Params) (page.Result[*Block], error) {
	if _, err := authz.Require(ctx, authz.PermUnitsReadAll); err != nil {
		return page.Result[*Block]{}, err
	}
	condo, err := s.condo(ctx)
	if err != nil {
		return page.Result[*Block]{}, err
	}
	rows, err := s.q.ListBlocks(ctx, dbgen.ListBlocksParams{CondominiumID: condo, PageLimit: pg.Limit(), PageOffset: pg.Offset()})
	if err != nil {
		return page.Result[*Block]{}, err
	}
	total, err := s.q.CountBlocks(ctx, condo)
	if err != nil {
		return page.Result[*Block]{}, err
	}
	out := make([]*Block, len(rows))
	for i, r := range rows {
		out[i] = blockOf(r.ID, r.Name, r.UnitCount, r.CreatedAt)
	}
	return page.New(out, pg, total), nil
}

// GetBlock returns one block. Needs units.read_all.
func (s *Service) GetBlock(ctx context.Context, id string) (*Block, error) {
	if _, err := authz.Require(ctx, authz.PermUnitsReadAll); err != nil {
		return nil, err
	}
	condo, err := s.condo(ctx)
	if err != nil {
		return nil, err
	}
	uid, ok := parseID(id)
	if !ok {
		return nil, ErrNotFound
	}
	r, err := s.q.GetBlock(ctx, dbgen.GetBlockParams{CondominiumID: condo, ID: uid})
	if db.IsNoRows(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return blockOf(r.ID, r.Name, r.UnitCount, r.CreatedAt), nil
}

// CreateBlock makes a block. Needs units.write.
func (s *Service) CreateBlock(ctx context.Context, req BlockRequest) (*Block, error) {
	if _, err := authz.Require(ctx, authz.PermUnitsWrite); err != nil {
		return nil, err
	}
	name, err := cleanName(req.Name, maxBlock, "name")
	if err != nil {
		return nil, err
	}
	condo, err := s.condo(ctx)
	if err != nil {
		return nil, err
	}
	id := uuid.New()
	b := &Block{ID: id.String(), Name: name, CreatedAt: time.Now().UTC()}
	err = s.trail.Atomic(ctx, func(ctx context.Context) (audit.Entry, error) {
		if _, ierr := s.q.InsertBlock(ctx, dbgen.InsertBlockParams{ID: id, CondominiumID: condo, Name: name}); ierr != nil {
			return audit.Entry{}, ierr
		}
		return audit.Entry{Action: audit.ActionCreate, Resource: resBlock, ResourceID: b.ID, After: b}, nil
	})
	if db.IsUniqueViolation(err) {
		return nil, fmt.Errorf("%w: a block with this name already exists", ErrDuplicate)
	}
	if err != nil {
		return nil, err
	}
	return b, nil
}

// UpdateBlock renames a block. Needs units.write.
func (s *Service) UpdateBlock(ctx context.Context, id string, req BlockRequest) (*Block, error) {
	if _, err := authz.Require(ctx, authz.PermUnitsWrite); err != nil {
		return nil, err
	}
	name, err := cleanName(req.Name, maxBlock, "name")
	if err != nil {
		return nil, err
	}
	condo, err := s.condo(ctx)
	if err != nil {
		return nil, err
	}
	uid, ok := parseID(id)
	if !ok {
		return nil, ErrNotFound
	}
	var out *Block
	err = s.trail.Atomic(ctx, func(ctx context.Context) (audit.Entry, error) {
		before, gerr := s.q.GetBlock(ctx, dbgen.GetBlockParams{CondominiumID: condo, ID: uid})
		if db.IsNoRows(gerr) {
			return audit.Entry{}, ErrNotFound
		}
		if gerr != nil {
			return audit.Entry{}, gerr
		}
		if _, uerr := s.q.UpdateBlock(ctx, dbgen.UpdateBlockParams{CondominiumID: condo, ID: uid, Name: name}); uerr != nil {
			return audit.Entry{}, uerr
		}
		out = blockOf(uid, name, before.UnitCount, before.CreatedAt)
		return audit.Entry{Action: audit.ActionUpdate, Resource: resBlock, ResourceID: id,
			Before: blockOf(before.ID, before.Name, before.UnitCount, before.CreatedAt), After: out}, nil
	})
	if db.IsUniqueViolation(err) {
		return nil, fmt.Errorf("%w: a block with this name already exists", ErrDuplicate)
	}
	if err != nil {
		return nil, err
	}
	return out, nil
}

// DeleteBlock removes an empty block. Needs units.write.
func (s *Service) DeleteBlock(ctx context.Context, id string) error {
	if _, err := authz.Require(ctx, authz.PermUnitsWrite); err != nil {
		return err
	}
	condo, err := s.condo(ctx)
	if err != nil {
		return err
	}
	uid, ok := parseID(id)
	if !ok {
		return ErrNotFound
	}
	err = s.trail.Atomic(ctx, func(ctx context.Context) (audit.Entry, error) {
		before, gerr := s.q.GetBlock(ctx, dbgen.GetBlockParams{CondominiumID: condo, ID: uid})
		if db.IsNoRows(gerr) {
			return audit.Entry{}, ErrNotFound
		}
		if gerr != nil {
			return audit.Entry{}, gerr
		}
		if _, derr := s.q.DeleteBlock(ctx, dbgen.DeleteBlockParams{CondominiumID: condo, ID: uid}); derr != nil {
			return audit.Entry{}, derr
		}
		return audit.Entry{Action: audit.ActionDelete, Resource: resBlock, ResourceID: id,
			Before: blockOf(before.ID, before.Name, before.UnitCount, before.CreatedAt)}, nil
	})
	if db.IsForeignKeyViolation(err) {
		return conflictf("the block still has units; move or delete them first")
	}
	return err
}

// ── people ────────────────────────────────────────────────────────────────────

// cleanPerson validates and normalises a person payload.
type cleanedPerson struct {
	name              string
	cpf, email, phone *string
}

func cleanPersonRequest(req PersonRequest) (cleanedPerson, error) {
	var c cleanedPerson
	var err error
	if c.name, err = cleanName(req.FullName, maxName, "fullName"); err != nil {
		return c, err
	}
	if strings.TrimSpace(req.CPF) != "" {
		d, ok := NormalizeCPF(req.CPF)
		if !ok {
			return c, validationf("cpf is invalid")
		}
		c.cpf = &d
	}
	if strings.TrimSpace(req.Email) != "" {
		e, eerr := pkgemail.Normalize(req.Email)
		if eerr != nil {
			return c, validationf("email is invalid")
		}
		c.email = &e
	}
	if strings.TrimSpace(req.Phone) != "" {
		p, ok := NormalizePhone(req.Phone)
		if !ok {
			return c, validationf("phone is invalid (area code and number, digits only)")
		}
		c.phone = &p
	}
	return c, nil
}

func toPerson(r dbgen.Person, lvl level) Person {
	p := Person{ID: r.ID.String(), FullName: r.FullName, CreatedAt: r.CreatedAt}
	if lvl >= levelHousehold {
		p.Email, p.Phone = deref(r.Email), deref(r.Phone)
	}
	if lvl >= levelContact {
		acct := r.UserID != nil
		p.HasAccount = &acct
	}
	if cpf := deref(r.Cpf); cpf != "" {
		switch {
		case lvl >= levelFull:
			p.CPF = FormatCPF(cpf)
		case lvl >= levelContact:
			p.CPF = MaskCPF(cpf)
		}
	}
	return p
}

// auditPerson is the view of a person that goes to the audit trail: no full CPF.
func auditPerson(r dbgen.Person) Person { return toPerson(r, levelContact) }

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func unitRef(id, unitID uuid.UUID, number string, block *string, kind string, starts time.Time, ends *time.Time) UnitRef {
	today := civil.Today()
	u := UnitRef{LinkID: id.String(), UnitID: unitID.String(), UnitNumber: number, BlockName: deref(block), Kind: kind,
		StartsOn: civil.Format(starts), Active: !starts.After(today) && (ends == nil || ends.After(today))}
	if ends != nil {
		u.EndsOn = civil.Format(*ends)
	}
	return u
}

// unitsOfPeople loads the active links of the people, by person.
func (s *Service) unitsOfPeople(ctx context.Context, condo uuid.UUID, ids []uuid.UUID) (map[uuid.UUID][]UnitRef, error) {
	out := map[uuid.UUID][]UnitRef{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := s.q.ListActiveLinksByPeople(ctx, dbgen.ListActiveLinksByPeopleParams{CondominiumID: condo, PersonIds: ids})
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.PersonID] = append(out[r.PersonID], unitRef(r.ID, r.UnitID, r.UnitNumber, r.BlockName, r.Kind, r.StartsOn, r.EndsOn))
	}
	return out, nil
}

// myUnitIDs are the units the caller has an active link with.
func (s *Service) myUnitIDs(ctx context.Context, condo uuid.UUID, p authz.Principal) ([]uuid.UUID, error) {
	uid, ok := parseID(p.UserID)
	if !ok {
		return nil, nil
	}
	return s.q.ListUnitIDsByUserLinks(ctx, dbgen.ListUnitIDsByUserLinksParams{CondominiumID: condo, UserID: &uid})
}

// ListPeopleFilter narrows ListPeople.
type ListPeopleFilter struct {
	Query      string
	UnitID     string
	ActiveOnly bool
}

// ListPeople returns one page of people the caller may see: everyone for the
// roles with people.read_all, only the household of their units for a morador.
func (s *Service) ListPeople(ctx context.Context, f ListPeopleFilter, pg page.Params) (page.Result[Person], error) {
	p, err := authz.FromContext(ctx)
	if err != nil {
		return page.Result[Person]{}, err
	}
	condo, err := s.condo(ctx)
	if err != nil {
		return page.Result[Person]{}, err
	}
	lvl := levelOf(p)
	var visible []uuid.UUID
	var mine map[uuid.UUID]bool
	if lvl == levelNone {
		lvl = levelHousehold
		if visible, err = s.myUnitIDs(ctx, condo, p); err != nil {
			return page.Result[Person]{}, err
		}
		if len(visible) == 0 {
			return page.New[Person](nil, pg, 0), nil
		}
		mine = map[uuid.UUID]bool{}
		for _, id := range visible {
			mine[id] = true
		}
	}
	params := dbgen.ListPeopleParams{CondominiumID: condo, ActiveOnly: f.ActiveOnly || visible != nil,
		VisibleUnits: visible, PageLimit: pg.Limit(), PageOffset: pg.Offset()}
	if q := strings.TrimSpace(f.Query); q != "" {
		if utf8.RuneCountInString(q) > maxName {
			return page.Result[Person]{}, validationf("q is too long")
		}
		q = likeEscape(q)
		params.Q = &q
	}
	if f.UnitID != "" {
		uid, ok := parseID(f.UnitID)
		if !ok {
			return page.Result[Person]{}, validationf("unitId is invalid")
		}
		if visible != nil && !mine[uid] {
			return page.New[Person](nil, pg, 0), nil
		}
		params.UnitID = &uid
	}
	rows, err := s.q.ListPeople(ctx, params)
	if err != nil {
		return page.Result[Person]{}, err
	}
	total, err := s.q.CountPeople(ctx, dbgen.CountPeopleParams{CondominiumID: condo, Q: params.Q, UnitID: params.UnitID,
		ActiveOnly: params.ActiveOnly, VisibleUnits: visible})
	if err != nil {
		return page.Result[Person]{}, err
	}
	ids := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	units, err := s.unitsOfPeople(ctx, condo, ids)
	if err != nil {
		return page.Result[Person]{}, err
	}
	self, _ := parseID(p.UserID)
	out := make([]Person, len(rows))
	for i, r := range rows {
		rl := lvl
		if r.UserID != nil && *r.UserID == self {
			rl = levelFull // your own data is yours
		}
		out[i] = toPerson(r, rl)
		for _, u := range units[r.ID] {
			if mine != nil {
				uid, _ := parseID(u.UnitID)
				if !mine[uid] {
					continue // a household member's other homes are not for you
				}
			}
			out[i].Units = append(out[i].Units, u)
		}
	}
	return page.New(out, pg, total), nil
}

// likeEscape makes s literal inside an ILIKE pattern.
func likeEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// accessTo returns the level at which the caller sees the person, or levelNone.
func (s *Service) accessTo(ctx context.Context, condo uuid.UUID, p authz.Principal, r dbgen.Person) (level, error) {
	if lvl := levelOf(p); lvl != levelNone {
		return lvl, nil
	}
	uid, ok := parseID(p.UserID)
	if !ok {
		return levelNone, nil
	}
	if r.UserID != nil && *r.UserID == uid {
		return levelFull, nil
	}
	shares, err := s.q.PersonSharesUnitWithUser(ctx, dbgen.PersonSharesUnitWithUserParams{CondominiumID: condo, PersonID: r.ID, UserID: &uid})
	if err != nil {
		return levelNone, err
	}
	if shares {
		return levelHousehold, nil
	}
	return levelNone, nil
}

// GetPerson returns a person with all their links (history). What the caller
// may not see is reported as not found.
func (s *Service) GetPerson(ctx context.Context, id string) (*Person, error) {
	p, err := authz.FromContext(ctx)
	if err != nil {
		return nil, err
	}
	condo, err := s.condo(ctx)
	if err != nil {
		return nil, err
	}
	uid, ok := parseID(id)
	if !ok {
		return nil, ErrNotFound
	}
	r, err := s.q.GetPerson(ctx, dbgen.GetPersonParams{CondominiumID: condo, ID: uid})
	if db.IsNoRows(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	lvl, err := s.accessTo(ctx, condo, p, r)
	if err != nil {
		return nil, err
	}
	if lvl == levelNone {
		return nil, ErrNotFound
	}
	out := toPerson(r, lvl)
	links, err := s.q.ListLinksByPerson(ctx, dbgen.ListLinksByPersonParams{CondominiumID: condo, PersonID: uid})
	if err != nil {
		return nil, err
	}
	var mine map[uuid.UUID]bool
	if levelOf(p) == levelNone && lvl != levelFull {
		units, merr := s.myUnitIDs(ctx, condo, p)
		if merr != nil {
			return nil, merr
		}
		mine = map[uuid.UUID]bool{}
		for _, u := range units {
			mine[u] = true
		}
	}
	for _, l := range links {
		if mine != nil && !mine[l.UnitID] {
			continue
		}
		ref := unitRef(l.ID, l.UnitID, l.UnitNumber, l.BlockName, l.Kind, l.StartsOn, l.EndsOn)
		if lvl == levelBasic || lvl == levelHousehold {
			if !ref.Active {
				continue // the porteiro and the family see where people live now, not where they lived
			}
		}
		out.Units = append(out.Units, ref)
	}
	return &out, nil
}

// CreatePerson registers a person. Needs people.write.
func (s *Service) CreatePerson(ctx context.Context, req PersonRequest) (*Person, error) {
	if _, err := authz.Require(ctx, authz.PermPeopleWrite); err != nil {
		return nil, err
	}
	c, err := cleanPersonRequest(req)
	if err != nil {
		return nil, err
	}
	condo, err := s.condo(ctx)
	if err != nil {
		return nil, err
	}
	var row dbgen.Person
	err = s.trail.Atomic(ctx, func(ctx context.Context) (audit.Entry, error) {
		var ierr error
		if row, ierr = s.insertPerson(ctx, condo, c, false); ierr != nil {
			return audit.Entry{}, ierr
		}
		return audit.Entry{Action: audit.ActionCreate, Resource: resPerson, ResourceID: row.ID.String(), After: auditPerson(row)}, nil
	})
	if err != nil {
		return nil, err
	}
	out := toPerson(row, levelOf(authzPrincipal(ctx)))
	return &out, nil
}

func authzPrincipal(ctx context.Context) authz.Principal {
	p, _ := authz.FromContext(ctx)
	return p
}

func (s *Service) insertPerson(ctx context.Context, condo uuid.UUID, c cleanedPerson, demo bool) (dbgen.Person, error) {
	id := uuid.New()
	err := s.q.InsertPerson(ctx, dbgen.InsertPersonParams{ID: id, CondominiumID: condo, FullName: c.name, Cpf: c.cpf, Email: c.email, Phone: c.phone, IsDemo: demo})
	if db.IsUniqueViolation(err) {
		return dbgen.Person{}, fmt.Errorf("%w: a person with this CPF already exists", ErrDuplicate)
	}
	if err != nil {
		return dbgen.Person{}, err
	}
	return s.q.GetPerson(ctx, dbgen.GetPersonParams{CondominiumID: condo, ID: id})
}

// UpdatePerson replaces the data of a person. Needs people.write.
func (s *Service) UpdatePerson(ctx context.Context, id string, req PersonRequest) (*Person, error) {
	if _, err := authz.Require(ctx, authz.PermPeopleWrite); err != nil {
		return nil, err
	}
	c, err := cleanPersonRequest(req)
	if err != nil {
		return nil, err
	}
	condo, err := s.condo(ctx)
	if err != nil {
		return nil, err
	}
	uid, ok := parseID(id)
	if !ok {
		return nil, ErrNotFound
	}
	var after dbgen.Person
	err = s.trail.Atomic(ctx, func(ctx context.Context) (audit.Entry, error) {
		before, gerr := s.q.GetPerson(ctx, dbgen.GetPersonParams{CondominiumID: condo, ID: uid})
		if db.IsNoRows(gerr) {
			return audit.Entry{}, ErrNotFound
		}
		if gerr != nil {
			return audit.Entry{}, gerr
		}
		if _, uerr := s.q.UpdatePerson(ctx, dbgen.UpdatePersonParams{CondominiumID: condo, ID: uid, FullName: c.name, Cpf: c.cpf, Email: c.email, Phone: c.phone}); uerr != nil {
			return audit.Entry{}, uerr
		}
		var aerr error
		if after, aerr = s.q.GetPerson(ctx, dbgen.GetPersonParams{CondominiumID: condo, ID: uid}); aerr != nil {
			return audit.Entry{}, aerr
		}
		// An invitation is bound to the e-mail it was sent to: a new e-mail kills the pending ones.
		if !sameEmail(before.Email, after.Email) {
			if rerr := s.q.RevokePendingInvitations(ctx, dbgen.RevokePendingInvitationsParams{CondominiumID: condo, PersonID: uid}); rerr != nil {
				return audit.Entry{}, rerr
			}
		}
		return audit.Entry{Action: audit.ActionUpdate, Resource: resPerson, ResourceID: id, Before: auditPerson(before), After: auditPerson(after)}, nil
	})
	if db.IsUniqueViolation(err) {
		return nil, fmt.Errorf("%w: a person with this CPF already exists", ErrDuplicate)
	}
	if err != nil {
		return nil, err
	}
	out := toPerson(after, levelOf(authzPrincipal(ctx)))
	return &out, nil
}

// DeletePerson removes a person who has no link with any unit (past or present).
// Needs people.write.
func (s *Service) DeletePerson(ctx context.Context, id string) error {
	if _, err := authz.Require(ctx, authz.PermPeopleWrite); err != nil {
		return err
	}
	condo, err := s.condo(ctx)
	if err != nil {
		return err
	}
	uid, ok := parseID(id)
	if !ok {
		return ErrNotFound
	}
	err = s.trail.Atomic(ctx, func(ctx context.Context) (audit.Entry, error) {
		before, gerr := s.q.GetPerson(ctx, dbgen.GetPersonParams{CondominiumID: condo, ID: uid})
		if db.IsNoRows(gerr) {
			return audit.Entry{}, ErrNotFound
		}
		if gerr != nil {
			return audit.Entry{}, gerr
		}
		if _, derr := s.q.DeletePerson(ctx, dbgen.DeletePersonParams{CondominiumID: condo, ID: uid}); derr != nil {
			return audit.Entry{}, derr
		}
		return audit.Entry{Action: audit.ActionDelete, Resource: resPerson, ResourceID: id, Before: auditPerson(before)}, nil
	})
	if db.IsForeignKeyViolation(err) {
		return conflictf("the person has links with units (the history is kept); end them instead")
	}
	return err
}

func sameEmail(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
