package transport

import (
	"context"
	"encoding/json"
	"fmt"

	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ArrivalHandler credits a transport's manifest to its destination settlement when
// the caravan/ship arrives. It is the physical-mover successor to LogisticsArrivalHandler.
//
// Idempotent (CLAUDE.md "Event handlers"): claims the event in processed_deliveries,
// then re-checks the transport is still in transit under FOR UPDATE — so an
// interception (Del 3-fas-4) that flipped status to 'intercepted' cancels delivery,
// and a re-run never double-credits. Because the notify call below only ever runs
// after that claim succeeds and the crediting transaction commits, a re-run of this
// handler (worker retry, dead-letter replay) can never double-notify either — the
// second call returns at the "already processed" guard before it reaches notify.
type ArrivalHandler struct {
	pool *pgxpool.Pool
	hub  Broadcaster // nil-guarded; carries TransferDelivered to the destination's owner
}

// NewArrivalHandler creates an ArrivalHandler. hub may be nil (e.g. in tests that
// don't care about notifications).
func NewArrivalHandler(pool *pgxpool.Pool, hub Broadcaster) *ArrivalHandler {
	return &ArrivalHandler{pool: pool, hub: hub}
}

// Handle delivers the manifest to the destination, or no-ops if the transport was
// already delivered, intercepted, or lost.
func (h *ArrivalHandler) Handle(ctx context.Context, e events.ScheduledEvent) error {
	var p struct {
		TransportID uuid.UUID `json:"transport_id"`
	}
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return fmt.Errorf("unmarshal transport arrival: %w", err)
	}

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Exactly-once claim.
	ct, err := tx.Exec(ctx,
		`INSERT INTO processed_deliveries (event_id) VALUES ($1) ON CONFLICT DO NOTHING`, e.ID)
	if err != nil {
		return fmt.Errorf("claim transport arrival: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return nil // already processed
	}

	// Re-check the mover is still in transit; interception/loss cancels delivery.
	var status, kind string
	var destID *uuid.UUID
	var shipUnitID *uuid.UUID
	var ownerID uuid.UUID
	var destQ, destR int
	if err := tx.QueryRow(ctx,
		`SELECT status, dest_id, kind, ship_unit_id, owner_id, dest_q, dest_r
		 FROM transports WHERE id = $1 FOR UPDATE`, p.TransportID,
	).Scan(&status, &destID, &kind, &shipUnitID, &ownerID, &destQ, &destR); err != nil {
		return fmt.Errorf("load transport: %w", err)
	}
	if status != "in_transit" {
		return nil // intercepted, lost, or already delivered
	}

	// R3/R5 (megaron_plan_sjohandel_kraver_skepp.md): "ship_return" (the empty
	// hemresa after a single-shot naval transfer) and "damaged_return" (the
	// limped-home leg after a naval seizure, R5) are the two kinds whose
	// arrival means "this ship's journey is over — release it." Every other
	// kind leaves a bound ship exactly as bound as it was (R4: a standing sea
	// route keeps its ship for the route's whole lifetime, including between
	// legs in port).
	releaseShip := shipUnitID != nil && (kind == "ship_return" || kind == "damaged_return")
	if releaseShip {
		if err := h.releaseArrivedShip(ctx, tx, e.WorldID, *shipUnitID, ownerID, destID, destQ, destR); err != nil {
			return fmt.Errorf("release arrived ship: %w", err)
		}
	}

	if destID == nil {
		// Destination vanished (settlement removed). Nothing to credit — close it out.
		if _, err := tx.Exec(ctx,
			`UPDATE transports SET status = 'lost', updated_at = now() WHERE id = $1`, p.TransportID,
		); err != nil {
			return fmt.Errorf("mark transport lost: %w", err)
		}
		return tx.Commit(ctx)
	}

	// Credit each manifest good to the destination. cap 1_000_000 for a brand-new
	// row matches economy.goodCap (post-00d0722; never reintroduce the cap-1000 bug).
	rows, err := tx.Query(ctx,
		`SELECT good_key, quantity FROM transport_goods WHERE transport_id = $1`, p.TransportID)
	if err != nil {
		return fmt.Errorf("load manifest: %w", err)
	}
	type item struct {
		good string
		qty  float64
	}
	var manifest []item
	for rows.Next() {
		var it item
		if scanErr := rows.Scan(&it.good, &it.qty); scanErr != nil {
			rows.Close()
			return fmt.Errorf("scan manifest: %w", scanErr)
		}
		manifest = append(manifest, it)
	}
	rows.Close()

	for _, it := range manifest {
		if _, err := tx.Exec(ctx,
			`INSERT INTO settlement_goods (settlement_id, good_key, amount, rate, cap, calc_tick)
			 VALUES ($1, $2, $3, 0, 1000000, current_world_tick())
			 ON CONFLICT (settlement_id, good_key) DO UPDATE SET
			     amount = LEAST(
			         settled(settlement_goods.amount, settlement_goods.rate, settlement_goods.calc_tick) + $3,
			         settlement_goods.cap),
			     calc_tick = current_world_tick()`,
			*destID, it.good, it.qty,
		); err != nil {
			return fmt.Errorf("credit good %q: %w", it.good, err)
		}
	}

	if _, err := tx.Exec(ctx,
		`UPDATE transports SET status = 'delivered', updated_at = now() WHERE id = $1`, p.TransportID,
	); err != nil {
		return fmt.Errorf("mark transport delivered: %w", err)
	}

	// Load the destination's owner + name for the delivery notification, in the
	// same transaction as the credit (cheap join, no extra round trip after commit).
	var destOwnerID uuid.UUID
	var destName string
	if err := tx.QueryRow(ctx,
		`SELECT owner_id, name FROM settlements WHERE id = $1`, *destID,
	).Scan(&destOwnerID, &destName); err != nil {
		return fmt.Errorf("load destination owner: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	// Legibility gap (2026-07-24 sondrunda): the CLI already shows a departure ETA
	// ("arrives in X min", cmd/keryx/cmd_goods.go), but until now arrival credited
	// the goods completely silently — a Wanax checking too early sees nothing and
	// assumes the transfer vanished. Notify only on a genuine, committed delivery;
	// the intercepted/lost/dest-vanished branches above all return before this
	// point and never fire it. Fired once per transport by construction: this
	// point is only reached after the processed_deliveries claim succeeded, so a
	// worker retry of the same event stops at that claim guard, above, and never
	// re-notifies. Trade legs (economy.DeliveryHandler) already carry their own
	// TradeDelivery notification — this is TransferDelivered only, so intern
	// transfer/gift never double-notifies alongside a trade leg.
	if h.hub != nil && len(manifest) > 0 {
		goods := make([]map[string]any, 0, len(manifest))
		for _, it := range manifest {
			goods = append(goods, map[string]any{"good_key": it.good, "quantity": it.qty})
		}
		_ = h.hub.NotifyPlayer(ctx, e.WorldID, destOwnerID, "TransferDelivered", 3, map[string]any{
			"dest_id":   *destID,
			"dest_name": destName,
			"goods":     goods,
		})
	}

	return nil
}

// releaseArrivedShip is R3's frigörande, called (in the SAME tx as the
// transport's own status flip, before commit — R3's idempotency requirement)
// whenever a "ship_return" or "damaged_return" leg lands: the ship goes back
// to 'garrison' at its home port if that settlement is still an active
// settlement the owner holds, else at the owner's nearest other own port
// (R3: "finns hemstaden inte längre som egen stad"), else it is left
// `positioned` on the arrival hex and the owner is notified (R3: "ingen →
// skeppet står positioned på sista hex och en notis säger det").
func (h *ArrivalHandler) releaseArrivedShip(
	ctx context.Context, tx pgx.Tx,
	worldID, shipUnitID, ownerID uuid.UUID, destID *uuid.UUID, destQ, destR int,
) error {
	if destID != nil {
		var state string
		var curOwner uuid.UUID
		if err := tx.QueryRow(ctx,
			`SELECT state, owner_id FROM settlements WHERE id = $1`, *destID,
		).Scan(&state, &curOwner); err == nil && state == "active" && curOwner == ownerID {
			return ReleaseShip(ctx, tx, shipUnitID, *destID)
		}
	}

	if portID, _, _, found, err := NearestOwnPort(ctx, tx, worldID, ownerID, destQ, destR); err != nil {
		return err
	} else if found {
		return ReleaseShip(ctx, tx, shipUnitID, portID)
	}

	if err := StrandShip(ctx, tx, shipUnitID, destQ, destR); err != nil {
		return err
	}
	if h.hub != nil {
		_ = h.hub.NotifyPlayer(ctx, worldID, ownerID, "ShipStranded", 2, map[string]any{
			"unit_id": shipUnitID,
			"q":       destQ,
			"r":       destR,
		})
	}
	return nil
}
