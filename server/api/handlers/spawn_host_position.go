package handlers

import (
	"context"
	"fmt"
	"time"

	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/tick"
	"github.com/google/uuid"
)

// loadSpawnHostPositions batches stored projections, then delegates all route
// and legacy movement resolution to the same owner as Host status and eyes.
// Both spawn clearance and landmass load consume the returned physical hexes.
func loadSpawnHostPositions(ctx context.Context, db hostPositionDB, worldID uuid.UUID, now time.Time) ([]int, []int, error) {
	rows, err := db.Query(ctx, `SELECT hu.status,hu.category,hu.q,hu.r,
 hu.target_q,hu.target_r,hu.departs_at,hu.arrives_at,hu.depart_tick,hu.arrive_tick,hu.march_route
 FROM units hu JOIN founder_phase fp ON fp.host_unit_id=hu.id AND fp.active
 WHERE hu.world_id=$1 AND hu.q IS NOT NULL AND hu.r IS NOT NULL AND hu.status<>'disbanded'`, worldID)
	if err != nil {
		return nil, nil, fmt.Errorf("load spawn Hosts: %w", err)
	}
	var hosts []province.UnitPosition
	for rows.Next() {
		var row province.UnitPosition
		if err := rows.Scan(&row.Status, &row.Category, &row.Q, &row.R, &row.TargetQ, &row.TargetR, &row.DepartsAt, &row.ArrivesAt, &row.DepartTick, &row.ArriveTick, &row.Route); err != nil {
			rows.Close()
			return nil, nil, fmt.Errorf("scan spawn Host: %w", err)
		}
		hosts = append(hosts, row)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, nil, fmt.Errorf("read spawn Hosts: %w", err)
	}
	qs, rs := make([]int, 0, len(hosts)), make([]int, 0, len(hosts))
	if len(hosts) == 0 {
		return qs, rs, nil
	}
	anchor, err := tick.LoadAnchor(ctx, db, worldID)
	if err != nil {
		return nil, nil, fmt.Errorf("load spawn Host clock anchor: %w", err)
	}
	at := anchor.MilliAt(now)
	for _, host := range hosts {
		pos := host.Physical(ctx, db, worldID, now, at)
		qs = append(qs, pos.Q)
		rs = append(rs, pos.R)
	}
	return qs, rs, nil
}
