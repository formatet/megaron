package economy

import (
	"context"
	"math"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Dagsverkesskalans två invarianter, mätta mot den RIKTIGA production_rules-
// tabellen (megaron_plan_dagsverkesskalan §3, mig 136).
//
// Båda handlar om samma storhet — rate_per_tick / capL1, alltså vad EN gubbe
// producerar per tick — och den storheten går inte att läsa ur en enskild rad:
// den beror på hexens kapacitetsregel, som bor i Go. Därför ett DB-test och
// inte en ren tabellkoll.

func openScaleTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set — skipping DB integration test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	return pool
}

// capL1For returns the level-1 worker capacity a (terrain, building) pair gives
// for one good — the divisor placementYield uses. Mirrors hexGoodCaps' capOf
// for the hex path and workplaceSlotTable for the building path.
//
// Grain is excluded by the callers: hexGoodCaps pins its capL1 to 1
// unconditionally, so its rate_per_tick IS its per-gubbe figure and the
// building-vs-terrain comparison below works on the raw rates instead.
func capL1For(t *testing.T, good, terrain, building string) int {
	t.Helper()
	if terrain == "" {
		// Building-only rule (bronze, pottery, horses, the terrainless
		// lumbermill/winery/olive_press rows): capacity is the workplace's own
		// level-1 slots.
		return WorkplaceSlots(building, 1)
	}
	var rule hexCapacityRule
	var found bool
	if terrain == "plains" {
		for _, r := range plainsCapacityRules {
			if r.goodKey == good {
				rule, found = r, true
			}
		}
	} else if r, ok := terrainCapacityTable[terrain]; ok && r.goodKey == good {
		rule, found = r, true
	}
	if !found {
		for _, r := range depositCapacityTable {
			if r.goodKey == good {
				rule, found = r, true
			}
		}
	}
	if !found {
		// oil, wine and stone have no P3 rule and fall back to a flat cap.
		return HexFallbackCap
	}
	if building == "" || rule.relevantBuilding != building {
		return rule.capNoBuilding
	}
	return rule.capWithBuilding + WorkplaceSlots(building, 1)
}

type ruleRow struct {
	good, terrain, building string
	rate                    float64
}

func loadProductionRules(t *testing.T, pool *pgxpool.Pool) []ruleRow {
	t.Helper()
	rows, err := pool.Query(context.Background(),
		`SELECT good_key, COALESCE(terrain_type,''), COALESCE(building_type,''), rate_per_tick
		   FROM production_rules`)
	if err != nil {
		t.Fatalf("query production_rules: %v", err)
	}
	defer rows.Close()
	var out []ruleRow
	for rows.Next() {
		var r ruleRow
		if err := rows.Scan(&r.good, &r.terrain, &r.building, &r.rate); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out = append(out, r)
	}
	return out
}

// TestProductionRules_NoBuildingLowersPerGubbe is the §3.2 rule: a building must
// never make an individual gubbe produce LESS than he would on the bare terrain.
//
// Before mig 136 this was violated across half the catalogue — fish 86,4 → 21,6
// with a harbour, cedar 72 → 36 with a lumbermill, copper 28,8 → 11,52 with a
// mine, tin 14,4 → 7,2. The cause is structural, not a typo: a building raises
// the hex's capL1 (capWithBuilding + WorkplaceSlots) while rate_per_tick stays
// put, so total output rises only if you can fill the new slots — and the city
// that cannot pays for the building by making every existing worker worth less.
// That is involution, not intensification, and it makes the build a bad move for
// exactly the cities that need help most.
//
// This test caught a fifth violation the migration's own author missed: the
// terrainless timber/lumbermill row, whose capL1 comes from workplaceSlotTable
// rather than a hex rule and so escaped the first sweep.
func TestProductionRules_NoBuildingLowersPerGubbe(t *testing.T) {
	pool := openScaleTestPool(t)
	defer pool.Close()

	all := loadProductionRules(t, pool)

	// Bare-terrain baseline per (good, terrain).
	base := map[[2]string]float64{}
	for _, r := range all {
		if r.building == "" && r.terrain != "" {
			base[[2]string{r.good, r.terrain}] = r.rate / float64(capL1For(t, r.good, r.terrain, ""))
		}
	}

	for _, r := range all {
		if r.building == "" || r.terrain == "" {
			continue // bare terrain, or a terrainless building rule with no baseline to beat
		}
		baseline, ok := base[[2]string{r.good, r.terrain}]
		if !ok {
			continue // building unlocks a good this terrain cannot produce alone
		}
		perGubbe := r.rate / float64(capL1For(t, r.good, r.terrain, r.building))
		if r.good == GoodGrain {
			// capL1 is pinned to 1 for grain, so rate IS the per-gubbe figure.
			perGubbe, baseline = r.rate, base[[2]string{r.good, r.terrain}]*float64(capL1For(t, r.good, r.terrain, ""))
		}
		if perGubbe < baseline-1e-9 {
			t.Errorf("%s on %s with %s: %.4f per gubbe, LOWER than %.4f without the building — "+
				"a building must never make a worker less productive (mig 136, plan §3.2)",
				r.good, r.terrain, r.building, perGubbe, baseline)
		}
	}
}

// TestProductionRules_StandardTerrainYieldsOne is the dagsverkesskalan itself:
// one gubbe on a good's standard terrain, with no building, produces 1,00 per
// tick. That is what makes every cost figure elsewhere in the codebase readable
// as a count of man-days (Timothy 2026-08-27).
//
// The standard terrain is the good's BEST bare-terrain hex — the one the
// divisor in mig 136 was derived from.
func TestProductionRules_StandardTerrainYieldsOne(t *testing.T) {
	pool := openScaleTestPool(t)
	defer pool.Close()

	standard := map[string]string{
		GoodGrain:     "plains",
		GoodFish:      "coastal_sea",
		GoodTimber:    "forest_olive_grove",
		GoodCedar:     "forest_cedar",
		GoodLivestock: "plains",
		GoodStone:     "hills",
	}

	byKey := map[[3]string]float64{}
	for _, r := range loadProductionRules(t, pool) {
		byKey[[3]string{r.good, r.terrain, r.building}] = r.rate
	}

	for good, terrain := range standard {
		rate, ok := byKey[[3]string{good, terrain, ""}]
		if !ok {
			t.Errorf("%s: no bare-terrain rule for its standard terrain %s", good, terrain)
			continue
		}
		perGubbe := rate / float64(capL1For(t, good, terrain, ""))
		if good == GoodGrain {
			perGubbe = rate // capL1 pinned to 1
		}
		if math.Abs(perGubbe-1.0) > 0.01 {
			t.Errorf("%s on %s: %.4f per gubbe, expected 1,00 — the dagsverkesskala is the "+
				"reference every cost figure is read against (mig 136)", good, terrain, perGubbe)
		}
	}
}

// TestProductionRules_NoUnconditionalFlows guards Timothy's 2026-08-27 decision:
// the only thing a settlement gets without a gubbe working for it is the city
// hex's own grain ration (economy.NearjordGrainPerTick, a Go constant — not a
// production_rules row). Timber's 144/tick trickle and purple's 21,6/tick were
// removed by mig 136; they were the whole reason cities held 14 000–58 000 of
// goods nobody produced.
func TestProductionRules_NoUnconditionalFlows(t *testing.T) {
	pool := openScaleTestPool(t)
	defer pool.Close()

	for _, r := range loadProductionRules(t, pool) {
		if r.terrain == "" && r.building == "" {
			t.Errorf("%s has an unconditional production rule (%.4f/tick) — every good must be "+
				"worked for; the city hex's grain ration is a Go constant, not a rule row", r.good, r.rate)
		}
	}
}
