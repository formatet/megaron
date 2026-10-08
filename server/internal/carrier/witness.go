// Package carrier records physical passenger outcomes. Producers may read a
// sealed envelope but never project messenger state or deliver player news.
package carrier

import (
	"context"
	"encoding/json"
	"fmt"

	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const (
	Lost       = "CarrierPassengerLostV1"
	Rescued    = "CarrierPassengerRescuedV1"
	Landed     = "CarrierPassengerLandedV1"
	Redirected = "CarrierPassengerRedirectedV1"
	// TypesSQL is shared by producers and the consumer's pending-witness guard.
	TypesSQL = "('CarrierPassengerLostV1','CarrierPassengerRescuedV1','CarrierPassengerLandedV1','CarrierPassengerRedirectedV1')"
)

const passengerJoinSQL = `LEFT JOIN transports t ON t.id=m.carrier_transport_id
 LEFT JOIN LATERAL (SELECT id,event_type,payload FROM events WHERE stream_id=m.id AND event_type IN ` + TypesSQL + ` ORDER BY id DESC LIMIT 1) w ON true`
const passengerMatchSQL = `m.world_id=$1 AND m.passage_status='aboard'
 AND CASE WHEN COALESCE(w.id,0)>m.carrier_witness_id
 THEN w.event_type IN ('CarrierPassengerRescuedV1','CarrierPassengerRedirectedV1') AND w.payload->>'rescue_ship_id'=$2::text
 ELSE COALESCE(m.carrier_unit_id,t.ship_unit_id)=$2::uuid END`

func HasPassengerTx(ctx context.Context, tx pgx.Tx, world, ship uuid.UUID) (bool, error) {
	var yes bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM messengers m `+passengerJoinSQL+` WHERE `+passengerMatchSQL+`)`, world, ship).Scan(&yes)
	return yes, err
}

// DisembarkTx is a carrier dissolution beside an actual settlement: both
// ordinary and rescued runners can leave here, even without a formal port.
func DisembarkTx(ctx context.Context, tx pgx.Tx, store *events.Store, world, ship, settlement uuid.UUID, tick int) ([]*events.Event, error) {
	var q, r int
	if err := tx.QueryRow(ctx, `SELECT p.map_q,p.map_r FROM settlements s JOIN provinces p ON p.id=s.province_id WHERE s.id=$1 AND s.world_id=$2 AND s.state='active'`, settlement, world).Scan(&q, &r); err != nil {
		return nil, err
	}
	return record(ctx, tx, store, world, ship, Landed, nil, &settlement, "carrier_disbanded", q, r, tick)
}

// Witness is immutable evidence, stored on a PRIVATE messenger stream. Envelope
// is the entire sealed contents plus frozen endpoint names; never publish it on the
// carrier's unit/combat/province stream where the captor could read it.
type Witness struct {
	MessengerID    uuid.UUID       `json:"messenger_id"`
	SenderID       uuid.UUID       `json:"sender_wanax_id"`
	ShipID         uuid.UUID       `json:"ship_id"`
	RescueShipID   *uuid.UUID      `json:"rescue_ship_id,omitempty"`
	PortID         *uuid.UUID      `json:"port_id,omitempty"`
	Reason         string          `json:"reason"`
	Q              int             `json:"q"`
	R              int             `json:"r"`
	Tick           int             `json:"tick"`
	PreviousID     int64           `json:"previous_witness_id"`
	RescueShipName string          `json:"rescue_ship_name,omitempty"`
	PortName       string          `json:"port_name,omitempty"`
	Envelope       json.RawMessage `json:"envelope"`
}

// OutcomeTx must run with the physical ship locked, in its death/capture TX.
// A pending rescue is already physical truth: following it here lets a rescued
// passenger die or be rescued again BEFORE messenger has consumed the first
// witness. Pending landings/losses mean the passenger has left this ship.
func OutcomeTx(ctx context.Context, tx pgx.Tx, store *events.Store, world, ship uuid.UUID, rescue *uuid.UUID, reason string, q, r, tick int) ([]*events.Event, error) {
	kind := Lost
	if rescue != nil {
		kind = Rescued
	}
	if reason == "limped" {
		kind = Redirected
	}
	return record(ctx, tx, store, world, ship, kind, rescue, nil, reason, q, r, tick)
}

// PortTx records an actual port visit, even if the ship immediately sails
// again. It must be called by arrival, never inferred from a future ETA.
func PortTx(ctx context.Context, tx pgx.Tx, store *events.Store, world, ship, port uuid.UUID, tick int) ([]*events.Event, error) {
	var q, r int
	err := tx.QueryRow(ctx, `SELECT p.map_q,p.map_r FROM settlements s JOIN provinces p ON p.id=s.province_id WHERE s.id=$1 AND s.world_id=$2 AND s.state='active' AND (p.coastal OR EXISTS(SELECT 1 FROM buildings b WHERE b.settlement_id=s.id AND b.building_type='harbour'))`, port, world).Scan(&q, &r)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return record(ctx, tx, store, world, ship, Landed, nil, &port, "port", q, r, tick)
}

// ShoreTx preserves ordinary passage missions that land on a shore without
// a port. A rescued passenger stays aboard until a PORT; a normal boarded
// passenger may leave at its original mission's physical disembarkation.
func ShoreTx(ctx context.Context, tx pgx.Tx, store *events.Store, world, ship uuid.UUID, q, r, tick int) ([]*events.Event, error) {
	return record(ctx, tx, store, world, ship, Landed, nil, nil, "shore", q, r, tick)
}

func record(ctx context.Context, tx pgx.Tx, store *events.Store, world, ship uuid.UUID, kind string, rescue, port *uuid.UUID, reason string, q, r, tick int) ([]*events.Event, error) {
	rows, err := tx.Query(ctx, `SELECT m.id,m.sender_id,COALESCE(w.id,0),COALESCE(w.event_type='CarrierPassengerRescuedV1',false),
 jsonb_build_object('id',m.id,'sender_id',m.sender_id,'kind',m.kind,'sent_at',m.sent_at,'sent_tick',m.sent_tick,'message_text',m.message_text,'reply_text',m.reply_text,'trade_offer',m.trade_offer,'order_payload',m.order_payload,'origin_name',COALESCE(os.name,ou.name),'destination_name',ds.name,'origin',jsonb_build_object('id',COALESCE(m.origin_id,m.origin_unit_id),'name',COALESCE(os.name,ou.name),'q',COALESCE(op.map_q,m.origin_q),'r',COALESCE(op.map_r,m.origin_r)),'destination',jsonb_build_object('id',m.destination_id,'name',ds.name,'wanax_name',recipient.wanax_name,'unit_name',COALESCE(NULLIF(du.name,''),du.type),'own_unit',COALESCE(du.owner_id=m.sender_id,false),'q',COALESCE(m.dest_q,dp.map_q,m.hex_q),'r',COALESCE(m.dest_r,dp.map_r,m.hex_r)))
 FROM messengers m
 `+passengerJoinSQL+`
 LEFT JOIN settlements os ON os.id=m.origin_id
 LEFT JOIN provinces op ON op.id=os.province_id
 LEFT JOIN units ou ON ou.id=m.origin_unit_id
 LEFT JOIN settlements ds ON ds.id=m.destination_id
 LEFT JOIN provinces dp ON dp.id=ds.province_id
 LEFT JOIN units du ON m.kind='order' AND du.id::text=m.order_payload->>'unit_id' AND du.world_id=m.world_id
 LEFT JOIN players recipient ON recipient.id=COALESCE(du.owner_id,ds.owner_id)
  WHERE `+passengerMatchSQL+`
 ORDER BY m.id FOR UPDATE OF m`, world, ship)
	if err != nil {
		return nil, fmt.Errorf("read physical passengers: %w", err)
	}
	var passengers []Witness
	for rows.Next() {
		w := Witness{ShipID: ship, RescueShipID: rescue, PortID: port, Reason: reason, Q: q, R: r, Tick: tick}
		var rescued bool
		if err := rows.Scan(&w.MessengerID, &w.SenderID, &w.PreviousID, &rescued, &w.Envelope); err != nil {
			rows.Close()
			return nil, err
		}
		if kind == Landed && port == nil && rescued {
			continue
		}
		passengers = append(passengers, w)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var recorded []*events.Event
	for _, w := range passengers {
		if rescue != nil {
			if err := tx.QueryRow(ctx, `SELECT COALESCE(name,type) FROM units WHERE id=$1`, *rescue).Scan(&w.RescueShipName); err != nil {
				return nil, err
			}
		}
		if port != nil {
			if err := tx.QueryRow(ctx, `SELECT name FROM settlements WHERE id=$1`, *port).Scan(&w.PortName); err != nil {
				return nil, err
			}
		}
		e, err := store.AppendTx(ctx, tx, w.MessengerID, events.StreamType("messenger"), kind, w, world, nil)
		if err != nil {
			return nil, err
		}
		recorded = append(recorded, e)
	}
	return recorded, nil
}
