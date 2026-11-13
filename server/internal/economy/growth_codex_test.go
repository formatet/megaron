package economy

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestCodexGrowthValues(t *testing.T) {
	raw, err := os.ReadFile("../../../web/static/codex/growth.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{fmt.Sprintf("**%g%% of its people per game day**", PopulationGrowthRatePerTick*100), fmt.Sprintf("at least **%g%% of the meal**", VarietyMinShare*100)} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("growth article missing %s", want)
		}
	}
	// The article states the wall ceilings in citizens (1 citizen = 100 people), like the City panel.
	c := populationCaps
	want := fmt.Sprintf("%d citizens (%d %03d people) with no wall, then %d, %d and %d citizens", c[0]/100, c[0]/1000, c[0]%1000, c[1]/100, c[2]/100, c[3]/100)
	if !strings.Contains(string(raw), want) {
		t.Errorf("growth article missing wall ceilings %q", want)
	}
}
