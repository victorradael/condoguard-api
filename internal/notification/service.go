package notification

import (
	"context"
	"fmt"
	"time"

	"github.com/victorradael/condoguard/api/internal/audit"
	"github.com/victorradael/condoguard/api/internal/authz"
	"github.com/victorradael/condoguard/api/pkg/page"
)

// Service holds the business logic for the notification domain.
//
// Authorization: admins send, edit and delete notifications and see all of
// them; other users only see the ones addressed to a unit or shop they own.
// A notification the caller may not see is reported as not found.
type Service struct {
	repo  Repository
	units authz.UnitResolver
	trail audit.Trail
}

const auditResource = "notification"

// WithAudit makes every change write its audit entry in the same transaction.
func (s *Service) WithAudit(t audit.Trail) *Service {
	s.trail = t
	return s
}

// NewService creates a notification Service. units resolves which units and
// shops a non-admin caller owns.
func NewService(repo Repository, units authz.UnitResolver) *Service {
	return &Service{repo: repo, units: units, trail: audit.NewTrail()}
}

// Create validates the request and persists the notification.
// Business rules:
//   - needs notifications.write
//   - the author (createdById) is the authenticated user
//   - message is required
//   - at least one recipient (residentId or shopOwnerId) is required
//   - new notifications start unread for every recipient
func (s *Service) Create(ctx context.Context, req CreateRequest) (*Notification, error) {
	p, err := authz.Require(ctx, authz.PermNotificationsWrite)
	if err != nil {
		return nil, err
	}
	if req.Message == "" {
		return nil, fmt.Errorf("%w: message is required", ErrValidation)
	}
	if len(req.ResidentIDs) == 0 && len(req.ShopOwnerIDs) == 0 {
		return nil, fmt.Errorf("%w: at least one recipient (residentId or shopOwnerId) is required", ErrValidation)
	}

	n := &Notification{
		Message:      req.Message,
		CreatedByID:  p.UserID,
		CreatedAt:    time.Now().UTC(),
		ResidentIDs:  req.ResidentIDs,
		ShopOwnerIDs: req.ShopOwnerIDs,
	}
	if n.ResidentIDs == nil {
		n.ResidentIDs = []string{}
	}
	if n.ShopOwnerIDs == nil {
		n.ShopOwnerIDs = []string{}
	}

	err = s.trail.Atomic(ctx, func(ctx context.Context) (audit.Entry, error) {
		if err = s.repo.Save(ctx, n); err != nil {
			return audit.Entry{}, err
		}
		return audit.Entry{Action: audit.ActionCreate, Resource: auditResource, ResourceID: n.ID, After: n}, nil
	})
	if err != nil {
		return nil, err
	}
	return n, nil
}

// GetByID returns the notification, with the caller's read state, if the
// caller may see it; ErrNotFound otherwise.
func (s *Service) GetByID(ctx context.Context, id string) (*Notification, error) {
	p, scope, err := authz.ScopeFor(ctx, s.units, authz.PermNotificationsReadAll)
	if err != nil {
		return nil, err
	}
	n, err := s.repo.FindByID(ctx, id, p.UserID)
	if err != nil {
		return nil, err
	}
	if !scope.SeesAny(n.ResidentIDs, n.ShopOwnerIDs) {
		return nil, ErrNotFound
	}
	return n, nil
}

// List returns one page of the notifications the caller may see, newest first.
func (s *Service) List(ctx context.Context, pg page.Params) (page.Result[*Notification], error) {
	p, scope, err := authz.ScopeFor(ctx, s.units, authz.PermNotificationsReadAll)
	if err != nil {
		return page.Result[*Notification]{}, err
	}
	list, total, err := s.repo.FindAll(ctx, scope, p.UserID, pg)
	if err != nil {
		return page.Result[*Notification]{}, err
	}
	return page.New(list, pg, total), nil
}

// Update applies message and recipient list changes. Needs notifications.write.
// A recipient list present in the body replaces the stored one, even when
// empty ([] clears it); an absent (or null) list is kept. The notification
// must still have at least one recipient afterwards.
func (s *Service) Update(ctx context.Context, id string, req UpdateRequest) (*Notification, error) {
	p, err := authz.Require(ctx, authz.PermNotificationsWrite)
	if err != nil {
		return nil, err
	}
	n, err := s.repo.FindByID(ctx, id, p.UserID)
	if err != nil {
		return nil, err
	}
	before := *n

	if req.Message != "" {
		n.Message = req.Message
	}
	if req.ResidentIDs != nil {
		n.ResidentIDs = req.ResidentIDs
	}
	if req.ShopOwnerIDs != nil {
		n.ShopOwnerIDs = req.ShopOwnerIDs
	}
	if len(n.ResidentIDs) == 0 && len(n.ShopOwnerIDs) == 0 {
		return nil, fmt.Errorf("%w: at least one recipient (residentId or shopOwnerId) is required", ErrValidation)
	}

	err = s.trail.Atomic(ctx, func(ctx context.Context) (audit.Entry, error) {
		if err = s.repo.Update(ctx, n); err != nil {
			return audit.Entry{}, err
		}
		return audit.Entry{Action: audit.ActionUpdate, Resource: auditResource, ResourceID: n.ID, Before: before, After: n}, nil
	})
	if err != nil {
		return nil, err
	}
	return n, nil
}

// MarkAsRead records that the caller read the notification.
// Business rules: only users who can see the notification may mark it; the
// read state is per user (one recipient reading it does not mark it for the
// others); the operation is idempotent — the first ReadAt is kept.
func (s *Service) MarkAsRead(ctx context.Context, id string) (*Notification, error) {
	n, err := s.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if n.Read {
		return n, nil
	}

	p, err := authz.FromContext(ctx)
	if err != nil {
		return nil, err
	}
	at, err := s.repo.MarkRead(ctx, id, p.UserID)
	if err != nil {
		return nil, err
	}
	n.Read, n.ReadAt = true, &at
	return n, nil
}

// Delete removes the notification. Needs notifications.write.
func (s *Service) Delete(ctx context.Context, id string) error {
	p, err := authz.Require(ctx, authz.PermNotificationsWrite)
	if err != nil {
		return err
	}
	return s.trail.Atomic(ctx, func(ctx context.Context) (audit.Entry, error) {
		before, err := s.repo.FindByID(ctx, id, p.UserID)
		if err != nil {
			return audit.Entry{}, err
		}
		if err = s.repo.Delete(ctx, id); err != nil {
			return audit.Entry{}, err
		}
		return audit.Entry{Action: audit.ActionDelete, Resource: auditResource, ResourceID: id, Before: before}, nil
	})
}
