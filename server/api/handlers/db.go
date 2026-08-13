package handlers

import (
	"context"
	"errors"
	"fmt"
	"time"

	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/kharis"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/settlement"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// loadTerrainProvince fetches terrain data for a province hex tile.
func loadTerrainProvince(ctx context.Context, pool *pgxpool.Pool, id, worldID uuid.UUID) (*province.Province, error) {
	var p province.Province
	err := pool.QueryRow(ctx,
		`SELECT id, world_id, map_q, map_r, terrain_type, territory_state, controller_id,
		        copper_deposit, tin_deposit,
		        COALESCE(silver_deposit, false), COALESCE(cedar_deposit, false),
		        COALESCE(coastal, false)
		 FROM provinces WHERE id = $1 AND world_id = $2`,
		id, worldID,
	).Scan(&p.ID, &p.WorldID, &p.MapTile.Q, &p.MapTile.R,
		&p.TerrainType, &p.TerritoryState, &p.ControllerID,
		&p.CopperDeposit, &p.TinDeposit, &p.SilverDeposit, &p.CedarDeposit, &p.Coastal)
	if err != nil {
		return nil, fmt.Errorf("scan province: %w", err)
	}
	return &p, nil
}

// loadSettlement fetches a settlement by its own UUID.
func loadSettlement(ctx context.Context, pool *pgxpool.Pool, id, worldID uuid.UUID) (*settlement.Settlement, error) {
	row := pool.QueryRow(ctx,
		`SELECT id, world_id, province_id, name, culture_id, owner_id, kingdom_id,
		        control_type, founded_from, governor_id, governor_is_ai,
		        loyalty, loyalty_trend, wall_level, is_capital, state, besieged, population,
		        COALESCE((SELECT settled(amount,rate,calc_tick) FROM settlement_goods sg WHERE sg.settlement_id=settlements.id AND sg.good_key='silver'),0),
		        COALESCE((SELECT rate FROM settlement_goods sg WHERE sg.settlement_id=settlements.id AND sg.good_key='silver'),0),
		        COALESCE((SELECT cap  FROM settlement_goods sg WHERE sg.settlement_id=settlements.id AND sg.good_key='silver'),1000),
		        now(),
		        -- Army = the units-table garrison (single source of truth since the
		        -- SB7 drop of the frozen settlements.* army columns). Same source the
		        -- combat resolver reads, so shown army == army that actually fights.
		        COALESCE((SELECT SUM(size) FROM units u WHERE u.settlement_id=settlements.id AND u.status='garrison' AND u.type='spearman'),0)::int,
		        COALESCE((SELECT SUM(size) FROM units u WHERE u.settlement_id=settlements.id AND u.status='garrison' AND u.type='war_chariot'),0)::int,
		        COALESCE((SELECT SUM(size) FROM units u WHERE u.settlement_id=settlements.id AND u.status='garrison' AND u.type='galley'),0)::int,
		        COALESCE((SELECT SUM(size) FROM units u WHERE u.settlement_id=settlements.id AND u.status='garrison' AND u.type='elite_infantry'),0)::int,
		        COALESCE((SELECT SUM(size) FROM units u WHERE u.settlement_id=settlements.id AND u.status='garrison' AND u.type='war_galley'),0)::int,
		        COALESCE((SELECT SUM(size) FROM units u WHERE u.settlement_id=settlements.id AND u.status='garrison' AND u.type='merchantman'),0)::int,
		        updated_at
		 FROM settlements WHERE id = $1 AND world_id = $2`,
		id, worldID,
	)
	return scanSettlement(row)
}

// loadSettlementByProvince fetches the settlement that occupies a given province tile.
func loadSettlementByProvince(ctx context.Context, pool *pgxpool.Pool, provinceID, worldID uuid.UUID) (*settlement.Settlement, error) {
	var id uuid.UUID
	err := pool.QueryRow(ctx,
		`SELECT id FROM settlements WHERE province_id = $1 AND world_id = $2`,
		provinceID, worldID,
	).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("no settlement")
	}
	if err != nil {
		return nil, err
	}
	return loadSettlement(ctx, pool, id, worldID)
}

// resolveSettlementID returns the settlement UUID for a given province tile.
func resolveSettlementID(ctx context.Context, pool *pgxpool.Pool, provinceID, worldID uuid.UUID) (uuid.UUID, error) {
	var id uuid.UUID
	err := pool.QueryRow(ctx,
		`SELECT id FROM settlements WHERE province_id = $1 AND world_id = $2`,
		provinceID, worldID,
	).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, fmt.Errorf("no settlement for province %s", provinceID)
	}
	return id, err
}

// scanSettlement reads a settlement from a pgx.Row.
func scanSettlement(row pgx.Row) (*settlement.Settlement, error) {
	var s settlement.Settlement
	var silverCalcAt time.Time

	err := row.Scan(
		&s.ID, &s.WorldID, &s.ProvinceID, &s.Name, &s.CultureID,
		&s.OwnerID, &s.KingdomID, &s.ControlType, &s.FoundedFrom,
		&s.GovernorID, &s.GovernorIsAI,
		&s.Loyalty, &s.LoyaltyTrend, &s.WallLevel, &s.IsCapital, &s.State, &s.Besieged, &s.Population,
		&s.Resources.Silver.Amount, &s.Resources.Silver.RatePerMinute, &s.Resources.Silver.Cap, &silverCalcAt,
		&s.Army.Spearman, &s.Army.WarChariot, &s.Army.Ship, &s.Army.EliteInfantry,
		&s.Army.WarGalley, &s.Army.Merchantman,
		&s.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scan settlement: %w", err)
	}

	s.Resources.Silver.LastCalcAt = silverCalcAt

	return &s, nil
}

// KharisState holds the live kharis pool and cult choice for a Wanax in a world.
type KharisState struct {
	Amount    float64
	Rate      float64
	Cap       float64
	CultLevel string
	// MaxTempleLevel is the level of the Wanax's grandest temple. It BINDS Cap
	// (kharis.EffectiveKharisCap), so surfaces can explain a ceiling short of 100
	// instead of leaving a Wanax wondering why devotion stopped paying.
	MaxTempleLevel int
}

// loadLaborCapacities returns, per good, the share of a settlement's population
// that can actually be employed producing it, plus the summed level of the
// buildings that produce it (levels is informational display only — capacity
// itself is driven by economy.WorkplaceSlots' (P2) and economy.LoadHexCapacity's
// (P3) absolute headcounts). Allocation beyond the capacity produces nothing
// (economy.LaborCapacity), so every surface that shows or accepts a labor
// weight needs this to avoid being silently wrong.
func loadLaborCapacities(ctx context.Context, pool *pgxpool.Pool, settlementID uuid.UUID, laborPool int) (capacities map[string]float64, levels map[string]int) {
	capacities = make(map[string]float64)
	levels = make(map[string]int)

	// The universe of goods to report a capacity for — every good with ANY
	// production_rule, not just ones this settlement's catchment currently
	// matches (a Wanax can still see "0% capacity" for a good their city
	// cannot yet work) — plus, per good, whether ANY terrain-only rule exists
	// at all. hasFieldPath only feeds economy.GoodLaborTerrainBase's fallback
	// for goods §8.3 doesn't cover yet (oil/wine/stone) — see LaborCapacity.
	allGoods := make(map[string]bool)
	fieldPath := make(map[string]bool)
	grows, _ := pool.Query(ctx,
		`SELECT good_key, bool_or(building_type IS NULL) FROM production_rules GROUP BY good_key`)
	if grows != nil {
		for grows.Next() {
			var k string
			var f bool
			if grows.Scan(&k, &f) == nil {
				allGoods[k] = true
				fieldPath[k] = f
			}
		}
		grows.Close()
	}

	lrows, _ := pool.Query(ctx,
		`SELECT good_key, SUM(level)::int FROM (
		     SELECT DISTINCT pr.good_key, b.building_type, b.level
		     FROM production_rules pr
		     JOIN buildings b ON b.settlement_id = $1 AND b.building_type = pr.building_type
		 ) t GROUP BY good_key`,
		settlementID)
	if lrows != nil {
		for lrows.Next() {
			var k string
			var lvl int
			if lrows.Scan(&k, &lvl) == nil {
				levels[k] = lvl
			}
		}
		lrows.Close()
	}

	buildingSlots, _ := economy.LoadWorkplaceSlots(ctx, pool, settlementID)
	hexSlots, _ := economy.LoadHexCapacity(ctx, pool, settlementID)

	for good := range allGoods {
		capacities[good] = economy.LaborCapacity(good, fieldPath[good], hexSlots[good], buildingSlots[good], laborPool)
	}
	return capacities, levels
}

// loadPlayerKharis reads the current kharis pool for a player in a world.
// Cap is the EFFECTIVE ceiling: the record's kharis_cap bounded by what the
// Wanax's grandest temple earns. Reporting the raw column would show 100 while
// the tick binds them to 50 — the exact confusion this ceiling must not create.
func loadPlayerKharis(ctx context.Context, pool *pgxpool.Pool, playerID, worldID uuid.UUID) (KharisState, error) {
	var k KharisState
	err := pool.QueryRow(ctx,
		`SELECT
		    GREATEST(0, settled(pwr.kharis_amount, pwr.kharis_rate, pwr.kharis_calc_tick)),
		    pwr.kharis_rate, pwr.kharis_cap, pwr.cult_level,
		    COALESCE((
		        SELECT MAX(b.level)
		        FROM settlements s
		        JOIN buildings b ON b.settlement_id = s.id AND b.building_type = 'temple'
		        WHERE s.owner_id = pwr.player_id AND s.world_id = pwr.world_id
		          AND s.state NOT IN ('sunk', 'collapsed', 'razed')
		    ), 0)
		 FROM player_world_records pwr
		 WHERE pwr.player_id = $1 AND pwr.world_id = $2`,
		playerID, worldID,
	).Scan(&k.Amount, &k.Rate, &k.Cap, &k.CultLevel, &k.MaxTempleLevel)
	if err == nil {
		k.Cap = kharis.EffectiveKharisCap(k.Cap, k.MaxTempleLevel)
	}
	return k, err
}
