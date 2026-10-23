package economy

import (
	"context"
	"fmt"
	"formatet/megaron/server/internal/tick"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

// GoodShipbuilding is a non-inventory placement role (parked, zero weight).
const GoodShipbuilding = "shipbuilding"

// StartShipyardWork saves paid work, then derives its rate from physical staffing.
// Sync's calc=NULL forces an initial rate/queue derivation even at this tick.
func StartShipyardWork(ctx context.Context, db Tx, id uuid.UUID, work int) error {
	if _, err := db.Exec(ctx, `UPDATE units SET shipyard_work_remaining=$2,shipyard_work_calc_tick=NULL,shipyard_work_running=false WHERE id=$1`, id, work); err != nil {
		return err
	}
	_, err := db.Exec(ctx, `SELECT sync_shipyard_work($1)`, id)
	return err
}

// CompleteShipyardWorkTransition atomically captures old timers BEFORE workers
// start. No free workers, repayment or restart of the full recipe is introduced.
func CompleteShipyardWorkTransition(ctx context.Context, pool *pgxpool.Pool) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var done bool
	if err = tx.QueryRow(ctx, `SELECT completed FROM shipyard_work_transition WHERE id=true FOR UPDATE`).Scan(&done); err != nil {
		return err
	}
	if done {
		return tx.Commit(ctx)
	}
	rows, err := tx.Query(ctx, `SELECT u.id,u.world_id,u.build_complete_at,
 (SELECT min(e.due_tick) FROM scheduled_events e WHERE e.payload->>'unit_id'=u.id::text AND e.event_type IN ('TrainComplete','ShipRepairComplete') AND e.processed_at IS NULL AND e.failed_at IS NULL)
 FROM units u WHERE u.category='naval' AND u.status IN ('forming','repairing') AND u.shipyard_work_remaining IS NULL ORDER BY u.id FOR UPDATE OF u`)
	if err != nil {
		return err
	}
	type job struct {
		id, world uuid.UUID
		at        *time.Time
		due       *int
	}
	var jobs []job
	for rows.Next() {
		var j job
		if err = rows.Scan(&j.id, &j.world, &j.at, &j.due); err != nil {
			rows.Close()
			return err
		}
		jobs = append(jobs, j)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	for _, j := range jobs {
		anchor, err := tick.LoadAnchor(ctx, tx, j.world)
		if err != nil {
			return err
		}
		due := anchor.Tick
		if j.due != nil {
			due = *j.due
		} else if j.at != nil {
			due = anchor.TickAtOrAfter(*j.at)
		} else {
			return fmt.Errorf("shipyard transition: active job %s has no saved deadline", j.id)
		}
		// An already-due healthy legacy timer has earned all its work under
		// the old rule. Let its unchanged handler finish it once; never turn
		// completed work into a new staffing obligation at deployment.
		if j.due != nil && due <= anchor.Tick {
			continue
		}
		if err := StartShipyardWork(ctx, tx, j.id, max(0, due-anchor.Tick)); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE scheduled_events SET processed_at=now() WHERE payload->>'unit_id'=$1 AND event_type IN ('TrainComplete','ShipRepairComplete') AND processed_at IS NULL`, j.id.String()); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE shipyard_work_transition SET completed=true WHERE id=true`); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ShipyardWork is a read projection; timing remains owned by tick.Anchor.
type ShipyardWork struct {
	Remaining  *int
	Waiting    bool
	CompleteAt *time.Time
}

func LoadShipyardWork(ctx context.Context, db Tx, id uuid.UUID) (ShipyardWork, error) {
	var out ShipyardWork
	var world uuid.UUID
	var due *int
	err := db.QueryRow(ctx, `SELECT world_id,CASE WHEN shipyard_work_remaining IS NULL THEN NULL ELSE shipyard_work_left(shipyard_work_remaining,shipyard_work_running,shipyard_work_calc_tick,(SELECT current_tick FROM worlds WHERE id=units.world_id)) END,
 shipyard_work_remaining IS NOT NULL AND NOT shipyard_work_running,
 CASE WHEN shipyard_work_running THEN shipyard_work_calc_tick+shipyard_work_remaining ELSE NULL END FROM units WHERE id=$1`, id).Scan(&world, &out.Remaining, &out.Waiting, &due)
	if err != nil {
		return out, err
	}
	if due != nil {
		anchor, err := tick.LoadAnchor(ctx, db, world)
		if err != nil {
			return out, err
		}
		at := anchor.WallAt(int64(*due) * 1000)
		out.CompleteAt = &at
	}
	return out, nil
}
