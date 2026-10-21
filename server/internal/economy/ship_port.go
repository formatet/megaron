package economy

import (
	"context"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// FreightPort returns physical, owned food access for a bound ship. Its home
// settlement alone is not location evidence after departure. A terminal lost
// leg gives no port; a completed leg gives only its actual destination.
func FreightPort(ctx context.Context, db interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, ship uuid.UUID) (*uuid.UUID, error) {
	var port *uuid.UUID
	err := db.QueryRow(ctx, `SELECT s.id FROM units u
 LEFT JOIN LATERAL (SELECT id,status,dest_id FROM transports WHERE ship_unit_id=u.id ORDER BY created_at DESC,id DESC LIMIT 1) last ON true
 LEFT JOIN settlements s ON s.id=CASE WHEN last.id IS NULL THEN u.settlement_id WHEN last.status='delivered' THEN last.dest_id END AND s.owner_id=u.owner_id AND s.state='active'
 WHERE u.id=$1 AND u.status='freighting' AND NOT EXISTS(SELECT 1 FROM transports t WHERE t.ship_unit_id=u.id AND t.status='in_transit')`, ship).Scan(&port)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return port, err
}
