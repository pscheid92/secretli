package domain

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestValidPublicID(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want bool
	}{
		{name: "valid", id: strings.Repeat("A", PublicIDLength), want: true},
		{name: "valid url chars", id: strings.Repeat("a", PublicIDLength-2) + "-_", want: true},
		{name: "empty", id: "", want: false},
		{name: "short", id: strings.Repeat("A", PublicIDLength-1), want: false},
		{name: "long", id: strings.Repeat("A", PublicIDLength+1), want: false},
		{name: "padding", id: strings.Repeat("A", PublicIDLength-1) + "=", want: false},
		{name: "slash", id: strings.Repeat("A", PublicIDLength-1) + "/", want: false},
		{name: "plus", id: strings.Repeat("A", PublicIDLength-1) + "+", want: false},
		{name: "space", id: strings.Repeat("A", PublicIDLength-1) + " ", want: false},
		{name: "newline", id: strings.Repeat("A", PublicIDLength-1) + "\n", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ValidPublicID(tt.id); got != tt.want {
				t.Fatalf("ValidPublicID(%q) = %t, want %t", tt.id, got, tt.want)
			}
		})
	}
}

func TestValidToken(t *testing.T) {
	tests := []struct {
		name  string
		token string
		want  bool
	}{
		{name: "valid", token: strings.Repeat("A", TokenLength), want: true},
		{name: "valid url chars", token: strings.Repeat("a", TokenLength-2) + "-_", want: true},
		{name: "empty", token: "", want: false},
		{name: "short", token: strings.Repeat("A", TokenLength-1), want: false},
		{name: "long", token: strings.Repeat("A", TokenLength+1), want: false},
		{name: "padding", token: strings.Repeat("A", TokenLength-1) + "=", want: false},
		{name: "slash", token: strings.Repeat("A", TokenLength-1) + "/", want: false},
		{name: "plus", token: strings.Repeat("A", TokenLength-1) + "+", want: false},
		{name: "space", token: strings.Repeat("A", TokenLength-1) + " ", want: false},
		{name: "newline", token: strings.Repeat("A", TokenLength-1) + "\n", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ValidToken(tt.token); got != tt.want {
				t.Fatalf("ValidToken(%q) = %t, want %t", tt.token, got, tt.want)
			}
		})
	}
}

func TestValidEncryptedMeta(t *testing.T) {
	nonceV1 := rawURL(strings.Repeat("1", metadataV1NonceLength))
	nonceV2 := rawURL(strings.Repeat("2", metadataV2NonceLength))
	ciphertext := rawURL("ciphertext")

	tests := []struct {
		name     string
		envelope string
		want     bool
	}{
		{name: "valid v1", envelope: "v1$" + nonceV1 + "$" + ciphertext, want: true},
		{name: "valid v2", envelope: "v2$" + nonceV2 + "$" + ciphertext, want: true},
		{name: "empty", envelope: "", want: false},
		{name: "bad version", envelope: "v3$" + nonceV1 + "$" + ciphertext, want: false},
		{name: "missing part", envelope: "v1$" + nonceV1, want: false},
		{name: "extra part", envelope: "v1$" + nonceV1 + "$" + ciphertext + "$extra", want: false},
		{name: "bad nonce base64url", envelope: "v1$" + strings.Repeat("A", 15) + "+$" + ciphertext, want: false},
		{name: "v1 wrong nonce length", envelope: "v1$" + nonceV2 + "$" + ciphertext, want: false},
		{name: "v2 wrong nonce length", envelope: "v2$" + nonceV1 + "$" + ciphertext, want: false},
		{name: "empty ciphertext", envelope: "v1$" + nonceV1 + "$", want: false},
		{name: "bad ciphertext char", envelope: "v1$" + nonceV1 + "$cipher+", want: false},
		{name: "bad ciphertext base64", envelope: "v1$" + nonceV1 + "$A", want: false},
		{name: "oversized", envelope: "v1$" + nonceV1 + "$" + strings.Repeat("A", EncryptedMetaMaxBytes), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ValidEncryptedMeta(tt.envelope); got != tt.want {
				t.Fatalf("ValidEncryptedMeta(%q) = %t, want %t", tt.envelope, got, tt.want)
			}
		})
	}
}

func rawURL(s string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(s))
}
