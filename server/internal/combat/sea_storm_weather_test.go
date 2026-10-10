package combat

import (
	"context"
	"testing"

	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/hexgrid"
	"formatet/megaron/server/internal/province"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func openSea(hexgrid.Coord) bool { return true }

func moved(a, b StormShape) (slots int) {
	for i := range a {
		if a[i] != b[i] {
			slots++
		}
	}
	return slots
}

// Property (megaron_plan_stormar.md): after every tick the storm is three distinct,
// connected hexes and at most ONE hex has moved, by exactly one step.
func TestStepStorm_OneHexStepsAndTheUnitStaysConnected(t *testing.T) {
	for seed := uint64(1); seed <= 40; seed++ {
		dice := newStormDice(seed)
		s := StormShape{{Q: 0, R: 0}, {Q: 1, R: 0}, {Q: 0, R: 1}}
		heading := int(seed % 6)
		for step := 0; step < 300; step++ {
			next, h, slot := StepStorm(s, heading, openSea, dice)
			if slot < 0 {
				t.Fatalf("seed %d step %d: open sea always has a legal step", seed, step)
			}
			if !next.Connected() {
				t.Fatalf("seed %d step %d: storm %v is no longer connected", seed, step, next)
			}
			if next[0] == next[1] || next[1] == next[2] || next[0] == next[2] {
				t.Fatalf("seed %d step %d: storm %v has overlapping hexes", seed, step, next)
			}
			if moved(s, next) != 1 || hexgrid.Distance(s[slot], next[slot]) != 1 {
				t.Fatalf("seed %d step %d: %v -> %v is not one hex stepping one step", seed, step, s, next)
			}
			s, heading = next, h
		}
	}
}

// A hex in a straight line cannot leave its place: the middle one would tear the storm in two.
func TestStepStorm_MiddleOfALineNeverMoves(t *testing.T) {
	line := StormShape{{Q: 0, R: 0}, {Q: 1, R: 0}, {Q: 2, R: 0}}
	for seed := uint64(1); seed <= 60; seed++ {
		_, _, slot := StepStorm(line, 0, openSea, newStormDice(seed))
		if slot == 1 {
			t.Fatalf("seed %d: the middle hex of a straight line moved and disconnected the storm", seed)
		}
	}
}

// Storms stay on sea: a one-hex-wide channel lets them move, land never takes a hex.
func TestStepStorm_NeverEntersLand(t *testing.T) {
	sea := func(c hexgrid.Coord) bool { return c.R >= 0 && c.R <= 2 && c.Q >= 0 && c.Q <= 6 }
	s := StormShape{{Q: 1, R: 1}, {Q: 2, R: 1}, {Q: 1, R: 2}}
	dice := newStormDice(9)
	heading := 0
	for step := 0; step < 500; step++ {
		s, heading, _ = StepStorm(s, heading, sea, dice)
		for _, c := range s {
			if !sea(c) {
				t.Fatalf("step %d: storm hex %v is on land", step, c)
			}
		}
	}
}

func TestStepStorm_BoxedInStaysPutAndTurns(t *testing.T) {
	s := StormShape{{Q: 0, R: 0}, {Q: 1, R: 0}, {Q: 0, R: 1}}
	only := func(c hexgrid.Coord) bool { return s.has(c) }
	next, _, slot := StepStorm(s, 0, only, newStormDice(1))
	if slot != -1 || next != s {
		t.Fatalf("a storm with no sea to step into must stay (slot %d, %v)", slot, next)
	}
}

// seaWorld makes an n×n all-sea world.
func seaWorld(t *testing.T, n int) (*pgxpool.Pool, uuid.UUID) {
	t.Helper()
	pool := testPool(t)
	worldID := sweepTestWorld(t)
	for q := 0; q < n; q++ {
		for r := 0; r < n; r++ {
			if _, err := pool.Exec(context.Background(), `INSERT INTO map_tiles (world_id, q, r, terrain) VALUES ($1, $2, $3, 'deep_sea')`, worldID, q, r); err != nil {
				t.Fatal(err)
			}
		}
	}
	return pool, worldID
}

func stormTracks(t *testing.T, pool *pgxpool.Pool, worldID uuid.UUID) (storms, rows int) {
	t.Helper()
	ctx := context.Background()
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM sea_storms WHERE world_id = $1`, worldID).Scan(&storms)
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM sea_storm_track t JOIN sea_storms s ON s.id = t.storm_id WHERE s.world_id = $1`, worldID).Scan(&rows)
	return
}

func TestSeaStormWeather_OneStormPerThirtySixSeaHexesAndNoDoubleStep(t *testing.T) {
	pool, worldID := seaWorld(t, 12) // 144 hexes → 4 storms
	h := NewSeaStormScanHandler(pool, events.NewScheduler(pool, nil), events.NewStore(pool), nil)
	h.Dice = newStormDice(5)
	runStormDay(t, h, worldID, 1)
	storms, rows := stormTracks(t, pool, worldID)
	if storms != 144/SeaHexesPerStorm {
		t.Fatalf("storms = %d, want %d (144 sea hexes / %d)", storms, 144/SeaHexesPerStorm, SeaHexesPerStorm)
	}
	runStormDay(t, h, worldID, 1) // duplicate scan of the same tick
	if _, again := stormTracks(t, pool, worldID); again != rows {
		t.Fatalf("a duplicate scan changed the tracks: %d -> %d rows", rows, again)
	}
	for day := 2; day <= 6; day++ {
		runStormDay(t, h, worldID, day)
	}
	if s2, rows2 := stormTracks(t, pool, worldID); s2 != storms || rows2 != storms*3*6 {
		t.Fatalf("after 6 ticks: %d storms, %d rows, want %d storms and %d rows", s2, rows2, storms, storms*3*6)
	}
}

// A scan that ran late fills the missed ticks with exactly the steps a daily scan would have taken.
func TestSeaStormWeather_LateScanEqualsDailyScans(t *testing.T) {
	read := func(pool *pgxpool.Pool, worldID uuid.UUID) map[[3]int]bool {
		out := map[[3]int]bool{}
		rows, err := pool.Query(context.Background(),
			`SELECT t.tick, t.q, t.r FROM sea_storm_track t JOIN sea_storms s ON s.id = t.storm_id WHERE s.world_id = $1`, worldID)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		for rows.Next() {
			var k [3]int
			if err := rows.Scan(&k[0], &k[1], &k[2]); err != nil {
				t.Fatal(err)
			}
			out[k] = true
		}
		return out
	}
	poolA, worldA := seaWorld(t, 7) // 49 hexes → one storm, so the dice order is the same
	a := NewSeaStormScanHandler(poolA, events.NewScheduler(poolA, nil), events.NewStore(poolA), nil)
	a.Dice = newStormDice(7)
	for day := 1; day <= 8; day++ {
		runStormDay(t, a, worldA, day)
	}
	poolB, worldB := seaWorld(t, 7)
	b := NewSeaStormScanHandler(poolB, events.NewScheduler(poolB, nil), events.NewStore(poolB), nil)
	b.Dice = newStormDice(7)
	runStormDay(t, b, worldB, 1)
	runStormDay(t, b, worldB, 8)
	ta, tb := read(poolA, worldA), read(poolB, worldB)
	if len(ta) == 0 || len(ta) != len(tb) {
		t.Fatalf("daily scans wrote %d track hexes, the late scan %d", len(ta), len(tb))
	}
	for k := range ta {
		if !tb[k] {
			t.Fatalf("late scan lacks daily-scan hex %v", k)
		}
	}
}

var _ province.TileGraph
