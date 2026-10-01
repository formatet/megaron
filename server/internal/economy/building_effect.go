package economy

// What a hex-bound building does on a hex (byggnadsregeln): one row per good the
// rule table ties to the building on that hex, computed by the SAME rule
// functions production uses (hex_rules.go). The two strings the API carries are
// formatted here so no surface recomputes them.

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// HexBuildEffect is one good's effect of a building on a hex.
type HexBuildEffect struct {
	Good          string
	PlacesWithout int     // worker places without the building (0 = the good cannot be produced without it: silver)
	RateWithout   float64 // r0, rate per worker without the building (0 when NeedsBuilding)
	PlacesWith    int     // P0 + BuildingExtraPlaces
	RateByLevel   [3]float64
	NeedsBuilding bool // silver: no base yield, the hex gives nothing without a mine
}

// HexBuildEffects returns the effect rows of buildingType on a hex of this
// terrain/deposits. r0 is read from production_rules: the building-free
// terrain/deposit row(s) for the good, or (silver) the mine row.
func HexBuildEffects(ctx context.Context, tx Tx, buildingType, terrain string, copperDep, tinDep, silverDep bool) ([]HexBuildEffect, error) {
	rules := hexRules(terrain, copperDep, tinDep, silverDep)
	var out []HexBuildEffect
	for _, rule := range rules {
		if rule.building != buildingType {
			continue
		}
		needs := rule.good == "silver"
		var r0 float64
		var err error
		if needs {
			err = tx.QueryRow(ctx,
				`SELECT COALESCE(SUM(pr.rate_per_tick), 0) FROM production_rules pr
				 JOIN goods g ON g.key = pr.good_key AND g.status = 'active'
				 WHERE pr.good_key = $1 AND pr.building_type = 'mine' AND pr.terrain_type = $2`,
				rule.good, terrain).Scan(&r0)
		} else {
			err = tx.QueryRow(ctx,
				`SELECT COALESCE(SUM(pr.rate_per_tick), 0) FROM production_rules pr
				 JOIN goods g ON g.key = pr.good_key AND g.status = 'active'
				 WHERE pr.good_key = $1 AND pr.building_type IS NULL AND pr.terrain_type = $2
				   AND (pr.requires_deposit IS NULL
				        OR (pr.requires_deposit = 'copper' AND $3) OR (pr.requires_deposit = 'tin' AND $4)
				        OR (pr.requires_deposit = 'silver' AND $5))`,
				rule.good, terrain, copperDep, tinDep, silverDep).Scan(&r0)
		}
		if err != nil {
			return nil, fmt.Errorf("hex build effects: %w", err)
		}
		if r0 <= 0 {
			continue // the rule names the good but the world has no production row for it here
		}
		e := HexBuildEffect{Good: rule.good, PlacesWith: rule.basePlaces + BuildingExtraPlaces, NeedsBuilding: needs}
		if !needs {
			e.PlacesWithout, e.RateWithout = rule.basePlaces, r0
		}
		for l := 1; l <= 3; l++ {
			e.RateByLevel[l-1] = r0 * (1.0 + BuildingRatePerLevel*float64(l))
		}
		out = append(out, e)
	}
	return out, nil
}

// fmtRate: two decimals, trailing zeros dropped but at least one decimal (1.0, 1.7, 0.33, 2.83).
func fmtRate(v float64) string {
	s := strconv.FormatFloat(math.Round(v*100)/100, 'f', -1, 64)
	if !strings.Contains(s, ".") {
		s += ".0"
	}
	return s
}

// levelMult is the per-worker multiplier a building of level l gives: 1 + 0.7·l.
func levelMult(l int) string {
	return "×" + fmtRate(1.0+BuildingRatePerLevel*float64(l))
}

// BuildEffectText is the "effect" string, in words (Timothy 2026-09-30):
// "grain production ×1.7 · space for 4 more workers"; silver, which has no
// base yield: "silver can be mined · space for 5 workers". Goods joined by "; ".
func BuildEffectText(effects []HexBuildEffect) string {
	parts := make([]string, 0, len(effects))
	for _, e := range effects {
		if e.NeedsBuilding {
			parts = append(parts, fmt.Sprintf("%s can be mined · space for %d workers", e.Good, e.PlacesWith))
			continue
		}
		parts = append(parts, fmt.Sprintf("%s production %s · space for %d more workers", e.Good, levelMult(1), e.PlacesWith-e.PlacesWithout))
	}
	return strings.Join(parts, "; ")
}

// UpgradeEffectText is the "upgrade_effect" string for a building at currentLevel:
// "grain production ×1.7 → ×2.4". Empty at max level (3) or with no rows.
func UpgradeEffectText(effects []HexBuildEffect, currentLevel int) string {
	if currentLevel < 1 || currentLevel >= 3 {
		return ""
	}
	parts := make([]string, 0, len(effects))
	for _, e := range effects {
		parts = append(parts, fmt.Sprintf("%s production %s → %s", e.Good, levelMult(currentLevel), levelMult(currentLevel+1)))
	}
	return strings.Join(parts, "; ")
}
