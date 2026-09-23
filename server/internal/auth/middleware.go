package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

type contextKey string

const contextKeyPlayerID contextKey = "playerID"
const contextKeyUsername contextKey = "username"

// Middleware returns an HTTP middleware that validates JWT access tokens.
// Checks Authorization: Bearer header first, then the poleia_token cookie.
// Returns 401 on failure — use WebMiddleware for browser routes that should redirect.
func Middleware(svc *Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := tokenFromRequest(r)
			if token == "" {
				writeUnauthorized(w)
				return
			}
			claims, err := svc.ValidateAccessToken(token)
			if err != nil {
				writeUnauthorized(w)
				return
			}
			ctx := context.WithValue(r.Context(), contextKeyPlayerID, claims.PlayerID)
			ctx = context.WithValue(ctx, contextKeyUsername, claims.Username)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// SessionExpiredMessage is the 401 body's error text. It is written for the web
// player, where every refusal surface renders the server's own `error` string
// (formatApiError): a tab left open overnight is the normal case in a game
// built around nine-hour absences, and a plain-text 401 used to reach them as
// "error 401". keryx replaces it with its own login hint (cmd/keryx apiError).
const SessionExpiredMessage = "Your session has expired — reload the page to log in again."

func writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": SessionExpiredMessage})
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
	if c, err := r.Cookie("poleia_token"); err == nil {
		return c.Value
	}
	return ""
}

// PlayerIDFromContext extracts the authenticated player ID from a request context.
func PlayerIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(contextKeyPlayerID).(uuid.UUID)
	return id, ok
}
