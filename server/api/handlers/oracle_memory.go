package handlers

import (
	"context"
	"fmt"
	"math/rand"

	"formatet/megaron/server/internal/hexgrid"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/religion"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// applyOracleMemory is a new effect, leaving legacy oracle outcomes unchanged.
// It adds only unknown terrain to memory and never creates an eye or activity.
func (h *SettlementHandler) applyOracleMemory(ctx context.Context, tx pgx.Tx, worldID, playerID uuid.UUID, spec religion.PrayerSpec) (map[string]any, string, error) {
	eyes := loadLiveEyes(ctx, h.pool, worldID, playerID, h.clk.Now())
	remembered := loadRememberedTiles(ctx, h.pool, worldID, playerID)
	graph, err := province.LoadTileGraph(ctx, tx, worldID)
	if err != nil {
		return nil, "", err
	}
	known := func(p province.MapPosition) bool {
		return remembered[[2]int{p.Q, p.R}] || province.AnyEyeSees(eyes, p, graph[[2]int{p.Q, p.R}])
	}
	rows, err := tx.Query(ctx, `SELECT q,r,CASE WHEN copper_deposit THEN 'copper' WHEN tin_deposit THEN 'tin' ELSE 'silver' END FROM map_tiles WHERE world_id=$1 AND (copper_deposit OR tin_deposit OR COALESCE(silver_deposit,false)) ORDER BY q,r`, worldID)
	if err != nil {
		return nil, "", err
	}
	type oreHex struct {
		p   province.MapPosition
		ore string
	}
	candidates := []oreHex{}
	for rows.Next() {
		var c oreHex
		if err = rows.Scan(&c.p.Q, &c.p.R, &c.ore); err != nil {
			rows.Close()
			return nil, "", err
		}
		if !known(c.p) {
			candidates = append(candidates, c)
		}
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, "", err
	}
	if len(candidates) == 0 {
		return map[string]any{"revealed_ore": nil, "revealed_tiles": []any{}}, "Potnia finds no unseen vein of ore to mark on your map.", nil
	}
	c := candidates[rand.Intn(len(candidates))]
	revealed := []map[string]any{}
	for _, pos := range hexgrid.Disk(hexgrid.Coord{Q: c.p.Q, R: c.p.R}, 2) {
		p := province.MapPosition{Q: pos.Q, R: pos.R}
		if _, exists := graph[[2]int{p.Q, p.R}]; !exists || known(p) {
			continue
		}
		tag, err := tx.Exec(ctx, `INSERT INTO player_scouted_tiles(world_id,player_id,q,r) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, worldID, playerID, p.Q, p.R)
		if err != nil {
			return nil, "", err
		}
		if tag.RowsAffected() > 0 {
			revealed = append(revealed, map[string]any{"q": p.Q, "r": p.R})
		}
	}
	return map[string]any{"revealed_ore": map[string]any{"q": c.p.Q, "r": c.p.R, "ore": c.ore}, "revealed_tiles": revealed, "tier": "remembered"}, fmt.Sprintf("Potnia marks a vein of %s at (%d,%d) and the unknown ground around it as memory — no living eyes are there.", c.ore, c.p.Q, c.p.R), nil
}
