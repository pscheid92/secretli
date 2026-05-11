package domain

import (
	"encoding/base64"
	"strings"
)

const (
	PublicIDLength        = 22
	TokenLength           = 43
	EncryptedMetaMaxBytes = 8192

	metadataV2NonceLength = 24
)

func ValidPublicID(publicID string) bool {
	return len(publicID) == PublicIDLength && isUnpaddedBase64URL(publicID)
}

func ValidToken(token string) bool {
	return len(token) == TokenLength && isUnpaddedBase64URL(token)
}

func ValidEncryptedMeta(envelope string) bool {
	if envelope == "" || len(envelope) > EncryptedMetaMaxBytes {
		return false
	}

	parts := strings.Split(envelope, "$")
	if len(parts) != 3 {
		return false
	}

	if parts[0] != "v2" {
		return false
	}

	nonce, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || len(nonce) != metadataV2NonceLength {
		return false
	}

	if parts[2] == "" || !isUnpaddedBase64URL(parts[2]) {
		return false
	}

	_, err = base64.RawURLEncoding.DecodeString(parts[2])
	return err == nil
}

func isUnpaddedBase64URL(value string) bool {
	if value == "" {
		return false
	}

	for _, r := range value {
		if r >= 'A' && r <= 'Z' {
			continue
		}
		if r >= 'a' && r <= 'z' {
			continue
		}
		if r >= '0' && r <= '9' {
			continue
		}
		if r == '-' || r == '_' {
			continue
		}
		return false
	}

	return true
}
