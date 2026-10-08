package economy

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"time"

	"formatet/megaron/server/internal/carrier"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/gossip"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/tick"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
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
		).Scan(&tstatus); err != nil {
			return fmt.Errorf("load delivery transport: %w", err)
		}
		if tstatus != "in_transit" {
			if hasRoute {
				if _, err := tx.Exec(ctx, `UPDATE trade_routes SET resolved=true WHERE id=$1`, p.TradeRouteID); err != nil {
					return err
				}
			}
			return tx.Commit(ctx)
		}
	}

	// Temple tithe (Timothy 2026-07-22, vägval c): when the silver leg of a sale
	// of a RELIGIOUSLY CODED good lands, the temple takes its tenth before the
	// seller sees it. Only where a temple stands — no temple, no collection.
	// The counterpart good rides in ThenReturn (this is the silver leg; the goods
	// travel back separately), so the pair is known here without another lookup.
	var titheEvent *events.Event
	credited := delivered
	if p.GoodKey == "silver" && len(p.ThenReturn) > 0 {
		var counterpart struct {
			GoodKey string `json:"good_key"`
		}
		if json.Unmarshal(p.ThenReturn, &counterpart) == nil && counterpart.GoodKey != "" {
			var religious, hasTemple bool
			if err := tx.QueryRow(ctx,
				`SELECT COALESCE((SELECT g.religious FROM goods g WHERE g.key = $1), false),
				        EXISTS (SELECT 1 FROM buildings b
				                WHERE b.settlement_id = $2 AND b.building_type = 'temple')`,
				counterpart.GoodKey, p.DestinationID,
			).Scan(&religious, &hasTemple); err != nil {
				return fmt.Errorf("read tithe: %w", err)
			}

			if toTemple, toSeller := Tithe(delivered, religious, hasTemple); toTemple > 0 {
				credited = toSeller
				slog.Info("temple tithe", "settlement", p.DestinationID, "good", counterpart.GoodKey,
					"silver", delivered, "tithe", toTemple)
				if h.eventStore != nil {
					titheEvent, err = h.eventStore.AppendTx(ctx, tx, p.DestinationID, events.StreamProvince, "TempleTithe",
						map[string]any{
							"good_key":     counterpart.GoodKey,
							"trade_silver": delivered,
							"tithe":        toTemple,
							"to_seller":    toSeller,
						}, e.WorldID, nil)
					if err != nil {
						return fmt.Errorf("record tithe: %w", err)
					}
				}
			}
		}
	}

	// Credit goods to destination — silver is now a normal good in settlement_goods.
	// cap 1_000_000 for a brand-new row matches economy.goodCap (post-00d0722; never
	// reintroduce the cap-100 bug — this literal was stale until 2026-07-24, silently
	// truncating a second trade delivery of a good the settlement had never held
	// before down to 100, see trade_delivery_stale_cap_test.go).
	if _, err = tx.Exec(ctx,
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
		 VALUES ($1, $2, $3, 0, $4, current_world_tick())
		 ON CONFLICT (settlement_id, good_key) DO UPDATE SET
		     amount = LEAST(
		         settled(settlement_goods.amount, settlement_goods.rate, settlement_goods.calc_tick)
		             + $3,
		         settlement_goods.cap),
		     calc_tick = current_world_tick()`,
		p.DestinationID, p.GoodKey, credited, province.DefaultGoodStorageCap,
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
		if _, err := tx.Exec(ctx, `UPDATE transports SET status='delivered',updated_at=now() WHERE id=$1`, p.TransportID); err != nil {
			return err
		}
	}

	// R4 (megaron_plan_sjohandel_mellan_spelare.md): does leg 1's ship (if any)
	// still owe an EMPTY voyage home, or is the counterpart's ThenReturn leg 2
	// its way home instead? Read once, before either branch below decides.
	var leg1ShipUnitID *uuid.UUID
	if p.TransportID != (uuid.UUID{}) {
		if err := tx.QueryRow(ctx, `SELECT ship_unit_id FROM transports WHERE id=$1`, p.TransportID).Scan(&leg1ShipUnitID); err != nil {
			return fmt.Errorf("read delivery ship: %w", err)
		}
	}

	// Sjöhandel kräver skepp (megaron_plan_sjohandel_kraver_skepp.md R3): a
	// naval leg that bound a real ship is only half done — the ship still has
	// to sail home before it's free again. Raw SQL, not transport.Dispatch:
	// economy may not import the transport package (G1), same reason the
	// ThenReturn leg above is built by hand. transport.ArrivalHandler (already
	// registered for ScheduledTransportArrival, whoever inserted the row it
	// fires for) does the actual release when this leg lands.
	//
	// R4 (sjöhandel mellan spelare): skipped when a ThenReturn leg exists —
	// that chained leg 2 IS the ship's way home (same ship, cargo instead of
	// an empty hold); dispatching BOTH would sail the same hull twice.
	if p.TransportID != (uuid.UUID{}) && len(p.ThenReturn) == 0 {
		if err := dispatchShipReturnLeg(ctx, tx, h.scheduler, e.WorldID, p.TransportID, p.DestinationID); err != nil {
			return fmt.Errorf("dispatch ship return leg: %w", err)
		}
	}

	// Chain: if this was a silver leg, dispatch the goods return now — as its own
	// physical caravan (leg 2), so the return trip is visible and interceptable too.
	if len(p.ThenReturn) > 0 {
		if h.scheduler == nil {
			return fmt.Errorf("trade return requires scheduler")
		}
		var ret struct {
			DestinationID string                 `json:"destination_id"`
			GoodKey       string                 `json:"good_key"`
			Quantity      float64                `json:"quantity"`
			MessengerID   string                 `json:"messenger_id"`
			TravelMins    float64                `json:"travel_mins"`
			Journey       *province.TradeJourney `json:"journey,omitempty"`
			TravelTicks   int                    `json:"travel_ticks,omitempty"`
			OwnerID       string                 `json:"owner_id"`
			OriginQ       int                    `json:"origin_q"`
			OriginR       int                    `json:"origin_r"`
			DestQ         int                    `json:"dest_q"`
			DestR         int                    `json:"dest_r"`
		}
		if jsonErr := json.Unmarshal(p.ThenReturn, &ret); jsonErr != nil {
			return fmt.Errorf("decode trade return: %w", jsonErr)
		}
		if ret.DestinationID == "" {
			return fmt.Errorf("trade return destination missing")
		}
		{
			var currentTick int
			if err := tx.QueryRow(ctx, `SELECT current_tick FROM worlds WHERE id=$1`, e.WorldID).Scan(&currentTick); err != nil {
				return err
			}
			travelTicks := int(math.Round(ret.TravelMins / 60))
			if travelTicks < 1 {
				travelTicks = 1
			}
			var saved []byte
			var departure *int
			departsAt := h.scheduler.Clock().Now()
			arrivesAt := departsAt.Add(time.Duration(ret.TravelMins * float64(time.Minute)))
			if ret.Journey != nil {
				if err := ret.Journey.Validate(); err != nil {
					return err
				}
				if ret.TravelTicks != 0 && ret.TravelTicks != ret.Journey.TravelTicks {
					return fmt.Errorf("return journey duration mismatch")
				}
				travelTicks = ret.Journey.TravelTicks
				var err error
				saved, err = json.Marshal(ret.Journey)
				if err != nil {
					return err
				}
				departure = &currentTick
				arrivesAt = departsAt.Add(tick.RealUntil(travelTicks, 0))
			}

			// Build the return caravan (leg 2: this settlement → the buyer/seller origin).
			// Raw SQL: economy may not import the transport package (G1).
			// Arrival, mover, manifest and timer commit atomically.
			var leg2ID uuid.UUID
			retOwner, err := uuid.Parse(ret.OwnerID)
			if err != nil {
				return fmt.Errorf("return owner: %w", err)
			}
			retDest, err := uuid.Parse(ret.DestinationID)
			if err != nil {
				return fmt.Errorf("return destination: %w", err)
			}
			leg2Category := "land"
			var leg2ShipUnitID *uuid.UUID
			if leg1ShipUnitID != nil {
				// R4: the SAME ship carries leg 2 home — its owner is the
				// ship's real owner (the trade's initiator), not whoever's
				// city leg 2 happens to depart from (the land-caravan
				// ownership rule ret.OwnerID otherwise encodes, still
				// correct for a non-naval leg 2 below).
				leg2Category = "naval"
				leg2ShipUnitID = leg1ShipUnitID
				if serr := tx.QueryRow(ctx, `SELECT owner_id FROM units WHERE id = $1`, *leg1ShipUnitID).Scan(&retOwner); serr != nil {
					return fmt.Errorf("read ship owner for return: %w", serr)
				}
			}
			if ret.Journey != nil && ret.Journey.Category != leg2Category {
				return fmt.Errorf("return journey category mismatch")
			}
			if scanErr := tx.QueryRow(ctx,
				`INSERT INTO transports
				   (world_id, owner_id, kind, origin_id, dest_id, category,
				    origin_q, origin_r, dest_q, dest_r, departs_at, arrives_at, due_tick, interceptable, ship_unit_id, journey, departed_tick)
                VALUES ($1,$2,'trade_return',$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,true,$13,$14,$15)
				 RETURNING id`,
				e.WorldID, retOwner, p.DestinationID, retDest, leg2Category,
				ret.OriginQ, ret.OriginR, ret.DestQ, ret.DestR, departsAt, arrivesAt, currentTick+travelTicks, leg2ShipUnitID, saved, departure,
			).Scan(&leg2ID); scanErr != nil {
				return fmt.Errorf("insert return transport: %w", scanErr)
			}
			if _, err := tx.Exec(ctx,
				`INSERT INTO transport_goods (transport_id, good_key, quantity) VALUES ($1,$2,$3)`,
				leg2ID, ret.GoodKey, ret.Quantity); err != nil {
				return fmt.Errorf("insert return goods: %w", err)
			}

			if ret.Journey != nil && ret.MessengerID != "" {
				messengerID, err := uuid.Parse(ret.MessengerID)
				if err != nil {
					return fmt.Errorf("return messenger: %w", err)
				}
				keyTick, keyAt := "goods_arrival_tick", "goods_arrives_at"
				if ret.GoodKey == "silver" {
					keyTick, keyAt = "silver_arrival_tick", "silver_arrives_at"
				}
				if _, err := tx.Exec(ctx, `UPDATE messengers SET trade_offer=trade_offer || jsonb_build_object($2::text,$3::int,$4::text,$5::timestamptz) WHERE id=$1`, messengerID, keyTick, currentTick+travelTicks, keyAt, arrivesAt); err != nil {
					return fmt.Errorf("update chained ETA: %w", err)
				}
			}
			if err := h.scheduler.EnqueueTickTx(ctx, tx, e.WorldID, events.ScheduledTradeReturn,
				map[string]any{
					"destination_id": ret.DestinationID,
					"good_key":       ret.GoodKey,
					"quantity":       ret.Quantity,
					"messenger_id":   ret.MessengerID,
					"transport_id":   leg2ID.String(),
				}, currentTick+travelTicks); err != nil {
				return fmt.Errorf("schedule return: %w", err)
			}
		}
	}

	var passengerEvents []*events.Event
	if leg1ShipUnitID != nil {
		store := h.eventStore
		if store == nil {
			store = events.NewStore(h.pool)
		}
		passengerEvents, err = carrier.PortTx(ctx, tx, store, e.WorldID, *leg1ShipUnitID, p.DestinationID, e.DueTick)
		if err != nil {
			return err
		}
	}
	var deliveredEvent *events.Event
	if h.eventStore != nil {
		deliveredEvent, err = h.eventStore.AppendTx(ctx, tx, p.DestinationID, events.StreamProvince, "TradeDelivery", map[string]any{"good_key": p.GoodKey, "quantity": delivered, "route_id": p.TradeRouteID}, e.WorldID, nil)
		if err != nil {
			return fmt.Errorf("record delivery: %w", err)
		}
	}
	notice := map[string]any{"destination_id": p.DestinationID, "good_key": p.GoodKey, "quantity": delivered}
	recipient, noticeID, err := persistTradeNotice(ctx, tx, e.WorldID, p.DestinationID, "TradeDelivery", notice)
	if err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit delivery: %w", err)
	}
	for _, record := range append(passengerEvents, titheEvent, deliveredEvent) {
		if record != nil {
			if h.eventStore != nil {
				h.eventStore.RecordCommitted(ctx, record)
			}
		}
	}
	deliverTradeNotice(ctx, h.hub, e.WorldID, recipient, noticeID, "TradeDelivery", notice)

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

// dispatchShipReturnLeg is R3's hemresa: if the transport that just delivered
// bound a real ship (ship_unit_id set — sjöhandel kräver skepp), that ship
// still owes an empty voyage home before it's free again. No-ops silently
// when the leg wasn't naval or bound no ship (every land caravan and every
// pre-slice naval transport, R6). A failure rolls the delivery back so a
// ship can never be left bound without a return mover and timer.
func dispatchShipReturnLeg(ctx context.Context, tx pgx.Tx, sched *events.Scheduler, worldID uuid.UUID, transportID, arrivedID uuid.UUID) error {
	var shipUnitID *uuid.UUID
	var homeID, actualArrivalID *uuid.UUID
	var ownerID uuid.UUID
	var homeQ, homeR, arriveQ, arriveR int
	if err := tx.QueryRow(ctx,
		`SELECT ship_unit_id, origin_id, dest_id, owner_id, origin_q, origin_r, dest_q, dest_r
		 FROM transports WHERE id = $1`, transportID,
	).Scan(&shipUnitID, &homeID, &actualArrivalID, &ownerID, &homeQ, &homeR, &arriveQ, &arriveR); err != nil {
		return fmt.Errorf("load outbound leg: %w", err)
	}
	if shipUnitID == nil {
		return nil // land caravan, or a naval transport that never bound a ship (R6)
	}

	if sched == nil {
		return fmt.Errorf("ship return requires scheduler")
	}
	journey, err := province.PlanTradeJourney(ctx, tx, worldID, province.MapPosition{Q: arriveQ, R: arriveR}, province.MapPosition{Q: homeQ, R: homeR}, "naval")
	if err != nil {
		return err
	}
	raw, err := json.Marshal(journey)
	if err != nil {
		return err
	}
	var currentTick int
	if err := tx.QueryRow(ctx, `SELECT current_tick FROM worlds WHERE id=$1`, worldID).Scan(&currentTick); err != nil {
		return err
	}
	departsAt := sched.Clock().Now()
	arrivesAt := departsAt.Add(tick.RealUntil(journey.TravelTicks, 0))

	var returnID uuid.UUID
	if err := tx.QueryRow(ctx,
		`INSERT INTO transports
		   (world_id, owner_id, kind, origin_id, dest_id, category,
		    origin_q, origin_r, dest_q, dest_r, departs_at, arrives_at, due_tick,
		    interceptable, ship_unit_id, journey, departed_tick)
         VALUES ($1,$2,'ship_return',$3,$4,'naval',$5,$6,$7,$8,$9,$10,$11,true,$12,$13,$14)
		 RETURNING id`,
		worldID, ownerID, actualArrivalID, homeID,
		arriveQ, arriveR, homeQ, homeR, departsAt, arrivesAt, currentTick+journey.TravelTicks, *shipUnitID, raw, currentTick,
	).Scan(&returnID); err != nil {
		return fmt.Errorf("insert ship return leg: %w", err)
	}
	return sched.EnqueueTickTx(ctx, tx, worldID, events.ScheduledTransportArrival,
		map[string]any{"transport_id": returnID}, currentTick+journey.TravelTicks)
}

// committedTradeNotifier is defined by this consumer; notify.Hub satisfies it
// without introducing an upward import. Test broadcasters can use NotifyPlayer.
type committedTradeNotifier interface {
	DeliverCommittedNotification(context.Context, uuid.UUID, uuid.UUID, string, string, int, any)
}

func persistTradeNotice(ctx context.Context, tx pgx.Tx, worldID, destID uuid.UUID, kind string, payload any) (uuid.UUID, string, error) {
	var owner *uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT owner_id FROM settlements WHERE id=$1`, destID).Scan(&owner); err != nil {
		return uuid.Nil, "", err
	}
	if owner == nil || *owner == uuid.Nil {
		return uuid.Nil, "", nil
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return uuid.Nil, "", err
	}
	var id string
	if err := tx.QueryRow(ctx, `INSERT INTO notifications (world_id,player_id,kind,level,body_json) VALUES ($1,$2,$3,3,$4) RETURNING id`, worldID, *owner, kind, raw).Scan(&id); err != nil {
		return uuid.Nil, "", fmt.Errorf("persist trade notice: %w", err)
	}
	return *owner, id, nil
}

func deliverTradeNotice(ctx context.Context, hub Broadcaster, worldID, owner uuid.UUID, id, kind string, payload any) {
	if hub == nil || owner == uuid.Nil {
		return
	}
	if committed, ok := hub.(committedTradeNotifier); ok {
		committed.DeliverCommittedNotification(ctx, worldID, owner, id, kind, 3, payload)
		return
	}
	_ = hub.NotifyPlayer(ctx, worldID, owner, kind, 3, payload)
}
