// Package secretbox encrypts small secrets at rest (the TOTP seed) with
// AES-256-GCM. The key comes from the environment, never from the code.
package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
)

// KeyBytes is the size of the key.
const KeyBytes = 32

// ErrBadKey is returned for a key that is not Base64 of 32 bytes.
var ErrBadKey = fmt.Errorf("secretbox: key must be Base64 of %d bytes (openssl rand -base64 32)", KeyBytes)

// ErrDecrypt is returned when the data is not ours, was altered, or the key is wrong.
var ErrDecrypt = errors.New("secretbox: cannot decrypt")

// Box seals and opens values.
type Box struct{ aead cipher.AEAD }

// New builds a Box from the Base64 key.
func New(key string) (*Box, error) {
	k, err := base64.StdEncoding.DecodeString(key)
	if err != nil || len(k) != KeyBytes {
		return nil, ErrBadKey
	}
	block, err := aes.NewCipher(k)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{aead: aead}, nil
}

// Seal encrypts plain; context (for example the user ID) is authenticated but
// not stored, so a sealed value cannot be moved to another row.
func (b *Box) Seal(plain, context []byte) ([]byte, error) {
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return b.aead.Seal(nonce, nonce, plain, context), nil
}

// Open decrypts what Seal produced with the same context.
func (b *Box) Open(sealed, context []byte) ([]byte, error) {
	n := b.aead.NonceSize()
	if len(sealed) < n+b.aead.Overhead() {
		return nil, ErrDecrypt
	}
	plain, err := b.aead.Open(nil, sealed[:n], sealed[n:], context)
	if err != nil {
		return nil, ErrDecrypt
	}
	return plain, nil
}
