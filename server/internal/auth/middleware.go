package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

type contextKey string

const contextKeyPlayerID contextKey = "playerID"
const contextKeyUsername contextKey = "username"

// Middleware returns an HTTP middleware that validates JWT access tokens.
// Checks Authorization: Bearer header first, then the thalassa_token cookie.
// Returns 401 on failure — use WebMiddleware for browser routes that should redirect.
func Middleware(svc *Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := tokenFromRequest(r)
			if token == "" {
				http.Error(w, "missing or malformed token", http.StatusUnauthorized)
				return
			}
			claims, err := svc.ValidateAccessToken(token)
			if err != nil {
				http.Error(w, "invalid token", http.StatusUnauthorized)
				return
			}
			ctx := context.WithValue(r.Context(), contextKeyPlayerID, claims.PlayerID)
			ctx = context.WithValue(ctx, contextKeyUsername, claims.Username)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// WebMiddleware is like Middleware but redirects to / on failure instead of 401.
// Use this for browser-facing HTML routes.
func WebMiddleware(svc *Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := tokenFromRequest(r)
			if token != "" {
				if claims, err := svc.ValidateAccessToken(token); err == nil {
					ctx := context.WithValue(r.Context(), contextKeyPlayerID, claims.PlayerID)
					ctx = context.WithValue(ctx, contextKeyUsername, claims.Username)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}
			http.Redirect(w, r, "/", http.StatusSeeOther)
		})
	}
}

// OptionalMiddleware is like Middleware but never fails — it silently sets context
// if the token is valid, and passes through unauthenticated requests unchanged.
// Use for endpoints that serve both authenticated and anonymous clients (e.g. the map).
func OptionalMiddleware(svc *Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if token := tokenFromRequest(r); token != "" {
				if claims, err := svc.ValidateAccessToken(token); err == nil {
					ctx := context.WithValue(r.Context(), contextKeyPlayerID, claims.PlayerID)
					ctx = context.WithValue(ctx, contextKeyUsername, claims.Username)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// tokenFromRequest extracts the JWT from Authorization header or cookie.
func tokenFromRequest(r *http.Request) string {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	if c, err := r.Cookie("thalassa_token"); err == nil {
		return c.Value
	}
	return ""
}

// PlayerIDFromContext extracts the authenticated player ID from a request context.
func PlayerIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(contextKeyPlayerID).(uuid.UUID)
	return id, ok
}

// UsernameFromContext extracts the authenticated username from a request context.
func UsernameFromContext(ctx context.Context) (string, bool) {
	name, ok := ctx.Value(contextKeyUsername).(string)
	return name, ok
}
