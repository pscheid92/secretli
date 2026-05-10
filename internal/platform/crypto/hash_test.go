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

func TestTokenHash(t *testing.T) {
	got := TokenHash("secret-token")
	want := "930bbdc51b6aed5c2a5678fd6e28dee7a05e8a4b643cfc0b4427c3efb86c0d94"
	if got != want {
		t.Errorf("TokenHash() = %q, want %q", got, want)
	}
	if len(got) != 64 {
		t.Errorf("TokenHash() length = %d, want 64", len(got))
	}
}
