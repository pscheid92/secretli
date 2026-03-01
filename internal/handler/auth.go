package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/pscheid92/secretli/internal/model"
	"github.com/pscheid92/secretli/internal/store"
	"golang.org/x/crypto/bcrypt"
)

type AuthHandler struct {
	userRepo     store.UserRepo
	sessionRepo  store.SessionRepo
	sessionMaxAge time.Duration
	cookieDomain  string
	cookieSecure  bool
}

func NewAuthHandler(userRepo store.UserRepo, sessionRepo store.SessionRepo, sessionMaxAge time.Duration, cookieDomain string, cookieSecure bool) *AuthHandler {
	return &AuthHandler{
		userRepo:      userRepo,
		sessionRepo:   sessionRepo,
		sessionMaxAge: sessionMaxAge,
		cookieDomain:  cookieDomain,
		cookieSecure:  cookieSecure,
	}
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req model.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "email and password are required")
		return
	}

	if len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), 12)
	if err != nil {
		slog.Error("failed to hash password", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	user := &model.User{
		Email:        req.Email,
		PasswordHash: string(hash),
		DisplayName:  req.DisplayName,
	}

	if err := h.userRepo.Create(r.Context(), user); err != nil {
		if errors.Is(err, store.ErrDuplicateEmail) {
			writeError(w, http.StatusConflict, "email already registered")
			return
		}
		slog.Error("failed to create user", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	sessionID, err := h.sessionRepo.Create(r.Context(), user.ID)
	if err != nil {
		slog.Error("failed to create session", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	h.setSessionCookie(w, sessionID)
	writeJSON(w, http.StatusCreated, user)
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req model.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "email and password are required")
		return
	}

	user, err := h.userRepo.GetByEmail(r.Context(), req.Email)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if err != nil {
		slog.Error("failed to get user", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	sessionID, err := h.sessionRepo.Create(r.Context(), user.ID)
	if err != nil {
		slog.Error("failed to create session", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	h.setSessionCookie(w, sessionID)
	writeJSON(w, http.StatusOK, user)
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("session_id")
	if err == nil && cookie.Value != "" {
		_ = h.sessionRepo.Delete(r.Context(), cookie.Value)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "session_id",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: http.SameSiteLaxMode,
		Domain:   h.cookieDomain,
	})

	w.WriteHeader(http.StatusNoContent)
}

func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	writeJSON(w, http.StatusOK, user)
}

func (h *AuthHandler) setSessionCookie(w http.ResponseWriter, sessionID string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "session_id",
		Value:    sessionID,
		Path:     "/",
		MaxAge:   int(h.sessionMaxAge.Seconds()),
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: http.SameSiteLaxMode,
		Domain:   h.cookieDomain,
	})
}
