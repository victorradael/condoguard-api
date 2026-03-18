package user

import (
	"context"
	"fmt"

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
func (s *Service) Create(ctx context.Context, req CreateRequest) (*User, error) {
	if req.Username == "" {
		return nil, fmt.Errorf("%w: username is required", ErrValidation)
	}
	if req.Email == "" {
		return nil, fmt.Errorf("%w: email is required", ErrValidation)
	}
	if req.Password == "" {
		return nil, fmt.Errorf("%w: password is required", ErrValidation)
	}

	hashed, err := password.Hash(req.Password)
	if err != nil {
		return nil, err
	}

	roles := req.Roles
	if len(roles) == 0 {
		roles = []string{"ROLE_USER"}
	}

	u := &User{
		Username: req.Username,
		Email:    req.Email,
		Password: hashed,
		Roles:    roles,
	}

	if err := s.repo.Save(ctx, u); err != nil {
		return nil, err
	}
	return u, nil
}

// GetByID returns the user with the given ID or ErrNotFound.
func (s *Service) GetByID(ctx context.Context, id string) (*User, error) {
	return s.repo.FindByID(ctx, id)
}

// List returns all users.
func (s *Service) List(ctx context.Context) ([]*User, error) {
	return s.repo.FindAll(ctx)
}

// Update applies the mutable fields (username, roles) to the existing user.
// Email is intentionally ignored — it is immutable after creation.
func (s *Service) Update(ctx context.Context, id string, req UpdateRequest) (*User, error) {
	u, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if req.Username != "" {
		u.Username = req.Username
	}
	if len(req.Roles) > 0 {
		u.Roles = req.Roles
	}
	// req.Email is silently ignored — business rule: email is immutable.

	if err := s.repo.Update(ctx, u); err != nil {
		return nil, err
	}
	return u, nil
}

// Delete removes the user with the given ID or returns ErrNotFound.
func (s *Service) Delete(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}
