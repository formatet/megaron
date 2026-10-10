package combat

// Upptäckarexpeditionen (megaron_plan_upptackarexpeditionen.md, Timothy
// 2026-09-30 + 2026-10-06): an explore order covers an AREA — a chosen hex and
// the ground around it — for a chosen number of ticks. The unit walks leg by
// leg to the nearest hex in the area its Wanax has never seen, and turns for
// home no later than half its length, reserving the actual rounded route home
// before each leg. Once home it reports what it saw.
//
// The legs are ordinary marches (dispatchLeg) carrying march_intent
// "explore"; unit_expeditions holds the rest. A row only governs the unit
// while its leg_arrive_tick equals units.arrive_tick. New orders and recall
// explicitly delete the row; the tick match also rejects stale legacy state.

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"sort"

	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/hexgrid"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/tick"
	"formatet/megaron/server/internal/unit"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Tunables, not canon (the plan says so): how far around the chosen hex the
// area reaches, and the lengths a Wanax may give an expedition, in ticks.
const (
	ExpeditionAreaRadius   = 5
	ExpeditionMinTicks     = 4
	ExpeditionMaxTicks     = 30
	ExpeditionDefaultTicks = 10
	// expeditionPathTries bounds the A* searches per leg: candidates are
	// tried nearest first, and an unreachable one (an island, a valley behind
	// a river) must not cost a search for every hex of the area.
	expeditionPathTries = 8
)

// NormalizeExpeditionLength applies the server-owned default and bounds.
func NormalizeExpeditionLength(length int) (int, *OrderReject) {
	if length == 0 {
		length = ExpeditionDefaultTicks
	}
	if length < ExpeditionMinTicks || length > ExpeditionMaxTicks {
		return 0, reject(http.StatusBadRequest, "an expedition lasts %d to %s (asked for %d)", ExpeditionMinTicks, tick.FormatDays(ExpeditionMaxTicks), length)
	}
	return length, nil
}

// Turn reasons, as stored in unit_expeditions.turn_reason and the
// ExpeditionTurnedHome payload.
const (
	ExpeditionTurnHalfTime  = "half_time"
	ExpeditionTurnAreaKnown = "area_known"
	ExpeditionTurnNoPath    = "no_path"
	ExpeditionTurnRecalled  = "recalled"
)

// ExpeditionTurnTick is the last tick a leg may end on: half the length after
// departure, rounded down. Each leg separately reserves its actual return cost.
func ExpeditionTurnTick(startTick, lengthTicks int) int {
	return startTick + lengthTicks/2
}

// legTravelTicks is how many ticks a leg of raw path cost moveTicks takes this
// unit — the one rounding StartMarch and dispatchLeg both use.
func legTravelTicks(t unit.Type, crew int, laden bool, moveTicks float64) int {
	return max(1, int(math.Round(moveTicks*TravelFactor(t, crew, laden))))
}

// expeditionLeg is the outcome of choosing the next leg.
type expeditionLeg struct {
	Target province.MapPosition
	Path   []province.MapPosition
	Cost   float64 // raw path cost, before TravelFactor
	Found  bool
	// AnyUnknown is true when the area still held unseen ground the unit
	// could stand on, even if none of it could be reached.
	AnyUnknown bool
}

// nextExpeditionLeg picks the nearest hex in the area around centre that the
// player has never seen and the unit can stand on, by hex distance from `from`
// and then (q,r) — deterministic, and read only from the store
// (player_scouted_tiles), never from what the player last looked at.
func nextExpeditionLeg(
	ctx context.Context, db province.Queryer, g province.TileGraph,
	worldID, playerID uuid.UUID, centre, from province.MapPosition, category string,
) (expeditionLeg, error) {
	disk := hexgrid.Disk(hexgrid.Coord{Q: centre.Q, R: centre.R}, ExpeditionAreaRadius)
	qs, rs := hexgrid.QRArrays(disk)
	known := make(map[[2]int]bool)
	rows, err := db.Query(ctx,
		`SELECT t.q, t.r FROM player_scouted_tiles t
		   JOIN unnest($3::int[], $4::int[]) AS p(q, r) ON t.q = p.q AND t.r = p.r
		  WHERE t.world_id = $1 AND t.player_id = $2`,
		worldID, playerID, qs, rs,
	)
	if err != nil {
		return expeditionLeg{}, fmt.Errorf("load known tiles: %w", err)
	}
	for rows.Next() {
		var q, r int
		if err := rows.Scan(&q, &r); err != nil {
			rows.Close()
			return expeditionLeg{}, fmt.Errorf("scan known tile: %w", err)
		}
		known[[2]int{q, r}] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return expeditionLeg{}, fmt.Errorf("load known tiles: %w", err)
	}

	var cands []province.MapPosition
	for _, c := range disk {
		terrain, onMap := g[[2]int{c.Q, c.R}]
		if !onMap || known[[2]int{c.Q, c.R}] || !province.IsPassable(terrain, category) {
			continue
		}
		if c.Q == from.Q && c.R == from.R {
			continue
		}
		cands = append(cands, province.MapPosition{Q: c.Q, R: c.R})
	}
	sort.Slice(cands, func(i, j int) bool {
		di, dj := province.HexDistance(from, cands[i]), province.HexDistance(from, cands[j])
		if di != dj {
			return di < dj
		}
		if cands[i].Q != cands[j].Q {
			return cands[i].Q < cands[j].Q
		}
		return cands[i].R < cands[j].R
	})

	leg := expeditionLeg{AnyUnknown: len(cands) > 0}
	for i, c := range cands {
		if i >= expeditionPathTries {
			break
		}
		if path, cost, ok := g.FindPath(from, c, category); ok {
			leg.Target, leg.Path, leg.Cost, leg.Found = c, path, cost, true
			return leg, nil
		}
	}
	return leg, nil
}

// expeditionRow is one unit_expeditions row.
type expeditionRow struct {
	AreaQ, AreaR  int
	LengthTicks   int
	StartTick     int
	TurnTick      int
	LegArriveTick int
	Homeward      bool
	TurnReason    *string
	Furthest      int
}

// loadExpedition returns the unit's expedition if it still governs the unit
// (leg_arrive_tick == units.arrive_tick). A stale row — the unit was recalled,
// redirected or given a new order — is deleted and reported as absent, so the
// unit falls back to what its march intent alone says. Order writers also
// delete explicitly, since different courses can have the same arrival tick.
func loadExpedition(ctx context.Context, tx pgx.Tx, unitID uuid.UUID) (expeditionRow, bool, error) {
	var e expeditionRow
	var unitArriveTick *int
	err := tx.QueryRow(ctx,
		`SELECT e.area_q, e.area_r, e.length_ticks, e.start_tick, e.turn_tick,
		        e.leg_arrive_tick, e.homeward, e.turn_reason, e.furthest, u.arrive_tick
		   FROM unit_expeditions e JOIN units u ON u.id = e.unit_id
		  WHERE e.unit_id = $1 FOR UPDATE OF e`,
		unitID,
	).Scan(&e.AreaQ, &e.AreaR, &e.LengthTicks, &e.StartTick, &e.TurnTick,
		&e.LegArriveTick, &e.Homeward, &e.TurnReason, &e.Furthest, &unitArriveTick)
	if err == pgx.ErrNoRows {
		return expeditionRow{}, false, nil
	}
	if err != nil {
		return expeditionRow{}, false, fmt.Errorf("load expedition: %w", err)
	}
	if unitArriveTick == nil || *unitArriveTick != e.LegArriveTick {
		if _, err := tx.Exec(ctx, `DELETE FROM unit_expeditions WHERE unit_id = $1`, unitID); err != nil {
			return expeditionRow{}, false, fmt.Errorf("drop stale expedition: %w", err)
		}
		return expeditionRow{}, false, nil
	}
	return e, true, nil
}

// expeditionArrived ends one leg: record what the unit saw along it, then
// send it on to the next unseen hex — or home, if the next leg would end
// after the turn tick, the area holds nothing more to see, or nothing left in
// it can be reached.
func (h *UnitArrivalHandler) expeditionArrived(
	ctx context.Context, tx pgx.Tx,
	u unitRow, destQ, destR int, worldID uuid.UUID, e expeditionRow,
) error {
	g, err := province.LoadTileGraph(ctx, tx, worldID)
	if err != nil {
		return fmt.Errorf("expedition: load map: %w", err)
	}
	here := province.MapPosition{Q: destQ, R: destR}

	// What the eye saw on the way, hex by hex (invariant 2: the next target
	// must not depend on whether the player read the map mid-march).
	walked := []province.MapPosition{here}
	if path, _, ok := g.FindPath(province.MapPosition{Q: u.q, R: u.r}, here, u.category); ok {
		walked = path
	}
	eyeKind := province.EyeLandUnit
	if u.category == "naval" {
		eyeKind = province.EyeShip
	}
	seen, err := province.SweepLiveRadiusAlong(ctx, tx, worldID, u.ownerID, walked, eyeKind)
	if err != nil {
		return fmt.Errorf("expedition: sweep sight: %w", err)
	}
	if len(seen) > 0 {
		sq := make([]int, len(seen))
		sr := make([]int, len(seen))
		for i, p := range seen {
			sq[i], sr[i] = p.Q, p.R
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO unit_expedition_seen (unit_id, q, r)
			 SELECT $1, p.q, p.r FROM unnest($2::int[], $3::int[]) AS p(q, r)
			 ON CONFLICT DO NOTHING`,
			u.id, sq, sr,
		); err != nil {
			return fmt.Errorf("expedition: record sight: %w", err)
		}
	}

	home, err := expeditionReturnDestination(ctx, tx, u, destQ, destR, worldID)
	if err != nil {
		return fmt.Errorf("expedition: resolve home: %w", err)
	}
	if home == nil {
		return h.stopExpedition(ctx, tx, u, destQ, destR, worldID)
	}
	u.homeSettlementID = &home.id
	if _, err := tx.Exec(ctx, `UPDATE units SET home_settlement_id = $2 WHERE id = $1`, u.id, home.id); err != nil {
		return err
	}
	furthest := max(e.Furthest, province.HexDistance(province.MapPosition{Q: home.q, R: home.r}, here))

	var currentTick int
	if err := tx.QueryRow(ctx, `SELECT current_world_tick()`).Scan(&currentTick); err != nil {
		return fmt.Errorf("expedition: read tick: %w", err)
	}

	leg, err := nextExpeditionLeg(ctx, tx, g, worldID, u.ownerID,
		province.MapPosition{Q: e.AreaQ, R: e.AreaR}, here, u.category)
	if err != nil {
		return fmt.Errorf("expedition: choose next leg: %w", err)
	}

	reason := ""
	switch {
	case !leg.AnyUnknown:
		reason = ExpeditionTurnAreaKnown
	case !leg.Found:
		reason = ExpeditionTurnNoPath
	case currentTick+legTravelTicks(unit.Type(u.utype), u.crew, u.cargoUnitID != nil, leg.Cost) > e.TurnTick:
		reason = ExpeditionTurnHalfTime
	}

	if reason == "" {
		_, returnCost, reachable := g.FindPath(leg.Target, province.MapPosition{Q: home.q, R: home.r}, u.category)
		if !reachable {
			reason = ExpeditionTurnNoPath
		} else if currentTick+legTravelTicks(unit.Type(u.utype), u.crew, u.cargoUnitID != nil, leg.Cost)+legTravelTicks(unit.Type(u.utype), u.crew, u.cargoUnitID != nil, returnCost) > e.StartTick+e.LengthTicks {
			reason = ExpeditionTurnHalfTime
		}
	}

	if reason == "" {
		_, arriveTick, err := h.dispatchLeg(ctx, tx, u, destQ, destR, leg.Target.Q, leg.Target.R,
			leg.Path, leg.Cost, "explore", true, worldID)
		if err != nil {
			return fmt.Errorf("expedition: next leg: %w", err)
		}
		if _, err := tx.Exec(ctx,
			`UPDATE unit_expeditions SET leg_arrive_tick = $2, furthest = $3 WHERE unit_id = $1`,
			u.id, arriveTick, furthest,
		); err != nil {
			return fmt.Errorf("expedition: save leg: %w", err)
		}
		return nil
	}

	_, arriveTick, err := h.dispatchLeg(ctx, tx, u, destQ, destR, home.q, home.r, home.path, home.cost, "explore_return", false, worldID)
	if err != nil {
		return err
	}

	if _, err := tx.Exec(ctx,
		`UPDATE unit_expeditions
		    SET homeward = true, turn_reason = $2, leg_arrive_tick = $3, furthest = $4
		  WHERE unit_id = $1`,
		u.id, reason, arriveTick, furthest,
	); err != nil {
		return fmt.Errorf("expedition: save turn: %w", err)
	}

	event, err := h.eventStore.AppendTx(ctx, tx, u.id, events.StreamType(unit.StreamUnit), unit.EventExpeditionTurnedHome,
		unit.ExpeditionTurnedHomePayload{
			UnitID: u.id, Q: destQ, R: destR, AreaQ: e.AreaQ, AreaR: e.AreaR,
			Reason: reason, HomeSettlementID: *u.homeSettlementID, ArriveTick: arriveTick,
		}, worldID, nil)
	if err != nil {
		return err
	}
	if err := h.expeditionOutcome(ctx, tx, event, worldID, u.ownerID, "ExpeditionTurnedHome", 5, map[string]any{
		"unit_id":     u.id,
		"name":        unit.LoadDisplayName(ctx, tx, u.id),
		"q":           destQ,
		"r":           destR,
		"area_q":      e.AreaQ,
		"area_r":      e.AreaR,
		"reason":      reason,
		"arrive_tick": arriveTick,
	}); err != nil {
		return err
	}
	slog.Info("expedition turning for home", "unit", u.id, "reason", reason, "q", destQ, "r", destR, "arrive_tick", arriveTick)
	return nil
}

// expeditionHome files the homecoming report (the periplus) for a unit that
// has just re-garrisoned at settlementID, and ends the expedition.
func (h *UnitArrivalHandler) expeditionHome(
	ctx context.Context, tx pgx.Tx,
	u unitRow, settlementID uuid.UUID, worldID uuid.UUID, e expeditionRow,
) error {
	var currentTick, hexesSeen int
	if err := tx.QueryRow(ctx, `SELECT current_world_tick()`).Scan(&currentTick); err != nil {
		return fmt.Errorf("expedition report: read tick: %w", err)
	}
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM unit_expedition_seen WHERE unit_id = $1`, u.id,
	).Scan(&hexesSeen); err != nil {
		return fmt.Errorf("expedition report: count sight: %w", err)
	}

	finds := []unit.ExpeditionFind{}
	rows, err := tx.Query(ctx,
		`SELECT k.kind, t.q, t.r
		   FROM unit_expedition_seen s
		   JOIN map_tiles t ON t.world_id = $2 AND t.q = s.q AND t.r = s.r
		   CROSS JOIN LATERAL (VALUES
		       ('copper', t.copper_deposit),
		       ('tin',    t.tin_deposit),
		       ('silver', COALESCE(t.silver_deposit, false)),
		       ('cedar',  COALESCE(t.cedar_deposit, false))
		   ) AS k(kind, present)
		  WHERE s.unit_id = $1 AND k.present
		  ORDER BY k.kind, t.q, t.r`,
		u.id, worldID,
	)
	if err != nil {
		return fmt.Errorf("expedition report: deposits: %w", err)
	}
	for rows.Next() {
		var f unit.ExpeditionFind
		if err := rows.Scan(&f.Kind, &f.Q, &f.R); err != nil {
			rows.Close()
			return err
		}
		finds = append(finds, f)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	rows, err = tx.Query(ctx,
		`SELECT p.map_q, p.map_r, st.name, COALESCE(pl.wanax_name, pl.username, '')
		   FROM unit_expedition_seen s
		   JOIN provinces p ON p.world_id = $2 AND p.map_q = s.q AND p.map_r = s.r
		   JOIN settlements st ON st.province_id = p.id AND st.state = 'active'
		   LEFT JOIN players pl ON pl.id = st.owner_id
		  WHERE s.unit_id = $1 AND st.owner_id IS DISTINCT FROM $3
		  ORDER BY p.map_q, p.map_r`,
		u.id, worldID, u.ownerID,
	)
	if err != nil {
		return fmt.Errorf("expedition report: cities: %w", err)
	}
	for rows.Next() {
		f := unit.ExpeditionFind{Kind: "city"}
		if err := rows.Scan(&f.Q, &f.R, &f.Name, &f.Owner); err != nil {
			rows.Close()
			return err
		}
		finds = append(finds, f)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	turnReason := ""
	if e.TurnReason != nil {
		turnReason = *e.TurnReason
	}
	payload := unit.ExpeditionReportPayload{
		UnitID: u.id, SettlementID: settlementID, AreaQ: e.AreaQ, AreaR: e.AreaR,
		TicksOut: currentTick - e.StartTick, Furthest: e.Furthest, HexesSeen: hexesSeen,
		TurnReason: turnReason, Finds: finds,
	}
	event, err := h.eventStore.AppendTx(ctx, tx, u.id, events.StreamType(unit.StreamUnit), unit.EventExpeditionReport,
		payload, worldID, nil)
	if err != nil {
		return err
	}
	if err := h.expeditionOutcome(ctx, tx, event, worldID, u.ownerID, "ExpeditionReport", 4, map[string]any{
		"unit_id":     u.id,
		"name":        unit.LoadDisplayName(ctx, tx, u.id),
		"area_q":      e.AreaQ,
		"area_r":      e.AreaR,
		"ticks_out":   payload.TicksOut,
		"furthest":    payload.Furthest,
		"hexes_seen":  hexesSeen,
		"turn_reason": turnReason,
		"finds":       finds,
	}); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM unit_expeditions WHERE unit_id = $1`, u.id); err != nil {
		return fmt.Errorf("expedition report: close: %w", err)
	}
	slog.Info("expedition home", "unit", u.id, "ticks_out", payload.TicksOut, "hexes_seen", hexesSeen, "finds", len(finds))
	return nil
}
