package resident

// Resident represents a residential unit in a condominium.
type Resident struct {
	ID            string `json:"id"`
	UnitNumber    string `json:"unitNumber"`
	Floor         int    `json:"floor"`
	CondominiumID string `json:"condominiumId"`
	OwnerID       string `json:"ownerId"`
}

// CreateRequest is the payload for POST /residents.
type CreateRequest struct {
	UnitNumber    string `json:"unitNumber"`
	Floor         int    `json:"floor"`
	CondominiumID string `json:"condominiumId"`
	OwnerID       string `json:"ownerId"`
}

// UpdateRequest is the payload for PUT /residents/{id}.
type UpdateRequest struct {
	UnitNumber string `json:"unitNumber"`
	Floor      int    `json:"floor"`
}
