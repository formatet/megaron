package handlers

// DB integration tests (real Postgres, gated by DATABASE_URL) for the escort
// adoption bug found in play 2026-08-05: Timothy founded Iolkos after marching
// his two spearman cohorts off the founding hex first, and both starved to
// death two game days later from a fully solvent metropolis. The root is
// found_metropolis.go's escort UPDATE, which only adopted a cohort standing
// EXACTLY on the founding hex (`status='positioned' AND q=$5 AND r=$6`) — a
// cohort that had moved never got support_settlement_id, and since migration
// 100 that column is the sole payer of both grain and silver upkeep.
//
// Canon (Timothy 2026-08-05, temenos_enheter.md §"Canon 2026-08-05"):
//  1. Adoption at founding is AUTOMATIC and UNCONDITIONAL — every unit the
//     Wanax owns in the founder phase becomes the new metropolis's, wherever
//     it stands. A cohort that is not on the founding hex stays exactly where
//     it is (status/q/r untouched) and merely gains a payer; only the cohort
//     actually standing on the founding hex becomes garrison.
//  2. Before founding, the horde (and its escort) eats nothing: grain_rate=0
//     but grain_amount keeps its current value as a dowry poured into the
//     metropolis at founding. Silver upkeep IS still paid from the store.

import (
	"context"
	"os"
	"testing"

	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func escortTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set — skipping DB integration test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// escortCatchmentOffsets mirrors foundingCatchmentOffsets (create_metropolis_founding_test.go):
// the centre hex followed by its 6 neighbours, all plains so founding never
// stumbles on a terrain gate unrelated to this bug.
var escortCatchmentOffsets = [7][2]int{
	{0, 0}, {1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, -1}, {-1, 1},
}

// seedEscortWorld creates an active world + player + the 7 catchment tiles
// around (0,0), archiving any leftover active test world first (single-
// active-world enforcement, current_world_tick() picks WHERE status='active').
func seedEscortWorld(t *testing.T, pool *pgxpool.Pool) (worldID, playerID uuid.UUID) {
	t.Helper()
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status = 'archived' WHERE status = 'active'`,
	); err != nil {
		t.Fatalf("archive leftover active test worlds: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status, current_tick) VALUES ($1, 'active', 0) RETURNING id`,
		"test-escort-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `UPDATE worlds SET status='archived' WHERE id=$1`, worldID) })

	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
		"escort-"+uuid.New().String(), "escort-"+uuid.New().String()+"@test.invalid",
	).Scan(&playerID); err != nil {
		t.Fatalf("create player: %v", err)
	}

	for i, off := range escortCatchmentOffsets {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, $3, 'plains')`,
			worldID, off[0], off[1],
		); err != nil {
			t.Fatalf("seed catchment tile %d: %v", i, err)
		}
	}
	return worldID, playerID
}

// TestEscortAdoption_MovedCohortIsAdoptedInPlace is T1: a spearman that
// marched off the founding hex before the Wanax settled must still be
// adopted (support_settlement_id + ordinal set) — and must stay exactly
// where it stands, never teleported into the garrison (Timothy's canon
// decision #1: adoption is positional only for WHO becomes garrison, never
// for WHO gets a payer).
func TestEscortAdoption_MovedCohortIsAdoptedInPlace(t *testing.T) {
	pool := escortTestPool(t)
	ctx := context.Background()
	worldID, playerID := seedEscortWorld(t, pool)
	eventStore := events.NewStore(pool)
	sitosCfg := economy.LoadSitosConfig()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback(ctx)

	if _, err := seedNomadicHost(ctx, tx, eventStore, worldID, playerID, 0, 0); err != nil {
		t.Fatalf("seedNomadicHost: %v", err)
	}

	var spearIDs []uuid.UUID
	rows, err := tx.Query(ctx,
		`SELECT id FROM units WHERE world_id = $1 AND owner_id = $2 AND type = 'spearman' ORDER BY created_at`,
		worldID, playerID,
	)
	if err != nil {
		t.Fatalf("query escort spearmen: %v", err)
	}
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			t.Fatalf("scan spearman id: %v", err)
		}
		spearIDs = append(spearIDs, id)
	}
	rows.Close()
	if len(spearIDs) != 2 {
		t.Fatalf("seedNomadicHost produced %d spearmen, want 2", len(spearIDs))
	}
	movedID, stayedID := spearIDs[0], spearIDs[1]

	// The moved cohort scouts away from the founding hex before the Wanax
	// settles — exactly what killed Timothy's escort in play.
	const movedQ, movedR = 5, 5
	if _, err := tx.Exec(ctx,
		`UPDATE units SET q = $1, r = $2 WHERE id = $3`, movedQ, movedR, movedID,
	); err != nil {
		t.Fatalf("move escort cohort off founding hex: %v", err)
	}

	founded, err := foundMetropolisFromNomadicHost(ctx, tx, eventStore, sitosCfg, worldID, playerID, "Iolkos-"+uuid.New().String(), "achaean")
	if err != nil {
		t.Fatalf("foundMetropolisFromNomadicHost: %v", err)
	}

	var supportID *uuid.UUID
	var ordinal *int
	var status string
	var q, r *int
	var settlementID *uuid.UUID
	if err := tx.QueryRow(ctx,
		`SELECT support_settlement_id, ordinal, status, q, r, settlement_id FROM units WHERE id = $1`,
		movedID,
	).Scan(&supportID, &ordinal, &status, &q, &r, &settlementID); err != nil {
		t.Fatalf("read moved cohort after founding: %v", err)
	}

	if supportID == nil || *supportID != founded.SettlementID {
		t.Errorf("moved cohort support_settlement_id = %v, want %v — the escort that scouted away starves silently (the exact bug found in play)", supportID, founded.SettlementID)
	}
	if ordinal == nil {
		t.Error("moved cohort ordinal is NULL — an unadopted unit also loses its name")
	}
	if status != "positioned" {
		t.Errorf("moved cohort status = %q, want 'positioned' — adoption must not move it", status)
	}
	if q == nil || r == nil || *q != movedQ || *r != movedR {
		t.Errorf("moved cohort position = (%v,%v), want (%d,%d) — adoption teleported a field unit", q, r, movedQ, movedR)
	}
	if settlementID != nil {
		t.Errorf("moved cohort settlement_id = %v, want NULL — it is not garrisoned, it is still in the field", *settlementID)
	}

	// T2 (regression guard, same run): the cohort left standing on the
	// founding hex becomes garrison exactly as before the fix.
	var stayedSupportID *uuid.UUID
	var stayedOrdinal *int
	var stayedStatus string
	var stayedQ, stayedR *int
	var stayedSettlementID *uuid.UUID
	if err := tx.QueryRow(ctx,
		`SELECT support_settlement_id, ordinal, status, q, r, settlement_id FROM units WHERE id = $1`,
		stayedID,
	).Scan(&stayedSupportID, &stayedOrdinal, &stayedStatus, &stayedQ, &stayedR, &stayedSettlementID); err != nil {
		t.Fatalf("read stayed cohort after founding: %v", err)
	}
	if stayedStatus != "garrison" {
		t.Errorf("stayed cohort status = %q, want 'garrison'", stayedStatus)
	}
	if stayedQ != nil || stayedR != nil {
		t.Errorf("stayed cohort position = (%v,%v), want (NULL,NULL)", stayedQ, stayedR)
	}
	if stayedSettlementID == nil || *stayedSettlementID != founded.SettlementID {
		t.Errorf("stayed cohort settlement_id = %v, want %v", stayedSettlementID, founded.SettlementID)
	}
	if stayedSupportID == nil || *stayedSupportID != founded.SettlementID {
		t.Errorf("stayed cohort support_settlement_id = %v, want %v", stayedSupportID, founded.SettlementID)
	}
	if stayedOrdinal == nil {
		t.Error("stayed cohort ordinal is NULL")
	}

	// T3: ordinals are distinct, drawn from the (settlement, type) counter —
	// two spearmen must never share a regimental number.
	if ordinal != nil && stayedOrdinal != nil {
		if *ordinal == *stayedOrdinal {
			t.Errorf("both spearmen got ordinal %d — ordinals must be distinct", *ordinal)
		}
		got := map[int]bool{*ordinal: true, *stayedOrdinal: true}
		if !got[1] || !got[2] {
			t.Errorf("ordinals = {%d, %d}, want {1, 2}", *ordinal, *stayedOrdinal)
		}
	}
}

// TestEscortAdoption_HordeEatsNothingBeforeFounding is T4 (canon decision #2,
// Timothy 2026-08-05): the founder phase's grain drains at rate 0 — the horde
// and its escort carry their own rations — but grain_amount keeps its current
// value as a dowry poured into the metropolis at founding. Silver upkeep is
// still owed and must stay negative.
func TestEscortAdoption_HordeEatsNothingBeforeFounding(t *testing.T) {
	pool := escortTestPool(t)
	ctx := context.Background()
	worldID, playerID := seedEscortWorld(t, pool)
	eventStore := events.NewStore(pool)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback(ctx)

	if _, err := seedNomadicHost(ctx, tx, eventStore, worldID, playerID, 0, 0); err != nil {
		t.Fatalf("seedNomadicHost: %v", err)
	}

	var grainAmount, grainRate, silverRate float64
	var calcTick int
	if err := tx.QueryRow(ctx,
		`SELECT grain_amount, grain_rate, silver_rate, calc_tick FROM founder_phase WHERE world_id = $1 AND owner_id = $2`,
		worldID, playerID,
	).Scan(&grainAmount, &grainRate, &silverRate, &calcTick); err != nil {
		t.Fatalf("read founder_phase after seeding: %v", err)
	}

	if grainRate != 0 {
		t.Errorf("founder_phase.grain_rate = %v, want 0 — the horde must eat nothing before founding", grainRate)
	}
	if grainAmount <= 0 {
		t.Errorf("founder_phase.grain_amount = %v, want > 0 — the carried dowry must survive", grainAmount)
	}
	if silverRate >= 0 {
		t.Errorf("founder_phase.silver_rate = %v, want negative — sold is still owed to men who serve", silverRate)
	}

	// Let time pass (stepping the world tick, the same lever the acceptance
	// rig uses) and verify the settled() grain reading is unchanged.
	if _, err := tx.Exec(ctx, `UPDATE worlds SET current_tick = current_tick + 500 WHERE id = $1`, worldID); err != nil {
		t.Fatalf("advance world tick: %v", err)
	}
	var settledGrain float64
	if err := tx.QueryRow(ctx,
		`SELECT settled(grain_amount, grain_rate, calc_tick) FROM founder_phase WHERE world_id = $1 AND owner_id = $2`,
		worldID, playerID,
	).Scan(&settledGrain); err != nil {
		t.Fatalf("read settled grain after advancing tick: %v", err)
	}
	if settledGrain != grainAmount {
		t.Errorf("settled grain after 500 ticks = %v, want unchanged %v — the horde drained a store it should be carrying", settledGrain, grainAmount)
	}
}
