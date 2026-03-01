package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pscheid92/secretli/internal/model"
	"github.com/pscheid92/secretli/internal/store"
	"golang.org/x/crypto/bcrypt"
)

// --- Mock UserRepo ---

type mockUserRepo struct {
	users    map[string]*model.User
	nextID   int64
	createFn func(*model.User) error
}

func newMockUserRepo() *mockUserRepo {
	return &mockUserRepo{users: make(map[string]*model.User), nextID: 1}
}

func (m *mockUserRepo) Create(_ context.Context, user *model.User) error {
	if m.createFn != nil {
		return m.createFn(user)
	}
	if _, exists := m.users[user.Email]; exists {
		return store.ErrDuplicateEmail
	}
	user.ID = m.nextID
	m.nextID++
	user.CreatedAt = time.Now()
	user.UpdatedAt = time.Now()
	m.users[user.Email] = user
	return nil
}

func (m *mockUserRepo) GetByEmail(_ context.Context, email string) (*model.User, error) {
	u, ok := m.users[email]
	if !ok {
		return nil, store.ErrNotFound
	}
	return u, nil
}

func (m *mockUserRepo) GetByID(_ context.Context, id int64) (*model.User, error) {
	for _, u := range m.users {
		if u.ID == id {
			return u, nil
		}
	}
	return nil, store.ErrNotFound
}

// --- Mock SessionRepo ---

type mockSessionRepo struct {
	sessions map[string]int64 // sessionID -> userID
	userRepo *mockUserRepo
	nextSess int
}

func newMockSessionRepo(userRepo *mockUserRepo) *mockSessionRepo {
	return &mockSessionRepo{sessions: make(map[string]int64), userRepo: userRepo, nextSess: 1}
}

func (m *mockSessionRepo) Create(_ context.Context, userID int64) (string, error) {
	id := "session-" + string(rune('0'+m.nextSess))
	m.nextSess++
	m.sessions[id] = userID
	return id, nil
}

func (m *mockSessionRepo) GetByIDWithUser(_ context.Context, sessionID string) (*model.User, error) {
	userID, ok := m.sessions[sessionID]
	if !ok {
		return nil, store.ErrNotFound
	}
	return m.userRepo.GetByID(context.Background(), userID)
}

func (m *mockSessionRepo) Delete(_ context.Context, sessionID string) error {
	delete(m.sessions, sessionID)
	return nil
}

func (m *mockSessionRepo) DeleteExpiredSessions(_ context.Context) (int64, error) {
	return 0, nil
}

// --- Tests ---

func newTestAuthHandler() (*AuthHandler, *mockUserRepo, *mockSessionRepo) {
	ur := newMockUserRepo()
	sr := newMockSessionRepo(ur)
	ah := NewAuthHandler(ur, sr, 24*time.Hour, "", false)
	return ah, ur, sr
}

func TestRegister_Success(t *testing.T) {
	ah, _, _ := newTestAuthHandler()

	body, _ := json.Marshal(map[string]string{
		"email":        "test@example.com",
		"password":     "password123",
		"display_name": "Test User",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	ah.Register(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d. body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["email"] != "test@example.com" {
		t.Errorf("email = %v, want test@example.com", resp["email"])
	}

	// Should set session cookie
	cookies := rec.Result().Cookies()
	found := false
	for _, c := range cookies {
		if c.Name == "session_id" && c.Value != "" {
			found = true
		}
	}
	if !found {
		t.Error("expected session_id cookie to be set")
	}
}

func TestRegister_DuplicateEmail(t *testing.T) {
	ah, ur, _ := newTestAuthHandler()

	hash, _ := bcrypt.GenerateFromPassword([]byte("password123"), 12)
	ur.users["test@example.com"] = &model.User{
		ID: 1, Email: "test@example.com", PasswordHash: string(hash),
	}

	body, _ := json.Marshal(map[string]string{
		"email":    "test@example.com",
		"password": "password123",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	ah.Register(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusConflict)
	}
}

func TestRegister_MissingFields(t *testing.T) {
	ah, _, _ := newTestAuthHandler()

	body, _ := json.Marshal(map[string]string{"email": "test@example.com"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	ah.Register(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestRegister_ShortPassword(t *testing.T) {
	ah, _, _ := newTestAuthHandler()

	body, _ := json.Marshal(map[string]string{
		"email":    "test@example.com",
		"password": "short",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	ah.Register(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestLogin_Success(t *testing.T) {
	ah, ur, _ := newTestAuthHandler()

	hash, _ := bcrypt.GenerateFromPassword([]byte("password123"), 12)
	ur.users["test@example.com"] = &model.User{
		ID: 1, Email: "test@example.com", PasswordHash: string(hash),
	}

	body, _ := json.Marshal(map[string]string{
		"email":    "test@example.com",
		"password": "password123",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	ah.Login(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d. body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	ah, ur, _ := newTestAuthHandler()

	hash, _ := bcrypt.GenerateFromPassword([]byte("password123"), 12)
	ur.users["test@example.com"] = &model.User{
		ID: 1, Email: "test@example.com", PasswordHash: string(hash),
	}

	body, _ := json.Marshal(map[string]string{
		"email":    "test@example.com",
		"password": "wrongpassword",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	ah.Login(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestLogin_WrongEmail(t *testing.T) {
	ah, _, _ := newTestAuthHandler()

	body, _ := json.Marshal(map[string]string{
		"email":    "nonexistent@example.com",
		"password": "password123",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	ah.Login(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestLogout(t *testing.T) {
	ah, _, sr := newTestAuthHandler()
	sr.sessions["test-session"] = 1

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "test-session"})
	rec := httptest.NewRecorder()

	ah.Logout(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}

	if _, ok := sr.sessions["test-session"]; ok {
		t.Error("session should have been deleted")
	}
}

func TestMe_Authenticated(t *testing.T) {
	ah, _, _ := newTestAuthHandler()

	user := &model.User{ID: 1, Email: "test@example.com", DisplayName: "Test"}
	ctx := ContextWithUser(context.Background(), user)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	ah.Me(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["email"] != "test@example.com" {
		t.Errorf("email = %v, want test@example.com", resp["email"])
	}
}

func TestMe_Unauthenticated(t *testing.T) {
	ah, _, _ := newTestAuthHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	rec := httptest.NewRecorder()

	ah.Me(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}
