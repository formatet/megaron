package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
)

// Exercise the real payment path, frozen refund, duplicate request and a
// worker which leased TrainComplete before cancellation removed the queue row.
func TestCancelTrainingPaidReservationLifecycle(t *testing.T) {
	for _, kind := range []string{"spearman", "galley"} {
		t.Run(kind, func(t *testing.T) {
			f := setupRecruitShipFixture(t)
			ctx := context.Background()
			read := func() (int, map[string]float64) {
				t.Helper()
				var pop int
				if err := f.pool.QueryRow(ctx, `SELECT population FROM settlements WHERE id=$1`, f.settlementID).Scan(&pop); err != nil {
					t.Fatal(err)
				}
				rows, err := f.pool.Query(ctx, `SELECT good_key,amount FROM settlement_goods WHERE settlement_id=$1`, f.settlementID)
				if err != nil {
					t.Fatal(err)
				}
				defer rows.Close()
				goods := map[string]float64{}
				for rows.Next() {
					var k string
					var n float64
					if err := rows.Scan(&k, &n); err != nil {
						t.Fatal(err)
					}
					goods[k] = n
				}
				if err := rows.Err(); err != nil {
					t.Fatal(err)
				}
				return pop, goods
			}
			before, goodsBefore := read()
			rec, resp := f.post(t, "/worlds/"+f.worldID.String()+"/provinces/"+f.provinceID.String()+"/recruit", map[string]any{"unit_type": kind})
			if rec.Code != http.StatusCreated {
				t.Fatalf("recruit: %d %s", rec.Code, rec.Body.String())
			}
			id := uuid.MustParse(resp["unit_id"].(string))
			popPaid, goodsPaid := read()
			if popPaid >= before {
				t.Fatal("real recruitment did not reserve people")
			}
			var paid map[string]float64
			if err := f.pool.QueryRow(ctx, `SELECT recruitment_costs FROM units WHERE id=$1`, id).Scan(&paid); err != nil {
				t.Fatal(err)
			}
			if len(paid) == 0 {
				t.Fatal("no frozen reservation")
			}
			for k, n := range paid {
				if goodsBefore[k]-goodsPaid[k] != n {
					t.Fatalf("%s recorded %v, actually paid %v", k, n, goodsBefore[k]-goodsPaid[k])
				}
			}
			path := "/worlds/" + f.worldID.String() + "/units/" + id.String() + "/cancel-training"
			rec, resp = f.post(t, path, nil)
			if rec.Code != 200 {
				t.Fatalf("cancel: %d %s", rec.Code, rec.Body.String())
			}
			if resp["pop_restored"] != float64(before-popPaid) {
				t.Fatalf("restored: %v", resp)
			}
			after, goodsAfter := read()
			if after != before {
				t.Fatalf("population %d want %d", after, before)
			}
			for k, n := range goodsBefore {
				if goodsAfter[k] != n {
					t.Fatalf("%s=%v want %v", k, goodsAfter[k], n)
				}
			}
			rec, resp = f.post(t, path, nil)
			if rec.Code != 200 || resp["already_cancelled"] != true {
				t.Fatalf("replay: %d %v", rec.Code, resp)
			}
			payload, _ := json.Marshal(combat.TrainCompletePayload{SettlementID: f.settlementID, UnitType: kind, Count: 1, UnitID: id})
			if err := f.trainH.Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, Payload: payload}); err != nil {
				t.Fatal(err)
			}
			var state string
			if err := f.pool.QueryRow(ctx, `SELECT status FROM units WHERE id=$1`, id).Scan(&state); err != nil {
				t.Fatal(err)
			}
			if state != "disbanded" {
				t.Fatalf("stale worker resurrected %s", state)
			}
			replayPop, replayGoods := read()
			if replayPop != before {
				t.Fatal("duplicate population refund")
			}
			for k, n := range goodsBefore {
				if replayGoods[k] != n {
					t.Fatalf("duplicate %s refund", k)
				}
			}
			var jobs int
			if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM scheduled_events WHERE world_id=$1 AND event_type='TrainComplete' AND payload->>'unit_id'=$2 AND processed_at IS NULL`, f.worldID, id.String()).Scan(&jobs); err != nil {
				t.Fatal(err)
			}
			if jobs != 0 {
				t.Fatal("training timer remains")
			}
		})
	}
}

func TestCancelTrainingRefusesFinishedForeignAndUnknownReservations(t *testing.T) {
	f := setupRecruitShipFixture(t)
	ctx := context.Background()
	recruit := func() uuid.UUID {
		t.Helper()
		r, d := f.post(t, "/worlds/"+f.worldID.String()+"/provinces/"+f.provinceID.String()+"/recruit", map[string]any{"unit_type": "galley"})
		if r.Code != 201 {
			t.Fatal(r.Body.String())
		}
		return uuid.MustParse(d["unit_id"].(string))
	}
	cancel := func(id uuid.UUID) int {
		r, _ := f.post(t, "/worlds/"+f.worldID.String()+"/units/"+id.String()+"/cancel-training", nil)
		return r.Code
	}
	id := recruit()
	payload, _ := json.Marshal(combat.TrainCompletePayload{SettlementID: f.settlementID, UnitType: "galley", Count: 1, UnitID: id})
	if err := f.trainH.Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, Payload: payload}); err != nil {
		t.Fatal(err)
	}
	if code := cancel(id); code != 409 {
		t.Fatalf("finished: %d", code)
	}
	id = recruit()
	if _, err := f.pool.Exec(ctx, `UPDATE units SET recruitment_costs=NULL WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if code := cancel(id); code != 409 {
		t.Fatalf("unknown reservation: %d", code)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE units SET settlement_id=NULL WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if code := cancel(id); code != 409 {
		t.Fatalf("no city: %d", code)
	}
	// Requesting an absent or foreign unit cannot reveal its reservation.
	if code := cancel(uuid.New()); code != 404 {
		t.Fatalf("absent: %d", code)
	}
	other := setupRecruitShipFixture(t)
	if _, err := f.pool.Exec(ctx, `UPDATE units SET owner_id=$2 WHERE id=$1`, id, other.playerID); err != nil {
		t.Fatal(err)
	}
	if code := cancel(id); code != 404 {
		t.Fatalf("foreign: %d", code)
	}
}
