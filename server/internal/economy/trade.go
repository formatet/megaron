package economy

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/poleia/server/internal/events"
	"github.com/poleia/server/internal/timescale"
)

// OfferExpiryHandler refunds escrowed silver to the buyer when a trade offer
// expires without being accepted or declined.
type OfferExpiryHandler struct {
	pool      *pgxpool.Pool
	scheduler *events.Scheduler
}

// NewOfferExpiryHandler creates an OfferExpiryHandler.
func NewOfferExpiryHandler(pool *pgxpool.Pool, sched *events.Scheduler) *OfferExpiryHandler {
	return &OfferExpiryHandler{pool: pool, scheduler: sched}
}

// Handle processes a ScheduledOfferExpiry event. Idempotent: does nothing if the
// offer is no longer pending (already accepted, declined, or previously expired).
func (h *OfferExpiryHandler) Handle(ctx context.Context, e events.ScheduledEvent) error {
	var p struct {
		MessengerID string `json:"messenger_id"`
	}
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return fmt.Errorf("unmarshal offer expiry: %w", err)
	}
	messengerID, err := uuid.Parse(p.MessengerID)
	if err != nil {
		return fmt.Errorf("parse messenger_id: %w", err)
	}

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Guarded flip: only act if the offer is still pending.
	tag, err := tx.Exec(ctx,
		`UPDATE messengers SET trade_offer = trade_offer || '{"status":"expired"}'
		  WHERE id=$1 AND trade_offer->>'status'='pending'`,
		messengerID,
	)
	if err != nil {
		return fmt.Errorf("expire offer: %w", err)
	}
	if tag.RowsAffected() == 0 {
		// Already resolved (accepted/declined/expired) — idempotent no-op.
		return tx.Commit(ctx)
	}

	// Read origin_id and offer_silver from the now-expired messenger.
	var originID uuid.UUID
	var offerSilver float64
	if err := tx.QueryRow(ctx,
		`SELECT origin_id, (trade_offer->>'offer_silver')::float FROM messengers WHERE id=$1`,
		messengerID,
	).Scan(&originID, &offerSilver); err != nil {
		return fmt.Errorf("read expired messenger: %w", err)
	}

	// Refund escrowed silver to buyer (origin settlement).
	if _, err = tx.Exec(ctx,
		`UPDATE settlement_goods
		    SET amount  = settled(amount, rate, calc_at) + $1,
		        calc_at = now()
		  WHERE settlement_id=$2 AND good_key='silver'`,
		offerSilver, originID,
	); err != nil {
		return fmt.Errorf("refund silver on expiry: %w", err)
	}

	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit expiry refund: %w", err)
	}

	slog.Info("trade offer expired, silver refunded", "messenger", messengerID, "silver", offerSilver)
	return nil
}

const tradeRiskPct = 0.05 // 5% chance a caravan is lost to storm or pirates

var tradeLostReasons = []string{"storm", "pirates", "pirates", "storm", "bandits"}

// DeliveryHandler processes ScheduledTradeDelivery events.
type DeliveryHandler struct {
	pool       *pgxpool.Pool
	eventStore *events.Store
	hub        Broadcaster
	scheduler  *events.Scheduler
}

// NewDeliveryHandler creates a DeliveryHandler.
func NewDeliveryHandler(pool *pgxpool.Pool, eventStore *events.Store, hub Broadcaster, sched *events.Scheduler) *DeliveryHandler {
	return &DeliveryHandler{pool: pool, eventStore: eventStore, hub: hub, scheduler: sched}
}

// Handle delivers goods to the destination settlement.
func (h *DeliveryHandler) Handle(ctx context.Context, e events.ScheduledEvent) error {
	var p struct {
		TradeRouteID      uuid.UUID       `json:"trade_route_id"`
		DestinationID     uuid.UUID       `json:"destination_id"`
		GoodKey           string          `json:"good_key"`
		Quantity          float64         `json:"quantity"`
		DeliveredQuantity float64         `json:"delivered_quantity"` // includes distance bonus
		ThenReturn        json.RawMessage `json:"then_return,omitempty"`
	}
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return fmt.Errorf("unmarshal trade delivery: %w", err)
	}
	// Backward-compat: old events without delivered_quantity use raw quantity.
	delivered := p.DeliveredQuantity
	if delivered <= 0 {
		delivered = p.Quantity
	}

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Exactly-once claim: the worker marks the event done in a separate statement
	// after this tx commits, so a crash in between would re-run this handler.
	// trade_routes.resolved guards route-based legs, but messenger-trade legs
	// (zero-UUID trade_route_id) have no route row — without this marker a retry
	// would double-credit silver and double-schedule the goods return.
	ct, err := tx.Exec(ctx,
		`INSERT INTO processed_deliveries (event_id) VALUES ($1) ON CONFLICT DO NOTHING`,
		e.ID,
	)
	if err != nil {
		return fmt.Errorf("claim delivery: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return nil // already processed in an earlier run
	}

	// Idempotency: only check trade_routes for route-based deliveries (zero UUID = direct silver leg).
	hasRoute := p.TradeRouteID != (uuid.UUID{})
	if hasRoute {
		var resolved bool
		if err := tx.QueryRow(ctx,
			`SELECT resolved FROM trade_routes WHERE id = $1 FOR UPDATE`,
			p.TradeRouteID,
		).Scan(&resolved); err != nil {
			return nil // Route gone or already cleaned up.
		}
		if resolved {
			return nil
		}
	}

	// Trade risk: 5% chance caravan is lost to storm or pirates.
	if rand.Float64() < tradeRiskPct {
		reason := tradeLostReasons[rand.Intn(len(tradeLostReasons))]
		if _, err = tx.Exec(ctx, `UPDATE trade_routes SET resolved = true WHERE id = $1`, p.TradeRouteID); err != nil {
			return fmt.Errorf("mark lost route resolved: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit loss: %w", err)
		}
		_, _ = h.eventStore.Append(ctx, p.DestinationID, events.StreamProvince, "TradeLost",
			map[string]any{"good_key": p.GoodKey, "quantity": p.Quantity, "reason": reason, "route_id": p.TradeRouteID},
			e.WorldID, nil, // e.ID is a scheduled_events id, not an events(id) — would break events_causation_fkey.
		)
		if h.hub != nil {
			var ownerID uuid.UUID
			_ = h.pool.QueryRow(ctx, `SELECT owner_id FROM settlements WHERE id = $1`, p.DestinationID).Scan(&ownerID)
			_ = h.hub.NotifyPlayer(ctx, e.WorldID, ownerID, "TradeLost", 3, map[string]any{
				"destination_id": p.DestinationID,
				"good_key":       p.GoodKey,
				"quantity":       p.Quantity,
				"reason":         reason,
			})
		}
		slog.Info("trade lost", "route", p.TradeRouteID, "good", p.GoodKey, "reason", reason)
		return nil
	}

	// Credit goods to destination — silver is now a normal good in settlement_goods.
	if _, err = tx.Exec(ctx,
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_at)
		 VALUES ($1, $2, $3, 0, 100, now())
		 ON CONFLICT (settlement_id, good_key) DO UPDATE SET
		     amount = LEAST(
		         settled(settlement_goods.amount, settlement_goods.rate, settlement_goods.calc_at)
		             + $3,
		         settlement_goods.cap),
		     calc_at = now()`,
		p.DestinationID, p.GoodKey, delivered,
	); err != nil {
		return fmt.Errorf("credit goods: %w", err)
	}

	if hasRoute {
		if _, err = tx.Exec(ctx,
			`UPDATE trade_routes SET resolved = true WHERE id = $1`,
			p.TradeRouteID,
		); err != nil {
			return fmt.Errorf("mark resolved: %w", err)
		}
	}

	// Chain: if this was a silver leg, dispatch the goods return now.
	if len(p.ThenReturn) > 0 && h.scheduler != nil {
		var ret struct {
			DestinationID string  `json:"destination_id"`
			GoodKey       string  `json:"good_key"`
			Quantity      float64 `json:"quantity"`
			MessengerID   string  `json:"messenger_id"`
			TravelMins    float64 `json:"travel_mins"`
		}
		if jsonErr := json.Unmarshal(p.ThenReturn, &ret); jsonErr == nil && ret.DestinationID != "" {
			// Apply the global time-compression factor — the silver leg and the
			// offer trip both go through timescale.Apply, but this chained goods
			// leg didn't, so under TIME_SCALE>1 the copper crawled in at full
			// real-time speed while everything else was compressed (goods never
			// seemed to arrive). dist*30min == TradeTravelDuration's pre-scale value.
			returnAt := h.scheduler.Clock().Now().Add(
				timescale.Apply(time.Duration(ret.TravelMins * float64(time.Minute))))
			_ = h.scheduler.EnqueueTx(ctx, tx, e.WorldID, events.ScheduledTradeReturn,
				map[string]any{
					"destination_id": ret.DestinationID,
					"good_key":       ret.GoodKey,
					"quantity":       ret.Quantity,
					"messenger_id":   ret.MessengerID,
				}, returnAt)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	// causation is nil: e.ID is a scheduled_events row id, not an events(id), so passing
	// it would violate events_causation_fkey. A timer-driven delivery has no causing event.
	if _, err := h.eventStore.Append(ctx, p.DestinationID, events.StreamProvince, "TradeDelivery",
		map[string]any{"good_key": p.GoodKey, "quantity": delivered, "route_id": p.TradeRouteID},
		e.WorldID, nil,
	); err != nil {
		slog.Error("record TradeDelivery event", "err", err)
	}

	if h.hub != nil {
		var ownerID uuid.UUID
		_ = h.pool.QueryRow(ctx, `SELECT owner_id FROM settlements WHERE id = $1`, p.DestinationID).Scan(&ownerID)
		_ = h.hub.NotifyPlayer(ctx, e.WorldID, ownerID, "TradeDelivery", 3, map[string]any{
			"destination_id": p.DestinationID,
			"good_key":       p.GoodKey,
			"quantity":       delivered,
		})
	}

	// Record market snapshot: the caravan owner now knows the destination's prices.
	// (Fix: origin_id is the settlement UUID, not owner_id — owner_id doesn't exist in trade_routes)
	var originSettlementID uuid.UUID
	if err := h.pool.QueryRow(ctx,
		`SELECT origin_id FROM trade_routes WHERE id = $1`, p.TradeRouteID,
	).Scan(&originSettlementID); err == nil {
		var ownerID uuid.UUID
		if err := h.pool.QueryRow(ctx,
			`SELECT owner_id FROM settlements WHERE id = $1`, originSettlementID,
		).Scan(&ownerID); err == nil {
			if snapErr := RecordMarketSnapshot(ctx, h.pool, ownerID, p.DestinationID); snapErr != nil {
				slog.Error("market snapshot on delivery", "err", snapErr)
			}
		}
	}

	slog.Info("trade delivery", "destination", p.DestinationID, "good", p.GoodKey, "qty", delivered)
	return nil
}
