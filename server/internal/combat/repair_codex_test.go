package combat

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestCodexRepairValues(t *testing.T) {
	raw, err := os.ReadFile("../../../web/static/codex/ship-types.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{fmt.Sprintf("**%g%% of the ship's build price**", repairCostFractionPerHullPoint*100), fmt.Sprintf("**%d game day**", RepairDurationTicks(1))} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("ship repair article missing %s", want)
		}
	}
}
