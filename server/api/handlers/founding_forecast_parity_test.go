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

			var actualRate float64
			if err := pool.QueryRow(ctx,
				`SELECT rate FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'grain'`, sid,
			).Scan(&actualRate); err != nil {
				t.Fatalf("load actual grain rate: %v", err)
			}

			// The metropolis preview always assumes a farm (starter_farm=1):
			// createMetropolis's own Demeter-gift comment establishes this is
			// always safe — on ground where a farm wouldn't help, assuming one
			// changes nothing (no farm-compatible terrain, no matching
			// production_rules row), so the with-farm assumption degrades
			// exactly to the building-free reality.
			_, forecastNet, err := economy.FoundingGrainNetPerTick(
				ctx, pool, worldID, center, map[string]int{"farm": 1}, nil, 4000)
			if err != nil {
				t.Fatalf("FoundingGrainNetPerTick: %v", err)
			}

			diff := math.Abs(forecastNet - actualRate)
			// 1% of the actual rate, with a small absolute floor so a
			// near-zero actual rate doesn't demand impossible precision.
			tolerance := math.Max(0.5, math.Abs(actualRate)*0.01)
			t.Logf("%s: forecast_net=%.4f actual_rate=%.4f diff=%.4f tolerance=%.4f",
				tc.name, forecastNet, actualRate, diff, tolerance)
			if diff > tolerance {
				t.Errorf("%s: forecast net %.4f vs actual rate %.4f — diff %.4f exceeds tolerance %.4f",
					tc.name, forecastNet, actualRate, diff, tolerance)
			}
		})
	}
}
