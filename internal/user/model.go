package user

// User represents a CondoGuard user as returned by the API.
// The Password field is never serialised to JSON.
type User struct {
	ID       string   `json:"id"`
	Username string   `json:"username"`
	Email    string   `json:"email"`
	Password string   `json:"-"`
	Roles    []string `json:"roles"`
	seq      int      // insertion order, in-memory repository only
}

// CreateRequest is the payload for POST /users.
type CreateRequest struct {
	Username string   `json:"username"`
	Email    string   `json:"email"`
	Password string   `json:"password"`
	Roles    []string `json:"roles"`
}

// UpdateRequest is the payload for PUT /users/{id}.
// Email is intentionally absent — it is immutable after creation.
type UpdateRequest struct {
	Username string   `json:"username"`
	Email    string   `json:"email"` // accepted but silently ignored
	Roles    []string `json:"roles"`
}
