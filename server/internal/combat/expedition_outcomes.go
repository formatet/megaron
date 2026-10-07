package combat

import (
	"context"
	"encoding/json"
	"fmt"

	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/unit"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// CommittedNotificationDelivery delivers a personal archive row without inserting
// another one. Optional: durable notifications do not depend on a live hub.
type CommittedNotificationDelivery interface {
	DeliverCommittedNotification(context.Context, uuid.UUID, uuid.UUID, string, string, int, any)
}

type expeditionOutcome struct {
	event    *events.Event
	owner    uuid.UUID
	id, kind string
	level    int
	body     any
}

func (h *UnitArrivalHandler) expeditionOutcome(ctx context.Context, tx pgx.Tx, event *events.Event, world, owner uuid.UUID, kind string, level int, body any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	var id string
	if err := tx.QueryRow(ctx, `INSERT INTO notifications (world_id, player_id, kind, level, body_json) VALUES ($1,$2,$3,$4,$5) RETURNING id`, world, owner, kind, level, raw).Scan(&id); err != nil {
		return fmt.Errorf("persist expedition notification: %w", err)
	}
	if h.expeditionOutcomes != nil {
		*h.expeditionOutcomes = append(*h.expeditionOutcomes, expeditionOutcome{event, owner, id, kind, level, body})
	}
	return nil
}

type expeditionHomeDestination struct {
	id   uuid.UUID
	q, r int
	path []province.MapPosition
	cost float64
}

// Prefer the original home while owned and reachable, otherwise nearest reachable
// own active settlement. Lock ownership through the arrival transaction.
func expeditionReturnDestination(ctx context.Context, tx province.Queryer, u unitRow, q, r int, world uuid.UUID) (*expeditionHomeDestination, error) {
	g, err := province.LoadTileGraph(ctx, tx, world)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT s.id, p.map_q, p.map_r FROM settlements s JOIN provinces p ON p.id = s.province_id WHERE s.world_id=$1 AND s.owner_id=$2 AND s.state='active' ORDER BY CASE WHEN s.id=$3 THEN 0 ELSE 1 END, greatest(abs(p.map_q-$4),abs(p.map_r-$5),abs(p.map_q+p.map_r-$4-$5)), s.id FOR SHARE OF s`, world, u.ownerID, u.homeSettlementID, q, r)
	if err != nil {
		return nil, err
	}
	var candidates []expeditionHomeDestination
	for rows.Next() {
		var home expeditionHomeDestination
		if err := rows.Scan(&home.id, &home.q, &home.r); err != nil {
			rows.Close()
			return nil, err
		}
		candidates = append(candidates, home)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, home := range candidates {
		if u.category == "naval" {
			sq, sr, found, err := province.NearestSeaNeighbor(ctx, tx, world, home.q, home.r)
			if err != nil {
				return nil, err
			}
			if !found {
				continue
			}
			home.q, home.r = sq, sr
		}
		path, cost, ok := g.FindPath(province.MapPosition{Q: q, R: r}, province.MapPosition{Q: home.q, R: home.r}, u.category)
		if ok || (q == home.q && r == home.r) {
			home.path, home.cost = path, cost
			return &home, nil
		}
	}
	return nil, nil
}

func (h *UnitArrivalHandler) stopExpedition(ctx context.Context, tx pgx.Tx, u unitRow, q, r int, world uuid.UUID) error {
	if _, err := tx.Exec(ctx, `DELETE FROM unit_expeditions WHERE unit_id=$1`, u.id); err != nil {
		return err
	}
	// Stop exactly where the expedition arrived; neither foreign garrisons nor
	// unrelated pickup arrivals may claim a unit whose mission has ended.
	if _, err := tx.Exec(ctx, `UPDATE units SET status='positioned', q=$2, r=$3, settlement_id=NULL, home_settlement_id=NULL, target_q=NULL, target_r=NULL, departs_at=NULL, arrives_at=NULL, depart_tick=NULL, arrive_tick=NULL, march_intent=NULL, march_route=NULL, updated_at=now() WHERE id=$1`, u.id, q, r); err != nil {
		return err
	}
	if u.cargoUnitID != nil {
		if _, err := tx.Exec(ctx, `UPDATE units SET q=$2,r=$3,settlement_id=NULL,updated_at=now() WHERE id=$1 AND status='embarked'`, *u.cargoUnitID, q, r); err != nil {
			return err
		}
	}
	event, err := h.eventStore.AppendTx(ctx, tx, u.id, events.StreamType(unit.StreamUnit), unit.EventUnitArrived, unit.UnitArrivedPayload{UnitID: u.id, Q: q, R: r, NewStatus: "positioned"}, world, nil)
	if err != nil {
		return err
	}
	return h.expeditionOutcome(ctx, tx, event, world, u.ownerID, "UnitArrived", 4, map[string]any{"unit_id": u.id, "name": unit.LoadDisplayName(ctx, tx, u.id), "q": q, "r": r, "status": "positioned"})
}
