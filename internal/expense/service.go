package expense

import (
	"context"
	"fmt"

	"github.com/victorradael/condoguard/api/internal/audit"
	"github.com/victorradael/condoguard/api/internal/authz"
	"github.com/victorradael/condoguard/api/pkg/page"
)

// Service holds the business logic for the expense domain.
//
// Authorization: admins manage every expense of the condominium; other users
// only read the expenses of the units and shops they own. An expense the
// caller may not see is reported as not found.
type Service struct {
	repo  Repository
	units authz.UnitResolver
	trail audit.Trail
}

const auditResource = "expense"

// WithAudit makes every change write its audit entry in the same transaction.
func (s *Service) WithAudit(t audit.Trail) *Service {
	s.trail = t
	return s
}

// NewService creates an expense Service. units resolves which units and
// shops a non-admin caller owns.
func NewService(repo Repository, units authz.UnitResolver) *Service {
	return &Service{repo: repo, units: units, trail: audit.NewTrail()}
}

// Create validates the request and persists the expense.
// Business rules:
//   - description is required
//   - amountCents must be positive (centavos, integer)
//   - dueDate is required (non-zero)
//   - must be linked to exactly one unit (residentId OR shopOwnerId)
//   - needs expenses.write
func (s *Service) Create(ctx context.Context, req CreateRequest) (*Expense, error) {
	if _, err := authz.Require(ctx, authz.PermExpensesWrite); err != nil {
		return nil, err
	}
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

	err := s.trail.Atomic(ctx, func(ctx context.Context) (audit.Entry, error) {
		if err := s.repo.Save(ctx, e); err != nil {
			return audit.Entry{}, err
		}
		return audit.Entry{Action: audit.ActionCreate, Resource: auditResource, ResourceID: e.ID, After: e}, nil
	})
	if err != nil {
		return nil, err
	}
	return e, nil
}

// GetByID returns the expense if the caller may see it, ErrNotFound otherwise.
func (s *Service) GetByID(ctx context.Context, id string) (*Expense, error) {
	_, scope, err := authz.ScopeFor(ctx, s.units, authz.PermExpensesReadAll)
	if err != nil {
		return nil, err
	}
	e, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !scope.Sees(e.ResidentID, e.ShopOwnerID) {
		return nil, ErrNotFound
	}
	return e, nil
}

// List returns one page of the expenses the caller may see. f.Scope is
// always set from the caller; any value passed in is ignored.
func (s *Service) List(ctx context.Context, f Filter, p page.Params) (page.Result[*Expense], error) {
	_, scope, err := authz.ScopeFor(ctx, s.units, authz.PermExpensesReadAll)
	if err != nil {
		return page.Result[*Expense]{}, err
	}
	f.Scope = scope
	list, total, err := s.repo.FindAll(ctx, f, p)
	if err != nil {
		return page.Result[*Expense]{}, err
	}
	return page.New(list, p, total), nil
}

// Update applies description, amountCents and dueDate changes. Needs expenses.write.
// amountCents must be positive if provided (non-zero).
func (s *Service) Update(ctx context.Context, id string, req UpdateRequest) (*Expense, error) {
	if _, err := authz.Require(ctx, authz.PermExpensesWrite); err != nil {
		return nil, err
	}
	e, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	before := *e

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

	err = s.trail.Atomic(ctx, func(ctx context.Context) (audit.Entry, error) {
		if err := s.repo.Update(ctx, e); err != nil {
			return audit.Entry{}, err
		}
		return audit.Entry{Action: audit.ActionUpdate, Resource: auditResource, ResourceID: e.ID, Before: before, After: e}, nil
	})
	if err != nil {
		return nil, err
	}
	return e, nil
}

// Delete removes the expense. Needs expenses.write.
func (s *Service) Delete(ctx context.Context, id string) error {
	if _, err := authz.Require(ctx, authz.PermExpensesWrite); err != nil {
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
