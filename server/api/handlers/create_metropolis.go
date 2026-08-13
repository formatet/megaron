package handlers

import (
	"context"
	"log/slog"

	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/hexgrid"
	"formatet/megaron/server/internal/loyalty"
	"formatet/megaron/server/internal/religion"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// metropolisParams describes the capital to raise. Population is a parameter rather
// than a constant because the two callers disagree: an ordinary join lands 5 000
// people (W1), while a Nomadic Host founds with the 4 000 civilians it carried
// (temenos_nomadic_host_plan.md). Everything downstream — the Sitos seeds, grain
// consumption, labor — derives from it.
type metropolisParams struct {
	WorldID  uuid.UUID
	PlayerID uuid.UUID

	// Tile the capital is raised on. Deposit flags are copied onto the province
	// row; mapgen owns the truth, this is the snapshot the economy reads.
	Q, R    int
	Terrain string
	Copper  bool
	Tin     bool
	Silver  bool
	Cedar   bool
	Coastal bool

	Name       string
	Culture    string
	Population int
}

// createdMetropolis identifies the rows createMetropolis raised.
type createdMetropolis struct {
	ProvinceID   uuid.UUID
	SettlementID uuid.UUID
}

// metropolisError carries the message the HTTP layer should show alongside the real
// cause. Without it, extracting createMetropolis out of the join handler would
// collapse eight distinct response bodies into one — a behaviour change hiding
// inside a refactor.
type metropolisError struct {
	userMsg string
	cause   error
}

func (e *metropolisError) Error() string { return e.userMsg + ": " + e.cause.Error() }
func (e *metropolisError) Unwrap() error { return e.cause }

// UserMessage is the response body the caller should write.
func (e *metropolisError) UserMessage() string { return e.userMsg }

// createMetropolis raises a player's capital: province, settlement, opening stores,
// the Sitos seeds, Demeter's conditional starter farm, labor weights, and the first
// production pass. It runs inside the caller's transaction and does NOT commit.
//
// It deliberately does NOT create starter units. Ordering is load-bearing:
// starter-unit seeding deducts its men from settlements.population AFTER
// RecomputeProduction has already read that population, so the opening rates are
// computed on the undrafted figure. Callers therefore keep unit seeding — and
// the founding path wants entirely different units from the join path anyway
// (it already has its spearmen; only a coastal galley is owed).
//
// Extracted from Join so the Nomadic Host's founding transaction raises its
// capital through exactly the same path rather than a parallel copy that drifts.
func createMetropolis(ctx context.Context, tx pgx.Tx, sitosCfg economy.SitosConfig, p metropolisParams) (createdMetropolis, error) {
	var out createdMetropolis

	// Create the province tile row — copy deposit flags from map_tiles.
	err := tx.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type, territory_state,
		                        copper_deposit, tin_deposit, silver_deposit, cedar_deposit, coastal)
		 VALUES ($1, $2, $3, $4, 'controlled', $5, $6, $7, $8, $9) RETURNING id`,
		p.WorldID, p.Q, p.R, p.Terrain, p.Copper, p.Tin, p.Silver, p.Cedar, p.Coastal,
	).Scan(&out.ProvinceID)
	if err != nil {
		return out, &metropolisError{"could not create province", err}
	}

	// Create the settlement (capital).
	// Silver now lives in settlement_goods (seeded below via GenesisSilverLiquid).
	err = tx.QueryRow(ctx,
		`INSERT INTO settlements
		 (world_id, province_id, name, culture_id, owner_id, control_type, is_capital, loyalty, loyalty_points, population)
		 VALUES ($1,$2,$3,$4,$5,'capital',true,3,$6,$7)
		 RETURNING id`,
		p.WorldID, out.ProvinceID, p.Name, p.Culture, p.PlayerID, loyalty.LoyaltyStartCapital, p.Population,
	).Scan(&out.SettlementID)
	if err != nil {
		return out, &metropolisError{"could not create settlement", err}
	}

	// No Sitos seed. The fund's genesis silver is gone with migration 106 — the
	// granary holds food, not silver, and it starts empty: a city has no reserve
	// before its first surplus. The capital's own starting silver still comes
	// from GenesisSilverLiquid below, which is one of the two sanctioned faucets
	// (B3: starting silver and mines).

	// Link province back to its controlling settlement.
	if _, err = tx.Exec(ctx,
		`UPDATE provinces SET controller_id = $1 WHERE id = $2`,
		out.SettlementID, out.ProvinceID,
	); err != nil {
		return out, &metropolisError{"could not link province", err}
	}

	// Seed a zero row for every good so the settlement always has full inventory
	// schema regardless of terrain. RecomputeProduction (below) writes actual rates
	// from catchment tiles; zero rows here ensure non-producible goods are visible.
	if _, err = tx.Exec(ctx,
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
		 SELECT $1, g.key,
		        CASE g.key
		            WHEN 'grain'     THEN 300
		            WHEN 'timber'    THEN 200
		            WHEN 'stone'     THEN 300
		            WHEN 'livestock' THEN $2::int
		            ELSE 0
		        END,
		        0,
		        1000000, -- non-binding storage ceiling (mirrors economy.goodCap);
		                 -- the old per-good caps predated the 2026-07-05 cap
		                 -- loosening and pinned never-produced/never-crafted goods
		                 -- at a low binding value (silver's real cap is set by the
		                 -- Sitos liquid-silver seed below)
		        current_world_tick()
		 FROM goods g
		 ON CONFLICT (settlement_id, good_key) DO NOTHING`,
		out.SettlementID, economy.FoundingHerdLivestock,
	); err != nil {
		return out, &metropolisError{"could not seed goods", err}
	}

	// Sitos genesis seed: sow LIQUID silver (goods.silver), separate from the
	// fund seed above — a settlement with 0 liquid silver can't pay for buy
	// offers or army upkeep even with a full fund (temenos_sitos.md). Same
	// silver-invariant exception class as the fund seed. Runs before
	// RecomputeProduction below so a same-tick recompute settles from this
	// amount rather than the bulk-insert's placeholder 0.
	if grainBaseValue, gbErr := economy.GoodBaseValue(ctx, tx, "grain"); gbErr != nil {
		slog.Error("sitos genesis: load grain base value for liquid silver", "err", gbErr)
	} else {
		liquidSeed, liquidCap := economy.GenesisSilverLiquid(p.Population, grainBaseValue, sitosCfg)
		if _, err := tx.Exec(ctx,
			`UPDATE settlement_goods SET amount = $1, cap = $2, calc_tick = current_world_tick()
			 WHERE settlement_id = $3 AND good_key = 'silver'`,
			liquidSeed, liquidCap, out.SettlementID,
		); err != nil {
			slog.Error("sitos genesis: seed liquid silver failed", "err", err, "settlement", out.SettlementID)
		}
	}

	// Compute starting kharis_rate from local pantheon power.
	regions := religion.DefaultPantheonRegions()
	var maxPower float64
	for _, reg := range regions {
		if pw := religion.LocalPower(reg, p.Q, p.R); pw > maxPower {
			maxPower = pw
		}
	}
	kharisRate := maxPower * 0.05

	// Record in player_world_records (also sets initial kharis_rate from pantheon
	// geography). kharis_amount + kharis_calc_tick MUST be seeded here: join.go
	// already created this row with the column defaults (amount 0, calc_tick 0),
	// so this Exec always hits the ON CONFLICT branch on founding. Without seeding
	// calc_tick, settled(0, kharis_rate>0, 0) accrues the rate across the WHOLE world
	// history on the first read — a founder on a tick-87k world saw kharis ~350
	// until the first kharis tick clamped it (soak 2026-07-23, both probes). Seed the
	// amount at the temple-less ceiling (kharis.TempleKharisCeiling(0) = 25) — the
	// favour a Wanax with no temple yet can hold — and stamp calc_tick to now so the
	// meter starts where it is, not inflated. Strawman start — temenos_balans_spakar.md §9.
	const starterKharis = 25.0
	if _, err = tx.Exec(ctx,
		`INSERT INTO player_world_records (player_id, world_id, settlement_id, status, kharis_rate, kharis_amount, kharis_calc_tick)
		 VALUES ($1, $2, $3, 'active', $4, $5, current_world_tick())
		 ON CONFLICT (player_id, world_id) DO UPDATE SET
		     settlement_id = EXCLUDED.settlement_id,
		     status = 'active',
		     kharis_rate = EXCLUDED.kharis_rate,
		     kharis_amount = EXCLUDED.kharis_amount,
		     kharis_calc_tick = current_world_tick()`,
		p.PlayerID, p.WorldID, out.SettlementID, kharisRate, starterKharis,
	); err != nil {
		return out, &metropolisError{"could not record join", err}
	}

	// Demeter's gift: a metropolis founds BUILDING-FREE (like a colony) — the Wanax
	// raises farm/lumbermill/temple/market themselves. The single exception is a
	// starter FARM, granted only where the land can grow wheat. Test: if assuming a
	// farm would raise the catchment's grain potential above its building-free base,
	// at least one catchment hex carries farm-compatible terrain (plains/
	// river_valley/river_delta), so Demeter grants the farm. On barren ground she
	// grants nothing — and the founding grain forecast still reads true there, since
	// its with-farm assumption equals the building-free base when no farm helps.
	// Must precede RecomputeProduction so the farm's grain is picked up.
	// (Poseidon's galley — the coastal gift — is granted by the caller.)
	catchmentHexes := hexgrid.Ring(hexgrid.Coord{Q: p.Q, R: p.R}, hexgrid.CatchmentRadius)
	grainNoFarm, err := economy.CatchmentBasePotentialAt(ctx, tx, p.WorldID, catchmentHexes, nil)
	if err != nil {
		return out, &metropolisError{"could not read catchment potential", err}
	}
	grainWithFarm, err := economy.CatchmentBasePotentialAt(ctx, tx, p.WorldID, catchmentHexes, []string{"farm"})
	if err != nil {
		return out, &metropolisError{"could not read catchment potential with farm", err}
	}
	if grainWithFarm["grain"] > grainNoFarm["grain"]+1e-9 {
		if _, err = tx.Exec(ctx,
			`INSERT INTO buildings (settlement_id, building_type, level) VALUES ($1, 'farm', 1)
			 ON CONFLICT (settlement_id, building_type) DO NOTHING`,
			out.SettlementID,
		); err != nil {
			return out, &metropolisError{"could not grant Demeter's farm", err}
		}
	}

	// P4 (megaron_plan_fysisk_gubbemodell.md): auto-place the starting gubbar on
	// the best available food hexes, greedily, stopping once the city is
	// self-sufficient — not placing every gubbe on food by default. Whatever
	// gubbar aren't needed for food stay unplaced, ready for the Wanax to
	// assign via the (not-yet-built) stadsvy. Founds on barren ground without
	// Demeter's farm still yields nothing — self-sufficiency is
	// geography-gated, by design; PlaceStartingWorkforce's sufficient=false
	// return is not separately surfaced here because the grain_ticks warning
	// below (read from the real post-RecomputeProduction rate) already covers
	// exactly this case.
	if _, _, err = economy.PlaceStartingWorkforce(ctx, tx, out.SettlementID); err != nil {
		return out, &metropolisError{"could not place starting workforce", err}
	}

	// RecomputeProduction reads catchment tiles and settlement_placement rows,
	// then writes rates.
	if err = economy.RecomputeProduction(ctx, tx, out.SettlementID); err != nil {
		return out, &metropolisError{"could not init production", err}
	}

	return out, nil
}
