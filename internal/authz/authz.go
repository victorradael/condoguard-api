// Package authz holds the authorization rules shared by the domain services:
// who is calling (Principal), what their roles in the current condominium
// allow (the permission matrix) and which units they own (Units).
//
// Roles belong to the membership of a person in a condominium, so one person
// can be síndico in one condominium and morador in another. The token carries
// the roles of the condominium it was issued for. A morador (and any role
// without the *ReadAll permission) only sees the records tied to the units
// they own. Data from other condominiums is never visible: the repositories
// filter by the tenant in the context and PostgreSQL Row-Level Security does
// it again. The matrix is documented in docs/PAPEIS_E_PERMISSOES.md.
package authz

import (
	"context"
	"errors"
	"net/http"
	"slices"

	"github.com/victorradael/condoguard/api/internal/middleware"
)

// Roles of a membership (memberships.roles).
const (
	RoleAdministradora = "administradora"
	RoleSindico        = "sindico"
	RoleConselho       = "conselho"
	RolePorteiro       = "porteiro"
	RoleMorador        = "morador"
)

// Roles lists every role, from the broadest to the narrowest.
var Roles = []string{RoleAdministradora, RoleSindico, RoleConselho, RolePorteiro, RoleMorador}

// Permission is one thing a role may do inside a condominium.
type Permission string

// Permissions checked by the services.
const (
	PermCondominiumManage    Permission = "condominium.manage"
	PermUsersManage          Permission = "users.manage"
	PermUsersRead            Permission = "users.read"
	PermUnitsWrite           Permission = "units.write"
	PermUnitsReadAll         Permission = "units.read_all"
	PermExpensesWrite        Permission = "expenses.write"
	PermExpensesReadAll      Permission = "expenses.read_all"
	PermNotificationsWrite   Permission = "notifications.write"
	PermNotificationsReadAll Permission = "notifications.read_all"
	PermAuditRead            Permission = "audit.read"
)

// Matrix says which permissions each role holds. Anything not listed is denied.
var Matrix = map[string][]Permission{
	RoleAdministradora: {PermCondominiumManage, PermUsersManage, PermUsersRead, PermUnitsWrite, PermUnitsReadAll,
		PermExpensesWrite, PermExpensesReadAll, PermNotificationsWrite, PermNotificationsReadAll, PermAuditRead},
	RoleSindico: {PermUsersManage, PermUsersRead, PermUnitsWrite, PermUnitsReadAll,
		PermExpensesWrite, PermExpensesReadAll, PermNotificationsWrite, PermNotificationsReadAll, PermAuditRead},
	RoleConselho: {PermUsersRead, PermUnitsReadAll, PermExpensesReadAll, PermNotificationsReadAll, PermAuditRead},
	RolePorteiro: {PermUnitsReadAll},
	RoleMorador:  {},
}

// ErrForbidden is returned when the caller may not perform the operation.
var ErrForbidden = errors.New("authz: forbidden")

// ErrUnauthenticated is returned when there is no authenticated caller in the context.
var ErrUnauthenticated = errors.New("authz: no authenticated user")

// Status maps an authorization error to its HTTP status (401 or 403).
// ok is false for any other error.
func Status(err error) (status int, msg string, ok bool) {
	switch {
	case errors.Is(err, ErrForbidden):
		return http.StatusForbidden, "insufficient role", true
	case errors.Is(err, ErrUnauthenticated):
		return http.StatusUnauthorized, "authentication required", true
	}
	return 0, "", false
}

// ValidRole reports whether role is one the API knows.
func ValidRole(role string) bool { return slices.Contains(Roles, role) }

// RolesCan reports whether any of the roles holds the permission.
func RolesCan(roles []string, perm Permission) bool {
	for _, r := range roles {
		if slices.Contains(Matrix[r], perm) {
			return true
		}
	}
	return false
}

// Principal is the authenticated caller.
type Principal struct {
	UserID string
	Roles  []string
}

// Can reports whether the caller's roles grant the permission.
func (p Principal) Can(perm Permission) bool { return RolesCan(p.Roles, perm) }

// OperatorID is the caller recorded by AsOperator.
const OperatorID = "operator"

// AsOperator returns ctx acting as the administradora of the condominium, for trusted tools
// that already hold database credentials (cmd/createadmin bootstraps the
// first admin this way). Never call it on a request path.
func AsOperator(ctx context.Context) context.Context {
	return middleware.WithIdentity(ctx, OperatorID, []string{RoleAdministradora})
}

// FromContext returns the caller set by the authentication middleware.
func FromContext(ctx context.Context) (Principal, error) {
	id := middleware.UserIDFromContext(ctx)
	if id == "" {
		return Principal{}, ErrUnauthenticated
	}
	return Principal{UserID: id, Roles: middleware.RolesFromContext(ctx)}, nil
}

// Require returns the caller if their roles grant perm, ErrForbidden otherwise.
func Require(ctx context.Context, perm Permission) (Principal, error) {
	p, err := FromContext(ctx)
	if err != nil {
		return p, err
	}
	if !p.Can(perm) {
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
// All = true when the caller holds the read-all permission of the resource;
// otherwise only records tied to Units.
type Scope struct {
	All   bool
	Units Units
}

// ScopeFor builds the list scope of the caller in ctx.
func ScopeFor(ctx context.Context, units UnitResolver, readAll Permission) (Principal, Scope, error) {
	p, err := FromContext(ctx)
	if err != nil {
		return p, Scope{}, err
	}
	if p.Can(readAll) {
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

// RequirePermission is a route guard: 403 unless the caller's roles grant
// perm. It must run after the authentication middleware.
func RequirePermission(perm Permission) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !RolesCan(middleware.RolesFromContext(r.Context()), perm) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":"insufficient role"}`))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// MFARoles are the roles that must sign in with a second factor: whoever
// manages or oversees a condominium (administradora, síndico, conselho).
var MFARoles = []string{RoleAdministradora, RoleSindico, RoleConselho}

// RequiresMFA reports whether any of the roles must use a second factor.
func RequiresMFA(roles []string) bool {
	for _, r := range roles {
		if slices.Contains(MFARoles, r) {
			return true
		}
	}
	return false
}
