package user

import "errors"

// Sentinel errors for the user domain.
var (
	ErrNotFound   = errors.New("user: not found")
	ErrDuplicate  = errors.New("user: email already registered")
	ErrValidation = errors.New("user: validation error")
)

// IsNotFoundError reports whether err is (or wraps) ErrNotFound.
func IsNotFoundError(err error) bool { return errors.Is(err, ErrNotFound) }

// IsDuplicateError reports whether err is (or wraps) ErrDuplicate.
func IsDuplicateError(err error) bool { return errors.Is(err, ErrDuplicate) }

// IsValidationError reports whether err is (or wraps) ErrValidation.
func IsValidationError(err error) bool { return errors.Is(err, ErrValidation) }
