package people

import "time"

// Kinds of link between a person and a unit.
const (
	KindOwner     = "proprietario"
	KindCoOwner   = "coproprietario"
	KindTenant    = "inquilino"
	KindResident  = "morador"
	KindDependent = "dependente"
)

// Kinds lists every kind of link.
var Kinds = []string{KindOwner, KindCoOwner, KindTenant, KindResident, KindDependent}

func validKind(k string) bool {
	for _, v := range Kinds {
		if v == k {
			return true
		}
	}
	return false
}

// householdKinds are the kinds the person responsible for a unit (owner,
// co-owner, tenant) may add and end without the síndico.
func householdKind(k string) bool { return k == KindResident || k == KindDependent }

// responsibleKinds are the kinds that make someone responsible for the unit.
func responsibleKind(k string) bool { return k == KindOwner || k == KindCoOwner || k == KindTenant }

// Block is a tower or block of the condominium.
type Block struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	UnitCount int64     `json:"unitCount"`
	CreatedAt time.Time `json:"createdAt"`
}

// BlockRequest is the payload of POST /blocks and PUT /blocks/{id}.
type BlockRequest struct {
	Name string `json:"name"`
}

// UnitRef says where a person is linked (summary in the people list).
type UnitRef struct {
	LinkID     string `json:"linkId"`
	UnitID     string `json:"unitId"`
	UnitNumber string `json:"unitNumber"`
	BlockName  string `json:"blockName,omitempty"`
	Kind       string `json:"kind"`
	StartsOn   string `json:"startsOn"`
	EndsOn     string `json:"endsOn,omitempty"`
	Active     bool   `json:"active"`
}

// Person is a person registered in the condominium. CPF, e-mail and phone are
// filled only as far as the caller may see them (see package doc).
type Person struct {
	ID         string    `json:"id"`
	FullName   string    `json:"fullName"`
	CPF        string    `json:"cpf,omitempty"`
	Email      string    `json:"email,omitempty"`
	Phone      string    `json:"phone,omitempty"`
	HasAccount *bool     `json:"hasAccount,omitempty"`
	Units      []UnitRef `json:"units,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
}

// PersonRequest is the payload of POST /people and PUT /people/{id}.
type PersonRequest struct {
	FullName string `json:"fullName"`
	CPF      string `json:"cpf"`
	Email    string `json:"email"`
	Phone    string `json:"phone"`
}

// Link is one line of the history of a unit.
type Link struct {
	ID         string `json:"id"`
	UnitID     string `json:"unitId"`
	PersonID   string `json:"personId"`
	PersonName string `json:"personName,omitempty"`
	Kind       string `json:"kind"`
	StartsOn   string `json:"startsOn"`
	EndsOn     string `json:"endsOn,omitempty"`
	Active     bool   `json:"active"`
}

// LinkRequest is the payload of POST /units/{id}/links. Either PersonID or
// Person (a new person, or the one with the same CPF) names who is linked.
// StartsOn defaults to today.
type LinkRequest struct {
	PersonID string         `json:"personId"`
	Person   *PersonRequest `json:"person"`
	Kind     string         `json:"kind"`
	StartsOn string         `json:"startsOn"`
	EndsOn   string         `json:"endsOn"`
}

// EndLinkRequest is the payload of PUT /links/{id}/end. EndsOn is the first day
// the person is no longer linked; it defaults to today.
type EndLinkRequest struct {
	EndsOn string `json:"endsOn"`
}

// TransferRequest is the payload of POST /units/{id}/transfers: from
// EffectiveDate the unit has a new owner (kind proprietario) or tenant (kind
// inquilino); the previous ones end on that day.
type TransferRequest struct {
	Kind          string         `json:"kind"`
	EffectiveDate string         `json:"effectiveDate"`
	PersonID      string         `json:"personId"`
	Person        *PersonRequest `json:"person"`
}

// TransferResult says what the swap did.
type TransferResult struct {
	Created Link   `json:"created"`
	Ended   []Link `json:"ended"`
}

// Invitation is the state of an invitation sent to a person.
type Invitation struct {
	ID         string     `json:"id"`
	PersonID   string     `json:"personId"`
	Email      string     `json:"email"`
	Channel    string     `json:"channel"`
	CreatedAt  time.Time  `json:"createdAt"`
	ExpiresAt  time.Time  `json:"expiresAt"`
	AcceptedAt *time.Time `json:"acceptedAt,omitempty"`
	RevokedAt  *time.Time `json:"revokedAt,omitempty"`
	Status     string     `json:"status"` // pending, accepted, revoked, expired
}

// InviteRequest is the payload of POST /people/{id}/invitations. Channel "email"
// sends the link to the person's e-mail and shows nothing to the caller;
// "link" returns the link/code once so the caller can pass it on (WhatsApp).
type InviteRequest struct {
	Channel string `json:"channel"`
}

// InviteResult is the answer to an invitation. Token and Link are present only for channel "link".
type InviteResult struct {
	Invitation Invitation `json:"invitation"`
	Token      string     `json:"token,omitempty"`
	Link       string     `json:"link,omitempty"`
}

// InvitationInfo is what the holder of a token sees before accepting.
type InvitationInfo struct {
	CondominiumName string `json:"condominiumName"`
	PersonName      string `json:"personName"`
	Email           string `json:"email"`
	// AccountExists: the e-mail already has an account, so accepting asks for its password.
	AccountExists bool `json:"accountExists"`
}

// AcceptRequest is the payload of POST /auth/invitations/accept. Username is
// asked only when the account is new.
type AcceptRequest struct {
	Token    string `json:"token"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// AcceptResult tells the client where to sign in.
type AcceptResult struct {
	CondominiumID string `json:"condominiumId"`
	Email         string `json:"email"`
	// VerifyEmail: a confirmation e-mail was sent and may be required before signing in.
	VerifyEmail bool `json:"verifyEmail"`
}

// MyUnit is a unit of the signed-in person with their role in it (GET /me/units).
type MyUnit struct {
	UnitRef
	// CanManageHousehold: the person is responsible for the unit and may register
	// the other residents and dependents.
	CanManageHousehold bool `json:"canManageHousehold"`
}

// Profile is the signed-in person (GET /me/profile).
type Profile struct {
	Person Person   `json:"person"`
	Units  []MyUnit `json:"units"`
}

// ProfileRequest is the payload of PUT /me/profile: the person edits their own
// name and phone (the e-mail is the login and the CPF is set by the síndico).
type ProfileRequest struct {
	FullName string `json:"fullName"`
	Phone    string `json:"phone"`
}
