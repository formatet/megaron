package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"formatet/megaron/server/internal/auth"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/hexgrid"
	"formatet/megaron/server/internal/province"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// parsePositiveInt parses a URL path param as a positive integer (gubbe
// ordinals are always ≥1, settlement_placement's own CHECK constraint).
func parsePositiveInt(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return 0, strconv.ErrSyntax
	}
	return n, nil
}

// placementSettlement loads and ownership-checks the settlement behind a
// provinceID URL param, plus the province's own hex — shared setup for all
// three placement endpoints. Mirrors LaborAlloc's ownership query.
func (h *ProvinceHandler) placementSettlement(r *http.Request, worldID, provinceID, playerID uuid.UUID) (settlementID uuid.UUID, population int, center hexgrid.Coord, ok bool) {
	var q, r2 int
	if err := h.pool.QueryRow(r.Context(),
		`SELECT s.id, s.population, p.map_q, p.map_r
		 FROM settlements s JOIN provinces p ON p.id = s.province_id
		 WHERE s.province_id = $1 AND s.world_id = $2 AND s.owner_id = $3`,
		provinceID, worldID, playerID,
	).Scan(&settlementID, &population, &q, &r2); err != nil {
		return uuid.Nil, 0, hexgrid.Coord{}, false
	}
	return settlementID, population, hexgrid.Coord{Q: q, R: r2}, true
}

// Placements handles GET /worlds/:worldID/provinces/:provinceID/placements —
// every placed gubbe plus the derived pool size, for the (unbuilt) P5 stadsvy
// and any interim client to read.
func (h *ProvinceHandler) Placements(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}
	provinceID, err := uuid.Parse(chi.URLParam(r, "provinceID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid province ID")
		return
	}
	playerID, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	settlementID, population, center, ok := h.placementSettlement(r, worldID, provinceID, playerID)
	if !ok {
		writeError(w, http.StatusForbidden, "not your settlement")
		return
	}

	rows, err := h.pool.Query(r.Context(),
		`SELECT gubbe_ordinal, target_kind, hex_q, hex_r, building_type, good_key
		 FROM settlement_placement WHERE settlement_id = $1 ORDER BY gubbe_ordinal`,
		settlementID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load placements")
		return
	}
	type placementOut struct {
		GubbeOrdinal int     `json:"gubbe_ordinal"`
		TargetKind   string  `json:"target_kind"`
		HexQ         *int    `json:"hex_q,omitempty"`
		HexR         *int    `json:"hex_r,omitempty"`
		HexOrdinal   *int    `json:"hex_ordinal,omitempty"`
		BuildingType *string `json:"building_type,omitempty"`
		GoodKey      string  `json:"good_key"`
	}
	var out []placementOut
	for rows.Next() {
		var p placementOut
		var hexQ, hexR *int
		if err := rows.Scan(&p.GubbeOrdinal, &p.TargetKind, &hexQ, &hexR, &p.BuildingType, &p.GoodKey); err != nil {
			rows.Close()
			writeError(w, http.StatusInternalServerError, "could not read placement")
			return
		}
		if hexQ != nil && hexR != nil {
			p.HexQ, p.HexR = hexQ, hexR
			if ord, found := hexgrid.RingOrdinal(center, hexgrid.CatchmentRadius, hexgrid.Coord{Q: *hexQ, R: *hexR}); found {
				p.HexOrdinal = &ord
			}
		}
		out = append(out, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "could not read placements")
		return
	}

	totalGubbar := population / 100
	writeJSON(w, http.StatusOK, map[string]any{
		"placements":   out,
		"total_gubbar": totalGubbar,
		"pool_size":    totalGubbar - len(out),
	})
}

// PlacementOptions handles GET
// /worlds/:worldID/provinces/:provinceID/placement-options — the per-hex and
// per-building production menu (P4's LoadHexProductionOptions /
// LoadBuildingProductionOptions), merged with current occupancy so a client
// never has to duplicate placementYield's math. This is the data source for
// P5's stadsvy grid AND `keryx city` — hex math (Ring/RingOrdinal) stays
// server-side; a client only ever sees hex_ordinal 1..18, matching the
// address space P0-UI answer 7 locked in. FOW-gated: an unrevealed catchment
// hex is simply absent from the response (P0-UI §Tvärgående lås: "FOW-hex =
// svart = icke-placerbar, även i rastret") — the grid always has 18 fixed
// ordinal slots, so a client renders any missing ordinal as fog.
func (h *ProvinceHandler) PlacementOptions(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}
	provinceID, err := uuid.Parse(chi.URLParam(r, "provinceID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid province ID")
		return
	}
	playerID, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	settlementID, population, center, ok := h.placementSettlement(r, worldID, provinceID, playerID)
	if !ok {
		writeError(w, http.StatusForbidden, "not your settlement")
		return
	}

	placed, err := economy.LoadPlacementCounts(r.Context(), h.pool, settlementID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load placements")
		return
	}
	placedOrdinals, err := loadPlacedOrdinals(r.Context(), h.pool, settlementID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load placements")
		return
	}

	hexOptions, err := economy.LoadHexProductionOptions(r.Context(), h.pool, settlementID, nil) // menu listing — siege denial gates YIELD, not the placement UI
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load catchment")
		return
	}
	eyes := loadLiveEyes(r.Context(), h.pool, worldID, playerID, h.clk.Now())
	remembered := loadRememberedTiles(r.Context(), h.pool, worldID, playerID)

	type goodOut struct {
		GoodKey        string  `json:"good_key"`
		RatePerTick    float64 `json:"rate_per_tick"`
		Cap            *int    `json:"cap,omitempty"` // absent/null = no P3 hexCapacityRule/WorkplaceSlots entry for this good at all
		Placed         int     `json:"placed"`
		PlacedOrdinals []int   `json:"placed_ordinals,omitempty"`
		MarginalYield  float64 `json:"marginal_yield"`
	}
	buildGoods := func(rate map[string]float64, cap map[string]int, placedGoods map[string]int, ordinalsGoods map[string][]int) []goodOut {
		out := make([]goodOut, 0, len(rate))
		for good, rate := range rate {
			g := goodOut{
				GoodKey:        good,
				RatePerTick:    rate,
				Placed:         placedGoods[good],
				PlacedOrdinals: ordinalsGoods[good],
			}
			if c := cap[good]; c > 0 {
				g.Cap = &c
				// Grain keeps placementYield's rate × placed shape (not
				// rate/cap × placed like every other good) — see
				// megaron_plan_grain_cap.md and placementYield's doc comment.
				// It IS capped now, just not capacity-divided.
				if good == economy.GoodGrain {
					g.MarginalYield = rate
				} else {
					g.MarginalYield = rate / float64(c)
				}
			}
			out = append(out, g)
		}
		return out
	}

	type hexOut struct {
		HexQ       int       `json:"hex_q"`
		HexR       int       `json:"hex_r"`
		HexOrdinal int       `json:"hex_ordinal"`
		Terrain    string    `json:"terrain"`
		Goods      []goodOut `json:"goods"`
	}
	hexes := make([]hexOut, 0, len(hexOptions))
	for _, opt := range hexOptions {
		if !knownToPlayer(eyes, remembered, province.MapPosition{Q: opt.Coord.Q, R: opt.Coord.R}, opt.Terrain) {
			continue
		}
		ordinal, found := hexgrid.RingOrdinal(center, hexgrid.CatchmentRadius, opt.Coord)
		if !found {
			continue
		}
		hexes = append(hexes, hexOut{
			HexQ:       opt.Coord.Q,
			HexR:       opt.Coord.R,
			HexOrdinal: ordinal,
			Terrain:    opt.Terrain,
			Goods:      buildGoods(opt.RatePerGood, opt.CapPerGood, placed.Hex[opt.Coord], placedOrdinals.Hex[opt.Coord]),
		})
	}

	buildingOptions, err := economy.LoadBuildingProductionOptions(r.Context(), h.pool, settlementID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load buildings")
		return
	}
	type buildingOut struct {
		BuildingType string    `json:"building_type"`
		Level        int       `json:"level"`
		Goods        []goodOut `json:"goods"`
	}
	buildings := make([]buildingOut, 0, len(buildingOptions))
	for _, opt := range buildingOptions {
		buildings = append(buildings, buildingOut{
			BuildingType: opt.BuildingType,
			Level:        opt.Level,
			Goods:        buildGoods(opt.RatePerGood, opt.CapPerGood, placed.Building[opt.BuildingType], placedOrdinals.Building[opt.BuildingType]),
		})
	}

	totalGubbar := population / 100
	writeJSON(w, http.StatusOK, map[string]any{
		"hexes":        hexes,
		"buildings":    buildings,
		"total_gubbar": totalGubbar,
		"pool_size":    totalGubbar - placed.Total,
	})
}

// placedOrdinals mirrors economy.PlacementCounts but keeps the actual gubbe
// ordinals (not just a count) — a client needs a concrete ordinal to issue
// DELETE .../placements/{ordinal} when it removes one gubbe from a specific
// hex/good or building/good.
type placedOrdinals struct {
	Hex      map[hexgrid.Coord]map[string][]int
	Building map[string]map[string][]int
}

func loadPlacedOrdinals(ctx context.Context, tx economy.Tx, settlementID uuid.UUID) (placedOrdinals, error) {
	out := placedOrdinals{
		Hex:      make(map[hexgrid.Coord]map[string][]int),
		Building: make(map[string]map[string][]int),
	}
	rows, err := tx.Query(ctx,
		`SELECT gubbe_ordinal, target_kind, hex_q, hex_r, building_type, good_key
		 FROM settlement_placement WHERE settlement_id = $1`,
		settlementID,
	)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var gubbeOrdinal int
		var kind, goodKey string
		var hexQ, hexR *int
		var buildingType *string
		if err := rows.Scan(&gubbeOrdinal, &kind, &hexQ, &hexR, &buildingType, &goodKey); err != nil {
			return out, err
		}
		switch kind {
		case "hex":
			c := hexgrid.Coord{Q: *hexQ, R: *hexR}
			if out.Hex[c] == nil {
				out.Hex[c] = make(map[string][]int)
			}
			out.Hex[c][goodKey] = append(out.Hex[c][goodKey], gubbeOrdinal)
		case "building":
			if out.Building[*buildingType] == nil {
				out.Building[*buildingType] = make(map[string][]int)
			}
			out.Building[*buildingType][goodKey] = append(out.Building[*buildingType][goodKey], gubbeOrdinal)
		}
	}
	return out, rows.Err()
}

// PlaceGubbe handles POST /worlds/:worldID/provinces/:provinceID/placements —
// place the next free gubbe (server-assigned ordinal, lowest available; P0-UI
// answer 2: the Wanax picks WHERE, not which numbered gubbe) on a hex or
// building slot. Validates ownership, FOW ("avslöjad" — P0-UI answer 5/§Tvärgående
// lås: a fog hex is never placeable), that the target/good combination is a
// real production option, and capacity — grain included, since
// megaron_plan_grain_cap.md (2026-08-22): a hard per-hex cap on EVERY good,
// grain no longer exempt. Recomputes production
// immediately (P0-UI: "Placering slår igenom omedelbart"). Accepts either
// hex_q/hex_r or hex_ordinal (1..18, P0-UI answer 7's address space) — the
// latter keeps Ring() math server-side so keryx/web never duplicate it.
func (h *ProvinceHandler) PlaceGubbe(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}
	provinceID, err := uuid.Parse(chi.URLParam(r, "provinceID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid province ID")
		return
	}
	playerID, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req struct {
		TargetKind   string `json:"target_kind"` // "hex" | "building"
		HexQ         *int   `json:"hex_q"`
		HexR         *int   `json:"hex_r"`
		HexOrdinal   *int   `json:"hex_ordinal"` // alternative to hex_q/hex_r — 1..18, resolved via Ring() below
		BuildingType string `json:"building_type"`
		GoodKey      string `json:"good_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.GoodKey == "" {
		writeError(w, http.StatusBadRequest, `invalid JSON — expected {"target_kind":"hex","hex_ordinal":5,"good_key":"grain"} (or "hex_q"/"hex_r" instead of "hex_ordinal") or {"target_kind":"building","building_type":"stonequarry","good_key":"stone"}`)
		return
	}

	settlementID, population, center, ok := h.placementSettlement(r, worldID, provinceID, playerID)
	if !ok {
		writeError(w, http.StatusForbidden, "not your settlement")
		return
	}
	totalGubbar := population / 100

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "transaction error")
		return
	}
	defer tx.Rollback(r.Context())

	// Next free gubbe_ordinal: lowest 1..totalGubbar not already placed.
	placed, err := economy.LoadPlacementCounts(r.Context(), tx, settlementID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load placements")
		return
	}
	if placed.Total >= totalGubbar {
		writeError(w, http.StatusConflict, "no gubbar left in the pool — every gubbe is already placed")
		return
	}
	usedOrdinals := make(map[int]bool, placed.Total)
	orows, err := tx.Query(r.Context(), `SELECT gubbe_ordinal FROM settlement_placement WHERE settlement_id = $1`, settlementID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load ordinals")
		return
	}
	for orows.Next() {
		var o int
		if orows.Scan(&o) == nil {
			usedOrdinals[o] = true
		}
	}
	orows.Close()
	gubbeOrdinal := 0
	for o := 1; o <= totalGubbar; o++ {
		if !usedOrdinals[o] {
			gubbeOrdinal = o
			break
		}
	}
	if gubbeOrdinal == 0 {
		writeError(w, http.StatusConflict, "no gubbar left in the pool — every gubbe is already placed")
		return
	}

	switch req.TargetKind {
	case "hex":
		var hex hexgrid.Coord
		switch {
		case req.HexOrdinal != nil:
			ring := hexgrid.Ring(center, hexgrid.CatchmentRadius)
			if *req.HexOrdinal < 1 || *req.HexOrdinal > len(ring) {
				writeError(w, http.StatusBadRequest, "hex_ordinal out of range (1..18)")
				return
			}
			hex = ring[*req.HexOrdinal-1]
		case req.HexQ != nil && req.HexR != nil:
			hex = hexgrid.Coord{Q: *req.HexQ, R: *req.HexR}
			if _, found := hexgrid.RingOrdinal(center, hexgrid.CatchmentRadius, hex); !found {
				writeError(w, http.StatusBadRequest, "that hex is not in this settlement's catchment")
				return
			}
		default:
			writeError(w, http.StatusBadRequest, "hex placement requires hex_ordinal, or hex_q and hex_r")
			return
		}

		// FOW gate: a fog hex is never placeable (P0-UI §Tvärgående lås).
		// economy may not import province (G1), so this check lives here.
		var terrain string
		if err := h.pool.QueryRow(r.Context(),
			`SELECT terrain FROM map_tiles WHERE world_id = $1 AND q = $2 AND r = $3`,
			worldID, hex.Q, hex.R,
		).Scan(&terrain); err != nil {
			writeError(w, http.StatusBadRequest, "unknown hex")
			return
		}
		eyes := loadLiveEyes(r.Context(), h.pool, worldID, playerID, h.clk.Now())
		remembered := loadRememberedTiles(r.Context(), h.pool, worldID, playerID)
		if !knownToPlayer(eyes, remembered, province.MapPosition{Q: hex.Q, R: hex.R}, terrain) {
			writeError(w, http.StatusForbidden, "that hex hasn't been scouted yet — fog-of-war hexes can't be staffed")
			return
		}

		hexOptions, err := economy.LoadHexProductionOptions(r.Context(), tx, settlementID, nil) // validation menu — siege denial gates YIELD, not the placement UI
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not load catchment")
			return
		}
		var opt *economy.HexOption
		for i := range hexOptions {
			if hexOptions[i].Coord == hex {
				opt = &hexOptions[i]
				break
			}
		}
		if opt == nil || opt.RatePerGood[req.GoodKey] <= 0 {
			writeError(w, http.StatusUnprocessableEntity, "this hex has no production option for that good")
			return
		}
		cap := opt.CapPerGood[req.GoodKey]
		if cap <= 0 || placed.Hex[hex][req.GoodKey] >= cap {
			writeError(w, http.StatusConflict, "this hex is fully staffed for that good")
			return
		}
		if _, err := tx.Exec(r.Context(),
			`INSERT INTO settlement_placement (settlement_id, gubbe_ordinal, target_kind, hex_q, hex_r, good_key)
			 VALUES ($1, $2, 'hex', $3, $4, $5)`,
			settlementID, gubbeOrdinal, hex.Q, hex.R, req.GoodKey,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "could not place gubbe")
			return
		}

	case "building":
		if req.BuildingType == "" {
			writeError(w, http.StatusBadRequest, "building placement requires building_type")
			return
		}
		buildingOptions, err := economy.LoadBuildingProductionOptions(r.Context(), tx, settlementID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not load buildings")
			return
		}
		var opt *economy.BuildingOption
		for i := range buildingOptions {
			if buildingOptions[i].BuildingType == req.BuildingType {
				opt = &buildingOptions[i]
				break
			}
		}
		if opt == nil || opt.RatePerGood[req.GoodKey] <= 0 {
			writeError(w, http.StatusUnprocessableEntity, "this building has no production option for that good")
			return
		}
		cap := opt.CapPerGood[req.GoodKey]
		if cap <= 0 || placed.Building[req.BuildingType][req.GoodKey] >= cap {
			writeError(w, http.StatusConflict, "this building is fully staffed for that good")
			return
		}
		if _, err := tx.Exec(r.Context(),
			`INSERT INTO settlement_placement (settlement_id, gubbe_ordinal, target_kind, building_type, good_key)
			 VALUES ($1, $2, 'building', $3, $4)`,
			settlementID, gubbeOrdinal, req.BuildingType, req.GoodKey,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "could not place gubbe")
			return
		}

	default:
		writeError(w, http.StatusBadRequest, `target_kind must be "hex" or "building"`)
		return
	}

	if err := economy.RecomputeProduction(r.Context(), tx, settlementID); err != nil {
		writeError(w, http.StatusInternalServerError, "recompute production failed")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "commit failed")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"gubbe_ordinal": gubbeOrdinal})
}

// UnplaceGubbe handles DELETE /worlds/:worldID/provinces/:provinceID/placements/:ordinal
// — return a gubbe to the pool. Flytt av placerad gubbe = gratis + omedelbar
// (P0-UI §Tvärgående lås) — a Wanax re-placing a gubbe elsewhere just issues
// an unplace then a place, no cost either way.
func (h *ProvinceHandler) UnplaceGubbe(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}
	provinceID, err := uuid.Parse(chi.URLParam(r, "provinceID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid province ID")
		return
	}
	playerID, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	ordinal, err := parsePositiveInt(chi.URLParam(r, "ordinal"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid gubbe ordinal")
		return
	}

	settlementID, _, _, ok := h.placementSettlement(r, worldID, provinceID, playerID)
	if !ok {
		writeError(w, http.StatusForbidden, "not your settlement")
		return
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "transaction error")
		return
	}
	defer tx.Rollback(r.Context())

	tag, err := tx.Exec(r.Context(),
		`DELETE FROM settlement_placement WHERE settlement_id = $1 AND gubbe_ordinal = $2`,
		settlementID, ordinal,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not unplace gubbe")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "that gubbe isn't placed")
		return
	}

	if err := economy.RecomputeProduction(r.Context(), tx, settlementID); err != nil {
		writeError(w, http.StatusInternalServerError, "recompute production failed")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "commit failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"unplaced": ordinal})
}

// SlaughterLivestock handles POST
// /worlds/:worldID/provinces/:provinceID/slaughter-livestock — a Wanax's
// deliberate choice to trade one animal for population, the herd's
// strongest sink (S1c, megaron_plan_foda_konsistens.md, Timothy 2026-08-07:
// "En livestock kan kompensera för de sista tio popsen när en ny gubbe ska
// skapas, alltså BLI tio nya pops"). Player-initiated only, never automatic
// — an automatic slaughter would silently eat a herd a Wanax was saving as
// a svältreserv or a temple offering, and tacit decisions are exactly what
// the plan's fallback chain (S1) already avoids by never doing this on its
// own. Rejected with 422 if the settlement holds no livestock; livestock is
// a whole-animal stock, so exactly one animal is removed, never a fraction.
//
// If the +10 crosses a new full hundred, the newly-born gubbe is auto-placed
// through the SAME hook population growth already uses
// (economy.PlaceNextGubbeOnBestFoodHex, called identically from kharis/
// tick.go applyDecay's own oldGubbar/newGubbar crossing loop) — not a
// second, parallel placement path. Population is capped at
// economy.MaxGenesisPopulation (30000, "the settlement population soft cap"
// per that constant's own doc comment) — the same ceiling growth's tick
// query enforces (kharis/tick.go: GREATEST(101, LEAST(30000, ...))).
// Slaughter is just another population add and must not open a way past an
// invariant the rest of the game already holds.
func (h *ProvinceHandler) SlaughterLivestock(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}
	provinceID, err := uuid.Parse(chi.URLParam(r, "provinceID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid province ID")
		return
	}
	playerID, ok := auth.PlayerIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	settlementID, _, _, ok := h.placementSettlement(r, worldID, provinceID, playerID)
	if !ok {
		writeError(w, http.StatusForbidden, "not your settlement")
		return
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "transaction error")
		return
	}
	defer tx.Rollback(r.Context())

	// Whole-animal deduction: only proceeds if the settled stock is >= 1 —
	// mirrors Gift's atomic "UPDATE ... WHERE settled(...) >= amount" idiom
	// so the check and the deduction can't race apart.
	tag, err := tx.Exec(r.Context(),
		`UPDATE settlement_goods
		   SET amount    = settled(amount, rate, calc_tick) - 1,
		       calc_tick = current_world_tick()
		 WHERE settlement_id = $1 AND good_key = $2
		   AND settled(amount, rate, calc_tick) >= 1`,
		settlementID, economy.GoodLivestock,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not slaughter livestock")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusUnprocessableEntity, "no livestock to slaughter")
		return
	}

	var oldPop int
	if err := tx.QueryRow(r.Context(),
		`SELECT population FROM settlements WHERE id = $1 FOR UPDATE`,
		settlementID,
	).Scan(&oldPop); err != nil {
		writeError(w, http.StatusInternalServerError, "could not read population")
		return
	}
	newPop := oldPop + economy.LivestockSlaughterPopGain
	if newPop > economy.MaxGenesisPopulation {
		newPop = economy.MaxGenesisPopulation
	}
	if _, err := tx.Exec(r.Context(),
		`UPDATE settlements SET population = $2 WHERE id = $1`,
		settlementID, newPop,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "could not update population")
		return
	}

	// Crossing a new full hundred → auto-place the new gubbe on the best
	// food hex, the exact oldGubbar/newGubbar loop kharis/tick.go applyDecay
	// runs after ordinary grain-funded growth — reused verbatim, not
	// reinvented, so growth and slaughter place gubbar identically.
	oldGubbar := oldPop / 100
	newGubbar := newPop / 100
	gubbarPlaced := 0
	for ordinal := oldGubbar + 1; ordinal <= newGubbar; ordinal++ {
		placed, perr := economy.PlaceNextGubbeOnBestFoodHex(r.Context(), tx, settlementID, ordinal)
		if perr != nil {
			writeError(w, http.StatusInternalServerError, "could not place new gubbe")
			return
		}
		if placed {
			gubbarPlaced++
		}
	}

	// Population changed → grain consumption changed; keep rates in sync
	// immediately, matching PlaceGubbe's own "placering slår igenom
	// omedelbart" norm rather than leaving it for the next tick.
	if err := economy.RecomputeProduction(r.Context(), tx, settlementID); err != nil {
		writeError(w, http.StatusInternalServerError, "recompute production failed")
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "commit failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"population":    newPop,
		"gubbar_placed": gubbarPlaced,
	})
}

// BackfillPlacements handles POST /admin/worlds/:worldID/backfill-placements —
// X-Admin-Key gated (same pattern as god.go/reports.go), operator-triggered,
// NOT run automatically on boot. Runs economy.BackfillPlacements for every
// settlement in the world that predates P4 (population > 0, zero placement
// rows) — see that function's doc comment for why this can't just happen at
// RecomputeProduction time.
func (h *ProvinceHandler) BackfillPlacements(w http.ResponseWriter, r *http.Request) {
	if !requireAdminKey(w, r) {
		return
	}
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}
	n, err := economy.BackfillPlacements(r.Context(), h.pool, worldID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "backfill failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"settlements_backfilled": n})
}
