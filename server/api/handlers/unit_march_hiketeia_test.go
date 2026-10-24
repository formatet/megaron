package handlers

import (
	"context"
	"net/http"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/religion"
	"github.com/google/uuid"
)

// A march on a hex where you can see a risen host is refused with the
// hiketeia warning until the order is confirmed (Timothy 2026-10-24).
func TestMarch_RisenHostNeedsConfirmation(t *testing.T) {
	f := setupRecruitShipFixture(t)
	ctx := context.Background()
	for q := 0; q <= 2; q++ {
		if _, err := f.pool.Exec(ctx, `INSERT INTO map_tiles(world_id,q,r,terrain) VALUES($1,$2,0,'plains')`, f.worldID, q); err != nil {
			t.Fatal(err)
		}
	}
	var fallen, host uuid.UUID
	if err := f.pool.QueryRow(ctx, `INSERT INTO players(username,password_hash) VALUES($1,'x') RETURNING id`, "fallen-"+uuid.NewString()).Scan(&fallen); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `INSERT INTO units(world_id,owner_id,type,category,size,crew,status,q,r) VALUES($1,$2,'nomadic_host','land',1,0,'positioned',2,0) RETURNING id`, f.worldID, fallen).Scan(&host); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO founder_phase(world_id,owner_id,host_unit_id,population,grain_amount,grain_rate,silver_amount,silver_rate,calc_tick,active,is_refugee) VALUES($1,$2,$3,500,0,0,60,0,0,true,true)`, f.worldID, fallen, host); err != nil {
		t.Fatal(err)
	}
	clk := clock.NewTestClock(time.Now())
	uh := NewUnitHandler(f.pool, events.NewScheduler(f.pool, clk), events.NewStore(f.pool), clk)
	f.router.Post("/worlds/{worldID}/units/{unitID}/march", uh.March)
	id := insertUnit(t, f, "garrison", &f.settlementID, &f.settlementID, 100)
	path := "/worlds/" + f.worldID.String() + "/units/" + id.String() + "/march"

	rec, resp := f.post(t, path, map[string]any{"target_q": 2, "target_r": 0})
	if rec.Code != http.StatusConflict || resp["hiketeia"] != true || resp["error"] != religion.HiketeiaWarning {
		t.Fatalf("unconfirmed march on a risen host = %d %v, want 409 with the hiketeia warning", rec.Code, resp)
	}
	var status string
	if err := f.pool.QueryRow(ctx, `SELECT status FROM units WHERE id=$1`, id).Scan(&status); err != nil || status != "garrison" {
		t.Fatalf("unit left without confirmation: %q %v", status, err)
	}
	rec, resp = f.post(t, path, map[string]any{"target_q": 2, "target_r": 0, "confirm_hiketeia": true})
	if rec.Code != http.StatusAccepted {
		t.Fatalf("confirmed march = %d %v, want 202", rec.Code, resp)
	}
}
