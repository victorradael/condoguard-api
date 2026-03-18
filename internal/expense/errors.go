package expense

import "errors"

var (
	ErrNotFound   = errors.New("expense: not found")
	ErrValidation = errors.New("expense: validation error")
)

func IsNotFoundError(err error) bool   { return errors.Is(err, ErrNotFound) }
func IsValidationError(err error) bool { return errors.Is(err, ErrValidation) }
