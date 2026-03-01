package handler

import (
	"net/http"
	"strconv"

	"github.com/pscheid92/secretli/internal/store"
)

type UserHandler struct {
	userSecretRepo store.UserSecretRepo
}

func NewUserHandler(userSecretRepo store.UserSecretRepo) *UserHandler {
	return &UserHandler{userSecretRepo: userSecretRepo}
}

func (h *UserHandler) ListSecrets(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	page := 1
	perPage := 20
	if v := r.URL.Query().Get("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			page = n
		}
	}
	if v := r.URL.Query().Get("per_page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			perPage = n
		}
	}

	secrets, total, err := h.userSecretRepo.ListByUser(r.Context(), user.ID, page, perPage)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"secrets":  secrets,
		"page":     page,
		"per_page": perPage,
		"total":    total,
	})
}
