package messenger

import (
	"context"
	"encoding/json"
	"fmt"

	"formatet/megaron/server/internal/carrier"
	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/province"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const MessengerLostAtSea = "MessengerLostAtSea"
const MessengerRescuedAtSea = "MessengerRescuedAtSea"

// Loss is the sole instant exception. The private physical witness also
// names ships/positions/predecessors; those are OTHER news and must not enter
// this sender dispatch (especially when an unknown rescuer later founders).
type LossNotice struct {
	MessengerID uuid.UUID       `json:"messenger_id"`
	SenderID    uuid.UUID       `json:"sender_wanax_id"`
	Reason      string          `json:"reason"`
	Tick        int             `json:"tick"`
	Envelope    json.RawMessage `json:"envelope"`
}

// Pending physical evidence invalidates old terminal timers even before the
// passage scan has projected it (storm priority precedes courier arrival).
func pendingCarrierWitness(ctx context.Context, db queryRower, id uuid.UUID) (bool, error) {
	var pending bool
	err := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM events e JOIN messengers m ON m.id=e.stream_id WHERE m.id=$1 AND e.id>m.carrier_witness_id AND e.event_type IN `+carrier.TypesSQL+`)`, id).Scan(&pending)
	return pending, err
}

func (h *PassageScanHandler) consumeCarrierWitnesses(ctx context.Context, world uuid.UUID, currentTick int) error {
	rows, err := h.pool.Query(ctx, `SELECT e.id,e.event_type,e.payload FROM events e JOIN messengers m ON m.id=e.stream_id WHERE m.world_id=$1 AND e.id>m.carrier_witness_id AND e.event_type IN `+carrier.TypesSQL+` ORDER BY e.id`, world)
	if err != nil {
		return err
	}
	type evidence struct {
		id   int64
		kind string
		raw  []byte
	}
	var all []evidence
	for rows.Next() {
		var e evidence
		if err := rows.Scan(&e.id, &e.kind, &e.raw); err != nil {
			rows.Close()
			return err
		}
		all = append(all, e)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, e := range all {
		var w carrier.Witness
		if err := json.Unmarshal(e.raw, &w); err != nil {
			return err
		}
		if err := h.applyCarrierWitness(ctx, world, currentTick, e.id, e.kind, w); err != nil {
			return err
		}
	}
	return nil
}

func (h *PassageScanHandler) applyCarrierWitness(ctx context.Context, world uuid.UUID, currentTick int, id int64, kind string, w carrier.Witness) error {
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var previous int64
	var status string
	var passage *string
	if err := tx.QueryRow(ctx, `SELECT carrier_witness_id,status,passage_status FROM messengers WHERE id=$1 FOR UPDATE`, w.MessengerID).Scan(&previous, &status, &passage); err != nil {
		return err
	}
	if previous >= id {
		return nil
	}
	if previous != w.PreviousID {
		return fmt.Errorf("carrier witness %d waits for predecessor %d (applied %d)", id, w.PreviousID, previous)
	}
	var noticeID string
	var lossNotice LossNotice
	if passage != nil && *passage == "aboard" && status != "lost" && status != "arrived" {
		switch kind {
		case carrier.Lost:
			if _, err := tx.Exec(ctx, `UPDATE messengers SET status='lost',passage_status=NULL,passage_port_id=NULL,passage_since_tick=NULL,passage_lost_until_tick=NULL,carrier_transport_id=NULL,carrier_unit_id=NULL,carrier_name=NULL,disembark_q=NULL,disembark_r=NULL,boarded_at=NULL,disembark_at=NULL,passage_generation=passage_generation+1 WHERE id=$1`, w.MessengerID); err != nil {
				return err
			}
			lossNotice = LossNotice{w.MessengerID, w.SenderID, w.Reason, w.Tick, w.Envelope}
			raw, err := json.Marshal(lossNotice)
			if err != nil {
				return err
			}
			if err := tx.QueryRow(ctx, `INSERT INTO notifications(world_id,player_id,kind,level,body_json) VALUES($1,$2,$3,2,$4) RETURNING id`, world, w.SenderID, MessengerLostAtSea, raw).Scan(&noticeID); err != nil {
				return err
			}
		case carrier.Rescued, carrier.Redirected:
			if w.RescueShipID == nil {
				return fmt.Errorf("rescue witness %d has no physical ship", id)
			}
			if _, err := tx.Exec(ctx, `UPDATE messengers SET carrier_transport_id=NULL,carrier_unit_id=$2,carrier_name=(SELECT name FROM units WHERE id=$2),disembark_q=NULL,disembark_r=NULL,disembark_at=NULL,passage_generation=passage_generation+1 WHERE id=$1`, w.MessengerID, *w.RescueShipID); err != nil {
				return err
			}
		case carrier.Landed:
			if w.PortID == nil && w.Reason != "shore" {
				return fmt.Errorf("landing witness %d has no port", id)
			}
			if err := h.landCarrierPassenger(ctx, tx, world, currentTick, w); err != nil {
				return err
			}
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE messengers SET carrier_witness_id=$2 WHERE id=$1`, w.MessengerID, id); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	// The archive row and projection committed together. A live hub only
	// forwards that row; retries cannot create another notification.
	if noticeID != "" {
		if delivery, ok := h.hub.(combat.CommittedNotificationDelivery); ok {
			delivery.DeliverCommittedNotification(ctx, world, w.SenderID, noticeID, MessengerLostAtSea, 2, lossNotice)
		}
	}
	return nil
}

func (h *PassageScanHandler) landCarrierPassenger(ctx context.Context, tx pgx.Tx, world uuid.UUID, currentTick int, w carrier.Witness) error {
	q, r, err := FinalTargetTx(ctx, tx, w.MessengerID)
	if err != nil {
		return err
	}
	// A rescued runner may use the foreign port it actually reached, just as
	// a returning runner does. No teleport to the sender's owned port.
	var route RouteDecision
	if w.PortID != nil {
		route, err = PlanReturnRoute(ctx, tx, world, *w.PortID, province.MapPosition{Q: w.Q, R: w.R}, province.MapPosition{Q: q, R: r})
	} else {
		route, err = PlanOutboundRoute(ctx, tx, world, w.SenderID, province.MapPosition{Q: w.Q, R: w.R}, province.MapPosition{Q: q, R: r})
	}
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE messengers SET passage_status='aboard',carrier_transport_id=NULL,carrier_unit_id=NULL,carrier_name=NULL,passage_port_id=COALESCE($5,passage_port_id),passage_since_tick=NULL,disembark_q=$2,disembark_r=$3,disembark_at=$4 WHERE id=$1`, w.MessengerID, w.Q, w.R, h.clk.Now(), w.PortID); err != nil {
		return err
	}
	if route.Mode == RouteLand {
		return scheduleCompletion(ctx, tx, h.scheduler, w.MessengerID, currentTick+route.Ticks, h.clk.Now().Add(route.Dur))
	}
	if !route.PortFound {
		return fmt.Errorf("landed runner %s has no reachable continuation port", w.MessengerID)
	}
	_, err = tx.Exec(ctx, `UPDATE messengers SET passage_status='awaiting_passage',passage_port_id=$2,passage_since_tick=$3,arrives_at=$4,passage_generation=passage_generation+1 WHERE id=$1`, w.MessengerID, route.Port.SettlementID, currentTick+route.LandTicks, h.clk.Now().Add(route.LandDur))
	return err
}

// A rescue is NOT instant news. Only a runner physically returning home can
// carry this frozen account back to its sender. The report and return commit
// together; replaying the return cannot add another archive row.
func persistRescueReport(ctx context.Context, tx pgx.Tx, world, messenger uuid.UUID) (string, uuid.UUID, json.RawMessage, error) {
	var sender uuid.UUID
	var body json.RawMessage
	err := tx.QueryRow(ctx, `SELECT m.sender_id,jsonb_build_object('messenger_id',m.id,'sender_wanax_id',m.sender_id,'journey',
 (SELECT jsonb_agg(jsonb_build_object('event_type',e.event_type,'ship',e.payload->>'rescue_ship_name','port',e.payload->>'port_name','tick',e.payload->'tick') ORDER BY e.id)
 FROM events e WHERE e.stream_id=m.id AND e.id<=m.carrier_witness_id AND e.event_type IN ('CarrierPassengerRescuedV1','CarrierPassengerLandedV1')))
 FROM messengers m WHERE m.id=$1 AND EXISTS(SELECT 1 FROM events e WHERE e.stream_id=m.id AND e.id<=m.carrier_witness_id AND e.event_type='CarrierPassengerRescuedV1')`, messenger).Scan(&sender, &body)
	if err == pgx.ErrNoRows {
		return "", uuid.Nil, nil, nil
	}
	if err != nil {
		return "", uuid.Nil, nil, err
	}
	var id string
	err = tx.QueryRow(ctx, `INSERT INTO notifications(world_id,player_id,kind,level,body_json) VALUES($1,$2,$3,3,$4) RETURNING id`, world, sender, MessengerRescuedAtSea, body).Scan(&id)
	return id, sender, body, err
}
