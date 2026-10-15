package combat

// Vägar V2 (megaron_plan_vagar.md, Timothy 2026-10-15 B1–B3). A road edge is
// built by an army from stone its city paid when the order was given:
//
//	order given (one tx) ─ stone out of the city ─ road_sites row ─ stone
//	caravan city→"from" hex (transports.kind road_stone, interceptable) ─
//	and a Runner with the build_road order to the army (messenger verb)
//
// The build starts when BOTH the caravan and the army stand at the "from"
// hex; it ends RoadEdgeBuilt ticks later. Stone is never refunded (B3). A road
// site that never completes stays visible as state "waiting" (silence would be
// the failure mode). Upward communication from transport goes via the
// ScheduledRoadSite event (G1).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strings"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/transport"
	"formatet/megaron/server/internal/unit"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// EventRoadEdgeBuilt is the audit event + player notification when an edge is finished.
const EventRoadEdgeBuilt = "RoadEdgeBuilt"

// StreamRoad is the event stream of road sites.
const StreamRoad events.StreamType = "road"

// BuildRoadOrder is the build_road verb's payload (OrderDeliveryPayload.BuildRoad).
// SiteID is the road_sites row created when the order was given.
type BuildRoadOrder struct {
	WorldID  uuid.UUID `json:"world_id"`
	PlayerID uuid.UUID `json:"player_id"`
	UnitID   uuid.UUID `json:"unit_id"`
	SiteID   uuid.UUID `json:"site_id"`
	FromQ    int       `json:"from_q"`
	FromR    int       `json:"from_r"`
	ToQ      int       `json:"to_q"`
	ToR      int       `json:"to_r"`
}

// roadSitePayload is the ScheduledRoadSite payload. phase: "check" | "build".
type roadSitePayload struct {
	SiteID uuid.UUID `json:"site_id"`
	Phase  string    `json:"phase"`
}

func terrainWords(t string) string { return strings.ReplaceAll(t, "_", " ") }

// roadTerrainRefusal explains, in player language, why a hex cannot carry a road.
func roadTerrainRefusal(t string) string {
	switch t {
	case "river_ford":
		return "a road ends at a river ford — there are no bridges, you wade the ford as before"
	case "river", "coastal_sea", "deep_sea":
		return "a road cannot be built on water"
	case "mountain_limestone", "mountain_red":
		return "a road cannot be built through mountains"
	}
	return "a road cannot be built on " + terrainWords(t)
}

// ValidateRoadEdge checks the map side of a road order: both hexes exist and
// carry roads, they are neighbours, and the edge is not already built. It
// returns the terrain of the target hex (what the edge costs and takes).
func ValidateRoadEdge(ctx context.Context, db province.Queryer, worldID uuid.UUID, from, to province.MapPosition) (toTerrain string, rej *OrderReject) {
	if province.HexDistance(from, to) != 1 {
		return "", reject(http.StatusUnprocessableEntity,
			"a road edge joins two neighbouring hexes — (%d,%d) and (%d,%d) are not neighbours", from.Q, from.R, to.Q, to.R)
	}
	terrain := map[province.MapPosition]string{}
	for _, p := range []province.MapPosition{from, to} {
		var t string
		rows, err := db.Query(ctx, `SELECT terrain FROM map_tiles WHERE world_id = $1 AND q = $2 AND r = $3`, worldID, p.Q, p.R)
		if err != nil {
			return "", reject(http.StatusInternalServerError, "could not read the map")
		}
		found := rows.Next()
		if found {
			if err := rows.Scan(&t); err != nil {
				rows.Close()
				return "", reject(http.StatusInternalServerError, "could not read the map")
			}
		}
		rows.Close()
		if !found {
			return "", reject(http.StatusNotFound, "hex (%d,%d) is not on the map", p.Q, p.R)
		}
		terrain[p] = t
	}
	for _, p := range []province.MapPosition{from, to} {
		if !province.IsRoadableTerrain(terrain[p]) {
			return "", reject(http.StatusUnprocessableEntity, "no road at (%d,%d): %s", p.Q, p.R, roadTerrainRefusal(terrain[p]))
		}
	}
	roads, err := province.LoadRoadSet(ctx, db, worldID)
	if err != nil {
		return "", reject(http.StatusInternalServerError, "could not read the roads")
	}
	if roads.Has(from, to) {
		return "", reject(http.StatusConflict, "a road already joins (%d,%d) and (%d,%d)", from.Q, from.R, to.Q, to.R)
	}
	return terrain[to], nil
}

// unitHexState reads where a unit stands: positioned at its q/r, or garrisoned
// in the city at that hex. ok=false when it stands nowhere (marching, embarked…).
func unitHexState(ctx context.Context, db province.Queryer, unitID uuid.UUID) (status string, pos province.MapPosition, ok bool, err error) {
	rows, err := db.Query(ctx,
		`SELECT u.status, u.q, u.r, p.map_q, p.map_r
		   FROM units u
		   LEFT JOIN settlements s ON s.id = u.settlement_id
		   LEFT JOIN provinces p ON p.id = s.province_id
		  WHERE u.id = $1`, unitID)
	if err != nil {
		return "", pos, false, err
	}
	defer rows.Close()
	if !rows.Next() {
		return "", pos, false, rows.Err()
	}
	var uq, ur, pq, pr *int
	if err := rows.Scan(&status, &uq, &ur, &pq, &pr); err != nil {
		return "", pos, false, err
	}
	switch status {
	case "positioned":
		if uq != nil && ur != nil {
			return status, province.MapPosition{Q: *uq, R: *ur}, true, nil
		}
	case "garrison":
		if pq != nil && pr != nil {
			return status, province.MapPosition{Q: *pq, R: *pr}, true, nil
		}
	}
	return status, pos, false, nil
}

// worldTick reads the world's own tick by id — current_world_tick() is NULL
// unless the world is the one active world, which an order must not depend on.
func worldTick(ctx context.Context, db interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}, worldID uuid.UUID) (int, error) {
	var t int
	err := db.QueryRow(ctx, `SELECT current_tick FROM worlds WHERE id = $1`, worldID).Scan(&t)
	return t, err
}

func floorTenth(x float64) float64 { return math.Floor(x*10+1e-9) / 10 }

// PlaceRoadSite is the order's synchronous half, inside the caller's tx: it
// validates everything, takes the stone out of the home city, writes the
// road_sites row and sends the stone caravan. Any *OrderReject (or error) means
// the caller rolls the tx back — nothing happened, no stone left the city.
// A previous active site of the same army is cancelled (stone not refunded, B3).
func PlaceRoadSite(ctx context.Context, tx pgx.Tx, sched *events.Scheduler, clk clock.Clock, o BuildRoadOrder, homeSettlementID uuid.UUID) (siteID uuid.UUID, stone float64, err error) {
	var owner, world uuid.UUID
	var utype, category, status string
	var stance *string
	if scanErr := tx.QueryRow(ctx,
		`SELECT owner_id, world_id, type, category, status, stance FROM units WHERE id = $1 FOR UPDATE`, o.UnitID,
	).Scan(&owner, &world, &utype, &category, &status, &stance); scanErr != nil {
		return uuid.Nil, 0, reject(http.StatusNotFound, "unit not found")
	}
	if owner != o.PlayerID || world != o.WorldID {
		return uuid.Nil, 0, reject(http.StatusNotFound, "unit not found")
	}
	if category != string(unit.CategoryLand) || unit.Type(utype) == unit.TypeNomadicHost {
		return uuid.Nil, 0, reject(http.StatusUnprocessableEntity, "only a land army can build a road")
	}
	if status != string(unit.StatusGarrison) && status != string(unit.StatusPositioned) {
		return uuid.Nil, 0, reject(http.StatusUnprocessableEntity,
			"this army is %s and cannot build a road now — it must stand in a city or in the field (a marching army can be recalled first)", status)
	}

	from := province.MapPosition{Q: o.FromQ, R: o.FromR}
	to := province.MapPosition{Q: o.ToQ, R: o.ToR}
	if stance != nil && *stance == string(unit.StanceFortify) {
		// A fortified army cannot march (StartMarch refuses); it can only build where it stands.
		if _, pos, standing, posErr := unitHexState(ctx, tx, o.UnitID); posErr != nil {
			return uuid.Nil, 0, posErr
		} else if !standing || pos != from {
			return uuid.Nil, 0, reject(http.StatusUnprocessableEntity,
				"the army is in fortify stance and cannot march to (%d,%d); change its stance first, or build the road from where it stands", from.Q, from.R)
		}
	}
	toTerrain, rej := ValidateRoadEdge(ctx, tx, o.WorldID, from, to)
	if rej != nil {
		return uuid.Nil, 0, rej
	}

	var homeName string
	var homeQ, homeR int
	var homeOwner uuid.UUID
	var homeState string
	if scanErr := tx.QueryRow(ctx,
		`SELECT s.name, s.owner_id, s.state, p.map_q, p.map_r
		   FROM settlements s JOIN provinces p ON p.id = s.province_id
		  WHERE s.id = $1 AND s.world_id = $2`, homeSettlementID, o.WorldID,
	).Scan(&homeName, &homeOwner, &homeState, &homeQ, &homeR); scanErr != nil || homeOwner != o.PlayerID || homeState != "active" {
		return uuid.Nil, 0, reject(http.StatusUnprocessableEntity, "the road needs one of your cities to supply the stone")
	}

	stone = province.RoadStoneCost(toTerrain)
	var have float64
	if scanErr := tx.QueryRow(ctx,
		`SELECT COALESCE((SELECT GREATEST(0, settled(amount, rate, calc_tick))
		                    FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'stone' FOR UPDATE), 0)`,
		homeSettlementID,
	).Scan(&have); scanErr != nil {
		return uuid.Nil, 0, fmt.Errorf("read city stone: %w", scanErr)
	}
	if have+1e-9 < stone {
		return uuid.Nil, 0, reject(http.StatusUnprocessableEntity,
			"%s has %g of %g stone — one road edge into %s takes %g stone",
			homeName, floorTenth(have), stone, terrainWords(toTerrain), stone)
	}
	if _, execErr := tx.Exec(ctx,
		`UPDATE settlement_goods
		    SET amount = GREATEST(0, settled(amount, rate, calc_tick) - $2), calc_tick = current_world_tick()
		  WHERE settlement_id = $1 AND good_key = 'stone'`, homeSettlementID, stone); execErr != nil {
		return uuid.Nil, 0, fmt.Errorf("take stone from city: %w", execErr)
	}

	// A new road order replaces this army's earlier one (stone is not refunded).
	if _, execErr := tx.Exec(ctx,
		`UPDATE road_sites SET state = 'cancelled' WHERE unit_id = $1 AND state IN ('waiting', 'building')`, o.UnitID); execErr != nil {
		return uuid.Nil, 0, fmt.Errorf("replace earlier road site: %w", execErr)
	}

	if scanErr := tx.QueryRow(ctx,
		`INSERT INTO road_sites (world_id, owner_id, unit_id, from_q, from_r, to_q, to_r, stone, home_settlement_id, created_tick)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9, (SELECT current_tick FROM worlds WHERE id = $1)) RETURNING id`,
		o.WorldID, o.PlayerID, o.UnitID, o.FromQ, o.FromR, o.ToQ, o.ToR, stone, homeSettlementID,
	).Scan(&siteID); scanErr != nil {
		return uuid.Nil, 0, fmt.Errorf("insert road site: %w", scanErr)
	}

	// The stone goes as a physical caravan to the "from" hex (a hex, not a city).
	transportID, dispErr := transport.Dispatch(ctx, tx, sched, transport.DispatchParams{
		WorldID: o.WorldID, OwnerID: o.PlayerID, Kind: transport.KindRoadStone,
		OriginID: homeSettlementID, Category: "land",
		OriginQ: homeQ, OriginR: homeR, DestQ: o.FromQ, DestR: o.FromR,
		DepartsAt: clk.Now(), Manifest: transport.Manifest{"stone": stone}, Interceptable: true,
	})
	if dispErr != nil {
		if errors.Is(dispErr, province.ErrNoTradePath) {
			return uuid.Nil, 0, reject(http.StatusUnprocessableEntity,
				"no land route for the stone caravan from %s to (%d,%d)", homeName, o.FromQ, o.FromR)
		}
		return uuid.Nil, 0, fmt.Errorf("dispatch stone caravan: %w", dispErr)
	}
	if _, execErr := tx.Exec(ctx, `UPDATE road_sites SET stone_transport_id = $2 WHERE id = $1`, siteID, transportID); execErr != nil {
		return uuid.Nil, 0, fmt.Errorf("link stone caravan: %w", execErr)
	}
	return siteID, stone, nil
}

func enqueueRoadSite(ctx context.Context, tx pgx.Tx, sched *events.Scheduler, worldID, siteID uuid.UUID, phase string, dueTick int) error {
	return sched.EnqueueTickTx(ctx, tx, worldID, events.ScheduledRoadSite, roadSitePayload{SiteID: siteID, Phase: phase}, dueTick)
}

// ExecuteBuildRoad is the Runner's delivery: the army learns the order. Already
// standing at the "from" hex it is flagged there; otherwise it marches there by
// the ordinary march (StartMarch — no second marching code). Rejections surface
// as OrderFailed to the owner (the caller); the site then stays "waiting" and visible.
func ExecuteBuildRoad(ctx context.Context, pool *pgxpool.Pool, sched *events.Scheduler, store *events.Store, clk clock.Clock, o BuildRoadOrder) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)

	var state string
	var siteUnit *uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT state, unit_id FROM road_sites WHERE id = $1 FOR UPDATE`, o.SiteID).Scan(&state, &siteUnit); err != nil {
		return reject(http.StatusUnprocessableEntity, "the road order no longer exists")
	}
	if state != "waiting" || siteUnit == nil || *siteUnit != o.UnitID {
		return reject(http.StatusUnprocessableEntity, "a newer order replaced the road order before the Runner arrived")
	}
	var owner uuid.UUID
	var category string
	if err := tx.QueryRow(ctx, `SELECT owner_id, category FROM units WHERE id = $1 FOR UPDATE`, o.UnitID).Scan(&owner, &category); err != nil || owner != o.PlayerID {
		return reject(http.StatusUnprocessableEntity, "the army is gone")
	}
	status, pos, standing, err := unitHexState(ctx, tx, o.UnitID)
	if err != nil {
		return err
	}
	if !standing && status != string(unit.StatusMarching) {
		return reject(http.StatusUnprocessableEntity, "the army is %s and cannot take the road order", status)
	}
	if standing && pos.Q == o.FromQ && pos.R == o.FromR {
		if _, err := tx.Exec(ctx, `UPDATE road_sites SET army_arrived = true WHERE id = $1`, o.SiteID); err != nil {
			return err
		}
		tick, err := worldTick(ctx, tx, o.WorldID)
		if err != nil {
			return err
		}
		if err := enqueueRoadSite(ctx, tx, sched, o.WorldID, o.SiteID, "check", tick); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	// Not there yet: march. KeepRoadSite stops StartMarch from cancelling this very site.
	_, err = StartMarch(ctx, pool, sched, store, clk, MarchOrder{
		WorldID: o.WorldID, PlayerID: o.PlayerID, UnitID: o.UnitID,
		TargetQ: o.FromQ, TargetR: o.FromR, KeepRoadSite: &o.SiteID,
	}, nil)
	return err
}

// RoadSitesCancelFor ends an army's active road sites because it was given a
// new order (B3: the stone is not refunded). keep spares one site (the order
// that is itself walking the army to its road).
func RoadSitesCancelFor(ctx context.Context, tx pgx.Tx, unitID uuid.UUID, keep *uuid.UUID) error {
	_, err := tx.Exec(ctx,
		`UPDATE road_sites SET state = 'cancelled'
		  WHERE unit_id = $1 AND state IN ('waiting', 'building') AND ($2::uuid IS NULL OR id <> $2)`,
		unitID, keep)
	return err
}

// markRoadArmyArrived is called when an army comes to rest at (q,r): any road
// site waiting for it there learns the army is present.
func markRoadArmyArrived(ctx context.Context, tx pgx.Tx, sched *events.Scheduler, worldID, unitID uuid.UUID, q, r int) error {
	rows, err := tx.Query(ctx,
		`UPDATE road_sites SET army_arrived = true
		  WHERE unit_id = $1 AND state = 'waiting' AND army_arrived = false AND from_q = $2 AND from_r = $3
		  RETURNING id`, unitID, q, r)
	if err != nil {
		return err
	}
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil // the common case: no road waits for this army here
	}
	tick, err := worldTick(ctx, tx, worldID)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := enqueueRoadSite(ctx, tx, sched, worldID, id, "check", tick); err != nil {
			return err
		}
	}
	return nil
}

// RoadSiteHandler drives ScheduledRoadSite: "check" starts the build when stone
// and army are both there; "build" finishes the edge. Both phases are idempotent:
// the site row's state moves one way under FOR UPDATE.
type RoadSiteHandler struct {
	pool  *pgxpool.Pool
	sched *events.Scheduler
	store *events.Store
	hub   Broadcaster
}

func NewRoadSiteHandler(pool *pgxpool.Pool, sched *events.Scheduler, store *events.Store, hub Broadcaster) *RoadSiteHandler {
	return &RoadSiteHandler{pool: pool, sched: sched, store: store, hub: hub}
}

type roadSiteRow struct {
	state                     string
	stoneArrived, armyArrived bool
	unitID                    *uuid.UUID
	owner                     uuid.UUID
	from, to                  province.MapPosition
}

func (h *RoadSiteHandler) lock(ctx context.Context, tx pgx.Tx, id uuid.UUID) (roadSiteRow, bool, error) {
	var s roadSiteRow
	err := tx.QueryRow(ctx,
		`SELECT state, stone_arrived, army_arrived, unit_id, owner_id, from_q, from_r, to_q, to_r
		   FROM road_sites WHERE id = $1 FOR UPDATE`, id,
	).Scan(&s.state, &s.stoneArrived, &s.armyArrived, &s.unitID, &s.owner, &s.from.Q, &s.from.R, &s.to.Q, &s.to.R)
	if errors.Is(err, pgx.ErrNoRows) {
		return s, false, nil
	}
	return s, err == nil, err
}

// armyReady: the army still exists, is the owner's, stands at the from hex and
// is in no battle.
func (h *RoadSiteHandler) armyReady(ctx context.Context, tx pgx.Tx, s roadSiteRow) (size int, ok bool, why string, err error) {
	if s.unitID == nil {
		return 0, false, "the army no longer exists", nil
	}
	var owner uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT owner_id, size FROM units WHERE id = $1`, *s.unitID).Scan(&owner, &size); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, false, "the army no longer exists", nil
		}
		return 0, false, "", err
	}
	if owner != s.owner {
		return 0, false, "the army changed hands", nil
	}
	_, pos, standing, err := unitHexState(ctx, tx, *s.unitID)
	if err != nil {
		return 0, false, "", err
	}
	if !standing || pos != s.from {
		return 0, false, fmt.Sprintf("the army is no longer at (%d,%d)", s.from.Q, s.from.R), nil
	}
	var inBattle bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM battle_participants bp JOIN battles b ON b.id = bp.battle_id
		                WHERE bp.unit_id = $1 AND b.status = 'active' AND bp.left_tick IS NULL)`, *s.unitID,
	).Scan(&inBattle); err != nil {
		return 0, false, "", err
	}
	if inBattle {
		return 0, false, "the army was drawn into a battle", nil
	}
	return size, true, "", nil
}

func (h *RoadSiteHandler) Handle(ctx context.Context, e events.ScheduledEvent) error {
	var p roadSitePayload
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return fmt.Errorf("unmarshal road site event: %w", err)
	}
	switch p.Phase {
	case "check":
		return h.check(ctx, e, p.SiteID)
	case "build":
		return h.build(ctx, e, p.SiteID)
	}
	return fmt.Errorf("road site event: unknown phase %q", p.Phase)
}

func (h *RoadSiteHandler) check(ctx context.Context, e events.ScheduledEvent, siteID uuid.UUID) error {
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	s, found, err := h.lock(ctx, tx, siteID)
	if err != nil {
		return err
	}
	if !found || s.state != "waiting" || !s.stoneArrived || !s.armyArrived {
		return nil // not ready (the other half will check again), or already moved on
	}
	size, ok, why, err := h.armyReady(ctx, tx, s)
	if err != nil {
		return err
	}
	if !ok {
		slog.Info("road site: army not ready, site stays waiting", "site", siteID, "why", why)
		return nil
	}
	var toTerrain string
	if err := tx.QueryRow(ctx, `SELECT terrain FROM map_tiles WHERE world_id = $1 AND q = $2 AND r = $3`, e.WorldID, s.to.Q, s.to.R).Scan(&toTerrain); err != nil {
		return fmt.Errorf("road site: target terrain: %w", err)
	}
	tick, err := worldTick(ctx, tx, e.WorldID)
	if err != nil {
		return err
	}
	due := tick + province.RoadBuildTicks(toTerrain, size)
	if _, err := tx.Exec(ctx, `UPDATE road_sites SET state = 'building', build_due_tick = $2 WHERE id = $1`, siteID, due); err != nil {
		return err
	}
	if err := enqueueRoadSite(ctx, tx, h.sched, e.WorldID, siteID, "build", due); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (h *RoadSiteHandler) build(ctx context.Context, e events.ScheduledEvent, siteID uuid.UUID) error {
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	s, found, err := h.lock(ctx, tx, siteID)
	if err != nil {
		return err
	}
	if !found || s.state != "building" {
		return nil // done, cancelled by a newer order, or replayed
	}
	_, ok, why, err := h.armyReady(ctx, tx, s)
	if err != nil {
		return err
	}
	if !ok {
		if _, err := tx.Exec(ctx, `UPDATE road_sites SET state = 'cancelled' WHERE id = $1`, siteID); err != nil {
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		if h.hub != nil {
			_ = h.hub.NotifyPlayer(ctx, e.WorldID, s.owner, "OrderFailed", 2, map[string]any{
				"verb": "build_road", "unit_id": s.unitID,
				"reason": fmt.Sprintf("the road to (%d,%d) was never finished: %s — the stone is lost", s.to.Q, s.to.R, why),
			})
		}
		return nil
	}
	tick, err := worldTick(ctx, tx, e.WorldID)
	if err != nil {
		return err
	}
	k := province.RoadKey(s.from, s.to)
	if _, err := tx.Exec(ctx,
		`INSERT INTO road_edges (world_id, q1, r1, q2, r2, built_by_wanax, built_tick)
		 VALUES ($1,$2,$3,$4,$5,$6,$7) ON CONFLICT DO NOTHING`,
		e.WorldID, k[0], k[1], k[2], k[3], s.owner, tick); err != nil {
		return fmt.Errorf("insert road edge: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE road_sites SET state = 'done' WHERE id = $1`, siteID); err != nil {
		return err
	}
	ev, err := h.store.AppendTx(ctx, tx, siteID, StreamRoad, EventRoadEdgeBuilt, map[string]any{
		"from_q": s.from.Q, "from_r": s.from.R, "to_q": s.to.Q, "to_r": s.to.R, "built_tick": tick, "built_by": s.owner,
	}, e.WorldID, nil)
	if err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	h.store.RecordCommitted(ctx, ev)
	if h.hub != nil {
		_ = h.hub.NotifyPlayer(ctx, e.WorldID, s.owner, EventRoadEdgeBuilt, 4, map[string]any{
			"unit_id": s.unitID, "q": s.from.Q, "r": s.from.R, "target_q": s.to.Q, "target_r": s.to.R,
		})
	}
	return nil
}
