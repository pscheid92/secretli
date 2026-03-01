package crypto

import "testing"

func TestTokensEqual_Match(t *testing.T) {
	if !TokensEqual("abc123", "abc123") {
		t.Error("TokensEqual returned false for identical tokens")
	}
}

func TestTokensEqual_Mismatch(t *testing.T) {
	if TokensEqual("abc123", "xyz789") {
		t.Error("TokensEqual returned true for different tokens")
	}
}

func TestTokensEqual_Empty(t *testing.T) {
	if !TokensEqual("", "") {
		t.Error("TokensEqual returned false for two empty strings")
	}
	if TokensEqual("", "notempty") {
		t.Error("TokensEqual returned true for empty vs non-empty")
	}
}
