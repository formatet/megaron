package transport

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

type codexSeizureRoll float64

func (d codexSeizureRoll) Float64() float64 { return float64(d) }
func TestCodexSeizureOdds(t *testing.T) {
	raw, err := os.ReadFile("../../../web/static/codex/transfers.md")
	if err != nil {
		t.Fatal(err)
	}
	counts := map[NavalSeizureOutcome]int{}
	for i := 0; i < 1000; i++ {
		counts[rollNavalSeizureOutcome(codexSeizureRoll(float64(i)/1000))]++
	}
	for _, row := range []struct {
		label   string
		outcome NavalSeizureOutcome
	}{{"Captured", NavalSeizureCaptured}, {"Returns damaged", NavalSeizureLimped}, {"Sunk", NavalSeizureSunk}} {
		want := fmt.Sprintf("| %s | %g%% |", row.label, float64(counts[row.outcome])/10)
		if !strings.Contains(string(raw), want) {
			t.Errorf("seizure article missing %s", want)
		}
	}
}
