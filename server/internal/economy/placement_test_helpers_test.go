package economy

import (
	"context"
	"testing"

	"formatet/megaron/server/internal/hexgrid"
	"github.com/google/uuid"
)

// placeHexGubbe inserts one settlement_placement row for a hex target — the
// test-fixture equivalent of a Wanax clicking "place" in the P5 stadsvy that
// doesn't exist yet. gubbeOrdinal must be unique per settlement across every
// placeHexGubbe/placeBuildingGubbe call in a test (settlement_placement's
// UNIQUE(settlement_id, gubbe_ordinal) constraint).
func placeHexGubbe(t *testing.T, pool Tx, settlementID uuid.UUID, gubbeOrdinal int, hex hexgrid.Coord, goodKey string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO settlement_placement (settlement_id, gubbe_ordinal, target_kind, hex_q, hex_r, good_key)
		 VALUES ($1, $2, 'hex', $3, $4, $5)`,
		settlementID, gubbeOrdinal, hex.Q, hex.R, goodKey,
	); err != nil {
		t.Fatalf("place hex gubbe #%d at (%d,%d) for %s: %v", gubbeOrdinal, hex.Q, hex.R, goodKey, err)
	}
}

// placeBuildingGubbe is placeHexGubbe's building-slot sibling.
func placeBuildingGubbe(t *testing.T, pool Tx, settlementID uuid.UUID, gubbeOrdinal int, buildingType, goodKey string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO settlement_placement (settlement_id, gubbe_ordinal, target_kind, building_type, good_key)
		 VALUES ($1, $2, 'building', $3, $4)`,
		settlementID, gubbeOrdinal, buildingType, goodKey,
	); err != nil {
		t.Fatalf("place building gubbe #%d in %s for %s: %v", gubbeOrdinal, buildingType, goodKey, err)
	}
}

// placeFullRing fills EVERY hex of the settlement's 18-hex ring (center
// Q,R) with capPerHex gubbar each, all doing goodKey — "fully staffed
// catchment" for tests that assert a hex-capped good's ceiling.
// gubbeOrdinal starts at startOrdinal and increments per placed gubbe.
func placeFullRing(t *testing.T, pool Tx, settlementID uuid.UUID, center hexgrid.Coord, goodKey string, capPerHex, startOrdinal int) {
	t.Helper()
	ordinal := startOrdinal
	for _, hex := range hexgrid.Ring(center, hexgrid.CatchmentRadius) {
		for i := 0; i < capPerHex; i++ {
			placeHexGubbe(t, pool, settlementID, ordinal, hex, goodKey)
			ordinal++
		}
	}
}
