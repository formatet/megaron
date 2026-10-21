package messenger

import (
	"context"
	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/province"
	"testing"
)

func TestDeliveredMarchReplacesMovingCourse(t *testing.T) {
	for _, delivered := range []bool{false, true} {
		name := "old_sealed_order"
		if delivered {
			name = "new_delivered_order"
		}
		t.Run(name, func(t *testing.T) {
			f := setupSingleRecall(t, "redirect")
			f.payload.Verb = "march"
			f.payload.Recall = nil
			f.payload.March = &combat.MarchOrder{WorldID: f.world, PlayerID: f.owner, UnitID: f.unitID, TargetQ: 0, TargetR: 0, Delivered: delivered}
			if err := f.handler.Handle(context.Background(), f.event()); err != nil {
				t.Fatal(err)
			}
			var q int
			if err := f.pool.QueryRow(context.Background(), `SELECT target_q FROM units WHERE id=$1`, f.unitID).Scan(&q); err != nil {
				t.Fatal(err)
			}
			if delivered {
				if q != 0 || len(f.hub.failures) != 0 {
					t.Fatalf("target=%d failures=%v", q, f.hub.failures)
				}
				if err := f.handler.Handle(context.Background(), f.event()); err != nil {
					t.Fatal(err)
				}
				var n int
				f.pool.QueryRow(context.Background(), `SELECT count(*) FROM events WHERE stream_id=$1 AND event_type='UnitMarchOrdered'`, f.unitID).Scan(&n)
				if n != 1 {
					t.Fatalf("replay started %d marches", n)
				}
			} else if q != 4 || len(f.hub.failures) != 1 {
				t.Fatalf("old envelope changed semantics: target=%d failures=%v", q, f.hub.failures)
			}
		})
	}
}

func TestLateRunnerDeliversAtDestination(t *testing.T) {
	for _, stillThere := range []bool{true, false} {
		name := "still_at_destination"
		if !stillThere {
			name = "unit_left_destination"
		}
		t.Run(name, func(t *testing.T) {
			f := setupSingleRecall(t, "redirect")
			ctx := context.Background()
			q := 4
			if !stillThere {
				q = 3
			}
			_, err := f.pool.Exec(ctx, `UPDATE units SET status='positioned',q=$2,r=0,target_q=NULL,target_r=NULL,departs_at=NULL,arrives_at=NULL,depart_tick=NULL,arrive_tick=NULL,march_route=NULL WHERE id=$1`, f.unitID, q)
			if err != nil {
				t.Fatal(err)
			}
			f.payload.AfterArrival = &combat.MarchOrder{WorldID: f.world, PlayerID: f.owner, UnitID: f.unitID, TargetQ: 0, TargetR: 0, ExpectedPosition: &province.MapPosition{Q: 4, R: 0}}
			if err := f.handler.Handle(ctx, f.event()); err != nil {
				t.Fatal(err)
			}
			var status string
			var target *int
			if err := f.pool.QueryRow(ctx, `SELECT status,target_q FROM units WHERE id=$1`, f.unitID).Scan(&status, &target); err != nil {
				t.Fatal(err)
			}
			if stillThere {
				if status != "marching" || target == nil || *target != 0 || len(f.hub.failures) != 0 {
					t.Fatalf("status=%s target=%v failures=%v", status, target, f.hub.failures)
				}
			} else if status != "positioned" || len(f.hub.failures) != 1 {
				t.Fatalf("remote order applied: status=%s failures=%v", status, f.hub.failures)
			}
		})
	}
}

func TestLateRunnerDoesNotCommandUnitThatStartedAnotherMarch(t *testing.T) {
	f := setupSingleRecall(t, "redirect")
	f.payload.MeetAtDestination = true
	f.payload.AfterArrival = &combat.MarchOrder{WorldID: f.world, PlayerID: f.owner, UnitID: f.unitID, TargetQ: 0, TargetR: 0, ExpectedPosition: &province.MapPosition{Q: 4, R: 0}}
	if err := f.handler.Handle(context.Background(), f.event()); err != nil {
		t.Fatal(err)
	}
	if len(f.hub.failures) != 1 {
		t.Fatalf("Runner at destination commanded a distant moving unit: %v", f.hub.failures)
	}
}

func TestLateRecallToUnitAlreadyHomeDoesNotStartAnotherMarch(t *testing.T) {
	f := setupSingleRecall(t, "recall")
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `UPDATE units SET status='garrison',settlement_id=support_settlement_id,q=NULL,r=NULL,target_q=NULL,target_r=NULL,depart_tick=NULL,arrive_tick=NULL,march_route=NULL WHERE id=$1`, f.unitID); err != nil {
		t.Fatal(err)
	}
	f.payload.MeetAtDestination = true
	f.payload.AfterArrival = &combat.MarchOrder{WorldID: f.world, PlayerID: f.owner, UnitID: f.unitID, ExpectedPosition: &province.MapPosition{Q: 0, R: 0}}
	if err := f.handler.Handle(ctx, f.event()); err != nil {
		t.Fatal(err)
	}
	var status string
	f.pool.QueryRow(ctx, `SELECT status FROM units WHERE id=$1`, f.unitID).Scan(&status)
	if status != "garrison" || len(f.hub.failures) > 0 {
		t.Fatalf("status=%s failures=%v", status, f.hub.failures)
	}
}
