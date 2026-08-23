package handlers

import (
	"context"
	"fmt"

	"formatet/megaron/server/internal/combat"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/unit"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// The founder phase's opening figures (temenos_nomadic_host_plan.md §Grundregler).
// The two spearmen cohorts sit ON TOP of these civilians — 1 200 people leave the
// starting line, and only the 1 000 become the metropolis's population at founding
// (decision Timothy 2026-08-23, replacing the original 4 000 set Timothy 2026-07-15:
// 4 000 = 40 gubbar, and no catchment — 18 workable hexes, grain caps 4/8/10/12 per
// hex by farm level — can absorb 40 while the city still eats for all 4 000; 1 000 =
// 10 gubbar fits under the caps with room to spare, so a fresh metropolis can
// actually feed itself). Soldiers are separate from population throughout.
const (
	nomadicHostPopulation   = 1000
	nomadicHostSpearmen     = 2
	nomadicHostSpearmenSize = 100 // men per cohort

	// nomadicHostRationTicks is how long the escort's pay lasts, in ticks.
	//
	// Master retuned 2880 → 120 for tick=day (Timothy 2026-08-06): rates are ×24
	// after mig 109 and the store derives below as rate × ration_ticks, so 2 880
	// would have blown the opening silver 480 → 11 520. 120 keeps the calibrated
	// 480 with the UN-halved silver column. SLICE B (Timothy 2026-08-05) then
	// halved that column (silver 2 → 1), which on its own drops the store
	// 480 → 240 — so the ration doubles to 240, exactly as master's own retune
	// note foresaw ("balans/silverupkeep-halveras doubles it to 240"). Net:
	// 2 spearmen × 1 silver × 240 ticks = 480, the calibrated opening, restored.
	//
	// It is affordable only because the host itself eats nothing. Were the 1 000
	// civilians fed from this store it would drown the opening's scarcity outright.
	// (Original decision Timothy 2026-07-15.)
	//
	// Left UNCHANGED by the 2026-08-23 population cut (4 000 → 1 000): this is the
	// escort's own sold, paid for the two spearmen cohorts only — it does not
	// scale with civilian headcount and nothing above derives it from population.
	nomadicHostRationTicks = 240

	// nomadicHostDowryGrain is the grain the horde carries into the metropolis
	// at founding — a BALANCE FIGURE, not a derivation. It was originally
	// derived from the upkeep table (2 spearmen × 5 grain ÷ ... = 1200), but
	// SLICE A (Timothy 2026-08-05) recalibrated soldier grain upkeep (garrison =
	// a civilian's ration, field = double) for reasons that have nothing to do
	// with the dowry — re-deriving it from the same formula would have silently
	// twentyfolded it to 24000. Kanon 2026-08-05: "horden äter inget" fixed the
	// RATE (grainRate = 0), not the AMOUNT. This constant is now frozen at the
	// pre-recalibration value and changes only when someone deliberately changes
	// the dowry itself.
	//
	// Left UNCHANGED by the 2026-08-23 population cut (4 000 → 1 000): a fixed
	// dowry against a quarter of the old population is relatively four times more
	// generous per capita, which is a real balance effect — but it's a SEPARATE
	// spak from the population number, and Timothy has not decided on it.
	// Touching both in one slice would make the population change impossible to
	// measure on its own. Flagged, not silently absorbed.
	nomadicHostDowryGrain = 1200
)

// seedNomadicHost creates a player's founder phase: the host token, its two
// free-standing spearmen cohorts, and the locked store that feeds them all.
// It runs inside the caller's transaction and does NOT commit.
//
// No settlement, no province: those are born at founding. The host stands on the
// map (status 'positioned', q/r set, settlement_id NULL), which is also why
// combat.UpkeepHandler must skip these units — it processes 'positioned' and
// would bill the cohorts a second time (temenos_nomadic_host_bygg.md B3).
func seedNomadicHost(
	ctx context.Context,
	tx pgx.Tx,
	eventStore *events.Store,
	worldID, playerID uuid.UUID,
	q, r int,
) (uuid.UUID, error) {
	// The host token: one movable marker. Its 1 000 people live in
	// founder_phase.population — units.size is 0–100 for land and means men.
	var hostID uuid.UUID
	if err := tx.QueryRow(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, q, r)
		 VALUES ($1, $2, $3, $4, 1, 0, 'positioned', $5, $6)
		 RETURNING id`,
		worldID, playerID, string(unit.TypeNomadicHost), string(unit.CategoryOf(unit.TypeNomadicHost)), q, r,
	).Scan(&hostID); err != nil {
		return uuid.Nil, fmt.Errorf("insert nomadic host: %w", err)
	}

	// The escort: two ordinary spearmen cohorts, standing with the host. They are
	// ordinary units in every way except who pays them (the store, until founding).
	spearIDs := make([]uuid.UUID, 0, nomadicHostSpearmen)
	for i := 0; i < nomadicHostSpearmen; i++ {
		var id uuid.UUID
		if err := tx.QueryRow(ctx,
			`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, q, r)
			 VALUES ($1, $2, $3, $4, $5, 0, 'positioned', $6, $7)
			 RETURNING id`,
			worldID, playerID, string(unit.TypeSpearman), string(unit.CategoryLand),
			nomadicHostSpearmenSize, q, r,
		).Scan(&id); err != nil {
			return uuid.Nil, fmt.Errorf("insert host spearman %d: %w", i+1, err)
		}
		spearIDs = append(spearIDs, id)
	}

	// The store pays the ESCORT's sold. Kanon 2026-08-05 (temenos_enheter.md
	// §"Canon 2026-08-05"): the horde AND its escort live on subsistence up to
	// founding — a people on the move carries its own rations and does not
	// draw from a granary that does not exist. Grain is therefore not drained
	// at all; it is carried: the whole store is poured into the metropolis at
	// founding as a dowry (Timothy 2026-08-05). Sold IS still paid — it is
	// owed to men who serve, unlike food to a people feeding itself.
	//
	// Silver comes from the same function the settled game uses, never hardcoded,
	// so a calibration change moves the founder phase with it. UnitUpkeep is per
	// tick (combat/upkeep.go — a tick is a day), so it sits beside a per-tick rate
	// directly. Status is "positioned" — the horde stands on the map before
	// founding — but silver is statusoberoende (UnitUpkeep never doubles it), so
	// the status doesn't change the figure; it is just the honest status.
	perTick := combat.UnitUpkeep(string(unit.TypeSpearman), string(unit.CategoryLand), nomadicHostSpearmenSize, "positioned")
	grainRate := 0.0
	silverRate := -float64(nomadicHostSpearmen) * perTick.Silver

	// grainAmount is the named dowry constant (SLICE A, 2026-08-05) — see
	// nomadicHostDowryGrain's comment: it stopped following the upkeep table
	// when that table was recalibrated for an unrelated reason. silverAmount
	// still follows its rate, so silver alone still tracks the upkeep table.
	grainAmount := float64(nomadicHostDowryGrain)
	silverAmount := -silverRate * nomadicHostRationTicks

	// Upsert, not insert: founder_phase is unique per (world, owner), and a Wanax
	// who founded once keeps that row forever (active=false, founded_tick set).
	// A plain INSERT therefore raised 23505 and surfaced as a 500 for anyone who
	// had lost every settlement — a Wanax whose last city fell could never take
	// the field again, and the server's answer was an opaque
	// "could not create nomadic host" (soak 2026-07-22, two sacked daemons).
	//
	// Reaching this call already proves the player is landless: Join returns
	// early both when a settlement exists and when an active phase exists. So the
	// conflict case is exactly "begin again", and it resets the whole row — new
	// host, full rations, founded_tick cleared — rather than leaving a half-spent
	// phase behind.
	if _, err := tx.Exec(ctx,
		`INSERT INTO founder_phase
		   (world_id, owner_id, host_unit_id, population,
		    grain_amount, grain_rate, silver_amount, silver_rate, calc_tick, active)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, current_world_tick(), true)
		 ON CONFLICT (world_id, owner_id) DO UPDATE SET
		   host_unit_id  = EXCLUDED.host_unit_id,
		   population    = EXCLUDED.population,
		   grain_amount  = EXCLUDED.grain_amount,
		   grain_rate    = EXCLUDED.grain_rate,
		   silver_amount = EXCLUDED.silver_amount,
		   silver_rate   = EXCLUDED.silver_rate,
		   calc_tick     = EXCLUDED.calc_tick,
		   founded_tick  = NULL,
		   active        = true`,
		worldID, playerID, hostID, nomadicHostPopulation,
		grainAmount, grainRate, silverAmount, silverRate,
	); err != nil {
		return uuid.Nil, fmt.Errorf("insert founder phase: %w", err)
	}

	// UnitFormed for each unit, on its own stream: these units have no settlement
	// to be an aggregate of yet. PopDrawn is 0 — the host's people were never in a
	// city's population to draw from (the cohorts ride on top of the 1 000).
	formed := append([]uuid.UUID{hostID}, spearIDs...)
	types := append([]unit.Type{unit.TypeNomadicHost},
		make([]unit.Type, nomadicHostSpearmen)...)
	sizes := append([]int{1}, make([]int, nomadicHostSpearmen)...)
	for i := 1; i <= nomadicHostSpearmen; i++ {
		types[i] = unit.TypeSpearman
		sizes[i] = nomadicHostSpearmenSize
	}

	for i, id := range formed {
		payload := unit.UnitFormedPayload{
			UnitID:      id,
			OwnerID:     playerID,
			WorldID:     worldID,
			UnitType:    string(types[i]),
			Category:    string(unit.CategoryOf(types[i])),
			InitialSize: sizes[i],
			Crew:        0,
			PopDrawn:    0,
		}
		if _, err := eventStore.Append(ctx, id, events.StreamType(unit.StreamUnit),
			unit.EventUnitFormed, payload, worldID, nil,
		); err != nil {
			return uuid.Nil, fmt.Errorf("append UnitFormed for %s: %w", types[i], err)
		}
	}

	return hostID, nil
}
