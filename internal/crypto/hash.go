package crypto

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
)

func HashToken(token string) string {
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		// If not valid base64, hash the raw string bytes
		h := sha256.Sum256([]byte(token))
		return hex.EncodeToString(h[:])
	}
	h := sha256.Sum256(decoded)
	return hex.EncodeToString(h[:])
}

func VerifyToken(token, storedHash string) bool {
	computed := HashToken(token)
	return subtle.ConstantTimeCompare([]byte(computed), []byte(storedHash)) == 1
}
