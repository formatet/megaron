package combat

// Stormar som syns och driver (megaron_plan_stormar.md, Timothy 2026-10-10).
//
// A storm is a unit of three connected sea hexes — a polyhex that crawls. Each
// tick ONE of its hexes steps to an adjacent hex and the unit must still be
// connected afterwards; the other two stand still. The steps are rolled here,
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

// SeaHexesPerStorm: the world holds floor(sea hexes / this) storms (Timothy's 36).
const SeaHexesPerStorm = 36

// StormHexes is how many hexes one storm covers.
const StormHexes = 3

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

// StormShape is a storm's three hexes. Slot i keeps its identity across steps.
type StormShape [StormHexes]hexgrid.Coord

func cart(c hexgrid.Coord) (x, y float64) {
	return float64(c.Q) + float64(c.R)/2, float64(c.R) * math.Sqrt(3) / 2
}

func centroid(s StormShape) (x, y float64) {
	for _, c := range s {
		cx, cy := cart(c)
		x += cx
		y += cy
	}
	return x / StormHexes, y / StormHexes
}

func (s StormShape) has(c hexgrid.Coord) bool {
	for _, h := range s {
		if h == c {
			return true
		}
	}
	return false
}

// Connected reports whether the three hexes form one connected group
// (at least two of the three pairs are adjacent).
func (s StormShape) Connected() bool {
	n := 0
	for i := 0; i < StormHexes; i++ {
		for j := i + 1; j < StormHexes; j++ {
			if hexgrid.Distance(s[i], s[j]) == 1 {
				n++
			}
		}
	}
	return n >= 2
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
	}
	var cands []cand
	ox, oy := centroid(s)
	hx, hy := cart(stormDirs[heading])
	for slot := 0; slot < StormHexes; slot++ {
		for _, d := range stormDirs {
			to := hexgrid.Coord{Q: s[slot].Q + d.Q, R: s[slot].R + d.R}
			if s.has(to) || !sea(to) {
				continue
			}
			next := s
			next[slot] = to
			if !next.Connected() {
				continue
			}
			nx, ny := centroid(next)
			cands = append(cands, cand{slot, to, (nx-ox)*hx + (ny-oy)*hy})
		}
	}
	if len(cands) == 0 {
		return s, dice.Intn(6), -1
	}
	pick := cands[dice.Intn(len(cands))]
	if dice.Float64() >= stormRandomChance {
		best := cands[0].gain
		for _, c := range cands {
			best = math.Max(best, c.gain)
		}
		var top []cand
		for _, c := range cands {
			if c.gain >= best-1e-9 {
				top = append(top, c)
			}
		}
		pick = top[dice.Intn(len(top))]
	}
	s[pick.slot] = pick.to
	return s, heading, pick.slot
}

// seedShape finds a fresh connected three-hex shape on unused sea hexes.
func seedShape(sea []hexgrid.Coord, used map[hexgrid.Coord]bool, isSea func(hexgrid.Coord) bool, dice economy.Dice) (StormShape, bool) {
	free := func(c hexgrid.Coord) bool { return isSea(c) && !used[c] }
	for try := 0; try < 60; try++ {
		a := sea[dice.Intn(len(sea))]
		if !free(a) {
			continue
		}
		var opts []hexgrid.Coord
		for _, d := range stormDirs {
			if n := (hexgrid.Coord{Q: a.Q + d.Q, R: a.R + d.R}); free(n) {
				opts = append(opts, n)
			}
		}
		if len(opts) == 0 {
			continue
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
			continue
		}
		return StormShape{a, b, opts2[dice.Intn(len(opts2))]}, true
	}
	return StormShape{}, false
}

// ensureStorms creates the world's storms once: floor(sea hexes / SeaHexesPerStorm).
func ensureStorms(ctx context.Context, tx pgx.Tx, worldID uuid.UUID, tick int, graph province.TileGraph, dice economy.Dice) error {
	var have int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM sea_storms WHERE world_id = $1`, worldID).Scan(&have); err != nil {
		return err
	}
	if have > 0 {
		return nil
	}
	var sea []hexgrid.Coord
	for k, terrain := range graph {
		if isSeaTerrain(terrain) {
			sea = append(sea, hexgrid.Coord{Q: k[0], R: k[1]})
		}
	}
	want := len(sea) / SeaHexesPerStorm
	if want == 0 {
		return nil
	}
	sort.Slice(sea, func(i, j int) bool {
		if sea[i].Q != sea[j].Q {
			return sea[i].Q < sea[j].Q
		}
		return sea[i].R < sea[j].R
	})
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
		var id uuid.UUID
		if err := tx.QueryRow(ctx,
			`INSERT INTO sea_storms (world_id, heading, created_tick) VALUES ($1, $2, $3) RETURNING id`,
			worldID, dice.Intn(6), tick).Scan(&id); err != nil {
			return err
		}
		if err := writeStormTrack(ctx, tx, id, tick, shape); err != nil {
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

// advanceStorms extends every storm's track to dueTick, one step per tick, in one TX
// per storm. A duplicated or late scan never writes a (storm, tick) twice.
func advanceStorms(ctx context.Context, tx pgx.Tx, worldID uuid.UUID, dueTick int, graph province.TileGraph, dice economy.Dice) error {
	rows, err := tx.Query(ctx, `SELECT id, heading FROM sea_storms WHERE world_id = $1 ORDER BY id FOR UPDATE`, worldID)
	if err != nil {
		return err
	}
	type st struct {
		id      uuid.UUID
		heading int
	}
	var storms []st
	for rows.Next() {
		var s st
		if err := rows.Scan(&s.id, &s.heading); err != nil {
			rows.Close()
			return err
		}
		storms = append(storms, s)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	isSea := func(c hexgrid.Coord) bool { return isSeaTerrain(graph[[2]int{c.Q, c.R}]) }
	for _, s := range storms {
		shape, last, err := latestStormShape(ctx, tx, s.id)
		if err != nil {
			// One broken storm must not stop the sea for every ship.
			slog.Warn("sea storm: unreadable track, storm skipped", "storm", s.id, "err", err)
			continue
		}
		heading := s.heading
		from := last + 1
		if dueTick-last > stormCatchUpMax {
			from = dueTick - stormCatchUpMax + 1
		}
		for t := from; t <= dueTick; t++ {
			shape, heading, _ = StepStorm(shape, heading, isSea, dice)
			if err := writeStormTrack(ctx, tx, s.id, t, shape); err != nil {
				return err
			}
		}
		if dueTick > last {
			if _, err := tx.Exec(ctx, `UPDATE sea_storms SET heading = $2 WHERE id = $1`, s.id, heading); err != nil {
				return err
			}
		}
	}
	_, err = tx.Exec(ctx,
		`DELETE FROM sea_storm_track WHERE tick < $2 AND storm_id IN (SELECT id FROM sea_storms WHERE world_id = $1)`,
		worldID, dueTick-stormTrackKeepTicks)
	return err
}

// latestStormShape reads a storm's most recent stored shape and its tick.
func latestStormShape(ctx context.Context, q interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}, id uuid.UUID) (StormShape, int, error) {
	rows, err := q.Query(ctx,
		`SELECT tick, slot, q, r FROM sea_storm_track
		  WHERE storm_id = $1 AND tick = (SELECT max(tick) FROM sea_storm_track WHERE storm_id = $1)
		  ORDER BY slot`, id)
	if err != nil {
		return StormShape{}, 0, err
	}
	defer rows.Close()
	var s StormShape
	tick, n := 0, 0
	for rows.Next() {
		var slot int
		var c hexgrid.Coord
		if err := rows.Scan(&tick, &slot, &c.Q, &c.R); err != nil {
			return StormShape{}, 0, err
		}
		if slot < 0 || slot >= StormHexes {
			return StormShape{}, 0, fmt.Errorf("storm %s: bad slot %d", id, slot)
		}
		s[slot] = c
		n++
	}
	if n != StormHexes {
		return StormShape{}, 0, fmt.Errorf("storm %s: track has %d hexes, want %d", id, n, StormHexes)
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
