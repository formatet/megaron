package messenger

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/movement"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/tick"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// SaveLandRouteTx freezes an independent land leg at its actual dispatch (which
// can be a future carrier landing). Rounded total and blessings are unchanged.
func SaveLandRouteTx(ctx context.Context, tx pgx.Tx, world, id uuid.UUID, from, to province.MapPosition, departure, arrival time.Time, minimumTick ...int) error {
	g, roads, err := province.LoadWorldGraph(ctx, tx, world)
	if err != nil {
		return err
	}
	path, _, ok := g.FindPath(from, to, province.CategoryCourier, roads)
	if !ok {
		return fmt.Errorf("runner land route is unreachable")
	}
	costs := g.StepHours(path, province.CategoryCourier, roads)
	hexes := make([][2]int, len(path))
	milli := make([]int64, len(costs))
	for i, p := range path {
		hexes[i] = [2]int{p.Q, p.R}
	}
	for i, c := range costs {
		milli[i] = int64(math.Round(c * 1000))
	}
	a, err := tick.LoadAnchor(ctx, tx, world)
	if err != nil {
		return err
	}
	cadence := time.Duration(tick.TickSeconds) * time.Second
	ticks := int(math.Round(float64(arrival.Sub(departure)) / float64(cadence)))
	if ticks < 0 {
		return fmt.Errorf("negative runner duration")
	}
	dispatch := a.ExactMilliAt(departure)
	if dispatch < int64(a.Tick)*1000 {
		dispatch = int64(a.Tick) * 1000
	}
	if len(minimumTick) > 0 && dispatch < int64(minimumTick[0])*1000 {
		dispatch = int64(minimumTick[0]) * 1000
	}
	c := movement.NewRouteClock(dispatch, departure, cadence, ticks)
	route := c.Route(hexes, milli)
	raw, err := json.Marshal(route)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE messengers SET land_route=$2,passage_generation=passage_generation+1 WHERE id=$1`, id, raw)
	return err
}

// SaveDepartureTx resolves correspondence endpoints once at dispatch; subsequent
// reads never search the world again for this leg. Awaiting runners stop at port.
func SaveDepartureTx(ctx context.Context, tx pgx.Tx, world, id uuid.UUID, now time.Time) error {
	var from, to province.MapPosition
	var arrival time.Time
	err := tx.QueryRow(ctx, `SELECT COALESCE(op.map_q,m.origin_q),COALESCE(op.map_r,m.origin_r),
 CASE WHEN m.passage_status='awaiting_passage' THEN pp.map_q ELSE COALESCE(m.dest_q,dp.map_q) END,
 CASE WHEN m.passage_status='awaiting_passage' THEN pp.map_r ELSE COALESCE(m.dest_r,dp.map_r) END,m.arrives_at
 FROM messengers m LEFT JOIN settlements os ON os.id=m.origin_id LEFT JOIN provinces op ON op.id=os.province_id
 LEFT JOIN settlements ds ON ds.id=m.destination_id LEFT JOIN provinces dp ON dp.id=ds.province_id
 LEFT JOIN settlements ps ON ps.id=m.passage_port_id LEFT JOIN provinces pp ON pp.id=ps.province_id WHERE m.id=$1`, id).Scan(&from.Q, &from.R, &to.Q, &to.R, &arrival)
	if err != nil {
		return err
	}
	return SaveLandRouteTx(ctx, tx, world, id, from, to, now, arrival)
}

// EnqueueLandTx uses the saved clock and current generation. V1 remains for
// pre-existing rows; precise V2 jobs already have the generation/status guard.
func EnqueueLandTx(ctx context.Context, tx pgx.Tx, s *events.Scheduler, world uuid.UUID, typ events.ScheduledEventType, payload any, due int) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	var body map[string]any
	if err = json.Unmarshal(raw, &body); err != nil {
		return err
	}
	var routeRaw []byte
	var generation int
	if err = tx.QueryRow(ctx, `SELECT land_route,passage_generation FROM messengers WHERE id=$1`, body["messenger_id"]).Scan(&routeRaw, &generation); err != nil {
		return err
	}
	if len(routeRaw) == 0 {
		return s.EnqueueTickTx(ctx, tx, world, typ, payload, due)
	}
	var route movement.StoredRoute
	if err = json.Unmarshal(routeRaw, &route); err != nil {
		return err
	}
	body["passage_generation"] = generation
	switch typ {
	case events.ScheduledOrderDelivery:
		typ = events.ScheduledOrderDeliveryV2
	case events.ScheduledMessengerArrival:
		typ = events.ScheduledMessengerArrivalV2
	case events.ScheduledMessengerReturn:
		typ = events.ScheduledMessengerReturnV2
	}
	return s.EnqueueDeadlineTx(ctx, tx, world, typ, body, route.EndTick, route.WallAt(int64(route.EndTick)*1000+route.OffsetMilli))
}
