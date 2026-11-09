package user

import (
	"context"
	"fmt"
	"slices"

	"github.com/victorradael/condoguard/api/internal/authz"
	pkgemail "github.com/victorradael/condoguard/api/pkg/email"
	"github.com/victorradael/condoguard/api/pkg/page"
	"github.com/victorradael/condoguard/api/pkg/password"
)

// Service holds the business logic for the user domain.
type Service struct {
	repo Repository
}

// NewService creates a user Service.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// Create validates the request, hashes the password and persists the user.
// Only admins may call it, so it may grant any known role.
func (s *Service) Create(ctx context.Context, req CreateRequest) (*User, error) {
	if _, err := authz.RequireAdmin(ctx); err != nil {
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
		roles = []string{authz.RoleUser}
	}
	if err = validateRoles(roles); err != nil {
		return nil, err
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

	if err := s.repo.Save(ctx, u); err != nil {
		return nil, err
	}
	return u, nil
}

// GetByID returns the user with the given ID or ErrNotFound. It is not gated
// here: GET /users/me reads the caller's own account through it.
func (s *Service) GetByID(ctx context.Context, id string) (*User, error) {
	return s.repo.FindByID(ctx, id)
}

// List returns one page of the condominium's users.
func (s *Service) List(ctx context.Context, p page.Params) (page.Result[*User], error) {
	if _, err := authz.RequireAdmin(ctx); err != nil {
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
	if _, err := authz.RequireAdmin(ctx); err != nil {
		return nil, err
	}
	if len(req.Roles) > 0 {
		if err := validateRoles(req.Roles); err != nil {
			return nil, err
		}
	}
	u, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if req.Username != "" {
		u.Username = req.Username
	}
	rolesChanged := len(req.Roles) > 0 && !sameRoles(u.Roles, req.Roles)
	if len(req.Roles) > 0 {
		u.Roles = req.Roles
	}
	// req.Email is silently ignored — business rule: email is immutable.

	if err := s.repo.Update(ctx, u); err != nil {
		return nil, err
	}
	if rolesChanged {
		if err := s.repo.RevokeSessions(ctx, u.ID); err != nil {
			return nil, err
		}
	}
	return u, nil
}

// Delete removes the user with the given ID or returns ErrNotFound.
func (s *Service) Delete(ctx context.Context, id string) error {
	if _, err := authz.RequireAdmin(ctx); err != nil {
		return err
	}
	return s.repo.Delete(ctx, id)
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
			return fmt.Errorf("%w: unknown role %q (use %s or %s)", ErrValidation, r, authz.RoleUser, authz.RoleAdmin)
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
