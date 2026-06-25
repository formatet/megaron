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

// TickMinutes is the runtime tick cadence (minutes of real time per tick).
// Read once from TICK_MINUTES at init; mirrors the value used by Worker.
// Handlers use this to convert tick durations to approximate wall-clock times
// for display purposes (build_queue.complete_at, messenger arrives_at, etc.).
var TickMinutes = func() int {
	if v := os.Getenv("TICK_MINUTES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return defaultTickMinutes
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
	tickMinutes int
}

// New creates a Worker. clk is the sole time source; pass clock.TestClock in
// tests. TICK_MINUTES env var overrides the default 60-minute cadence.
func New(pool *pgxpool.Pool, clk clock.Clock, store *events.Store) *Worker {
	minutes := defaultTickMinutes
	if v := os.Getenv("TICK_MINUTES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			minutes = n
		}
	}
	return &Worker{
		pool:        pool,
		clock:       clk,
		store:       store,
		tickMinutes: minutes,
	}
}

// Run polls for pending tick advances until ctx is cancelled.
// Runs once immediately on startup to catch up after downtime, then every
// 30 seconds thereafter.
func (w *Worker) Run(ctx context.Context) {
	slog.Info("tick worker started", "tick_minutes", w.tickMinutes)
	tickDur := time.Duration(w.tickMinutes) * time.Minute

	// Catch-up pass on startup.
	if err := w.advancePending(ctx, tickDur); err != nil {
		slog.Error("tick advance failed on startup", "err", err)
	}

	ticker := time.NewTicker(30 * time.Second)
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
