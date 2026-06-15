package handlers

// seedStarterUnits creates the initial garrison units for a new settlement (C7).
//
// Coast terrain → 1 galley (crew 20) + 1 infantry (100 men).
// Inland terrain → 2 infantry (100 men each).
//
// All units are immediately garrison-ready (status = 'garrison').
// Population is decremented by total men drawn (land: 100/unit; naval: crew).
// Dual-write: old integer army columns on settlements are updated in parallel.
// A UnitFormed event is appended to the unit stream for each unit created.
//
// Must be called inside an open pgx.Tx; the transaction is NOT committed here.
// Idempotency: protected by the outer join/respawn idempotency check (the
// settlement only exists once; if the call is repeated it finds existing units).

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/poleia/server/internal/events"
	"github.com/poleia/server/internal/unit"
)

// isCoastalTerrain returns true for terrain types that give naval starter units.
func isCoastalTerrain(terrain string) bool {
	return terrain == "coast" || terrain == "coastal_sea"
}

// seedStarterUnits inserts garrison units for the new settlement and adjusts population.
func seedStarterUnits(
	ctx context.Context,
	tx pgx.Tx,
	eventStore *events.Store,
	settlementID, ownerID, worldID uuid.UUID,
	q, r int,
	terrain string,
) error {
	type spec struct {
		utype  unit.Type
		size   int // land men or naval size (always 1)
		crew   int // 0 for land
		popCost int // men drawn from population
		dualCol string // settlements integer column to dual-write
		dualVal int    // value to add to that column
	}

	var units []spec
	if isCoastalTerrain(terrain) {
		// Kuststad: galley (besättning 20) + 1 infanterienhet (100 man)
		units = []spec{
			{utype: unit.TypeGalley, size: 1, crew: 20, popCost: 20, dualCol: "ship", dualVal: 20},
			{utype: unit.TypeInfantry, size: 100, crew: 0, popCost: 100, dualCol: "infantry", dualVal: 100},
		}
	} else {
		// Inlandsstad: 2 infanterienheter (2 × 100 man)
		units = []spec{
			{utype: unit.TypeInfantry, size: 100, crew: 0, popCost: 100, dualCol: "infantry", dualVal: 100},
			{utype: unit.TypeInfantry, size: 100, crew: 0, popCost: 100, dualCol: "infantry", dualVal: 100},
		}
	}

	totalPopDrawn := 0
	for _, sp := range units {
		totalPopDrawn += sp.popCost
	}

	// Deduct population for all starter units at once.
	if _, err := tx.Exec(ctx,
		`UPDATE settlements SET population = population - $1 WHERE id = $2`,
		totalPopDrawn, settlementID,
	); err != nil {
		return fmt.Errorf("deduct starter population: %w", err)
	}

	for _, sp := range units {
		cat := unit.CategoryOf(sp.utype)

		var unitID uuid.UUID
		if err := tx.QueryRow(ctx,
			`INSERT INTO units
			   (world_id, owner_id, type, category, size, crew, status, settlement_id)
			 VALUES ($1, $2, $3, $4, $5, $6, 'garrison', $7)
			 RETURNING id`,
			worldID, ownerID, string(sp.utype), string(cat),
			sp.size, sp.crew, settlementID,
		).Scan(&unitID); err != nil {
			return fmt.Errorf("insert starter unit %s: %w", sp.utype, err)
		}

		// Dual-write: update old integer army column so legacy combat/display works.
		if _, err := tx.Exec(ctx,
			fmt.Sprintf(`UPDATE settlements SET %s = %s + $1 WHERE id = $2`, sp.dualCol, sp.dualCol),
			sp.dualVal, settlementID,
		); err != nil {
			return fmt.Errorf("dual-write %s for starter unit: %w", sp.dualCol, err)
		}

		// Emit UnitFormed event (outcome, not intention; idempotency lives in join check).
		payload := unit.UnitFormedPayload{
			UnitID:       unitID,
			OwnerID:      ownerID,
			WorldID:      worldID,
			SettlementID: settlementID,
			UnitType:     string(sp.utype),
			Category:     string(cat),
			InitialSize:  sp.size,
			Crew:         sp.crew,
			PopDrawn:     sp.popCost,
		}
		if _, err := eventStore.Append(ctx, settlementID, events.StreamType(unit.StreamUnit),
			unit.EventUnitFormed, payload, worldID, nil,
		); err != nil {
			return fmt.Errorf("append UnitFormed for starter unit %s: %w", sp.utype, err)
		}
	}

	return nil
}
