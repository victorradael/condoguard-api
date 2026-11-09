package shopowner

// ShopOwner represents a commercial unit in a condominium.
type ShopOwner struct {
	ID       string `json:"id"`
	ShopName string `json:"shopName"`
	CNPJ     string `json:"cnpj"`
	Floor    int    `json:"floor"`
	OwnerID  string `json:"ownerId"`
}

// CreateRequest is the payload for POST /shopOwners.
type CreateRequest struct {
	ShopName string `json:"shopName"`
	CNPJ     string `json:"cnpj"`
	Floor    int    `json:"floor"`
	OwnerID  string `json:"ownerId"`
}

// UpdateRequest is the payload for PUT /shopOwners/{id}.
// CNPJ is intentionally absent — it is immutable after creation.
type UpdateRequest struct {
	ShopName string `json:"shopName"`
	Floor    int    `json:"floor"`
}
