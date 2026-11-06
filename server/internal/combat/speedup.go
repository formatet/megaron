package combat

import (
	"context"
	"encoding/json"
	"time"

	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/movement"
	"formatet/megaron/server/internal/religion"
	"formatet/megaron/server/internal/unit"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// SpeedUpMarchingUnit shortens what is left of a marching ship's voyage (bönesystemet
// v3 steg 2b), the way ExecuteRecall turns a unit: the route is rewritten, arrive_tick
// moves, and a new UnitArrivalV2 is queued; the old one is ignored (its RouteKey no
// longer matches). Idempotent through the route's own mark. A ship on an expedition,
// already sped by a departure prayer, or without a precise saved route is left alone.
func SpeedUpMarchingUnit(ctx context.Context, tx pgx.Tx, scheduler *events.Scheduler, world, unitID, blessing uuid.UUID, now time.Time) (*religion.SpedUp, error) {
	var status string
	var depart, arrive *int
	var raw []byte
	var stillness *int
	var expedition bool
	if err := tx.QueryRow(ctx, `SELECT status,depart_tick,arrive_tick,march_route,stillness_depart_tick,EXISTS(SELECT 1 FROM unit_expeditions e WHERE e.unit_id=units.id) FROM units WHERE id=$1 FOR UPDATE`, unitID).
		Scan(&status, &depart, &arrive, &raw, &stillness, &expedition); err != nil {
		return nil, err
	}
	if status != string(unit.StatusMarching) || stillness != nil || expedition {
		return nil, nil
	}
	r, ok := LoadActiveRoute(raw, status, depart, arrive)
	if !ok || r.DepartureAt.IsZero() || r.TickNanoseconds <= 0 {
		return nil, nil
	}
	sped, ok := movement.SpeedUpRemaining(r, r.MilliAt(now), religion.SpeedNum, religion.SpeedDen)
	if !ok {
		return nil, nil
	}
	sped.Sped = blessing.String()
	out, err := json.Marshal(sped)
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `UPDATE units SET arrive_tick=$2,march_route=$3,updated_at=now() WHERE id=$1`, unitID, sped.EndTick, out); err != nil {
		return nil, err
	}
	end := sped.EndTick
	if err := enqueueUnitArrivalTx(ctx, tx, scheduler, world, unit.ScheduledUnitArrivalPayload{UnitID: unitID, WorldID: world, ArriveTick: &end}, end); err != nil {
		return nil, err
	}
	return &religion.SpedUp{Kind: "unit", ID: unitID, OldArriveTick: r.EndTick, NewArriveTick: end}, nil
}
