package province

import (
	"context"
	"math"
	"time"

	"formatet/megaron/server/internal/hexgrid"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

// LoadLiveEyes returns the player's tier-1 (live) vision sources: own and allied
// settlements, plus own units currently on the map (marching or positioned — units
// still 'forming'/'garrison' have no q/r of their own and are seen only via their
// settlement's eye; 'embarked' units carry no position, they move with their ship;
// 'disbanded' units no longer exist — most disband paths leave q/r in place, so
// the status filter, not the position, is what keeps them from seeing).
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
		   AND status NOT IN ('embarked', 'disbanded')
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
		`SELECT COALESCE(op.map_q, m.origin_q), COALESCE(op.map_r, m.origin_r),
		        COALESCE(m.dest_q, dp.map_q), COALESCE(m.dest_r, dp.map_r),
		        m.sent_at, m.arrives_at, m.passage_status, pp.map_q, pp.map_r
		 FROM messengers m
		 -- Bifynd from 3b-1 (megaron_plan_ordna_passage.md), same root: the
		 -- true origin is origin_id (a settlement, joined here) or, for a
		 -- host-sent messenger (mig 087), origin_q/origin_r — NEVER hex_q/hex_r.
		 -- hex_q/hex_r only happens to equal the origin for kind='order'
		 -- (unit.go sendOrderCourier) and kind='recall' (province.go's march
		 -- recall), which deliberately write it there; a plain diplomatic
		 -- message (Send/SendFromHost) writes the DESTINATION into hex_q/hex_r
		 -- instead, which made pos==target==destination for the whole outbound
		 -- leg of every ordinary message. origin_id/origin_q are authoritative
		 -- for every kind alike (messengers_exactly_one_origin, mig 087) — same
		 -- join finalTargetQuery (messenger/passage.go) and the return-leg
		 -- query below already use.
		 LEFT JOIN settlements os ON os.id = m.origin_id
		 LEFT JOIN provinces op ON op.id = os.province_id
		 LEFT JOIN settlements ds ON ds.id = m.destination_id
		 LEFT JOIN provinces dp ON dp.id = ds.province_id
		 -- megaron_plan_ordna_passage.md 3b-1: pp/pps resolve the port hex for a
		 -- runner that is awaiting_passage there — its eye must sit at the port,
		 -- never interpolated toward a final target across the sea it hasn't
		 -- crossed yet.
		 LEFT JOIN settlements pps ON pps.id = m.passage_port_id
		 LEFT JOIN provinces pp ON pp.id = pps.province_id
		 WHERE m.world_id = $1 AND m.sender_id = $2 AND m.status = 'outbound' AND NOT EXISTS(SELECT 1 FROM events e WHERE e.stream_id=m.id AND e.event_type='CarrierPassengerRescuedV1')`,
		worldID, playerID,
	)
	if err == nil {
		for mRows.Next() {
			var oq, or_ int
			var dq, dr *int
			var sentAt, arrivesAt time.Time
			var passageStatus *string
			var portQ, portR *int
			if mRows.Scan(&oq, &or_, &dq, &dr, &sentAt, &arrivesAt, &passageStatus, &portQ, &portR) != nil {
				continue
			}
			// 3b-1: a sea-lift runner's eye follows the leg it is actually on,
			// not the flat origin→final-target courier interpolation below
			// (which assumes an unbroken crossing and, for awaiting_passage,
			// reads an arrives_at that is its PAST landward arrival at the
			// port — clamping progress to 1 and leaking an eye on the far
			// shore for the whole wait).
			if passageStatus != nil {
				switch *passageStatus {
				case "aboard", "returning_sealed":
					// Sealed cargo aboard a carrier, or sealed between carrier
					// and port: the runner itself reveals nothing — the carrier
					// is its own eye, if any. This also covers the landward leg
					// after disembark (stop condition in the plan): passage_status
					// stays 'aboard' for the whole voyage, so no eye is given
					// there either, conservatively, rather than adding a column
					// for the landward leg's own start time.
					continue
				case "awaiting_passage":
					// Standing at its port, not mid-crossing.
					if portQ != nil && portR != nil {
						eyes = append(eyes, Eye{Pos: MapPosition{Q: *portQ, R: *portR}, Kind: EyeLandUnit})
					}
					continue
				}
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
		        m.return_departs_at, m.arrives_at, m.passage_status, pp.map_q, pp.map_r, m.withdrawn
		 FROM messengers m
		 LEFT JOIN settlements os ON os.id = m.origin_id
		 LEFT JOIN provinces op ON op.id = os.province_id
		 LEFT JOIN settlements ds ON ds.id = m.destination_id
		 LEFT JOIN provinces dp ON dp.id = ds.province_id
		 -- megaron_plan_ordna_passage.md 3b-1: the return leg can also be
		 -- awaiting_passage (in a foreign port, R6 of megaron_plan_budet_liftar.md).
		 LEFT JOIN settlements pps ON pps.id = m.passage_port_id
		 LEFT JOIN provinces pp ON pp.id = pps.province_id
		 WHERE m.world_id = $1 AND m.sender_id = $2 AND m.status = 'returning'
		   AND m.return_departs_at IS NOT NULL AND NOT EXISTS(SELECT 1 FROM events e WHERE e.stream_id=m.id AND e.event_type='CarrierPassengerRescuedV1')`,
		worldID, playerID,
	)
	if err == nil {
		for rRows.Next() {
			var sq, sr int  // return start = delivery point (destination)
			var hq, hr *int // return end = home (origin)
			var departsAt, arrivesAt time.Time
			var passageStatus *string
			var portQ, portR *int
			var withdrawn bool
			if rRows.Scan(&sq, &sr, &hq, &hr, &departsAt, &arrivesAt, &passageStatus, &portQ, &portR, &withdrawn) != nil {
				continue
			}
			// 3b-1: same leg-follows-status rule as the outbound query above.
			if passageStatus != nil {
				switch *passageStatus {
				case "aboard", "returning_sealed":
					continue
				case "awaiting_passage":
					if portQ != nil && portR != nil {
						eyes = append(eyes, Eye{Pos: MapPosition{Q: *portQ, R: *portR}, Kind: EyeLandUnit})
					}
					continue
				}
			}
			// 3b-4 R5 "kalla tillbaka" (planner review fix): a withdrawn
			// runner's return leg starts at the PORT it turned back from,
			// never the destination it never reached — sq,sr (from
			// destination_id/dest_q/dest_r) names the WRONG city here, and a
			// courier route from that unreached destination back to origin
			// may not even exist (that is exactly why it needed a ship).
			// CallBack deliberately leaves passage_port_id set for this.
			startQ, startR := sq, sr
			if withdrawn && portQ != nil && portR != nil {
				startQ, startR = *portQ, *portR
			}
			pos := MapPosition{Q: startQ, R: startR}
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

	loadSeaHorizons(ctx, db, worldID, eyes)
	return eyes
}

// loadSeaHorizons fills every eye's open-water horizon (SetSeaHorizons) from the
// map. Runs as a post-pass because a marching eye's position is only known after
// interpolation.
//
// One batched query for the sea hexes inside the SeaHorizonRadius disk of every
// eye (≤ 61·len(eyes) hexes, deduplicated), not a full map load — the cost must not
// grow with map size. Only sea hexes are fetched: the sightline asks nothing else.
func loadSeaHorizons(ctx context.Context, db Queryer, worldID uuid.UUID, eyes []Eye) {
	if len(eyes) == 0 {
		return
	}

	want := make(map[MapPosition]struct{}, len(eyes)*61)
	for _, e := range eyes {
		for _, c := range hexgrid.Disk(hexgrid.Coord{Q: e.Pos.Q, R: e.Pos.R}, SeaHorizonRadius) {
			want[MapPosition{Q: c.Q, R: c.R}] = struct{}{}
		}
	}
	qs := make([]int32, 0, len(want))
	rs := make([]int32, 0, len(want))
	for p := range want {
		qs = append(qs, int32(p.Q))
		rs = append(rs, int32(p.R))
	}

	rows, err := db.Query(ctx,
		`SELECT t.q, t.r, t.terrain
		   FROM map_tiles t
		   JOIN unnest($2::int[], $3::int[]) AS p(q, r) ON t.q = p.q AND t.r = p.r
		  WHERE t.world_id = $1 AND t.terrain IN ('coastal_sea', 'deep_sea')`,
		worldID, qs, rs,
	)
	if err != nil {
		// Fail closed: every eye keeps a nil horizon and reads the sea at its
		// ordinary vantage. A failed lookup may hide fog, never reveal it.
		return
	}
	terrain := make(map[MapPosition]string)
	for rows.Next() {
		var p MapPosition
		var t string
		if rows.Scan(&p.Q, &p.R, &t) == nil {
			terrain[p] = t
		}
	}
	rows.Close()

	SetSeaHorizons(eyes, func(p MapPosition) string { return terrain[p] })
}

// SweepLiveRadius force-records into player_scouted_tiles every tile that a live
// eye of eyeKind standing at pos would currently reveal, using the same sight
// test (Eye.Sees: LiveRadius + the open-water sightline) the ordinary /map read
// applies via AnyEyeSees. /map normally does this memory-write itself on every read (see
// api/handlers/world.go), so it self-heals for any unit that stays put — but a
// unit whose live-vision window closes within the same transaction it opened
// (combat/unit_arrival.go's exploreArrived: the ship turns for home immediately
// on arrival) may never sit still long enough for a player's /map poll to catch
// it, leaving the radius around the explored hex permanently fogged even though
// the hex itself was swept. Call this once, at the moment the eye is known to
// stand at pos, to force the same memory-write /map would have done had the
// player happened to read the map at that exact instant.
//
// db must support both Query and Exec — pgx.Tx and *pgxpool.Pool both do.
// The candidate disk is SeaHorizonRadius (4): the widest reach a ship or land
// unit has (the open horizon, or a land eye at a mountain). The same disk holds
// every hex the sightline can cross, so it also feeds SetSeaHorizons.
func SweepLiveRadius(ctx context.Context, db interface {
	Queryer
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}, worldID, playerID uuid.UUID, pos MapPosition, eyeKind string) error {
	_, err := SweepLiveRadiusAlong(ctx, db, worldID, playerID, []MapPosition{pos}, eyeKind)
	return err
}

// SweepLiveRadiusAlong is SweepLiveRadius for an eye that stood, in turn, on
// every hex of path — an expedition leg (combat/expedition.go) records what it
// saw along the whole way, not only where it stopped, so its next target never
// depends on whether the player happened to read the map mid-march
// (megaron_plan_upptackarexpeditionen.md, invariant 2). Returns every tile
// seen, whether or not it was already in player_scouted_tiles.
func SweepLiveRadiusAlong(ctx context.Context, db interface {
	Queryer
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}, worldID, playerID uuid.UUID, path []MapPosition, eyeKind string) ([]MapPosition, error) {
	if len(path) == 0 {
		return nil, nil
	}
	diskSet := make(map[hexgrid.Coord]bool)
	for _, pos := range path {
		for _, c := range hexgrid.Disk(hexgrid.Coord{Q: pos.Q, R: pos.R}, SeaHorizonRadius) {
			diskSet[c] = true
		}
	}
	disk := make([]hexgrid.Coord, 0, len(diskSet))
	for c := range diskSet {
		disk = append(disk, c)
	}
	qs, rs := hexgrid.QRArrays(disk)

	rows, err := db.Query(ctx,
		`SELECT t.q, t.r, t.terrain
		   FROM map_tiles t
		   JOIN unnest($2::int[], $3::int[]) AS p(q, r) ON t.q = p.q AND t.r = p.r
		  WHERE t.world_id = $1`,
		worldID, qs, rs,
	)
	if err != nil {
		return nil, err
	}
	terrain := make(map[MapPosition]string, len(disk))
	for rows.Next() {
		var p MapPosition
		var t string
		if rows.Scan(&p.Q, &p.R, &t) == nil {
			terrain[p] = t
		}
	}
	rows.Close()

	eyes := make([]Eye, len(path))
	for i, pos := range path {
		eyes[i] = Eye{Pos: pos, Kind: eyeKind}
	}
	SetSeaHorizons(eyes, func(p MapPosition) string { return terrain[p] })

	var seen []MapPosition
	var seenQ, seenR []int
	for target, t := range terrain {
		if !AnyEyeSees(eyes, target, t) {
			continue
		}
		seen = append(seen, target)
		seenQ = append(seenQ, target.Q)
		seenR = append(seenR, target.R)
	}
	if len(seen) == 0 {
		return nil, nil
	}
	if _, err := db.Exec(ctx,
		`INSERT INTO player_scouted_tiles (world_id, player_id, q, r)
		 SELECT $1, $2, p.q, p.r FROM unnest($3::int[], $4::int[]) AS p(q, r)
		 ON CONFLICT (world_id, player_id, q, r) DO NOTHING`,
		worldID, playerID, seenQ, seenR,
	); err != nil {
		return nil, err
	}
	return seen, nil
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
