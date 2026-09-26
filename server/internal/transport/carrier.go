// Carrier binding for naval transports (megaron_plan_sjohandel_kraver_skepp.md).
//
// Until this slice, ResolveTradeRoute could hand back "naval" for a trade/
// transfer/standing-order leg without any real vessel ever changing hands — the
// sea route was priced and timed, but no galley/merchantman was ever taken out
// of service to sail it. This file is the substrate every naval caller (the
// Trade handler in api/handlers, the standing-order sweep in internal/combat)
// binds a real ship through: find one, bind it, release it later.
//
// R1: allowed carrier types are galley and merchantman ONLY — never war_galley
// (it may never carry cargo). Capacity is expressed in the same weight units
// economy.IsShippableGood already prices goods in (quantity × weight); callers
// compute a manifest's total weight themselves (transport may not import
// economy — see intercept.go's own note on the same G1 boundary) and compare
// it against the capacity this file reports.
package transport

import (
	"context"
	"errors"
	"fmt"

	"formatet/megaron/server/internal/province"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Ship carrying capacity, in weight units (strawman — Timothy 2026-09-26,
// "okalibrerat, justeras med prissättningen"). merchantman is the everyday
// trade hull (crew 10) and carries more than a galley (crew 20, dual-purpose
// combat/transport) — the plan's own numbers, not derived from crew size.
const (
	ShipCapacityGalley      = 60.0
	ShipCapacityMerchantman = 200.0
)

// ShipCapacityFor returns the carrying capacity for an allowed carrier type.
// ok is false for any type that may never carry cargo — including
// "war_galley", which must NEVER be selected as a carrier (R1) — and for any
// unrecognized type.
func ShipCapacityFor(shipType string) (capacity float64, ok bool) {
	switch shipType {
	case "merchantman":
		return ShipCapacityMerchantman, true
	case "galley":
		return ShipCapacityGalley, true
	default:
		return 0, false
	}
}

// FreeShip is a carrier found idle in a settlement, locked and ready to bind.
type FreeShip struct {
	ID       uuid.UUID
	Type     string
	Name     *string
	Capacity float64
}

// FindFreeShip locks and returns the owner's best free galley/merchantman
// standing in settlementID: merchantman before galley (R1 — the bigger,
// everyday trade hull first), tie-broken deterministically by created_at then
// id. "Free" means owned by the caller, docked (status='garrison'), in this
// settlement, and not already carrying land-unit cargo.
//
// Uses FOR UPDATE SKIP LOCKED so a concurrent caller racing for a DIFFERENT
// free ship in the same city is not serialized behind this one, while two
// callers racing for the SAME (only) free ship never both get it — the loser
// sees zero rows, exactly the "double-binding impossible" invariant (R2).
// found=false means no eligible ship exists right now; the caller decides
// whether to fall back to land or reject.
func FindFreeShip(ctx context.Context, tx pgx.Tx, worldID, ownerID, settlementID uuid.UUID) (FreeShip, bool, error) {
	var s FreeShip
	err := tx.QueryRow(ctx,
		`SELECT id, type, name FROM units
		 WHERE world_id = $1 AND owner_id = $2 AND settlement_id = $3
		   AND status = 'garrison' AND cargo_unit_id IS NULL
		   AND type IN ('galley', 'merchantman')
		 ORDER BY CASE type WHEN 'merchantman' THEN 0 ELSE 1 END, created_at, id
		 LIMIT 1
		 FOR UPDATE SKIP LOCKED`,
		worldID, ownerID, settlementID,
	).Scan(&s.ID, &s.Type, &s.Name)
	if errors.Is(err, pgx.ErrNoRows) {
		return FreeShip{}, false, nil
	}
	if err != nil {
		return FreeShip{}, false, fmt.Errorf("find free ship: %w", err)
	}
	cap, _ := ShipCapacityFor(s.Type)
	s.Capacity = cap
	return s, true, nil
}

// BindShip flips a ship found by FindFreeShip (same transaction — the caller
// still holds its row lock) to 'freighting'. Guarded by the status='garrison'
// WHERE so a caller that skips FindFreeShip's lock never silently re-binds an
// already-bound ship; RowsAffected()==0 is reported as an error.
func BindShip(ctx context.Context, tx pgx.Tx, shipID uuid.UUID) error {
	tag, err := tx.Exec(ctx,
		`UPDATE units SET status = 'freighting', updated_at = now() WHERE id = $1 AND status = 'garrison'`,
		shipID,
	)
	if err != nil {
		return fmt.Errorf("bind ship: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("bind ship: %s was not a free (garrison) ship", shipID)
	}
	return nil
}

// ReleaseShip frees a bound ship back to garrison at settlementID, its new
// home port. Guarded by status='freighting' — releasing a ship that already
// isn't bound (e.g. a re-run of an idempotent arrival handler) is a no-op,
// never an error.
func ReleaseShip(ctx context.Context, tx pgx.Tx, shipID, settlementID uuid.UUID) error {
	_, err := tx.Exec(ctx,
		`UPDATE units SET status = 'garrison', settlement_id = $2, updated_at = now()
		 WHERE id = $1 AND status = 'freighting'`,
		shipID, settlementID,
	)
	if err != nil {
		return fmt.Errorf("release ship: %w", err)
	}
	return nil
}

// StrandShip is R3's last-resort release: the ship's home port no longer
// exists AND the owner has no other settlement to return it to. The ship is
// left `positioned` on the sea hex it last reached rather than vanishing or
// staying permanently 'freighting' with nowhere to report to — a Wanax who
// logs in finds a real, orderable unit sitting on the map, not a silent loss.
func StrandShip(ctx context.Context, tx pgx.Tx, shipID uuid.UUID, q, r int) error {
	_, err := tx.Exec(ctx,
		`UPDATE units SET status = 'positioned', settlement_id = NULL, q = $2, r = $3, updated_at = now()
		 WHERE id = $1 AND status = 'freighting'`,
		shipID, q, r,
	)
	if err != nil {
		return fmt.Errorf("strand ship: %w", err)
	}
	return nil
}

// NearestOwnPort resolves where a bound ship should be released when it comes
// home: the owner's nearest active settlement with a shipyard, falling back
// to the owner's nearest active settlement at all (mirrors
// combat.nearestOwnShipyardSettlement's own fallback reasoning — a returning
// ship must never be stranded just because the Wanax hasn't built a shipyard
// yet). transport may not import combat (G1: combat sits ABOVE transport), so
// this is its own small copy of the same query rather than a shared call.
// found=false means the owner has no settlement left at all — the caller
// falls back to StrandShip.
func NearestOwnPort(ctx context.Context, tx pgx.Tx, worldID, ownerID uuid.UUID, fromQ, fromR int) (settlementID uuid.UUID, q, r int, found bool, err error) {
	rows, qErr := tx.Query(ctx,
		`SELECT s.id, p.map_q, p.map_r,
		        EXISTS(SELECT 1 FROM buildings b WHERE b.settlement_id = s.id AND b.building_type = 'shipyard') AS has_shipyard
		 FROM settlements s JOIN provinces p ON p.id = s.province_id
		 WHERE s.owner_id = $1 AND s.world_id = $2 AND s.state = 'active'`,
		ownerID, worldID,
	)
	if qErr != nil {
		return uuid.Nil, 0, 0, false, qErr
	}
	defer rows.Close()

	type candidate struct {
		id          uuid.UUID
		q, r        int
		hasShipyard bool
	}
	var withYard, all []candidate
	for rows.Next() {
		var c candidate
		if scanErr := rows.Scan(&c.id, &c.q, &c.r, &c.hasShipyard); scanErr != nil {
			return uuid.Nil, 0, 0, false, scanErr
		}
		all = append(all, c)
		if c.hasShipyard {
			withYard = append(withYard, c)
		}
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return uuid.Nil, 0, 0, false, rowsErr
	}

	pick := withYard
	if len(pick) == 0 {
		pick = all
	}
	if len(pick) == 0 {
		return uuid.Nil, 0, 0, false, nil
	}

	best := pick[0]
	bestDist := province.HexDistance(province.MapPosition{Q: fromQ, R: fromR}, province.MapPosition{Q: best.q, R: best.r})
	for _, c := range pick[1:] {
		d := province.HexDistance(province.MapPosition{Q: fromQ, R: fromR}, province.MapPosition{Q: c.q, R: c.r})
		if d < bestDist {
			best, bestDist = c, d
		}
	}
	return best.id, best.q, best.r, true, nil
}
