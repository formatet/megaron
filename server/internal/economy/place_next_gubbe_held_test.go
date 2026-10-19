package economy

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// Haiku-7: Kydonia's growth gubbe was placed on a hex Tylissos already
// worked, and each city then named the other as holder. Automatic placement
// must skip a hex another city holds.
func TestPlaceNextGubbeOnBestFoodHex_SkipsHexHeldByAnotherCity(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	settlementID := seedFullRingFixture(t, 100, 1000, "plains")

	// Where would the first gubbe go with no neighbour?
	if placed, err := PlaceNextGubbeOnBestFoodHex(ctx, pool, settlementID, 1); err != nil || !placed {
		t.Fatalf("first placement: placed=%v err=%v", placed, err)
	}
	var bestQ, bestR int
	if err := pool.QueryRow(ctx,
		`DELETE FROM settlement_placement WHERE settlement_id = $1 RETURNING hex_q, hex_r`, settlementID,
	).Scan(&bestQ, &bestR); err != nil {
		t.Fatalf("read best hex: %v", err)
	}

	// A neighbouring city now works that hex.
	var neighbourID uuid.UUID
	if err := pool.QueryRow(ctx,
		`WITH p AS (
		   INSERT INTO provinces (world_id, map_q, map_r, terrain_type)
		   SELECT world_id, 4, 0, 'plains' FROM settlements WHERE id = $1 RETURNING id, world_id)
		 INSERT INTO settlements (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, population)
		 SELECT p.world_id, p.id, 'Neighbour', 'achaean', s.owner_id, 'colony', false, 100
		 FROM p, settlements s WHERE s.id = $1 RETURNING id`,
		settlementID,
	).Scan(&neighbourID); err != nil {
		t.Fatalf("create neighbour: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_placement (settlement_id, gubbe_ordinal, target_kind, hex_q, hex_r, good_key)
		 VALUES ($1, 1, 'hex', $2, $3, 'grain')`,
		neighbourID, bestQ, bestR,
	); err != nil {
		t.Fatalf("neighbour placement: %v", err)
	}

	if placed, err := PlaceNextGubbeOnBestFoodHex(ctx, pool, settlementID, 1); err != nil || !placed {
		t.Fatalf("placement beside neighbour: placed=%v err=%v", placed, err)
	}
	var q, r int
	if err := pool.QueryRow(ctx,
		`SELECT hex_q, hex_r FROM settlement_placement WHERE settlement_id = $1`, settlementID,
	).Scan(&q, &r); err != nil {
		t.Fatalf("read placement: %v", err)
	}
	if q == bestQ && r == bestR {
		t.Fatalf("growth gubbe placed on (%d,%d), which the neighbour already holds", q, r)
	}
}
