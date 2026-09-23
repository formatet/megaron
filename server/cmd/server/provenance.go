package main

import (
	"context"
	"encoding/json"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// buildCommit is stamped by the Docker build (-X main.buildCommit=…, see
// Dockerfile), where the build context has no .git and Go's own VCS stamp is
// therefore absent. Everywhere else — air on CT 126, a plain go build — Go
// embeds vcs.revision itself and commitFromBuildInfo reads it.
var buildCommit = "unknown"

// commitFromBuildInfo returns the commit a binary was built from: the
// ldflags stamp if set, else Go's vcs.revision (short, "+dirty" when built
// from a modified tree), else "unknown". Never guessed — an "unknown" in a
// provenance line is honest; a plausible-looking hash that isn't the running
// code is exactly how the 2026-08-30 soak measured the wrong binary
// (megaron_plan_riggens_harkomst.md).
func commitFromBuildInfo(stamped string, info *debug.BuildInfo, ok bool) string {
	if stamped != "" && stamped != "unknown" {
		return stamped
	}
	if !ok || info == nil {
		return "unknown"
	}
	var rev string
	var dirty bool
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if rev == "" {
		return "unknown"
	}
	if len(rev) > 7 {
		rev = rev[:7]
	}
	if dirty {
		rev += "+dirty"
	}
	return rev
}

// healthz is the liveness/readiness probe for deploy verification and
// monitoring. Public, no auth. It pings the DB with a short deadline so a 200
// means the server can actually serve, and it states WHAT is serving: the
// commit this process was built from and the migration version its database
// is on. A healthz that only says "ok" answered 200 from the old process
// after a deploy more than once; commit + migration make that visible.
//
// migration is null (never a guessed number) if schema_migrations can't be
// read; migration_dirty is true only when golang-migrate left the flag set.
func healthz(pool *pgxpool.Pool, commit string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		w.Header().Set("Content-Type", "application/json")
		if err := pool.Ping(ctx); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "unavailable", "commit": commit})
			return
		}
		body := map[string]any{"status": "ok", "commit": commit, "migration": nil}
		var version int
		var dirty bool
		if err := pool.QueryRow(ctx, `SELECT version, dirty FROM schema_migrations`).Scan(&version, &dirty); err == nil {
			body["migration"] = version
			if dirty {
				body["migration_dirty"] = true
			}
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(body)
	}
}
