package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/tick"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func caravanState(t *testing.T, pool *pgxpool.Pool, world uuid.UUID) string {
	t.Helper()
	var state string
	err := pool.QueryRow(context.Background(), `SELECT jsonb_build_object(
 'goods',(SELECT jsonb_agg(to_jsonb(g) ORDER BY g.settlement_id,g.good_key) FROM settlement_goods g JOIN settlements s ON s.id=g.settlement_id WHERE s.world_id=$1),
 'ships',(SELECT jsonb_agg(to_jsonb(u) ORDER BY id) FROM units u WHERE world_id=$1),
 'offers',(SELECT jsonb_agg(to_jsonb(m) ORDER BY id) FROM messengers m WHERE world_id=$1),
 'loyalty',(SELECT jsonb_agg(to_jsonb(s) ORDER BY id) FROM settlements s WHERE world_id=$1),
 'transports',(SELECT count(*) FROM transports WHERE world_id=$1),
 'routes',(SELECT count(*) FROM trade_routes WHERE world_id=$1),
 'jobs',(SELECT count(*) FROM scheduled_events WHERE world_id=$1),
 'events',(SELECT count(*) FROM events WHERE world_id=$1))::text`, world).Scan(&state)
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func TestCaravanTransfer_NoRouteHasNoSideEffects(t *testing.T) {
	f := setupTradeInternalFixture(t)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `DELETE FROM map_tiles WHERE world_id=$1 AND q=1`, f.worldID); err != nil {
		t.Fatal(err)
	}
	before := caravanState(t, f.pool, f.worldID)
	code, resp := f.post(t, "/worlds/"+f.worldID.String()+"/provinces/"+f.originProvince.String()+"/trade", map[string]any{"destination_id": f.destID, "good_key": "silver", "quantity": 100})
	if code != 422 {
		t.Fatalf("status %d: %v", code, resp)
	}
	if after := caravanState(t, f.pool, f.worldID); after != before {
		t.Fatalf("refused route changed state\nbefore %s\nafter %s", before, after)
	}
}

func TestCaravanTransfer_BentPathSavedAndETAAnchored(t *testing.T) {
	f := setupTradeInternalFixture(t)
	ctx := context.Background()
	originalCadence := tick.TickSeconds
	tick.TickSeconds = 6
	t.Cleanup(func() { tick.TickSeconds = originalCadence })
	if _, err := f.pool.Exec(ctx, `DELETE FROM map_tiles WHERE world_id=$1`, f.worldID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO map_tiles(world_id,q,r,terrain) VALUES
 ($1,0,0,'forest_cedar'),($1,0,1,'plains'),($1,1,1,'river_ford'),($1,2,1,'plains'),($1,3,0,'plains')`, f.worldID); err != nil {
		t.Fatal(err)
	}
	code, resp := f.post(t, "/worlds/"+f.worldID.String()+"/provinces/"+f.originProvince.String()+"/trade", map[string]any{"destination_id": f.destID, "good_key": "silver", "quantity": 100})
	if code != 201 {
		t.Fatalf("status %d: %v", code, resp)
	}
	var raw []byte
	var dep, arr, due int
	if err := f.pool.QueryRow(ctx, `SELECT t.journey,t.departed_tick,t.due_tick,e.due_tick FROM transports t JOIN scheduled_events e ON e.payload->>'transport_id'=t.id::text WHERE t.id=$1`, resp["transport_id"]).Scan(&raw, &dep, &arr, &due); err != nil {
		t.Fatal(err)
	}
	var journey province.TradeJourney
	if err := json.Unmarshal(raw, &journey); err != nil {
		t.Fatal(err)
	}
	want := []province.MapPosition{{Q: 0, R: 0}, {Q: 0, R: 1}, {Q: 1, R: 1}, {Q: 2, R: 1}, {Q: 3, R: 0}}
	if !reflect.DeepEqual(journey.Path, want) || !reflect.DeepEqual(journey.StepCosts, []int64{750, 2500, 750, 750}) || journey.TravelTicks != 7 {
		t.Fatalf("journey=%+v", journey)
	}
	if arr-dep != 7 || due != arr || resp["arrival_tick"] != float64(arr) || resp["travel_ticks"] != float64(7) {
		t.Fatalf("ticks dep%d arr%d due%d response%v", dep, arr, due, resp)
	}
	parsed, err := time.Parse(time.RFC3339Nano, resp["arrives_at"].(string))
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Sub(f.clk.Now()) != tick.RealUntil(7, 0) {
		t.Fatalf("wall ETA duration %v", parsed.Sub(f.clk.Now()))
	}
}

// Inject the persistence failure after ship binding and goods debit, proving rollback of all writes.
func TestCaravanTransfer_ScheduleFailureRollsBackShipAndGoods(t *testing.T) {
	f := setupTradeNavalFixture(t)
	origin, prov := f.settlement(t, "Byblos", 0, true, true)
	dest, _ := f.settlement(t, "Ugarit", 5, true, false)
	for q := 1; q < 5; q++ {
		f.mapTile(t, q, 0, "coastal_sea")
	}
	f.ship(t, origin, "merchantman")
	installCaravanJobFailure(t, f.pool, f.worldID)
	before := caravanState(t, f.pool, f.worldID)
	code, resp := f.post(t, "/worlds/"+f.worldID.String()+"/provinces/"+prov.String()+"/trade", map[string]any{"destination_id": dest, "good_key": "grain", "quantity": 10})
	if code != 500 {
		t.Fatalf("status%d: %v", code, resp)
	}
	if after := caravanState(t, f.pool, f.worldID); after != before {
		t.Fatal("failed schedule leaked ship, debit, route, transport or job")
	}
}

func installCaravanJobFailure(t *testing.T, pool *pgxpool.Pool, world uuid.UUID) {
	t.Helper()
	name := "caravan_fail_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	sql := fmt.Sprintf(`CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.world_id='%s'::uuid AND NEW.event_type IN ('TradeDelivery','TransportArrival') THEN RAISE EXCEPTION 'caravan schedule failure injection'; END IF; RETURN NEW; END $$; CREATE TRIGGER %s BEFORE INSERT ON scheduled_events FOR EACH ROW EXECUTE FUNCTION %s()`, name, world, name, name)
	if _, err := pool.Exec(context.Background(), sql); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, err := pool.Exec(context.Background(), fmt.Sprintf(`DROP TRIGGER %s ON scheduled_events; DROP FUNCTION %s()`, name, name))
		if err != nil {
			t.Error(err)
		}
	})
}

func setupLandCaravanOffer(t *testing.T, kind string) (*navalPlayerFixture, uuid.UUID, uuid.UUID, uuid.UUID) {
	t.Helper()
	f := setupNavalPlayerFixture(t)
	origin := f.settlement(t, "Seller", 0, f.initiatorID, false)
	dest := f.settlement(t, "Buyer", 3, f.counterpartyID, false)
	f.mapTile(t, 1, 0, "river_ford")
	f.mapTile(t, 2, 0, "plains")
	if _, err := f.pool.Exec(context.Background(), `UPDATE map_tiles SET terrain='forest_cedar' WHERE world_id=$1 AND q=0`, f.worldID); err != nil {
		t.Fatal(err)
	}
	f.good(t, origin, "silver", 500)
	f.good(t, origin, "copper", 100)
	f.good(t, dest, "silver", 500)
	f.good(t, dest, "copper", 100)
	offer := map[string]any{"kind": "sell", "offer_good": "copper", "offer_qty": 20, "want_silver": 80}
	if kind == "buy" {
		offer = map[string]any{"kind": "buy", "want_good": "copper", "want_qty": 20, "offer_silver": 80}
	}
	code, resp := f.post(t, f.initiatorToken, "/worlds/"+f.worldID.String()+"/settlements/"+origin.String()+"/messengers", map[string]any{"destination_id": dest, "message": "Trade", "trade_offer": offer})
	if code != 201 {
		t.Fatalf("send status%d: %v", code, resp)
	}
	id, err := uuid.Parse(resp["id"].(string))
	if err != nil {
		t.Fatal(err)
	}
	f.deliverMessenger(t, id)
	return f, origin, dest, id
}

func TestCaravanAccept_NoRouteHasNoSideEffects(t *testing.T) {
	f, _, _, id := setupLandCaravanOffer(t, "sell")
	if _, err := f.pool.Exec(context.Background(), `DELETE FROM map_tiles WHERE world_id=$1 AND q=1`, f.worldID); err != nil {
		t.Fatal(err)
	}
	before := caravanState(t, f.pool, f.worldID)
	code, resp := f.post(t, f.counterpartyToken, "/worlds/"+f.worldID.String()+"/messengers/"+id.String()+"/trade-accept", nil)
	if code != 422 {
		t.Fatalf("accept status%d: %v", code, resp)
	}
	if after := caravanState(t, f.pool, f.worldID); after != before {
		t.Fatal("no route changed trade state")
	}
}

func TestCaravanAccept_ScheduleFailureLeavesOfferPending(t *testing.T) {
	f, _, _, id := setupLandCaravanOffer(t, "sell")
	installCaravanJobFailure(t, f.pool, f.worldID)
	before := caravanState(t, f.pool, f.worldID)
	code, resp := f.post(t, f.counterpartyToken, "/worlds/"+f.worldID.String()+"/messengers/"+id.String()+"/trade-accept", nil)
	if code != 500 {
		t.Fatalf("accept status%d: %v", code, resp)
	}
	if after := caravanState(t, f.pool, f.worldID); after != before {
		t.Fatal("schedule failure changed offer, debit or physical caravan")
	}
}

func TestCaravanAccept_ConcurrentBuyAndSellCommitOnceWithAsymmetricETA(t *testing.T) {
	for _, kind := range []string{"buy", "sell"} {
		t.Run(kind, func(t *testing.T) {
			f, _, dest, id := setupLandCaravanOffer(t, kind)
			path := "/worlds/" + f.worldID.String() + "/messengers/" + id.String() + "/trade-accept"
			results := make(chan *httptest.ResponseRecorder, 2)
			for i := 0; i < 2; i++ {
				go func() {
					req := httptest.NewRequest(http.MethodPost, path, strings.NewReader("null"))
					req.Header.Set("Authorization", "Bearer "+f.counterpartyToken)
					rec := httptest.NewRecorder()
					f.router.ServeHTTP(rec, req)
					results <- rec
				}()
			}
			success, conflict := 0, 0
			for i := 0; i < 2; i++ {
				rec := <-results
				if rec.Code == 200 {
					success++
					var resp map[string]any
					if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
						t.Fatal(err)
					}
					if resp["travel_ticks"] != float64(6) || resp["return_travel_ticks"] != float64(8) {
						t.Fatalf("asymmetric ETA: %v", resp)
					}
				} else if rec.Code == 409 {
					conflict++
				} else {
					t.Fatalf("accept status%d: %s", rec.Code, rec.Body.String())
				}
			}
			if success != 1 || conflict != 1 {
				t.Fatalf("accepted%d conflict%d", success, conflict)
			}
			var amount float64
			good := "silver"
			want := 420.
			if kind == "buy" {
				good = "copper"
				want = 80
			}
			if err := f.pool.QueryRow(context.Background(), `SELECT amount FROM settlement_goods WHERE settlement_id=$1 AND good_key=$2`, dest, good).Scan(&amount); err != nil {
				t.Fatal(err)
			}
			if amount != want {
				t.Fatalf("debited twice: %f wanted%f", amount, want)
			}
			var raw []byte
			var count int
			if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM transports WHERE world_id=$1`, f.worldID).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 1 {
				t.Fatalf("transport count%d", count)
			}
			if err := f.pool.QueryRow(context.Background(), `SELECT payload FROM scheduled_events WHERE world_id=$1 AND event_type='TradeDelivery'`, f.worldID).Scan(&raw); err != nil {
				t.Fatal(err)
			}
			var payload struct {
				Return struct {
					Journey province.TradeJourney `json:"journey"`
					Ticks   int                   `json:"travel_ticks"`
				} `json:"then_return"`
			}
			if err := json.Unmarshal(raw, &payload); err != nil {
				t.Fatal(err)
			}
			if payload.Return.Ticks != 8 || payload.Return.Journey.TravelTicks != 8 {
				t.Fatalf("return journey%+v", payload)
			}
		})
	}
}

func TestCaravanGift_NoRouteHasNoSideEffects(t *testing.T) {
	f := setupTradeInternalFixture(t)
	h := NewSettlementHandler(f.pool, events.NewStore(f.pool), f.scheduler, f.clk, economy.SitosConfig{})
	f.router.Post("/worlds/{worldID}/settlements/{settlementID}/gift", h.Gift)
	if _, err := f.pool.Exec(context.Background(), `DELETE FROM map_tiles WHERE world_id=$1 AND q=1`, f.worldID); err != nil {
		t.Fatal(err)
	}
	before := caravanState(t, f.pool, f.worldID)
	code, resp := f.post(t, "/worlds/"+f.worldID.String()+"/settlements/"+f.destID.String()+"/gift", map[string]any{"silver": 100})
	if code != 422 {
		t.Fatalf("gift status%d: %v", code, resp)
	}
	if after := caravanState(t, f.pool, f.worldID); after != before {
		t.Fatal("refused gift changed goods, loyalty or jobs")
	}
}
