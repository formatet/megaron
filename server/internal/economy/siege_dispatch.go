package economy

import (
	"context"
	"fmt"

	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/hexgrid"
	"github.com/google/uuid"
)

// EventSiegeStarted / EventSiegeLifted are the dispatch kinds fired on each
// siege transition (plan §Acceptanskriterier 1) — without them, a besieged
// settlement is a silent production loss, exactly what asynchronicity gate 2
// forbids.
const (
	EventSiegeStarted = "SiegeStarted"
	EventSiegeLifted  = "SiegeLifted"
)

// SiegeDispatchPayload names the settlement and its OWN hex (q/r — "⌖ Take
// me there" points at the besieged settlement itself, not the besieger: the
// player wants to see and defend their city, and the besieger can be up to
// siegeCheckRadius hexes away, not a useful navigation target — plan's "Öppen
// designdetalj"). BesiegerName/BesiegerUnit name the largest besieging force
// (LoadBesiegers, ORDER BY size DESC) and are empty for SiegeLifted.
type SiegeDispatchPayload struct {
	SettlementID uuid.UUID `json:"settlement_id"`
	WorldID      uuid.UUID `json:"world_id"`
	Name         string    `json:"name"`
	Q            int       `json:"q"`
	R            int       `json:"r"`
	BesiegerName string    `json:"besieger_name,omitempty"`
	BesiegerUnit string    `json:"besieger_unit,omitempty"`
}

// SyncSiegeState mirrors SyncHexBlockade (hex_blockade.go) with one
// architectural difference, spelled out below. It detects the START/END
// TRANSITION of a settlement's siege and fires the owner's dispatch.
//
// The difference from SyncHexBlockade: settlement_placement.blockaded (mig
// 142) is written by SyncHexBlockade itself, so that syncer can compare live
// state against its own flag. settlements.besieged (mig 122) is instead
// written by RecomputeProduction (recompute.go ~line 622) BEFORE this
// function runs each tick — by the time SyncSiegeState reads besieged, it is
// already this tick's fresh value, not "yesterday's". Comparing besieged
// against itself would always see no change. siege_notified (mig 143) is
// this function's OWN memory of the last state it dispatched for, untouched
// by RecomputeProduction, which is what makes the comparison possible.
//
// store/hub may be nil (tests, and any future caller with no hub wired) —
// both nil-guarded, matching SyncHexBlockade.
func SyncSiegeState(ctx context.Context, tx Tx, store *events.Store, hub BlockadeNotifier, worldID, settlementID uuid.UUID) error {
	var ownerID uuid.UUID
	var name string
	var besieged, siegeNotified bool
	var q, r int
	if err := tx.QueryRow(ctx,
		`SELECT s.owner_id, s.name, s.besieged, s.siege_notified, prov.map_q, prov.map_r
		 FROM settlements s JOIN provinces prov ON prov.id = s.province_id
		 WHERE s.id = $1`,
		settlementID,
	).Scan(&ownerID, &name, &besieged, &siegeNotified, &q, &r); err != nil {
		return fmt.Errorf("sync siege state: load settlement: %w", err)
	}
	if ownerID == uuid.Nil {
		return nil // no wanax to dispatch to (e.g. a not-yet-claimed settlement)
	}
	if besieged == siegeNotified {
		return nil // no transition since the last sync
	}

	if _, err := tx.Exec(ctx,
		`UPDATE settlements SET siege_notified = $2 WHERE id = $1`,
		settlementID, besieged,
	); err != nil {
		return fmt.Errorf("sync siege state: write: %w", err)
	}

	payload := SiegeDispatchPayload{
		SettlementID: settlementID, WorldID: worldID, Name: name,
		Q: q, R: r,
	}
	kind := EventSiegeLifted
	level := 3
	if besieged {
		kind = EventSiegeStarted
		level = 2
		center := hexgrid.Coord{Q: q, R: r}
		besiegers, err := LoadBesiegers(ctx, tx, worldID, ownerID, center)
		if err != nil {
			return fmt.Errorf("sync siege state: load besiegers: %w", err)
		}
		if len(besiegers) > 0 {
			payload.BesiegerName = besiegers[0].OwnerName
			payload.BesiegerUnit = besiegers[0].UnitType
		}
	}

	if store != nil {
		_, _ = store.Append(ctx, settlementID, events.StreamProvince, kind, payload, worldID, nil)
	}
	if hub != nil {
		_ = hub.NotifyPlayer(ctx, worldID, ownerID, kind, level, payload)
	}
	return nil
}
