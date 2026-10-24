package handlers

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/kharis"
)

// Exercise the public status endpoint and the actual tick handler, keeping
// independent expected outcomes so a shared-formula mutation cannot pass.
func TestNextCitizenStatusMatchesActualGrowthTick(t *testing.T) {
	for i, tc := range []struct {
		name        string
		pop         int
		meal        string
		stale       bool
		rate, unmet float64
		gain        int
		ticks       any
	}{
		{"variety", 1264, `{"grain":25,"fish":25,"wine":25,"oil":25}`, false, 5000, 0, 8, 5},
		{"half even rounding", 1700, `{}`, false, 5000, 0, 8, 13},
		{"minimum one", 20, `{}`, false, 5000, 0, 1, 80},
		{"clamp", 1999, `{}`, false, 5000, 0, 1, 1},
		{"at cap", 2000, `{}`, false, 5000, 0, 0, nil},
		{"over cap", 2001, `{}`, false, 5000, 0, 0, nil},
		{"no food surplus", 1264, `{}`, false, 0, 0, 0, nil},
		{"starving", 1264, `{}`, false, 5000, 1, 0, nil},
		{"stale meal", 1264, `{"grain":25,"fish":25,"wine":25,"oil":25}`, true, 5000, 0, 6, 6},
		{"tiny wine does not count", 1264, `{"grain":99,"wine":1}`, false, 5000, 0, 6, 6},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := setupTradeNavalFixture(t)
			ctx := context.Background()
			sid, pid := f.settlement(t, "Citizen city", 0, false, true)
			if _, err := f.pool.Exec(ctx, `UPDATE settlements SET population=$2,wall_level=0,food_unmet_amount=$3,food_consumed=$4::jsonb,food_consumed_tick=current_world_tick()-$5 WHERE id=$1`, sid, tc.pop, tc.unmet, tc.meal, map[bool]int{false: 0, true: 1}[tc.stale]); err != nil {
				t.Fatal(err)
			}
			if _, err := f.pool.Exec(ctx, `UPDATE settlement_goods SET rate=$2 WHERE settlement_id=$1 AND good_key='grain'`, sid, tc.rate); err != nil {
				t.Fatal(err)
			}
			ph := NewProvinceHandler(f.pool, f.scheduler, f.clk, events.NewStore(f.pool), nil)
			f.router.Get("/worlds/{worldID}/provinces/{provinceID}", ph.Get)
			req := httptest.NewRequest("GET", "/worlds/"+f.worldID.String()+"/provinces/"+pid.String(), nil)
			req.Header.Set("Authorization", "Bearer "+f.token)
			rec := httptest.NewRecorder()
			f.router.ServeHTTP(rec, req)
			if rec.Code != 200 {
				t.Fatalf("status%d %s", rec.Code, rec.Body.String())
			}
			var status struct {
				Settlement map[string]any `json:"settlement"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
				t.Fatal(err)
			}
			surface, ok := status.Settlement["next_citizen_growth_per_tick"].(float64)
			if !ok || surface != float64(tc.gain) {
				t.Fatalf("surface growth=%v want%d", status.Settlement["next_citizen_growth_per_tick"], tc.gain)
			}
			if status.Settlement["next_citizen_progress"] != float64(tc.pop%100) {
				t.Fatalf("progress=%v", status.Settlement["next_citizen_progress"])
			}
			wantTicks := any(nil)
			if tc.ticks != nil {
				wantTicks = float64(tc.ticks.(int))
			}
			if status.Settlement["next_citizen_in_ticks"] != wantTicks {
				t.Fatalf("estimate=%v want%v", status.Settlement["next_citizen_in_ticks"], wantTicks)
			}
			h := kharis.NewTickHandler(f.pool, f.scheduler, events.NewStore(f.pool), nil)
			if err := h.Handle(ctx, events.ScheduledEvent{ID: int64(960000 + i), WorldID: f.worldID, DueTick: 0}); err != nil {
				t.Fatal(err)
			}
			var after int
			if err := f.pool.QueryRow(ctx, `SELECT population FROM settlements WHERE id=$1`, sid).Scan(&after); err != nil {
				t.Fatal(err)
			}
			actual := max(0, after-tc.pop)
			if actual != int(surface) {
				t.Fatalf("surface%v actual tick%d (%d→%d)", surface, actual, tc.pop, after)
			}
		})
	}
}
