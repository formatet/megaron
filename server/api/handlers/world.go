package handlers

import (
	"context"
	"encoding/json"
	"math/rand"
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

// worldNamePool — Egyptian primordial / Zep Tepi ("the first time") names. A reseed
// with no explicit name draws from here so worlds read as myth, not UUID. Recurrence
// is dressed as a dynasty ("Nun II") since worlds.name is UNIQUE.
var worldNamePool = []string{
	"Nun", "Atum", "Benben", "Zep-Tepi", "Khepri",
	"Naunet", "Heh", "Hauhet", "Kek", "Kauket", "Amun", "Amaunet",
}

// pickWorldName returns an unused mythic world name, avoiding the 3 most recently
// created worlds (no repeats in a row) and appending a dynastic numeral on collision.
func (h *WorldHandler) pickWorldName(ctx context.Context) (string, error) {
	rows, err := h.pool.Query(ctx, `SELECT name FROM worlds ORDER BY created_at DESC`)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	taken := map[string]bool{}
	var recent []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return "", err
		}
		taken[n] = true
		if len(recent) < 3 {
			recent = append(recent, n)
		}
	}
	if err := rows.Err(); err != nil {
		return "", err
	}

	recentBase := map[string]bool{}
	for _, n := range recent {
		recentBase[n] = true
	}
	candidates := make([]string, 0, len(worldNamePool))
	for _, n := range worldNamePool {
		if !recentBase[n] {
			candidates = append(candidates, n)
		}
	}
	if len(candidates) == 0 {
		candidates = worldNamePool
	}

	rng := rand.New(rand.NewSource(h.clk.Now().UnixNano()))
	base := candidates[rng.Intn(len(candidates))]
	if !taken[base] {
		return base, nil
	}
	for i := 2; ; i++ {
		cand := base + " " + roman(i)
		if !taken[cand] {
			return cand, nil
		}
	}
}

// roman renders small positive integers as Roman numerals (dynasty suffixes).
func roman(n int) string {
	vals := []struct {
		v int
		s string
	}{{1000, "M"}, {900, "CM"}, {500, "D"}, {400, "CD"}, {100, "C"}, {90, "XC"},
		{50, "L"}, {40, "XL"}, {10, "X"}, {9, "IX"}, {5, "V"}, {4, "IV"}, {1, "I"}}
	out := ""
	for _, p := range vals {
		for n >= p.v {
			out += p.s
			n -= p.v
		}
	}
	return out
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
		name, err := h.pickWorldName(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not assign world name")
			return
		}
		req.Name = name
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

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not begin transaction")
		return
	}
	defer tx.Rollback(r.Context()) //nolint:errcheck

	// Archive any currently active world before inserting the new one, and
	// cascade to its player records so a Wanax is only ever 'active' in the one
	// live world (single-world enforcement — otherwise stale records accumulate
	// across reseeds and clients can't tell which world they belong to).
	if _, err := tx.Exec(r.Context(),
		`UPDATE player_world_records SET status = 'archived'
		 WHERE status = 'active'
		   AND world_id IN (SELECT id FROM worlds WHERE status = 'active')`); err != nil {
		writeError(w, http.StatusInternalServerError, "could not archive player records")
		return
	}
	if _, err := tx.Exec(r.Context(),
		`UPDATE worlds SET status = 'archived' WHERE status = 'active'`); err != nil {
		writeError(w, http.StatusInternalServerError, "could not archive active world")
		return
	}

	var id uuid.UUID
	if err := tx.QueryRow(r.Context(),
		`INSERT INTO worlds (name, map_seed, map_width, map_height)
		 VALUES ($1, $2, $3, $4) RETURNING id`,
		req.Name, seed, req.MapWidth, req.MapHeight,
	).Scan(&id); err != nil {
		writeError(w, http.StatusConflict, "world name already exists or DB error")
		return
	}

	tiles, effSeed := world.GenerateMap(id, seed, req.MapWidth, req.MapHeight)

	// GenerateMap may reseed to satisfy map invariants — persist the seed that
	// actually produced the stored map so it stays reproducible.
	if effSeed != seed {
		if _, err := tx.Exec(r.Context(),
			`UPDATE worlds SET map_seed = $1 WHERE id = $2`, effSeed, id); err != nil {
			writeError(w, http.StatusInternalServerError, "could not persist effective map seed")
			return
		}
	}

	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "could not commit world creation")
		return
	}

	if err := h.storeTiles(r.Context(), id, tiles); err != nil {
		writeError(w, http.StatusInternalServerError, "could not store map tiles")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "map_seed": effSeed})
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
		        kingdom_min_size, kingdom_max_size, map_seed, map_width, map_height, created_at,
		        current_tick
		 FROM worlds WHERE id = $1`,
		worldID,
	).Scan(
		&wld.ID, &wld.Name, &wld.State, &wld.Prestige, &wld.EraNumber, &wld.EraStartedAt,
		&wld.MaxProvinces, &wld.MinEraWeeks, &wld.MaxEraWeeks,
		&wld.KingdomMinSize, &wld.KingdomMaxSize, &wld.MapSeed, &wld.MapWidth, &wld.MapHeight,
		&wld.CreatedAt, &wld.CurrentTick,
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

	collapse := world.ComputeCollapse(&wld, activeWars, wld.CurrentTick)
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
		`SELECT q, r, terrain, coastal, fertility, mineral, copper_deposit, tin_deposit,
		        COALESCE(cedar_deposit,false), COALESCE(silver_deposit,false)
		 FROM map_tiles WHERE world_id = $1`,
		worldID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load map")
		return
	}
	defer rows.Close()

	var eyes []province.Eye
	var remembered map[[2]int]bool
	if authenticated {
		eyes = loadLiveEyes(r.Context(), h.pool, worldID, playerID)
		remembered = loadRememberedTiles(r.Context(), h.pool, worldID, playerID)
	}

	type tileView struct {
		Q             int     `json:"q"`
		R             int     `json:"r"`
		Terrain       string  `json:"terrain"`
		Coastal       bool    `json:"coastal,omitempty"`
		Visible       bool    `json:"visible"`
		Tier          string  `json:"tier"` // "live" | "remembered" | "fog"
		Frontier      bool    `json:"frontier,omitempty"`
		F             float64 `json:"fertility,omitempty"`
		M             float64 `json:"mineral,omitempty"`
		CopperDeposit bool    `json:"copper_deposit,omitempty"`
		TinDeposit    bool    `json:"tin_deposit,omitempty"`
		CedarDeposit  bool    `json:"cedar_deposit,omitempty"`
		SilverDeposit bool    `json:"silver_deposit,omitempty"`
	}
	var tiles []tileView
	tierByPos := map[[2]int]string{}
	var liveTiles []province.MapPosition
	for rows.Next() {
		var t tileView
		if err := rows.Scan(&t.Q, &t.R, &t.Terrain, &t.Coastal, &t.F, &t.M, &t.CopperDeposit, &t.TinDeposit,
			&t.CedarDeposit, &t.SilverDeposit); err != nil {
			continue
		}
		pos := province.MapPosition{Q: t.Q, R: t.R}
		switch {
		case !authenticated:
			t.Tier = "live"
		case province.AnyEyeSees(eyes, pos, t.Terrain):
			t.Tier = "live"
			liveTiles = append(liveTiles, pos)
		case remembered[[2]int{t.Q, t.R}]:
			t.Tier = "remembered"
		default:
			t.Tier = "fog"
		}
		t.Visible = t.Tier != "fog"
		if !t.Visible {
			t.F = 0
			t.M = 0
			t.Coastal = false
			t.CopperDeposit = false
			t.TinDeposit = false
			t.CedarDeposit = false
			t.SilverDeposit = false
			t.Terrain = "fog"
		}
		tierByPos[[2]int{t.Q, t.R}] = t.Tier
		tiles = append(tiles, t)
	}

	// Frontier: a fog tile adjacent to a known (live or remembered) tile — the edge
	// of the explored world, so the client/CLI can point the player at where to
	// explore next (temenos_synlighet.md tier 3).
	for i := range tiles {
		if tiles[i].Tier != "fog" {
			continue
		}
		for _, n := range province.HexNeighbors(province.MapPosition{Q: tiles[i].Q, R: tiles[i].R}) {
			if nt, ok := tierByPos[[2]int{n.Q, n.R}]; ok && nt != "fog" {
				tiles[i].Frontier = true
				break
			}
		}
	}

	if tiles == nil {
		tiles = []tileView{}
	}

	// Memory grows where the player currently sees: upsert this turn's live tiles
	// into player_scouted_tiles so they remain (dimmed) after the eye moves on.
	// Idempotent — ON CONFLICT DO NOTHING, safe to run on every Map read.
	if authenticated && len(liveTiles) > 0 {
		batch := &pgx.Batch{}
		for _, pos := range liveTiles {
			batch.Queue(
				`INSERT INTO player_scouted_tiles (world_id, player_id, q, r)
				 VALUES ($1, $2, $3, $4) ON CONFLICT (world_id, player_id, q, r) DO NOTHING`,
				worldID, playerID, pos.Q, pos.R,
			)
		}
		br := h.pool.SendBatch(r.Context(), batch)
		_ = br.Close()
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

	// ownSettlements: set of province_ids owned by the player.
	ownSettlements := map[uuid.UUID]bool{}
	if authenticated {
		origins = h.visibleOrigins(r.Context(), worldID, playerID)
		// Capital for kingdom check.
		_ = h.pool.QueryRow(r.Context(),
			`SELECT s.province_id, s.kingdom_id
			 FROM settlements s
			 WHERE s.world_id = $1 AND s.owner_id = $2 AND s.is_capital = true`,
			worldID, playerID,
		).Scan(&ownProvinceID, &ownKingdomID)
		// All provinces owned by player.
		sRows, _ := h.pool.Query(r.Context(),
			`SELECT province_id FROM settlements WHERE world_id = $1 AND owner_id = $2`,
			worldID, playerID,
		)
		if sRows != nil {
			for sRows.Next() {
				var pid uuid.UUID
				if sRows.Scan(&pid) == nil {
					ownSettlements[pid] = true
				}
			}
			sRows.Close()
		}
	}

	rows, err := h.pool.Query(r.Context(),
		`SELECT p.id, s.id, s.name, s.culture_id, s.kingdom_id, p.map_q, p.map_r,
		        s.state, s.wall_level, COALESCE(pl.username, ''), COALESCE(k.name, ''),
		        s.infantry + s.elite_infantry + s.chariot + s.ship + s.war_galley + s.merchantman AS army_total,
		        EXISTS (SELECT 1 FROM build_queue bq WHERE bq.settlement_id = s.id) AS build_active,
		        EXISTS (SELECT 1 FROM scheduled_events se WHERE se.event_type = 'TrainComplete'
		                AND se.processed_at IS NULL
		                AND (se.payload->>'settlement_id')::uuid = s.id) AS train_active
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
		IsCapital    bool       `json:"is_capital"`
		Allied       bool       `json:"allied"`
		Visible      bool       `json:"visible"`
		IsOutpost    bool       `json:"is_outpost,omitempty"`
		ArmyTotal    int        `json:"army_total,omitempty"`
		BuildActive  bool       `json:"build_active,omitempty"`
		TrainActive  bool       `json:"train_active,omitempty"`
	}
	var markers []provinceMarker
	for rows.Next() {
		var m provinceMarker
		if err := rows.Scan(&m.ID, &m.SettlementID, &m.Name, &m.Culture, &m.KingdomID, &m.Q, &m.R, &m.State, &m.Walls, &m.Owner, &m.KingdomName, &m.ArmyTotal, &m.BuildActive, &m.TrainActive); err != nil {
			continue
		}
		pos := province.MapPosition{Q: m.Q, R: m.R}
		m.Visible = !authenticated || province.VisibleFrom(pos, origins, 6)
		if authenticated {
			m.Own = ownSettlements[m.ID]
			m.IsCapital = m.ID == ownProvinceID
			m.Allied = !m.Own && ownKingdomID != nil && m.KingdomID != nil && *ownKingdomID == *m.KingdomID
		}
		if !m.Visible {
			continue // don't reveal fog tiles
		}
		if !m.Own {
			// Don't expose enemy/neutral garrison or activity — FOW.
			m.ArmyTotal = 0
			m.BuildActive = false
			m.TrainActive = false
		}
		markers = append(markers, m)
	}
	// A row error here (e.g. a NULL scanned into a non-nullable dest) closes the pgx
	// result set mid-stream, silently truncating markers. Surface it instead of
	// returning a partial/empty map that looks like fog-of-war.
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "could not load provinces")
		return
	}
	if markers == nil {
		markers = []provinceMarker{}
	}

	// Also include outpost provinces (controlled by a player but no settlement row).
	orows, _ := h.pool.Query(r.Context(),
		`SELECT p.id, p.map_q, p.map_r, p.owner_id, pl.username
		 FROM provinces p
		 JOIN players pl ON pl.id = p.owner_id
		 WHERE p.world_id = $1 AND p.outpost_feeds IS NOT NULL
		   AND NOT EXISTS (SELECT 1 FROM settlements s WHERE s.province_id = p.id)`,
		worldID,
	)
	if orows != nil {
		defer orows.Close()
		for orows.Next() {
			var m provinceMarker
			var ownerID uuid.UUID
			if err := orows.Scan(&m.ID, &m.Q, &m.R, &ownerID, &m.Owner); err != nil {
				continue
			}
			m.IsOutpost = true
			m.Name = "Outpost"
			pos := province.MapPosition{Q: m.Q, R: m.R}
			m.Visible = !authenticated || province.VisibleFrom(pos, origins, 6)
			if !m.Visible {
				continue
			}
			if authenticated {
				m.Own = ownerID == playerID
			}
			markers = append(markers, m)
		}
	}

	writeJSON(w, http.StatusOK, markers)
}

// visibleOrigins loads the map positions of the player's province and all allied
// kingdom member provinces, plus the origin and target endpoints of the player's
// unresolved marches. These are the "eyes" used for fog-of-war calculation.
func (h *WorldHandler) visibleOrigins(ctx context.Context, worldID, playerID uuid.UUID) []province.MapPosition {
	return loadVisibleOrigins(ctx, h.pool, worldID, playerID)
}

// loadVisibleOrigins is the package-level implementation of the KNOWN-set query
// (live ∪ remembered ∪ contacted), shared by WorldHandler and MessengerHandler (and
// any future handler that needs to gate access by contact/visibility) — NOT the
// tiered live-vision eyes used for map rendering (see loadLiveEyes). Keeping this
// generous flat-radius set is the CRITICAL invariant from temenos_synlighet.md:
// messenger Send and the Wanaxes directory must keep gating on everything a player
// has ever discovered, not just what their eyes currently see, or shrinking live
// sight to 2-3 hexes would lock players out of cities they already contacted.
// Only h.pool is needed, so extracting to a free function avoids constructing a
// partial WorldHandler.
func loadVisibleOrigins(ctx context.Context, pool *pgxpool.Pool, worldID, playerID uuid.UUID) []province.MapPosition {
	rows, err := pool.Query(ctx,
		`SELECT DISTINCT pos.q, pos.r FROM (
		     -- Own and allied settlements.
		     SELECT p.map_q AS q, p.map_r AS r
		     FROM provinces p
		     JOIN settlements s ON s.province_id = p.id
		     WHERE p.world_id = $1 AND (
		         s.owner_id = $2
		         OR (s.kingdom_id IS NOT NULL AND s.kingdom_id IN (
		             SELECT km.kingdom_id FROM kingdom_members km WHERE km.player_id = $2
		         ))
		     )
		     UNION ALL
		     -- Origin and target endpoints of the player's in-flight marches.
		     SELECT op.map_q, op.map_r
		     FROM marching_armies ma
		     JOIN provinces op ON op.id = ma.origin_id
		     JOIN settlements os ON os.province_id = ma.origin_id
		     WHERE ma.world_id = $1 AND ma.resolved = false AND os.owner_id = $2
		     UNION ALL
		     -- Explore marches are excluded: vision is revealed on arrival, not dispatch.
		     SELECT tp.map_q, tp.map_r
		     FROM marching_armies ma
		     JOIN provinces tp ON tp.id = ma.target_id
		     JOIN settlements os ON os.province_id = ma.origin_id
		     WHERE ma.world_id = $1 AND ma.resolved = false AND os.owner_id = $2
		       AND ma.intent != 'explore'
		     UNION ALL
		     -- Settlements this player has contacted by messenger stay visible:
		     -- once a messenger reaches its destination (delivered and onward),
		     -- contact is established, so the destination remains on the map
		     -- afterwards — this is how a Wanax discovers trade partners.
		     SELECT dp.map_q, dp.map_r
		     FROM messengers m
		     JOIN settlements ds ON ds.id = m.destination_id
		     JOIN provinces dp ON dp.id = ds.province_id
		     WHERE m.world_id = $1 AND m.sender_id = $2
		       AND m.status IN ('delivered', 'returning', 'arrived')
		     UNION ALL
		     -- Provinces scouted by explore marches remain visible after the ship returns.
		     SELECT p.map_q, p.map_r
		     FROM player_scouted_provinces sp
		     JOIN provinces p ON p.id = sp.province_id
		     WHERE sp.world_id = $1 AND sp.player_id = $2
		     UNION ALL
		     -- Tiles scouted by unit marches (route-swept FOW).
		     SELECT q, r FROM player_scouted_tiles WHERE world_id = $1 AND player_id = $2
		 ) pos`,
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

// loadLiveEyes returns the player's tier-1 (live) vision sources: own and allied
// settlements, plus own units currently on the map (marching or positioned — units
// still 'forming'/'garrison' have no q/r of their own and are seen only via their
// settlement's eye; 'embarked' units carry no position, they move with their ship).
// Each eye is typed so province.LiveRadius can size vision per temenos_synlighet.md's
// per-eye-kind × per-target-terrain table. Scouted tiles/provinces and messenger
// contacts are NOT eyes — they are tier-2 memory (see loadRememberedTiles).
func loadLiveEyes(ctx context.Context, pool *pgxpool.Pool, worldID, playerID uuid.UUID) []province.Eye {
	rows, err := pool.Query(ctx,
		`SELECT q, r, kind FROM (
		     SELECT p.map_q AS q, p.map_r AS r, 'settlement' AS kind
		     FROM provinces p
		     JOIN settlements s ON s.province_id = p.id
		     WHERE p.world_id = $1 AND (
		         s.owner_id = $2
		         OR (s.kingdom_id IS NOT NULL AND s.kingdom_id IN (
		             SELECT km.kingdom_id FROM kingdom_members km WHERE km.player_id = $2
		         ))
		     )
		     UNION ALL
		     SELECT u.q, u.r,
		            CASE WHEN u.category = 'naval' THEN 'ship' ELSE 'land-unit' END AS kind
		     FROM units u
		     WHERE u.world_id = $1 AND u.owner_id = $2
		       AND u.status != 'embarked'
		       AND u.q IS NOT NULL AND u.r IS NOT NULL
		 ) eyes`,
		worldID, playerID,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var eyes []province.Eye
	for rows.Next() {
		var e province.Eye
		if err := rows.Scan(&e.Pos.Q, &e.Pos.R, &e.Kind); err == nil {
			eyes = append(eyes, e)
		}
	}
	return eyes
}

// loadRememberedTiles returns the set of tiles the player has ever live-seen and
// therefore remembers (tier 2, temenos_synlighet.md): tiles swept by unit marches,
// provinces scouted by explore marches, and cities reached by messenger contact.
// These stay visible (terrain + last-seen deposits/city, dimmed, frozen) after the
// live eye that revealed them moves on — but carry no live activity.
func loadRememberedTiles(ctx context.Context, pool *pgxpool.Pool, worldID, playerID uuid.UUID) map[[2]int]bool {
	rows, err := pool.Query(ctx,
		`SELECT q, r FROM player_scouted_tiles WHERE world_id = $1 AND player_id = $2
		 UNION
		 SELECT p.map_q, p.map_r
		 FROM player_scouted_provinces sp
		 JOIN provinces p ON p.id = sp.province_id
		 WHERE sp.world_id = $1 AND sp.player_id = $2
		 UNION
		 SELECT dp.map_q, dp.map_r
		 FROM messengers m
		 JOIN settlements ds ON ds.id = m.destination_id
		 JOIN provinces dp ON dp.id = ds.province_id
		 WHERE m.world_id = $1 AND m.sender_id = $2
		   AND m.status IN ('delivered', 'returning', 'arrived')`,
		worldID, playerID,
	)
	if err != nil {
		return map[[2]int]bool{}
	}
	defer rows.Close()

	set := map[[2]int]bool{}
	for rows.Next() {
		var q, r int
		if err := rows.Scan(&q, &r); err == nil {
			set[[2]int{q, r}] = true
		}
	}
	return set
}

// Marches handles GET /worlds/:worldID/marches — all unresolved marching armies visible
// to the player. Used by the map renderer to draw animated walkers.
//
// Live activity only (temenos_synlighet.md): a march is shown only while its origin
// or target sits on a tier-1 LIVE tile — remembered (tier-2) tiles show frozen
// terrain/last-seen state, never moving armies.
func (h *WorldHandler) Marches(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}
	playerID, authenticated := auth.PlayerIDFromContext(r.Context())

	var eyes []province.Eye
	if authenticated {
		eyes = loadLiveEyes(r.Context(), h.pool, worldID, playerID)
	}

	rows, err := h.pool.Query(r.Context(),
		`SELECT ma.id, ma.intent,
		        op.map_q, op.map_r, op.terrain_type, tp.map_q, tp.map_r, tp.terrain_type,
		        ma.departs_at, ma.arrives_at,
		        (COALESCE(ma.ship,0) + COALESCE(ma.war_galley,0) + COALESCE(ma.merchantman,0)) > 0 AS is_naval
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
		IsNaval   bool      `json:"is_naval,omitempty"`
	}

	visible := func(q, r int, terrain string) bool {
		if !authenticated {
			return false
		}
		return province.AnyEyeSees(eyes, province.MapPosition{Q: q, R: r}, terrain)
	}

	var markers []marchMarker
	for rows.Next() {
		var m marchMarker
		var originTerrain, targetTerrain string
		if err := rows.Scan(&m.ID, &m.Intent,
			&m.OriginQ, &m.OriginR, &originTerrain, &m.TargetQ, &m.TargetR, &targetTerrain,
			&m.DepartsAt, &m.ArrivesAt, &m.IsNaval); err != nil {
			continue
		}
		if !visible(m.OriginQ, m.OriginR, originTerrain) && !visible(m.TargetQ, m.TargetR, targetTerrain) {
			continue
		}
		markers = append(markers, m)
	}
	if markers == nil {
		markers = []marchMarker{}
	}
	writeJSON(w, http.StatusOK, markers)
}

// MapTrades handles GET /worlds/:worldID/trades — active trade caravans visible to the
// player. Used by the map renderer to draw animated caravan walkers.
//
// Live activity only (temenos_synlighet.md) — same tier-1 gate as Marches.
func (h *WorldHandler) MapTrades(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}
	playerID, authenticated := auth.PlayerIDFromContext(r.Context())

	var eyes []province.Eye
	if authenticated {
		eyes = loadLiveEyes(r.Context(), h.pool, worldID, playerID)
	}

	rows, err := h.pool.Query(r.Context(),
		`SELECT tr.id, tr.good_key, op.map_q, op.map_r, op.terrain_type,
		        dp.map_q, dp.map_r, dp.terrain_type, tr.departs_at, tr.arrives_at
		 FROM trade_routes tr
		 JOIN settlements os ON os.id = tr.origin_id
		 JOIN provinces op ON op.id = os.province_id
		 JOIN settlements ds ON ds.id = tr.destination_id
		 JOIN provinces dp ON dp.id = ds.province_id
		 WHERE tr.world_id = $1 AND tr.resolved = false`,
		worldID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load trades")
		return
	}
	defer rows.Close()

	type tradeMarker struct {
		ID        uuid.UUID `json:"id"`
		GoodKey   string    `json:"good_key"`
		OriginQ   int       `json:"origin_q"`
		OriginR   int       `json:"origin_r"`
		DestQ     int       `json:"dest_q"`
		DestR     int       `json:"dest_r"`
		DepartsAt time.Time `json:"departs_at"`
		ArrivesAt time.Time `json:"arrives_at"`
	}

	var markers []tradeMarker
	for rows.Next() {
		var m tradeMarker
		var originTerrain, destTerrain string
		if err := rows.Scan(&m.ID, &m.GoodKey, &m.OriginQ, &m.OriginR, &originTerrain,
			&m.DestQ, &m.DestR, &destTerrain, &m.DepartsAt, &m.ArrivesAt); err != nil {
			continue
		}
		if authenticated &&
			!province.AnyEyeSees(eyes, province.MapPosition{Q: m.OriginQ, R: m.OriginR}, originTerrain) &&
			!province.AnyEyeSees(eyes, province.MapPosition{Q: m.DestQ, R: m.DestR}, destTerrain) {
			continue
		}
		markers = append(markers, m)
	}
	if markers == nil {
		markers = []tradeMarker{}
	}
	writeJSON(w, http.StatusOK, markers)
}

// MapMessengers handles GET /worlds/:worldID/messengers — outbound messengers visible
// to the player. Used by the map renderer to draw animated messenger walkers.
//
// Live activity only (temenos_synlighet.md) — same tier-1 gate as Marches.
func (h *WorldHandler) MapMessengers(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}
	playerID, authenticated := auth.PlayerIDFromContext(r.Context())

	var eyes []province.Eye
	if authenticated {
		eyes = loadLiveEyes(r.Context(), h.pool, worldID, playerID)
	}

	rows, err := h.pool.Query(r.Context(),
		`SELECT m.id, op.map_q, op.map_r, op.terrain_type,
		        COALESCE(dp.map_q, m.dest_q), COALESCE(dp.map_r, m.dest_r), COALESCE(dp.terrain_type, ''),
		        m.sent_at, m.arrives_at
		 FROM messengers m
		 JOIN settlements os ON os.id = m.origin_id
		 JOIN provinces op ON op.id = os.province_id
		 LEFT JOIN settlements ds ON ds.id = m.destination_id
		 LEFT JOIN provinces dp ON dp.id = ds.province_id
		 WHERE m.world_id = $1 AND m.status = 'outbound'`,
		worldID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load messengers")
		return
	}
	defer rows.Close()

	type messengerMarker struct {
		ID        uuid.UUID `json:"id"`
		OriginQ   int       `json:"origin_q"`
		OriginR   int       `json:"origin_r"`
		DestQ     int       `json:"dest_q"`
		DestR     int       `json:"dest_r"`
		SentAt    time.Time `json:"sent_at"`
		ArrivesAt time.Time `json:"arrives_at"`
	}

	var markers []messengerMarker
	for rows.Next() {
		var m messengerMarker
		var originTerrain, destTerrain string
		if err := rows.Scan(&m.ID, &m.OriginQ, &m.OriginR, &originTerrain, &m.DestQ, &m.DestR, &destTerrain,
			&m.SentAt, &m.ArrivesAt); err != nil {
			continue
		}
		if authenticated &&
			!province.AnyEyeSees(eyes, province.MapPosition{Q: m.OriginQ, R: m.OriginR}, originTerrain) &&
			!province.AnyEyeSees(eyes, province.MapPosition{Q: m.DestQ, R: m.DestR}, destTerrain) {
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
			`INSERT INTO map_tiles (world_id, q, r, terrain, coastal, fertility, mineral, copper_deposit, tin_deposit, silver_deposit, cedar_deposit)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
			 ON CONFLICT (world_id, q, r) DO NOTHING`,
			worldID, t.Q, t.R, string(t.Terrain), t.Coastal, t.Fertility, t.Mineral, t.CopperDeposit, t.TinDeposit, t.SilverDeposit, t.CedarDeposit,
		)
	}
	br := h.pool.SendBatch(ctx, batch)
	return br.Close()
}

// Wanaxes handles GET /worlds/{worldID}/wanaxes — FOW-gated trade-discovery directory
// for API clients. Returns only settlements the requesting player can currently see
// (within 6 hexes of their visibleOrigins — same FOW rule as /provinces). Army strength
// is deliberately NOT exposed. Unauthenticated requests receive an empty list.
func (h *WorldHandler) Wanaxes(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}
	playerID, authenticated := auth.PlayerIDFromContext(r.Context())

	// Without authentication there is no FOW context — return empty list rather than
	// leaking the full global settlement catalog.
	if !authenticated {
		writeJSON(w, http.StatusOK, []struct{}{})
		return
	}

	origins := h.visibleOrigins(r.Context(), worldID, playerID)

	rows, err := h.pool.Query(r.Context(),
		`SELECT s.id, s.name, p.username, s.culture_id, prov.terrain_type,
		        (SELECT k.name FROM kingdoms k
		         JOIN kingdom_members km ON km.kingdom_id = k.id
		         WHERE km.player_id = s.owner_id AND k.world_id = $1 LIMIT 1),
		        s.owner_id,
		        COALESCE(prov.copper_deposit, false),
		        COALESCE(prov.tin_deposit, false),
		        COALESCE(prov.silver_deposit, false),
		        COALESCE(prov.cedar_deposit, false),
		        COALESCE(prov.map_q, 0),
		        COALESCE(prov.map_r, 0),
		        COALESCE(prov.id::text, ''),
		        s.is_capital
		 FROM settlements s
		 LEFT JOIN players p ON p.id = s.owner_id
		 LEFT JOIN provinces prov ON prov.id = s.province_id
		 WHERE s.world_id = $1 AND s.owner_id IS NOT NULL AND s.state != 'sunk'
		 ORDER BY s.name`,
		worldID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	defer rows.Close()

	type entry struct {
		SettlementID  string `json:"settlement_id"`
		Name          string `json:"name"`
		Owner         string `json:"owner"`
		Culture       string `json:"culture"`
		Terrain       string `json:"terrain"`
		Kingdom       string `json:"kingdom,omitempty"`
		Own           bool   `json:"own"`
		CopperDeposit bool   `json:"copper_deposit,omitempty"`
		TinDeposit    bool   `json:"tin_deposit,omitempty"`
		SilverDeposit bool   `json:"silver_deposit,omitempty"`
		CedarDeposit  bool   `json:"cedar_deposit,omitempty"`
		ProvinceID    string `json:"province_id,omitempty"`
		IsCapital     bool   `json:"is_capital,omitempty"`
	}
	var result []entry
	for rows.Next() {
		var e entry
		var ownerID *uuid.UUID
		var kingdom *string
		var terrain *string
		var q, r int
		if err := rows.Scan(&e.SettlementID, &e.Name, &e.Owner, &e.Culture, &terrain, &kingdom,
			&ownerID,
			&e.CopperDeposit, &e.TinDeposit, &e.SilverDeposit, &e.CedarDeposit,
			&q, &r, &e.ProvinceID, &e.IsCapital); err != nil {
			continue
		}
		// FOW gate: skip settlements the player cannot currently see.
		if !province.VisibleFrom(province.MapPosition{Q: q, R: r}, origins, 6) {
			continue
		}
		if kingdom != nil {
			e.Kingdom = *kingdom
		}
		if terrain != nil {
			e.Terrain = *terrain
		}
		if ownerID != nil && *ownerID == playerID {
			e.Own = true
		}
		result = append(result, e)
	}
	if result == nil {
		result = []entry{}
	}
	writeJSON(w, http.StatusOK, result)
}
