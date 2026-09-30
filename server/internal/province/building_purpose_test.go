package province

import (
	"strings"
	"testing"
)

// TestBuildingPurposes_MakeNoGoodsClaims is acceptance criterion 5
// (megaron_plan_byggnad_pa_hex.md §B): a role line may describe what a
// building DOES, never claim which goods rise or by how much — that's
// economy.BuildingEffectsForCatalogue/BuildingEffectsForHex's job now, and a
// hand-written claim here can drift from those numbers (farm used to say
// "raises grain and oil production from plains; wine from hills and
// plains" while the numbers disagreed about which terrains actually apply).
// Pinned narrowly to the exact regression: farm's line must not name oil.
func TestBuildingPurposes_MakeNoGoodsClaims(t *testing.T) {
	farm := BuildingPurposes[BuildingFarm]
	if farm == "" {
		t.Fatal("farm has no purpose line")
	}
	if strings.Contains(strings.ToLower(farm), "oil") {
		t.Errorf("farm's purpose line must not mention oil (a real side effect, but not one the role line should claim): %q", farm)
	}

	// No purpose line may claim "produces"/"raises"/"increases" a specific
	// good by name and amount — that verb+good combination is exactly the
	// stale-claim shape this slice removes.
	staleVerbs := []string{"raises", "increases", "produces "}
	for bt, purpose := range BuildingPurposes {
		if bt == BuildingTemple {
			// Cult is temple labor, not a production_rules good (migration
			// 094 removed it from the table entirely) — its own untouched
			// path, megaron_cult_ar_ingen_vara_plan.md. "Produces cult" is
			// not a claim economy.BuildingEffects* could ever contradict.
			continue
		}
		lower := strings.ToLower(purpose)
		for _, verb := range staleVerbs {
			if strings.Contains(lower, verb) {
				t.Errorf("%s's purpose line %q still uses a production-rate verb (%q) — "+
					"that claim belongs in economy.BuildingEffects*, not here", bt, purpose, verb)
			}
		}
	}
}
