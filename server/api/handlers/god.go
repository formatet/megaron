package handlers

import (
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// GodHandler provides admin-only, FOW-free world views for debugging and playtesting.
type GodHandler struct {
	pool *pgxpool.Pool
}

// NewGodHandler creates a GodHandler.
func NewGodHandler(pool *pgxpool.Pool) *GodHandler {
	return &GodHandler{pool: pool}
}

// requireAdminKey checks the X-Admin-Key header against the POLEIA_ADMIN_KEY env var.
// Returns true and proceeds if the key matches; otherwise writes 403 and returns false.
func requireAdminKey(w http.ResponseWriter, r *http.Request) bool {
	secret := os.Getenv("POLEIA_ADMIN_KEY")
	if secret == "" || r.Header.Get("X-Admin-Key") != secret {
		writeError(w, http.StatusForbidden, "admin key required")
		return false
	}
	return true
}

// View handles GET /admin/worlds/{worldID}/god-view.
// Returns the full map + all settlements without any FOW filtering.
// Requires X-Admin-Key header matching POLEIA_ADMIN_KEY env var.
func (h *GodHandler) View(w http.ResponseWriter, r *http.Request) {
	if !requireAdminKey(w, r) {
		return
	}
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}

	// Full map — no FOW.
	tileRows, err := h.pool.Query(r.Context(),
		`SELECT q, r, terrain, fertility, mineral,
		        copper_deposit, tin_deposit,
		        COALESCE(cedar_deposit,false), COALESCE(silver_deposit,false),
		        COALESCE(coastal,false)
		 FROM map_tiles WHERE world_id = $1 ORDER BY r, q`,
		worldID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load map")
		return
	}
	defer tileRows.Close()

	type tileView struct {
		Q             int     `json:"q"`
		R             int     `json:"r"`
		Terrain       string  `json:"terrain"`
		Fertility     float64 `json:"fertility,omitempty"`
		Mineral       float64 `json:"mineral,omitempty"`
		CopperDeposit bool    `json:"copper_deposit,omitempty"`
		TinDeposit    bool    `json:"tin_deposit,omitempty"`
		CedarDeposit  bool    `json:"cedar_deposit,omitempty"`
		SilverDeposit bool    `json:"silver_deposit,omitempty"`
		Coastal       bool    `json:"coastal,omitempty"`
	}
	var tiles []tileView
	for tileRows.Next() {
		var t tileView
		if err := tileRows.Scan(&t.Q, &t.R, &t.Terrain, &t.Fertility, &t.Mineral,
			&t.CopperDeposit, &t.TinDeposit, &t.CedarDeposit, &t.SilverDeposit, &t.Coastal); err != nil {
			continue
		}
		tiles = append(tiles, t)
	}
	if tiles == nil {
		tiles = []tileView{}
	}

	// All settlements — full data, no FOW.
	settRows, err := h.pool.Query(r.Context(),
		`SELECT s.id, s.name, s.state, s.culture_id,
		        p.map_q, p.map_r, p.terrain_type,
		        COALESCE(pl.username, ''), s.owner_id,
		        s.population, s.wall_level,
		        s.infantry + s.elite_infantry + s.chariot + s.ship + s.war_galley + s.merchantman AS army_total,
		        COALESCE(k.name, ''),
		        s.battle_frenzy_until,
		        COALESCE((SELECT GREATEST(0, settled(kharis_amount, kharis_rate, kharis_calc_at))
		                  FROM player_world_records pwr
		                  WHERE pwr.player_id = s.owner_id AND pwr.world_id = s.world_id), 0)
		 FROM settlements s
		 LEFT JOIN provinces p ON p.id = s.province_id
		 LEFT JOIN players pl ON pl.id = s.owner_id
		 LEFT JOIN kingdoms k ON k.id = s.kingdom_id
		 WHERE s.world_id = $1 AND s.state != 'sunk'
		 ORDER BY s.name`,
		worldID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load settlements")
		return
	}
	defer settRows.Close()

	type settlementView struct {
		ID          string   `json:"id"`
		Name        string   `json:"name"`
		State       string   `json:"state"`
		Culture     string   `json:"culture"`
		Q           *int     `json:"q"`
		R           *int     `json:"r"`
		Terrain     *string  `json:"terrain"`
		Owner       string   `json:"owner"`
		OwnerID     *string  `json:"owner_id"`
		Population  float64  `json:"population"`
		WallLevel   int      `json:"wall_level"`
		ArmyTotal   int      `json:"army_total"`
		Kingdom     string   `json:"kingdom,omitempty"`
		Frenzied    bool     `json:"frenzied,omitempty"`
		Kharis      float64  `json:"kharis"`
	}

	var settlements []settlementView
	for settRows.Next() {
		var s settlementView
		var ownerID *uuid.UUID
		var frenzyUntil *time.Time
		var q, r *int
		var terrain *string
		var kharis float64
		if err := settRows.Scan(
			&s.ID, &s.Name, &s.State, &s.Culture,
			&q, &r, &terrain,
			&s.Owner, &ownerID,
			&s.Population, &s.WallLevel, &s.ArmyTotal,
			&s.Kingdom, &frenzyUntil, &kharis,
		); err != nil {
			continue
		}
		s.Q = q
		s.R = r
		s.Terrain = terrain
		if ownerID != nil {
			sid := ownerID.String()
			s.OwnerID = &sid
		}
		s.Kharis = kharis
		s.Frenzied = frenzyUntil != nil
		settlements = append(settlements, s)
	}
	if settlements == nil {
		settlements = []settlementView{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"world_id":    worldID,
		"tiles":       tiles,
		"settlements": settlements,
	})
}
