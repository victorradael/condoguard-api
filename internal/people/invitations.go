package people

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/victorradael/condoguard/api/internal/account"
	"github.com/victorradael/condoguard/api/internal/audit"
	"github.com/victorradael/condoguard/api/internal/authz"
	"github.com/victorradael/condoguard/api/internal/db"
	"github.com/victorradael/condoguard/api/internal/db/dbgen"
	"github.com/victorradael/condoguard/api/internal/middleware"
	"github.com/victorradael/condoguard/api/internal/tenant"
	"github.com/victorradael/condoguard/api/pkg/mailer"
	"github.com/victorradael/condoguard/api/pkg/password"
)

// InviteTTL is how long an invitation works. It travels by e-mail or chat and may wait a while.
const InviteTTL = 7 * 24 * time.Hour

// Invitation channels.
const (
	ChannelEmail = "email"
	ChannelLink  = "link"
)

// Verifier sends the e-mail confirmation to an account created from a "link"
// invitation (account.Service does it).
type Verifier interface {
	SendVerification(ctx context.Context, userID, email, username string)
}

type invitations struct {
	mail     mailer.Sender
	baseURL  string
	verifier Verifier
}

// WithInvitations enables invitations: mail sends them, baseURL is where the web
// app lives (the link points to /convite) and verifier confirms the e-mail of
// accounts whose invitation did not prove it.
func (s *Service) WithInvitations(mail mailer.Sender, baseURL string, verifier Verifier) *Service {
	s.inv = &invitations{mail: mail, baseURL: strings.TrimRight(baseURL, "/"), verifier: verifier}
	return s
}

func (s *Service) invitationLink(token string) string {
	return s.inv.baseURL + "/convite?token=" + url.QueryEscape(token)
}

func invitationStatus(r dbgen.ListInvitationsByPersonRow) string {
	switch {
	case r.AcceptedAt != nil:
		return "accepted"
	case r.RevokedAt != nil:
		return "revoked"
	case !r.ExpiresAt.After(time.Now()):
		return "expired"
	}
	return "pending"
}

func invitationDTO(r dbgen.ListInvitationsByPersonRow) Invitation {
	return Invitation{ID: r.ID.String(), PersonID: r.PersonID.String(), Email: r.Email, Channel: r.Channel, CreatedAt: r.CreatedAt,
		ExpiresAt: r.ExpiresAt, AcceptedAt: r.AcceptedAt, RevokedAt: r.RevokedAt, Status: invitationStatus(r)}
}

// Invite makes an invitation for a person who has an e-mail and no account yet,
// replacing the pending ones. Channel "email" sends the link to the person and
// shows nothing to the caller; "link" returns the link and the code once, to be
// passed on by the caller (WhatsApp). Needs people.write.
func (s *Service) Invite(ctx context.Context, personID string, req InviteRequest) (*InviteResult, error) {
	p, err := authz.Require(ctx, authz.PermPeopleWrite)
	if err != nil {
		return nil, err
	}
	if s.inv == nil {
		return nil, fmt.Errorf("people: invitations are not configured")
	}
	if req.Channel == "" {
		req.Channel = ChannelEmail
	}
	if req.Channel != ChannelEmail && req.Channel != ChannelLink {
		return nil, validationf("channel must be %s or %s", ChannelEmail, ChannelLink)
	}
	condo, err := s.condo(ctx)
	if err != nil {
		return nil, err
	}
	pid, ok := parseID(personID)
	if !ok {
		return nil, ErrNotFound
	}
	token, hash, err := account.NewToken()
	if err != nil {
		return nil, err
	}
	id := uuid.New()
	expires := time.Now().Add(InviteTTL)
	var person dbgen.Person
	err = s.trail.Atomic(ctx, func(ctx context.Context) (audit.Entry, error) {
		var gerr error
		if person, gerr = s.q.GetPerson(ctx, dbgen.GetPersonParams{CondominiumID: condo, ID: pid}); db.IsNoRows(gerr) {
			return audit.Entry{}, ErrNotFound
		} else if gerr != nil {
			return audit.Entry{}, gerr
		}
		if person.UserID != nil {
			return audit.Entry{}, conflictf("the person already has an account")
		}
		if person.Email == nil {
			return audit.Entry{}, validationf("the person has no e-mail; add one before inviting")
		}
		if rerr := s.q.RevokePendingInvitations(ctx, dbgen.RevokePendingInvitationsParams{CondominiumID: condo, PersonID: pid}); rerr != nil {
			return audit.Entry{}, rerr
		}
		var by *uuid.UUID
		if u, uok := parseID(p.UserID); uok {
			by = &u
		}
		if ierr := s.q.InsertInvitation(ctx, dbgen.InsertInvitationParams{ID: id, CondominiumID: condo, PersonID: pid, Email: *person.Email,
			TokenHash: hash, Channel: req.Channel, CreatedBy: by, ExpiresAt: expires}); ierr != nil {
			return audit.Entry{}, ierr
		}
		// The token is not in the trail: only that an invitation was issued, to whom and how.
		return audit.Entry{Action: audit.ActionCreate, Resource: resInvitation, ResourceID: id.String(),
			After: map[string]any{"personId": personID, "email": *person.Email, "channel": req.Channel, "expiresAt": expires}}, nil
	})
	if err != nil {
		return nil, err
	}
	res := &InviteResult{Invitation: Invitation{ID: id.String(), PersonID: personID, Email: *person.Email, Channel: req.Channel,
		CreatedAt: time.Now().UTC(), ExpiresAt: expires, Status: "pending"}}
	if req.Channel == ChannelLink {
		res.Token, res.Link = token, s.invitationLink(token)
		return res, nil
	}
	if err = s.sendInvitation(ctx, *person.Email, person.FullName, token); err != nil {
		slog.Error("people: invitation e-mail not sent", "error", err)
		return nil, fmt.Errorf("%w: the e-mail could not be sent; try again or use a link", ErrMail)
	}
	return res, nil
}

// ErrMail is returned when the invitation e-mail could not be delivered.
var ErrMail = fmt.Errorf("people: mail failure")

func (s *Service) sendInvitation(ctx context.Context, to, name, token string) error {
	cond := "o seu condomínio"
	if id, err := tenant.CondominiumID(ctx); err == nil {
		if u, ok := parseID(id); ok {
			if c, cerr := s.q.GetCondominium(ctx, u); cerr == nil {
				cond = c.Name
			}
		}
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	defer cancel()
	return s.inv.mail.Send(ctx, mailer.Message{To: to, Subject: "Convite para o Condoguard — " + cond, Body: fmt.Sprintf(
		"Olá, %s.\n\nVocê foi convidado(a) para acessar o Condoguard de %s.\n\n"+
			"Abra o link abaixo para ativar a sua conta (vale por 7 dias e só pode ser usado uma vez):\n%s\n\n"+
			"No aplicativo, informe este código: %s\n\n"+
			"Se você não esperava este convite, ignore este e-mail.\n", name, cond, s.invitationLink(token), token)})
}

// ListInvitations returns the latest invitations of a person. Needs people.write.
func (s *Service) ListInvitations(ctx context.Context, personID string) ([]Invitation, error) {
	if _, err := authz.Require(ctx, authz.PermPeopleWrite); err != nil {
		return nil, err
	}
	condo, err := s.condo(ctx)
	if err != nil {
		return nil, err
	}
	pid, ok := parseID(personID)
	if !ok {
		return nil, ErrNotFound
	}
	if _, err = s.q.GetPerson(ctx, dbgen.GetPersonParams{CondominiumID: condo, ID: pid}); db.IsNoRows(err) {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, err
	}
	rows, err := s.q.ListInvitationsByPerson(ctx, dbgen.ListInvitationsByPersonParams{CondominiumID: condo, PersonID: pid})
	if err != nil {
		return nil, err
	}
	out := make([]Invitation, len(rows))
	for i, r := range rows {
		out[i] = invitationDTO(r)
	}
	return out, nil
}

type peeked struct {
	ID, CondoID, PersonID                 uuid.UUID
	CondoName, PersonName, Email, Channel string
}

func (s *Service) peek(ctx context.Context, token string) (peeked, error) {
	var k peeked
	if !account.ValidToken(token) {
		return k, ErrInvalidToken
	}
	err := db.NewScoped(s.pool).QueryRow(ctx,
		`SELECT id, condominium_id, condominium_name, person_id, person_name, email, channel FROM invitation_peek($1)`,
		account.HashToken(token)).Scan(&k.ID, &k.CondoID, &k.CondoName, &k.PersonID, &k.PersonName, &k.Email, &k.Channel)
	if db.IsNoRows(err) {
		return k, ErrInvalidToken
	}
	return k, err
}

// InspectInvitation says what an invitation is for, to the holder of its token.
func (s *Service) InspectInvitation(ctx context.Context, token string) (*InvitationInfo, error) {
	k, err := s.peek(ctx, token)
	if err != nil {
		return nil, err
	}
	_, err = s.q.GetUserCredentialsByEmail(ctx, k.Email)
	if err != nil && !db.IsNoRows(err) {
		return nil, err
	}
	return &InvitationInfo{CondominiumName: k.CondoName, PersonName: k.PersonName, Email: k.Email, AccountExists: err == nil}, nil
}

// AcceptInvitation spends the invitation and gives the person an account in the
// condominium with the morador role. A new e-mail creates the account (username
// and a password that meets the policy); an e-mail that already has an account
// must prove it with its password. An invitation sent by e-mail proves the
// e-mail; one passed on as a link does not, so a confirmation e-mail goes out.
func (s *Service) AcceptInvitation(ctx context.Context, req AcceptRequest) (*AcceptResult, error) {
	k, err := s.peek(ctx, req.Token)
	if err != nil {
		return nil, err
	}
	local := k.Email
	if i := strings.IndexByte(local, '@'); i >= 0 {
		local = local[:i]
	}
	ctx = tenant.WithCondominiumID(ctx, k.CondoID.String())

	existing, err := s.q.GetUserCredentialsByEmail(ctx, k.Email)
	if err != nil && !db.IsNoRows(err) {
		return nil, err
	}
	isNew := db.IsNoRows(err)

	var userID uuid.UUID
	var hashed, username string
	if isNew {
		username = strings.TrimSpace(req.Username)
		if username == "" {
			username = k.PersonName
		}
		if len([]rune(username)) > maxName {
			return nil, validationf("username is too long")
		}
		if verr := password.Validate(req.Password, local, username); verr != nil {
			return nil, fmt.Errorf("%w: %v", ErrValidation, verr)
		}
		if hashed, err = password.Hash(req.Password); err != nil {
			return nil, err
		}
		userID = uuid.New()
	} else {
		if !password.Verify(req.Password, existing.PasswordHash) {
			return nil, ErrBadCredentials
		}
		userID, username = existing.ID, existing.Username
	}

	// The new account is the actor of its own activation in the trail.
	ctx = middleware.WithIdentity(ctx, userID.String(), []string{authz.RoleMorador})
	err = s.trail.Atomic(ctx, func(ctx context.Context) (audit.Entry, error) {
		n, aerr := s.q.AcceptInvitation(ctx, dbgen.AcceptInvitationParams{CondominiumID: k.CondoID, ID: k.ID})
		if aerr != nil {
			return audit.Entry{}, aerr
		}
		if n == 0 {
			return audit.Entry{}, ErrInvalidToken // somebody else got there first
		}
		if isNew {
			if ierr := s.q.InsertUser(ctx, dbgen.InsertUserParams{ID: userID, Username: username, Email: k.Email, PasswordHash: hashed}); ierr != nil {
				return audit.Entry{}, ierr
			}
		}
		has, merr := s.q.MembershipExists(ctx, dbgen.MembershipExistsParams{UserID: userID, CondominiumID: k.CondoID})
		if merr != nil {
			return audit.Entry{}, merr
		}
		if !has {
			if ierr := s.q.InsertMembership(ctx, dbgen.InsertMembershipParams{UserID: userID, CondominiumID: k.CondoID, Roles: []string{authz.RoleMorador}}); ierr != nil {
				return audit.Entry{}, ierr
			}
		}
		if isNew && k.Channel == ChannelEmail {
			if verr := s.q.MarkEmailVerified(ctx, userID); verr != nil {
				return audit.Entry{}, verr
			}
		}
		linked, lerr := s.q.SetPersonUser(ctx, dbgen.SetPersonUserParams{CondominiumID: k.CondoID, ID: k.PersonID, UserID: &userID})
		if lerr != nil {
			return audit.Entry{}, lerr
		}
		if linked == 0 {
			return audit.Entry{}, conflictf("the person already has an account")
		}
		return audit.Entry{Action: audit.ActionUpdate, Resource: resInvitation, ResourceID: k.ID.String(),
			After: map[string]any{"accepted": true, "userId": userID.String(), "newAccount": isNew}}, nil
	})
	if db.IsUniqueViolation(err) {
		return nil, conflictf("this person or account is already linked")
	}
	if err != nil {
		return nil, err
	}
	res := &AcceptResult{CondominiumID: k.CondoID.String(), Email: k.Email}
	if isNew && k.Channel != ChannelEmail && s.inv != nil && s.inv.verifier != nil {
		s.inv.verifier.SendVerification(ctx, userID.String(), k.Email, username)
		res.VerifyEmail = true
	}
	return res, nil
}
