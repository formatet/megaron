package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/tick"
	"formatet/megaron/server/internal/unit"
	"formatet/megaron/server/internal/world"
)

// poseidonGalleyCrew is the crew of the gift galley, drawn from the new
// metropolis's population. It mirrors the coastal starter galley
// (starter_units.go) rather than UnitSpecs["galley"].PopCost — that field is an
// affordability gate and a catalogue figure, never a population deduction.
const poseidonGalleyCrew = 20

// Errors the founding transaction can refuse with. The handler maps them to
// status codes; they are named so callers cannot mistake one refusal for another.
var (
	errNoFounderPhase  = errors.New("no active founder phase")
	errAlreadySettled  = errors.New("player already holds a metropolis")
	errHexNotSettlable = errors.New("hex cannot be settled")
)

// foundedMetropolis reports what the founding produced, for the notice.
type foundedMetropolis struct {
	ProvinceID    uuid.UUID
	SettlementID  uuid.UUID
	Q, R          int
	Coastal       bool
	GalleyID      *uuid.UUID // Poseidon's gift; nil inland
	GrainCarried  float64    // store moved into the city
	SilverCarried float64
}

// foundMetropolisFromNomadicHost turns a wandering host into a player's first and
// only metropolis, on the hex it currently stands on. It runs inside the caller's
// transaction and does NOT commit.
//
// The host dissolves permanently in the act — that is the whole shape of the
// founder phase: one irreversible choice of where to become a people with a place.
// Everything the host still carried becomes the city's opening stock, so a player
// who found their site early is rewarded with a fuller granary rather than having
// the surplus evaporate.
//
// Grain = 0 does NOT block founding: an immobilised host may still settle where it
// stands (temenos_nomadic_host_plan.md §Förbrukning). Starving in place with no way
// out would be a trap, not a deadline.
func foundMetropolisFromNomadicHost(
	ctx context.Context,
	tx pgx.Tx,
	eventStore *events.Store,
	sitosCfg economy.SitosConfig,
	worldID, playerID uuid.UUID,
	name, culture string,
) (foundedMetropolis, error) {
	var out foundedMetropolis

	// 1. Guard: an active founder phase, and the host still on the map.
	var (
		phaseID uuid.UUID
		hostID  uuid.UUID
		pop     int
		q, r    int
	)
	err := tx.QueryRow(ctx,
		`SELECT fp.id, fp.host_unit_id, fp.population, u.q, u.r
		 FROM founder_phase fp
		 JOIN units u ON u.id = fp.host_unit_id
		 WHERE fp.world_id = $1 AND fp.owner_id = $2 AND fp.active
		   AND u.q IS NOT NULL AND u.r IS NOT NULL`,
		worldID, playerID,
	).Scan(&phaseID, &hostID, &pop, &q, &r)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, errNoFounderPhase
	}
	if err != nil {
		return out, fmt.Errorf("load founder phase: %w", err)
	}

	// Belt and braces against a second metropolis: founder_phase.active is the
	// real one-shot flag, but a player who somehow holds a settlement must never
	// mint another capital row through this path.
	var existing int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM settlements WHERE world_id = $1 AND owner_id = $2`,
		worldID, playerID,
	).Scan(&existing); err != nil {
		return out, fmt.Errorf("check existing settlements: %w", err)
	}
	if existing > 0 {
		return out, errAlreadySettled
	}

	// 2. The hex must still be free and land. The host can be standing anywhere it
	// walked, including next to someone who settled first while it was en route.
	var terrain string
	var copperDep, tinDep, silverDep, cedarDep, coastal bool
	err = tx.QueryRow(ctx,
		`SELECT mt.terrain, mt.copper_deposit, mt.tin_deposit,
		        COALESCE(mt.silver_deposit,false), COALESCE(mt.cedar_deposit,false),
		        COALESCE(mt.coastal,false)
		 FROM map_tiles mt
		 LEFT JOIN provinces p ON p.world_id = mt.world_id AND p.map_q = mt.q AND p.map_r = mt.r
		 WHERE mt.world_id = $1 AND mt.q = $2 AND mt.r = $3
		   AND p.id IS NULL
		   AND mt.terrain NOT IN ('coastal_sea','deep_sea','mountain_limestone','mountain_red','semi_desert')`,
		worldID, q, r,
	).Scan(&terrain, &copperDep, &tinDep, &silverDep, &cedarDep, &coastal)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, errHexNotSettlable
	}
	if err != nil {
		return out, fmt.Errorf("load founding tile: %w", err)
	}

	// 3. Settle the store BEFORE anything else writes: settled() derives from the
	// current tick, and the rows we are about to touch would otherwise re-anchor it.
	// Clamped at zero — a host that ran out does not owe the new city a debt.
	var grainLeft, silverLeft float64
	if err := tx.QueryRow(ctx,
		`SELECT GREATEST(0, settled(grain_amount, grain_rate, calc_tick)),
		        GREATEST(0, settled(silver_amount, silver_rate, calc_tick))
		 FROM founder_phase WHERE id = $1`,
		phaseID,
	).Scan(&grainLeft, &silverLeft); err != nil {
		return out, fmt.Errorf("settle founder store: %w", err)
	}

	// 4. Raise the metropolis through the same path an ordinary join uses.
	m, err := createMetropolis(ctx, tx, sitosCfg, metropolisParams{
		WorldID:    worldID,
		PlayerID:   playerID,
		Q:          q,
		R:          r,
		Terrain:    terrain,
		Copper:     copperDep,
		Tin:        tinDep,
		Silver:     silverDep,
		Cedar:      cedarDep,
		Coastal:    coastal,
		Name:       name,
		Culture:    culture,
		Population: pop,
	})
	if err != nil {
		return out, err
	}

	// 5. The escort comes home: the cohorts become an ordinary garrison, and
	// combat.UpkeepHandler takes them over the moment the phase goes inactive
	// (step 7). From here they draw pay and eat from the city like any other unit.
	if _, err := tx.Exec(ctx,
		`UPDATE units SET settlement_id = $1, status = 'garrison', q = NULL, r = NULL
		 WHERE world_id = $2 AND owner_id = $3 AND id <> $4
		   AND status = 'positioned' AND q = $5 AND r = $6`,
		m.SettlementID, worldID, playerID, hostID, q, r,
	); err != nil {
		return out, fmt.Errorf("attach escort to metropolis: %w", err)
	}

	// 6. Pour the remaining rations into the city's stores. ADD rather than set:
	// createMetropolis already seeded opening amounts, and the host's grain is
	// carried in on top of them.
	for good, amount := range map[string]float64{"grain": grainLeft, "silver": silverLeft} {
		if amount <= 0 {
			continue
		}
		if _, err := tx.Exec(ctx,
			`UPDATE settlement_goods
			 SET amount = settled(amount, rate, calc_tick) + $1, calc_tick = current_world_tick()
			 WHERE settlement_id = $2 AND good_key = $3`,
			amount, m.SettlementID, good,
		); err != nil {
			return out, fmt.Errorf("carry %s into metropolis: %w", good, err)
		}
	}

	// 7. The host dissolves, permanently. active=false + founded_tick is the design's
	// one-shot flag: the row stays as the record of how this player began.
	if _, err := tx.Exec(ctx,
		`UPDATE units SET status = 'disbanded', q = NULL, r = NULL WHERE id = $1`, hostID,
	); err != nil {
		return out, fmt.Errorf("dissolve host: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`UPDATE founder_phase SET active = false, founded_tick = current_world_tick(), host_unit_id = NULL
		 WHERE id = $1`, phaseID,
	); err != nil {
		return out, fmt.Errorf("close founder phase: %w", err)
	}

	// 8. Poseidon's gift: a coastal founding is owed exactly one galley, once.
	// Built here rather than via seedStarterUnits — that function's coastal branch
	// also raises a spearman, and this player already marched in with two.
	if coastal {
		var galleyID uuid.UUID
		if err := tx.QueryRow(ctx,
			`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, settlement_id)
			 VALUES ($1, $2, $3, $4, 1, $5, 'garrison', $6)
			 RETURNING id`,
			worldID, playerID, string(unit.TypeGalley), string(unit.CategoryNaval),
			poseidonGalleyCrew, m.SettlementID,
		).Scan(&galleyID); err != nil {
			return out, fmt.Errorf("poseidon galley: %w", err)
		}
		// Her crew are citizens: the city is that many hands short in the fields.
		if _, err := tx.Exec(ctx,
			`UPDATE settlements SET population = population - $1 WHERE id = $2`,
			poseidonGalleyCrew, m.SettlementID,
		); err != nil {
			return out, fmt.Errorf("poseidon galley crew: %w", err)
		}
		if _, err := eventStore.Append(ctx, galleyID, events.StreamType(unit.StreamUnit),
			unit.EventUnitFormed, unit.UnitFormedPayload{
				UnitID:       galleyID,
				OwnerID:      playerID,
				WorldID:      worldID,
				SettlementID: m.SettlementID,
				UnitType:     string(unit.TypeGalley),
				Category:     string(unit.CategoryNaval),
				InitialSize:  1,
				Crew:         poseidonGalleyCrew,
				PopDrawn:     poseidonGalleyCrew,
			}, worldID, nil,
		); err != nil {
			return out, fmt.Errorf("append UnitFormed for poseidon galley: %w", err)
		}
		out.GalleyID = &galleyID

		// The crew left after RecomputeProduction ran inside createMetropolis, so
		// the opening rates were computed on a population that is now 20 smaller.
		// Recompute so the city's first reading is honest.
		if err := economy.RecomputeProduction(ctx, tx, m.SettlementID); err != nil {
			return out, fmt.Errorf("recompute after poseidon galley: %w", err)
		}
	}

	out.ProvinceID, out.SettlementID = m.ProvinceID, m.SettlementID
	out.Q, out.R, out.Coastal = q, r, coastal
	out.GrainCarried, out.SilverCarried = grainLeft, silverLeft
	return out, nil
}

// Settle handles POST /worlds/{worldID}/founding/settle — the Nomadic Host
// becomes a metropolis where it stands.
//
// The forecast the player saw before confirming is served separately by
// /colonize-preview (shared surface, temenos_nomadic_host_bygg.md Fas 4); this
// endpoint is the irreversible half and re-checks every guard itself. The host may
// have walked, starved or been beaten to the hex since the preview was drawn.
func (h *JoinHandler) Settle(w http.ResponseWriter, r *http.Request) {
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
		Name    string `json:"name"`
		Culture string `json:"culture"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "transaction error")
		return
	}
	defer tx.Rollback(r.Context())

	// Culture is fixed at join in the ordinary path; the founder phase has none
	// stored yet, so a settle without one falls back the same way join does.
	if req.Culture == "" {
		req.Culture = string(province.CultureAkhaier)
	}
	if req.Name == "" {
		req.Name = province.SettlementNameForCulture(req.Culture)
	}

	founded, err := foundMetropolisFromNomadicHost(
		r.Context(), tx, h.eventStore, h.sitosCfg, worldID, playerID, req.Name, req.Culture,
	)
	switch {
	case errors.Is(err, errNoFounderPhase):
		writeError(w, http.StatusConflict, "you have no wandering host to settle")
		return
	case errors.Is(err, errAlreadySettled):
		writeError(w, http.StatusConflict, "you already hold a metropolis")
		return
	case errors.Is(err, errHexNotSettlable):
		writeError(w, http.StatusUnprocessableEntity,
			"this ground cannot be settled — it is taken, or it is sea or mountain")
		return
	case err != nil:
		slog.Error("settle: found metropolis failed", "err", err, "player", playerID, "world", worldID)
		msg := "could not found metropolis"
		var me *metropolisError
		if errors.As(err, &me) {
			msg = me.UserMessage()
		}
		writeError(w, http.StatusInternalServerError, msg)
		return
	}

	// The real opening grain balance, read back inside the TX: RecomputeProduction
	// already wrote the net rate, and the host's carried grain is poured in — this
	// is what the notice reports, never a re-derivation. (Same pattern as
	// ColonyFounded, internal/combat/unit_arrival.go.)
	var grainAmount, grainNet float64
	_ = tx.QueryRow(r.Context(),
		`SELECT amount, rate FROM settlement_goods
		 WHERE settlement_id = $1 AND good_key = 'grain'`,
		founded.SettlementID,
	).Scan(&grainAmount, &grainNet)

	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "commit failed")
		return
	}

	// MetropolisFounded — the design's founding notice (grain balance, carried
	// stock, known/unknown catchment, Poseidon status). Sent only after a
	// successful commit; nil-guarded like every hub site.
	if h.hub != nil {
		known, unknownHexes := countKnownCatchment(r.Context(), h.pool, h.clk, worldID, playerID, founded.Q, founded.R)
		payload := map[string]any{
			"settlement_id":      founded.SettlementID,
			"province_id":        founded.ProvinceID,
			"name":               req.Name,
			"q":                  founded.Q,
			"r":                  founded.R,
			"coastal":            founded.Coastal,
			"grain_amount":       grainAmount,
			"grain_net_per_tick": grainNet,
			"grain_carried":      founded.GrainCarried,
			"silver_carried":     founded.SilverCarried,
			"known_hexes":        known,
			"unknown_hexes":      unknownHexes,
		}
		if founded.GalleyID != nil {
			payload["poseidon_gift"] = *founded.GalleyID
		}
		// grain_days: how long the stock lasts at the current deficit; omitted
		// when the metropolis is self-sustaining (net ≥ 0).
		if grainNet < 0 {
			if dailyDrain := -grainNet * float64(events.TicksPerDay); dailyDrain > 0 {
				payload["grain_days"] = grainAmount / dailyDrain
			}
		}
		_ = h.hub.NotifyPlayer(r.Context(), worldID, playerID, "MetropolisFounded", 2, payload)
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"province_id":    founded.ProvinceID,
		"settlement_id":  founded.SettlementID,
		"tile":           world.MapTile{Q: founded.Q, R: founded.R},
		"coastal":        founded.Coastal,
		"poseidon_gift":  founded.GalleyID,
		"grain_carried":  founded.GrainCarried,
		"silver_carried": founded.SilverCarried,
	})
}

// FoundingStatus handles GET /worlds/{worldID}/founding/status — the founder
// phase read surface for the web Host panel and keryx `founding status`. Store
// amounts are settled at read (lazy tuple, mig 086); ticks_left is derived from
// amount/-rate, never stored, and clients derive game-day and real-time ETAs
// from ticks (B2: never a stored wall clock). tick_seconds rides along so a
// client can convert ticks to real time without knowing the server's cadence.
func (h *JoinHandler) FoundingStatus(w http.ResponseWriter, r *http.Request) {
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

	var (
		hostID                *uuid.UUID
		q, r2                 *int
		population            int
		grainAmt, grainRate   float64
		silverAmt, silverRate float64
		currentTick           int
	)
	err = h.pool.QueryRow(r.Context(),
		`SELECT fp.host_unit_id, u.q, u.r, fp.population,
		        GREATEST(0, settled(fp.grain_amount, fp.grain_rate, fp.calc_tick)), fp.grain_rate,
		        GREATEST(0, settled(fp.silver_amount, fp.silver_rate, fp.calc_tick)), fp.silver_rate,
		        current_world_tick()
		 FROM founder_phase fp
		 LEFT JOIN units u ON u.id = fp.host_unit_id
		 WHERE fp.world_id = $1 AND fp.owner_id = $2 AND fp.active`,
		worldID, playerID,
	).Scan(&hostID, &q, &r2, &population, &grainAmt, &grainRate, &silverAmt, &silverRate, &currentTick)
	if err != nil {
		// No active phase — settled (or never a founder). Not an error: the panel
		// and keryx both key off active=false to show nothing.
		writeJSON(w, http.StatusOK, map[string]any{"active": false})
		return
	}

	var spearmenInField int
	_ = h.pool.QueryRow(r.Context(),
		`SELECT count(*) FROM units
		 WHERE world_id = $1 AND owner_id = $2 AND type = 'spearman'
		   AND status IN ('positioned', 'marching')`,
		worldID, playerID,
	).Scan(&spearmenInField)

	store := func(amount, rate float64) map[string]any {
		s := map[string]any{"amount": amount, "rate_per_tick": rate}
		if rate < 0 {
			s["ticks_left"] = int(amount / -rate)
		}
		return s
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"active":            true,
		"host_unit_id":      hostID,
		"q":                 q,
		"r":                 r2,
		"population":        population,
		"spearmen_in_field": spearmenInField,
		"current_tick":      currentTick,
		"tick_seconds":      tick.TickSeconds,
		"grain":             store(grainAmt, grainRate),
		"silver":            store(silverAmt, silverRate),
	})
}

// countKnownCatchment reports how many of a founding hex's 7 catchment tiles the
// player has actually seen (live vision ∪ remembered memory — the same tiering as
// Map and ColonizePreview), for the founding notice. FOW-safe by construction: it
// returns counts only, never a fogged tile's contents.
func countKnownCatchment(ctx context.Context, pool *pgxpool.Pool, clk clock.Clock, worldID, playerID uuid.UUID, q, r int) (known, unknown int) {
	// Centre + 6 neighbours — SAME axial offsets as economy.RecomputeProduction
	// and ColonizePreview; keep them identical.
	catchment := [][2]int{
		{q, r},
		{q + 1, r}, {q - 1, r},
		{q, r + 1}, {q, r - 1},
		{q + 1, r - 1}, {q - 1, r + 1},
	}
	eyes := loadLiveEyes(ctx, pool, worldID, playerID, clk.Now())
	remembered := loadRememberedTiles(ctx, pool, worldID, playerID)

	qs := make([]int32, len(catchment))
	rs := make([]int32, len(catchment))
	for i, c := range catchment {
		qs[i], rs[i] = int32(c[0]), int32(c[1])
	}
	terrains := map[[2]int]string{}
	rows, err := pool.Query(ctx,
		`SELECT mt.q, mt.r, mt.terrain
		 FROM map_tiles mt
		 JOIN unnest($2::int[], $3::int[]) AS hx(q, r) ON hx.q = mt.q AND hx.r = mt.r
		 WHERE mt.world_id = $1`,
		worldID, qs, rs,
	)
	if err != nil {
		return 0, len(catchment)
	}
	defer rows.Close()
	for rows.Next() {
		var tq, tr int
		var terrain string
		if rows.Scan(&tq, &tr, &terrain) == nil {
			terrains[[2]int{tq, tr}] = terrain
		}
	}

	for _, c := range catchment {
		key := [2]int{c[0], c[1]}
		terrain, exists := terrains[key]
		if exists && (province.AnyEyeSees(eyes, province.MapPosition{Q: c[0], R: c[1]}, terrain) || remembered[key]) {
			known++
		} else {
			unknown++
		}
	}
	return known, unknown
}
