package messenger

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"formatet/megaron/server/internal/carrier"
	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/tick"
	"formatet/megaron/server/internal/unit"
	"github.com/jackc/pgx/v5"
)

// advanceFollowingOrder owns the atomic decision to claim, walk another leg, or
// wait at the reached hex. It never executes an order. Only the new explicit
// opt-in comes here; the old handler and old envelopes retain their semantics.
func (h *OrderDeliveryHandler) advanceFollowingOrder(ctx context.Context, p *OrderDeliveryPayload) (deliver bool, failure string, err error) {
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return false, "", err
	}
	defer tx.Rollback(ctx)
	var at province.MapPosition
	var deadline time.Time
	err = tx.QueryRow(ctx, `SELECT dest_q,dest_r,arrives_at FROM messengers
 WHERE id=$1 AND world_id=$2 AND sender_id=$3 AND kind='order' AND status='outbound'
 AND passage_generation=$4 AND (passage_status IS NULL OR passage_status='aboard')
 AND NOT EXISTS(SELECT 1 FROM events e WHERE e.stream_id=messengers.id AND e.id>messengers.carrier_witness_id AND e.event_type IN `+carrier.TypesSQL+`)
 FOR UPDATE`, p.MessengerID, p.WorldID, p.PlayerID, p.PassageGeneration).Scan(&at.Q, &at.R, &deadline)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, "", nil
	}
	if err != nil {
		return false, "", err
	}
	now := h.clk.Now()
	if now.Before(deadline) {
		return false, "", nil
	}
	u := &unit.Unit{ID: p.UnitID}
	err = tx.QueryRow(ctx, `SELECT world_id,owner_id,type,status,settlement_id,q,r,target_q,target_r,departs_at,arrives_at,depart_tick,arrive_tick,march_route
 FROM units WHERE id=$1 FOR SHARE`, p.UnitID).Scan(&u.WorldID, &u.OwnerID, &u.Type, &u.Status, &u.SettlementID, &u.Q, &u.R, &u.TargetQ, &u.TargetR, &u.DepartsAt, &u.ArrivesAt, &u.DepartTick, &u.ArriveTick, &u.MarchRoute)
	if errors.Is(err, pgx.ErrNoRows) {
		failure = "the Runner could not find this unit — it no longer exists"
	} else if err != nil {
		return false, "", err
	} else if u.OwnerID != p.PlayerID || u.WorldID != p.WorldID {
		failure = "this unit is no longer yours"
	} else if u.Status == unit.StatusDisbanded {
		failure = "the Runner could not find this unit — it has been disbanded"
	}
	claim := func() error {
		_, err := tx.Exec(ctx, `UPDATE messengers SET status='arrived',passage_status=NULL,carrier_transport_id=NULL,carrier_unit_id=NULL,carrier_name=NULL WHERE id=$1`, p.MessengerID)
		if err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	if failure != "" {
		return true, failure, claim()
	}
	if unit.CategoryOf(u.Type) != unit.CategoryLand {
		return true, "the Runner cannot follow a ship at sea", claim()
	}
	// Embarked/temporarily unlocated living units are not reported dead or missed.
	// Keep the Runner at the reached hex and retry physically from there later.
	if u.Status == unit.StatusEmbarked {
		return false, "", h.waitFollowingOrderTx(ctx, tx, p, at, now)
	}
	pos, err := combat.CurrentUnitPosition(ctx, tx, h.clk, u)
	if err != nil {
		var rej *combat.OrderReject
		if errors.As(err, &rej) {
			return false, "", h.waitFollowingOrderTx(ctx, tx, p, at, now)
		}
		return false, "", err
	}
	if pos == at {
		p.FollowWaiting = false
		if p.March != nil {
			p.March.ExpectedPosition = &pos
			p.March.Delivered = true
		}
		if p.AfterArrival != nil {
			p.AfterArrival.ExpectedPosition = &pos
			p.AfterArrival.Delivered = true
			p.MeetAtDestination = false
		}
		return true, "", claim()
	}
	target := pos
	if u.Status == unit.StatusMarching && u.TargetQ != nil && u.TargetR != nil {
		var intercepted bool
		if route, ok := combat.LoadActiveRoute(u.MarchRoute, string(u.Status), u.DepartTick, u.ArriveTick); ok {
			a, e := tick.LoadAnchor(ctx, tx, p.WorldID)
			if e != nil {
				return false, "", e
			}
			target, intercepted, err = InterceptCourierTargetRoute(ctx, tx, p.WorldID, at, route, a, now)
		} else if u.Q != nil && u.R != nil && u.DepartsAt != nil && u.ArrivesAt != nil {
			target, intercepted, err = InterceptCourierTarget(ctx, tx, p.WorldID, at, province.MapPosition{Q: *u.Q, R: *u.R}, province.MapPosition{Q: *u.TargetQ, R: *u.TargetR}, unit.MoveCategory(u.Type), *u.DepartsAt, *u.ArrivesAt, now)
		}
		if err != nil {
			return false, "", err
		}
		if !intercepted {
			target = province.MapPosition{Q: *u.TargetQ, R: *u.TargetR}
		}
	}
	a, err := tick.LoadAnchor(ctx, tx, p.WorldID)
	if err != nil {
		return false, "", err
	}
	arrival, due, passage, since, err := ResolveDeparture(ctx, tx, p.WorldID, p.PlayerID, at, target, now, a.Tick)
	if errors.Is(err, ErrNoPort) {
		return false, "", h.waitFollowingOrderTx(ctx, tx, p, at, now)
	}
	if err != nil {
		return false, "", err
	}
	p.PursuitLegs++
	p.FollowWaiting = false
	var port any
	var sinceArg any
	landTarget := target
	if passage != nil {
		port = passage.SettlementID
		sinceArg = since
		landTarget = province.MapPosition{Q: passage.Q, R: passage.R}
	}
	// The original sender/sent_at stay intact. The new leg's geometry and clock
	// supply the current physical origin, including after a carrier's landing.
	if _, err := tx.Exec(ctx, `UPDATE messengers SET hex_q=$2,hex_r=$3,dest_q=$4,dest_r=$5,arrives_at=$6,order_payload=$7,
 passage_status=$8,passage_port_id=$9,passage_since_tick=$10,passage_stalled_notified_tick=NULL,
 carrier_transport_id=NULL,carrier_unit_id=NULL,carrier_name=NULL,disembark_q=NULL,disembark_r=NULL,boarded_at=NULL,disembark_at=NULL
 WHERE id=$1`, p.MessengerID, at.Q, at.R, target.Q, target.R, arrival, mustPursuitJSON(p), PassageStatusArg(passage), port, sinceArg); err != nil {
		return false, "", err
	}
	if err := SaveLandRouteTx(ctx, tx, p.WorldID, p.MessengerID, at, landTarget, now, arrival); err != nil {
		return false, "", err
	}
	if passage == nil {
		if err := EnqueueLandTx(ctx, tx, h.scheduler, p.WorldID, events.ScheduledOrderDelivery, p, due); err != nil {
			return false, "", err
		}
	}
	return false, "", tx.Commit(ctx)
}

func mustPursuitJSON(p *OrderDeliveryPayload) []byte { raw, _ := json.Marshal(p); return raw }

// Waiting is not travel and grants no new eye. Retain the reached leg so every
// map/vision reader holds the same endpoint; only the explicit retry job changes.
func (h *OrderDeliveryHandler) waitFollowingOrderTx(ctx context.Context, tx pgx.Tx, p *OrderDeliveryPayload, at province.MapPosition, now time.Time) error {
	a, err := tick.LoadAnchor(ctx, tx, p.WorldID)
	if err != nil {
		return err
	}
	arrival := now.Add(time.Duration(tick.TickSeconds) * time.Second)
	if err := tx.QueryRow(ctx, `UPDATE messengers SET passage_generation=passage_generation+1,arrives_at=$2,hex_q=$3,hex_r=$4,
 passage_status=NULL,carrier_transport_id=NULL,carrier_unit_id=NULL,carrier_name=NULL
 WHERE id=$1 RETURNING passage_generation`, p.MessengerID, arrival, at.Q, at.R).Scan(&p.PassageGeneration); err != nil {
		return err
	}
	p.FollowWaiting = true
	if _, err := tx.Exec(ctx, `UPDATE messengers SET order_payload=$2 WHERE id=$1`, p.MessengerID, mustPursuitJSON(p)); err != nil {
		return err
	}
	if err := h.scheduler.EnqueueDeadlineTx(ctx, tx, p.WorldID, events.ScheduledOrderDeliveryV2, p, a.Tick+1, arrival); err != nil {
		return fmt.Errorf("schedule following wait: %w", err)
	}
	return tx.Commit(ctx)
}
