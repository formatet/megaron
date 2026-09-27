// Package messenger — "kalla tillbaka" (megaron_plan_ordna_passage.md,
// slice 3b-4, R5): the player's answer to a PassageStalled dispatch when the
// runner is stuck 'awaiting_passage' in the SENDER'S OWN port. The runner
// walks home over land — the same road it already walked to reach the port —
// and never delivers anything. A runner waiting in a FOREIGN port (the
// return-leg pickup case) cannot be called back this way: it has nothing to
// walk home FROM without first being carried, which is exactly what a
// call-back is meant to avoid ordering (ErrCallBackNotOwnPort).
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

// ErrCallBackNotOwnPort is CallBack's rejection for a runner waiting in a
// foreign port (R5: "bara för ett utgående bud i en egen hamn").
var ErrCallBackNotOwnPort = fmt.Errorf("runner is waiting in a foreign port — call-back only works for an outbound runner in your own port")

// ErrCallBackNotYours is CallBack's rejection when messengerID belongs to a
// different sender — a 403, not the generic 500 an unwrapped error would
// otherwise fall into at the HTTP layer.
var ErrCallBackNotYours = fmt.Errorf("not your runner")

// CallBackResult is CallBack's outcome.
type CallBackResult struct {
	ReturnsAt time.Time
	// Started is false when the messenger was not 'awaiting_passage' at the
	// claim — already boarded, already resolved, or never existed in that
	// state to begin with. Nothing was read or written in that case.
	Started bool
}

// CallBack turns a runner stuck 'awaiting_passage' in the sender's OWN port
// around, walking it home over land to its ORIGIN (never its target — it
// delivers nothing) — marked withdrawn=true so the home-arrival notice
// (ReturnHandler.notifyReturned) can say "came home undelivered"/"order
// withdrawn" instead of the ordinary no-reply text. Idempotent: claims the
// row FOR UPDATE and no-ops (Started=false) unless status=='outbound' &&
// passage_status=='awaiting_passage' at claim time — same pattern as
// StartReturnLeg (return_leg.go).
func CallBack(ctx context.Context, pool *pgxpool.Pool, sched *events.Scheduler, worldID, messengerID, callerID uuid.UUID, now time.Time, currentTick int) (CallBackResult, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return CallBackResult{}, fmt.Errorf("call back: begin: %w", err)
	}
	defer tx.Rollback(ctx)

	var senderID uuid.UUID
	var status string
	var passageStatus *string
	var passagePortID *uuid.UUID
	var originID *uuid.UUID
	var originQ, originR *int
	var generation int
	if err := tx.QueryRow(ctx,
		`SELECT sender_id, status, passage_status, passage_port_id, origin_id, origin_q, origin_r, passage_generation
		   FROM messengers WHERE id = $1 FOR UPDATE`,
		messengerID,
	).Scan(&senderID, &status, &passageStatus, &passagePortID, &originID, &originQ, &originR, &generation); err != nil {
		return CallBackResult{}, fmt.Errorf("call back: load messenger: %w", err)
	}
	if senderID != callerID {
		return CallBackResult{}, ErrCallBackNotYours
	}
	if status != "outbound" || passageStatus == nil || *passageStatus != "awaiting_passage" || passagePortID == nil {
		return CallBackResult{Started: false}, nil
	}

	var portOwnerID uuid.UUID
	var portQ, portR int
	if err := tx.QueryRow(ctx,
		`SELECT s.owner_id, p.map_q, p.map_r FROM settlements s JOIN provinces p ON p.id = s.province_id WHERE s.id = $1`,
		*passagePortID,
	).Scan(&portOwnerID, &portQ, &portR); err != nil {
		return CallBackResult{}, fmt.Errorf("call back: load port: %w", err)
	}
	if portOwnerID != callerID {
		return CallBackResult{}, ErrCallBackNotOwnPort
	}

	var oQ, oR int
	switch {
	case originID != nil:
		if err := tx.QueryRow(ctx,
			`SELECT p.map_q, p.map_r FROM provinces p JOIN settlements s ON s.province_id = p.id WHERE s.id = $1`,
			*originID,
		).Scan(&oQ, &oR); err != nil {
			return CallBackResult{}, fmt.Errorf("call back: load origin: %w", err)
		}
	case originQ != nil && originR != nil:
		oQ, oR = *originQ, *originR
	default:
		return CallBackResult{}, fmt.Errorf("call back: runner has no known origin to return to")
	}

	// The runner already walked this exact road (port to origin, over land —
	// ResolveDeparture never writes an 'awaiting_passage' row without a real
	// landward leg to the port) to get here, so a route home is guaranteed;
	// !ok would be an internal inconsistency, not a player-facing case.
	ticks, dur, ok, cErr := CourierTravel(ctx, tx, worldID,
		province.MapPosition{Q: portQ, R: portR}, province.MapPosition{Q: oQ, R: oR})
	if cErr != nil {
		return CallBackResult{}, fmt.Errorf("call back: route home: %w", cErr)
	}
	if !ok {
		return CallBackResult{}, fmt.Errorf("call back: no land route from the port back to the runner's own origin, despite it having walked that same road to get here")
	}
	returnsAt := now.Add(dur)
	dueTick := currentTick + ticks

	if _, err := tx.Exec(ctx,
		`UPDATE messengers
		    SET status = 'returning', withdrawn = true,
		        return_departs_at = $2, arrives_at = $3,
		        passage_status = NULL, passage_port_id = NULL, passage_since_tick = NULL,
		        passage_stalled_notified_tick = NULL
		  WHERE id = $1 AND status = 'outbound' AND passage_status = 'awaiting_passage'`,
		messengerID, now, returnsAt,
	); err != nil {
		return CallBackResult{}, fmt.Errorf("call back: update messenger: %w", err)
	}
	if err := sched.EnqueueTickTx(ctx, tx, worldID, events.ScheduledMessengerReturn,
		ReturnPayload{MessengerID: messengerID, PassageGeneration: generation}, dueTick,
	); err != nil {
		return CallBackResult{}, fmt.Errorf("call back: schedule return: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return CallBackResult{}, fmt.Errorf("call back: commit: %w", err)
	}
	return CallBackResult{ReturnsAt: returnsAt, Started: true}, nil
}
