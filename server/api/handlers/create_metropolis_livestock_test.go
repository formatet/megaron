package handlers

import (
	"context"
	"testing"

	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/province"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestFounding_Metropolis_SeedsStartingHerd verifies S1d
// (megaron_plan_foda_konsistens.md §S1d, Timothy 2026-08-07: "om det är något
// nomader har så är det boskap"): a newly-founded metropolis starts with a
// livestock stock instead of zero — the Nomadic Host's carried herd. The
// figure is economy.FoundingHerdLivestock (a calibration ratt, not a lock).
func TestFounding_Metropolis_SeedsStartingHerd(t *testing.T) {
	terrains := [7]string{"mountain_limestone", "mountain_limestone", "mountain_limestone", "mountain_limestone", "mountain_limestone", "mountain_limestone", "mountain_limestone"}
	pool, sid := foundMetropolisFixture(t, terrains)

	amount := foundingLivestockAmount(t, pool, sid)
	if amount != float64(economy.FoundingHerdLivestock) {
		t.Errorf("expected livestock=%d at founding, got %v", economy.FoundingHerdLivestock, amount)
	}
}

func foundingLivestockAmount(t *testing.T, pool *pgxpool.Pool, settlementID uuid.UUID) float64 {
	t.Helper()
	var amount float64
	if err := pool.QueryRow(context.Background(),
		`SELECT amount FROM settlement_goods WHERE settlement_id=$1 AND good_key='livestock'`,
		settlementID,
	).Scan(&amount); err != nil {
		t.Fatalf("load livestock amount: %v", err)
	}
	return amount
}

// TestFounding_Metropolis_StartsWithoutTimberOrStone — megaron_plan_byggkostnader
// steg 3 / invariant 4: a new city's stock cannot pay for any recipe in the
// catalogue (all need timber or stone), so the catchment has to be worked first.
// The colony path (combat.foundColony) carries a second copy of the same CASE
// and shares economy.ColonyGrainSeed.
func TestFounding_Metropolis_StartsWithoutTimberOrStone(t *testing.T) {
	terrains := [7]string{"plains", "plains", "plains", "plains", "plains", "plains", "plains"}
	pool, sid := foundMetropolisFixture(t, terrains)

	stock := map[string]float64{}
	rows, err := pool.Query(context.Background(),
		`SELECT good_key, amount FROM settlement_goods WHERE settlement_id=$1`, sid)
	if err != nil {
		t.Fatalf("load stock: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var k string
		var a float64
		if err := rows.Scan(&k, &a); err != nil {
			t.Fatal(err)
		}
		stock[k] = a
	}
	if stock["timber"] != 0 || stock["stone"] != 0 {
		t.Errorf("founding stock timber=%v stone=%v, want 0 and 0", stock["timber"], stock["stone"])
	}
	if stock["grain"] != 8 {
		t.Errorf("founding grain = %v, want 8", stock["grain"])
	}
	for bt, spec := range province.BuildingSpecs {
		afford := true
		for g, c := range spec.Costs {
			if stock[g] < c {
				afford = false
			}
		}
		if afford {
			t.Errorf("%s is affordable from the founding stock — the bootstrap rule is broken", bt)
		}
	}
}
