package handlers

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/poleia/server/internal/economy"
	"github.com/poleia/server/internal/loyalty"
	"github.com/poleia/server/internal/religion"
)

// capitalParams describes the capital to raise. Population is a parameter rather
// than a constant because the two callers disagree: an ordinary join lands 5 000
// people (W1), while a Nomadic Host founds with the 4 000 civilians it carried
// (temenos_nomadic_host_plan.md). Everything downstream — the Sitos seeds, grain
// consumption, labor — derives from it.
type capitalParams struct {
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

// createdCapital identifies the rows createCapital raised.
type createdCapital struct {
	ProvinceID   uuid.UUID
	SettlementID uuid.UUID
}

// capitalError carries the message the HTTP layer should show alongside the real
// cause. Without it, extracting createCapital out of the join handler would
// collapse eight distinct response bodies into one — a behaviour change hiding
// inside a refactor.
type capitalError struct {
	userMsg string
	cause   error
}

func (e *capitalError) Error() string { return e.userMsg + ": " + e.cause.Error() }
func (e *capitalError) Unwrap() error { return e.cause }

// UserMessage is the response body the caller should write.
func (e *capitalError) UserMessage() string { return e.userMsg }

// createCapital raises a player's capital: province, settlement, opening stores,
// the Sitos seeds, starter buildings, labor weights, and the first production
// pass. It runs inside the caller's transaction and does NOT commit.
//
// It deliberately does NOT create starter units. Ordering is load-bearing:
// seedStarterUnits deducts its men from settlements.population AFTER
// RecomputeProduction has already read that population, so the opening rates are
// computed on the undrafted figure. Callers therefore keep unit seeding — and
// the founding path wants entirely different units from the join path anyway
// (it already has its spearmen; only a coastal galley is owed).
//
// Extracted from Join so the Nomadic Host's founding transaction raises its
// capital through exactly the same path rather than a parallel copy that drifts.
func createCapital(ctx context.Context, tx pgx.Tx, sitosCfg economy.SitosConfig, p capitalParams) (createdCapital, error) {
	var out createdCapital

	// Create the province tile row — copy deposit flags from map_tiles.
	err := tx.QueryRow(ctx,
		`INSERT INTO provinces (world_id, map_q, map_r, terrain_type, territory_state,
		                        copper_deposit, tin_deposit, silver_deposit, cedar_deposit, coastal)
		 VALUES ($1, $2, $3, $4, 'controlled', $5, $6, $7, $8, $9) RETURNING id`,
		p.WorldID, p.Q, p.R, p.Terrain, p.Copper, p.Tin, p.Silver, p.Cedar, p.Coastal,
	).Scan(&out.ProvinceID)
	if err != nil {
		return out, &capitalError{"could not create province", err}
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
		return out, &capitalError{"could not create settlement", err}
	}

	// Sitos genesis seed: sow the fund's starting silver (a deliberate silver-
	// invariant exception, like start-grain/pop — see temenos_sitos.md).
	if grainBaseValue, gbErr := economy.GoodBaseValue(ctx, tx, "grain"); gbErr != nil {
		slog.Error("sitos genesis: load grain base value", "err", gbErr)
	} else {
		seed, _ := economy.GenesisFundSeed(p.Population, grainBaseValue, sitosCfg)
		if _, err := tx.Exec(ctx,
			`UPDATE settlements SET sitos_fund_silver = $1 WHERE id = $2`, seed, out.SettlementID,
		); err != nil {
			slog.Error("sitos genesis seed failed", "err", err, "settlement", out.SettlementID)
		}
	}

	// Link province back to its controlling settlement.
	if _, err = tx.Exec(ctx,
		`UPDATE provinces SET controller_id = $1 WHERE id = $2`,
		out.SettlementID, out.ProvinceID,
	); err != nil {
		return out, &capitalError{"could not link province", err}
	}

	// Seed a zero row for every good so the settlement always has full inventory
	// schema regardless of terrain. RecomputeProduction (below) writes actual rates
	// from catchment tiles; zero rows here ensure non-producible goods are visible.
	if _, err = tx.Exec(ctx,
		`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
		 SELECT $1, g.key,
		        CASE g.key
		            WHEN 'grain'  THEN 300
		            WHEN 'timber' THEN 200
		            WHEN 'stone'  THEN 300
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
		out.SettlementID,
	); err != nil {
		return out, &capitalError{"could not seed goods", err}
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

	// Record in player_world_records (also sets initial kharis_rate from pantheon geography).
	if _, err = tx.Exec(ctx,
		`INSERT INTO player_world_records (player_id, world_id, settlement_id, status, kharis_rate)
		 VALUES ($1, $2, $3, 'active', $4)
		 ON CONFLICT (player_id, world_id) DO UPDATE SET
		     settlement_id = EXCLUDED.settlement_id,
		     status = 'active',
		     kharis_rate = EXCLUDED.kharis_rate`,
		p.PlayerID, p.WorldID, out.SettlementID, kharisRate,
	); err != nil {
		return out, &capitalError{"could not record join", err}
	}

	// Seed the minimal starter building set (farm/lumbermill/temple/market) so the
	// religion + silver subsystems are alive from t=0. Must precede RecomputeProduction.
	if err = seedStarterBuildings(ctx, tx, out.SettlementID); err != nil {
		return out, &capitalError{"could not seed starter buildings", err}
	}

	// Seed baseline labor weights: grain dominates (85%) so the starter city is
	// self-sufficient from t=0; cult gets a server-floor (15%) so the temple is
	// never inert and kharis doesn't decay before the first agent allocation.
	// These two goods are the only ones seeded explicitly — other goods start at
	// zero weight and are allocated by the Wanax/agent via LaborAlloc. Together
	// they satisfy both hard invariants: "grain feeds the city" and "cult always
	// produces so kharis has a floor."
	if _, err = tx.Exec(ctx,
		`INSERT INTO settlement_labor (settlement_id, good_key, weight)
		 VALUES ($1, 'grain', 0.85), ($1, 'cult', 0.15)
		 ON CONFLICT (settlement_id, good_key) DO NOTHING`,
		out.SettlementID,
	); err != nil {
		return out, &capitalError{"could not seed labor weights", err}
	}

	// RecomputeProduction reads catchment tiles and settlement_labor weights, then
	// writes rates. The equal-weight seeder (len(weights)==0 path) is bypassed since
	// we already seeded grain + cult above.
	if err = economy.RecomputeProduction(ctx, tx, out.SettlementID); err != nil {
		return out, &capitalError{"could not init production", err}
	}

	return out, nil
}
