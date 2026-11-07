package handlers

// Fågeloraklet (bönesystemet v3 steg 5d): Asterion's live sight over one NAMED place of your own map
// for one game day (province.BirdOracleTicks), radius province.BirdOracleRadius. The place is a city
// whose name you know (your own, or a foreign one in your city memory) or a named geographic place
// whose anchor hex you know. Never a hex without a name, never a place you do not know. The eye is a
// row in oracle_sights read by province.LoadLiveEyes; what it sees is written as memory at the cast and
// again by the closing sweep. The watched are told nothing.

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/tick"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type prayerTargetPlace struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Where string `json:"where"`
}

// birdOracleTarget is a resolved, validated target.
type birdOracleTarget struct {
	Kind string // "city" or the place kind (island, cape, ...)
	Name string
	Pos  province.MapPosition
}

type oracleCity struct {
	id    uuid.UUID
	name  string
	pos   province.MapPosition
	where string
}

// birdOracleCities lists the cities the Wanax may name: their own, and every foreign one in the city memory.
func birdOracleCities(ctx context.Context, db province.Queryer, worldID, playerID uuid.UUID) ([]oracleCity, error) {
	rows, err := db.Query(ctx,
		`SELECT s.id, s.name, p.map_q, p.map_r, 'your city' FROM settlements s JOIN provinces p ON p.id = s.province_id
		  WHERE s.world_id = $1 AND s.owner_id = $2 AND s.state <> 'sunk'
		 UNION
		 SELECT pss.settlement_id, pss.name, pss.q, pss.r, 'held by ' || pss.owner_name
		   FROM player_seen_settlements pss JOIN settlements s ON s.id = pss.settlement_id
		  WHERE pss.world_id = $1 AND pss.player_id = $2 AND s.state <> 'sunk' AND s.owner_id IS DISTINCT FROM $2
		 ORDER BY 2, 1`, worldID, playerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []oracleCity
	for rows.Next() {
		var c oracleCity
		if err := rows.Scan(&c.id, &c.name, &c.pos.Q, &c.pos.R, &c.where); err != nil {
			return nil, err
		}
		c.where = fmt.Sprintf("%s, near (%d,%d)", c.where, c.pos.Q, c.pos.R)
		out = append(out, c)
	}
	return out, rows.Err()
}

type oraclePlace struct {
	kind, name string
	pos        province.MapPosition
}

// birdOraclePlaces lists the named geographic places whose anchor hex the Wanax knows (live or memory).
func birdOraclePlaces(ctx context.Context, pool *pgxpool.Pool, clk clock.Clock, worldID, playerID uuid.UUID) ([]oraclePlace, error) {
	rows, err := pool.Query(ctx,
		`SELECT n.kind, n.name, n.anchor_q, n.anchor_r, COALESCE(t.terrain, '')
		   FROM place_names n LEFT JOIN map_tiles t ON t.world_id = n.world_id AND t.q = n.anchor_q AND t.r = n.anchor_r
		  WHERE n.world_id = $1 AND n.name IS NOT NULL ORDER BY lower(n.name)`, worldID)
	if err != nil {
		return nil, err
	}
	type cand struct {
		oraclePlace
		terrain string
	}
	var cands []cand
	for rows.Next() {
		var c cand
		if err := rows.Scan(&c.kind, &c.name, &c.pos.Q, &c.pos.R, &c.terrain); err != nil {
			rows.Close()
			return nil, err
		}
		cands = append(cands, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil || len(cands) == 0 {
		return nil, err
	}
	eyes := loadLiveEyes(ctx, pool, worldID, playerID, clk.Now())
	remembered := loadRememberedTiles(ctx, pool, worldID, playerID)
	var out []oraclePlace
	for _, c := range cands {
		if c.terrain != "" && knownToPlayer(eyes, remembered, c.pos, c.terrain) {
			out = append(out, c.oraclePlace)
		}
	}
	return out, nil
}

// checkBirdOracleTarget resolves the target inside the cast TX, before any gift is taken.
func (h *SettlementHandler) checkBirdOracleTarget(ctx context.Context, tx pgx.Tx, worldID, playerID uuid.UUID, target string) (*birdOracleTarget, int, string) {
	target = strings.TrimSpace(target)
	if target == "" {
		return nil, http.StatusBadRequest, "name the place to watch: target = the name of a city or a named place of your map"
	}
	cities, err := birdOracleCities(ctx, tx, worldID, playerID)
	if err != nil {
		return nil, http.StatusInternalServerError, "could not read your map"
	}
	var hits []oracleCity
	seen := map[uuid.UUID]bool{}
	for _, c := range cities {
		if !seen[c.id] && (c.id.String() == target || strings.EqualFold(c.name, target)) {
			seen[c.id] = true
			hits = append(hits, c)
		}
	}
	if len(hits) > 1 {
		return nil, http.StatusBadRequest, "more than one city on your map has that name: use its id"
	}
	if len(hits) == 1 {
		return &birdOracleTarget{Kind: "city", Name: hits[0].name, Pos: hits[0].pos}, 0, ""
	}
	places, err := birdOraclePlaces(ctx, h.pool, h.clk, worldID, playerID)
	if err != nil {
		return nil, http.StatusInternalServerError, "could not read your map"
	}
	for _, p := range places {
		if strings.EqualFold(p.name, target) {
			return &birdOracleTarget{Kind: p.kind, Name: p.name, Pos: p.pos}, 0, ""
		}
	}
	return nil, http.StatusNotFound, "no such named place on your map"
}

// applyBirdOracle opens the eye and writes what it sees as memory. recorded is what RiteCast keeps.
func (h *SettlementHandler) applyBirdOracle(ctx context.Context, tx pgx.Tx, worldID, playerID uuid.UUID, t *birdOracleTarget, nowTick int) (map[string]any, string, error) {
	anchor, err := tick.LoadAnchor(ctx, tx, worldID)
	if err != nil {
		return nil, "", err
	}
	from := anchor.MilliAt(h.clk.Now())
	until := from + int64(province.BirdOracleTicks)*1000
	if _, err = tx.Exec(ctx,
		`INSERT INTO oracle_sights (world_id, player_id, q, r, place_kind, place_name, from_milli, until_milli)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		worldID, playerID, t.Pos.Q, t.Pos.R, t.Kind, t.Name, from, until); err != nil {
		return nil, "", err
	}
	if err = province.SweepLiveRadius(ctx, tx, worldID, playerID, t.Pos, province.EyeOracle); err != nil {
		return nil, "", err
	}
	recorded := map[string]any{"place_kind": t.Kind, "place_name": t.Name, "q": t.Pos.Q, "r": t.Pos.R,
		"radius": province.BirdOracleRadius, "until_tick": nowTick + province.BirdOracleTicks}
	msg := fmt.Sprintf("Asterion's birds go out over %s. For %s you see what stands within %d hexes of it, as if your own people were there. Afterwards it is memory.",
		t.Name, tick.FormatDays(province.BirdOracleTicks), province.BirdOracleRadius)
	return recorded, msg, nil
}

// scheduleBirdOracleClose queues the closing memory sweep for the day after the cast.
func (h *SettlementHandler) scheduleBirdOracleClose(ctx context.Context, tx pgx.Tx, worldID, playerID uuid.UUID, t *birdOracleTarget, nowTick int) error {
	return h.scheduler.EnqueueTickTx(ctx, tx, worldID, events.ScheduledBirdOracleClose,
		combat.BirdOracleClosePayload{WorldID: worldID, PlayerID: playerID, Q: t.Pos.Q, R: t.Pos.R}, nowTick+province.BirdOracleTicks)
}

// loadBirdOracleTargets lists what the Wanax may name: cities (own and remembered) and known named places.
func (h *ProvinceHandler) loadBirdOracleTargets(ctx context.Context, owner, worldID uuid.UUID) (*prayerTargets, error) {
	cities, err := birdOracleCities(ctx, h.pool, worldID, owner)
	if err != nil {
		return nil, err
	}
	places, err := birdOraclePlaces(ctx, h.pool, h.clk, worldID, owner)
	if err != nil {
		return nil, err
	}
	out := &prayerTargets{Unit: []prayerTargetUnit{}, Kind: "place", Cities: []prayerTargetCity{}, Places: []prayerTargetPlace{}}
	for _, c := range cities {
		out.Cities = append(out.Cities, prayerTargetCity{ID: c.id.String(), Name: c.name, Where: c.where})
	}
	for _, p := range places {
		out.Places = append(out.Places, prayerTargetPlace{ID: p.name, Name: p.name, Where: fmt.Sprintf("%s, near (%d,%d)", p.kind, p.pos.Q, p.pos.R)})
	}
	return out, nil
}
