package economy

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// PopulationGrowthRatePerTick is the fed city's full rate, before meal variety.
// Growth costs no additional food; only a positive production surplus permits it.
const PopulationGrowthRatePerTick = 0.005

// PopulationGrowthSQL is the one projection consumed by growth mutation and
// status. It correlates with a settlements row named s. SQL is built only from
// this package's authoritative constants, never request text. Preserve PostgreSQL
// float8 ROUND semantics, including the numeric actual-meal variety multiplier.
func PopulationGrowthSQL() string {
	foods := make([]string, len(FoodGoods))
	for i, good := range FoodGoods {
		foods[i] = "'" + strings.ReplaceAll(good, "'", "''") + "'"
	}
	caps := make([]string, len(populationCaps))
	for i, cap := range populationCaps {
		caps[i] = strconv.Itoa(cap)
	}
	return fmt.Sprintf(`SELECT inputs.*,
 CASE WHEN NOT can_grow OR unmet>0 THEN 0
      WHEN food_net>0 AND pop<pop_cap THEN
        LEAST(pop_cap-pop,GREATEST(1,ROUND(pop*%g::float8*variety)))
      ELSE 0 END::int AS growth_per_tick
 FROM (
 SELECT s.population AS pop,s.food_unmet_amount AS unmet,
 s.owner_id IS NOT NULL AND s.state NOT IN ('sunk','collapsed') AS can_grow,
 (SELECT COALESCE(SUM(sg.rate),0) FROM settlement_goods sg
  WHERE sg.settlement_id=s.id AND sg.good_key=ANY(ARRAY[%s]::text[]))
  +%s
  -s.population*%g::float AS food_net,
 (1.0+0.1*LEAST(4,GREATEST(0,(
   SELECT COUNT(*) FROM jsonb_each_text(s.food_consumed) AS meal(good,qty)
   WHERE (meal.good=ANY(ARRAY[%s]::text[]) OR meal.good='livestock') AND meal.qty::numeric>0
     AND meal.qty::numeric*(CASE WHEN meal.good='livestock' THEN %d ELSE 1 END) >= %g::numeric*(
       SELECT SUM(part.qty::numeric*(CASE WHEN part.good='livestock' THEN %d ELSE 1 END)) FROM jsonb_each_text(s.food_consumed) AS part(good,qty)
       WHERE (part.good=ANY(ARRAY[%s]::text[]) OR part.good='livestock') AND part.qty::numeric>0
     ) AND s.food_consumed_tick=current_world_tick()
 )-1))) AS variety,
 (ARRAY[%s]::int[])[LEAST(GREATEST(s.wall_level,0),3)+1] AS pop_cap
 ) inputs`, PopulationGrowthRatePerTick, strings.Join(foods, ","), HerdSurplusRateSQL(), GrainConsumptionPerCitizenPerTick, strings.Join(foods, ","), livestockFoodValue, VarietyMinShare, livestockFoodValue, strings.Join(foods, ","), strings.Join(caps, ","))
}

type NextCitizen struct {
	Progress      int  `json:"next_citizen_progress"`
	GrowthPerTick int  `json:"next_citizen_growth_per_tick"`
	InTicks       *int `json:"next_citizen_in_ticks"`
}

type growthReader interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

// LoadNextCitizen estimates the next hundred using this tick's gross growth.
// Later reinforcement and changing meals, production or walls can change arrival.
func LoadNextCitizen(ctx context.Context, db growthReader, id uuid.UUID) (NextCitizen, error) {
	var out NextCitizen
	var pop, cap int
	err := db.QueryRow(ctx, `SELECT pop,pop_cap,growth_per_tick FROM settlements s CROSS JOIN LATERAL (`+PopulationGrowthSQL()+`) growth WHERE s.id=$1`, id).Scan(&pop, &cap, &out.GrowthPerTick)
	if err != nil {
		return out, err
	}
	out.Progress = pop % 100
	remaining := 100 - out.Progress
	if out.GrowthPerTick > 0 && cap-pop >= remaining {
		n := (remaining + out.GrowthPerTick - 1) / out.GrowthPerTick
		out.InTicks = &n
	}
	return out, nil
}
