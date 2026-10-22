package economy

import (
	"context"
	"github.com/google/uuid"
)

// TempleWorkersPerLevel is physical capacity, not a population share.
const TempleWorkersPerLevel = 2

func TempleWorkplaces(level int) int { return max(0, level) * TempleWorkersPerLevel }

type TempleStaffing struct {
	SettlementID      uuid.UUID
	Staffed, Capacity int
}

// One reader for physical staffing: invalid/missing altars cannot employ workers.
// No cult production, inventory or legacy labour weights are read here.
func templeStaffing(ctx context.Context, db Tx, world, owner, settlement uuid.UUID) ([]TempleStaffing, error) {
	rows, err := db.Query(ctx, `SELECT s.id,
 coalesce((SELECT max(level) FROM buildings b WHERE b.settlement_id=s.id AND b.building_type='temple'),0),
 (SELECT count(*) FROM settlement_placement p WHERE p.settlement_id=s.id AND p.target_kind='building' AND p.building_type='temple' AND p.good_key='cult')
 FROM settlements s WHERE s.state NOT IN ('sunk','collapsed','razed')
 AND ($1::uuid='00000000-0000-0000-0000-000000000000' OR s.world_id=$1)
 AND ($2::uuid='00000000-0000-0000-0000-000000000000' OR s.owner_id=$2)
 AND ($3::uuid='00000000-0000-0000-0000-000000000000' OR s.id=$3)`, world, owner, settlement)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TempleStaffing
	for rows.Next() {
		var s TempleStaffing
		var level, placed int
		if err := rows.Scan(&s.SettlementID, &level, &placed); err != nil {
			return nil, err
		}
		s.Capacity = TempleWorkplaces(level)
		s.Staffed = min(s.Capacity, placed)
		out = append(out, s)
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
