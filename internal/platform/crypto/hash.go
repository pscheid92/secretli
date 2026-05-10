package crypto

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
)

// TokensEqual compares two tokens in constant time to prevent timing attacks.
func TokensEqual(a, b string) bool {
	x, y := []byte(a), []byte(b)
	return subtle.ConstantTimeCompare(x, y) == 1
}

// TokenHash returns the fixed-length SHA-256 digest used for stored bearer tokens.
func TokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
