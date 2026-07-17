package handlers

import (
	"context"
	"encoding/json"
	"math"
	"math/rand"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/poleia/server/internal/auth"
	"github.com/poleia/server/internal/clock"
	"github.com/poleia/server/internal/economy"
	"github.com/poleia/server/internal/events"
	"github.com/poleia/server/internal/province"
	"github.com/poleia/server/internal/tick"
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

	writeJSON(w, http.StatusOK, map[string]any{
		"id":             wld.ID,
		"name":           wld.Name,
		"state":          wld.State,
		"prestige":       wld.Prestige,
		"era_number":     wld.EraNumber,
		"era_started_at": wld.EraStartedAt,
		"created_at":     wld.CreatedAt,
		// Tick anchor for the client (Tid & kalender Fas B / K4 contract):
		// current_tick + the runtime cadence let countdowns be recomputed
		// locally from an authoritative arrival_tick without a new fetch.
		"current_tick": wld.CurrentTick,
		"tick_seconds": tick.TickSeconds,
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
		eyes = loadLiveEyes(r.Context(), h.pool, worldID, playerID, h.clk.Now())
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

// ColonizePreview handles GET /worlds/:worldID/colonize-preview?q=&r= — a
// read-only grain/goods forecast for founding a colony on the given hex, shown
// to the Wanax BEFORE the colonize march is dispatched (DEL A of
// megaron_koloni_legibilitet_plan.md). It runs no colonize gates (cap, empty
// hex, ...) — those belong to the March POST; this only reports what the target
// hex's catchment could feed a new colony.
//
// FOW-CRITICAL: the catchment is tiered with the SAME live∪remembered logic as
// Map (loadLiveEyes + loadRememberedTiles + AnyEyeSees). A fog hex is reported as
// {known:false} with NO terrain/deposit fields, and contributes nothing to the
// goods/grain estimate — an unseen hex's contents must never leak here.
func (h *WorldHandler) ColonizePreview(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}
	q, qErr := strconv.Atoi(r.URL.Query().Get("q"))
	rr, rErr := strconv.Atoi(r.URL.Query().Get("r"))
	if qErr != nil || rErr != nil {
		writeError(w, http.StatusBadRequest, "q and r query params are required integers")
		return
	}

	ctx := r.Context()
	playerID, authenticated := auth.PlayerIDFromContext(ctx)

	// Catchment = centre + 6 neighbours. SAME axial offsets as
	// economy.RecomputeProduction (recompute.go step 2) — keep them identical.
	catchment := [][2]int{
		{q, rr},
		{q + 1, rr}, {q - 1, rr},
		{q, rr + 1}, {q, rr - 1},
		{q + 1, rr - 1}, {q - 1, rr + 1},
	}

	// Live vision + remembered memory, exactly as Map computes them.
	var eyes []province.Eye
	var remembered map[[2]int]bool
	if authenticated {
		eyes = loadLiveEyes(ctx, h.pool, worldID, playerID, h.clk.Now())
		remembered = loadRememberedTiles(ctx, h.pool, worldID, playerID)
	}

	// Fetch the 7 catchment tiles server-side (needed to compute visibility); we
	// only ever REVEAL a tile's terrain/deposits in the response if it is known.
	qs := make([]int32, len(catchment))
	rs := make([]int32, len(catchment))
	for i, c := range catchment {
		qs[i], rs[i] = int32(c[0]), int32(c[1])
	}
	type catTile struct {
		terrain                          string
		coastal, copper, tin, cedar, sil bool
	}
	tiles := map[[2]int]catTile{}
	trows, err := h.pool.Query(ctx,
		`SELECT mt.q, mt.r, mt.terrain, mt.coastal, mt.copper_deposit, mt.tin_deposit,
		        COALESCE(mt.cedar_deposit,false), COALESCE(mt.silver_deposit,false)
		 FROM map_tiles mt
		 JOIN unnest($2::int[], $3::int[]) AS hx(q, r) ON hx.q = mt.q AND hx.r = mt.r
		 WHERE mt.world_id = $1`,
		worldID, qs, rs,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load catchment tiles")
		return
	}
	for trows.Next() {
		var tq, tr int
		var ct catTile
		if err := trows.Scan(&tq, &tr, &ct.terrain, &ct.coastal, &ct.copper, &ct.tin, &ct.cedar, &ct.sil); err != nil {
			continue
		}
		tiles[[2]int{tq, tr}] = ct
	}
	trows.Close()

	type catchmentEntry struct {
		Q             int    `json:"q"`
		R             int    `json:"r"`
		Known         bool   `json:"known"`
		Terrain       string `json:"terrain,omitempty"`
		Coastal       bool   `json:"coastal,omitempty"`
		CopperDeposit bool   `json:"copper_deposit,omitempty"`
		TinDeposit    bool   `json:"tin_deposit,omitempty"`
		SilverDeposit bool   `json:"silver_deposit,omitempty"`
		CedarDeposit  bool   `json:"cedar_deposit,omitempty"`
	}
	view := make([]catchmentEntry, 0, len(catchment))
	var knownHexes [][2]int
	unknown := 0
	for _, c := range catchment {
		key := [2]int{c[0], c[1]}
		ct, exists := tiles[key]
		known := false
		if exists {
			switch {
			case !authenticated:
				// Mirror Map: an unauthenticated caller has no FOW context and sees
				// the world as live (the /map endpoint is public the same way).
				known = true
			case province.AnyEyeSees(eyes, province.MapPosition{Q: c[0], R: c[1]}, ct.terrain):
				known = true
			case remembered[key]:
				known = true
			}
		}
		e := catchmentEntry{Q: c[0], R: c[1], Known: known}
		if known {
			e.Terrain = ct.terrain
			e.Coastal = ct.coastal
			e.CopperDeposit = ct.copper
			e.TinDeposit = ct.tin
			e.SilverDeposit = ct.sil
			e.CedarDeposit = ct.cedar
			knownHexes = append(knownHexes, key)
		} else {
			unknown++
		}
		view = append(view, e)
	}

	// Goods potential over KNOWN hexes only (building-free), and the same with a
	// hypothetical farm to answer "could this place ever feed itself?".
	goods, err := economy.CatchmentBasePotentialAt(ctx, h.pool, worldID, knownHexes, nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not compute catchment potential")
		return
	}
	withFarm, err := economy.CatchmentBasePotentialAt(ctx, h.pool, worldID, knownHexes, []string{"farm"})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not compute farm potential")
		return
	}

	// Founding grain balance. Consumption at the founding population, per tick,
	// re-using the exported calibration constants (never duplicated here).
	// ?pop= and ?seed= let the founder phase reuse this exact forecast for the
	// metropolis (4 000 people, the host's carried grain as stock) instead of
	// growing its own endpoint — temenos_nomadic_host_fas4_plan.md 4.3. Defaults
	// are the colony's, so every existing caller is untouched.
	forecastPop := economy.ColonyBaseFoundingPopulation
	if v, err := strconv.Atoi(r.URL.Query().Get("pop")); err == nil && v > 0 {
		forecastPop = v
	}
	seed := float64(economy.ColonyGrainSeed)
	if v, err := strconv.ParseFloat(r.URL.Query().Get("seed"), 64); err == nil && v >= 0 {
		seed = v
	}
	consumptionPerTick := float64(forecastPop) *
		economy.GrainConsumptionPerCitizenPerDay / float64(events.TicksPerDay)
	basePerTick := goods["grain"]
	estNetPerTick := basePerTick - consumptionPerTick

	var daysUntilEmpty *float64
	if estNetPerTick < 0 {
		dailyDrain := -estNetPerTick * float64(events.TicksPerDay)
		if dailyDrain > 0 {
			d := seed / dailyDrain
			daysUntilEmpty = &d
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"catchment": view,
		"goods":     goods,
		"grain": map[string]any{
			"base_per_tick":      basePerTick,
			"est_net_per_tick":   estNetPerTick,
			"seed":               seed,
			"days_until_empty":   daysUntilEmpty,
			"with_farm_per_tick": withFarm["grain"],
		},
		"unknown_hexes": unknown,
	})
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
		        COALESCE((SELECT SUM(size) FROM units u WHERE u.settlement_id = s.id AND u.status = 'garrison'), 0)::int AS army_total,
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
//
// A marching unit's stored (q,r) is its ORIGIN hex — March (unit.go) never moves it,
// only target_q/target_r/departs_at/arrives_at are set at dispatch. So a marching
// unit's eye is placed at its interpolated position along its route (see
// interpolatedEyePos) instead of the stored origin, or the fog bubble would sit at
// the harbour for the whole voyage instead of tracking the ship. now must come from
// the caller's injected clock.Clock — never time.Now() (CLAUDE.md Time rule).
func loadLiveEyes(ctx context.Context, pool *pgxpool.Pool, worldID, playerID uuid.UUID, now time.Time) []province.Eye {
	var eyes []province.Eye

	sRows, err := pool.Query(ctx,
		`SELECT p.map_q, p.map_r
		 FROM provinces p
		 JOIN settlements s ON s.province_id = p.id
		 WHERE p.world_id = $1 AND (
		     s.owner_id = $2
		     OR (s.kingdom_id IS NOT NULL AND s.kingdom_id IN (
		         SELECT km.kingdom_id FROM kingdom_members km WHERE km.player_id = $2
		     ))
		 )`,
		worldID, playerID,
	)
	if err == nil {
		for sRows.Next() {
			var pos province.MapPosition
			if sRows.Scan(&pos.Q, &pos.R) == nil {
				eyes = append(eyes, province.Eye{Pos: pos, Kind: province.EyeSettlement})
			}
		}
		sRows.Close()
	}

	uRows, err := pool.Query(ctx,
		`SELECT status, q, r, target_q, target_r, category, type, departs_at, arrives_at
		 FROM units
		 WHERE world_id = $1 AND owner_id = $2
		   AND status != 'embarked'
		   AND q IS NOT NULL AND r IS NOT NULL`,
		worldID, playerID,
	)
	if err == nil {
		for uRows.Next() {
			var status, category, utype string
			var q, r int
			var targetQ, targetR *int
			var departsAt, arrivesAt *time.Time
			if uRows.Scan(&status, &q, &r, &targetQ, &targetR, &category, &utype, &departsAt, &arrivesAt) != nil {
				continue
			}
			kind := province.EyeLandUnit
			if category == "naval" {
				kind = province.EyeShip
			}
			pos := province.MapPosition{Q: q, R: r}
			if status == "marching" && targetQ != nil && targetR != nil && departsAt != nil && arrivesAt != nil {
				path, _, ok, pathErr := province.FindPath(ctx, pool, worldID, pos,
					province.MapPosition{Q: *targetQ, R: *targetR}, category)
				if pathErr == nil && ok && len(path) > 0 {
					pos = interpolatedEyePos(now, *departsAt, *arrivesAt, path)
				}
				// FindPath failure/empty path: best-effort fallback to the stored
				// origin (q,r) set above — never drop the eye.
			}
			eyes = append(eyes, province.Eye{Pos: pos, Kind: kind})
		}
		uRows.Close()
	}

	// Hemerodromoi — the player's own outbound messengers (orders, diplomatic
	// letters, recalls) are live eyes along their route, seeing as a land unit
	// (temenos_synlighet.md §Nivå 1, beslut Timothy 2026-07-16). Position is
	// interpolated along the courier A*-path (land at 2× spearman speed, sea by
	// boat) between sent_at and arrives_at — same pattern as marching units.
	// The return leg (status='returning') carries no usable timing window on
	// the row (arrives_at still holds the outbound arrival) and is left without
	// an eye until that plumbing exists; couriers are uninterceptable either way.
	mRows, err := pool.Query(ctx,
		`SELECT m.hex_q, m.hex_r,
		        COALESCE(m.dest_q, dp.map_q), COALESCE(m.dest_r, dp.map_r),
		        m.sent_at, m.arrives_at
		 FROM messengers m
		 LEFT JOIN settlements ds ON ds.id = m.destination_id
		 LEFT JOIN provinces dp ON dp.id = ds.province_id
		 WHERE m.world_id = $1 AND m.sender_id = $2 AND m.status = 'outbound'`,
		worldID, playerID,
	)
	if err == nil {
		for mRows.Next() {
			var oq, or_ int
			var dq, dr *int
			var sentAt, arrivesAt time.Time
			if mRows.Scan(&oq, &or_, &dq, &dr, &sentAt, &arrivesAt) != nil {
				continue
			}
			pos := province.MapPosition{Q: oq, R: or_}
			if dq != nil && dr != nil {
				path, _, ok, pathErr := province.FindPath(ctx, pool, worldID, pos,
					province.MapPosition{Q: *dq, R: *dr}, province.CategoryCourier)
				if pathErr == nil && ok && len(path) > 0 {
					pos = interpolatedEyePos(now, sentAt, arrivesAt, path)
				}
				// FindPath failure: best-effort fallback to the origin hex — the
				// courier still sees from somewhere, never drops the eye.
			}
			eyes = append(eyes, province.Eye{Pos: pos, Kind: province.EyeLandUnit})
		}
		mRows.Close()
	}

	return eyes
}

// interpolatedEyePos returns a marching unit's live-vision position: its location
// along path at the current point in its journey, linearly interpolated by elapsed
// wall-clock time between departsAt and arrivesAt (temenos_synlighet.md tier 1 —
// vision must track a moving ship/unit, not just its departure or arrival hex).
//
// progress is clamped to [0,1] so a read before departure or after arrival snaps to
// the first/last hex rather than extrapolating. idx = round(progress*(len(path)-1))
// picks the nearest path hex to the elapsed fraction. Pure and DB-free: path is
// precomputed by the caller (province.FindPath) so this is unit-testable with fixed
// time.Time values and no clock. An empty path has no position to return; callers
// must check len(path) > 0 before calling and fall back to the unit's stored (q,r)
// otherwise — this function returns the zero MapPosition rather than panicking.
func interpolatedEyePos(now, departsAt, arrivesAt time.Time, path []province.MapPosition) province.MapPosition {
	if len(path) == 0 {
		return province.MapPosition{}
	}

	var progress float64
	if total := arrivesAt.Sub(departsAt); total > 0 {
		progress = float64(now.Sub(departsAt)) / float64(total)
	}
	if progress < 0 {
		progress = 0
	} else if progress > 1 {
		progress = 1
	}

	idx := int(math.Round(progress * float64(len(path)-1)))
	if idx < 0 {
		idx = 0
	} else if idx >= len(path) {
		idx = len(path) - 1
	}
	return path[idx]
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
		eyes = loadLiveEyes(r.Context(), h.pool, worldID, playerID, h.clk.Now())
	}

	rows, err := h.pool.Query(r.Context(),
		`SELECT ma.id, ma.intent,
		        op.map_q, op.map_r, op.terrain_type, tp.map_q, tp.map_r, tp.terrain_type,
		        ma.departs_at, ma.arrives_at, ma.depart_tick, ma.arrive_tick,
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
		// Tick-anchored timing (mig 089, tid Fas B): authoritative under tempo
		// changes where the ISO snapshots go stale. Nil on pre-089 rows — the
		// client falls back to arrives_at (ui/time.js msUntil).
		DepartTick  *int `json:"depart_tick,omitempty"`
		ArrivalTick *int `json:"arrival_tick,omitempty"`
		IsNaval     bool `json:"is_naval,omitempty"`
		// Path is the A* route [[q,r],...] the army actually follows (via sea for
		// naval, around mountains for land) — the client animates along it instead
		// of a straight origin→target line, so the walker is drawn where the unit
		// truly is (matches province.InterpolatePosition). Empty ⇒ client falls back
		// to the straight line.
		Path [][2]int `json:"path,omitempty"`
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
			&m.DepartsAt, &m.ArrivesAt, &m.DepartTick, &m.ArrivalTick, &m.IsNaval); err != nil {
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
	// Attach the real A* route to each march. Load the tile graph once, then path
	// every marker against it — per-marker FindPath would reload all tiles each time.
	if len(markers) > 0 {
		if g, gerr := province.LoadTileGraph(r.Context(), h.pool, worldID); gerr == nil {
			for i := range markers {
				cat := "land"
				if markers[i].IsNaval {
					cat = "naval"
				}
				markers[i].Path = marchPathWaypoints(g, markers[i].OriginQ, markers[i].OriginR, markers[i].TargetQ, markers[i].TargetR, cat)
			}
		}
	}
	writeJSON(w, http.StatusOK, markers)
}

// marchPathWaypoints returns the A* route as [[q,r],...] (origin+target included)
// for a unit of the given category over a pre-loaded tile graph. Returns nil when
// no route exists or the path is trivial, so the client falls back to a straight line.
func marchPathWaypoints(g province.TileGraph, oq, or, tq, tr int, category string) [][2]int {
	path, _, ok := g.FindPath(province.MapPosition{Q: oq, R: or}, province.MapPosition{Q: tq, R: tr}, category)
	if !ok || len(path) < 2 {
		return nil
	}
	out := make([][2]int, len(path))
	for i, p := range path {
		out[i] = [2]int{p.Q, p.R}
	}
	return out
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
		eyes = loadLiveEyes(r.Context(), h.pool, worldID, playerID, h.clk.Now())
	}

	// Physical caravans in motion (movement-motor transport layer). Every in-transit
	// row is a real mover — trade legs (delivery + return) AND internal transfers —
	// so all of them render, and an intercepted caravan (status != 'in_transit')
	// vanishes from the map the moment it is seized. good_key = the heaviest manifest
	// good, for display. origin/dest coords come off the transport row itself; the
	// settlement join only supplies terrain for the fog-of-war gate.
	rows, err := h.pool.Query(r.Context(),
		`SELECT t.id,
		        COALESCE((SELECT g.good_key FROM transport_goods g
		                  WHERE g.transport_id = t.id ORDER BY g.quantity DESC LIMIT 1), ''),
		        t.origin_q, t.origin_r, COALESCE(op.terrain_type, ''),
		        t.dest_q, t.dest_r, COALESCE(dp.terrain_type, ''),
		        t.departs_at, t.arrives_at
		 FROM transports t
		 LEFT JOIN settlements os ON os.id = t.origin_id
		 LEFT JOIN provinces op ON op.id = os.province_id
		 LEFT JOIN settlements ds ON ds.id = t.dest_id
		 LEFT JOIN provinces dp ON dp.id = ds.province_id
		 WHERE t.world_id = $1 AND t.status = 'in_transit'`,
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
		eyes = loadLiveEyes(r.Context(), h.pool, worldID, playerID, h.clk.Now())
	}

	rows, err := h.pool.Query(r.Context(),
		`SELECT m.id, m.sender_id, m.kind, m.order_payload->>'unit_id',
		        COALESCE(op.map_q, m.origin_q), COALESCE(op.map_r, m.origin_r),
		        COALESCE(op.terrain_type, omt.terrain, ''),
		        COALESCE(dp.map_q, m.dest_q), COALESCE(dp.map_r, m.dest_r), COALESCE(dp.terrain_type, ''),
		        m.sent_at, m.arrives_at
		 FROM messengers m
		 -- LEFT: a host-sent messenger (mig 087) has no origin settlement; its frozen
		 -- departure point (origin_q/origin_r) places it, with terrain off the tile.
		 LEFT JOIN settlements os ON os.id = m.origin_id
		 LEFT JOIN provinces op ON op.id = os.province_id
		 LEFT JOIN map_tiles omt ON omt.world_id = m.world_id AND omt.q = m.origin_q AND omt.r = m.origin_r
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
		ID      uuid.UUID `json:"id"`
		OriginQ int       `json:"origin_q"`
		OriginR int       `json:"origin_r"`
		DestQ   int       `json:"dest_q"`
		DestR   int       `json:"dest_r"`
		SentAt  time.Time `json:"sent_at"`
		ArrivesAt time.Time `json:"arrives_at"`
		// Own marks the requesting player's hemerodromoi — the client draws
		// them along their WHOLE route (dimmed over fog): a player's own
		// courier is information they already possess (temenos_orderlopare_
		// plan.md Fas 5). Foreign messengers keep the tier-1 endpoint gate.
		Own  bool   `json:"own"`
		Kind string `json:"kind"`
		// OrderUnitID ties a kind='order' hemerodromos to the unit it is
		// running to, so the unit card can show "order på väg" + courier ETA.
		OrderUnitID *uuid.UUID `json:"order_unit_id,omitempty"`
	}

	var markers []messengerMarker
	for rows.Next() {
		var m messengerMarker
		var senderID uuid.UUID
		var orderUnitID *string
		var originTerrain, destTerrain string
		if err := rows.Scan(&m.ID, &senderID, &m.Kind, &orderUnitID,
			&m.OriginQ, &m.OriginR, &originTerrain, &m.DestQ, &m.DestR, &destTerrain,
			&m.SentAt, &m.ArrivesAt); err != nil {
			continue
		}
		m.Own = authenticated && senderID == playerID
		if m.Own && orderUnitID != nil {
			if uid, err := uuid.Parse(*orderUnitID); err == nil {
				m.OrderUnitID = &uid
			}
		}
		if !m.Own && authenticated &&
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

// cityEntry is one row of the /cities directory (temenos_gossip.md PASS 2b).
// Knowledge is "known" (seen/remembered/contacted — exact q,r/terrain/deposits)
// or "rumour" (heard of via gossip — fuzzy Bearing + IndustryHint only, Q/R nil,
// never contactable until explored into the known set).
type cityEntry struct {
	SettlementID  string `json:"settlement_id"`
	Name          string `json:"name"`
	Owner         string `json:"owner,omitempty"`
	OwnerID       string `json:"owner_id,omitempty"`
	Culture       string `json:"culture,omitempty"`
	Terrain       string `json:"terrain,omitempty"`
	Kingdom       string `json:"kingdom,omitempty"`
	Own           bool   `json:"own,omitempty"`
	CopperDeposit bool   `json:"copper_deposit,omitempty"`
	TinDeposit    bool   `json:"tin_deposit,omitempty"`
	SilverDeposit bool   `json:"silver_deposit,omitempty"`
	CedarDeposit  bool   `json:"cedar_deposit,omitempty"`
	ProvinceID    string `json:"province_id,omitempty"`
	IsCapital     bool   `json:"is_capital,omitempty"`
	Knowledge     string `json:"knowledge"` // "known" | "rumour"
	Q             *int   `json:"q,omitempty"`
	R             *int   `json:"r,omitempty"`
	Bearing       string `json:"bearing,omitempty"`
	IndustryHint  string `json:"industry_hint,omitempty"`
	Note          string `json:"note,omitempty"`
}

// loadCities builds the combined known + rumour-known directory for playerID:
// the "known" tier is exactly the legacy Wanaxes/wanaxes gate (loadVisibleOrigins
// KNOWN-set: own/allied + contacted + remembered), carrying exact coordinates;
// the "rumour" tier comes from known_settlements (level='rumour', minus anything
// already in the known tier) with a fuzzy bearing off the nearest known
// settlement instead of exact coordinates. Returns nil for an unauthenticated
// caller (playerID == uuid.Nil is not checked here — callers gate that).
func (h *WorldHandler) loadCities(ctx context.Context, worldID, playerID uuid.UUID) []cityEntry {
	origins := h.visibleOrigins(ctx, worldID, playerID)

	rows, err := h.pool.Query(ctx,
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
		return nil
	}
	defer rows.Close()

	var result []cityEntry
	known := map[string]bool{}
	for rows.Next() {
		var e cityEntry
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
		// FOW gate: skip settlements the player cannot currently see/remember/contact.
		if !province.VisibleFrom(province.MapPosition{Q: q, R: r}, origins, 6) {
			continue
		}
		if kingdom != nil {
			e.Kingdom = *kingdom
		}
		if terrain != nil {
			e.Terrain = *terrain
		}
		if ownerID != nil {
			e.OwnerID = ownerID.String()
			if *ownerID == playerID {
				e.Own = true
			}
		}
		e.Knowledge = "known"
		qCopy, rCopy := q, r
		e.Q, e.R = &qCopy, &rCopy
		result = append(result, e)
		known[e.SettlementID] = true
	}

	// Rumour tier: settlements heard of via gossip but never seen/remembered/contacted.
	rumourRows, err := h.pool.Query(ctx,
		`SELECT s.id, s.name, p.username, s.culture_id, prov.terrain_type,
		        (SELECT k.name FROM kingdoms k
		         JOIN kingdom_members km ON km.kingdom_id = k.id
		         WHERE km.player_id = s.owner_id AND k.world_id = $1 LIMIT 1),
		        s.owner_id,
		        COALESCE(prov.copper_deposit, false),
		        COALESCE(prov.tin_deposit, false),
		        COALESCE(prov.silver_deposit, false),
		        COALESCE(prov.cedar_deposit, false),
		        prov.map_q, prov.map_r,
		        COALESCE(ks.industry_hint, '')
		 FROM known_settlements ks
		 JOIN settlements s ON s.id = ks.settlement_id
		 LEFT JOIN players p ON p.id = s.owner_id
		 LEFT JOIN provinces prov ON prov.id = s.province_id
		 WHERE ks.world_id = $1 AND ks.player_id = $2 AND ks.level = 'rumour'
		   AND s.state != 'sunk'`,
		worldID, playerID,
	)
	if err != nil {
		return result
	}
	defer rumourRows.Close()

	for rumourRows.Next() {
		var e cityEntry
		var ownerID *uuid.UUID
		var kingdom, terrain *string
		var q, r *int
		if err := rumourRows.Scan(&e.SettlementID, &e.Name, &e.Owner, &e.Culture, &terrain, &kingdom,
			&ownerID,
			&e.CopperDeposit, &e.TinDeposit, &e.SilverDeposit, &e.CedarDeposit,
			&q, &r, &e.IndustryHint); err != nil {
			continue
		}
		if known[e.SettlementID] {
			continue // already known at a stronger tier — do not downgrade the listing.
		}
		if kingdom != nil {
			e.Kingdom = *kingdom
		}
		if terrain != nil {
			e.Terrain = *terrain
		}
		if ownerID != nil {
			e.OwnerID = ownerID.String()
		}
		e.Knowledge = "rumour"
		e.Note = "explore to confirm — not contactable yet"
		if q != nil && r != nil {
			target := province.MapPosition{Q: *q, R: *r}
			if landmarkName, landmarkPos, ok := nearestLandmark(result, target); ok {
				e.Bearing = province.FuzzyBearing(target, landmarkPos) + " of " + landmarkName
			}
		}
		result = append(result, e)
	}
	return result
}

// nearestLandmark finds the closest known-tier city entry to target, for use
// as the reference point in a rumour's fuzzy bearing (temenos_gossip.md PASS 2b:
// "landmark = the nearest of the player's own settlements + remembered named
// cities" — the known tier already covers both).
func nearestLandmark(known []cityEntry, target province.MapPosition) (name string, pos province.MapPosition, ok bool) {
	best := -1
	for _, e := range known {
		if e.Q == nil || e.R == nil {
			continue
		}
		candidate := province.MapPosition{Q: *e.Q, R: *e.R}
		dist := province.HexDistance(candidate, target)
		if best == -1 || dist < best {
			best = dist
			name = e.Name
			pos = candidate
			ok = true
		}
	}
	return name, pos, ok
}

// Cities handles GET /worlds/{worldID}/cities — the combined known + rumour-known
// settlement directory (temenos_gossip.md PASS 2b). See loadCities for the tier
// split. Unauthenticated requests receive an empty list.
func (h *WorldHandler) Cities(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}
	playerID, authenticated := auth.PlayerIDFromContext(r.Context())
	if !authenticated {
		writeJSON(w, http.StatusOK, []struct{}{})
		return
	}

	result := h.loadCities(r.Context(), worldID, playerID)
	if result == nil {
		result = []cityEntry{}
	}
	writeJSON(w, http.StatusOK, result)
}

// diplomacyEntry is one Wanax in the /diplomacy directory: an owner of at least
// one known or rumour-known city.
type diplomacyEntry struct {
	Owner        string `json:"owner"`
	OwnerID      string `json:"owner_id,omitempty"`
	Kingdom      string `json:"kingdom,omitempty"`
	KnownCities  int    `json:"known_cities"`
	RumourCities int    `json:"rumour_cities"`
	RumourOnly   bool   `json:"rumour_only,omitempty"`
	Own          bool   `json:"own,omitempty"`
}

// Diplomacy handles GET /worlds/{worldID}/diplomacy — a ruler-centric view of
// Cities: one row per Wanax who owns a known or rumour-known city, flagging
// rumour-only rulers (nobody whose cities the player has actually seen/
// remembered/contacted yet). Unauthenticated requests receive an empty list.
func (h *WorldHandler) Diplomacy(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}
	playerID, authenticated := auth.PlayerIDFromContext(r.Context())
	if !authenticated {
		writeJSON(w, http.StatusOK, []struct{}{})
		return
	}

	cities := h.loadCities(r.Context(), worldID, playerID)

	byOwner := map[string]*diplomacyEntry{}
	var order []string
	for _, c := range cities {
		if c.OwnerID == "" || c.Own {
			continue // skip ownerless rows and the player's own cities
		}
		d, seen := byOwner[c.OwnerID]
		if !seen {
			d = &diplomacyEntry{Owner: c.Owner, OwnerID: c.OwnerID, Kingdom: c.Kingdom}
			byOwner[c.OwnerID] = d
			order = append(order, c.OwnerID)
		}
		if c.Knowledge == "known" {
			d.KnownCities++
		} else {
			d.RumourCities++
		}
	}

	result := make([]diplomacyEntry, 0, len(order))
	for _, id := range order {
		d := byOwner[id]
		d.RumourOnly = d.KnownCities == 0
		result = append(result, *d)
	}
	writeJSON(w, http.StatusOK, result)
}
