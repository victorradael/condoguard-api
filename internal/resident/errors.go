package resident

import "errors"

var (
	ErrNotFound   = errors.New("resident: not found")
	ErrDuplicate  = errors.New("resident: unit number already exists in this condominium")
	ErrValidation = errors.New("resident: validation error")
)

func IsNotFoundError(err error) bool   { return errors.Is(err, ErrNotFound) }
func IsDuplicateError(err error) bool  { return errors.Is(err, ErrDuplicate) }
func IsValidationError(err error) bool { return errors.Is(err, ErrValidation) }
