package combat

// Ship-at-sea order gating (megaron_plan_skeppsuppdrag_landsatt.md R3/R4).
//
// Two small, DB-driven helpers shared by dispatch (march_start.go), arrival
// (unit_arrival.go) and the two order-INTAKE points (api/handlers/unit.go,
// internal/messenger/order_delivery.go) — kept out of all of them to avoid
// duplicating the same "settlement of mine sits next to this hex" query.

import (
	"context"
	"net/http"
	"time"

	"formatet/megaron/server/internal/hexgrid"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/unit"
	"github.com/google/uuid"
)

// friendlySettlementAdjacent reports the id of an ACTIVE settlement owned by
// ownerID that sits directly next to (q,r), if any — R4's "ends in a port"
// test for a naval unit's plain march. Mirrors province.NearestSeaNeighbor's
// own neighbour-scan shape, but the other direction (land next to the ship's
// sea target, not sea next to a settlement).
func friendlySettlementAdjacent(ctx context.Context, db province.Queryer, worldID, ownerID uuid.UUID, q, r int) (uuid.UUID, bool, error) {
	for _, d := range hexgrid.Neighbors(hexgrid.Coord{Q: q, R: r}) {
		rows, err := db.Query(ctx,
			`SELECT s.id FROM provinces p JOIN settlements s ON s.province_id = p.id
			 WHERE p.world_id = $1 AND p.map_q = $2 AND p.map_r = $3
			   AND s.owner_id = $4 AND s.state = 'active'`,
			worldID, d.Q, d.R, ownerID,
		)
		if err != nil {
			return uuid.Nil, false, err
		}
		var found bool
		var id uuid.UUID
		if rows.Next() {
			if scanErr := rows.Scan(&id); scanErr != nil {
				rows.Close()
				return uuid.Nil, false, scanErr
			}
			found = true
		}
		rows.Close()
		if found {
			return id, true, nil
		}
	}
	return uuid.Nil, false, nil
}

// ownerHasNoSettlements reports whether ownerID holds zero active settlements
// in worldID — R6's stranded-ship trigger (transport.StrandShip): such an
// owner can never have a ship sent home (dispatchReturnHome has nowhere to
// route it), so RequireShipInPort exempts their ships from the port
// requirement below.
func ownerHasNoSettlements(ctx context.Context, db province.Queryer, worldID, ownerID uuid.UUID) bool {
	rows, err := db.Query(ctx,
		`SELECT 1 FROM settlements WHERE world_id = $1 AND owner_id = $2 AND state = 'active' LIMIT 1`,
		worldID, ownerID)
	if err != nil {
		return false // fail closed: an owner we can't check is treated as having a settlement
	}
	defer rows.Close()
	return !rows.Next()
}

// RequireShipInPort enforces R3: no order — Runner-borne or immediate —
// reaches a naval unit unless it is docked at its own port (status=garrison).
// A ship's mission already carries it home on its own (R1/R5/R6); an order to
// a ship at sea would promise a command the messenger pillar cannot deliver
// (nothing reaches a moving/standing-off ship). Always nil for a land unit or
// a ship already in port. name is the caller's already-resolved display name
// (unit.LoadDisplayName) — this file has no QueryRow-shaped handle of its
// own, only province.Queryer's Query.
//
// Exception: an owner with NO active settlement at all (see
// ownerHasNoSettlements) is the sole case allowed to keep giving orders to a
// ship that isn't in port — otherwise a stranded ship would be lost to the
// player forever (R6). Checked fresh every call, never cached: the moment the
// owner founds or recaptures a settlement, the exception stops applying.
//
// Deliberately NOT called from inside StartMarch/SetStance/ExecuteRecall/
// SetStandingOrders — those stay the shared validate+execute core other
// callers rely on unchanged, in particular
// TestStartMarch_ExploreFromFieldPositionResolvesNearestOwnedHome, which
// drives StartMarch directly against a field-positioned ship whose owner HAS
// a settlement and must keep succeeding (the P7 fix it pins). Call this at
// the two order-INTAKE points instead — the synchronous API handler and the
// order-courier delivery handler — so a courier is never even sent toward a
// ship that will always refuse it, and a stale courier already in flight
// across this deploy still gets an honest OrderFailed.
func RequireShipInPort(ctx context.Context, db province.Queryer, worldID, ownerID uuid.UUID, category unit.Category, status unit.Status, name string, targetQ, targetR *int, arrivesAt *time.Time) *OrderReject {
	if category != unit.CategoryNaval || status == unit.StatusGarrison {
		return nil
	}
	if ownerHasNoSettlements(ctx, db, worldID, ownerID) {
		return nil // stranded exception — R6
	}
	if name == "" {
		name = "your ship"
	}
	if status == unit.StatusMarching && targetQ != nil && targetR != nil && arrivesAt != nil {
		return reject(http.StatusUnprocessableEntity,
			"ships at sea take no orders — %s is on its mission to (%d,%d) and returns home %s",
			name, *targetQ, *targetR, arrivesAt.Local().Format(time.RFC3339))
	}
	return reject(http.StatusUnprocessableEntity,
		"ships at sea take no orders — %s is on its mission and will return to port on its own", name)
}
