package handlers

import (
	"hash/fnv"
	"net/http"
	"sort"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/poleia/server/internal/auth"
)

// ruralProjectionTypes is the building set projected onto catchment hexes in the
// Fas A2 slice (megaron_lokal_varld.md). The set is a sequencing limit, not
// canon — the placement rule below generalises to every building type; widen the
// slice by adding here (and giving the client a sprite in render/map.js).
var ruralProjectionTypes = []string{"farm", "mine", "lumbermill"}

// ruralProjection is one map-anchored representation of a city building on a
// compatible catchment hex — a CARTOGRAPHIC PROJECTION of an existing settlement
// building, never a standalone economic object (megaron_lokal_varld.md
// §Ruralprojektion). The mechanical building still lives in the settlement's
// building row; the client draws a sprite here and its object card leads back to
// the city's building context.
type ruralProjection struct {
	SettlementID uuid.UUID `json:"settlement_id"`
	ProvinceID   uuid.UUID `json:"province_id"`
	Name         string    `json:"name"`
	BuildingType string    `json:"building_type"`
	Q            int       `json:"q"`
	R            int       `json:"r"`
}

// RuralProjections returns rural building projections for the authenticated
// player's OWN settlements. The compatible-hex matching REUSES the exact
// production_rules join RecomputeProduction/CatchmentBasePotential run
// (economy/catchment.go) — the doc's hard rule is "use the same matching, don't
// invent a parallel". Only own cities are emitted, so the owner's catchment is
// inherently within their FOW; no extra fog gating is needed.
//
// Deterministic placement (megaron_lokal_varld.md §Deterministisk platsregel):
// among the 6 ring hexes (the centre IS the city), keep those compatible with
// the building type, prefer a TERRAIN/DEPOSIT-specific match over a generic
// anywhere-rule (so a mine settles at the mountain when one is in the ring,
// not on plains via its stone rule), then break ties with a stable hash of
// (settlement, building, hex). No hex carries two projections. A building with
// no compatible visible hex is simply omitted here — it stays visible in the
// city drawer (rule 5). The output never wanders across zoom/reconnect/new
// state because the inputs (buildings, terrain) are stable.
func (h *WorldHandler) RuralProjections(w http.ResponseWriter, r *http.Request) {
	worldID, err := uuid.Parse(chi.URLParam(r, "worldID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid world ID")
		return
	}
	playerID, authenticated := auth.PlayerIDFromContext(r.Context())
	if !authenticated {
		writeJSON(w, http.StatusOK, []ruralProjection{})
		return
	}

	// One candidate row per (settlement, building, compatible ring hex). The
	// join predicates mirror economy/catchment.go verbatim; `specific` is true
	// when the match came from a terrain- or deposit-bound rule (vs a generic
	// terrain_type IS NULL / requires_deposit IS NULL anywhere-rule). Ring hexes
	// only — the centre hex holds the city itself. Hexes occupied by any other
	// province are excluded so a projection never lands on a neighbour's city.
	rows, err := h.pool.Query(r.Context(),
		`SELECT s.id, s.province_id, s.name, b.building_type, mt.q, mt.r,
		        bool_or(pr.terrain_type IS NOT NULL OR pr.requires_deposit IS NOT NULL) AS specific
		 FROM settlements s
		 JOIN provinces sp ON sp.id = s.province_id
		 JOIN buildings b ON b.settlement_id = s.id AND b.building_type = ANY($3)
		 JOIN map_tiles mt ON mt.world_id = $1
		     AND mt.terrain NOT IN ('deep_sea', 'coastal_sea')
		     AND (
		         (mt.q = sp.map_q+1 AND mt.r = sp.map_r  ) OR (mt.q = sp.map_q-1 AND mt.r = sp.map_r  ) OR
		         (mt.q = sp.map_q   AND mt.r = sp.map_r+1) OR (mt.q = sp.map_q   AND mt.r = sp.map_r-1) OR
		         (mt.q = sp.map_q+1 AND mt.r = sp.map_r-1) OR (mt.q = sp.map_q-1 AND mt.r = sp.map_r+1)
		     )
		 JOIN production_rules pr ON pr.building_type = b.building_type
		     AND (pr.terrain_type IS NULL OR pr.terrain_type = mt.terrain)
		     AND (NOT pr.requires_coastal OR mt.coastal)
		     AND (pr.requires_deposit IS NULL
		          OR (pr.requires_deposit = 'copper' AND mt.copper_deposit)
		          OR (pr.requires_deposit = 'tin'    AND mt.tin_deposit)
		          OR (pr.requires_deposit = 'silver' AND COALESCE(mt.silver_deposit, false))
		          OR (pr.requires_deposit = 'cedar'  AND COALESCE(mt.cedar_deposit, false)))
		 WHERE s.world_id = $1 AND s.owner_id = $2
		   AND NOT EXISTS (SELECT 1 FROM provinces p2 WHERE p2.world_id = $1 AND p2.map_q = mt.q AND p2.map_r = mt.r)
		 GROUP BY s.id, s.province_id, s.name, b.building_type, mt.q, mt.r`,
		worldID, playerID, ruralProjectionTypes,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load rural projections")
		return
	}
	defer rows.Close()

	cities := map[uuid.UUID]*ruralCity{}
	for rows.Next() {
		var sid, pid uuid.UUID
		var name, btype string
		var q, rr int
		var specific bool
		if err := rows.Scan(&sid, &pid, &name, &btype, &q, &rr, &specific); err != nil {
			continue
		}
		ci := cities[sid]
		if ci == nil {
			ci = &ruralCity{provinceID: pid, name: name, cands: map[string][]ruralCandidate{}}
			cities[sid] = ci
		}
		ci.cands[btype] = append(ci.cands[btype], ruralCandidate{q: q, r: rr, specific: specific})
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "could not load rural projections")
		return
	}

	writeJSON(w, http.StatusOK, placeRural(cities))
}

type ruralCandidate struct {
	q, r     int
	specific bool
}

type ruralCity struct {
	provinceID uuid.UUID
	name       string
	cands      map[string][]ruralCandidate // building_type → compatible ring hexes
}

// placeRural resolves each city's candidate hexes into at most one projection per
// building type, applying the deterministic placement rule. Pure (no DB) so the
// three regression risks — a mine on plains instead of the mountain, two
// projections on one hex, and placement wandering between requests — are unit
// tested without a DB harness. Output is sorted for byte-stable responses.
func placeRural(cities map[uuid.UUID]*ruralCity) []ruralProjection {
	out := []ruralProjection{}
	for sid, ci := range cities {
		used := map[[2]int]bool{}
		// Fixed building order so placement is independent of map iteration order.
		for _, btype := range ruralProjectionTypes {
			cands := append([]ruralCandidate(nil), ci.cands[btype]...)
			if len(cands) == 0 {
				continue
			}
			// Specific matches first, then stable hash of (settlement, building, hex).
			sort.Slice(cands, func(i, j int) bool {
				if cands[i].specific != cands[j].specific {
					return cands[i].specific // true sorts before false
				}
				return ruralHexKey(sid, btype, cands[i].q, cands[i].r) <
					ruralHexKey(sid, btype, cands[j].q, cands[j].r)
			})
			var pick *ruralCandidate
			for i := range cands {
				if !used[[2]int{cands[i].q, cands[i].r}] {
					pick = &cands[i]
					break
				}
			}
			if pick == nil {
				continue // every compatible hex already taken by another projection
			}
			used[[2]int{pick.q, pick.r}] = true
			out = append(out, ruralProjection{
				SettlementID: sid,
				ProvinceID:   ci.provinceID,
				Name:         ci.name,
				BuildingType: btype,
				Q:            pick.q,
				R:            pick.r,
			})
		}
	}
	// Stable output order (map iteration is random) so responses are byte-stable.
	sort.Slice(out, func(i, j int) bool {
		if out[i].Q != out[j].Q {
			return out[i].Q < out[j].Q
		}
		return out[i].R < out[j].R
	})
	return out
}

func ruralHexKey(sid uuid.UUID, btype string, q, r int) uint64 {
	hh := fnv.New64a()
	hh.Write(sid[:])
	hh.Write([]byte(btype))
	var b [8]byte
	b[0] = byte(q)
	b[1] = byte(q >> 8)
	b[2] = byte(r)
	b[3] = byte(r >> 8)
	hh.Write(b[:])
	return hh.Sum64()
}
