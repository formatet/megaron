package economy

// Building effects (megaron_plan_byggnad_pa_hex.md §B, B1): computes, per
// building type, exactly what a citizen placed at its site produces —
// without the building, and at levels 1..3 — using the SAME functions the
// real production path uses (hexGoodCaps, placementYield, WorkplaceSlots).
// This file never invents its own copy of that math; it only decides WHICH
// production_rules rows apply to a given (terrain, deposit) site, using the
// identical row filter LoadHexProductionOptionsAt's SQL applies (see
// rowAppliesToSite below) — if that SQL's WHERE clause ever changes, this
// predicate must change with it (mirror comment, same pattern
// recompute.go's workplaceSlotTable/HexBoundBuildingTypes already uses for
// province's copies).
//
// The player-facing problem this closes: BuildingPurposes (province/building.go)
// used to hand-write claims like "Farm raises grain and oil" that can drift
// from the actual production_rules/capacity-table data. From this slice on,
// every claim about what a building produces is a NUMBER computed here, not
// prose written by hand.

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
)

// waterTerrains mirrors LoadHexProductionOptionsAt's final WHERE clause
// literal list — a water-terrain hex only matches a production_rules row
// that names ITS OWN terrain explicitly; a NULL-terrain ("any terrain") row
// never reaches water. Kept as its own copy for the same G1 reason
// recompute.go's HexBoundBuildingTypes mirror exists: this predicate must
// reproduce the SQL exactly, not reinterpret it.
var waterTerrains = map[string]bool{
	"deep_sea":    true,
	"coastal_sea": true,
	"river":       true,
	"river_ford":  true,
}

// productionRuleRow is one production_rules row, restricted to ACTIVE goods
// (the same `g.status = 'active'` join LoadHexProductionOptionsAt and
// LoadBuildingProductionOptions use) — a parked good (pottery, horses,
// purple) never produces an effect row.
type productionRuleRow struct {
	hasTerrain      bool
	terrain         string
	hasBuilding     bool
	buildingType    string
	goodKey         string
	rate            float64
	requiresCoastal bool
	requiresDeposit string // "" | "copper" | "tin" | "silver" | "cedar"
}

// loadActiveProductionRules loads every production_rules row naming an
// active good, once. Ordered by id for a deterministic scan order —
// candidateHexSites and the effect builders below re-sort their own output,
// but a stable input order keeps behaviour reproducible if that ever changes.
func loadActiveProductionRules(ctx context.Context, tx Tx) ([]productionRuleRow, error) {
	rows, err := tx.Query(ctx,
		`SELECT pr.terrain_type, pr.building_type, pr.good_key, pr.rate_per_tick,
		        pr.requires_coastal, COALESCE(pr.requires_deposit, '')
		 FROM production_rules pr
		 JOIN goods g ON g.key = pr.good_key AND g.status = 'active'
		 ORDER BY pr.id`,
	)
	if err != nil {
		return nil, fmt.Errorf("load active production rules: %w", err)
	}
	defer rows.Close()
	var out []productionRuleRow
	for rows.Next() {
		var terrain, buildingType *string
		var r productionRuleRow
		if err := rows.Scan(&terrain, &buildingType, &r.goodKey, &r.rate, &r.requiresCoastal, &r.requiresDeposit); err != nil {
			return nil, fmt.Errorf("load active production rules: scan: %w", err)
		}
		if terrain != nil {
			r.hasTerrain = true
			r.terrain = *terrain
		}
		if buildingType != nil {
			r.hasBuilding = true
			r.buildingType = *buildingType
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load active production rules: rows: %w", err)
	}
	return out, nil
}

// rowAppliesToSite reports whether row would be included by
// LoadHexProductionOptionsAt's SQL for a hex with the given terrain,
// coastal flag and deposit flags — the terrain/coastal/deposit gate ONLY.
// Building membership (whether row.buildingType is actually built here) is
// checked separately by the caller, exactly as LoadHexProductionOptionsAt
// splits its coarse SQL prefilter from its precise Go-side builtAt check.
func rowAppliesToSite(row productionRuleRow, terrain string, coastal, copperDep, tinDep, silverDep, cedarDep bool) bool {
	if row.hasTerrain && row.terrain != terrain {
		return false
	}
	// A NULL-terrain row naming a building is a pure building workplace row
	// (LoadBuildingProductionOptions' territory) — never a hex row.
	if !row.hasTerrain && row.hasBuilding {
		return false
	}
	if row.requiresCoastal && !coastal {
		return false
	}
	switch row.requiresDeposit {
	case "":
	case "copper":
		if !copperDep {
			return false
		}
	case "tin":
		if !tinDep {
			return false
		}
	case "silver":
		if !silverDep {
			return false
		}
	case "cedar":
		if !cedarDep {
			return false
		}
	default:
		return false
	}
	// Final WHERE: a water-terrain hex only matches a row naming ITS OWN
	// terrain explicitly.
	if waterTerrains[terrain] && row.terrain != terrain {
		return false
	}
	return true
}

// hexSite is one candidate (terrain, deposit) combination a hex-bound
// production building's effect can be shown for.
type hexSite struct {
	Terrain string
	Deposit string // "" = no deposit requirement
}

// candidateHexSites returns every distinct site a building type BT could
// stand on and change SOMETHING about (rate or capacity), from two sources
// (megaron_plan_byggnad_pa_hex.md §B): (1) every production_rules row naming
// BT with a terrain, and (2) every entry in the P3 capacity tables
// (terrainCapacityTable, plainsCapacityRules, depositCapacityTable) whose
// relevantBuilding is BT. Source (2) is the one that brings in
// lumbermill+forest_olive_grove and farm+hills: lumbermill has NO
// forest_olive_grove production_rules row at all (only forest_cedar) — the
// olive-grove effect is capacity-only (BT raises the CAP, not the rate,
// which is exactly why its per-gubbe rate SHRINKS, see
// TestBuildingEffect_LumbermillOnForestOliveGrove).
func candidateHexSites(rules []productionRuleRow, bt string) []hexSite {
	seen := make(map[hexSite]bool)
	for _, row := range rules {
		if row.hasBuilding && row.buildingType == bt && row.hasTerrain {
			seen[hexSite{Terrain: row.terrain, Deposit: row.requiresDeposit}] = true
		}
	}
	for terrain, rule := range terrainCapacityTable {
		if rule.relevantBuilding == bt {
			seen[hexSite{Terrain: terrain}] = true
		}
	}
	for _, rule := range plainsCapacityRules {
		if rule.relevantBuilding == bt {
			seen[hexSite{Terrain: "plains"}] = true
		}
	}
	for deposit, rule := range depositCapacityTable {
		if rule.relevantBuilding != bt {
			continue
		}
		covered := false
		for s := range seen {
			if s.Deposit == deposit {
				covered = true
				break
			}
		}
		if !covered {
			seen[hexSite{Deposit: deposit}] = true
		}
	}
	out := make([]hexSite, 0, len(seen))
	for s := range seen {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Terrain != out[j].Terrain {
			return out[i].Terrain < out[j].Terrain
		}
		return out[i].Deposit < out[j].Deposit
	})
	return out
}

// EffectTier is one building level's (or, with Level==0, the unbuilt
// baseline's) realized per-gubbe rate and the number of gubbar that can be
// placed to earn it — read straight out of placementYield and the same cap
// map placementYield was clipped against.
type EffectTier struct {
	Level    int     `json:"level,omitempty"`
	PerGubbe float64 `json:"per_gubbe"`
	Gubbar   int     `json:"gubbar"`
}

// EffectRow is one good's production_rules-derived effect at one site
// (kind "hex"/"refining_ceiling") or settlement-wide (kind "workplace"/
// "refining") for one building type. Text is the ONE line the server
// renders from these numbers — keryx and web print it verbatim so the two
// surfaces can never drift apart (megaron_plan_byggnad_pa_hex.md §B).
type EffectRow struct {
	Good    string       `json:"good"`
	Kind    string       `json:"kind"` // "hex" | "workplace" | "refining" | "refining_ceiling"
	Terrain string       `json:"terrain,omitempty"`
	Deposit string       `json:"deposit,omitempty"`
	Without EffectTier   `json:"without"`
	Levels  []EffectTier `json:"levels"`
	// CatchmentWide is true for a city-scope building's "hex" row (harbour's
	// coastal_sea+fish row): it applies to EVERY matching catchment hex, not
	// to one specific placed building's own hex, unlike a HexBoundBuildingTypes
	// entry (farm/mine/lumbermill/stonequarry).
	CatchmentWide bool   `json:"catchment_wide,omitempty"`
	Text          string `json:"text"`
}

// round2 renders a rate to the same two-decimal precision the plan's
// worked examples use (§B: "rounding 2 decimals").
func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

// resolveCaps returns a good's (capL1, placeCap, mult), falling back to
// HexFallbackCap exactly as LoadHexProductionOptionsAt does for a good with
// a real rate but no P3 hexCapacityRule entry (oil, wine, stone).
func resolveCaps(good string, capsL1, placeCap map[string]int, mult map[string]float64) (int, int, float64) {
	cL1, ok := capsL1[good]
	if !ok {
		cL1 = HexFallbackCap
	}
	pCap, ok := placeCap[good]
	if !ok {
		pCap = HexFallbackCap
	}
	m, ok := mult[good]
	if !ok {
		m = 1.0
	}
	return cL1, pCap, m
}

// tierAt computes one level's EffectTier for good at a hex whose terrain/
// deposit flags are given, with buildingLevels naming the level BT stands
// at (empty map = no building — the "without" baseline). rate is the
// summed production_rules rate that applies at this site for this good
// (baseline rows, or baseline+building rows — see effectRowsForSite).
func tierAt(good string, rate float64, terrain string, copperDep, tinDep, silverDep bool, buildingLevels map[string]int, level int) EffectTier {
	_, capsL1, mult, placeCap := hexGoodCaps(terrain, copperDep, tinDep, silverDep, buildingLevels)
	capL1, pCap, m := resolveCaps(good, capsL1, placeCap, mult)
	perGubbe := placementYield(good, rate, capL1, pCap, m, 1)
	return EffectTier{Level: level, PerGubbe: round2(perGubbe), Gubbar: pCap}
}

// effectRowsForSite computes every "hex"/"refining_ceiling" EffectRow for
// building type bt standing on a site with the given terrain and deposit
// flags (copperDep/tinDep/silverDep — cedar has no deposit-flag production
// rule today, see rowAppliesToSite). Shared by the catalogue (synthetic
// sites, one deposit flag true at a time — see candidateHexSites) and the
// built-buildings list (the hex's REAL map_tiles flags), so both read
// through the identical formula — the parity this slice's acceptance #7
// requires.
func effectRowsForSite(rules []productionRuleRow, bt, terrain string, copperDep, tinDep, silverDep, coastal bool) []EffectRow {
	baselineRate := map[string]float64{}
	buildingRate := map[string]float64{}
	boostRate := map[string]float64{}
	var order []string
	seen := map[string]bool{}
	note := func(g string) {
		if !seen[g] {
			seen[g] = true
			order = append(order, g)
		}
	}
	for _, row := range rules {
		if !rowAppliesToSite(row, terrain, coastal, copperDep, tinDep, silverDep, false) {
			continue
		}
		if !row.hasBuilding {
			baselineRate[row.goodKey] += row.rate
			note(row.goodKey)
			continue
		}
		if row.buildingType != bt {
			continue // some other building's row on this same terrain — not BT's effect
		}
		note(row.goodKey)
		if refBT, ok := weakestLinkRefiningBuilding[row.goodKey]; ok && refBT == bt {
			boostRate[row.goodKey] += row.rate
		} else {
			buildingRate[row.goodKey] += row.rate
		}
	}
	sort.Strings(order)

	var out []EffectRow
	for _, good := range order {
		without := tierAt(good, baselineRate[good], terrain, copperDep, tinDep, silverDep, map[string]int{}, 0)
		if baselineRate[good] == 0 {
			// A capacity rule without any production_rules row (silver on a
			// deposit before a mine): LoadHexProductionOptionsAt never offers
			// the good here, so nothing can be placed — not "1 × 0.00".
			without = EffectTier{Level: 0}
		}
		withRate := baselineRate[good] + buildingRate[good]
		levels := make([]EffectTier, 0, 3)
		changed := false
		for level := 1; level <= 3; level++ {
			t := tierAt(good, withRate, terrain, copperDep, tinDep, silverDep, map[string]int{bt: level}, level)
			levels = append(levels, t)
			if t.PerGubbe != without.PerGubbe || t.Gubbar != without.Gubbar {
				changed = true
			}
		}
		if changed {
			out = append(out, EffectRow{
				Good: good, Kind: "hex", Terrain: terrain,
				Deposit:       depositForGood(good, copperDep, tinDep, silverDep),
				Without:       without,
				Levels:        levels,
				CatchmentWide: !HexBoundBuildingTypes[bt],
				Text:          renderHexText(good, terrain, depositForGood(good, copperDep, tinDep, silverDep), !HexBoundBuildingTypes[bt], without, levels),
			})
		}

		if boostRate[good] == 0 {
			continue
		}
		var boostLevels []EffectTier
		for level := 1; level <= 3; level++ {
			boostLevels = append(boostLevels, tierAt(good, boostRate[good], terrain, copperDep, tinDep, silverDep, map[string]int{bt: level}, level))
		}
		boostWithout := EffectTier{Level: 0}
		out = append(out, EffectRow{
			Good: good, Kind: "refining_ceiling", Terrain: terrain,
			Deposit:       depositForGood(good, copperDep, tinDep, silverDep),
			Without:       boostWithout,
			Levels:        boostLevels,
			CatchmentWide: !HexBoundBuildingTypes[bt],
			Text:          renderCeilingText(good, terrain, boostWithout, boostLevels),
		})
	}
	return out
}

// depositForGood names the deposit a row's good comes from — only on the
// metal's own row (copper on a copper hex), never on another good that
// happens to share the hex (grain on a copper hill is not "with a copper
// deposit"). A real hex can carry more than one flag.
func depositForGood(good string, copperDep, tinDep, silverDep bool) string {
	switch {
	case good == GoodCopper && copperDep, good == GoodTin && tinDep, good == GoodSilver && silverDep:
		return good
	default:
		return ""
	}
}

// workplaceEffectRows computes every terrain-free (city-wide, pooled)
// workplace row for building type bt — LoadBuildingProductionOptions'
// territory. good ∈ {oil, wine, bronze} is tagged "refining" (a
// pressarbetare/vinmakare/gjutare's own refining capacity, realized only
// alongside a matching hex's boost — see effectRowsForSite's
// "refining_ceiling" rows); every other good is a plain "workplace".
func workplaceEffectRows(rules []productionRuleRow, bt string) []EffectRow {
	rate := map[string]float64{}
	var order []string
	seen := map[string]bool{}
	for _, row := range rules {
		if row.hasTerrain || !row.hasBuilding || row.buildingType != bt {
			continue
		}
		if !seen[row.goodKey] {
			seen[row.goodKey] = true
			order = append(order, row.goodKey)
		}
		rate[row.goodKey] += row.rate
	}
	sort.Strings(order)

	var out []EffectRow
	for _, good := range order {
		capL1 := WorkplaceSlots(bt, 1)
		without := EffectTier{Level: 0, PerGubbe: 0, Gubbar: 0}
		levels := make([]EffectTier, 0, 3)
		changed := false
		for level := 1; level <= 3; level++ {
			capL := WorkplaceSlots(bt, level)
			mult := 1.0
			if capL1 > 0 {
				mult = float64(capL) / float64(capL1)
			}
			perGubbe := placementYield(good, rate[good], capL1, capL1, mult, 1)
			t := EffectTier{Level: level, PerGubbe: round2(perGubbe), Gubbar: capL1}
			levels = append(levels, t)
			if t.PerGubbe != 0 || t.Gubbar != 0 {
				changed = true
			}
		}
		if !changed {
			continue
		}
		kind := "workplace"
		if good == GoodOil || good == GoodWine || good == GoodBronze {
			kind = "refining"
		}
		out = append(out, EffectRow{
			Good: good, Kind: kind,
			Without: without, Levels: levels,
			Text: renderWorkplaceText(good, kind, without, levels),
		})
	}
	return out
}

// BuildingEffectsForCatalogue returns building type bt's full effect-row set
// for the static catalogue (GET /api/v1/buildings): one "hex"/"refining_ceiling"
// row set per candidateHexSites site (each candidate deposit flag treated as
// true for that site, coastal always true — no active row requires coastal
// today, see rowAppliesToSite), followed by bt's workplace/refining rows.
func BuildingEffectsForCatalogue(ctx context.Context, tx Tx, bt string) ([]EffectRow, error) {
	rules, err := loadActiveProductionRules(ctx, tx)
	if err != nil {
		return nil, fmt.Errorf("building effects for catalogue: %w", err)
	}
	return buildingEffectsForCatalogueFromRules(rules, bt), nil
}

func buildingEffectsForCatalogueFromRules(rules []productionRuleRow, bt string) []EffectRow {
	var out []EffectRow
	for _, site := range candidateHexSites(rules, bt) {
		out = append(out, effectRowsForSite(rules, bt, site.Terrain,
			site.Deposit == "copper", site.Deposit == "tin", site.Deposit == "silver", true)...)
	}
	out = append(out, workplaceEffectRows(rules, bt)...)
	return out
}

// BuildingEffectsForHex returns building type bt's effect rows for ONE real
// hex (a built building's own site): its actual terrain and map_tiles
// deposit/coastal flags, plus bt's workplace/refining rows (settlement-wide,
// independent of any one hex).
func BuildingEffectsForHex(ctx context.Context, tx Tx, bt, terrain string, copperDep, tinDep, silverDep, coastal bool) ([]EffectRow, error) {
	rules, err := loadActiveProductionRules(ctx, tx)
	if err != nil {
		return nil, fmt.Errorf("building effects for hex: %w", err)
	}
	out := effectRowsForSite(rules, bt, terrain, copperDep, tinDep, silverDep, coastal)
	out = append(out, workplaceEffectRows(rules, bt)...)
	return out, nil
}

// renderHexText/renderCeilingText/renderWorkplaceText are the ONE place an
// effect row's human-readable line is built — keryx and web print this
// string verbatim so the two surfaces can never disagree about what a
// building does (megaron_plan_byggnad_pa_hex.md §B).
func renderHexText(good, terrain, deposit string, catchmentWide bool, without EffectTier, levels []EffectTier) string {
	site := "on " + terrainText(terrain)
	if catchmentWide {
		site = "on every " + terrainText(terrain) + " hex"
	}
	if deposit != "" {
		site += " with a " + deposit + " deposit"
	}
	return fmt.Sprintf("%s %s: %s → %s", good, site, withoutText(without), levelsText(levels))
}

func renderCeilingText(good, terrain string, without EffectTier, levels []EffectTier) string {
	return fmt.Sprintf("extra %s on %s (only with a worker in the building): %s → %s", good, terrainText(terrain), withoutText(without), levelsText(levels))
}

func renderWorkplaceText(good, kind string, without EffectTier, levels []EffectTier) string {
	label := "in the building"
	if kind == "refining" {
		label = "refined in the building"
	}
	return fmt.Sprintf("%s %s: %s → %s", good, label, withoutText(without), levelsText(levels))
}

// terrainText is a terrain key as the player reads it: "forest_olive_grove"
// → "forest olive grove".
func terrainText(terrain string) string {
	return strings.ReplaceAll(terrain, "_", " ")
}

// withoutText is the "no building" side of the arrow: "none" when the site
// holds no worker for this good without the building, never "0 × 0.00".
func withoutText(t EffectTier) string {
	if t.Gubbar == 0 {
		return "none"
	}
	return fmt.Sprintf("%d × %.2f/tick", t.Gubbar, t.PerGubbe)
}

func levelsText(levels []EffectTier) string {
	s := ""
	for i, t := range levels {
		if i > 0 {
			s += " · "
		}
		s += fmt.Sprintf("L%d %d × %.2f", t.Level, t.Gubbar, t.PerGubbe)
	}
	return s
}
