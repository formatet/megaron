// Package gossip implements the rumor/gossip mechanism: narrative news that
// originates locally (kingdom formation, colony founding, ...) and then
// spreads transitively through the contact graph when a messenger reaches a
// settlement (see internal/messenger's ArrivalHandler).
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

// maxHops bounds how many contact-hops a rumor can travel before it dies out.
const maxHops = 3

// rumorMaxAge is the recency gate for both broadcast and propagation — stale
// rumors do not travel.
const rumorMaxAge = "3 days"

// Broadcast creates a new rumor at sourceSettlementID and delivers it (hops=0)
// to every settlement owner within radius hexes. All recipients share one
// rumor_id, which is what lets the rumor later propagate further via
// PropagateOnContact. ON CONFLICT DO NOTHING makes a re-run harmless.
func Broadcast(ctx context.Context, tx Tx, worldID, sourceSettlementID uuid.UUID, category, text string, radius int) error {
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

	rumorID := uuid.New()
	for _, recipID := range recipients {
		if _, err := tx.Exec(ctx,
			`INSERT INTO gossip_events (world_id, recipient_id, source_region, category, text, rumor_id, hops)
			 VALUES ($1, $2, $3, $4, $5, $6, 0)
			 ON CONFLICT (recipient_id, rumor_id) WHERE rumor_id IS NOT NULL DO NOTHING`,
			worldID, recipID, regionName, category, text, rumorID,
		); err != nil {
			return err
		}
	}
	return nil
}

// PropagateOnContact runs when learnerID's messenger reaches sourceSettlementID.
// The learner picks up the source settlement owner's freshest rumors (that
// haven't already died out at hops>=maxHops) and gains its own copy at
// hops+1, tagged with the source settlement's name as the region they heard
// it from. ON CONFLICT DO NOTHING means a rumor reaches a given player at most
// once no matter how many times or paths it travels — no infinite loops, and
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
		`SELECT rumor_id, category, text, hops
		 FROM gossip_events
		 WHERE world_id = $1 AND recipient_id = $2
		   AND rumor_id IS NOT NULL AND hops < $3
		   AND generated_at > now() - interval '`+rumorMaxAge+`'
		 ORDER BY generated_at DESC
		 LIMIT 4`,
		worldID, *teacherID, maxHops,
	)
	if err != nil {
		return err
	}

	type rumor struct {
		id       uuid.UUID
		category string
		text     string
		hops     int
	}
	var rumors []rumor
	for rows.Next() {
		var rm rumor
		if scanErr := rows.Scan(&rm.id, &rm.category, &rm.text, &rm.hops); scanErr == nil {
			rumors = append(rumors, rm)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, rm := range rumors {
		if _, err := tx.Exec(ctx,
			`INSERT INTO gossip_events (world_id, recipient_id, source_region, category, text, rumor_id, hops)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)
			 ON CONFLICT (recipient_id, rumor_id) WHERE rumor_id IS NOT NULL DO NOTHING`,
			worldID, learnerID, sourceName, rm.category, rm.text, rm.id, rm.hops+1,
		); err != nil {
			return err
		}
	}
	return nil
}
