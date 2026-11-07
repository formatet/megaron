package combat

// Stormbönerna (bönesystemet v3 steg 3b, Timothy 2026-11-07): "Still the Storm" shrinks a named
// storm by one hex (the hex grows back after StormRegrowTicks; a storm shrunk to nothing dies) and
// "Wake the Storm" lets an ordinary storm be born on a sea hex. Both take the SAME advisory lock as
// the weather scan, so they are serialised against it, and both store their outcome (the roll is
// made once). Idempotent on cast_key: the same key twice is one effect.

import (
	"context"
	"errors"
	"sort"

	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/hexgrid"
	"formatet/megaron/server/internal/province"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// StormRegrowTicks: a hex taken away by a prayer grows back this many ticks later.
const StormRegrowTicks = 24

var (
	ErrStormGone    = errors.New("that storm is gone")
	ErrWakeNotSea   = errors.New("a storm can only be woken on open sea")
	ErrWakeOccupied = errors.New("there is no room for a storm there")
)

func lockStorms(ctx context.Context, tx pgx.Tx, worldID uuid.UUID) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, "sea_storms:"+worldID.String())
	return err
}

// CalmResult is what one Still the Storm cast did.
type CalmResult struct {
	Removed    hexgrid.Coord
	RegrowTick int // 0 when the storm died
	Remaining  int
	Died       bool
}

// removableSlots lists the slots that can go with the rest still connected (for three in a row:
// the two ends, never the middle).
func removableSlots(s StormShape) []int {
	var out []int
	for i := range s {
		rest := make(StormShape, 0, len(s)-1)
		rest = append(rest, s[:i]...)
		rest = append(rest, s[i+1:]...)
		if len(rest) == 0 || rest.Connected() {
			out = append(out, i)
		}
	}
	return out
}

// CalmStorm takes one hex off a living storm: the storm's latest track row is rewritten without it
// (so the next scan tick starts from the smaller shape), the hex is recorded for regrowth, and a
// storm with nothing left dies. Not the caller's job to check ownership: the storm helps everyone.
func CalmStorm(ctx context.Context, tx pgx.Tx, worldID, stormID uuid.UUID, tick int, castKey uuid.UUID, dice economy.Dice) (*CalmResult, error) {
	if err := lockStorms(ctx, tx, worldID); err != nil {
		return nil, err
	}
	var q, r, regrow int
	err := tx.QueryRow(ctx, `SELECT removed_q, removed_r, regrow_tick FROM sea_storm_calms WHERE cast_key = $1`, castKey).Scan(&q, &r, &regrow)
	if err == nil {
		return &CalmResult{Removed: hexgrid.Coord{Q: q, R: r}, RegrowTick: regrow}, nil
	}
	if err != pgx.ErrNoRows {
		return nil, err
	}
	var live bool
	if err := tx.QueryRow(ctx,
		`SELECT true FROM sea_storms WHERE id = $1 AND world_id = $2 AND died_tick IS NULL AND dies_tick > $3 FOR UPDATE`,
		stormID, worldID, tick).Scan(&live); err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrStormGone
		}
		return nil, err
	}
	shape, last, err := latestStormShape(ctx, tx, stormID)
	if err != nil {
		return nil, err
	}
	cands := removableSlots(shape)
	slot := cands[dice.Intn(len(cands))]
	removed := shape[slot]
	res := &CalmResult{Removed: removed, Remaining: len(shape) - 1}
	if len(shape) == 1 {
		// Nothing left: dead from the first tick without track. The track before stays.
		res.Died = true
		if _, err := tx.Exec(ctx, `UPDATE sea_storms SET died_tick = $2, dies_tick = LEAST(dies_tick, $2) WHERE id = $1`, stormID, last+1); err != nil {
			return nil, err
		}
	} else {
		res.RegrowTick = tick + StormRegrowTicks
		rest := make(StormShape, 0, len(shape)-1)
		rest = append(rest, shape[:slot]...)
		rest = append(rest, shape[slot+1:]...)
		if _, err := tx.Exec(ctx, `DELETE FROM sea_storm_track WHERE storm_id = $1 AND tick = $2`, stormID, last); err != nil {
			return nil, err
		}
		if err := writeStormTrack(ctx, tx, stormID, last, rest); err != nil {
			return nil, err
		}
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO sea_storm_calms (storm_id, cast_key, cast_tick, regrow_tick, removed_q, removed_r) VALUES ($1, $2, $3, $4, $5, $6)`,
		stormID, castKey, tick, tick+StormRegrowTicks, removed.Q, removed.R); err != nil {
		return nil, err
	}
	// A hex the storm left may be a fishing place that was shut; the scan only looks at hexes in the track.
	if err := recomputeFishingAt(ctx, tx, worldID, removed); err != nil {
		return nil, err
	}
	if res.Died {
		res.RegrowTick = 0
	}
	return res, nil
}

// recomputeFishingAt recomputes every city with a fisher placed on hex, so its stored fish rate follows
// the storm that left it (economy/storm_fishing.go reads the track for the current tick).
func recomputeFishingAt(ctx context.Context, tx pgx.Tx, worldID uuid.UUID, hex hexgrid.Coord) error {
	rows, err := tx.Query(ctx,
		`SELECT DISTINCT p.settlement_id FROM settlement_placement p JOIN settlements s ON s.id = p.settlement_id AND s.world_id = $1
		  WHERE p.target_kind = 'hex' AND p.good_key = 'fish' AND p.hex_q = $2 AND p.hex_r = $3`, worldID, hex.Q, hex.R)
	if err != nil {
		return err
	}
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, id := range ids {
		if err := economy.RecomputeProduction(ctx, tx, id); err != nil {
			return err
		}
	}
	return nil
}

// regrowHex adds one hex to shape: a sea neighbour of the storm that no living storm occupies. The
// candidates are sorted so the die replays; none -> no roll and (shape, false): try again next tick.
func regrowHex(shape StormShape, occupied map[hexgrid.Coord]bool, isSea func(hexgrid.Coord) bool, dice economy.Dice) (StormShape, bool) {
	seen := map[hexgrid.Coord]bool{}
	var cands []hexgrid.Coord
	for _, c := range shape {
		for _, d := range stormDirs {
			n := hexgrid.Coord{Q: c.Q + d.Q, R: c.R + d.R}
			if seen[n] || shape.has(n) || occupied[n] || !isSea(n) {
				continue
			}
			seen[n] = true
			cands = append(cands, n)
		}
	}
	if len(cands) == 0 {
		return shape, false
	}
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].Q != cands[j].Q {
			return cands[i].Q < cands[j].Q
		}
		return cands[i].R < cands[j].R
	})
	out := shape.clone()
	return append(out, cands[dice.Intn(len(cands))]), true
}

// WakeResult is what one Wake the Storm cast did.
type WakeResult struct {
	StormID uuid.UUID
	Hexes   []hexgrid.Coord
}

// WakeStorm is a storm's birth on a chosen sea hex: the hex and two connected sea neighbours no living
// storm stands on, a life rolled like any birth, no name, no claim. Nothing marks it as woken.
func WakeStorm(ctx context.Context, tx pgx.Tx, worldID uuid.UUID, hex hexgrid.Coord, tick int, castKey uuid.UUID, dice economy.Dice) (*WakeResult, error) {
	if err := lockStorms(ctx, tx, worldID); err != nil {
		return nil, err
	}
	var sid uuid.UUID
	var q, r int
	err := tx.QueryRow(ctx, `SELECT storm_id, hex_q, hex_r FROM sea_storm_wakes WHERE cast_key = $1`, castKey).Scan(&sid, &q, &r)
	if err == nil {
		shape, _, lerr := latestStormShape(ctx, tx, sid)
		if lerr != nil {
			shape = StormShape{{Q: q, R: r}}
		}
		return &WakeResult{StormID: sid, Hexes: shape}, nil
	}
	if err != pgx.ErrNoRows {
		return nil, err
	}
	graph, err := province.LoadTileGraph(ctx, tx, worldID)
	if err != nil {
		return nil, err
	}
	isSea := func(c hexgrid.Coord) bool { return isSeaTerrain(graph[[2]int{c.Q, c.R}]) }
	if !isSea(hex) {
		return nil, ErrWakeNotSea
	}
	used, err := livingStormHexes(ctx, tx, worldID, tick)
	if err != nil {
		return nil, err
	}
	if used[hex] {
		return nil, ErrWakeOccupied
	}
	shape, ok := seedShapeAt(hex, used, isSea, dice)
	if !ok {
		return nil, ErrWakeOccupied
	}
	heading, life := dice.Intn(6), stormLife(dice)
	id, err := insertStorm(ctx, tx, worldID, tick, heading, tick+life, shape)
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO sea_storm_wakes (cast_key, storm_id, cast_tick, hex_q, hex_r) VALUES ($1, $2, $3, $4, $5)`,
		castKey, id, tick, hex.Q, hex.R); err != nil {
		return nil, err
	}
	return &WakeResult{StormID: id, Hexes: shape}, nil
}

// livingStormHexes is every hex a living storm stands on in its latest stored shape.
func livingStormHexes(ctx context.Context, tx pgx.Tx, worldID uuid.UUID, tick int) (map[hexgrid.Coord]bool, error) {
	rows, err := tx.Query(ctx, `SELECT id FROM sea_storms WHERE world_id = $1 AND died_tick IS NULL AND dies_tick > $2 ORDER BY id`, worldID, tick)
	if err != nil {
		return nil, err
	}
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	used := map[hexgrid.Coord]bool{}
	for _, id := range ids {
		shape, _, err := latestStormShape(ctx, tx, id)
		if err != nil {
			continue // an unreadable storm occupies nothing we can name
		}
		for _, c := range shape {
			used[c] = true
		}
	}
	return used, nil
}
