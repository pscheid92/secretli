package httpserver

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/pscheid92/secretli/internal/domain"
	"github.com/pscheid92/secretli/internal/platform/config"
)

// transferMockRepo mirrors the Postgres transfer semantics in memory: the
// smallest free nameplate, one claim, write-once messages.
type transferMockRepo struct {
	mu        sync.Mutex
	transfers map[string]domain.Transfer
	messages  map[string][]byte
	full      bool
}

func newTransferMockRepo() *transferMockRepo {
	return &transferMockRepo{transfers: map[string]domain.Transfer{}, messages: map[string][]byte{}}
}

func (m *transferMockRepo) active(t domain.Transfer, now time.Time) bool {
	return (t.State == domain.TransferStateOpen || t.State == domain.TransferStateClaimed) && now.Before(t.ExpiresAt)
}

func (m *transferMockRepo) CreateTransfer(_ context.Context, t *domain.Transfer, maxNameplate int, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.full {
		return domain.ErrConflict
	}
	used := map[int]bool{}
	for _, existing := range m.transfers {
		if m.active(existing, now) {
			used[existing.Nameplate] = true
		}
	}
	for n := 1; n <= maxNameplate; n++ {
		if !used[n] {
			t.Nameplate = n
			m.transfers[t.TransferID] = *t
			return nil
		}
	}
	return domain.ErrConflict
}

func (m *transferMockRepo) ClaimTransfer(_ context.Context, nameplate int, receiverTokenHash string, now time.Time) (*domain.Transfer, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, t := range m.transfers {
		if t.Nameplate != nameplate || !m.active(t, now) {
			continue
		}
		if t.State == domain.TransferStateClaimed {
			return nil, domain.ErrConflict
		}
		t.State = domain.TransferStateClaimed
		t.ReceiverTokenHash = receiverTokenHash
		t.ClaimedAt = &now
		m.transfers[id] = t
		return &t, nil
	}
	return nil, domain.ErrNotFound
}

func (m *transferMockRepo) GetTransfer(_ context.Context, transferID string) (*domain.Transfer, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.transfers[transferID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return &t, nil
}

func messageKey(transferID, side, phase string) string {
	return transferID + "|" + side + "|" + phase
}

func (m *transferMockRepo) PutTransferMessage(_ context.Context, msg *domain.TransferMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.transfers[msg.TransferID]; !ok {
		return domain.ErrNotFound
	}
	key := messageKey(msg.TransferID, msg.Side, msg.Phase)
	if _, ok := m.messages[key]; ok {
		return domain.ErrDuplicate
	}
	m.messages[key] = msg.Data
	return nil
}

func (m *transferMockRepo) GetTransferMessage(_ context.Context, transferID, side, phase string) (*domain.TransferMessage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	data, ok := m.messages[messageKey(transferID, side, phase)]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return &domain.TransferMessage{TransferID: transferID, Side: side, Phase: phase, Data: data}, nil
}

func (m *transferMockRepo) CloseTransfer(_ context.Context, transferID, reason string, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.transfers[transferID]
	if !ok || t.State == domain.TransferStateClosed {
		return domain.ErrNotFound
	}
	t.State = domain.TransferStateClosed
	t.CloseReason = reason
	t.ClosedAt = &now
	m.transfers[transferID] = t
	return nil
}

func (m *transferMockRepo) DeleteEndedTransfers(_ context.Context, endedBefore time.Time) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var n int64
	for id, t := range m.transfers {
		if t.ExpiresAt.Before(endedBefore) || (t.ClosedAt != nil && t.ClosedAt.Before(endedBefore)) {
			delete(m.transfers, id)
			n++
		}
	}
	return n, nil
}

// --- Test helpers ---

type transferTestServer struct {
	e    *echo.Echo
	repo *transferMockRepo
}

// fakeTransferEvents stands in for the Postgres listener: a test signals a
// transfer by hand, where a trigger would after a write.
type fakeTransferEvents struct {
	mu      sync.Mutex
	waiters map[string][]chan struct{}
}

func (f *fakeTransferEvents) Subscribe(transferID string) (<-chan struct{}, func()) {
	signal := make(chan struct{}, 1)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.waiters == nil {
		f.waiters = map[string][]chan struct{}{}
	}
	f.waiters[transferID] = append(f.waiters[transferID], signal)
	return signal, func() {}
}

func (f *fakeTransferEvents) signal(transferID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, signal := range f.waiters[transferID] {
		select {
		case signal <- struct{}{}:
		default:
		}
	}
}

func newTransferTestServer(t *testing.T) *transferTestServer {
	t.Helper()
	return newTransferTestServerWith(t, nil, 300*time.Millisecond, 10*time.Millisecond)
}

// newTransferTestServerWith uses events, long-polls of pollWait and the given
// re-check. With a re-check far beyond the test, a poll that answers quickly
// was woken by a signal.
func newTransferTestServerWith(t *testing.T, events *fakeTransferEvents, pollWait, recheck time.Duration) *transferTestServer {
	t.Helper()
	repo := newTransferMockRepo()
	var h *TransferHandler
	if events == nil {
		h = NewTransferHandler(repo, nil)
	} else {
		h = NewTransferHandler(repo, events)
	}
	h.pollWait = pollWait
	h.recheckInterval = recheck

	e := echo.New()
	e.HTTPErrorHandler = httpErrorHandler
	e.POST("/api/v1/transfers", h.CreateTransfer)
	e.POST("/api/v1/transfers/claim", h.ClaimTransfer)
	e.PUT("/api/v1/transfers/:transferID/messages/:phase", h.PutMessage)
	e.GET("/api/v1/transfers/:transferID/messages/:phase", h.GetMessage)
	e.DELETE("/api/v1/transfers/:transferID", h.CloseTransfer)
	return &transferTestServer{e: e, repo: repo}
}

func (s *transferTestServer) do(t *testing.T, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	s.e.ServeHTTP(rec, req)
	return rec
}

type openedTransfer struct {
	Nameplate   int    `json:"nameplate"`
	TransferID  string `json:"transfer_id"`
	SenderToken string `json:"sender_token"`
}

type claimedTransfer struct {
	TransferID    string `json:"transfer_id"`
	ReceiverToken string `json:"receiver_token"`
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode %s: %v", rec.Body.String(), err)
	}
	return v
}

func (s *transferTestServer) open(t *testing.T) openedTransfer {
	t.Helper()
	rec := s.do(t, http.MethodPost, "/api/v1/transfers", "", nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: status = %d, body = %s", rec.Code, rec.Body)
	}
	return decode[openedTransfer](t, rec)
}

func (s *transferTestServer) claim(t *testing.T, nameplate int) claimedTransfer {
	t.Helper()
	rec := s.do(t, http.MethodPost, "/api/v1/transfers/claim", "", map[string]int{"nameplate": nameplate})
	if rec.Code != http.StatusOK {
		t.Fatalf("claim: status = %d, body = %s", rec.Code, rec.Body)
	}
	return decode[claimedTransfer](t, rec)
}

func messagePath(transferID, phase string) string {
	return fmt.Sprintf("/api/v1/transfers/%s/messages/%s", transferID, phase)
}

func data(b []byte) map[string]string {
	return map[string]string{"data": base64.RawURLEncoding.EncodeToString(b)}
}

func (s *transferTestServer) put(t *testing.T, transferID, phase, token string, b []byte) *httptest.ResponseRecorder {
	t.Helper()
	return s.do(t, http.MethodPut, messagePath(transferID, phase), token, data(b))
}

func (s *transferTestServer) get(t *testing.T, transferID, phase, token string) *httptest.ResponseRecorder {
	t.Helper()
	return s.do(t, http.MethodGet, messagePath(transferID, phase), token, nil)
}

func messageData(t *testing.T, rec *httptest.ResponseRecorder) []byte {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("get message: status = %d, body = %s", rec.Code, rec.Body)
	}
	b, err := base64.RawURLEncoding.DecodeString(decode[map[string]string](t, rec)["data"])
	if err != nil {
		t.Fatalf("decode data: %v", err)
	}
	return b
}

func goneReason(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	if rec.Code != http.StatusGone {
		t.Fatalf("status = %d, want 410; body = %s", rec.Code, rec.Body)
	}
	body := decode[struct {
		Details map[string]string `json:"details"`
	}](t, rec)
	return body.Details["reason"]
}

// --- Tests ---

func TestTransferRelaysAFullHandOver(t *testing.T) {
	s := newTransferTestServer(t)
	sender := s.open(t)
	if sender.Nameplate != 1 {
		t.Errorf("nameplate = %d, want the smallest free number 1", sender.Nameplate)
	}
	receiver := s.claim(t, sender.Nameplate)
	if receiver.TransferID != sender.TransferID {
		t.Fatalf("claimed transfer %q, want %q", receiver.TransferID, sender.TransferID)
	}
	id := sender.TransferID

	for _, rec := range []*httptest.ResponseRecorder{
		s.put(t, id, domain.TransferPhaseShare, sender.SenderToken, []byte("share A")),
		s.put(t, id, domain.TransferPhaseShare, receiver.ReceiverToken, []byte("share B")),
		s.put(t, id, domain.TransferPhaseConfirm, receiver.ReceiverToken, []byte("tag B")),
		s.put(t, id, domain.TransferPhasePayload, sender.SenderToken, []byte("sealed link")),
	} {
		if rec.Code != http.StatusNoContent {
			t.Fatalf("put: status = %d, body = %s", rec.Code, rec.Body)
		}
	}

	if got := messageData(t, s.get(t, id, domain.TransferPhaseShare, sender.SenderToken)); string(got) != "share B" {
		t.Errorf("sender read share %q, want the receiver's", got)
	}
	if got := messageData(t, s.get(t, id, domain.TransferPhaseShare, receiver.ReceiverToken)); string(got) != "share A" {
		t.Errorf("receiver read share %q, want the sender's", got)
	}
	if got := messageData(t, s.get(t, id, domain.TransferPhaseConfirm, sender.SenderToken)); string(got) != "tag B" {
		t.Errorf("sender read confirmation %q", got)
	}

	// The sender may finish before the receiver has read the payload.
	if rec := s.do(t, http.MethodDelete, "/api/v1/transfers/"+id+"?reason=done", sender.SenderToken, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("close: status = %d", rec.Code)
	}
	if got := messageData(t, s.get(t, id, domain.TransferPhasePayload, receiver.ReceiverToken)); string(got) != "sealed link" {
		t.Errorf("receiver read payload %q", got)
	}
}

func TestTransferNameplatesAreReusedOnceATransferEnds(t *testing.T) {
	s := newTransferTestServer(t)
	first := s.open(t)
	second := s.open(t)
	if first.Nameplate != 1 || second.Nameplate != 2 {
		t.Fatalf("nameplates = %d, %d, want 1, 2", first.Nameplate, second.Nameplate)
	}

	s.do(t, http.MethodDelete, "/api/v1/transfers/"+first.TransferID, first.SenderToken, nil)

	if third := s.open(t); third.Nameplate != 1 {
		t.Errorf("nameplate = %d, want 1 again", third.Nameplate)
	}
}

func TestTransferCanBeClaimedOnlyOnce(t *testing.T) {
	s := newTransferTestServer(t)
	sender := s.open(t)
	s.claim(t, sender.Nameplate)

	rec := s.do(t, http.MethodPost, "/api/v1/transfers/claim", "", map[string]int{"nameplate": sender.Nameplate})
	if rec.Code != http.StatusConflict {
		t.Errorf("second claim: status = %d, want 409", rec.Code)
	}
}

func TestTransferClaimRejectsUnknownAndInvalidNameplates(t *testing.T) {
	s := newTransferTestServer(t)
	for nameplate, want := range map[int]int{42: http.StatusNotFound, 0: http.StatusBadRequest, 1000: http.StatusBadRequest} {
		rec := s.do(t, http.MethodPost, "/api/v1/transfers/claim", "", map[string]int{"nameplate": nameplate})
		if rec.Code != want {
			t.Errorf("claim %d: status = %d, want %d", nameplate, rec.Code, want)
		}
	}
}

func TestTransferMessagesAreWriteOnce(t *testing.T) {
	s := newTransferTestServer(t)
	sender := s.open(t)
	s.put(t, sender.TransferID, domain.TransferPhaseShare, sender.SenderToken, []byte("first"))

	rec := s.put(t, sender.TransferID, domain.TransferPhaseShare, sender.SenderToken, []byte("second"))
	if rec.Code != http.StatusConflict {
		t.Errorf("rewrite: status = %d, want 409", rec.Code)
	}
}

func TestTransferEnforcesWhoWritesWhichPhase(t *testing.T) {
	s := newTransferTestServer(t)
	sender := s.open(t)
	receiver := s.claim(t, sender.Nameplate)
	id := sender.TransferID

	cases := []struct {
		name string
		rec  *httptest.ResponseRecorder
	}{
		{"sender writes confirm", s.put(t, id, domain.TransferPhaseConfirm, sender.SenderToken, []byte("x"))},
		{"receiver writes payload", s.put(t, id, domain.TransferPhasePayload, receiver.ReceiverToken, []byte("x"))},
		{"sender reads payload", s.get(t, id, domain.TransferPhasePayload, sender.SenderToken)},
		{"receiver reads confirm", s.get(t, id, domain.TransferPhaseConfirm, receiver.ReceiverToken)},
		{"unknown phase", s.put(t, id, "secret", sender.SenderToken, []byte("x"))},
	}
	for _, tc := range cases {
		if tc.rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", tc.name, tc.rec.Code)
		}
	}
}

func TestTransferRejectsForeignAndMalformedTokens(t *testing.T) {
	s := newTransferTestServer(t)
	sender := s.open(t)
	other := s.open(t)

	if rec := s.get(t, sender.TransferID, domain.TransferPhaseShare, other.SenderToken); rec.Code != http.StatusForbidden {
		t.Errorf("another transfer's token: status = %d, want 403", rec.Code)
	}
	if rec := s.get(t, sender.TransferID, domain.TransferPhaseShare, "short"); rec.Code != http.StatusBadRequest {
		t.Errorf("malformed token: status = %d, want 400", rec.Code)
	}
	if rec := s.get(t, "not-a-transfer", domain.TransferPhaseShare, sender.SenderToken); rec.Code != http.StatusBadRequest {
		t.Errorf("malformed transfer id: status = %d, want 400", rec.Code)
	}
}

func TestTransferLongPollReturnsOnceTheOtherSideWrites(t *testing.T) {
	s := newTransferTestServer(t)
	sender := s.open(t)
	receiver := s.claim(t, sender.Nameplate)

	go func() {
		time.Sleep(50 * time.Millisecond)
		s.put(t, sender.TransferID, domain.TransferPhaseShare, receiver.ReceiverToken, []byte("late share"))
	}()

	if got := messageData(t, s.get(t, sender.TransferID, domain.TransferPhaseShare, sender.SenderToken)); string(got) != "late share" {
		t.Errorf("read %q, want the share written while polling", got)
	}
}

func TestTransferLongPollWakesOnANotification(t *testing.T) {
	events := &fakeTransferEvents{}
	s := newTransferTestServerWith(t, events, 20*time.Second, 10*time.Second)
	sender := s.open(t)
	receiver := s.claim(t, sender.Nameplate)

	go func() {
		time.Sleep(50 * time.Millisecond)
		s.put(t, sender.TransferID, domain.TransferPhaseShare, receiver.ReceiverToken, []byte("woken"))
		events.signal(sender.TransferID)
	}()

	start := time.Now()
	if got := messageData(t, s.get(t, sender.TransferID, domain.TransferPhaseShare, sender.SenderToken)); string(got) != "woken" {
		t.Errorf("read %q, want the share that was signalled", got)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("answered after %v, want the notification to wake it", elapsed)
	}
}

func TestTransferLongPollRechecksWhenANotificationNeverArrives(t *testing.T) {
	events := &fakeTransferEvents{}
	s := newTransferTestServerWith(t, events, 20*time.Second, 100*time.Millisecond)
	sender := s.open(t)
	receiver := s.claim(t, sender.Nameplate)

	go func() {
		time.Sleep(50 * time.Millisecond)
		// Written without a signal, as while the listener reconnects.
		s.put(t, sender.TransferID, domain.TransferPhaseShare, receiver.ReceiverToken, []byte("rechecked"))
	}()

	start := time.Now()
	if got := messageData(t, s.get(t, sender.TransferID, domain.TransferPhaseShare, sender.SenderToken)); string(got) != "rechecked" {
		t.Errorf("read %q, want the share found by the re-check", got)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("answered after %v, want the re-check to find it", elapsed)
	}
}

func TestTransferLongPollEndsWhenTheTransferExpires(t *testing.T) {
	events := &fakeTransferEvents{}
	s := newTransferTestServerWith(t, events, 20*time.Second, 10*time.Second)
	sender := s.open(t)
	s.repo.mu.Lock()
	transfer := s.repo.transfers[sender.TransferID]
	transfer.ExpiresAt = time.Now().Add(100 * time.Millisecond)
	s.repo.transfers[sender.TransferID] = transfer
	s.repo.mu.Unlock()

	start := time.Now()
	rec := s.get(t, sender.TransferID, domain.TransferPhaseShare, sender.SenderToken)

	if reason := goneReason(t, rec); reason != domain.TransferCloseExpired {
		t.Errorf("reason = %q, want expired", reason)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("answered after %v, want it right at expiry", elapsed)
	}
}

func TestTransferLongPollTimesOutWithNoContent(t *testing.T) {
	s := newTransferTestServer(t)
	sender := s.open(t)

	start := time.Now()
	rec := s.get(t, sender.TransferID, domain.TransferPhaseShare, sender.SenderToken)

	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", rec.Code)
	}
	if elapsed := time.Since(start); elapsed < 300*time.Millisecond {
		t.Errorf("returned after %v, want it to wait the poll window", elapsed)
	}
}

func TestTransferClosedTellsTheOtherSideWhy(t *testing.T) {
	s := newTransferTestServer(t)
	sender := s.open(t)
	receiver := s.claim(t, sender.Nameplate)

	s.do(t, http.MethodDelete, "/api/v1/transfers/"+sender.TransferID+"?reason=mismatch", sender.SenderToken, nil)

	if reason := goneReason(t, s.get(t, sender.TransferID, domain.TransferPhasePayload, receiver.ReceiverToken)); reason != domain.TransferCloseMismatch {
		t.Errorf("reason = %q, want mismatch", reason)
	}
	if reason := goneReason(t, s.put(t, sender.TransferID, domain.TransferPhaseShare, receiver.ReceiverToken, []byte("x"))); reason != domain.TransferCloseMismatch {
		t.Errorf("write after close: reason = %q, want mismatch", reason)
	}
}

func TestTransferCloseRejectsUnknownReasonsAndIsIdempotent(t *testing.T) {
	s := newTransferTestServer(t)
	sender := s.open(t)
	path := "/api/v1/transfers/" + sender.TransferID

	if rec := s.do(t, http.MethodDelete, path+"?reason=because", sender.SenderToken, nil); rec.Code != http.StatusBadRequest {
		t.Errorf("unknown reason: status = %d, want 400", rec.Code)
	}
	for range 2 {
		if rec := s.do(t, http.MethodDelete, path, sender.SenderToken, nil); rec.Code != http.StatusNoContent {
			t.Errorf("close: status = %d, want 204", rec.Code)
		}
	}
}

func TestTransferExpiredIsGone(t *testing.T) {
	s := newTransferTestServer(t)
	sender := s.open(t)
	s.repo.mu.Lock()
	expired := s.repo.transfers[sender.TransferID]
	expired.ExpiresAt = time.Now().Add(-time.Second)
	s.repo.transfers[sender.TransferID] = expired
	s.repo.mu.Unlock()

	if reason := goneReason(t, s.get(t, sender.TransferID, domain.TransferPhaseShare, sender.SenderToken)); reason != domain.TransferCloseExpired {
		t.Errorf("reason = %q, want expired", reason)
	}
	if rec := s.do(t, http.MethodPost, "/api/v1/transfers/claim", "", map[string]int{"nameplate": sender.Nameplate}); rec.Code != http.StatusNotFound {
		t.Errorf("claim after expiry: status = %d, want 404", rec.Code)
	}
}

func TestTransferMessageMustBeSmallBase64URL(t *testing.T) {
	s := newTransferTestServer(t)
	sender := s.open(t)
	path := messagePath(sender.TransferID, domain.TransferPhaseShare)

	for name, body := range map[string]map[string]string{
		"empty":     {"data": ""},
		"too large": data(make([]byte, domain.TransferMessageMaxBytes+1)),
		"padded":    {"data": "YQ=="},
		"standard":  {"data": "a+/b"},
	} {
		if rec := s.do(t, http.MethodPut, path, sender.SenderToken, body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", name, rec.Code)
		}
	}
	if rec := s.do(t, http.MethodPut, path, sender.SenderToken, data(make([]byte, domain.TransferMessageMaxBytes))); rec.Code != http.StatusNoContent {
		t.Errorf("largest allowed message: status = %d, want 204", rec.Code)
	}
}

func TestTransferCreateReportsAFullRelay(t *testing.T) {
	s := newTransferTestServer(t)
	s.repo.full = true

	rec := s.do(t, http.MethodPost, "/api/v1/transfers", "", nil)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rec.Code)
	}
}

func TestTransferRoutesAreRegistered(t *testing.T) {
	repo := fullMockRepo{mockSecretRepo: newMockRepo(), uploadMockRepo: newUploadMockRepo(), transferMockRepo: newTransferMockRepo()}
	app, err := New(config.Config{}, "test", nil, repo, newUploadMockStore(), nil, prometheus.NewRegistry())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	rec := httptest.NewRecorder()

	app.echo.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/transfers", strings.NewReader("")))

	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201; body = %s", rec.Code, rec.Body)
	}
}
