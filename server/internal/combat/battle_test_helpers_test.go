package combat

// Shared test helpers for driving the KR3 battle state machine
// (megaron_plan_kr3_stridssystem.md §2) forward in tests that don't care
// about the real scheduled_events plumbing — they just want the battle
// resolved to its conclusion.

import (
	"context"
	"encoding/json"
	"testing"

	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// runBattleToEnd calls BattleTickHandler.Handle for battleID repeatedly,
// tick_index 1, 2, 3, ..., until battles.status flips to 'ended' or maxTicks
// is exhausted (a safety valve — a test fixture that never terminates should
// fail loudly, not hang).
func runBattleToEnd(t *testing.T, pool *pgxpool.Pool, h *BattleTickHandler, worldID, battleID uuid.UUID, maxTicks int) {
	t.Helper()
	ctx := context.Background()
	raw, err := json.Marshal(battleTickPayload{BattleID: battleID})
	if err != nil {
		t.Fatalf("marshal battle tick payload: %v", err)
	}

	for tick := 1; tick <= maxTicks; tick++ {
		var status string
		if err := pool.QueryRow(ctx, `SELECT status FROM battles WHERE id = $1`, battleID).Scan(&status); err != nil {
			t.Fatalf("read battle status: %v", err)
		}
		if status == "ended" {
			return
		}
		if err := h.Handle(ctx, events.ScheduledEvent{WorldID: worldID, DueTick: tick, Payload: raw}); err != nil {
			t.Fatalf("battle tick handle (tick %d): %v", tick, err)
		}
	}

	var status string
	_ = pool.QueryRow(ctx, `SELECT status FROM battles WHERE id = $1`, battleID).Scan(&status)
	if status != "ended" {
		t.Fatalf("battle %s did not end within %d ticks (status still %q)", battleID, maxTicks, status)
	}
}
