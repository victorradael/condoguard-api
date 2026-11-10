package people

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/victorradael/condoguard/api/internal/audit"
	"github.com/victorradael/condoguard/api/internal/authz"
	"github.com/victorradael/condoguard/api/internal/db"
	"github.com/victorradael/condoguard/api/internal/db/dbgen"
	"github.com/victorradael/condoguard/api/pkg/civil"
)

func linkDTO(id, unitID, personID uuid.UUID, name, kind string, starts time.Time, ends *time.Time) Link {
	today := civil.Today()
	l := Link{ID: id.String(), UnitID: unitID.String(), PersonID: personID.String(), PersonName: name, Kind: kind,
		StartsOn: civil.Format(starts), Active: !starts.After(today) && (ends == nil || ends.After(today))}
	if ends != nil {
		l.EndsOn = civil.Format(*ends)
	}
	return l
}

func linkOfRow(r dbgen.UnitLink, name string) Link {
	return linkDTO(r.ID, r.UnitID, r.PersonID, name, r.Kind, r.StartsOn, r.EndsOn)
}

// parseDay reads an optional day; def is used when s is empty.
func parseDay(s, field string, def time.Time) (time.Time, error) {
	if s == "" {
		return def, nil
	}
	d, err := civil.Parse(s)
	if err != nil {
		return time.Time{}, validationf("%s must be YYYY-MM-DD", field)
	}
	if d.Year() < 1900 || d.Year() > 2200 {
		return time.Time{}, validationf("%s is out of range", field)
	}
	return d, nil
}

// unitExists checks the unit belongs to the condominium.
func (s *Service) unitExists(ctx context.Context, condo, unit uuid.UUID) error {
	_, err := s.q.GetResidentByID(ctx, dbgen.GetResidentByIDParams{CondominiumID: condo, ID: unit})
	if db.IsNoRows(err) {
		return ErrNotFound
	}
	return err
}

// household reports whether the caller (without people.write) is responsible for
// the unit. A caller with no link at all gets ErrNotFound, as if the unit did not exist.
func (s *Service) household(ctx context.Context, condo, unit uuid.UUID, p authz.Principal) (responsible bool, err error) {
	uid, ok := parseID(p.UserID)
	if !ok {
		return false, ErrNotFound
	}
	linked, err := s.q.UserHasLink(ctx, dbgen.UserHasLinkParams{CondominiumID: condo, UnitID: unit, UserID: &uid})
	if err != nil {
		return false, err
	}
	if !linked {
		if p.Can(authz.PermUnitsReadAll) {
			return false, nil // sees the unit (porteiro, conselho) but is not part of it: forbidden, not hidden
		}
		return false, ErrNotFound
	}
	return s.q.UserIsResponsible(ctx, dbgen.UserIsResponsibleParams{CondominiumID: condo, UnitID: unit, UserID: &uid})
}

// History lists the links of a unit, newest first. The roles that read contacts
// (conselho, síndico, administradora) get the whole history; the porteiro and the
// people living in the unit get who is linked now.
func (s *Service) History(ctx context.Context, unitID string, activeOnly bool) ([]Link, error) {
	p, err := authz.FromContext(ctx)
	if err != nil {
		return nil, err
	}
	condo, err := s.condo(ctx)
	if err != nil {
		return nil, err
	}
	unit, ok := parseID(unitID)
	if !ok {
		return nil, ErrNotFound
	}
	if err = s.unitExists(ctx, condo, unit); err != nil {
		return nil, err
	}
	if !p.Can(authz.PermPeopleReadContact) {
		activeOnly = true
		if !p.Can(authz.PermPeopleReadAll) {
			if _, err = s.household(ctx, condo, unit, p); err != nil {
				return nil, err
			}
		}
	}
	rows, err := s.q.ListLinksByUnit(ctx, dbgen.ListLinksByUnitParams{CondominiumID: condo, UnitID: unit, ActiveOnly: activeOnly})
	if err != nil {
		return nil, err
	}
	out := make([]Link, len(rows))
	for i, r := range rows {
		out[i] = linkDTO(r.ID, r.UnitID, r.PersonID, r.PersonName, r.Kind, r.StartsOn, r.EndsOn)
	}
	return out, nil
}

// resolvePerson finds or makes the person a link or transfer names. A person
// created here is part of the same transaction. byWriter says whether the caller
// may look people up (people.write); others can only describe a new person.
func (s *Service) resolvePerson(ctx context.Context, condo uuid.UUID, id string, in *PersonRequest, byWriter bool) (dbgen.Person, error) {
	switch {
	case id != "" && in != nil:
		return dbgen.Person{}, validationf("give personId or person, not both")
	case id != "":
		if !byWriter {
			return dbgen.Person{}, authz.ErrForbidden
		}
		uid, ok := parseID(id)
		if !ok {
			return dbgen.Person{}, validationf("personId is invalid")
		}
		r, err := s.q.GetPerson(ctx, dbgen.GetPersonParams{CondominiumID: condo, ID: uid})
		if db.IsNoRows(err) {
			return dbgen.Person{}, validationf("person not found")
		}
		return r, err
	case in != nil:
		c, err := cleanPersonRequest(*in)
		if err != nil {
			return dbgen.Person{}, err
		}
		if c.cpf != nil {
			// The same CPF is the same person. Only who may look people up is told so.
			existing, ferr := s.q.FindPersonByCPF(ctx, dbgen.FindPersonByCPFParams{CondominiumID: condo, Cpf: c.cpf})
			if ferr == nil {
				if !byWriter {
					return dbgen.Person{}, fmt.Errorf("%w: this person is already registered; ask the síndico to link them", ErrConflict)
				}
				return existing, nil
			}
			if !db.IsNoRows(ferr) {
				return dbgen.Person{}, ferr
			}
		}
		return s.insertPerson(ctx, condo, c, false)
	}
	return dbgen.Person{}, validationf("personId or person is required")
}

func linkConflict(err error) error {
	if db.IsExclusionViolation(err) {
		switch db.ConstraintName(err) {
		case "unit_links_one_owner":
			return conflictf("the unit already has a principal owner in this period; use a transfer to change the owner")
		case "unit_links_no_repeat":
			return conflictf("the person already has this link with the unit in this period")
		}
		return conflictf("the link overlaps another one")
	}
	return err
}

// AddLink links a person to a unit. The síndico and the administradora (people.write)
// can add any kind of link, with a known person or a new one; whoever is responsible
// for the unit (owner, co-owner or tenant) can add residents and dependents of it.
func (s *Service) AddLink(ctx context.Context, unitID string, req LinkRequest) (*Link, error) {
	p, err := authz.FromContext(ctx)
	if err != nil {
		return nil, err
	}
	condo, err := s.condo(ctx)
	if err != nil {
		return nil, err
	}
	unit, ok := parseID(unitID)
	if !ok {
		return nil, ErrNotFound
	}
	if err = s.unitExists(ctx, condo, unit); err != nil {
		return nil, err
	}
	writer := p.Can(authz.PermPeopleWrite)
	if !writer {
		responsible, herr := s.household(ctx, condo, unit, p)
		if herr != nil {
			return nil, herr
		}
		if !responsible || !householdKind(req.Kind) {
			return nil, authz.ErrForbidden
		}
	}
	if !validKind(req.Kind) {
		return nil, validationf("kind must be one of %v", Kinds)
	}
	today := civil.Today()
	starts, err := parseDay(req.StartsOn, "startsOn", today)
	if err != nil {
		return nil, err
	}
	var ends *time.Time
	if req.EndsOn != "" {
		e, eerr := parseDay(req.EndsOn, "endsOn", time.Time{})
		if eerr != nil {
			return nil, eerr
		}
		if !e.After(starts) {
			return nil, validationf("endsOn must be after startsOn")
		}
		ends = &e
	}
	var out Link
	err = s.trail.Atomic(ctx, func(ctx context.Context) (audit.Entry, error) {
		person, rerr := s.resolvePerson(ctx, condo, req.PersonID, req.Person, writer)
		if rerr != nil {
			return audit.Entry{}, rerr
		}
		id := uuid.New()
		if ierr := s.q.InsertLink(ctx, dbgen.InsertLinkParams{ID: id, CondominiumID: condo, UnitID: unit, PersonID: person.ID,
			Kind: req.Kind, StartsOn: starts, EndsOn: ends}); ierr != nil {
			return audit.Entry{}, linkConflict(ierr)
		}
		out = linkDTO(id, unit, person.ID, person.FullName, req.Kind, starts, ends)
		return audit.Entry{Action: audit.ActionCreate, Resource: resLink, ResourceID: out.ID, After: out}, nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// EndLink ends a link on a day (default today): from that day the person is no
// longer linked. Same authorization as AddLink, and the responsible person can
// only end residents and dependents.
func (s *Service) EndLink(ctx context.Context, linkID string, req EndLinkRequest) (*Link, error) {
	p, err := authz.FromContext(ctx)
	if err != nil {
		return nil, err
	}
	condo, err := s.condo(ctx)
	if err != nil {
		return nil, err
	}
	lid, ok := parseID(linkID)
	if !ok {
		return nil, ErrNotFound
	}
	end, err := parseDay(req.EndsOn, "endsOn", civil.Today())
	if err != nil {
		return nil, err
	}
	var out Link
	err = s.trail.Atomic(ctx, func(ctx context.Context) (audit.Entry, error) {
		before, gerr := s.q.GetLink(ctx, dbgen.GetLinkParams{CondominiumID: condo, ID: lid})
		if db.IsNoRows(gerr) {
			return audit.Entry{}, ErrNotFound
		}
		if gerr != nil {
			return audit.Entry{}, gerr
		}
		if !p.Can(authz.PermPeopleWrite) {
			responsible, herr := s.household(ctx, condo, before.UnitID, p)
			if herr != nil {
				return audit.Entry{}, herr
			}
			if !responsible || !householdKind(before.Kind) {
				return audit.Entry{}, authz.ErrForbidden
			}
			// The responsible person only closes links that are still running, and
			// not retroactively: past ends are reserved for people.write.
			today := civil.Today()
			if before.EndsOn != nil && !before.EndsOn.After(today) {
				return audit.Entry{}, validationf("the link has already ended")
			}
			if end.Before(today) {
				return audit.Entry{}, validationf("endsOn cannot be in the past")
			}
		}
		if !end.After(before.StartsOn) {
			return audit.Entry{}, validationf("endsOn must be after the start of the link (%s)", civil.Format(before.StartsOn))
		}
		if _, uerr := s.q.EndLink(ctx, dbgen.EndLinkParams{CondominiumID: condo, ID: lid, EndsOn: &end}); uerr != nil {
			return audit.Entry{}, linkConflict(uerr)
		}
		after := before
		after.EndsOn = &end
		person, perr := s.q.GetPerson(ctx, dbgen.GetPersonParams{CondominiumID: condo, ID: before.PersonID})
		if perr != nil {
			return audit.Entry{}, perr
		}
		out = linkOfRow(after, person.FullName)
		return audit.Entry{Action: audit.ActionUpdate, Resource: resLink, ResourceID: linkID, Before: linkOfRow(before, person.FullName), After: out}, nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// Transfer swaps the owner (kind proprietario) or the tenant (kind inquilino) of
// a unit on a day: the previous ones end that day (for owners that includes the
// co-owners) and the new one starts, in one transaction, keeping the history.
// Needs people.write.
func (s *Service) Transfer(ctx context.Context, unitID string, req TransferRequest) (*TransferResult, error) {
	if _, err := authz.Require(ctx, authz.PermPeopleWrite); err != nil {
		return nil, err
	}
	var kinds []string
	switch req.Kind {
	case KindOwner:
		kinds = []string{KindOwner, KindCoOwner}
	case KindTenant:
		kinds = []string{KindTenant}
	default:
		return nil, validationf("kind must be %s or %s", KindOwner, KindTenant)
	}
	condo, err := s.condo(ctx)
	if err != nil {
		return nil, err
	}
	unit, ok := parseID(unitID)
	if !ok {
		return nil, ErrNotFound
	}
	day, err := parseDay(req.EffectiveDate, "effectiveDate", civil.Today())
	if err != nil {
		return nil, err
	}
	if err = s.unitExists(ctx, condo, unit); err != nil {
		return nil, err
	}
	var res TransferResult
	err = s.trail.Atomic(ctx, func(ctx context.Context) (audit.Entry, error) {
		person, rerr := s.resolvePerson(ctx, condo, req.PersonID, req.Person, true)
		if rerr != nil {
			return audit.Entry{}, rerr
		}
		later, cerr := s.q.CountLinksStartingAfter(ctx, dbgen.CountLinksStartingAfterParams{CondominiumID: condo, UnitID: unit, Kinds: kinds, OnDay: day})
		if cerr != nil {
			return audit.Entry{}, cerr
		}
		if later > 0 {
			return audit.Entry{}, conflictf("the unit has %s links starting on or after %s; pick a later day", req.Kind, civil.Format(day))
		}
		open, oerr := s.q.ListOpenLinksForUpdate(ctx, dbgen.ListOpenLinksForUpdateParams{CondominiumID: condo, UnitID: unit, Kinds: kinds, OnDay: day})
		if oerr != nil {
			return audit.Entry{}, oerr
		}
		res.Ended = make([]Link, 0, len(open))
		for _, l := range open {
			if _, eerr := s.q.EndLink(ctx, dbgen.EndLinkParams{CondominiumID: condo, ID: l.ID, EndsOn: &day}); eerr != nil {
				return audit.Entry{}, linkConflict(eerr)
			}
			prev, perr := s.q.GetPerson(ctx, dbgen.GetPersonParams{CondominiumID: condo, ID: l.PersonID})
			if perr != nil {
				return audit.Entry{}, perr
			}
			l.EndsOn = &day
			res.Ended = append(res.Ended, linkOfRow(l, prev.FullName))
		}
		id := uuid.New()
		if ierr := s.q.InsertLink(ctx, dbgen.InsertLinkParams{ID: id, CondominiumID: condo, UnitID: unit, PersonID: person.ID, Kind: req.Kind, StartsOn: day}); ierr != nil {
			return audit.Entry{}, linkConflict(ierr)
		}
		res.Created = linkDTO(id, unit, person.ID, person.FullName, req.Kind, day, nil)
		return audit.Entry{Action: audit.ActionUpdate, Resource: resTransfer, ResourceID: unitID, Before: res.Ended, After: res.Created}, nil
	})
	if err != nil {
		return nil, err
	}
	return &res, nil
}

// DeleteLink removes a link that has not started before today (a registration
// mistake: adding the wrong person, or the wrong day). A link that already
// started is history and is ended with EndLink instead. Same authorization as EndLink.
func (s *Service) DeleteLink(ctx context.Context, linkID string) error {
	p, err := authz.FromContext(ctx)
	if err != nil {
		return err
	}
	condo, err := s.condo(ctx)
	if err != nil {
		return err
	}
	lid, ok := parseID(linkID)
	if !ok {
		return ErrNotFound
	}
	return s.trail.Atomic(ctx, func(ctx context.Context) (audit.Entry, error) {
		before, gerr := s.q.GetLink(ctx, dbgen.GetLinkParams{CondominiumID: condo, ID: lid})
		if db.IsNoRows(gerr) {
			return audit.Entry{}, ErrNotFound
		}
		if gerr != nil {
			return audit.Entry{}, gerr
		}
		if !p.Can(authz.PermPeopleWrite) {
			responsible, herr := s.household(ctx, condo, before.UnitID, p)
			if herr != nil {
				return audit.Entry{}, herr
			}
			if !responsible || !householdKind(before.Kind) {
				return audit.Entry{}, authz.ErrForbidden
			}
		}
		if before.StartsOn.Before(civil.Today()) {
			return audit.Entry{}, conflictf("the link started on %s; end it instead of deleting it", civil.Format(before.StartsOn))
		}
		person, perr := s.q.GetPerson(ctx, dbgen.GetPersonParams{CondominiumID: condo, ID: before.PersonID})
		if perr != nil {
			return audit.Entry{}, perr
		}
		if _, derr := s.q.DeleteLink(ctx, dbgen.DeleteLinkParams{CondominiumID: condo, ID: lid}); derr != nil {
			return audit.Entry{}, derr
		}
		return audit.Entry{Action: audit.ActionDelete, Resource: resLink, ResourceID: linkID, Before: linkOfRow(before, person.FullName)}, nil
	})
}
