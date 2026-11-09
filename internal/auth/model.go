package auth

// User represents a CondoGuard user stored in the database.
type User struct {
	ID       string   `json:"id"`
	Username string   `json:"username"`
	Email    string   `json:"email"`
	Password string   `json:"-"` // bcrypt hash; never serialised
	Roles    []string `json:"roles"`
}

// RegisterRequest is the payload for POST /auth/register.
type RegisterRequest struct {
	Username string   `json:"username"`
	Email    string   `json:"email"`
	Password string   `json:"password"`
	Roles    []string `json:"roles"`
}

// LoginRequest is the payload for POST /auth/login.
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginResponse is the body returned on a successful login.
type LoginResponse struct {
	Token string   `json:"token"`
	Roles []string `json:"roles"`
}
