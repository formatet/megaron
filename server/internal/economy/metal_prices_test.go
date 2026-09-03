package economy

// Migration 139 fixes two things about metal prices that migration 136 left
// behind. This test locks the RELATIONS, never the literals: it reads the
// bronze recipe out of the database and recomputes, so the day someone
// rebalances the recipe the test still means what it says. A test that
// compares a constant to itself proves nothing.
//
// (1) Tin lost its premium. Mig 136 (dagsverkesskalan) multiplied every good's
//     base_value by its own old production rate so that value per MAN-DAY held
//     still. Before: copper 6, tin 12 — tin was worth double. After: both
//     172.8. That is consistent in its own frame (one man-day of copper equals
//     one man-day of tin) but it prices LABOUR, not SCARCITY — and tin's
//     scarcity is geological. The map generator already knows: copper scales
//     with the player count (copperSourceTarget = max(4, players/6)) while tin
//     is capped, and "productive tin" is by far the commonest reason a
//     generated world is rejected. The economy did not know.
//
// (2) Bronze was never priced at all. `grep bronze
//     136_dagsverkesskalan.up.sql` returns nothing — bronze comes from a
//     recipe, not a hex, so it had no production rate to scale, and nobody
//     priced the output afterwards. The result: a recipe eating 9 copper and
//     1 tin, each worth 172.8/unit, produced one bronze worth 20. The pricing
//     said smelting destroyed 92% of the value.

import (
	"context"
	"math"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func openMetalPriceTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set — skipping DB integration test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func baseValueOf(t *testing.T, pool *pgxpool.Pool, key string) float64 {
	t.Helper()
	var v float64
	if err := pool.QueryRow(context.Background(),
		`SELECT base_value FROM goods WHERE key = $1`, key).Scan(&v); err != nil {
		t.Fatalf("read base_value for %s: %v", key, err)
	}
	return v
}

// Tin is scarcer than copper on every map the generator produces, and the
// economy must say so. The ratio, not the numbers, is the contract.
func TestMetalPrices_TinIsWorthTwiceCopper(t *testing.T) {
	pool := openMetalPriceTestPool(t)
	copper := baseValueOf(t, pool, "copper")
	tin := baseValueOf(t, pool, "tin")

	if copper <= 0 {
		t.Fatalf("copper base_value = %v, want > 0 — the anchor cannot be zero", copper)
	}
	if got := tin / copper; math.Abs(got-2.0) > 1e-9 {
		t.Errorf("tin/copper = %.4f (tin %v, copper %v), want exactly 2 — tin is capped by the map "+
			"generator while copper scales with the player count, so it must not be priced as an equal",
			got, tin, copper)
	}
}

// Smelting must pay. Bronze is the chain's endpoint and additionally requires a
// sea voyage (copper and tin never share a landmass by design), so its output
// has to be worth more than the sum of what goes in. The ingredient value is
// read from the recipe so this stays true if the recipe changes.
func TestMetalPrices_BronzeIsWorthMoreThanItsIngredients(t *testing.T) {
	pool := openMetalPriceTestPool(t)
	ctx := context.Background()

	var ingredientValue float64
	if err := pool.QueryRow(ctx,
		`SELECT SUM(ri.quantity * g.base_value)
		   FROM recipes r
		   JOIN recipe_ingredients ri ON ri.recipe_id = r.id
		   JOIN goods g ON g.key = ri.good_key
		  WHERE r.output_key = 'bronze' AND r.building_type = 'foundry'`,
	).Scan(&ingredientValue); err != nil {
		t.Fatalf("read bronze recipe ingredient value: %v", err)
	}
	if ingredientValue <= 0 {
		t.Fatalf("bronze recipe ingredient value = %v — the recipe must exist and cost something",
			ingredientValue)
	}

	bronze := baseValueOf(t, pool, "bronze")
	if bronze <= ingredientValue {
		t.Fatalf("bronze base_value = %v but its ingredients are worth %v — smelting would destroy "+
			"value, so nobody would ever make bronze", bronze, ingredientValue)
	}

	// The margin is a rule (×1.5), not a number pulled from the air; migration
	// 139 derives it from this same query.
	if got := bronze / ingredientValue; math.Abs(got-1.5) > 1e-9 {
		t.Errorf("bronze/ingredients = %.4f, want 1.5 (the smelting margin migration 139 applies)", got)
	}
}

// Silver is the currency's anchor (numéraire) and migration 139 must not have
// touched it — every price in the game is denominated against it.
func TestMetalPrices_SilverAnchorUntouched(t *testing.T) {
	pool := openMetalPriceTestPool(t)
	if got := baseValueOf(t, pool, "silver"); got != 1 {
		t.Errorf("silver base_value = %v, want 1 — silver is the numéraire and must stay the anchor", got)
	}
}
