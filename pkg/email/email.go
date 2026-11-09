// Package email normalises and validates e-mail addresses used as login.
package email

import (
	"errors"
	"net/mail"
	"strings"
)

// ErrInvalid is returned for anything that is not a plain address.
var ErrInvalid = errors.New("email is invalid")

// maxLen is the longest address SMTP allows.
const maxLen = 254

// Normalize trims and lower-cases s and checks it is a single plain address
// (no display name, no comments). The result is the form stored and looked up.
func Normalize(s string) (string, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" || len(s) > maxLen {
		return "", ErrInvalid
	}
	a, err := mail.ParseAddress(s)
	if err != nil || a.Address != s || a.Name != "" || !strings.Contains(s[strings.LastIndexByte(s, '@')+1:], ".") {
		return "", ErrInvalid
	}
	return s, nil
}
