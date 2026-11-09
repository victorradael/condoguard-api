package password

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

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

// Password policy for new and changed passwords.
const (
	// MinLength is the minimum number of characters.
	MinLength = 10
	// MaxBytes is bcrypt's input limit; longer passwords would be silently truncated.
	MaxBytes = 72
)

// ErrWeak is wrapped by every policy violation returned by Validate.
var ErrWeak = errors.New("password does not meet the policy")

// common holds passwords that pass the length and character rules but are
// among the first an attacker tries.
var common = map[string]bool{
	"1234567890a": true, "a1234567890": true, "password123": true, "password1234": true,
	"qwerty12345": true, "qwertyuiop1": true, "senha12345": true, "senha123456": true,
	"condoguard1": true, "condoguard123": true, "abc1234567": true, "abcd123456": true,
	"admin12345": true, "admin123456": true, "iloveyou123": true, "welcome123": true,
}

// Validate checks plain against the password policy: at least MinLength
// characters, at most MaxBytes bytes, at least one letter and one digit, not
// a well-known password and not containing any of personal (e.g. the e-mail's
// local part or the username, when 4+ characters long).
func Validate(plain string, personal ...string) error {
	if utf8.RuneCountInString(plain) < MinLength {
		return fmt.Errorf("%w: must have at least %d characters", ErrWeak, MinLength)
	}
	if len(plain) > MaxBytes {
		return fmt.Errorf("%w: must have at most %d bytes", ErrWeak, MaxBytes)
	}
	var letter, digit bool
	for _, r := range plain {
		letter = letter || unicode.IsLetter(r)
		digit = digit || unicode.IsDigit(r)
	}
	if !letter || !digit {
		return fmt.Errorf("%w: must contain letters and digits", ErrWeak)
	}
	lower := strings.ToLower(plain)
	if common[lower] {
		return fmt.Errorf("%w: too common", ErrWeak)
	}
	for _, p := range personal {
		p = strings.ToLower(strings.TrimSpace(p))
		if len(p) >= 4 && strings.Contains(lower, p) {
			return fmt.Errorf("%w: must not contain your e-mail or username", ErrWeak)
		}
	}
	return nil
}
