package combat

// Regression test for "the build-complete notification points at a dead
// lever" (megaron_todo.md, 2026-08-26). Before this fix BuildComplete's hint
// told the player a LABOR PERCENT would start production — either "15%
// auto-allocated" or "set a labor percent" — but post-P4
// (megaron_plan_fysisk_gubbemodell.md) production only starts once a gubbe
// is physically placed on a hex or building slot (settlement_placement,
// keryx place/staff or the city's placement grid); settlement_labor's
// weight does nothing for any good except cult. The old text pointed the
// player at a surface that no longer exists.
//
// This locks in the honest replacement text AND proves the fix leaves
// cult's settlement_labor devotion weight — the one good that IS still
// live on that table — completely untouched.

import (
	"context"
	"encoding/json"
	"math"
	"testing"

	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// newBuildNotifyFixture creates a minimal world/player/province/settlement
// plus one catchment-ring map tile (plains, no deposit, non-coastal) so a
// completed 'mine' building unlocks 'stone' — production_rules has NULL
// terrain_type and NULL requires_deposit for mine/stone (mig 018, rates
// tuned since by 079/129 but the NULL/NULL shape is untouched) — without
// depending on mapgen or deposit RNG.
func newBuildNotifyFixture(t *testing.T, pool *pgxpool.Pool) (worldID, ownerID, settlementID uuid.UUID) {
	t.Helper()
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `UPDATE worlds SET status = 'archived' WHERE status = 'active'`); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'active') RETURNING id`,
		"test-buildnotify-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID)
	})

	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, password_hash) VALUES ($1, 'x') RETURNING id`,
		"buildnotify-"+uuid.New().String(),
	).Scan(&ownerID); err != nil {
		t.Fatalf("create player: %v", err)
	}

	var provinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type) VALUES ($1, 0, 0, 'plains') RETURNING id`,
		worldID,
	).Scan(&provinceID); err != nil {
		t.Fatalf("create province: %v", err)
	}

	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, state, population)
		 VALUES ($1, $2, 'Notifytown', 'achaean', $3, 'capital', true, 'active', 1000) RETURNING id`,
		worldID, provinceID, ownerID,
	).Scan(&settlementID); err != nil {
		t.Fatalf("create settlement: %v", err)
	}

	// (1,0) is distance 1 from the settlement's own hex (0,0) — inside the
	// radius-2 catchment ring (hexgrid.Ring), plain, no deposits, no coast.
	if _, err := pool.Exec(ctx,
		`INSERT INTO map_tiles (world_id, q, r, terrain, coastal) VALUES ($1, 1, 0, 'plains', false)`,
		worldID,
	); err != nil {
		t.Fatalf("seed catchment tile: %v", err)
	}

	return worldID, ownerID, settlementID
}

// runBuildComplete enqueues buildingType on settlementID and runs
// BuildCompleteHandler.Handle once, returning the captured notification
// payload (nil if the handler never called NotifyPlayer).
func runBuildComplete(t *testing.T, pool *pgxpool.Pool, worldID, settlementID uuid.UUID, buildingType string) map[string]any {
	t.Helper()
	ctx := context.Background()

	queueID := uuid.New()
	if _, err := pool.Exec(ctx,
		`INSERT INTO build_queue (id, settlement_id, world_id, building_type, complete_at)
		 VALUES ($1, $2, $3, $4, now())`,
		queueID, settlementID, worldID, buildingType,
	); err != nil {
		t.Fatalf("seed build_queue: %v", err)
	}

	payload, err := json.Marshal(BuildCompletePayload{
		SettlementID: settlementID,
		BuildQueueID: queueID,
		BuildingType: buildingType,
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	evt := events.ScheduledEvent{WorldID: worldID, DueTick: 1, Payload: payload}

	fb := &fakeBroadcaster{}
	h := NewBuildCompleteHandler(pool, events.NewStore(pool), fb)
	if err := h.Handle(ctx, evt); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	for i, kind := range fb.notified {
		if kind == "BuildComplete" {
			body, ok := fb.payloads[i].(map[string]any)
			if !ok {
				t.Fatalf("BuildComplete payload has unexpected type %T", fb.payloads[i])
			}
			return body
		}
	}
	return nil
}

// TestBuildComplete_HintPointsToPlacement_NotDeadLabor is the red/green
// proof for the fix: a completed 'mine' unlocks 'stone' (no prior
// placement exists), and the hint must name the real surface — placement —
// not a labor percentage that has been inert since P4.
func TestBuildComplete_HintPointsToPlacement_NotDeadLabor(t *testing.T) {
	pool := testPool(t)
	worldID, _, settlementID := newBuildNotifyFixture(t, pool)

	body := runBuildComplete(t, pool, worldID, settlementID, "mine")
	if body == nil {
		t.Fatalf("no BuildComplete notification was sent")
	}

	hint, _ := body["hint"].(string)
	const want = "mine is built but produces nothing until staffed — place a gubbe to work stone (`keryx place`/`staff`, or the city's placement grid) to start production."
	if hint != want {
		t.Errorf("hint mismatch:\n got:  %q\n want: %q", hint, want)
	}

	unlocked, _ := body["unlocked_goods"].([]string)
	if len(unlocked) != 1 || unlocked[0] != "stone" {
		t.Errorf("unlocked_goods = %v, want [stone]", unlocked)
	}

	// The old code wrote a dead labor weight for the unlocked good via
	// AutoAllocateUnlocked. Prove that write is gone: no settlement_labor
	// row for 'stone' should exist after the build completes.
	var stoneRowExists bool
	if err := pool.QueryRow(context.Background(),
		`SELECT EXISTS(SELECT 1 FROM settlement_labor WHERE settlement_id = $1 AND good_key = 'stone')`,
		settlementID,
	).Scan(&stoneRowExists); err != nil {
		t.Fatalf("check settlement_labor: %v", err)
	}
	if stoneRowExists {
		t.Errorf("settlement_labor has a 'stone' row — a dead weight was written for a non-cult good")
	}
}

// TestBuildComplete_PreservesCultWeight proves the one live settlement_labor
// path — cult devotion — is untouched by a build completion for an
// unrelated building. Before this fix, AutoAllocateUnlocked explicitly
// excluded cult from its budget math but never wrote to it either; this
// test guards against a future edit reintroducing a write on that path.
func TestBuildComplete_PreservesCultWeight(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	worldID, _, settlementID := newBuildNotifyFixture(t, pool)

	const cultWeight = 0.20
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_labor (settlement_id, good_key, weight) VALUES ($1, 'cult', $2)`,
		settlementID, cultWeight,
	); err != nil {
		t.Fatalf("seed cult weight: %v", err)
	}

	runBuildComplete(t, pool, worldID, settlementID, "mine")

	var got float64
	if err := pool.QueryRow(ctx,
		`SELECT weight FROM settlement_labor WHERE settlement_id = $1 AND good_key = 'cult'`,
		settlementID,
	).Scan(&got); err != nil {
		t.Fatalf("read cult weight after build: %v", err)
	}
	// settlement_labor.weight is a Postgres real (float32) — compare with
	// tolerance, never !=, or a float32→float64 round-trip artifact reads as
	// a false regression (megaron memory: KH1 cult-preserve test).
	if math.Abs(got-cultWeight) > 1e-4 {
		t.Errorf("cult weight changed: got %v, want unchanged %v", got, cultWeight)
	}
}
