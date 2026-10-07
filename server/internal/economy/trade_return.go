package economy

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/province"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TradeReturnHandler delivers goods to the buyer after a trade offer is accepted.
type TradeReturnHandler struct {
	pool       *pgxpool.Pool
	eventStore *events.Store
	hub        Broadcaster
	Dice       Dice // exported so tests can override; defaults to wallDice (production behaviour).
}

// NewTradeReturnHandler creates a TradeReturnHandler.
func NewTradeReturnHandler(pool *pgxpool.Pool, eventStore *events.Store, hub Broadcaster) *TradeReturnHandler {
	return &TradeReturnHandler{pool: pool, eventStore: eventStore, hub: hub, Dice: wallDice{}}
}

// Handle credits goods to the buyer settlement when a negotiated trade return arrives.
func (h *TradeReturnHandler) Handle(ctx context.Context, e events.ScheduledEvent) error {
	var p struct {
		DestinationID uuid.UUID `json:"destination_id"` // buyer's settlement
		GoodKey       string    `json:"good_key"`
		Quantity      float64   `json:"quantity"`
		MessengerID   uuid.UUID `json:"messenger_id"`
		TransportID   uuid.UUID `json:"transport_id"` // physical return caravan (0 = legacy event)
	}
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return fmt.Errorf("unmarshal trade return: %w", err)
	}

	// Idempotency: check messenger offer not already returned.
	var offerStatus string
	if err := h.pool.QueryRow(ctx,
		`SELECT trade_offer->>'status' FROM messengers WHERE id=$1`,
		p.MessengerID,
	).Scan(&offerStatus); err != nil {
		return nil // messenger gone
	}
	if offerStatus == "returned" {
		return nil
	}

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Physical-caravan interception veto: if the return caravan was intercepted or
	// lost (Del 3-fas-4), cancel delivery — the goods were seized en route.
	if p.TransportID != (uuid.UUID{}) {
		var tstatus string
		if qErr := tx.QueryRow(ctx,
			`SELECT status FROM transports WHERE id = $1 FOR UPDATE`, p.TransportID,
		).Scan(&tstatus); qErr != nil {
			return fmt.Errorf("load trade return transport: %w", qErr)
		}
		if tstatus != "in_transit" {
			return tx.Commit(ctx)
		}
	}

	// Trade risk: exactly the same rule as the outbound leg (isInternalTransfer +
	// tradeRiskPct, trade.go) — a negotiated trade between two wanaxes is external
	// on BOTH legs (CLAUDE.md trade-lagret punkt 2/3), so the return leg must not
	// silently skip the die just because it's the second half of the round trip.
	// tradeRouteID is always zero here (this leg has no trade_routes row); origin
	// is resolved from the return caravan's own transports.origin_id — the
	// settlement that is sending this leg back — via isInternalTransfer's existing
	// transport-based lookup. No transport (legacy event) ⇒ unresolvable ⇒ external
	// (fail-external, never guess internal).
	if !isInternalTransfer(ctx, tx, uuid.UUID{}, p.TransportID, p.DestinationID) && h.Dice.Float64() < tradeRiskPct {
		reason := tradeLostReasons[h.Dice.Intn(len(tradeLostReasons))]
		if p.TransportID != (uuid.UUID{}) {
			if _, err = tx.Exec(ctx,
				`UPDATE transports SET status = 'lost', updated_at = now() WHERE id = $1`, p.TransportID,
			); err != nil {
				return fmt.Errorf("mark lost return transport: %w", err)
			}
		}
		// Mark the offer's round trip concluded so a retry of this event (or the
		// auto-return path) doesn't re-roll — same terminal flip as the success
		// path below, just without ever crediting the buyer.
		if _, err = tx.Exec(ctx,
			`UPDATE messengers SET trade_offer = trade_offer || '{"status":"returned"}' WHERE id=$1`,
			p.MessengerID,
		); err != nil {
			return fmt.Errorf("mark returned (lost): %w", err)
		}
		var lostEvent *events.Event
		if h.eventStore != nil {
			lostEvent, err = h.eventStore.AppendTx(ctx, tx, p.DestinationID, events.StreamProvince, "TradeLost", map[string]any{"good_key": p.GoodKey, "quantity": p.Quantity, "reason": reason, "messenger_id": p.MessengerID}, e.WorldID, nil)
			if err != nil {
				return fmt.Errorf("record return loss: %w", err)
			}
		}
		notice := map[string]any{"destination_id": p.DestinationID, "good_key": p.GoodKey, "quantity": p.Quantity, "reason": reason}
		recipient, noticeID, err := persistTradeNotice(ctx, tx, e.WorldID, p.DestinationID, "TradeLost", notice)
		if err != nil {
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit return loss: %w", err)
		}
		if lostEvent != nil {
			h.eventStore.RecordCommitted(ctx, lostEvent)
		}
		deliverTradeNotice(ctx, h.hub, e.WorldID, recipient, noticeID, "TradeLost", notice)

		slog.Info("trade return lost", "messenger", p.MessengerID, "good", p.GoodKey, "reason", reason)
		return nil
	}

	// Credit goods to buyer — silver is now a normal good in settlement_goods.
	// cap = goodCap(good), matching every other credit path (arrival.go, trade.go,
	// province.go Craft). Previously hard-coded 1_000_000 inline here — same value
	// as goodCap() today, so not presently truncating anything, but a literal that
	// silently drifts from goodCap() the day that function's return value changes
	// is exactly the failure class trade_delivery_stale_cap_test.go documents
	// (cap=100 before 2026-07-24). Routed through the shared function instead.
	if _, err = tx.Exec(ctx,
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
		 VALUES ($1, $2, $3, 0, $4, current_world_tick())
		 ON CONFLICT (settlement_id, good_key) DO UPDATE SET
		     amount = LEAST(
		         settled(settlement_goods.amount, settlement_goods.rate, settlement_goods.calc_tick)
		             + $3,
		         settlement_goods.cap),
		     calc_tick = current_world_tick()`,
		p.DestinationID, p.GoodKey, p.Quantity, goodCap(p.GoodKey),
	); err != nil {
		return fmt.Errorf("credit goods to buyer: %w", err)
	}

	// Mark as returned.
	if _, err = tx.Exec(ctx,
		`UPDATE messengers SET trade_offer = trade_offer || '{"status":"returned"}' WHERE id=$1`,
		p.MessengerID,
	); err != nil {
		return fmt.Errorf("mark returned: %w", err)
	}

	// The return caravan has arrived.
	if p.TransportID != (uuid.UUID{}) {
		if _, err := tx.Exec(ctx, `UPDATE transports SET status='delivered',updated_at=now() WHERE id=$1`, p.TransportID); err != nil {
			return err
		}
	}

	// R4 (megaron_plan_sjohandel_mellan_spelare.md): this leg landing IS the
	// ship's homecoming when it bound one (leg 2 sailed the initiator's own
	// hull home instead of an empty ship_return leg, DeliveryHandler.Handle).
	// p.DestinationID is the initiator's own settlement (ThenReturn always
	// credits the trade's origin) — exactly R3's "origin" for release
	// purposes, so the same fallback order applies: still an active
	// settlement the owner holds ⇒ garrison there; otherwise the owner's
	// nearest own port; no settlement left at all ⇒ left `positioned` on the
	// arrival hex (R3, transport.StrandShip's exact contract, copied here
	// because economy may not import transport — G1).
	if p.TransportID != (uuid.UUID{}) {
		var shipUnitID *uuid.UUID
		var ownerID uuid.UUID
		var destQ, destR int
		var journey json.RawMessage
		if serr := tx.QueryRow(ctx,
			`SELECT ship_unit_id, owner_id, dest_q, dest_r, journey FROM transports WHERE id = $1`, p.TransportID,
		).Scan(&shipUnitID, &ownerID, &destQ, &destR, &journey); serr != nil {
			return fmt.Errorf("read return carrier: %w", serr)
		}
		if shipUnitID != nil {
			if len(journey) > 0 {
				var saved province.TradeJourney
				if err := json.Unmarshal(journey, &saved); err != nil {
					return err
				}
				if err := saved.Validate(); err != nil {
					return err
				}
				last := saved.Path[len(saved.Path)-1]
				destQ, destR = last.Q, last.R
			}
			if rerr := releaseShipAfterTradeReturn(ctx, tx, e.WorldID, *shipUnitID, ownerID, p.DestinationID, destQ, destR); rerr != nil {
				return fmt.Errorf("release ship after trade return: %w", rerr)
			}
		}
	}

	var returnedEvent *events.Event
	if h.eventStore != nil {
		returnedEvent, err = h.eventStore.AppendTx(ctx, tx, p.DestinationID, events.StreamProvince, "TradeReturn", map[string]any{"good_key": p.GoodKey, "quantity": p.Quantity, "messenger_id": p.MessengerID}, e.WorldID, nil)
		if err != nil {
			return fmt.Errorf("record trade return: %w", err)
		}
	}
	notice := map[string]any{"destination_id": p.DestinationID, "good_key": p.GoodKey, "quantity": p.Quantity}
	recipient, noticeID, err := persistTradeNotice(ctx, tx, e.WorldID, p.DestinationID, "TradeReturn", notice)
	if err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit trade return: %w", err)
	}
	if returnedEvent != nil {
		h.eventStore.RecordCommitted(ctx, returnedEvent)
	}
	deliverTradeNotice(ctx, h.hub, e.WorldID, recipient, noticeID, "TradeReturn", notice)

	slog.Info("trade return delivered", "buyer", p.DestinationID, "good", p.GoodKey, "qty", p.Quantity)
	return nil
}

// releaseShipAfterTradeReturn is R4's frigörande when leg 2 of a negotiated
// trade sailed the initiator's own ship home (megaron_plan_sjohandel_mellan_
// spelare.md). economy may not import transport (G1), so — same reasoning as
// dispatchShipReturnLeg above, and carrier.go's own NearestOwnPort doc
// comment about crossing this exact boundary — this is its own small copy of
// transport.ReleaseShip / NearestOwnPort / StrandShip's exact SQL and
// fallback order, not a shared call. homeID is where the ship should end up
// (R3's fallback order): still an active settlement the owner holds ⇒
// garrison there; else the owner's nearest own settlement (shipyard
// preferred) ⇒ garrison there; no settlement left at all ⇒ left `positioned`
// on the arrival hex.
func releaseShipAfterTradeReturn(ctx context.Context, tx pgx.Tx, worldID, shipUnitID, ownerID, homeID uuid.UUID, homeQ, homeR int) error {
	var state string
	var curOwner uuid.UUID
	if err := tx.QueryRow(ctx,
		`SELECT state, owner_id FROM settlements WHERE id = $1`, homeID,
	).Scan(&state, &curOwner); err == nil && state == "active" && curOwner == ownerID {
		_, err := tx.Exec(ctx,
			`UPDATE units SET status = 'garrison', settlement_id = $2, updated_at = now()
			 WHERE id = $1 AND status = 'freighting'`, shipUnitID, homeID)
		return err
	}

	rows, err := tx.Query(ctx,
		`SELECT s.id, p.map_q, p.map_r,
		        EXISTS(SELECT 1 FROM buildings b WHERE b.settlement_id = s.id AND b.building_type = 'shipyard') AS has_shipyard
		 FROM settlements s JOIN provinces p ON p.id = s.province_id
		 WHERE s.owner_id = $1 AND s.world_id = $2 AND s.state = 'active'`,
		ownerID, worldID)
	if err != nil {
		return err
	}
	defer rows.Close()

	type candidate struct {
		id          uuid.UUID
		q, r        int
		hasShipyard bool
	}
	var withYard, all []candidate
	for rows.Next() {
		var c candidate
		if scanErr := rows.Scan(&c.id, &c.q, &c.r, &c.hasShipyard); scanErr != nil {
			return scanErr
		}
		all = append(all, c)
		if c.hasShipyard {
			withYard = append(withYard, c)
		}
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return rowsErr
	}

	pick := withYard
	if len(pick) == 0 {
		pick = all
	}
	if len(pick) == 0 {
		_, err := tx.Exec(ctx,
			`UPDATE units SET status = 'positioned', settlement_id = NULL, q = $2, r = $3, updated_at = now()
			 WHERE id = $1 AND status = 'freighting'`, shipUnitID, homeQ, homeR)
		return err
	}
	best := pick[0]
	bestDist := province.HexDistance(province.MapPosition{Q: homeQ, R: homeR}, province.MapPosition{Q: best.q, R: best.r})
	for _, c := range pick[1:] {
		d := province.HexDistance(province.MapPosition{Q: homeQ, R: homeR}, province.MapPosition{Q: c.q, R: c.r})
		if d < bestDist {
			best, bestDist = c, d
		}
	}
	_, err = tx.Exec(ctx,
		`UPDATE units SET status = 'garrison', settlement_id = $2, updated_at = now()
		 WHERE id = $1 AND status = 'freighting'`, shipUnitID, best.id)
	return err
}
