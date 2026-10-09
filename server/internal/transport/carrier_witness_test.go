package transport

import (
	"context"
	"encoding/json"
	"testing"

	"formatet/megaron/server/internal/carrier"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/province"
	"github.com/google/uuid"
)

func TestSeizure_PassengerWitnessesBothReferencesAndRetry(t *testing.T) {
	for _, tc := range []struct {
		name  string
		roll  float64
		naval bool
		kind  string
	}{{"capture", .1, false, carrier.Rescued}, {"limped", .6, false, carrier.Redirected}, {"sunk without rescue", .9, false, carrier.Lost}, {"sunk with rescue", .9, true, carrier.Rescued}} {
		t.Run(tc.name, func(t *testing.T) {
			pool := testPool(t)
			f := newNavalSeizureFixture(t, pool)
			ctx := context.Background()
			var rescuer uuid.UUID
			if err := pool.QueryRow(ctx, `SELECT id FROM units WHERE world_id=$1 AND owner_id=$2`, f.worldID, f.raider).Scan(&rescuer); err != nil {
				t.Fatal(err)
			}
			if tc.naval {
				if _, err := pool.Exec(ctx, `UPDATE units SET type='galley',category='naval',size=1,crew=20 WHERE id=$1`, rescuer); err != nil {
					t.Fatal(err)
				}
			}
			var runners []uuid.UUID
			for _, reference := range []string{"transport", "unit"} {
				var runner uuid.UUID
				var transportID, unitID *uuid.UUID
				if reference == "transport" {
					transportID = &f.transportID
				} else {
					unitID = &f.shipID
				}
				if err := pool.QueryRow(ctx, `INSERT INTO messengers(world_id,sender_id,origin_id,destination_id,message_text,kind,status,hex_q,hex_r,arrives_at,passage_status,carrier_transport_id,carrier_unit_id) VALUES($1,$2,$3,$4,'SEALED PRIVATE TEXT','message','outbound',3,0,now(),'aboard',$5,$6) RETURNING id`, f.worldID, f.owner, f.sourceID, f.destID, transportID, unitID).Scan(&runner); err != nil {
					t.Fatal(err)
				}
				runners = append(runners, runner)
			}
			h := NewInterceptScanHandler(pool, events.NewScheduler(pool, f.clk), events.NewStore(pool), nil, f.clk)
			h.Dice = fixedDice{tc.roll}
			if err := h.Handle(ctx, events.ScheduledEvent{WorldID: f.worldID, DueTick: 1}); err != nil {
				t.Fatal(err)
			}
			// Replay THIS seizure, not a scan of the new limped-home voyage (which
			// is a separate interceptable transport with its own physical risks).
			if err := h.seize(ctx, f.worldID, inFlightTransport{id: f.transportID}, rescuer, f.raider, province.MapPosition{Q: 2, R: 0}); err != nil {
				t.Fatal(err)
			}
			for _, runner := range runners {
				var count int
				var raw []byte
				var stream string
				if err := pool.QueryRow(ctx, `SELECT count(*) FROM events WHERE stream_id=$1 AND event_type IN `+carrier.TypesSQL, runner).Scan(&count); err != nil || count != 1 {
					t.Fatalf("one witness per runner on retry: %d %v", count, err)
				}
				if err := pool.QueryRow(ctx, `SELECT payload,stream_type FROM events WHERE stream_id=$1 AND event_type=$2`, runner, tc.kind).Scan(&raw, &stream); err != nil {
					t.Fatal(err)
				}
				var w carrier.Witness
				if err := json.Unmarshal(raw, &w); err != nil {
					t.Fatal(err)
				}
				if stream != "messenger" || w.SenderID != f.owner {
					t.Fatalf("private stream and sender ownership: %s %s", stream, raw)
				}
				if tc.kind != carrier.Lost {
					expected := f.shipID
					if tc.naval {
						expected = rescuer
					}
					if w.RescueShipID == nil || *w.RescueShipID != expected {
						t.Fatalf("rescue must name actual physical ship %s: %s", expected, raw)
					}
				}
				var passage string
				if err := pool.QueryRow(ctx, `SELECT passage_status FROM messengers WHERE id=$1`, runner).Scan(&passage); err != nil || passage != "aboard" {
					t.Fatalf("lower layer must not write messenger projection: %s %v", passage, err)
				}
			}
		})
	}
}
