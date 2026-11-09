package shopowner

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/victorradael/condoguard/api/internal/authz"
	"github.com/victorradael/condoguard/api/pkg/page"
)

// Prefixes the routes are served under. /shop-owners is the canonical path
// (the one the web client uses); /shopOwners is kept for older clients.
var Prefixes = []string{"/shop-owners", "/shopOwners"}

// NewHandler wires the shopowner CRUD routes behind the provided auth middleware.
// Admins manage all shops; other users only read their own (see Service).
func NewHandler(svc *Service, authMW func(http.Handler) http.Handler) http.Handler {
	h := &handler{svc: svc}

	mux := http.NewServeMux()
	for _, p := range Prefixes {
		mux.HandleFunc("GET "+p, h.list)
		mux.HandleFunc("POST "+p, h.create)
		mux.HandleFunc("GET "+p+"/{id}", h.getByID)
		mux.HandleFunc("PUT "+p+"/{id}", h.update)
		mux.HandleFunc("DELETE "+p+"/{id}", h.delete)
	}

	return authMW(mux)
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
	list, err := h.svc.List(r.Context(), p)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (h *handler) create(w http.ResponseWriter, r *http.Request) {
	var req CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	shop, err := h.svc.Create(r.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, ErrValidation):
			writeError(w, http.StatusUnprocessableEntity, err.Error())
		case errors.Is(err, ErrDuplicate):
			writeError(w, http.StatusConflict, "CNPJ already registered")
		default:
			writeServiceError(w, err)
		}
		return
	}
	writeJSON(w, http.StatusCreated, shop)
}

func (h *handler) getByID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	shop, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusNotFound, "shopowner not found")
			return
		}
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, shop)
}

func (h *handler) update(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req UpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	shop, err := h.svc.Update(r.Context(), id, req)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusNotFound, "shopowner not found")
			return
		}
		if errors.Is(err, ErrValidation) {
			writeError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, shop)
}

func (h *handler) delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.svc.Delete(r.Context(), id); err != nil {
		if errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusNotFound, "shopowner not found")
			return
		}
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// writeServiceError answers 401/403 for authorization errors and 500 otherwise.
func writeServiceError(w http.ResponseWriter, err error) {
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
