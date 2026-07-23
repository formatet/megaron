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
// The two spearmen cohorts sit ON TOP of these civilians — 4 200 people leave the
// starting line, and only the 4 000 become the metropolis's population at founding
// (decision Timothy 2026-07-15). Soldiers are separate from population throughout.
const (
	nomadicHostPopulation   = 4000
	nomadicHostSpearmen     = 2
	nomadicHostSpearmenSize = 100 // men per cohort

	// nomadicHostRationTicks is how long the escort's pay and rations last:
	// four game months (2 880 ticks = 120 game days at 24 ticks/day).
	//
	// The figure is sized in the player's time, not the world's: at the intended
	// cadence (TICK_MINUTES=2, ~1 game month per real day) this is ~4 real days —
	// long enough to sleep on the decision, which an async game must allow. Four
	// game DAYS, the number the design first named, would have been ~3 real hours:
	// log off overnight, come back to a starved escort.
	//
	// It is affordable only because the host itself eats nothing: 2 880 ticks of
	// escort keep is ~1 200 grain and ~480 silver, which is modest to carry into
	// the new metropolis (an ordinary city seeds with 300 grain). Were the 4 000
	// civilians fed from this store it would be ~241 000 grain and would drown the
	// opening's scarcity outright. (Decision Timothy 2026-07-15.)
	nomadicHostRationTicks = 2880
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
	// The host token: one movable marker. Its 4 000 people live in
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

	// The store feeds and pays the ESCORT ONLY. The host itself has no upkeep at
	// all while it wanders (decision Timothy 2026-07-15): a people on the move
	// forages as it goes, and does not eat like a city of 4 000 sitting still.
	// That is also what keeps the founding honest — were the 4 000 fed from the
	// store, four days of city rations would pour into the new metropolis and
	// drown the geography→scarcity pressure the opening exists to create.
	//
	// Rates come from the same functions the settled game uses, never hardcoded,
	// so a calibration change moves the founder phase with it. UpkeepSpecs is per
	// DAY (combat/upkeep.go:13, fired every TicksPerDay) and must be divided down
	// before it can sit beside a per-tick rate.
	perDay := combat.UnitUpkeep(string(unit.TypeSpearman), string(unit.CategoryLand), nomadicHostSpearmenSize)
	grainRate := -float64(nomadicHostSpearmen) * perDay.Grain / float64(events.TicksPerDay)
	silverRate := -float64(nomadicHostSpearmen) * perDay.Silver / float64(events.TicksPerDay)

	// Amounts follow from the rates, so the two can never disagree about how long
	// the escort lasts.
	grainAmount := -grainRate * nomadicHostRationTicks
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
	// city's population to draw from (the cohorts ride on top of the 4 000).
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
