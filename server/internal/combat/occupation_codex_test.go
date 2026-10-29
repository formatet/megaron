package combat

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestCodexOccupationValues(t *testing.T) {
	raw, err := os.ReadFile("../../../web/static/codex/occupation.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{fmt.Sprintf("Hold it for %d game days", occupationTicksToAnnex), fmt.Sprintf("for %d game days", burnRecolonizeKaren)} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("occupation article missing %s", want)
		}
	}
	if strings.Contains(string(raw), "a third of its people") && sackPopLossFraction != 1.0/3.0 {
		t.Error("sack population fraction changed; update Codex")
	}
}
