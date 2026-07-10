package combat

// Metropolis-succession and game-over (Timothy 2026-07-10). Exercises the shared
// handleOwnerCityLoss directly: losing a capital while a colony remains promotes
// the highest-loyalty survivor (no dispossession); losing the last city ends the
// game (dispossessed + last_settlement_id anchored for the epitaph). Skips when
// DATABASE_URL is unset, like the other DB integration tests.

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestSuccession_PromoteThenGameOver(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status) VALUES ($1, 'active') RETURNING id`,
		"test-world-"+uuid.New().String(),
	).Scan(&worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `UPDATE worlds SET status='archived' WHERE id=$1`, worldID) })

	var playerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
		"wanax-"+uuid.New().String(), uuid.New().String()+"@test.invalid",
	).Scan(&playerID); err != nil {
		t.Fatalf("create player: %v", err)
	}

	mkProvince := func(q, r int) uuid.UUID {
		var id uuid.UUID
		if err := pool.QueryRow(ctx,
			`INSERT INTO provinces (world_id, map_q, map_r) VALUES ($1, $2, $3) RETURNING id`,
			worldID, q, r,
		).Scan(&id); err != nil {
			t.Fatalf("create province (%d,%d): %v", q, r, err)
		}
		return id
	}
	mkSettlement := func(prov uuid.UUID, name string, capital bool, loyalty, pop int) uuid.UUID {
		var id uuid.UUID
		if err := pool.QueryRow(ctx,
			`INSERT INTO settlements
			   (world_id, province_id, name, culture_id, owner_id, is_capital, state, population, loyalty)
			 VALUES ($1, $2, $3, 'akhaier', $4, $5, 'active', $6, $7) RETURNING id`,
			worldID, prov, name, playerID, capital, pop, loyalty,
		).Scan(&id); err != nil {
			t.Fatalf("create settlement %s: %v", name, err)
		}
		return id
	}

	// Capital (low loyalty) plus two colonies — the higher-loyalty colony should win
	// the succession even though the other has more people.
	capital := mkSettlement(mkProvince(0, 0), "Tiryns", true, 2, 5000)
	richColony := mkSettlement(mkProvince(3, 0), "Nafplio", false, 3, 3000)
	loyalColony := mkSettlement(mkProvince(6, 0), "Asine", false, 4, 800)

	if _, err := pool.Exec(ctx,
		`INSERT INTO player_world_records (player_id, world_id, settlement_id, status)
		 VALUES ($1, $2, $3, 'active')`,
		playerID, worldID, capital,
	); err != nil {
		t.Fatalf("create player_world_records: %v", err)
	}

	// ── 1. Capital falls, colonies remain → promote the loyal colony, no game over ──
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx1: %v", err)
	}
	if _, err := tx.Exec(ctx,
		`UPDATE settlements SET owner_id = NULL, state = 'collapsed' WHERE id = $1`, capital); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("clear capital owner: %v", err)
	}
	over, err := handleOwnerCityLoss(ctx, tx, playerID, worldID, capital)
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("handleOwnerCityLoss (capital): %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit tx1: %v", err)
	}
	if over {
		t.Errorf("gameOver = true after capital fell with colonies remaining; want false")
	}

	var loyalIsCapital, richIsCapital bool
	_ = pool.QueryRow(ctx, `SELECT is_capital FROM settlements WHERE id=$1`, loyalColony).Scan(&loyalIsCapital)
	_ = pool.QueryRow(ctx, `SELECT is_capital FROM settlements WHERE id=$1`, richColony).Scan(&richIsCapital)
	if !loyalIsCapital {
		t.Errorf("highest-loyalty colony (Asine) was not promoted to capital")
	}
	if richIsCapital {
		t.Errorf("lower-loyalty colony (Nafplio) was wrongly promoted to capital")
	}

	var status string
	var recSettlement *uuid.UUID
	if err := pool.QueryRow(ctx,
		`SELECT status, settlement_id FROM player_world_records WHERE player_id=$1 AND world_id=$2`,
		playerID, worldID,
	).Scan(&status, &recSettlement); err != nil {
		t.Fatalf("read records after promotion: %v", err)
	}
	if status != "active" {
		t.Errorf("status = %q after promotion; want active", status)
	}
	if recSettlement == nil || *recSettlement != loyalColony {
		t.Errorf("records.settlement_id not repointed to the promoted colony")
	}

	// ── 2. Both colonies fall (last one triggers game over) ──
	// Collapse the rich colony first (not the last) — still no game over.
	tx2, _ := pool.Begin(ctx)
	_, _ = tx2.Exec(ctx, `UPDATE settlements SET owner_id=NULL, state='collapsed' WHERE id=$1`, richColony)
	over2, err := handleOwnerCityLoss(ctx, tx2, playerID, worldID, richColony)
	if err != nil {
		_ = tx2.Rollback(ctx)
		t.Fatalf("handleOwnerCityLoss (rich colony): %v", err)
	}
	_ = tx2.Commit(ctx)
	if over2 {
		t.Errorf("gameOver = true while the promoted capital still stood; want false")
	}

	// Now the promoted capital is the last city — its fall is game over.
	tx3, _ := pool.Begin(ctx)
	_, _ = tx3.Exec(ctx, `UPDATE settlements SET owner_id=NULL, state='collapsed' WHERE id=$1`, loyalColony)
	over3, err := handleOwnerCityLoss(ctx, tx3, playerID, worldID, loyalColony)
	if err != nil {
		_ = tx3.Rollback(ctx)
		t.Fatalf("handleOwnerCityLoss (last city): %v", err)
	}
	_ = tx3.Commit(ctx)
	if !over3 {
		t.Errorf("gameOver = false after the last city fell; want true")
	}

	var finalStatus string
	var lastSettlement *uuid.UUID
	if err := pool.QueryRow(ctx,
		`SELECT status, last_settlement_id FROM player_world_records WHERE player_id=$1 AND world_id=$2`,
		playerID, worldID,
	).Scan(&finalStatus, &lastSettlement); err != nil {
		t.Fatalf("read records after game over: %v", err)
	}
	if finalStatus != "dispossessed" {
		t.Errorf("status = %q after last city fell; want dispossessed", finalStatus)
	}
	if lastSettlement == nil || *lastSettlement != loyalColony {
		t.Errorf("last_settlement_id not anchored to the fallen last city")
	}
}
