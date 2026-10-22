package economy

import (
	"context"
	"github.com/google/uuid"
)

// TempleWorkersPerLevel is physical capacity, not a population share.
const TempleWorkersPerLevel = 2

func TempleWorkplaces(level int) int { return max(0, level) * TempleWorkersPerLevel }

// TempleStaffing is a city's temple service: the sum over its temples, and
// each temple on its own (ORDER 15 — one temple per god, workers stand at one).
type TempleStaffing struct {
	SettlementID      uuid.UUID
	Staffed, Capacity int
	Temples           []TempleWorkplace
}

// TempleWorkplace is one god's temple and the workers standing at it.
type TempleWorkplace struct {
	BuildingID        uuid.UUID
	SettlementID      uuid.UUID
	GodID             string
	Level             int
	Staffed, Capacity int
}

// One reader for physical staffing: invalid/missing altars cannot employ workers.
// No cult production, inventory or legacy labour weights are read here.
// Capacity from one temple never covers workers placed at another.
func templeStaffing(ctx context.Context, db Tx, world, owner, settlement uuid.UUID) ([]TempleStaffing, error) {
	rows, err := db.Query(ctx, `SELECT s.id, b.id, b.god_id, b.level,
 (SELECT count(*) FROM settlement_placement p WHERE p.settlement_id=s.id AND p.building_id=b.id AND p.good_key='cult')
 FROM settlements s LEFT JOIN buildings b ON b.settlement_id=s.id AND b.building_type='temple'
 WHERE s.state NOT IN ('sunk','collapsed','razed')
 AND ($1::uuid='00000000-0000-0000-0000-000000000000' OR s.world_id=$1)
 AND ($2::uuid='00000000-0000-0000-0000-000000000000' OR s.owner_id=$2)
 AND ($3::uuid='00000000-0000-0000-0000-000000000000' OR s.id=$3)
 ORDER BY s.id, b.god_id`, world, owner, settlement)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TempleStaffing
	for rows.Next() {
		var sid uuid.UUID
		var bid *uuid.UUID
		var god *string
		var level *int
		var placed int
		if err := rows.Scan(&sid, &bid, &god, &level, &placed); err != nil {
			return nil, err
		}
		if len(out) == 0 || out[len(out)-1].SettlementID != sid {
			out = append(out, TempleStaffing{SettlementID: sid})
		}
		if bid == nil {
			continue
		}
		t := TempleWorkplace{BuildingID: *bid, SettlementID: sid, GodID: *god, Level: *level, Capacity: TempleWorkplaces(*level)}
		t.Staffed = min(t.Capacity, placed)
		c := &out[len(out)-1]
		c.Temples = append(c.Temples, t)
		c.Staffed += t.Staffed
		c.Capacity += t.Capacity
	}
	return out, rows.Err()
}
func LoadTempleStaffing(ctx context.Context, db Tx, id uuid.UUID) (TempleStaffing, error) {
	rows, err := templeStaffing(ctx, db, uuid.Nil, uuid.Nil, id)
	if err != nil {
		return TempleStaffing{}, err
	}
	if len(rows) == 0 {
		return TempleStaffing{}, nil
	}
	return rows[0], nil
}
func RealmTempleStaffing(ctx context.Context, db Tx, owner, world uuid.UUID) (staffed, capacity int, err error) {
	rows, err := templeStaffing(ctx, db, world, owner, uuid.Nil)
	if err != nil {
		return 0, 0, err
	}
	for _, s := range rows {
		staffed += s.Staffed
		capacity += s.Capacity
	}
	return staffed, capacity, nil
}

// RealmTemples lists every standing temple of a Wanax with its own staffing.
func RealmTemples(ctx context.Context, db Tx, owner, world uuid.UUID) ([]TempleWorkplace, error) {
	rows, err := templeStaffing(ctx, db, world, owner, uuid.Nil)
	if err != nil {
		return nil, err
	}
	var out []TempleWorkplace
	for _, s := range rows {
		out = append(out, s.Temples...)
	}
	return out, nil
}
