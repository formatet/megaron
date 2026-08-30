package handlers

// Pins BuildingCatalogue's build-time field to GAME-DAYS, not wall-clock
// minutes. The old field (duration_minutes = DurationTicks*TickMinutes) read a
// 6-tick farm as "360" — nonsense to a player who counts in game-days, and
// flat wrong at a sub-minute TICK_SECONDS dev cadence. Same bug class as the
// rite-cooldown fix (cli-sanning, commit 2769042): a tick count dressed up as
// wall time.
//
// Non-circular: the surface's number is checked against the ENGINE's own
// spec.DurationTicks (one tick = one game-day, megaron_plan_ticket_ar_dygnet),
// and explicitly against the OLD ×TickMinutes formula it must no longer equal.
// Reverting the handler to DurationTicks*TickMinutes fails the mutation guard.
// DATABASE_URL-gated via p10TestPool, same skip as the other catalogue tests.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/tick"
)

func TestBuildingCatalogue_DurationInGameDaysNotMinutes(t *testing.T) {
	pool := p10TestPool(t)
	ctx := context.Background()
	_ = ctx

	ph := NewProvinceHandler(pool, nil, clock.NewTestClock(time.Now()), economy.SitosConfig{}, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/buildings", nil)
	rec := httptest.NewRecorder()
	ph.BuildingCatalogue(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("BuildingCatalogue = %d: %s", rec.Code, rec.Body.String())
	}

	var catalogue []struct {
		Type             string `json:"type"`
		DurationGameDays int    `json:"duration_game_days"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &catalogue); err != nil {
		t.Fatalf("decode catalogue: %v (body: %s)", err, rec.Body.String())
	}
	if len(catalogue) == 0 {
		t.Fatal("empty building catalogue")
	}

	sawMultiTick := false
	for _, b := range catalogue {
		spec, ok := province.BuildingSpecs[province.BuildingType(b.Type)]
		if !ok {
			t.Errorf("catalogue lists unknown building %q", b.Type)
			continue
		}
		// One tick is one game-day: the surface must report the raw tick count.
		if b.DurationGameDays != spec.DurationTicks {
			t.Errorf("%s: duration_game_days = %d, want %d (= DurationTicks; a tick IS a game-day)",
				b.Type, b.DurationGameDays, spec.DurationTicks)
		}
		// Mutation guard: the value must NOT be the old minutes figure. With the
		// default 60-min tick this is a 60× gap, so any building with a real
		// duration separates the two formulas cleanly.
		if spec.DurationTicks > 0 {
			sawMultiTick = true
			oldMinutes := spec.DurationTicks * tick.TickMinutes
			if b.DurationGameDays == oldMinutes && oldMinutes != spec.DurationTicks {
				t.Errorf("%s: duration_game_days = %d equals the OLD DurationTicks*TickMinutes (%d) — regressed to wall-clock minutes",
					b.Type, b.DurationGameDays, oldMinutes)
			}
		}
	}
	if !sawMultiTick {
		t.Fatal("no building with DurationTicks > 0 — cannot distinguish game-days from minutes")
	}
}
