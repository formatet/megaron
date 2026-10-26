package combat

// Geographic naming follows entered hexes of frozen journeys, never an eye's
// visible radius. One recurring scan owns claims for units and transports.
// Messengers have no frozen hex route and deliberately do not participate.
import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/hexgrid"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/tick"
	"formatet/megaron/server/internal/unit"
	"formatet/megaron/server/internal/world"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PlaceReachScanHandler struct {
	pool      *pgxpool.Pool
	scheduler *events.Scheduler
	store     *events.Store
	hub       Broadcaster
}

func NewPlaceReachScanHandler(pool *pgxpool.Pool, scheduler *events.Scheduler, store *events.Store, hub Broadcaster) *PlaceReachScanHandler {
	return &PlaceReachScanHandler{pool, scheduler, store, hub}
}

func placeReachLock(ctx context.Context, tx pgx.Tx, worldID uuid.UUID) error {
	// All entry scanners and recall/redirect take this before actor row locks.
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 617))`, worldID.String())
	return err
}

// BeforeArrival preserves reached hexes before handlers remove frozen journeys.
// Naming is best effort: a persistence failure must never stop movement.
func (h *PlaceReachScanHandler) BeforeArrival(next events.Handler) events.Handler {
	return func(ctx context.Context, e events.ScheduledEvent) error {
		if err := h.flushEntries(ctx, e); err != nil {
			slog.Warn("place reach before arrival failed; continuing movement", "world", e.WorldID, "error", err)
		}
		return next(ctx, e)
	}
}
func (h *PlaceReachScanHandler) Handle(ctx context.Context, e events.ScheduledEvent) error {
	if err := h.flushEntries(ctx, e); err != nil {
		return err
	}
	return h.scheduler.EnqueueTickRecurring(ctx, e.WorldID, events.ScheduledPlaceReachScan, struct{}{}, e.DueTick, 1)
}
func (h *PlaceReachScanHandler) flushEntries(ctx context.Context, e events.ScheduledEvent) error {
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err = placeReachLock(ctx, tx, e.WorldID); err != nil {
		return err
	}
	through := int64(e.DueTick) * 1000
	if e.EventType == events.ScheduledUnitArrivalV2 {
		a, err := tick.LoadAnchor(ctx, tx, e.WorldID)
		if err != nil {
			return err
		}
		through = a.MilliAt(h.scheduler.Clock().Now())
	}
	outcomes, err := recordPlaceEntriesTx(ctx, tx, h.store, e.WorldID, through)
	if err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	deliverPlaceReach(ctx, h.store, h.hub, e.WorldID, outcomes)
	return nil
}

type placeVoyage struct {
	id, owner      uuid.UUID
	displayID      uuid.UUID
	key, name, typ string
	route          StoredRoute
	departed       time.Time
	last           int64
	skip           bool
}
type placeEntry struct {
	voyage    int
	milli     int64
	hex       [2]int
	sightOnly bool
}

type placeReachTarget struct {
	place world.Place
	// A known object hex for the notice's map action. For a massif this is
	// the bordering mountain, not the land hex the unit actually entered.
	hex [2]int
}

func placeReachTargets(tiles []world.MapTile) map[[2]int][]placeReachTarget {
	terrain := make(map[[2]int]world.Terrain, len(tiles))
	for _, t := range tiles {
		terrain[[2]int{t.Q, t.R}] = t.Terrain
	}
	objects := map[[2]int][]placeReachTarget{}
	for _, p := range world.Places(tiles) {
		if p.Kind != "massif" {
			for _, hex := range p.Hexes {
				objects[hex] = append(objects[hex], placeReachTarget{p, hex})
			}
			continue
		}
		// Mountains remain impassable. First entry on bordering land grants
		// the right; sight alone never does. Deduplicate shared borders and
		// choose the first canonical adjacent mountain as the visible witness.
		seen := map[[2]int]bool{}
		for _, hex := range p.Hexes {
			for _, n := range hexgrid.Neighbors(hexgrid.Coord{Q: hex[0], R: hex[1]}) {
				border := [2]int{n.Q, n.R}
				t, exists := terrain[border]
				if !exists || !province.IsPassable(string(t), "land") || seen[border] {
					continue
				}
				seen[border] = true
				objects[border] = append(objects[border], placeReachTarget{p, hex})
			}
		}
	}
	return objects
}

func loadPlaceVoyages(ctx context.Context, tx pgx.Tx, worldID uuid.UUID) ([]placeVoyage, error) {
	var out []placeVoyage
	// Transport then unit matches storm/interception row-lock order.
	rows, err := tx.Query(ctx, `SELECT t.id,t.owner_id,COALESCE(u.name,''),COALESCE(u.type,'caravan'),t.kind,t.journey,t.departed_tick,t.due_tick,t.departs_at,t.ship_unit_id FROM transports t LEFT JOIN units u ON u.id=t.ship_unit_id WHERE t.world_id=$1 AND t.status='in_transit' AND t.journey IS NOT NULL AND t.departed_tick IS NOT NULL ORDER BY t.id FOR UPDATE OF t`, worldID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var v placeVoyage
		var raw []byte
		var dep, arr int
		var kind string
		var shipID *uuid.UUID
		if err = rows.Scan(&v.id, &v.owner, &v.name, &v.typ, &kind, &raw, &dep, &arr, &v.departed, &shipID); err != nil {
			rows.Close()
			return nil, err
		}
		r, ok := journeyRoute(raw, dep, arr)
		if !ok {
			slog.Warn("place reach skips invalid transport route", "world", worldID, "transport", v.id)
			continue
		}
		v.route = r
		v.key = "t:" + v.id.String()
		if shipID != nil {
			v.displayID = *shipID
		}
		if v.name == "" {
			if v.typ == "caravan" {
				v.name = "Caravan"
			} else {
				v.name = unit.DisplayName(v.typ)
			}
		}
		out = append(out, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	rows, err = tx.Query(ctx, `SELECT id,owner_id,COALESCE(name,''),type,march_route,depart_tick,arrive_tick,departs_at FROM units WHERE world_id=$1 AND status='marching' AND size>0 AND march_route IS NOT NULL ORDER BY id FOR UPDATE`, worldID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var v placeVoyage
		var raw []byte
		var dep, arr *int
		var departed *time.Time
		if err = rows.Scan(&v.id, &v.owner, &v.name, &v.typ, &raw, &dep, &arr, &departed); err != nil {
			return nil, err
		}
		r, ok := LoadActiveRoute(raw, "marching", dep, arr)
		if !ok {
			slog.Warn("place reach skips invalid unit route", "world", worldID, "unit", v.id)
			continue
		}
		v.route = r
		v.displayID = v.id
		if departed != nil {
			v.departed = *departed
		}
		v.key = fmt.Sprintf("u:%s:%x", v.id, sha256.Sum256(raw))
		out = append(out, v)
	}
	return out, rows.Err()
}

// recordPlaceEntriesTx is the sole physical-entry hook. Recurring scans and a
// route replacement flush call it with their actual cutoff, while holding the
// same world lock. Claims, durable notifications and journey progress commit
// together; competing actors are ordered by exact saved-route entry time.
func recordPlaceEntriesTx(ctx context.Context, tx pgx.Tx, store *events.Store, worldID uuid.UUID, through int64) ([]expeditionOutcome, error) {
	rows, err := tx.Query(ctx, `SELECT q,r,terrain FROM map_tiles WHERE world_id=$1`, worldID)
	if err != nil {
		return nil, err
	}
	var tiles []world.MapTile
	for rows.Next() {
		var t world.MapTile
		if err = rows.Scan(&t.Q, &t.R, &t.Terrain); err != nil {
			rows.Close()
			return nil, err
		}
		tiles = append(tiles, t)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	objects := placeReachTargets(tiles)
	voyages, err := loadPlaceVoyages(ctx, tx, worldID)
	if err != nil {
		return nil, err
	}
	var deployTick int
	var appliedAt *time.Time
	err = tx.QueryRow(ctx, `SELECT first_tick,applied_at FROM place_reach_epoch WHERE world_id=$1`, worldID).Scan(&deployTick, &appliedAt)
	if err != nil && err != pgx.ErrNoRows {
		return nil, err
	}
	var entries []placeEntry
	for i := range voyages {
		v := &voyages[i]
		entered, err := RouteEnterMilli(v.route)
		if err != nil {
			slog.Warn("place reach skips unreadable route", "world", worldID, "journey", v.key, "error", err)
			v.skip = true
			continue
		}
		initial := int64(v.route.StartTick)*1000 - 1
		// Existing journeys skip the deployment's partial tick. New departures
		// after deployment retain their immediate first-step boundary.
		if appliedAt != nil && v.departed.Before(*appliedAt) && initial < int64(deployTick)*1000-1 {
			initial = int64(deployTick)*1000 - 1
		}
		if _, err = tx.Exec(ctx, `INSERT INTO place_reach_progress(journey_key,world_id,last_milli,last_tick) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, v.key, worldID, initial, int(initial/1000)); err != nil {
			return nil, err
		}
		if err = tx.QueryRow(ctx, `SELECT last_milli FROM place_reach_progress WHERE journey_key=$1 FOR UPDATE`, v.key).Scan(&v.last); err != nil {
			return nil, err
		}
		first := 1
		if v.route.Version == 2 && v.displayID == v.id {
			first = 0 // departure is an eye, but grants no new naming right
		}
		for j := first; j < len(entered); j++ {
			if entered[j] > v.last && entered[j] <= through {
				entries = append(entries, placeEntry{i, entered[j], v.route.Hexes[j], j == 0})
			}
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		if a.milli != b.milli {
			return a.milli < b.milli
		}
		return voyages[a.voyage].key < voyages[b.voyage].key
	})
	var outcomes []expeditionOutcome
	for _, entry := range entries {
		v := voyages[entry.voyage]
		if v.route.Version == 2 {
			// A reached unit hex is an eye even if no map read sampled it.
			// Transports retain their existing memory semantics.
			if v.displayID == v.id {
				kind := province.EyeLandUnit
				if unit.CategoryOf(unit.Type(v.typ)) == unit.CategoryNaval {
					kind = province.EyeShip
				}
				if err = province.SweepLiveRadius(ctx, tx, worldID, v.owner, province.MapPosition{Q: entry.hex[0], R: entry.hex[1]}, kind); err != nil {
					return nil, err
				}
			} else if _, err = tx.Exec(ctx, `INSERT INTO player_scouted_tiles(world_id,player_id,q,r) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, worldID, v.owner, entry.hex[0], entry.hex[1]); err != nil {
				return nil, err
			}
		}
		if entry.sightOnly {
			continue
		}
		for _, target := range objects[entry.hex] {
			p := target.place
			if _, err = tx.Exec(ctx, `INSERT INTO place_names(world_id,kind,anchor_q,anchor_r) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, worldID, p.Kind, p.Anchor[0], p.Anchor[1]); err != nil {
				return nil, err
			}
			var named *int
			err = tx.QueryRow(ctx, `UPDATE place_names SET claimed_by=$5,claimed_tick=$6 WHERE world_id=$1 AND kind=$2 AND anchor_q=$3 AND anchor_r=$4 AND claimed_by IS NULL RETURNING named_tick`, worldID, p.Kind, p.Anchor[0], p.Anchor[1], v.owner, int(entry.milli/1000)).Scan(&named)
			if err == pgx.ErrNoRows {
				continue
			}
			if err != nil {
				return nil, err
			}
			if _, err = tx.Exec(ctx, `INSERT INTO player_scouted_tiles(world_id,player_id,q,r) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, worldID, v.owner, entry.hex[0], entry.hex[1]); err != nil {
				return nil, err
			}
			if target.hex != entry.hex {
				// Adjacent mountains are visible from this physical border step.
				// Retain only this witness, never the unseen rest of the massif.
				if _, err = tx.Exec(ctx, `INSERT INTO player_scouted_tiles(world_id,player_id,q,r) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, worldID, v.owner, target.hex[0], target.hex[1]); err != nil {
					return nil, err
				}
			}
			if named != nil {
				continue
			} // admin-spent right never promises a naming action
			displayName := v.name
			if v.displayID != uuid.Nil {
				if name := unit.LoadDisplayName(ctx, tx, v.displayID); name != "" {
					displayName = name
				}
			}
			subjectID := v.id
			if v.displayID != uuid.Nil {
				subjectID = v.displayID
			}
			body := map[string]any{"unit_id": subjectID, "unit_name": displayName, "unit_type": v.typ, "kind": p.Kind, "place_id": fmt.Sprintf("%d,%d", p.Anchor[0], p.Anchor[1]), "q": target.hex[0], "r": target.hex[1]}
			raw, _ := json.Marshal(body)
			var id string
			if err = tx.QueryRow(ctx, `INSERT INTO notifications(world_id,player_id,kind,level,body_json) VALUES($1,$2,'PlaceFirstReached',1,$3) RETURNING id`, worldID, v.owner, raw).Scan(&id); err != nil {
				return nil, err
			}
			event, err := store.AppendTx(ctx, tx, worldID, events.StreamWorld, "PlaceFirstReached", body, worldID, nil)
			if err != nil {
				return nil, err
			}
			outcomes = append(outcomes, expeditionOutcome{event: event, owner: v.owner, id: id, kind: "PlaceFirstReached", level: 1, body: body})
		}
	}
	for _, v := range voyages {
		if !v.skip && through > v.last {
			if _, err = tx.Exec(ctx, `UPDATE place_reach_progress SET last_milli=$2,last_tick=$3 WHERE journey_key=$1`, v.key, through, int(through/1000)); err != nil {
				return nil, err
			}
		}
	}
	return outcomes, nil
}
func deliverPlaceReach(ctx context.Context, store *events.Store, hub Broadcaster, worldID uuid.UUID, outcomes []expeditionOutcome) {
	for _, o := range outcomes {
		store.RecordCommitted(ctx, o.event)
		if d, ok := hub.(CommittedNotificationDelivery); ok {
			d.DeliverCommittedNotification(ctx, worldID, o.owner, o.id, o.kind, o.level, o.body)
		}
	}
}
