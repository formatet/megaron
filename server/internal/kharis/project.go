package kharis

import (
	"context"

	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/religion"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ProjectDailyNet computes, read-only, the kharis a Wanax would net on the NEXT
// maintained day: workers at fed temples × KharisPerTempleWorker × scarcity −
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
	var templeCities, cityCount int
	err = pool.QueryRow(ctx,
		`SELECT
            0::float8, 0::float8,
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
		          AND s3.state NOT IN ('sunk', 'collapsed', 'razed')
		    ), 0)`,
		playerID, worldID,
	).Scan(&devotionSum, &devotionCapacity, &templeCities, &cityCount)
	if err != nil {
		return 0, false, 0, 0, err
	}

	temples, serr := economy.RealmTemples(ctx, pool, playerID, worldID)
	if serr != nil {
		return 0, false, 0, 0, serr
	}
	workers := map[uuid.UUID]int{}
	for _, t := range temples {
		workers[t.BuildingID] = t.Staffed
		devotionSum += float64(t.Staffed)
		devotionCapacity += float64(t.Capacity)
	}

	dailyDecay := computeDailyDecay(cityCount)
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
	fedWorkers := projectFedWorkers(ctx, pool, playerID, worldID, values, workers)

	gain := fedWorkers * KharisPerTempleWorker * scarcity
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
// projectFedWorkers counts the workers at temples whose own gift is planned
// in full — the tick's rule, read-only.
func projectFedWorkers(ctx context.Context, pool *pgxpool.Pool, playerID, worldID uuid.UUID, values map[string]float64, workers map[uuid.UUID]int) float64 {
	offers, err := loadTempleOffers(ctx, pool, playerID, worldID, values)
	if err != nil {
		return 0
	}
	var fed float64
	for _, o := range offers {
		if o.Fed {
			fed += float64(workers[o.BuildingID])
		}
	}
	return fed
}

// ProjectDecay exposes the same city burden used by maintenance and net projection.
func ProjectDecay(ctx context.Context, pool *pgxpool.Pool, playerID, worldID uuid.UUID) (cityCount int, baseDecay, cityDecay float64, err error) {
	err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM settlements WHERE owner_id=$1 AND world_id=$2 AND state NOT IN ('sunk','collapsed','razed')`, playerID, worldID).Scan(&cityCount)
	if err != nil {
		return 0, 0, 0, err
	}
	return cityCount, decayBas, computeDailyDecay(cityCount) - decayBas, nil
}
