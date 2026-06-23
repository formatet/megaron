package events

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/poleia/server/internal/clock"
)

// ScheduledEventType identifies what should happen when the event fires.
type ScheduledEventType string

const (
	ScheduledArmyArrival       ScheduledEventType = "ArmyArrival"
	ScheduledBuildComplete     ScheduledEventType = "BuildComplete"
	ScheduledTrainComplete     ScheduledEventType = "TrainComplete"
	ScheduledCollapseCheck     ScheduledEventType = "CollapseCheck"
	ScheduledDivineRoll        ScheduledEventType = "DivineRoll"
	ScheduledLoyaltyDecayTick  ScheduledEventType = "LoyaltyDecayTick"
	ScheduledColonyPenaltyTick ScheduledEventType = "ColonyPenaltyTick"
	ScheduledBorrowedArmyTick  ScheduledEventType = "BorrowedArmyTick"
	ScheduledMessengerArrival  ScheduledEventType = "MessengerArrival"
	ScheduledMessengerReturn   ScheduledEventType = "MessengerReturn"
	ScheduledKharisTick        ScheduledEventType = "KharisTick"
	ScheduledTradeDelivery     ScheduledEventType = "TradeDelivery"
	ScheduledTradeReturn       ScheduledEventType = "TradeReturn"
	ScheduledRespawn           ScheduledEventType = "Respawn"
	ScheduledRecallArrival     ScheduledEventType = "RecallArrival"
	ScheduledLogisticsArrival  ScheduledEventType = "LogisticsArrival"
	// C1 — unit model; handler registered in C2+.
	ScheduledUnitArrival ScheduledEventType = "UnitArrival"
	// C-collapse — city exhausted to ≤100 pop; warband spawned.
	ScheduledCollapseSettlement ScheduledEventType = "CollapseSettlement"
	// W4e — daily grain+silver upkeep for all active units.
	ScheduledUpkeepTick ScheduledEventType = "UpkeepTick"
)

// ScheduledEvent is a pending game event stored durably in PostgreSQL.
type ScheduledEvent struct {
	ID           int64
	WorldID      uuid.UUID
	EventType    ScheduledEventType
	Payload      json.RawMessage
	ProcessAfter time.Time
	ProcessedAt  *time.Time
	FailedAt     *time.Time
	Attempts     int
}

// Scheduler enqueues timed game events.
type Scheduler struct {
	pool  *pgxpool.Pool
	clock clock.Clock
}

// NewScheduler creates a Scheduler backed by the given connection pool.
// clk is used for all due_at calculations — pass a TestClock in tests.
func NewScheduler(pool *pgxpool.Pool, clk clock.Clock) *Scheduler {
	return &Scheduler{pool: pool, clock: clk}
}

// Clock returns the scheduler's clock (used by delivery handlers for chained events).
func (s *Scheduler) Clock() clock.Clock { return s.clock }

// EnqueueAfter schedules an event to fire d duration from now (game time).
func (s *Scheduler) EnqueueAfter(ctx context.Context, worldID uuid.UUID, eventType ScheduledEventType, payload any, d time.Duration) error {
	return s.Enqueue(ctx, worldID, eventType, payload, s.clock.Now().Add(d))
}

// Enqueue schedules a new event to be processed at or after processAfter.
func (s *Scheduler) Enqueue(ctx context.Context, worldID uuid.UUID, eventType ScheduledEventType, payload any, processAfter time.Time) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal scheduled payload: %w", err)
	}
	_, err = s.pool.Exec(ctx,
		`INSERT INTO scheduled_events (world_id, event_type, payload, process_after)
		 VALUES ($1, $2, $3, $4)`,
		worldID, string(eventType), raw, processAfter,
	)
	return err
}

// EnqueueTx schedules a new event within an existing transaction. Use this when
// you need the enqueue to be atomic with other DB work (e.g. trade deductions).
func (s *Scheduler) EnqueueTx(ctx context.Context, tx pgx.Tx, worldID uuid.UUID, eventType ScheduledEventType, payload any, processAfter time.Time) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal scheduled payload: %w", err)
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO scheduled_events (world_id, event_type, payload, process_after)
		 VALUES ($1, $2, $3, $4)`,
		worldID, string(eventType), raw, processAfter,
	)
	return err
}

// EnqueueAfterTx schedules a relative event within an existing transaction.
func (s *Scheduler) EnqueueAfterTx(ctx context.Context, tx pgx.Tx, worldID uuid.UUID, eventType ScheduledEventType, payload any, d time.Duration) error {
	return s.EnqueueTx(ctx, tx, worldID, eventType, payload, s.clock.Now().Add(d))
}

// Handler processes a single scheduled event. It must propagate ctx to all DB calls.
type Handler func(ctx context.Context, e ScheduledEvent) error

// DefaultHandlerTimeout is applied to every handler unless overridden via WithHandlerTimeout.
const DefaultHandlerTimeout = 5 * time.Second

// DeadLetterAttempts is the number of consecutive failures before an event is dead-lettered.
const DeadLetterAttempts = 3

// Worker polls for due events every 10 seconds and dispatches them to registered handlers.
// Each handler is called with a per-handler timeout context (G2). Three consecutive
// timeouts or errors mark the event as dead-lettered for manual inspection.
type Worker struct {
	pool     *pgxpool.Pool
	clock    clock.Clock
	handlers map[ScheduledEventType]Handler
	timeouts map[ScheduledEventType]time.Duration
}

// NewWorker creates a Worker. clk must be the same Clock used by Scheduler.
func NewWorker(pool *pgxpool.Pool, clk clock.Clock) *Worker {
	return &Worker{
		pool:     pool,
		clock:    clk,
		handlers: make(map[ScheduledEventType]Handler),
		timeouts: make(map[ScheduledEventType]time.Duration),
	}
}

// Register binds a handler to a scheduled event type using the default timeout.
func (w *Worker) Register(t ScheduledEventType, h Handler) {
	w.handlers[t] = h
}

// RegisterWithTimeout binds a handler and overrides the per-call context timeout.
func (w *Worker) RegisterWithTimeout(t ScheduledEventType, h Handler, timeout time.Duration) {
	w.handlers[t] = h
	w.timeouts[t] = timeout
}

// Run polls for due events until ctx is cancelled.
func (w *Worker) Run(ctx context.Context) {
	slog.Info("scheduled event worker started")
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("scheduled event worker stopped")
			return
		case <-ticker.C:
			if err := w.processBatch(ctx); err != nil {
				slog.Error("event worker batch failed", "err", err)
			}
		}
	}
}

func (w *Worker) processBatch(ctx context.Context) error {
	now := w.clock.Now()
	rows, err := w.pool.Query(ctx,
		`UPDATE scheduled_events
		 SET attempts = attempts + 1
		 WHERE id IN (
		     SELECT id FROM scheduled_events
		     WHERE processed_at IS NULL
		       AND failed_at IS NULL
		       AND process_after <= $1
		       AND world_id IN (SELECT id FROM worlds WHERE status = 'active')
		     ORDER BY process_after
		     LIMIT 20
		     FOR UPDATE SKIP LOCKED
		 )
		 RETURNING id, world_id, event_type, payload, process_after, processed_at, failed_at, attempts`,
		now,
	)
	if err != nil {
		return fmt.Errorf("claim events: %w", err)
	}
	defer rows.Close()

	var batch []ScheduledEvent
	for rows.Next() {
		var e ScheduledEvent
		if err := rows.Scan(&e.ID, &e.WorldID, &e.EventType, &e.Payload, &e.ProcessAfter, &e.ProcessedAt, &e.FailedAt, &e.Attempts); err != nil {
			return fmt.Errorf("scan scheduled event: %w", err)
		}
		batch = append(batch, e)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, e := range batch {
		w.dispatch(ctx, e)
	}
	return nil
}

func (w *Worker) dispatch(ctx context.Context, e ScheduledEvent) {
	h, ok := w.handlers[e.EventType]
	if !ok {
		slog.Warn("no handler for scheduled event type", "type", e.EventType)
		w.markFailed(ctx, e.ID)
		return
	}

	timeout := DefaultHandlerTimeout
	if t, ok := w.timeouts[e.EventType]; ok {
		timeout = t
	}

	hCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if err := h(hCtx, e); err != nil {
		slog.Error("scheduled event handler failed",
			"type", e.EventType, "id", e.ID, "attempt", e.Attempts, "err", err)
		if e.Attempts >= DeadLetterAttempts {
			slog.Error("dead-lettering event after repeated failures",
				"type", e.EventType, "id", e.ID)
			w.markFailed(ctx, e.ID)
		}
		return
	}
	w.markDone(ctx, e.ID)
}

func (w *Worker) markDone(ctx context.Context, id int64) {
	if _, err := w.pool.Exec(ctx,
		`UPDATE scheduled_events SET processed_at = now() WHERE id = $1`, id,
	); err != nil {
		slog.Error("mark event done failed", "id", id, "err", err)
	}
}

func (w *Worker) markFailed(ctx context.Context, id int64) {
	if _, err := w.pool.Exec(ctx,
		`UPDATE scheduled_events SET failed_at = now() WHERE id = $1`, id,
	); err != nil {
		slog.Error("mark event failed", "id", id, "err", err)
	}
}
