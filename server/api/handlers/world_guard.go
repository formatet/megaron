package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RequireActiveWorld is middleware that rejects state-changing requests aimed at
// a non-active (archived) world. Only one world is ever status='active' (the
// single live world — see migration 063 + the one_active_world unique index);
// the timed-event worker only processes that world (scheduler.go). A client left
// pointing at a stale/archived world id would otherwise have its writes accepted
// but never ticked (marches stuck 'marching', builds never completing) — the
// orphaned-march class of bug. We fail those writes loudly instead.
//
// Reads (GET/HEAD/OPTIONS) pass through: archived worlds stay viewable for
// history/spectating. Requests without a parseable worldID, or for an unknown
// world, fall through to the handler (which returns its own 400/404).
func RequireActiveWorld(pool *pgxpool.Pool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
				next.ServeHTTP(w, r)
				return
			}
			worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
			if err != nil {
				next.ServeHTTP(w, r) // no/invalid worldID — let the handler decide
				return
			}
			var status string
			if err := pool.QueryRow(r.Context(),
				`SELECT status FROM worlds WHERE id = $1`, worldID,
			).Scan(&status); err != nil {
				next.ServeHTTP(w, r) // unknown world — let the handler 404
				return
			}
			if status != "active" {
				writeError(w, http.StatusConflict,
					"this world is archived — a newer world is live; point your client at the active world (GET /api/v1/worlds shows it)")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
