package economy

import (
	"context"
	"encoding/json"
	"fmt"
	"math"

	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/province"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// GiftShipment captures the identities at departure; it survives changed or
// removed settlements and is distinct from all legacy trade event payloads.
type GiftShipment struct {
	TransportID     uuid.UUID `json:"transport_id"`
	RouteID         uuid.UUID `json:"route_id"`
	SenderID        uuid.UUID `json:"sender_id"`
	RecipientID     uuid.UUID `json:"recipient_id"`
	OriginID        uuid.UUID `json:"origin_id"`
	DestinationID   uuid.UUID `json:"destination_id"`
	OriginName      string    `json:"origin_name"`
	DestinationName string    `json:"destination_name"`
	SenderName      string    `json:"sender_name"`
	RecipientName   string    `json:"recipient_name"`
	GoodKey         string    `json:"good_key"`
	Quantity        float64   `json:"quantity"`
}

type GiftOutcome struct {
	GiftShipment
	ActualRecipientID   *uuid.UUID `json:"actual_recipient_id"`
	ActualRecipientName string     `json:"actual_recipient_name"`
	CreditedQuantity    float64    `json:"credited_quantity"`
	ReturnedQuantity    float64    `json:"returned_quantity"`
	ReturnTransportID   *uuid.UUID `json:"return_transport_id,omitempty"`
	LostQuantity        float64    `json:"lost_quantity"`
	Reason              string     `json:"reason"`
	OwnerChanged        bool       `json:"owner_changed"`
}

// HandleGift processes only the new GiftDelivery timer. Legacy trade deliveries
// keep their frozen semantics and notification types in Handle.
func (h *DeliveryHandler) HandleGift(ctx context.Context, e events.ScheduledEvent) error {
	var p GiftShipment
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return err
	}
	if p.TransportID == uuid.Nil || p.SenderID == uuid.Nil || p.RecipientID == uuid.Nil || p.Quantity <= 0 || math.IsNaN(p.Quantity) || math.IsInf(p.Quantity, 0) {
		return fmt.Errorf("invalid gift shipment")
	}
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	ct, err := tx.Exec(ctx, `INSERT INTO processed_deliveries(event_id) VALUES($1) ON CONFLICT DO NOTHING`, e.ID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return nil
	}
	var resolved bool
	if err := tx.QueryRow(ctx, `SELECT resolved FROM trade_routes WHERE id=$1 FOR UPDATE`, p.RouteID).Scan(&resolved); err != nil && err != pgx.ErrNoRows {
		return err
	}
	if resolved {
		return tx.Commit(ctx)
	}
	var status string
	var ship *uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT status,ship_unit_id FROM transports WHERE id=$1 AND world_id=$2 FOR UPDATE`, p.TransportID, e.WorldID).Scan(&status, &ship); err != nil {
		return err
	}
	var recorded bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM events WHERE world_id=$1 AND event_type IN('GiftDelivered','GiftLost') AND payload->>'transport_id'=$2)`, e.WorldID, p.TransportID.String()).Scan(&recorded); err != nil {
		return err
	}
	if recorded {
		return tx.Commit(ctx)
	}
	if status == "delivered" {
		return tx.Commit(ctx)
	}
	out := GiftOutcome{GiftShipment: p, LostQuantity: p.Quantity}
	var state string
	err = tx.QueryRow(ctx, `SELECT s.state,s.owner_id,COALESCE(NULLIF(pl.wanax_name,''),pl.username,'') FROM settlements s LEFT JOIN players pl ON pl.id=s.owner_id WHERE s.id=$1 AND s.world_id=$2 FOR UPDATE OF s`, p.DestinationID, e.WorldID).Scan(&state, &out.ActualRecipientID, &out.ActualRecipientName)
	if err != nil && err != pgx.ErrNoRows {
		return err
	}
	lossReason := ""
	if status == "in_transit" && err != pgx.ErrNoRows && state == "active" && out.ActualRecipientID != nil {
		lossReason = h.deliveryLoss(ctx, tx, p.RouteID, p.TransportID, p.DestinationID)
	}
	out.OwnerChanged = out.ActualRecipientID != nil && *out.ActualRecipientID != p.RecipientID
	switch {
	case status != "in_transit":
		out.Reason = "intercepted"
		// This means physically sent home on the existing damaged-return voyage,
		// not an immediate stock refund. That voyage can still be intercepted.
		var returned float64
		er := tx.QueryRow(ctx, `SELECT (payload->>'return_transport_id')::uuid,COALESCE((payload->'returned_goods'->>$3)::double precision,0) FROM events WHERE world_id=$1 AND event_type='TransportDamagedReturnDispatched' AND payload->>'transport_id'=$2 ORDER BY id DESC LIMIT 1`, e.WorldID, p.TransportID.String(), p.GoodKey).Scan(&out.ReturnTransportID, &returned)
		if er != nil && er != pgx.ErrNoRows {
			return er
		}
		out.ReturnedQuantity = math.Min(p.Quantity, math.Max(0, returned))
		out.LostQuantity = p.Quantity - out.ReturnedQuantity
	case err == pgx.ErrNoRows || state != "active" || out.ActualRecipientID == nil:
		out.Reason = "destination_fallen"
	case lossReason != "":
		out.Reason = lossReason
	default:
		// Materialise lazy stock under the goods row lock. Record actual delta,
		// including a newly introduced row's cap; never report truncated cargo as delivered.
		if _, err := tx.Exec(ctx, `INSERT INTO settlement_goods(settlement_id,good_key,amount,rate,cap,calc_tick) VALUES($1,$2,0,0,$3,current_world_tick()) ON CONFLICT DO NOTHING`, p.DestinationID, p.GoodKey, province.DefaultGoodStorageCap); err != nil {
			return err
		}
		var amount, cap float64
		if err := tx.QueryRow(ctx, `SELECT settled(amount,rate,calc_tick),cap FROM settlement_goods WHERE settlement_id=$1 AND good_key=$2 FOR UPDATE`, p.DestinationID, p.GoodKey).Scan(&amount, &cap); err != nil {
			return err
		}
		out.CreditedQuantity = math.Min(p.Quantity, math.Max(0, cap-amount))
		out.LostQuantity = p.Quantity - out.CreditedQuantity
		if out.LostQuantity > 0 {
			out.Reason = "storage_full"
		}
		if _, err := tx.Exec(ctx, `UPDATE settlement_goods SET amount=$3,calc_tick=current_world_tick() WHERE settlement_id=$1 AND good_key=$2`, p.DestinationID, p.GoodKey, amount+out.CreditedQuantity); err != nil {
			return err
		}
	}
	if status == "in_transit" {
		terminal := "delivered"
		if out.CreditedQuantity == 0 {
			terminal = "lost"
		}
		if _, err := tx.Exec(ctx, `UPDATE transports SET status=$2,updated_at=now() WHERE id=$1`, p.TransportID, terminal); err != nil {
			return err
		}
		// No goods return. The existing empty voyage releases the sender's real
		// naval carrier even when the cargo was lost or the destination fell.
		if ship != nil {
			if err := dispatchShipReturnLeg(ctx, tx, h.scheduler, e.WorldID, p.TransportID, p.DestinationID); err != nil {
				return err
			}
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE trade_routes SET resolved=true WHERE id=$1`, p.RouteID); err != nil {
		return err
	}
	kind := "GiftDelivered"
	if out.CreditedQuantity == 0 {
		kind = "GiftLost"
	}
	event, err := h.eventStore.AppendTx(ctx, tx, p.OriginID, events.StreamProvince, kind, out, e.WorldID, nil)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return err
	}
	// Identity-based notices survive city ownership changes. De-duplicate a
	// sender who conquered the destination or an unchanged intended recipient.
	recipients := map[uuid.UUID]bool{p.SenderID: true, p.RecipientID: true}
	if out.ActualRecipientID != nil {
		recipients[*out.ActualRecipientID] = true
	}
	notices := map[uuid.UUID]string{}
	for recipient := range recipients {
		var id string
		if err := tx.QueryRow(ctx, `INSERT INTO notifications(world_id,player_id,kind,level,body_json) VALUES($1,$2,$3,3,$4) RETURNING id`, e.WorldID, recipient, kind, raw).Scan(&id); err != nil {
			return err
		}
		notices[recipient] = id
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	h.eventStore.RecordCommitted(ctx, event)
	for recipient, id := range notices {
		deliverTradeNotice(ctx, h.hub, e.WorldID, recipient, id, kind, out)
	}
	return nil
}
