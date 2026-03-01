package crypto

import "testing"

func TestHashToken_Deterministic(t *testing.T) {
	token := "dGVzdHRva2VuMTIzNDU2Nzg"
	h1 := HashToken(token)
	h2 := HashToken(token)
	if h1 != h2 {
		t.Errorf("HashToken not deterministic: %q != %q", h1, h2)
	}
}

func TestHashToken_DifferentInputs(t *testing.T) {
	h1 := HashToken("dGVzdHRva2VuMTIzNDU2Nzg")
	h2 := HashToken("ZGlmZmVyZW50dG9rZW4xMjM")
	if h1 == h2 {
		t.Error("HashToken produced same hash for different inputs")
	}
}

func TestVerifyToken_Valid(t *testing.T) {
	token := "dGVzdHRva2VuMTIzNDU2Nzg"
	hash := HashToken(token)
	if !VerifyToken(token, hash) {
		t.Error("VerifyToken returned false for valid token")
	}
}

func TestVerifyToken_Invalid(t *testing.T) {
	hash := HashToken("dGVzdHRva2VuMTIzNDU2Nzg")
	if VerifyToken("ZGlmZmVyZW50dG9rZW4xMjM", hash) {
		t.Error("VerifyToken returned true for invalid token")
	}
}

func TestHashToken_NonBase64Fallback(t *testing.T) {
	// This string contains characters invalid in base64url: spaces, special chars
	nonB64 := "not valid base64!!! @#$%"
	h1 := HashToken(nonB64)
	h2 := HashToken(nonB64)

	if h1 == "" {
		t.Error("HashToken returned empty string for non-base64 input")
	}
	if h1 != h2 {
		t.Errorf("HashToken not deterministic for non-base64: %q != %q", h1, h2)
	}

	// Ensure non-base64 hashes differently from a valid base64 token
	b64Hash := HashToken("dGVzdHRva2VuMTIzNDU2Nzg")
	if h1 == b64Hash {
		t.Error("non-base64 hash should differ from base64 hash")
	}
}

func TestVerifyToken_NonBase64(t *testing.T) {
	token := "not-base64!@#"
	hash := HashToken(token)
	if !VerifyToken(token, hash) {
		t.Error("VerifyToken returned false for valid non-base64 token")
	}
	if VerifyToken("other-non-base64!@#", hash) {
		t.Error("VerifyToken returned true for different non-base64 token")
	}
}
