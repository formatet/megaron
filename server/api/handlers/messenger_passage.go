package handlers

// ArrangePassage — R1, megaron_plan_ordna_passage.md, slice 3b-3.
//
// POST /worlds/{worldID}/messengers/{messengerID}/passage {ship_id}: a Wanax
// arranges for one of their own ships, standing in port, to carry a runner
// that is currently 'awaiting_passage'. See combat.StartMarch's "passage"
// intent for the ship's own march, and combat.UnitArrivalHandler.
// passageArrived / messenger.PassageScanHandler's release phase for what
// happens at either end of the voyage.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/hexgrid"
	"formatet/megaron/server/internal/messenger"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/unit"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PassageHandler handles the passage-arranging HTTP endpoint. Kept separate
// from MessengerHandler so its constructor's signature never has to change
// (several test files construct MessengerHandler directly) — this handler
// needs an *events.Store for combat.StartMarch, which MessengerHandler does
// not otherwise hold.
type PassageHandler struct {
	pool       *pgxpool.Pool
	scheduler  *events.Scheduler
	eventStore *events.Store
	clk        clock.Clock
}

// NewPassageHandler creates a PassageHandler.
func NewPassageHandler(pool *pgxpool.Pool, scheduler *events.Scheduler, eventStore *events.Store, clk clock.Clock) *PassageHandler {
	return &PassageHandler{pool: pool, scheduler: scheduler, eventStore: eventStore, clk: clk}
}

// Arrange handles POST /worlds/{worldID}/messengers/{messengerID}/passage.
func (h *PassageHandler) Arrange(w http.ResponseWriter, r *http.Request) {
	playerID, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}
	messengerID, err := uuid.Parse(chi.URLParam(r, "messengerID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid messenger ID")
		return
	}

	var req struct {
		ShipID uuid.UUID `json:"ship_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.ShipID == uuid.Nil {
		writeError(w, http.StatusBadRequest, "ship_id is required")
		return
	}

	ctx := r.Context()

	// Load and validate the runner: must be the caller's own, and actually
	// waiting for passage right now (R1's first condition).
	var senderID uuid.UUID
	var msgWorldID uuid.UUID
	var passageStatus *string
	var passagePortID *uuid.UUID
	if err := h.pool.QueryRow(ctx,
		`SELECT sender_id, world_id, passage_status, passage_port_id FROM messengers WHERE id = $1`,
		messengerID,
	).Scan(&senderID, &msgWorldID, &passageStatus, &passagePortID); err != nil {
		writeError(w, http.StatusNotFound, "messenger not found")
		return
	}
	if msgWorldID != worldID {
		writeError(w, http.StatusForbidden, "messenger not in this world")
		return
	}
	if senderID != playerID {
		writeError(w, http.StatusForbidden, "not your runner")
		return
	}
	if passageStatus == nil || *passageStatus != "awaiting_passage" || passagePortID == nil {
		writeError(w, http.StatusUnprocessableEntity, "this runner is not waiting for passage")
		return
	}

	// The port the runner is standing in, and who holds it — decides whether
	// this is the outbound case (own port: ship must dock there) or the
	// pickup case (foreign port: any own port will do).
	var portOwnerID uuid.UUID
	var portName string
	var portQ, portR int
	if err := h.pool.QueryRow(ctx,
		`SELECT s.owner_id, s.name, p.map_q, p.map_r
		   FROM settlements s JOIN provinces p ON p.id = s.province_id WHERE s.id = $1`,
		*passagePortID,
	).Scan(&portOwnerID, &portName, &portQ, &portR); err != nil {
		writeError(w, http.StatusInternalServerError, "could not load the runner's port")
		return
	}
	ownPort := portOwnerID == playerID

	// Load and validate the ship (R1's second/third/fourth conditions).
	var shipOwnerID, shipWorldID uuid.UUID
	var shipType, shipStatus string
	var shipSettlementID *uuid.UUID
	if err := h.pool.QueryRow(ctx,
		`SELECT owner_id, world_id, type, status, settlement_id FROM units WHERE id = $1`,
		req.ShipID,
	).Scan(&shipOwnerID, &shipWorldID, &shipType, &shipStatus, &shipSettlementID); err != nil {
		writeError(w, http.StatusNotFound, "ship not found")
		return
	}
	if shipWorldID != worldID {
		writeError(w, http.StatusForbidden, "ship not in this world")
		return
	}
	if shipOwnerID != playerID {
		writeError(w, http.StatusForbidden, "not your ship")
		return
	}
	if shipType != string(unit.TypeGalley) && shipType != string(unit.TypeMerchantman) {
		writeError(w, http.StatusUnprocessableEntity,
			"a war galley cannot carry a runner — passage needs a galley or merchantman")
		return
	}
	if shipStatus != string(unit.StatusGarrison) || shipSettlementID == nil {
		writeError(w, http.StatusUnprocessableEntity,
			"the ship must be docked in its own port, free of any mission, to be sent on passage")
		return
	}
	if ownPort && *shipSettlementID != *passagePortID {
		writeError(w, http.StatusUnprocessableEntity,
			fmt.Sprintf("the ship must be docked at %s — the runner is waiting there", portName))
		return
	}

	// R2: resolve the disembark hex.
	var disembarkQ, disembarkR int
	if ownPort {
		targetQ, targetR, ftErr := messenger.FinalTarget(ctx, h.pool, messengerID)
		if ftErr != nil {
			writeError(w, http.StatusInternalServerError, "could not resolve the runner's destination")
			return
		}
		dq, dr, rErr := resolveOutboundDisembark(ctx, h.pool, worldID, *shipSettlementID, targetQ, targetR)
		if rErr != nil {
			writeError(w, http.StatusUnprocessableEntity, rErr.Error())
			return
		}
		disembarkQ, disembarkR = dq, dr
	} else {
		// Hämtning (R2): the disembark hex is the foreign port's own hex — the
		// ship goes to fetch the runner, not to walk it any further.
		disembarkQ, disembarkR = portQ, portR
	}

	msgID := messengerID
	res, err := combat.StartMarch(ctx, h.pool, h.scheduler, h.eventStore, h.clk, combat.MarchOrder{
		WorldID:            worldID,
		PlayerID:           playerID,
		UnitID:             req.ShipID,
		TargetQ:            disembarkQ,
		TargetR:            disembarkR,
		Intent:             "passage",
		PassageMessengerID: &msgID,
	}, nil) // no FOW check: the disembark hex is server-computed, not player-typed
	if err != nil {
		var rej *combat.OrderReject
		if errors.As(err, &rej) {
			writeError(w, rej.Status, rej.Reason)
			return
		}
		writeError(w, http.StatusInternalServerError, "passage failed")
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]any{
		"unit_id":        res.UnitID,
		"messenger_id":   messengerID,
		"departs_at":     res.DepartsAt,
		"arrives_at":     res.ArrivesAt,
		"arrival_tick":   res.ArrivalTick,
		"duration_ticks": res.DurationTicks,
		"disembark_q":    disembarkQ,
		"disembark_r":    disembarkR,
	})
}

// CallBack handles POST /worlds/{worldID}/messengers/{messengerID}/call-back
// — R5's "kalla tillbaka" (megaron_plan_ordna_passage.md, slice 3b-4): turn a
// runner stuck 'awaiting_passage' in the caller's OWN port back home over
// land, delivering nothing. See messenger.CallBack's own doc comment for the
// mechanics and messenger.ErrCallBackNotOwnPort for the foreign-port refusal.
func (h *PassageHandler) CallBack(w http.ResponseWriter, r *http.Request) {
	playerID, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}
	messengerID, err := uuid.Parse(chi.URLParam(r, "messengerID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid messenger ID")
		return
	}

	ctx := r.Context()
	var currentTick int
	_ = h.pool.QueryRow(ctx, `SELECT current_world_tick()`).Scan(&currentTick)

	res, err := messenger.CallBack(ctx, h.pool, h.scheduler, worldID, messengerID, playerID, h.clk.Now(), currentTick)
	if err != nil {
		switch {
		case errors.Is(err, messenger.ErrCallBackNotOwnPort):
			writeError(w, http.StatusUnprocessableEntity, err.Error())
		case errors.Is(err, messenger.ErrCallBackNotYours):
			writeError(w, http.StatusForbidden, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, "call back failed")
		}
		return
	}
	if !res.Started {
		writeError(w, http.StatusUnprocessableEntity, "this runner is not currently waiting for passage")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"messenger_id": messengerID,
		"status":       "returning",
		"returns_at":   res.ReturnsAt,
	})
}

// disembarkSearchMaxRadius bounds resolveOutboundDisembark's ring search
// (below) — generous enough for any world this codebase seeds (64×64 is the
// largest on record, megaron_moc.md), while still refusing to scan an
// unbounded map. A world too large for this radius to ever reach a shore
// gets resolveOutboundDisembark's own honest 422, never a silent hang.
const disembarkSearchMaxRadius = 60

// resolveOutboundDisembark is R2's outbound half: disembark at the runner's
// true destination hex directly if a settlement there is coastal or
// harboured (the common case — most cities are, and the target is already
// known to the player: it is their own runner's destination). Otherwise, the
// nearest EMPTY coastal land hex — reachable by a naval route from the
// ship's own port, with a land route onward to the target — expanding
// outward from the target ring by ring (hexgrid.Ring) and taking the
// shortest-land-cost candidate in the first ring that yields any.
//
// Candidates are never settlements (fixed 2026-09-27, planner review): the
// first version of this search picked among ACTIVE SETTLEMENTS regardless of
// owner or whether the calling player had ever seen them — a Wanax could
// discover a foreign city purely by arranging passage near it, and an order
// to a unit on a landmass with no city at all could never get a disembark
// hex at all. Searching raw, empty coastline instead needs no visibility
// check and no settlement to exist: it never queries anything that could
// leak one, and it works on genuinely empty shores.
func resolveOutboundDisembark(ctx context.Context, pool *pgxpool.Pool, worldID, shipPortSettlementID uuid.UUID, targetQ, targetR int) (int, int, error) {
	if isPort, err := settlementAtHexIsPort(ctx, pool, worldID, targetQ, targetR); err != nil {
		return 0, 0, err
	} else if isPort {
		return targetQ, targetR, nil
	}

	var shipPortQ, shipPortR int
	if err := pool.QueryRow(ctx,
		`SELECT p.map_q, p.map_r FROM provinces p JOIN settlements s ON s.province_id = p.id WHERE s.id = $1`,
		shipPortSettlementID,
	).Scan(&shipPortQ, &shipPortR); err != nil {
		return 0, 0, fmt.Errorf("load ship's own port: %w", err)
	}

	graph, err := province.LoadTileGraph(ctx, pool, worldID)
	if err != nil {
		return 0, 0, err
	}
	shipSeaQ, shipSeaR, foundSea := graphNearestSeaNeighbor(graph, shipPortQ, shipPortR)
	if !foundSea {
		return 0, 0, fmt.Errorf("the ship's own port has no sea approach")
	}

	target := hexgrid.Coord{Q: targetQ, R: targetR}
	for radius := 1; radius <= disembarkSearchMaxRadius; radius++ {
		bestQ, bestR := 0, 0
		bestLandCost := -1.0
		for _, c := range hexgrid.Ring(target, radius) {
			terrain, onMap := graph[[2]int{c.Q, c.R}]
			if !onMap || !isDryLandTerrain(terrain) {
				continue
			}
			candSeaQ, candSeaR, foundCandSea := graphNearestSeaNeighbor(graph, c.Q, c.R)
			if !foundCandSea {
				continue // not itself a coastal hex
			}
			_, landCost, landOK := graph.FindPath(
				province.MapPosition{Q: c.Q, R: c.R}, province.MapPosition{Q: targetQ, R: targetR}, province.CategoryCourierLand)
			if !landOK {
				continue
			}
			_, _, navalOK := graph.FindPath(
				province.MapPosition{Q: shipSeaQ, R: shipSeaR}, province.MapPosition{Q: candSeaQ, R: candSeaR}, "naval")
			if !navalOK {
				continue
			}
			// Never a settlement — any owner, seen or not (see doc comment
			// above). Checked last: it is the only DB round trip per
			// candidate, so cheaper checks eliminate most candidates first.
			if settled, sErr := hexIsSettled(ctx, pool, worldID, c.Q, c.R); sErr != nil {
				return 0, 0, sErr
			} else if settled {
				continue
			}
			if bestLandCost < 0 || landCost < bestLandCost {
				bestQ, bestR, bestLandCost = c.Q, c.R, landCost
			}
		}
		if bestLandCost >= 0 {
			return bestQ, bestR, nil
		}
	}
	return 0, 0, fmt.Errorf(
		"no open shore near the runner's destination that this ship can reach by sea — arrange passage is not possible here")
}

// isDryLandTerrain excludes sea/river (a ship cannot make landfall standing
// in water) and mountains (StartMarch's own passage validation would refuse
// them anyway — see march_start.go's passageMission branch) — the same two
// exclusions "land mission" applies to its own chosen target.
func isDryLandTerrain(terrain string) bool {
	switch terrain {
	case "coastal_sea", "deep_sea", "river", "river_ford",
		"mountain_limestone", "mountain_red":
		return false
	}
	return true
}

// isSeaOrRiverTerrain matches province.NearestSeaNeighbor's own definition of
// "water a ship can float on" — duplicated here (not exported there) so the
// ring search can test it against an in-memory TileGraph instead of paying a
// DB round trip per candidate hex.
func isSeaOrRiverTerrain(terrain string) bool {
	switch terrain {
	case "coastal_sea", "deep_sea", "river", "river_ford":
		return true
	}
	return false
}

// graphNearestSeaNeighbor is province.NearestSeaNeighbor against an
// already-loaded TileGraph, in axialDirs order (hexgrid.Neighbors) — same
// semantics, zero DB round trips, used here once per ring candidate instead
// of once per DB call.
func graphNearestSeaNeighbor(graph province.TileGraph, q, r int) (sq, sr int, found bool) {
	for _, d := range hexgrid.Neighbors(hexgrid.Coord{Q: q, R: r}) {
		if terrain, ok := graph[[2]int{d.Q, d.R}]; ok && isSeaOrRiverTerrain(terrain) {
			return d.Q, d.R, true
		}
	}
	return 0, 0, false
}

// hexIsSettled reports whether an ACTIVE settlement (any owner) sits exactly
// at (q,r) — the one DB check resolveOutboundDisembark's candidate loop
// makes, and the reason a candidate is never chosen on the strength of a
// city existing there (see the function's own doc comment).
func hexIsSettled(ctx context.Context, pool *pgxpool.Pool, worldID uuid.UUID, q, r int) (bool, error) {
	var settled bool
	err := pool.QueryRow(ctx,
		`SELECT EXISTS(
		    SELECT 1 FROM provinces p JOIN settlements s ON s.province_id = p.id
		    WHERE p.world_id = $1 AND p.map_q = $2 AND p.map_r = $3 AND s.state = 'active')`,
		worldID, q, r,
	).Scan(&settled)
	return settled, err
}

// settlementAtHexIsPort reports whether an active settlement sits exactly at
// (q,r) and is coastal or has a harbour — the same predicate messenger.
// ownedCoastalPorts (passage.go) uses for a player's OWN candidate ports,
// widened here to any owner: R2 only asks "can a ship dock adjacent to the
// runner's actual target", not "does the runner's owner hold it".
func settlementAtHexIsPort(ctx context.Context, pool *pgxpool.Pool, worldID uuid.UUID, q, r int) (bool, error) {
	var isPort bool
	err := pool.QueryRow(ctx,
		`SELECT EXISTS(
		    SELECT 1 FROM settlements s JOIN provinces p ON p.id = s.province_id
		    WHERE p.world_id = $1 AND p.map_q = $2 AND p.map_r = $3 AND s.state = 'active'
		      AND (COALESCE(p.coastal, false)
		           OR EXISTS(SELECT 1 FROM buildings b WHERE b.settlement_id = s.id AND b.building_type = 'harbour')))`,
		worldID, q, r,
	).Scan(&isPort)
	return isPort, err
}

// eligiblePassageShip is one ship a Wanax could hand ArrangePassage for a
// given waiting runner — surfaced so the web/keryx layers list real,
// server-validated choices instead of re-deriving R1's rule client-side
// (megaron_arbetssatt "the client must never promise an action the server
// cannot perform").
type eligiblePassageShip struct {
	ID             uuid.UUID `json:"id"`
	Name           string    `json:"name"`
	Type           string    `json:"type"`
	SettlementID   uuid.UUID `json:"settlement_id"`
	SettlementName string    `json:"settlement_name"`
}

// eligiblePassageShips lists playerID's own galleys/merchantmen sitting idle
// in port that satisfy R1 for a runner currently waiting at passagePortID,
// owned by passagePortOwnerID: docked at exactly that port if the runner
// waits in one of the player's own settlements, or docked at ANY of the
// player's own ports if the runner waits in a foreign one (the pickup case).
func eligiblePassageShips(ctx context.Context, pool *pgxpool.Pool, worldID, playerID, passagePortID, passagePortOwnerID uuid.UUID) ([]eligiblePassageShip, error) {
	query := `SELECT u.id, u.name, u.type, u.settlement_id, s.name
	            FROM units u JOIN settlements s ON s.id = u.settlement_id
	           WHERE u.world_id = $1 AND u.owner_id = $2 AND u.status = 'garrison'
	             AND u.type IN ('galley', 'merchantman')`
	args := []any{worldID, playerID}
	if passagePortOwnerID == playerID {
		query += ` AND u.settlement_id = $3`
		args = append(args, passagePortID)
	}
	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []eligiblePassageShip
	for rows.Next() {
		var s eligiblePassageShip
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
		out = []eligiblePassageShip{}
	}
	return out, rows.Err()
}
