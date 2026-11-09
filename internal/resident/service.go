package resident

import (
	"context"
	"fmt"

	"github.com/victorradael/condoguard/api/internal/authz"
	"github.com/victorradael/condoguard/api/pkg/page"
)

// Service holds the business logic for the resident domain.
//
// Authorization: admins manage every unit of the condominium; other users
// only read the units they own (ownerId). A unit the caller may not see is
// reported as not found, so its existence does not leak.
type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// Create validates the request and persists the resident. Admins only.
// Business rule: unitNumber must be unique within the condominium.
func (s *Service) Create(ctx context.Context, req CreateRequest) (*Resident, error) {
	if _, err := authz.RequireAdmin(ctx); err != nil {
		return nil, err
	}
	if req.UnitNumber == "" {
		return nil, fmt.Errorf("%w: unitNumber is required", ErrValidation)
	}
	if req.CondominiumID == "" {
		return nil, fmt.Errorf("%w: condominiumId is required", ErrValidation)
	}
	if req.OwnerID == "" {
		return nil, fmt.Errorf("%w: ownerId is required", ErrValidation)
	}

	res := &Resident{
		UnitNumber:    req.UnitNumber,
		Floor:         req.Floor,
		CondominiumID: req.CondominiumID,
		OwnerID:       req.OwnerID,
	}

	if err := s.repo.Save(ctx, res); err != nil {
		return nil, err
	}
	return res, nil
}

// GetByID returns the unit if the caller may see it, ErrNotFound otherwise.
func (s *Service) GetByID(ctx context.Context, id string) (*Resident, error) {
	p, err := authz.FromContext(ctx)
	if err != nil {
		return nil, err
	}
	res, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !p.IsAdmin() && res.OwnerID != p.UserID {
		return nil, ErrNotFound
	}
	return res, nil
}

// List returns one page of the units the caller may see.
func (s *Service) List(ctx context.Context, pg page.Params) (page.Result[*Resident], error) {
	p, err := authz.FromContext(ctx)
	if err != nil {
		return page.Result[*Resident]{}, err
	}
	owner := p.UserID
	if p.IsAdmin() {
		owner = ""
	}
	list, total, err := s.repo.FindAll(ctx, owner, pg)
	if err != nil {
		return page.Result[*Resident]{}, err
	}
	return page.New(list, pg, total), nil
}

// Update applies unitNumber and floor changes. Admins only.
// Business rule: unitNumber must remain unique within the same condominium.
func (s *Service) Update(ctx context.Context, id string, req UpdateRequest) (*Resident, error) {
	if _, err := authz.RequireAdmin(ctx); err != nil {
		return nil, err
	}
	res, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// Check uniqueness only when the unit number is actually changing.
	if req.UnitNumber != "" && req.UnitNumber != res.UnitNumber {
		exists, err := s.repo.ExistsUnit(ctx, res.CondominiumID, req.UnitNumber, id)
		if err != nil {
			return nil, err
		}
		if exists {
			return nil, ErrDuplicate
		}
		res.UnitNumber = req.UnitNumber
	}

	if req.Floor != 0 {
		res.Floor = req.Floor
	}

	if err := s.repo.Update(ctx, res); err != nil {
		return nil, err
	}
	return res, nil
}

// Delete removes the unit. Admins only.
func (s *Service) Delete(ctx context.Context, id string) error {
	if _, err := authz.RequireAdmin(ctx); err != nil {
		return err
	}
	return s.repo.Delete(ctx, id)
}

// UnitIDsOwnedBy returns the IDs of the units owned by userID.
func (s *Service) UnitIDsOwnedBy(ctx context.Context, userID string) ([]string, error) {
	return s.repo.IDsByOwner(ctx, userID)
}
