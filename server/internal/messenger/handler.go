// Package messenger implements the messenger delivery and return lifecycle.
package messenger

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/poleia/server/internal/economy"
	"github.com/poleia/server/internal/events"
	"github.com/poleia/server/internal/gossip"
)

// ArrivalPayload is the scheduled event payload for a messenger reaching its destination.
type ArrivalPayload struct {
	MessengerID uuid.UUID `json:"messenger_id"`
}

// ReturnPayload is the scheduled event payload for a messenger returning home.
type ReturnPayload struct {
	MessengerID uuid.UUID `json:"messenger_id"`
}

// ArrivalHandler handles MessengerArrival events.
type ArrivalHandler struct {
	pool      *pgxpool.Pool
	scheduler *events.Scheduler
	store     *events.Store
}

// NewArrivalHandler creates an ArrivalHandler.
func NewArrivalHandler(pool *pgxpool.Pool, sched *events.Scheduler, store *events.Store) *ArrivalHandler {
	return &ArrivalHandler{pool: pool, scheduler: sched, store: store}
}

// Handle marks the messenger as delivered and schedules an auto-return after 48 hours
// in case the recipient never replies.
func (h *ArrivalHandler) Handle(ctx context.Context, e events.ScheduledEvent) error {
	var payload ArrivalPayload
	if err := json.Unmarshal(e.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal messenger arrival: %w", err)
	}

	var status string
	var destinationID uuid.UUID
	err := h.pool.QueryRow(ctx,
		`SELECT status, destination_id FROM messengers WHERE id = $1`,
		payload.MessengerID,
	).Scan(&status, &destinationID)
	if err != nil {
		return nil // deleted or not found — silently skip
	}
	if status != "outbound" {
		return nil
	}

	_, err = h.pool.Exec(ctx,
		`UPDATE messengers SET status = 'delivered' WHERE id = $1`,
		payload.MessengerID,
	)
	if err != nil {
		return fmt.Errorf("mark messenger delivered: %w", err)
	}

	_, _ = h.store.Append(ctx, destinationID, events.StreamProvince, "MessengerArrived",
		map[string]any{"messenger_id": payload.MessengerID}, e.WorldID, nil)

	// Record market snapshot: the sender now knows the destination's prices.
	var senderID uuid.UUID
	if err := h.pool.QueryRow(ctx,
		`SELECT sender_id FROM messengers WHERE id = $1`, payload.MessengerID,
	).Scan(&senderID); err == nil {
		if snapErr := economy.RecordMarketSnapshot(ctx, h.pool, senderID, destinationID); snapErr != nil {
			slog.Error("market snapshot on messenger arrival", "err", snapErr)
		}

		// Gossip mechanism: contact spreads any rumors the destination's owner is
		// carrying (temenos_gossip.md PASS 2b — detailed market knowledge stays
		// firsthand only; see RecordMarketSnapshot above). Best-effort — never fail
		// the arrival over this.
		if err := gossip.PropagateOnContact(ctx, h.pool, senderID, destinationID, e.WorldID); err != nil {
			slog.Error("propagate gossip on messenger arrival", "err", err)
		}
	}

	slog.Info("messenger delivered", "id", payload.MessengerID, "destination", destinationID)

	// Auto-return after 48 ticks (48 game hours) if the recipient does not reply sooner.
	return h.scheduler.EnqueueTick(ctx, e.WorldID, events.ScheduledMessengerReturn,
		ReturnPayload{MessengerID: payload.MessengerID}, e.DueTick+48)
}

// ReturnHandler handles MessengerReturn events.
type ReturnHandler struct {
	pool  *pgxpool.Pool
	store *events.Store
}

// NewReturnHandler creates a ReturnHandler.
func NewReturnHandler(pool *pgxpool.Pool, store *events.Store) *ReturnHandler {
	return &ReturnHandler{pool: pool, store: store}
}

// Handle marks the messenger as arrived (back home) and notifies the origin settlement.
// Idempotent: if the messenger is already 'arrived', does nothing.
func (h *ReturnHandler) Handle(ctx context.Context, e events.ScheduledEvent) error {
	var payload ReturnPayload
	if err := json.Unmarshal(e.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal messenger return: %w", err)
	}

	var status string
	var originID uuid.UUID
	// origin_id is NULL for a host-sent messenger (mig 087); the origin unit is
	// then the stream the MessengerReturned event belongs to.
	err := h.pool.QueryRow(ctx,
		`SELECT status, COALESCE(origin_id, origin_unit_id) FROM messengers WHERE id = $1`,
		payload.MessengerID,
	).Scan(&status, &originID)
	if err != nil {
		return nil
	}
	if status == "arrived" {
		return nil
	}

	_, err = h.pool.Exec(ctx,
		`UPDATE messengers SET status = 'arrived' WHERE id = $1`,
		payload.MessengerID,
	)
	if err != nil {
		return fmt.Errorf("mark messenger arrived: %w", err)
	}

	_, _ = h.store.Append(ctx, originID, events.StreamProvince, "MessengerReturned",
		map[string]any{"messenger_id": payload.MessengerID}, e.WorldID, nil)

	slog.Info("messenger returned home", "id", payload.MessengerID)
	return nil
}
