package handlers

// B2 (Timothy 2026-08-05, megaron_mvp_mandag.md §B2): join's occupancy count
// only summed DISTINCT owner_id over settlements. A player in the founder
// phase (a wandering host that has not founded a metropolis yet) has no
// settlement row, so a world could fill past max_provinces entirely with
// hosts still looking for a site — the cap didn't hold. The 409 also claimed
// "you are queued", which was false: no queue exists (that's D2, post-MVP).
//
// Red-before (join.go unmodified): a lone active founder_phase row doesn't
// move playerCount off 0, so a cap of 1 lets a second joiner straight through;
// and the 409 body (once the cap does trip) reads "world is full — you are
// queued".

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
)

// TestJoin_FounderPhaseCountsTowardCap pins that a wandering host (active
// founder_phase, no settlement) occupies a place in max_provinces.
func TestJoin_FounderPhaseCountsTowardCap(t *testing.T) {
	pool := citiesTestPool(t)
	ctx := context.Background()
	worldID := seedJoinableWorld(t, pool)
	if _, err := pool.Exec(ctx, `UPDATE worlds SET max_provinces = 1 WHERE id = $1`, worldID); err != nil {
		t.Fatalf("cap world at 1: %v", err)
	}

	authSvc := auth.NewService(pool, "test-secret")
	hostOwnerID, _ := registerViewer(t, ctx, authSvc, "founder-phase-host")
	if _, err := pool.Exec(ctx,
		`INSERT INTO founder_phase (world_id, owner_id, population, grain_amount, grain_rate, silver_amount, silver_rate, active)
		 VALUES ($1, $2, 100, 0, 0, 0, 0, true)`,
		worldID, hostOwnerID,
	); err != nil {
		t.Fatalf("seed active founder_phase: %v", err)
	}

	_, token := registerViewer(t, ctx, authSvc, "second-joiner")
	r := joinCultureRouter(pool, authSvc, clock.NewTestClock(time.Now()))
	req := httptest.NewRequest(http.MethodPost, "/worlds/"+worldID.String()+"/join", strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("POST /join against a world capped at 1, already holding one founder-phase host = %d %q, want 409", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "queued") {
		t.Errorf("409 body %q still promises a queue — none exists (D2 is post-MVP)", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "world is full") {
		t.Errorf("409 body %q doesn't say the world is full", rec.Body.String())
	}
}
