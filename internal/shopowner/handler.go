package shopowner

import (
	"encoding/json"
	"errors"
	"net/http"
)

// NewHandler wires the shopowner CRUD routes behind the provided auth middleware.
// Any authenticated user may access these routes.
func NewHandler(svc *Service, authMW func(http.Handler) http.Handler) http.Handler {
	h := &handler{svc: svc}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /shopOwners", h.list)
	mux.HandleFunc("POST /shopOwners", h.create)
	mux.HandleFunc("GET /shopOwners/{id}", h.getByID)
	mux.HandleFunc("PUT /shopOwners/{id}", h.update)
	mux.HandleFunc("DELETE /shopOwners/{id}", h.delete)

	return authMW(mux)
}

type handler struct {
	svc *Service
}

func (h *handler) list(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
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
			writeError(w, http.StatusInternalServerError, "internal error")
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
		writeError(w, http.StatusInternalServerError, "internal error")
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
		writeError(w, http.StatusInternalServerError, "internal error")
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
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
