package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/go-chi/chi/v5"
)

// Literal durations pin the speed-to-time direction and normal nearest-tick
// rounding through authenticated HTTP, dispatch and persisted arrival ticks.
func TestHostSpeed_HTTPPreviewDispatchAndStoredMarch(t *testing.T) {
	for _, tc := range []struct {
		terrain string
		want    int
	}{
		{"plains", 2}, {"forest_cedar", 4}, {"forest_olive_grove", 3}, {"hills", 4}, {"semi_desert", 7},
	} {
		t.Run(tc.terrain, func(t *testing.T) {
			pool := unitLoadTestPool(t)
			ctx := context.Background()
			f, _ := setupNomadicHostRecallWorld(t)
			if _, err := pool.Exec(ctx, `UPDATE worlds SET current_tick=5 WHERE id=$1`, f.worldID); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, `UPDATE map_tiles SET terrain=$2 WHERE world_id=$1`, f.worldID, tc.terrain); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, `UPDATE units SET status='positioned',q=0,r=0,target_q=NULL,target_r=NULL,depart_tick=NULL,arrive_tick=NULL,departs_at=NULL,arrives_at=NULL WHERE id=$1`, f.hostID); err != nil {
				t.Fatal(err)
			}
			// Old host march: 4 plains steps at the former factor 2. Its stored
			// schedule and path must remain authoritative under the new factor.
			route := `{"start_tick":0,"end_tick":8,"hexes":[[0,0],[1,0],[2,0],[3,0],[4,0]],"costs":[1000,1000,1000,1000]}`
			if _, err := pool.Exec(ctx, `UPDATE units SET type='nomadic_host',depart_tick=0,arrive_tick=8,march_route=$2 WHERE id=$1`, f.spearmanID, route); err != nil {
				t.Fatal(err)
			}
			snapshot := func() string {
				t.Helper()
				var s string
				if err := pool.QueryRow(ctx, `SELECT row_to_json(u)::text FROM units u WHERE id=$1`, f.spearmanID).Scan(&s); err != nil {
					t.Fatal(err)
				}
				return s
			}
			old := snapshot()
			clk := clock.NewTestClock(f.now)
			uh := NewUnitHandler(pool, events.NewScheduler(pool, clk), events.NewStore(pool), clk)
			r := chi.NewRouter()
			r.Use(auth.Middleware(auth.NewService(pool, "test-secret")))
			r.Get("/worlds/{worldID}/units/{unitID}/march-preview", uh.MarchPreview)
			r.Post("/worlds/{worldID}/units/{unitID}/march", uh.March)
			r.Get("/worlds/{worldID}/units", uh.ListUnits)
			base := "/worlds/" + f.worldID.String() + "/units"
			req := func(method, path, body string) *httptest.ResponseRecorder {
				q := httptest.NewRequest(method, path, strings.NewReader(body))
				q.Header.Set("Authorization", "Bearer "+f.accessToken)
				out := httptest.NewRecorder()
				r.ServeHTTP(out, q)
				return out
			}
			type timing struct {
				Arrival  int    `json:"arrival_tick"`
				Duration int    `json:"duration_ticks"`
				UTC      string `json:"arrives_at_utc"`
			}
			preview := req(http.MethodGet, base+"/"+f.hostID.String()+"/march-preview?target_q=2&target_r=0", "")
			if preview.Code != 200 {
				t.Fatalf("preview %d: %s", preview.Code, preview.Body.String())
			}
			var p timing
			if err := json.Unmarshal(preview.Body.Bytes(), &p); err != nil {
				t.Fatal(err)
			}
			if p.Duration != tc.want || p.Arrival != 5+tc.want {
				t.Fatalf("preview %+v want duration=%d arrival=%d", p, tc.want, 5+tc.want)
			}
			actual := req(http.MethodPost, base+"/"+f.hostID.String()+"/march", `{"target_q":2,"target_r":0}`)
			if actual.Code != 202 {
				t.Fatalf("dispatch %d: %s", actual.Code, actual.Body.String())
			}
			var a timing
			if err := json.Unmarshal(actual.Body.Bytes(), &a); err != nil {
				t.Fatal(err)
			}
			if a != p {
				t.Fatalf("actual %+v differs from preview %+v", a, p)
			}
			var depart, arrive int
			if err := pool.QueryRow(ctx, `SELECT depart_tick,arrive_tick FROM units WHERE id=$1`, f.hostID).Scan(&depart, &arrive); err != nil {
				t.Fatal(err)
			}
			if depart != 5 || arrive != p.Arrival {
				t.Fatalf("stored %d→%d vs %+v", depart, arrive, p)
			}
			list := req(http.MethodGet, base, "")
			if list.Code != 200 {
				t.Fatal(list.Body.String())
			}
			var units struct {
				Units []struct {
					ID string `json:"id"`
					timing
				}
			}
			if err := json.Unmarshal(list.Body.Bytes(), &units); err != nil {
				t.Fatal(err)
			}
			found := false
			for _, u := range units.Units {
				if u.ID == f.spearmanID.String() {
					found = true
					if u.Arrival != 8 || u.Duration != 8 {
						t.Fatalf("old stored ETA changed: %+v", u)
					}
				}
			}
			if !found || snapshot() != old {
				t.Fatal("reading or dispatch changed the old host march")
			}
			t.Log(fmt.Sprintf("%s: preview=dispatch=stored %d ticks; old host arrival 8 and row unchanged", tc.terrain, p.Duration))
		})
	}
}
