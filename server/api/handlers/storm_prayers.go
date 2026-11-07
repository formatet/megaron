package handlers

// Stormbönerna (bönesystemet v3 steg 3b): Still the Storm and Wake the Storm, Rite side.
// The effect itself is combat.CalmStorm / combat.WakeStorm; this file validates the target,
// builds what the Wanax is told (never where the storm is now) and what the event records.

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/hexgrid"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/religion"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type prayerTargetStorm struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Where string `json:"where"`
}

func isStormPrayer(effect string) bool {
	return effect == religion.EffectStormCalm || effect == religion.EffectStormWake
}

func (h *SettlementHandler) stormDice() economy.Dice {
	if h.RiteDice != nil {
		return h.RiteDice
	}
	return economy.NewWallDice()
}

// parseHexTarget reads "q,r".
func parseHexTarget(s string) (hexgrid.Coord, bool) {
	a, b, ok := strings.Cut(strings.TrimSpace(s), ",")
	if !ok {
		return hexgrid.Coord{}, false
	}
	q, e1 := strconv.Atoi(strings.TrimSpace(a))
	r, e2 := strconv.Atoi(strings.TrimSpace(b))
	return hexgrid.Coord{Q: q, R: r}, e1 == nil && e2 == nil
}

// checkStormTarget validates the target of a storm prayer inside the cast TX. For Still the Storm it
// returns the storm; for Wake the Storm the hex. code != 0 is the HTTP refusal. The offering is not
// yet taken when this runs.
func (h *SettlementHandler) checkStormTarget(ctx context.Context, tx pgx.Tx, worldID, playerID uuid.UUID, spec religion.PrayerSpec, target string, nowTick int) (*uuid.UUID, *hexgrid.Coord, int, string) {
	if spec.EffectType == religion.EffectStormCalm {
		target = strings.TrimSpace(target)
		if target == "" {
			return nil, nil, http.StatusBadRequest, "name the storm to still: target = the name of a storm on your storm map"
		}
		// Only a storm on YOUR storm map (where you last saw it), alive, can be named as the target.
		var id uuid.UUID
		var name *string
		var err error
		if sid, perr := uuid.Parse(target); perr == nil {
			err = tx.QueryRow(ctx, `SELECT s.id, s.name FROM sea_storms s JOIN player_storm_sightings p ON p.storm_id = s.id AND p.player_id = $2
			  WHERE s.id = $3 AND s.world_id = $1 AND s.died_tick IS NULL AND s.dies_tick > $4`, worldID, playerID, sid, nowTick).Scan(&id, &name)
		} else {
			err = tx.QueryRow(ctx, `SELECT s.id, s.name FROM sea_storms s JOIN player_storm_sightings p ON p.storm_id = s.id AND p.player_id = $2
			  WHERE lower(s.name) = lower($3) AND s.world_id = $1 AND s.died_tick IS NULL AND s.dies_tick > $4`, worldID, playerID, target, nowTick).Scan(&id, &name)
		}
		if err == pgx.ErrNoRows {
			return nil, nil, http.StatusNotFound, "no such storm on your storm map"
		}
		if err != nil {
			return nil, nil, http.StatusInternalServerError, "could not read the storm map"
		}
		if name == nil {
			return nil, nil, http.StatusBadRequest, "only a storm that has been named can be stilled — name it first"
		}
		return &id, nil, 0, ""
	}
	// Wake the Storm: one sea hex of your own map. An unknown hex is answered as if it did not exist.
	hex, ok := parseHexTarget(target)
	if !ok {
		return nil, nil, http.StatusBadRequest, "name a sea hex of your map: target = \"q,r\""
	}
	graph, err := province.LoadTileGraph(ctx, tx, worldID)
	if err != nil {
		return nil, nil, http.StatusInternalServerError, "could not read the map"
	}
	terrain, exists := graph[[2]int{hex.Q, hex.R}]
	eyes := loadLiveEyes(ctx, h.pool, worldID, playerID, h.clk.Now())
	remembered := loadRememberedTiles(ctx, h.pool, worldID, playerID)
	if !exists || !knownToPlayer(eyes, remembered, province.MapPosition{Q: hex.Q, R: hex.R}, terrain) {
		return nil, nil, http.StatusNotFound, "that hex is not on your map"
	}
	if !province.IsThalassa(terrain) {
		return nil, nil, http.StatusBadRequest, "a storm can only be woken on open sea"
	}
	return nil, &hex, 0, ""
}

// applyStormCalm runs the cast. The first map is what the RiteCast event records, the second what
// the Wanax is shown: that the storm shrank, never where it is now.
func (h *SettlementHandler) applyStormCalm(ctx context.Context, tx pgx.Tx, worldID uuid.UUID, stormID uuid.UUID, nowTick int) (recorded, shown map[string]any, message string, code int, err error) {
	castKey := uuid.New()
	res, err := combat.CalmStorm(ctx, tx, worldID, stormID, nowTick, castKey, h.stormDice())
	if err == combat.ErrStormGone {
		return nil, nil, "", http.StatusConflict, nil
	}
	if err != nil {
		return nil, nil, "", 0, err
	}
	var name string
	_ = tx.QueryRow(ctx, `SELECT COALESCE(name,'') FROM sea_storms WHERE id = $1`, stormID).Scan(&name)
	recorded = map[string]any{"storm_id": stormID.String(), "cast_key": castKey.String(),
		"removed_hex": map[string]int{"q": res.Removed.Q, "r": res.Removed.R}, "remaining_hexes": res.Remaining, "died": res.Died}
	shown = map[string]any{"storm_id": stormID.String(), "storm_name": name, "shrunk": true, "died": res.Died}
	if res.Died {
		message = fmt.Sprintf("The Sea Daimons still %s for good — the storm is no more.", name)
	} else {
		recorded["regrow_tick"] = res.RegrowTick
		message = fmt.Sprintf("The Sea Daimons still %s — the storm shrinks by a hex. The hex will return in time.", name)
	}
	return recorded, shown, message, 0, nil
}

// applyStormWake runs the cast: the event records the new storm; the Wanax is told only that a storm
// was woken on the hex they named.
func (h *SettlementHandler) applyStormWake(ctx context.Context, tx pgx.Tx, worldID uuid.UUID, hex hexgrid.Coord, nowTick int) (recorded, shown map[string]any, message string, code int, err error) {
	castKey := uuid.New()
	res, err := combat.WakeStorm(ctx, tx, worldID, hex, nowTick, castKey, h.stormDice())
	if err == combat.ErrWakeOccupied || err == combat.ErrWakeNotSea {
		return nil, nil, err.Error(), http.StatusConflict, nil
	}
	if err != nil {
		return nil, nil, "", 0, err
	}
	hexes := make([]map[string]int, 0, len(res.Hexes))
	for _, c := range res.Hexes {
		hexes = append(hexes, map[string]int{"q": c.Q, "r": c.R})
	}
	recorded = map[string]any{"storm_id": res.StormID.String(), "cast_key": castKey.String(), "hexes": hexes}
	shown = map[string]any{"hex": map[string]int{"q": hex.Q, "r": hex.R}, "woken": true}
	message = fmt.Sprintf("The Sea Daimons wake a storm over (%d,%d). It will be as any storm: it strikes whoever sails into it.", hex.Q, hex.R)
	return recorded, shown, message, 0, nil
}

// loadStormTargets lists the named, living storms on the Wanax's storm map, with where they were
// last seen (never where they are now).
func (h *ProvinceHandler) loadStormTargets(ctx context.Context, owner uuid.UUID, worldID uuid.UUID, nowTick int) (*prayerTargets, error) {
	rows, err := h.pool.Query(ctx, `SELECT s.id, s.name, p.seen_tick, p.hexes->0->>'q', p.hexes->0->>'r'
	  FROM player_storm_sightings p JOIN sea_storms s ON s.id = p.storm_id
	 WHERE p.player_id = $1 AND s.world_id = $2 AND s.name IS NOT NULL AND s.died_tick IS NULL AND s.dies_tick > $3
	 ORDER BY lower(s.name)`, owner, worldID, nowTick)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := &prayerTargets{Unit: []prayerTargetUnit{}, Kind: "storm"}
	for rows.Next() {
		var id uuid.UUID
		var name, q, r string
		var seen int
		if err := rows.Scan(&id, &name, &seen, &q, &r); err != nil {
			return nil, err
		}
		out.Storms = append(out.Storms, prayerTargetStorm{ID: id.String(), Name: name, Where: fmt.Sprintf("last seen near (%s,%s)", q, r)})
	}
	return out, rows.Err()
}
