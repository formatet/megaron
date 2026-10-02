package economy

import (
	"context"
	"fmt"

	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/hexgrid"
	"github.com/google/uuid"
)

// Delad catchment (megaron_plan_delad_catchment.md, Timothy 2026-10-02): a hex
// held by ANOTHER settlement (it has gubbar placed there — hex ownership,
// §2/§2b) can be TAKEN by a settlement whose Wanax has a unit of their own
// standing on it in fortify or sentry, provided the holder's Wanax does not
// also have such a unit there (then the units fight it out first). Taking is
// done by placing a gubbe there (api/handlers PlaceGubbe); the holder's
// gubbar go home to its pool and its building on the hex changes owner.
//
// This file is the ONE place the rule stands. The write gate (PlaceGubbe) and
// both read surfaces (PlacementOptions, /goods) all go through LoadHexHolds,
// so a read surface can never promise what the write gate would reject.

// EventHexTaken is the dispatch kind the holder receives when its hex is
// taken. Same shape as HexBlockaded (hex_blockade.go).
const EventHexTaken = "HexTaken"

// HexTakenPayload names the loser's settlement, the taker's, the hex (q/r —
// so "Take me there" resolves it directly), how many gubbar went home to the
// loser's pool, and the building (type, or empty) that changed hands.
type HexTakenPayload struct {
	SettlementID uuid.UUID `json:"settlement_id"` // the loser
	WorldID      uuid.UUID `json:"world_id"`
	Name         string    `json:"name"`  // the loser's settlement
	Taker        string    `json:"taker"` // the taker's settlement
	Q            int       `json:"q"`
	R            int       `json:"r"`
	Workers      int       `json:"workers"`
	Building     string    `json:"building"`
}

// TakeVerdict is the answer to "may the placer take this held hex".
type TakeVerdict int

const (
	// TakeNoOwnUnit: the placer has no unit in fortify/sentry on the hex.
	TakeNoOwnUnit TakeVerdict = iota
	// TakeHolderDefends: the holder's Wanax has a unit in fortify/sentry on
	// the hex — battle decides first.
	TakeHolderDefends
	// TakeSameOwner: both settlements belong to one Wanax. Not takeable,
	// plain "holds this hex" (non-scope of the plan).
	TakeSameOwner
	// TakeYes: the hex can be taken.
	TakeYes
)

// HexHold describes who holds a hex (as seen by one placing settlement) and
// whether the placer may take it.
type HexHold struct {
	HolderID      uuid.UUID
	HolderOwnerID uuid.UUID
	HolderName    string
	// Workers is the holder's gubbar on the hex, per good.
	Workers map[string]int
	Verdict TakeVerdict
}

// Takeable reports Verdict == TakeYes.
func (h HexHold) Takeable() bool { return h.Verdict == TakeYes }

// TotalWorkers sums Workers over every good.
func (h HexHold) TotalWorkers() int {
	n := 0
	for _, c := range h.Workers {
		n += c
	}
	return n
}

// LoadHexHolds returns, for each hex in hexes that some settlement OTHER than
// settlementID holds, who holds it and the take verdict for placerOwner.
// Hexes nobody else holds are absent.
func LoadHexHolds(ctx context.Context, tx Tx, worldID, settlementID, placerOwner uuid.UUID, hexes []hexgrid.Coord) (map[hexgrid.Coord]HexHold, error) {
	out := make(map[hexgrid.Coord]HexHold)
	if len(hexes) == 0 {
		return out, nil
	}
	q, r := hexgrid.QRArrays(hexes)
	rows, err := tx.Query(ctx,
		`SELECT sp.hex_q, sp.hex_r, s.id, COALESCE(s.owner_id, '00000000-0000-0000-0000-000000000000'::uuid), s.name, sp.good_key, COUNT(*)
		 FROM settlement_placement sp
		 JOIN settlements s ON s.id = sp.settlement_id
		 JOIN unnest($2::int[], $3::int[]) AS wanted(q, r) ON wanted.q = sp.hex_q AND wanted.r = sp.hex_r
		 WHERE s.world_id = $1 AND sp.target_kind = 'hex' AND sp.settlement_id != $4
		 GROUP BY sp.hex_q, sp.hex_r, s.id, s.owner_id, s.name, sp.good_key
		 ORDER BY sp.hex_q, sp.hex_r, s.id`,
		worldID, q, r, settlementID,
	)
	if err != nil {
		return nil, fmt.Errorf("load hex holds: query: %w", err)
	}
	for rows.Next() {
		var qq, rr, n int
		var sid, owner uuid.UUID
		var name, good string
		if err := rows.Scan(&qq, &rr, &sid, &owner, &name, &good, &n); err != nil {
			rows.Close()
			return nil, fmt.Errorf("load hex holds: scan: %w", err)
		}
		c := hexgrid.Coord{Q: qq, R: rr}
		h, seen := out[c]
		if !seen {
			h = HexHold{HolderID: sid, HolderOwnerID: owner, HolderName: name, Workers: make(map[string]int)}
		}
		if h.HolderID != sid {
			continue // a second holder on the same hex (legacy overlap): the first, by id, is THE holder
		}
		h.Workers[good] += n
		out[c] = h
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load hex holds: rows: %w", err)
	}
	if len(out) == 0 {
		return out, nil
	}

	// Who stands guard (positioned, fortify/sentry) on the held hexes.
	held := make([]hexgrid.Coord, 0, len(out))
	for c := range out {
		held = append(held, c)
	}
	hq, hr := hexgrid.QRArrays(held)
	urows, err := tx.Query(ctx,
		`SELECT DISTINCT u.q, u.r, u.owner_id
		 FROM units u
		 JOIN unnest($2::int[], $3::int[]) AS wanted(q, r) ON wanted.q = u.q AND wanted.r = u.r
		 WHERE u.world_id = $1 AND u.status = 'positioned' AND u.stance IN ('fortify', 'sentry')`,
		worldID, hq, hr,
	)
	if err != nil {
		return nil, fmt.Errorf("load hex holds: units: %w", err)
	}
	type key struct {
		c     hexgrid.Coord
		owner uuid.UUID
	}
	standing := make(map[key]bool)
	for urows.Next() {
		var qq, rr int
		var owner uuid.UUID
		if err := urows.Scan(&qq, &rr, &owner); err != nil {
			urows.Close()
			return nil, fmt.Errorf("load hex holds: units scan: %w", err)
		}
		standing[key{hexgrid.Coord{Q: qq, R: rr}, owner}] = true
	}
	urows.Close()
	if err := urows.Err(); err != nil {
		return nil, fmt.Errorf("load hex holds: units rows: %w", err)
	}

	for c, h := range out {
		switch {
		case h.HolderOwnerID == placerOwner:
			h.Verdict = TakeSameOwner
		case !standing[key{c, placerOwner}]:
			h.Verdict = TakeNoOwnUnit
		case standing[key{c, h.HolderOwnerID}]:
			h.Verdict = TakeHolderDefends
		default:
			h.Verdict = TakeYes
		}
		out[c] = h
	}
	return out, nil
}

// TakeableHexes is the rule in one line: the hexes among holds the placer may
// take — the placer's Wanax has a unit positioned in fortify/sentry on the
// hex AND the holder's Wanax does not. (The units are read by LoadHexHolds;
// this is the filter every surface applies.)
func TakeableHexes(holds map[hexgrid.Coord]HexHold) map[hexgrid.Coord]bool {
	out := make(map[hexgrid.Coord]bool)
	for c, h := range holds {
		if h.Takeable() {
			out[c] = true
		}
	}
	return out
}

// ApplyHexHolds is what the read surfaces (PlacementOptions, /goods) do with
// holds: a takeable hex is not full — the holder's gubbar are subtracted from
// its occupancy, since they go home at the take — and every other held hex
// reads as fully staffed (MarkHeldHexesFullyOccupied). Returns occupancy
// unchanged (same map) when holds is empty.
func ApplyHexHolds(hexOptions []HexOption, holds map[hexgrid.Coord]HexHold, occupancy map[hexgrid.Coord]map[string]int) map[hexgrid.Coord]map[string]int {
	if len(holds) == 0 {
		return occupancy
	}
	takeable := TakeableHexes(holds)
	out := make(map[hexgrid.Coord]map[string]int, len(occupancy))
	for hex, goods := range occupancy {
		if h, ok := holds[hex]; ok && takeable[hex] {
			cp := make(map[string]int, len(goods))
			for g, n := range goods {
				cp[g] = n - h.Workers[g]
			}
			out[hex] = cp
			continue
		}
		out[hex] = goods
	}
	notTakeable := make(map[hexgrid.Coord]bool)
	for c := range holds {
		if !takeable[c] {
			notTakeable[c] = true
		}
	}
	return MarkHeldHexesFullyOccupied(hexOptions, notTakeable, out)
}

// DispatchHexTaken records the HexTaken event and pushes it to the loser's
// Wanax. store/hub may be nil. Call AFTER the take's transaction committed.
func DispatchHexTaken(ctx context.Context, store *events.Store, hub BlockadeNotifier, loserOwner uuid.UUID, p HexTakenPayload) {
	if store != nil {
		_, _ = store.Append(ctx, p.SettlementID, events.StreamProvince, EventHexTaken, p, p.WorldID, nil)
	}
	if hub != nil && loserOwner != uuid.Nil {
		_ = hub.NotifyPlayer(ctx, p.WorldID, loserOwner, EventHexTaken, 2, p)
	}
}
