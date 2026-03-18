package expense

import "time"

// Expense represents a financial charge linked to a residential or commercial unit.
// AmountCents is stored as int64 (centavos) to avoid floating-point imprecision.
type Expense struct {
	ID          string    `json:"id"          bson:"_id,omitempty"`
	Description string    `json:"description" bson:"description"`
	AmountCents int64     `json:"amountCents" bson:"amountCents"`
	DueDate     time.Time `json:"dueDate"     bson:"dueDate"`
	ResidentID  string    `json:"residentId"  bson:"residentId,omitempty"`
	ShopOwnerID string    `json:"shopOwnerId" bson:"shopOwnerId,omitempty"`
}

// CreateRequest is the payload for POST /expenses.
type CreateRequest struct {
	Description string    `json:"description"`
	AmountCents int64     `json:"amountCents"`
	DueDate     time.Time `json:"dueDate"`
	ResidentID  string    `json:"residentId"`
	ShopOwnerID string    `json:"shopOwnerId"`
}

// UpdateRequest is the payload for PUT /expenses/{id}.
type UpdateRequest struct {
	Description string    `json:"description"`
	AmountCents int64     `json:"amountCents"`
	DueDate     time.Time `json:"dueDate"`
}

// Filter holds optional date-range constraints for list queries.
type Filter struct {
	From *time.Time
	To   *time.Time
}
