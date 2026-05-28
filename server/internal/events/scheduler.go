package events

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ScheduledEventType identifies what should happen when the event fires.
type ScheduledEventType string

const (
	ScheduledArmyArrival      ScheduledEventType = "ArmyArrival"
	ScheduledBuildComplete    ScheduledEventType = "BuildComplete"
	ScheduledTrainComplete    ScheduledEventType = "TrainComplete"
	ScheduledCollapseCheck    ScheduledEventType = "CollapseCheck"
	ScheduledDivineRoll       ScheduledEventType = "DivineRoll"
	ScheduledLoyaltyDecayTick  ScheduledEventType = "LoyaltyDecayTick"
	ScheduledColonyPenaltyTick  ScheduledEventType = "ColonyPenaltyTick"
	ScheduledBorrowedArmyTick   ScheduledEventType = "BorrowedArmyTick"
	ScheduledMessengerArrival   ScheduledEventType = "MessengerArrival"
	ScheduledMessengerReturn    ScheduledEventType = "MessengerReturn"
	ScheduledKharisTick         ScheduledEventType = "KharisTick"
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

// Scheduler enqueues and dequeues timed game events.
type Scheduler struct {
	pool *pgxpool.Pool
}

// NewScheduler creates a Scheduler backed by the given connection pool.
func NewScheduler(pool *pgxpool.Pool) *Scheduler {
	return &Scheduler{pool: pool}
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

// Handler processes a single scheduled event payload.
type Handler func(ctx context.Context, e ScheduledEvent) error

// Worker polls for due events every 10 seconds and dispatches them to registered handlers.
type Worker struct {
	pool     *pgxpool.Pool
	handlers map[ScheduledEventType]Handler
}

// NewWorker creates a Worker with the given connection pool.
func NewWorker(pool *pgxpool.Pool) *Worker {
	return &Worker{
		pool:     pool,
		handlers: make(map[ScheduledEventType]Handler),
	}
}

// Register binds a handler to a scheduled event type.
func (w *Worker) Register(t ScheduledEventType, h Handler) {
	w.handlers[t] = h
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
	// Claim up to 20 due events atomically.
	rows, err := w.pool.Query(ctx,
		`UPDATE scheduled_events
		 SET attempts = attempts + 1
		 WHERE id IN (
		     SELECT id FROM scheduled_events
		     WHERE processed_at IS NULL
		       AND failed_at IS NULL
		       AND process_after <= now()
		     ORDER BY process_after
		     LIMIT 20
		     FOR UPDATE SKIP LOCKED
		 )
		 RETURNING id, world_id, event_type, payload, process_after, processed_at, failed_at, attempts`,
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

	if err := h(ctx, e); err != nil {
		slog.Error("scheduled event handler failed", "type", e.EventType, "id", e.ID, "err", err)
		if e.Attempts >= 3 {
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
