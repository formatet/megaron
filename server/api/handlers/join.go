package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/poleia/server/internal/auth"
	"github.com/poleia/server/internal/economy"
	"github.com/poleia/server/internal/province"
	"github.com/poleia/server/internal/religion"
	"github.com/poleia/server/internal/world"
)


// JoinHandler handles POST /worlds/:worldID/join.
type JoinHandler struct {
	pool *pgxpool.Pool
}

// NewJoinHandler creates a JoinHandler.
func NewJoinHandler(pool *pgxpool.Pool) *JoinHandler {
	return &JoinHandler{pool: pool}
}

// Join creates a province + settlement for the authenticated player in the given world.
// If a settlement already exists, returns the existing one.
func (h *JoinHandler) Join(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}
	playerID, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	// Already has a settlement in this world?
	var existingProvID uuid.UUID
	if err := h.pool.QueryRow(r.Context(),
		`SELECT province_id FROM settlements WHERE world_id = $1 AND owner_id = $2`,
		worldID, playerID,
	).Scan(&existingProvID); err == nil {
		writeJSON(w, http.StatusOK, map[string]any{"province_id": existingProvID, "existing": true})
		return
	}

	// Verify world is in a joinable state.
	var wState string
	var maxProvinces int
	if err := h.pool.QueryRow(r.Context(),
		`SELECT state, max_provinces FROM worlds WHERE id = $1`,
		worldID,
	).Scan(&wState, &maxProvinces); err != nil {
		writeError(w, http.StatusNotFound, "world not found")
		return
	}
	if wState != "forming" && wState != "active" {
		writeError(w, http.StatusConflict, "world is not accepting new players")
		return
	}

	// Count current players via settlements.
	var playerCount int
	_ = h.pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM settlements WHERE world_id = $1 AND owner_id IS NOT NULL`,
		worldID,
	).Scan(&playerCount)
	if playerCount >= maxProvinces {
		writeError(w, http.StatusConflict, "world is full — you are queued")
		return
	}

	// Decode optional preferences.
	var req struct {
		ProvinceName string `json:"province_name"`
		Culture      string `json:"culture"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	if req.Culture == "" {
		// Random culture when joining via web (no preference specified).
		cultures := []province.Culture{
			province.CultureAkhaier, province.CultureKhemetiu, province.CultureKnaani,
			province.CultureThrakes, province.CultureMinoan, province.CultureHatti,
		}
		req.Culture = string(cultures[playerCount%len(cultures)])
	}
	if req.ProvinceName == "" {
		req.ProvinceName = province.SettlementNameForCulture(req.Culture)
	}

	// Find an unclaimed tile (no province row exists yet for this tile).
	// Find a suitable spawn tile:
	// - not already a province
	// - eligible terrain
	// - at least 4 hexes from any existing settlement (no clustering)
	// - prefer the landmass with fewer settlements to balance east/west
	var q, r2 int
	var terrainType string
	var copperDeposit, tinDeposit, silverDeposit, cedarDeposit bool
	err = h.pool.QueryRow(r.Context(),
		`WITH west_count AS (
		     SELECT count(*) FROM provinces WHERE world_id = $1 AND map_q < 20
		 ),
		 east_count AS (
		     SELECT count(*) FROM provinces WHERE world_id = $1 AND map_q >= 20
		 )
		 SELECT mt.q, mt.r, mt.terrain,
		        mt.copper_deposit, mt.tin_deposit,
		        COALESCE(mt.silver_deposit, false), COALESCE(mt.cedar_deposit, false)
		 FROM map_tiles mt
		 LEFT JOIN provinces p ON p.world_id = mt.world_id AND p.map_q = mt.q AND p.map_r = mt.r
		 WHERE mt.world_id = $1
		   AND p.id IS NULL
		   AND mt.terrain IN ('plains','hills','river_valley')
		   AND NOT EXISTS (
		       SELECT 1 FROM provinces p2
		       WHERE p2.world_id = $1
		         AND (ABS(mt.q - p2.map_q) + ABS(mt.r - p2.map_r) +
		              ABS((mt.q + mt.r) - (p2.map_q + p2.map_r))) / 2 <= 4
		   )
		 ORDER BY
		   CASE
		     WHEN (SELECT count FROM west_count) <= (SELECT count FROM east_count)
		       THEN (mt.q < 20)::int
		     ELSE (mt.q >= 20)::int
		   END DESC,
		   RANDOM()
		 LIMIT 1`,
		worldID,
	).Scan(&q, &r2, &terrainType, &copperDeposit, &tinDeposit, &silverDeposit, &cedarDeposit)
	if err != nil {
		writeError(w, http.StatusConflict, "no available tiles — try again")
		return
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "transaction error")
		return
	}
	defer tx.Rollback(r.Context())

	// Create the province tile row — copy deposit flags from map_tiles.
	var provinceID uuid.UUID
	err = tx.QueryRow(r.Context(),
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type, territory_state,
		                        copper_deposit, tin_deposit, silver_deposit, cedar_deposit)
		 VALUES ($1, $2, $3, $4, 'controlled', $5, $6, $7, $8) RETURNING id`,
		worldID, q, r2, terrainType, copperDeposit, tinDeposit, silverDeposit, cedarDeposit,
	).Scan(&provinceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create province")
		return
	}

	// Create the settlement (capital). Starting population 2000 (Part B).
	// Gold is the only column resource; kharis lives on player_world_records (set below).
	var settlementID uuid.UUID
	err = tx.QueryRow(r.Context(),
		`INSERT INTO settlements
		 (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, population)
		 VALUES ($1,$2,$3,$4,$5,'capital',true,2000)
		 RETURNING id`,
		worldID, provinceID, req.ProvinceName, req.Culture, playerID,
	).Scan(&settlementID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create settlement")
		return
	}

	// Link province back to its controlling settlement.
	_, err = tx.Exec(r.Context(),
		`UPDATE provinces SET controller_id = $1 WHERE id = $2`,
		settlementID, provinceID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not link province")
		return
	}

	// Seed a zero row for every good first so the settlement always has full
	// inventory schema regardless of terrain. The production-rule UPSERT below
	// only adds rate for goods the terrain actually produces.
	_, err = tx.Exec(r.Context(),
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_at)
		 SELECT $1, g.key,
		        CASE g.key
		            WHEN 'grain'  THEN 300
		            WHEN 'timber' THEN 200
		            WHEN 'stone'  THEN 300
		            ELSE 0
		        END,
		        0,
		        CASE g.key
		            WHEN 'grain'  THEN 1000
		            WHEN 'timber' THEN 500
		            WHEN 'cedar'  THEN 500
		            WHEN 'stone'  THEN 1000
		            WHEN 'copper' THEN 300
		            WHEN 'tin'    THEN 300
		            ELSE 200
		        END,
		        now()
		 FROM goods g
		 ON CONFLICT (settlement_id, good_key) DO NOTHING`,
		settlementID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not seed goods")
		return
	}

	// Init settlement_goods from terrain-only production rules.
	// Cap is chosen per good: staples (grain) get 1000, bulk (cedar, stone) get 500-1000,
	// other goods default to 200.
	_, err = tx.Exec(r.Context(),
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_at)
		 SELECT $1, agg.good_key, 0, agg.rate,
		        CASE agg.good_key
		            WHEN 'grain'  THEN 1000
		            WHEN 'timber' THEN 500
		            WHEN 'cedar'  THEN 500
		            WHEN 'stone'  THEN 1000
		            WHEN 'copper' THEN 300
		            WHEN 'tin'    THEN 300
		            ELSE 200
		        END,
		        now()
		 FROM (
		     -- Aggregate per good_key: a terrain may match several terrain-only rules
		     -- for one good (e.g. forest + universal timber), and ON CONFLICT cannot
		     -- dedupe rows within a single INSERT statement.
		     SELECT pr.good_key, SUM(pr.rate_per_min) AS rate
		     FROM production_rules pr
		     WHERE pr.building_type IS NULL
		       AND (pr.terrain_type = $2 OR pr.terrain_type IS NULL)
		       AND (pr.requires_deposit IS NULL
		            OR (pr.requires_deposit = 'copper' AND $3)
		            OR (pr.requires_deposit = 'tin'    AND $4)
		            OR (pr.requires_deposit = 'silver' AND $5)
		            OR (pr.requires_deposit = 'cedar'  AND $6))
		     GROUP BY pr.good_key
		 ) agg
		 ON CONFLICT (settlement_id, good_key) DO UPDATE SET
		     rate = settlement_goods.rate + EXCLUDED.rate`,
		settlementID, terrainType, copperDeposit, tinDeposit, silverDeposit, cedarDeposit,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not init goods")
		return
	}

	// Sprint 5 — Catchment: add 30% of terrain production from adjacent uncontrolled tiles.
	// Uses a savepoint so a query error doesn't abort the outer transaction.
	if _, spErr := tx.Exec(r.Context(), `SAVEPOINT catchment`); spErr == nil {
		_, catchErr := tx.Exec(r.Context(),
			`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_at)
			 SELECT $1, agg.good_key, 0,
			     agg.rate,
			     CASE agg.good_key
			         WHEN 'grain'  THEN 1000 WHEN 'timber' THEN 500 WHEN 'cedar' THEN 500 WHEN 'stone' THEN 1000
			         WHEN 'copper' THEN 300  WHEN 'tin'   THEN 300 ELSE 200
			     END,
			     now()
			 FROM (
			     -- Aggregate per good_key: several adjacent tiles can produce the same
			     -- good, and ON CONFLICT cannot dedupe rows within one INSERT statement.
			     SELECT pr.good_key, SUM(pr.rate_per_min) * 0.30 AS rate
			     FROM map_tiles mt
			     JOIN production_rules pr ON pr.terrain_type = mt.terrain AND pr.building_type IS NULL
			         AND (pr.requires_deposit IS NULL
			              OR (pr.requires_deposit = 'copper' AND mt.copper_deposit)
			              OR (pr.requires_deposit = 'tin'    AND mt.tin_deposit)
			              OR (pr.requires_deposit = 'silver' AND COALESCE(mt.silver_deposit, false))
			              OR (pr.requires_deposit = 'cedar'  AND COALESCE(mt.cedar_deposit, false)))
			     WHERE mt.world_id = $2
			       AND (
			           (mt.q = $3+1 AND mt.r = $4  ) OR (mt.q = $3-1 AND mt.r = $4  ) OR
			           (mt.q = $3   AND mt.r = $4+1) OR (mt.q = $3   AND mt.r = $4-1) OR
			           (mt.q = $3+1 AND mt.r = $4-1) OR (mt.q = $3-1 AND mt.r = $4+1)
			       )
			       AND mt.terrain NOT IN ('deep_sea','coastal_sea','mountain_limestone','mountain_red','semi_desert')
			       AND NOT EXISTS (
			           SELECT 1 FROM provinces p
			           WHERE p.world_id = $2 AND p.map_q = mt.q AND p.map_r = mt.r
			             AND p.territory_state = 'controlled'
			       )
			     GROUP BY pr.good_key
			 ) agg
			 ON CONFLICT (settlement_id, good_key) DO UPDATE SET
			     amount = LEAST(EXCLUDED.cap,
			         settlement_goods.amount +
			         EXTRACT(EPOCH FROM (now() - settlement_goods.calc_at))/60 * settlement_goods.rate),
			     rate = settlement_goods.rate + EXCLUDED.rate,
			     calc_at = now()`,
			settlementID, worldID, q, r2,
		)
		if catchErr != nil {
			slog.Warn("catchment production init failed", "err", catchErr)
			_, _ = tx.Exec(r.Context(), `ROLLBACK TO SAVEPOINT catchment`)
		} else {
			_, _ = tx.Exec(r.Context(), `RELEASE SAVEPOINT catchment`)
		}
	}

	// Compute starting kharis_rate from local pantheon power.
	regions := religion.DefaultPantheonRegions()
	var maxPower float64
	for _, reg := range regions {
		if p := religion.LocalPower(reg, q, r2); p > maxPower {
			maxPower = p
		}
	}
	kharisRate := maxPower * 0.05

	// Record in player_world_records (also sets initial kharis_rate from pantheon geography).
	_, err = tx.Exec(r.Context(),
		`INSERT INTO player_world_records (player_id, world_id, settlement_id, status, kharis_rate)
		 VALUES ($1, $2, $3, 'active', $4)
		 ON CONFLICT (player_id, world_id) DO UPDATE SET
		     settlement_id = EXCLUDED.settlement_id,
		     status = 'active',
		     kharis_rate = EXCLUDED.kharis_rate`,
		playerID, worldID, settlementID, kharisRate,
	)
	if err != nil {
		slog.Error("could not record join", "err", err, "player", playerID, "world", worldID)
		writeError(w, http.StatusInternalServerError, "could not record join")
		return
	}

	// Seed initial citizen allocations over this settlement's producible goods.
	// Producible = a good with rate > 0 from the terrain/deposit init above.
	// Count producible goods first, then distribute labor_pool evenly.
	var producibleCount int
	_ = tx.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM settlement_goods sg WHERE sg.settlement_id = $1 AND sg.rate > 0`,
		settlementID,
	).Scan(&producibleCount)
	if producibleCount == 0 {
		producibleCount = 1 // floor: avoid division by zero
	}
	// labor_pool at join = population (no army yet). Starting population: 2000 (Part B).
	var startPop int
	_ = tx.QueryRow(r.Context(),
		`SELECT population FROM settlements WHERE id = $1`, settlementID,
	).Scan(&startPop)
	if startPop < 1 {
		startPop = 2000
	}
	perGood := startPop / producibleCount
	if perGood < 1 {
		perGood = 1
	}
	if _, err = tx.Exec(r.Context(),
		`INSERT INTO settlement_labor (settlement_id, good_key, citizens)
		 SELECT sg.settlement_id, sg.good_key, $2
		 FROM settlement_goods sg
		 WHERE sg.settlement_id = $1 AND sg.rate > 0
		 ON CONFLICT (settlement_id, good_key) DO NOTHING`,
		settlementID, perGood,
	); err != nil {
		slog.Error("could not seed labor citizens", "err", err)
		writeError(w, http.StatusInternalServerError, "could not seed labor")
		return
	}
	if err := economy.RecomputeProduction(r.Context(), tx, settlementID); err != nil {
		slog.Error("could not recompute production on join", "err", err)
		writeError(w, http.StatusInternalServerError, "could not init production")
		return
	}

	// Transition world to active if still forming.
	if wState == "forming" {
		_, _ = tx.Exec(r.Context(),
			`UPDATE worlds SET state = 'active' WHERE id = $1 AND state = 'forming'`,
			worldID,
		)
	}

	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "commit failed")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"province_id": provinceID,
		"tile":        world.MapTile{Q: q, R: r2},
		"culture":     req.Culture,
	})
}
