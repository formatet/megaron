package handlers

import (
	"fmt"
	"net/http"
	"strings"

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

// ordersBeforeStartExempt are the state-changing routes a Wanax may still use
// while the world is forming, matched by the tail of the chi route pattern.
// Everything else that writes is refused — the list is fail-closed, so a new
// verb added to the gated group is gated without anyone remembering to.
//   - join: joining is what starts the world.
//   - reports: a bug report is about the game, not an order inside it.
//   - notifications read-all / delete: housekeeping of the Wanax's own inbox.
var ordersBeforeStartExempt = []string{
	"/worlds/{worldID}/join",
	"/worlds/{worldID}/reports",
	"/worlds/{worldID}/notifications/read-all",
	"/worlds/{worldID}/notifications",
}

// RequireStartedWorld refuses every order given to a world whose clock has
// not begun (worlds.state = 'forming', waiting for `needed` Wanaxes to join).
// Decision Timothy 2026-09-25: "No orders before the world has begun." Before
// this, orders were accepted and queued on a frozen due_tick, then all
// resolved in one burst when time started.
//
// Reads pass through (a Wanax may look around), as do the exempt routes above.
// The refusal is 409 + {"error": …} with the live N of M, which the web's
// formatApiError and keryx's apiError both print verbatim.
//
// Must be installed with r.Use inside a chi Group (an inline mux), so it runs
// after routing and the matched route pattern is known.
func RequireStartedWorld(pool *pgxpool.Pool, needed int) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
				next.ServeHTTP(w, r)
				return
			}
			if rc := chi.RouteContext(r.Context()); rc != nil {
				pattern := rc.RoutePattern()
				for _, tail := range ordersBeforeStartExempt {
					if strings.HasSuffix(pattern, tail) {
						next.ServeHTTP(w, r)
						return
					}
				}
			}
			worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
			if err != nil {
				next.ServeHTTP(w, r) // no/invalid worldID — let the handler decide
				return
			}
			var state string
			var joined int
			if err := pool.QueryRow(r.Context(),
				`SELECT w.state,
				        (SELECT count(*) FROM player_world_records p
				          WHERE p.world_id = w.id AND p.status = 'active')
				   FROM worlds w WHERE w.id = $1`, worldID,
			).Scan(&state, &joined); err != nil {
				next.ServeHTTP(w, r) // unknown world — let the handler 404
				return
			}
			if state == "forming" {
				writeError(w, http.StatusConflict, fmt.Sprintf(
					"The world has not begun — %d of %d Wanaxes have arrived. Orders can be given once it starts.",
					joined, needed))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
