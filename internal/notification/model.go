package notification

import "time"

// Notification represents a message broadcast to a set of residents and/or shopowners.
// Read and ReadAt are the state for the user asking: each recipient marks
// their own reading.
type Notification struct {
	ID           string     `json:"id"`
	Message      string     `json:"message"`
	CreatedByID  string     `json:"createdById"`
	CreatedAt    time.Time  `json:"createdAt"`
	Read         bool       `json:"read"`
	ReadAt       *time.Time `json:"readAt"`
	ResidentIDs  []string   `json:"residentIds"`
	ShopOwnerIDs []string   `json:"shopOwnerIds"`
}

// CreateRequest is the payload for POST /notifications.
// The author (createdById) is always the authenticated user; a value sent
// by the client is ignored.
type CreateRequest struct {
	Message      string   `json:"message"`
	ResidentIDs  []string `json:"residentIds"`
	ShopOwnerIDs []string `json:"shopOwnerIds"`
}

// UpdateRequest is the payload for PUT /notifications/{id}.
type UpdateRequest struct {
	Message      string   `json:"message"`
	ResidentIDs  []string `json:"residentIds"`
	ShopOwnerIDs []string `json:"shopOwnerIds"`
}
