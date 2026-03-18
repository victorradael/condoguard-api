package shopowner

import (
	"context"
	"fmt"
)

// Service holds the business logic for the shopowner domain.
type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// Create validates the request, formats and validates the CNPJ, then persists.
func (s *Service) Create(ctx context.Context, req CreateRequest) (*ShopOwner, error) {
	if req.ShopName == "" {
		return nil, fmt.Errorf("%w: shopName is required", ErrValidation)
	}
	if req.OwnerID == "" {
		return nil, fmt.Errorf("%w: ownerId is required", ErrValidation)
	}

	// CNPJ validation is enforced in the service layer (business rule).
	if err := ValidateCNPJ(req.CNPJ); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrValidation, err.Error())
	}

	shop := &ShopOwner{
		ShopName: req.ShopName,
		CNPJ:     FormatCNPJ(req.CNPJ), // normalize to standard format before save
		Floor:    req.Floor,
		OwnerID:  req.OwnerID,
	}

	if err := s.repo.Save(ctx, shop); err != nil {
		return nil, err
	}
	return shop, nil
}

func (s *Service) GetByID(ctx context.Context, id string) (*ShopOwner, error) {
	return s.repo.FindByID(ctx, id)
}

func (s *Service) List(ctx context.Context) ([]*ShopOwner, error) {
	return s.repo.FindAll(ctx)
}

// Update applies shopName and floor changes. CNPJ is immutable after creation.
func (s *Service) Update(ctx context.Context, id string, req UpdateRequest) (*ShopOwner, error) {
	shop, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if req.ShopName != "" {
		shop.ShopName = req.ShopName
	}
	if req.Floor != 0 {
		shop.Floor = req.Floor
	}

	if err := s.repo.Update(ctx, shop); err != nil {
		return nil, err
	}
	return shop, nil
}

func (s *Service) Delete(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}
