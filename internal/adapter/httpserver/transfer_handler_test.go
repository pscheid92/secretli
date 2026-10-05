package httpserver

import (
	"bytes"
	"context"
	"crypto/rand"
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
// smallest free nameplate, one claim, legs written once in order.
type transferMockRepo struct {
	mu        sync.Mutex
	transfers map[string]domain.Transfer
	full      bool
}

func newTransferMockRepo() *transferMockRepo {
	return &transferMockRepo{transfers: map[string]domain.Transfer{}}
}

func (m *transferMockRepo) active(t domain.Transfer, now time.Time) bool {
	return t.ClosedAt == nil && now.Before(t.ExpiresAt)
}

func (m *transferMockRepo) CreateTransfer(_ context.Context, t *domain.Transfer, maxNameplate int, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.full {
		return domain.ErrConflict
	}
	if _, taken := m.transfers[t.TransferID]; taken {
		return domain.ErrDuplicate
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
		if t.Claimed() {
			return nil, domain.ErrConflict
		}
		t.ReceiverTokenHash = receiverTokenHash
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

func (m *transferMockRepo) AnswerTransfer(_ context.Context, transferID string, share, confirmation []byte, now time.Time) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.transfers[transferID]
	if !ok || !m.active(t, now) || !t.Claimed() || t.Answered() {
		return false, nil
	}
	t.AnswerShare, t.AnswerConfirmation = share, confirmation
	m.transfers[transferID] = t
	return true, nil
}

func (m *transferMockRepo) DeliverTransfer(_ context.Context, transferID string, delivery []byte, now time.Time) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.transfers[transferID]
	if !ok || !m.active(t, now) || !t.Answered() {
		return false, nil
	}
	t.Delivery = delivery
	t.CloseReason = domain.TransferCloseDone
	t.ClosedAt = &now
	m.transfers[transferID] = t
	return true, nil
}

func (m *transferMockRepo) CloseTransfer(_ context.Context, transferID, reason string, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.transfers[transferID]
	if !ok || t.ClosedAt != nil {
		return domain.ErrNotFound
	}
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

// expire makes the transfer run out after d.
func (m *transferMockRepo) expire(transferID string, d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t := m.transfers[transferID]
	t.ExpiresAt = time.Now().Add(d)
	m.transfers[transferID] = t
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
	e.POST("/api/v1/transfers/:transferID/answer", h.PostAnswer)
	e.GET("/api/v1/transfers/:transferID/answer", h.AwaitAnswer)
	e.POST("/api/v1/transfers/:transferID/delivery", h.PostDelivery)
	e.GET("/api/v1/transfers/:transferID/delivery", h.AwaitDelivery)
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

func b64(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

// filled returns n bytes of value, standing in for a share, tag or sealed link.
func filled(n int, value byte) []byte {
	return bytes.Repeat([]byte{value}, n)
}

func newTransferID(t *testing.T) string {
	t.Helper()
	id := make([]byte, 32)
	if _, err := rand.Read(id); err != nil {
		t.Fatalf("random transfer id: %v", err)
	}
	return b64(id)
}

type openedTransfer struct {
	TransferID  string
	Nameplate   int    `json:"nameplate"`
	SenderToken string `json:"sender_token"`
}

type claimedTransfer struct {
	TransferID    string `json:"transfer_id"`
	ReceiverToken string `json:"receiver_token"`
	Offer         string `json:"offer"`
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode %s: %v", rec.Body.String(), err)
	}
	return v
}

func (s *transferTestServer) create(t *testing.T, transferID string, offer []byte) *httptest.ResponseRecorder {
	t.Helper()
	return s.do(t, http.MethodPost, "/api/v1/transfers", "", map[string]string{"transfer_id": transferID, "offer": b64(offer)})
}

func (s *transferTestServer) open(t *testing.T) openedTransfer {
	t.Helper()
	id := newTransferID(t)
	rec := s.create(t, id, filled(domain.TransferShareBytes, 'a'))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: status = %d, body = %s", rec.Code, rec.Body)
	}
	opened := decode[openedTransfer](t, rec)
	opened.TransferID = id
	return opened
}

func (s *transferTestServer) claim(t *testing.T, nameplate int) claimedTransfer {
	t.Helper()
	rec := s.do(t, http.MethodPost, "/api/v1/transfers/claim", "", map[string]int{"nameplate": nameplate})
	if rec.Code != http.StatusOK {
		t.Fatalf("claim: status = %d, body = %s", rec.Code, rec.Body)
	}
	return decode[claimedTransfer](t, rec)
}

func legPath(transferID, leg string) string {
	return fmt.Sprintf("/api/v1/transfers/%s/%s", transferID, leg)
}

func (s *transferTestServer) answer(t *testing.T, transferID, token string, share, confirmation []byte) *httptest.ResponseRecorder {
	t.Helper()
	return s.do(t, http.MethodPost, legPath(transferID, "answer"), token,
		map[string]string{"share": b64(share), "confirmation": b64(confirmation)})
}

func (s *transferTestServer) awaitAnswer(t *testing.T, transferID, token string) *httptest.ResponseRecorder {
	t.Helper()
	return s.do(t, http.MethodGet, legPath(transferID, "answer"), token, nil)
}

func (s *transferTestServer) deliver(t *testing.T, transferID, token string, sealed []byte) *httptest.ResponseRecorder {
	t.Helper()
	return s.do(t, http.MethodPost, legPath(transferID, "delivery"), token, map[string]string{"sealed": b64(sealed)})
}

func (s *transferTestServer) awaitDelivery(t *testing.T, transferID, token string) *httptest.ResponseRecorder {
	t.Helper()
	return s.do(t, http.MethodGet, legPath(transferID, "delivery"), token, nil)
}

func (s *transferTestServer) close(t *testing.T, transferID, token, reason string) *httptest.ResponseRecorder {
	t.Helper()
	return s.do(t, http.MethodDelete, "/api/v1/transfers/"+transferID+"?reason="+reason, token, nil)
}

// field decodes one base64url field of a 200 response.
func field(t *testing.T, rec *httptest.ResponseRecorder, name string) []byte {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body)
	}
	b, err := base64.RawURLEncoding.DecodeString(decode[map[string]string](t, rec)[name])
	if err != nil {
		t.Fatalf("decode %s: %v", name, err)
	}
	return b
}

func expectStatus(t *testing.T, what string, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Errorf("%s: status = %d, want %d; body = %s", what, rec.Code, want, rec.Body)
	}
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

var (
	answerShare  = filled(domain.TransferShareBytes, 'b')
	answerTag    = filled(domain.TransferConfirmationBytes, 't')
	sealedLink   = filled(domain.TransferDeliveryBytes, 's')
	anotherValue = filled(domain.TransferShareBytes, 'x')
)

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
	if receiver.Offer != b64(filled(domain.TransferShareBytes, 'a')) {
		t.Errorf("claim returned offer %q, want the sender's", receiver.Offer)
	}
	id := sender.TransferID

	expectStatus(t, "answer", s.answer(t, id, receiver.ReceiverToken, answerShare, answerTag), http.StatusNoContent)
	answered := s.awaitAnswer(t, id, sender.SenderToken)
	if got := field(t, answered, "share"); !bytes.Equal(got, answerShare) {
		t.Errorf("answer share = %q", got)
	}
	expectStatus(t, "deliver", s.deliver(t, id, sender.SenderToken, sealedLink), http.StatusNoContent)

	// Delivering closed the transfer as done; the link is still handed out.
	if got := field(t, s.awaitDelivery(t, id, receiver.ReceiverToken), "sealed"); !bytes.Equal(got, sealedLink) {
		t.Errorf("delivery = %q", got)
	}
	if stored := s.repo.transfers[id]; stored.CloseReason != domain.TransferCloseDone || stored.ClosedAt == nil {
		t.Errorf("transfer closed as %q at %v, want done", stored.CloseReason, stored.ClosedAt)
	}
}

func TestTransferNameplatesAreReusedOnceATransferEnds(t *testing.T) {
	s := newTransferTestServer(t)
	first := s.open(t)
	second := s.open(t)
	if second.Nameplate != 2 {
		t.Fatalf("second nameplate = %d, want 2", second.Nameplate)
	}

	expectStatus(t, "close", s.close(t, first.TransferID, first.SenderToken, domain.TransferCloseCancelled), http.StatusNoContent)

	if third := s.open(t); third.Nameplate != 1 {
		t.Errorf("nameplate after close = %d, want 1 again", third.Nameplate)
	}
}

func TestTransferCreateChecksTheIdAndTheOffer(t *testing.T) {
	s := newTransferTestServer(t)
	offer := filled(domain.TransferShareBytes, 'a')

	expectStatus(t, "malformed id", s.create(t, "not-a-transfer-id", offer), http.StatusBadRequest)
	expectStatus(t, "short offer", s.create(t, newTransferID(t), offer[:31]), http.StatusBadRequest)

	id := newTransferID(t)
	expectStatus(t, "create", s.create(t, id, offer), http.StatusCreated)
	expectStatus(t, "reused id", s.create(t, id, offer), http.StatusConflict)
}

func TestTransferCanBeClaimedOnlyOnce(t *testing.T) {
	s := newTransferTestServer(t)
	sender := s.open(t)
	s.claim(t, sender.Nameplate)

	rec := s.do(t, http.MethodPost, "/api/v1/transfers/claim", "", map[string]int{"nameplate": sender.Nameplate})

	expectStatus(t, "second claim", rec, http.StatusConflict)
}

func TestTransferClaimRejectsUnknownAndInvalidNameplates(t *testing.T) {
	s := newTransferTestServer(t)

	for nameplate, want := range map[int]int{7: http.StatusNotFound, 0: http.StatusBadRequest, 1000: http.StatusBadRequest} {
		rec := s.do(t, http.MethodPost, "/api/v1/transfers/claim", "", map[string]int{"nameplate": nameplate})
		expectStatus(t, fmt.Sprintf("nameplate %d", nameplate), rec, want)
	}
}

func TestTransferLegsAreWrittenOnceAndIdenticalRetriesSucceed(t *testing.T) {
	s := newTransferTestServer(t)
	sender := s.open(t)
	receiver := s.claim(t, sender.Nameplate)
	id := sender.TransferID

	expectStatus(t, "answer", s.answer(t, id, receiver.ReceiverToken, answerShare, answerTag), http.StatusNoContent)
	expectStatus(t, "same answer again", s.answer(t, id, receiver.ReceiverToken, answerShare, answerTag), http.StatusNoContent)
	expectStatus(t, "other answer", s.answer(t, id, receiver.ReceiverToken, anotherValue, answerTag), http.StatusConflict)

	expectStatus(t, "deliver", s.deliver(t, id, sender.SenderToken, sealedLink), http.StatusNoContent)
	// The first delivery closed the transfer; a retry of it still succeeds.
	expectStatus(t, "same delivery again", s.deliver(t, id, sender.SenderToken, sealedLink), http.StatusNoContent)
	expectStatus(t, "other delivery", s.deliver(t, id, sender.SenderToken, filled(domain.TransferDeliveryBytes, 'z')), http.StatusConflict)
}

func TestTransferEnforcesWhoDoesWhichLeg(t *testing.T) {
	s := newTransferTestServer(t)
	sender := s.open(t)
	receiver := s.claim(t, sender.Nameplate)
	id := sender.TransferID

	expectStatus(t, "sender answers", s.answer(t, id, sender.SenderToken, answerShare, answerTag), http.StatusForbidden)
	expectStatus(t, "receiver awaits the answer", s.awaitAnswer(t, id, receiver.ReceiverToken), http.StatusForbidden)
	expectStatus(t, "receiver delivers", s.deliver(t, id, receiver.ReceiverToken, sealedLink), http.StatusForbidden)
	expectStatus(t, "sender awaits the delivery", s.awaitDelivery(t, id, sender.SenderToken), http.StatusForbidden)
}

func TestTransferDeliveryNeedsTheAnswerFirst(t *testing.T) {
	s := newTransferTestServer(t)
	sender := s.open(t)
	s.claim(t, sender.Nameplate)

	expectStatus(t, "deliver before the answer", s.deliver(t, sender.TransferID, sender.SenderToken, sealedLink), http.StatusConflict)
}

func TestTransferRejectsForeignAndMalformedTokens(t *testing.T) {
	s := newTransferTestServer(t)
	sender := s.open(t)
	other := s.open(t)

	expectStatus(t, "other transfer's token", s.awaitAnswer(t, sender.TransferID, other.SenderToken), http.StatusForbidden)
	expectStatus(t, "malformed transfer id", s.awaitAnswer(t, "not-a-transfer", sender.SenderToken), http.StatusBadRequest)
	expectStatus(t, "no token", s.awaitAnswer(t, sender.TransferID, ""), http.StatusBadRequest)
}

func TestTransferValuesMustHaveTheirExactSize(t *testing.T) {
	s := newTransferTestServer(t)
	sender := s.open(t)
	receiver := s.claim(t, sender.Nameplate)
	id := sender.TransferID

	expectStatus(t, "short share", s.answer(t, id, receiver.ReceiverToken, answerShare[:31], answerTag), http.StatusBadRequest)
	expectStatus(t, "long confirmation", s.answer(t, id, receiver.ReceiverToken, answerShare, append(answerTag, 0)), http.StatusBadRequest)
	padded := s.do(t, http.MethodPost, legPath(id, "answer"), receiver.ReceiverToken,
		map[string]string{"share": b64(answerShare) + "=", "confirmation": b64(answerTag)})
	expectStatus(t, "padded base64", padded, http.StatusBadRequest)

	expectStatus(t, "answer", s.answer(t, id, receiver.ReceiverToken, answerShare, answerTag), http.StatusNoContent)
	expectStatus(t, "short delivery", s.deliver(t, id, sender.SenderToken, sealedLink[:551]), http.StatusBadRequest)
}

func TestTransferLongPollReturnsOnceTheOtherSideWrites(t *testing.T) {
	s := newTransferTestServer(t)
	sender := s.open(t)
	receiver := s.claim(t, sender.Nameplate)

	go func() {
		time.Sleep(50 * time.Millisecond)
		s.answer(t, sender.TransferID, receiver.ReceiverToken, answerShare, answerTag)
	}()

	if got := field(t, s.awaitAnswer(t, sender.TransferID, sender.SenderToken), "confirmation"); !bytes.Equal(got, answerTag) {
		t.Errorf("confirmation = %q, want the answer written while polling", got)
	}
}

func TestTransferLongPollWakesOnANotification(t *testing.T) {
	events := &fakeTransferEvents{}
	s := newTransferTestServerWith(t, events, 20*time.Second, 10*time.Second)
	sender := s.open(t)
	receiver := s.claim(t, sender.Nameplate)

	go func() {
		time.Sleep(50 * time.Millisecond)
		s.answer(t, sender.TransferID, receiver.ReceiverToken, answerShare, answerTag)
		events.signal(sender.TransferID)
	}()

	start := time.Now()
	if got := field(t, s.awaitAnswer(t, sender.TransferID, sender.SenderToken), "share"); !bytes.Equal(got, answerShare) {
		t.Errorf("share = %q, want the one that was signalled", got)
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
	expectStatus(t, "answer", s.answer(t, sender.TransferID, receiver.ReceiverToken, answerShare, answerTag), http.StatusNoContent)

	go func() {
		time.Sleep(50 * time.Millisecond)
		// Written without a signal, as while the listener reconnects.
		s.deliver(t, sender.TransferID, sender.SenderToken, sealedLink)
	}()

	start := time.Now()
	if got := field(t, s.awaitDelivery(t, sender.TransferID, receiver.ReceiverToken), "sealed"); !bytes.Equal(got, sealedLink) {
		t.Errorf("delivery = %q, want the one found by the re-check", got)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("answered after %v, want the re-check to find it", elapsed)
	}
}

func TestTransferLongPollEndsWhenTheTransferExpires(t *testing.T) {
	events := &fakeTransferEvents{}
	s := newTransferTestServerWith(t, events, 20*time.Second, 10*time.Second)
	sender := s.open(t)
	s.repo.expire(sender.TransferID, 100*time.Millisecond)

	start := time.Now()
	rec := s.awaitAnswer(t, sender.TransferID, sender.SenderToken)

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
	rec := s.awaitAnswer(t, sender.TransferID, sender.SenderToken)

	expectStatus(t, "empty poll", rec, http.StatusNoContent)
	if elapsed := time.Since(start); elapsed < 300*time.Millisecond {
		t.Errorf("returned after %v, want it to wait the poll window", elapsed)
	}
}

func TestTransferClosedTellsTheOtherSideWhy(t *testing.T) {
	s := newTransferTestServer(t)
	sender := s.open(t)
	receiver := s.claim(t, sender.Nameplate)

	expectStatus(t, "close", s.close(t, sender.TransferID, sender.SenderToken, domain.TransferCloseMismatch), http.StatusNoContent)

	if reason := goneReason(t, s.awaitDelivery(t, sender.TransferID, receiver.ReceiverToken)); reason != domain.TransferCloseMismatch {
		t.Errorf("waiting receiver: reason = %q, want mismatch", reason)
	}
	if reason := goneReason(t, s.answer(t, sender.TransferID, receiver.ReceiverToken, answerShare, answerTag)); reason != domain.TransferCloseMismatch {
		t.Errorf("answer after close: reason = %q, want mismatch", reason)
	}
}

func TestTransferCloseTakesOnlyCancelOrMismatchAndIsIdempotent(t *testing.T) {
	s := newTransferTestServer(t)
	sender := s.open(t)

	expectStatus(t, "close as done", s.close(t, sender.TransferID, sender.SenderToken, domain.TransferCloseDone), http.StatusBadRequest)
	expectStatus(t, "unknown reason", s.close(t, sender.TransferID, sender.SenderToken, "bored"), http.StatusBadRequest)
	expectStatus(t, "close", s.close(t, sender.TransferID, sender.SenderToken, domain.TransferCloseCancelled), http.StatusNoContent)
	expectStatus(t, "close again", s.close(t, sender.TransferID, sender.SenderToken, domain.TransferCloseCancelled), http.StatusNoContent)
}

func TestTransferExpiredIsGone(t *testing.T) {
	s := newTransferTestServer(t)
	sender := s.open(t)
	receiver := s.claim(t, sender.Nameplate)
	s.repo.expire(sender.TransferID, -time.Second)

	if reason := goneReason(t, s.awaitAnswer(t, sender.TransferID, sender.SenderToken)); reason != domain.TransferCloseExpired {
		t.Errorf("poll: reason = %q, want expired", reason)
	}
	if reason := goneReason(t, s.answer(t, sender.TransferID, receiver.ReceiverToken, answerShare, answerTag)); reason != domain.TransferCloseExpired {
		t.Errorf("answer: reason = %q, want expired", reason)
	}
}

func TestTransferCreateReportsAFullRelay(t *testing.T) {
	s := newTransferTestServer(t)
	s.repo.full = true

	expectStatus(t, "create", s.create(t, newTransferID(t), filled(domain.TransferShareBytes, 'a')), http.StatusServiceUnavailable)
}

func TestTransferRoutesAreRegistered(t *testing.T) {
	repo := fullMockRepo{mockSecretRepo: newMockRepo(), uploadMockRepo: newUploadMockRepo(), transferMockRepo: newTransferMockRepo()}
	app, err := New(config.Config{}, "test", nil, repo, newUploadMockStore(), nil, prometheus.NewRegistry())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	body := fmt.Sprintf(`{"transfer_id":%q,"offer":%q}`, newTransferID(t), b64(filled(domain.TransferShareBytes, 'a')))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/transfers", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	app.echo.ServeHTTP(rec, req)

	expectStatus(t, "create", rec, http.StatusCreated)
}
