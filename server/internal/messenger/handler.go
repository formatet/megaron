// Package messenger implements the messenger delivery and return lifecycle.
package messenger

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"formatet/megaron/server/internal/combat"
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
	ReplyStayTicks   = 48  // 48 ticks
	OfferExpiryTicks = 168 // 168 ticks
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
	// hub is combat.Broadcaster, reused rather than a new interface — the same
	// consumer interface march_recall.go and order_delivery.go already take
	// (G1: messenger sits above combat, and neither imports notify).
	hub combat.Broadcaster
}

// NewArrivalHandler creates an ArrivalHandler.
func NewArrivalHandler(pool *pgxpool.Pool, sched *events.Scheduler, store *events.Store, hub combat.Broadcaster) *ArrivalHandler {
	return &ArrivalHandler{pool: pool, scheduler: sched, store: store, hub: hub}
}

// Handle marks the messenger as delivered and schedules an auto-return after 48 hours
// in case the recipient never replies.
//
// The flip (outbound→delivered) and the return-scheduling live in ONE
// transaction (megaron_plan_budbararens_ankomst_tx.md): a crash between the
// two used to leave status='delivered' committed with no
// ScheduledMessengerReturn ever enqueued — a permanently stranded messenger
// that a retry silently no-ops on (status != "outbound" trips the replay
// guard and returns nil). The row is locked FOR UPDATE before the status
// check, closing the same race the package's other five claim-sites already
// guard against (order_delivery.go, march_recall.go, recall.go ×2).
func (h *ArrivalHandler) Handle(ctx context.Context, e events.ScheduledEvent) error {
	var payload ArrivalPayload
	if err := json.Unmarshal(e.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal messenger arrival: %w", err)
	}

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin messenger arrival: %w", err)
	}
	defer tx.Rollback(ctx)

	var status string
	var destinationID, senderID uuid.UUID
	var carriesOffer bool
	err = tx.QueryRow(ctx,
		`SELECT status, destination_id, sender_id, trade_offer IS NOT NULL
		   FROM messengers WHERE id = $1 FOR UPDATE`,
		payload.MessengerID,
	).Scan(&status, &destinationID, &senderID, &carriesOffer)
	if err != nil {
		return nil // deleted or not found — silently skip
	}
	if status != "outbound" {
		return nil // idempotent replay: already delivered (or further along)
	}

	if _, err := tx.Exec(ctx,
		`UPDATE messengers SET status = 'delivered' WHERE id = $1 AND status = 'outbound'`,
		payload.MessengerID,
	); err != nil {
		return fmt.Errorf("mark messenger delivered: %w", err)
	}

	// Gossip mechanism: contact spreads any rumors the destination's owner is
	// carrying (temenos_gossip.md PASS 2b — detailed market knowledge stays
	// firsthand only; see the market snapshot below). Best-effort — never fail
	// the arrival over this. Runs in the SAME tx (gossip.Tx accepts pgx.Tx).
	if err := gossip.PropagateOnContact(ctx, tx, senderID, destinationID, e.WorldID); err != nil {
		slog.Error("propagate gossip on messenger arrival", "err", err)
	}

	// Auto-return once the stay is up, if the recipient does not reply sooner.
	// An offer-bearing messenger stays as long as its offer lives — see stayTicks.
	// This MUST commit atomically with the flip above — see the doc comment.
	if err := h.scheduler.EnqueueTickTx(ctx, tx, e.WorldID, events.ScheduledMessengerReturn,
		ReturnPayload{MessengerID: payload.MessengerID}, e.DueTick+stayTicks(carriesOffer),
	); err != nil {
		return fmt.Errorf("schedule messenger return: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit messenger arrival: %w", err)
	}

	// Best-effort, after commit — events.Store holds a *pgxpool.Pool (no AppendTx,
	// greppverifierat 2026-09-01) and RecordMarketSnapshot takes a *pgxpool.Pool
	// (called from several other sites; widening its signature is its own,
	// larger change — non-scope). Both are revision/notice, not mechanics: losing
	// either to a post-commit crash costs the player nothing structural.
	_, _ = h.store.Append(ctx, destinationID, events.StreamProvince, "MessengerArrived",
		map[string]any{"messenger_id": payload.MessengerID}, e.WorldID, nil)
	if snapErr := economy.RecordMarketSnapshot(ctx, h.pool, senderID, destinationID); snapErr != nil {
		slog.Error("market snapshot on messenger arrival", "err", snapErr)
	}

	h.notifyDelivered(ctx, e.WorldID, payload.MessengerID)

	slog.Info("messenger delivered", "id", payload.MessengerID, "destination", destinationID)
	return nil
}

// notifyDelivered tells the recipient a messenger is standing in their court.
//
// This is the load-bearing async channel — the taxonomy puts "budbärare
// levererad till dig: läs + svara" at level 2 — and until now it was the only
// channel in the game that said nothing at all: neither handler here held a
// hub, so a delivered messenger produced no dispatch, no archive row and no
// badge. You learned about it by happening to open the diplomacy drawer.
//
// Runs after commit and best-effort, like the audit event and market snapshot
// above it: the claim (FOR UPDATE + status guard) has already made this run
// exactly once per delivery, so the risk here is loss on a crash, never a
// duplicate notice.
func (h *ArrivalHandler) notifyDelivered(ctx context.Context, worldID, messengerID uuid.UUID) {
	if h.hub == nil {
		return
	}
	var recipientID, senderID uuid.UUID
	var destName, senderName, messageText string
	var originName *string
	var q, r int
	var offer []byte
	if err := h.pool.QueryRow(ctx,
		`SELECT s.owner_id, m.sender_id, s.name, pr.map_q, pr.map_r,
		        COALESCE(pl.wanax_name, pl.username), m.message_text,
		        o.name, m.trade_offer
		   FROM messengers m
		   JOIN settlements s  ON s.id  = m.destination_id
		   JOIN provinces   pr ON pr.id = s.province_id
		   JOIN players     pl ON pl.id = m.sender_id
		   LEFT JOIN settlements o ON o.id = m.origin_id
		  WHERE m.id = $1`,
		messengerID,
	).Scan(&recipientID, &senderID, &destName, &q, &r, &senderName, &messageText, &originName, &offer); err != nil {
		slog.Warn("messenger arrival notice lookup", "messenger", messengerID, "err", err)
		return
	}
	// A messenger you sent to your own city is your own errand, not news
	// (megaron_notifikationer.md: never notify a player about their own act).
	if recipientID == uuid.Nil || recipientID == senderID {
		return
	}

	body := map[string]any{
		"messenger_id": messengerID,
		"name":         destName, // the city the messenger reached — yours
		"q":            q,        // "⌖ Take me there" goes to that city
		"r":            r,
		"from":         senderName,
		"message":      messageText,
	}
	if originName != nil {
		body["from_settlement"] = *originName
	}
	// An offer-bearing messenger is a decision with a clock on it: the offer can
	// only be accepted while its bearer stands there (stayTicks). Level 2, and
	// the terms ride along so the notice states the actual bargain rather than
	// "a messenger arrived".
	level := 3
	if len(offer) > 0 {
		var terms map[string]any
		if err := json.Unmarshal(offer, &terms); err == nil {
			body["offer"] = terms
			level = 2
		}
	}
	_ = h.hub.NotifyPlayer(ctx, worldID, recipientID, "MessengerArrival", level, body)
}

// ReturnHandler handles MessengerReturn events.
type ReturnHandler struct {
	pool  *pgxpool.Pool
	store *events.Store
	hub   combat.Broadcaster
}

// NewReturnHandler creates a ReturnHandler.
func NewReturnHandler(pool *pgxpool.Pool, store *events.Store, hub combat.Broadcaster) *ReturnHandler {
	return &ReturnHandler{pool: pool, store: store, hub: hub}
}

// Handle marks the messenger as arrived (back home) and notifies the origin settlement.
// Idempotent: if the messenger is already 'arrived', does nothing.
//
// Shares its root with ArrivalHandler.Handle (megaron_plan_budbararens_ankomst_tx.md)
// but not its crash window: nothing is scheduled after this flip, so a plain
// atomic claim (FOR UPDATE + conditional UPDATE) is enough — no transaction
// needed. A crash here loses at most the MessengerReturned event, not a
// stranded unit.
func (h *ReturnHandler) Handle(ctx context.Context, e events.ScheduledEvent) error {
	var payload ReturnPayload
	if err := json.Unmarshal(e.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal messenger return: %w", err)
	}

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin messenger return: %w", err)
	}
	defer tx.Rollback(ctx)

	var status string
	var originID uuid.UUID
	// origin_id is NULL for a host-sent messenger (mig 087); the origin unit is
	// then the stream the MessengerReturned event belongs to.
	err = tx.QueryRow(ctx,
		`SELECT status, COALESCE(origin_id, origin_unit_id) FROM messengers WHERE id = $1 FOR UPDATE`,
		payload.MessengerID,
	).Scan(&status, &originID)
	if err != nil {
		return nil
	}
	if status == "arrived" {
		return nil // idempotent replay
	}

	if _, err := tx.Exec(ctx,
		`UPDATE messengers SET status = 'arrived' WHERE id = $1 AND status != 'arrived'`,
		payload.MessengerID,
	); err != nil {
		return fmt.Errorf("mark messenger arrived: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit messenger return: %w", err)
	}

	_, _ = h.store.Append(ctx, originID, events.StreamProvince, "MessengerReturned",
		map[string]any{"messenger_id": payload.MessengerID}, e.WorldID, nil)

	h.notifyReturned(ctx, e.WorldID, payload.MessengerID)

	slog.Info("messenger returned home", "id", payload.MessengerID)
	return nil
}

// notifyReturned tells the sender their messenger is home — and whether it
// brought an answer. The reply arrives on the RETURN, never in place
// (CLAUDE.md: messengers are physical; command is never instant), so this is
// the moment the conversation actually closes for the sender. It was silent
// too: a Wanax who sent a messenger and logged out had no way to learn the
// answer had come back short of opening the diplomacy drawer and reading rows.
//
// Same best-effort, post-claim placement as notifyDelivered — see there.
func (h *ReturnHandler) notifyReturned(ctx context.Context, worldID, messengerID uuid.UUID) {
	if h.hub == nil {
		return
	}
	var senderID uuid.UUID
	var reply *string
	var originName, destName *string
	var q, r *int
	if err := h.pool.QueryRow(ctx,
		`SELECT m.sender_id, m.reply_text, o.name, d.name, opr.map_q, opr.map_r
		   FROM messengers m
		   LEFT JOIN settlements o   ON o.id   = m.origin_id
		   LEFT JOIN provinces   opr ON opr.id = o.province_id
		   LEFT JOIN settlements d   ON d.id   = m.destination_id
		  WHERE m.id = $1`,
		messengerID,
	).Scan(&senderID, &reply, &originName, &destName, &q, &r); err != nil {
		slog.Warn("messenger return notice lookup", "messenger", messengerID, "err", err)
		return
	}
	if senderID == uuid.Nil {
		return
	}

	body := map[string]any{"messenger_id": messengerID}
	if originName != nil {
		body["name"] = *originName
	}
	if destName != nil {
		body["to"] = *destName
	}
	// A host-sent messenger has no origin settlement (origin_id NULL, mig 087),
	// so it has no home city to jump to. Leave q/r out rather than inventing a
	// destination — the button disables itself, which is honest, and inventing
	// the capital here is exactly the "tyst fallback som gissar" the working
	// method forbids.
	if q != nil && r != nil {
		body["q"], body["r"] = *q, *r
	}
	// Level 2 when an answer came back — that is a thing to read and act on.
	// A messenger returning unanswered is information, not a decision.
	level := 3
	if reply != nil && *reply != "" {
		body["replied"] = true
		body["reply"] = *reply
		level = 2
	}
	_ = h.hub.NotifyPlayer(ctx, worldID, senderID, "MessengerReturned", level, body)
}
