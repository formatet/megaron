package handlers

import (
	"context"
	"fmt"

	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/kharis"
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
// DefaultWorldStartWanaxes is how many Wanaxes must have joined before the
// world clock begins, when POLEIA_WORLD_START_WANAXES is unset (Timothy
// 2026-09-10; made a server setting 2026-09-25). Until then a world stays
// 'forming': players can join, see their host and the land around it, file
// reports — but give no orders (RequireStartedWorld, world_guard.go). The live
// threshold is read once at process start in cmd/server and injected into
// JoinHandler, WorldHandler and RequireStartedWorld. To start a world early or
// by hand:
//
//	UPDATE worlds SET state = 'active', last_tick_at = now() WHERE id = '…';
//
// (last_tick_at must be reset, or the tick worker's catch-up races through
// every tick of the wait — see the transition in join.go.)
const DefaultWorldStartWanaxes = 4

const (
	nomadicHostPopulation   = 1000
	nomadicHostSpearmen     = 2
	nomadicHostSpearmenSize = 100 // men per cohort

	// Fixed opening reserve (Timothy 2026-10-13): no starting amount changes
	// when ordinary spearmen stop drawing silver. Not population or payroll.
	nomadicHostStartingSilver = 480.0

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
// hostSpec is what a host sets out with. A newcomer gets newcomerHost; a Wanax
// rising again after losing every city gets a smaller one (Rise, rise.go).
type hostSpec struct {
	population int     // civilians, founder_phase.population
	spearmen   int     // escort cohorts of nomadicHostSpearmenSize men
	silver     float64 // fixed opening reserve, independent of upkeep
	refugee    bool    // returning hosts never receive a founding top-up
}

var newcomerHost = hostSpec{population: nomadicHostPopulation, spearmen: nomadicHostSpearmen, silver: nomadicHostStartingSilver}

func seedNomadicHost(
	ctx context.Context,
	tx pgx.Tx,
	eventStore *events.Store,
	worldID, playerID uuid.UUID,
	q, r int,
) (uuid.UUID, error) {
	return seedHost(ctx, tx, eventStore, worldID, playerID, q, r, newcomerHost)
}

func seedHost(
	ctx context.Context,
	tx pgx.Tx,
	eventStore *events.Store,
	worldID, playerID uuid.UUID,
	q, r int,
	spec hostSpec,
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
	//
	// Each cohort carries its regiment number from the start (Timothy 2026-09-25:
	// two identical "Spearmen" could not be told apart). Before founding there is
	// no settlement, so there is no unit_ordinals counter to draw from (it is
	// keyed on settlement_id) — but a fresh Wanax owns no other spearmen, so the
	// escort is simply 1st and 2nd, and renders "1st Spearmen of <Wanax>"
	// (unit.LandUnitName). At founding foundMetropolis re-draws both numbers from
	// the metropolis's counter in this same order, so 1st stays 1st.
	spearIDs := make([]uuid.UUID, 0, spec.spearmen)
	for i := 0; i < spec.spearmen; i++ {
		var id uuid.UUID
		if err := tx.QueryRow(ctx,
			`INSERT INTO units (world_id, owner_id, type, category, size, crew, status, q, r, ordinal)
			 VALUES ($1, $2, $3, $4, $5, 0, 'positioned', $6, $7, $8)
			 RETURNING id`,
			worldID, playerID, string(unit.TypeSpearman), string(unit.CategoryLand),
			nomadicHostSpearmenSize, q, r, i+1,
		).Scan(&id); err != nil {
			return uuid.Nil, fmt.Errorf("insert host spearman %d: %w", i+1, err)
		}
		spearIDs = append(spearIDs, id)
	}

	// The host and escort live on subsistence until founding. Neither food
	// nor silver drains from this fixed reserve (Timothy 2026-10-13).
	grainRate := 0.0
	silverRate := 0.0
	grainAmount := float64(nomadicHostDowryGrain)
	silverAmount := spec.silver

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
		    grain_amount, grain_rate, silver_amount, silver_rate, calc_tick, active, is_refugee)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, current_world_tick(), true, $9)
		 ON CONFLICT (world_id, owner_id) DO UPDATE SET
		   host_unit_id  = EXCLUDED.host_unit_id,
		   population    = EXCLUDED.population,
		   grain_amount  = EXCLUDED.grain_amount,
		   grain_rate    = EXCLUDED.grain_rate,
		   silver_amount = EXCLUDED.silver_amount,
		   silver_rate   = EXCLUDED.silver_rate,
		   calc_tick     = EXCLUDED.calc_tick,
		   founded_tick  = NULL,
		   active        = true,
		   is_refugee    = EXCLUDED.is_refugee`,
		worldID, playerID, hostID, spec.population,
		grainAmount, grainRate, silverAmount, silverRate, spec.refugee,
	); err != nil {
		return uuid.Nil, fmt.Errorf("insert founder phase: %w", err)
	}

	// Both join and rise begin a new Host life here. With no capital there is
	// no geographic Kharis income. Keep the last city's witness and founding
	// tick and settle the existing relationship, bounded by the shared floor, in this
	// world's tick (also valid before a forming world starts).
	if _, err := tx.Exec(ctx,
		`INSERT INTO player_world_records (player_id, world_id, status, kharis_amount, kharis_rate, kharis_calc_tick)
		 VALUES ($1, $2, 'active', $3, 0, (SELECT current_tick FROM worlds WHERE id = $2))
		 ON CONFLICT (player_id, world_id) DO UPDATE SET
		   status = 'active',
		   kharis_amount = GREATEST(settled(player_world_records.kharis_amount, player_world_records.kharis_rate, player_world_records.kharis_calc_tick), EXCLUDED.kharis_amount),
		   kharis_rate = 0, kharis_calc_tick = EXCLUDED.kharis_calc_tick`,
		playerID, worldID, kharis.Floor,
	); err != nil {
		return uuid.Nil, fmt.Errorf("record Host life: %w", err)
	}

	// UnitFormed for each unit, on its own stream: these units have no settlement
	// to be an aggregate of yet. PopDrawn is 0 — the host's people were never in a
	// city's population to draw from (the cohorts ride on top of the 1 000).
	formed := append([]uuid.UUID{hostID}, spearIDs...)
	types := append([]unit.Type{unit.TypeNomadicHost},
		make([]unit.Type, spec.spearmen)...)
	sizes := append([]int{1}, make([]int, spec.spearmen)...)
	for i := 1; i <= spec.spearmen; i++ {
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
