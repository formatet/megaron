package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/poleia/server/internal/auth"
	"github.com/poleia/server/internal/clock"
	"github.com/poleia/server/internal/economy"
	"github.com/poleia/server/internal/events"
	"github.com/poleia/server/internal/notify"
	"github.com/poleia/server/internal/province"
	"github.com/poleia/server/internal/world"
)

// joinStartingPopulation is the population an ordinary join lands with (W1).
// The Nomadic Host founds with its own carried civilians instead — see
// metropolisParams.Population.
const joinStartingPopulation = 5000

// JoinHandler handles POST /worlds/:worldID/join.
type JoinHandler struct {
	pool       *pgxpool.Pool
	eventStore *events.Store
	sitosCfg   economy.SitosConfig
	clk        clock.Clock
	hub        *notify.Hub // nil-guarded; carries the MetropolisFounded notice
}

// NewJoinHandler creates a JoinHandler.
func NewJoinHandler(pool *pgxpool.Pool, eventStore *events.Store, sitosCfg economy.SitosConfig, clk clock.Clock, hub *notify.Hub) *JoinHandler {
	return &JoinHandler{pool: pool, eventStore: eventStore, sitosCfg: sitosCfg, clk: clk, hub: hub}
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

	// Already wandering as a host? Same idempotency guarantee as above — a player
	// gets exactly one founder phase per world, ever (founder_phase's unique key
	// enforces it; this is the friendly answer rather than a constraint violation).
	var existingHostID uuid.UUID
	if err := h.pool.QueryRow(r.Context(),
		`SELECT host_unit_id FROM founder_phase
		 WHERE world_id = $1 AND owner_id = $2 AND active`,
		worldID, playerID,
	).Scan(&existingHostID); err == nil {
		writeJSON(w, http.StatusOK, map[string]any{"host_unit_id": existingHostID, "existing": true})
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
		// Occupancy counts BOTH settled provinces and wandering hosts: a host that
		// has not founded yet is occupied ground too. Counting only provinces would
		// compare 0 against 0 for the whole founder phase and pile every player into
		// one valley — and worlds will later mix the two, when players spawn a host
		// into an already-inhabited world (Timothy 2026-07-15).
		`WITH hosts AS (
		     SELECT hu.q, hu.r
		     FROM units hu
		     JOIN founder_phase fp ON fp.host_unit_id = hu.id AND fp.active
		     WHERE hu.world_id = $1 AND hu.q IS NOT NULL AND hu.r IS NOT NULL
		 ),
		 west_count AS (
		     SELECT (SELECT count(*) FROM provinces WHERE world_id = $1 AND map_q <= $2)
		          + (SELECT count(*) FROM hosts WHERE q <= $2) AS count
		 ),
		 east_count AS (
		     SELECT (SELECT count(*) FROM provinces WHERE world_id = $1 AND map_q > $2)
		          + (SELECT count(*) FROM hosts WHERE q > $2) AS count
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
		   -- Keep clear of settled ground …
		   AND NOT EXISTS (
		       SELECT 1 FROM provinces p2
		       WHERE p2.world_id = $1
		         AND (ABS(mt.q - p2.map_q) + ABS(mt.r - p2.map_r) +
		              ABS((mt.q + mt.r) - (p2.map_q + p2.map_r))) / 2 <= 4
		   )
		   -- … and of other hosts, by the same measure.
		   AND NOT EXISTS (
		       SELECT 1 FROM hosts h
		       WHERE (ABS(mt.q - h.q) + ABS(mt.r - h.r) +
		              ABS((mt.q + mt.r) - (h.q + h.r))) / 2 <= 4
		   )
		   -- NOTE: the old "starter catchment must hold a grain tile" filter is
		   -- deliberately gone. It was a self-sufficiency invariant for a capital
		   -- born where it lands; a host carries four months of rations and is
		   -- meant to go looking for its site. Pre-picking fertile ground would
		   -- answer the question the founder phase exists to ask. The grain check
		   -- lives in the founding forecast instead
		   -- (temenos_nomadic_host_plan.md §Spawn, §Platsprognos).
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

	// A new player arrives as a Nomadic Host: a people on the move with four
	// months of rations, looking for somewhere to become a city. No settlement and
	// no province are created here — those are born at founding, on a hex the
	// player chose (temenos_nomadic_host_plan.md). createMetropolis stands ready for
	// that founding transaction; join no longer calls it.
	hostID, err := seedNomadicHost(r.Context(), tx, h.eventStore, worldID, playerID, q, r2)
	if err != nil {
		slog.Error("join: could not seed nomadic host", "err", err, "player", playerID, "world", worldID)
		writeError(w, http.StatusInternalServerError, "could not create nomadic host")
		return
	}

	// Record the player as active with no settlement yet — settlement_id is
	// nullable and stays NULL until founding fills it in. kharis_rate keeps its
	// default: it is derived from the pantheon geography of the capital's hex,
	// which does not exist yet.
	if _, err := tx.Exec(r.Context(),
		`INSERT INTO player_world_records (player_id, world_id, status)
		 VALUES ($1, $2, 'active')
		 ON CONFLICT (player_id, world_id) DO UPDATE SET status = 'active'`,
		playerID, worldID,
	); err != nil {
		slog.Error("could not record join", "err", err, "player", playerID, "world", worldID)
		writeError(w, http.StatusInternalServerError, "could not record join")
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
		"host_unit_id": hostID,
		"tile":         world.MapTile{Q: q, R: r2},
		"culture":      req.Culture,
		"population":   nomadicHostPopulation,
	})
}
