package notification

import "errors"

var (
	ErrNotFound   = errors.New("notification: not found")
	ErrValidation = errors.New("notification: validation error")
)

func IsNotFoundError(err error) bool   { return errors.Is(err, ErrNotFound) }
func IsValidationError(err error) bool { return errors.Is(err, ErrValidation) }
