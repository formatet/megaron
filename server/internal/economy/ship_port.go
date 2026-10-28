package economy

import (
	"context"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// FreightPort returns physical, owned food access for a bound ship. Its home
// settlement alone is not location evidence after departure. A terminal lost
// leg gives no port; a completed leg gives only its actual destination. Legs
// from before the latest binding are history: the ship was released and may
// have moved as a unit since, so its dock (settlement_id) is the witness.
func FreightPort(ctx context.Context, db interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, ship uuid.UUID) (*uuid.UUID, error) {
	var port *uuid.UUID
	err := db.QueryRow(ctx, `SELECT s.id FROM units u
 LEFT JOIN LATERAL (SELECT id,status,dest_id FROM transports WHERE ship_unit_id=u.id AND (u.freight_bound_at IS NULL OR created_at>=u.freight_bound_at) ORDER BY created_at DESC,id DESC LIMIT 1) last ON true
 LEFT JOIN settlements s ON s.id=CASE WHEN last.id IS NULL THEN u.settlement_id WHEN last.status='delivered' THEN last.dest_id END AND s.owner_id=u.owner_id AND s.state='active'
 WHERE u.id=$1 AND u.status='freighting' AND NOT EXISTS(SELECT 1 FROM transports t WHERE t.ship_unit_id=u.id AND t.status='in_transit')`, ship).Scan(&port)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return port, err
}

// ShipDeparturePort resolves food access from physical location, never the
// support/upkeep anchor. Garrison settlement_id is a docking witness; freight
// uses the latest completed physical leg through the existing freight owner.
// No harbour building is required: ordinary coastal docking already permits
// departure. At-sea ships have no port and sail on food already aboard.
func ShipDeparturePort(ctx context.Context, db interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, ship uuid.UUID) (*uuid.UUID, error) {
	port, err := FreightPort(ctx, db, ship)
	if err != nil || port != nil {
		return port, err
	}
	err = db.QueryRow(ctx, `SELECT s.id FROM units u
 JOIN settlements s ON s.id=u.settlement_id AND s.world_id=u.world_id AND s.owner_id=u.owner_id AND s.state='active'
 JOIN provinces p ON p.id=s.province_id AND p.world_id=u.world_id
 WHERE u.id=$1 AND u.category='naval' AND u.status='garrison'
 AND (u.q IS NULL OR u.q=p.map_q) AND (u.r IS NULL OR u.r=p.map_r)`, ship).Scan(&port)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return port, err
}
