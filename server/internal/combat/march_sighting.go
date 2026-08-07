package combat

import (
	"context"
	"fmt"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/province"
	"github.com/google/uuid"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	// marchSightingScanIntervalTicks is how often the sweep runs. Every tick,
	// like the interception scan — a march that walks into your live tier must
	// be reported while there is still travel time left to answer it
	// (asynkronitetsgrinden, megaron_arbetssatt §14).
	marchSightingScanIntervalTicks = 1

	// ForeignMarchSightedKind is the notification kind. Its payload semantics are
	// frozen forever (CLAUDE.md §Events) — a changed meaning needs a new kind,
	// never a new reading of this one.
	ForeignMarchSightedKind = "ForeignMarchSighted"
)

// MarchSightingHandler is the recurring sweep that tells a Wanax, once, that a
// foreign march has become visible to them. It is the notification that starts
// the clock in the asynchronicity gate: until it existed, a defender's first
// signal about an approaching army arrived AFTER the battle was resolved, so the
// inequality "order travel + defender travel < attacker's remaining travel" could
// not even be tested — the defender had no start time.
//
// The visibility gate is deliberately the SAME one GET /worlds/{id}/foreign-units
// uses (province.LoadLiveEyes + the unit's INTERPOLATED CURRENT position +
// province.AnyEyeSees), not a second copy of the rules. Two copies diverge the
// first time anyone touches vision. Never the departure hex, never the target hex:
// "it is heading for my city, therefore I know about it" is exactly the leak this
// gate exists to prevent.
//
// Idempotent (CLAUDE.md §Events): every notification is guarded by the march_key
// dedupe query in notifySighting, so running the same sweep twice produces zero
// extra notifications.
type MarchSightingHandler struct {
	pool      *pgxpool.Pool
	scheduler *events.Scheduler
	hub       Broadcaster // may be nil in tests
	clk       clock.Clock
}

// NewMarchSightingHandler creates a MarchSightingHandler.
func NewMarchSightingHandler(pool *pgxpool.Pool, sched *events.Scheduler, hub Broadcaster, clk clock.Clock) *MarchSightingHandler {
	return &MarchSightingHandler{pool: pool, scheduler: sched, hub: hub, clk: clk}
}

// sightedMarch is one marching unit resolved to its current position, ready to be
// tested against each candidate receiver's eyes.
type sightedMarch struct {
	id         uuid.UUID
	owner      uuid.UUID
	ownerName  string
	unitType   string
	category   string
	size       int
	stance     string
	targetQ    int
	targetR    int
	arrivesAt  time.Time
	arriveTick *int
	pos        province.MapPosition
	terrain    string
	key        string
}

// Handle sweeps every marching unit in the world once, notifying each player who
// can currently see one, then re-enqueues itself.
func (h *MarchSightingHandler) Handle(ctx context.Context, e events.ScheduledEvent) error {
	now := h.clk.Now()

	marches, err := h.loadMarches(ctx, e.WorldID, now)
	if err != nil {
		return err
	}
	// Nothing on the move: requeue without loading the tile graph, the eyes or
	// the receiver list. The common case in a quiet world costs one query.
	if len(marches) == 0 {
		return h.requeue(ctx, e)
	}

	receivers, err := h.loadReceivers(ctx, e.WorldID)
	if err != nil {
		return err
	}
	owned, err := h.loadOwnedSettlementHexes(ctx, e.WorldID)
	if err != nil {
		return err
	}

	for _, playerID := range receivers {
		eyes := province.LoadLiveEyes(ctx, h.pool, e.WorldID, playerID, now)
		if len(eyes) == 0 {
			continue
		}
		for _, m := range marches {
			if m.owner == playerID {
				continue // never warn a Wanax about their own march
			}
			if !province.AnyEyeSees(eyes, m.pos, m.terrain) {
				continue
			}
			h.notifySighting(ctx, e.WorldID, playerID, m, owned)
		}
	}

	return h.requeue(ctx, e)
}

// loadMarches reads every marching unit and resolves it to the position it
// occupies right now, exactly as /foreign-units does.
func (h *MarchSightingHandler) loadMarches(ctx context.Context, worldID uuid.UUID, now time.Time) ([]sightedMarch, error) {
	rows, err := h.pool.Query(ctx,
		`SELECT u.id, u.owner_id, COALESCE(pl.username,''), u.type, u.category, u.size,
		        COALESCE(u.stance,''), u.q, u.r, u.target_q, u.target_r,
		        u.departs_at, u.arrives_at, u.arrive_tick
		 FROM units u
		 LEFT JOIN players pl ON pl.id = u.owner_id
		 WHERE u.world_id = $1 AND u.status = 'marching' AND u.owner_id IS NOT NULL
		   AND u.q IS NOT NULL AND u.r IS NOT NULL
		   AND u.target_q IS NOT NULL AND u.target_r IS NOT NULL
		   AND u.departs_at IS NOT NULL AND u.arrives_at IS NOT NULL`,
		worldID,
	)
	if err != nil {
		return nil, fmt.Errorf("march sighting: load marches: %w", err)
	}

	type rawMarch struct {
		sightedMarch
		originQ   int
		originR   int
		departsAt time.Time
	}
	var raw []rawMarch
	for rows.Next() {
		var m rawMarch
		if scanErr := rows.Scan(&m.id, &m.owner, &m.ownerName, &m.unitType, &m.category, &m.size,
			&m.stance, &m.originQ, &m.originR, &m.targetQ, &m.targetR,
			&m.departsAt, &m.arrivesAt, &m.arriveTick); scanErr != nil {
			rows.Close()
			return nil, fmt.Errorf("march sighting: scan march: %w", scanErr)
		}
		raw = append(raw, m)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("march sighting: load marches: %w", err)
	}
	if len(raw) == 0 {
		return nil, nil
	}

	g, err := province.LoadTileGraph(ctx, h.pool, worldID)
	if err != nil {
		return nil, fmt.Errorf("march sighting: load tile graph: %w", err)
	}

	out := make([]sightedMarch, 0, len(raw))
	for _, m := range raw {
		pos := province.MapPosition{Q: m.originQ, R: m.originR}
		if path, _, ok := g.FindPath(pos, province.MapPosition{Q: m.targetQ, R: m.targetR}, m.category); ok && len(path) > 0 {
			pos = province.InterpolateAlongPath(now, m.departsAt, m.arrivesAt, path)
		}
		// Pathfinding failure keeps the stored origin hex — best-effort, same as
		// /foreign-units. The unit is never silently dropped for it.

		terrain, ok := g[[2]int{pos.Q, pos.R}]
		if !ok {
			// Fail closed: with no map_tiles row for the hex there is no way to
			// decide who can see it, so nobody is told. Same rule as the surface.
			continue
		}

		s := m.sightedMarch
		s.pos = pos
		s.terrain = terrain
		// Dedupe key. A string, compared as a string — never as a time. A NEW
		// march by the same unit gets a new departs_at and therefore a new key,
		// so it warrants a new warning; the same march never does.
		s.key = m.id.String() + "@" + m.departsAt.UTC().Format(time.RFC3339)
		out = append(out, s)
	}
	return out, nil
}

// loadReceivers returns every player with a foothold in the world — a settlement
// or a unit. A player with neither has no eyes and cannot see anything anyway.
func (h *MarchSightingHandler) loadReceivers(ctx context.Context, worldID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := h.pool.Query(ctx,
		`SELECT DISTINCT owner_id FROM settlements WHERE world_id = $1 AND owner_id IS NOT NULL
		 UNION
		 SELECT DISTINCT owner_id FROM units WHERE world_id = $1 AND owner_id IS NOT NULL`,
		worldID,
	)
	if err != nil {
		return nil, fmt.Errorf("march sighting: load receivers: %w", err)
	}
	defer rows.Close()

	var out []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if scanErr := rows.Scan(&id); scanErr != nil {
			return nil, fmt.Errorf("march sighting: scan receiver: %w", scanErr)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// ownedHex is a settlement seen from the map: who holds it and what it is called.
type ownedHex struct {
	owner uuid.UUID
	id    uuid.UUID
	name  string
}

// loadOwnedSettlementHexes maps each settled hex to its holder, so a march's
// target hex can be tested against the RECEIVER's own cities. Only the receiver's
// own holdings may raise the level — that keeps the urgency decision inside what
// the receiver already knows, never inside what the scan knows.
func (h *MarchSightingHandler) loadOwnedSettlementHexes(ctx context.Context, worldID uuid.UUID) (map[[2]int]ownedHex, error) {
	rows, err := h.pool.Query(ctx,
		`SELECT p.map_q, p.map_r, s.owner_id, s.id, s.name
		 FROM settlements s JOIN provinces p ON p.id = s.province_id
		 WHERE s.world_id = $1 AND s.owner_id IS NOT NULL`,
		worldID,
	)
	if err != nil {
		return nil, fmt.Errorf("march sighting: load settlement hexes: %w", err)
	}
	defer rows.Close()

	out := make(map[[2]int]ownedHex)
	for rows.Next() {
		var q, r int
		var oh ownedHex
		if scanErr := rows.Scan(&q, &r, &oh.owner, &oh.id, &oh.name); scanErr != nil {
			return nil, fmt.Errorf("march sighting: scan settlement hex: %w", scanErr)
		}
		out[[2]int{q, r}] = oh
	}
	return out, rows.Err()
}

// notifySighting sends one ForeignMarchSighted to playerID, unless this exact
// march has already been reported to them.
//
// The dedupe query deliberately has NO "read_at IS NULL" clause, unlike its model
// notifyUpkeepUnpaid (upkeep.go). There, a READ warning should be able to return
// next period because each period is a materially more urgent notice. Here it is
// the opposite: the scan runs every tick for as long as the army stays in sight,
// so keying on unread rows would re-alarm the same march over and over the moment
// the player read it — punishing them for reading.
func (h *MarchSightingHandler) notifySighting(ctx context.Context, worldID, playerID uuid.UUID, m sightedMarch, owned map[[2]int]ownedHex) {
	if h.hub == nil {
		return
	}

	var exists bool
	if err := h.pool.QueryRow(ctx,
		`SELECT EXISTS (
		    SELECT 1 FROM notifications
		    WHERE world_id = $1 AND player_id = $2 AND kind = $3
		      AND body_json->>'march_key' = $4
		 )`,
		worldID, playerID, ForeignMarchSightedKind, m.key,
	).Scan(&exists); err == nil && exists {
		return
	}

	// level follows the file's scale (2 = urgent, 3 = info) and the irreversibility
	// gradient (Timothy 2026-08-03): a march ONTO one of your own cities is the
	// heavy end and must be urgent; one merely crossing your field of view is a
	// line of information, not an alarm. Two levels, one notification type.
	level := 3
	payload := map[string]any{
		"march_key": m.key,
		"unit_id":   m.id,
		"owner_id":  m.owner,
		"owner":     m.ownerName,
		"unit_type": m.unitType,
		"category":  m.category,
		"size":      m.size,
		"stance":    m.stance,
		// q/r is the OBSERVATION hex — where the unit was when it was seen — not
		// its origin and not its destination.
		"q":          m.pos.Q,
		"r":          m.pos.R,
		"target_q":   m.targetQ,
		"target_r":   m.targetR,
		"arrives_at": m.arrivesAt.UTC().Format(time.RFC3339),
	}
	if m.arriveTick != nil {
		// The tick is what the player actually counts in — speldygn, not wall clock.
		payload["arrive_tick"] = *m.arriveTick
	}
	if oh, ok := owned[[2]int{m.targetQ, m.targetR}]; ok && oh.owner == playerID {
		level = 2
		payload["threatens_settlement_id"] = oh.id
		payload["threatens_name"] = oh.name
	}

	_ = h.hub.NotifyPlayer(ctx, worldID, playerID, ForeignMarchSightedKind, level, payload)
}

// requeue schedules the next sweep.
func (h *MarchSightingHandler) requeue(ctx context.Context, e events.ScheduledEvent) error {
	return h.scheduler.EnqueueTickRecurring(ctx, e.WorldID, events.ScheduledMarchSightingScan,
		struct{}{}, e.DueTick, marchSightingScanIntervalTicks)
}
