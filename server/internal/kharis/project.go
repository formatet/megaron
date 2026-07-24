package kharis

import (
	"context"
	"sort"

	"formatet/megaron/server/internal/religion"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ProjectDailyNet computes, read-only, the kharis a Wanax would net on the NEXT
// maintained day: devotionSum × kharisPerTempleDay × offerFraction × scarcity −
// dailyDecay — the exact formula processMaintenance applies, with the exact same
// feeding rule (traditional oil+wine OR substitution to the same divine worth).
// It exists so `status` can answer "will my kharis rise or fall" honestly: the
// bare geographic kharis_rate a Wanax sees ("passiv +0.1/dygn") excludes decay
// and temple maintenance entirely, so a fading L1 Wanax read a rising number
// (soak 2026-07-24). Nothing here mutates state — it never feeds a temple, it
// only asks whether one COULD be fed.
//
// hasTemples is false when the Wanax holds no standing temple: then the day is
// "missed", gain is 0, and the net is just −dailyDecay.
func ProjectDailyNet(ctx context.Context, pool *pgxpool.Pool, playerID, worldID uuid.UUID) (net float64, hasTemples bool, devotionSum, devotionCapacity float64, err error) {
	var templeCities, templelessColonies int
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
		    ), 0)`,
		playerID, worldID, TempleDevotionPerLevel,
	).Scan(&devotionSum, &devotionCapacity, &templeCities, &templelessColonies)
	if err != nil {
		return 0, false, 0, 0, err
	}

	dailyDecay := computeDailyDecay(templelessColonies)
	if templeCities == 0 {
		// Missed day: no gain, decay is the whole story.
		return -dailyDecay, false, devotionSum, devotionCapacity, nil
	}

	values, verr := religion.LoadDivineValues(ctx, pool, worldID)
	scarcity := scarcityFromValues(values, verr)
	fed := projectFedTemples(ctx, pool, playerID, worldID, values)
	offerFraction := computeOfferFraction(fed, templeCities)

	gain := devotionSum * kharisPerTempleDay * offerFraction * scarcity
	return gain - dailyDecay, true, devotionSum, devotionCapacity, nil
}

// scarcityFromValues mirrors templeOfferScarcity without a TickHandler receiver,
// so both the tick and this projection read the same number.
func scarcityFromValues(values map[string]float64, err error) float64 {
	if err != nil || len(values) == 0 {
		return 1.0
	}
	offering := map[string]float64{"oil": OfferOilPerTemple, "wine": OfferWinePerTemple}
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

// projectFedTemples counts how many of the Wanax's temple cities COULD be fed
// today — traditional oil+wine, or substitution to the same divine worth — WITHOUT
// deducting anything. Mirrors applyTempleOffering + feedTempleBySubstitution.
func projectFedTemples(ctx context.Context, pool *pgxpool.Pool, playerID, worldID uuid.UUID, values map[string]float64) int {
	rows, err := pool.Query(ctx,
		`SELECT s.id,
		    COALESCE((SELECT settled(sg.amount, sg.rate, sg.calc_tick)
		              FROM settlement_goods sg WHERE sg.settlement_id = s.id AND sg.good_key = 'oil'), 0) AS oil,
		    COALESCE((SELECT settled(sg.amount, sg.rate, sg.calc_tick)
		              FROM settlement_goods sg WHERE sg.settlement_id = s.id AND sg.good_key = 'wine'), 0) AS wine
		 FROM settlements s
		 WHERE s.owner_id = $1 AND s.world_id = $2 AND s.state NOT IN ('sunk', 'collapsed')
		   AND EXISTS (SELECT 1 FROM buildings b WHERE b.settlement_id = s.id AND b.building_type = 'temple')`,
		playerID, worldID,
	)
	if err != nil {
		return 0
	}
	type temple struct {
		id        uuid.UUID
		oil, wine float64
	}
	var temples []temple
	for rows.Next() {
		var t temple
		if rows.Scan(&t.id, &t.oil, &t.wine) == nil {
			temples = append(temples, t)
		}
	}
	rows.Close()

	// Traditional worth the altar is due today (values may be empty on a fresh
	// world — then substitution can't be priced, so only the oil+wine path counts).
	due := OfferOilPerTemple*values["oil"] + OfferWinePerTemple*values["wine"]

	fed := 0
	for _, t := range temples {
		if t.oil >= OfferOilPerTemple && t.wine >= OfferWinePerTemple {
			fed++
			continue
		}
		if due > 0 && canSubstituteFeed(ctx, pool, t.id, values, due) {
			fed++
		}
	}
	return fed
}

// canSubstituteFeed reports whether a city holds goods worth `due` in the gods'
// reckoning — the read-only half of feedTempleBySubstitution (no UPDATE).
func canSubstituteFeed(ctx context.Context, pool *pgxpool.Pool, settlementID uuid.UUID, values map[string]float64, due float64) bool {
	rows, err := pool.Query(ctx,
		`SELECT sg.good_key, GREATEST(0, settled(sg.amount, sg.rate, sg.calc_tick))
		 FROM settlement_goods sg
		 WHERE sg.settlement_id = $1 AND sg.good_key <> 'silver'
		   AND GREATEST(0, settled(sg.amount, sg.rate, sg.calc_tick)) > 0`,
		settlementID)
	if err != nil {
		return false
	}
	type stock struct {
		amount, value float64
	}
	var available []stock
	for rows.Next() {
		var key string
		var amount float64
		if rows.Scan(&key, &amount) == nil {
			if v := values[key]; v > 0 {
				available = append(available, stock{amount: amount, value: v})
			}
		}
	}
	rows.Close()
	// Dearest-first, exactly as the real feeder plans the gift.
	sort.Slice(available, func(i, j int) bool { return available[i].value > available[j].value })
	paid := 0.0
	for _, st := range available {
		if paid >= due {
			break
		}
		paid += st.amount * st.value
	}
	return paid >= due
}
