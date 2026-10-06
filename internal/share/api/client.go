// Package api is the HTTP client for a Secretli server: upload sessions and
// parts, metadata, retrieval sessions, range reads and deletion. It knows
// nothing about encryption; the share package drives it.
package api

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	// MaxTransientAttempts is how often a request is tried when the failure
	// looks temporary: a network error, rate limiting, or a server error.
	MaxTransientAttempts = 3
	maxRetryAfter        = 30 * time.Second

	headerMetadataToken = "X-Metadata-Token" //nolint:gosec // header names, not credentials
	headerBlobToken     = "X-Blob-Token"     //nolint:gosec
	headerDeletionToken = "X-Deletion-Token" //nolint:gosec
	headerRequestID     = "X-Request-ID"
	headerPartOffset    = "X-Part-Offset"
	headerPartSize      = "X-Part-Size"
	headerPartSHA256    = "X-Part-SHA256"
)

// Client talks to one Secretli server.
type Client struct {
	// BaseURL is the server's origin, such as https://secretli.app.
	BaseURL string
	// HTTP is the client used for every request. Cancellation and deadlines
	// come from the context, so it needs no timeout of its own.
	HTTP      *http.Client
	UserAgent string
}

// New makes a client for the server at baseURL.
func New(baseURL string) *Client {
	return &Client{BaseURL: strings.TrimRight(baseURL, "/"), HTTP: &http.Client{}, UserAgent: "secretli-cli"}
}

// Error is an answer the server gave that was not a success.
type Error struct {
	Status     int
	Message    string
	RequestID  string
	RetryAfter string
	Details    map[string]any
}

func (e *Error) Error() string {
	if e.Status == 0 {
		return "network error: " + e.Message
	}
	return fmt.Sprintf("%s (HTTP %d)", e.Message, e.Status)
}

// IsStatus reports whether err is a server answer with this status.
func IsStatus(err error, status int) bool {
	var apiErr *Error
	return errors.As(err, &apiErr) && apiErr.Status == status
}

// Gone is what the server tells about a secret that is no longer there: the
// details of its 410 answer, kept for a week after the secret ended.
type Gone struct {
	// Outcome is opened, expired or deleted.
	Outcome       string
	BurnAfterRead bool
	// EndedAt is when it happened; for an expired secret, its expiry.
	EndedAt time.Time
	// FirstOpenedAt is when a recipient first opened it, if anyone did.
	FirstOpenedAt *time.Time
	// OpenedByOwner marks a one-time secret the owner opened themselves.
	OpenedByOwner bool
}

// AsGone extracts the story from a 410 answer.
func AsGone(err error) (*Gone, bool) {
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusGone || apiErr.Details == nil {
		return nil, false
	}
	outcome, _ := apiErr.Details["outcome"].(string)
	endedAt, ok := apiErr.Details["ended_at"].(string)
	if outcome == "" || !ok {
		return nil, false
	}
	ended, err := time.Parse(time.RFC3339, endedAt)
	if err != nil {
		return nil, false
	}
	gone := &Gone{Outcome: outcome, EndedAt: ended}
	gone.BurnAfterRead, _ = apiErr.Details["burn_after_read"].(bool)
	gone.OpenedByOwner, _ = apiErr.Details["opened_by_owner"].(bool)
	if first, ok := apiErr.Details["first_opened_at"].(string); ok {
		if t, err := time.Parse(time.RFC3339, first); err == nil {
			gone.FirstOpenedAt = &t
		}
	}
	return gone, true
}

// Metadata is what the server knows about a live secret.
type Metadata struct {
	EncryptedMeta string
	BlobSize      int64
	BurnAfterRead bool
	ExpiresAt     time.Time
	CreatedAt     time.Time
	// OpenedAt is when a recipient first opened a reusable secret, if one has.
	OpenedAt *time.Time
}

// Metadata fetches a secret's metadata. A secret that is gone comes back as
// an *Error with status 410 that AsGone can read.
func (c *Client) Metadata(ctx context.Context, publicID, metadataToken string) (*Metadata, error) {
	var body struct {
		EncryptedMeta string  `json:"encrypted_meta"`
		BlobSize      int64   `json:"blob_size"`
		BurnAfterRead bool    `json:"burn_after_read"`
		ExpiresAt     string  `json:"expires_at"`
		CreatedAt     string  `json:"created_at"`
		OpenedAt      *string `json:"opened_at"`
	}
	headers := http.Header{headerMetadataToken: {metadataToken}}
	if err := c.doJSON(ctx, http.MethodGet, "/api/v1/secrets/"+publicID+"/meta", headers, nil, &body); err != nil {
		return nil, err
	}
	expires, err := time.Parse(time.RFC3339, body.ExpiresAt)
	if err != nil {
		return nil, fmt.Errorf("metadata expires_at: %w", err)
	}
	created, err := time.Parse(time.RFC3339, body.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("metadata created_at: %w", err)
	}
	meta := &Metadata{EncryptedMeta: body.EncryptedMeta, BlobSize: body.BlobSize, BurnAfterRead: body.BurnAfterRead, ExpiresAt: expires, CreatedAt: created}
	if body.OpenedAt != nil {
		if t, err := time.Parse(time.RFC3339, *body.OpenedAt); err == nil {
			meta.OpenedAt = &t
		}
	}
	return meta, nil
}

// RetrievalSession is a window of 15 minutes for reading the blob.
type RetrievalSession struct {
	Token         string
	BlobSize      int64
	ExpiresAt     time.Time
	BurnAfterRead bool
}

// StartRetrievalSession opens a secret for reading, which ends a one-time
// secret. deletionToken, when the caller holds the owner link, marks the
// owner's own look.
func (c *Client) StartRetrievalSession(ctx context.Context, publicID, blobToken, deletionToken string) (*RetrievalSession, error) {
	var body struct {
		SessionToken  string `json:"session_token"`
		BlobSize      int64  `json:"blob_size"`
		ExpiresAt     string `json:"expires_at"`
		BurnAfterRead bool   `json:"burn_after_read"`
	}
	headers := http.Header{headerBlobToken: {blobToken}}
	if deletionToken != "" {
		headers.Set(headerDeletionToken, deletionToken)
	}
	if err := c.doJSON(ctx, http.MethodPost, "/api/v1/secrets/"+publicID+"/retrieval-session", headers, nil, &body); err != nil {
		return nil, err
	}
	expires, err := time.Parse(time.RFC3339, body.ExpiresAt)
	if err != nil {
		return nil, fmt.Errorf("session expires_at: %w", err)
	}
	return &RetrievalSession{Token: body.SessionToken, BlobSize: body.BlobSize, ExpiresAt: expires, BurnAfterRead: body.BurnAfterRead}, nil
}

// ReadRange reads bundle bytes start to end, both inclusive, within a session.
func (c *Client) ReadRange(ctx context.Context, publicID, sessionToken string, start, end int64) ([]byte, error) {
	headers := http.Header{
		"Authorization": {"Bearer " + sessionToken},
		"Range":         {fmt.Sprintf("bytes=%d-%d", start, end)},
	}
	resp, err := c.do(ctx, http.MethodGet, "/api/v1/secrets/"+publicID+"/blob", headers, nil, 0)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusPartialContent {
		return nil, &Error{Status: resp.StatusCode, Message: fmt.Sprintf("expected a partial content answer, got %d", resp.StatusCode), RequestID: resp.Header.Get(headerRequestID)}
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, end-start+2))
	if err != nil {
		return nil, &Error{Message: err.Error()}
	}
	if int64(len(data)) != end-start+1 {
		return nil, &Error{Status: resp.StatusCode, Message: "range answer has the wrong size"}
	}
	return data, nil
}

// Delete removes a secret for everyone; it needs the owner's deletion token.
func (c *Client) Delete(ctx context.Context, publicID, metadataToken, deletionToken string) error {
	headers := http.Header{headerMetadataToken: {metadataToken}, headerDeletionToken: {deletionToken}}
	return c.doJSON(ctx, http.MethodDelete, "/api/v1/secrets/"+publicID, headers, nil, nil)
}

// UploadRequest declares a new secret: its tokens, metadata, lifetime and
// the exact size of the encrypted bundle that will be uploaded.
type UploadRequest struct {
	PublicID      string `json:"public_id"`
	MetadataToken string `json:"metadata_token"`
	BlobToken     string `json:"blob_token"`
	DeletionToken string `json:"deletion_token"`
	EncryptedMeta string `json:"encrypted_meta"`
	Expiration    string `json:"expiration"`
	BurnAfterRead bool   `json:"burn_after_read"`
	BlobSize      int64  `json:"blob_size"`
}

// UploadSession is an open multipart upload.
type UploadSession struct {
	ID        string
	Token     string
	PublicID  string
	PartSize  int64
	BlobSize  int64
	ExpiresAt time.Time
}

// StartUpload creates the upload session for a new secret.
func (c *Client) StartUpload(ctx context.Context, req UploadRequest) (*UploadSession, error) {
	var body struct {
		SessionID   string `json:"session_id"`
		UploadToken string `json:"upload_token"`
		PublicID    string `json:"public_id"`
		PartSize    int64  `json:"part_size"`
		BlobSize    int64  `json:"blob_size"`
		ExpiresAt   string `json:"expires_at"`
	}
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("encode upload request: %w", err)
	}
	if err := c.doJSON(ctx, http.MethodPost, "/api/v1/secrets/uploads", http.Header{"Content-Type": {"application/json"}}, payload, &body); err != nil {
		return nil, err
	}
	expires, err := time.Parse(time.RFC3339, body.ExpiresAt)
	if err != nil {
		return nil, fmt.Errorf("upload expires_at: %w", err)
	}
	return &UploadSession{ID: body.SessionID, Token: body.UploadToken, PublicID: body.PublicID, PartSize: body.PartSize, BlobSize: body.BlobSize, ExpiresAt: expires}, nil
}

// UploadPart sends one part of the bundle. Parts are numbered from 1 and
// every part but the last must be at least 5 MiB.
func (c *Client) UploadPart(ctx context.Context, sessionID, token string, partNumber int, offset int64, data []byte, sha256Hex string) error {
	headers := http.Header{
		"Authorization":  {"Bearer " + token},
		"Content-Type":   {"application/octet-stream"},
		headerPartOffset: {strconv.FormatInt(offset, 10)},
		headerPartSize:   {strconv.Itoa(len(data))},
		headerPartSHA256: {sha256Hex},
	}
	path := fmt.Sprintf("/api/v1/secrets/uploads/%s/parts/%d", sessionID, partNumber)
	return c.doJSON(ctx, http.MethodPut, path, headers, data, nil)
}

// CompleteUpload turns the uploaded parts into the secret and returns its
// expiry. Repeating it after a lost answer gives the same result.
func (c *Client) CompleteUpload(ctx context.Context, sessionID, token string) (time.Time, error) {
	var body struct {
		ExpiresAt string `json:"expires_at"`
	}
	headers := http.Header{"Authorization": {"Bearer " + token}}
	if err := c.doJSON(ctx, http.MethodPost, "/api/v1/secrets/uploads/"+sessionID+"/complete", headers, nil, &body); err != nil {
		return time.Time{}, err
	}
	expires, err := time.Parse(time.RFC3339, body.ExpiresAt)
	if err != nil {
		return time.Time{}, fmt.Errorf("complete expires_at: %w", err)
	}
	return expires, nil
}

// AbortUpload releases an upload that will not be completed.
func (c *Client) AbortUpload(ctx context.Context, sessionID, token string) error {
	headers := http.Header{"Authorization": {"Bearer " + token}}
	return c.doJSON(ctx, http.MethodDelete, "/api/v1/secrets/uploads/"+sessionID, headers, nil, nil)
}

// doJSON performs a request and decodes a JSON answer into out, if out is set.
func (c *Client) doJSON(ctx context.Context, method, path string, headers http.Header, body []byte, out any) error {
	resp, err := c.do(ctx, method, path, headers, body, int64(len(body)))
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if out == nil || resp.StatusCode == http.StatusNoContent {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return &Error{Status: resp.StatusCode, Message: "unreadable answer: " + err.Error(), RequestID: resp.Header.Get(headerRequestID)}
	}
	return nil
}

// do sends the request, retrying transient failures, and returns a response
// with a 2xx status; anything else becomes an *Error.
func (c *Client) do(ctx context.Context, method, path string, headers http.Header, body []byte, contentLength int64) (*http.Response, error) {
	var lastErr error
	for attempt := 1; attempt <= MaxTransientAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		resp, err := c.once(ctx, method, path, headers, body, contentLength)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		var apiErr *Error
		if !errors.As(err, &apiErr) || !transient(apiErr.Status) || attempt == MaxTransientAttempts {
			return nil, err
		}
		if err := sleep(ctx, retryDelay(attempt, apiErr.RetryAfter)); err != nil {
			return nil, err
		}
	}
	return nil, lastErr
}

func (c *Client) once(ctx context.Context, method, path string, headers http.Header, body []byte, contentLength int64) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, reader)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.ContentLength = contentLength
	for name, values := range headers {
		for _, v := range values {
			req.Header.Add(name, v)
		}
	}
	requestID := newRequestID()
	req.Header.Set(headerRequestID, requestID)
	req.Header.Set("User-Agent", c.UserAgent)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, &Error{Message: err.Error(), RequestID: requestID}
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return resp, nil
	}
	defer func() { _ = resp.Body.Close() }()
	apiErr := &Error{Status: resp.StatusCode, Message: fmt.Sprintf("request failed (%d)", resp.StatusCode), RetryAfter: resp.Header.Get("Retry-After")}
	if id := resp.Header.Get(headerRequestID); id != "" {
		apiErr.RequestID = id
	} else {
		apiErr.RequestID = requestID
	}
	var payload struct {
		Error   string         `json:"error"`
		Details map[string]any `json:"details"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64*1024)).Decode(&payload); err == nil {
		if payload.Error != "" {
			apiErr.Message = payload.Error
		}
		apiErr.Details = payload.Details
	}
	return nil, apiErr
}

// transient is a failure worth retrying: no answer, rate limiting, or a
// server error. Client errors are final.
func transient(status int) bool {
	return status == 0 || status == http.StatusTooManyRequests || status >= 500
}

func retryDelay(attempt int, retryAfter string) time.Duration {
	if retryAfter != "" {
		if seconds, err := strconv.Atoi(retryAfter); err == nil && seconds >= 0 {
			return min(time.Duration(seconds)*time.Second, maxRetryAfter)
		}
	}
	return time.Duration(attempt) * 250 * time.Millisecond
}

func sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "req-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	h := hex.EncodeToString(b[:])
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}
