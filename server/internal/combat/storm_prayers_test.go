package combat

// Stormbönerna 3b: Still the Storm and Wake the Storm (combat side).

import (
	"context"
	"testing"

	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/hexgrid"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// stormAt stores a storm with its track at `tick` and sets the weather clock to that tick.
func stormAt(t *testing.T, pool *pgxpool.Pool, worldID uuid.UUID, tick int, shape StormShape, dies int) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	id := newTestStorm(t, pool, worldID, dies)
	tx, _ := pool.Begin(ctx)
	if err := writeStormTrack(ctx, tx, id, tick, shape); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE worlds SET storm_weather_tick = $2 WHERE id = $1`, worldID, tick); err != nil {
		t.Fatal(err)
	}
	return id
}

func calmIt(t *testing.T, pool *pgxpool.Pool, worldID, stormID uuid.UUID, tick int, key uuid.UUID, dice economy.Dice) (*CalmResult, error) {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	res, err := CalmStorm(ctx, tx, worldID, stormID, tick, key, dice)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return res, nil
}

func trackAt(t *testing.T, pool *pgxpool.Pool, id uuid.UUID, tick int) StormShape {
	t.Helper()
	rows, err := pool.Query(context.Background(), `SELECT q, r FROM sea_storm_track WHERE storm_id = $1 AND tick = $2 ORDER BY slot`, id, tick)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var s StormShape
	for rows.Next() {
		var c hexgrid.Coord
		_ = rows.Scan(&c.Q, &c.R)
		s = append(s, c)
	}
	return s
}

var line3 = StormShape{{Q: 2, R: 3}, {Q: 3, R: 3}, {Q: 4, R: 3}}

// Three in a row: only an end can go, whatever the die says; the rest stays connected.
func TestCalmStorm_ALineLosesAnEndNeverTheMiddle(t *testing.T) {
	for v := 0; v < 3; v++ {
		pool, worldID := seaWorld(t, 8)
		id := stormAt(t, pool, worldID, 1, line3, 1000)
		res, err := calmIt(t, pool, worldID, id, 1, uuid.New(), fixedDice{v: v})
		if err != nil {
			t.Fatal(err)
		}
		if res.Removed == (hexgrid.Coord{Q: 3, R: 3}) {
			t.Fatalf("die %d took the middle of a line", v)
		}
		got := trackAt(t, pool, id, 1)
		if len(got) != 2 || !got.Connected() || got.has(res.Removed) {
			t.Fatalf("die %d: shape after = %v (removed %v)", v, got, res.Removed)
		}
	}
}

// 3 hexes -> 2 on the next scan tick; the hex grows back after 24 ticks and not before; the
// regrowth is stamped.
func TestCalmStorm_NextTickHasOneHexLessAndTheHexGrowsBackAfter24(t *testing.T) {
	pool, worldID := seaWorld(t, 8)
	ctx := context.Background()
	id := stormAt(t, pool, worldID, 1, line3, 1000)
	res, err := calmIt(t, pool, worldID, id, 1, uuid.New(), newStormDice(1))
	if err != nil {
		t.Fatal(err)
	}
	if res.RegrowTick != 1+StormRegrowTicks || res.Remaining != 2 {
		t.Fatalf("result %+v", res)
	}
	dice := newStormDice(2)
	scanTo(t, pool, worldID, 2, dice)
	if n := len(trackAt(t, pool, id, 2)); n != 2 {
		t.Fatalf("next tick has %d hexes, want 2", n)
	}
	scanTo(t, pool, worldID, 1+StormRegrowTicks-1, dice)
	if n := len(trackAt(t, pool, id, StormRegrowTicks)); n != 2 {
		t.Fatalf("tick %d has %d hexes, want 2 (not grown back yet)", StormRegrowTicks, n)
	}
	scanTo(t, pool, worldID, 1+StormRegrowTicks, dice)
	got := trackAt(t, pool, id, 1+StormRegrowTicks)
	if len(got) != 3 || !got.Connected() {
		t.Fatalf("after 24 ticks: %v, want 3 connected hexes", got)
	}
	var regrown *int
	_ = pool.QueryRow(ctx, `SELECT regrown_tick FROM sea_storm_calms WHERE storm_id = $1`, id).Scan(&regrown)
	if regrown == nil || *regrown != 1+StormRegrowTicks {
		t.Fatalf("regrown_tick = %v, want %d", regrown, 1+StormRegrowTicks)
	}
	scanTo(t, pool, worldID, 80, dice)
	if n := len(trackAt(t, pool, id, 80)); n != 3 {
		t.Fatalf("a storm that grew back must not keep growing: %d hexes", n)
	}
}

// Three prayers inside 24 ticks (3 -> 2 -> 1 -> 0) kill the storm; it writes no more track.
func TestCalmStorm_ThreePrayersWithin24TicksKillTheStorm(t *testing.T) {
	pool, worldID := seaWorld(t, 8)
	ctx := context.Background()
	id := stormAt(t, pool, worldID, 1, line3, 1000)
	dice := newStormDice(5)
	for i := 0; i < 3; i++ {
		res, err := calmIt(t, pool, worldID, id, 1+i, uuid.New(), dice)
		if err != nil {
			t.Fatalf("prayer %d: %v", i+1, err)
		}
		if res.Died != (i == 2) {
			t.Fatalf("prayer %d: died = %v", i+1, res.Died)
		}
		if i < 2 {
			scanTo(t, pool, worldID, 2+i, dice)
		}
	}
	var died *int
	_ = pool.QueryRow(ctx, `SELECT died_tick FROM sea_storms WHERE id = $1`, id).Scan(&died)
	if died == nil {
		t.Fatal("died_tick not set after the third prayer")
	}
	var before int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM sea_storm_track WHERE storm_id = $1`, id).Scan(&before)
	scanTo(t, pool, worldID, 60, dice)
	var after int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM sea_storm_track WHERE storm_id = $1`, id).Scan(&after)
	if after != before {
		t.Fatalf("the dead storm wrote track: %d -> %d rows", before, after)
	}
	if _, err := calmIt(t, pool, worldID, id, 5, uuid.New(), dice); err != ErrStormGone {
		t.Fatalf("calming a dead storm: %v, want ErrStormGone", err)
	}
}

func TestCalmStorm_APrayerOnAOneHexStormKillsIt(t *testing.T) {
	pool, worldID := seaWorld(t, 8)
	id := stormAt(t, pool, worldID, 1, StormShape{{Q: 3, R: 3}}, 1000)
	res, err := calmIt(t, pool, worldID, id, 1, uuid.New(), newStormDice(1))
	if err != nil || !res.Died {
		t.Fatalf("res %+v err %v", res, err)
	}
	var died *int
	_ = pool.QueryRow(context.Background(), `SELECT died_tick FROM sea_storms WHERE id = $1`, id).Scan(&died)
	if died == nil {
		t.Fatal("died_tick not set")
	}
}

// The same cast key twice is one effect.
func TestCalmStorm_TheSameCastKeyTwiceIsOneEffect(t *testing.T) {
	pool, worldID := seaWorld(t, 8)
	id := stormAt(t, pool, worldID, 1, line3, 1000)
	key := uuid.New()
	a, err := calmIt(t, pool, worldID, id, 1, key, newStormDice(1))
	if err != nil {
		t.Fatal(err)
	}
	b, err := calmIt(t, pool, worldID, id, 1, key, newStormDice(9))
	if err != nil {
		t.Fatal(err)
	}
	if a.Removed != b.Removed {
		t.Fatalf("second cast removed %v, first %v", b.Removed, a.Removed)
	}
	if n := len(trackAt(t, pool, id, 1)); n != 2 {
		t.Fatalf("%d hexes after the same cast twice, want 2", n)
	}
	var calms int
	_ = pool.QueryRow(context.Background(), `SELECT count(*) FROM sea_storm_calms WHERE storm_id = $1`, id).Scan(&calms)
	if calms != 1 {
		t.Fatalf("%d calm rows, want 1", calms)
	}
}

// A late scan equals daily scans with a shrink and a regrowth in the interval.
func TestStormRegrow_LateScanEqualsDailyScansWithAShrinkAndARegrowth(t *testing.T) {
	setup := func() (*pgxpool.Pool, uuid.UUID, uuid.UUID, *stormDice) {
		pool, worldID := seaWorld(t, 8)
		id := stormAt(t, pool, worldID, 1, line3, 1000)
		dice := newStormDice(33)
		scanTo(t, pool, worldID, 3, dice)
		if _, err := calmIt(t, pool, worldID, id, 3, uuid.New(), dice); err != nil {
			t.Fatal(err)
		}
		return pool, worldID, id, dice
	}
	read := func(pool *pgxpool.Pool, id uuid.UUID) map[[4]int]bool {
		out := map[[4]int]bool{}
		rows, err := pool.Query(context.Background(), `SELECT tick, slot, q, r FROM sea_storm_track WHERE storm_id = $1`, id)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		for rows.Next() {
			var k [4]int
			_ = rows.Scan(&k[0], &k[1], &k[2], &k[3])
			out[k] = true
		}
		return out
	}
	poolA, worldA, idA, diceA := setup()
	for tick := 4; tick <= 40; tick++ {
		scanTo(t, poolA, worldA, tick, diceA)
	}
	poolB, worldB, idB, diceB := setup()
	scanTo(t, poolB, worldB, 40, diceB)
	ta, tb := read(poolA, idA), read(poolB, idB)
	var regrown int
	_ = poolA.QueryRow(context.Background(), `SELECT count(*) FROM sea_storm_calms WHERE storm_id = $1 AND regrown_tick = 27`, idA).Scan(&regrown)
	if regrown != 1 || len(trackAt(t, poolA, idA, 26)) != 2 || len(trackAt(t, poolA, idA, 27)) != 3 {
		t.Fatal("the daily run must cover the shrink (3 -> 2) and the regrowth at tick 27")
	}
	if len(ta) != len(tb) {
		t.Fatalf("daily %d track hexes, late %d", len(ta), len(tb))
	}
	for k := range ta {
		if !tb[k] {
			t.Fatalf("late scan lacks daily track hex %v", k)
		}
	}
}

// Regrowth never lands on a hex another living storm stands on; with no room it waits.
func TestRegrowHex_SkipsOccupiedHexesAndWaitsWhenThereIsNoRoom(t *testing.T) {
	shape := StormShape{{Q: 0, R: 0}}
	occupied := map[hexgrid.Coord]bool{}
	for _, d := range stormDirs[1:] {
		occupied[hexgrid.Coord{Q: d.Q, R: d.R}] = true
	}
	for v := 0; v < 6; v++ {
		got, ok := regrowHex(shape, occupied, openSea, fixedDice{v: v})
		if !ok || got[1] != (hexgrid.Coord{Q: stormDirs[0].Q, R: stormDirs[0].R}) {
			t.Fatalf("die %d: %v %v, want the only free neighbour", v, got, ok)
		}
	}
	occupied[hexgrid.Coord{Q: stormDirs[0].Q, R: stormDirs[0].R}] = true
	if _, ok := regrowHex(shape, occupied, openSea, fixedDice{}); ok {
		t.Fatal("regrew with no free neighbour")
	}
}

func wakeIt(t *testing.T, pool *pgxpool.Pool, worldID uuid.UUID, hex hexgrid.Coord, tick int, key uuid.UUID, dice economy.Dice) (*WakeResult, error) {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	res, err := WakeStorm(ctx, tx, worldID, hex, tick, key, dice)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return res, nil
}

// A woken storm is an ordinary one: the target hex plus two connected sea neighbours, a life in
// range, no name, no claim.
func TestWakeStorm_IsBornOnTheTargetWithTwoConnectedNeighboursAndNoName(t *testing.T) {
	pool, worldID := seaWorld(t, 8)
	target := hexgrid.Coord{Q: 4, R: 4}
	res, err := wakeIt(t, pool, worldID, target, 10, uuid.New(), newStormDice(4))
	if err != nil {
		t.Fatal(err)
	}
	got := trackAt(t, pool, res.StormID, 10)
	if len(got) != 3 || !got.Connected() || !got.has(target) {
		t.Fatalf("shape %v", got)
	}
	var name, claimed *string
	var created, dies int
	var died *int
	if err := pool.QueryRow(context.Background(), `SELECT name, claimed_by::text, created_tick, dies_tick, died_tick FROM sea_storms WHERE id = $1`, res.StormID).
		Scan(&name, &claimed, &created, &dies, &died); err != nil {
		t.Fatal(err)
	}
	if name != nil || claimed != nil || died != nil || created != 10 {
		t.Fatalf("name %v claimed %v created %d died %v", name, claimed, created, died)
	}
	if life := dies - 10; life < StormLifeMinTicks || life > StormLifeMaxTicks {
		t.Fatalf("life %d outside the ordinary range", life)
	}
}

func TestWakeStorm_RefusesLandAndOccupiedWaterAndIsIdempotent(t *testing.T) {
	pool, worldID := seaWorld(t, 8)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `UPDATE map_tiles SET terrain = 'plains' WHERE world_id = $1 AND q = 0 AND r = 0`, worldID); err != nil {
		t.Fatal(err)
	}
	if _, err := wakeIt(t, pool, worldID, hexgrid.Coord{Q: 0, R: 0}, 10, uuid.New(), newStormDice(1)); err != ErrWakeNotSea {
		t.Fatalf("land: %v", err)
	}
	stormAt(t, pool, worldID, 10, line3, 1000)
	if _, err := wakeIt(t, pool, worldID, hexgrid.Coord{Q: 3, R: 3}, 10, uuid.New(), newStormDice(1)); err != ErrWakeOccupied {
		t.Fatalf("occupied: %v", err)
	}
	var before int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM sea_storms WHERE world_id = $1`, worldID).Scan(&before)
	key := uuid.New()
	a, err := wakeIt(t, pool, worldID, hexgrid.Coord{Q: 6, R: 6}, 10, key, newStormDice(2))
	if err != nil {
		t.Fatal(err)
	}
	b, err := wakeIt(t, pool, worldID, hexgrid.Coord{Q: 6, R: 6}, 10, key, newStormDice(3))
	if err != nil || b.StormID != a.StormID {
		t.Fatalf("same key: %v %v", b, err)
	}
	var after int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM sea_storms WHERE world_id = $1`, worldID).Scan(&after)
	if after != before+1 {
		t.Fatalf("storms %d -> %d, want exactly one new", before, after)
	}
	// Its neighbours never overlap the living storm either.
	for _, c := range trackAt(t, pool, a.StormID, 10) {
		if line3.has(c) {
			t.Fatalf("woken storm overlaps a living one at %v", c)
		}
	}
}
