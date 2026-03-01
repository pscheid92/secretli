package crypto

import "crypto/subtle"

// TokensEqual compares two tokens in constant time to prevent timing attacks.
func TokensEqual(a, b string) bool {
	x, y := []byte(a), []byte(b)
	return subtle.ConstantTimeCompare(x, y) == 1
}
