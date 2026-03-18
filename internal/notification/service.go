package notification

import (
	"context"
	"fmt"
	"time"
)

// Service holds the business logic for the notification domain.
type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// Create validates the request and persists the notification.
// Business rules:
//   - message is required
//   - createdById is required
//   - at least one recipient (residentId or shopOwnerId) is required
//   - new notifications start as unread
func (s *Service) Create(ctx context.Context, req CreateRequest) (*Notification, error) {
	if req.Message == "" {
		return nil, fmt.Errorf("%w: message is required", ErrValidation)
	}
	if req.CreatedByID == "" {
		return nil, fmt.Errorf("%w: createdById is required", ErrValidation)
	}
	if len(req.ResidentIDs) == 0 && len(req.ShopOwnerIDs) == 0 {
		return nil, fmt.Errorf("%w: at least one recipient (residentId or shopOwnerId) is required", ErrValidation)
	}

	n := &Notification{
		Message:      req.Message,
		CreatedByID:  req.CreatedByID,
		CreatedAt:    time.Now().UTC(),
		Read:         false,
		ResidentIDs:  req.ResidentIDs,
		ShopOwnerIDs: req.ShopOwnerIDs,
	}
	if n.ResidentIDs == nil {
		n.ResidentIDs = []string{}
	}
	if n.ShopOwnerIDs == nil {
		n.ShopOwnerIDs = []string{}
	}

	if err := s.repo.Save(ctx, n); err != nil {
		return nil, err
	}
	return n, nil
}

func (s *Service) GetByID(ctx context.Context, id string) (*Notification, error) {
	return s.repo.FindByID(ctx, id)
}

func (s *Service) List(ctx context.Context) ([]*Notification, error) {
	return s.repo.FindAll(ctx)
}

// Update applies message and recipient list changes.
func (s *Service) Update(ctx context.Context, id string, req UpdateRequest) (*Notification, error) {
	n, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if req.Message != "" {
		n.Message = req.Message
	}
	if len(req.ResidentIDs) > 0 {
		n.ResidentIDs = req.ResidentIDs
	}
	if len(req.ShopOwnerIDs) > 0 {
		n.ShopOwnerIDs = req.ShopOwnerIDs
	}

	if err := s.repo.Update(ctx, n); err != nil {
		return nil, err
	}
	return n, nil
}

// MarkAsRead transitions the notification to the read state.
// Business rule: the operation is idempotent — calling it on an already-read
// notification must succeed without changing ReadAt.
func (s *Service) MarkAsRead(ctx context.Context, id string) (*Notification, error) {
	n, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if n.Read {
		// Already read — return as-is (idempotent).
		return n, nil
	}

	now := time.Now().UTC()
	n.Read = true
	n.ReadAt = &now

	if err := s.repo.Update(ctx, n); err != nil {
		return nil, err
	}
	return n, nil
}

func (s *Service) Delete(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}
