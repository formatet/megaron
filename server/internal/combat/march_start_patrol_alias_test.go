package combat

// "sentry" used to be the ONLY name for this naval march intent — the same
// word that also names the land STANCE you hold in place (unit.StanceSentry,
// untouched). One word, two different orders, picked out only by which flag
// carried it (megaron_plan_cli_sanning, 2026-08-26). This test fastens the
// rename at the StartMarch layer, on both sides of the alias:
//   - a fresh order sent with the OLD name ("sentry") is still accepted, and
//     normalised to the new canonical name before it is persisted — so every
//     march dispatched from here on is unambiguous in the DB too.
//   - the NEW name ("patrol") works identically, unaliased.
//
// internal/combat/unit_arrival_sentry_test.go covers the read side: a unit
// already in flight with the pre-rename literal "sentry" written to
// march_intent (dispatched before this change deployed) must still land.

import (
	"context"
	"strings"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

func TestStartMarch_SentryIsADeprecatedAliasForPatrol(t *testing.T) {
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
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID) })

	var ownerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
		"patrol-alias-"+uuid.New().String(), "patrol-alias-"+uuid.New().String()+"@test.invalid",
	).Scan(&ownerID); err != nil {
		t.Fatalf("create test player: %v", err)
	}

	// Coastal capital at (0,0), open sea running east to (3,0). Same layout as
	// unit_arrival_sentry_test.go, so NearestSeaNeighbor resolves the harbour
	// (departure) hex to (1,0) deterministically.
	for _, tl := range []struct {
		q, r    int
		terrain string
	}{
		{0, 0, "plains"}, {1, 0, "coastal_sea"}, {2, 0, "coastal_sea"}, {3, 0, "coastal_sea"},
	} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1,$2,$3,$4)`,
			worldID, tl.q, tl.r, tl.terrain); err != nil {
			t.Fatalf("insert map tile (%d,%d): %v", tl.q, tl.r, err)
		}
	}

	var capitalProvinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1,0,0,'plains') RETURNING id`,
		worldID).Scan(&capitalProvinceID); err != nil {
		t.Fatalf("create capital province: %v", err)
	}
	var capitalID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
		 VALUES ($1,$2,'Capital City','achaean',$3,'capital',true) RETURNING id`,
		worldID, capitalProvinceID, ownerID).Scan(&capitalID); err != nil {
		t.Fatalf("create capital settlement: %v", err)
	}

	clk := clock.NewTestClock(time.Now())
	scheduler := events.NewScheduler(pool, clk)
	eventStore := events.NewStore(pool)

	// dispatch sends one garrisoned galley off with the given intent string
	// and returns the march_intent the order actually persisted.
	dispatch := func(t *testing.T, intent string) string {
		t.Helper()
		var shipID uuid.UUID
		if err := pool.QueryRow(ctx,
			`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, settlement_id)
			 VALUES ($1,$2,'galley','naval',1,20,'garrison',$3) RETURNING id`,
			worldID, ownerID, capitalID,
		).Scan(&shipID); err != nil {
			t.Fatalf("create garrisoned ship: %v", err)
		}

		if _, err := StartMarch(ctx, pool, scheduler, eventStore, clk, MarchOrder{
			WorldID: worldID, PlayerID: ownerID, UnitID: shipID,
			TargetQ: 3, TargetR: 0, Intent: intent,
		}, nil); err != nil {
			t.Fatalf("StartMarch(intent=%q) failed: %v", intent, err)
		}

		var marchIntent string
		if err := pool.QueryRow(ctx,
			`SELECT COALESCE(march_intent, '') FROM units WHERE id = $1`, shipID,
		).Scan(&marchIntent); err != nil {
			t.Fatalf("load dispatched unit: %v", err)
		}
		return marchIntent
	}

	t.Run("old name sentry normalises to patrol on write", func(t *testing.T) {
		got := dispatch(t, "sentry")
		if got != "patrol" {
			t.Errorf("march_intent = %q after dispatching intent=\"sentry\", want \"patrol\" — the alias must "+
				"normalise before persisting, or the DB re-accumulates the ambiguous old name", got)
		}
	})

	t.Run("new name patrol works unaliased", func(t *testing.T) {
		got := dispatch(t, "patrol")
		if got != "patrol" {
			t.Errorf("march_intent = %q after dispatching intent=\"patrol\", want \"patrol\"", got)
		}
	})
}

// An intent the server has never heard of is still rejected, and the error
// now names "patrol" — the current, unambiguous name — instead of the
// retired "sentry" (the message is guidance for what to send NEXT, not a
// promise that every historical alias still appears in it).
func TestStartMarch_UnknownIntentErrorNamesPatrolNotSentry(t *testing.T) {
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
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID) })

	var ownerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
		"bad-intent-"+uuid.New().String(), "bad-intent-"+uuid.New().String()+"@test.invalid",
	).Scan(&ownerID); err != nil {
		t.Fatalf("create test player: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1,0,0,'plains'), ($1,1,0,'plains')`,
		worldID); err != nil {
		t.Fatalf("insert map tiles: %v", err)
	}
	var unitID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, q, r)
		 VALUES ($1,$2,'spearman','land',100,0,'positioned',0,0) RETURNING id`,
		worldID, ownerID,
	).Scan(&unitID); err != nil {
		t.Fatalf("create positioned unit: %v", err)
	}

	clk := clock.NewTestClock(time.Now())
	scheduler := events.NewScheduler(pool, clk)
	eventStore := events.NewStore(pool)

	_, err := StartMarch(ctx, pool, scheduler, eventStore, clk, MarchOrder{
		WorldID: worldID, PlayerID: ownerID, UnitID: unitID,
		TargetQ: 1, TargetR: 0, Intent: "bogus",
	}, nil)
	if err == nil {
		t.Fatal("StartMarch(intent=\"bogus\") succeeded, want a rejection")
	}
	if got := err.Error(); !strings.Contains(got, "patrol") || strings.Contains(got, "\"sentry\"") {
		t.Errorf("unknown-intent error = %q — want it to name \"patrol\" and not the retired \"sentry\"", got)
	}
}
