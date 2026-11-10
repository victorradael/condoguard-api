package account

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
)

// NewHandler serves the public account routes. They need no session: the
// token (or the e-mail) in the body is the credential. The router puts the
// per-IP limit in front of them.
//
//	POST /auth/password/forgot   {email}                  → 202, same answer for any e-mail
//	POST /auth/password/reset    {token, newPassword}     → 204
//	POST /auth/email/verify      {token}                  → 204
//	POST /auth/email/resend      {email}                  → 202, same answer for any e-mail
func NewHandler(svc *Service) http.Handler {
	h := &handler{svc: svc}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /auth/password/forgot", h.forgot)
	mux.HandleFunc("POST /auth/password/reset", h.reset)
	mux.HandleFunc("POST /auth/email/verify", h.verify)
	mux.HandleFunc("POST /auth/email/resend", h.resend)
	return noStore(mux)
}

type handler struct{ svc *Service }

func noStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

const accepted = "If the e-mail has an account, we sent instructions to it."

func (h *handler) forgot(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	if !decode(w, r, &req) {
		return
	}
	if err := h.svc.RequestPasswordReset(r.Context(), req.Email); err != nil {
		slog.ErrorContext(r.Context(), "account: forgot", "error", err)
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"message": accepted})
}

func (h *handler) resend(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	if !decode(w, r, &req) {
		return
	}
	if err := h.svc.RequestVerification(r.Context(), req.Email); err != nil {
		slog.ErrorContext(r.Context(), "account: resend", "error", err)
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"message": accepted})
}

func (h *handler) reset(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token       string `json:"token"`
		NewPassword string `json:"newPassword"`
	}
	if !decode(w, r, &req) {
		return
	}
	switch err := h.svc.ResetPassword(r.Context(), req.Token, req.NewPassword); {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, ErrInvalidToken):
		writeError(w, http.StatusBadRequest, "invalid or expired token")
	case errors.Is(err, ErrValidation):
		writeError(w, http.StatusUnprocessableEntity, err.Error())
	default:
		slog.ErrorContext(r.Context(), "account: reset", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}

func (h *handler) verify(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token string `json:"token"`
	}
	if !decode(w, r, &req) {
		return
	}
	switch err := h.svc.VerifyEmail(r.Context(), req.Token); {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, ErrInvalidToken):
		writeError(w, http.StatusBadRequest, "invalid or expired token")
	default:
		slog.ErrorContext(r.Context(), "account: verify", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
