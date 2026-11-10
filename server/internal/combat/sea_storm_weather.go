package combat

// Stormar som syns och driver (megaron_plan_stormar.md, Timothy 2026-10-10).
//
// A storm is a unit of three connected sea hexes — a polyhex that crawls. Each
// tick it takes two shape steps: ONE hex moves to an adjacent hex per step,
// and the unit must stay connected after each step. The steps are rolled here,
// once, and stored as the track (sea_storm_track) — ships are hit by the stored
// track, never by a fresh die. One storm per SeaHexesPerStorm sea hexes.

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"sort"

	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/hexgrid"
	"formatet/megaron/server/internal/province"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// SeaHexesPerStorm: the world holds floor(sea hexes / this) storms (Timothy 2026-10-10: 100, was 36 — seafaring was lethal).
const SeaHexesPerStorm = 100

// StormHexes is how many hexes a storm covers when it is born. A storm may later cover fewer
// (1..StormHexes; the storm prayers shrink it, megaron_bonesystem_v3 §Steg 3).
const StormHexes = 3

// A storm's life is rolled once at birth, uniformly in [StormLifeMinTicks, StormLifeMaxTicks]
// (Timothy 2026-11-07; alpha numbers, tune freely), and stored as sea_storms.dies_tick.
const (
	StormLifeMinTicks = 200
	StormLifeMaxTicks = 300
)

// stormMeanLife is the mean of the life roll; births per tick = wanted storms / this, so the
// average number of living storms stays at one per SeaHexesPerStorm sea hexes.
const stormMeanLife = float64(StormLifeMinTicks+StormLifeMaxTicks) / 2

// stormLife rolls one storm's life in ticks.
func stormLife(dice economy.Dice) int {
	return StormLifeMinTicks + dice.Intn(StormLifeMaxTicks-StormLifeMinTicks+1)
}

// stormBirthCount is how many storms are born this tick: floor(chance) and one more with the
// remainder as probability, where chance = want / mean life.
func stormBirthCount(want int, dice economy.Dice) int {
	chance := float64(want) / stormMeanLife
	n := int(chance)
	if dice.Float64() < chance-float64(n) {
		n++
	}
	return n
}

// StormStepsPerTick doubles the former drift speed (Timothy 2026-10-20).
const StormStepsPerTick = 2

const (
	stormTurnChance   = 0.15 // chance per tick that the heading turns one step
	stormRandomChance = 0.20 // chance per tick that the step is any legal step
	// stormTrackKeepTicks: track rows older than this are pruned. A voyage is only
	// ever checked against ticks after its own high-water mark.
	stormTrackKeepTicks = 120
	// stormCatchUpMax bounds the steps one scan writes after a long outage.
	stormCatchUpMax = 200
)

// stormDirs are the six hex directions in ANGULAR order, so heading±1 is a 60° turn.
var stormDirs = [6]hexgrid.Coord{{Q: 1, R: 0}, {Q: 1, R: -1}, {Q: 0, R: -1}, {Q: -1, R: 0}, {Q: -1, R: 1}, {Q: 0, R: 1}}

// StormShape is a storm's hexes, 1..StormHexes of them. Slot i keeps its identity across steps.
// A slice, not an array: the length is the storm's size, and the size varies.
type StormShape []hexgrid.Coord

func (s StormShape) clone() StormShape { return append(StormShape(nil), s...) }

func cart(c hexgrid.Coord) (x, y float64) {
	return float64(c.Q) + float64(c.R)/2, float64(c.R) * math.Sqrt(3) / 2
}

func centroid(s StormShape) (x, y float64) {
	for _, c := range s {
		cx, cy := cart(c)
		x += cx
		y += cy
	}
	return x / float64(len(s)), y / float64(len(s))
}

func (s StormShape) has(c hexgrid.Coord) bool {
	for _, h := range s {
		if h == c {
			return true
		}
	}
	return false
}

// Connected reports whether the hexes form one connected group (1 hex: always;
// 2: adjacent; 3: at least two of the three pairs adjacent).
func (s StormShape) Connected() bool {
	if len(s) == 0 {
		return false
	}
	seen := make([]bool, len(s))
	seen[0] = true
	stack := []int{0}
	reached := 1
	for len(stack) > 0 {
		i := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for j := range s {
			if !seen[j] && hexgrid.Distance(s[i], s[j]) == 1 {
				seen[j] = true
				reached++
				stack = append(stack, j)
			}
		}
	}
	return reached == len(s)
}

// StepStorm moves ONE hex of the storm to an adjacent sea hex so that the unit stays
// connected. The heading steers: the legal step that carries the storm's centre
// furthest along it wins; with low chance the heading turns or any legal step is
// taken instead. No legal step: nothing moves and the storm picks a new heading.
// Returns the new shape, the new heading and which slot moved (-1 = none).
func StepStorm(s StormShape, heading int, sea func(hexgrid.Coord) bool, dice economy.Dice) (StormShape, int, int) {
	if dice.Float64() < stormTurnChance {
		heading = (heading + dice.Intn(2)*2 - 1 + 6) % 6
	}
	type cand struct {
		slot int
		to   hexgrid.Coord
		gain float64
		dx   float64
		dy   float64
	}
	var cands []cand
	ox, oy := centroid(s)
	hx, hy := cart(stormDirs[heading])
	for slot := range s {
		for _, d := range stormDirs {
			to := hexgrid.Coord{Q: s[slot].Q + d.Q, R: s[slot].R + d.R}
			if s.has(to) || !sea(to) {
				continue
			}
			next := s.clone()
			next[slot] = to
			if !next.Connected() {
				continue
			}
			nx, ny := centroid(next)
			cands = append(cands, cand{slot, to, (nx-ox)*hx + (ny-oy)*hy, nx - ox, ny - oy})
		}
	}
	if len(cands) == 0 {
		return s, dice.Intn(6), -1
	}
	pick := cands[dice.Intn(len(cands))]
	best := cands[0].gain
	for _, c := range cands {
		best = math.Max(best, c.gain)
	}
	if best <= 1e-9 {
		// Pressed against a coast: no legal step carries the storm forward, so it turns away.
		// Turn to the side where a legal step does carry the storm forward; dice decide a tie.
		sides := [2]int{(heading + 5) % 6, (heading + 1) % 6}
		var reach [2]float64
		for i, h := range sides {
			ux, uy := cart(stormDirs[h])
			reach[i] = math.Inf(-1)
			for _, c := range cands {
				reach[i] = math.Max(reach[i], c.dx*ux+c.dy*uy)
			}
		}
		switch {
		case reach[0] > reach[1]+1e-9:
			heading = sides[0]
		case reach[1] > reach[0]+1e-9:
			heading = sides[1]
		default:
			heading = sides[dice.Intn(2)]
		}
	}
	if dice.Float64() >= stormRandomChance {
		var top []cand
		for _, c := range cands {
			if c.gain >= best-1e-9 {
				top = append(top, c)
			}
		}
		pick = top[dice.Intn(len(top))]
	}
	s = s.clone()
	s[pick.slot] = pick.to
	return s, heading, pick.slot
}

// seedShape finds a fresh connected three-hex shape on unused sea hexes.
func seedShape(sea []hexgrid.Coord, used map[hexgrid.Coord]bool, isSea func(hexgrid.Coord) bool, dice economy.Dice) (StormShape, bool) {
	for try := 0; try < 60; try++ {
		a := sea[dice.Intn(len(sea))]
		if shape, ok := seedShapeAt(a, used, isSea, dice); ok {
			return shape, true
		}
	}
	return nil, false
}

// seedShapeAt grows a connected three-hex shape from a on unused sea hexes (false when a or its
// surroundings leave no room). The Wake the Storm prayer starts here with the hex it was given.
func seedShapeAt(a hexgrid.Coord, used map[hexgrid.Coord]bool, isSea func(hexgrid.Coord) bool, dice economy.Dice) (StormShape, bool) {
	free := func(c hexgrid.Coord) bool { return isSea(c) && !used[c] }
	if !free(a) {
		return nil, false
	}
	var opts []hexgrid.Coord
	for _, d := range stormDirs {
		if n := (hexgrid.Coord{Q: a.Q + d.Q, R: a.R + d.R}); free(n) {
			opts = append(opts, n)
		}
	}
	if len(opts) == 0 {
		return nil, false
	}
	b := opts[dice.Intn(len(opts))]
	var opts2 []hexgrid.Coord
	for _, base := range []hexgrid.Coord{a, b} {
		for _, d := range stormDirs {
			n := hexgrid.Coord{Q: base.Q + d.Q, R: base.R + d.R}
			if free(n) && n != a && n != b {
				opts2 = append(opts2, n)
			}
		}
	}
	if len(opts2) == 0 {
		return nil, false
	}
	return StormShape{a, b, opts2[dice.Intn(len(opts2))]}, true
}

// seaHexes lists the world's sea hexes in a fixed order (so the dice replay exactly).
func seaHexes(graph province.TileGraph) []hexgrid.Coord {
	var sea []hexgrid.Coord
	for k, terrain := range graph {
		if isSeaTerrain(terrain) {
			sea = append(sea, hexgrid.Coord{Q: k[0], R: k[1]})
		}
	}
	sort.Slice(sea, func(i, j int) bool {
		if sea[i].Q != sea[j].Q {
			return sea[i].Q < sea[j].Q
		}
		return sea[i].R < sea[j].R
	})
	return sea
}

// insertStorm stores a new storm and its first track row. dies is the stored outcome of the life roll.
func insertStorm(ctx context.Context, tx pgx.Tx, worldID uuid.UUID, tick, heading, dies int, shape StormShape) (uuid.UUID, error) {
	var id uuid.UUID
	if err := tx.QueryRow(ctx,
		`INSERT INTO sea_storms (world_id, heading, created_tick, dies_tick) VALUES ($1, $2, $3, $4) RETURNING id`,
		worldID, heading, tick, dies).Scan(&id); err != nil {
		return id, err
	}
	return id, writeStormTrack(ctx, tx, id, tick, shape)
}

// ensureStorms seeds a new world's storms once: floor(sea hexes / SeaHexesPerStorm). "Once" is
// "the world has never had a storm": sea_storms rows are never deleted when a storm dies, so
// count(*) over ALL rows is that test and a world whose storms have all died is not reseeded
// (births refill it). Each seeded storm gets a random age — dies_tick = tick + 1..life — so
// they do not all die together.
func ensureStorms(ctx context.Context, tx pgx.Tx, worldID uuid.UUID, tick int, graph province.TileGraph, dice economy.Dice) error {
	var have int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM sea_storms WHERE world_id = $1`, worldID).Scan(&have); err != nil {
		return err
	}
	if have > 0 {
		return nil
	}
	sea := seaHexes(graph)
	want := len(sea) / SeaHexesPerStorm
	if want == 0 {
		return nil
	}
	isSea := func(c hexgrid.Coord) bool { return isSeaTerrain(graph[[2]int{c.Q, c.R}]) }
	used := map[hexgrid.Coord]bool{}
	for i := 0; i < want; i++ {
		shape, ok := seedShape(sea, used, isSea, dice)
		if !ok {
			continue
		}
		for _, c := range shape {
			used[c] = true
		}
		if _, err := insertStorm(ctx, tx, worldID, tick, dice.Intn(6), tick+1+dice.Intn(stormLife(dice)), shape); err != nil {
			return err
		}
	}
	return nil
}

func writeStormTrack(ctx context.Context, tx pgx.Tx, id uuid.UUID, tick int, s StormShape) error {
	for slot, c := range s {
		if _, err := tx.Exec(ctx,
			`INSERT INTO sea_storm_track (storm_id, tick, slot, q, r) VALUES ($1, $2, $3, $4, $5)
			 ON CONFLICT DO NOTHING`, id, tick, slot, c.Q, c.R); err != nil {
			return err
		}
	}
	return nil
}

// liveStorm is a storm the weather step walks forward.
type liveStorm struct {
	id      uuid.UUID
	heading int
	created int
	dies    int
	calms   []*pendingCalm // Still the Storm hexes waiting to grow back
	shape   StormShape
	last    int  // last tick with a stored track row
	stepped bool // moved in this scan: heading to store
}

// pendingCalm is a hex a prayer took that has not grown back yet.
type pendingCalm struct {
	id     uuid.UUID
	regrow int
}

// advanceStorms walks the world's weather to dueTick, TICK BY TICK (so a late scan rolls the
// dice in exactly the order daily scans would): at each tick every living storm takes its two
// shape steps, then new storms are born. A storm writes no track from its dies_tick on; the
// row stays and gets died_tick. The last rolled tick is kept in worlds.storm_weather_tick, so
// a duplicate or late scan never rolls a tick twice. One TX.
func advanceStorms(ctx context.Context, tx pgx.Tx, worldID uuid.UUID, dueTick int, graph province.TileGraph, dice economy.Dice) error {
	// Order: a stable one for the dice. Birth hex breaks ties between storms born the same tick
	// (ids are random); the track row may be pruned for old storms, then id decides.
	rows, err := tx.Query(ctx,
		`SELECT s.id, s.heading, s.created_tick, s.dies_tick FROM sea_storms s
		   LEFT JOIN sea_storm_track t0 ON t0.storm_id = s.id AND t0.tick = s.created_tick AND t0.slot = 0
		  WHERE s.world_id = $1 AND s.died_tick IS NULL
		  ORDER BY s.created_tick, t0.q, t0.r, s.id FOR UPDATE OF s`, worldID)
	if err != nil {
		return err
	}
	var storms []*liveStorm
	for rows.Next() {
		s := &liveStorm{}
		if err := rows.Scan(&s.id, &s.heading, &s.created, &s.dies); err != nil {
			rows.Close()
			return err
		}
		storms = append(storms, s)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	calmRows, err := tx.Query(ctx,
		`SELECT c.storm_id, c.id, c.regrow_tick FROM sea_storm_calms c JOIN sea_storms s ON s.id = c.storm_id
		  WHERE s.world_id = $1 AND s.died_tick IS NULL AND c.regrown_tick IS NULL ORDER BY c.regrow_tick, c.id`, worldID)
	if err != nil {
		return err
	}
	for calmRows.Next() {
		var sid uuid.UUID
		c := &pendingCalm{}
		if err := calmRows.Scan(&sid, &c.id, &c.regrow); err != nil {
			calmRows.Close()
			return err
		}
		for _, s := range storms {
			if s.id == sid {
				s.calms = append(s.calms, c)
			}
		}
	}
	calmRows.Close()
	if err := calmRows.Err(); err != nil {
		return err
	}
	anchor := dueTick - 1
	var weatherTick *int
	if err := tx.QueryRow(ctx, `SELECT storm_weather_tick FROM worlds WHERE id = $1`, worldID).Scan(&weatherTick); err != nil {
		return err
	}
	derived := false
	for _, s := range storms {
		shape, last, err := latestStormShape(ctx, tx, s.id)
		if err != nil {
			// One broken storm must not stop the sea for every ship.
			slog.Warn("sea storm: unreadable track, storm skipped", "storm", s.id, "err", err)
			s.shape = nil
			continue
		}
		s.shape, s.last = shape, last
		if weatherTick == nil && (!derived || last > anchor) {
			anchor, derived = last, true
		}
	}
	if weatherTick != nil {
		anchor = *weatherTick
	}
	from := anchor + 1
	if dueTick-anchor > stormCatchUpMax {
		from = dueTick - stormCatchUpMax + 1
	}
	isSea := func(c hexgrid.Coord) bool { return isSeaTerrain(graph[[2]int{c.Q, c.R}]) }
	sea := seaHexes(graph)
	want := len(sea) / SeaHexesPerStorm
	for t := from; t <= dueTick; t++ {
		used := map[hexgrid.Coord]bool{}
		// Regrowth looks at where every living storm stands at the start of the tick.
		occupied := map[hexgrid.Coord]bool{}
		for _, s := range storms {
			if s.shape != nil && t >= s.created && t < s.dies {
				for _, c := range s.shape {
					occupied[c] = true
				}
			}
		}
		for _, s := range storms {
			if s.shape == nil || t >= s.dies || t < s.created {
				continue // unreadable, dead (no more track), or not yet born (a woken storm, late scan)
			}
			if t > s.last {
				for len(s.calms) > 0 && s.calms[0].regrow <= t && len(s.shape) < StormHexes {
					grown, ok := regrowHex(s.shape, occupied, isSea, dice)
					if !ok {
						break // no room: try again next tick
					}
					s.shape = grown
					occupied[grown[len(grown)-1]] = true
					if _, err := tx.Exec(ctx, `UPDATE sea_storm_calms SET regrown_tick = $2 WHERE id = $1`, s.calms[0].id, t); err != nil {
						return err
					}
					s.calms = s.calms[1:]
				}
				for step := 0; step < StormStepsPerTick; step++ {
					s.shape, s.heading, _ = StepStorm(s.shape, s.heading, isSea, dice)
				}
				if err := writeStormTrack(ctx, tx, s.id, t, s.shape); err != nil {
					return err
				}
				s.last, s.stepped = t, true
			}
			for _, c := range s.shape {
				used[c] = true
			}
		}
		if want == 0 {
			continue
		}
		for n := stormBirthCount(want, dice); n > 0; n-- {
			shape, ok := seedShape(sea, used, isSea, dice)
			if !ok {
				continue
			}
			for _, c := range shape {
				used[c] = true
			}
			heading, life := dice.Intn(6), stormLife(dice)
			id, err := insertStorm(ctx, tx, worldID, t, heading, t+life, shape)
			if err != nil {
				return err
			}
			storms = append(storms, &liveStorm{id: id, heading: heading, dies: t + life, shape: shape, last: t})
		}
	}
	for _, s := range storms {
		if s.stepped {
			if _, err := tx.Exec(ctx, `UPDATE sea_storms SET heading = $2 WHERE id = $1`, s.id, s.heading); err != nil {
				return err
			}
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE sea_storms SET died_tick = dies_tick WHERE world_id = $1 AND died_tick IS NULL AND dies_tick <= $2`, worldID, dueTick); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE worlds SET storm_weather_tick = GREATEST(COALESCE(storm_weather_tick, 0), $2) WHERE id = $1`, worldID, dueTick); err != nil {
		return err
	}
	_, err = tx.Exec(ctx,
		`DELETE FROM sea_storm_track WHERE tick < $2 AND storm_id IN (SELECT id FROM sea_storms WHERE world_id = $1)`,
		worldID, dueTick-stormTrackKeepTicks)
	return err
}

// latestStormShape reads a storm's most recent stored shape (1..StormHexes hexes in slots
// 0..n-1) and its tick.
func latestStormShape(ctx context.Context, q interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}, id uuid.UUID) (StormShape, int, error) {
	rows, err := q.Query(ctx,
		`SELECT tick, slot, q, r FROM sea_storm_track
		  WHERE storm_id = $1 AND tick = (SELECT max(tick) FROM sea_storm_track WHERE storm_id = $1)
		  ORDER BY slot`, id)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var s StormShape
	tick := 0
	for rows.Next() {
		var slot int
		var c hexgrid.Coord
		if err := rows.Scan(&tick, &slot, &c.Q, &c.R); err != nil {
			return nil, 0, err
		}
		if slot != len(s) || slot >= StormHexes {
			return nil, 0, fmt.Errorf("storm %s: bad slot %d", id, slot)
		}
		s = append(s, c)
	}
	if len(s) == 0 {
		return nil, 0, fmt.Errorf("storm %s: track has no hexes", id)
	}
	return s, tick, rows.Err()
}

var stormHeadingNames = [6]string{"E", "NE", "NW", "W", "SW", "SE"}

// StormHeadingName names a heading (index into stormDirs) for the player surfaces.
func StormHeadingName(h int) string {
	if h < 0 || h >= len(stormHeadingNames) {
		return ""
	}
	return stormHeadingNames[h]
}
