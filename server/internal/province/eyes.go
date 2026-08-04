package province

import (
	"context"
	"math"
	"time"

	"github.com/google/uuid"
)

// LoadLiveEyes returns the player's tier-1 (live) vision sources: own and allied
// settlements, plus own units currently on the map (marching or positioned — units
// still 'forming'/'garrison' have no q/r of their own and are seen only via their
// settlement's eye; 'embarked' units carry no position, they move with their ship).
// Each eye is typed so LiveRadius can size vision per temenos_synlighet.md's
// per-eye-kind × per-target-terrain table. Scouted tiles/provinces and messenger
// contacts are NOT eyes — they are tier-2 memory (see loadRememberedTiles in
// api/handlers/world.go).
//
// A marching unit's stored (q,r) is its ORIGIN hex — March (unit.go) never moves it,
// only target_q/target_r/departs_at/arrives_at are set at dispatch. So a marching
// unit's eye is placed at its interpolated position along its route (see
// InterpolateAlongPath) instead of the stored origin, or the fog bubble would sit at
// the harbour for the whole voyage instead of tracking the ship. now must come from
// the caller's injected clock.Clock — never time.Now() (CLAUDE.md Time rule).
func LoadLiveEyes(ctx context.Context, db Queryer, worldID, playerID uuid.UUID, now time.Time) []Eye {
	var eyes []Eye

	sRows, err := db.Query(ctx,
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
			var pos MapPosition
			if sRows.Scan(&pos.Q, &pos.R) == nil {
				eyes = append(eyes, Eye{Pos: pos, Kind: EyeSettlement})
			}
		}
		sRows.Close()
	}

	uRows, err := db.Query(ctx,
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
			kind := EyeLandUnit
			if category == "naval" {
				kind = EyeShip
			}
			pos := MapPosition{Q: q, R: r}
			if status == "marching" && targetQ != nil && targetR != nil && departsAt != nil && arrivesAt != nil {
				path, _, ok, pathErr := FindPath(ctx, db, worldID, pos,
					MapPosition{Q: *targetQ, R: *targetR}, category)
				if pathErr == nil && ok && len(path) > 0 {
					pos = InterpolateAlongPath(now, *departsAt, *arrivesAt, path)
				}
				// FindPath failure/empty path: best-effort fallback to the stored
				// origin (q,r) set above — never drop the eye.
			}
			eyes = append(eyes, Eye{Pos: pos, Kind: kind})
		}
		uRows.Close()
	}

	// Runners — the player's own outbound messengers (orders, diplomatic
	// letters, recalls) are live eyes along their route, seeing as a land unit
	// (temenos_synlighet.md §Nivå 1, beslut Timothy 2026-07-16). Position is
	// interpolated along the courier A*-path (land at 2× spearman speed, sea by
	// boat) between sent_at and arrives_at — same pattern as marching units.
	// The return leg (status='returning') is handled by the second query below;
	// on reply the courier gets a fresh return window (return_departs_at→arrives_at)
	// so it can be interpolated home just like the outbound leg. Couriers are
	// uninterceptable either way — the eye is purely so the sender's own runner
	// keeps revealing fog on the way there AND back.
	mRows, err := db.Query(ctx,
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
			pos := MapPosition{Q: oq, R: or_}
			if dq != nil && dr != nil {
				path, _, ok, pathErr := FindPath(ctx, db, worldID, pos,
					MapPosition{Q: *dq, R: *dr}, CategoryCourier)
				if pathErr == nil && ok && len(path) > 0 {
					pos = InterpolateAlongPath(now, sentAt, arrivesAt, path)
				}
				// FindPath failure: best-effort fallback to the origin hex — the
				// courier still sees from somewhere, never drops the eye.
			}
			eyes = append(eyes, Eye{Pos: pos, Kind: EyeLandUnit})
		}
		mRows.Close()
	}

	// Return leg: a replied courier runs from the delivery point (destination)
	// home to its origin. Built on explicit origin/dest joins rather than
	// hex_q/hex_r (whose meaning differs between send paths); window is the fresh
	// return_departs_at→arrives_at set by the Reply handler.
	rRows, err := db.Query(ctx,
		`SELECT COALESCE(dp.map_q, m.dest_q), COALESCE(dp.map_r, m.dest_r),
		        COALESCE(op.map_q, m.origin_q), COALESCE(op.map_r, m.origin_r),
		        m.return_departs_at, m.arrives_at
		 FROM messengers m
		 LEFT JOIN settlements os ON os.id = m.origin_id
		 LEFT JOIN provinces op ON op.id = os.province_id
		 LEFT JOIN settlements ds ON ds.id = m.destination_id
		 LEFT JOIN provinces dp ON dp.id = ds.province_id
		 WHERE m.world_id = $1 AND m.sender_id = $2 AND m.status = 'returning'
		   AND m.return_departs_at IS NOT NULL`,
		worldID, playerID,
	)
	if err == nil {
		for rRows.Next() {
			var sq, sr int  // return start = delivery point (destination)
			var hq, hr *int // return end = home (origin)
			var departsAt, arrivesAt time.Time
			if rRows.Scan(&sq, &sr, &hq, &hr, &departsAt, &arrivesAt) != nil {
				continue
			}
			pos := MapPosition{Q: sq, R: sr}
			if hq != nil && hr != nil {
				path, _, ok, pathErr := FindPath(ctx, db, worldID, pos,
					MapPosition{Q: *hq, R: *hr}, CategoryCourier)
				if pathErr == nil && ok && len(path) > 0 {
					pos = InterpolateAlongPath(now, departsAt, arrivesAt, path)
				}
			}
			eyes = append(eyes, Eye{Pos: pos, Kind: EyeLandUnit})
		}
		rRows.Close()
	}

	return eyes
}

// InterpolateAlongPath returns a marching unit's live-vision position: its location
// along path at the current point in its journey, linearly interpolated by elapsed
// wall-clock time between departsAt and arrivesAt (temenos_synlighet.md tier 1 —
// vision must track a moving ship/unit, not just its departure or arrival hex).
//
// progress is clamped to [0,1] so a read before departure or after arrival snaps to
// the first/last hex rather than extrapolating. idx = round(progress*(len(path)-1))
// picks the nearest path hex to the elapsed fraction. Pure and DB-free: path is
// precomputed by the caller (FindPath) so this is unit-testable with fixed
// time.Time values and no clock. An empty path has no position to return; callers
// must check len(path) > 0 before calling and fall back to the unit's stored (q,r)
// otherwise — this function returns the zero MapPosition rather than panicking.
func InterpolateAlongPath(now, departsAt, arrivesAt time.Time, path []MapPosition) MapPosition {
	if len(path) == 0 {
		return MapPosition{}
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
