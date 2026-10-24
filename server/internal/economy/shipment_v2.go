package economy

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"

	"formatet/megaron/server/internal/carrier"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/province"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// CargoItem uses quantities, never catalogue weights, for cargo space.
type CargoItem struct {
	GoodKey  string  `json:"good_key"`
	Quantity float64 `json:"quantity"`
}

// NormalizeCargo merges duplicate goods and establishes deterministic debit order.
func NormalizeCargo(items []CargoItem) ([]CargoItem, float64, error) {
	if len(items) == 0 || len(items) > 100 {
		return nil, 0, fmt.Errorf("items must contain between 1 and 100 goods")
	}
	sums := map[string]float64{}
	total := 0.0
	for _, it := range items {
		if it.GoodKey == "" || it.Quantity <= 0 || math.IsNaN(it.Quantity) || math.IsInf(it.Quantity, 0) {
			return nil, 0, fmt.Errorf("each item needs a good_key and finite positive quantity")
		}
		sums[it.GoodKey] += it.Quantity
		total += it.Quantity
	}
	if math.IsInf(total, 0) {
		return nil, 0, fmt.Errorf("cargo quantity is too large")
	}
	keys := make([]string, 0, len(sums))
	for k := range sums {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]CargoItem, 0, len(keys))
	for _, k := range keys {
		out = append(out, CargoItem{k, sums[k]})
	}
	return out, total, nil
}

type ShipmentV2 struct {
	TransportID     uuid.UUID   `json:"transport_id"`
	RouteID         uuid.UUID   `json:"route_id"`
	SenderID        uuid.UUID   `json:"sender_id"`
	RecipientID     uuid.UUID   `json:"recipient_id"`
	OriginID        uuid.UUID   `json:"origin_id"`
	DestinationID   uuid.UUID   `json:"destination_id"`
	OriginName      string      `json:"origin_name"`
	DestinationName string      `json:"destination_name"`
	SenderName      string      `json:"sender_name"`
	RecipientName   string      `json:"recipient_name"`
	Items           []CargoItem `json:"items"`
	Kind            string      `json:"kind"`
}

type CargoOutcome struct {
	CargoItem
	CreditedQuantity float64 `json:"credited_quantity"`
	ReturnedQuantity float64 `json:"returned_quantity"`
	LostQuantity     float64 `json:"lost_quantity"`
}

type ShipmentOutcomeV2 struct {
	ShipmentV2
	Items               []CargoOutcome `json:"items"`
	ActualRecipientID   *uuid.UUID     `json:"actual_recipient_id"`
	ActualRecipientName string         `json:"actual_recipient_name"`
	ReturnTransportID   *uuid.UUID     `json:"return_transport_id,omitempty"`
	Reason              string         `json:"reason"`
	OwnerChanged        bool           `json:"owner_changed"`
}

// HandleShipmentV2 owns list-format deliveries only. Legacy timers retain their handlers.
func (h *DeliveryHandler) HandleShipmentV2(ctx context.Context, e events.ScheduledEvent) error {
	var p ShipmentV2
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return err
	}
	items, _, err := NormalizeCargo(p.Items)
	if err != nil {
		return err
	}
	p.Items = items
	if p.TransportID == uuid.Nil || p.SenderID == uuid.Nil || p.RecipientID == uuid.Nil || (p.Kind != "gift" && p.Kind != "transfer") {
		return fmt.Errorf("invalid shipment")
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
	if err := tx.QueryRow(ctx, `SELECT resolved FROM trade_routes WHERE id=$1 FOR UPDATE`, p.RouteID).Scan(&resolved); err != nil {
		// Legacy internal delivery stops when its route has disappeared (for
		// example, when the destination was deleted and the FK cascaded).
		if err == pgx.ErrNoRows && p.Kind == "transfer" {
			return nil
		}
		if err != pgx.ErrNoRows {
			return err
		}
	}
	if resolved {
		return tx.Commit(ctx)
	}
	var status string
	var ship *uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT status,ship_unit_id FROM transports WHERE id=$1 AND world_id=$2 FOR UPDATE`, p.TransportID, e.WorldID).Scan(&status, &ship); err != nil {
		return err
	}
	if status == "delivered" {
		return tx.Commit(ctx)
	}
	out := ShipmentOutcomeV2{ShipmentV2: p, Items: make([]CargoOutcome, 0, len(items))}
	var state string
	err = tx.QueryRow(ctx, `SELECT s.state,s.owner_id,COALESCE(NULLIF(pl.wanax_name,''),pl.username,'') FROM settlements s LEFT JOIN players pl ON pl.id=s.owner_id WHERE s.id=$1 AND s.world_id=$2 FOR UPDATE OF s`, p.DestinationID, e.WorldID).Scan(&state, &out.ActualRecipientID, &out.ActualRecipientName)
	if err != nil && err != pgx.ErrNoRows {
		return err
	}
	out.OwnerChanged = out.ActualRecipientID != nil && *out.ActualRecipientID != p.RecipientID
	deliver := status == "in_transit" && err == nil && state == "active" && out.ActualRecipientID != nil
	// Internal delivery preserves the legacy route contract: credit the
	// destination that still exists, regardless of its current owner/state.
	// Gifts deliberately keep their separate active-recipient rules.
	if p.Kind == "transfer" {
		deliver = status == "in_transit" && err == nil
	}
	returned := map[string]float64{}
	switch {
	case status == "foundered":
		out.Reason = "foundered"
	case status != "in_transit":
		out.Reason = "intercepted"
		var raw []byte
		er := tx.QueryRow(ctx, `SELECT (payload->>'return_transport_id')::uuid,payload->'returned_goods' FROM events WHERE world_id=$1 AND event_type='TransportDamagedReturnDispatched' AND payload->>'transport_id'=$2 ORDER BY id DESC LIMIT 1`, e.WorldID, p.TransportID.String()).Scan(&out.ReturnTransportID, &raw)
		if er != nil && er != pgx.ErrNoRows {
			return er
		}
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &returned); err != nil {
				return err
			}
		}
	case !deliver:
		if out.Reason == "" {
			out.Reason = "destination_fallen"
		}
	}
	var passengerEvents []*events.Event
	creditedTotal := 0.0
	for _, it := range items {
		line := CargoOutcome{CargoItem: it, LostQuantity: it.Quantity}
		if deliver {
			if err := tx.QueryRow(ctx, `SELECT credit_settlement_good($1,$2,$3,'shipment_delivery_v2',$4)`, p.DestinationID, it.GoodKey, it.Quantity, province.DefaultGoodStorageCap).Scan(&line.CreditedQuantity); err != nil {
				return err
			}
		} else {
			line.ReturnedQuantity = math.Min(it.Quantity, math.Max(0, returned[it.GoodKey]))
		}
		line.LostQuantity = it.Quantity - line.CreditedQuantity - line.ReturnedQuantity
		if deliver && line.LostQuantity > 0 {
			out.Reason = "storage_full"
		}
		creditedTotal += line.CreditedQuantity
		out.Items = append(out.Items, line)
	}
	if status == "in_transit" {
		terminal := "delivered"
		if creditedTotal == 0 {
			terminal = "lost"
		}
		if _, err := tx.Exec(ctx, `UPDATE transports SET status=$2,updated_at=now() WHERE id=$1`, p.TransportID, terminal); err != nil {
			return err
		}
		if ship != nil {
			if deliver {
				if err := HospitalityAtTransportPort(ctx, tx, p.TransportID, p.DestinationID); err != nil {
					return err
				}
			}
			if state == "active" && out.ActualRecipientID != nil {
				passengerEvents, err = carrier.PortTx(ctx, tx, h.eventStore, e.WorldID, *ship, p.DestinationID, e.DueTick)
				if err != nil {
					return err
				}
			}
			if err := dispatchShipReturnLeg(ctx, tx, h.scheduler, e.WorldID, p.TransportID); err != nil {
				return err
			}
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE trade_routes SET resolved=true WHERE id=$1`, p.RouteID); err != nil {
		return err
	}
	kind := "TransferDeliveredV2"
	if p.Kind == "gift" {
		kind = "GiftDeliveredV2"
	}
	if creditedTotal == 0 {
		kind = "TransferLostV2"
		if p.Kind == "gift" {
			kind = "GiftLostV2"
		}
	}
	record, err := h.eventStore.AppendTx(ctx, tx, p.OriginID, events.StreamProvince, kind, out, e.WorldID, nil)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return err
	}
	recipients := map[uuid.UUID]bool{p.SenderID: true}
	if p.Kind == "gift" {
		recipients[p.RecipientID] = true
		if out.ActualRecipientID != nil {
			recipients[*out.ActualRecipientID] = true
		}
	}
	type personalNotice struct {
		id   string
		body map[string]any
	}
	notices := map[uuid.UUID]personalNotice{}
	for recipient := range recipients {
		var body map[string]any
		if err := json.Unmarshal(raw, &body); err != nil {
			return err
		}
		if recipient != p.SenderID {
			delete(body, "origin_id")
			delete(body, "origin_name")
		}
		personalRaw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		var id string
		if err := tx.QueryRow(ctx, `INSERT INTO notifications(world_id,player_id,kind,level,body_json) VALUES($1,$2,$3,3,$4) RETURNING id`, e.WorldID, recipient, kind, personalRaw).Scan(&id); err != nil {
			return err
		}
		notices[recipient] = personalNotice{id, body}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	h.eventStore.RecordCommitted(ctx, record)
	for _, witness := range passengerEvents {
		h.eventStore.RecordCommitted(ctx, witness)
	}
	for recipient, notice := range notices {
		deliverTradeNotice(ctx, h.hub, e.WorldID, recipient, notice.id, kind, notice.body)
	}
	return nil
}
