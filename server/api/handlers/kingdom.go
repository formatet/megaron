package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/poleia/server/internal/ai"
	"github.com/poleia/server/internal/auth"
	"github.com/poleia/server/internal/clock"
)

// KingdomHandler handles HTTP requests for kingdom endpoints.
type KingdomHandler struct {
	pool *pgxpool.Pool
	clk  clock.Clock
}

// NewKingdomHandler creates a KingdomHandler.
func NewKingdomHandler(pool *pgxpool.Pool, clk clock.Clock) *KingdomHandler {
	return &KingdomHandler{pool: pool, clk: clk}
}

// List handles GET /worlds/:worldID/kingdoms.
func (h *KingdomHandler) List(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}

	rows, err := h.pool.Query(r.Context(),
		`SELECT k.id, k.name, k.prestige, COUNT(km.player_id) AS member_count, k.created_at
		 FROM kingdoms k
		 LEFT JOIN kingdom_members km ON km.kingdom_id = k.id
		 WHERE k.world_id = $1
		 GROUP BY k.id ORDER BY k.prestige DESC`,
		worldID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list kingdoms")
		return
	}
	defer rows.Close()

	type kSummary struct {
		ID          uuid.UUID `json:"id"`
		Name        string    `json:"name"`
		Prestige    int       `json:"prestige"`
		MemberCount int       `json:"member_count"`
		CreatedAt   time.Time `json:"created_at"`
	}
	var kingdoms []kSummary
	for rows.Next() {
		var k kSummary
		if err := rows.Scan(&k.ID, &k.Name, &k.Prestige, &k.MemberCount, &k.CreatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "scan error")
			return
		}
		kingdoms = append(kingdoms, k)
	}
	if kingdoms == nil {
		kingdoms = []kSummary{}
	}
	writeJSON(w, http.StatusOK, kingdoms)
}

// Found handles POST /worlds/:worldID/kingdoms.
func (h *KingdomHandler) Found(w http.ResponseWriter, r *http.Request) {
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

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		writeError(w, http.StatusBadRequest, "kingdom name required")
		return
	}

	// Verify player has a capital settlement and is not already in a kingdom.
	var settlementID uuid.UUID
	var existingKingdom *uuid.UUID
	err = h.pool.QueryRow(r.Context(),
		`SELECT id, kingdom_id FROM settlements
		 WHERE world_id = $1 AND owner_id = $2 AND is_capital = true AND state = 'active'`,
		worldID, playerID,
	).Scan(&settlementID, &existingKingdom)
	if err != nil {
		writeError(w, http.StatusForbidden, "no active settlement in this world")
		return
	}
	if existingKingdom != nil {
		writeError(w, http.StatusConflict, "already a member of a kingdom")
		return
	}

	var kingdomID uuid.UUID
	err = h.pool.QueryRow(r.Context(),
		`INSERT INTO kingdoms (world_id, name) VALUES ($1, $2) RETURNING id`,
		worldID, req.Name,
	).Scan(&kingdomID)
	if err != nil {
		writeError(w, http.StatusConflict, "kingdom name already taken")
		return
	}

	_, err = h.pool.Exec(r.Context(),
		`INSERT INTO kingdom_members (kingdom_id, player_id, role) VALUES ($1, $2, 'basileus')`,
		kingdomID, playerID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not add founder")
		return
	}

	_, err = h.pool.Exec(r.Context(),
		`UPDATE settlements SET kingdom_id = $1 WHERE id = $2`,
		kingdomID, settlementID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not link settlement to kingdom")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"id": kingdomID, "name": req.Name})
}

// Invite handles POST /worlds/:worldID/kingdoms/:kingdomID/invite.
func (h *KingdomHandler) Invite(w http.ResponseWriter, r *http.Request) {
	kingdomID, err := uuid.Parse(chi.URLParam(r, "kingdomID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid kingdom ID")
		return
	}
	playerID, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	// Only king may invite (general role removed with new model).
	var role string
	err = h.pool.QueryRow(r.Context(),
		`SELECT role FROM kingdom_members WHERE kingdom_id = $1 AND player_id = $2`,
		kingdomID, playerID,
	).Scan(&role)
	if err != nil || role != "basileus" {
		writeError(w, http.StatusForbidden, "only the basileus may invite")
		return
	}

	var req struct {
		ProvinceID string `json:"province_id"` // province tile ID of the invitee's capital
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	targetProvinceID, err := uuid.Parse(req.ProvinceID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid province ID")
		return
	}

	_, err = h.pool.Exec(r.Context(),
		`INSERT INTO kingdom_invitations (kingdom_id, province_id, invited_by, expires_at)
		 VALUES ($1, $2, $3, now() + interval '48 hours')`,
		kingdomID, targetProvinceID, playerID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create invitation")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Join handles POST /worlds/:worldID/kingdoms/:kingdomID/join.
func (h *KingdomHandler) Join(w http.ResponseWriter, r *http.Request) {
	kingdomID, err := uuid.Parse(chi.URLParam(r, "kingdomID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid kingdom ID")
		return
	}
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

	// Find player's capital settlement that isn't already in a kingdom.
	var settlementID uuid.UUID
	var provinceID uuid.UUID
	err = h.pool.QueryRow(r.Context(),
		`SELECT id, province_id FROM settlements
		 WHERE world_id = $1 AND owner_id = $2 AND state = 'active' AND kingdom_id IS NULL AND is_capital = true`,
		worldID, playerID,
	).Scan(&settlementID, &provinceID)
	if err != nil {
		writeError(w, http.StatusForbidden, "no eligible settlement")
		return
	}

	// Verify valid invitation by province_id.
	var inviteID uuid.UUID
	err = h.pool.QueryRow(r.Context(),
		`SELECT id FROM kingdom_invitations
		 WHERE kingdom_id = $1 AND province_id = $2
		   AND accepted_at IS NULL AND expires_at > now()`,
		kingdomID, provinceID,
	).Scan(&inviteID)
	if err != nil {
		writeError(w, http.StatusForbidden, "no valid invitation")
		return
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "transaction error")
		return
	}
	defer tx.Rollback(r.Context())

	_, err = tx.Exec(r.Context(),
		`INSERT INTO kingdom_members (kingdom_id, player_id, role) VALUES ($1, $2, 'member')`,
		kingdomID, playerID,
	)
	if err != nil {
		writeError(w, http.StatusConflict, "already a member")
		return
	}
	_, _ = tx.Exec(r.Context(),
		`UPDATE settlements SET kingdom_id = $1 WHERE id = $2`,
		kingdomID, settlementID,
	)
	_, _ = tx.Exec(r.Context(),
		`UPDATE kingdom_invitations SET accepted_at = now() WHERE id = $1`,
		inviteID,
	)

	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "commit failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Leave handles DELETE /worlds/:worldID/kingdoms/:kingdomID/leave.
func (h *KingdomHandler) Leave(w http.ResponseWriter, r *http.Request) {
	kingdomID, err := uuid.Parse(chi.URLParam(r, "kingdomID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid kingdom ID")
		return
	}
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

	var role string
	err = h.pool.QueryRow(r.Context(),
		`SELECT role FROM kingdom_members WHERE kingdom_id = $1 AND player_id = $2`,
		kingdomID, playerID,
	).Scan(&role)
	if err != nil {
		writeError(w, http.StatusNotFound, "not a member")
		return
	}
	if role == "basileus" {
		writeError(w, http.StatusConflict, "basileus must abdicate or disband kingdom before leaving")
		return
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "transaction error")
		return
	}
	defer tx.Rollback(r.Context())

	_, _ = tx.Exec(r.Context(),
		`DELETE FROM kingdom_members WHERE kingdom_id = $1 AND player_id = $2`,
		kingdomID, playerID,
	)
	// Clear kingdom_id from all player's settlements in this kingdom.
	_, _ = tx.Exec(r.Context(),
		`UPDATE settlements SET kingdom_id = NULL
		 WHERE world_id = $1 AND owner_id = $2 AND kingdom_id = $3`,
		worldID, playerID, kingdomID,
	)

	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "commit failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Invitations handles GET /worlds/:worldID/kingdoms/invitations.
// Returns pending kingdom invitations for the current player's settlement.
func (h *KingdomHandler) Invitations(w http.ResponseWriter, r *http.Request) {
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

	rows, err := h.pool.Query(r.Context(),
		`SELECT ki.id, ki.kingdom_id, k.name, p.username, ki.expires_at
		 FROM kingdom_invitations ki
		 JOIN kingdoms k ON k.id = ki.kingdom_id
		 JOIN players p ON p.id = ki.invited_by
		 JOIN settlements s ON s.province_id = ki.province_id
		 WHERE s.world_id = $1 AND s.owner_id = $2
		   AND ki.accepted_at IS NULL AND ki.expires_at > now()
		 ORDER BY ki.expires_at`,
		worldID, playerID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load invitations")
		return
	}
	defer rows.Close()

	type inv struct {
		ID         uuid.UUID `json:"id"`
		KingdomID  uuid.UUID `json:"kingdom_id"`
		KingdomName string   `json:"kingdom_name"`
		InvitedBy  string   `json:"invited_by"`
		ExpiresAt  time.Time `json:"expires_at"`
	}
	var result []inv
	for rows.Next() {
		var i inv
		if err := rows.Scan(&i.ID, &i.KingdomID, &i.KingdomName, &i.InvitedBy, &i.ExpiresAt); err == nil {
			result = append(result, i)
		}
	}
	if result == nil {
		result = []inv{}
	}
	writeJSON(w, http.StatusOK, result)
}

// Council handles GET /worlds/:worldID/kingdoms/:kingdomID/council.
func (h *KingdomHandler) Council(w http.ResponseWriter, r *http.Request) {
	kingdomID, err := uuid.Parse(chi.URLParam(r, "kingdomID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid kingdom ID")
		return
	}

	rows, err := h.pool.Query(r.Context(),
		`SELECT km.player_id, p.username, km.role, km.joined_at
		 FROM kingdom_members km
		 JOIN players p ON p.id = km.player_id
		 WHERE km.kingdom_id = $1 ORDER BY km.joined_at`,
		kingdomID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load council")
		return
	}
	defer rows.Close()

	type member struct {
		PlayerID  uuid.UUID `json:"player_id"`
		Username  string    `json:"username"`
		Role      string    `json:"role"`
		JoinedAt  time.Time `json:"joined_at"`
	}
	var members []member
	for rows.Next() {
		var m member
		if err := rows.Scan(&m.PlayerID, &m.Username, &m.Role, &m.JoinedAt); err != nil {
			continue
		}
		members = append(members, m)
	}
	if members == nil {
		members = []member{}
	}
	writeJSON(w, http.StatusOK, members)
}

// BorrowArmy handles POST /worlds/:worldID/kingdoms/:kingdomID/borrow-army.
// The king requests an army from a member settlement. Units are removed from the
// settlement and recorded in borrowed_armies until returned.
func (h *KingdomHandler) BorrowArmy(w http.ResponseWriter, r *http.Request) {
	kingdomID, err := uuid.Parse(chi.URLParam(r, "kingdomID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid kingdom ID")
		return
	}
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

	// Caller must be king of this kingdom.
	var role string
	err = h.pool.QueryRow(r.Context(),
		`SELECT role FROM kingdom_members WHERE kingdom_id = $1 AND player_id = $2`,
		kingdomID, playerID,
	).Scan(&role)
	if err != nil || role != "basileus" {
		writeError(w, http.StatusForbidden, "only the basileus may borrow armies")
		return
	}

	var req struct {
		LenderPlayerID string `json:"lender_player_id"`
		Infantry       int    `json:"infantry"`
		Cavalry        int    `json:"cavalry"`
		Catapult        int    `json:"catapult"`
		Priest         int    `json:"priest"`
		Ship           int    `json:"ship"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	lenderID, err := uuid.Parse(req.LenderPlayerID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid lender player ID")
		return
	}
	if req.Infantry < 0 || req.Cavalry < 0 || req.Catapult < 0 || req.Priest < 0 || req.Ship < 0 {
		writeError(w, http.StatusBadRequest, "unit counts must be non-negative")
		return
	}
	if req.Infantry+req.Cavalry+req.Catapult+req.Priest+req.Ship == 0 {
		writeError(w, http.StatusBadRequest, "must borrow at least one unit")
		return
	}

	// Lender must be a member of this kingdom.
	var lenderRole string
	err = h.pool.QueryRow(r.Context(),
		`SELECT role FROM kingdom_members WHERE kingdom_id = $1 AND player_id = $2`,
		kingdomID, lenderID,
	).Scan(&lenderRole)
	if err != nil {
		writeError(w, http.StatusForbidden, "lender is not a kingdom member")
		return
	}

	// Find lender's capital settlement in this world.
	var settlementID uuid.UUID
	err = h.pool.QueryRow(r.Context(),
		`SELECT id FROM settlements
		 WHERE world_id = $1 AND owner_id = $2 AND is_capital = true`,
		worldID, lenderID,
	).Scan(&settlementID)
	if err != nil {
		writeError(w, http.StatusNotFound, "lender has no capital in this world")
		return
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "transaction error")
		return
	}
	defer tx.Rollback(r.Context())

	// Deduct units from lender's settlement — fails if insufficient.
	tag, err := tx.Exec(r.Context(),
		`UPDATE settlements SET
		   infantry = infantry - $1,
		   cavalry  = cavalry  - $2,
		   catapult = catapult - $3,
		   priest   = priest   - $4,
		   ship     = ship     - $5
		 WHERE id = $6
		   AND infantry >= $1
		   AND cavalry  >= $2
		   AND catapult >= $3
		   AND priest   >= $4
		   AND ship     >= $5`,
		req.Infantry, req.Cavalry, req.Catapult, req.Priest, req.Ship, settlementID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not deduct units")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusUnprocessableEntity, "insufficient units in lender's settlement")
		return
	}

	// Record the borrowed army.
	var borrowID uuid.UUID
	err = tx.QueryRow(r.Context(),
		`INSERT INTO borrowed_armies (kingdom_id, lender_id, infantry, cavalry, catapult, priest, ship)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING id`,
		kingdomID, lenderID, req.Infantry, req.Cavalry, req.Catapult, req.Priest, req.Ship,
	).Scan(&borrowID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not record borrowed army")
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "commit failed")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"id":       borrowID,
		"infantry": req.Infantry,
		"cavalry":  req.Cavalry,
		"catapult": req.Catapult,
		"priest":   req.Priest,
		"ship":     req.Ship,
	})
}

// AssignRole handles PATCH /worlds/:worldID/kingdoms/:kingdomID/council/:role.
func (h *KingdomHandler) AssignRole(w http.ResponseWriter, r *http.Request) {
	kingdomID, err := uuid.Parse(chi.URLParam(r, "kingdomID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid kingdom ID")
		return
	}
	targetRole := chi.URLParam(r, "role")
	playerID, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var actorRole string
	err = h.pool.QueryRow(r.Context(),
		`SELECT role FROM kingdom_members WHERE kingdom_id = $1 AND player_id = $2`,
		kingdomID, playerID,
	).Scan(&actorRole)
	if err != nil || actorRole != "basileus" {
		writeError(w, http.StatusForbidden, "only the basileus may assign roles")
		return
	}

	var req struct {
		PlayerID string `json:"player_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	targetPlayerID, err := uuid.Parse(req.PlayerID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid player ID")
		return
	}

	_, err = h.pool.Exec(r.Context(),
		`UPDATE kingdom_members SET role = $1 WHERE kingdom_id = $2 AND player_id = $3`,
		targetRole, kingdomID, targetPlayerID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not assign role")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// CallElection handles POST /worlds/:worldID/kingdoms/:kingdomID/election.
// Any member may call an election, but only on a Sunday and only when the
// kingdom lock (7 days post-election) has expired.
func (h *KingdomHandler) CallElection(w http.ResponseWriter, r *http.Request) {
	kingdomID, err := uuid.Parse(chi.URLParam(r, "kingdomID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid kingdom ID")
		return
	}
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

	// Caller must be a member.
	err = h.pool.QueryRow(r.Context(),
		`SELECT role FROM kingdom_members WHERE kingdom_id = $1 AND player_id = $2`,
		kingdomID, playerID,
	).Scan(new(string))
	if err != nil {
		writeError(w, http.StatusForbidden, "not a kingdom member")
		return
	}

	// Elections only on Sundays.
	if h.clk.Now().UTC().Weekday() != time.Sunday {
		writeError(w, http.StatusUnprocessableEntity, "elections can only be called on a Sunday")
		return
	}

	// Check kingdom lock.
	var kingLocked *time.Time
	err = h.pool.QueryRow(r.Context(),
		`SELECT king_locked_until FROM kingdoms WHERE id = $1`,
		kingdomID,
	).Scan(&kingLocked)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load kingdom")
		return
	}
	if kingLocked != nil && h.clk.Now().Before(*kingLocked) {
		writeError(w, http.StatusConflict, "kingdom is locked until "+kingLocked.Format(time.RFC3339))
		return
	}

	var req struct {
		CandidateID string `json:"candidate_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	candidateID, err := uuid.Parse(req.CandidateID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid candidate ID")
		return
	}

	// Candidate must be a member.
	var candidateExists bool
	_ = h.pool.QueryRow(r.Context(),
		`SELECT EXISTS (SELECT 1 FROM kingdom_members WHERE kingdom_id = $1 AND player_id = $2)`,
		kingdomID, candidateID,
	).Scan(&candidateExists)
	if !candidateExists {
		writeError(w, http.StatusUnprocessableEntity, "candidate is not a kingdom member")
		return
	}

	closesAt := h.clk.Now().UTC().Truncate(24 * time.Hour).Add(24 * time.Hour)
	var electionID uuid.UUID
	err = h.pool.QueryRow(r.Context(),
		`INSERT INTO kingdom_elections (kingdom_id, world_id, candidate_id, called_by, closes_at)
		 VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		kingdomID, worldID, candidateID, playerID, closesAt,
	).Scan(&electionID)
	if err != nil {
		writeError(w, http.StatusConflict, "election already in progress or could not create")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"election_id":  electionID,
		"candidate_id": candidateID,
		"closes_at":    closesAt,
	})
}

// Vote handles POST /worlds/:worldID/kingdoms/:kingdomID/vote.
// Each member casts one vote. When all members have voted, the election resolves.
// Divine intervention is applied per ai.DivineInterventionProbability.
func (h *KingdomHandler) Vote(w http.ResponseWriter, r *http.Request) {
	kingdomID, err := uuid.Parse(chi.URLParam(r, "kingdomID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid kingdom ID")
		return
	}
	playerID, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	// Caller must be a member.
	err = h.pool.QueryRow(r.Context(),
		`SELECT role FROM kingdom_members WHERE kingdom_id = $1 AND player_id = $2`,
		kingdomID, playerID,
	).Scan(new(string))
	if err != nil {
		writeError(w, http.StatusForbidden, "not a kingdom member")
		return
	}

	// Find open election.
	var electionID uuid.UUID
	err = h.pool.QueryRow(r.Context(),
		`SELECT id FROM kingdom_elections
		 WHERE kingdom_id = $1 AND resolved_at IS NULL AND closes_at > now()
		 ORDER BY called_at DESC LIMIT 1`,
		kingdomID,
	).Scan(&electionID)
	if err != nil {
		writeError(w, http.StatusNotFound, "no open election")
		return
	}

	var req struct {
		CandidateID string `json:"candidate_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	voteCandidateID, err := uuid.Parse(req.CandidateID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid candidate ID")
		return
	}

	_, err = h.pool.Exec(r.Context(),
		`INSERT INTO kingdom_votes (election_id, voter_id, candidate_id) VALUES ($1, $2, $3)`,
		electionID, playerID, voteCandidateID,
	)
	if err != nil {
		writeError(w, http.StatusConflict, "already voted in this election")
		return
	}

	// Check if all members have voted → resolve.
	var memberCount, voteCount int
	_ = h.pool.QueryRow(r.Context(),
		`SELECT
		   (SELECT COUNT(*) FROM kingdom_members WHERE kingdom_id = $1),
		   (SELECT COUNT(*) FROM kingdom_votes WHERE election_id = $2)`,
		kingdomID, electionID,
	).Scan(&memberCount, &voteCount)

	resolved := false
	if voteCount >= memberCount {
		resolved = h.resolveElection(r.Context(), electionID, kingdomID)
	}

	writeJSON(w, http.StatusOK, map[string]any{"voted": true, "resolved": resolved})
}

// resolveElection tallies votes, applies divine intervention, crowns winner,
// locks kingdom for 7 days.
func (h *KingdomHandler) resolveElection(ctx context.Context, electionID, kingdomID uuid.UUID) bool {
	var winnerID uuid.UUID
	err := h.pool.QueryRow(ctx,
		`SELECT candidate_id FROM kingdom_votes
		 WHERE election_id = $1
		 GROUP BY candidate_id ORDER BY COUNT(*) DESC LIMIT 1`,
		electionID,
	).Scan(&winnerID)
	if err != nil {
		return false
	}

	// Divine intervention check based on winner's kharis.
	var kharis int
	_ = h.pool.QueryRow(ctx,
		`SELECT COALESCE(CAST(
		   GREATEST(0,
		     kharis_amount + (EXTRACT(EPOCH FROM (now() - kharis_calc_at))/60 * kharis_rate)
		   ) AS INT
		 ), 0)
		 FROM settlements
		 WHERE owner_id = $1 AND is_capital = true
		 LIMIT 1`,
		winnerID,
	).Scan(&kharis)

	divineOverride := false
	p := ai.DivineInterventionProbability(kharis)
	if p > 0 {
		divineOverride = (kharis % 100) < int(p*100)
	}

	if divineOverride {
		var altWinner uuid.UUID
		err = h.pool.QueryRow(ctx,
			`SELECT candidate_id FROM kingdom_votes
			 WHERE election_id = $1
			 GROUP BY candidate_id ORDER BY COUNT(*) ASC LIMIT 1`,
			electionID,
		).Scan(&altWinner)
		if err == nil && altWinner != winnerID {
			winnerID = altWinner
		}
	}

	lockedUntil := h.clk.Now().UTC().Add(7 * 24 * time.Hour)

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return false
	}
	defer tx.Rollback(ctx)

	_, _ = tx.Exec(ctx,
		`UPDATE kingdom_members SET role = 'basileus'
		 WHERE kingdom_id = $1 AND player_id = $2`,
		kingdomID, winnerID,
	)
	_, _ = tx.Exec(ctx,
		`UPDATE kingdom_members SET role = 'member'
		 WHERE kingdom_id = $1 AND player_id != $2 AND role = 'basileus'`,
		kingdomID, winnerID,
	)
	_, _ = tx.Exec(ctx,
		`UPDATE kingdoms SET king_locked_until = $1 WHERE id = $2`,
		lockedUntil, kingdomID,
	)
	_, _ = tx.Exec(ctx,
		`UPDATE kingdom_elections
		 SET resolved_at = now(), winner_id = $1, divine_override = $2
		 WHERE id = $3`,
		winnerID, divineOverride, electionID,
	)

	return tx.Commit(ctx) == nil
}

// ElectionStatus handles GET /worlds/:worldID/kingdoms/:kingdomID/election.
// Returns the current open election, or {"active":false} if none.
func (h *KingdomHandler) ElectionStatus(w http.ResponseWriter, r *http.Request) {
	kingdomID, err := uuid.Parse(chi.URLParam(r, "kingdomID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid kingdom ID")
		return
	}

	var electionID uuid.UUID
	var candidateID uuid.UUID
	var candidateName string
	var calledBy uuid.UUID
	var closesAt time.Time
	var voteCount, memberCount int
	err = h.pool.QueryRow(r.Context(),
		`SELECT ke.id, ke.candidate_id, p.username, ke.called_by, ke.closes_at,
		        (SELECT COUNT(*) FROM kingdom_votes WHERE election_id = ke.id),
		        (SELECT COUNT(*) FROM kingdom_members WHERE kingdom_id = ke.kingdom_id)
		 FROM kingdom_elections ke
		 JOIN players p ON p.id = ke.candidate_id
		 WHERE ke.kingdom_id = $1 AND ke.resolved_at IS NULL AND ke.closes_at > now()
		 ORDER BY ke.called_at DESC LIMIT 1`,
		kingdomID,
	).Scan(&electionID, &candidateID, &candidateName, &calledBy, &closesAt, &voteCount, &memberCount)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"active": false})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"active":         true,
		"election_id":    electionID,
		"candidate_id":   candidateID,
		"candidate_name": candidateName,
		"called_by":      calledBy,
		"closes_at":      closesAt,
		"vote_count":     voteCount,
		"member_count":   memberCount,
	})
}

// BorrowedArmiesList handles GET /worlds/:worldID/kingdoms/:kingdomID/borrowed-armies.
// Returns all unreturned borrowed armies for this kingdom.
func (h *KingdomHandler) BorrowedArmiesList(w http.ResponseWriter, r *http.Request) {
	kingdomID, err := uuid.Parse(chi.URLParam(r, "kingdomID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid kingdom ID")
		return
	}

	rows, err := h.pool.Query(r.Context(),
		`SELECT ba.id, ba.lender_id, p.username,
		        ba.infantry, ba.cavalry, ba.catapult, ba.priest, ba.ship, ba.borrowed_at
		 FROM borrowed_armies ba
		 JOIN players p ON p.id = ba.lender_id
		 WHERE ba.kingdom_id = $1 AND ba.returned_at IS NULL
		 ORDER BY ba.borrowed_at`,
		kingdomID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load borrowed armies")
		return
	}
	defer rows.Close()

	type entry struct {
		ID         uuid.UUID `json:"id"`
		LenderID   uuid.UUID `json:"lender_id"`
		LenderName string    `json:"lender_name"`
		Infantry   int       `json:"infantry"`
		Cavalry    int       `json:"cavalry"`
		Catapult   int       `json:"catapult"`
		Priest     int       `json:"priest"`
		Ship       int       `json:"ship"`
		BorrowedAt time.Time `json:"borrowed_at"`
	}
	var result []entry
	for rows.Next() {
		var e entry
		if err := rows.Scan(&e.ID, &e.LenderID, &e.LenderName,
			&e.Infantry, &e.Cavalry, &e.Catapult, &e.Priest, &e.Ship, &e.BorrowedAt); err == nil {
			result = append(result, e)
		}
	}
	if result == nil {
		result = []entry{}
	}
	writeJSON(w, http.StatusOK, result)
}
