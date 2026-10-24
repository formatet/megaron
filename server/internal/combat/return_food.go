package combat

import (
	"context"
	"fmt"
	"math"
	"strings"

	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/tick"
	"formatet/megaron/server/internal/unit"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// returnForFood is a local captain's decision, before today's meal. It shares
// physical position, path costs, crew/cargo speed and dispatch with ordinary
// returns. The upkeep event's existing per-unit claim owns this invocation.
func (h *UpkeepHandler) returnForFood(ctx context.Context, outer upkeepUnitRow, world uuid.UUID) error {
	if outer.category != "naval" || h.arrivals == nil || h.clk == nil || outer.status != "positioned" && outer.status != "marching" {
		return nil
	}
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var u unitRow
	var position province.UnitPosition
	var q, r *int
	var intent *string
	var landQ, landR *int
	u.id = outer.id
	u.category = "naval"
	err = tx.QueryRow(ctx, `SELECT owner_id,type,size,crew,status,q,r,target_q,target_r,departs_at,arrives_at,depart_tick,arrive_tick,march_route,home_settlement_id,cargo_unit_id,provisions,march_intent,land_target_q,land_target_r FROM units WHERE id=$1 AND world_id=$2 FOR UPDATE`, u.id, world).Scan(&u.ownerID, &u.utype, &u.size, &u.crew, &position.Status, &q, &r, &position.TargetQ, &position.TargetR, &position.DepartsAt, &position.ArrivesAt, &position.DepartTick, &position.ArriveTick, &position.Route, &u.homeSettlementID, &u.cargoUnitID, &u.provisions, &intent, &landQ, &landR)
	if err != nil {
		return err
	}
	if q == nil || r == nil || position.Status != "positioned" && position.Status != "marching" || intent != nil && strings.HasSuffix(*intent, "return") {
		return nil
	}
	position.Q, position.R, position.Category = *q, *r, "naval"
	anchor, err := tick.LoadAnchor(ctx, tx, world)
	if err != nil {
		return err
	}
	now := h.clk.Now()
	here := position.Physical(ctx, tx, world, now, anchor.MilliAt(now))
	home, err := expeditionReturnDestination(ctx, tx, u, here.Q, here.R, world)
	if err != nil {
		return err
	}
	if home == nil {
		return nil
	}
	homeTicks := legTravelTicks(unit.Type(u.utype), u.crew, u.cargoUnitID != nil, home.cost)
	ration, err := shipRationTx(ctx, tx, u)
	if err != nil {
		return err
	}
	if ration <= 0 {
		return nil
	}
	if u.provisions > ration*float64(homeTicks+1)+1e-9 {
		return nil
	}
	// A reachable town at the ship's current destination is its nearer food
	// stop, irrespective of its ruler. Arrival hospitality owns the refill.
	if position.Status == "marching" && position.ArriveTick != nil && position.TargetQ != nil && position.TargetR != nil {
		targetQ, targetR := *position.TargetQ, *position.TargetR
		isCityMission := intent == nil || *intent == "" || *intent == "passage" || *intent == "land" || *intent == "pickup"
		if landQ != nil && landR != nil {
			targetQ, targetR = *landQ, *landR
		}
		var city bool
		if isCityMission {
			err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM settlements s JOIN provinces p ON p.id=s.province_id WHERE s.world_id=$1 AND s.state='active' AND p.coastal AND greatest(abs(p.map_q-$2),abs(p.map_r-$3),abs(p.map_q+p.map_r-$2-$3))<=1)`, world, targetQ, targetR).Scan(&city)
			if err != nil {
				return err
			}
		}
		remaining := max(0, *position.ArriveTick-anchor.Tick)
		if city && u.provisions+1e-9 >= ration*float64(remaining) {
			return nil
		}
	}
	u.homeSettlementID = &home.id
	if err = h.arrivals.dispatchReturnHome(ctx, tx, u, here.Q, here.R, world, returnReasonFood); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func shipRationTx(ctx context.Context, tx pgx.Tx, u unitRow) (float64, error) {
	cargoKind := ""
	cargoSize := 0
	if u.cargoUnitID != nil {
		if err := tx.QueryRow(ctx, `SELECT type,size FROM units WHERE id=$1 AND status='embarked'`, *u.cargoUnitID).Scan(&cargoKind, &cargoSize); err != nil && err != pgx.ErrNoRows {
			return 0, err
		}
	}
	return VoyageRation(u.utype, u.size, cargoKind, cargoSize, u.crew), nil
}

// foodReturned reports only physical arrival home. A captured/removed home is
// resolved by the existing expedition destination owner before any garrison.
func (h *UnitArrivalHandler) foodReturned(ctx context.Context, tx pgx.Tx, u unitRow, q, r int, world uuid.UUID) error {
	home, err := expeditionReturnDestination(ctx, tx, u, q, r, world)
	if err != nil {
		return err
	}
	if home == nil {
		return h.arriveGarrison(ctx, tx, u, q, r, nil, world)
	}
	u.homeSettlementID = &home.id
	if q != home.q || r != home.r {
		return h.dispatchReturnHome(ctx, tx, u, q, r, world, returnReasonFood)
	}
	if err = h.exploreReturned(ctx, tx, u, q, r, world); err != nil {
		return err
	}
	body := map[string]any{"unit_id": u.id, "name": unit.LoadDisplayName(ctx, tx, u.id), "crew_after": u.crew, "reason": "food_for_way_home", "q": q, "r": r, "settlement_id": home.id}
	event, err := h.eventStore.AppendTx(ctx, tx, u.id, events.StreamType(unit.StreamUnit), "ShipCameHomeForFood", body, world, nil)
	if err != nil {
		return err
	}
	return h.expeditionOutcome(ctx, tx, event, world, u.ownerID, "ShipCameHomeForFood", 3, body)
}

// hospitalityAtPort uses the same combat return route/speed as the captain.
// Call only for a physically reached town, before an immediate onward leg.
func hospitalityAtPort(ctx context.Context, tx pgx.Tx, u unitRow, port, world uuid.UUID) error {
	var q, r int
	if err := tx.QueryRow(ctx, `SELECT p.map_q,p.map_r FROM settlements s JOIN provinces p ON p.id=s.province_id WHERE s.id=$1 AND s.world_id=$2`, port, world).Scan(&q, &r); err != nil {
		return err
	}
	seaQ, seaR, found, err := province.NearestSeaNeighbor(ctx, tx, world, q, r)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	home, err := expeditionReturnDestination(ctx, tx, u, seaQ, seaR, world)
	if err != nil {
		return err
	}
	if home == nil || home.id == port {
		return nil
	}
	ration, err := shipRationTx(ctx, tx, u)
	if err != nil {
		return err
	}
	needed := ration * float64(legTravelTicks(unit.Type(u.utype), u.crew, u.cargoUnitID != nil, home.cost)+1)
	if math.IsNaN(needed) || math.IsInf(needed, 0) {
		return fmt.Errorf("invalid home food reserve")
	}
	return economy.HospitalityFood(ctx, tx, u.id, port, needed)
}

// resolve a town explicitly targeted by a mission before dispatching its next
// leg; the common arrival witness finalizer may run after that leg starts.
func hospitalityAtTarget(ctx context.Context, tx pgx.Tx, u unitRow, q, r int, world uuid.UUID) error {
	var port uuid.UUID
	err := tx.QueryRow(ctx, `SELECT s.id FROM settlements s JOIN provinces p ON p.id=s.province_id WHERE s.world_id=$1 AND s.state='active' AND (p.coastal OR EXISTS(SELECT 1 FROM buildings b WHERE b.settlement_id=s.id AND b.building_type='harbour')) AND p.map_q=$2 AND p.map_r=$3`, world, q, r).Scan(&port)
	if err == pgx.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	return hospitalityAtPort(ctx, tx, u, port, world)
}
