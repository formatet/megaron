package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/poleia/server/internal/auth"
	"github.com/poleia/server/internal/clock"
	"github.com/poleia/server/internal/province"
	"github.com/poleia/server/internal/world"
)

// WorldHandler handles HTTP requests for world endpoints.
type WorldHandler struct {
	pool    *pgxpool.Pool
	authSvc *auth.Service
	clk     clock.Clock
}

// NewWorldHandler creates a WorldHandler.
func NewWorldHandler(pool *pgxpool.Pool, authSvc *auth.Service, clk clock.Clock) *WorldHandler {
	return &WorldHandler{pool: pool, authSvc: authSvc, clk: clk}
}

// List handles GET /worlds.
func (h *WorldHandler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pool.Query(r.Context(),
		`SELECT id, name, state, prestige, era_number, created_at FROM worlds ORDER BY created_at DESC`,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list worlds")
		return
	}
	defer rows.Close()

	type worldSummary struct {
		ID        uuid.UUID `json:"id"`
		Name      string    `json:"name"`
		State     string    `json:"state"`
		Prestige  int       `json:"prestige"`
		EraNumber int       `json:"era_number"`
		CreatedAt time.Time `json:"created_at"`
	}
	var worlds []worldSummary
	for rows.Next() {
		var ws worldSummary
		if err := rows.Scan(&ws.ID, &ws.Name, &ws.State, &ws.Prestige, &ws.EraNumber, &ws.CreatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "scan error")
			return
		}
		worlds = append(worlds, ws)
	}
	if worlds == nil {
		worlds = []worldSummary{}
	}
	writeJSON(w, http.StatusOK, worlds)
}

// Create handles POST /worlds (admin only — validated at router level).
func (h *WorldHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name      string `json:"name"`
		MapSeed   *int64 `json:"map_seed"`
		MapWidth  int    `json:"map_width"`
		MapHeight int    `json:"map_height"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "world name required")
		return
	}
	if req.MapWidth == 0 {
		req.MapWidth = 40
	}
	if req.MapHeight == 0 {
		req.MapHeight = 30
	}

	var seed int64
	if req.MapSeed != nil {
		seed = *req.MapSeed
	} else {
		seed = h.clk.Now().UnixNano()
	}

	var id uuid.UUID
	err := h.pool.QueryRow(r.Context(),
		`INSERT INTO worlds (name, map_seed, map_width, map_height)
		 VALUES ($1, $2, $3, $4) RETURNING id`,
		req.Name, seed, req.MapWidth, req.MapHeight,
	).Scan(&id)
	if err != nil {
		writeError(w, http.StatusConflict, "world name already exists or DB error")
		return
	}

	tiles := world.GenerateMap(id, seed, req.MapWidth, req.MapHeight)
	if err := h.storeTiles(r.Context(), id, tiles); err != nil {
		writeError(w, http.StatusInternalServerError, "could not store map tiles")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "map_seed": seed})
}

// Get handles GET /worlds/:worldID.
func (h *WorldHandler) Get(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}

	var wld world.World
	err = h.pool.QueryRow(r.Context(),
		`SELECT id, name, state, prestige, era_number, era_started_at,
		        max_provinces, min_era_weeks, max_era_weeks,
		        kingdom_min_size, kingdom_max_size, map_seed, map_width, map_height, created_at
		 FROM worlds WHERE id = $1`,
		worldID,
	).Scan(
		&wld.ID, &wld.Name, &wld.State, &wld.Prestige, &wld.EraNumber, &wld.EraStartedAt,
		&wld.MaxProvinces, &wld.MinEraWeeks, &wld.MaxEraWeeks,
		&wld.KingdomMinSize, &wld.KingdomMaxSize, &wld.MapSeed, &wld.MapWidth, &wld.MapHeight,
		&wld.CreatedAt,
	)
	if err != nil {
		writeError(w, http.StatusNotFound, "world not found")
		return
	}

	var activeWars int
	_ = h.pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM marching_armies WHERE world_id = $1 AND resolved = false AND intent = 'attack'`,
		worldID,
	).Scan(&activeWars)

	collapse := world.ComputeCollapse(&wld, activeWars, h.clk.Now())
	writeJSON(w, http.StatusOK, map[string]any{
		"id":             wld.ID,
		"name":           wld.Name,
		"state":          wld.State,
		"prestige":       wld.Prestige,
		"era_number":     wld.EraNumber,
		"era_started_at": wld.EraStartedAt,
		"collapse":       collapse,
		"created_at":     wld.CreatedAt,
	})
}

// Map handles GET /worlds/:worldID/map with fog-of-war filtering.
func (h *WorldHandler) Map(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}

	playerID, authenticated := auth.PlayerIDFromContext(r.Context())

	rows, err := h.pool.Query(r.Context(),
		`SELECT q, r, terrain, fertility, mineral, copper_deposit, tin_deposit FROM map_tiles WHERE world_id = $1`,
		worldID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load map")
		return
	}
	defer rows.Close()

	var origins []province.MapPosition
	if authenticated {
		origins = h.visibleOrigins(r.Context(), worldID, playerID)
	}

	type tileView struct {
		Q             int     `json:"q"`
		R             int     `json:"r"`
		Terrain       string  `json:"terrain"`
		Visible       bool    `json:"visible"`
		F             float64 `json:"fertility,omitempty"`
		M             float64 `json:"mineral,omitempty"`
		CopperDeposit bool    `json:"copper_deposit,omitempty"`
		TinDeposit    bool    `json:"tin_deposit,omitempty"`
	}
	var tiles []tileView
	for rows.Next() {
		var t tileView
		if err := rows.Scan(&t.Q, &t.R, &t.Terrain, &t.F, &t.M, &t.CopperDeposit, &t.TinDeposit); err != nil {
			continue
		}
		pos := province.MapPosition{Q: t.Q, R: t.R}
		t.Visible = !authenticated || province.VisibleFrom(pos, origins, 5)
		if !t.Visible {
			t.F = 0
			t.M = 0
			t.CopperDeposit = false
			t.TinDeposit = false
			t.Terrain = "fog"
		}
		tiles = append(tiles, t)
	}
	if tiles == nil {
		tiles = []tileView{}
	}
	writeJSON(w, http.StatusOK, tiles)
}

// Provinces handles GET /worlds/:worldID/provinces — returns all province markers for the map.
// Fog-filtered: unauthenticated players see territory markers only; authenticated players
// see their own and allied provinces with full detail.
func (h *WorldHandler) Provinces(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}

	playerID, authenticated := auth.PlayerIDFromContext(r.Context())

	var origins []province.MapPosition
	var ownProvinceID uuid.UUID
	var ownKingdomID *uuid.UUID

	if authenticated {
		origins = h.visibleOrigins(r.Context(), worldID, playerID)
		// Get player's own province tile ID and kingdom for "own" marker.
		_ = h.pool.QueryRow(r.Context(),
			`SELECT s.province_id, s.kingdom_id
			 FROM settlements s
			 WHERE s.world_id = $1 AND s.owner_id = $2 AND s.is_capital = true`,
			worldID, playerID,
		).Scan(&ownProvinceID, &ownKingdomID)
	}

	rows, err := h.pool.Query(r.Context(),
		`SELECT p.id, s.id, s.name, s.culture_id, s.kingdom_id, p.map_q, p.map_r,
		        s.state, s.wall_level, pl.username, COALESCE(k.name, '')
		 FROM provinces p
		 JOIN settlements s ON s.province_id = p.id
		 LEFT JOIN players pl ON pl.id = s.owner_id
		 LEFT JOIN kingdoms k ON k.id = s.kingdom_id
		 WHERE p.world_id = $1`,
		worldID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load provinces")
		return
	}
	defer rows.Close()

	type provinceMarker struct {
		ID           uuid.UUID  `json:"id"`
		SettlementID uuid.UUID  `json:"settlement_id"`
		Name         string     `json:"name"`
		Culture      string     `json:"culture"`
		KingdomID    *uuid.UUID `json:"kingdom_id,omitempty"`
		KingdomName  string     `json:"kingdom_name,omitempty"`
		Q            int        `json:"q"`
		R            int        `json:"r"`
		State        string     `json:"state"`
		Walls        int        `json:"walls"`
		Owner        string     `json:"owner,omitempty"`
		Own          bool       `json:"own"`
		Allied       bool       `json:"allied"`
		Visible      bool       `json:"visible"`
	}
	var markers []provinceMarker
	for rows.Next() {
		var m provinceMarker
		if err := rows.Scan(&m.ID, &m.SettlementID, &m.Name, &m.Culture, &m.KingdomID, &m.Q, &m.R, &m.State, &m.Walls, &m.Owner, &m.KingdomName); err != nil {
			continue
		}
		pos := province.MapPosition{Q: m.Q, R: m.R}
		m.Visible = !authenticated || province.VisibleFrom(pos, origins, 5)
		if authenticated {
			m.Own = m.ID == ownProvinceID
			m.Allied = ownKingdomID != nil && m.KingdomID != nil && *ownKingdomID == *m.KingdomID
		}
		if !m.Visible {
			continue // don't reveal fog tiles
		}
		markers = append(markers, m)
	}
	if markers == nil {
		markers = []provinceMarker{}
	}
	writeJSON(w, http.StatusOK, markers)
}

// visibleOrigins loads the map positions of the player's province and all allied
// kingdom member provinces. These are the "eyes" used for fog-of-war calculation.
func (h *WorldHandler) visibleOrigins(ctx context.Context, worldID, playerID uuid.UUID) []province.MapPosition {
	// Provinces the player can see: own settlements and allied kingdom settlements.
	rows, err := h.pool.Query(ctx,
		`SELECT p.map_q, p.map_r
		 FROM provinces p
		 JOIN settlements s ON s.province_id = p.id
		 WHERE p.world_id = $1 AND (
		     s.owner_id = $2
		     OR (s.kingdom_id IS NOT NULL AND s.kingdom_id IN (
		         SELECT km.kingdom_id FROM kingdom_members km
		         WHERE km.player_id = $2
		     ))
		 )`,
		worldID, playerID,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var origins []province.MapPosition
	for rows.Next() {
		var pos province.MapPosition
		if err := rows.Scan(&pos.Q, &pos.R); err == nil {
			origins = append(origins, pos)
		}
	}
	return origins
}

// Marches handles GET /worlds/:worldID/marches — all unresolved marching armies visible
// to the player. Used by the map renderer to draw animated walkers.
func (h *WorldHandler) Marches(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}
	playerID, authenticated := auth.PlayerIDFromContext(r.Context())

	// Fog of war: only show marches originating from or heading to visible provinces.
	var visQ, visR []int
	if authenticated {
		origins := h.visibleOrigins(r.Context(), worldID, playerID)
		for _, o := range origins {
			visQ = append(visQ, o.Q)
			visR = append(visR, o.R)
		}
	}

	rows, err := h.pool.Query(r.Context(),
		`SELECT ma.id, ma.intent,
		        op.map_q, op.map_r, tp.map_q, tp.map_r,
		        ma.departs_at, ma.arrives_at
		 FROM marching_armies ma
		 JOIN provinces op ON op.id = ma.origin_id
		 JOIN provinces tp ON tp.id = ma.target_id
		 WHERE ma.world_id = $1 AND ma.resolved = false`,
		worldID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load marches")
		return
	}
	defer rows.Close()

	type marchMarker struct {
		ID        uuid.UUID `json:"id"`
		Intent    string    `json:"intent"`
		OriginQ   int       `json:"origin_q"`
		OriginR   int       `json:"origin_r"`
		TargetQ   int       `json:"target_q"`
		TargetR   int       `json:"target_r"`
		DepartsAt time.Time `json:"departs_at"`
		ArrivesAt time.Time `json:"arrives_at"`
	}

	visible := func(q, r int) bool {
		if !authenticated {
			return false
		}
		return province.VisibleFrom(province.MapPosition{Q: q, R: r}, func() []province.MapPosition {
			ps := make([]province.MapPosition, len(visQ))
			for i := range visQ {
				ps[i] = province.MapPosition{Q: visQ[i], R: visR[i]}
			}
			return ps
		}(), 5)
	}

	var markers []marchMarker
	for rows.Next() {
		var m marchMarker
		if err := rows.Scan(&m.ID, &m.Intent,
			&m.OriginQ, &m.OriginR, &m.TargetQ, &m.TargetR,
			&m.DepartsAt, &m.ArrivesAt); err != nil {
			continue
		}
		if !visible(m.OriginQ, m.OriginR) && !visible(m.TargetQ, m.TargetR) {
			continue
		}
		markers = append(markers, m)
	}
	if markers == nil {
		markers = []marchMarker{}
	}
	writeJSON(w, http.StatusOK, markers)
}

// MapMessengers handles GET /worlds/:worldID/messengers — outbound messengers visible
// to the player. Used by the map renderer to draw animated messenger walkers.
func (h *WorldHandler) MapMessengers(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}
	playerID, authenticated := auth.PlayerIDFromContext(r.Context())

	var origins []province.MapPosition
	if authenticated {
		origins = h.visibleOrigins(r.Context(), worldID, playerID)
	}

	rows, err := h.pool.Query(r.Context(),
		`SELECT m.id, op.map_q, op.map_r, dp.map_q, dp.map_r, m.sent_at, m.arrives_at
		 FROM messengers m
		 JOIN settlements os ON os.id = m.origin_id
		 JOIN provinces op ON op.id = os.province_id
		 JOIN settlements ds ON ds.id = m.destination_id
		 JOIN provinces dp ON dp.id = ds.province_id
		 WHERE m.world_id = $1 AND m.status = 'outbound'`,
		worldID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load messengers")
		return
	}
	defer rows.Close()

	type messengerMarker struct {
		ID       uuid.UUID `json:"id"`
		OriginQ  int       `json:"origin_q"`
		OriginR  int       `json:"origin_r"`
		DestQ    int       `json:"dest_q"`
		DestR    int       `json:"dest_r"`
		SentAt   time.Time `json:"sent_at"`
		ArrivesAt time.Time `json:"arrives_at"`
	}

	var markers []messengerMarker
	for rows.Next() {
		var m messengerMarker
		if err := rows.Scan(&m.ID, &m.OriginQ, &m.OriginR, &m.DestQ, &m.DestR,
			&m.SentAt, &m.ArrivesAt); err != nil {
			continue
		}
		if authenticated && !province.VisibleFrom(province.MapPosition{Q: m.OriginQ, R: m.OriginR}, origins, 5) &&
			!province.VisibleFrom(province.MapPosition{Q: m.DestQ, R: m.DestR}, origins, 5) {
			continue
		}
		markers = append(markers, m)
	}
	if markers == nil {
		markers = []messengerMarker{}
	}
	writeJSON(w, http.StatusOK, markers)
}

func (h *WorldHandler) storeTiles(ctx context.Context, worldID uuid.UUID, tiles []world.MapTile) error {
	batch := &pgx.Batch{}
	for _, t := range tiles {
		batch.Queue(
			`INSERT INTO map_tiles (world_id, q, r, terrain, fertility, mineral, copper_deposit, tin_deposit)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			 ON CONFLICT (world_id, q, r) DO NOTHING`,
			worldID, t.Q, t.R, string(t.Terrain), t.Fertility, t.Mineral, t.CopperDeposit, t.TinDeposit,
		)
	}
	br := h.pool.SendBatch(ctx, batch)
	return br.Close()
}
