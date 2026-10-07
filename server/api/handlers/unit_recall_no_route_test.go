package handlers

import (
	"context"
	"encoding/json"
	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/province"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRecallNoRoute_HTTPRejectsNamed(t *testing.T) {
	for _, missing := range []bool{false, true} {
		label := "course"
		if missing {
			label = "position"
		}
		t.Run(label, func(t *testing.T) {
			f, router := setupNomadicHostRecallWorld(t)
			pool := unitLoadTestPool(t)
			ctx := context.Background()
			// Pin an authoritative route to the current world tick so HTTP and core
			// both locate the host on the far side of the now-missing route tile.
			if !missing {
				var current int
				if err := pool.QueryRow(ctx, `SELECT current_world_tick()`).Scan(&current); err != nil {
					t.Fatal(err)
				}
				route, ok := combat.BuildRoute([]province.MapPosition{{Q: 0}, {Q: 1}, {Q: 2}, {Q: 3}, {Q: 4}}, []float64{.75, .75, .75, .75}, current-1, current+1)
				if !ok {
					t.Fatal("fixture route")
				}
				raw, _ := json.Marshal(route)
				if _, err := pool.Exec(ctx, `UPDATE units SET march_route=$2,depart_tick=$3,arrive_tick=$4 WHERE id=$1`, f.hostID, raw, current-1, current+1); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := pool.Exec(ctx, `DELETE FROM map_tiles WHERE world_id=$1 AND q=1`, f.worldID); err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodPost, "/worlds/"+f.worldID.String()+"/units/"+f.hostID.String()+"/recall", nil)
			req.Header.Set("Authorization", "Bearer "+f.accessToken)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != 422 || !strings.Contains(rec.Body.String(), "route") {
				t.Fatalf("unnamed route rejection: %d %s", rec.Code, rec.Body.String())
			}
			var target, queued int
			if err := pool.QueryRow(ctx, `SELECT target_q FROM units WHERE id=$1`, f.hostID).Scan(&target); err != nil {
				t.Fatal(err)
			}
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM scheduled_events WHERE world_id=$1`, f.worldID).Scan(&queued); err != nil {
				t.Fatal(err)
			}
			if target != 4 || queued != 0 {
				t.Fatalf("rejected HTTP mutated unit/queue: %d/%d", target, queued)
			}
		})
	}
}

// Ordinary units must also stop before an origin-guess can aim a real Runner.
func TestRecallNoRoute_HTTPDoesNotDispatchFromGuessedOrigin(t *testing.T) {
	f, router := setupNomadicHostRecallWorld(t)
	pool := unitLoadTestPool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `DELETE FROM map_tiles WHERE world_id=$1 AND q=1`, f.worldID); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/worlds/"+f.worldID.String()+"/units/"+f.spearmanID.String()+"/recall", nil)
	req.Header.Set("Authorization", "Bearer "+f.accessToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != 422 || !strings.Contains(rec.Body.String(), "no passable outbound route") {
		t.Fatalf("guessed origin dispatched: %d %s", rec.Code, rec.Body.String())
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM messengers WHERE world_id=$1`, f.worldID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("guessed origin created %d messengers", count)
	}
}
