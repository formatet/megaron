package economy

import (
	"context"
	"testing"

	"formatet/megaron/server/internal/hexgrid"
	"github.com/google/uuid"
)

// TestPrunePlacementsToPopulation_S1bOrder is S1b's own acceptance criterion
// (megaron_plan_placeringsbeskarning.md AC2, megaron_plan_foda_konsistens.md
// §S1b LOCKED): a settlement with 1 hantverk/förädling placement, 1 tempel
// placement and 2 food placements, pruned down to floor(pop/100)=2, loses
// the craft and temple rows and keeps BOTH food rows.
func TestPrunePlacementsToPopulation_S1bOrder(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	settlementID := seedFullRingFixture(t, 100 /*pop*/, 250, "plains") // floor(250/100) = 2
	placeBuildingGubbe(t, pool, settlementID, 1, "foundry", GoodBronze)
	// A temple worker stands at a real temple (mig 202).
	if _, err := pool.Exec(ctx, `INSERT INTO buildings(settlement_id,building_type,level,god_id) VALUES($1,'temple',1,'potnia')`, settlementID); err != nil {
		t.Fatal(err)
	}
	placeBuildingGubbe(t, pool, settlementID, 2, "temple", GoodCult)
	placeHexGubbe(t, pool, settlementID, 3, hexRing0(t)[0], GoodGrain)
	placeHexGubbe(t, pool, settlementID, 4, hexRing0(t)[1], GoodGrain)

	liftedRows, err := PrunePlacementsToPopulation(ctx, pool, settlementID)
	if err != nil {
		t.Fatalf("PrunePlacementsToPopulation: %v", err)
	}
	pruned := len(liftedRows)
	if pruned != 2 {
		t.Errorf("pruned = %d, want 2 (bronze + cult)", pruned)
	}

	rows, err := pool.Query(ctx,
		`SELECT gubbe_ordinal, good_key FROM settlement_placement WHERE settlement_id = $1 ORDER BY gubbe_ordinal`,
		settlementID,
	)
	if err != nil {
		t.Fatalf("read survivors: %v", err)
	}
	defer rows.Close()
	var survivors []struct {
		ordinal int
		good    string
	}
	for rows.Next() {
		var s struct {
			ordinal int
			good    string
		}
		if err := rows.Scan(&s.ordinal, &s.good); err != nil {
			t.Fatalf("scan survivor: %v", err)
		}
		survivors = append(survivors, s)
	}
	if len(survivors) != 2 {
		t.Fatalf("survivors = %d, want 2", len(survivors))
	}
	for _, s := range survivors {
		if s.good != GoodGrain {
			t.Errorf("survivor ordinal %d has good_key %q, want %q — the fallback chain must never eat the last food",
				s.ordinal, s.good, GoodGrain)
		}
	}
}

// TestPrunePlacementsToPopulation_ConvergesInOnePass is AC3: an already
// badly-broken settlement (5 non-food placements against a cap of 2) is
// fixed in a SINGLE call, not one row per tick — the fix for the 6 live
// CT126 cities that had accumulated up to 300 phantom placements.
func TestPrunePlacementsToPopulation_ConvergesInOnePass(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	settlementID := seedFullRingFixture(t, 100 /*pop*/, 200, "plains") // floor(200/100) = 2
	hex := hexRing0(t)[0]
	for ordinal := 1; ordinal <= 5; ordinal++ {
		placeHexGubbe(t, pool, settlementID, ordinal, hex, GoodTimber)
	}

	liftedRows, err := PrunePlacementsToPopulation(ctx, pool, settlementID)
	if err != nil {
		t.Fatalf("PrunePlacementsToPopulation: %v", err)
	}
	pruned := len(liftedRows)
	if pruned != 3 {
		t.Errorf("pruned = %d, want 3 (5 placements down to cap 2, in one pass)", pruned)
	}

	var remaining int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM settlement_placement WHERE settlement_id = $1`, settlementID,
	).Scan(&remaining); err != nil {
		t.Fatalf("count remaining: %v", err)
	}
	if remaining != 2 {
		t.Fatalf("remaining = %d, want 2 (cap for pop 200)", remaining)
	}

	var survivingOrdinals []int
	rows, err := pool.Query(ctx,
		`SELECT gubbe_ordinal FROM settlement_placement WHERE settlement_id = $1 ORDER BY gubbe_ordinal`,
		settlementID,
	)
	if err != nil {
		t.Fatalf("read surviving ordinals: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var o int
		if err := rows.Scan(&o); err != nil {
			t.Fatalf("scan ordinal: %v", err)
		}
		survivingOrdinals = append(survivingOrdinals, o)
	}
	if len(survivingOrdinals) != 2 || survivingOrdinals[0] != 1 || survivingOrdinals[1] != 2 {
		t.Errorf("surviving ordinals = %v, want [1 2] (lowest ordinal within a tier survives longest)", survivingOrdinals)
	}
}

// TestPrunePlacementsToPopulation_IdempotentWhenWithinCap is AC4: a
// settlement already within its cap is left completely untouched — this
// function runs every tick for every active settlement, so a healthy city
// must never see a row disappear.
func TestPrunePlacementsToPopulation_IdempotentWhenWithinCap(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	settlementID := seedFullRingFixture(t, 100 /*pop*/, 500, "plains") // floor(500/100) = 5
	hexes := hexRing0(t)
	placeHexGubbe(t, pool, settlementID, 1, hexes[0], GoodGrain)
	placeHexGubbe(t, pool, settlementID, 2, hexes[1], GoodGrain)
	placeHexGubbe(t, pool, settlementID, 3, hexes[2], GoodGrain)

	liftedRows, err := PrunePlacementsToPopulation(ctx, pool, settlementID)
	if err != nil {
		t.Fatalf("PrunePlacementsToPopulation: %v", err)
	}
	pruned := len(liftedRows)
	if pruned != 0 {
		t.Errorf("pruned = %d, want 0 (3 placements, cap 5 — already within bounds)", pruned)
	}

	var remaining int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM settlement_placement WHERE settlement_id = $1`, settlementID,
	).Scan(&remaining); err != nil {
		t.Fatalf("count remaining: %v", err)
	}
	if remaining != 3 {
		t.Errorf("remaining = %d, want 3 (untouched)", remaining)
	}
}

// TestPlaceNextGubbeOnBestFoodHex_OrdinalAlreadyTakenFallsToPoolWithoutError
// covers §Kända fällor #1 (megaron_plan_placeringsbeskarning.md): growth's
// oldGubbar+1..newGubbar loop (kharis/tick.go,
// applyDecay growth) can hand PlaceNextGubbeOnBestFoodHex an ordinal a
// prune left occupied on a surviving row. That must degrade to "falls to
// the pool", the same as "every food slot full" — never a hard error that
// would abort the whole tick loop or fail a growth tick.
func TestPlaceNextGubbeOnBestFoodHex_OrdinalAlreadyTakenFallsToPoolWithoutError(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	settlementID := seedFullRingFixture(t, 100 /*pop*/, 500, "plains")
	placeHexGubbe(t, pool, settlementID, 7, hexRing0(t)[0], GoodGrain) // ordinal 7 already occupied

	placed, err := PlaceNextGubbeOnBestFoodHex(ctx, pool, settlementID, 7)
	if err != nil {
		t.Fatalf("PlaceNextGubbeOnBestFoodHex with a colliding ordinal must not error, got: %v", err)
	}
	if placed {
		t.Error("expected placed=false (ordinal already taken by a surviving row), got true")
	}

	var count int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM settlement_placement WHERE settlement_id = $1 AND gubbe_ordinal = 7`, settlementID,
	).Scan(&count); err != nil {
		t.Fatalf("count ordinal 7: %v", err)
	}
	if count != 1 {
		t.Errorf("rows at ordinal 7 = %d, want 1 (unchanged, no duplicate/overwrite attempt)", count)
	}
}

// hexRing0 returns the standard catchment ring around (0,0) —
// seedFullRingFixture always plants its settlement at province (0,0) — for
// tests that just need real, distinct hex_q/hex_r values and don't care
// which ring position they land on.
func hexRing0(t *testing.T) []hexgrid.Coord {
	t.Helper()
	ring := hexgrid.Ring(hexgrid.Coord{Q: 0, R: 0}, hexgrid.CatchmentRadius)
	if len(ring) < 3 {
		t.Fatalf("catchment ring too small for fixture use: %d hexes", len(ring))
	}
	return ring
}

// TestPrunePlacementsToPopulation_ReturnsLiftedRows: the rows come back with what
// each gubbe was doing, the S1b order picks refining before food, a second run is
// empty, and the temple row names its god.
func TestPrunePlacementsToPopulation_ReturnsLiftedRows(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	settlementID := seedFullRingFixture(t, 100, 300, "plains") // cap 3
	if _, err := pool.Exec(ctx, `INSERT INTO buildings(settlement_id,building_type,level,god_id) VALUES($1,'temple',1,'potnia')`, settlementID); err != nil {
		t.Fatal(err)
	}
	hexes := hexRing0(t)
	placeHexGubbe(t, pool, settlementID, 1, hexes[0], GoodGrain)
	placeBuildingGubbe(t, pool, settlementID, 2, "temple", GoodCult)
	placeBuildingGubbe(t, pool, settlementID, 3, "olive_press", GoodOil)
	placeHexGubbe(t, pool, settlementID, 4, hexes[1], GoodGrain)

	lifted, err := PrunePlacementsToPopulation(ctx, pool, settlementID)
	if err != nil {
		t.Fatal(err)
	}
	if len(lifted) != 1 {
		t.Fatalf("lifted = %d rows, want 1", len(lifted))
	}
	if l := lifted[0]; l.Ordinal != 3 || l.GoodKey != GoodOil || l.TargetKind != "building" || l.BuildingType != "olive_press" {
		t.Errorf("lifted = %+v, want the olive press oil gubbe (refining goes before food and temple)", l)
	}

	// Cap 2 now: temple goes next (before food) and names its god.
	if _, err := pool.Exec(ctx, `UPDATE settlements SET population = 250 WHERE id = $1`, settlementID); err != nil {
		t.Fatal(err)
	}
	lifted, err = PrunePlacementsToPopulation(ctx, pool, settlementID)
	if err != nil {
		t.Fatal(err)
	}
	if len(lifted) != 1 || lifted[0].GodID == nil || *lifted[0].GodID != "potnia" || lifted[0].GoodKey != GoodCult {
		t.Fatalf("lifted = %+v, want the temple gubbe with god potnia", lifted)
	}

	// Food lifted last, with its hex.
	if _, err := pool.Exec(ctx, `UPDATE settlements SET population = 100 WHERE id = $1`, settlementID); err != nil {
		t.Fatal(err)
	}
	lifted, err = PrunePlacementsToPopulation(ctx, pool, settlementID)
	if err != nil {
		t.Fatal(err)
	}
	if len(lifted) != 1 || lifted[0].Ordinal != 4 || lifted[0].HexQ == nil || *lifted[0].HexQ != hexes[1].Q || *lifted[0].HexR != hexes[1].R {
		t.Fatalf("lifted = %+v, want ordinal 4 at hex %v", lifted, hexes[1])
	}

	again, err := PrunePlacementsToPopulation(ctx, pool, settlementID)
	if err != nil || len(again) != 0 {
		t.Errorf("second run = %v, %v; want empty", again, err)
	}
}

type liftRecorder struct {
	kinds   []string
	payload []any
	level   []int
}

func (r *liftRecorder) NotifyPlayer(ctx context.Context, worldID, playerID uuid.UUID, kind string, level int, payload any) error {
	r.kinds = append(r.kinds, kind)
	r.payload = append(r.payload, payload)
	r.level = append(r.level, level)
	return nil
}

func TestAnnouncePlacementsLifted_TextAndNotification(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	settlementID := seedFullRingFixture(t, 100, 300, "plains")
	var worldID uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT world_id FROM settlements WHERE id=$1`, settlementID).Scan(&worldID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE settlements SET name='Gournia' WHERE id=$1`, settlementID); err != nil {
		t.Fatal(err)
	}
	q, r := 30, 45
	god := "potnia"
	name := func(string) string { return "Temple of Potnia" }

	rec := &liftRecorder{}
	AnnouncePlacementsLifted(ctx, pool, nil, rec, worldID, settlementID, LiftCauseDraft,
		[]LiftedPlacement{{Ordinal: 3, TargetKind: "hex", HexQ: &q, HexR: &r, GoodKey: GoodGrain}}, name)
	if len(rec.kinds) != 1 || rec.kinds[0] != "PlacementsLifted" {
		t.Fatalf("notifications = %v, want one PlacementsLifted", rec.kinds)
	}
	want := "A citizen left their work in Gournia: 1 on grain at (30, 45) (men were drafted)."
	if got := rec.payload[0].(PlacementsLiftedPayload).Text; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}

	if got := PlacementsLiftedText("Gournia", LiftCauseDecline, []LiftedPlacement{{TargetKind: "building", BuildingType: "olive_press", GoodKey: GoodOil}}, name); got !=
		"A citizen left their work in Gournia: 1 at the olive press (oil) (the city has fewer people than posts)." {
		t.Errorf("building text = %q", got)
	}
	if got := PlacementsLiftedText("Gournia", LiftCauseReinforcement, []LiftedPlacement{{TargetKind: "building", BuildingType: "temple", GoodKey: GoodCult, GodID: &god}}, name); got !=
		"A citizen left their work in Gournia: 1 serving at the temple of Potnia (men were sent to reinforce a unit)." {
		t.Errorf("temple text = %q", got)
	}
	multi := PlacementsLiftedText("Gournia", LiftCauseDraft, []LiftedPlacement{
		{TargetKind: "hex", HexQ: &q, HexR: &r, GoodKey: GoodGrain},
		{TargetKind: "hex", HexQ: &q, HexR: &r, GoodKey: GoodGrain},
		{TargetKind: "building", BuildingType: "olive_press", GoodKey: GoodOil},
	}, name)
	if multi != "3 citizens left their work in Gournia: grain at (30, 45) ×2, olive press (oil) ×1 (men were drafted)." {
		t.Errorf("multi text = %q", multi)
	}

	none := &liftRecorder{}
	AnnouncePlacementsLifted(ctx, pool, nil, none, worldID, settlementID, LiftCauseDecline, nil, name)
	if len(none.kinds) != 0 {
		t.Errorf("empty lift notified: %v", none.kinds)
	}
}
