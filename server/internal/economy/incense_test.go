package economy

// Incense (order 32): a deposit-flagged scrub_maquis / semi_desert hex gives two
// worker places and 0.25 incense per worker per tick; the same terrain without
// the flag gives none. DB test (testPool).

import (
	"context"
	"math"
	"testing"

	"formatet/megaron/server/internal/hexgrid"
)

func TestHexRules_IncenseDepositGivesTwoPlacesNoBuilding(t *testing.T) {
	for _, terrain := range []string{"scrub_maquis", "semi_desert"} {
		places, mult := hexGoodPlaces(terrain, false, false, false, true, map[string]int{}, GoodIncense)
		if places != 2 || mult != 1.0 {
			t.Errorf("%s with deposit: incense places/mult = %d/%.1f, want 2/1.0", terrain, places, mult)
		}
		for _, r := range hexRules(terrain, false, false, false, false) {
			if r.good == GoodIncense {
				t.Errorf("%s without the deposit must not offer incense", terrain)
			}
		}
		for _, r := range hexRules(terrain, false, false, false, true) {
			if r.good == GoodIncense && r.building != "" {
				t.Errorf("incense must need no building, rule names %q", r.building)
			}
		}
	}
}

func TestIncense_WorkersOnDepositHexProduceIt(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	settlementID := seedFullRingFixture(t, 0, 300, "scrub_maquis")

	var worldID string
	if err := pool.QueryRow(ctx, `SELECT world_id FROM settlements WHERE id = $1`, settlementID).Scan(&worldID); err != nil {
		t.Fatalf("read world: %v", err)
	}
	ring := hexgrid.Ring(hexgrid.Coord{Q: 0, R: 0}, hexgrid.CatchmentRadius)
	depositHex, plainHex := ring[0], ring[1]
	if _, err := pool.Exec(ctx,
		`UPDATE map_tiles SET incense_deposit = true WHERE world_id = $1 AND q = $2 AND r = $3`,
		worldID, depositHex.Q, depositHex.R); err != nil {
		t.Fatalf("flag incense hex: %v", err)
	}

	opts, err := LoadHexProductionOptions(ctx, pool, settlementID, nil)
	if err != nil {
		t.Fatalf("LoadHexProductionOptions: %v", err)
	}
	for _, o := range opts {
		switch o.Coord {
		case depositHex:
			if o.CapPerGood[GoodIncense] != 2 || math.Abs(o.RatePerGood[GoodIncense]-0.25) > 1e-9 {
				t.Errorf("deposit hex: incense cap/rate = %d/%v, want 2/0.25", o.CapPerGood[GoodIncense], o.RatePerGood[GoodIncense])
			}
		case plainHex:
			if _, has := o.RatePerGood[GoodIncense]; has {
				t.Errorf("plain scrub hex must not offer incense")
			}
		}
	}

	placeHexGubbe(t, pool, settlementID, 1, depositHex, GoodIncense)
	placeHexGubbe(t, pool, settlementID, 2, depositHex, GoodIncense)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := RecomputeProduction(ctx, tx, settlementID); err != nil {
		tx.Rollback(ctx)
		t.Fatalf("RecomputeProduction: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var rate float64
	if err := pool.QueryRow(ctx,
		`SELECT rate FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'incense'`, settlementID,
	).Scan(&rate); err != nil {
		t.Fatalf("read incense row: %v", err)
	}
	if math.Abs(rate-0.5) > 1e-9 {
		t.Errorf("two workers on the incense hex: rate = %v per tick, want 0.5", rate)
	}
}

// The trickle row family (NULL terrain AND NULL building) is unconditional; incense
// rows are terrain-gated so no city gets incense for free.
func TestIncense_NotInTheUnconditionalTrickle(t *testing.T) {
	pool := testPool(t)
	got, err := UnconditionalPotential(context.Background(), pool)
	if err != nil {
		t.Fatal(err)
	}
	if v, has := got[GoodIncense]; has {
		t.Errorf("incense leaked into UnconditionalPotential: %v", v)
	}
}
