package user

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/victorradael/condoguard/api/internal/audit"
	"github.com/victorradael/condoguard/api/internal/authz"
	pkgemail "github.com/victorradael/condoguard/api/pkg/email"
	"github.com/victorradael/condoguard/api/pkg/page"
	"github.com/victorradael/condoguard/api/pkg/password"
)

// Service holds the business logic for the user domain.
type Service struct {
	repo      Repository
	trail     audit.Trail
	links     Links
	twoFactor TwoFactor
}

const auditResource = "user"

// NewService creates a user Service.
func NewService(repo Repository) *Service {
	return &Service{repo: repo, trail: audit.NewTrail()}
}

// WithAudit makes every change write its audit entry in the same transaction.
func (s *Service) WithAudit(t audit.Trail) *Service {
	s.trail = t
	return s
}

// mayGrant reports whether the caller may give (or take away) these roles:
// only whoever manages the condominium itself can touch the administradora role,
// so a síndico cannot promote anyone, or themselves, above their own level.
func mayGrant(p authz.Principal, roles []string) bool {
	return !slices.Contains(roles, authz.RoleAdministradora) || p.Can(authz.PermCondominiumManage)
}

// Create validates the request, hashes the password and persists the user.
// Needs users.manage; the administradora role needs condominium.manage.
func (s *Service) Create(ctx context.Context, req CreateRequest) (*User, error) {
	p, err := authz.Require(ctx, authz.PermUsersManage)
	if err != nil {
		return nil, err
	}
	if req.Username == "" {
		return nil, fmt.Errorf("%w: username is required", ErrValidation)
	}
	email, err := pkgemail.Normalize(req.Email)
	if err != nil {
		return nil, fmt.Errorf("%w: a valid email is required", ErrValidation)
	}
	if err = password.Validate(req.Password, localPart(email), req.Username); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrValidation, err)
	}

	roles := req.Roles
	if len(roles) == 0 {
		roles = []string{authz.RoleMorador}
	}
	if err = validateRoles(roles); err != nil {
		return nil, err
	}
	if !mayGrant(p, roles) {
		return nil, authz.ErrForbidden
	}

	hashed, err := password.Hash(req.Password)
	if err != nil {
		return nil, err
	}

	u := &User{
		Username: req.Username,
		Email:    email,
		Password: hashed,
		Roles:    roles,
	}

	err = s.trail.Atomic(ctx, func(ctx context.Context) (audit.Entry, error) {
		if serr := s.repo.Save(ctx, u); serr != nil {
			return audit.Entry{}, serr
		}
		return audit.Entry{Action: audit.ActionCreate, Resource: auditResource, ResourceID: u.ID, After: u}, nil
	})
	if err != nil {
		return nil, err
	}
	return u, nil
}

// Get returns a user of the condominium. Needs users.read.
func (s *Service) Get(ctx context.Context, id string) (*User, error) {
	if _, err := authz.Require(ctx, authz.PermUsersRead); err != nil {
		return nil, err
	}
	return s.repo.FindByID(ctx, id)
}

// GetByID returns the user with the given ID or ErrNotFound. It is not gated
// here: GET /users/me reads the caller's own account through it.
func (s *Service) GetByID(ctx context.Context, id string) (*User, error) {
	return s.repo.FindByID(ctx, id)
}

// List returns one page of the condominium's users.
func (s *Service) List(ctx context.Context, p page.Params) (page.Result[*User], error) {
	if _, err := authz.Require(ctx, authz.PermUsersRead); err != nil {
		return page.Result[*User]{}, err
	}
	list, total, err := s.repo.FindAll(ctx, p)
	if err != nil {
		return page.Result[*User]{}, err
	}
	return page.New(list, p, total), nil
}

// Update applies the mutable fields (username, roles) to the existing user.
// Email is intentionally ignored — it is immutable after creation.
// Changing the roles revokes the user's refresh tokens, so the old roles do
// not survive a refresh (the current access token still expires on its own).
func (s *Service) Update(ctx context.Context, id string, req UpdateRequest) (*User, error) {
	p, err := authz.Require(ctx, authz.PermUsersManage)
	if err != nil {
		return nil, err
	}
	if len(req.Roles) > 0 {
		if verr := validateRoles(req.Roles); verr != nil {
			return nil, verr
		}
		if !mayGrant(p, req.Roles) {
			return nil, authz.ErrForbidden
		}
	}
	u, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !mayGrant(p, u.Roles) {
		return nil, authz.ErrForbidden
	}
	before := *u
	before.Roles = slices.Clone(u.Roles)

	if req.Username != "" {
		u.Username = req.Username
	}
	rolesChanged := len(req.Roles) > 0 && !sameRoles(u.Roles, req.Roles)
	if len(req.Roles) > 0 {
		u.Roles = req.Roles
	}
	// req.Email is silently ignored — business rule: email is immutable.

	err = s.trail.Atomic(ctx, func(ctx context.Context) (audit.Entry, error) {
		if uerr := s.repo.Update(ctx, u); uerr != nil {
			return audit.Entry{}, uerr
		}
		if rolesChanged {
			if rerr := s.repo.RevokeSessions(ctx, u.ID); rerr != nil {
				return audit.Entry{}, rerr
			}
		}
		return audit.Entry{Action: audit.ActionUpdate, Resource: auditResource, ResourceID: u.ID, Before: before, After: u}, nil
	})
	if err != nil {
		return nil, err
	}
	return u, nil
}

// Delete takes the user out of the condominium (and deletes the account when
// it was the last one) or returns ErrNotFound. Needs users.manage.
func (s *Service) Delete(ctx context.Context, id string) error {
	p, err := authz.Require(ctx, authz.PermUsersManage)
	if err != nil {
		return err
	}
	return s.trail.Atomic(ctx, func(ctx context.Context) (audit.Entry, error) {
		before, err := s.repo.FindByID(ctx, id)
		if err != nil {
			return audit.Entry{}, err
		}
		if !mayGrant(p, before.Roles) {
			return audit.Entry{}, authz.ErrForbidden
		}
		if err = s.repo.RevokeSessions(ctx, id); err != nil {
			return audit.Entry{}, err
		}
		if err = s.repo.Delete(ctx, id); err != nil {
			return audit.Entry{}, err
		}
		return audit.Entry{Action: audit.ActionDelete, Resource: auditResource, ResourceID: id, Before: before}, nil
	})
}

func sameRoles(a, b []string) bool {
	a, b = slices.Clone(a), slices.Clone(b)
	slices.Sort(a)
	slices.Sort(b)
	return slices.Equal(slices.Compact(a), slices.Compact(b))
}

func validateRoles(roles []string) error {
	for _, r := range roles {
		if !authz.ValidRole(r) {
			return fmt.Errorf("%w: unknown role %q (use one of %s)", ErrValidation, r, strings.Join(authz.Roles, ", "))
		}
	}
	return nil
}

func localPart(email string) string {
	for i := range len(email) {
		if email[i] == '@' {
			return email[:i]
		}
	}
	return email
}
