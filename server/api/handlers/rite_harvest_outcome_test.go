package handlers

// DB integration tests for megaron_plan_ritens_utfall.md: applyHarvestBlessing
// used to report its INTENTION ({"multiplier":1.25}) rather than its outcome —
// a HARD-rule violation (CLAUDE.md §Events: "events store outcomes, not
// intentions"). It now shares its RETURNING-captured-delta form with the
// tick-level applyDivineBlessing harvest_blessing branch
// (internal/kharis/tick.go:1766-1782), and reports why nothing was gained
// when the granary was empty or already full, instead of a text that always
// claims success. Calls applyHarvestBlessing directly — it is the function
// under test, not the rite-cast flow around it (plan §4 step 5). Real
// Postgres, gated by DATABASE_URL (armyDisplayTestPool).

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/religion"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// riteHarvestFixture creates a world/player/settlement with grain seeded at
// the given amount (rate=0, calc_tick=0 — settled() == amount regardless of
// when in the test current_world_tick() resolves, same precaution
// internal/kharis/divine_effects_test.go's seedDivineGarrison documents) and
// a cap of 1000.
func riteHarvestFixture(t *testing.T, pool *pgxpool.Pool, grainAmount float64) (worldID, settlementID uuid.UUID) {
	t.Helper()
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active'`,
	); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'active') RETURNING id`,
		"test-world-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create test world: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `UPDATE worlds SET status = 'archived' WHERE id = $1`, worldID)
	})

	authSvc := auth.NewService(pool, "test-secret")
	username := "rite-harvest-" + uuid.New().String()
	accessToken, _, err := authSvc.Register(ctx, username, "x")
	if err != nil {
		t.Fatalf("register test player: %v", err)
	}
	claims, err := authSvc.ValidateAccessToken(accessToken)
	if err != nil {
		t.Fatalf("validate minted token: %v", err)
	}

	var provinceID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type, coastal) VALUES ($1, 0, 0, 'plains', true) RETURNING id`,
		worldID,
	).Scan(&provinceID); err != nil {
		t.Fatalf("create province: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital)
		 VALUES ($1, $2, 'Phaistos', 'minoan', $3, 'capital', true) RETURNING id`,
		worldID, provinceID, claims.PlayerID,
	).Scan(&settlementID); err != nil {
		t.Fatalf("create settlement: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
		 VALUES ($1, 'grain', $2, 0, 1000, 0)`,
		settlementID, grainAmount,
	); err != nil {
		t.Fatalf("seed grain: %v", err)
	}
	return worldID, settlementID
}

func callApplyHarvestBlessing(t *testing.T, pool *pgxpool.Pool, settlementID uuid.UUID) (effect map[string]any, message string) {
	t.Helper()
	ctx := context.Background()
	h := NewSettlementHandler(pool, events.NewStore(pool), events.NewScheduler(pool, clock.NewTestClock(time.Now())), clock.NewTestClock(time.Now()))
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback(ctx)

	spec := religion.PrayerSpec{ID: "minoan_harvest_blessing", EffectType: religion.EffectHarvestBlessing, God: "Poseidon"}
	effect, message, err = h.applyHarvestBlessing(ctx, tx, settlementID, spec)
	if err != nil {
		t.Fatalf("applyHarvestBlessing: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
	return effect, message
}

func TestHarvestBlessing_EmptyGranaryReportsZero(t *testing.T) {
	pool := armyDisplayTestPool(t)
	_, settlementID := riteHarvestFixture(t, pool, 0)

	effect, message := callApplyHarvestBlessing(t, pool, settlementID)

	gained, _ := effect["gained"].(float64)
	if gained != 0 {
		t.Errorf("gained = %v, want 0 (empty granary)", gained)
	}
	if !strings.Contains(message, "empty") {
		t.Errorf("message = %q, want it to say the granary is empty", message)
	}
	mult, _ := effect["multiplier"].(float64)
	if mult != 1.25 {
		t.Errorf("multiplier = %v, want unchanged 1.25 (old field keeps its meaning)", mult)
	}
}

func TestHarvestBlessing_ReportsActualGain(t *testing.T) {
	pool := armyDisplayTestPool(t)
	_, settlementID := riteHarvestFixture(t, pool, 400)

	effect, message := callApplyHarvestBlessing(t, pool, settlementID)

	gained, _ := effect["gained"].(float64)
	if gained != 100 {
		t.Errorf("gained = %v, want 100 (400 * 0.25)", gained)
	}
	if !strings.Contains(message, "+100") {
		t.Errorf("message = %q, want it to contain +100", message)
	}
}

func TestHarvestBlessing_ClippedByCapReportsWhatFit(t *testing.T) {
	pool := armyDisplayTestPool(t)
	_, settlementID := riteHarvestFixture(t, pool, 900) // cap 1000: 900*1.25=1125, clipped to 1000

	effect, message := callApplyHarvestBlessing(t, pool, settlementID)

	gained, _ := effect["gained"].(float64)
	if gained != 100 {
		t.Errorf("gained = %v, want 100 (clipped to cap 1000, NOT 225 = 900*0.25)", gained)
	}
	if !strings.Contains(message, "+100") {
		t.Errorf("message = %q, want it to contain the actual clipped gain +100", message)
	}
}

// TestHarvestBlessing_RiteCastEventCarriesGained proves the event payload
// (not just the direct return value) carries the real outcome — the whole
// point of the fix is that RiteCast becomes calibratable.
func TestHarvestBlessing_RiteCastEventCarriesGained(t *testing.T) {
	pool := armyDisplayTestPool(t)
	worldID, settlementID := riteHarvestFixture(t, pool, 400)
	ctx := context.Background()

	effect, _ := callApplyHarvestBlessing(t, pool, settlementID)
	store := events.NewStore(pool)
	if _, err := store.Append(ctx, settlementID, events.StreamReligion, "RiteCast", map[string]any{
		"effect_type": religion.EffectHarvestBlessing,
		"effect":      effect,
	}, worldID, nil); err != nil {
		t.Fatalf("append RiteCast: %v", err)
	}

	var raw []byte
	if err := pool.QueryRow(ctx,
		`SELECT payload FROM events WHERE stream_id = $1 AND event_type = 'RiteCast' ORDER BY id DESC LIMIT 1`,
		settlementID,
	).Scan(&raw); err != nil {
		t.Fatalf("read RiteCast event: %v", err)
	}
	var p struct {
		Effect struct {
			Gained float64 `json:"gained"`
		} `json:"effect"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatalf("unmarshal RiteCast payload: %v", err)
	}
	if p.Effect.Gained != 100 {
		t.Errorf("RiteCast event effect.gained = %v, want 100 (the actual DB delta)", p.Effect.Gained)
	}
}
