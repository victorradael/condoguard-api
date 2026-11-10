package resident

// Resident represents a residential unit in a condominium.
//
// The unit sits in a block (BlockID, optional); the number is unique inside the
// block. CondominiumID is the free-text field of the first API versions and is
// kept for compatibility (default: the id of the condominium). OwnerID is the
// legacy "account responsible"; the owners and residents are now the unit links
// (see package people).
type Resident struct {
	ID            string `json:"id"`
	UnitNumber    string `json:"unitNumber"`
	Floor         int    `json:"floor"`
	CondominiumID string `json:"condominiumId"`
	OwnerID       string `json:"ownerId"`
	BlockID       string `json:"blockId,omitempty"`
	BlockName     string `json:"blockName,omitempty"`
}

// CreateRequest is the payload for POST /residents (and /units).
type CreateRequest struct {
	UnitNumber    string `json:"unitNumber"`
	Floor         int    `json:"floor"`
	CondominiumID string `json:"condominiumId"`
	OwnerID       string `json:"ownerId"`
	BlockID       string `json:"blockId"`
}

// UpdateRequest is the payload for PUT /residents/{id}. BlockID nil leaves the
// block as it is; "" takes the unit out of its block.
type UpdateRequest struct {
	UnitNumber string  `json:"unitNumber"`
	Floor      int     `json:"floor"`
	BlockID    *string `json:"blockId"`
}

// Filter narrows a list of units. Viewer "" lists every unit; otherwise only
// the units the user may see (legacy owner or active link).
type Filter struct {
	Viewer  string
	BlockID string
	// ByNumber orders by block and unit number instead of creation.
	ByNumber bool
}
