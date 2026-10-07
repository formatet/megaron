package messenger

import (
	"context"
	"errors"
	"fmt"

	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/unit"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const RecallGroupMemberVerb = "recall_group_member"

type RecallGroupMember struct {
	UnitID     uuid.UUID `json:"unit_id"`
	DepartTick *int      `json:"depart_tick"`
	ArriveTick *int      `json:"arrive_tick"`
	Route      []byte    `json:"route"`
}

type RecallGroup struct {
	Q       int                 `json:"q"`
	R       int                 `json:"r"`
	Members []RecallGroupMember `json:"members"`
}

// EnqueueRecallGroup schedules one independently receipted event per recipient,
// with a shared physical messenger. It also rebuilds every member after passage.
func EnqueueRecallGroup(ctx context.Context, tx pgx.Tx, sched *events.Scheduler, p OrderDeliveryPayload, dueTick int) error {
	if p.Verb != RecallGroupMemberVerb || p.RecallGroup == nil || len(p.RecallGroup.Members) == 0 {
		return fmt.Errorf("invalid recall group")
	}
	for _, m := range p.RecallGroup.Members {
		member := p
		member.UnitID = m.UnitID
		member.Recall = &combat.RecallOrder{WorldID: p.WorldID, UnitID: m.UnitID, Mode: "recall"}
		if err := sched.EnqueueTickTx(ctx, tx, p.WorldID, events.ScheduledOrderDelivery, member, dueTick); err != nil {
			return err
		}
	}
	return nil
}

func (h *OrderDeliveryHandler) recallGroupMember(ctx context.Context, e events.ScheduledEvent, p OrderDeliveryPayload) error {
	if p.RecallGroup == nil || p.Recall == nil || p.Recall.UnitID != p.UnitID || p.Recall.Mode != "recall" {
		return fmt.Errorf("invalid recall group member payload")
	}
	var member *RecallGroupMember
	for i := range p.RecallGroup.Members {
		if p.RecallGroup.Members[i].UnitID == p.UnitID {
			member = &p.RecallGroup.Members[i]
			break
		}
	}
	if member == nil {
		return fmt.Errorf("recipient absent from recall group")
	}
	var eligible bool
	if err := h.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM messengers WHERE id=$1 AND world_id=$2 AND sender_id=$3 AND passage_generation=$4 AND status IN ('outbound','arrived') AND (passage_status IS NULL OR passage_status='aboard'))`, p.MessengerID, p.WorldID, p.PlayerID, p.PassageGeneration).Scan(&eligible); err != nil {
		return err
	}
	if !eligible {
		return nil
	}
	var done bool
	if err := h.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM processed_deliveries WHERE event_id=$1)`, e.ID).Scan(&done); err != nil {
		return err
	}
	if done {
		return h.finishRecallMember(ctx, e, p)
	}
	result, err := combat.ExecuteRecallDelivery(ctx, h.pool, h.scheduler, h.eventStore, h.clk, *p.Recall, combat.RecallDelivery{
		EventID: e.ID, OwnerID: p.PlayerID, Meeting: province.MapPosition{Q: p.RecallGroup.Q, R: p.RecallGroup.R}, DepartTick: member.DepartTick, ArriveTick: member.ArriveTick, Route: member.Route,
	})
	if errors.Is(err, combat.ErrRecallDeliveryReplay) {
		return h.finishRecallMember(ctx, e, p)
	}
	if err != nil {
		var rejection *combat.OrderReject
		if !errors.As(err, &rejection) {
			return err
		}
		h.notifyOrderFailed(ctx, p, rejection.Reason)
	} else if result == nil {
		h.notifyOrderFailed(ctx, p, "the unit was no longer marching when the group Runner reached it")
	} else if h.hub != nil {
		_ = h.hub.NotifyPlayer(ctx, p.WorldID, p.PlayerID, "UnitRecalled", 3, map[string]any{"unit_id": p.UnitID, "name": unit.LoadDisplayName(ctx, h.pool, p.UnitID), "q": result.FromQ, "r": result.FromR, "target_q": result.NewTargetQ, "target_r": result.NewTargetR, "arrives_at": result.ArrivesAt})
	}
	return h.finishRecallMember(ctx, e, p)
}

func (h *OrderDeliveryHandler) finishRecallMember(ctx context.Context, e events.ScheduledEvent, p OrderDeliveryPayload) error {
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `INSERT INTO processed_deliveries(event_id) VALUES($1) ON CONFLICT DO NOTHING`, e.ID); err != nil {
		return err
	}
	// Serialise group completion, including concurrent members. Every delivery
	// stays separately receipted; the shared messenger completes after the last.
	var locked uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT id FROM messengers WHERE id=$1 FOR UPDATE`, p.MessengerID).Scan(&locked); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE messengers SET status='arrived',passage_status=NULL,carrier_transport_id=NULL,carrier_unit_id=NULL,carrier_name=NULL
  WHERE id=$1 AND passage_generation=$2 AND NOT EXISTS(
   SELECT 1 FROM scheduled_events se WHERE se.event_type=$3 AND se.payload->>'messenger_id'=$1::text AND se.payload->>'verb'=$4 AND COALESCE((se.payload->>'passage_generation')::int,0)=$2
   AND NOT EXISTS(SELECT 1 FROM processed_deliveries pd WHERE pd.event_id=se.id))`, p.MessengerID, p.PassageGeneration, string(events.ScheduledOrderDelivery), RecallGroupMemberVerb); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
