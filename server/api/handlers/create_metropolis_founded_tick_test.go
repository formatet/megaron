package handlers

import (
	"context"
	"testing"

	"formatet/megaron/server/internal/economy"
	"github.com/google/uuid"
)

// Nådefristens substrat (mig 132): founded_tick måste stämplas på den väg en
// RIKTIG spelare tar, inte bara på den fixturerna tar.
//
// Fällan: join.go skapar redan en player_world_records-rad när spelaren går med i
// världen, långt före grundningen. createMetropolis kör därför alltid mot en
// befintlig rad och tar DO UPDATE-grenen. Stod founded_tick bara i VALUES vore
// kolumnen kvar på sin DEFAULT 0 för varenda spelare, nådefristen hade aldrig
// gällt någon, och ingenting hade sett annorlunda ut — därav det här testet.
//
// Världens tick sätts till 500 med flit: på en värld vid tick 0 är DEFAULT 0 och
// current_world_tick() omöjliga att skilja åt, och testet hade varit grönt under
// både rätt och fel kod.
func TestCreateMetropolis_StampsFoundedTickOnTheJoinUpsertPath(t *testing.T) {
	pool := foundingTestPool(t)
	ctx := context.Background()

	const worldTick = 500

	if _, err := pool.Exec(ctx,
		`UPDATE worlds SET status='archived' WHERE status = 'active'`,
	); err != nil {
		t.Fatalf("archive leftover test worlds: %v", err)
	}
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO worlds (name, status, current_tick) VALUES ($1, 'active', $2) RETURNING id`,
		"test-foundedtick-"+uuid.New().String(), worldTick,
	).Scan(&worldID); err != nil {
		t.Fatalf("create world: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `UPDATE worlds SET status='archived' WHERE id=$1`, worldID) })

	var playerID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO players (username, email, password_hash) VALUES ($1, $2, 'x') RETURNING id`,
		"foundedtick-"+uuid.New().String(), "foundedtick-"+uuid.New().String()+"@test.invalid",
	).Scan(&playerID); err != nil {
		t.Fatalf("create player: %v", err)
	}

	// Steg 1 — exakt vad join.go gör innan spelaren grundat något.
	if _, err := pool.Exec(ctx,
		`INSERT INTO player_world_records (player_id, world_id, status) VALUES ($1, $2, 'active')`,
		playerID, worldID,
	); err != nil {
		t.Fatalf("seed join row: %v", err)
	}

	for i, off := range foundingCatchmentOffsets {
		if _, err := pool.Exec(ctx,
			`INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, $3, 'plains')`,
			worldID, off[0], off[1],
		); err != nil {
			t.Fatalf("seed catchment tile %d: %v", i, err)
		}
	}

	// Steg 2 — grundningen, som alltså träffar DO UPDATE-grenen.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback(ctx)
	if _, err := createMetropolis(ctx, tx, economy.LoadSitosConfig(), metropolisParams{
		WorldID:    worldID,
		PlayerID:   playerID,
		Q:          0,
		R:          0,
		Terrain:    "plains",
		Name:       "Nadefristen-" + uuid.New().String(),
		Culture:    "achaean",
		Population: 1000,
	}); err != nil {
		t.Fatalf("createMetropolis: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit founding tx: %v", err)
	}

	var foundedTick int
	if err := pool.QueryRow(ctx,
		`SELECT founded_tick FROM player_world_records WHERE player_id=$1 AND world_id=$2`,
		playerID, worldID,
	).Scan(&foundedTick); err != nil {
		t.Fatalf("read founded_tick: %v", err)
	}
	if foundedTick != worldTick {
		t.Errorf("founded_tick = %d, vill ha %d — grundningen stämplade inte tick på "+
			"DO UPDATE-grenen, så nådefristen gäller ingen som gått via join.go",
			foundedTick, worldTick)
	}
}
