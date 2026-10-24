package handlers

// DB integration tests for megaron_plan_ritens_utfall.md: applyHarvestBlessing
// used to report its INTENTION ({"multiplier":1.25}) rather than its outcome —
// a HARD-rule violation (CLAUDE.md §Events: "events store outcomes, not
// intentions"). It now shares its RETURNING-captured-delta form with the
// tick-level applyDivineBlessing harvest_blessing branch
// (internal/kharis/tick.go:1766-1782), and reports why nothing was gained
// when the city stock was empty or already full, instead of a text that always
// claims success. Calls applyHarvestBlessing directly — it is the function
// under test, not the rite-cast flow around it (plan §4 step 5). Real
// Postgres, gated by DATABASE_URL (armyDisplayTestPool).

import (
	"context"
	"math"
	"strings"
	"testing"
	"time"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/economy"
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

func grainRate(t *testing.T, pool *pgxpool.Pool, settlementID uuid.UUID) float64 {
	t.Helper()
	var r float64
	if err := pool.QueryRow(context.Background(),
		`SELECT rate FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'grain'`, settlementID,
	).Scan(&r); err != nil {
		t.Fatalf("read grain rate: %v", err)
	}
	return r
}

// TestHarvestBlessing_RaisesYieldForItsSpan: the harvest blessing is a quarter
// more yield for economy.HarvestBlessingTicks, then nothing (Timothy
// 2026-10-10) — no longer ×1.25 on the grain store, which gave the most to the
// fullest food stock and nothing to an empty one.
func TestHarvestBlessing_RaisesYieldForItsSpan(t *testing.T) {
	pool := armyDisplayTestPool(t)
	ctx := context.Background()
	worldID, settlementID := riteHarvestFixture(t, pool, 0) // an EMPTY food stock is blessed too
	if err := economy.RecomputeProduction(ctx, pool, settlementID); err != nil {
		t.Fatalf("baseline recompute: %v", err)
	}
	before := grainRate(t, pool, settlementID)
	if before <= 0 {
		t.Fatalf("baseline grain rate %v — the test needs some production to raise", before)
	}
	var now int
	if err := pool.QueryRow(ctx, `SELECT current_world_tick()`).Scan(&now); err != nil {
		t.Fatalf("read tick: %v", err)
	}

	effect, msg := callApplyHarvestBlessing(t, pool, settlementID)

	if got, want := grainRate(t, pool, settlementID), before*(1+economy.HarvestBlessingYieldBonus); math.Abs(got-want) > 1e-9 {
		t.Errorf("blessed grain rate %v, want %v (a quarter more)", got, want)
	}
	if until, _ := effect["until_tick"].(int); until != now+economy.HarvestBlessingTicks {
		t.Errorf("until_tick = %v, want %d", effect["until_tick"], now+economy.HarvestBlessingTicks)
	}
	if !strings.Contains(msg, "25 game days") {
		t.Errorf("message %q does not say how long", msg)
	}

	if _, err := pool.Exec(ctx, `UPDATE worlds SET current_tick = $2 WHERE id = $1`,
		worldID, now+economy.HarvestBlessingTicks); err != nil {
		t.Fatalf("advance to the end of the span: %v", err)
	}
	if err := economy.RecomputeProduction(ctx, pool, settlementID); err != nil {
		t.Fatalf("recompute after the span: %v", err)
	}
	if got := grainRate(t, pool, settlementID); math.Abs(got-before) > 1e-9 {
		t.Errorf("grain rate %v after the blessing ended, want %v", got, before)
	}
}
