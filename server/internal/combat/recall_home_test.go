package combat

import (
	"context"
	"testing"

	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/unit"
	"github.com/google/uuid"
)

// Start and resolve an actual expedition leg before recalling: its persisted
// q/r now identify the next leg's origin, not the home settlement.
func TestRecallExpeditionLaterLegHome(t *testing.T) {
	for _, naval := range []bool{false, true} {
		name := "land"
		if naval {
			name = "naval"
		}
		t.Run(name, func(t *testing.T) {
			f := newExpeditionFixture(t, -2, 25, nil)
			ctx, pool := context.Background(), testPool(t)
			if naval {
				if _, err := pool.Exec(ctx, `UPDATE map_tiles SET terrain=CASE WHEN q=0 AND r=0 THEN 'plains' ELSE 'coastal_sea' END WHERE world_id=$1`, f.worldID); err != nil {
					t.Fatal(err)
				}
				if _, err := pool.Exec(ctx, `UPDATE units SET type='galley',category='naval',size=1,crew=100 WHERE id=$1`, f.unitID); err != nil {
					t.Fatal(err)
				}
			}
			f.start(t, 9, 0, 20)
			if err := f.h.Handle(ctx, f.nextArrival(t)); err != nil {
				t.Fatal(err)
			}
			var q, r int
			var intent string
			if err := pool.QueryRow(ctx, `SELECT q,r,march_intent FROM units WHERE id=$1`, f.unitID).Scan(&q, &r, &intent); err != nil {
				t.Fatal(err)
			}
			if q == 0 && r == 0 {
				t.Fatal("repro did not reach a later expedition leg")
			}
			if intent != "explore" {
				t.Fatalf("repro already returning: %s", intent)
			}
			// Delivery at the start of this later leg, with the real route/tick anchor.
			if _, err := pool.Exec(ctx, `UPDATE worlds SET last_tick_at=$2 WHERE id=$1`, f.worldID, f.clk.Now()); err != nil {
				t.Fatal(err)
			}
			applied, err := ExecuteRecall(ctx, pool, f.scheduler, f.eventStore, f.clk, RecallOrder{WorldID: f.worldID, UnitID: f.unitID, Mode: "recall"})
			if err != nil {
				t.Fatal(err)
			}
			if applied == nil {
				t.Fatal("recall missed")
			}
			var homeQ, homeR int
			if naval {
				// A ship returns to home's sea neighbour, not the land settlement hex.
				var found bool
				homeQ, homeR, found, err = province.NearestSeaNeighbor(ctx, pool, f.worldID, 0, 0)
				if err != nil || !found {
					t.Fatalf("home sea neighbour: %v, found=%v", err, found)
				}
				if applied.NewTargetQ != homeQ || applied.NewTargetR != homeR {
					t.Fatalf("naval target=(%d,%d), want home sea neighbour (%d,%d)", applied.NewTargetQ, applied.NewTargetR, homeQ, homeR)
				}
				if homeQ == q && homeR == r {
					t.Fatalf("recalled to leg origin (%d,%d)", q, r)
				}
			} else if applied.NewTargetQ != 0 || applied.NewTargetR != 0 {
				t.Fatalf("recall target=(%d,%d), want home (0,0), later-leg origin=(%d,%d)", applied.NewTargetQ, applied.NewTargetR, q, r)
			}
			event := f.nextArrival(t)
			for i := 0; i < 2; i++ {
				if err := f.h.Handle(ctx, event); err != nil {
					t.Fatal(err)
				}
			}
			var status string
			var sid, home *uuid.UUID
			var rows int
			if err := pool.QueryRow(ctx, `SELECT status,settlement_id,home_settlement_id,q,r FROM units WHERE id=$1`, f.unitID).Scan(&status, &sid, &home, &q, &r); err != nil {
				t.Fatal(err)
			}
			if status != "garrison" || sid == nil || *sid != f.capitalID || home != nil {
				t.Fatalf("arrival status=%s settlement=%v home=%v, want home garrison", status, sid, home)
			}
			if q != homeQ || r != homeR {
				t.Fatalf("arrival=(%d,%d), want (%d,%d)", q, r, homeQ, homeR)
			}
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM unit_expeditions WHERE unit_id=$1`, f.unitID).Scan(&rows); err != nil {
				t.Fatal(err)
			}
			if rows != 0 || len(f.eventPayloads(t, unit.EventExpeditionReport)) != 0 {
				t.Fatal("recall retained/reported cancelled expedition")
			}
			if len(f.eventPayloads(t, unit.EventUnitArrived)) != 1 {
				t.Fatal("arrival retry produced duplicate outcomes")
			}
		})
	}
}

func TestRecallPlainAndExpeditionRedirectDestinations(t *testing.T) {
	for _, mode := range []string{"recall", "redirect"} {
		t.Run(mode, func(t *testing.T) {
			f := newExpeditionFixture(t, -2, 16, nil)
			ctx, pool := context.Background(), testPool(t)
			if mode == "redirect" {
				f.start(t, 9, 0, 20)
				if err := f.h.Handle(ctx, f.nextArrival(t)); err != nil {
					t.Fatal(err)
				}
			} else {
				if _, err := pool.Exec(ctx, `INSERT INTO player_scouted_tiles(world_id,player_id,q,r) SELECT world_id,$2,q,r FROM map_tiles WHERE world_id=$1 ON CONFLICT DO NOTHING`, f.worldID, f.ownerID); err != nil {
					t.Fatal(err)
				}
				if _, err := StartMarch(ctx, pool, f.scheduler, f.eventStore, f.clk, MarchOrder{WorldID: f.worldID, PlayerID: f.ownerID, UnitID: f.unitID, TargetQ: 9, TargetR: 0}, nil); err != nil {
					t.Fatal(err)
				}
			}
			q, r := 4, 0
			order := RecallOrder{WorldID: f.worldID, UnitID: f.unitID, Mode: mode}
			wantQ := 0
			if mode == "redirect" {
				order.NewTargetQ, order.NewTargetR = &q, &r
				wantQ = 4
			}
			applied, err := ExecuteRecall(ctx, pool, f.scheduler, f.eventStore, f.clk, order)
			if err != nil {
				t.Fatal(err)
			}
			if applied == nil || applied.NewTargetQ != wantQ || applied.NewTargetR != 0 {
				t.Fatalf("destination changed: %+v", applied)
			}
			if err := f.h.Handle(ctx, f.nextArrival(t)); err != nil {
				t.Fatal(err)
			}
			var status string
			if err := pool.QueryRow(ctx, `SELECT status FROM units WHERE id=$1`, f.unitID).Scan(&status); err != nil {
				t.Fatal(err)
			}
			wantStatus := "garrison"
			if mode == "redirect" {
				wantStatus = "marching" // Redirect preserves Explore intent, including its legacy return leg.
			}
			if status != wantStatus {
				t.Fatalf("status=%s, want %s", status, wantStatus)
			}
			if mode == "redirect" {
				f.runUntilHome(t)
			}
		})
	}
}

func TestRecallExpeditionLostHome(t *testing.T) {
	for _, fallback := range []bool{true, false} {
		t.Run(map[bool]string{true: "fallback", false: "refusal"}[fallback], func(t *testing.T) {
			f := newExpeditionFixture(t, -2, 16, nil)
			ctx, pool := context.Background(), testPool(t)
			f.start(t, 9, 0, 20)
			if err := f.h.Handle(ctx, f.nextArrival(t)); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, `UPDATE settlements SET owner_id=NULL WHERE id=$1`, f.capitalID); err != nil {
				t.Fatal(err)
			}
			var sid uuid.UUID
			if fallback {
				var pid uuid.UUID
				if err := pool.QueryRow(ctx, `INSERT INTO provinces(world_id,map_q,map_r,terrain_type) VALUES($1,-1,0,'plains') RETURNING id`, f.worldID).Scan(&pid); err != nil {
					t.Fatal(err)
				}
				if err := pool.QueryRow(ctx, `INSERT INTO settlements(world_id,province_id,name,culture_id,owner_id,control_type) VALUES($1,$2,'Fallback','achaean',$3,'colony') RETURNING id`, f.worldID, pid, f.ownerID).Scan(&sid); err != nil {
					t.Fatal(err)
				}
			}
			var before, after string
			if err := pool.QueryRow(ctx, `SELECT row_to_json(u)::text FROM units u WHERE id=$1`, f.unitID).Scan(&before); err != nil {
				t.Fatal(err)
			}
			applied, err := ExecuteRecall(ctx, pool, f.scheduler, f.eventStore, f.clk, RecallOrder{WorldID: f.worldID, UnitID: f.unitID, Mode: "recall"})
			if !fallback {
				if err == nil || applied != nil {
					t.Fatalf("without home recall=%+v err=%v", applied, err)
				}
				if err := pool.QueryRow(ctx, `SELECT row_to_json(u)::text FROM units u WHERE id=$1`, f.unitID).Scan(&after); err != nil {
					t.Fatal(err)
				}
				if before != after {
					t.Fatal("refused recall changed the course")
				}
				var rows int
				if err := pool.QueryRow(ctx, `SELECT count(*) FROM unit_expeditions WHERE unit_id=$1`, f.unitID).Scan(&rows); err != nil {
					t.Fatal(err)
				}
				if rows != 1 {
					t.Fatal("refused recall cancelled expedition")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if applied == nil || applied.NewTargetQ != -1 || applied.NewTargetR != 0 {
				t.Fatalf("wrong fallback: %+v", applied)
			}
			if err := f.h.Handle(ctx, f.nextArrival(t)); err != nil {
				t.Fatal(err)
			}
			var actual *uuid.UUID
			var status string
			if err := pool.QueryRow(ctx, `SELECT status,settlement_id FROM units WHERE id=$1`, f.unitID).Scan(&status, &actual); err != nil {
				t.Fatal(err)
			}
			if status != "garrison" || actual == nil || *actual != sid {
				t.Fatalf("fallback status=%s settlement=%v", status, actual)
			}
		})
	}
}

// Knowledge lives in the player's persistent map, independently of the
// expedition-only sight list used to format the homecoming notification.
func TestRecallExpeditionPreservesMapKnowledge(t *testing.T) {
	copper := [2]int{1, 2}
	f := newExpeditionFixture(t, -2, 16, &copper)
	ctx, pool := context.Background(), testPool(t)
	f.start(t, 9, 0, 20)
	if err := f.h.Handle(ctx, f.nextArrival(t)); err != nil {
		t.Fatal(err)
	}
	rows, err := pool.Query(ctx, `SELECT q,r FROM player_scouted_tiles WHERE world_id=$1 AND player_id=$2 ORDER BY q,r`, f.worldID, f.ownerID)
	if err != nil {
		t.Fatal(err)
	}
	var knownQ, knownR []int
	for rows.Next() {
		var q, r int
		if err := rows.Scan(&q, &r); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		knownQ = append(knownQ, q)
		knownR = append(knownR, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(knownQ) == 0 {
		t.Fatal("no pre-recall knowledge to test")
	}
	var seenBefore int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM unit_expedition_seen WHERE unit_id=$1`, f.unitID).Scan(&seenBefore); err != nil {
		t.Fatal(err)
	}
	if seenBefore == 0 {
		t.Fatal("no mission sight list to cancel")
	}
	assertKnowledge := func(stage string) {
		t.Helper()
		var missing int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM unnest($3::int[],$4::int[]) AS old(q,r) WHERE NOT EXISTS(SELECT 1 FROM player_scouted_tiles p WHERE p.world_id=$1 AND p.player_id=$2 AND p.q=old.q AND p.r=old.r)`, f.worldID, f.ownerID, knownQ, knownR).Scan(&missing); err != nil {
			t.Fatal(err)
		}
		if missing != 0 {
			t.Fatalf("%s lost %d known tiles", stage, missing)
		}
		var copperKnown bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM player_scouted_tiles p JOIN map_tiles t ON t.world_id=p.world_id AND t.q=p.q AND t.r=p.r WHERE p.world_id=$1 AND p.player_id=$2 AND p.q=1 AND p.r=2 AND t.copper_deposit)`, f.worldID, f.ownerID).Scan(&copperKnown); err != nil {
			t.Fatal(err)
		}
		if !copperKnown {
			t.Fatalf("%s lost known copper deposit", stage)
		}
	}
	assertKnowledge("before recall")
	if _, err := ExecuteRecall(ctx, pool, f.scheduler, f.eventStore, f.clk, RecallOrder{WorldID: f.worldID, UnitID: f.unitID, Mode: "recall"}); err != nil {
		t.Fatal(err)
	}
	assertKnowledge("after delivery")
	if err := f.h.Handle(ctx, f.nextArrival(t)); err != nil {
		t.Fatal(err)
	}
	assertKnowledge("after homecoming")
	var seenAfter, reports int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM unit_expedition_seen WHERE unit_id=$1`, f.unitID).Scan(&seenAfter); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE player_id=$1 AND kind='ExpeditionReport'`, f.ownerID).Scan(&reports); err != nil {
		t.Fatal(err)
	}
	if seenAfter != 0 || reports != 0 {
		t.Fatalf("cancelled mission sight=%d reports=%d", seenAfter, reports)
	}
	t.Logf("persistent known tiles=%d, mission sight before=%d after=%d, missing after recall/home=0, copper still known, report notifications=%d", len(knownQ), seenBefore, seenAfter, reports)
}
