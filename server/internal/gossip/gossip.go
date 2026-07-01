// Package gossip implements the rumor/gossip mechanism: narrative news that
// originates locally (kingdom formation, colony founding, mine/trade activity,
// a settlement falling, ...) and then spreads through the contact graph when a
// messenger reaches a settlement (see internal/messenger's ArrivalHandler).
//
// PASS 2b (temenos_gossip.md): a rumor has weight. MINOR rumors (industry, a
// mine opening, a colony founded, a trade delivered) spread only from
// firsthand witnesses — a recipient who only heard it secondhand (hops>0)
// cannot pass it on. MAJOR rumors (a settlement falls — collapse or conquest)
// spread as hearsay across several hops, because big news travels. See
// PropagateOnContact's gate.
//
// G1: this package is a leaf — it depends only on pgxpool/pgx and uuid, so it
// can be called from any tier (messenger, combat, api/handlers) without
// creating an upward dependency.
package gossip

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Tx is the minimal interface accepted by this package's functions so they
// work with both pgx.Tx and pgxpool.Pool (mirrors economy.Tx).
type Tx interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// Importance levels a rumor can carry (temenos_gossip.md PASS 2b).
const (
	ImportanceMinor = "minor"
	ImportanceMajor = "major"
)

// maxMajorHops bounds how many contact-hops a MAJOR rumor can travel before it
// dies out. MINOR rumors never travel past hops=0 in the first place — see
// PropagateOnContact's gate — so this constant only matters for major news.
const maxMajorHops = 4

// rumorMaxAge is the recency gate for both broadcast and propagation — stale
// rumors do not travel.
const rumorMaxAge = "3 days"

// Broadcast creates a new rumor at sourceSettlementID and delivers it (hops=0)
// to every settlement owner within radius hexes. All recipients share one
// rumor_id, which is what lets the rumor later propagate further via
// PropagateOnContact. ON CONFLICT DO NOTHING makes a re-run harmless.
//
// importance is ImportanceMinor or ImportanceMajor. subjectSettlementID names
// the settlement the rumor is ABOUT (may be uuid.Nil for rumors with no single
// subject, e.g. kingdom formation); when set, every recipient also registers
// that settlement as rumour-known in known_settlements (industryHint carries a
// coarse clue, e.g. "copper", may be ""). See temenos_gossip.md PASS 2b.
func Broadcast(
	ctx context.Context, tx Tx, worldID, sourceSettlementID uuid.UUID,
	category, text string, radius int,
	importance string, subjectSettlementID uuid.UUID, industryHint string,
) error {
	var regionName string
	if err := tx.QueryRow(ctx,
		`SELECT name FROM settlements WHERE id = $1`, sourceSettlementID,
	).Scan(&regionName); err != nil {
		return err
	}

	rows, err := tx.Query(ctx,
		`SELECT DISTINCT s2.owner_id
		 FROM settlements s1
		 JOIN provinces p1 ON p1.id = s1.province_id
		 JOIN provinces p2 ON p2.world_id = p1.world_id
		 JOIN settlements s2 ON s2.province_id = p2.id
		 WHERE s1.id = $1
		   AND s2.owner_id IS NOT NULL
		   AND (ABS(p2.map_q - p1.map_q) + ABS(p2.map_r - p1.map_r) +
		        ABS((p2.map_q + p2.map_r) - (p1.map_q + p1.map_r))) / 2 <= $2`,
		sourceSettlementID, radius,
	)
	if err != nil {
		return err
	}
	var recipients []uuid.UUID
	for rows.Next() {
		var recipID uuid.UUID
		if scanErr := rows.Scan(&recipID); scanErr == nil {
			recipients = append(recipients, recipID)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	var subjectArg *uuid.UUID
	if subjectSettlementID != uuid.Nil {
		subjectArg = &subjectSettlementID
	}
	var hintArg *string
	if industryHint != "" {
		hintArg = &industryHint
	}

	rumorID := uuid.New()
	for _, recipID := range recipients {
		if _, err := tx.Exec(ctx,
			`INSERT INTO gossip_events
			   (world_id, recipient_id, source_region, category, text, rumor_id, hops,
			    importance, subject_settlement_id, industry_hint)
			 VALUES ($1, $2, $3, $4, $5, $6, 0, $7, $8, $9)
			 ON CONFLICT (recipient_id, rumor_id) WHERE rumor_id IS NOT NULL DO NOTHING`,
			worldID, recipID, regionName, category, text, rumorID, importance, subjectArg, hintArg,
		); err != nil {
			return err
		}
		if subjectArg != nil {
			if err := upsertKnownSettlement(ctx, tx, worldID, recipID, *subjectArg, industryHint); err != nil {
				return err
			}
		}
	}
	return nil
}

// PropagateOnContact runs when learnerID's messenger reaches sourceSettlementID.
// The learner picks up a subset of the source settlement owner's ("the
// teacher's") rumors and gains its own copy at hops+1, tagged with the source
// settlement's name as the region they heard it from.
//
// The gate (temenos_gossip.md PASS 2b) is importance-dependent:
//   - MAJOR rumors propagate while hops < maxMajorHops — hearsay travels.
//   - MINOR rumors propagate ONLY when the teacher holds them at hops=0, i.e.
//     the teacher witnessed the event firsthand. A learner who receives a minor
//     rumor this way holds it at hops=1 and, per this same gate, can never pass
//     it on again (their copy is no longer at hops=0).
//
// ON CONFLICT DO NOTHING means a rumor reaches a given player at most once no
// matter how many times or paths it travels — no infinite loops, and
// re-running this handler is safe.
func PropagateOnContact(ctx context.Context, tx Tx, learnerID, sourceSettlementID, worldID uuid.UUID) error {
	var teacherID *uuid.UUID
	if err := tx.QueryRow(ctx,
		`SELECT owner_id FROM settlements WHERE id = $1`, sourceSettlementID,
	).Scan(&teacherID); err != nil {
		return err
	}
	if teacherID == nil || *teacherID == learnerID {
		return nil
	}

	var sourceName string
	if err := tx.QueryRow(ctx,
		`SELECT name FROM settlements WHERE id = $1`, sourceSettlementID,
	).Scan(&sourceName); err != nil {
		return err
	}

	rows, err := tx.Query(ctx,
		`SELECT rumor_id, category, text, hops, importance, subject_settlement_id, industry_hint
		 FROM gossip_events
		 WHERE world_id = $1 AND recipient_id = $2
		   AND rumor_id IS NOT NULL
		   AND ((importance = '`+ImportanceMajor+`' AND hops < $3)
		        OR (importance = '`+ImportanceMinor+`' AND hops = 0))
		   AND generated_at > now() - interval '`+rumorMaxAge+`'
		 ORDER BY generated_at DESC
		 LIMIT 4`,
		worldID, *teacherID, maxMajorHops,
	)
	if err != nil {
		return err
	}

	type rumor struct {
		id         uuid.UUID
		category   string
		text       string
		hops       int
		importance string
		subject    *uuid.UUID
		hint       *string
	}
	var rumors []rumor
	for rows.Next() {
		var rm rumor
		if scanErr := rows.Scan(&rm.id, &rm.category, &rm.text, &rm.hops, &rm.importance, &rm.subject, &rm.hint); scanErr == nil {
			rumors = append(rumors, rm)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, rm := range rumors {
		if _, err := tx.Exec(ctx,
			`INSERT INTO gossip_events
			   (world_id, recipient_id, source_region, category, text, rumor_id, hops,
			    importance, subject_settlement_id, industry_hint)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			 ON CONFLICT (recipient_id, rumor_id) WHERE rumor_id IS NOT NULL DO NOTHING`,
			worldID, learnerID, sourceName, rm.category, rm.text, rm.id, rm.hops+1, rm.importance, rm.subject, rm.hint,
		); err != nil {
			return err
		}
		if rm.subject != nil {
			hint := ""
			if rm.hint != nil {
				hint = *rm.hint
			}
			if err := upsertKnownSettlement(ctx, tx, worldID, learnerID, *rm.subject, hint); err != nil {
				return err
			}
		}
	}
	return nil
}

// upsertKnownSettlement registers subjectSettlementID as rumour-known for
// playerID: a fourth, weaker knowledge tier (temenos_gossip.md PASS 2b) — the
// player has heard OF the settlement (fuzzy position, coarse industry hint)
// but has not seen/remembered/contacted it. Never downgrades: if the player
// already knows this settlement at any level (including a prior rumour row),
// this is a no-op.
func upsertKnownSettlement(ctx context.Context, tx Tx, worldID, playerID, settlementID uuid.UUID, industryHint string) error {
	var hint *string
	if industryHint != "" {
		hint = &industryHint
	}
	_, err := tx.Exec(ctx,
		`INSERT INTO known_settlements (world_id, player_id, settlement_id, level, industry_hint)
		 VALUES ($1, $2, $3, 'rumour', $4)
		 ON CONFLICT (world_id, player_id, settlement_id) DO NOTHING`,
		worldID, playerID, settlementID, hint,
	)
	return err
}
