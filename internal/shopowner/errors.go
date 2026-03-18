package shopowner

import "errors"

var (
	ErrNotFound   = errors.New("shopowner: not found")
	ErrDuplicate  = errors.New("shopowner: CNPJ already registered")
	ErrValidation = errors.New("shopowner: validation error")
)

func IsNotFoundError(err error) bool   { return errors.Is(err, ErrNotFound) }
func IsDuplicateError(err error) bool  { return errors.Is(err, ErrDuplicate) }
func IsValidationError(err error) bool { return errors.Is(err, ErrValidation) }
