package handlers

import (
	"net/http"

	"formatet/megaron/server/internal/auth"
	"github.com/google/uuid"
)

// asPlayer mounts h as if playerID were signed in. A city's own body is only
// served to its ruler (megaron_kontrakt_frammande_stad), so tests reading
// the owner's body read it as the owner.
func asPlayer(playerID uuid.UUID, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h(w, r.WithContext(auth.ContextWithPlayer(r.Context(), playerID)))
	}
}
