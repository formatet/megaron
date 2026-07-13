package messenger

import (
	"os"
	"strings"
	"testing"
)

// TestNoTickMinutesUsage is a grep-guard (Fas A Run 2 sweep): internal/messenger
// must derive every display duration/ETA through RealUntil in package tick
// (exact via TickSeconds), never the deprecated TickMinutes var — it floors
// to 1 minute and silently misrepresents a sub-minute TICK_SECONDS cadence
// (e.g. TICK_SECONDS=6 reads as "1 minute/tick", 10x too long). See
// RealUntil/EtaAt (internal/tick/eta.go) and the doc comments on TickMinutes
// itself (internal/tick/worker.go) for the full rationale.
//
// Scoped to the qualified identifier, package-selector + var name joined at
// runtime (see `forbidden` below) so this guard's own source text doesn't
// trip itself, not the bare var name alone: prose explaining the history
// (travel_duration_test.go, recall.go doc comments) legitimately names the
// deprecated var without depending on it.
//
// api/handlers is deliberately NOT grep-guarded to zero: province.go's
// BuildingCatalogue/UnitCatalogue keep the deprecated var for their static
// per-type DURATION fields ("how long this build takes in general" — not an
// instance ETA), so a package-wide zero-tolerance guard doesn't apply there.
func TestNoTickMinutesUsage(t *testing.T) {
	forbidden := "tick" + "." + "TickMinutes"

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		data, err := os.ReadFile(e.Name())
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		if strings.Contains(string(data), forbidden) {
			t.Errorf("%s references %s — internal/messenger must derive display durations via tick.RealUntil, not the minute-floored TickMinutes", e.Name(), forbidden)
		}
	}
}
