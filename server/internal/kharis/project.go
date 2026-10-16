package kharis

import (
	"context"

	"formatet/megaron/server/internal/religion"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ProjectDailyNet computes, read-only, the kharis a Wanax would net on the NEXT
// maintained day: devotionSum × kharisPerTempleDay × offerFraction × scarcity −
// dailyDecay — the exact formula processMaintenance applies, with the exact same
// feeding rule (early grain reference or local substitutions to the same divine worth).
// It exists so `status` can answer "will my kharis rise or fall" honestly: the
// bare geographic kharis_rate a Wanax sees ("passiv +0.1/dygn") excludes decay
// and temple maintenance entirely, so a fading L1 Wanax read a rising number
// (soak 2026-07-24). Nothing here mutates state — it never feeds a temple, it
// only asks whether one COULD be fed.
//
// hasTemples is false when the Wanax holds no standing temple: then the day is
// "missed", gain is 0, and the net is just −dailyDecay.
func ProjectDailyNet(ctx context.Context, pool *pgxpool.Pool, playerID, worldID uuid.UUID) (net float64, hasTemples bool, devotionSum, devotionCapacity float64, err error) {
	var templeCities, templelessColonies, ticksSinceFounding int
	err = pool.QueryRow(ctx,
		`SELECT
		    COALESCE((
		        SELECT SUM(LEAST(GREATEST(0, COALESCE(sl.weight, 0)), $3::float8 * GREATEST(1, b.level)))
		        FROM settlements s2
		        JOIN buildings b ON b.settlement_id = s2.id AND b.building_type = 'temple'
		        LEFT JOIN settlement_labor sl ON sl.settlement_id = s2.id AND sl.good_key = 'cult'
		        WHERE s2.owner_id = $1 AND s2.world_id = $2
		          AND s2.state NOT IN ('sunk', 'collapsed', 'razed')
		    ), 0),
		    COALESCE((
		        -- The devotion the Wanax's temples COULD employ if fully staffed —
		        -- $3 × level per temple. When this exceeds the allocated devotionSum,
		        -- a bigger temple is standing idle: the Wanax raised its level but
		        -- never allocated the cult labor to fill it, so kharis does not climb
		        -- (sondrunda 2026-07-24: built L2, net stayed −0.1, blamed wine).
		        SELECT SUM($3::float8 * GREATEST(1, b.level))
		        FROM settlements s5
		        JOIN buildings b ON b.settlement_id = s5.id AND b.building_type = 'temple'
		        WHERE s5.owner_id = $1 AND s5.world_id = $2
		          AND s5.state NOT IN ('sunk', 'collapsed', 'razed')
		    ), 0),
		    COALESCE((
		        SELECT COUNT(DISTINCT s2.id)
		        FROM settlements s2
		        JOIN buildings b ON b.settlement_id = s2.id AND b.building_type = 'temple'
		        WHERE s2.owner_id = $1 AND s2.world_id = $2
		          AND s2.state NOT IN ('sunk', 'collapsed', 'razed')
		    ), 0),
		    COALESCE((
		        SELECT COUNT(*)
		        FROM settlements s3
		        WHERE s3.owner_id = $1 AND s3.world_id = $2
		          AND s3.is_capital = false AND s3.state NOT IN ('sunk', 'collapsed')
		          AND NOT EXISTS (
		              SELECT 1 FROM buildings b
		              WHERE b.settlement_id = s3.id AND b.building_type = 'temple'
		          )
		    ), 0),
		    -- Same age the tick reads (tick.go): no record means no founding to
		    -- shield, so fall outside the grace.
		    COALESCE((
		        SELECT current_world_tick() - pwr.founded_tick
		        FROM player_world_records pwr
		        WHERE pwr.player_id = $1 AND pwr.world_id = $2
		    ), $4::int)`,
		playerID, worldID, TempleDevotionPerLevel, foundingGraceTicks,
	).Scan(&devotionSum, &devotionCapacity, &templeCities, &templelessColonies, &ticksSinceFounding)
	if err != nil {
		return 0, false, 0, 0, err
	}

	dailyDecay := decayAfterGrace(templelessColonies, ticksSinceFounding)
	if templeCities == 0 {
		// Missed day: no gain, decay is the whole story.
		return -dailyDecay, false, devotionSum, devotionCapacity, nil
	}

	values, verr := religion.LoadDivineValues(ctx, pool, worldID)
	baseValues, berr := religion.LoadLaborValues(ctx, pool)
	if berr != nil {
		return 0, false, 0, 0, berr
	}
	scarcity := scarcityFromValues(values, baseValues, verr)
	fed := projectFedTemples(ctx, pool, playerID, worldID, values)
	offerFraction := computeOfferFraction(fed, templeCities)

	gain := devotionSum * kharisPerTempleDay * offerFraction * scarcity
	return gain - dailyDecay, true, devotionSum, devotionCapacity, nil
}

// scarcityFromValues mirrors templeOfferScarcity without a TickHandler receiver,
// so both the tick and this projection read the same number.
func scarcityFromValues(values, baseValues map[string]float64, err error) float64 {
	if err != nil || len(values) == 0 {
		return 1.0
	}
	offering := templeOfferingRecipe()
	var divine, base float64
	for good, amount := range offering {
		divine += amount * values[good]
		base += amount * baseValues[good]
	}
	if base <= 0 || divine <= 0 {
		return 1.0
	}
	return clampTempleScarcity(divine / base)
}

// projectFedTemples uses the same local gift plan as maintenance and status.
func projectFedTemples(ctx context.Context, pool *pgxpool.Pool, playerID, worldID uuid.UUID, values map[string]float64) int {
	offers, err := loadTempleOffers(ctx, pool, playerID, worldID, values)
	if err != nil {
		return 0
	}
	fed := 0
	for _, o := range offers {
		if o.Fed {
			fed++
		}
	}
	return fed
}
