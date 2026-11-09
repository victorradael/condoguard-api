package user

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/victorradael/condoguard/api/internal/account"
	"github.com/victorradael/condoguard/api/internal/audit"
	"github.com/victorradael/condoguard/api/internal/authz"
)

// Audit actions of the account-recovery help a síndico gives.
const (
	ActionIssueLink = "issue_link"
	ActionReset2FA  = "reset_2fa"
)

// Links issues the recovery/verification links the síndico passes on.
type Links interface {
	IssueLink(ctx context.Context, userID, purpose, issuerID string) (account.Link, error)
}

// TwoFactor removes the second factor of an account of the condominium.
type TwoFactor interface {
	ResetTwoFactor(ctx context.Context, userID string) error
}

// WithAccountHelp turns on the routes where a síndico helps a member who lost
// access: a link/code to pass on (WhatsApp, in the PoC) and a 2FA reset.
func (s *Service) WithAccountHelp(l Links, t TwoFactor) *Service {
	s.links, s.twoFactor = l, t
	return s
}

// IssueLink makes a one-time link for another member of the condominium.
// Needs users.manage, and the target must not outrank the caller (a síndico
// cannot take over an administradora's account). The token is returned once
// and never written to the audit trail.
func (s *Service) IssueLink(ctx context.Context, id, purpose string) (account.Link, error) {
	var link account.Link
	p, err := authz.Require(ctx, authz.PermUsersManage)
	if err != nil {
		return link, err
	}
	if s.links == nil {
		return link, errors.New("user: account links are not configured")
	}
	err = s.trail.Atomic(ctx, func(ctx context.Context) (audit.Entry, error) {
		target, err := s.repo.FindByID(ctx, id)
		if err != nil {
			return audit.Entry{}, err
		}
		if !mayGrant(p, target.Roles) {
			return audit.Entry{}, authz.ErrForbidden
		}
		if link, err = s.links.IssueLink(ctx, id, purpose, p.UserID); err != nil {
			return audit.Entry{}, err
		}
		return audit.Entry{Action: ActionIssueLink, Resource: auditResource, ResourceID: id,
			After: map[string]any{"purpose": purpose, "expiresAt": link.ExpiresAt}}, nil
	})
	return link, err
}

// ResetTwoFactor removes the second factor of another member (lost phone and
// codes) and ends their sessions. Same authorization as IssueLink.
func (s *Service) ResetTwoFactor(ctx context.Context, id string) error {
	p, err := authz.Require(ctx, authz.PermUsersManage)
	if err != nil {
		return err
	}
	if s.twoFactor == nil {
		return errors.New("user: two-factor reset is not configured")
	}
	return s.trail.Atomic(ctx, func(ctx context.Context) (audit.Entry, error) {
		target, err := s.repo.FindByID(ctx, id)
		if err != nil {
			return audit.Entry{}, err
		}
		if !mayGrant(p, target.Roles) {
			return audit.Entry{}, authz.ErrForbidden
		}
		if err := s.twoFactor.ResetTwoFactor(ctx, id); err != nil {
			return audit.Entry{}, err
		}
		return audit.Entry{Action: ActionReset2FA, Resource: auditResource, ResourceID: id}, nil
	})
}

func (h *handler) issueLink(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Purpose string `json:"purpose"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	link, err := h.svc.IssueLink(r.Context(), r.PathValue("id"), req.Purpose)
	switch {
	case err == nil:
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusCreated, link)
	case errors.Is(err, ErrNotFound):
		writeError(w, http.StatusNotFound, "user not found")
	case errors.Is(err, account.ErrValidation):
		writeError(w, http.StatusUnprocessableEntity, err.Error())
	default:
		failed(w, err)
	}
}

func (h *handler) resetTwoFactor(w http.ResponseWriter, r *http.Request) {
	err := h.svc.ResetTwoFactor(r.Context(), r.PathValue("id"))
	switch {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, ErrNotFound):
		writeError(w, http.StatusNotFound, "user not found")
	default:
		failed(w, err)
	}
}
