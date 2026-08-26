package handlers

import (
	"context"
	"math"
	"testing"

	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/hexgrid"
	"github.com/google/uuid"
)

// TestFoundingGrainForecast_MatchesRealFounding is Slice B's acceptance
// criterion 1 (megaron_plan_grundningsprognosen.md §3): /colonize-preview's
// grain forecast must match what a real founding actually produces, to
// within 1%, because both now run through the SAME formula
// (economy.rankedFoodSlotsAt + economy.placeGreedyOnFoodSlots via
// economy.FoundingGrainNetPerTick / economy.PlaceStartingWorkforce) instead
// of the pre-P4 linear estimate (0,85×pop/REF_LABOR, uncapped, no per-hex
// tak) the plan measured promising 490-85 190/tick against a real
// ~2 447/tick outcome for the SAME 4 000-people catchment (2026-08-25,
// kodläst — see the plan's §1 table).
//
// Three catchments, per the plan's requirement not to trust a single
// fixture: a wheat-friendly plain (grain plentiful, self-sufficient
// quickly), a river valley/delta (even richer grain), and a barren coastal
// site with almost no farmland (forces the self-sufficiency-limited /
// hungry branch — fish covering most of the demand).
//
// RED-BEFORE (recorded 2026-08-26, old dead formula reproduced verbatim
// against this SAME live fixture data — see the commit message for the full
// method and the three numbers): the old 0,85×pop/REF_LABOR estimate was off
// by roughly 30-300x on these three catchments. GREEN-AFTER: all three
// within the 1% tolerance below.
func TestFoundingGrainForecast_MatchesRealFounding(t *testing.T) {
	cases := []struct {
		name     string
		terrains [7]string
	}{
		{
			name:     "slatt",
			terrains: [7]string{"plains", "plains", "plains", "plains", "plains", "plains", "plains"},
		},
		{
			name:     "floddal_delta",
			terrains: [7]string{"plains", "river_valley", "river_valley", "river_delta", "river_delta", "mountain_limestone", "mountain_limestone"},
		},
		{
			name:     "kust_lite_aker",
			terrains: [7]string{"plains", "mountain_limestone", "mountain_limestone", "mountain_limestone", "mountain_limestone", "mountain_limestone", "coastal_sea"},
		},
	}

	// The fixture founds a metropolis at this population; the forecast must be
	// asked about the same one, or the two sides answer different questions.
	const pop = 4000

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pool, sid := foundMetropolisFixture(t, tc.terrains)
			ctx := context.Background()

			var worldID uuid.UUID
			var q, r int
			if err := pool.QueryRow(ctx,
				`SELECT prov.world_id, prov.map_q, prov.map_r
				 FROM settlements s JOIN provinces prov ON prov.id = s.province_id
				 WHERE s.id = $1`, sid,
			).Scan(&worldID, &q, &r); err != nil {
				t.Fatalf("load settlement coords: %v", err)
			}
			center := hexgrid.Coord{Q: q, R: r}

			var actualRate, actualFishRate float64
			if err := pool.QueryRow(ctx,
				`SELECT rate FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'grain'`, sid,
			).Scan(&actualRate); err != nil {
				t.Fatalf("load actual grain rate: %v", err)
			}
			// Fish may legitimately be absent (an inland catchment has no
			// fish row) — a missing row is a zero rate, not a failure.
			if err := pool.QueryRow(ctx,
				`SELECT COALESCE((SELECT rate FROM settlement_goods
				                  WHERE settlement_id = $1 AND good_key = 'fish'), 0)`, sid,
			).Scan(&actualFishRate); err != nil {
				t.Fatalf("load actual fish rate: %v", err)
			}

			// The metropolis preview always assumes a farm (starter_farm=1):
			// createMetropolis's own Demeter-gift comment establishes this is
			// always safe — on ground where a farm wouldn't help, assuming one
			// changes nothing (no farm-compatible terrain, no matching
			// production_rules row), so the with-farm assumption degrades
			// exactly to the building-free reality.
			forecastProd, forecastNet, err := economy.FoundingGrainNetPerTick(
				ctx, pool, worldID, center, map[string]int{"farm": 1}, nil, pop)
			if err != nil {
				t.Fatalf("FoundingGrainNetPerTick: %v", err)
			}

			// ── The claim: forecast PRODUCTION == the real founding's rate ──
			// Since Utfodringsordningen D1 (2026-08-26) settlement_goods.rate
			// is RAW production — the population's meal is debited from stock
			// by FoodTick, not folded into the rate. So the quantity that must
			// match the stored rate is the forecast's GROSS term, and this
			// comparison shares no food-split code with the forecast at all:
			// one side is the real PlaceStartingWorkforce writing through
			// RecomputeProduction, the other is the forecast's own greedy run.
			// That is exactly the placement math this slice replaced, and it
			// is why this assertion is not circular.
			// ⚠️ Do NOT compare forecastNet against actualRate — it passed
			// before D1 only because the stored rate was itself net, and it
			// would now fail by the population's whole daily need (2 000/tick
			// at 4 000 invånare). That regression is what caught this at merge.
			diff := math.Abs(forecastProd - actualRate)
			// 1% of the actual rate, with a small absolute floor so a
			// near-zero actual rate doesn't demand impossible precision.
			tolerance := math.Max(0.5, math.Abs(actualRate)*0.01)
			t.Logf("%s: forecast_prod=%.4f forecast_net=%.4f actual_rate=%.4f diff=%.4f tol=%.4f",
				tc.name, forecastProd, forecastNet, actualRate, diff, tolerance)
			if diff > tolerance {
				t.Errorf("%s: forecast production %.4f vs actual rate %.4f — diff %.4f exceeds tolerance %.4f",
					tc.name, forecastProd, actualRate, diff, tolerance)
			}

			// ── The projection: net is what the player is actually shown ──
			// It is the forecast's answer to "will this site feed my people?",
			// so it must equal the food invariant applied to the SAME stored
			// rates the city really got (grain first, fish for the remainder).
			// Weaker than the check above (it shares FoodConsumptionSplit),
			// but it catches the forecast netting against the wrong inputs.
			wantNet, _, _ := economy.FoodConsumptionSplit(
				economy.GrainConsumptionPerTick(pop), actualRate, actualFishRate, 0)
			if netDiff := math.Abs(forecastNet - wantNet); netDiff > tolerance {
				t.Errorf("%s: forecast net %.4f vs food-invariant net %.4f on the real rates — diff %.4f exceeds %.4f",
					tc.name, forecastNet, wantNet, netDiff, tolerance)
			}
		})
	}
}
