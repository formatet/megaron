package messenger

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/events"
)

// A runner carrying a "declined" home and lost at sea: the sender learns of
// the loss at once, and the reservation held for the offer comes home then —
// otherwise it would be gone with the runner (Timothy 2026-10-24).
//
// An offer declined before the marker existed was refunded on the click; its
// runner's loss must not pay it out a second time.
func TestCarrierFate_LossReturnsDeclinedReservation(t *testing.T) {
	for _, tc := range []struct {
		name, marker string
		want         float64
	}{{"held", `"refund_on_return":true,`, 31}, {"refunded_before_marker", ``, 0}} {
		t.Run(tc.name, func(t *testing.T) { lossReturnsDeclinedReservation(t, tc.marker, tc.want) })
	}
}

func lossReturnsDeclinedReservation(t *testing.T, marker string, want float64) {
	f := setupPassageFixture(t)
	carrierHumanNames(t, f)
	ctx := context.Background()
	clk := clock.NewTestClock(time.Now())
	sched := events.NewScheduler(f.pool, clk)
	ship := f.marchingShip(t, 0, 0, 4, 0, 500, 504)
	route := combat.StoredRoute{StartTick: 500, EndTick: 504, Hexes: [][2]int{{0, 0}, {1, 0}, {2, 0}, {3, 0}, {4, 0}}, Costs: []int64{1000, 1000, 1000, 1000}}
	raw, _ := json.Marshal(route)
	if _, err := f.pool.Exec(ctx, `UPDATE units SET hull=1,march_route=$2 WHERE id=$1`, ship, raw); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) VALUES($1,'bronze',0,0,1000000,0)`, f.originID); err != nil {
		t.Fatal(err)
	}
	runner := f.waitingMessenger(t, f.originID, 500)
	offer := `{"kind":"sell","status":"declined",` + marker + `"offer_good":"bronze","offer_qty":31,"want_silver":145.7}`
	if _, err := f.pool.Exec(ctx, `UPDATE messengers SET passage_status='aboard',carrier_unit_id=$2,trade_offer=$3 WHERE id=$1`, runner, ship, offer); err != nil {
		t.Fatal(err)
	}
	storm := combat.NewSeaStormScanHandler(f.pool, sched, events.NewStore(f.pool), nil)
	carrierStormOver(t, f.pool, f.worldID)
	f.setTick(t, 501)
	if err := storm.Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: 501}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := f.handler().Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: 501}); err != nil {
			t.Fatal(err)
		}
	}
	var status string
	var bronze float64
	if err := f.pool.QueryRow(ctx, `SELECT status FROM messengers WHERE id=$1`, runner).Scan(&status); err != nil || status != "lost" {
		t.Fatalf("runner status %q (%v), want lost", status, err)
	}
	if err := f.pool.QueryRow(ctx, `SELECT amount FROM settlement_goods WHERE settlement_id=$1 AND good_key='bronze'`, f.originID).Scan(&bronze); err != nil {
		t.Fatal(err)
	}
	if bronze != want {
		t.Fatalf("bronze at origin after loss = %v, want %v", bronze, want)
	}
}
