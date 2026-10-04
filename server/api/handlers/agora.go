package handlers

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"formatet/megaron/server/internal/auth"
	"github.com/google/uuid"
)

var ErrAgoraPending = errors.New("Your chat account is not ready yet; try again later")

type AgoraAccount struct {
	Enabled    bool   `json:"enabled"`
	State      string `json:"state,omitempty"`
	Localpart  string `json:"localpart,omitempty"`
	Homeserver string `json:"homeserver,omitempty"`
	UserID     string `json:"user_id,omitempty"`
}

type AgoraPassword struct {
	Password   string `json:"password"`
	UserID     string `json:"user_id"`
	Homeserver string `json:"homeserver"`
}

// AgoraService is owned by its HTTP consumer; cmd/server supplies orchestration.
type AgoraService interface {
	Account(context.Context, uuid.UUID) (AgoraAccount, error)
	Password(context.Context, uuid.UUID) (AgoraPassword, error)
}

type AgoraHandler struct{ service AgoraService }

func NewAgoraHandler(service AgoraService) *AgoraHandler { return &AgoraHandler{service: service} }

// RequireAgoraBearer enforces G3 for these account routes even when older auth
// middleware accepts a navigation cookie as a fallback.
func RequireAgoraBearer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") || strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")) == "" {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *AgoraHandler) Get(w http.ResponseWriter, r *http.Request) {
	playerID, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.service == nil {
		writeJSON(w, http.StatusOK, AgoraAccount{Enabled: false})
		return
	}
	account, err := h.service.Account(r.Context(), playerID)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "Community chat is unavailable; try again later")
		return
	}
	writeJSON(w, http.StatusOK, account)
}

func (h *AgoraHandler) Password(w http.ResponseWriter, r *http.Request) {
	// Every outcome is private, including errors during upstream reset.
	w.Header().Set("Cache-Control", "no-store")
	playerID, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.service == nil {
		writeError(w, http.StatusServiceUnavailable, "Community chat is unavailable")
		return
	}
	password, err := h.service.Password(r.Context(), playerID)
	if errors.Is(err, ErrAgoraPending) {
		writeError(w, http.StatusConflict, ErrAgoraPending.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "Community chat is unavailable; try again later")
		return
	}
	writeJSON(w, http.StatusOK, password)
}
