package shopowner

import (
	"context"
	"fmt"

	"github.com/victorradael/condoguard/api/internal/audit"
	"github.com/victorradael/condoguard/api/internal/authz"
	"github.com/victorradael/condoguard/api/pkg/page"
)

// Service holds the business logic for the shopowner domain.
//
// Authorization: admins manage every shop of the condominium; other users
// only read the shops they own (ownerId). A shop the caller may not see is
// reported as not found, so its existence does not leak.
type Service struct {
	repo  Repository
	trail audit.Trail
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo, trail: audit.NewTrail()}
}

// WithAudit makes every change write its audit entry in the same transaction.
func (s *Service) WithAudit(t audit.Trail) *Service {
	s.trail = t
	return s
}

const auditResource = "shop_owner"

// Create validates the request, formats and validates the CNPJ, then persists. Needs units.write.
func (s *Service) Create(ctx context.Context, req CreateRequest) (*ShopOwner, error) {
	if _, err := authz.Require(ctx, authz.PermUnitsWrite); err != nil {
		return nil, err
	}
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

	err := s.trail.Atomic(ctx, func(ctx context.Context) (audit.Entry, error) {
		if err := s.repo.Save(ctx, shop); err != nil {
			return audit.Entry{}, err
		}
		return audit.Entry{Action: audit.ActionCreate, Resource: auditResource, ResourceID: shop.ID, After: shop}, nil
	})
	if err != nil {
		return nil, err
	}
	return shop, nil
}

// GetByID returns the shop if the caller may see it, ErrNotFound otherwise.
func (s *Service) GetByID(ctx context.Context, id string) (*ShopOwner, error) {
	p, err := authz.FromContext(ctx)
	if err != nil {
		return nil, err
	}
	shop, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !p.Can(authz.PermUnitsReadAll) && shop.OwnerID != p.UserID {
		return nil, ErrNotFound
	}
	return shop, nil
}

// List returns one page of the shops the caller may see.
func (s *Service) List(ctx context.Context, pg page.Params) (page.Result[*ShopOwner], error) {
	p, err := authz.FromContext(ctx)
	if err != nil {
		return page.Result[*ShopOwner]{}, err
	}
	owner := p.UserID
	if p.Can(authz.PermUnitsReadAll) {
		owner = ""
	}
	list, total, err := s.repo.FindAll(ctx, owner, pg)
	if err != nil {
		return page.Result[*ShopOwner]{}, err
	}
	return page.New(list, pg, total), nil
}

// Update applies shopName and floor changes. CNPJ is immutable after creation. Needs units.write.
func (s *Service) Update(ctx context.Context, id string, req UpdateRequest) (*ShopOwner, error) {
	if _, err := authz.Require(ctx, authz.PermUnitsWrite); err != nil {
		return nil, err
	}
	shop, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	before := *shop

	if req.ShopName != "" {
		shop.ShopName = req.ShopName
	}
	if req.Floor != 0 {
		shop.Floor = req.Floor
	}

	err = s.trail.Atomic(ctx, func(ctx context.Context) (audit.Entry, error) {
		if uerr := s.repo.Update(ctx, shop); uerr != nil {
			return audit.Entry{}, uerr
		}
		return audit.Entry{Action: audit.ActionUpdate, Resource: auditResource, ResourceID: shop.ID, Before: before, After: shop}, nil
	})
	if err != nil {
		return nil, err
	}
	return shop, nil
}

// Delete removes the shop. Needs units.write.
func (s *Service) Delete(ctx context.Context, id string) error {
	if _, err := authz.Require(ctx, authz.PermUnitsWrite); err != nil {
		return err
	}
	return s.trail.Atomic(ctx, func(ctx context.Context) (audit.Entry, error) {
		before, err := s.repo.FindByID(ctx, id)
		if err != nil {
			return audit.Entry{}, err
		}
		if err = s.repo.Delete(ctx, id); err != nil {
			return audit.Entry{}, err
		}
		return audit.Entry{Action: audit.ActionDelete, Resource: auditResource, ResourceID: id, Before: before}, nil
	})
}

// UnitIDsOwnedBy returns the IDs of the shops owned by userID.
func (s *Service) UnitIDsOwnedBy(ctx context.Context, userID string) ([]string, error) {
	return s.repo.IDsByOwner(ctx, userID)
}
