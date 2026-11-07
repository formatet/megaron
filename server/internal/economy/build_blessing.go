package economy

import (
	"context"
	"sort"

	"github.com/google/uuid"
)

// BuildSpedUp is the outcome recorded in RiteCast.effect.sped_up for one build the
// Builder's Blessing shortened: Kind "build" (the city's build queue) or "ship" (a shipyard job).
type BuildSpedUp struct {
	Kind       string    `json:"kind"`
	ID         uuid.UUID `json:"id"`
	OldDueTick int       `json:"old_due_tick"`
	NewDueTick int       `json:"new_due_tick"`
}

type buildJob struct {
	kind string
	id   uuid.UUID
	due  int
	sped bool
}

// buildJobs lists the running construction of the cities: the active build of each queue and
// every ship still being built ('forming') with a shipyard job. due is 0 for a job with no rate yet.
func buildJobs(ctx context.Context, db Tx, cities []uuid.UUID) (map[uuid.UUID]buildJob, error) {
	out := map[uuid.UUID]buildJob{}
	rows, err := db.Query(ctx, `SELECT 'build', q.id, CASE WHEN q.work_rate>0 THEN q.work_calc_tick+(q.work_remaining+q.work_rate-1)/q.work_rate ELSE 0 END, q.sped
 FROM build_queue q WHERE q.settlement_id=ANY($1) AND q.work_calc_tick IS NOT NULL
 UNION ALL
 SELECT 'ship', u.id, CASE WHEN u.shipyard_work_rate>0 THEN u.shipyard_work_calc_tick+(u.shipyard_work_remaining+u.shipyard_work_rate-1)/u.shipyard_work_rate ELSE 0 END, u.shipyard_sped
 FROM units u WHERE u.settlement_id=ANY($1) AND u.category='naval' AND u.status='forming' AND u.shipyard_work_remaining IS NOT NULL`, cities)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var j buildJob
		if err := rows.Scan(&j.kind, &j.id, &j.due, &j.sped); err != nil {
			return nil, err
		}
		out[j.id] = j
	}
	return out, rows.Err()
}

// BlessBuilds applies a Builder's Blessing that has just been laid (the prayer_blessings row is
// already in this TX) to what the cities are building now: each running build and each ship
// being built is cut ONCE to three quarters of its remaining time. The cut itself is done by the
// SQL that owns the queue's clock (sync_build_queue / sync_shipyard_work, mig 251); the finish
// event it replaces is retired by the revision those functions bump. Returns what was shortened.
func BlessBuilds(ctx context.Context, db Tx, cities []uuid.UUID) ([]BuildSpedUp, error) {
	before, err := buildJobs(ctx, db, cities)
	if err != nil {
		return nil, err
	}
	for _, c := range cities {
		if _, err := db.Exec(ctx, `SELECT sync_build_queue($1)`, c); err != nil {
			return nil, err
		}
	}
	for id, j := range before {
		if j.kind == "ship" && !j.sped {
			if _, err := db.Exec(ctx, `SELECT sync_shipyard_work($1)`, id); err != nil {
				return nil, err
			}
		}
	}
	after, err := buildJobs(ctx, db, cities)
	if err != nil {
		return nil, err
	}
	out := []BuildSpedUp{}
	for id, a := range after {
		if b, ok := before[id]; ok && a.sped && !b.sped {
			out = append(out, BuildSpedUp{Kind: a.kind, ID: id, OldDueTick: b.due, NewDueTick: a.due})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID.String() < out[j].ID.String() })
	return out, nil
}
