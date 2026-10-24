package transport

import (
	"context"
	"testing"

	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/province"
	"github.com/google/uuid"
)

// One physical ship carries the entire mixed manifest through every outcome.
func TestInterceptScan_MixedManifestEveryOutcomeAndRetry(t *testing.T) {
	for _, tc := range []struct {
		name string
		roll float64
	}{{"captured", .1}, {"limped", .5}, {"sunk", .9}} {
		t.Run(tc.name, func(t *testing.T) {
			pool := testPool(t)
			ctx := context.Background()
			f := newNavalSeizureFixture(t, pool)
			if _, err := pool.Exec(ctx, `INSERT INTO transport_goods(transport_id,good_key,quantity) VALUES($1,'grain',20)`, f.transportID); err != nil {
				t.Fatal(err)
			}
			h := NewInterceptScanHandler(pool, events.NewScheduler(pool, f.clk), events.NewStore(pool), nil, f.clk)
			h.Dice = fixedDice{tc.roll}
			if err := h.Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: 1}); err != nil {
				t.Fatal(err)
			}
			// Retry the exact original seizure, rather than scanning newly dispatched
			// return voyages which are independently exposed to interception.
			if err := h.seize(ctx, f.worldID, inFlightTransport{id: f.transportID}, uuid.Nil, f.raider, province.MapPosition{Q: 1, R: 0}); err != nil {
				t.Fatal(err)
			}
			for good, sent := range map[string]float64{"silver": 100, "grain": 20} {
				var looted, returning float64
				if err := pool.QueryRow(ctx, `SELECT COALESCE((SELECT settled(amount,rate,calc_tick) FROM settlement_goods WHERE settlement_id=$1 AND good_key=$2),0)`, f.raiderCapital, good).Scan(&looted); err != nil {
					t.Fatal(err)
				}
				if err := pool.QueryRow(ctx, `SELECT COALESCE(sum(g.quantity),0) FROM transport_goods g JOIN transports tr ON tr.id=g.transport_id WHERE tr.world_id=$1 AND tr.kind='damaged_return' AND g.good_key=$2`, f.worldID, good).Scan(&returning); err != nil {
					t.Fatal(err)
				}
				wantLoot, wantReturn := 0.0, 0.0
				if tc.name == "captured" {
					wantLoot = sent
				}
				if tc.name == "limped" {
					wantReturn = sent / 2
				}
				if looted != wantLoot || returning != wantReturn {
					t.Errorf("%s: loot/return %v/%v want %v/%v", good, looted, returning, wantLoot, wantReturn)
				}
			}
			var status string
			if err := pool.QueryRow(ctx, `SELECT status FROM transports WHERE id=$1`, f.transportID).Scan(&status); err != nil {
				t.Fatal(err)
			}
			if status != "intercepted" {
				t.Errorf("original status=%s", status)
			}
			var returns, captures int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM transports WHERE world_id=$1 AND kind='damaged_return'`, f.worldID).Scan(&returns); err != nil {
				t.Fatal(err)
			}
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM scheduled_events WHERE world_id=$1 AND event_type='NavalSeizureOutcome'`, f.worldID).Scan(&captures); err != nil {
				t.Fatal(err)
			}
			wantReturns, wantCaptures := 0, 0
			if tc.name == "limped" {
				wantReturns = 1
			}
			if tc.name == "captured" {
				wantCaptures = 1
			}
			if returns != wantReturns || captures != wantCaptures {
				t.Errorf("return/capture events=%d/%d want%d/%d", returns, captures, wantReturns, wantCaptures)
			}
			var shipStatus string
			if err := pool.QueryRow(ctx, `SELECT status FROM units WHERE id=$1`, f.shipID).Scan(&shipStatus); err != nil {
				t.Fatal(err)
			}
			wantStatus := "freighting"
			if tc.name == "sunk" {
				wantStatus = "disbanded"
			}
			if shipStatus != wantStatus {
				t.Errorf("ship=%s want%s", shipStatus, wantStatus)
			}
		})
	}
}
