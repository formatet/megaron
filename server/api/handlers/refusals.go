package handlers

// Avslagsloggen (megaron_plan_avslagslogg.md) — TILLFÄLLIG, felsökning under
// alfatestet, ingen spelfunktion (Timothy 2026-10-08). A refusal returned before
// any order exists (can't afford, wrong place, world not begun) leaves no trace in
// the game tables; this records it so tools/alfa_funnel.py can show where a
// stranger got stuck. Remove the middleware, this file, its test and add a drop
// migration if the playtest shows it is not needed.

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"formatet/megaron/server/internal/auth"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const refusalBodyPeek = 2048

type refusalRecorder struct {
	http.ResponseWriter
	status int
	body   bytes.Buffer
}

func (rr *refusalRecorder) WriteHeader(code int) {
	if rr.status == 0 {
		rr.status = code
	}
	rr.ResponseWriter.WriteHeader(code)
}

func (rr *refusalRecorder) Write(b []byte) (int, error) {
	if rr.status == 0 {
		rr.status = http.StatusOK
	}
	if room := refusalBodyPeek - rr.body.Len(); room > 0 {
		if len(b) < room {
			room = len(b)
		}
		rr.body.Write(b[:room])
	}
	return rr.ResponseWriter.Write(b)
}

// RecordRefusals sits inside the authenticated world group, right after
// auth.Middleware, so the world guards' own refusals are caught too. Only
// writes; a failure to record is logged and never changes the response.
func RecordRefusals(pool *pgxpool.Pool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			default:
				next.ServeHTTP(w, r)
				return
			}
			rr := &refusalRecorder{ResponseWriter: w}
			next.ServeHTTP(rr, r)
			if rr.status < 400 || rr.status >= 500 || rr.status == http.StatusUnauthorized {
				return
			}
			playerID, ok := auth.PlayerIDFromContext(r.Context())
			if !ok {
				return
			}
			worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
			if err != nil {
				return
			}
			route := r.URL.Path
			if rc := chi.RouteContext(r.Context()); rc != nil && rc.RoutePattern() != "" {
				route = rc.RoutePattern()
			}
			code := refusalCode(rr.body.Bytes())
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if _, err := pool.Exec(ctx,
				`INSERT INTO refusals (world_id, player_id, route, method, status, code, world_tick)
				 SELECT $1, $2, $3, $4, $5, $6, current_tick FROM worlds WHERE id = $1`,
				worldID, playerID, route, r.Method, rr.status, code,
			); err != nil {
				slog.Warn("refusal log: insert failed", "route", route, "status", rr.status, "err", err)
			}
		})
	}
}

// refusalCode prefers the machine error_code, else the error text cut to 200
// runes. It is server text; the request body is never read.
func refusalCode(body []byte) string {
	var v struct {
		Error     string `json:"error"`
		ErrorCode string `json:"error_code"`
	}
	_ = json.Unmarshal(body, &v)
	if v.ErrorCode != "" {
		return v.ErrorCode
	}
	s := []rune(v.Error)
	if len(s) > 200 {
		s = s[:200]
	}
	if len(s) == 0 {
		return "(no error text)"
	}
	return string(s)
}
