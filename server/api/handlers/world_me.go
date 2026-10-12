package handlers

import (
	"net/http"

	"formatet/megaron/server/internal/auth"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// WorldProfile is the caller's five-field profile for one joined world.
type WorldProfile struct {
	WanaxName   string  `json:"wanax_name"`
	FoundedTick *int    `json:"founded_tick"`
	Cities      int     `json:"cities"`
	Population  int     `json:"population"`
	Kharis      float64 `json:"kharis"`
}

// Me handles GET /worlds/{worldID}/me. Identity comes exclusively from Bearer
// authentication; neither a query nor a path can select another Wanax.
func (h *WorldHandler) Me(w http.ResponseWriter, r *http.Request) {
	playerID, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}
	var profile WorldProfile
	err = h.pool.QueryRow(r.Context(), `
		SELECT COALESCE(p.wanax_name, p.username),
		       CASE WHEN fp.owner_id IS NOT NULL AND fp.founded_tick IS NULL
		            THEN NULL ELSE pwr.founded_tick END
		FROM player_world_records pwr
		JOIN players p ON p.id = pwr.player_id
		LEFT JOIN founder_phase fp ON fp.owner_id = pwr.player_id AND fp.world_id = pwr.world_id
		WHERE pwr.player_id = $1 AND pwr.world_id = $2`, playerID, worldID,
	).Scan(&profile.WanaxName, &profile.FoundedTick)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "not joined to this world")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load profile")
		return
	}
	// Match the owned active settlements in SettlementsOverview, including
	// empty realms after collapse and Hosts that have not founded yet.
	err = h.pool.QueryRow(r.Context(), `
		SELECT COUNT(*), COALESCE(SUM(population), 0)
		FROM settlements
		WHERE owner_id = $1 AND world_id = $2 AND state = 'active'`, playerID, worldID,
	).Scan(&profile.Cities, &profile.Population)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load profile settlements")
		return
	}
	kharis, err := loadPlayerKharis(r.Context(), h.pool, playerID, worldID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load profile kharis")
		return
	}
	profile.Kharis = kharis.Amount
	writeJSON(w, http.StatusOK, profile)
}
