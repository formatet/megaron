package province

import (
	"context"
	"encoding/json"
	"formatet/megaron/server/internal/movement"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"time"
)

// SweepRunnerRoutes records only reached prefixes of independent land legs.
// The existing world tick batch and precise terminal jobs call it, so logging
// out cannot skip sight. Runners receive no geographic naming or interception.
func SweepRunnerRoutes(ctx context.Context, db interface {
	Queryer
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}, world uuid.UUID, now time.Time) error {
	rows, err := db.Query(ctx, `SELECT sender_id,land_route,passage_status FROM messengers m
 WHERE world_id=$1 AND status IN ('outbound','returning') AND land_route IS NOT NULL
 AND NOT EXISTS(SELECT 1 FROM events e WHERE e.stream_id=m.id AND e.event_type='CarrierPassengerRescuedV1') AND NOT EXISTS(SELECT 1 FROM events e WHERE e.stream_id=m.id AND e.id>m.carrier_witness_id AND e.event_type IN ('CarrierPassengerLostV1','CarrierPassengerRescuedV1','CarrierPassengerLandedV1','CarrierPassengerRedirectedV1'))`, world)
	if err != nil {
		return err
	}
	type runner struct {
		owner   uuid.UUID
		raw     []byte
		passage *string
	}
	var all []runner
	for rows.Next() {
		var r runner
		if err = rows.Scan(&r.owner, &r.raw, &r.passage); err != nil {
			rows.Close()
			return err
		}
		all = append(all, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, r := range all {
		_, active, timed := RunnerLandPosition(r.raw, r.passage, now)
		if !timed || !active {
			continue
		}
		var route movement.StoredRoute
		if err = json.Unmarshal(r.raw, &route); err != nil {
			return err
		}
		var path []MapPosition
		if len(route.Hexes) == 1 {
			path = []MapPosition{{Q: route.Hexes[0][0], R: route.Hexes[0][1]}}
		} else {
			boundaries, e := movement.Boundaries(route.Move())
			if e != nil {
				return e
			}
			for i, at := range boundaries {
				if int64(at) <= route.MilliAt(now) {
					p := route.Hexes[i]
					path = append(path, MapPosition{Q: p[0], R: p[1]})
				}
			}
		}
		if _, err = SweepLiveRadiusAlong(ctx, db, world, r.owner, path, EyeLandUnit); err != nil {
			return err
		}
	}
	return nil
}
