package handlers

import (
	"context"
	"errors"
	"fmt"
	"time"

	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/tick"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type hostPositionDB interface {
	province.Queryer
	tick.Queryer
}
type physicalHost struct {
	AtMilli     int64
	PhaseID, ID uuid.UUID
	Population  int
	Status      string
	Position    province.MapPosition
}

// loadPhysicalHost reads the same movement owner as live eyes. Founding locks
// the phase and unit before resolving the route; read surfaces do not mutate.
func loadPhysicalHost(ctx context.Context, db hostPositionDB, worldID, playerID uuid.UUID, now time.Time, lock bool) (physicalHost, error) {
	var host physicalHost
	var row province.UnitPosition
	query := `SELECT fp.id,fp.host_unit_id,fp.population,u.status,u.category,u.q,u.r,
  u.target_q,u.target_r,u.departs_at,u.arrives_at,u.depart_tick,u.arrive_tick,u.march_route
  FROM founder_phase fp JOIN units u ON u.id=fp.host_unit_id
  WHERE fp.world_id=$1 AND fp.owner_id=$2 AND fp.active
   AND u.q IS NOT NULL AND u.r IS NOT NULL AND u.status<>'disbanded'`
	if lock {
		query += ` FOR UPDATE OF fp,u`
	}
	err := db.QueryRow(ctx, query, worldID, playerID).Scan(&host.PhaseID, &host.ID, &host.Population,
		&row.Status, &row.Category, &row.Q, &row.R, &row.TargetQ, &row.TargetR, &row.DepartsAt, &row.ArrivesAt,
		&row.DepartTick, &row.ArriveTick, &row.Route)
	if err != nil {
		return host, err
	}
	anchor, err := tick.LoadAnchor(ctx, db, worldID)
	if err != nil {
		return host, fmt.Errorf("load Host clock anchor: %w", err)
	}
	host.Status = row.Status
	host.AtMilli = anchor.MilliAt(now)
	host.Position = row.Physical(ctx, db, worldID, now, anchor.MilliAt(now))
	return host, nil
}

type foundingSite struct {
	terrain                                      string
	copper, tin, silver, cedar, incense, coastal bool
}

// Shared read-only preflight: the actual founding and its status words must
// agree about the current site. No balance, vision or terrain rule is changed.
func loadFoundingSite(ctx context.Context, db hostPositionDB, worldID, playerID uuid.UUID, q, r int) (foundingSite, error) {
	var site foundingSite
	var existing int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM settlements WHERE world_id=$1 AND owner_id=$2`, worldID, playerID).Scan(&existing); err != nil {
		return site, fmt.Errorf("check existing settlements: %w", err)
	}
	if existing > 0 {
		return site, errAlreadySettled
	}
	err := db.QueryRow(ctx, `SELECT mt.terrain,mt.copper_deposit,mt.tin_deposit,
 COALESCE(mt.silver_deposit,false),COALESCE(mt.cedar_deposit,false),COALESCE(mt.incense_deposit,false),COALESCE(mt.coastal,false)
 FROM map_tiles mt LEFT JOIN provinces p ON p.world_id=mt.world_id AND p.map_q=mt.q AND p.map_r=mt.r
 WHERE mt.world_id=$1 AND mt.q=$2 AND mt.r=$3 AND p.id IS NULL
 AND mt.terrain NOT IN ('coastal_sea','deep_sea','river','river_ford','mountain_limestone','mountain_red','semi_desert')`, worldID, q, r).Scan(&site.terrain, &site.copper, &site.tin, &site.silver, &site.cedar, &site.incense, &site.coastal)
	if errors.Is(err, pgx.ErrNoRows) {
		return site, errHexNotSettlable
	}
	if err != nil {
		return site, fmt.Errorf("load founding tile: %w", err)
	}
	if conflict, err := province.SettlementCatchmentOverlap(ctx, db, worldID, q, r); err != nil {
		return site, fmt.Errorf("check catchment overlap: %w", err)
	} else if conflict != nil {
		return site, &catchmentOverlapError{q: q, r: r, conflict: conflict}
	}
	return site, nil
}

type hostMapState struct {
	AtMilli      int64  `json:"at_milli"`
	Q            int    `json:"q"`
	R            int    `json:"r"`
	Status       string `json:"status"`
	CanFoundHere bool   `json:"can_found_here"`
	Reason       string `json:"reason,omitempty"`
}

func projectHostState(ctx context.Context, db hostPositionDB, worldID, playerID uuid.UUID, host physicalHost) (hostMapState, error) {
	state := hostMapState{AtMilli: host.AtMilli, Q: host.Position.Q, R: host.Position.R, Status: "waiting"}
	_, err := loadFoundingSite(ctx, db, worldID, playerID, state.Q, state.R)
	var overlap *catchmentOverlapError
	switch {
	case err == nil:
		state.CanFoundHere = true
		state.Status = "can_found_here"
	case errors.Is(err, errAlreadySettled):
		state.Reason = "You already hold a metropolis."
	case errors.Is(err, errHexNotSettlable):
		state.Reason = "This ground cannot be settled — it is taken, or its terrain cannot support a metropolis."
	case errors.As(err, &overlap):
		state.Reason = "This ground's catchment overlaps another settlement."
	default:
		return state, err
	}
	if host.Status == "marching" {
		state.Status = "marching"
	}
	return state, nil
}

func attachHostState(ctx context.Context, db hostPositionDB, worldID, playerID uuid.UUID, now time.Time, summaries []unitSummary) error {
	host, err := loadPhysicalHost(ctx, db, worldID, playerID, now, false)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	state, err := projectHostState(ctx, db, worldID, playerID, host)
	if err != nil {
		return err
	}
	for i := range summaries {
		if summaries[i].ID == host.ID {
			summaries[i].HostState = &state
			summaries[i].CurrentQ = &state.Q
			summaries[i].CurrentR = &state.R
		}
	}
	return nil
}

type hostMovedError struct{ Position province.MapPosition }

func (e *hostMovedError) Error() string { return "Host moved from the confirmed founding site" }
