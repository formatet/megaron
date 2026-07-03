package capabilities

import (
	"fmt"
	"sort"

	"github.com/poleia/server/internal/province"
)

// canBuild: constructing SOME building is (almost) always possible — most
// building types (farm, barracks, temple, ...) carry no structural gate at
// all, only a resource cost, which capabilities does not check (see craft/
// recruit for the affordance pattern this DOES apply to). The two building
// types that DO carry a live structural gate (harbour: coastal; mine/
// silver_mine: catchment deposit) are already fully surfaced by the existing
// `poleia build --list` / `GET /buildings` catalogue (requires_coastal /
// requires_deposits per type) — the anchor this capabilities layer
// generalizes from. Duplicating that per-type gate here as a single
// AND-computed `available` would misrepresent reality (build IS usable even
// when harbour specifically is not), so build is listed trivially, per F3.
func canBuild(cc checkContext) Verb {
	return verb("build", CategoryProvince,
		"Queue construction of a building in this settlement (see `poleia build --list` for per-type costs and gates such as coastal/deposit).",
		nil)
}

func canCancelBuild(cc checkContext) Verb {
	return verb("cancel-build", CategoryProvince,
		"Cancel a queued building and refund its costs.", nil)
}

func canAllocate(cc checkContext) Verb {
	return verb("allocate", CategoryProvince,
		"Set the share of population working each producible good.", nil)
}

// canCraft checks the load-bearing bronze recipe (recipe_id=1: copper+tin →
// bronze @ foundry) — the MVP-chain's craft step. Luxury (recipe 2) is a
// second recipe the same endpoint serves but is not part of the MVP chain
// this spec calls out, so it is not separately modeled here.
// TODO: Fas 3 unify with handler gate.
func canCraft(cc checkContext) Verb {
	const recipeID = 1
	type ingredient struct {
		good string
		qty  float64
	}
	var buildingType, outputKey string
	_ = cc.pool.QueryRow(cc.ctx,
		`SELECT building_type, output_key FROM recipes WHERE id = $1`, recipeID,
	).Scan(&buildingType, &outputKey)
	if buildingType == "" {
		buildingType, outputKey = "foundry", "bronze"
	}

	rows, err := cc.pool.Query(cc.ctx,
		`SELECT good_key, quantity FROM recipe_ingredients WHERE recipe_id = $1 ORDER BY good_key`, recipeID)
	var ingredients []ingredient
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var i ingredient
			if rows.Scan(&i.good, &i.qty) == nil {
				ingredients = append(ingredients, i)
			}
		}
	}

	hasFoundry := cc.hasBuilding(buildingType)
	reqs := []Requirement{
		req(fmt.Sprintf("%s built", buildingType), hasFoundry,
			boolDetail(hasFoundry, buildingType+" built", buildingType+" not built"),
			fmt.Sprintf("build a %s (`poleia build --type %s`)", buildingType, buildingType)),
	}
	for _, i := range ingredients {
		have := cc.goodAmount(i.good)
		ok := have >= i.qty
		reqs = append(reqs, req(
			fmt.Sprintf("%s >= %.0f (per unit crafted)", i.good, i.qty),
			ok,
			fmt.Sprintf("%s %.0f/%.0f", i.good, have, i.qty),
			fmt.Sprintf("acquire %s via a colony/mine or trade for it", i.good),
		))
	}
	return verb("craft", CategoryProvince,
		fmt.Sprintf("Smelt %s at your %s from the recipe's ingredients.", outputKey, buildingType), reqs)
}

// canRecruit checks population and, for a representative minimal batch (10
// men), building requirements + affordability per unit type — mirroring
// api/handlers/province.go Recruit's own gates.
// TODO: Fas 3 unify with handler gate.
func canRecruit(cc checkContext) Verb {
	pop := cc.population()
	popOK := pop > 0
	reqs := []Requirement{
		req("population > 0", popOK,
			fmt.Sprintf("population %d", pop),
			"grow population (idle labor, or wait for grain surplus) before recruiting"),
	}

	// Affordability per type for a 10-man batch — enumerate deterministically.
	types := make([]string, 0, len(province.UnitSpecs))
	for t := range province.UnitSpecs {
		types = append(types, t)
	}
	sort.Strings(types)
	var affordable []string
	for _, t := range types {
		spec := province.UnitSpecs[t]
		if spec.RequiresBarracks && !cc.hasBuilding("barracks") {
			continue
		}
		if spec.RequiresStable && !cc.hasBuilding("stable") {
			continue
		}
		if spec.RequiresHarbour && !cc.hasBuilding("harbour") {
			continue
		}
		if spec.RequiresFoundry && !cc.hasBuilding("foundry") {
			continue
		}
		afford := true
		for good, perMan := range spec.Costs {
			if cc.goodAmount(good) < perMan*10 {
				afford = false
				break
			}
		}
		if afford {
			affordable = append(affordable, t)
		}
	}
	afforded := len(affordable) > 0
	detail := "none affordable for a 10-man batch right now"
	if afforded {
		detail = "affordable now: " + joinComma(affordable)
	}
	reqs = append(reqs, req("at least one unit type affordable (building + goods) for a 10-man batch",
		afforded, detail, "build the required building (barracks/stable/harbour/foundry) and stock the per-man goods cost"))

	return verb("recruit", CategoryProvince,
		"Draft population into a military unit (land units grow to 100 men before they can deploy).", reqs)
}

// canAbandon checks whether the player has any non-capital active settlement
// to give up — abandon never targets the capital (settlement.go Abandon).
func canAbandon(cc checkContext) Verb {
	_, nonCapital := cc.ownSettlements()
	ok := nonCapital > 0
	return verb("abandon", CategoryProvince,
		"Voluntarily give up a colony (not your capital), freeing its hex and a settlement slot.",
		[]Requirement{
			req("a non-capital settlement to give up", ok,
				fmt.Sprintf("%d/1 abandonable colonies", nonCapital),
				"found or hold a colony beyond your capital before abandoning one"),
		})
}

func boolDetail(ok bool, yes, no string) string {
	if ok {
		return yes
	}
	return no
}

func joinComma(items []string) string {
	out := ""
	for i, s := range items {
		if i > 0 {
			out += ", "
		}
		out += s
	}
	return out
}
