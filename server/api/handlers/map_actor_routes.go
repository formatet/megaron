package handlers

import (
	"context"
	"encoding/json"
	"time"

	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/movement"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/tick"
	"formatet/megaron/server/internal/transport"
	"github.com/google/uuid"
)

// Observed civilian actors carry only what current sight reveals. Parties keep
// their existing shipment/message information in the separate full projection.
func foreignCivilianProjection(id uuid.UUID, currentQ, currentR *int, heading, kind, owner string, caravan bool) map[string]any {
	out := map[string]any{"id": id, "current_q": currentQ, "current_r": currentR, "heading": heading}
	if caravan {
		out["mine"] = false
		out["owner"] = owner
		out["category"] = kind
		out["role"] = ""
	} else {
		out["own"] = false
		out["sender"] = owner
		out["kind"] = kind
	}
	return out
}
func pathCoordinates(path []province.MapPosition) [][2]int {
	out := make([][2]int, len(path))
	for i, p := range path {
		out[i] = [2]int{p.Q, p.R}
	}
	return out
}

// Only the two legs of accepted bilateral trade establish the recipient's
// advance route knowledge. Gifts need no consent; own logistics/ship returns
// are known through ownership, never by reclassifying their recipient.
func agreedTransportKind(kind string) bool {
	return kind == "trade" || kind == "trade_return"
}

// The compass is the same ScreenCompass used by ReadMarch, but its current
// step comes from the frozen journey's weighted clock, matching SavedPosition.
func frozenHeading(path []province.MapPosition, costs []int64, start, end int, at int64) string {
	if len(path) < 2 {
		return ""
	}
	hexes := make([]movement.Hex, len(path))
	for i, p := range path {
		hexes[i] = movement.Hex{Q: p.Q, R: p.R}
	}
	bounds, err := movement.Boundaries(movement.Move{StartTick: start, EndTick: end, Hexes: hexes, Costs: costs})
	if err != nil {
		return ""
	}
	idx := 0
	for i := 0; i < len(bounds)-1; i++ {
		if int64(bounds[i]) <= at {
			idx = i + 1
		}
	}
	if idx >= len(path)-1 {
		idx = len(path) - 2
	}
	return province.ScreenCompass(path[idx], path[idx+1])
}

type carrierRoute struct {
	Path               [][2]int
	Costs              []int64
	Start, End         int
	CurrentQ, CurrentR int
	Heading            string
}

func (h *WorldHandler) messengerCarrierRoute(ctx context.Context, worldID uuid.UUID, carrierUnit, carrierTransport *uuid.UUID, now time.Time) *carrierRoute {
	anchor, err := tick.LoadAnchor(ctx, h.pool, worldID)
	if err != nil {
		return nil
	}
	at := anchor.MilliAt(now)
	if carrierTransport != nil {
		var raw []byte
		var start *int
		var end int
		if err := h.pool.QueryRow(ctx, `SELECT journey,departed_tick,due_tick FROM transports WHERE id=$1 AND world_id=$2 AND status='in_transit'`, *carrierTransport, worldID).Scan(&raw, &start, &end); err != nil || start == nil {
			return nil
		}
		pos, err := transport.SavedPosition(raw, *start, end, at)
		if err != nil {
			return nil
		}
		var j province.TradeJourney
		if json.Unmarshal(raw, &j) != nil {
			return nil
		}
		return &carrierRoute{pathCoordinates(j.Path), j.StepCosts, *start, end, pos.Q, pos.R, frozenHeading(j.Path, j.StepCosts, *start, end, at)}
	}
	if carrierUnit != nil {
		var raw []byte
		var start, end *int
		var status string
		if err := h.pool.QueryRow(ctx, `SELECT march_route,depart_tick,arrive_tick,status FROM units WHERE id=$1 AND world_id=$2`, *carrierUnit, worldID).Scan(&raw, &start, &end, &status); err != nil {
			return nil
		}
		r, ok := combat.LoadActiveRoute(raw, status, start, end)
		if !ok {
			return nil
		}
		pos, err := combat.RoutePositionAt(r, at)
		if err != nil {
			return nil
		}
		path := make([]province.MapPosition, len(r.Hexes))
		for i, h := range r.Hexes {
			path[i] = province.MapPosition{Q: h[0], R: h[1]}
		}
		return &carrierRoute{r.Hexes, r.Costs, r.StartTick, r.EndTick, pos.Q, pos.R, frozenHeading(path, r.Costs, r.StartTick, r.EndTick, at)}
	}
	return nil
}
