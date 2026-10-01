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

// TestRuleTable_NoBuildingLowersPerGubbe is the §3.2 rule (mig 136) restated for
// byggnadsregeln: a building must never make an individual gubbe produce LESS
// than he would on the bare terrain, nor shrink the hex. Over the whole rule
// table at every level, multiplier >= 1 and places grow by exactly
// BuildingExtraPlaces — the rule cannot lower anything by construction, and this
// pins that.
func TestRuleTable_NoBuildingLowersPerGubbe(t *testing.T) {
	for _, terrain := range append(append([]string{}, ruleTerrains...), "hills_with_deposits") {
		for _, rule := range hexRules(terrain, true, true, true) {
			p0, m0 := rule.placesAndMult(0)
			if m0 != 1.0 || p0 != rule.basePlaces {
				t.Errorf("%s/%s: unbuilt must be P0 x r0, got %d x %.2f", terrain, rule.good, p0, m0)
			}
			prevMult := m0
			for level := 1; level <= 3; level++ {
				p, m := rule.placesAndMult(level)
				if rule.building == "" {
					if p != p0 || m != 1.0 {
						t.Errorf("%s/%s has no building but level %d changed it", terrain, rule.good, level)
					}
					continue
				}
				if p != p0+BuildingExtraPlaces {
					t.Errorf("%s/%s level %d: places %d, want %d", terrain, rule.good, level, p, p0+BuildingExtraPlaces)
				}
				if m < prevMult || m < 1.0 {
					t.Errorf("%s/%s level %d: multiplier %.2f lowers the per-gubbe rate", terrain, rule.good, level, m)
				}
				prevMult = m
			}
		}
	}
}

// TestProductionRules_StandardTerrainYieldsOne is the dagsverkesskala itself:
// one gubbe on a good's standard terrain, with no building, produces 1,00 per
// tick (Timothy 2026-08-27). Since mig 155 every terrain row IS the rate per
// gubbe, so this reads the rate directly.
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
		if math.Abs(rate-1.0) > 0.01 {
			t.Errorf("%s on %s: %.4f per gubbe, expected 1,00 — the dagsverkesskala is the "+
				"reference every cost figure is read against (mig 136)", good, terrain, rate)
		}
	}
}

// TestProductionRules_NoBuildingRowsForRuleBuildings: mig 155 replaced the
// farm/harbour/lumbermill/stonequarry rows and mine's copper/tin/stone rows
// with the rule; only silver's mine rows may remain.
func TestProductionRules_NoBuildingRowsForRuleBuildings(t *testing.T) {
	pool := openScaleTestPool(t)
	defer pool.Close()
	for _, r := range loadProductionRules(t, pool) {
		switch r.building {
		case "farm", "harbour", "lumbermill", "stonequarry":
			t.Errorf("leftover %s row for %s on %q", r.building, r.good, r.terrain)
		case "mine":
			if r.good != "silver" {
				t.Errorf("leftover mine row for %s", r.good)
			}
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
