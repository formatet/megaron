package combat

import (
	"context"
	"fmt"
	"time"

	"formatet/megaron/server/internal/carrier"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/transport"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Naval disappearance from upkeep is also a physical carrier death. Keep
// crew/size accounting as before, but freeze the passenger outcome in that TX.
func (h *UpkeepHandler) disbandNavalCarrier(ctx context.Context, u upkeepUnitRow, world uuid.UUID, reason string, unpaid *int) error {
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var state carrierLocation
	if err := tx.QueryRow(ctx, `SELECT status,settlement_id,q,r,target_q,target_r,march_route,depart_tick,arrive_tick,departs_at,arrives_at FROM units WHERE id=$1 AND world_id=$2 FOR UPDATE`, u.id, world).Scan(&state.status, &state.settlement, &state.q, &state.r, &state.tq, &state.tr, &state.route, &state.departTick, &state.arriveTick, &state.departs, &state.arrives); err != nil {
		return err
	}
	if state.status == "disbanded" {
		return tx.Commit(ctx)
	}
	has, err := carrier.HasPassengerTx(ctx, tx, world, u.id)
	if err != nil {
		return err
	}
	var witnesses []*events.Event
	if has {
		var currentTick int
		if err := tx.QueryRow(ctx, `SELECT current_world_tick()`).Scan(&currentTick); err != nil {
			return err
		}
		settlement, q, r, err := h.carrierDeathLocation(ctx, tx, world, u.id, state, currentTick)
		if err != nil {
			return err
		}
		if settlement != nil {
			witnesses, err = carrier.DisembarkTx(ctx, tx, h.store, world, u.id, *settlement, currentTick)
		} else {
			witnesses, err = carrier.OutcomeTx(ctx, tx, h.store, world, u.id, nil, reason, q, r, currentTick)
		}
		if err != nil {
			return err
		}
	}
	if unpaid == nil {
		_, err = tx.Exec(ctx, `UPDATE units SET status='disbanded',crew=0,updated_at=now() WHERE id=$1`, u.id)
	} else {
		_, err = tx.Exec(ctx, `UPDATE units SET status='disbanded',size=0,unpaid_periods=$2,updated_at=now() WHERE id=$1`, u.id, *unpaid)
	}
	if err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	for _, w := range witnesses {
		h.store.RecordCommitted(ctx, w)
	}
	return nil
}

type carrierLocation struct {
	status                 string
	settlement             *uuid.UUID
	q, r, tq, tr           *int
	route                  []byte
	departTick, arriveTick *int
	departs, arrives       *time.Time
}

// Sailing ships still carry frozen departure coordinates/settlement_id. Use
// the committed route at this upkeep tick, never that stale "home" anchor.
func (h *UpkeepHandler) carrierDeathLocation(ctx context.Context, tx pgx.Tx, world, ship uuid.UUID, s carrierLocation, currentTick int) (*uuid.UUID, int, int, error) {
	var pos province.MapPosition
	known := false
	if s.status == "freighting" {
		var raw []byte
		var departure *int
		var due, oq, or, dq, dr int
		var departs, arrives time.Time
		err := tx.QueryRow(ctx, `SELECT journey,departed_tick,due_tick,origin_q,origin_r,dest_q,dest_r,departs_at,arrives_at FROM transports WHERE ship_unit_id=$1 AND world_id=$2 AND status='in_transit' ORDER BY due_tick,id LIMIT 1`, ship, world).Scan(&raw, &departure, &due, &oq, &or, &dq, &dr, &departs, &arrives)
		if err == nil {
			if len(raw) > 0 && departure != nil {
				pos, err = transport.SavedPosition(raw, *departure, due, int64(currentTick)*1000)
				known = err == nil
			} else if h.arrivals != nil && h.arrivals.clk != nil {
				pos, known, err = province.InterpolatePosition(ctx, tx, world, province.MapPosition{Q: oq, R: or}, province.MapPosition{Q: dq, R: dr}, "naval", departs, arrives, h.arrivals.clk.Now())
			} else {
				return nil, 0, 0, fmt.Errorf("legacy freight passenger has no injected clock")
			}
			if err != nil {
				return nil, 0, 0, err
			}
		} else if err != pgx.ErrNoRows {
			return nil, 0, 0, err
		} else {
			// A standing route can hold a bound ship between voyages. Its original
			// settlement_id is stale; the last DELIVERED leg names its actual port.
			var last *uuid.UUID
			err := tx.QueryRow(ctx, `SELECT dest_id FROM transports WHERE ship_unit_id=$1 AND world_id=$2 AND status='delivered' ORDER BY updated_at DESC,id DESC LIMIT 1`, ship, world).Scan(&last)
			if err == nil && last != nil {
				s.settlement = last
			} else if err != nil && err != pgx.ErrNoRows {
				return nil, 0, 0, err
			}
		}
	}
	if s.status == "marching" {
		if route, active := LoadActiveRoute(s.route, s.status, s.departTick, s.arriveTick); active {
			var err error
			pos, err = RoutePositionAt(route, int64(currentTick)*1000)
			if err != nil {
				return nil, 0, 0, err
			}
			known = true
		} else if s.q != nil && s.r != nil && s.tq != nil && s.tr != nil && s.departs != nil && s.arrives != nil && h.arrivals != nil && h.arrivals.clk != nil {
			var err error
			pos, known, err = province.InterpolatePosition(ctx, tx, world, province.MapPosition{Q: *s.q, R: *s.r}, province.MapPosition{Q: *s.tq, R: *s.tr}, "naval", *s.departs, *s.arrives, h.arrivals.clk.Now())
			if err != nil {
				return nil, 0, 0, err
			}
		} else {
			return nil, 0, 0, fmt.Errorf("marching carrier passenger has no physical route")
		}
	}
	if !known && s.settlement != nil {
		err := tx.QueryRow(ctx, `SELECT p.map_q,p.map_r FROM settlements s JOIN provinces p ON p.id=s.province_id WHERE s.id=$1 AND s.world_id=$2 AND s.state='active'`, *s.settlement, world).Scan(&pos.Q, &pos.R)
		if err == nil {
			return s.settlement, pos.Q, pos.R, nil
		}
		if err != pgx.ErrNoRows {
			return nil, 0, 0, err
		}
	}
	if !known {
		if s.q == nil || s.r == nil {
			return nil, 0, 0, fmt.Errorf("carrier passenger location unknown")
		}
		pos = province.MapPosition{Q: *s.q, R: *s.r}
	}
	// First try a settlement ON this hex, then the real adjacent shore (any
	// owner). Both are physically walkable; neither guesses a home port.
	var settlement uuid.UUID
	err := tx.QueryRow(ctx, `SELECT s.id FROM settlements s JOIN provinces p ON p.id=s.province_id WHERE s.world_id=$1 AND s.state='active' AND p.map_q=$2 AND p.map_r=$3`, world, pos.Q, pos.R).Scan(&settlement)
	if err == nil {
		return &settlement, pos.Q, pos.R, nil
	}
	if err != pgx.ErrNoRows {
		return nil, 0, 0, err
	}
	id, q, r, found, err := province.NearestSettlementNeighbor(ctx, tx, world, pos.Q, pos.R)
	if err != nil {
		return nil, 0, 0, err
	}
	if found {
		return &id, q, r, nil
	}
	return nil, pos.Q, pos.R, nil
}
