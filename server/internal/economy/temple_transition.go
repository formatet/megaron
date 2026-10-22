package economy

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"math"
	"sort"
)

// CompleteTempleStaffingTransition runs before workers/HTTP at startup. Its one
// transaction settles old production, transfers real jobs, recomputes new rates
// and retires the legacy weights. Any shortage aborts the entire transition.
func CompleteTempleStaffingTransition(ctx context.Context, pool *pgxpool.Pool) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := transitionTempleStaffing(ctx, tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func transitionTempleStaffing(ctx context.Context, tx pgx.Tx) error {
	var done bool
	if err := tx.QueryRow(ctx, `SELECT completed FROM temple_staffing_transition WHERE id=true FOR UPDATE`).Scan(&done); err != nil {
		return err
	}
	if done {
		return nil
	}
	rows, err := tx.Query(ctx, `SELECT s.id,s.population,l.weight,coalesce((SELECT max(level) FROM buildings b WHERE b.settlement_id=s.id AND b.building_type='temple'),0)
 FROM settlements s JOIN settlement_labor l ON l.settlement_id=s.id AND l.good_key='cult'
 WHERE s.state NOT IN ('sunk','collapsed','razed') ORDER BY s.id`)
	if err != nil {
		return err
	}
	type city struct {
		id                uuid.UUID
		population, level int
		weight            float64
	}
	var cities []city
	for rows.Next() {
		var c city
		if err := rows.Scan(&c.id, &c.population, &c.weight, &c.level); err != nil {
			rows.Close()
			return err
		}
		cities = append(cities, c)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, c := range cities {
		if c.level < 1 || c.weight <= 0 {
			continue
		}
		if err := tx.QueryRow(ctx, `SELECT population FROM settlements WHERE id=$1 FOR UPDATE`, c.id).Scan(&c.population); err != nil {
			return err
		}
		total := c.population / 100
		target := min(TempleWorkplaces(c.level), total, max(1, int(math.Round(c.weight*float64(c.population)/100))))
		// Settle the previous jobs before their placement changes.
		if err := RecomputeProduction(ctx, tx, c.id); err != nil {
			return err
		}
		prows, err := tx.Query(ctx, `SELECT gubbe_ordinal,good_key,target_kind,coalesce(building_type,'') FROM settlement_placement WHERE settlement_id=$1`, c.id)
		if err != nil {
			return err
		}
		type job struct{ ordinal, priority int }
		var movable []job
		used := map[int]bool{}
		staffed := 0
		for prows.Next() {
			var n int
			var good, kind, building string
			if err := prows.Scan(&n, &good, &kind, &building); err != nil {
				prows.Close()
				return err
			}
			used[n] = true
			if good == GoodCult && kind == "building" && building == "temple" {
				staffed++
				continue
			}
			if good == GoodGrain || good == GoodFish || good == GoodLivestock || good == GoodCult {
				continue
			}
			priority := 3
			if good == GoodBronze || good == GoodOil || good == GoodWine {
				priority = 4
			}
			movable = append(movable, job{n, priority})
		}
		if err := prows.Err(); err != nil {
			prows.Close()
			return err
		}
		prows.Close()
		for n := 1; n <= total && staffed < target; n++ {
			if used[n] {
				continue
			}
			if _, err := tx.Exec(ctx, `INSERT INTO settlement_placement(settlement_id,gubbe_ordinal,target_kind,building_type,good_key,building_id) VALUES($1,$2,'building','temple','cult',(SELECT id FROM buildings WHERE settlement_id=$1 AND building_type='temple' ORDER BY level DESC LIMIT 1))`, c.id, n); err != nil {
				return err
			}
			staffed++
		}
		sort.Slice(movable, func(i, j int) bool {
			if movable[i].priority != movable[j].priority {
				return movable[i].priority > movable[j].priority
			}
			return movable[i].ordinal > movable[j].ordinal
		})
		for _, job := range movable {
			if staffed >= target {
				break
			}
			if _, err := tx.Exec(ctx, `UPDATE settlement_placement SET target_kind='building',building_type='temple',good_key='cult',hex_q=NULL,hex_r=NULL,building_id=(SELECT id FROM buildings WHERE settlement_id=$1 AND building_type='temple' ORDER BY level DESC LIMIT 1) WHERE settlement_id=$1 AND gubbe_ordinal=$2`, c.id, job.ordinal); err != nil {
				return err
			}
			staffed++
		}
		if staffed < target {
			return fmt.Errorf("temple staffing transition: settlement %s needs %d workers; only %d free/non-food workers available", c.id, target, staffed)
		}
		if err := RecomputeProduction(ctx, tx, c.id); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `DROP TABLE settlement_labor; UPDATE temple_staffing_transition SET completed=true WHERE id=true`); err != nil {
		return err
	}
	return nil
}
