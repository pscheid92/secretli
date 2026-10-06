package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pscheid92/secretli/internal/domain"
)

// goneDetails is what the metadata endpoint tells about a secret that is gone.
type goneDetails struct {
	Outcome       string `json:"outcome"`
	BurnAfterRead bool   `json:"burn_after_read"`
	EndedAt       string `json:"ended_at"`
	FirstOpenedAt string `json:"first_opened_at"`
	OpenedByOwner bool   `json:"opened_by_owner"`
}

func getMetadata(t *testing.T, h *SecretHandler, publicID, metadataToken string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/secrets/"+publicID+"/meta", nil)
	req.Header.Set(HeaderMetadataToken, metadataToken)
	rec := httptest.NewRecorder()
	c := newEchoContext(req, rec)
	c.SetParamNames("publicID")
	c.SetParamValues(publicID)
	callHandler(c, h.SecretMetadata)
	return rec
}

func decodeGone(t *testing.T, rec *httptest.ResponseRecorder) goneDetails {
	t.Helper()
	if rec.Code != http.StatusGone {
		t.Fatalf("status = %d, want %d. body: %s", rec.Code, http.StatusGone, rec.Body.String())
	}
	var body struct {
		Error   string      `json:"error"`
		Details goneDetails `json:"details"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	return body.Details
}

func decodeMetadata(t *testing.T, rec *httptest.ResponseRecorder) domain.SecretMetadataResponse {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d. body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var body domain.SecretMetadataResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	return body
}

// startSessionAs opens a secret as a recipient, or as the owner when the
// deletion token is given.
func startSessionAs(t *testing.T, h *SecretHandler, publicID, blobToken, deletionToken string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets/"+publicID+"/retrieval-session", nil)
	req.Header.Set(HeaderBlobToken, blobToken)
	if deletionToken != "" {
		req.Header.Set(HeaderDeletionToken, deletionToken)
	}
	rec := httptest.NewRecorder()
	c := newEchoContext(req, rec)
	c.SetParamNames("publicID")
	c.SetParamValues(publicID)
	callHandler(c, h.StartRetrievalSession)
	return rec
}

func deleteSecretAs(t *testing.T, h *SecretHandler, publicID, metadataToken, deletionToken string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/secrets/"+publicID, nil)
	req.Header.Set(HeaderMetadataToken, metadataToken)
	req.Header.Set(HeaderDeletionToken, deletionToken)
	rec := httptest.NewRecorder()
	c := newEchoContext(req, rec)
	c.SetParamNames("publicID")
	c.SetParamValues(publicID)
	callHandler(c, h.DeleteSecret)
	return rec
}

func TestSecretMetadata_OpenedOneTimeSecretTellsWhenItWasOpened(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	h := NewSecretHandler(repo, fs, testMetrics())
	publicID := testPublicID("gone opened")
	token := testToken("gone opened token")
	seedSecret(repo, fs, publicID, token, testToken("gone opened deletion"), true)

	startTestRetrievalSession(t, h, publicID, token)

	details := decodeGone(t, getMetadata(t, h, publicID, token))
	if details.Outcome != "opened" || !details.BurnAfterRead || details.OpenedByOwner {
		t.Errorf("details = %+v, want a one-time secret opened by a recipient", details)
	}
	if details.EndedAt == "" || details.FirstOpenedAt != details.EndedAt {
		t.Errorf("first_opened_at = %q, ended_at = %q, want the same moment", details.FirstOpenedAt, details.EndedAt)
	}
}

func TestSecretMetadata_OwnerOpeningTheirOneTimeSecretIsToldApart(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	h := NewSecretHandler(repo, fs, testMetrics())
	publicID := testPublicID("gone owner opened")
	token := testToken("gone owner opened token")
	deletionToken := testToken("gone owner opened deletion")
	seedSecret(repo, fs, publicID, token, deletionToken, true)

	if rec := startSessionAs(t, h, publicID, token, deletionToken); rec.Code != http.StatusCreated {
		t.Fatalf("owner open status = %d, want %d. body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	details := decodeGone(t, getMetadata(t, h, publicID, token))
	if details.Outcome != "opened" || !details.OpenedByOwner {
		t.Errorf("details = %+v, want opened by the owner", details)
	}
	if details.FirstOpenedAt != "" {
		t.Errorf("first_opened_at = %q, want none: no recipient got it", details.FirstOpenedAt)
	}
}

func TestSecretMetadata_DeletedSecretTellsWhenItWasDeleted(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	h := NewSecretHandler(repo, fs, testMetrics())
	publicID := testPublicID("gone deleted")
	token := testToken("gone deleted token")
	deletionToken := testToken("gone deleted deletion")
	seedSecret(repo, fs, publicID, token, deletionToken, false)

	if rec := deleteSecretAs(t, h, publicID, token, deletionToken); rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want %d. body: %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}

	details := decodeGone(t, getMetadata(t, h, publicID, token))
	if details.Outcome != "deleted" || details.BurnAfterRead || details.OpenedByOwner {
		t.Errorf("details = %+v, want a deleted reusable secret", details)
	}
	if details.EndedAt == "" || details.FirstOpenedAt != "" {
		t.Errorf("ended_at = %q, first_opened_at = %q, want a time and none", details.EndedAt, details.FirstOpenedAt)
	}
}

func TestSecretMetadata_WhatBecameOfASecretNeedsItsLink(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	h := NewSecretHandler(repo, fs, testMetrics())
	publicID := testPublicID("gone guarded")
	token := testToken("gone guarded token")
	seedSecret(repo, fs, publicID, token, testToken("gone guarded deletion"), true)
	startTestRetrievalSession(t, h, publicID, token)
	// Once the cleanup has taken the row, only the tombstone is left.
	if _, err := repo.DeleteExpired(context.Background(), time.Now(), 10, func(string) error { return nil }); err != nil {
		t.Fatalf("cleanup: %v", err)
	}

	rec := getMetadata(t, h, publicID, testToken("somebody else's token"))

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d: a guess must learn nothing", rec.Code, http.StatusNotFound)
	}
}

func TestSecretMetadata_ExpiredSecretAwaitingCleanupIsGoneAsExpired(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	h := NewSecretHandler(repo, fs, testMetrics())
	publicID := testPublicID("gone expired")
	token := testToken("gone expired token")
	seedSecret(repo, fs, publicID, token, testToken("gone expired deletion"), true)
	expiredAt := time.Now().Add(-time.Minute)
	repo.secrets[publicID].ExpiresAt = expiredAt

	details := decodeGone(t, getMetadata(t, h, publicID, token))

	if details.Outcome != "expired" || details.FirstOpenedAt != "" {
		t.Errorf("details = %+v, want expired unopened", details)
	}
	if details.EndedAt != expiredAt.UTC().Format(time.RFC3339) {
		t.Errorf("ended_at = %q, want the expiry %v", details.EndedAt, expiredAt)
	}
}

func TestSecretMetadata_ReusableSecretTellsWhenARecipientFirstOpenedIt(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	h := NewSecretHandler(repo, fs, testMetrics())
	publicID := testPublicID("reusable opened")
	token := testToken("reusable opened token")
	deletionToken := testToken("reusable opened deletion")
	seedSecret(repo, fs, publicID, token, deletionToken, false)

	// The owner looking at their own secret does not count.
	if rec := startSessionAs(t, h, publicID, token, deletionToken); rec.Code != http.StatusCreated {
		t.Fatalf("owner open status = %d. body: %s", rec.Code, rec.Body.String())
	}
	if meta := decodeMetadata(t, getMetadata(t, h, publicID, token)); meta.OpenedAt != nil {
		t.Errorf("opened_at = %q after the owner's own look, want none", *meta.OpenedAt)
	}

	startTestRetrievalSession(t, h, publicID, token)
	meta := decodeMetadata(t, getMetadata(t, h, publicID, token))
	if meta.OpenedAt == nil {
		t.Fatal("opened_at missing after a recipient opened the secret")
	}
	if _, err := time.Parse(time.RFC3339, *meta.OpenedAt); err != nil {
		t.Errorf("opened_at = %q, want RFC 3339", *meta.OpenedAt)
	}
}

func TestStartRetrievalSession_MalformedDeletionTokenIsRejected(t *testing.T) {
	repo := newMockRepo()
	fs := newMockFileStore()
	h := NewSecretHandler(repo, fs, testMetrics())
	publicID := testPublicID("malformed deletion")
	token := testToken("malformed deletion token")
	seedSecret(repo, fs, publicID, token, testToken("malformed deletion deletion"), false)

	rec := startSessionAs(t, h, publicID, token, "not a token")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}
