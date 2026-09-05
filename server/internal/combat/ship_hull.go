package combat

// megaron_plan_skeppsreparation.md Slice B — hull is drawn on BOTH sides of a
// battle proportional to the naval losses actually taken; the routed side's
// surviving damaged ships link home toward the nearest own shipyard, the
// winning side's damaged ships keep their orders (§Beslut B3). This file
// holds everything that hangs off resolveTick's single "battle just ended"
// branch (battle.go calls applyNavalHullDamage exactly once from there —
// see that call site's comment for the idempotency argument).
//
// Calibration (both explicitly delegated to the implementer by the plan,
// strawman not canon — pinned in ship_hull_test.go):
//   - casualty-fraction → hull-point mapping: hullLossForCasualtyFraction.
//   - embarked-cohort loss fraction ("3/5" example): cargoSizeAfterHullLoss.

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"

	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/tick"
	"formatet/megaron/server/internal/unit"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// hullLossForCasualtyFraction is the pinned casualty-fraction → hull-point
// mapping (§Slice B point 2, "mappa casualty-fraktionen → hull-poäng; pinna
// kurvan"). fraction is the SIDE's naval-only loss this battle — see
// applyNavalHullDamage's doc comment for why it is computed at side
// granularity rather than per-ship (a single vessel's own size is always
// exactly 1, so a per-ship casualty fraction can only ever be 0% or 100%
// under the existing size-based combat model; that 100% case is already the
// pre-existing outright-sink path and never reaches this function — see
// applyNavalHullDamage's sizes[i] <= 0 skip). Linear, rounded to the nearest
// whole hull point, clamped to [0, hullMax].
func hullLossForCasualtyFraction(fraction float64) int {
	if fraction <= 0 {
		return 0
	}
	if fraction >= 1 {
		return hullMax
	}
	return int(math.Round(hullMax * fraction))
}

// cargoSizeAfterHullLoss is the pinned embarked-cohort loss mapping (§Slice B
// point 5, Timothy's "3/5" example — the plan flags it as ambiguous between
// "loses the hull LOSS fraction" and "loses the REMAINING hull fraction" and
// asks the implementer to pick one and test it). §Beslut B3's own prose is
// unambiguous ("förlorar manskap PROPORTIONELLT MOT skeppets hull-FÖRLUST"),
// so that reading wins: the cohort loses the SAME fraction of its manpower
// that the ship just lost of its hull (hullLoss/hullMax), not the fraction
// still standing. A full 5→0 hull loss (sunk) takes every man with it.
func cargoSizeAfterHullLoss(size, hullLoss int) int {
	if hullLoss <= 0 || size <= 0 {
		return size
	}
	if hullLoss >= hullMax {
		return 0
	}
	lost := int(math.Floor(float64(size) * float64(hullLoss) / float64(hullMax)))
	return size - lost
}

// applyNavalHullDamage is §Slice B points 2/3/4/5/6, run exactly once from
// resolveTick's "battle just ended" branch (see that call site). For each
// side it computes ONE naval-only casualty fraction for the whole battle
// (initialSizes are each participant's size when it FIRST joined the battle,
// battle_participants.initial_size — stable across every tick the battle
// spanned; sizes are this tick's final, already-persisted values) and maps
// it to a single hullLoss via hullLossForCasualtyFraction, then draws that
// many points off every surviving naval participant on that side alike.
//
// Side-wide (not per-ship) is a deliberate reading of "dra hull proportio-
// nellt mot förlusterna", not an accident: a naval unit's own size is always
// exactly 1 (unit.CrewFor/model.go), so a PER-SHIP casualty fraction can only
// ever be 0% (untouched) or 100% (already sunk by the ordinary per-round
// distributeLosses/apply-final-sizes machinery earlier in resolveTick, which
// is why sizes[i] <= 0 is skipped below — that ship is already gone). Without
// a side-wide fraction, a ship that SURVIVES its battle could never take
// graded damage at all, and Slice B point 4's whole premise (a damaged ship
// hits softer, so repairing it is worth the timber) would have nothing to
// act on.
func (h *BattleTickHandler) applyNavalHullDamage(
	ctx context.Context, tx pgx.Tx,
	battleID, worldID uuid.UUID, q, r, tickIndex int,
	participants []battleParticipant, initialSizes, sizes []int, bySide map[string][]int,
	attRouted, defRouted bool,
) {
	for _, side := range []string{"attacker", "defender"} {
		idx := bySide[side]
		var navalInitial, navalFinal int
		for _, i := range idx {
			if unit.CategoryOf(unit.Type(participants[i].utype)) != unit.CategoryNaval {
				continue
			}
			navalInitial += initialSizes[i]
			navalFinal += sizes[i]
		}
		if navalInitial <= 0 {
			continue // no naval participants on this side at all
		}
		hullLoss := hullLossForCasualtyFraction(float64(navalInitial-navalFinal) / float64(navalInitial))
		if hullLoss <= 0 {
			continue // this side's fleet took no losses this battle
		}

		// §Beslut B3: only the ROUTED/losing side's survivors are forced
		// home — the winning side keeps its orders even though it took the
		// same proportional hull draw.
		routedHome := (side == "attacker" && attRouted) || (side == "defender" && defRouted)

		for _, i := range idx {
			if unit.CategoryOf(unit.Type(participants[i].utype)) != unit.CategoryNaval {
				continue
			}
			if sizes[i] <= 0 {
				continue // already sunk this tick by the ordinary casualty roll — hull is moot
			}
			h.drawShipHull(ctx, tx, battleID, worldID, q, r, tickIndex, participants[i], hullLoss, routedHome)
		}
	}
}

// drawShipHull applies one ship's hull draw, its cargo's proportional loss,
// the sink-at-zero outcome, the owner's notification, and — for a routed
// survivor — the home march dispatch. Best-effort past the hull UPDATE
// itself: a DB error here is logged and the battle-tick tx still commits
// (the battle's own outcome must never hinge on this side-effect succeeding,
// same posture as notifyBattleEnded elsewhere in this file).
func (h *BattleTickHandler) drawShipHull(
	ctx context.Context, tx pgx.Tx,
	battleID, worldID uuid.UUID, q, r, tickIndex int,
	p battleParticipant, hullLoss int, routedHome bool,
) {
	var newHull int
	if err := tx.QueryRow(ctx,
		`UPDATE units SET hull = GREATEST(0, hull - $2), updated_at = now() WHERE id = $1 RETURNING hull`,
		p.unitID, hullLoss,
	).Scan(&newHull); err != nil {
		slog.Warn("battle tick: draw ship hull failed", "unit", p.unitID, "err", err)
		return
	}
	sunk := newHull <= 0

	if p.cargoUnitID != nil {
		h.applyCargoLoss(ctx, tx, *p.cargoUnitID, hullLoss, sunk)
	}

	if sunk {
		if _, err := tx.Exec(ctx,
			`UPDATE units SET status = 'disbanded', size = 0, updated_at = now() WHERE id = $1`,
			p.unitID,
		); err != nil {
			slog.Warn("battle tick: sink hull-damaged ship failed", "unit", p.unitID, "err", err)
		}
		if _, err := tx.Exec(ctx,
			`UPDATE battle_participants SET current_size = 0, left_tick = $3
			 WHERE battle_id = $1 AND unit_id = $2 AND left_tick IS NULL`,
			battleID, p.unitID, tickIndex,
		); err != nil {
			slog.Warn("battle tick: mark hull-sunk participant left failed", "unit", p.unitID, "err", err)
		}
	}

	h.notifyShipDamaged(ctx, battleID, worldID, q, r, p, newHull, sunk, routedHome && !sunk)

	if routedHome && !sunk {
		if err := h.sendDamagedShipHome(ctx, tx, worldID, p.unitID, p.ownerID, p.utype, tickIndex); err != nil {
			slog.Warn("battle tick: send damaged ship home failed", "unit", p.unitID, "err", err)
		}
	}
}

// applyCargoLoss is §Slice B point 5 (cargoSizeAfterHullLoss's doc comment
// has the calibration argument). Only ever touches a still-embarked cargo
// unit (status = 'embarked') — a ship's cargo_unit_id can point at a unit
// that already disembarked or was otherwise disbanded, and this must not
// resurrect or otherwise disturb it.
func (h *BattleTickHandler) applyCargoLoss(ctx context.Context, tx pgx.Tx, cargoUnitID uuid.UUID, hullLoss int, shipSunk bool) {
	if shipSunk {
		// "Sänks skeppet (hull=0) går kohorten under med det" — full loss,
		// not just this draw's proportional share.
		if _, err := tx.Exec(ctx,
			`UPDATE units SET status = 'disbanded', size = 0, updated_at = now() WHERE id = $1 AND status = 'embarked'`,
			cargoUnitID,
		); err != nil {
			slog.Warn("battle tick: sink embarked cargo with ship failed", "cargo", cargoUnitID, "err", err)
		}
		return
	}
	if hullLoss <= 0 {
		return
	}
	var size int
	if err := tx.QueryRow(ctx,
		`SELECT size FROM units WHERE id = $1 AND status = 'embarked' FOR UPDATE`, cargoUnitID,
	).Scan(&size); err != nil {
		if err != pgx.ErrNoRows {
			slog.Warn("battle tick: load embarked cargo for hull loss failed", "cargo", cargoUnitID, "err", err)
		}
		return // no longer embarked (or gone) — nothing to reduce
	}
	newSize := cargoSizeAfterHullLoss(size, hullLoss)
	if newSize == size {
		return
	}
	if _, err := tx.Exec(ctx,
		`UPDATE units SET size = $2, updated_at = now() WHERE id = $1`, cargoUnitID, newSize,
	); err != nil {
		slog.Warn("battle tick: apply embarked cargo hull loss failed", "cargo", cargoUnitID, "err", err)
	}
}

// notifyShipDamaged is §Slice B point 6 — the ShipDamaged notification +
// audit event. Best-effort: never blocks the battle-tick tx (matches every
// other notify call in this file).
func (h *BattleTickHandler) notifyShipDamaged(ctx context.Context, battleID, worldID uuid.UUID, q, r int, p battleParticipant, hull int, sunk, returningHome bool) {
	_, _ = h.eventStore.Append(ctx, p.unitID, events.StreamType(unit.StreamUnit), EventShipDamaged,
		ShipDamagedPayload{
			BattleID: battleID, WorldID: worldID, UnitID: p.unitID, UnitType: p.utype,
			Hull: hull, HullMax: hullMax, Sunk: sunk, ReturningHome: returningHome,
		}, worldID, nil,
	)
	if h.hub == nil {
		return
	}
	level := 3
	if sunk {
		level = 4
	}
	if err := h.hub.NotifyPlayer(ctx, worldID, p.ownerID, EventShipDamaged, level, map[string]any{
		"unit_id":        p.unitID,
		"unit_type":      p.utype,
		"name":           unit.LoadDisplayName(ctx, h.pool, p.unitID),
		"q":              q,
		"r":              r,
		"hull":           hull,
		"hull_max":       hullMax,
		"sunk":           sunk,
		"returning_home": returningHome,
	}); err != nil {
		slog.Warn("battle tick: notify ship damaged failed", "unit", p.unitID, "err", err)
	}
}

// sendDamagedShipHome is §Slice B point 3's home march. It resolves the
// nearest own settlement with a shipyard (falling back to the nearest own
// settlement at all if the owner has not built one yet — a damaged ship
// should never be stranded mid-ocean just because Slice A's building is
// still unbuilt; it simply cannot start repairing until Slice C lands and
// the Wanax builds one), then turns the ship for home exactly like an
// ordinary march dispatch (march_start.go's StartMarch), stamping
// march_intent = 'damaged_return' so unit_arrival.go's damagedShipReturned
// re-garrisons it on arrival instead of trying a normal hex→settlement
// lookup (a ship's return leg targets the SEA hex adjacent to home, which
// has no settlement row of its own — same reason dispatchReturnHome's
// exploreReturned counterpart exists).
//
// tickIndex doubles as "the current world tick" here — resolveTick is always
// called with e.DueTick, which for a BattleTick IS the world tick it fires
// on (battleTickIntervalTicks = 1), so no extra current_world_tick() query
// is needed.
func (h *BattleTickHandler) sendDamagedShipHome(
	ctx context.Context, tx pgx.Tx,
	worldID, unitID, ownerID uuid.UUID, utype string, tickIndex int,
) error {
	var fromQ, fromR, crew int
	var cargoUnitID *uuid.UUID
	if err := tx.QueryRow(ctx,
		`SELECT q, r, crew, cargo_unit_id FROM units WHERE id = $1`, unitID,
	).Scan(&fromQ, &fromR, &crew, &cargoUnitID); err != nil {
		return fmt.Errorf("send damaged ship home: load position: %w", err)
	}

	homeSettlementID, homeQ, homeR, found, err := nearestOwnShipyardSettlement(ctx, tx, worldID, ownerID, fromQ, fromR)
	if err != nil {
		return fmt.Errorf("send damaged ship home: resolve home port: %w", err)
	}
	if !found {
		slog.Warn("send damaged ship home: owner has no settlement to return to, leaving ship in place", "unit", unitID, "owner", ownerID)
		return nil
	}

	if seaQ, seaR, seaFound, seaErr := province.NearestSeaNeighbor(ctx, tx, worldID, homeQ, homeR); seaErr != nil {
		return fmt.Errorf("send damaged ship home: resolve sea approach: %w", seaErr)
	} else if seaFound {
		homeQ, homeR = seaQ, seaR
	} else {
		slog.Warn("send damaged ship home: home settlement has no adjacent sea hex, using its land hex", "unit", unitID, "settlement", homeSettlementID)
	}

	_, pathTicks, pathOK, pathErr := province.FindPath(ctx, tx, worldID,
		province.MapPosition{Q: fromQ, R: fromR}, province.MapPosition{Q: homeQ, R: homeR}, "naval")
	var moveTicks float64
	if pathErr == nil && pathOK {
		moveTicks = pathTicks
	} else {
		// Defensive fallback, same shape as dispatchReturnHome's — the outbound
		// leg into this battle already proved passability between these regions.
		if pathErr != nil {
			slog.Warn("send damaged ship home: FindPath failed, falling back to straight line", "unit", unitID, "err", pathErr)
		}
		dist := province.HexDistance(province.MapPosition{Q: fromQ, R: fromR}, province.MapPosition{Q: homeQ, R: homeR})
		if dist < 1 {
			dist = 1
		}
		moveTicks = province.TerrainMoveTicks("coastal_sea") * float64(dist)
	}
	// Mirrors march_start.go's TravelFactor — the one home for every leg.
	moveTicks *= TravelFactor(unit.Type(utype), crew, cargoUnitID != nil)
	travelTicks := int(math.Round(moveTicks))
	if travelTicks < 1 {
		travelTicks = 1
	}

	arrivesAt := h.clk.Now().Add(time.Duration(travelTicks*tick.TickSeconds) * time.Second)
	returnIntent := "damaged_return"

	if _, err := tx.Exec(ctx,
		`UPDATE units SET
		   status             = 'marching',
		   q                  = $2,
		   r                  = $3,
		   target_q           = $4,
		   target_r           = $5,
		   departs_at         = now(),
		   arrives_at         = $6,
		   depart_tick        = $8,
		   arrive_tick        = $9,
		   settlement_id      = NULL,
		   stance             = NULL,
		   sentry_q           = NULL,
		   sentry_r           = NULL,
		   home_settlement_id = $10,
		   march_intent       = $7,
		   updated_at         = now()
		 WHERE id = $1`,
		unitID, fromQ, fromR, homeQ, homeR, arrivesAt, returnIntent, tickIndex, tickIndex+travelTicks, homeSettlementID,
	); err != nil {
		return fmt.Errorf("send damaged ship home: dispatch march: %w", err)
	}

	if h.scheduler == nil {
		return fmt.Errorf("send damaged ship home: no scheduler configured")
	}
	if err := h.scheduler.EnqueueTickTx(ctx, tx, worldID, events.ScheduledUnitArrival,
		unit.ScheduledUnitArrivalPayload{UnitID: unitID, WorldID: worldID}, tickIndex+travelTicks,
	); err != nil {
		return fmt.Errorf("send damaged ship home: schedule arrival: %w", err)
	}
	return nil
}

// nearestOwnShipyardSettlement finds the owner's closest active settlement
// that has a shipyard building (by straight hex distance from fromQ/fromR —
// a selection heuristic, not the actual route; sendDamagedShipHome routes
// the real march via FindPath afterward). Falls back to the nearest own
// active settlement at all when none has a shipyard yet, so a damaged ship
// is never stranded just because Slice A's building hasn't been built —
// see sendDamagedShipHome's doc comment.
func nearestOwnShipyardSettlement(ctx context.Context, tx pgx.Tx, worldID, ownerID uuid.UUID, fromQ, fromR int) (settlementID uuid.UUID, homeQ, homeR int, found bool, err error) {
	rows, qErr := tx.Query(ctx,
		`SELECT s.id, p.map_q, p.map_r,
		        EXISTS(SELECT 1 FROM buildings b WHERE b.settlement_id = s.id AND b.building_type = 'shipyard') AS has_shipyard
		 FROM settlements s JOIN provinces p ON p.id = s.province_id
		 WHERE s.owner_id = $1 AND s.world_id = $2 AND s.state = 'active'`,
		ownerID, worldID,
	)
	if qErr != nil {
		return uuid.Nil, 0, 0, false, qErr
	}
	defer rows.Close()

	type candidate struct {
		id          uuid.UUID
		q, r        int
		hasShipyard bool
	}
	var withYard, allCandidates []candidate
	for rows.Next() {
		var c candidate
		if scanErr := rows.Scan(&c.id, &c.q, &c.r, &c.hasShipyard); scanErr != nil {
			return uuid.Nil, 0, 0, false, scanErr
		}
		allCandidates = append(allCandidates, c)
		if c.hasShipyard {
			withYard = append(withYard, c)
		}
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return uuid.Nil, 0, 0, false, rowsErr
	}

	pick := withYard
	if len(pick) == 0 {
		pick = allCandidates
	}
	if len(pick) == 0 {
		return uuid.Nil, 0, 0, false, nil
	}

	best := pick[0]
	bestDist := province.HexDistance(province.MapPosition{Q: fromQ, R: fromR}, province.MapPosition{Q: best.q, R: best.r})
	for _, c := range pick[1:] {
		d := province.HexDistance(province.MapPosition{Q: fromQ, R: fromR}, province.MapPosition{Q: c.q, R: c.r})
		if d < bestDist {
			best, bestDist = c, d
		}
	}
	return best.id, best.q, best.r, true, nil
}
