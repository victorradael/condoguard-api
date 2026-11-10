package auth

// User represents a CondoGuard user stored in the database.
type User struct {
	ID            string   `json:"id"`
	CondominiumID string   `json:"-"`
	Username      string   `json:"username"`
	Email         string   `json:"email"`
	Password      string   `json:"-"` // bcrypt hash; never serialised
	Roles         []string `json:"roles"`
	EmailVerified bool     `json:"-"`
}

// Membership is the place of an account in one condominium.
type Membership struct {
	CondominiumID string   `json:"id"`
	Name          string   `json:"name"`
	Roles         []string `json:"roles"`
}

// RegisterRequest is the payload for POST /auth/register.
// Public sign-up always creates a resident (morador); there is no way to
// ask for another role here. Admins create other admins via POST /users.
type RegisterRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

// LoginRequest is the payload for POST /auth/login.
//
// UseCookies selects the web flow: the tokens go only into httpOnly cookies
// and are left out of the body, so page scripts never see them. Without it
// (the app) the tokens are returned in the body.
//
// CondominiumID picks the condominium to sign in to when the account belongs
// to more than one; it may be left out otherwise.
type LoginRequest struct {
	Email         string `json:"email"`
	Password      string `json:"password"`
	CondominiumID string `json:"condominiumId,omitempty"`
	UseCookies    bool   `json:"useCookies"`
}

// RefreshRequest is the payload for POST /auth/refresh and /auth/logout.
// RefreshToken is empty in the web flow, where the cookie carries it.
type RefreshRequest struct {
	RefreshToken string `json:"refreshToken"`
}

// TokenPair is returned by login and refresh.
// AccessToken and RefreshToken are omitted in the cookie (web) flow.
type TokenPair struct {
	AccessToken  string   `json:"accessToken,omitempty"`
	RefreshToken string   `json:"refreshToken,omitempty"`
	TokenType    string   `json:"tokenType"`
	ExpiresIn    int      `json:"expiresIn"` // access-token lifetime, seconds
	Roles        []string `json:"roles"`
}
