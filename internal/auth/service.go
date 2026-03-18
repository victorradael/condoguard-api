package auth

import (
	"context"
	"errors"
	"fmt"

	pkgjwt "github.com/victorradael/condoguard/api/pkg/jwt"
	"github.com/victorradael/condoguard/api/pkg/password"
)

// ErrInvalidCredentials is returned when username or password is wrong.
var ErrInvalidCredentials = errors.New("auth: invalid credentials")

// Service holds the business logic for the auth domain.
type Service struct {
	repo   Repository
	jwtSvc *pkgjwt.Service
}

// NewService creates an auth Service.
func NewService(repo Repository, jwtSvc *pkgjwt.Service) *Service {
	return &Service{repo: repo, jwtSvc: jwtSvc}
}

// Register creates a new user after validating the request and hashing the password.
func (s *Service) Register(ctx context.Context, req RegisterRequest) error {
	if req.Email == "" {
		return fmt.Errorf("%w: email is required", ErrValidation)
	}
	if req.Password == "" {
		return fmt.Errorf("%w: password is required", ErrValidation)
	}
	if req.Username == "" {
		return fmt.Errorf("%w: username is required", ErrValidation)
	}

	hashed, err := password.Hash(req.Password)
	if err != nil {
		return err
	}

	if len(req.Roles) == 0 {
		req.Roles = []string{"ROLE_USER"}
	}

	user := &User{
		Username: req.Username,
		Email:    req.Email,
		Password: hashed,
		Roles:    req.Roles,
	}
	return s.repo.Save(ctx, user)
}

// Login validates credentials and returns a LoginResponse containing a JWT.
func (s *Service) Login(ctx context.Context, req LoginRequest) (*LoginResponse, error) {
	if req.Username == "" {
		return nil, fmt.Errorf("%w: username is required", ErrValidation)
	}
	if req.Password == "" {
		return nil, fmt.Errorf("%w: password is required", ErrValidation)
	}

	user, err := s.repo.FindByUsername(ctx, req.Username)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}

	if !password.Verify(req.Password, user.Password) {
		return nil, ErrInvalidCredentials
	}

	token, err := s.jwtSvc.GenerateToken(user.ID, user.Roles)
	if err != nil {
		return nil, err
	}

	return &LoginResponse{Token: token, Roles: user.Roles}, nil
}

// ErrValidation signals an input validation failure.
var ErrValidation = errors.New("validation error")
