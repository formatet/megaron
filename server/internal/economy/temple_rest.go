package economy

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// AdvanceTempleRest counts every resting temple of the world down by one for
// the given tick — but only a fully staffed temple recovers; an understaffed
// one just has its last_tick moved so the rest stands still. The last_tick
// guard makes a tick run twice count down once (idempotent). A rest that
// reaches 0 is deleted. Called from the kharis tick.
func AdvanceTempleRest(ctx context.Context, db Tx, world uuid.UUID, tickNo int) error {
	rows, err := db.Query(ctx, `SELECT tr.building_id, b.level,
 (SELECT count(*) FROM settlement_placement p WHERE p.building_id=b.id AND p.good_key='cult')
 FROM temple_rest tr JOIN buildings b ON b.id=tr.building_id JOIN settlements s ON s.id=b.settlement_id
 WHERE s.world_id=$1 AND tr.last_tick < $2`, world, tickNo)
	if err != nil {
		return fmt.Errorf("temple rest: %w", err)
	}
	type rest struct {
		id            uuid.UUID
		level, placed int
	}
	var due []rest
	for rows.Next() {
		var r rest
		if err := rows.Scan(&r.id, &r.level, &r.placed); err != nil {
			rows.Close()
			return fmt.Errorf("temple rest: %w", err)
		}
		due = append(due, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, r := range due {
		step := 0
		if c := TempleWorkplaces(r.level); c > 0 && r.placed >= c {
			step = 1
		}
		if _, err := db.Exec(ctx, `UPDATE temple_rest SET remaining_ticks=remaining_ticks-$2, last_tick=$3 WHERE building_id=$1 AND last_tick<$3`, r.id, step, tickNo); err != nil {
			return fmt.Errorf("temple rest: %w", err)
		}
	}
	if _, err := db.Exec(ctx, `DELETE FROM temple_rest WHERE remaining_ticks<=0`); err != nil {
		return fmt.Errorf("temple rest: %w", err)
	}
	return nil
}
