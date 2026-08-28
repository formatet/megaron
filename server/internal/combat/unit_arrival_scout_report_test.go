package combat

// Scout report (megaron_todo.md, §Friktion — "explore-enheten kommer hem utan
// rapport — kartan uppdateras tyst"): a unit reaching its explore target must
// tell its owner what it found there (terrain + any deposits), not just that
// it's turning home. This test drives the real resolve() dispatcher through
// the outbound explore arrival and asserts a ScoutReport notification/event
// carries the destination hex's actual deposit flags — the copper_deposit
// read at (3,0) below is the same map_tiles read foundColony already uses.

import (
	"context"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

func TestExploreOrder_ScoutReport(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	// Single-active-world invariant (same guard as TestExploreOrder_AutoReturn).
	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active'`,
	); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'active') RETURNING id`,
		"test-world-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID)
	})

	var ownerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, password_hash) VALUES ($1, 'x') RETURNING id`,
		"scout-"+uuid.New().String(),
	).Scan(&ownerID); err != nil {
		t.Fatalf("create test player: %v", err)
	}

	// Map: coastal capital at (0,0), open sea east to (3,0) — the explore
	// target hex (3,0) carries a known copper deposit, so the report can be
	// asserted against a concrete "found something" case.
	tiles := []struct {
		q, r     int
		terrain  string
		copperDep bool
	}{
		{0, 0, "plains", false},
		{1, 0, "coastal_sea", false},
		{2, 0, "coastal_sea", false},
		{3, 0, "coastal_sea", true},
	}
	for _, tl := range tiles {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain, copper_deposit) VALUES ($1, $2, $3, $4, $5)`,
			worldID, tl.q, tl.r, tl.terrain, tl.copperDep,
		); err != nil {
			t.Fatalf("insert map tile (%d,%d): %v", tl.q, tl.r, err)
		}
	}

	var capitalProvinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`,
		worldID,
	).Scan(&capitalProvinceID); err != nil {
		t.Fatalf("create capital province: %v", err)
	}
	var capitalID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
		 VALUES ($1, $2, 'Capital City', 'achaean', $3, 'capital', true) RETURNING id`,
		worldID, capitalProvinceID, ownerID,
	).Scan(&capitalID); err != nil {
		t.Fatalf("create capital settlement: %v", err)
	}

	var unitID uuid.UUID
	arrivesAt := time.Now()
	if err := pool.QueryRow(ctx,
		`INSERT INTO units
		   (world_id, owner_id, type, category, size, crew, status, q, r,
		    target_q, target_r, departs_at, arrives_at, march_intent, home_settlement_id)
		 VALUES ($1, $2, 'galley', 'naval', 1, 20, 'marching', 1, 0,
		         3, 0, now(), $3, 'explore', $4)
		 RETURNING id`,
		worldID, ownerID, arrivesAt, capitalID,
	).Scan(&unitID); err != nil {
		t.Fatalf("create exploring unit: %v", err)
	}

	fb := &fakeBroadcaster{}
	h := &UnitArrivalHandler{
		pool:       pool,
		eventStore: events.NewStore(pool),
		hub:        fb,
		scheduler:  events.NewScheduler(pool, clock.NewTestClock(time.Now())),
		clk:        clock.NewTestClock(time.Now()),
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	if err := h.resolve(ctx, tx, unitID, worldID); err != nil {
		tx.Rollback(ctx)
		t.Fatalf("resolve (target arrival): %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit tx: %v", err)
	}

	// A ScoutReport must have fired alongside the existing UnitExploreReturned
	// "turning home" ping — the new report, not a replacement.
	var scoutPayload map[string]any
	foundScoutReport := false
	foundExploreReturned := false
	for i, kind := range fb.notified {
		switch kind {
		case "ScoutReport":
			foundScoutReport = true
			scoutPayload, _ = fb.payloads[i].(map[string]any)
		case "UnitExploreReturned":
			foundExploreReturned = true
		}
	}
	if !foundScoutReport {
		t.Fatalf("NotifyPlayer calls = %v, want a ScoutReport among them", fb.notified)
	}
	if !foundExploreReturned {
		t.Errorf("NotifyPlayer calls = %v, want UnitExploreReturned kept alongside ScoutReport", fb.notified)
	}
	if scoutPayload == nil {
		t.Fatalf("ScoutReport payload was not a map[string]any: %v", fb.payloads)
	}
	if terrain, _ := scoutPayload["terrain"].(string); terrain != "coastal_sea" {
		t.Errorf("ScoutReport terrain = %q, want %q", terrain, "coastal_sea")
	}
	if copper, _ := scoutPayload["copper_deposit"].(bool); !copper {
		t.Errorf("ScoutReport copper_deposit = %v, want true (the explore target's known deposit)", copper)
	}
	if q, _ := scoutPayload["q"].(int); q != 3 {
		t.Errorf("ScoutReport q = %v, want 3", q)
	}
	if r, _ := scoutPayload["r"].(int); r != 0 {
		t.Errorf("ScoutReport r = %v, want 0", r)
	}
}
