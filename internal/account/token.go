package account

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"strings"
)

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// NewToken returns a random 160-bit token and its hash. The token is shown as
// eight groups of four characters, so it works both as a link parameter and as
// a code someone types or pastes from a WhatsApp message. Only the hash is stored.
func NewToken() (token string, hash []byte, err error) {
	b := make([]byte, 20)
	if _, err = rand.Read(b); err != nil {
		return "", nil, err
	}
	raw := b32.EncodeToString(b) // 32 characters
	groups := make([]string, 0, 8)
	for i := 0; i < len(raw); i += 4 {
		groups = append(groups, raw[i:i+4])
	}
	token = strings.Join(groups, "-")
	return token, HashToken(token), nil
}

// HashToken canonicalises what the user sent (case, dashes and spaces do not
// matter) and returns its SHA-256.
func HashToken(token string) []byte {
	canon := strings.ToUpper(strings.NewReplacer("-", "", " ", "", "\t", "", "\n", "", "\r", "").Replace(token))
	sum := sha256.Sum256([]byte(canon))
	return sum[:]
}

// wellFormed reports whether the input has the shape of a token, so obviously
// wrong input is refused without touching the database.
func wellFormed(token string) bool {
	canon := strings.NewReplacer("-", "", " ", "").Replace(strings.ToUpper(strings.TrimSpace(token)))
	if len(canon) != 32 {
		return false
	}
	_, err := b32.DecodeString(canon)
	return err == nil
}

// ValidToken reports whether the input has the shape of a token (see wellFormed).
func ValidToken(token string) bool { return wellFormed(token) }
