// Package transport models goods in physical transit across the map — the caravans
// and ships that carry an internal transfer or a trade delivery from one settlement
// to another. Unlike the old abstract scheduled-event model, a transport has an
// origin and destination hex and a route, so its live position can be computed
// (movement.PositionAt) for rendering and interception.
//
// G1: transport uses province, movement, tick, events and clock (downward deps), so
// despite living conceptually at the messenger tier it is safe for combat (Del 2b
// sack, internal/combat/sack.go) to call transport.Dispatch directly for plunder
// caravans — there is no cycle. CLAUDE.md's G1 diagram predates this package and
// doesn't list it; treat transport as sitting just above province/settlement.
package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/movement"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/tick"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Manifest maps a good key to the quantity a transport carries. Silver is a good
// like any other here.
type Manifest map[string]float64

// DispatchParams describes one caravan/ship to send. The caller must already have
// deducted the manifest goods from the source in the same transaction — Dispatch
// only creates the mover and schedules its arrival.
type DispatchParams struct {
	// Journey is planned before the caller mutates goods or binds its ship.
	Journey       *province.TradeJourney
	DepartedTick  int
	WorldID       uuid.UUID
	OwnerID       uuid.UUID
	Kind          string // "transfer" | "trade" | "trade_return"
	OriginID      uuid.UUID
	DestID        uuid.UUID
	Category      string // "land" (caravan) | "naval" (ship)
	OriginQ       int
	OriginR       int
	DestQ         int
	DestR         int
	DepartsAt     time.Time
	ArrivesAt     time.Time
	DueTick       int
	Manifest      Manifest
	Interceptable bool
	// StandingOrderID tags this mover as belonging to a standing order
	// (megaron_plan_staende_leverans.md) — nil for every other transport kind.
	// The standing-order sweep (internal/combat) reads this column back to
	// find "is a caravan already in flight for this route" without a second
	// table; nothing else in the package reads or writes it.
	StandingOrderID *uuid.UUID
	// ShipUnitID (megaron_plan_sjohandel_kraver_skepp.md R2) is the real
	// galley/merchantman bound to this naval mover — nil for every land
	// caravan and for a naval transport predating this slice (R6). Read back
	// by InterceptScanHandler.seize (R5, which ship is at risk) and by
	// ArrivalHandler (R3/R5-limped: kind "ship_return"/"damaged_return" and a
	// non-nil ShipUnitID means "release this ship on arrival").
	ShipUnitID *uuid.UUID
}

// Dispatch inserts a transport mover and its goods manifest, then schedules the
// generic ScheduledTransportArrival. Use this for movers whose arrival is handled
// by the transport ArrivalHandler (e.g. internal transfers). Returns the new id.
func Dispatch(ctx context.Context, tx pgx.Tx, sched *events.Scheduler, p DispatchParams) (uuid.UUID, error) {
	p, err := prepareJourney(ctx, tx, p)
	if err != nil {
		return uuid.Nil, err
	}
	id, err := insertRow(ctx, tx, p)
	if err != nil {
		return uuid.Nil, err
	}
	if err := sched.EnqueueTickTx(ctx, tx, p.WorldID, events.ScheduledTransportArrival,
		map[string]any{"transport_id": id}, p.DueTick); err != nil {
		return uuid.Nil, fmt.Errorf("schedule transport arrival: %w", err)
	}
	return id, nil
}

// CreateShadow inserts a transport mover and its manifest WITHOUT scheduling an
// arrival. Use it for legs whose arrival is already driven by a domain-specific
// event/handler (e.g. trade delivery/return): the physical row exists purely for
// map position and interception, and that handler marks it delivered/lost itself.
func CreateShadow(ctx context.Context, tx pgx.Tx, p DispatchParams) (uuid.UUID, error) {
	p, err := prepareJourney(ctx, tx, p)
	if err != nil {
		return uuid.Nil, err
	}
	return insertRow(ctx, tx, p)
}

func insertRow(ctx context.Context, tx pgx.Tx, p DispatchParams) (uuid.UUID, error) {
	raw, err := json.Marshal(p.Journey)
	if err != nil {
		return uuid.Nil, err
	}
	var id uuid.UUID
	if err := tx.QueryRow(ctx,
		`INSERT INTO transports
		   (world_id, owner_id, kind, origin_id, dest_id, category,
		    origin_q, origin_r, dest_q, dest_r, departs_at, arrives_at, due_tick, interceptable,
		    standing_order_id, ship_unit_id, journey, departed_tick)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)
		 RETURNING id`,
		p.WorldID, p.OwnerID, p.Kind, p.OriginID, p.DestID, p.Category,
		p.OriginQ, p.OriginR, p.DestQ, p.DestR, p.DepartsAt, p.ArrivesAt, p.DueTick, p.Interceptable,
		p.StandingOrderID, p.ShipUnitID, raw, p.DepartedTick,
	).Scan(&id); err != nil {
		return uuid.Nil, fmt.Errorf("insert transport: %w", err)
	}

	for good, qty := range p.Manifest {
		if qty <= 0 {
			continue
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO transport_goods (transport_id, good_key, quantity) VALUES ($1,$2,$3)`,
			id, good, qty,
		); err != nil {
			return uuid.Nil, fmt.Errorf("insert transport manifest %q: %w", good, err)
		}
	}
	return id, nil
}

// CurrentPosition is compatibility for pre-migration160 NULL-journey rows only.
// New transports must use SavedPosition, never search their route at read time.
func CurrentPosition(
	ctx context.Context, db province.Queryer, worldID uuid.UUID,
	originQ, originR, destQ, destR int, category string,
	departsAt, arrivesAt, now time.Time,
) (province.MapPosition, bool, error) {
	return province.InterpolatePosition(ctx, db, worldID,
		province.MapPosition{Q: originQ, R: originR},
		province.MapPosition{Q: destQ, R: destR},
		category, departsAt, arrivesAt, now,
	)
}

// prepareJourney makes new writers fail closed even when a caller omitted its
// plan. Normal consumers pass the plan they used for their preflight decision.
func prepareJourney(ctx context.Context, tx pgx.Tx, p DispatchParams) (DispatchParams, error) {
	if p.Category == "naval" && p.ShipUnitID == nil {
		return p, fmt.Errorf("naval transport requires a real ship")
	}
	if p.Category == "naval" {
		var category, kind, status string
		var owner uuid.UUID
		if err := tx.QueryRow(ctx, `SELECT category,type,status,owner_id FROM units WHERE id=$1 AND world_id=$2 FOR UPDATE`, *p.ShipUnitID, p.WorldID).Scan(&category, &kind, &status, &owner); err != nil {
			return p, fmt.Errorf("load naval carrier: %w", err)
		}
		if category != "naval" || (kind != "galley" && kind != "merchantman") || status != "freighting" || owner != p.OwnerID {
			return p, fmt.Errorf("naval transport requires its owner's bound galley or merchantman")
		}
		var active bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM transports WHERE ship_unit_id=$1 AND status='in_transit')`, *p.ShipUnitID).Scan(&active); err != nil {
			return p, err
		}
		if active {
			return p, fmt.Errorf("ship already carries an active transport")
		}
	}

	if p.Journey == nil {
		j, err := province.PlanTradeJourney(ctx, tx, p.WorldID, province.MapPosition{Q: p.OriginQ, R: p.OriginR}, province.MapPosition{Q: p.DestQ, R: p.DestR}, p.Category)
		if err != nil {
			return p, err
		}
		p.Journey = &j
		if err := tx.QueryRow(ctx, `SELECT current_tick FROM worlds WHERE id=$1`, p.WorldID).Scan(&p.DepartedTick); err != nil {
			return p, err
		}
	}
	if err := p.Journey.Validate(); err != nil {
		return p, err
	}
	if p.Journey.Category != p.Category {
		return p, fmt.Errorf("journey category differs from transport")
	}
	origin, dest := province.MapPosition{Q: p.OriginQ, R: p.OriginR}, province.MapPosition{Q: p.DestQ, R: p.DestR}
	allowed := 0
	if p.Category == "naval" {
		allowed = 1
	}
	if province.HexDistance(origin, p.Journey.Path[0]) > allowed || province.HexDistance(dest, p.Journey.Path[len(p.Journey.Path)-1]) > allowed {
		return p, fmt.Errorf("journey endpoints differ from transport")
	}

	p.DueTick = p.DepartedTick + p.Journey.TravelTicks
	p.ArrivesAt = p.DepartsAt.Add(tick.RealUntil(p.DueTick, p.DepartedTick))
	// Timestamp fields are compatibility projections only; tick time owns ETA.
	return p, nil
}

// SavedPosition reads a frozen journey through the shared movement core. at is
// a world time in thousandths of a tick. It never reads tiles or reroutes.
func SavedPosition(raw []byte, departedTick, dueTick int, at int64) (province.MapPosition, error) {
	var j province.TradeJourney
	if err := json.Unmarshal(raw, &j); err != nil {
		return province.MapPosition{}, err
	}
	if err := j.Validate(); err != nil {
		return province.MapPosition{}, err
	}
	if dueTick-departedTick != j.TravelTicks {
		return province.MapPosition{}, fmt.Errorf("transport duration differs from saved journey")
	}
	if len(j.Path) == 1 {
		return j.Path[0], nil
	}
	hexes := make([]movement.Hex, len(j.Path))
	for i, p := range j.Path {
		hexes[i] = movement.Hex{Q: p.Q, R: p.R}
	}
	pos, err := movement.PositionAt(movement.Move{StartTick: departedTick, EndTick: dueTick, Hexes: hexes, Costs: j.StepCosts}, movement.Milli(at))
	return province.MapPosition{Q: pos.Q, R: pos.R}, err
}
