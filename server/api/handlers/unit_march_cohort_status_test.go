package handlers

import (
	"context"
	"net/http"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
)

// Reproduce the two Haiku 6 orders through authenticated HTTP and real PG:
// a completed cohort after losses is distinct from an unfinished recruitment.
func TestMarch_ExploreCompleted64AndForming40(t *testing.T) {
	f := setupRecruitShipFixture(t)
	ctx := context.Background()
	for q := 0; q <= 2; q++ {
		if _, err := f.pool.Exec(ctx,
			`INSERT INTO map_tiles(world_id,q,r,terrain) VALUES($1,$2,0,'plains')`, f.worldID, q); err != nil {
			t.Fatal(err)
		}
	}
	clk := clock.NewTestClock(time.Now())
	uh := NewUnitHandler(f.pool, events.NewScheduler(f.pool, clk), events.NewStore(f.pool), clk)
	f.router.Post("/worlds/{worldID}/units/{unitID}/march", uh.March)

	for _, tc := range []struct {
		name, status string
		size, code   int
	}{
		{"completed after losses", "garrison", 64, http.StatusAccepted},
		{"unfinished recruitment", "forming", 40, http.StatusUnprocessableEntity},
	} {
		t.Run(tc.name, func(t *testing.T) {
			id := insertUnit(t, f, tc.status, &f.settlementID, &f.settlementID, tc.size)
			rec, resp := f.post(t, "/worlds/"+f.worldID.String()+"/units/"+id.String()+"/march",
				map[string]any{"target_q": 2, "target_r": 0, "intent": "explore"})
			if rec.Code != tc.code {
				t.Fatalf("explore(%s, %d) = %d %s, want %d", tc.status, tc.size, rec.Code, rec.Body.String(), tc.code)
			}
			if tc.status == "forming" && resp["error"] != "unit cannot march: status is 'forming' (must be 'garrison' or 'positioned')" {
				t.Errorf("forming rejection = %v, want status explanation", resp["error"])
			}
			var status string
			var size int
			if err := f.pool.QueryRow(ctx, `SELECT status,size FROM units WHERE id=$1`, id).Scan(&status, &size); err != nil {
				t.Fatal(err)
			}
			wantStatus := tc.status
			if tc.code == http.StatusAccepted {
				wantStatus = "marching"
			}
			if status != wantStatus || size != tc.size {
				t.Errorf("stored unit = (%s, %d), want (%s, %d)", status, size, wantStatus, tc.size)
			}
		})
	}
}
