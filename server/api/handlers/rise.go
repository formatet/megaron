package handlers

import (
	"errors"
	"log/slog"
	"math/rand/v2"
	"net/http"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/world"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// When the last city falls (megaron_sista_staden.md, Timothy 2026-10-10) the
// Wanax chooses: rise again as a small host of refugees on another landmass, or
// leave this world. The account is kept either way.
const (
	risenHostMinGubbar   = 4  // 400 people
	risenHostMaxGubbar   = 8  // 800 people
	risenHostSpearmen    = 1  // one cohort, where a newcomer has two
	risenHostRationTicks = 60 // ticks of sold — "very limited silver"; a newcomer carries 240
)

// riseIntn draws the risen host's size; a test may pin it.
var riseIntn = rand.IntN

// Rise turns a dispossessed Wanax into a host again. The size is rolled once
// here and stored in founder_phase.population (events store outcomes). Grain is
// the same dowry a newcomer carries. The world's player cap does not apply: a
// returning Wanax already held a place in this world.
func (h *JoinHandler) Rise(w http.ResponseWriter, r *http.Request) {
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

	// Already risen? Answer as join does for an existing host.
	var existingHostID uuid.UUID
	if err := h.pool.QueryRow(r.Context(),
		`SELECT host_unit_id FROM founder_phase WHERE world_id = $1 AND owner_id = $2 AND active`,
		worldID, playerID,
	).Scan(&existingHostID); err == nil {
		writeJSON(w, http.StatusOK, map[string]any{"host_unit_id": existingHostID, "existing": true})
		return
	}

	var status string
	var fallenLandmass *int
	err = h.pool.QueryRow(r.Context(),
		`SELECT pwr.status, mt.landmass_id
		 FROM player_world_records pwr
		 LEFT JOIN settlements s ON s.id = pwr.last_settlement_id
		 LEFT JOIN provinces p ON p.id = s.province_id
		 LEFT JOIN map_tiles mt ON mt.world_id = pwr.world_id AND mt.q = p.map_q AND mt.r = p.map_r
		 WHERE pwr.player_id = $1 AND pwr.world_id = $2`,
		playerID, worldID,
	).Scan(&status, &fallenLandmass)
	if err != nil || status != "dispossessed" {
		writeError(w, http.StatusConflict, "only a Wanax who has lost every city can rise again")
		return
	}

	var mapWidth int
	if err := h.pool.QueryRow(r.Context(),
		`SELECT map_width FROM worlds WHERE id = $1`, worldID).Scan(&mapWidth); err != nil {
		writeError(w, http.StatusNotFound, "world not found")
		return
	}
	halfQ := (mapWidth - 1) / 2

	// Another landmass first; if none has room, anywhere a newcomer could stand.
	q, r2, err := pickSpawnTile(r.Context(), h.pool, worldID, halfQ, fallenLandmass)
	if errors.Is(err, pgx.ErrNoRows) && fallenLandmass != nil {
		slog.Info("rise: no free land on another landmass, falling back", "player", playerID, "landmass", *fallenLandmass)
		q, r2, err = pickSpawnTile(r.Context(), h.pool, worldID, halfQ, nil)
	}
	if err != nil {
		writeError(w, http.StatusConflict, "no free land remains in this world for a host to stand on")
		return
	}

	spec := hostSpec{
		population:  100 * (risenHostMinGubbar + riseIntn(risenHostMaxGubbar-risenHostMinGubbar+1)),
		spearmen:    risenHostSpearmen,
		rationTicks: risenHostRationTicks,
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "transaction error")
		return
	}
	defer tx.Rollback(r.Context())

	// Lock the record so two presses cannot seed two hosts.
	var locked string
	if err := tx.QueryRow(r.Context(),
		`SELECT status FROM player_world_records WHERE player_id = $1 AND world_id = $2 FOR UPDATE`,
		playerID, worldID,
	).Scan(&locked); err != nil || locked != "dispossessed" {
		writeError(w, http.StatusConflict, "only a Wanax who has lost every city can rise again")
		return
	}

	hostID, err := seedHost(r.Context(), tx, h.eventStore, worldID, playerID, q, r2, spec)
	if err != nil {
		slog.Error("rise: could not seed host", "err", err, "player", playerID, "world", worldID)
		writeError(w, http.StatusInternalServerError, "could not gather the survivors")
		return
	}
	// last_settlement_id is kept: it is the fallen city, and only the next fall replaces it.
	if _, err := tx.Exec(r.Context(),
		`UPDATE player_world_records SET status = 'active' WHERE player_id = $1 AND world_id = $2`,
		playerID, worldID,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "could not record the rising")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "commit failed")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"host_unit_id": hostID,
		"tile":         world.MapTile{Q: q, R: r2},
		"population":   spec.population,
	})
}

// Leave ends a dispossessed Wanax's part in this world. The account stays; the
// next world is open to them. Idempotent.
func (h *JoinHandler) Leave(w http.ResponseWriter, r *http.Request) {
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
	tag, err := h.pool.Exec(r.Context(),
		`UPDATE player_world_records SET status = 'departed'
		 WHERE player_id = $1 AND world_id = $2 AND status IN ('dispossessed', 'departed')`,
		playerID, worldID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not leave the world")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusConflict, "only a Wanax who has lost every city can leave the world")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "departed"})
}
