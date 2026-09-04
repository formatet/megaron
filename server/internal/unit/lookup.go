package unit

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// DisplayNameLoader is the DB access LoadDisplayName needs — a *pgxpool.Pool
// and a pgx.Tx both satisfy it, same shape as ordinalQuerier in naming.go
// (kept separate: the two exist for different reasons and may grow apart).
type DisplayNameLoader interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// LoadDisplayName re-derives the namnstandarden-formatted name for one unit
// at the moment a notification fires (megaron_plan_dispatches.md §4/§6:2,
// Timothy 2026-09-04: "notiserna namnger inte" — a notification that only
// says "Scout" or "A unit" describes a CATEGORY, not the subject a player
// recognizes; the name already exists on every unit, e.g. "Nomadic Host of
// formatet"). This is the same formatting api/handlers/unit.go's
// unitSummaries uses for `unit list` — reused here, not reinvented, so the
// grammar never drifts between the two surfaces ("Servern formaterar,
// aldrig klienterna", naming.go's own rule).
//
// The caller must WRITE the result into the notification payload, never
// just the unit_id: the Notifications archive is a permanent record, and a
// unit can vanish for good later (sunk, disbanded, annexed away), at which
// point a client-side re-lookup would come back empty. A disbanded unit's
// row survives (upkeep.go's attrition/desertion paths only flip status,
// never delete), so this still resolves correctly for the loss notification
// that reports the disbanding itself.
//
// Best-effort: any lookup failure returns "" so the caller falls back to
// its existing generic wording instead of losing the notification outright.
func LoadDisplayName(ctx context.Context, q DisplayNameLoader, unitID uuid.UUID) string {
	var utype, category string
	var ordinal *int
	var supportID *uuid.UUID
	var shipName *string
	var ownerID uuid.UUID
	err := q.QueryRow(ctx,
		`SELECT type, category, ordinal, support_settlement_id, name, owner_id
		 FROM units WHERE id = $1`, unitID,
	).Scan(&utype, &category, &ordinal, &supportID, &shipName, &ownerID)
	if err != nil {
		return ""
	}

	if utype == string(TypeNomadicHost) {
		var wanax string
		_ = q.QueryRow(ctx,
			`SELECT COALESCE(wanax_name, username) FROM players WHERE id = $1`, ownerID,
		).Scan(&wanax)
		return HostName(wanax).DisplayName
	}

	town := ""
	if supportID != nil {
		_ = q.QueryRow(ctx, `SELECT name FROM settlements WHERE id = $1`, *supportID).Scan(&town)
	}

	if category == string(CategoryNaval) {
		sn := ""
		if shipName != nil {
			sn = *shipName
		}
		return ShipDisplayName(utype, sn, town).DisplayName
	}

	ord := 0
	if ordinal != nil {
		ord = *ordinal
	}
	return LandUnitName(utype, ord, town).DisplayName
}
