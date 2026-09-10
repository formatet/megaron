package tick

import (
	"os"
	"strings"
	"testing"
)

// The world start gate (Timothy 2026-09-10): a world stays 'forming' until
// worldStartWanaxes players have joined, and a forming world's clock must not
// move. The gate lives in exactly one place — the tick worker's world
// selection — because the economy (settled/current_world_tick) and the event
// scheduler (due_tick <= current_tick) are both derived from current_tick and
// therefore freeze with it.
//
// This test guards the query text itself. That is unusual, and deliberate: the
// failure mode is a world that ticks when it should not, which no unit test
// downstream would notice and which would silently burn the alpha's game days
// exactly as it did before the gate existed (live world stood at tick 148 with
// zero Wanaxes).

func TestTickWorker_SelectsOnlyStartedWorlds(t *testing.T) {
	src := readWorkerSource(t)

	// The single SELECT that decides which world may advance.
	idx := strings.Index(src, "SELECT id FROM worlds")
	if idx < 0 {
		t.Fatal("could not find the world-selection query in worker.go")
	}
	query := src[idx:]
	if end := strings.Index(query, "`"); end > 0 {
		query = query[:end]
	}

	if !strings.Contains(query, "state = 'active'") {
		t.Errorf("the tick worker must not advance a forming world — no state gate in:\n%s", query)
	}
	if !strings.Contains(query, "status = 'active'") {
		t.Errorf("the lifecycle gate must stay: an archived world must not tick either:\n%s", query)
	}
}

// last_tick_at is reset on activation (join.go). If it were not, a world that
// sat forming for hours would be "owed" every tick of the wait and would race
// through them the instant it started — undoing the whole point of waiting.
// The worker advances by ADDITION for catch-up accuracy, which is what makes
// that reset load-bearing rather than cosmetic.
func TestTickWorker_AdvancesByAdditionSoCatchUpIsAccurate(t *testing.T) {
	src := readWorkerSource(t)
	if !strings.Contains(src, "last_tick_at = last_tick_at + ") {
		t.Fatal("worker no longer advances last_tick_at by addition — " +
			"if this became SET now(), the last_tick_at reset in join.go's world start " +
			"is no longer needed and its comment is misleading; if it stayed addition, " +
			"the reset is required. Either way, join.go must be revisited together with this.")
	}
}

func readWorkerSource(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("worker.go")
	if err != nil {
		t.Fatalf("read worker.go: %v", err)
	}
	return string(b)
}
