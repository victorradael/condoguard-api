package expense

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"
)

// NewHandler wires the expense CRUD routes behind the provided auth middleware.
// Any authenticated user may access these routes.
func NewHandler(svc *Service, authMW func(http.Handler) http.Handler) http.Handler {
	h := &handler{svc: svc}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /expenses", h.list)
	mux.HandleFunc("POST /expenses", h.create)
	mux.HandleFunc("GET /expenses/{id}", h.getByID)
	mux.HandleFunc("PUT /expenses/{id}", h.update)
	mux.HandleFunc("DELETE /expenses/{id}", h.delete)

	return authMW(mux)
}

type handler struct {
	svc *Service
}

// list handles GET /expenses with optional ?from=<RFC3339>&to=<RFC3339> query params.
func (h *handler) list(w http.ResponseWriter, r *http.Request) {
	var f Filter

	if raw := r.URL.Query().Get("from"); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid 'from' date: use RFC3339 format (e.g. 2026-01-01T00:00:00Z)")
			return
		}
		f.From = &t
	}
	if raw := r.URL.Query().Get("to"); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid 'to' date: use RFC3339 format")
			return
		}
		f.To = &t
	}

	list, err := h.svc.List(r.Context(), f)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// createBody is the HTTP-layer representation of CreateRequest.
// DueDate is accepted as an RFC3339 string so JSON unmarshaling works correctly.
type createBody struct {
	Description string `json:"description"`
	AmountCents int64  `json:"amountCents"`
	DueDate     string `json:"dueDate"`
	ResidentID  string `json:"residentId"`
	ShopOwnerID string `json:"shopOwnerId"`
}

func (h *handler) create(w http.ResponseWriter, r *http.Request) {
	var body createBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	req := CreateRequest{
		Description: body.Description,
		AmountCents: body.AmountCents,
		ResidentID:  body.ResidentID,
		ShopOwnerID: body.ShopOwnerID,
	}
	if body.DueDate != "" {
		t, err := time.Parse(time.RFC3339, body.DueDate)
		if err != nil {
			writeError(w, http.StatusUnprocessableEntity, "dueDate must be RFC3339 format")
			return
		}
		req.DueDate = t
	}

	e, err := h.svc.Create(r.Context(), req)
	if err != nil {
		if errors.Is(err, ErrValidation) {
			writeError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, e)
}

// updateBody mirrors createBody for PUT requests.
type updateBody struct {
	Description string `json:"description"`
	AmountCents int64  `json:"amountCents"`
	DueDate     string `json:"dueDate"`
}

func (h *handler) update(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body updateBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	req := UpdateRequest{
		Description: body.Description,
		AmountCents: body.AmountCents,
	}
	if body.DueDate != "" {
		t, err := time.Parse(time.RFC3339, body.DueDate)
		if err != nil {
			writeError(w, http.StatusUnprocessableEntity, "dueDate must be RFC3339 format")
			return
		}
		req.DueDate = t
	}

	e, err := h.svc.Update(r.Context(), id, req)
	if err != nil {
		switch {
		case errors.Is(err, ErrNotFound):
			writeError(w, http.StatusNotFound, "expense not found")
		case errors.Is(err, ErrValidation):
			writeError(w, http.StatusUnprocessableEntity, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	writeJSON(w, http.StatusOK, e)
}

func (h *handler) getByID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	e, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusNotFound, "expense not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, e)
}

func (h *handler) delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.svc.Delete(r.Context(), id); err != nil {
		if errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusNotFound, "expense not found")
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
