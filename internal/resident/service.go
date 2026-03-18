package resident

import (
	"context"
	"fmt"
)

// Service holds the business logic for the resident domain.
type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// Create validates the request and persists the resident.
// Business rule: unitNumber must be unique within the condominium.
func (s *Service) Create(ctx context.Context, req CreateRequest) (*Resident, error) {
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

func (s *Service) GetByID(ctx context.Context, id string) (*Resident, error) {
	return s.repo.FindByID(ctx, id)
}

func (s *Service) List(ctx context.Context) ([]*Resident, error) {
	return s.repo.FindAll(ctx)
}

// Update applies unitNumber and floor changes.
// Business rule: unitNumber must remain unique within the same condominium.
func (s *Service) Update(ctx context.Context, id string, req UpdateRequest) (*Resident, error) {
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

func (s *Service) Delete(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}
