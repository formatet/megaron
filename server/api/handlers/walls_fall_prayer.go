package handlers

// Murarna faller (bönesystemet v3 steg 5c): Potnia's offensive prayer, Rite side. The target is a
// foreign city of YOUR map whose REMEMBERED wall is at least level 1 (player_seen_settlements, the
// city memory - never the live wall). The cast is answered now; the wall falls
// religion.WallsFallTicks later (combat.WallsFallHandler). The caster is told only that Potnia
// heard; the target's owner gets an omen when in good standing, and always a notice when it falls
// - never who prayed.

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/kharis"
	"formatet/megaron/server/internal/religion"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// NoticeWallsOmen is the notification kind the target's owner gets when the wrath gathers.
const NoticeWallsOmen = "WallsOmen"

type prayerTargetCity struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Where string `json:"where"`
}

type wallsFallTarget struct {
	SettlementID uuid.UUID
	OwnerID      uuid.UUID
	Name         string
	Omen         bool // the owner's standing is good: they are warned at the cast
}

// checkWallsFallTarget validates the target inside the cast TX, before any gift is taken.
func (h *SettlementHandler) checkWallsFallTarget(ctx context.Context, tx pgx.Tx, worldID, playerID uuid.UUID, target string) (*wallsFallTarget, int, string) {
	target = strings.TrimSpace(target)
	if target == "" {
		return nil, http.StatusBadRequest, "name the city whose wall should fall: target = a foreign city on your map, by name or id"
	}
	rows, err := tx.Query(ctx,
		`SELECT pss.settlement_id, s.owner_id, pss.name, pss.wall_level
		   FROM player_seen_settlements pss JOIN settlements s ON s.id = pss.settlement_id
		  WHERE pss.world_id = $1 AND pss.player_id = $2 AND s.state <> 'sunk'
		    AND (pss.settlement_id::text = $3 OR lower(pss.name) = lower($3))`, worldID, playerID, target)
	if err != nil {
		return nil, http.StatusInternalServerError, "could not read your map"
	}
	type hit struct {
		id    uuid.UUID
		owner *uuid.UUID
		name  string
		wall  int
	}
	var hits []hit
	for rows.Next() {
		var x hit
		if err := rows.Scan(&x.id, &x.owner, &x.name, &x.wall); err != nil {
			rows.Close()
			return nil, http.StatusInternalServerError, "could not read your map"
		}
		hits = append(hits, x)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, http.StatusInternalServerError, "could not read your map"
	}
	var own bool
	_ = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM settlements WHERE world_id=$1 AND owner_id=$2 AND (id::text=$3 OR lower(name)=lower($3)))`,
		worldID, playerID, target).Scan(&own)
	for _, x := range hits {
		if x.owner != nil && *x.owner == playerID {
			own = true
		}
	}
	if own {
		return nil, http.StatusBadRequest, "Potnia will not bring down your own walls: name a city of another Wanax"
	}
	if len(hits) == 0 {
		return nil, http.StatusNotFound, "no such city on your map"
	}
	if len(hits) > 1 {
		return nil, http.StatusBadRequest, "more than one city on your map has that name: use its id"
	}
	x := hits[0]
	if x.owner == nil {
		return nil, http.StatusNotFound, "no such city on your map"
	}
	if x.wall < 1 {
		return nil, http.StatusBadRequest, fmt.Sprintf("you remember no wall at %s: the prayer needs a wall you have seen", x.name)
	}
	var standing float64
	_ = tx.QueryRow(ctx,
		`SELECT GREATEST(0, settled(kharis_amount, kharis_rate, kharis_calc_tick)) FROM player_world_records WHERE player_id = $1 AND world_id = $2`,
		*x.owner, worldID).Scan(&standing)
	return &wallsFallTarget{SettlementID: x.id, OwnerID: *x.owner, Name: x.name, Omen: standing >= kharis.BlessThreshold}, 0, ""
}

// applyWallsFall runs the cast: nothing changes now. The first map is what RiteCast records, the second
// what the Wanax is shown - that Potnia heard, never any wall level.
func (h *SettlementHandler) applyWallsFall(t *wallsFallTarget, nowTick int) (recorded, shown map[string]any, message string) {
	recorded = map[string]any{"target_settlement_id": t.SettlementID.String(), "falls_tick": nowTick + religion.WallsFallTicks, "omen_sent": t.Omen}
	shown = map[string]any{"heard": true}
	message = fmt.Sprintf("Potnia hears you. Whether the walls of %s fall, you will learn by seeing the city again.", t.Name)
	return recorded, shown, message
}

// scheduleWallsFall puts the fall in the queue inside the cast TX, once the cast event has an id.
func (h *SettlementHandler) scheduleWallsFall(ctx context.Context, tx pgx.Tx, worldID uuid.UUID, t *wallsFallTarget, castEventID int64, nowTick int) error {
	return h.scheduler.EnqueueTickTx(ctx, tx, worldID, events.ScheduledWallsFall,
		combat.WallsFallPayload{WorldID: worldID, SettlementID: t.SettlementID, CastEventID: castEventID}, nowTick+religion.WallsFallTicks)
}

// notifyWallsOmen warns the target's owner that someone has prayed against their walls - not who.
func (h *SettlementHandler) notifyWallsOmen(ctx context.Context, worldID uuid.UUID, t *wallsFallTarget) {
	if h.Hub == nil || !t.Omen {
		return
	}
	_ = h.Hub.NotifyPlayer(ctx, worldID, t.OwnerID, NoticeWallsOmen, 3,
		map[string]any{"settlement_id": t.SettlementID, "name": t.Name})
}

// loadWallsFallTargets lists the foreign cities of the Wanax's map whose remembered wall is at least 1.
func (h *ProvinceHandler) loadWallsFallTargets(ctx context.Context, owner, worldID uuid.UUID) (*prayerTargets, error) {
	rows, err := h.pool.Query(ctx,
		`SELECT pss.settlement_id, pss.name, pss.owner_name, pss.q, pss.r, pss.wall_level
		   FROM player_seen_settlements pss JOIN settlements s ON s.id = pss.settlement_id
		  WHERE pss.world_id = $1 AND pss.player_id = $2 AND s.state <> 'sunk'
		    AND s.owner_id IS NOT NULL AND s.owner_id <> $2 AND pss.wall_level >= 1
		  ORDER BY lower(pss.name), pss.settlement_id`, worldID, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := &prayerTargets{Unit: []prayerTargetUnit{}, Kind: "city", Cities: []prayerTargetCity{}}
	for rows.Next() {
		var id uuid.UUID
		var name, holder string
		var q, r, wall int
		if err := rows.Scan(&id, &name, &holder, &q, &r, &wall); err != nil {
			return nil, err
		}
		out.Cities = append(out.Cities, prayerTargetCity{ID: id.String(), Name: name,
			Where: fmt.Sprintf("held by %s, near (%d,%d), wall level %d as you last saw it", holder, q, r, wall)})
	}
	return out, rows.Err()
}
