package handlers

import (
	"context"
	"testing"

	"formatet/megaron/server/internal/economy"
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
