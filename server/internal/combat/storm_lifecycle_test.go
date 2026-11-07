package combat

// Stormar föds och dör (megaron_plan_stormar.md §Stormar föds och dör, Timothy 2026-11-07).

import (
	"context"
	"testing"

	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/hexgrid"
	"formatet/megaron/server/internal/province"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// fixedDice returns the same Float64 and Intn(n) = min(v, n-1) every time.
type fixedDice struct {
	f float64
	v int
}

func (d fixedDice) Float64() float64 { return d.f }
func (d fixedDice) Intn(n int) int {
	if d.v >= n {
		return n - 1
	}
	return d.v
}

func TestStormLife_IsRolledWithinTheRange(t *testing.T) {
	if got := stormLife(fixedDice{v: 0}); got != StormLifeMinTicks {
		t.Fatalf("lowest roll = %d, want %d", got, StormLifeMinTicks)
	}
	if got := stormLife(fixedDice{v: 1 << 30}); got != StormLifeMaxTicks {
		t.Fatalf("highest roll = %d, want %d", got, StormLifeMaxTicks)
	}
	d := newStormDice(3)
	seen := map[int]bool{}
	for i := 0; i < 5000; i++ {
		l := stormLife(d)
		if l < StormLifeMinTicks || l > StormLifeMaxTicks {
			t.Fatalf("life %d outside [%d, %d]", l, StormLifeMinTicks, StormLifeMaxTicks)
		}
		seen[l] = true
	}
	if len(seen) < 90 {
		t.Fatalf("only %d distinct lives in 5000 rolls: the roll is not spread", len(seen))
	}
}

// chance = want / mean life (250): 600 wanted storms is 2.4 births a tick — two for sure,
// a third when the die lands under 0.4.
func TestStormBirthCount_IsWantOverMeanLifeWithRemainderAsProbability(t *testing.T) {
	if got := stormBirthCount(600, fixedDice{f: 0.39}); got != 3 {
		t.Fatalf("die 0.39: %d births, want 3", got)
	}
	if got := stormBirthCount(600, fixedDice{f: 0.41}); got != 2 {
		t.Fatalf("die 0.41: %d births, want 2", got)
	}
	if got := stormBirthCount(250, fixedDice{f: 0.99}); got != 1 {
		t.Fatalf("exactly one a tick: %d births, want 1", got)
	}
	if got := stormBirthCount(0, fixedDice{f: 0}); got != 0 {
		t.Fatalf("no sea, no births: %d", got)
	}
}

// Over many ticks the average number of living storms is `want` (within 20 %), whether the
// world starts with its full complement or with none.
func TestStormLifecycle_AverageStaysAtWant(t *testing.T) {
	for _, start := range []string{"full", "empty"} {
		for _, want := range []int{4, 10} {
			dice := newStormDice(uint64(want))
			var dies []int // dies_tick of every living storm
			if start == "full" {
				for i := 0; i < want; i++ {
					dies = append(dies, 1+dice.Intn(stormLife(dice)))
				}
			}
			const ticks, warm = 3000, 500
			sum, n := 0, 0
			for tick := 1; tick <= ticks; tick++ {
				live := dies[:0]
				for _, d := range dies {
					if d > tick {
						live = append(live, d)
					}
				}
				dies = live
				for b := stormBirthCount(want, dice); b > 0; b-- {
					dies = append(dies, tick+stormLife(dice))
				}
				if tick > warm {
					sum += len(dies)
					n++
				}
			}
			avg := float64(sum) / float64(n)
			if avg < float64(want)*0.8 || avg > float64(want)*1.2 {
				t.Fatalf("start %s, want %d: average %.2f living storms outside ±20%%", start, want, avg)
			}
		}
	}
}

func TestStepStorm_OneAndTwoHexStormsStayConnectedAndOnSea(t *testing.T) {
	sea := func(c hexgrid.Coord) bool {
		return c.R >= -4 && c.R <= 4 && c.Q >= -6 && c.Q <= 6 && c.Q+c.R >= -6 && c.Q+c.R <= 6
	}
	for _, start := range []StormShape{
		{{Q: 0, R: 0}},
		{{Q: 0, R: 0}, {Q: 1, R: 0}},
	} {
		for seed := uint64(1); seed <= 40; seed++ {
			dice := newStormDice(seed)
			s, heading := start, int(seed%6)
			for step := 0; step < 300; step++ {
				next, h, slot := StepStorm(s, heading, sea, dice)
				if slot < 0 {
					t.Fatalf("size %d seed %d step %d: open sea has a legal step", len(start), seed, step)
				}
				if len(next) != len(start) || !next.Connected() {
					t.Fatalf("size %d seed %d step %d: %v is not %d connected hexes", len(start), seed, step, next, len(start))
				}
				if moved(s, next) != 1 || hexgrid.Distance(s[slot], next[slot]) != 1 {
					t.Fatalf("size %d: %v -> %v is not one hex stepping once", len(start), s, next)
				}
				for _, c := range next {
					if !sea(c) {
						t.Fatalf("size %d seed %d step %d: hex %v on land", len(start), seed, step, c)
					}
				}
				s, heading = next, h
			}
		}
	}
}

// A one-hex storm in a one-hex-wide sea channel still moves; the stepping input is not mutated.
func TestStepStorm_DoesNotMutateItsInput(t *testing.T) {
	s := StormShape{{Q: 0, R: 0}, {Q: 1, R: 0}}
	keep := s.clone()
	for seed := uint64(1); seed <= 30; seed++ {
		StepStorm(s, 0, openSea, newStormDice(seed))
		if !sameShape(s, keep) {
			t.Fatalf("seed %d: StepStorm changed its input %v -> %v", seed, keep, s)
		}
	}
}

func TestStormShape_ConnectedForEverySize(t *testing.T) {
	cases := []struct {
		s    StormShape
		want bool
	}{
		{StormShape{{Q: 0, R: 0}}, true},
		{StormShape{{Q: 0, R: 0}, {Q: 1, R: 0}}, true},
		{StormShape{{Q: 0, R: 0}, {Q: 2, R: 0}}, false},
		{StormShape{{Q: 0, R: 0}, {Q: 1, R: 0}, {Q: 2, R: 0}}, true},
		{StormShape{{Q: 0, R: 0}, {Q: 1, R: 0}, {Q: 5, R: 0}}, false},
	}
	for _, c := range cases {
		if got := c.s.Connected(); got != c.want {
			t.Errorf("%v.Connected() = %v, want %v", c.s, got, c.want)
		}
	}
}

func TestLatestStormShape_ReadsOneToThreeHexesAndRejectsTheRest(t *testing.T) {
	pool, worldID := seaWorld(t, 4)
	ctx := context.Background()
	for _, n := range []int{1, 2, 3} {
		id := newTestStorm(t, pool, worldID, 100000)
		shape := StormShape{{Q: 0, R: 0}, {Q: 1, R: 0}, {Q: 0, R: 1}}[:n]
		tx, _ := pool.Begin(ctx)
		if err := writeStormTrack(ctx, tx, id, 5, shape); err != nil {
			t.Fatal(err)
		}
		got, tick, err := latestStormShape(ctx, tx, id)
		_ = tx.Commit(ctx)
		if err != nil || tick != 5 || !sameShape(got, shape) {
			t.Fatalf("size %d: got %v tick %d err %v", n, got, tick, err)
		}
	}
	// four hexes, and a hole in the slots, are both broken tracks
	bad := newTestStorm(t, pool, worldID, 100000)
	for slot := 0; slot < 4; slot++ {
		_, _ = pool.Exec(ctx, `INSERT INTO sea_storm_track VALUES ($1, 5, $2, $2, 0)`, bad, slot)
	}
	if _, _, err := latestStormShape(ctx, pool, bad); err == nil {
		t.Fatal("a four-hex track must be refused")
	}
	hole := newTestStorm(t, pool, worldID, 100000)
	_, _ = pool.Exec(ctx, `INSERT INTO sea_storm_track VALUES ($1, 5, 0, 0, 0), ($1, 5, 2, 2, 0)`, hole)
	if _, _, err := latestStormShape(ctx, pool, hole); err == nil {
		t.Fatal("a track with a missing slot must be refused")
	}
}

func newTestStorm(t *testing.T, pool *pgxpool.Pool, worldID uuid.UUID, dies int) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO sea_storms (world_id, heading, created_tick, dies_tick) VALUES ($1, 0, 0, $2) RETURNING id`, worldID, dies).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func loadGraph(t *testing.T, pool *pgxpool.Pool, worldID uuid.UUID) province.TileGraph {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	g, err := province.LoadTileGraph(ctx, tx, worldID)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func scanTo(t *testing.T, pool *pgxpool.Pool, worldID uuid.UUID, tick int, dice interface {
	Float64() float64
	Intn(int) int
}) {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if err := advanceStorms(ctx, tx, worldID, tick, loadGraph(t, pool, worldID), dice); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

// A storm with dies_tick = T has track through T-1 and none from T; the scan stamps died_tick;
// the row, its name and the track written before death stay.
func TestStormLifecycle_DeadStormWritesNoMoreTrackAndKeepsItsRow(t *testing.T) {
	pool, worldID := seaWorld(t, 6) // 36 sea hexes: no births (want 0)
	ctx := context.Background()
	id := newTestStorm(t, pool, worldID, 5)
	tx, _ := pool.Begin(ctx)
	_ = writeStormTrack(ctx, tx, id, 1, StormShape{{Q: 1, R: 1}, {Q: 2, R: 1}, {Q: 1, R: 2}})
	_ = tx.Commit(ctx)
	_, _ = pool.Exec(ctx, `UPDATE sea_storms SET name = 'Old Grey', claimed_by = NULL WHERE id = $1`, id)
	_, _ = pool.Exec(ctx, `UPDATE worlds SET storm_weather_tick = 1 WHERE id = $1`, worldID)

	scanTo(t, pool, worldID, 4, newStormDice(1))
	var died *int
	_ = pool.QueryRow(ctx, `SELECT died_tick FROM sea_storms WHERE id = $1`, id).Scan(&died)
	if died != nil {
		t.Fatalf("died_tick = %d at tick 4, storm dies at 5", *died)
	}
	scanTo(t, pool, worldID, 9, newStormDice(2))
	var maxTick, rows int
	_ = pool.QueryRow(ctx, `SELECT max(tick), count(DISTINCT tick) FROM sea_storm_track WHERE storm_id = $1`, id).Scan(&maxTick, &rows)
	if maxTick != 4 || rows != 4 {
		t.Fatalf("track up to tick %d over %d ticks, want through 4 (T-1) over 4 ticks", maxTick, rows)
	}
	var name string
	if err := pool.QueryRow(ctx, `SELECT name, died_tick FROM sea_storms WHERE id = $1`, id).Scan(&name, &died); err != nil {
		t.Fatalf("the dead storm's row is gone: %v", err)
	}
	if died == nil || *died != 5 || name != "Old Grey" {
		t.Fatalf("died_tick %v name %q, want 5 and the name kept", died, name)
	}
	scanTo(t, pool, worldID, 12, newStormDice(3)) // a dead storm is not walked again
	_ = pool.QueryRow(ctx, `SELECT max(tick) FROM sea_storm_track WHERE storm_id = $1`, id).Scan(&maxTick)
	if maxTick != 4 {
		t.Fatalf("a later scan wrote track for the dead storm: max tick %d", maxTick)
	}
}

// New world: `want` storms with spread dies_tick; a world whose storms have all died is not
// reseeded in a lump.
func TestStormLifecycle_NewWorldSeedsSpreadAgesAndNeverReseeds(t *testing.T) {
	pool, worldID := seaWorld(t, 30) // 900 hexes → 9 storms
	ctx := context.Background()
	graph := loadGraph(t, pool, worldID)
	dice := newStormDice(11)
	tx, _ := pool.Begin(ctx)
	if err := ensureStorms(ctx, tx, worldID, 40, graph, dice); err != nil {
		t.Fatal(err)
	}
	_ = tx.Commit(ctx)
	rows, err := pool.Query(ctx, `SELECT dies_tick FROM sea_storms WHERE world_id = $1`, worldID)
	if err != nil {
		t.Fatal(err)
	}
	distinct := map[int]bool{}
	n := 0
	for rows.Next() {
		var d int
		_ = rows.Scan(&d)
		n++
		distinct[d] = true
		if d <= 40 || d > 40+StormLifeMaxTicks {
			t.Fatalf("dies_tick %d outside (40, %d]", d, 40+StormLifeMaxTicks)
		}
	}
	rows.Close()
	if n != 900/SeaHexesPerStorm || len(distinct) < n-1 {
		t.Fatalf("%d storms with %d distinct dies_tick, want %d storms with spread ages", n, len(distinct), 900/SeaHexesPerStorm)
	}
	// every storm dies; ensureStorms must not seed the world again
	_, _ = pool.Exec(ctx, `UPDATE sea_storms SET dies_tick = 41, died_tick = 41 WHERE world_id = $1`, worldID)
	tx, _ = pool.Begin(ctx)
	if err := ensureStorms(ctx, tx, worldID, 500, graph, dice); err != nil {
		t.Fatal(err)
	}
	_ = tx.Commit(ctx)
	var after int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM sea_storms WHERE world_id = $1`, worldID).Scan(&after)
	if after != n {
		t.Fatalf("storms after all died: %d, was %d — the world was reseeded", after, n)
	}
}

// Births go through the real scan: a certain-birth world gets new living storms on free sea,
// each with a stored life of 200-300 ticks, and a duplicate scan of the same tick rolls nothing.
func TestStormLifecycle_ScanBirthsNewStormsWithStoredLife(t *testing.T) {
	pool, worldID := seaWorld(t, 30) // want 9 → 0.036 births per tick
	ctx := context.Background()
	dice := newStormDice(5)
	tx, _ := pool.Begin(ctx)
	if err := ensureStorms(ctx, tx, worldID, 1, loadGraph(t, pool, worldID), dice); err != nil {
		t.Fatal(err)
	}
	_ = tx.Commit(ctx)
	var seeded int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM sea_storms WHERE world_id = $1`, worldID).Scan(&seeded)
	for tick := 2; tick <= 400; tick++ {
		scanTo(t, pool, worldID, tick, dice)
	}
	var total, born int
	_ = pool.QueryRow(ctx, `SELECT count(*), count(*) FILTER (WHERE created_tick > 1) FROM sea_storms WHERE world_id = $1`, worldID).Scan(&total, &born)
	if born == 0 || total != seeded+born {
		t.Fatalf("after 400 ticks: %d storms, %d born after the seed (seeded %d)", total, born, seeded)
	}
	var bad int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM sea_storms WHERE world_id = $1 AND created_tick > 1
	    AND (dies_tick - created_tick < $2 OR dies_tick - created_tick > $3)`, worldID, StormLifeMinTicks, StormLifeMaxTicks).Scan(&bad)
	if bad != 0 {
		t.Fatalf("%d born storms have a life outside [%d, %d]", bad, StormLifeMinTicks, StormLifeMaxTicks)
	}
	var living int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM sea_storms WHERE world_id = $1 AND died_tick IS NULL`, worldID).Scan(&living)
	if living < 5 || living > 14 {
		t.Fatalf("%d living storms after 400 ticks, want near 9", living)
	}
	// a duplicate scan of an already-rolled tick changes nothing
	var rowsBefore int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM sea_storm_track t JOIN sea_storms s ON s.id=t.storm_id WHERE s.world_id=$1`, worldID).Scan(&rowsBefore)
	scanTo(t, pool, worldID, 400, newStormDice(99))
	var rowsAfter, totalAfter int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM sea_storm_track t JOIN sea_storms s ON s.id=t.storm_id WHERE s.world_id=$1`, worldID).Scan(&rowsAfter)
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM sea_storms WHERE world_id = $1`, worldID).Scan(&totalAfter)
	if rowsAfter != rowsBefore || totalAfter != total {
		t.Fatalf("a duplicate scan changed the weather: tracks %d -> %d, storms %d -> %d", rowsBefore, rowsAfter, total, totalAfter)
	}
}

// A late scan equals the daily scans, also when storms die and are born inside the interval.
func TestStormLifecycle_LateScanEqualsDailyScansAcrossDeathsAndBirths(t *testing.T) {
	setup := func() (*pgxpool.Pool, uuid.UUID, *stormDice) {
		pool, worldID := seaWorld(t, 60) // 3600 hexes → 36 storms, 0.144 births a tick
		dice := newStormDice(21)
		ctx := context.Background()
		tx, _ := pool.Begin(ctx)
		if err := ensureStorms(ctx, tx, worldID, 1, loadGraph(t, pool, worldID), dice); err != nil {
			t.Fatal(err)
		}
		_ = tx.Commit(ctx)
		// the same six storms (lowest first hex) die at tick 12, whatever their random ids
		_, err := pool.Exec(ctx, `UPDATE sea_storms SET dies_tick = 12 WHERE id IN (
		   SELECT s.id FROM sea_storms s JOIN sea_storm_track t ON t.storm_id = s.id AND t.tick = 1 AND t.slot = 0
		    WHERE s.world_id = $1 ORDER BY t.q, t.r LIMIT 6)`, worldID)
		if err != nil {
			t.Fatal(err)
		}
		return pool, worldID, dice
	}
	read := func(pool *pgxpool.Pool, worldID uuid.UUID) (map[[4]int]bool, int, int) {
		out := map[[4]int]bool{}
		rows, err := pool.Query(context.Background(),
			`SELECT t.tick, t.slot, t.q, t.r FROM sea_storm_track t JOIN sea_storms s ON s.id = t.storm_id WHERE s.world_id = $1`, worldID)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		for rows.Next() {
			var k [4]int
			_ = rows.Scan(&k[0], &k[1], &k[2], &k[3])
			out[k] = true
		}
		var born, died int
		_ = pool.QueryRow(context.Background(), `SELECT count(*) FILTER (WHERE created_tick > 1), count(*) FILTER (WHERE died_tick IS NOT NULL) FROM sea_storms WHERE world_id = $1`, worldID).Scan(&born, &died)
		return out, born, died
	}
	poolA, worldA, diceA := setup()
	for tick := 2; tick <= 30; tick++ {
		scanTo(t, poolA, worldA, tick, diceA)
	}
	poolB, worldB, diceB := setup()
	scanTo(t, poolB, worldB, 30, diceB) // one late scan
	ta, bornA, diedA := read(poolA, worldA)
	tb, bornB, diedB := read(poolB, worldB)
	if bornA == 0 || diedA != 6 {
		t.Fatalf("the daily run must cover births and deaths: born %d, died %d", bornA, diedA)
	}
	if bornA != bornB || diedA != diedB || len(ta) != len(tb) {
		t.Fatalf("daily: %d born %d died %d track hexes; late: %d born %d died %d track hexes", bornA, diedA, len(ta), bornB, diedB, len(tb))
	}
	for k := range ta {
		if !tb[k] {
			t.Fatalf("late scan lacks daily-scan track hex %v", k)
		}
	}
}
