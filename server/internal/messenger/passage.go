// Package messenger — budet liftar (megaron_plan_budet_liftar.md, slice 3a).
//
// A messenger whose route needs sea used to cross it instantly at the flat
// abstract province.CourierSeaTicks rate (the "abstract boat"). This file
// makes that crossing physical: the messenger runs to its own nearest
// coastal/harbour port (a real landward leg, no sea involved) and waits there
// (passage_status='awaiting_passage'). It boards an eligible carrier — a real
// naval transport or a ship mission departing that port, owned by the
// messenger's sender — when one leaves (passage_status='aboard'), then
// disembarks at the carrier's own destination and runs the rest of the way on
// land. A carrier lost to interception seals the messenger back to its port
// after a short delay (R4). A messenger no real carrier picks up in time is a
// PLAYER decision now, not a mechanic — see the RESERVE's removal note below
// (R5, slice 3b-4).
//
// Invariant (R2/R4): boarding never changes the CARRIER's own order, and a
// lost carrier never loses or reveals the messenger's contents — only delays
// it. A messenger is never killed or read by this mechanic.
//
// The RESERVE — the old abstract crossing a messenger with no real carrier
// took after a short wait — is GONE (megaron_plan_ordna_passage.md, slice
// 3b-4, R1): a messenger with no ship never crosses on its own. A Wanax
// decides what happens next, through a PassageStalled dispatch (R5) — arrange
// passage, call the runner back, or let it wait.
package messenger

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/tick"
	"formatet/megaron/server/internal/unit"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Strawman timings (Timothy 2026-09-26 pattern: "okalibrerat, justeras med
// prissättningen" — same spirit as transport.ShipCapacityFor's constants).
const (
	// PassageStallNoticeTicks is 3b-4's R5: how long a messenger waits
	// 'awaiting_passage' with no eligible carrier before its sender gets a
	// PassageStalled dispatch — strawman, uncalibrated, same spirit as
	// transport.ShipCapacityFor's own constants (Timothy 2026-09-26 pattern).
	// Unlike the reserve this replaced, nothing happens automatically once
	// this elapses — the dispatch is offered, not a deadline.
	PassageStallNoticeTicks = 3
	// PassageLostDelayTicks is R4: how long a messenger whose carrier was
	// captured/limped/sunk stays sealed before it reappears 'awaiting_passage'
	// at its port.
	PassageLostDelayTicks = 2
	// PassageScanIntervalTicks is how often ScheduledPassageScan re-fires —
	// same cadence as transport's own InterceptScan.
	PassageScanIntervalTicks = 1
	// passageDepartureWindow is how far back the boarding phase looks for a
	// naval transport's departs_at (a wall-clock timestamp; transports carry
	// no integer depart-tick column). A transport is only ever "just
	// departed" for one scan interval, so this window — converted through the
	// same tick.RealUntil every other ETA uses — makes each transport
	// reachable by exactly the scans that follow its dispatch, with a small
	// (strawman-acceptable) grace rather than an exact instant.
	passageDepartureWindowTicks = PassageScanIntervalTicks
)

// RouteMode is PlanOutboundRoute/PlanReturnRoute's verdict.
type RouteMode string

const (
	RouteLand RouteMode = "land" // a pure land-only courier route exists — unaffected by this file.
	RouteSea  RouteMode = "sea"  // no land-only route — the sea-lift mechanic applies (if a port was found).
)

// PassagePort is the port a sea-bound messenger runs to and waits at.
type PassagePort struct {
	SettlementID uuid.UUID
	Q, R         int
}

// PassageStatusArg is the literal to write into messengers.passage_status at
// INSERT time: "awaiting_passage" when ResolveDeparture/ResolveReturnDeparture
// returned a port, nil (plain SQL NULL) otherwise. A tiny helper so every
// dispatcher writes the same literal rather than repeating the string.
func PassageStatusArg(passage *PassagePort) *string {
	if passage == nil {
		return nil
	}
	s := "awaiting_passage"
	return &s
}

// RouteDecision is PlanOutboundRoute/PlanReturnRoute's result.
type RouteDecision struct {
	Mode RouteMode
	// Land: full travel ticks/duration from→to, exactly as CourierTravel would
	// report — R1's "as today" is literal: unchanged for any route with a land
	// alternative, even one CategoryCourier would itself cross a short sea hex
	// on (it may still be faster).
	Ticks int
	Dur   time.Duration
	// Sea: PortFound tells the caller whether an owned/standing port exists at
	// all. false means take the RESERVE immediately (no port to wait at) —
	// caller falls back to the plain CourierTravel(from,to) crossing, exactly
	// as before this slice; this is a documented, narrow edge case (an owner
	// with literally no reachable coastal/harbour settlement at all).
	PortFound bool
	Port      PassagePort
	// LandTicks/LandDur: the landward leg from `from` to the port (0 for the
	// return leg, where the messenger already stands at its port).
	LandTicks int
	LandDur   time.Duration
}

// PlanOutboundRoute decides whether a messenger dispatched by ownerID from
// `from` toward `to` needs the sea-lift mechanic (R1: "vid varje avsändande").
// Mode==RouteLand: proceed exactly as before this slice. Mode==RouteSea &&
// PortFound: run the landward leg to Port and wait there. Mode==RouteSea &&
// !PortFound: no reachable port at all — ResolveDeparture reports ErrNoPort
// (3b-4 R2; the old reserve fallback is gone).
func PlanOutboundRoute(ctx context.Context, db province.Queryer, worldID, ownerID uuid.UUID, from, to province.MapPosition) (RouteDecision, error) {
	_, _, landOK, err := province.FindPath(ctx, db, worldID, from, to, province.CategoryCourierLand)
	if err != nil {
		return RouteDecision{}, fmt.Errorf("plan outbound route: land check: %w", err)
	}
	if landOK {
		// CourierTravel (plain CategoryCourier, river still boatable —
		// megaron_floden_plan.md, untouched by 3b-4) rather than the
		// CategoryCourierLand cost above: a route with no sea alternative may
		// still shortcut a river by boat, exactly as before this slice — only
		// SEA shortcuts are gone (R3). landOK guarantees a CategoryCourierLand
		// path exists, and CategoryCourier permits everything that does plus
		// rivers, so ok=false here would be an internal inconsistency, not a
		// player-facing case.
		ticks, dur, ok, cErr := CourierTravel(ctx, db, worldID, from, to)
		if cErr != nil {
			return RouteDecision{}, fmt.Errorf("plan outbound route: land ticks: %w", cErr)
		}
		if !ok {
			return RouteDecision{}, fmt.Errorf("plan outbound route: land route found but courier travel time could not be computed")
		}
		return RouteDecision{Mode: RouteLand, Ticks: ticks, Dur: dur}, nil
	}

	ports, err := ownedCoastalPorts(ctx, db, worldID, ownerID)
	if err != nil {
		return RouteDecision{}, fmt.Errorf("plan outbound route: load owned ports: %w", err)
	}
	best := -1
	var bestTicks int
	var bestDur time.Duration
	for i, p := range ports {
		_, cost, ok, ferr := province.FindPath(ctx, db, worldID, from, province.MapPosition{Q: p.Q, R: p.R}, province.CategoryCourierLand)
		if ferr != nil {
			return RouteDecision{}, fmt.Errorf("plan outbound route: path to port: %w", ferr)
		}
		if !ok {
			continue
		}
		ticks, dur := ticksFromHours(cost)
		if best < 0 || ticks < bestTicks {
			best, bestTicks, bestDur = i, ticks, dur
		}
	}
	if best < 0 {
		return RouteDecision{Mode: RouteSea, PortFound: false}, nil
	}
	return RouteDecision{
		Mode: RouteSea, PortFound: true,
		Port:      PassagePort{SettlementID: ports[best].ID, Q: ports[best].Q, R: ports[best].R},
		LandTicks: bestTicks, LandDur: bestDur,
	}, nil
}

// PlanReturnRoute is PlanOutboundRoute's return-leg twin (R6): the port is
// wherever the messenger already stands (standingAt) — no landward leg is
// needed to reach it, since it is already there.
func PlanReturnRoute(ctx context.Context, db province.Queryer, worldID uuid.UUID, standingAt uuid.UUID, from, to province.MapPosition) (RouteDecision, error) {
	_, _, landOK, err := province.FindPath(ctx, db, worldID, from, to, province.CategoryCourierLand)
	if err != nil {
		return RouteDecision{}, fmt.Errorf("plan return route: land check: %w", err)
	}
	if landOK {
		// See PlanOutboundRoute's own comment: plain CourierTravel, not the
		// CategoryCourierLand cost — a river shortcut is still fine, only sea
		// shortcuts are gone (3b-4 R3).
		ticks, dur, ok, cErr := CourierTravel(ctx, db, worldID, from, to)
		if cErr != nil {
			return RouteDecision{}, fmt.Errorf("plan return route: land ticks: %w", cErr)
		}
		if !ok {
			return RouteDecision{}, fmt.Errorf("plan return route: land route found but courier travel time could not be computed")
		}
		return RouteDecision{Mode: RouteLand, Ticks: ticks, Dur: dur}, nil
	}
	return RouteDecision{
		Mode: RouteSea, PortFound: true,
		Port: PassagePort{SettlementID: standingAt, Q: from.Q, R: from.R},
	}, nil
}

// ticksFromHours mirrors CourierTravelOnGraph's own rounding (>=1 tick).
func ticksFromHours(hours float64) (int, time.Duration) {
	t := int(hours + 0.5)
	if t < 1 {
		t = 1
	}
	return t, tick.RealUntil(t, 0)
}

// portCandidate is one of ownerID's own active settlements eligible as a
// sea-lift boarding port: coastal, or harboured (settlementHasHarbour's own
// two-part gate, reproduced here — messenger may not import api/handlers).
type portCandidate struct {
	ID   uuid.UUID
	Q, R int
}

func ownedCoastalPorts(ctx context.Context, db province.Queryer, worldID, ownerID uuid.UUID) ([]portCandidate, error) {
	rows, err := db.Query(ctx,
		`SELECT s.id, p.map_q, p.map_r
		   FROM settlements s JOIN provinces p ON p.id = s.province_id
		  WHERE s.world_id = $1 AND s.owner_id = $2 AND s.state = 'active'
		    AND (COALESCE(p.coastal, false)
		         OR EXISTS(SELECT 1 FROM buildings b WHERE b.settlement_id = s.id AND b.building_type = 'harbour'))`,
		worldID, ownerID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []portCandidate
	for rows.Next() {
		var c portCandidate
		if err := rows.Scan(&c.ID, &c.Q, &c.R); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ErrNoPort is 3b-4's R2: an outbound route needs the sea and the sender has
// no reachable coastal/harboured settlement of their own AT ALL — there is no
// port to even wait at. Before this slice that fell back to the RESERVE (the
// abstract crossing, R1, now gone); now it is a visible, named rejection at
// send time instead of a silent success — every caller (Send, SendFromHost,
// sendOrderCourier) turns this into a 422 with an explaining sentence, never
// dispatches a messenger that can never reach a port.
var ErrNoPort = fmt.Errorf("no port of yours to take ship from — a runner cannot cross the sea without a ship")

// ResolveDeparture is the single entry point every outbound dispatcher (Send,
// SendFromHost, sendOrderCourier) calls in place of a bare CourierTravel, per
// R1's "alla anropare ska gå via den nya mekaniken". passage==nil: the caller
// proceeds exactly as before (schedule the terminal delivery event at
// dueTick/arrivesAt). passage!=nil: the caller instead writes an
// 'awaiting_passage' row (arrivesAt/dueTick are the LANDWARD leg to the port)
// and does NOT schedule the terminal event — the passage scan does that once
// a carrier is boarded. err wraps ErrNoPort (errors.Is) when the route needs
// the sea and no port exists at all — see ErrNoPort's own doc comment.
func ResolveDeparture(ctx context.Context, db province.Queryer, worldID, ownerID uuid.UUID, from, to province.MapPosition, now time.Time, currentTick int) (arrivesAt time.Time, dueTick int, passage *PassagePort, sinceTick int, err error) {
	rd, err := PlanOutboundRoute(ctx, db, worldID, ownerID, from, to)
	if err != nil {
		return time.Time{}, 0, nil, 0, err
	}
	if rd.Mode == RouteSea && !rd.PortFound {
		return time.Time{}, 0, nil, 0, ErrNoPort
	}
	if rd.Mode == RouteLand {
		return now.Add(rd.Dur), currentTick + rd.Ticks, nil, 0, nil
	}
	sinceTick = currentTick + rd.LandTicks
	return now.Add(rd.LandDur), sinceTick, &rd.Port, sinceTick, nil
}

// ResolveReturnDeparture is Reply's counterpart to ResolveDeparture (R6).
func ResolveReturnDeparture(ctx context.Context, db province.Queryer, worldID uuid.UUID, standingAt uuid.UUID, from, to province.MapPosition, now time.Time, currentTick int) (arrivesAt time.Time, dueTick int, passage *PassagePort, sinceTick int, err error) {
	rd, err := PlanReturnRoute(ctx, db, worldID, standingAt, from, to)
	if err != nil {
		return time.Time{}, 0, nil, 0, err
	}
	if rd.Mode == RouteLand {
		return now.Add(rd.Dur), currentTick + rd.Ticks, nil, 0, nil
	}
	return now, currentTick, &rd.Port, currentTick, nil
}

// PassageShipReleaser is the thin, downward-only capability messenger needs
// from combat to run R4's release phase (megaron_plan_ordna_passage.md
// 3b-3): send a passage_wait ship home. Defined here, in the CONSUMING
// package, per CLAUDE.md G1 ("consumer interfaces are defined in the
// consuming package, never in the implementing one") — combat.
// UnitArrivalHandler satisfies it without messenger ever being imported by
// combat.
type PassageShipReleaser interface {
	ReleasePassageWaitShip(ctx context.Context, tx pgx.Tx, shipID, worldID uuid.UUID) error
}

// PassageScanHandler drives boarding, disembark-scheduling, the R5 reserve
// fallback, R4's carrier-loss recovery and 3b-3's passage_wait release. One
// self-perpetuating instance per world (ScheduledPassageScan), same shape as
// transport.InterceptScanHandler.
type PassageScanHandler struct {
	pool      *pgxpool.Pool
	scheduler *events.Scheduler
	hub       combat.Broadcaster
	clk       clock.Clock
	ships     PassageShipReleaser
}

// NewPassageScanHandler creates a PassageScanHandler.
func NewPassageScanHandler(pool *pgxpool.Pool, sched *events.Scheduler, hub combat.Broadcaster, clk clock.Clock, ships PassageShipReleaser) *PassageScanHandler {
	return &PassageScanHandler{pool: pool, scheduler: sched, hub: hub, clk: clk, ships: ships}
}

// Handle runs the phases in order (each independent — a failure in one is
// logged, never aborts the others) then re-enqueues itself. releasePassageWait
// runs BEFORE boardShipMissions (megaron_plan_ordna_passage.md 3b-3 R4): a
// ship released this exact call gets depart_tick = currentTick, so
// boardShipMissions — later in this SAME call — boards its runner in the
// very same scan, regardless of which earlier tick the ship actually parked
// 'passage_wait' at.
func (h *PassageScanHandler) Handle(ctx context.Context, e events.ScheduledEvent) error {
	var currentTick int
	_ = h.pool.QueryRow(ctx, `SELECT current_world_tick()`).Scan(&currentTick)

	if err := h.promoteSealed(ctx, e.WorldID, currentTick); err != nil {
		slog.Error("passage scan: promote sealed", "err", err)
	}
	if err := h.notifyStalled(ctx, e.WorldID, currentTick); err != nil {
		slog.Error("passage scan: notify stalled", "err", err)
	}
	if err := h.releasePassageWait(ctx, e.WorldID, currentTick); err != nil {
		slog.Error("passage scan: release passage wait", "err", err)
	}
	if err := h.boardTransports(ctx, e.WorldID, currentTick); err != nil {
		slog.Error("passage scan: board transports", "err", err)
	}
	if err := h.boardShipMissions(ctx, e.WorldID, currentTick); err != nil {
		slog.Error("passage scan: board ship missions", "err", err)
	}
	if err := h.detectLostCarriers(ctx, e.WorldID, currentTick); err != nil {
		slog.Error("passage scan: detect lost carriers", "err", err)
	}

	return h.scheduler.EnqueueTickRecurring(ctx, e.WorldID, events.ScheduledPassageScan,
		struct{}{}, e.DueTick, PassageScanIntervalTicks)
}

// releasePassageWait is R4: a ship holding status='positioned', march_intent=
// 'passage_wait' is sent home the moment its arranged runner either (a)
// stands 'awaiting_passage' at the settlement adjacent to the ship's own hex
// (ready to travel with it), or (b) is done or gone (arrived, or the row is
// simply missing). Both conditions are read fresh every scan — no caching —
// same SKIP LOCKED claim idiom as every other phase in this file.
func (h *PassageScanHandler) releasePassageWait(ctx context.Context, worldID uuid.UUID, currentTick int) error {
	rows, err := h.pool.Query(ctx,
		`SELECT id FROM units
		  WHERE world_id = $1 AND status = 'positioned' AND march_intent = 'passage_wait'
		  FOR UPDATE SKIP LOCKED`,
		worldID,
	)
	if err != nil {
		return err
	}
	var shipIDs []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		shipIDs = append(shipIDs, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, id := range shipIDs {
		if err := h.releasePassageWaitOne(ctx, worldID, id, currentTick); err != nil {
			slog.Error("passage scan: release passage wait ship", "unit", id, "err", err)
		}
	}
	return nil
}

func (h *PassageScanHandler) releasePassageWaitOne(ctx context.Context, worldID, shipID uuid.UUID, currentTick int) error {
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var shipQ, shipR int
	var msgID *uuid.UUID
	if err := tx.QueryRow(ctx,
		`SELECT q, r, passage_messenger_id FROM units
		  WHERE id = $1 AND status = 'positioned' AND march_intent = 'passage_wait' FOR UPDATE`,
		shipID,
	).Scan(&shipQ, &shipR, &msgID); err != nil {
		return nil // already resolved by a racing pass
	}

	release, err := passageWaitShouldRelease(ctx, tx, worldID, shipQ, shipR, msgID)
	if err != nil {
		return err
	}
	if !release {
		return nil // still waiting — nothing to do, no write made
	}

	if h.ships == nil {
		return fmt.Errorf("release passage wait ship: no PassageShipReleaser configured")
	}
	if err := h.ships.ReleasePassageWaitShip(ctx, tx, shipID, worldID); err != nil {
		return fmt.Errorf("release passage wait ship: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	slog.Info("passage: passage_wait ship released", "unit", shipID)
	return nil
}

// passageWaitShouldRelease is R4's two release conditions: (a) the runner is
// ready — 'awaiting_passage' at the settlement adjacent to the ship's own
// hex — or (b) the runner is done or gone. A missing runner row (defensive:
// dispatch always sets passage_messenger_id) also releases — never strand a
// ship over a data inconsistency.
func passageWaitShouldRelease(ctx context.Context, tx pgx.Tx, worldID uuid.UUID, shipQ, shipR int, msgID *uuid.UUID) (bool, error) {
	if msgID == nil {
		return true, nil
	}
	var status string
	var passageStatus *string
	var passagePortID *uuid.UUID
	if err := tx.QueryRow(ctx,
		`SELECT status, passage_status, passage_port_id FROM messengers WHERE id = $1`, *msgID,
	).Scan(&status, &passageStatus, &passagePortID); err != nil {
		return true, nil // row gone — nothing left to wait for
	}
	if status == "arrived" {
		return true, nil
	}
	if passageStatus == nil || *passageStatus != "awaiting_passage" || passagePortID == nil {
		return false, nil
	}
	portAtShip, _, _, portFound, err := province.NearestSettlementNeighbor(ctx, tx, worldID, shipQ, shipR)
	if err != nil {
		return false, err
	}
	return portFound && portAtShip == *passagePortID, nil
}

// promoteSealed is R4's delay expiring: a messenger sealed by a lost carrier
// becomes 'awaiting_passage' again at its (unchanged) port once
// passage_lost_until_tick is reached — restarting the R5 stall clock: this is
// a NEW waiting spell, so passage_stalled_notified_tick resets to NULL and a
// fresh PassageStalled dispatch may fire for it ("efter en ny försegling").
func (h *PassageScanHandler) promoteSealed(ctx context.Context, worldID uuid.UUID, currentTick int) error {
	_, err := h.pool.Exec(ctx,
		`UPDATE messengers
		    SET passage_status = 'awaiting_passage', passage_since_tick = $2,
		        passage_lost_until_tick = NULL, passage_stalled_notified_tick = NULL
		  WHERE world_id = $1 AND passage_status = 'returning_sealed'
		    AND passage_lost_until_tick IS NOT NULL AND passage_lost_until_tick <= $2`,
		worldID, currentTick,
	)
	return err
}

// stalledEligibleShip is one of the sender's own ships a PassageStalled
// dispatch can point at as a real "arrange passage" choice — same shape as
// api/handlers.eligiblePassageShip, duplicated here rather than imported
// (CLAUDE.md G1: messenger sits BELOW api/handlers, so the import must never
// go the other way).
type stalledEligibleShip struct {
	ID             uuid.UUID `json:"id"`
	Name           string    `json:"name"`
	Type           string    `json:"type"`
	SettlementID   uuid.UUID `json:"settlement_id"`
	SettlementName string    `json:"settlement_name"`
}

// stalledEligibleShips mirrors api/handlers.eligiblePassageShips' own R1
// predicate: the sender's own idle galleys/merchantmen, docked at exactly the
// waiting port if it is the sender's OWN port, or at any of the sender's own
// ports for a foreign-port pickup.
func stalledEligibleShips(ctx context.Context, db province.Queryer, worldID, senderID, portID uuid.UUID, ownPort bool) ([]stalledEligibleShip, error) {
	query := `SELECT u.id, u.name, u.type, u.settlement_id, s.name
	            FROM units u JOIN settlements s ON s.id = u.settlement_id
	           WHERE u.world_id = $1 AND u.owner_id = $2 AND u.status = 'garrison'
	             AND u.type IN ('galley', 'merchantman')`
	args := []any{worldID, senderID}
	if ownPort {
		query += ` AND u.settlement_id = $3`
		args = append(args, portID)
	}
	rows, err := db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []stalledEligibleShip
	for rows.Next() {
		var s stalledEligibleShip
		var name *string
		if err := rows.Scan(&s.ID, &name, &s.Type, &s.SettlementID, &s.SettlementName); err != nil {
			return nil, err
		}
		s.Name = unit.DisplayName(s.Type)
		if name != nil && *name != "" {
			s.Name = *name
		}
		out = append(out, s)
	}
	if out == nil {
		out = []stalledEligibleShip{}
	}
	return out, rows.Err()
}

// notifyStalled is 3b-4's R5: a messenger stuck 'awaiting_passage' with no
// carrier for PassageStallNoticeTicks is a PLAYER DECISION, not a mechanic —
// megaron_styrande_beslut.md "Havet är fysiskt" (Timothy 2026-09-27): "det är
// en helt ny mekanik ... men det ska ske enligt en dispatch — spelaren får
// avgöra. Ingen automatisk reserv, ingen tidsgräns." Sent exactly once per
// waiting spell: passage_stalled_notified_tick is the marker, cleared only by
// promoteSealed on a fresh reseal — never re-sent every scan while the SAME
// spell continues.
func (h *PassageScanHandler) notifyStalled(ctx context.Context, worldID uuid.UUID, currentTick int) error {
	rows, err := h.pool.Query(ctx,
		`SELECT id FROM messengers
		  WHERE world_id = $1 AND passage_status = 'awaiting_passage'
		    AND passage_since_tick IS NOT NULL AND $2 - passage_since_tick >= $3
		    AND passage_stalled_notified_tick IS NULL
		  FOR UPDATE SKIP LOCKED`,
		worldID, currentTick, PassageStallNoticeTicks,
	)
	if err != nil {
		return err
	}
	var due []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		due = append(due, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, id := range due {
		if err := h.notifyStalledOne(ctx, worldID, id, currentTick); err != nil {
			slog.Error("passage scan: notify stalled", "messenger", id, "err", err)
		}
	}
	return nil
}

func (h *PassageScanHandler) notifyStalledOne(ctx context.Context, worldID, messengerID uuid.UUID, currentTick int) error {
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var senderID, portID uuid.UUID
	var kind string
	var orderPayload []byte
	if err := tx.QueryRow(ctx,
		`SELECT sender_id, kind, order_payload, passage_port_id FROM messengers
		  WHERE id = $1 AND passage_status = 'awaiting_passage' AND passage_stalled_notified_tick IS NULL
		  FOR UPDATE`,
		messengerID,
	).Scan(&senderID, &kind, &orderPayload, &portID); err != nil {
		return nil // already notified by a racing pass, or resolved
	}

	var portOwnerID uuid.UUID
	var portName string
	var portQ, portR int
	if err := tx.QueryRow(ctx,
		`SELECT s.owner_id, s.name, p.map_q, p.map_r FROM settlements s JOIN provinces p ON p.id = s.province_id WHERE s.id = $1`,
		portID,
	).Scan(&portOwnerID, &portName, &portQ, &portR); err != nil {
		return fmt.Errorf("notify stalled: load port: %w", err)
	}
	ownPort := portOwnerID == senderID

	targetQ, targetR, err := FinalTargetTx(ctx, tx, messengerID)
	if err != nil {
		return err
	}
	var targetName *string
	if err := tx.QueryRow(ctx,
		`SELECT s.name FROM provinces p JOIN settlements s ON s.province_id = p.id
		  WHERE p.world_id = $1 AND p.map_q = $2 AND p.map_r = $3 AND s.state = 'active' LIMIT 1`,
		worldID, targetQ, targetR,
	).Scan(&targetName); err != nil {
		targetName = nil // no settlement exactly there (a unit order) — best-effort only
	}

	var verb string
	if kind == "order" && len(orderPayload) > 0 {
		var p OrderDeliveryPayload
		if json.Unmarshal(orderPayload, &p) == nil {
			verb = p.Verb
		}
	}

	ships, err := stalledEligibleShips(ctx, tx, worldID, senderID, portID, ownPort)
	if err != nil {
		return err
	}

	tag, err := tx.Exec(ctx,
		`UPDATE messengers SET passage_stalled_notified_tick = $2
		  WHERE id = $1 AND passage_status = 'awaiting_passage' AND passage_stalled_notified_tick IS NULL`,
		messengerID, currentTick,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return nil // already notified by a racing pass
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}

	if h.hub != nil {
		body := map[string]any{
			"messenger_id": messengerID,
			"port_name":    portName,
			"port_q":       portQ,
			"port_r":       portR,
			// q/r (megaron_plan_dispatches.md §6:3): the generic "⌖ Take me
			// there" field every dispatch kind's resolveDestination reads
			// first — points at the port, where the decision is made.
			"q":              portQ,
			"r":              portR,
			"target_q":       targetQ,
			"target_r":       targetR,
			"own_port":       ownPort,
			"kind":           kind,
			"eligible_ships": ships,
		}
		if verb != "" {
			body["verb"] = verb
		}
		if targetName != nil {
			body["target_name"] = *targetName
		}
		_ = h.hub.NotifyPlayer(ctx, worldID, senderID, "PassageStalled", 2, body)
	}
	slog.Info("passage: stalled, dispatch sent", "messenger", messengerID, "port", portName, "own_port", ownPort)
	return nil
}

// boardTransports is R2 for real naval transports (transports.ship_unit_id):
// a departing transport's port and destination match any of the messengers
// waiting there. No integer depart-tick exists on transports (only the
// wall-clock departs_at), so "just departed" is a small window of the last
// passageDepartureWindowTicks — see that constant's doc comment.
func (h *PassageScanHandler) boardTransports(ctx context.Context, worldID uuid.UUID, currentTick int) error {
	windowStart := h.clk.Now().Add(-tick.RealUntil(passageDepartureWindowTicks, 0))
	rows, err := h.pool.Query(ctx,
		`SELECT id, owner_id, origin_id, dest_id, dest_q, dest_r, due_tick, arrives_at, ship_unit_id
		   FROM transports
		  WHERE world_id = $1 AND status = 'in_transit' AND ship_unit_id IS NOT NULL
		    AND category = 'naval' AND departs_at >= $2`,
		worldID, windowStart,
	)
	if err != nil {
		return err
	}
	type carrier struct {
		id, owner    uuid.UUID
		originID     uuid.UUID
		destID       uuid.UUID
		destQ, destR int
		dueTick      int
		arrivesAt    time.Time
		shipUnitID   uuid.UUID
	}
	var carriers []carrier
	for rows.Next() {
		var c carrier
		if err := rows.Scan(&c.id, &c.owner, &c.originID, &c.destID, &c.destQ, &c.destR, &c.dueTick, &c.arrivesAt, &c.shipUnitID); err != nil {
			rows.Close()
			return err
		}
		carriers = append(carriers, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, c := range carriers {
		var shipName *string
		var shipType string
		_ = h.pool.QueryRow(ctx, `SELECT name, type FROM units WHERE id = $1`, c.shipUnitID).Scan(&shipName, &shipType)
		name := unit.DisplayName(shipType)
		if shipName != nil && *shipName != "" {
			name = *shipName
		}
		// The PORT waiting messengers stand at is the transport's ORIGIN (where
		// it departed from); the disembark point is its destination.
		if err := h.boardEligible(ctx, worldID, c.owner, c.originID, province.MapPosition{Q: c.destQ, R: c.destR},
			c.dueTick, c.arrivesAt, boardedCarrier{transportID: &c.id, name: name}); err != nil {
			slog.Error("passage scan: board transport", "transport", c.id, "err", err)
		}
	}
	return nil
}

// boardShipMissions is R2 for a ship mission departing a port
// (megaron_plan_skeppsuppdrag_landsatt.md): any naval unit that started
// marching exactly this tick. A ship's own q/r is frozen to its departure
// (sea) hex for the whole march — province.NearestSettlementNeighbor resolves
// which settlement that hex belongs to (the port), and the ship's own
// destination — land_target_q/r for a 'land' mission, else its own
// target_q/r's neighbouring settlement — resolves where a boarded messenger
// can step ashore.
func (h *PassageScanHandler) boardShipMissions(ctx context.Context, worldID uuid.UUID, currentTick int) error {
	rows, err := h.pool.Query(ctx,
		`SELECT id, owner_id, type, q, r, target_q, target_r, land_target_q, land_target_r, arrive_tick, arrives_at, name
		   FROM units
		  WHERE world_id = $1 AND status = 'marching' AND depart_tick = $2
		    AND type IN ('galley', 'merchantman')`,
		worldID, currentTick,
	)
	if err != nil {
		return err
	}
	type ship struct {
		id, owner                uuid.UUID
		typ                      string
		q, r                     int
		targetQ, targetR         int
		landTargetQ, landTargetR *int
		arriveTick               int
		arrivesAt                time.Time
		name                     *string
	}
	var ships []ship
	for rows.Next() {
		var s ship
		if err := rows.Scan(&s.id, &s.owner, &s.typ, &s.q, &s.r, &s.targetQ, &s.targetR,
			&s.landTargetQ, &s.landTargetR, &s.arriveTick, &s.arrivesAt, &s.name); err != nil {
			rows.Close()
			return err
		}
		ships = append(ships, s)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, s := range ships {
		portID, _, _, portFound, err := province.NearestSettlementNeighbor(ctx, h.pool, worldID, s.q, s.r)
		if err != nil {
			slog.Error("passage scan: resolve ship's port", "unit", s.id, "err", err)
			continue
		}
		if !portFound {
			continue // departed from open water (colonize-in-place substrate etc.) — no port to check
		}
		var landQ, landR int
		var haveLand bool
		if s.landTargetQ != nil && s.landTargetR != nil {
			landQ, landR, haveLand = *s.landTargetQ, *s.landTargetR, true
		} else {
			_, lq, lr, ok, nerr := province.NearestSettlementNeighbor(ctx, h.pool, worldID, s.targetQ, s.targetR)
			if nerr != nil {
				slog.Error("passage scan: resolve ship's landing point", "unit", s.id, "err", nerr)
				continue
			}
			landQ, landR, haveLand = lq, lr, ok
		}
		if !haveLand {
			continue // patrol/explore/assault into open water — no disembark point
		}
		name := unit.DisplayName(s.typ)
		if s.name != nil && *s.name != "" {
			name = *s.name
		}
		if err := h.boardEligible(ctx, worldID, s.owner, portID, province.MapPosition{Q: landQ, R: landR},
			s.arriveTick, s.arrivesAt, boardedCarrier{unitID: &s.id, name: name}); err != nil {
			slog.Error("passage scan: board ship mission", "unit", s.id, "err", err)
		}
	}
	return nil
}

// boardedCarrier is which of the two carrier kinds boarded a messenger.
type boardedCarrier struct {
	transportID *uuid.UUID
	unitID      *uuid.UUID
	name        string
}

// boardEligible boards every messenger waiting at portID, owned by ownerID,
// for which a land route exists onward from disembarkAt to the messenger's
// true final target (R2's four conditions). No capacity limit (R2: "ingen
// kapacitetsgräns — bud tar ingen plats").
func (h *PassageScanHandler) boardEligible(ctx context.Context, worldID, ownerID, portID uuid.UUID, disembarkAt province.MapPosition, carrierDueTick int, carrierArrivesAt time.Time, carrier boardedCarrier) error {
	rows, err := h.pool.Query(ctx,
		`SELECT id FROM messengers
		  WHERE world_id = $1 AND sender_id = $2 AND passage_port_id = $3
		    AND passage_status = 'awaiting_passage'
		    AND passage_since_tick IS NOT NULL AND passage_since_tick <= $4
		  FOR UPDATE SKIP LOCKED`,
		worldID, ownerID, portID, carrierDueTick,
	)
	if err != nil {
		return err
	}
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, id := range ids {
		targetQ, targetR, err := FinalTarget(ctx, h.pool, id)
		if err != nil {
			slog.Error("passage scan: load final target", "messenger", id, "err", err)
			continue
		}
		_, _, landOK, err := province.FindPath(ctx, h.pool, worldID, disembarkAt, province.MapPosition{Q: targetQ, R: targetR}, province.CategoryCourierLand)
		if err != nil {
			slog.Error("passage scan: land path check", "messenger", id, "err", err)
			continue
		}
		if !landOK {
			continue // R2d fails — leave it waiting for a better carrier
		}
		if err := h.boardOne(ctx, worldID, id, disembarkAt, targetQ, targetR, carrierDueTick, carrierArrivesAt, carrier); err != nil {
			slog.Error("passage scan: board one", "messenger", id, "err", err)
		}
	}
	return nil
}

func (h *PassageScanHandler) boardOne(ctx context.Context, worldID, messengerID uuid.UUID, disembarkAt province.MapPosition, targetQ, targetR int, carrierDueTick int, carrierArrivesAt time.Time, carrier boardedCarrier) error {
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx,
		// Boarding ends this waiting spell, so the stall marker resets: the
		// next spell (the return leg in a foreign port, or a re-wait after
		// this carrier is lost) earns its own PassageStalled dispatch.
		`UPDATE messengers SET passage_status = 'aboard', carrier_transport_id = $2,
		        carrier_unit_id = $3, carrier_name = $4, passage_stalled_notified_tick = NULL
		  WHERE id = $1 AND passage_status = 'awaiting_passage'`,
		messengerID, carrier.transportID, carrier.unitID, carrier.name,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return nil // already boarded/resolved
	}

	// boardEligible already proved a CategoryCourierLand path exists from
	// disembarkAt to the target right before calling this — CourierTravel
	// (plain CategoryCourier) permits everything that does, plus rivers, so
	// !ok here would be an internal inconsistency: fail the boarding rather
	// than guess a landward-leg time, and let a later scan retry it.
	landTicks, landDur, ok, err := CourierTravel(ctx, tx, worldID, disembarkAt, province.MapPosition{Q: targetQ, R: targetR})
	if err != nil {
		return fmt.Errorf("board one: landward leg: %w", err)
	}
	if !ok {
		return fmt.Errorf("board one: no landward route from disembark point to target, despite boardEligible's own check")
	}
	dueTick := carrierDueTick + landTicks
	arrivesAt := carrierArrivesAt.Add(landDur)
	if err := scheduleCompletion(ctx, tx, h.scheduler, messengerID, dueTick, arrivesAt); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	slog.Info("passage: messenger boarded", "messenger", messengerID, "carrier", carrier.name, "arrives_at", arrivesAt)
	return nil
}

// lostCarrierRow is one messenger whose carrier detectLostCarriers found lost.
type lostCarrierRow struct {
	id, sender uuid.UUID
	carrierRef string // transport or unit id, for the log line only
}

// detectLostCarriers is R4: a boarded messenger whose carrier is lost is
// sealed back to its port for PassageLostDelayTicks. Two carrier kinds:
//   - transport (sjötransport): transport.seize flips transports.status to
//     'intercepted' — the single outcome-flip for all three of
//     captured/limped/sunk (R5 of megaron_plan_sjohandel_kraver_skepp.md).
//   - ship mission (skeppsuppdrag): the unit is 'disbanded' (sunk in combat)
//     or has changed owner (captured) — the two unit-table signals available
//     without messenger importing combat's naval-battle internals (G1).
func (h *PassageScanHandler) detectLostCarriers(ctx context.Context, worldID uuid.UUID, currentTick int) error {
	transportRows, err := h.pool.Query(ctx,
		`SELECT m.id, m.sender_id, t.id
		   FROM messengers m JOIN transports t ON t.id = m.carrier_transport_id
		  WHERE m.world_id = $1 AND m.passage_status = 'aboard' AND t.status = 'intercepted'
		  FOR UPDATE OF m SKIP LOCKED`,
		worldID,
	)
	if err != nil {
		return err
	}
	var lost []lostCarrierRow
	for transportRows.Next() {
		var r lostCarrierRow
		var transportID uuid.UUID
		if err := transportRows.Scan(&r.id, &r.sender, &transportID); err != nil {
			transportRows.Close()
			return err
		}
		r.carrierRef = "transport:" + transportID.String()
		lost = append(lost, r)
	}
	transportRows.Close()
	if err := transportRows.Err(); err != nil {
		return err
	}

	shipRows, err := h.pool.Query(ctx,
		`SELECT m.id, m.sender_id, u.id
		   FROM messengers m JOIN units u ON u.id = m.carrier_unit_id
		  WHERE m.world_id = $1 AND m.passage_status = 'aboard'
		    AND (u.status = 'disbanded' OR u.owner_id != m.sender_id)
		  FOR UPDATE OF m SKIP LOCKED`,
		worldID,
	)
	if err != nil {
		return err
	}
	for shipRows.Next() {
		var r lostCarrierRow
		var unitID uuid.UUID
		if err := shipRows.Scan(&r.id, &r.sender, &unitID); err != nil {
			shipRows.Close()
			return err
		}
		r.carrierRef = "unit:" + unitID.String()
		lost = append(lost, r)
	}
	shipRows.Close()
	if err := shipRows.Err(); err != nil {
		return err
	}

	for _, r := range lost {
		if err := h.sealLostCarrier(ctx, worldID, r, currentTick); err != nil {
			slog.Error("passage scan: seal lost carrier", "messenger", r.id, "err", err)
		}
	}
	return nil
}

// sealLostCarrier seals one messenger back to its port after
// PassageLostDelayTicks and bumps passage_generation — invalidating the
// terminal event scheduleCompletion already scheduled for the carrier this
// messenger just lost, so its (still-queued, now stale) firing is a no-op
// wherever it lands (megaron_plan_budet_liftar.md R4 review fix).
func (h *PassageScanHandler) sealLostCarrier(ctx context.Context, worldID uuid.UUID, r lostCarrierRow, currentTick int) error {
	tag, err := h.pool.Exec(ctx,
		`UPDATE messengers
		    SET passage_status = 'returning_sealed', passage_lost_until_tick = $2,
		        carrier_transport_id = NULL, carrier_unit_id = NULL, carrier_name = NULL,
		        passage_generation = passage_generation + 1
		  WHERE id = $1 AND passage_status = 'aboard'`,
		r.id, currentTick+PassageLostDelayTicks,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return nil // already sealed/resolved by a racing pass
	}
	if h.hub != nil {
		_ = h.hub.NotifyPlayer(ctx, worldID, r.sender, "OrderFailed", 3, map[string]any{
			"messenger_id": r.id,
			"reason":       "your runner's ship was lost — sealed and safe, back awaiting passage in port shortly",
		})
	}
	slog.Info("passage: carrier lost, messenger sealed", "messenger", r.id, "carrier", r.carrierRef)
	return nil
}

// queryRower is the minimal *pgxpool.Pool/pgx.Tx surface finalTargetQuery and
// scheduleCompletion need (both implement it).
type queryRower interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// FinalTarget/FinalTargetTx read a messenger's true ultimate destination hex —
// NOT hex_q/hex_r for an order envelope (which stores the RUNNER's ORIGIN
// there; dest_q/dest_r holds the unit's position — see api/handlers/unit.go
// sendOrderCourier's INSERT column order) and NOT hex_q/hex_r for a return
// leg (still the outbound destination; the return target is the origin).
// Exported (megaron_plan_ordna_passage.md 3b-3 R2) so api/handlers.
// ArrangePassage can resolve the same "where is this runner actually headed"
// question the sea-lift board mechanic already answers — one query, not two.
func FinalTarget(ctx context.Context, db queryRower, messengerID uuid.UUID) (q, r int, err error) {
	return finalTargetQuery(ctx, db, messengerID)
}

func FinalTargetTx(ctx context.Context, tx pgx.Tx, messengerID uuid.UUID) (q, r int, err error) {
	return finalTargetQuery(ctx, tx, messengerID)
}

func finalTargetQuery(ctx context.Context, db queryRower, messengerID uuid.UUID) (q, r int, err error) {
	var kind, status string
	var hexQ, hexR int
	var destQ, destR, originQ, originR, opQ, opR *int
	if err := db.QueryRow(ctx,
		`SELECT m.kind, m.status, m.hex_q, m.hex_r, m.dest_q, m.dest_r,
		        m.origin_q, m.origin_r, op.map_q, op.map_r
		   FROM messengers m
		   LEFT JOIN settlements os ON os.id = m.origin_id
		   LEFT JOIN provinces   op ON op.id = os.province_id
		  WHERE m.id = $1`,
		messengerID,
	).Scan(&kind, &status, &hexQ, &hexR, &destQ, &destR, &originQ, &originR, &opQ, &opR); err != nil {
		return 0, 0, fmt.Errorf("load final target: %w", err)
	}
	if kind == "order" {
		if destQ != nil && destR != nil {
			return *destQ, *destR, nil
		}
		return hexQ, hexR, nil
	}
	if status == "returning" {
		if opQ != nil && opR != nil {
			return *opQ, *opR, nil
		}
		if originQ != nil && originR != nil {
			return *originQ, *originR, nil
		}
	}
	return hexQ, hexR, nil
}

// scheduleCompletion (re)schedules the terminal delivery/arrival event for a
// messenger that has just resolved a plan for its passage (boarded a carrier,
// or took the reserve) — the SAME event kind/payload shape the original
// dispatcher would have scheduled directly had no sea-lift been needed, so
// ArrivalHandler/ReturnHandler/OrderDeliveryHandler's own delivery logic needs
// no changes — only a generation check (see those handlers) to recognise a
// firing this function's LATER call (after a lost-carrier re-boarding, or a
// reserve taken instead) has superseded.
//
// Deliberately does NOT touch passage_status: a boarded messenger stays
// 'aboard' — with carrier_transport_id/carrier_unit_id still set, so
// PassageScanHandler.detectLostCarriers can find it — for the WHOLE voyage
// and landward leg, only cleared by the delivery handler at actual arrival
// (megaron_plan_budet_liftar.md R4 review fix, 2026-09-26: passage_status was
// previously cleared here, the instant boarding happened, which made
// detectLostCarriers's own query dead code in the real flow — it could only
// ever match a hand-crafted test fixture).
func scheduleCompletion(ctx context.Context, tx pgx.Tx, sched *events.Scheduler, messengerID uuid.UUID, dueTick int, arrivesAt time.Time) error {
	var worldID uuid.UUID
	var kind, status string
	var orderPayload []byte
	var generation int
	if err := tx.QueryRow(ctx,
		`UPDATE messengers SET arrives_at = $2, passage_generation = passage_generation + 1
		  WHERE id = $1
		  RETURNING world_id, kind, status, order_payload, passage_generation`,
		messengerID, arrivesAt,
	).Scan(&worldID, &kind, &status, &orderPayload, &generation); err != nil {
		return fmt.Errorf("schedule completion: update messenger: %w", err)
	}

	if kind == "order" {
		var p OrderDeliveryPayload
		if len(orderPayload) > 0 {
			if err := json.Unmarshal(orderPayload, &p); err != nil {
				return fmt.Errorf("schedule completion: unmarshal order payload: %w", err)
			}
		}
		p.MessengerID = messengerID
		p.PassageGeneration = generation
		return sched.EnqueueTickTx(ctx, tx, worldID, events.ScheduledOrderDelivery, p, dueTick)
	}
	if status == "returning" {
		return sched.EnqueueTickTx(ctx, tx, worldID, events.ScheduledMessengerReturn,
			ReturnPayload{MessengerID: messengerID, PassageGeneration: generation}, dueTick)
	}
	if err := sched.EnqueueTickTx(ctx, tx, worldID, events.ScheduledMessengerArrival,
		ArrivalPayload{MessengerID: messengerID, PassageGeneration: generation}, dueTick); err != nil {
		return err
	}

	// A trade-offer-bearing messenger that needed the sea-lift could not have
	// its expiry precomputed at send time (Send/SendFromHost schedule
	// ScheduledOfferExpiry immediately for a land route — see api/handlers/
	// messenger.go) — the real arrival time depends on which carrier, if any,
	// picks it up. expires_at (the inbox's own filter column) and the expiry
	// event are both set here, now that the real arrival is known.
	var hasOffer bool
	if err := tx.QueryRow(ctx,
		`SELECT trade_offer IS NOT NULL FROM messengers WHERE id = $1`, messengerID,
	).Scan(&hasOffer); err != nil {
		return fmt.Errorf("schedule completion: check trade offer: %w", err)
	}
	if !hasOffer {
		return nil
	}
	expiresAt := arrivesAt.Add(tick.RealUntil(OfferExpiryTicks, 0))
	if _, err := tx.Exec(ctx,
		`UPDATE messengers SET expires_at = $2 WHERE id = $1`, messengerID, expiresAt,
	); err != nil {
		return fmt.Errorf("schedule completion: set offer expiry: %w", err)
	}
	return sched.EnqueueTickTx(ctx, tx, worldID, events.ScheduledOfferExpiry,
		map[string]any{"messenger_id": messengerID.String()}, dueTick+OfferExpiryTicks)
}
