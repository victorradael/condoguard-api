package expense

import (
	"time"

	"github.com/victorradael/condoguard/api/internal/authz"
)

// Expense represents a financial charge linked to a residential or commercial unit.
// AmountCents is stored as int64 (centavos) to avoid floating-point imprecision.
type Expense struct {
	ID          string    `json:"id"`
	Description string    `json:"description"`
	AmountCents int64     `json:"amountCents"`
	DueDate     time.Time `json:"dueDate"`
	ResidentID  string    `json:"residentId"`
	ShopOwnerID string    `json:"shopOwnerId"`
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

// Filter holds optional date-range constraints for list queries and the
// caller's visibility scope.
type Filter struct {
	From  *time.Time
	To    *time.Time
	Scope authz.Scope
}
