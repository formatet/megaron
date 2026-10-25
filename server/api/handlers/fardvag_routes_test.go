package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/transport"
	"github.com/google/uuid"
)

func assertThirdPartyMapPrivacy(t *testing.T, row map[string]any) {
	t.Helper()
	for _, key := range []string{"path", "step_costs", "origin_q", "origin_r", "dest_q", "dest_r", "target_q", "target_r", "sent_at", "departs_at", "arrives_at", "arrival_tick", "departure_tick", "travel_ticks", "leg_start", "leg_end", "carrier_unit_id", "carrier_transport_id", "ship_unit_id"} {
		if _, ok := row[key]; ok {
			t.Fatalf("third-party map actor leaked %s", key)
		}
	}
	if row["current_q"] == nil || row["current_r"] == nil || row["heading"] == "" {
		t.Fatalf("foreign mover lacks current hex and compass: %+v", row)
	}
}
func TestFardvagThirdPartyTradeProjection(t *testing.T) {
	f := setupMapTradesMineFixture(t)
	id := f.strangerCaravan(t, 12, 1, time.Now().Add(-time.Hour), time.Now().Add(time.Minute))
	row := findByID(f.get(t, f.tokenA), id)
	if row == nil {
		t.Fatal("visible foreign caravan missing")
	}
	assertThirdPartyMapPrivacy(t, row)
}
func TestFardvagThirdPartyMessengerProjectionAndRecipientKnowledge(t *testing.T) {
	f := setupMapTradesMineFixture(t)
	ctx := context.Background()
	var sender, origin, dest uuid.UUID
	if err := f.pool.QueryRow(ctx, `INSERT INTO players(username,password_hash) VALUES($1,'x') RETURNING id`, "runner-"+uuid.New().String()).Scan(&sender); err != nil {
		t.Fatal(err)
	}
	origin = insertSettlement(t, ctx, f.pool, f.worldID, insertProvince(t, ctx, f.pool, f.worldID, 2, 0), "Nearport", sender, true)
	dest = insertSettlement(t, ctx, f.pool, f.worldID, insertProvince(t, ctx, f.pool, f.worldID, 12, 0), "Beyond", sender, false)
	now := time.Now().UTC()
	var id uuid.UUID
	if err := f.pool.QueryRow(ctx, `INSERT INTO messengers(world_id,sender_id,origin_id,destination_id,kind,message_text,status,hex_q,hex_r,sent_at,arrives_at) VALUES($1,$2,$3,$4,'message','hi','outbound',2,0,$5,$6) RETURNING id`, f.worldID, sender, origin, dest, now, now.Add(time.Hour)).Scan(&id); err != nil {
		t.Fatal(err)
	}
	h := NewWorldHandler(f.pool, nil, clock.NewTestClock(now))
	f.router.Get("/worlds/{worldID}/messengers", h.MapMessengers)
	fetch := func() map[string]any {
		req := httptest.NewRequest(http.MethodGet, "/worlds/"+f.worldID.String()+"/messengers", nil)
		req.Header.Set("Authorization", "Bearer "+f.tokenA)
		rec := httptest.NewRecorder()
		f.router.ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatal(rec.Code, rec.Body.String())
		}
		var rows []map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &rows); err != nil {
			t.Fatal(err)
		}
		return findByID(rows, id.String())
	}
	row := fetch()
	if row == nil {
		t.Fatal("visible foreign runner missing")
	}
	assertThirdPartyMapPrivacy(t, row)
	if _, err := f.pool.Exec(ctx, `UPDATE settlements SET owner_id=$2 WHERE id=$1`, dest, f.playerA); err != nil {
		t.Fatal(err)
	}
	row = fetch()
	if row == nil || row["dest_q"] != float64(12) || row["arrives_at"] == nil || row["current_q"] == nil {
		t.Fatalf("recipient lost existing correspondence knowledge: %+v", row)
	}
}
func TestFardvagFrozenTradePathCostsAndWeightedHeading(t *testing.T) {
	f := setupMapTradesMineFixture(t)
	ctx := context.Background()
	j := province.TradeJourney{Category: "land", Path: []province.MapPosition{{Q: 0, R: 0}, {Q: 0, R: 1}, {Q: 1, R: 1}, {Q: 2, R: 0}}, StepCosts: []int64{750, 2500, 750}, TravelTicks: 6, Distance: 3}
	raw, _ := json.Marshal(j)
	if _, err := f.pool.Exec(ctx, `UPDATE transports SET journey=$1,departed_tick=10,due_tick=16 WHERE world_id=$2 AND owner_id=$3`, raw, f.worldID, f.playerA); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE worlds SET current_tick=12,last_tick_at=now() WHERE id=$1`, f.worldID); err != nil {
		t.Fatal(err)
	}
	row := findByGood(t, f.get(t, f.tokenA), "grain")
	wantPath, _ := json.Marshal(pathCoordinates(j.Path))
	gotPath, _ := json.Marshal(row["path"])
	wantCosts, _ := json.Marshal(j.StepCosts)
	gotCosts, _ := json.Marshal(row["step_costs"])
	if !reflect.DeepEqual(wantPath, gotPath) || !reflect.DeepEqual(wantCosts, gotCosts) {
		t.Fatalf("own caravan lost frozen path/costs: %+v", row)
	}
	pos, err := transport.SavedPosition(raw, 10, 16, 12000)
	if err != nil {
		t.Fatal(err)
	}
	if pos != (province.MapPosition{Q: 1, R: 1}) {
		t.Fatal("weighted fixture no longer exercises middle slow hex")
	}
	if heading := frozenHeading(j.Path, j.StepCosts, 10, 16, 12000); heading != province.ScreenCompass(j.Path[2], j.Path[3]) {
		t.Fatalf("heading came from unweighted wrong step: %q", heading)
	}
}
func TestFardvagWorldReturnsActualTickAnchor(t *testing.T) {
	f := setupMapTradesMineFixture(t)
	ctx := context.Background()
	at := time.Date(2026, 10, 12, 7, 0, 12, 0, time.UTC)
	if _, err := f.pool.Exec(ctx, `UPDATE worlds SET current_tick=37,last_tick_at=$2 WHERE id=$1`, f.worldID, at); err != nil {
		t.Fatal(err)
	}
	h := NewWorldHandler(f.pool, nil, clock.NewTestClock(at.Add(time.Minute)))
	f.router.Get("/worlds/{worldID}", h.Get)
	req := httptest.NewRequest(http.MethodGet, "/worlds/"+f.worldID.String(), nil)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	var row map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &row); err != nil {
		t.Fatal(err)
	}
	if row["tick_anchor_at"] != at.Format(time.RFC3339) || row["current_tick"] != float64(37) {
		t.Fatalf("world tick anchor reflects fetch time instead of actual tick start: %+v", row)
	}
}

func TestFardvagReturningRunnerUsesActualActiveWindow(t *testing.T) {
	f := setupMapTradesMineFixture(t)
	ctx := context.Background()
	var origin, dest uuid.UUID
	if err := f.pool.QueryRow(ctx, `SELECT id FROM settlements WHERE world_id=$1 AND owner_id=$2`, f.worldID, f.playerA).Scan(&origin); err != nil {
		t.Fatal(err)
	}
	dest = insertSettlement(t, ctx, f.pool, f.worldID, insertProvince(t, ctx, f.pool, f.worldID, 12, 0), "Farcity", f.playerB, false)
	now := time.Now().UTC()
	start, end := now.Add(-time.Hour), now.Add(time.Hour)
	var id uuid.UUID
	if err := f.pool.QueryRow(ctx, `INSERT INTO messengers(world_id,sender_id,origin_id,destination_id,kind,message_text,status,hex_q,hex_r,sent_at,return_departs_at,arrives_at) VALUES($1,$2,$3,$4,'message','hi','returning',0,0,$5,$6,$7) RETURNING id`, f.worldID, f.playerA, origin, dest, now.Add(-10*time.Hour), start, end).Scan(&id); err != nil {
		t.Fatal(err)
	}
	h := NewWorldHandler(f.pool, nil, clock.NewTestClock(now))
	f.router.Get("/worlds/{worldID}/messengers", h.MapMessengers)
	req := httptest.NewRequest(http.MethodGet, "/worlds/"+f.worldID.String()+"/messengers", nil)
	req.Header.Set("Authorization", "Bearer "+f.tokenA)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	var rows []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	row := findByID(rows, id.String())
	if row == nil || row["current_q"] != float64(6) || row["current_r"] != float64(0) {
		t.Fatalf("returning runner current position used obsolete outbound/stay duration: %+v", row)
	}
	path := row["path"].([]any)
	if !reflect.DeepEqual(path[0], []any{float64(12), float64(0)}) || !reflect.DeepEqual(path[len(path)-1], []any{float64(0), float64(0)}) {
		t.Fatalf("returning runner path is not current homeward leg: %+v", path)
	}
}
func TestFardvagOwnRunnerCurrentLandAndFrozenCarrierPaths(t *testing.T) {
	f := setupMapTradesMineFixture(t)
	ctx := context.Background()
	var origin, dest uuid.UUID
	if err := f.pool.QueryRow(ctx, `SELECT id FROM settlements WHERE world_id=$1 AND owner_id=$2`, f.worldID, f.playerA).Scan(&origin); err != nil {
		t.Fatal(err)
	}
	dest = insertSettlement(t, ctx, f.pool, f.worldID, insertProvince(t, ctx, f.pool, f.worldID, 12, 0), "Far", f.playerB, false)
	now := time.Now().UTC()
	var id uuid.UUID
	if err := f.pool.QueryRow(ctx, `INSERT INTO messengers(world_id,sender_id,origin_id,destination_id,kind,message_text,status,hex_q,hex_r,sent_at,arrives_at) VALUES($1,$2,$3,$4,'message','hi','outbound',0,0,$5,$6) RETURNING id`, f.worldID, f.playerA, origin, dest, now.Add(-time.Hour), now.Add(time.Hour)).Scan(&id); err != nil {
		t.Fatal(err)
	}
	h := NewWorldHandler(f.pool, nil, clock.NewTestClock(now))
	f.router.Get("/worlds/{worldID}/messengers", h.MapMessengers)
	fetch := func() map[string]any {
		req := httptest.NewRequest(http.MethodGet, "/worlds/"+f.worldID.String()+"/messengers", nil)
		req.Header.Set("Authorization", "Bearer "+f.tokenA)
		rec := httptest.NewRecorder()
		f.router.ServeHTTP(rec, req)
		var rows []map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &rows); err != nil {
			t.Fatal(err)
		}
		return findByID(rows, id.String())
	}
	row := fetch()
	if row == nil || row["current_q"] != float64(6) || len(row["path"].([]any)) != 13 {
		t.Fatalf("own current land leg missing real path: %+v", row)
	}
	// The carrier's saved weighted detour is deliberately unlike today's terrain.
	var ship uuid.UUID
	route := map[string]any{"start_tick": 10, "end_tick": 16, "hexes": [][2]int{{0, 0}, {0, 1}, {1, 1}, {2, 0}}, "costs": []int64{750, 2500, 750}}
	raw, _ := json.Marshal(route)
	if err := f.pool.QueryRow(ctx, `INSERT INTO units(world_id,owner_id,type,category,size,crew,status,q,r,target_q,target_r,march_route,depart_tick,arrive_tick) VALUES($1,$2,'galley','naval',1,20,'marching',0,0,2,0,$3,10,16) RETURNING id`, f.worldID, f.playerA, raw).Scan(&ship); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE worlds SET current_tick=12,last_tick_at=$2 WHERE id=$1`, f.worldID, now); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE messengers SET passage_status='aboard',passage_port_id=$2,carrier_unit_id=$3,disembark_q=2,disembark_r=0,boarded_at=$4,disembark_at=$5 WHERE id=$1`, id, origin, ship, now.Add(-time.Hour), now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	row = fetch()
	if row == nil || row["current_q"] != float64(1) || row["current_r"] != float64(1) || row["departure_tick"] != float64(10) || row["arrival_tick"] != float64(16) || len(row["step_costs"].([]any)) != 3 {
		t.Fatalf("aboard runner lost carrier's frozen path/weighted clock: %+v", row)
	}

	// A late boarding scan must not restart the carrier's saved route clock.
	departure := now.Add(-2 * time.Hour).Truncate(time.Microsecond)
	route["version"] = 2
	route["offset_milli"] = 733
	route["departure_at"] = departure
	route["tick_nanoseconds"] = int64(time.Hour)
	raw, _ = json.Marshal(route)
	if _, err := f.pool.Exec(ctx, `UPDATE units SET march_route=$2 WHERE id=$1`, ship, raw); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE messengers SET boarded_at=$2,disembark_at=$3,arrives_at=$4 WHERE id=$1`, id, now.Add(-time.Minute), departure.Add(6*time.Hour), departure.Add(7*time.Hour)); err != nil {
		t.Fatal(err)
	}
	row = fetch()
	got, err := time.Parse(time.RFC3339Nano, row["leg_start"].(string))
	if err != nil || !got.Equal(departure) {
		t.Fatalf("aboard route restarted at scan time: got%v want saved departure%v err%v", got, departure, err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE messengers SET passage_status='awaiting_passage',carrier_unit_id=NULL,arrives_at=$2 WHERE id=$1`, id, now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	row = fetch()
	if _, exists := row["path"]; exists {
		t.Fatal("waiting runner invented forward route")
	}
}

func TestFardvagAgreedRecipientKnowsRouteGiftDoesNot(t *testing.T) {
	for _, kind := range []string{"trade", "trade_return", "gift", "transfer", "standing_order_out", "standing_order_return", "plunder", "ship_return", "damaged_return"} {
		t.Run(kind, func(t *testing.T) {
			f := setupMapTradesMineFixture(t)
			ctx := context.Background()
			j := province.TradeJourney{Category: "land", Path: []province.MapPosition{{Q: 3, R: 0}, {Q: 2, R: 0}, {Q: 1, R: 0}, {Q: 0, R: 0}}, StepCosts: []int64{750, 2500, 750}, Distance: 3, TravelTicks: 6}
			raw, _ := json.Marshal(j)
			if _, err := f.pool.Exec(ctx, `UPDATE transports SET kind=$1,journey=$2,departed_tick=10,due_tick=16 WHERE world_id=$3 AND owner_id=$4 AND dest_q=0 AND dest_r=0`, kind, raw, f.worldID, f.playerB); err != nil {
				t.Fatal(err)
			}
			if _, err := f.pool.Exec(ctx, `UPDATE worlds SET current_tick=12,last_tick_at=now() WHERE id=$1`, f.worldID); err != nil {
				t.Fatal(err)
			}
			row := findByGood(t, f.get(t, f.tokenA), "copper")
			if row["mine"] != false || row["role"] != "recipient" || row["kind"] != kind {
				t.Fatalf("recipient ownership/kind was changed: %+v", row)
			}
			agreed := kind == "trade" || kind == "trade_return"
			if agreed {
				if row["route_known"] != true || len(row["path"].([]any)) != 4 || len(row["step_costs"].([]any)) != 3 {
					t.Fatalf("agreed recipient lost frozen route and costs: %+v", row)
				}
			} else {
				_, path := row["path"]
				_, costs := row["step_costs"]
				if row["route_known"] != false || path || costs {
					t.Fatalf("unagreed %s recipient received route knowledge: %+v", kind, row)
				}
			}
			if row["quantity"] != float64(7) || row["arrives_at"] == nil || row["arrival_tick"] != float64(16) {
				t.Fatal("party lost existing incoming shipment cargo/ETA")
			}
		})
	}
}
func TestFardvagGiftSenderStillKnowsOwnRoute(t *testing.T) {
	f := setupMapTradesMineFixture(t)
	ctx := context.Background()
	j := province.TradeJourney{Category: "land", Path: []province.MapPosition{{Q: 0, R: 0}, {Q: 1, R: 0}, {Q: 2, R: 0}, {Q: 3, R: 0}}, StepCosts: []int64{750, 2500, 750}, Distance: 3, TravelTicks: 6}
	raw, _ := json.Marshal(j)
	if _, err := f.pool.Exec(ctx, `UPDATE transports SET kind='gift',journey=$1,departed_tick=10,due_tick=16 WHERE world_id=$2 AND owner_id=$3`, raw, f.worldID, f.playerA); err != nil {
		t.Fatal(err)
	}
	row := findByGood(t, f.get(t, f.tokenA), "grain")
	if row["mine"] != true || row["route_known"] != true || len(row["path"].([]any)) != 4 || len(row["step_costs"].([]any)) != 3 {
		t.Fatalf("gift sender lost known owned journey: %+v", row)
	}
}
