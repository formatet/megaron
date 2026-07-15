package handlers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/poleia/server/internal/auth"
	"github.com/poleia/server/internal/economy"
	"github.com/poleia/server/internal/events"
	"github.com/poleia/server/internal/province"
	"github.com/poleia/server/internal/world"
)

// joinStartingPopulation is the population an ordinary join lands with (W1).
// The Nomadic Host founds with its own carried civilians instead — see
// capitalParams.Population.
const joinStartingPopulation = 5000

// JoinHandler handles POST /worlds/:worldID/join.
type JoinHandler struct {
	pool       *pgxpool.Pool
	eventStore *events.Store
	sitosCfg   economy.SitosConfig
}

// NewJoinHandler creates a JoinHandler.
func NewJoinHandler(pool *pgxpool.Pool, eventStore *events.Store, sitosCfg economy.SitosConfig) *JoinHandler {
	return &JoinHandler{pool: pool, eventStore: eventStore, sitosCfg: sitosCfg}
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

	// Verify world is in a joinable state; also read map_width so we can
	// compute the hemisphere boundary (half_q = (map_width-1)/2) for the
	// ore-catchment spawn bias further down.
	var wState string
	var maxProvinces, mapWidth int
	if err := h.pool.QueryRow(r.Context(),
		`SELECT state, max_provinces, map_width FROM worlds WHERE id = $1`,
		worldID,
	).Scan(&wState, &maxProvinces, &mapWidth); err != nil {
		writeError(w, http.StatusNotFound, "world not found")
		return
	}
	// half_q mirrors the validateMap/mapgen convention: west hemisphere is q <= halfQ,
	// east hemisphere is q > halfQ. Copper lives in the west, tin in the east.
	halfQ := (mapWidth - 1) / 2
	if wState != "forming" && wState != "active" {
		writeError(w, http.StatusConflict, "world is not accepting new players")
		return
	}

	// Count current players via DISTINCT owners — not settlement rows. A Wanax holds
	// many settlements (colonies), so COUNT(*) would falsely report "full" once colonies
	// outnumber max_provinces even with few actual players.
	var playerCount int
	_ = h.pool.QueryRow(r.Context(),
		`SELECT COUNT(DISTINCT owner_id) FROM settlements WHERE world_id = $1 AND owner_id IS NOT NULL`,
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
	// - tiebreak: prefer tiles whose 6-hex catchment contains the hemisphere's
	//   ore (copper for west q<=halfQ, tin for east q>halfQ) so early joiners
	//   land on ore-catchment tiles and produce ore from turn 1.
	var q, r2 int
	var terrainType string
	var copperDeposit, tinDeposit, silverDeposit, cedarDeposit, tileCoastal bool
	err = h.pool.QueryRow(r.Context(),
		`WITH west_count AS (
		     SELECT count(*) FROM provinces WHERE world_id = $1 AND map_q <= $2
		 ),
		 east_count AS (
		     SELECT count(*) FROM provinces WHERE world_id = $1 AND map_q > $2
		 )
		 SELECT mt.q, mt.r, mt.terrain,
		        mt.copper_deposit, mt.tin_deposit,
		        COALESCE(mt.silver_deposit, false), COALESCE(mt.cedar_deposit, false),
		        COALESCE(mt.coastal, false)
		 FROM map_tiles mt
		 LEFT JOIN provinces p ON p.world_id = mt.world_id AND p.map_q = mt.q AND p.map_r = mt.r
		 WHERE mt.world_id = $1
		   AND p.id IS NULL
		   AND mt.terrain NOT IN ('coastal_sea','deep_sea','mountain_limestone','mountain_red','semi_desert')
		   AND NOT EXISTS (
		       SELECT 1 FROM provinces p2
		       WHERE p2.world_id = $1
		         AND (ABS(mt.q - p2.map_q) + ABS(mt.r - p2.map_r) +
		              ABS((mt.q + mt.r) - (p2.map_q + p2.map_r))) / 2 <= 4
		   )
		   -- Self-sufficiency invariant: the starter catchment (the 6 adjacent
		   -- hexes RecomputeProduction reads) must contain at least one real
		   -- grain tile (plains/river_valley) so the capital can feed a basic
		   -- army without trade. hills grain (0.01/min) is a trickle, not food.
		   AND EXISTS (
		       SELECT 1 FROM map_tiles g
		       WHERE g.world_id = mt.world_id
		         AND g.terrain IN ('plains','river_valley')
		         AND ((g.q = mt.q+1 AND g.r = mt.r  ) OR (g.q = mt.q-1 AND g.r = mt.r  ) OR
		              (g.q = mt.q   AND g.r = mt.r+1) OR (g.q = mt.q   AND g.r = mt.r-1) OR
		              (g.q = mt.q+1 AND g.r = mt.r-1) OR (g.q = mt.q-1 AND g.r = mt.r+1))
		   )
		 ORDER BY
		   -- 1. Hemisphere balance: fill the side with fewer settlements first.
		   CASE
		     WHEN (SELECT count FROM west_count) <= (SELECT count FROM east_count)
		       THEN (mt.q <= $2)::int
		     ELSE (mt.q > $2)::int
		   END DESC,
		   -- 2. Ore-catchment bias (tiebreak within the winning hemisphere):
		   --    west tiles that have a copper-deposit neighbour rank ahead of those
		   --    that do not; east tiles prefer tin-deposit neighbours. This ensures
		   --    the first joiners land on ore-catchment tiles so they mine from
		   --    turn 1 — the self-sufficiency invariant is preserved because the
		   --    viability filters above still gate every candidate tile.
		   --    When no ore-catchment tile is eligible the bias is 0 for all and
		   --    we fall back to RANDOM() as before.
		   CASE
		     WHEN mt.q <= $2 THEN (
		       EXISTS (
		         SELECT 1 FROM map_tiles nb
		         WHERE nb.world_id = mt.world_id
		           AND nb.copper_deposit = true
		           AND ((nb.q = mt.q+1 AND nb.r = mt.r  ) OR (nb.q = mt.q-1 AND nb.r = mt.r  ) OR
		                (nb.q = mt.q   AND nb.r = mt.r+1) OR (nb.q = mt.q   AND nb.r = mt.r-1) OR
		                (nb.q = mt.q+1 AND nb.r = mt.r-1) OR (nb.q = mt.q-1 AND nb.r = mt.r+1))
		       )::int
		     )
		     ELSE (
		       EXISTS (
		         SELECT 1 FROM map_tiles nb
		         WHERE nb.world_id = mt.world_id
		           AND nb.tin_deposit = true
		           AND ((nb.q = mt.q+1 AND nb.r = mt.r  ) OR (nb.q = mt.q-1 AND nb.r = mt.r  ) OR
		                (nb.q = mt.q   AND nb.r = mt.r+1) OR (nb.q = mt.q   AND nb.r = mt.r-1) OR
		                (nb.q = mt.q+1 AND nb.r = mt.r-1) OR (nb.q = mt.q-1 AND nb.r = mt.r+1))
		       )::int
		     )
		   END DESC,
		   RANDOM()
		 LIMIT 1`,
		worldID, halfQ,
	).Scan(&q, &r2, &terrainType, &copperDeposit, &tinDeposit, &silverDeposit, &cedarDeposit, &tileCoastal)
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

	// Raise the capital: province, settlement, opening stores, Sitos seeds,
	// starter buildings, labor and the first production pass. Shared with the
	// Nomadic Host's founding transaction so the two cannot drift apart.
	cap, err := createCapital(r.Context(), tx, h.sitosCfg, capitalParams{
		WorldID:  worldID,
		PlayerID: playerID,
		Q:        q,
		R:        r2,
		Terrain:  terrainType,
		Copper:   copperDeposit,
		Tin:      tinDeposit,
		Silver:   silverDeposit,
		Cedar:    cedarDeposit,
		Coastal:  tileCoastal,
		Name:     req.ProvinceName,
		Culture:  req.Culture,
		Population: joinStartingPopulation,
	})
	if err != nil {
		slog.Error("join: could not create capital", "err", err, "player", playerID, "world", worldID)
		msg := "could not create capital"
		var ce *capitalError
		if errors.As(err, &ce) {
			msg = ce.UserMessage()
		}
		writeError(w, http.StatusInternalServerError, msg)
		return
	}
	provinceID, settlementID := cap.ProvinceID, cap.SettlementID

	// C7: create starter units for the new settlement.
	// Coast tile (respawn path) → 1 galley + 1 infantry garrison.
	// Inland tile (join path today) → 2 infantry garrisons.
	// Men drawn from population are accounted for inside seedStarterUnits.
	if err := seedStarterUnits(r.Context(), tx, h.eventStore, settlementID, playerID, worldID, q, r2, tileCoastal); err != nil {
		slog.Error("could not seed starter units", "err", err, "settlement", settlementID)
		writeError(w, http.StatusInternalServerError, "could not seed starter units")
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
