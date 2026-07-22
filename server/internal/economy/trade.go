package economy

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"math/rand"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/poleia/server/internal/events"
	"github.com/poleia/server/internal/gossip"
)

// OfferExpiryHandler refunds escrowed silver to the buyer when a trade offer
// expires without being accepted or declined.
type OfferExpiryHandler struct {
	pool      *pgxpool.Pool
	scheduler *events.Scheduler
	hub       Broadcaster // nil-guarded; carries OfferExpired to the offer's originator
}

// NewOfferExpiryHandler creates an OfferExpiryHandler.
func NewOfferExpiryHandler(pool *pgxpool.Pool, sched *events.Scheduler, hub Broadcaster) *OfferExpiryHandler {
	return &OfferExpiryHandler{pool: pool, scheduler: sched, hub: hub}
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

	// Read origin_id, destination_id, kind, and escrowed value from the now-expired messenger.
	var originID, destID uuid.UUID
	var kind, offerGood, wantGood string
	var offerSilver, offerQty, wantQty, wantSilver float64
	if err := tx.QueryRow(ctx,
		`SELECT origin_id, destination_id,
		        COALESCE(trade_offer->>'kind', 'buy'),
		        COALESCE((trade_offer->>'offer_silver')::float, 0),
		        COALESCE(trade_offer->>'offer_good', ''),
		        COALESCE((trade_offer->>'offer_qty')::float, 0),
		        COALESCE(trade_offer->>'want_good', ''),
		        COALESCE((trade_offer->>'want_qty')::float, 0),
		        COALESCE((trade_offer->>'want_silver')::float, 0)
		 FROM messengers WHERE id=$1`,
		messengerID,
	).Scan(&originID, &destID, &kind, &offerSilver, &offerGood, &offerQty,
		&wantGood, &wantQty, &wantSilver); err != nil {
		return fmt.Errorf("read expired messenger: %w", err)
	}

	// Refund escrowed value to origin:
	//   buy  → silver to buyer (origin)
	//   sell → goods to seller (origin)
	if kind == "sell" {
		if _, err = tx.Exec(ctx,
			`UPDATE settlement_goods
			    SET amount    = settled(amount, rate, calc_tick) + $1,
			        calc_tick = current_world_tick()
			  WHERE settlement_id=$2 AND good_key=$3`,
			offerQty, originID, offerGood,
		); err != nil {
			return fmt.Errorf("refund goods on expiry: %w", err)
		}
		if err = tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit expiry refund: %w", err)
		}
		slog.Info("trade offer expired, goods refunded", "messenger", messengerID, "good", offerGood, "qty", offerQty)
	} else {
		if _, err = tx.Exec(ctx,
			`UPDATE settlement_goods
			    SET amount    = settled(amount, rate, calc_tick) + $1,
			        calc_tick = current_world_tick()
			  WHERE settlement_id=$2 AND good_key='silver'`,
			offerSilver, originID,
		); err != nil {
			return fmt.Errorf("refund silver on expiry: %w", err)
		}
		if err = tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit expiry refund: %w", err)
		}
		slog.Info("trade offer expired, silver refunded", "messenger", messengerID, "silver", offerSilver)
	}

	// OfferExpired — the offer's originator gets an immediate decision notice
	// instead of only learning via the delayed TradeReturn on escrow arrival.
	// Fires only on the real transition above (RowsAffected>0 guarded the early
	// return), never on the idempotent no-op.
	// trade_offer fields are kind-dependent (buy: want_*/offer_silver · sell:
	// offer_*/want_silver) — pick per kind, mirroring OfferAccepted.
	if h.hub != nil {
		notifGood, notifQty, notifSilver := wantGood, wantQty, offerSilver
		if kind == "sell" {
			notifGood, notifQty, notifSilver = offerGood, offerQty, wantSilver
		}
		var ownerID uuid.UUID
		_ = h.pool.QueryRow(ctx, `SELECT owner_id FROM settlements WHERE id = $1`, originID).Scan(&ownerID)
		_ = h.hub.NotifyPlayer(ctx, e.WorldID, ownerID, "OfferExpired", 3, map[string]any{
			"messenger_id":    messengerID,
			"settlement_id":   originID,
			"counterparty_id": destID,
			"kind":            kind,
			"good_key":        notifGood,
			"quantity":        notifQty,
			"silver":          notifSilver,
			"resolution":      "expired",
		})
	}
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
		TransportID       uuid.UUID       `json:"transport_id"`       // physical caravan for this leg (0 = legacy event)
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

	// Physical-caravan interception veto: if this leg's transport was intercepted or
	// lost en route (Del 3-fas-4), cancel delivery — the goods were seized, not
	// delivered. FOR UPDATE so a concurrent interception can't race the credit.
	if p.TransportID != (uuid.UUID{}) {
		var tstatus string
		if err := tx.QueryRow(ctx,
			`SELECT status FROM transports WHERE id = $1 FOR UPDATE`, p.TransportID,
		).Scan(&tstatus); err == nil && tstatus != "in_transit" {
			if hasRoute {
				_, _ = tx.Exec(ctx, `UPDATE trade_routes SET resolved = true WHERE id = $1`, p.TradeRouteID)
			}
			return tx.Commit(ctx)
		}
	}

	// Trade risk: 5% chance caravan is lost to storm or pirates.
	if rand.Float64() < tradeRiskPct {
		reason := tradeLostReasons[rand.Intn(len(tradeLostReasons))]
		if _, err = tx.Exec(ctx, `UPDATE trade_routes SET resolved = true WHERE id = $1`, p.TradeRouteID); err != nil {
			return fmt.Errorf("mark lost route resolved: %w", err)
		}
		if p.TransportID != (uuid.UUID{}) {
			_, _ = tx.Exec(ctx, `UPDATE transports SET status = 'lost', updated_at = now() WHERE id = $1`, p.TransportID)
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

	// Temple tithe (Timothy 2026-07-22, vägval c): when the silver leg of a sale
	// of a RELIGIOUSLY CODED good lands, the priests take their tenth before the
	// seller sees it. Only where a temple stands — no priests, no collection.
	// The counterpart good rides in ThenReturn (this is the silver leg; the goods
	// travel back separately), so the pair is known here without another lookup.
	credited := delivered
	if p.GoodKey == "silver" && len(p.ThenReturn) > 0 {
		var counterpart struct {
			GoodKey string `json:"good_key"`
		}
		if json.Unmarshal(p.ThenReturn, &counterpart) == nil && counterpart.GoodKey != "" {
			var religious, hasTemple bool
			_ = tx.QueryRow(ctx,
				`SELECT COALESCE((SELECT g.religious FROM goods g WHERE g.key = $1), false),
				        EXISTS (SELECT 1 FROM buildings b
				                WHERE b.settlement_id = $2 AND b.building_type = 'temple')`,
				counterpart.GoodKey, p.DestinationID,
			).Scan(&religious, &hasTemple)

			if toTemple, toSeller := Tithe(delivered, religious, hasTemple); toTemple > 0 {
				credited = toSeller
				slog.Info("temple tithe", "settlement", p.DestinationID, "good", counterpart.GoodKey,
					"silver", delivered, "tithe", toTemple)
				if h.eventStore != nil {
					_, _ = h.eventStore.Append(ctx, p.DestinationID, events.StreamProvince, "TempleTithe",
						map[string]any{
							"good_key":     counterpart.GoodKey,
							"trade_silver": delivered,
							"tithe":        toTemple,
							"to_seller":    toSeller,
						}, e.WorldID, nil)
				}
			}
		}
	}

	// Credit goods to destination — silver is now a normal good in settlement_goods.
	if _, err = tx.Exec(ctx,
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
		 VALUES ($1, $2, $3, 0, 100, current_world_tick())
		 ON CONFLICT (settlement_id, good_key) DO UPDATE SET
		     amount = LEAST(
		         settled(settlement_goods.amount, settlement_goods.rate, settlement_goods.calc_tick)
		             + $3,
		         settlement_goods.cap),
		     calc_tick = current_world_tick()`,
		p.DestinationID, p.GoodKey, credited,
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

	// Leg 1's physical caravan has arrived.
	if p.TransportID != (uuid.UUID{}) {
		_, _ = tx.Exec(ctx, `UPDATE transports SET status = 'delivered', updated_at = now() WHERE id = $1`, p.TransportID)
	}

	// Chain: if this was a silver leg, dispatch the goods return now — as its own
	// physical caravan (leg 2), so the return trip is visible and interceptable too.
	if len(p.ThenReturn) > 0 && h.scheduler != nil {
		var ret struct {
			DestinationID string  `json:"destination_id"`
			GoodKey       string  `json:"good_key"`
			Quantity      float64 `json:"quantity"`
			MessengerID   string  `json:"messenger_id"`
			TravelMins    float64 `json:"travel_mins"`
			OwnerID       string  `json:"owner_id"`
			OriginQ       int     `json:"origin_q"`
			OriginR       int     `json:"origin_r"`
			DestQ         int     `json:"dest_q"`
			DestR         int     `json:"dest_r"`
		}
		if jsonErr := json.Unmarshal(p.ThenReturn, &ret); jsonErr == nil && ret.DestinationID != "" {
			var currentTick int
			_ = tx.QueryRow(ctx, `SELECT current_world_tick()`).Scan(&currentTick)
			travelTicks := int(math.Round(ret.TravelMins / 60))
			if travelTicks < 1 {
				travelTicks = 1
			}

			// Build the return caravan (leg 2: this settlement → the buyer/seller origin).
			// Raw SQL: economy may not import the transport package (G1). Best-effort —
			// a missing physical row must never block the goods return itself.
			var leg2ID uuid.UUID
			retOwner, _ := uuid.Parse(ret.OwnerID)
			retDest, _ := uuid.Parse(ret.DestinationID)
			if scanErr := tx.QueryRow(ctx,
				`INSERT INTO transports
				   (world_id, owner_id, kind, origin_id, dest_id, category,
				    origin_q, origin_r, dest_q, dest_r, departs_at, arrives_at, due_tick, interceptable)
				 VALUES ($1,$2,'trade_return',$3,$4,'land',$5,$6,$7,$8,
				         now(), now() + make_interval(mins => $9), $10, true)
				 RETURNING id`,
				e.WorldID, retOwner, p.DestinationID, retDest,
				ret.OriginQ, ret.OriginR, ret.DestQ, ret.DestR, ret.TravelMins, currentTick+travelTicks,
			).Scan(&leg2ID); scanErr == nil {
				_, _ = tx.Exec(ctx,
					`INSERT INTO transport_goods (transport_id, good_key, quantity) VALUES ($1,$2,$3)`,
					leg2ID, ret.GoodKey, ret.Quantity)
			}

			_ = h.scheduler.EnqueueTickTx(ctx, tx, e.WorldID, events.ScheduledTradeReturn,
				map[string]any{
					"destination_id": ret.DestinationID,
					"good_key":       ret.GoodKey,
					"quantity":       ret.Quantity,
					"messenger_id":   ret.MessengerID,
					"transport_id":   leg2ID.String(),
				}, currentTick+travelTicks)
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

	// Rumor: a completed delivery is minor news — witnessed only by nearby owners
	// (temenos_gossip.md PASS 2b). Subject = the origin (the settlement whose
	// surplus this good reveals), hint = the good, so it registers as
	// rumour-known for anyone who hears of it without having seen it.
	// Best-effort — never fail the delivery over gossip.
	if originSettlementID != uuid.Nil {
		var originName, destName string
		_ = h.pool.QueryRow(ctx, `SELECT name FROM settlements WHERE id = $1`, originSettlementID).Scan(&originName)
		_ = h.pool.QueryRow(ctx, `SELECT name FROM settlements WHERE id = $1`, p.DestinationID).Scan(&destName)
		if originName != "" && destName != "" {
			if err := gossip.Broadcast(ctx, h.pool, e.WorldID, originSettlementID, "economy",
				fmt.Sprintf("%s flows from %s to %s.", p.GoodKey, originName, destName), 6,
				gossip.ImportanceMinor, originSettlementID, p.GoodKey); err != nil {
				slog.Error("trade delivery: broadcast gossip", "err", err)
			}
		}
	}

	slog.Info("trade delivery", "destination", p.DestinationID, "good", p.GoodKey, "qty", delivered)
	return nil
}
