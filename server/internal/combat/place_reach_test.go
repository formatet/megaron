package combat

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/province"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type reachFixture struct {
	pool         *pgxpool.Pool
	world, owner uuid.UUID
	h            *PlaceReachScanHandler
	clk          *clock.TestClock
}

func reachWorld(t *testing.T) reachFixture {
	t.Helper()
	pool := testPool(t)
	w := sweepTestWorld(t)
	ctx := context.Background()
	var owner uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO players(username,password_hash) VALUES($1,'x') RETURNING id`, "reach-"+uuid.New().String()).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	for q := 0; q <= 8; q++ {
		terrain := "plains"
		if q == 2 || q == 6 {
			terrain = "river_ford"
		}
		if _, err := pool.Exec(ctx, `INSERT INTO map_tiles(world_id,q,r,terrain) VALUES($1,$2,0,$3)`, w, q, terrain); err != nil {
			t.Fatal(err)
		}
	}
	clk := clock.NewTestClock(time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC))
	sched := events.NewScheduler(pool, clk)
	return reachFixture{pool, w, owner, NewPlaceReachScanHandler(pool, sched, events.NewStore(pool), nil), clk}
}
func reachRoute(start, end, from, to int) StoredRoute {
	r := StoredRoute{StartTick: start, EndTick: end}
	dir := 1
	if to < from {
		dir = -1
	}
	for q := from; ; q += dir {
		r.Hexes = append(r.Hexes, [2]int{q, 0})
		if q == to {
			break
		}
		r.Costs = append(r.Costs, 1000)
	}
	return r
}
func (f reachFixture) mover(t *testing.T, typ string, owner uuid.UUID, r StoredRoute) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	raw, _ := json.Marshal(r)
	var id uuid.UUID
	cat := "land"
	if typ == "galley" {
		cat = "naval"
	}
	if err := f.pool.QueryRow(ctx, `INSERT INTO units(world_id,owner_id,type,category,size,crew,status,q,r,target_q,target_r,depart_tick,arrive_tick,march_route,departs_at,arrives_at,name) VALUES($1,$2,$3,$4,100,20,'marching',$5,0,$6,0,$7,$8,$9,$10,$11,'Ariadne') RETURNING id`, f.world, owner, typ, cat, r.Hexes[0][0], r.Hexes[len(r.Hexes)-1][0], r.StartTick, r.EndTick, raw, f.clk.Now(), f.clk.Now().Add(time.Hour)).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}
func (f reachFixture) scan(t *testing.T, tick int) {
	t.Helper()
	if err := f.h.Handle(context.Background(), events.ScheduledEvent{WorldID: f.world, DueTick: tick}); err != nil {
		t.Fatal(err)
	}
}
func (f reachFixture) claims(t *testing.T) int {
	t.Helper()
	var n int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM place_names WHERE world_id=$1 AND claimed_by IS NOT NULL`, f.world).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
func TestPlaceReachMovementHookFirstOwnerAndRerun(t *testing.T) {
	f := reachWorld(t)
	ctx := context.Background()
	var other uuid.UUID
	if err := f.pool.QueryRow(ctx, `INSERT INTO players(username,password_hash) VALUES($1,'x') RETURNING id`, "later-"+uuid.New().String()).Scan(&other); err != nil {
		t.Fatal(err)
	}
	// Insert later mover first: physical entry time, not row insertion, wins.
	f.mover(t, "infantry", other, reachRoute(1, 7, 0, 6))
	f.mover(t, "nomadic_host", f.owner, reachRoute(0, 6, 0, 6))
	f.scan(t, 3)
	if n := f.claims(t); n != 1 {
		t.Fatalf("physical movement reach hook missing: claims=%d want1", n)
	}
	var got uuid.UUID
	if err := f.pool.QueryRow(ctx, `SELECT claimed_by FROM place_names WHERE world_id=$1 AND anchor_q=2`, f.world).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != f.owner {
		t.Fatal("earliest physical entry did not retain first owner")
	}
	f.scan(t, 4)
	f.scan(t, 4)
	if err := f.pool.QueryRow(ctx, `SELECT claimed_by FROM place_names WHERE world_id=$1 AND anchor_q=2`, f.world).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != f.owner {
		t.Fatal("claimed_by IS NULL guard missing: later actor overwrote naming right")
	}
	var notices int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE world_id=$1 AND kind='PlaceFirstReached'`, f.world).Scan(&notices); err != nil {
		t.Fatal(err)
	}
	if notices != 1 {
		t.Fatalf("rerun emitted %d notices want1", notices)
	}
}
func TestPlaceReachArrivalIncludesLastHexBeforeHandler(t *testing.T) {
	f := reachWorld(t)
	id := f.mover(t, "infantry", f.owner, reachRoute(0, 2, 4, 6))
	f.scan(t, 0)
	f.scan(t, 2)
	if f.claims(t) != 1 {
		t.Fatal("arrival between scans lost final reached hex")
	}
	if events.TickPriority(events.ScheduledPlaceReachScan) >= events.TickPriority(events.ScheduledUnitArrival) {
		t.Fatal("reach scan must precede arrival route removal")
	}
	if _, err := f.pool.Exec(context.Background(), `UPDATE units SET status='positioned',march_route=NULL WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	f.scan(t, 3)
	if f.claims(t) != 1 {
		t.Fatal("completed movement changed first right")
	}
}
func TestPlaceReachDestroyedNeverReachesFutureHex(t *testing.T) {
	f := reachWorld(t)
	id := f.mover(t, "galley", f.owner, reachRoute(0, 8, 0, 8))
	f.scan(t, 0)
	if _, err := f.pool.Exec(context.Background(), `UPDATE units SET status='disbanded',size=0 WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	f.scan(t, 8)
	if f.claims(t) != 0 {
		t.Fatal("destroyed mover received post-destruction naming rights")
	}
}
func TestPlaceReachNoBackfillButNewDepartureCounts(t *testing.T) {
	f := reachWorld(t)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `INSERT INTO place_reach_epoch(world_id,first_tick,applied_at) VALUES($1,5,$2)`, f.world, f.clk.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	f.mover(t, "infantry", f.owner, reachRoute(0, 8, 0, 8))
	f.scan(t, 4)
	if f.claims(t) != 0 {
		t.Fatal("pre-deploy reached hex backfilled")
	}
	f.scan(t, 6)
	if f.claims(t) != 1 {
		t.Fatal("old journey future entry was lost")
	}
	// New route after deploy: immediate first step in the same tick counts.
	f.clk.Advance(2 * time.Minute)
	f.mover(t, "nomadic_host", f.owner, reachRoute(4, 5, 1, 2))
	f.scan(t, 4)
	if f.claims(t) != 2 {
		t.Fatal("new post-deploy departure first entry missing")
	}
}
func TestPlaceReachCaravanSavedJourney(t *testing.T) {
	f := reachWorld(t)
	ctx := context.Background()
	r := reachRoute(0, 6, 0, 6)
	var path []province.MapPosition
	for _, h := range r.Hexes {
		path = append(path, province.MapPosition{Q: h[0], R: h[1]})
	}
	j := province.TradeJourney{Category: "land", Path: path, StepCosts: r.Costs, TravelTicks: 9, Distance: 6}
	raw, _ := json.Marshal(j)
	if _, err := f.pool.Exec(ctx, `INSERT INTO transports(world_id,owner_id,kind,category,origin_q,origin_r,dest_q,dest_r,departs_at,arrives_at,due_tick,departed_tick,journey) VALUES($1,$2,'internal_transfer','land',0,0,6,0,$3,$4,9,0,$5)`, f.world, f.owner, f.clk.Now(), f.clk.Now().Add(time.Hour), raw); err != nil {
		t.Fatal(err)
	}
	f.scan(t, 3)
	if f.claims(t) != 1 {
		t.Fatal("caravan saved journey did not claim entered place")
	}
}
func TestPlaceReachAdminSpentObjectDoesNotPromiseNaming(t *testing.T) {
	f := reachWorld(t)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `INSERT INTO place_names(world_id,kind,anchor_q,anchor_r,named_tick) VALUES($1,'river',2,0,0)`, f.world); err != nil {
		t.Fatal(err)
	}
	f.mover(t, "infantry", f.owner, reachRoute(0, 6, 0, 6))
	f.scan(t, 3)
	var n int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE world_id=$1 AND kind='PlaceFirstReached'`, f.world).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal("admin-cleared object promised already-spent naming right")
	}
}

func TestPlaceReachRecallRedirectFlushOnlyEnteredOldRoute(t *testing.T) {
	for _, mode := range []string{"recall", "redirect"} {
		t.Run(mode, func(t *testing.T) {
			f := reachWorld(t)
			ctx := context.Background()
			id := f.mover(t, "infantry", f.owner, reachRoute(0, 8, 0, 8))
			if _, err := f.pool.Exec(ctx, `UPDATE worlds SET current_tick=2,last_tick_at=$2 WHERE id=$1`, f.world, f.clk.Now()); err != nil {
				t.Fatal(err)
			}
			q, r := 0, 0
			order := RecallOrder{WorldID: f.world, UnitID: id, Mode: mode}
			if mode == "redirect" {
				order.NewTargetQ = &q
				order.NewTargetR = &r
			}
			got, err := ExecuteRecall(ctx, f.pool, f.h.scheduler, f.h.store, f.clk, order)
			if err != nil {
				t.Fatal(err)
			}
			if got == nil {
				t.Fatal("recall missed")
			}
			if f.claims(t) != 1 {
				t.Fatal("recall/redirect lost already-entered old-route place")
			}
			f.scan(t, 20)
			var n int
			if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM place_names WHERE world_id=$1 AND anchor_q=6 AND claimed_by IS NOT NULL`, f.world).Scan(&n); err != nil {
				t.Fatal(err)
			}
			if n != 0 {
				t.Fatal("recall/redirect claimed future old-route hex")
			}
		})
	}
}
func TestPlaceReachRollbackKeepsClaimNoticeProgressAtomic(t *testing.T) {
	f := reachWorld(t)
	ctx := context.Background()
	f.mover(t, "nomadic_host", f.owner, reachRoute(0, 6, 0, 6))
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := placeReachLock(ctx, tx, f.world); err != nil {
		t.Fatal(err)
	}
	if _, err := recordPlaceEntriesTx(ctx, tx, f.h.store, f.world, 3000); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"place_names", "place_reach_progress", "notifications"} {
		var n int
		if err := f.pool.QueryRow(ctx, "SELECT count(*) FROM "+table+" WHERE world_id=$1", f.world).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Fatalf("rollback leaked %s rows", table)
		}
	}
	f.scan(t, 3)
	if f.claims(t) != 1 {
		t.Fatal("rollback consumed first-reach naming right")
	}
}

func TestPlaceReachRiverFordDeltaShareOneRight(t *testing.T) {
	f := reachWorld(t)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `UPDATE map_tiles SET terrain=CASE q WHEN 1 THEN 'river' WHEN 2 THEN 'river_ford' WHEN 3 THEN 'river_delta' ELSE 'plains' END WHERE world_id=$1`, f.world); err != nil {
		t.Fatal(err)
	}
	var other uuid.UUID
	if err := f.pool.QueryRow(ctx, `INSERT INTO players(username,password_hash) VALUES($1,'x') RETURNING id`, "chain-"+uuid.New().String()).Scan(&other); err != nil {
		t.Fatal(err)
	}
	f.mover(t, "galley", f.owner, reachRoute(0, 4, 0, 4))
	f.scan(t, 0)
	f.mover(t, "infantry", other, reachRoute(1, 4, 1, 4))
	f.scan(t, 3)
	f.scan(t, 3)
	var owner uuid.UUID
	var rows, notices int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM place_names WHERE world_id=$1`, f.world).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `SELECT claimed_by FROM place_names WHERE world_id=$1 AND kind='river' AND anchor_q=1`, f.world).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE world_id=$1 AND kind='PlaceFirstReached'`, f.world).Scan(&notices); err != nil {
		t.Fatal(err)
	}
	if rows != 1 || notices != 1 || owner != f.owner {
		t.Fatalf("river/ford/delta must retain one first right: rows=%d notices=%d owner=%s", rows, notices, owner)
	}
}
func TestPlaceReachGenericKindsFromPhysicalSavedRoute(t *testing.T) {
	for _, kind := range []string{"river", "desert", "forest"} {
		t.Run(kind, func(t *testing.T) {
			f := reachWorld(t)
			ctx := context.Background()
			terrain := map[string]string{"river": "river", "desert": "semi_desert", "forest": "forest_cedar"}[kind]
			if _, err := f.pool.Exec(ctx, `UPDATE map_tiles SET terrain=CASE WHEN q BETWEEN 1 AND 5 THEN $2 ELSE 'plains' END WHERE world_id=$1`, f.world, terrain); err != nil {
				t.Fatal(err)
			}
			f.mover(t, "nomadic_host", f.owner, reachRoute(0, 5, 0, 5))
			f.scan(t, 2)
			var owner uuid.UUID
			if err := f.pool.QueryRow(ctx, `SELECT claimed_by FROM place_names WHERE world_id=$1 AND kind=$2 AND anchor_q=1`, f.world, kind).Scan(&owner); err != nil {
				t.Fatalf("physical entry did not claim %s: %v", kind, err)
			}
			if owner != f.owner {
				t.Fatal("wrong generic first owner")
			}
			var visited int
			if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM player_scouted_tiles WHERE world_id=$1 AND player_id=$2 AND q=1 AND r=0`, f.world, f.owner).Scan(&visited); err != nil {
				t.Fatal(err)
			}
			if visited != 1 {
				t.Fatal("physical first reach failed to retain visited-hex memory")
			}
			var unseen int
			if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM player_scouted_tiles WHERE world_id=$1 AND player_id=$2 AND q=5 AND r=0`, f.world, f.owner).Scan(&unseen); err != nil {
				t.Fatal(err)
			}
			if unseen != 0 {
				t.Fatal("first reach revealed unentered object component")
			}
		})
	}
}

func TestPlaceReachNoticeUsesCanonicalUnitName(t *testing.T) {
	f := reachWorld(t)
	ctx := context.Background()
	id := f.mover(t, "nomadic_host", f.owner, reachRoute(0, 6, 0, 6))
	if _, err := f.pool.Exec(ctx, `UPDATE units SET name=NULL WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	f.scan(t, 3)
	var name string
	if err := f.pool.QueryRow(ctx, `SELECT body_json->>'unit_name' FROM notifications WHERE world_id=$1 AND kind='PlaceFirstReached'`, f.world).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(name, "Nomadic Host of ") {
		t.Fatalf("first reach notice lacks recognizable subject: %q", name)
	}
}
