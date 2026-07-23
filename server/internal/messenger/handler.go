// Package messenger implements the messenger delivery and return lifecycle.
package messenger

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/gossip"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ArrivalPayload is the scheduled event payload for a messenger reaching its destination.
type ArrivalPayload struct {
	MessengerID uuid.UUID `json:"messenger_id"`
}

// ReturnPayload is the scheduled event payload for a messenger returning home.
type ReturnPayload struct {
	MessengerID uuid.UUID `json:"messenger_id"`
}

// How long a messenger waits at its destination before heading home unanswered.
//
// A plain message waits ReplyStayTicks. A messenger carrying a trade offer waits
// OfferExpiryTicks — the same number the offer's expiry is scheduled on, because
// the two are one thing: the offer can only be read or accepted while its bearer
// is standing there (the inbox and both trade-accept paths require
// status='delivered'). When the bearer left after 48 ticks while the offer stayed
// pending for 168, the offer spent 120 ticks visible to nobody and acceptable by
// nobody, with the sender's escrow locked the whole time.
const (
	ReplyStayTicks   = 48     // 2 in-game days
	OfferExpiryTicks = 7 * 24 // 7 in-game days
)

// stayTicks returns how long a messenger waits at its destination.
func stayTicks(carriesOffer bool) int {
	if carriesOffer {
		return OfferExpiryTicks
	}
	return ReplyStayTicks
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
	var carriesOffer bool
	err := h.pool.QueryRow(ctx,
		`SELECT status, destination_id, trade_offer IS NOT NULL FROM messengers WHERE id = $1`,
		payload.MessengerID,
	).Scan(&status, &destinationID, &carriesOffer)
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

	// Auto-return once the stay is up, if the recipient does not reply sooner.
	// An offer-bearing messenger stays as long as its offer lives — see stayTicks.
	return h.scheduler.EnqueueTick(ctx, e.WorldID, events.ScheduledMessengerReturn,
		ReturnPayload{MessengerID: payload.MessengerID}, e.DueTick+stayTicks(carriesOffer))
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
