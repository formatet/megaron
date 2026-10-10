package combat

// Slice T (megaron_transportrisk.md, Timothy 2026-10-08), reworked by megaron_plan_stormar.md
// (Timothy 2026-10-10): the risk of the sea lives in the sea, as storms you can see. A
// storm is three connected sea hexes that crawl (sea_storm_weather.go). A ship that
// stands on any hex of a storm at a tick takes one hull point; at hull 0 it founders and
// takes its cargo and embarked troops with it. Ownership and errand never change the risk.
//
// One recurring scan per world (ScheduledSeaStormScan, first in the day —
// events.tickPrioritySea) first walks the storms to the scan's tick, then reads each
// voyage's SAVED route (units.march_route / transports.journey — never a re-search) and
// checks the hex the ship holds at every tick since its last check against the stored
// track. sea_storm_progress.last_tick (mig 168) is the per-voyage high-water mark, written
// in the same TX as the hull outcome: a retried or duplicated scan never hits a tick
// twice, and a scan that ran late checks every tick it missed. Outcomes are stored as
// outcomes (CLAUDE.md Events): ShipStormDamaged / ShipFoundered.
//
// G1: lives in combat because combat owns the unit route parser (route.go)
// and may read transports; transport may not import combat.
//
// Not here (slice T2): a messenger aboard a foundered ship. Until then the
// passage scan's existing lost-carrier path applies to it.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"formatet/megaron/server/internal/carrier"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/unit"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SeaStormScanIntervalTicks: every day, so no entered hex waits a day for its roll.
const SeaStormScanIntervalTicks = 1

const (
	EventShipStormDamaged = "ShipStormDamaged"
	EventShipFoundered    = "ShipFoundered"
)

// SeaStormCargo is one good lost with a foundered transport ship.
type SeaStormCargo struct {
	GoodKey  string  `json:"good_key"`
	Quantity float64 `json:"quantity"`
}

// SeaStormTroops is the embarked unit that drowned with a foundered ship.
type SeaStormTroops struct {
	UnitID   uuid.UUID `json:"unit_id"`
	UnitType string    `json:"unit_type"`
	Size     int       `json:"size"`
}

// SeaStormPayload is the stored outcome of one storm (event and notice body).
// OwnerID is the Wanax whose ship it was (principle 10: a Wanax's fortunes).
type SeaStormPayload struct {
	WorldID     uuid.UUID       `json:"world_id"`
	ShipID      uuid.UUID       `json:"ship_id"`
	OwnerID     uuid.UUID       `json:"owner_id"`
	ShipType    string          `json:"ship_type"`
	Name        string          `json:"name"`
	Q           int             `json:"q"`
	R           int             `json:"r"`
	Tick        int             `json:"tick"`
	HullBefore  int             `json:"hull_before"`
	Hull        int             `json:"hull"`
	HullMax     int             `json:"hull_max"`
	Foundered   bool            `json:"foundered"`
	Errand      string          `json:"errand"` // "march" or the transport kind
	TransportID *uuid.UUID      `json:"transport_id,omitempty"`
	Cargo       []SeaStormCargo `json:"cargo,omitempty"`
	Troops      *SeaStormTroops `json:"troops,omitempty"`
}

// SeaStormScanHandler processes the recurring ScheduledSeaStormScan.
type SeaStormScanHandler struct {
	pool       *pgxpool.Pool
	scheduler  *events.Scheduler
	eventStore *events.Store
	hub        Broadcaster
	// Dice is the weather seam (creating storms, their steps); defaults to economy.NewWallDice().
	Dice economy.Dice
}

func NewSeaStormScanHandler(pool *pgxpool.Pool, sched *events.Scheduler, store *events.Store, hub Broadcaster) *SeaStormScanHandler {
	return &SeaStormScanHandler{pool: pool, scheduler: sched, eventStore: store, hub: hub, Dice: economy.NewWallDice()}
}

// seaVoyage is one ship's current passage along a saved route.
type seaVoyage struct {
	key         string
	shipID      uuid.UUID
	owner       uuid.UUID
	shipType    string
	errand      string
	transportID *uuid.UUID
	cargoUnitID *uuid.UUID
	route       StoredRoute
	departTick  int
	arriveTick  int
}

func isSeaTerrain(t string) bool { return t == "coastal_sea" || t == "deep_sea" }

func (h *SeaStormScanHandler) Handle(ctx context.Context, e events.ScheduledEvent) error {
	graph, err := province.LoadTileGraph(ctx, h.pool, e.WorldID)
	if err != nil {
		return fmt.Errorf("sea storm scan: load terrain: %w", err)
	}
	if err := h.stepWeather(ctx, e.WorldID, e.DueTick, graph); err != nil {
		return fmt.Errorf("sea storm scan: weather: %w", err)
	}
	occupied, err := loadStormOccupancy(ctx, h.pool, e.WorldID, e.DueTick-stormTrackKeepTicks, e.DueTick)
	if err != nil {
		return err
	}
	voyages, err := h.loadVoyages(ctx, e.WorldID)
	if err != nil {
		return err
	}
	for _, v := range voyages {
		if err := h.sail(ctx, e.WorldID, e.DueTick, graph, occupied, v); err != nil {
			// One ship's failure must not stop the sea for every other ship;
			// its progress row did not move, so the next scan retries it.
			slog.Warn("sea storm scan: voyage failed", "voyage", v.key, "err", err)
		}
	}
	return h.scheduler.EnqueueTickRecurring(ctx, e.WorldID, events.ScheduledSeaStormScan,
		struct{}{}, e.DueTick, SeaStormScanIntervalTicks)
}

func (h *SeaStormScanHandler) loadVoyages(ctx context.Context, worldID uuid.UUID) ([]seaVoyage, error) {
	var out []seaVoyage
	rows, err := h.pool.Query(ctx,
		`SELECT id, owner_id, type, cargo_unit_id, march_route, status, depart_tick, arrive_tick
		 FROM units
		 WHERE world_id = $1 AND category = 'naval' AND status = 'marching' AND march_route IS NOT NULL`,
		worldID)
	if err != nil {
		return nil, fmt.Errorf("sea storm scan: load ships: %w", err)
	}
	for rows.Next() {
		var v seaVoyage
		var raw []byte
		var status string
		var dep, arr *int
		if err := rows.Scan(&v.shipID, &v.owner, &v.shipType, &v.cargoUnitID, &raw, &status, &dep, &arr); err != nil {
			rows.Close()
			return nil, fmt.Errorf("sea storm scan: scan ship: %w", err)
		}
		route, ok := LoadActiveRoute(raw, status, dep, arr)
		if !ok {
			continue // no saved route for this march: nothing to read hexes from
		}
		v.route, v.departTick, v.arriveTick, v.errand = route, *dep, *arr, "march"
		v.key = fmt.Sprintf("u:%s:%d:%d", v.shipID, *dep, *arr)
		out = append(out, v)
	}
	rows.Close()

	rows, err = h.pool.Query(ctx,
		`SELECT t.id, t.owner_id, t.kind, t.ship_unit_id, u.type, t.journey, t.departed_tick, t.due_tick
		 FROM transports t JOIN units u ON u.id = t.ship_unit_id
		 WHERE t.world_id = $1 AND t.status = 'in_transit' AND t.category = 'naval'
		   AND t.journey IS NOT NULL AND t.departed_tick IS NOT NULL`,
		worldID)
	if err != nil {
		return nil, fmt.Errorf("sea storm scan: load sea transports: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var v seaVoyage
		var tid uuid.UUID
		var raw []byte
		if err := rows.Scan(&tid, &v.owner, &v.errand, &v.shipID, &v.shipType, &raw, &v.departTick, &v.arriveTick); err != nil {
			return nil, fmt.Errorf("sea storm scan: scan transport: %w", err)
		}
		route, ok := journeyRoute(raw, v.departTick, v.arriveTick)
		if !ok {
			continue
		}
		v.route, v.transportID = route, &tid
		v.key = "t:" + tid.String()
		out = append(out, v)
	}
	return out, rows.Err()
}

// journeyRoute reads a transport's frozen journey as a StoredRoute so both
// kinds of voyage share one hex-entry clock (RouteEnterMilli).
func journeyRoute(raw []byte, departed, due int) (StoredRoute, bool) {
	var j province.TradeJourney
	if err := json.Unmarshal(raw, &j); err != nil || j.Validate() != nil || len(j.Path) < 2 {
		return StoredRoute{}, false
	}
	hexes := make([][2]int, len(j.Path))
	for i, p := range j.Path {
		hexes[i] = [2]int{p.Q, p.R}
	}
	r := StoredRoute{StartTick: departed, EndTick: due, Hexes: hexes, Costs: j.StepCosts}
	if r.Move().Validate() != nil {
		return StoredRoute{}, false
	}
	return r, true
}

// sail rolls every sea hex this voyage has entered since its last roll, up
// to the end of dueTick, in one TX with the progress mark.
func (h *SeaStormScanHandler) sail(ctx context.Context, worldID uuid.UUID, dueTick int, graph province.TileGraph, occupied stormOccupancy, v seaVoyage) error {
	enter, err := RouteEnterMilli(v.route)
	if err != nil {
		return err
	}

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// A voyage first seen now is checked from the previous scan's tick on: ticks before
	// that were never this scan's (a ship already at sea when the scan was deployed is
	// not retro-stormed). The mark is in ticks; last_hex only exists on older rows.
	if _, err := tx.Exec(ctx,
		`INSERT INTO sea_storm_progress (voyage_key, world_id, ship_unit_id, last_hex, last_tick)
		 VALUES ($1, $2, $3, 0, $4) ON CONFLICT (voyage_key) DO NOTHING`,
		v.key, worldID, v.shipID, dueTick-1); err != nil {
		return err
	}
	var lastTick *int
	if err := tx.QueryRow(ctx, `SELECT last_tick FROM sea_storm_progress WHERE voyage_key = $1 FOR UPDATE`, v.key).Scan(&lastTick); err != nil {
		return err
	}
	last := dueTick - 1
	if lastTick != nil {
		last = *lastTick
	}

	// Re-check under lock that the voyage is still this voyage. Transport
	// before ship, the same lock order as the intercept scan.
	if v.transportID != nil {
		var tstatus string
		if err := tx.QueryRow(ctx, `SELECT status FROM transports WHERE id = $1 FOR UPDATE`, *v.transportID).Scan(&tstatus); err != nil {
			return err
		}
		if tstatus != "in_transit" {
			return tx.Commit(ctx)
		}
	}
	var status string
	var hull int
	var dep, arr *int
	if err := tx.QueryRow(ctx,
		`SELECT status, hull, depart_tick, arrive_tick FROM units WHERE id = $1 FOR UPDATE`, v.shipID,
	).Scan(&status, &hull, &dep, &arr); err != nil {
		return err
	}
	if status == string(unit.StatusDisbanded) {
		return tx.Commit(ctx)
	}
	if v.transportID == nil && (status != string(unit.StatusMarching) || dep == nil || arr == nil || *dep != v.departTick || *arr != v.arriveTick) {
		return tx.Commit(ctx)
	}

	var storms []SeaStormPayload
	for t := last + 1; t <= dueTick; t++ {
		if t > v.arriveTick || int64(t)*1000 < enter[0] {
			continue // not under way at this tick
		}
		i := 0
		for j := range enter {
			if enter[j] <= int64(t)*1000 {
				i = j
			}
		}
		hx := v.route.Hexes[i]
		if !isSeaTerrain(graph[hx]) || !occupied.at(t, hx) {
			continue
		}
		before := hull
		hull--
		storms = append(storms, SeaStormPayload{
			WorldID: worldID, ShipID: v.shipID, OwnerID: v.owner, ShipType: v.shipType,
			Q: hx[0], R: hx[1], Tick: t, HullBefore: before, Hull: hull, HullMax: hullMax,
			Foundered: hull <= 0, Errand: v.errand, TransportID: v.transportID,
		})
		if hull <= 0 {
			break
		}
	}
	last = dueTick

	if _, err := tx.Exec(ctx,
		`UPDATE sea_storm_progress SET last_tick = $2, updated_at = now() WHERE voyage_key = $1`, v.key, last); err != nil {
		return err
	}
	// Keep one progress row per ship: an earlier march's mark is finished.
	if _, err := tx.Exec(ctx,
		`DELETE FROM sea_storm_progress WHERE ship_unit_id = $1 AND voyage_key <> $2`, v.shipID, v.key); err != nil {
		return err
	}
	if len(storms) == 0 {
		return tx.Commit(ctx)
	}

	name := unit.LoadDisplayName(ctx, tx, v.shipID)
	for i := range storms {
		storms[i].Name = name
	}
	if hull > 0 {
		if _, err := tx.Exec(ctx, `UPDATE units SET hull = $2, updated_at = now() WHERE id = $1`, v.shipID, hull); err != nil {
			return err
		}
	} else if err := h.founder(ctx, tx, v, &storms[len(storms)-1]); err != nil {
		return err
	}

	var recorded []*events.Event
	if hull <= 0 {
		last := storms[len(storms)-1]
		recorded, err = carrier.OutcomeTx(ctx, tx, h.eventStore, worldID, v.shipID, nil, "storm", last.Q, last.R, last.Tick)
		if err != nil {
			return err
		}
	}
	for _, s := range storms {
		kind := EventShipStormDamaged
		if s.Foundered {
			kind = EventShipFoundered
		}
		ev, err := h.eventStore.AppendTx(ctx, tx, v.shipID, events.StreamType(unit.StreamUnit), kind, s, worldID, nil)
		if err != nil {
			return err
		}
		recorded = append(recorded, ev)
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	for _, ev := range recorded {
		h.eventStore.RecordCommitted(ctx, ev)
	}
	if h.hub == nil {
		return nil
	}
	for _, s := range storms {
		kind, level := EventShipStormDamaged, 3
		if s.Foundered {
			kind, level = EventShipFoundered, 4
		}
		if err := h.hub.NotifyPlayer(ctx, worldID, v.owner, kind, level, s); err != nil {
			slog.Warn("sea storm scan: notify failed", "ship", v.shipID, "err", err)
		}
	}
	return nil
}

// founder sinks the ship with everything aboard: embarked troops drown, a
// transport's cargo is gone (its transport_goods rows stay as the record).
// Status 'foundered' makes every delivery/arrival handler skip the leg.
func (h *SeaStormScanHandler) founder(ctx context.Context, tx pgx.Tx, v seaVoyage, s *SeaStormPayload) error {
	if _, err := tx.Exec(ctx,
		`UPDATE units SET hull = 0, status = 'disbanded', size = 0, updated_at = now() WHERE id = $1`, v.shipID); err != nil {
		return err
	}
	// An expedition ends with its ship; what it saw stays in player_scouted_tiles.
	if _, err := tx.Exec(ctx, `DELETE FROM unit_expeditions WHERE unit_id = $1`, v.shipID); err != nil {
		return err
	}
	if v.cargoUnitID != nil {
		var t SeaStormTroops
		err := tx.QueryRow(ctx,
			`UPDATE units u SET status = 'disbanded', size = 0, updated_at = now()
			 FROM (SELECT id, type, size FROM units WHERE id = $1 AND status = 'embarked' FOR UPDATE) old
			 WHERE u.id = old.id RETURNING old.id, old.type, old.size`, *v.cargoUnitID,
		).Scan(&t.UnitID, &t.UnitType, &t.Size)
		if err != nil && err != pgx.ErrNoRows {
			return err
		}
		if err == nil {
			s.Troops = &t
		}
	}
	if v.transportID != nil {
		if _, err := tx.Exec(ctx,
			`UPDATE transports SET status = 'foundered', updated_at = now() WHERE id = $1`, *v.transportID); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT good_key, quantity FROM transport_goods WHERE transport_id = $1 ORDER BY good_key`, *v.transportID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var c SeaStormCargo
			if err := rows.Scan(&c.GoodKey, &c.Quantity); err != nil {
				return err
			}
			s.Cargo = append(s.Cargo, c)
		}
		return rows.Err()
	}
	return nil
}

// stormOccupancy is "which hexes a storm covers at which tick", read from the stored tracks.
type stormOccupancy map[int]map[[2]int]bool

func (o stormOccupancy) at(tick int, hx [2]int) bool { return o[tick][hx] }

func loadStormOccupancy(ctx context.Context, pool *pgxpool.Pool, worldID uuid.UUID, fromTick, toTick int) (stormOccupancy, error) {
	rows, err := pool.Query(ctx,
		`SELECT t.tick, t.q, t.r FROM sea_storm_track t JOIN sea_storms s ON s.id = t.storm_id
		  WHERE s.world_id = $1 AND t.tick BETWEEN $2 AND $3`, worldID, fromTick, toTick)
	if err != nil {
		return nil, fmt.Errorf("sea storm scan: load storm tracks: %w", err)
	}
	defer rows.Close()
	out := stormOccupancy{}
	for rows.Next() {
		var tick, q, r int
		if err := rows.Scan(&tick, &q, &r); err != nil {
			return nil, err
		}
		if out[tick] == nil {
			out[tick] = map[[2]int]bool{}
		}
		out[tick][[2]int{q, r}] = true
	}
	return out, rows.Err()
}

// stepWeather creates the world's storms on first sight and walks every track to dueTick.
func (h *SeaStormScanHandler) stepWeather(ctx context.Context, worldID uuid.UUID, dueTick int, graph province.TileGraph) error {
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, "sea_storms:"+worldID.String()); err != nil {
		return err
	}
	if err := ensureStorms(ctx, tx, worldID, dueTick, graph, h.Dice); err != nil {
		return err
	}
	if err := advanceStorms(ctx, tx, worldID, dueTick, graph, h.Dice); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
