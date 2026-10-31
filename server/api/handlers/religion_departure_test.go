package handlers

import (
	"context"
	"encoding/json"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/religion"
	"formatet/megaron/server/internal/transport"
	"github.com/google/uuid"
	"testing"
)

func TestBetaDeparturePrayerCastConsumeLateAndReturn(t *testing.T) {
	for _, effect := range []string{religion.EffectNextLandDeparture, religion.EffectNextShipDeparture} {
		t.Run(effect, func(t *testing.T) {
			f := newAlphaFixture(t)
			ctx := context.Background()
			prayer := religion.PrayerAsterionThread
			cat := "land"
			if effect == religion.EffectNextShipDeparture {
				prayer = religion.PrayerStillnessPassage
				cat = "naval"
			}
			if r := f.cast(f.cities[0], prayer); r.Code != 200 {
				t.Fatalf("cast %d %s", r.Code, r.Body.String())
			}
			tx, err := f.pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			j := province.TradeJourney{Category: cat, Path: []province.MapPosition{{Q: 0}, {Q: 1}, {Q: 2}, {Q: 3}, {Q: 4}}, StepCosts: []int64{1000, 1000, 1000, 1000}, Distance: 4, TravelTicks: 6}
			if err := transport.BlessDeparture(ctx, tx, f.world, f.player, f.cities[0], &j, true); err != nil {
				t.Fatal(err)
			}
			if j.TravelTicks != 5 || (effect == religion.EffectNextLandDeparture && !j.Thread) || (effect == religion.EffectNextShipDeparture && !j.Stillness) {
				t.Fatalf("missing departure blessing %+v", j)
			}
			before, _ := json.Marshal(j)
			if err := tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			// No renewal even after divine waiting time when the blessing is still pending.
			f.clearRest(t)
			if r := f.cast(f.cities[0], prayer); r.Code != 200 {
				t.Fatalf("new gift %d %s", r.Code, r.Body.String())
			}
			after, _ := json.Marshal(j)
			if string(before) != string(after) {
				t.Fatal("prayer after departure altered frozen journey")
			}
			// A fresh prayer at the return city must remain pending on a return leg too.
			_, err = f.pool.Exec(ctx, `INSERT INTO departure_prayers(settlement_id,owner_id,effect) VALUES($1,$2,$3)`, f.cities[1], f.player, effect)
			if err != nil {
				t.Fatal(err)
			}
			tx, err = f.pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			ret := province.TradeJourney{Category: cat, Path: []province.MapPosition{{Q: 4}, {Q: 3}, {Q: 2}, {Q: 1}, {Q: 0}}, StepCosts: []int64{1000, 1000, 1000, 1000}, Distance: 4, TravelTicks: 6}
			if err := transport.BlessDeparture(ctx, tx, f.world, f.player, f.cities[1], &ret, false); err != nil {
				t.Fatal(err)
			}
			if ret.TravelTicks != 6 || ret.Thread || ret.Stillness {
				t.Fatalf("return inherited blessing %+v", ret)
			}
			pending, err := religion.DeparturePending(ctx, tx, f.cities[1], f.player, effect)
			if err != nil || !pending {
				t.Fatalf("return consumed another gift %v %v", pending, err)
			}
			if err := tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			f.clearRest(t)
			if r := f.cast(f.cities[0], prayer); r.Code != 409 {
				t.Fatalf("renewed pending gift %d %s", r.Code, r.Body.String())
			}
		})
	}
}

func TestBetaDepartureOwnerStampAndRollback(t *testing.T) {
	f := newAlphaFixture(t)
	ctx := context.Background()
	if r := f.cast(f.cities[0], religion.PrayerAsterionThread); r.Code != 200 {
		t.Fatal(r.Body.String())
	}
	var foreign uuid.UUID
	if err := f.pool.QueryRow(ctx, `INSERT INTO players(username,password_hash) VALUES($1,'x') RETURNING id`, uuid.NewString()).Scan(&foreign); err != nil {
		t.Fatal(err)
	}
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `UPDATE settlements SET owner_id=$2 WHERE id=$1`, f.cities[0], foreign); err != nil {
		t.Fatal(err)
	}
	if b, err := religion.ConsumeDeparture(ctx, tx, f.world, foreign, f.cities[0], religion.EffectNextLandDeparture); err != nil || b {
		t.Fatalf("new owner inherited %v %v", b, err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	tx, err = f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if b, err := religion.ConsumeDeparture(ctx, tx, f.world, f.player, f.cities[0], religion.EffectNextLandDeparture); err != nil || !b {
		t.Fatalf("consume %v %v", b, err)
	}
	tx.Rollback(ctx)
	if b, err := religion.DeparturePending(ctx, f.pool, f.cities[0], f.player, religion.EffectNextLandDeparture); err != nil || !b {
		t.Fatal("failed departure consumed gift")
	}
}

func TestBetaPrayerAfterActualTransportDeparture(t *testing.T) {
	f := newAlphaFixture(t)
	ctx := context.Background()
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	j := province.TradeJourney{Category: "land", Path: []province.MapPosition{{Q: 0}, {Q: 1}}, StepCosts: []int64{1000}, Distance: 1, TravelTicks: 2}
	id, err := transport.CreateShadow(ctx, tx, transport.DispatchParams{WorldID: f.world, OwnerID: f.player, OriginID: f.cities[0], DestID: f.cities[1], Kind: "transfer", Category: "land", DestQ: 1, Journey: &j, DepartedTick: 100, DepartsAt: f.h.clk.Now(), Interceptable: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var before []byte
	var due int
	if err := f.pool.QueryRow(ctx, `SELECT journey,due_tick FROM transports WHERE id=$1`, id).Scan(&before, &due); err != nil {
		t.Fatal(err)
	}
	if r := f.cast(f.cities[0], religion.PrayerAsterionThread); r.Code != 200 {
		t.Fatal(r.Body.String())
	}
	var after []byte
	var later int
	if err := f.pool.QueryRow(ctx, `SELECT journey,due_tick FROM transports WHERE id=$1`, id).Scan(&after, &later); err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) || later != due {
		t.Fatal("prayer after physical departure changed journey")
	}
	var frozen province.TradeJourney
	json.Unmarshal(after, &frozen)
	if frozen.Thread {
		t.Fatal("late prayer protected departed caravan")
	}
}

func TestBetaRunnerRealSendConsumesThread(t *testing.T) {
	f := setupOriginEyeFixture(t)
	ctx := context.Background()
	owner, token := f.registerPlayer(t, "thread-sender")
	foreign, _ := f.registerPlayer(t, "thread-recipient")
	city := f.settlement(t, "Origin", 0, owner)
	dest := f.settlement(t, "Destination", 4, foreign)
	if _, err := f.pool.Exec(ctx, `INSERT INTO departure_prayers(settlement_id,owner_id,effect) VALUES($1,$2,'next_land_departure')`, city, owner); err != nil {
		t.Fatal(err)
	}
	send := func() uuid.UUID {
		t.Helper()
		code, resp := f.post(t, token, "/worlds/"+f.worldID.String()+"/settlements/"+city.String()+"/messengers", map[string]any{"destination_id": dest.String(), "message": "thread"})
		if code != 201 {
			t.Fatalf("send%d %v", code, resp)
		}
		return uuid.MustParse(resp["id"].(string))
	}
	for q := 0; q <= 4; q++ {
		terrain := "forest_olive_grove"
		if q == 0 {
			terrain = "plains"
		}
		if _, err := f.pool.Exec(ctx, `INSERT INTO map_tiles(world_id,q,r,terrain) VALUES($1,$2,0,$3) ON CONFLICT(world_id,q,r) DO UPDATE SET terrain=EXCLUDED.terrain`, f.worldID, q, terrain); err != nil {
			t.Fatal(err)
		}
	}
	msg := send()
	var due int
	if err := f.pool.QueryRow(ctx, `SELECT due_tick FROM scheduled_events WHERE payload->>'messenger_id'=$1 AND event_type IN ('MessengerArrival','MessengerArrivalV2')`, msg.String()).Scan(&due); err != nil {
		t.Fatal(err)
	}
	// Four forest courier steps at .75 ->3, blessed2.25 rounds2.
	var current int
	if err := f.pool.QueryRow(ctx, `SELECT current_tick FROM worlds WHERE id=$1`, f.worldID).Scan(&current); err != nil {
		t.Fatal(err)
	}
	if due-current != 2 {
		t.Fatalf("Runner departure factor missing: ticks%d", due-current)
	}
	var pending int
	f.pool.QueryRow(ctx, `SELECT count(*) FROM departure_prayers WHERE settlement_id=$1`, city).Scan(&pending)
	if pending != 0 {
		t.Fatal("Runner did not consume Thread")
	}
	_, err := f.pool.Exec(ctx, `INSERT INTO departure_prayers(settlement_id,owner_id,effect) VALUES($1,$2,'next_land_departure')`, city, owner)
	if err != nil {
		t.Fatal(err)
	}
	var after int
	f.pool.QueryRow(ctx, `SELECT due_tick FROM scheduled_events WHERE payload->>'messenger_id'=$1 AND event_type IN ('MessengerArrival','MessengerArrivalV2')`, msg.String()).Scan(&after)
	if after != due {
		t.Fatal("late prayer rewrote Runner arrival")
	}
}

func TestBetaPrayerAfterShipDepartureDoesNotProtectVoyage(t *testing.T) {
	f := newAlphaFixture(t)
	ctx := context.Background()
	var ship uuid.UUID
	if err := f.pool.QueryRow(ctx, `INSERT INTO units(world_id,owner_id,type,category,size,crew,status,q,r,target_q,target_r,depart_tick,arrive_tick) VALUES($1,$2,'galley','naval',1,20,'marching',0,0,4,0,100,104) RETURNING id`, f.world, f.player).Scan(&ship); err != nil {
		t.Fatal(err)
	}
	if r := f.cast(f.cities[0], religion.PrayerStillnessPassage); r.Code != 200 {
		t.Fatal(r.Body.String())
	}
	var protected bool
	var arr int
	if err := f.pool.QueryRow(ctx, `SELECT stillness_depart_tick IS NOT NULL,arrive_tick FROM units WHERE id=$1`, ship).Scan(&protected, &arr); err != nil {
		t.Fatal(err)
	}
	if protected || arr != 104 {
		t.Fatal("late prayer altered or protected the departed ship")
	}
}
