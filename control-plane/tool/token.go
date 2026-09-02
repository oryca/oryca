package tool

import (
	"crypto/rand"
	"encoding/hex"
)

// GenerateToken returns a cryptographically random hex string of length*2 characters.
// e.g. length=32 → 64-char hex string
func GenerateToken(length int) (string, error) {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
