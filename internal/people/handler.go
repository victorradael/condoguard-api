package people

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/victorradael/condoguard/api/internal/authz"
	"github.com/victorradael/condoguard/api/pkg/page"
)

// Routes are the paths NewHandler serves, for the router to mount them. The
// unit routes sit under /units/{id}/..., next to the unit CRUD of package resident.
var Routes = []string{
	"/blocks", "/blocks/{id}",
	"/people", "/people/{id}", "/people/{id}/invitations",
	"/units/{id}/links", "/units/{id}/transfers", "/links/{id}", "/links/{id}/end",
	"/imports", "/imports/preview",
	"/me/profile", "/me/units",
}

// NewHandler serves the authenticated routes behind authMW:
//
//	GET|POST /blocks   GET|PUT|DELETE /blocks/{id}
//	GET|POST /people   GET|PUT|DELETE /people/{id}
//	GET|POST /people/{id}/invitations
//	GET|POST /units/{id}/links   (GET: history; ?active=true for who is linked now)
//	POST /units/{id}/transfers   PUT /links/{id}/end   DELETE /links/{id} (not started yet)
//	POST /imports/preview  POST /imports   (text/csv body)
//	GET|PUT /me/profile   GET /me/units
func NewHandler(svc *Service, authMW func(http.Handler) http.Handler) http.Handler {
	h := &handler{svc: svc}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /blocks", h.listBlocks)
	mux.HandleFunc("POST /blocks", h.createBlock)
	mux.HandleFunc("GET /blocks/{id}", h.getBlock)
	mux.HandleFunc("PUT /blocks/{id}", h.updateBlock)
	mux.HandleFunc("DELETE /blocks/{id}", h.deleteBlock)
	mux.HandleFunc("GET /people", h.listPeople)
	mux.HandleFunc("POST /people", h.createPerson)
	mux.HandleFunc("GET /people/{id}", h.getPerson)
	mux.HandleFunc("PUT /people/{id}", h.updatePerson)
	mux.HandleFunc("DELETE /people/{id}", h.deletePerson)
	mux.HandleFunc("GET /people/{id}/invitations", h.listInvitations)
	mux.HandleFunc("POST /people/{id}/invitations", h.invite)
	mux.HandleFunc("GET /units/{id}/links", h.history)
	mux.HandleFunc("POST /units/{id}/links", h.addLink)
	mux.HandleFunc("POST /units/{id}/transfers", h.transfer)
	mux.HandleFunc("PUT /links/{id}/end", h.endLink)
	mux.HandleFunc("DELETE /links/{id}", h.deleteLink)
	mux.HandleFunc("POST /imports/preview", h.previewImport)
	mux.HandleFunc("POST /imports", h.commitImport)
	mux.HandleFunc("GET /me/profile", h.profile)
	mux.HandleFunc("PUT /me/profile", h.updateProfile)
	mux.HandleFunc("GET /me/units", h.myUnits)
	return authMW(noStore(mux))
}

// NewInspectHandler serves POST /auth/invitations/inspect {token}: who invited
// whom, for the holder of the token. No session: the token is the credential.
func NewInspectHandler(svc *Service) http.Handler {
	h := &handler{svc: svc}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /auth/invitations/inspect", h.inspect)
	return noStore(mux)
}

// NewAcceptHandler serves POST /auth/invitations/accept {token, username?,
// password} → 201. It takes a password (of an existing account), so the router
// puts a tighter per-IP limit in front than on inspect.
func NewAcceptHandler(svc *Service) http.Handler {
	h := &handler{svc: svc}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /auth/invitations/accept", h.accept)
	return noStore(mux)
}

type handler struct{ svc *Service }

func noStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store") // personal data and one-time tokens
		next.ServeHTTP(w, r)
	})
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return false
	}
	return true
}

func pageOf(w http.ResponseWriter, r *http.Request) (page.Params, bool) {
	p, err := page.FromRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return p, false
	}
	return p, true
}

// ── blocks ────────────────────────────────────────────────────────────────────

func (h *handler) listBlocks(w http.ResponseWriter, r *http.Request) {
	pg, ok := pageOf(w, r)
	if !ok {
		return
	}
	res, err := h.svc.ListBlocks(r.Context(), pg)
	reply(w, r, http.StatusOK, res, err)
}

func (h *handler) getBlock(w http.ResponseWriter, r *http.Request) {
	b, err := h.svc.GetBlock(r.Context(), r.PathValue("id"))
	reply(w, r, http.StatusOK, b, err)
}

func (h *handler) createBlock(w http.ResponseWriter, r *http.Request) {
	var req BlockRequest
	if !decode(w, r, &req) {
		return
	}
	b, err := h.svc.CreateBlock(r.Context(), req)
	reply(w, r, http.StatusCreated, b, err)
}

func (h *handler) updateBlock(w http.ResponseWriter, r *http.Request) {
	var req BlockRequest
	if !decode(w, r, &req) {
		return
	}
	b, err := h.svc.UpdateBlock(r.Context(), r.PathValue("id"), req)
	reply(w, r, http.StatusOK, b, err)
}

func (h *handler) deleteBlock(w http.ResponseWriter, r *http.Request) {
	replyEmpty(w, r, h.svc.DeleteBlock(r.Context(), r.PathValue("id")))
}

// ── people ────────────────────────────────────────────────────────────────────

func (h *handler) listPeople(w http.ResponseWriter, r *http.Request) {
	pg, ok := pageOf(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	res, err := h.svc.ListPeople(r.Context(), ListPeopleFilter{Query: q.Get("q"), UnitID: q.Get("unitId"), ActiveOnly: q.Get("active") == "true"}, pg)
	reply(w, r, http.StatusOK, res, err)
}

func (h *handler) getPerson(w http.ResponseWriter, r *http.Request) {
	p, err := h.svc.GetPerson(r.Context(), r.PathValue("id"))
	reply(w, r, http.StatusOK, p, err)
}

func (h *handler) createPerson(w http.ResponseWriter, r *http.Request) {
	var req PersonRequest
	if !decode(w, r, &req) {
		return
	}
	p, err := h.svc.CreatePerson(r.Context(), req)
	reply(w, r, http.StatusCreated, p, err)
}

func (h *handler) updatePerson(w http.ResponseWriter, r *http.Request) {
	var req PersonRequest
	if !decode(w, r, &req) {
		return
	}
	p, err := h.svc.UpdatePerson(r.Context(), r.PathValue("id"), req)
	reply(w, r, http.StatusOK, p, err)
}

func (h *handler) deletePerson(w http.ResponseWriter, r *http.Request) {
	replyEmpty(w, r, h.svc.DeletePerson(r.Context(), r.PathValue("id")))
}

// ── links ─────────────────────────────────────────────────────────────────────

func (h *handler) history(w http.ResponseWriter, r *http.Request) {
	links, err := h.svc.History(r.Context(), r.PathValue("id"), r.URL.Query().Get("active") == "true")
	if links == nil {
		links = []Link{}
	}
	reply(w, r, http.StatusOK, map[string]any{"items": links}, err)
}

func (h *handler) addLink(w http.ResponseWriter, r *http.Request) {
	var req LinkRequest
	if !decode(w, r, &req) {
		return
	}
	l, err := h.svc.AddLink(r.Context(), r.PathValue("id"), req)
	reply(w, r, http.StatusCreated, l, err)
}

func (h *handler) endLink(w http.ResponseWriter, r *http.Request) {
	var req EndLinkRequest
	if r.ContentLength != 0 && !decode(w, r, &req) {
		return
	}
	l, err := h.svc.EndLink(r.Context(), r.PathValue("id"), req)
	reply(w, r, http.StatusOK, l, err)
}

func (h *handler) deleteLink(w http.ResponseWriter, r *http.Request) {
	replyEmpty(w, r, h.svc.DeleteLink(r.Context(), r.PathValue("id")))
}

func (h *handler) transfer(w http.ResponseWriter, r *http.Request) {
	var req TransferRequest
	if !decode(w, r, &req) {
		return
	}
	res, err := h.svc.Transfer(r.Context(), r.PathValue("id"), req)
	reply(w, r, http.StatusCreated, res, err)
}

// ── invitations ───────────────────────────────────────────────────────────────

func (h *handler) invite(w http.ResponseWriter, r *http.Request) {
	var req InviteRequest
	if r.ContentLength != 0 && !decode(w, r, &req) {
		return
	}
	res, err := h.svc.Invite(r.Context(), r.PathValue("id"), req)
	reply(w, r, http.StatusCreated, res, err)
}

func (h *handler) listInvitations(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.ListInvitations(r.Context(), r.PathValue("id"))
	if list == nil {
		list = []Invitation{}
	}
	reply(w, r, http.StatusOK, map[string]any{"items": list}, err)
}

func (h *handler) inspect(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token string `json:"token"`
	}
	if !decode(w, r, &req) {
		return
	}
	info, err := h.svc.InspectInvitation(r.Context(), req.Token)
	reply(w, r, http.StatusOK, info, err)
}

func (h *handler) accept(w http.ResponseWriter, r *http.Request) {
	var req AcceptRequest
	if !decode(w, r, &req) {
		return
	}
	res, err := h.svc.AcceptInvitation(r.Context(), req)
	reply(w, r, http.StatusCreated, res, err)
}

// ── import ────────────────────────────────────────────────────────────────────

func readCSV(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	data, err := io.ReadAll(r.Body) // the body is already capped by the MaxBody middleware
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "file too large")
		return nil, false
	}
	return data, true
}

func (h *handler) previewImport(w http.ResponseWriter, r *http.Request) {
	data, ok := readCSV(w, r)
	if !ok {
		return
	}
	rep, err := h.svc.PreviewImport(r.Context(), data)
	reply(w, r, http.StatusOK, rep, err)
}

func (h *handler) commitImport(w http.ResponseWriter, r *http.Request) {
	data, ok := readCSV(w, r)
	if !ok {
		return
	}
	rep, err := h.svc.CommitImport(r.Context(), data)
	if errors.Is(err, ErrImportInvalid) {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "the file has errors; nothing was imported", "report": rep})
		return
	}
	reply(w, r, http.StatusCreated, rep, err)
}

// ── me ────────────────────────────────────────────────────────────────────────

func (h *handler) profile(w http.ResponseWriter, r *http.Request) {
	p, err := h.svc.Profile(r.Context())
	reply(w, r, http.StatusOK, p, err)
}

func (h *handler) updateProfile(w http.ResponseWriter, r *http.Request) {
	var req ProfileRequest
	if !decode(w, r, &req) {
		return
	}
	p, err := h.svc.UpdateProfile(r.Context(), req)
	reply(w, r, http.StatusOK, p, err)
}

func (h *handler) myUnits(w http.ResponseWriter, r *http.Request) {
	p, err := h.svc.Profile(r.Context())
	if err != nil {
		reply(w, r, http.StatusOK, nil, err)
		return
	}
	reply(w, r, http.StatusOK, map[string]any{"items": p.Units}, nil)
}

// ── replies ───────────────────────────────────────────────────────────────────

func reply(w http.ResponseWriter, r *http.Request, status int, v any, err error) {
	if err != nil {
		writeFailure(w, r, err)
		return
	}
	writeJSON(w, status, v)
}

func replyEmpty(w http.ResponseWriter, r *http.Request, err error) {
	if err != nil {
		writeFailure(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeFailure(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		writeError(w, http.StatusNotFound, "not found")
	case errors.Is(err, ErrValidation):
		writeError(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, ErrDuplicate), errors.Is(err, ErrConflict):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrInvalidToken):
		writeError(w, http.StatusBadRequest, "invalid or expired invitation")
	case errors.Is(err, ErrBadCredentials):
		writeError(w, http.StatusUnauthorized, "wrong password for this account")
	case errors.Is(err, ErrMail):
		writeError(w, http.StatusBadGateway, err.Error())
	default:
		if status, msg, ok := authz.Status(err); ok {
			writeError(w, status, msg)
			return
		}
		slog.ErrorContext(r.Context(), "people: request failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
