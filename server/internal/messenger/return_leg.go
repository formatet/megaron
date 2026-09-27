// Package messenger — the return leg (megaron_plan_ordna_passage.md, slice 3b-2).
package messenger

import (
	"context"
	"fmt"
	"time"

	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/province"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// StartReturnLegResult is StartReturnLeg's outcome.
type StartReturnLegResult struct {
	ReturnsAt time.Time
	Distance  int
	// PassageAwaiting is true when the return leg needs the sea-lift: the
	// messenger now stands 'awaiting_passage' at its port rather than actually
	// walking, and no terminal event has been scheduled yet — PassageScanHandler
	// does that once a carrier is boarded or the reserve is taken.
	PassageAwaiting bool
	// Started is false when the messenger was not 'delivered' at the claim —
	// already turned around (a reply raced a stay-end, or vice versa), already
	// home, or gone. Nothing was read or written in that case.
	Started bool
}

// StartReturnLeg turns a delivered messenger around and starts its journey
// home. It is the ONE place both a spoken Reply
// (api/handlers/messenger.go Reply) and an unanswered stay's end
// (ScheduledMessengerStayEnd, stay_end.go) do this, so the two can never
// disagree about how a return leg is planned, written or scheduled
// (megaron_plan_ordna_passage.md 3b-2 R2).
//
// replyText is nil when the stay simply ran out with no answer, or the
// recipient's words for a spoken Reply — the only difference between the two
// callers.
//
// Idempotent: claims the row FOR UPDATE inside its own transaction and
// no-ops (Started=false) unless status=='delivered' at claim time — the same
// claim-then-guard pattern ArrivalHandler and ReturnHandler already use
// (CLAUDE.md "Events: Idempotency").
func StartReturnLeg(ctx context.Context, pool *pgxpool.Pool, sched *events.Scheduler, worldID, messengerID uuid.UUID, now time.Time, currentTick int, replyText *string) (StartReturnLegResult, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return StartReturnLegResult{}, fmt.Errorf("start return leg: begin: %w", err)
	}
	defer tx.Rollback(ctx)

	var status string
	var destID uuid.UUID
	var originID *uuid.UUID
	var originQ, originR *int
	if err := tx.QueryRow(ctx,
		`SELECT status, destination_id, origin_id, origin_q, origin_r FROM messengers WHERE id = $1 FOR UPDATE`,
		messengerID,
	).Scan(&status, &destID, &originID, &originQ, &originR); err != nil {
		return StartReturnLegResult{}, nil // deleted or not found — silently skip, matching the sibling handlers
	}
	if status != "delivered" {
		return StartReturnLegResult{}, nil // no-op: already returning/arrived, or never delivered
	}

	var dQ, dR int
	if err := tx.QueryRow(ctx,
		`SELECT p.map_q, p.map_r FROM provinces p JOIN settlements s ON s.province_id = p.id WHERE s.id = $1`,
		destID,
	).Scan(&dQ, &dR); err != nil {
		return StartReturnLegResult{}, fmt.Errorf("start return leg: load destination coords: %w", err)
	}
	var oQ, oR int
	if originID != nil {
		if err := tx.QueryRow(ctx,
			`SELECT p.map_q, p.map_r FROM provinces p JOIN settlements s ON s.province_id = p.id WHERE s.id = $1`,
			*originID,
		).Scan(&oQ, &oR); err != nil {
			return StartReturnLegResult{}, fmt.Errorf("start return leg: load origin coords: %w", err)
		}
	} else if originQ != nil && originR != nil {
		oQ, oR = *originQ, *originR
	}

	// R6 (megaron_plan_budet_liftar.md): the return leg's port is wherever the
	// messenger already stands (destID) — no landward leg needed to reach it.
	returnsAt, dueTick, passage, sinceTick, err := ResolveReturnDeparture(
		ctx, tx, worldID, destID,
		province.MapPosition{Q: dQ, R: dR}, province.MapPosition{Q: oQ, R: oR}, now, currentTick)
	if err != nil {
		return StartReturnLegResult{}, fmt.Errorf("start return leg: resolve route: %w", err)
	}

	var passagePortID *uuid.UUID
	var passageSinceTickArg *int
	if passage != nil {
		passagePortID = &passage.SettlementID
		passageSinceTickArg = &sinceTick
	}

	// Give the return leg its own time window: return_departs_at = now,
	// arrives_at = the real homecoming (or, sea-lifted, the moment it starts
	// waiting at its port — now, since it already stands there). sent_at stays
	// untouched (the correspondence log keys on the original send).
	if _, err := tx.Exec(ctx,
		`UPDATE messengers SET reply_text = $1, status = 'returning',
		        return_departs_at = $2, arrives_at = $3,
		        passage_status = $5, passage_port_id = $6, passage_since_tick = $7
		  WHERE id = $4 AND status = 'delivered'`,
		replyText, now, returnsAt, messengerID, PassageStatusArg(passage), passagePortID, passageSinceTickArg,
	); err != nil {
		return StartReturnLegResult{}, fmt.Errorf("start return leg: update messenger: %w", err)
	}

	// passage == nil: a plain land return leg — schedule the real homecoming
	// now that its true travel time is known. passage != nil: no terminal
	// event yet — PassageScanHandler's scheduleCompletion schedules
	// ScheduledMessengerReturn once a carrier is boarded or the reserve is
	// taken (R6).
	if passage == nil {
		if err := sched.EnqueueTickTx(ctx, tx, worldID, events.ScheduledMessengerReturn,
			ReturnPayload{MessengerID: messengerID}, dueTick); err != nil {
			return StartReturnLegResult{}, fmt.Errorf("start return leg: schedule return: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return StartReturnLegResult{}, fmt.Errorf("start return leg: commit: %w", err)
	}

	return StartReturnLegResult{
		ReturnsAt:       returnsAt,
		Distance:        province.HexDistance(province.MapPosition{Q: dQ, R: dR}, province.MapPosition{Q: oQ, R: oR}),
		PassageAwaiting: passage != nil,
		Started:         true,
	}, nil
}
