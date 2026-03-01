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
