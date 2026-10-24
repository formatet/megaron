package economy

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"formatet/megaron/server/internal/province"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// HospitalityAtTransportPort is called only by a successful physical arrival,
// in its exactly-once delivery transaction. Freight uses its own saved journey
// clock rather than combat's crew-scaled march clock.
func HospitalityAtTransportPort(ctx context.Context, tx pgx.Tx, transport, port uuid.UUID) error {
	var ship *uuid.UUID
	var world uuid.UUID
	var kind, typ string
	var size, crew, oq, or, dq, dr int
	var standing *uuid.UUID
	err := tx.QueryRow(ctx, `SELECT t.ship_unit_id,t.world_id,t.kind,t.origin_q,t.origin_r,t.dest_q,t.dest_r,t.standing_order_id,COALESCE(u.type,''),COALESCE(u.size,0),COALESCE(u.crew,0)
 FROM transports t LEFT JOIN units u ON u.id=t.ship_unit_id WHERE t.id=$1`, transport).Scan(&ship, &world, &kind, &oq, &or, &dq, &dr, &standing, &typ, &size, &crew)
	if err != nil {
		return err
	}
	if ship == nil {
		return nil
	}
	// Completed return legs are already home: topping up and then unloading
	// would only borrow the same provisions from the host a second time.
	if strings.Contains(kind, "return") {
		return nil
	}
	home := province.MapPosition{Q: oq, R: or}
	from := province.MapPosition{Q: dq, R: dr}
	if standing != nil {
		if err = tx.QueryRow(ctx, `SELECT p.map_q,p.map_r FROM standing_orders s JOIN settlements c ON c.id=s.crewed_by_settlement_id JOIN provinces p ON p.id=c.province_id WHERE s.id=$1`, *standing).Scan(&home.Q, &home.R); err != nil {
			return err
		}
	}
	if from == home {
		return nil
	}
	// A chained trade return has already been saved before the port witness.
	// Its fixed duration is authoritative, including its departure blessing.
	var raw []byte
	err = tx.QueryRow(ctx, `SELECT journey FROM transports WHERE ship_unit_id=$1 AND status='in_transit' AND origin_id=$2 AND id<>$3 ORDER BY created_at DESC,id DESC LIMIT 1`, *ship, port, transport).Scan(&raw)
	var ticks int
	if err == nil && len(raw) > 0 {
		var j province.TradeJourney
		if err = json.Unmarshal(raw, &j); err != nil {
			return err
		}
		if err = j.Validate(); err != nil {
			return err
		}
		ticks = j.TravelTicks
	} else {
		if err != nil && err != pgx.ErrNoRows {
			return err
		}
		j, e := province.PlanTradeJourney(ctx, tx, world, from, home, "naval")
		if e != nil {
			return fmt.Errorf("hospitality home journey: %w", e)
		}
		ticks = j.TravelTicks
	}
	return HospitalityFood(ctx, tx, *ship, port, VoyageRation(typ, size, "", 0, crew)*float64(ticks+1))
}
