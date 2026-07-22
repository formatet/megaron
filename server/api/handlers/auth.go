package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"formatet/megaron/server/internal/auth"
)

// AuthHandler handles HTTP requests for auth endpoints.
type AuthHandler struct {
	svc *auth.Service
}

// NewAuthHandler creates an AuthHandler.
func NewAuthHandler(svc *auth.Service) *AuthHandler {
	return &AuthHandler{svc: svc}
}

type registerRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

func setTokenCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "poleia_token",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(15 * time.Minute),
	})
}

// Register handles POST /auth/register.
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Username == "" || req.Email == "" {
		writeError(w, http.StatusBadRequest, "username and email required")
		return
	}
	access, refresh, err := h.svc.Register(r.Context(), req.Username, req.Email, req.Password)
	if errors.Is(err, auth.ErrUserExists) {
		writeError(w, http.StatusConflict, "username or email already registered")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "registration failed")
		return
	}
	setTokenCookie(w, access)
	writeJSON(w, http.StatusCreated, tokenResponse{AccessToken: access, RefreshToken: refresh})
}

// Login handles POST /auth/login.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UsernameOrEmail string `json:"username_or_email"`
		Password        string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	access, refresh, err := h.svc.Login(r.Context(), req.UsernameOrEmail, req.Password)
	if errors.Is(err, auth.ErrUserNotFound) || errors.Is(err, auth.ErrInvalidPassword) {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "login failed")
		return
	}
	setTokenCookie(w, access)
	writeJSON(w, http.StatusOK, tokenResponse{AccessToken: access, RefreshToken: refresh})
}

// Refresh handles POST /auth/refresh.
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	access, refresh, err := h.svc.Refresh(r.Context(), req.RefreshToken)
	if errors.Is(err, auth.ErrInvalidToken) {
		writeError(w, http.StatusUnauthorized, "invalid or expired refresh token")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "refresh failed")
		return
	}
	writeJSON(w, http.StatusOK, tokenResponse{AccessToken: access, RefreshToken: refresh})
}

// Me handles GET /auth/me.
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	playerID, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	p, err := h.svc.Me(r.Context(), playerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load player")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":        p.ID,
		"username":  p.Username,
		"email":     p.Email,
		"era_count": p.EraCount,
		"created_at": p.CreatedAt,
	})
}
