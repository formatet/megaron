package messenger

import (
	"context"
	"errors"
	"strings"
	"testing"

	"formatet/megaron/server/internal/combat"
)

// The outgoing saved route still locates Delta at (2,0). Removing a tile
// behind it models a route that no longer exists when the Runner delivers.
func breakRecallRoute(t *testing.T, f singleRecallFixture, positionMissing bool) {
	t.Helper()
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `DELETE FROM map_tiles WHERE world_id=$1 AND q=1`, f.world); err != nil {
		t.Fatal(err)
	}
	if positionMissing {
		if _, err := f.pool.Exec(ctx, `UPDATE units SET march_route=NULL WHERE id=$1`, f.unitID); err != nil {
			t.Fatal(err)
		}
	}
}

func assertRecallUnchanged(t *testing.T, f singleRecallFixture) {
	t.Helper()
	var target, arrivals, success int
	ctx := context.Background()
	if err := f.pool.QueryRow(ctx, `SELECT target_q FROM units WHERE id=$1`, f.unitID).Scan(&target); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM scheduled_events WHERE world_id=$1 AND event_type='UnitArrival'`, f.world).Scan(&arrivals); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM events WHERE stream_id=$1 AND event_type IN ('MarchRecalled','MarchRedirected')`, f.unitID).Scan(&success); err != nil {
		t.Fatal(err)
	}
	if target != 4 || arrivals != 1 || success != 0 {
		t.Fatalf("rejected order changed course: target=%d arrivals=%d success=%d", target, arrivals, success)
	}
}

func TestRecallNoRoute_CoreRejectsWithoutMutation(t *testing.T) {
	for _, verb := range []string{"recall", "redirect"} {
		for _, missing := range []bool{false, true} {
			label := "course"
			if missing {
				label = "position"
			}
			t.Run(verb+"/"+label, func(t *testing.T) {
				f := setupSingleRecall(t, verb)
				breakRecallRoute(t, f, missing)
				res, err := combat.ExecuteRecall(context.Background(), f.pool, f.sched, f.store, f.clk, *f.payload.Recall)
				var rej *combat.OrderReject
				if res != nil || !errors.As(err, &rej) || rej.Status != 422 || !strings.Contains(rej.Reason, "route") {
					t.Fatalf("missing %s accepted or unnamed: result=%+v err=%v", label, res, err)
				}
				assertRecallUnchanged(t, f)
			})
		}
	}
}

func TestRecallNoRoute_DeliveryNamesReasonOnce(t *testing.T) {
	for _, verb := range []string{"recall", "redirect"} {
		t.Run(verb, func(t *testing.T) {
			f := setupSingleRecall(t, verb)
			breakRecallRoute(t, f, false)
			for i := 0; i < 2; i++ {
				if err := f.handler.Handle(context.Background(), f.event()); err != nil {
					t.Fatal(err)
				}
			}
			if len(f.hub.failures) != 1 {
				t.Fatalf("named failures=%+v", f.hub.failures)
			}
			p := f.hub.failures[0]
			if p["name"] != "Delta" || !strings.Contains(p["reason"].(string), "no passable route") {
				t.Fatalf("route reason hidden: %+v", p)
			}
			var audits int
			if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM events WHERE stream_id=$1 AND event_type='OrderDeliveryFailed' AND payload->>'reason'=$2`, f.unitID, p["reason"]).Scan(&audits); err != nil {
				t.Fatal(err)
			}
			if audits != 1 {
				t.Fatalf("failure audits=%d", audits)
			}
			assertRecallUnchanged(t, f)
		})
	}
}

func TestRecallNoRoute_TrivialRouteStillApplies(t *testing.T) {
	f := setupSingleRecall(t, "redirect")
	q, r := 2, 0
	f.payload.Recall.NewTargetQ = &q
	f.payload.Recall.NewTargetR = &r
	res, err := combat.ExecuteRecall(context.Background(), f.pool, f.sched, f.store, f.clk, *f.payload.Recall)
	if err != nil || res == nil || res.NewTargetQ != 2 {
		t.Fatalf("trivial route rejected: %+v %v", res, err)
	}
}
