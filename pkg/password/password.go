package password

import (
	"errors"

	"golang.org/x/crypto/bcrypt"
)

const cost = bcrypt.DefaultCost

// Hash returns a bcrypt hash of the plaintext password.
// Returns an error if the password is empty or hashing fails.
func Hash(plain string) (string, error) {
	if plain == "" {
		return "", errors.New("password: plaintext must not be empty")
	}
	b, err := bcrypt.GenerateFromPassword([]byte(plain), cost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// Verify reports whether plain matches the bcrypt hash.
// Returns false (never panics) on any error.
func Verify(plain, hash string) bool {
	if plain == "" || hash == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}
