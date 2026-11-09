package auth

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/victorradael/condoguard/api/internal/authz"
	"github.com/victorradael/condoguard/api/internal/middleware"
	"github.com/victorradael/condoguard/api/internal/tenant"
	pkgjwt "github.com/victorradael/condoguard/api/pkg/jwt"
)

// who is the user a second-factor request acts for.
type who struct {
	userID string
	// step is true when the user is still in the middle of a login (enrol step
	// token): finishing the enrolment then opens the session.
	step bool
}

// authenticated runs next for a signed-in user (access token). Without the
// authentication middleware configured, nothing is allowed through.
func (h *Handler) authenticated(next http.HandlerFunc) http.Handler {
	if h.authMW == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			writeError(w, http.StatusUnauthorized, "authentication required")
		})
	}
	return h.authMW(next)
}

// twoFactor serves setup/enable both to a user who is signed in and to one
// who just logged in and was told to enrol (step token in the body). The
// body is read once here, and handed on intact.
func (h *Handler) twoFactor(next func(w http.ResponseWriter, r *http.Request, id who, body []byte)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid body")
			return
		}
		if middleware.AccessTokenFromRequest(r) != "" {
			h.authenticated(func(w http.ResponseWriter, r *http.Request) {
				next(w, r, who{userID: middleware.UserIDFromContext(r.Context())}, body)
			}).ServeHTTP(w, r)
			return
		}
		var req struct {
			MFAToken string `json:"mfaToken"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		ctx, claims, err := h.svc.StepIdentity(r.Context(), req.MFAToken, pkgjwt.PurposeMFAEnroll)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid or expired two-factor token")
			return
		}
		next(w, r.WithContext(ctx), who{userID: claims.UserID, step: true}, body)
	}
}

func (h *Handler) setup2FA(w http.ResponseWriter, r *http.Request, id who, _ []byte) {
	res, err := h.svc.SetupTwoFactor(r.Context(), id.userID)
	if err != nil {
		h.mfaError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"secret": res.Secret, "otpauthUrl": res.OTPAuthURL})
}

func (h *Handler) enable2FA(w http.ResponseWriter, r *http.Request, id who, body []byte) {
	var req struct {
		Code       string `json:"code"`
		UseCookies bool   `json:"useCookies"`
	}
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if id.step && req.UseCookies && !middleware.CheckCookieOrigin(r, h.cookies.AllowedOrigins) {
		writeError(w, http.StatusForbidden, "origin not allowed")
		return
	}
	codes, tokens, err := h.svc.EnableTwoFactor(r.Context(), id.userID, req.Code, id.step)
	if err != nil {
		h.mfaError(w, r, err)
		return
	}
	if tokens == nil {
		writeJSON(w, http.StatusOK, map[string]any{"recoveryCodes": codes})
		return
	}
	h.respondTokensWith(w, tokens, req.UseCookies, map[string]any{"recoveryCodes": codes})
}

func (h *Handler) verify2FA(w http.ResponseWriter, r *http.Request) {
	var req VerifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.UseCookies && !middleware.CheckCookieOrigin(r, h.cookies.AllowedOrigins) {
		writeError(w, http.StatusForbidden, "origin not allowed")
		return
	}
	tokens, err := h.svc.VerifySecondFactor(r.Context(), req)
	if err != nil {
		h.mfaError(w, r, err)
		return
	}
	h.respondTokens(w, tokens, req.UseCookies)
}

func (h *Handler) disable2FA(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password     string `json:"password"`
		Code         string `json:"code"`
		RecoveryCode string `json:"recoveryCode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	code, recovery := req.Code, false
	if req.RecoveryCode != "" {
		code, recovery = req.RecoveryCode, true
	}
	err := h.svc.DisableTwoFactor(r.Context(), middleware.UserIDFromContext(r.Context()), req.Password, code, recovery)
	if err != nil {
		h.mfaError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) recoveryCodes(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	codes, err := h.svc.RegenerateRecoveryCodes(r.Context(), middleware.UserIDFromContext(r.Context()), req.Code)
	if err != nil {
		h.mfaError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"recoveryCodes": codes})
}

// revokeSessions signs the caller out everywhere.
func (h *Handler) revokeSessions(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.RevokeAllSessions(r.Context(), middleware.UserIDFromContext(r.Context())); err != nil {
		internalError(w, r, err)
		return
	}
	h.clearCookies(w)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) mfaError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrValidation):
		writeError(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, ErrInvalidMFAToken):
		writeError(w, http.StatusUnauthorized, "invalid or expired two-factor token")
	case errors.Is(err, ErrInvalidCode):
		writeError(w, http.StatusUnauthorized, "invalid code")
	case errors.Is(err, ErrInvalidCredentials):
		writeError(w, http.StatusUnauthorized, "invalid credentials")
	case errors.Is(err, ErrTooManyAttempts):
		w.Header().Set("Retry-After", "900")
		writeError(w, http.StatusTooManyRequests, "too many attempts, try again later")
	case errors.Is(err, ErrMFAAlreadyEnabled):
		writeError(w, http.StatusConflict, "two-factor authentication is already enabled")
	case errors.Is(err, ErrMFANotEnabled):
		writeError(w, http.StatusConflict, "two-factor authentication is not set up")
	case errors.Is(err, ErrMFAMandatory):
		writeError(w, http.StatusForbidden, "two-factor authentication is mandatory for your role")
	default:
		if status, msg, ok := authz.Status(err); ok {
			writeError(w, status, msg)
			return
		}
		if errors.Is(err, tenant.ErrMissing) {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		internalError(w, r, err)
	}
}
