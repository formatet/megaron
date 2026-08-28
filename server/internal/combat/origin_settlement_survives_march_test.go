package combat

// origin_settlement_id (mig 126, megaron_plan_rekryteringsmodell.md) must
// survive a march exactly like SupportSettlementID already does — the plan's
// explicit warning was not to repeat the home_settlement_id fella (mig 074,
// nulled on explore-return, unit_arrival.go). This proves the strongest,
// most common case: a plain march clears settlement_id (the unit leaves
// garrison) but must leave origin_settlement_id untouched, since the
// reinforce trickle (kharis/tick.go applyReinforcement) keys eligibility off
// settlement_id == origin_settlement_id — if origin ever drifted, a cohort
// could either wrongly keep reinforcing away from home or wrongly lose
// eligibility on return.

import (
	"context"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

func TestStartMarch_OriginSettlementIDSurvivesPlainMarch(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

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
		"origin-march-"+uuid.New().String(),
	).Scan(&ownerID); err != nil {
		t.Fatalf("create test player: %v", err)
	}

	// A straight line of plains from the capital (0,0) out to (0,3) — plain
	// march, no colonize/explore gates, targetKnown=nil below skips the FOW
	// check entirely (see march_start.go), so no visibility fixture is needed.
	for r := 0; r <= 3; r++ {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, 0, $2, 'plains')`,
			worldID, r,
		); err != nil {
			t.Fatalf("insert map tile (0,%d): %v", r, err)
		}
	}

	var capitalProv uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`,
		worldID,
	).Scan(&capitalProv); err != nil {
		t.Fatalf("create capital province: %v", err)
	}
	var capitalID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
		 VALUES ($1, $2, 'Origin Home', 'achaean', $3, 'capital', true) RETURNING id`,
		worldID, capitalProv, ownerID,
	).Scan(&capitalID); err != nil {
		t.Fatalf("create capital settlement: %v", err)
	}

	// A garrisoned, battle-worn cohort (62/100 — March has no size gate) whose
	// origin_settlement_id is the capital, exactly what api/handlers/province.go
	// Recruit sets at recruit time.
	var unitID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, status, settlement_id,
		                    support_settlement_id, origin_settlement_id)
		 VALUES ($1, $2, 'spearman', 'land', 62, 'garrison', $3, $3, $3)
		 RETURNING id`,
		worldID, ownerID, capitalID,
	).Scan(&unitID); err != nil {
		t.Fatalf("create garrisoned cohort: %v", err)
	}

	clk := clock.NewTestClock(time.Now())
	scheduler := events.NewScheduler(pool, clk)
	eventStore := events.NewStore(pool)

	if _, err := StartMarch(ctx, pool, scheduler, eventStore, clk, MarchOrder{
		WorldID: worldID, PlayerID: ownerID, UnitID: unitID,
		TargetQ: 0, TargetR: 3,
	}, nil); err != nil {
		t.Fatalf("StartMarch(plain march) = %v, want success", err)
	}

	var status string
	var settlementID, originID *uuid.UUID
	if err := pool.QueryRow(ctx,
		`SELECT status, settlement_id, origin_settlement_id FROM units WHERE id = $1`, unitID,
	).Scan(&status, &settlementID, &originID); err != nil {
		t.Fatalf("reload unit after march: %v", err)
	}
	if status != "marching" {
		t.Errorf("status after StartMarch = %q, want marching", status)
	}
	if settlementID != nil {
		t.Errorf("settlement_id after StartMarch = %v, want NULL (unit left garrison)", settlementID)
	}
	if originID == nil || *originID != capitalID {
		t.Errorf("origin_settlement_id after StartMarch = %v, want unchanged %v (must survive a march, "+
			"same invariant as SupportSettlementID — never repeat the home_settlement_id fella, mig 074)",
			originID, capitalID)
	}
}
