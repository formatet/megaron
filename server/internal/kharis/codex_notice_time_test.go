package kharis

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestCodexHungerRecovery(t *testing.T) {
	raw, err := os.ReadFile("../../../web/static/codex/food.md")
	if err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf("The warning clears after **%d consecutive fully fed game days**.", civilianRecoveryTicks)
	if !strings.Contains(string(raw), want) {
		t.Fatalf("Codex must explain current rule: %s", want)
	}
}
