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
	for _, cap := range populationCaps {
		want := fmt.Sprintf("%d %03d", cap/1000, cap%1000)
		if !strings.Contains(string(raw), want) {
			t.Errorf("growth article missing population cap %s", want)
		}
	}
}
