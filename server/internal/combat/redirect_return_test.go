package combat

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/tick"
	"formatet/megaron/server/internal/unit"
	"github.com/google/uuid"
)

func TestRedirectExpeditionReturnArrivesAtActualDestination(t *testing.T) {
	for _, toHome := range []bool{false, true} {
		name := "field"
		if toHome {
			name = "home"
		}
		t.Run(name, func(t *testing.T) {
			f := newExpeditionFixture(t, -2, 16, nil)
			ctx := context.Background()
			pool := testPool(t)
			if _, err := pool.Exec(ctx, `UPDATE units SET support_settlement_id=$2 WHERE id=$1`, f.unitID, f.capitalID); err != nil {
				t.Fatal(err)
			}
			f.start(t, 9, 0, 40)
			if err := f.h.Handle(ctx, f.nextArrival(t)); err != nil {
				t.Fatal(err)
			}
			// A real recall turns the active expedition into its physical return leg.
			if res, err := ExecuteRecall(ctx, pool, f.scheduler, f.eventStore, f.clk, RecallOrder{WorldID: f.worldID, UnitID: f.unitID, Mode: "recall"}); err != nil || res == nil {
				t.Fatalf("recall: %+v %v", res, err)
			}
			var oldTick int
			var intent string
			if err := pool.QueryRow(ctx, `SELECT arrive_tick,march_intent FROM units WHERE id=$1`, f.unitID).Scan(&oldTick, &intent); err != nil {
				t.Fatal(err)
			}
			if intent != "explore_return" {
				t.Fatalf("not testing a homeward leg: %s", intent)
			}
			oldRaw, _ := json.Marshal(unit.ScheduledUnitArrivalPayload{UnitID: f.unitID, WorldID: f.worldID, ArriveTick: &oldTick})
			f.clk.Advance(time.Duration(tick.TickSeconds) * time.Second)
			q, r := 4, 0
			if toHome {
				q = 0
			}
			res, err := ExecuteRecall(ctx, pool, f.scheduler, f.eventStore, f.clk, RecallOrder{WorldID: f.worldID, UnitID: f.unitID, Mode: "redirect", NewTargetQ: &q, NewTargetR: &r})
			if err != nil || res == nil {
				t.Fatalf("redirect: %+v %v", res, err)
			}
			var newIntent *string
			var home *uuid.UUID
			var rows int
			if err := pool.QueryRow(ctx, `SELECT march_intent,home_settlement_id FROM units WHERE id=$1`, f.unitID).Scan(&newIntent, &home); err != nil {
				t.Fatal(err)
			}
			if newIntent != nil || home != nil {
				t.Errorf("redirect retained home-arrival mission: intent%v home%v", newIntent, home)
			}
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM unit_expeditions WHERE unit_id=$1`, f.unitID).Scan(&rows); err != nil {
				t.Fatal(err)
			}
			if rows != 0 {
				t.Fatal("redirect kept canceled expedition")
			}
			// Resolve the new leg twice: no old-home report and no duplicate outcome.
			arrival := f.nextArrival(t)
			for i := 0; i < 2; i++ {
				if err := f.h.Handle(ctx, arrival); err != nil {
					t.Fatal(err)
				}
			}
			var status string
			var sid, support *uuid.UUID
			var actualQ, actualR int
			if err := pool.QueryRow(ctx, `SELECT status,settlement_id,support_settlement_id,q,r FROM units WHERE id=$1`, f.unitID).Scan(&status, &sid, &support, &actualQ, &actualR); err != nil {
				t.Fatal(err)
			}
			if actualQ != q || actualR != r || support == nil || *support != f.capitalID {
				t.Fatalf("physical position/payor changed: (%d,%d) support%v", actualQ, actualR, support)
			}
			if toHome {
				if status != "garrison" || sid == nil || *sid != f.capitalID {
					t.Fatalf("true home arrival not garrison: %s %v", status, sid)
				}
			} else if status != "positioned" || sid != nil {
				t.Fatalf("field arrival became remote garrison: status%s city%v actual(%d,%d)", status, sid, actualQ, actualR)
			}
			if len(f.eventPayloads(t, unit.EventExpeditionReport)) != 0 {
				t.Fatal("canceled expedition produced homecoming report")
			}
			if len(f.eventPayloads(t, unit.EventUnitArrived)) != 1 {
				t.Fatal("replayed arrival duplicated outcome")
			}
			// The previous return job may fire later; it must not drag the unit home.
			if err := f.h.Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, Payload: oldRaw}); err != nil {
				t.Fatal(err)
			}
			if err := pool.QueryRow(ctx, `SELECT q,r FROM units WHERE id=$1`, f.unitID).Scan(&actualQ, &actualR); err != nil {
				t.Fatal(err)
			}
			if actualQ != q || actualR != r {
				t.Fatal("old home-arrival job moved redirected unit")
			}
		})
	}
}
