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

// resolveOutboundDisembark is R2's outbound half: disembark at the runner's
// true destination hex directly if a settlement there is coastal or
// harboured (the common case — most cities are); otherwise the coastal/
// harboured settlement — reachable by a naval route from the ship's own
// port — with the shortest CategoryCourierLand time onward to the target.
//
// Candidates are ACTIVE SETTLEMENTS, not raw map tiles: a courier route is
// only ever asked to reach a named place in practice, and a bare unsettled
// coastal tile is nowhere a runner has reason to walk from. Searching every
// tile on a large world would be materially more expensive for no case this
// slice's acceptance criteria exercise.
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
	shipSeaQ, shipSeaR, foundSea, err := province.NearestSeaNeighbor(ctx, pool, worldID, shipPortQ, shipPortR)
	if err != nil {
		return 0, 0, err
	}
	if !foundSea {
		return 0, 0, fmt.Errorf("the ship's own port has no sea approach")
	}

	graph, err := province.LoadTileGraph(ctx, pool, worldID)
	if err != nil {
		return 0, 0, err
	}

	rows, err := pool.Query(ctx,
		`SELECT p.map_q, p.map_r FROM settlements s JOIN provinces p ON p.id = s.province_id
		 WHERE s.world_id = $1 AND s.state = 'active'
		   AND (COALESCE(p.coastal, false)
		        OR EXISTS(SELECT 1 FROM buildings b WHERE b.settlement_id = s.id AND b.building_type = 'harbour'))`,
		worldID,
	)
	if err != nil {
		return 0, 0, err
	}
	type candidate struct{ q, r int }
	var candidates []candidate
	for rows.Next() {
		var c candidate
		if err := rows.Scan(&c.q, &c.r); err != nil {
			rows.Close()
			return 0, 0, err
		}
		candidates = append(candidates, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, 0, err
	}

	bestQ, bestR := 0, 0
	bestLandCost := -1.0
	for _, c := range candidates {
		_, landCost, landOK := graph.FindPath(
			province.MapPosition{Q: c.q, R: c.r}, province.MapPosition{Q: targetQ, R: targetR}, province.CategoryCourierLand)
		if !landOK {
			continue
		}
		candSeaQ, candSeaR, foundCandSea, seaErr := province.NearestSeaNeighbor(ctx, pool, worldID, c.q, c.r)
		if seaErr != nil || !foundCandSea {
			continue
		}
		_, _, navalOK := graph.FindPath(
			province.MapPosition{Q: shipSeaQ, R: shipSeaR}, province.MapPosition{Q: candSeaQ, R: candSeaR}, "naval")
		if !navalOK {
			continue
		}
		if bestLandCost < 0 || landCost < bestLandCost {
			bestQ, bestR, bestLandCost = c.q, c.r, landCost
		}
	}
	if bestLandCost < 0 {
		return 0, 0, fmt.Errorf(
			"no coastal city near the runner's destination that this ship can reach by sea — arrange passage is not possible here")
	}
	return bestQ, bestR, nil
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
