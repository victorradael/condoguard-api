package expense

import (
	"context"
	"fmt"
)

// Service holds the business logic for the expense domain.
type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// Create validates the request and persists the expense.
// Business rules:
//   - description is required
//   - amountCents must be positive (centavos, integer)
//   - dueDate is required (non-zero)
//   - must be linked to exactly one unit (residentId OR shopOwnerId)
func (s *Service) Create(ctx context.Context, req CreateRequest) (*Expense, error) {
	if req.Description == "" {
		return nil, fmt.Errorf("%w: description is required", ErrValidation)
	}
	if req.AmountCents < 0 {
		return nil, fmt.Errorf("%w: amountCents must be zero or positive", ErrValidation)
	}
	if req.DueDate.IsZero() {
		return nil, fmt.Errorf("%w: dueDate is required", ErrValidation)
	}
	if req.ResidentID == "" && req.ShopOwnerID == "" {
		return nil, fmt.Errorf("%w: expense must be linked to a residentId or shopOwnerId", ErrValidation)
	}

	e := &Expense{
		Description: req.Description,
		AmountCents: req.AmountCents,
		DueDate:     req.DueDate.UTC(),
		ResidentID:  req.ResidentID,
		ShopOwnerID: req.ShopOwnerID,
	}

	if err := s.repo.Save(ctx, e); err != nil {
		return nil, err
	}
	return e, nil
}

func (s *Service) GetByID(ctx context.Context, id string) (*Expense, error) {
	return s.repo.FindByID(ctx, id)
}

func (s *Service) List(ctx context.Context, f Filter) ([]*Expense, error) {
	return s.repo.FindAll(ctx, f)
}

// Update applies description, amountCents and dueDate changes.
// amountCents must be positive if provided (non-zero).
func (s *Service) Update(ctx context.Context, id string, req UpdateRequest) (*Expense, error) {
	e, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if req.AmountCents < 0 {
		return nil, fmt.Errorf("%w: amountCents must be zero or positive", ErrValidation)
	}

	if req.Description != "" {
		e.Description = req.Description
	}
	if req.AmountCents > 0 {
		e.AmountCents = req.AmountCents
	}
	if !req.DueDate.IsZero() {
		e.DueDate = req.DueDate.UTC()
	}

	if err := s.repo.Update(ctx, e); err != nil {
		return nil, err
	}
	return e, nil
}

func (s *Service) Delete(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}


