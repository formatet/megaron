package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/carrier"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func TestRescuedRunner_SenderOutboxMapAndEyesKeepPositionUnknown(t *testing.T) {
	f := setupLegFixture(t)
	ctx := context.Background()
	now := time.Now()
	clk := clock.NewTestClock(now)
	store := events.NewStore(f.pool)
	if _, err := f.pool.Exec(ctx, `UPDATE worlds SET status='archived' WHERE status='active'`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE worlds SET status='active',current_tick=500 WHERE id=$1`, f.worldID); err != nil {
		t.Fatal(err)
	}
	var ship, rescuer, runner, enemy uuid.UUID
	if err := f.pool.QueryRow(ctx, `SELECT owner_id FROM settlements WHERE id=$1`, f.destID).Scan(&enemy); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `INSERT INTO units(world_id,owner_id,type,category,size,crew,status,q,r,name) VALUES($1,$2,'galley','naval',0,0,'disbanded',0,0,'Old Ship') RETURNING id`, f.worldID, f.viewerID).Scan(&ship); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `INSERT INTO units(world_id,owner_id,type,category,size,crew,status,q,r,name) VALUES($1,$2,'galley','naval',1,20,'positioned',3,0,'SECRET RESCUE SHIP') RETURNING id`, f.worldID, enemy).Scan(&rescuer); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE settlements SET name='SECRET FOREIGN PORT',owner_id=$2 WHERE id=$1`, f.portID, enemy); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `INSERT INTO messengers(world_id,sender_id,origin_id,destination_id,message_text,status,kind,hex_q,hex_r,arrives_at,passage_status,passage_port_id,carrier_unit_id) VALUES($1,$2,$3,$4,'sealed','outbound','message',20,0,$5,'aboard',$3,$6) RETURNING id`, f.worldID, f.viewerID, f.originID, f.destID, now.Add(time.Hour), ship).Scan(&runner); err != nil {
		t.Fatal(err)
	}
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	witnesses, err := carrier.OutcomeTx(ctx, tx, store, f.worldID, ship, &rescuer, "battle", 3, 0, 500)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if len(witnesses) != 1 {
		t.Fatalf("expected one physical rescue: %d", len(witnesses))
	}
	mh := NewMessengerHandler(f.pool, events.NewScheduler(f.pool, clk), clk, nil)
	router := chi.NewRouter()
	router.Use(auth.Middleware(f.authSvc))
	router.Get("/worlds/{worldID}/settlements/{settlementID}/messengers", mh.ListSent)
	for _, phase := range []string{"pending", "aboard", "awaiting_passage"} {
		if phase != "pending" {
			if _, err := f.pool.Exec(ctx, `UPDATE messengers SET carrier_unit_id=$2,carrier_name='SECRET RESCUE SHIP',passage_port_id=$3,passage_status=$4,carrier_witness_id=$5 WHERE id=$1`, runner, rescuer, f.portID, phase, witnesses[0].ID); err != nil {
				t.Fatal(err)
			}
		}
		req := httptest.NewRequest(http.MethodGet, "/worlds/"+f.worldID.String()+"/settlements/"+f.originID.String()+"/messengers", nil)
		req.Header.Set("Authorization", "Bearer "+f.token)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatalf("outbox %s: %d %s", phase, rec.Code, rec.Body.String())
		}
		for _, secret := range []string{rescuer.String(), f.portID.String(), "SECRET RESCUE SHIP", "SECRET FOREIGN PORT"} {
			if strings.Contains(rec.Body.String(), secret) {
				t.Fatalf("%s outbox leaks rescue/port %q: %s", phase, secret, rec.Body.String())
			}
		}
		var rows []map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &rows); err != nil || len(rows) != 1 || rows[0]["passage_status"] != "unknown" || rows[0]["can_arrange_passage"] != false {
			t.Fatalf("outbox must honestly say no word, no remote choices: %s %v", rec.Body.String(), err)
		}
		if got := callMessengersLeg(t, f.pool, f.authSvc, f.worldID, f.token, now); len(got) != 0 {
			t.Fatalf("%s map leaks rescued runner: %+v", phase, got)
		}
		for _, eye := range loadLiveEyes(ctx, f.pool, f.worldID, f.viewerID, now) {
			if eye.Pos.Q == 3 && eye.Pos.R == 0 {
				t.Fatalf("%s runner reveals foreign port through eyes: %+v", phase, eye)
			}
		}
	}
}
