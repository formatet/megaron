// Package tick drives the game's single integer tick counter.
//
// One TickWorker per server instance. Only this worker reads clock.Clock.Now()
// for cadence; all other game logic must use worlds.current_tick.
// Inject clock.TestClock in tests for deterministic, zero-wait tick advances.
package tick

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/poleia/server/internal/clock"
	"github.com/poleia/server/internal/events"
)

const (
	defaultTickMinutes = 60

	// EventWorldTick is the event type emitted after each tick advance.
	// Payload: WorldTickPayload.
	EventWorldTick = "WorldTick"
)

// tickSeconds resolves the real-time cadence (seconds per tick) from the
// environment. TICK_SECONDS wins when set (allows a sub-minute, sped-up dev
// cadence, e.g. TICK_SECONDS=6 for 10× a 1-minute tick); otherwise
// TICK_MINUTES × 60; otherwise the 60-minute default.
func tickSeconds() int {
	if v := os.Getenv("TICK_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	if v := os.Getenv("TICK_MINUTES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n * 60
		}
	}
	return defaultTickMinutes * 60
}

// TickSeconds is the runtime tick cadence in seconds of real time per tick.
// Use this (not TickMinutes) whenever a display time DRIVES animation — e.g. a
// marching unit's arrives_at, which the map interpolates the unit's position
// against: it must match the real tick-scheduled arrival or the unit appears
// frozen at its origin then teleports on arrival (acute on a sub-minute cadence).
var TickSeconds = tickSeconds()

// TickMinutes is the runtime tick cadence in minutes.
//
// Deprecated: floors tickSeconds()/60 at 1, so it silently misrepresents any
// sub-minute TICK_SECONDS cadence (e.g. TICK_SECONDS=6 reads as 1 minute —
// 10x too long). Do NOT use it for ETA/display-time derivation — use
// RealUntil/EtaAt (eta.go), which convert exactly via TickSeconds instead.
// TickMinutes is intentionally kept for the two use classes where a floored
// minute is fine or even desired: loyalty grace-window/threshold maths
// (internal/loyalty/decay.go, welfare.go — a coarse day-scale window, not a
// display ETA) and the static catalogue DURATION fields (province.go
// BuildingCatalogue/UnitCatalogue — "how long this type of build takes" in
// general, not "when this instance completes").
var TickMinutes = func() int {
	m := tickSeconds() / 60
	if m < 1 {
		m = 1
	}
	return m
}()

// WorldTickPayload is the event payload for a WorldTick event.
type WorldTickPayload struct {
	WorldID uuid.UUID `json:"world_id"`
	Tick    int       `json:"tick"`
}

// Worker advances worlds.current_tick by 1 every TICK_MINUTES real minutes.
// If the server was down for multiple tick periods the worker catches up one
// tick at a time (deterministic, never jumps to now).
type Worker struct {
	pool        *pgxpool.Pool
	clock       clock.Clock
	store       *events.Store
	tickSeconds int
}

// New creates a Worker. clk is the sole time source; pass clock.TestClock in
// tests. TICK_MINUTES env var overrides the default 60-minute cadence.
func New(pool *pgxpool.Pool, clk clock.Clock, store *events.Store) *Worker {
	return &Worker{
		pool:        pool,
		clock:       clk,
		store:       store,
		tickSeconds: tickSeconds(),
	}
}

// Run polls for pending tick advances until ctx is cancelled.
// Runs once immediately on startup to catch up after downtime, then every
// 30 seconds thereafter.
func (w *Worker) Run(ctx context.Context) {
	slog.Info("tick worker started", "tick_seconds", w.tickSeconds)
	tickDur := time.Duration(w.tickSeconds) * time.Second

	// Catch-up pass on startup.
	if err := w.advancePending(ctx, tickDur); err != nil {
		slog.Error("tick advance failed on startup", "err", err)
	}

	// Poll at least as often as the cadence (so a sub-minute tick actually fires
	// on time) but never faster than every 2 s, and never slower than 30 s.
	pollDur := tickDur
	if pollDur > 30*time.Second {
		pollDur = 30 * time.Second
	}
	if pollDur < 2*time.Second {
		pollDur = 2 * time.Second
	}
	ticker := time.NewTicker(pollDur)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			slog.Info("tick worker stopped")
			return
		case <-ticker.C:
			if err := w.advancePending(ctx, tickDur); err != nil {
				slog.Error("tick advance failed", "err", err)
			}
		}
	}
}

// advancePending loops until no active world has a pending tick.
// This implements the catch-up invariant: each iteration advances exactly one
// tick for one world — never jumps multiple ticks in one DB write.
func (w *Worker) advancePending(ctx context.Context, tickDur time.Duration) error {
	for {
		advanced, err := w.tryAdvanceOnce(ctx, tickDur)
		if err != nil {
			return err
		}
		if !advanced {
			return nil
		}
	}
}

// tryAdvanceOnce finds one active world whose tick is due and advances it by 1
// in a single transaction. Returns (true, nil) if a tick was advanced,
// (false, nil) if none were due.
//
// Idempotency: SELECT FOR UPDATE SKIP LOCKED prevents two concurrent workers
// from double-advancing the same world.
func (w *Worker) tryAdvanceOnce(ctx context.Context, tickDur time.Duration) (bool, error) {
	// cutoff: worlds whose last_tick_at is at or before this have a pending tick.
	cutoff := w.clock.Now().Add(-tickDur)

	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin tick tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var worldID uuid.UUID
	err = tx.QueryRow(ctx, `
		SELECT id FROM worlds
		WHERE status = 'active' AND last_tick_at <= $1
		ORDER BY last_tick_at
		LIMIT 1
		FOR UPDATE SKIP LOCKED
	`, cutoff).Scan(&worldID)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("find world to tick: %w", err)
	}

	// Advance tick by exactly 1 and bump last_tick_at by one tick duration.
	// Using addition (not SET now()) preserves catch-up accuracy.
	var newTick int
	err = tx.QueryRow(ctx, `
		UPDATE worlds
		SET current_tick = current_tick + 1,
		    last_tick_at = last_tick_at + ($1 * interval '1 second')
		WHERE id = $2
		RETURNING current_tick
	`, int(tickDur.Seconds()), worldID).Scan(&newTick)
	if err != nil {
		return false, fmt.Errorf("advance tick for world %s: %w", worldID, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit tick tx: %w", err)
	}

	// Emit WorldTick event after commit (best-effort; shadow in Fas 1).
	payload := WorldTickPayload{WorldID: worldID, Tick: newTick}
	if _, err := w.store.Append(ctx, worldID, events.StreamWorld, EventWorldTick, payload, worldID, nil); err != nil {
		slog.Warn("WorldTick event append failed", "world", worldID, "tick", newTick, "err", err)
	}

	slog.Info("world tick advanced", "world", worldID, "tick", newTick)
	return true, nil
}

// ticksDue returns how many ticks have elapsed since lastTickAt given the tick
// duration. Pure function; exported for tests.
func ticksDue(now, lastTickAt time.Time, tickDur time.Duration) int {
	if tickDur <= 0 {
		return 0
	}
	elapsed := now.Sub(lastTickAt)
	if elapsed < tickDur {
		return 0
	}
	return int(elapsed / tickDur)
}
