// Package transport models goods in physical transit across the map — the caravans
// and ships that carry an internal transfer or a trade delivery from one settlement
// to another. Unlike the old abstract scheduled-event model, a transport has an
// origin and destination hex and a route, so its live position can be computed
// (province.InterpolatePosition) for rendering and, later, interception.
//
// G1: transport itself only uses province, events, clock (no upward deps), so
// despite living conceptually at the messenger tier it is safe for combat (Del 2b
// sack, internal/combat/sack.go) to call transport.Dispatch directly for plunder
// caravans — there is no cycle. CLAUDE.md's G1 diagram predates this package and
// doesn't list it; treat transport as sitting just above province/settlement.
package transport

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/province"
)

// Manifest maps a good key to the quantity a transport carries. Silver is a good
// like any other here.
type Manifest map[string]float64

// DispatchParams describes one caravan/ship to send. The caller must already have
// deducted the manifest goods from the source in the same transaction — Dispatch
// only creates the mover and schedules its arrival.
type DispatchParams struct {
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
}

// Dispatch inserts a transport mover and its goods manifest, then schedules the
// generic ScheduledTransportArrival. Use this for movers whose arrival is handled
// by the transport ArrivalHandler (e.g. internal transfers). Returns the new id.
func Dispatch(ctx context.Context, tx pgx.Tx, sched *events.Scheduler, p DispatchParams) (uuid.UUID, error) {
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
	return insertRow(ctx, tx, p)
}

func insertRow(ctx context.Context, tx pgx.Tx, p DispatchParams) (uuid.UUID, error) {
	var id uuid.UUID
	if err := tx.QueryRow(ctx,
		`INSERT INTO transports
		   (world_id, owner_id, kind, origin_id, dest_id, category,
		    origin_q, origin_r, dest_q, dest_r, departs_at, arrives_at, due_tick, interceptable)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		 RETURNING id`,
		p.WorldID, p.OwnerID, p.Kind, p.OriginID, p.DestID, p.Category,
		p.OriginQ, p.OriginR, p.DestQ, p.DestR, p.DepartsAt, p.ArrivesAt, p.DueTick, p.Interceptable,
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

// CurrentPosition returns the hex a live transport currently occupies, re-walking
// its FindPath route in proportion to elapsed travel time (province.InterpolatePosition).
// ok is false when the route can no longer be found; callers fall back to the origin.
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
