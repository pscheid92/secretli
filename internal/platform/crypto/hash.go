package crypto

import "crypto/subtle"

// TokensEqual compares two tokens in constant time to prevent timing attacks.
func TokensEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
