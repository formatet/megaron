package world

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRiverMetadataBackfillIsAtomicAndPreservesSavedValues(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var id uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO worlds(name,status,map_width,map_height) VALUES('Saved river metadata','archived',9,9) RETURNING id`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM worlds WHERE id=$1`, id)
	changes := map[[2]int]Terrain{{1, 7}: TerrainCoastalSea, {7, 8}: TerrainRiverDelta}
	for q := 2; q <= 7; q++ {
		changes[[2]int{q, 7}] = TerrainRiver
	}
	for _, tile := range riverFlowFixture(changes) {
		if _, err := pool.Exec(ctx, `INSERT INTO map_tiles(world_id,q,r,terrain) VALUES($1,$2,$3,$4)`, id, tile.Q, tile.R, tile.Terrain); err != nil {
			t.Fatal(err)
		}
	}
	// Force a late write to fail after earlier cells were updated inside the TX.
	constraint := "river_backfill_probe_" + uuid.New().String()[:8]
	if _, err := pool.Exec(ctx, fmt.Sprintf(`ALTER TABLE map_tiles ADD CONSTRAINT %s CHECK(world_id<>'%s'::uuid OR q<>7 OR river_edges IS NULL)`, constraint, id)); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `ALTER TABLE map_tiles DROP CONSTRAINT IF EXISTS `+constraint)
	if err := CompleteRiverMetadata(ctx, pool); err == nil {
		t.Fatal("forced backfill failure was ignored")
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM map_tiles WHERE world_id=$1 AND river_edges IS NOT NULL`, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("failed backfill left %d partially written cells", n)
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE map_tiles DROP CONSTRAINT `+constraint); err != nil {
		t.Fatal(err)
	}
	if err := CompleteRiverMetadata(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM map_tiles WHERE world_id=$1 AND terrain='river' AND river_edges IS NOT NULL AND river_flow_known`, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 6 {
		t.Fatalf("saved direction on %d cells, want6", n)
	}
	// A new missing row can be filled, while an existing saved direction wins.
	if _, err := pool.Exec(ctx, `UPDATE map_tiles SET river_upstream_edges=1,river_downstream_edges=2 WHERE world_id=$1 AND q=3 AND r=7`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE map_tiles SET river_edges=NULL,river_upstream_edges=NULL,river_downstream_edges=NULL,river_flow_known=NULL WHERE world_id=$1 AND q=4 AND r=7`, id); err != nil {
		t.Fatal(err)
	}
	if err := CompleteRiverMetadata(ctx, pool); err != nil {
		t.Fatal(err)
	}
	var up, down int16
	if err := pool.QueryRow(ctx, `SELECT river_upstream_edges,river_downstream_edges FROM map_tiles WHERE world_id=$1 AND q=3 AND r=7`, id).Scan(&up, &down); err != nil {
		t.Fatal(err)
	}
	if up != 1 || down != 2 {
		t.Fatalf("startup rewrote saved direction: %d/%d", up, down)
	}
	if err := CompleteRiverMetadata(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM map_tiles WHERE world_id=$1 AND terrain='river' AND river_edges IS NULL`, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("%d cells still lack saved geography", n)
	}
}

func TestGeneratedRiversCarryFinalStaticMetadata(t *testing.T) {
	tiles, _ := GenerateMap(stubID{}, 12345, 40, 30)
	expected := RiverEdges(tiles)
	if len(expected) == 0 {
		t.Fatal("fixture has no rivers")
	}
	for _, tile := range tiles {
		edges, up, down, known := tile.RiverMetadataValues()
		if want, ok := expected[[2]int{tile.Q, tile.R}]; ok {
			if edges == nil || up == nil || down == nil || known == nil {
				t.Fatalf("generated river %d,%d lacks metadata", tile.Q, tile.R)
			}
			if *edges != riverEdgeMask(want) {
				t.Fatalf("generated river %d,%d has stale final edges", tile.Q, tile.R)
			}
		} else if edges != nil || up != nil || down != nil || known != nil {
			t.Fatalf("non-river %s carries river metadata", tile.Terrain)
		}
	}
}
