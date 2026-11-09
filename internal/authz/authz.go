// Package authz holds the authorization rules shared by the domain services:
// who is calling (Principal) and which units they own (Units).
//
// Roles: ROLE_ADMIN manages the condominium (síndico/administração) and sees
// everything in it; ROLE_USER is a resident or shop owner and only sees the
// records tied to the units they own. Data from other condominiums is never
// visible: the repositories filter by the tenant in the context.
package authz

import (
	"context"
	"errors"
	"net/http"
	"slices"

	"github.com/victorradael/condoguard/api/internal/middleware"
)

// Role names stored in users.roles and in the token.
const (
	RoleAdmin = "ROLE_ADMIN"
	RoleUser  = "ROLE_USER"
)

// ErrForbidden is returned when the caller may not perform the operation.
var ErrForbidden = errors.New("authz: forbidden")

// ErrUnauthenticated is returned when there is no authenticated caller in the context.
var ErrUnauthenticated = errors.New("authz: no authenticated user")

// Status maps an authorization error to its HTTP status (401 or 403).
// ok is false for any other error.
func Status(err error) (status int, msg string, ok bool) {
	switch {
	case errors.Is(err, ErrForbidden):
		return http.StatusForbidden, "admin role required", true
	case errors.Is(err, ErrUnauthenticated):
		return http.StatusUnauthorized, "authentication required", true
	}
	return 0, "", false
}

// ValidRole reports whether role is one the API knows.
func ValidRole(role string) bool { return role == RoleAdmin || role == RoleUser }

// Principal is the authenticated caller.
type Principal struct {
	UserID string
	Roles  []string
}

// IsAdmin reports whether the caller manages the condominium.
func (p Principal) IsAdmin() bool { return slices.Contains(p.Roles, RoleAdmin) }

// FromContext returns the caller set by the authentication middleware.
func FromContext(ctx context.Context) (Principal, error) {
	id := middleware.UserIDFromContext(ctx)
	if id == "" {
		return Principal{}, ErrUnauthenticated
	}
	return Principal{UserID: id, Roles: middleware.RolesFromContext(ctx)}, nil
}

// RequireAdmin returns the caller if they are an admin, ErrForbidden otherwise.
func RequireAdmin(ctx context.Context) (Principal, error) {
	p, err := FromContext(ctx)
	if err != nil {
		return p, err
	}
	if !p.IsAdmin() {
		return p, ErrForbidden
	}
	return p, nil
}

// Units are the residential units and shops a user owns.
type Units struct {
	ResidentIDs  []string
	ShopOwnerIDs []string
}

// HasResident reports whether id is one of the user's residential units.
func (u Units) HasResident(id string) bool { return id != "" && slices.Contains(u.ResidentIDs, id) }

// HasShopOwner reports whether id is one of the user's shops.
func (u Units) HasShopOwner(id string) bool {
	return id != "" && slices.Contains(u.ShopOwnerIDs, id)
}

// UnitResolver finds the units owned by a user in the current condominium.
type UnitResolver interface {
	UnitsOwnedBy(ctx context.Context, userID string) (Units, error)
}

// UnitResolverFunc adapts a function to UnitResolver.
type UnitResolverFunc func(ctx context.Context, userID string) (Units, error)

// UnitsOwnedBy calls f.
func (f UnitResolverFunc) UnitsOwnedBy(ctx context.Context, userID string) (Units, error) {
	return f(ctx, userID)
}

// Scope says which records of a list the caller may see.
// All = true for admins; otherwise only records tied to Units.
type Scope struct {
	All   bool
	Units Units
}

// ScopeFor builds the list scope of the caller in ctx.
func ScopeFor(ctx context.Context, units UnitResolver) (Principal, Scope, error) {
	p, err := FromContext(ctx)
	if err != nil {
		return p, Scope{}, err
	}
	if p.IsAdmin() {
		return p, Scope{All: true}, nil
	}
	u, err := units.UnitsOwnedBy(ctx, p.UserID)
	if err != nil {
		return p, Scope{}, err
	}
	return p, Scope{Units: u}, nil
}

// Sees reports whether a record tied to residentID / shopOwnerID is visible in s.
func (s Scope) Sees(residentID, shopOwnerID string) bool {
	return s.All || s.Units.HasResident(residentID) || s.Units.HasShopOwner(shopOwnerID)
}

// SeesAny reports whether a record addressed to any of the given units is visible in s.
func (s Scope) SeesAny(residentIDs, shopOwnerIDs []string) bool {
	if s.All {
		return true
	}
	for _, id := range residentIDs {
		if s.Units.HasResident(id) {
			return true
		}
	}
	for _, id := range shopOwnerIDs {
		if s.Units.HasShopOwner(id) {
			return true
		}
	}
	return false
}
