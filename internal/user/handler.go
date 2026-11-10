package user

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/victorradael/condoguard/api/internal/authz"
	"github.com/victorradael/condoguard/api/internal/middleware"
	"github.com/victorradael/condoguard/api/pkg/page"
)

// NewHandler wires the user routes behind the provided auth middleware.
// GET /users/me is open to any authenticated user; the other routes are
// checked by the service against the permission matrix (users.read for
// reading, users.manage for changes).
func NewHandler(svc *Service, authMW func(http.Handler) http.Handler) http.Handler {
	h := &handler{svc: svc}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /users/me", h.me)
	mux.HandleFunc("GET /users", h.list)
	mux.HandleFunc("POST /users", h.create)
	mux.HandleFunc("GET /users/{id}", h.getByID)
	mux.HandleFunc("PUT /users/{id}", h.update)
	mux.HandleFunc("DELETE /users/{id}", h.delete)
	mux.HandleFunc("POST /users/{id}/account-links", h.issueLink)
	mux.HandleFunc("DELETE /users/{id}/2fa", h.resetTwoFactor)

	return authMW(mux)
}

// me returns the authenticated user's own account (id, username, email, roles).
func (h *handler) me(w http.ResponseWriter, r *http.Request) {
	u, err := h.svc.GetByID(r.Context(), middleware.UserIDFromContext(r.Context()))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			// The token outlived the account.
			writeError(w, http.StatusUnauthorized, "user no longer exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, u)
}

type handler struct {
	svc *Service
}

func (h *handler) list(w http.ResponseWriter, r *http.Request) {
	p, err := page.FromRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	users, err := h.svc.List(r.Context(), p)
	if err != nil {
		failed(w, err)
		return
	}
	writeJSON(w, http.StatusOK, users)
}

func (h *handler) create(w http.ResponseWriter, r *http.Request) {
	var req CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	u, err := h.svc.Create(r.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, ErrValidation):
			writeError(w, http.StatusUnprocessableEntity, err.Error())
		case errors.Is(err, ErrDuplicate):
			writeError(w, http.StatusConflict, "email already registered")
		default:
			failed(w, err)
		}
		return
	}
	writeJSON(w, http.StatusCreated, u)
}

func (h *handler) getByID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	u, err := h.svc.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		failed(w, err)
		return
	}
	writeJSON(w, http.StatusOK, u)
}

func (h *handler) update(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req UpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	u, err := h.svc.Update(r.Context(), id, req)
	if err != nil {
		switch {
		case errors.Is(err, ErrNotFound):
			writeError(w, http.StatusNotFound, "user not found")
		case errors.Is(err, ErrValidation):
			writeError(w, http.StatusUnprocessableEntity, err.Error())
		default:
			failed(w, err)
		}
		return
	}
	writeJSON(w, http.StatusOK, u)
}

func (h *handler) delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.svc.Delete(r.Context(), id); err != nil {
		if errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		failed(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// failed answers an error the handler has no specific mapping for: the
// service's authorization errors keep their status, anything else is a 500.
func failed(w http.ResponseWriter, err error) {
	if status, msg, ok := authz.Status(err); ok {
		writeError(w, status, msg)
		return
	}
	writeError(w, http.StatusInternalServerError, "internal error")
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
