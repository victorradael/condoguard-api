package auth

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/victorradael/condoguard/api/internal/middleware"
)

// CookieOptions configures the session cookies of the web flow.
type CookieOptions struct {
	// Secure marks the cookies HTTPS-only. Only turn it off for plain-HTTP
	// local development.
	Secure bool
	// Domain is the cookie domain; empty means the API host only.
	Domain string
	// PathPrefix is the path the browser uses to reach the API when a proxy
	// mounts it under a prefix (e.g. "/api"). The refresh cookie is scoped to
	// PathPrefix+"/auth"; empty means the API is served at the root.
	PathPrefix string
	// AllowedOrigins are the browser origins allowed to log in, refresh or
	// log out with the cookie (CSRF protection).
	AllowedOrigins []string
}

// Handler wires the auth HTTP routes.
type Handler struct {
	svc     *Service
	cookies CookieOptions
}

// NewHandler creates an auth Handler.
func NewHandler(svc *Service, cookies CookieOptions) http.Handler {
	h := &Handler{svc: svc, cookies: cookies}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /auth/register", h.register)
	mux.HandleFunc("POST /auth/login", h.login)
	mux.HandleFunc("POST /auth/refresh", h.refresh)
	mux.HandleFunc("POST /auth/logout", h.logout)
	return mux
}

func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if err := h.svc.Register(r.Context(), req); err != nil {
		switch {
		case errors.Is(err, ErrValidation):
			writeError(w, http.StatusUnprocessableEntity, err.Error())
		case errors.Is(err, ErrDuplicateEmail):
			writeError(w, http.StatusConflict, "email already registered")
		case errors.Is(err, ErrRegistrationClosed):
			writeError(w, http.StatusForbidden, "sign-up is not open")
		default:
			internalError(w, r, err)
		}
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"message": "User registered successfully!"})
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	// Login CSRF: a cross-site form could otherwise sign the victim's browser
	// into the attacker's account. Only allowed origins get session cookies.
	if req.UseCookies && !middleware.CheckCookieOrigin(r, h.cookies.AllowedOrigins) {
		writeError(w, http.StatusForbidden, "origin not allowed")
		return
	}

	tokens, err := h.svc.Login(r.Context(), req)
	if err != nil {
		var need *CondominiumRequiredError
		switch {
		case errors.As(err, &need):
			writeJSON(w, http.StatusConflict, map[string]any{
				"error":        "condominiumId is required: the account belongs to more than one condominium",
				"condominiums": need.Condominiums,
			})
		case errors.Is(err, ErrValidation):
			writeError(w, http.StatusUnprocessableEntity, err.Error())
		case errors.Is(err, ErrInvalidCredentials):
			writeError(w, http.StatusUnauthorized, "invalid credentials")
		default:
			internalError(w, r, err)
		}
		return
	}

	h.respondTokens(w, tokens, req.UseCookies)
}

// refresh reads the refresh token from the body (app) or, when the body has
// none, from the cookie (web). The answer uses the same flow as the request.
func (h *Handler) refresh(w http.ResponseWriter, r *http.Request) {
	raw, fromCookie, ok := h.refreshToken(w, r)
	if !ok {
		return
	}
	tokens, err := h.svc.Refresh(r.Context(), raw)
	if err != nil {
		if errors.Is(err, ErrInvalidRefreshToken) {
			if fromCookie {
				h.clearCookies(w)
			}
			writeError(w, http.StatusUnauthorized, "invalid or expired refresh token")
			return
		}
		internalError(w, r, err)
		return
	}
	h.respondTokens(w, tokens, fromCookie)
}

// logout revokes the session and clears the cookies. It always answers 204.
func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	raw, _, ok := h.refreshToken(w, r)
	if !ok {
		return
	}
	if err := h.svc.Logout(r.Context(), raw); err != nil {
		internalError(w, r, err)
		return
	}
	h.clearCookies(w)
	w.WriteHeader(http.StatusNoContent)
}

// refreshToken extracts the token for refresh/logout. A cookie-borne token is
// only accepted from an allowed origin (CSRF). ok is false when an error
// response was already written.
func (h *Handler) refreshToken(w http.ResponseWriter, r *http.Request) (raw string, fromCookie, ok bool) {
	var req RefreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return "", false, false
	}
	if req.RefreshToken != "" {
		return req.RefreshToken, false, true
	}
	c, err := r.Cookie(middleware.RefreshCookie)
	if err != nil || c.Value == "" {
		return "", false, true
	}
	if !middleware.CheckCookieOrigin(r, h.cookies.AllowedOrigins) {
		writeError(w, http.StatusForbidden, "origin not allowed")
		return "", false, false
	}
	return c.Value, true, true
}

func (h *Handler) respondTokens(w http.ResponseWriter, t *Tokens, useCookies bool) {
	if !useCookies {
		writeJSON(w, http.StatusOK, t.Pair())
		return
	}
	h.setCookie(w, middleware.AccessCookie, t.AccessToken, "/", http.SameSiteLaxMode, t.AccessTTL)
	h.setCookie(w, middleware.RefreshCookie, t.RefreshToken, h.refreshPath(), http.SameSiteStrictMode, t.RefreshTTL)
	body := t.Pair()
	body.AccessToken, body.RefreshToken = "", "" // scripts on the page never see the tokens
	writeJSON(w, http.StatusOK, body)
}

func (h *Handler) clearCookies(w http.ResponseWriter) {
	h.setCookie(w, middleware.AccessCookie, "", "/", http.SameSiteLaxMode, -1)
	h.setCookie(w, middleware.RefreshCookie, "", h.refreshPath(), http.SameSiteStrictMode, -1)
}

// refreshPath is where the browser sends the refresh cookie: only the auth
// routes, as seen through the proxy prefix.
func (h *Handler) refreshPath() string { return h.cookies.PathPrefix + "/auth" }

// setCookie writes an httpOnly cookie. ttl < 0 deletes it.
func (h *Handler) setCookie(w http.ResponseWriter, name, value, path string, sameSite http.SameSite, ttl time.Duration) {
	c := &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     path,
		Domain:   h.cookies.Domain,
		HttpOnly: true,
		Secure:   h.cookies.Secure,
		SameSite: sameSite,
		MaxAge:   int(ttl.Seconds()),
	}
	if ttl < 0 {
		c.MaxAge = -1
	}
	http.SetCookie(w, c)
}

// ── helpers ───────────────────────────────────────────────────────────────────

func internalError(w http.ResponseWriter, r *http.Request, err error) {
	slog.ErrorContext(r.Context(), "auth: internal error", "error", err)
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
