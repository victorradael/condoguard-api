package people

import "errors"

// Errors of the domain. The handler maps them to HTTP statuses.
var (
	ErrNotFound   = errors.New("people: not found")
	ErrValidation = errors.New("people: validation error")
	ErrDuplicate  = errors.New("people: already exists")
	// ErrConflict is a business-rule clash (two principal owners, a block that still has units, ...).
	ErrConflict = errors.New("people: conflict")
	// ErrInvalidToken is an invitation token that is unknown, used, revoked or expired.
	ErrInvalidToken = errors.New("people: invalid or expired invitation")
	// ErrBadCredentials is a wrong password when accepting an invitation with an existing account.
	ErrBadCredentials = errors.New("people: wrong password")
)
