package authz_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/victorradael/condoguard/api/internal/authz"
	"github.com/victorradael/condoguard/api/internal/middleware"
)

// The matrix as documented in docs/PAPEIS_E_PERMISSOES.md. It is repeated
// here on purpose: changing a permission must be a deliberate edit of both.
var documented = map[string]map[authz.Permission]bool{
	authz.RoleAdministradora: {
		authz.PermCondominiumManage: true, authz.PermUsersManage: true, authz.PermUsersRead: true,
		authz.PermUnitsWrite: true, authz.PermUnitsReadAll: true, authz.PermExpensesWrite: true, authz.PermExpensesReadAll: true,
		authz.PermNotificationsWrite: true, authz.PermNotificationsReadAll: true, authz.PermAuditRead: true,
		authz.PermPeopleWrite: true, authz.PermPeopleReadAll: true, authz.PermPeopleReadContact: true, authz.PermPeopleReadCPF: true,
	},
	authz.RoleSindico: {
		authz.PermUsersManage: true, authz.PermUsersRead: true,
		authz.PermUnitsWrite: true, authz.PermUnitsReadAll: true, authz.PermExpensesWrite: true, authz.PermExpensesReadAll: true,
		authz.PermNotificationsWrite: true, authz.PermNotificationsReadAll: true, authz.PermAuditRead: true,
		authz.PermPeopleWrite: true, authz.PermPeopleReadAll: true, authz.PermPeopleReadContact: true, authz.PermPeopleReadCPF: true,
	},
	authz.RoleConselho: {
		authz.PermUsersRead: true, authz.PermUnitsReadAll: true, authz.PermExpensesReadAll: true,
		authz.PermNotificationsReadAll: true, authz.PermAuditRead: true,
		authz.PermPeopleReadAll: true, authz.PermPeopleReadContact: true,
	},
	authz.RolePorteiro: {authz.PermUnitsReadAll: true, authz.PermPeopleReadAll: true},
	authz.RoleMorador:  {},
}

var allPermissions = []authz.Permission{
	authz.PermCondominiumManage, authz.PermUsersManage, authz.PermUsersRead, authz.PermUnitsWrite, authz.PermUnitsReadAll,
	authz.PermExpensesWrite, authz.PermExpensesReadAll, authz.PermNotificationsWrite, authz.PermNotificationsReadAll, authz.PermAuditRead,
	authz.PermPeopleWrite, authz.PermPeopleReadAll, authz.PermPeopleReadContact, authz.PermPeopleReadCPF,
}

func TestMatrix_MatchesTheDocumentedOne(t *testing.T) {
	if len(authz.Roles) != len(documented) {
		t.Fatalf("roles: %v, documented %d", authz.Roles, len(documented))
	}
	for _, role := range authz.Roles {
		for _, perm := range allPermissions {
			if got, want := authz.RolesCan([]string{role}, perm), documented[role][perm]; got != want {
				t.Errorf("%s / %s: got %v, documented %v", role, perm, got, want)
			}
		}
	}
	// Every permission in the matrix is one the tests above know about.
	known := map[authz.Permission]bool{}
	for _, p := range allPermissions {
		known[p] = true
	}
	for role, perms := range authz.Matrix {
		for _, p := range perms {
			if !known[p] {
				t.Errorf("%s holds unknown permission %s", role, p)
			}
		}
	}
}

func TestMatrix_UnknownRolesGetNothingAndMultipleRolesAdd(t *testing.T) {
	for _, perm := range allPermissions {
		if authz.RolesCan([]string{"ROLE_ADMIN", "root", ""}, perm) || authz.RolesCan(nil, perm) {
			t.Errorf("an unknown or missing role got %s", perm)
		}
	}
	if !authz.RolesCan([]string{authz.RoleMorador, authz.RoleConselho}, authz.PermAuditRead) {
		t.Error("roles of one person must add up")
	}
	if authz.RolesCan([]string{authz.RoleMorador, authz.RoleConselho}, authz.PermUnitsWrite) {
		t.Error("conselho plus morador must not write units")
	}
}

func TestValidRole(t *testing.T) {
	for _, r := range authz.Roles {
		if !authz.ValidRole(r) {
			t.Errorf("%s must be valid", r)
		}
	}
	for _, r := range []string{"", "ROLE_ADMIN", "ROLE_USER", "admin", "Sindico"} {
		if authz.ValidRole(r) {
			t.Errorf("%q must be invalid", r)
		}
	}
}

func TestRequire(t *testing.T) {
	as := func(roles ...string) context.Context {
		return middleware.WithIdentity(context.Background(), "u1", roles)
	}
	if _, err := authz.Require(context.Background(), authz.PermAuditRead); !errors.Is(err, authz.ErrUnauthenticated) {
		t.Errorf("anonymous: expected ErrUnauthenticated, got %v", err)
	}
	if _, err := authz.Require(as(authz.RoleMorador), authz.PermAuditRead); !errors.Is(err, authz.ErrForbidden) {
		t.Errorf("morador: expected ErrForbidden, got %v", err)
	}
	if p, err := authz.Require(as(authz.RoleConselho), authz.PermAuditRead); err != nil || p.UserID != "u1" {
		t.Errorf("conselho: %+v, %v", p, err)
	}
}

func TestRequirePermission_Middleware(t *testing.T) {
	h := authz.RequirePermission(authz.PermUsersManage)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	for role, want := range map[string]int{
		authz.RoleAdministradora: http.StatusNoContent, authz.RoleSindico: http.StatusNoContent,
		authz.RoleConselho: http.StatusForbidden, authz.RolePorteiro: http.StatusForbidden, authz.RoleMorador: http.StatusForbidden,
		"ROLE_ADMIN": http.StatusForbidden,
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil).WithContext(middleware.WithIdentity(context.Background(), "u", []string{role})))
		if rec.Code != want {
			t.Errorf("%s: expected %d, got %d", role, want, rec.Code)
		}
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusForbidden {
		t.Errorf("no identity: expected 403, got %d", rec.Code)
	}
}
