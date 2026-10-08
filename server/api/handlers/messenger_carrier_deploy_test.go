package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/messenger"
	"github.com/google/uuid"
)

// A pre-T2 aboard row and its already queued generation-bearing terminal event
// are retained by migration 162. No test-driver projection hook fires the old
// timer: the real handlers receive the exact stored payload and due_tick.
func TestCarrierDeploy_AlreadyAboardWithQueuedLegacyTimer(t *testing.T) {
	for _, landingBeforeDeploy := range []bool{true, false} {
		name := "lands-after-deploy"
		if landingBeforeDeploy {
			name = "landed-before-deploy"
		}
		t.Run(name, func(t *testing.T) {
			f := setupPassageArrangeFixture(t)
			ctx := context.Background()
			home := f.settlement(t, "Deploy-Home", 0, f.initiatorID, true)
			destination := f.settlement(t, "Deploy-Destination", 5, f.counterpartyID, true)
			for q := 1; q <= 4; q++ {
				f.mapTile(t, q, 0, "coastal_sea")
			}
			ship := f.ship(t, home, f.initiatorID, "merchantman")
			code, _ := f.post(t, f.initiatorToken, "/worlds/"+f.worldID.String()+"/settlements/"+home.String()+"/messengers", map[string]any{"destination_id": destination.String(), "message": "already sealed before T2 deploy"})
			if code != 201 {
				t.Fatalf("send: %d", code)
			}
			var runner uuid.UUID
			if err := f.pool.QueryRow(ctx, `SELECT id FROM messengers WHERE origin_id=$1 AND destination_id=$2`, home, destination).Scan(&runner); err != nil {
				t.Fatal(err)
			}
			shipTick := f.arrangePassage(t, f.initiatorToken, runner, ship)
			old := f.loadPendingEventBy(t, string(events.ScheduledMessengerArrival), "messenger_id", runner)
			oldPayload := append([]byte(nil), old.Payload...)
			var applied int64
			var evidence int
			var generation int
			if err := f.pool.QueryRow(ctx, `SELECT carrier_witness_id,passage_generation,(SELECT count(*) FROM events WHERE stream_id=$1 AND event_type LIKE 'CarrierPassenger%V1') FROM messengers WHERE id=$1 AND passage_status='aboard'`, runner).Scan(&applied, &generation, &evidence); err != nil {
				t.Fatal(err)
			}
			if applied != 0 || evidence != 0 || generation == 0 {
				t.Fatalf("legacy deploy snapshot: applied=%d evidence=%d generation=%d", applied, evidence, generation)
			}
			var payload messenger.ArrivalPayload
			if err := json.Unmarshal(old.Payload, &payload); err != nil {
				t.Fatal(err)
			}
			if payload.PassageGeneration != generation || old.DueTick < shipTick {
				t.Fatalf("existing timer must preserve its boarding generation/real due: %+v", old)
			}

			// Run the actual carrier arrival without the acceptance helper's scan.
			f.setTick(t, shipTick)
			arrival := f.loadPendingEventBy(t, string(events.ScheduledUnitArrival), "unit_id", ship)
			if err := f.unitArrivalH.Handle(ctx, arrival); err != nil {
				t.Fatal(err)
			}
			f.markProcessed(t, arrival.ID)
			if landingBeforeDeploy {
				// Legacy DB snapshot: old arrival produced the same physical ship state
				// and retained the aboard runner/timer, but did not emit T2 witnesses.
				// Removing only that new evidence models the old binary, not a new
				// production exemption. Projection/generation have not run or changed.
				if _, err := f.pool.Exec(ctx, `DELETE FROM events WHERE stream_id=$1 AND event_type LIKE 'CarrierPassenger%V1'`, runner); err != nil {
					t.Fatal(err)
				}
			}
			f.setTick(t, old.DueTick)
			if err := f.msgArrivalH.Handle(ctx, old); err != nil {
				t.Fatal(err)
			}
			f.markProcessed(t, old.ID)
			got := f.messengerRow(t, runner)
			if landingBeforeDeploy {
				if got.status != "delivered" {
					t.Fatalf("legacy row with NO witness must arrive via unchanged queued timer: %+v", got)
				}
				if err := f.pool.QueryRow(ctx, `SELECT carrier_witness_id FROM messengers WHERE id=$1`, runner).Scan(&applied); err != nil || applied != 0 {
					t.Fatalf("legacy timer must not need invented evidence: %d %v", applied, err)
				}
				t.Logf("pre-deploy landing: retained generation=%d, old due_tick=%d -> delivered, witness_id=0", generation, old.DueTick)
			} else {
				if got.status != "outbound" {
					t.Fatalf("post-deploy landing's pending witness must block old timer: %+v", got)
				}
				// At this tick the real priority-10 terminal firing precedes the
				// priority-20 PassageScan. The scan must enqueue a future completion.
				f.runPassageScan(t)
				next := f.loadPendingEventBy(t, string(events.ScheduledMessengerArrival), "messenger_id", runner)
				var current messenger.ArrivalPayload
				if err := json.Unmarshal(next.Payload, &current); err != nil {
					t.Fatal(err)
				}
				if next.ID == old.ID || current.PassageGeneration <= generation || next.DueTick <= old.DueTick {
					t.Fatalf("projection must schedule a distinct future current-generation timer: old=%+v next=%+v", old, next)
				}
				if err := f.msgArrivalH.Handle(ctx, old); err != nil {
					t.Fatal(err)
				}
				if f.messengerRow(t, runner).status != "outbound" {
					t.Fatal("old timer replay must remain stale after projection")
				}
				f.setTick(t, next.DueTick)
				if err := f.msgArrivalH.Handle(ctx, next); err != nil {
					t.Fatal(err)
				}
				f.markProcessed(t, next.ID)
				if f.messengerRow(t, runner).status != "delivered" {
					t.Fatal("legacy aboard runner must finish via new real scheduled completion")
				}
				t.Logf("post-deploy landing: old generation=%d due=%d -> blocked; scan tick=%d -> generation=%d due=%d -> delivered", generation, old.DueTick, old.DueTick, current.PassageGeneration, next.DueTick)
			}
			if !bytes.Equal(old.Payload, oldPayload) {
				t.Fatal("existing queued payload was rewritten")
			}
			if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE kind IN ('MessengerLostAtSea','MessengerRescuedAtSea') AND body_json->>'messenger_id'=$1`, runner.String()).Scan(&evidence); err != nil || evidence != 0 {
				t.Fatalf("normal deploy voyage must not invent rescue/loss news: %d %v", evidence, err)
			}
		})
	}
}
