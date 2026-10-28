package gossip

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestCodexRumourAge(t *testing.T) {
	raw, err := os.ReadFile("../../../web/static/codex/gossip.md")
	if err != nil {
		t.Fatal(err)
	}
	var days int
	if _, err := fmt.Sscanf(rumorMaxAge, "%d days", &days); err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf("News received more than %d real hours ago is no longer passed on.", days*24)
	if !strings.Contains(string(raw), want) {
		t.Fatalf("Codex must explain current rule: %s", want)
	}
}
