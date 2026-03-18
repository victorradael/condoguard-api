package user

// User represents a CondoGuard user as returned by the API.
// The Password field is never serialised to JSON.
type User struct {
	ID       string   `json:"id"       bson:"_id,omitempty"`
	Username string   `json:"username" bson:"username"`
	Email    string   `json:"email"    bson:"email"`
	Password string   `json:"-"        bson:"password"`
	Roles    []string `json:"roles"    bson:"roles"`
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
