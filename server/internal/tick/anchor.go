package tick

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Queryer abstracts *pgxpool.Pool and pgx.Tx for LoadAnchor. Both concrete
// types satisfy this interface via their QueryRow method (mirrors
// province.Queryer's own reasoning for Query).
type Queryer interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Anchor pins a world's tick counter to a wall-clock instant: Tick is
// worlds.current_tick and At is worlds.last_tick_at, the nominal wall-clock
// start of Tick (set by the tick worker's own addition, worker.go
// tryAdvanceOnce — never a fresh now()). It converts between the two without
// ever calling clock.Clock.Now() itself: callers pass "now" in, exactly like
// every other function in this package (movement slice 2a, R3).
type Anchor struct {
	Tick int
	At   time.Time
}

// LoadAnchor reads a world's current tick anchor.
func LoadAnchor(ctx context.Context, db Queryer, worldID uuid.UUID) (Anchor, error) {
	var a Anchor
	if err := db.QueryRow(ctx,
		`SELECT current_tick, last_tick_at FROM worlds WHERE id = $1`, worldID,
	).Scan(&a.Tick, &a.At); err != nil {
		return Anchor{}, err
	}
	return a, nil
}

// MilliAt converts a wall-clock instant to a movement.Milli-compatible tick
// count (thousandths of a tick since world start): Tick*1000 plus how far
// into the CURRENT tick now falls, clamped to [0,999] so a now before At (a
// stale read) or an now past a full tick (the tick worker hasn't advanced
// yet) never produces an out-of-tick value. Returns int64, not movement.Milli,
// so this package does not need to import movement (G1: tick has zero
// internal deps).
func (a Anchor) MilliAt(now time.Time) int64 {
	tickDur := time.Duration(TickSeconds) * time.Second
	if tickDur <= 0 {
		return int64(a.Tick) * 1000
	}
	frac := now.Sub(a.At)
	m := frac * 1000 / tickDur
	if m < 0 {
		m = 0
	}
	if m > 999 {
		m = 999
	}
	return int64(a.Tick)*1000 + int64(m)
}

// WallAt is MilliAt's inverse: the wall-clock instant a given Milli (relative
// to this same Anchor) falls at.
func (a Anchor) WallAt(m int64) time.Time {
	tickDur := time.Duration(TickSeconds) * time.Second
	delta := m - int64(a.Tick)*1000
	return a.At.Add(time.Duration(delta) * tickDur / 1000)
}
