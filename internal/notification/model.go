package notification

import "time"

// Notification represents a message broadcast to a set of residents and/or shopowners.
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
type CreateRequest struct {
	Message      string   `json:"message"`
	CreatedByID  string   `json:"createdById"`
	ResidentIDs  []string `json:"residentIds"`
	ShopOwnerIDs []string `json:"shopOwnerIds"`
}

// UpdateRequest is the payload for PUT /notifications/{id}.
type UpdateRequest struct {
	Message      string   `json:"message"`
	ResidentIDs  []string `json:"residentIds"`
	ShopOwnerIDs []string `json:"shopOwnerIds"`
}
