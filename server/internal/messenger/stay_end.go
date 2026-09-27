package messenger

// StayEndHandler: a delivered messenger's stay ran out with no reply
// (megaron_plan_ordna_passage.md, slice 3b-2). It turns around and walks
// itself home, exactly the journey a spoken Reply starts — see
// StartReturnLeg (return_leg.go), which both share.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// StayEndPayload is the ScheduledMessengerStayEnd payload.
type StayEndPayload struct {
	MessengerID uuid.UUID `json:"messenger_id"`
}

// StayEndHandler handles ScheduledMessengerStayEnd events.
type StayEndHandler struct {
	pool      *pgxpool.Pool
	scheduler *events.Scheduler
	clk       clock.Clock
}

// NewStayEndHandler creates a StayEndHandler.
func NewStayEndHandler(pool *pgxpool.Pool, sched *events.Scheduler, clk clock.Clock) *StayEndHandler {
	return &StayEndHandler{pool: pool, scheduler: sched, clk: clk}
}

// Handle starts the return leg if the messenger is still 'delivered' —
// nobody replied during the stay. A no-op if a reply already turned it
// around, or a racing replay of this same event already did
// (StartReturnLeg's own FOR UPDATE claim makes this exactly-once).
func (h *StayEndHandler) Handle(ctx context.Context, e events.ScheduledEvent) error {
	var payload StayEndPayload
	if err := json.Unmarshal(e.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal messenger stay end: %w", err)
	}

	result, err := StartReturnLeg(ctx, h.pool, h.scheduler, e.WorldID, payload.MessengerID, h.clk.Now(), e.DueTick, nil)
	if err != nil {
		return fmt.Errorf("messenger stay end: %w", err)
	}
	if !result.Started {
		return nil // already replied (or an earlier replay of this event) — no-op
	}

	slog.Info("messenger stay ended — walking home unanswered",
		"id", payload.MessengerID, "returns_at", result.ReturnsAt, "passage_awaiting", result.PassageAwaiting)
	return nil
}
