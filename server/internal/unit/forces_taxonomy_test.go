package unit

import (
	"strings"
	"testing"
)

func TestForcesDisplayTaxonomyAndDynamicLandNames(t *testing.T) {
	for typ, want := range map[string]string{"elite_infantry": "Bronze Guard", "war_chariot": "Chariot"} {
		if got := DisplayName(typ); got != want {
			t.Fatalf("%s=%q want%q", typ, got, want)
		}
		if got := LandUnitName(typ, 1, "Knossos", "").DisplayName; got != "1st "+want+" of Knossos" {
			t.Fatal(got)
		}
	}
	for key := range displayNames {
		for _, retired := range RetiredDisplayLabels {
			if strings.Contains(DisplayName(key), retired) {
				t.Fatalf("retired label %q surfaced as %q", retired, DisplayName(key))
			}
		}
	}
}
