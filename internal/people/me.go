package people

import (
	"context"

	"github.com/google/uuid"

	"github.com/victorradael/condoguard/api/internal/audit"
	"github.com/victorradael/condoguard/api/internal/authz"
	"github.com/victorradael/condoguard/api/internal/db"
	"github.com/victorradael/condoguard/api/internal/db/dbgen"
)

func (s *Service) myPerson(ctx context.Context) (authz.Principal, uuid.UUID, dbgen.Person, error) {
	p, err := authz.FromContext(ctx)
	if err != nil {
		return p, uuid.UUID{}, dbgen.Person{}, err
	}
	condo, err := s.condo(ctx)
	if err != nil {
		return p, condo, dbgen.Person{}, err
	}
	uid, ok := parseID(p.UserID)
	if !ok {
		return p, condo, dbgen.Person{}, ErrNotFound
	}
	r, err := s.q.GetPersonByUser(ctx, dbgen.GetPersonByUserParams{CondominiumID: condo, UserID: &uid})
	if db.IsNoRows(err) {
		return p, condo, r, ErrNotFound // an account without a person record (e.g. the first síndico)
	}
	return p, condo, r, err
}

// Profile returns the signed-in person and the units they are linked to now.
// ErrNotFound when the account has no person record in this condominium.
func (s *Service) Profile(ctx context.Context) (*Profile, error) {
	p, condo, r, err := s.myPerson(ctx)
	if err != nil {
		return nil, err
	}
	uid, _ := parseID(p.UserID)
	rows, err := s.q.ListLinksByUser(ctx, dbgen.ListLinksByUserParams{CondominiumID: condo, UserID: &uid})
	if err != nil {
		return nil, err
	}
	out := &Profile{Person: toPerson(r, levelFull), Units: make([]MyUnit, 0, len(rows))}
	for _, l := range rows {
		out.Units = append(out.Units, MyUnit{
			UnitRef:            unitRef(l.ID, l.UnitID, l.UnitNumber, l.BlockName, l.Kind, l.StartsOn, l.EndsOn),
			CanManageHousehold: responsibleKind(l.Kind),
		})
	}
	return out, nil
}

// UpdateProfile lets a person edit their own name and phone.
func (s *Service) UpdateProfile(ctx context.Context, req ProfileRequest) (*Profile, error) {
	_, condo, r, err := s.myPerson(ctx)
	if err != nil {
		return nil, err
	}
	c, err := cleanPersonRequest(PersonRequest{FullName: req.FullName, Phone: req.Phone})
	if err != nil {
		return nil, err
	}
	err = s.trail.Atomic(ctx, func(ctx context.Context) (audit.Entry, error) {
		if _, uerr := s.q.UpdatePerson(ctx, dbgen.UpdatePersonParams{CondominiumID: condo, ID: r.ID, FullName: c.name, Cpf: r.Cpf, Email: r.Email, Phone: c.phone}); uerr != nil {
			return audit.Entry{}, uerr
		}
		after := r
		after.FullName, after.Phone = c.name, c.phone
		return audit.Entry{Action: audit.ActionUpdate, Resource: resPerson, ResourceID: r.ID.String(), Before: auditPerson(r), After: auditPerson(after)}, nil
	})
	if err != nil {
		return nil, err
	}
	return s.Profile(ctx)
}
