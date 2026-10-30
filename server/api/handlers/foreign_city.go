package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/hexgrid"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/religion"
	"formatet/megaron/server/internal/settlement"
	"formatet/megaron/server/internal/tick"
)

// Another ruler's city as the viewer knows it (megaron_kontrakt_frammande_stad,
// Timothy 2026-10-30). Three levels: unknown (the same 404 as an id that does
// not exist), seen (live sight now, or the S0 memory row with its date) and
// contact (the snapshot economy.RecordContact froze when something of the
// viewer's own reached the city). The body is built from a WHITELIST: a field
// added to the owner's body stays hidden here until someone adds it below.

type contactSnapshot struct {
	Tick      int
	Source    string
	Name      string
	OwnerID   *uuid.UUID
	OwnerName string
	Size      string
	Food      string
	Goods     []economy.ContactGood
	Buildings []economy.ContactBuilding
}

func loadContactSnapshot(ctx context.Context, db *pgxpool.Pool, worldID, playerID, settlementID uuid.UUID) (*contactSnapshot, error) {
	var c contactSnapshot
	var goods, buildings []byte
	err := db.QueryRow(ctx,
		`SELECT contact_tick, source, name, owner_id, owner_name, size, food, goods, buildings
		   FROM player_contacted_settlements
		  WHERE world_id = $1 AND player_id = $2 AND settlement_id = $3`,
		worldID, playerID, settlementID,
	).Scan(&c.Tick, &c.Source, &c.Name, &c.OwnerID, &c.OwnerName, &c.Size, &c.Food, &goods, &buildings)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(goods, &c.Goods); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(buildings, &c.Buildings); err != nil {
		return nil, err
	}
	return &c, nil
}

func loadSeenSettlement(ctx context.Context, db *pgxpool.Pool, worldID, playerID, settlementID uuid.UUID) (*seenCity, error) {
	var c seenCity
	err := db.QueryRow(ctx,
		`SELECT name, owner_id, owner_name, is_capital, state, wall_level, population, seen_tick
		   FROM player_seen_settlements WHERE world_id = $1 AND player_id = $2 AND settlement_id = $3`,
		worldID, playerID, settlementID,
	).Scan(&c.Name, &c.OwnerID, &c.OwnerName, &c.IsCapital, &c.State, &c.Walls, &c.Population, &c.SeenTick)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// foreignCityView returns the whitelisted body for viewer, or nil when the
// viewer knows nothing of the city (the caller answers exactly as for an id
// that does not exist). knownHex reports whether a hex is in the viewer's
// live sight or memory; deposits are only shown on such hexes.
func (h *ProvinceHandler) foreignCityView(ctx context.Context, worldID, viewer uuid.UUID, prov *province.Province, sett *settlement.Settlement) (map[string]any, func(q, r int, terrain string) bool, error) {
	now := h.clk.Now()
	eyes := loadLiveEyes(ctx, h.pool, worldID, viewer, now)
	remembered := loadRememberedTiles(ctx, h.pool, worldID, viewer)
	knownHex := func(q, r int, terrain string) bool {
		return knownToPlayer(eyes, remembered, province.MapPosition{Q: q, R: r}, terrain)
	}
	pos := province.MapPosition{Q: prov.MapTile.Q, R: prov.MapTile.R}
	live := province.AnyEyeSees(eyes, pos, prov.TerrainType)
	seen, err := loadSeenSettlement(ctx, h.pool, worldID, viewer, sett.ID)
	if err != nil {
		return nil, nil, err
	}
	if seen != nil && seen.State == "sunk" {
		seen = nil
	}
	contact, err := loadContactSnapshot(ctx, h.pool, worldID, viewer, sett.ID)
	if err != nil {
		return nil, nil, err
	}
	if !live && seen == nil && contact == nil {
		return nil, nil, nil
	}
	anchor, err := tick.LoadAnchor(ctx, h.pool, worldID)
	if err != nil {
		return nil, nil, err
	}

	body := map[string]any{"id": sett.ID}
	intel := map[string]any{"level": "seen"}
	switch {
	case live:
		var ownerName string
		if sett.OwnerID != nil {
			_ = h.pool.QueryRow(ctx, `SELECT COALESCE(wanax_name, username, '') FROM players WHERE id = $1`, *sett.OwnerID).Scan(&ownerName)
		}
		body["name"], body["owner_id"], body["owner_name"] = sett.Name, sett.OwnerID, ownerName
		body["is_capital"], body["state"], body["walls"] = sett.IsCapital, sett.State, sett.WallLevel
		body["size"] = province.SizeWord(sett.Population)
		body["culture"] = sett.CultureID
		// A siege is seen only while the city is in sight.
		body["besieged"] = sett.Besieged
		besiegedBy := []map[string]any{}
		if sett.Besieged && sett.OwnerID != nil {
			if bs, err := economy.LoadBesiegers(ctx, h.pool, worldID, *sett.OwnerID, hexgrid.Coord{Q: pos.Q, R: pos.R}); err == nil {
				for _, b := range bs {
					besiegedBy = append(besiegedBy, map[string]any{"owner_name": b.OwnerName, "unit_type": b.UnitType, "size": b.Size, "q": b.Q, "r": b.R})
				}
			}
		}
		body["besieged_by"] = besiegedBy
		types, err := visibleBuildings(ctx, h, sett.ID)
		if err != nil {
			return nil, nil, err
		}
		body["buildings"] = types
		intel["seen_at"], intel["seen_live"] = now, true
	case seen != nil && (contact == nil || seen.SeenTick >= contact.Tick):
		body["name"], body["owner_id"], body["owner_name"] = seen.Name, seen.OwnerID, seen.OwnerName
		body["is_capital"], body["state"], body["walls"] = seen.IsCapital, seen.State, seen.Walls
		body["size"] = province.SizeWord(seen.Population)
		body["culture"] = sett.CultureID
		intel["seen_at"], _ = lastSeen(seen.SeenTick, anchor)
	default:
		body["name"], body["owner_id"], body["owner_name"] = contact.Name, contact.OwnerID, contact.OwnerName
		body["size"] = contact.Size
		body["culture"] = sett.CultureID
		if seen != nil {
			body["is_capital"], body["state"], body["walls"] = seen.IsCapital, seen.State, seen.Walls
			intel["seen_at"], _ = lastSeen(seen.SeenTick, anchor)
		}
	}
	if contact != nil {
		at, _ := lastSeen(contact.Tick, anchor)
		buildings := make([]map[string]any, 0, len(contact.Buildings))
		for _, b := range contact.Buildings {
			buildings = append(buildings, buildingWord(b.Type, b.GodID, &b.Level))
		}
		intel["level"], intel["contact_at"], intel["contact_source"] = "contact", at, contact.Source
		intel["contact"] = map[string]any{
			"name": contact.Name, "owner_id": contact.OwnerID, "owner_name": contact.OwnerName,
			"size": contact.Size, "food": contact.Food, "goods": contact.Goods, "buildings": buildings,
		}
	}
	body["intel"] = intel
	if board := marketBoardView(ctx, h.pool, worldID, sett.ID, viewer, false); board != nil {
		body["market_board"] = board
	}
	return body, knownHex, nil
}

// visibleBuildings: what stands in a city, seen from outside — the type
// (a temple names its god), never the level.
func visibleBuildings(ctx context.Context, h *ProvinceHandler, settlementID uuid.UUID) ([]map[string]any, error) {
	rows, err := h.pool.Query(ctx,
		`SELECT DISTINCT building_type, COALESCE(god_id, '') FROM buildings
		  WHERE settlement_id = $1 AND level >= 1 AND hex_q IS NULL ORDER BY 1, 2`, settlementID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var typ, god string
		if err := rows.Scan(&typ, &god); err != nil {
			return nil, err
		}
		out = append(out, buildingWord(typ, god, nil))
	}
	return out, rows.Err()
}

func buildingWord(typ, god string, level *int) map[string]any {
	b := map[string]any{"type": typ, "name": province.BuildingDisplayName(typ)}
	if god != "" {
		b["god_id"], b["name"] = god, religion.TempleName(god)
	}
	if level != nil {
		b["level"] = *level
	}
	return b
}

// isOwnSettlement: the caller rules settlementID.
func (h *ProvinceHandler) isOwnSettlement(r *http.Request, settlementID uuid.UUID) bool {
	viewer, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		return false
	}
	var own bool
	_ = h.pool.QueryRow(r.Context(), `SELECT owner_id = $2 FROM settlements WHERE id = $1`, settlementID, viewer).Scan(&own)
	return own
}

// cityInLiveSight: the province's hex is in the caller's live sight now.
func (h *ProvinceHandler) cityInLiveSight(r *http.Request, worldID, provinceID uuid.UUID) bool {
	viewer, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		return false
	}
	prov, err := loadTerrainProvince(r.Context(), h.pool, provinceID, worldID)
	if err != nil {
		return false
	}
	eyes := loadLiveEyes(r.Context(), h.pool, worldID, viewer, h.clk.Now())
	return province.AnyEyeSees(eyes, province.MapPosition{Q: prov.MapTile.Q, R: prov.MapTile.R}, prov.TerrainType)
}
