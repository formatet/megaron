package economy

import (
	"context"
	"fmt"

	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/hexgrid"
	"github.com/google/uuid"
)

// Blockad med enhet (megaron_plan_blockad_med_enhet.md, Timothy 2026-09-05):
// "om en fientlig wanax ställer en enhet i fortify eller sentry på en hex så
// slutar gubbarna att producera från den hexen." Generalized, not a
// special-case for disputed hexes.
//
// This file has two jobs, deliberately kept separate:
//   - LoadEnemyPositionedHexes (read-only, no side effects) is what
//     RecomputeProduction calls every time it runs, to zero a blocked hex's
//     yield — that must stay cheap and pure (see recompute.go step 1c).
//   - SyncHexBlockade (below) is what detects the START/END TRANSITION and
//     fires the owner's dispatch (plan §3.4). It is NOT called by
//     RecomputeProduction itself — that function runs from ~15 call sites
//     (build complete, unit arrival, placement change, …) and firing a
//     dispatch from all of them would either spam on every unrelated
//     recompute or require threading a Broadcaster through every one of
//     those callers. Instead it runs once a day from kharis/tick.go's
//     existing per-settlement loop, which already recomputes production
//     daily and already holds a hub — exactly the cadence belägring's own
//     siege-starvation clock uses (applySiegeStarvationClock, same file).

// EventHexBlockaded / EventHexUnblockaded are the dispatch kinds fired on
// each transition (plan §3.4 — "utan den är mekaniken en tyst
// produktionsförlust, precis vad grind 2 förbjuder").
const (
	EventHexBlockaded   = "HexBlockaded"
	EventHexUnblockaded = "HexUnblockaded"
)

// HexBlockadePayload names the settlement, the hex (q/r — so "⌖ Take me
// there" resolves it directly, megaron_plan_dispatches.md §3/§6:3), and how
// many gubbar stopped or resumed there.
type HexBlockadePayload struct {
	SettlementID uuid.UUID `json:"settlement_id"`
	WorldID      uuid.UUID `json:"world_id"`
	Name         string    `json:"name"`
	Q            int       `json:"q"`
	R            int       `json:"r"`
	Workers      int       `json:"workers"`
}

// BlockadeNotifier is the minimal push-notification capability
// SyncHexBlockade needs — just NotifyPlayer, not the full economy.Broadcaster
// (which also demands BroadcastEvent). Kept to one method so kharis.Broadcaster
// (the hub type kharis/tick.go's TickHandler actually holds) satisfies it
// without a type assertion — G1 lets economy be imported by kharis, not the
// other way round, so this interface (not kharis.Broadcaster) has to be the
// one economy declares.
type BlockadeNotifier interface {
	NotifyPlayer(ctx context.Context, worldID, playerID uuid.UUID, kind string, level int, payload any) error
}

// LoadEnemyPositionedHexes returns every hex in the WORLD (not scoped to any
// one catchment) currently held by a unit belonging to any wanax but
// settlementOwner, standing in fortify or sentry. "Fientlig" = every wanax
// but yourself (plan §3.1) — kingdoms are POST-MVP and gated behind
// KINGDOMS_ENABLED; the day a kingdom-mate exemption ships, it is ONE line
// added to this WHERE clause, not a branch threaded through
// RecomputeProduction.
//
// Deliberately world-wide and NOT radius-limited (unlike belägring's
// siegeCheckRadius pre-filter, economy/siege.go) — this rule has no BFS to
// gate: it is read once per RecomputeProduction call and matched against a
// settlement's ~18 catchment ring hexes in memory, never re-queried per hex
// (megaron_plan_blockad_med_enhet.md §4's performance trap).
func LoadEnemyPositionedHexes(ctx context.Context, tx Tx, worldID, settlementOwner uuid.UUID) (map[hexgrid.Coord]bool, error) {
	rows, err := tx.Query(ctx,
		`SELECT DISTINCT q, r FROM units
		 WHERE world_id = $1 AND owner_id <> $2 AND status = 'positioned'
		   AND stance IN ('fortify', 'sentry') AND q IS NOT NULL AND r IS NOT NULL`,
		worldID, settlementOwner,
	)
	if err != nil {
		return nil, fmt.Errorf("load enemy positioned hexes: %w", err)
	}
	defer rows.Close()
	held := make(map[hexgrid.Coord]bool)
	for rows.Next() {
		var q, r int
		if err := rows.Scan(&q, &r); err != nil {
			return nil, fmt.Errorf("load enemy positioned hexes: scan: %w", err)
		}
		held[hexgrid.Coord{Q: q, R: r}] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load enemy positioned hexes: rows: %w", err)
	}
	return held, nil
}

// SyncHexBlockade detects, for one settlement, which of its HEX placements
// (settlement_placement.target_kind='hex') just crossed from unblocked to
// blocked or back — comparing the live blockade set against each row's own
// persisted `blockaded` flag (mig 142) — writes the new flag, and fires the
// owner's dispatch on every hex that transitioned (plan §3.4). Rule 3 of the
// plan ("gubben står kvar och återupptar") is why this is a flag flip, not a
// placement mutation: the row itself is never touched beyond `blockaded`.
//
// store/hub may be nil (tests, and any future caller with no hub wired) —
// every use is nil-guarded, matching the rest of the codebase's tick
// handlers (e.g. kharis's FoodTickHandler).
//
// Grouped by hex, not by placement row: every gubbe on the same hex shares
// the same blocked/unblocked state (it depends only on the hex's own
// coordinates), so they always transition together — one dispatch per hex,
// naming every gubbe stopped or resumed there, never one dispatch per gubbe.
func SyncHexBlockade(ctx context.Context, tx Tx, store *events.Store, hub BlockadeNotifier, worldID, settlementID uuid.UUID) error {
	var ownerID uuid.UUID
	var name string
	if err := tx.QueryRow(ctx,
		`SELECT owner_id, name FROM settlements WHERE id = $1`, settlementID,
	).Scan(&ownerID, &name); err != nil {
		return fmt.Errorf("sync hex blockade: load settlement: %w", err)
	}
	if ownerID == uuid.Nil {
		return nil // no wanax to dispatch to (e.g. a not-yet-claimed settlement)
	}

	blocked, err := LoadEnemyPositionedHexes(ctx, tx, worldID, ownerID)
	if err != nil {
		return fmt.Errorf("sync hex blockade: %w", err)
	}

	rows, err := tx.Query(ctx,
		`SELECT id, hex_q, hex_r, blockaded FROM settlement_placement
		 WHERE settlement_id = $1 AND target_kind = 'hex'`,
		settlementID,
	)
	if err != nil {
		return fmt.Errorf("sync hex blockade: load placements: %w", err)
	}
	type placementRow struct {
		id  int64
		c   hexgrid.Coord
		was bool
	}
	var placements []placementRow
	for rows.Next() {
		var pr placementRow
		var q, r int
		if err := rows.Scan(&pr.id, &q, &r, &pr.was); err != nil {
			rows.Close()
			return fmt.Errorf("sync hex blockade: scan: %w", err)
		}
		pr.c = hexgrid.Coord{Q: q, R: r}
		placements = append(placements, pr)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("sync hex blockade: rows: %w", err)
	}
	if len(placements) == 0 {
		return nil
	}

	newlyBlocked := make(map[hexgrid.Coord]int)
	newlyLifted := make(map[hexgrid.Coord]int)
	ids := make([]int64, 0, len(placements))
	newVals := make([]bool, 0, len(placements))
	for _, pr := range placements {
		now := blocked[pr.c]
		if now != pr.was {
			if now {
				newlyBlocked[pr.c]++
			} else {
				newlyLifted[pr.c]++
			}
		}
		ids = append(ids, pr.id)
		newVals = append(newVals, now)
	}

	if _, err := tx.Exec(ctx,
		`UPDATE settlement_placement AS sp SET blockaded = v.blockaded
		 FROM unnest($1::bigint[], $2::bool[]) AS v(id, blockaded)
		 WHERE sp.id = v.id`,
		ids, newVals,
	); err != nil {
		return fmt.Errorf("sync hex blockade: write: %w", err)
	}

	dispatch := func(coord hexgrid.Coord, workers int, kind string, level int) {
		payload := HexBlockadePayload{
			SettlementID: settlementID, WorldID: worldID, Name: name,
			Q: coord.Q, R: coord.R, Workers: workers,
		}
		if store != nil {
			_, _ = store.Append(ctx, settlementID, events.StreamProvince, kind, payload, worldID, nil)
		}
		if hub != nil {
			_ = hub.NotifyPlayer(ctx, worldID, ownerID, kind, level, payload)
		}
	}
	for coord, n := range newlyBlocked {
		dispatch(coord, n, EventHexBlockaded, 2)
	}
	for coord, n := range newlyLifted {
		dispatch(coord, n, EventHexUnblockaded, 3)
	}
	return nil
}
