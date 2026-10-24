package combat

import (
	"context"
	"fmt"

	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/religion"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// EventHiketeiaBroken records the outcome of attacking a risen host:
// {battle_id, attacker, host_owner, kharis_lost, kharis_before, kharis_after}.
const EventHiketeiaBroken = "HiketeiaBroken"

// breakHiketeiaTx charges an attacker who has just opened or joined a battle
// against a fallen Wanax's risen host that has not founded yet (founder_phase
// active and is_refugee; the protection ends at founding). Once per battle,
// attacker and host owner, in the battle's own transaction. An order that
// never reaches a battle costs nothing.
func (h *UnitArrivalHandler) breakHiketeiaTx(ctx context.Context, tx pgx.Tx, worldID, battleID uuid.UUID, attacker battleParticipant) error {
	if attacker.side != "attacker" {
		return nil
	}
	rows, err := tx.Query(ctx,
		`SELECT DISTINCT bp.owner_id FROM battle_participants bp
		   JOIN founder_phase fp ON fp.world_id = $2 AND fp.owner_id = bp.owner_id AND fp.active AND fp.is_refugee
		  WHERE bp.battle_id = $1 AND bp.side = 'defender' AND bp.owner_id <> $3
		    AND NOT EXISTS (SELECT 1 FROM events e WHERE e.stream_id = $1 AND e.event_type = '`+EventHiketeiaBroken+`'
		                     AND e.payload->>'attacker' = $3::text AND e.payload->>'host_owner' = bp.owner_id::text)`,
		battleID, worldID, attacker.ownerID)
	if err != nil {
		return fmt.Errorf("hiketeia: find risen hosts: %w", err)
	}
	var hosts []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		hosts = append(hosts, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, host := range hosts {
		before, after, err := religion.TakeHiketeiaKharisTx(ctx, tx, worldID, attacker.ownerID)
		if err != nil {
			return fmt.Errorf("hiketeia: take kharis: %w", err)
		}
		if _, err := h.eventStore.AppendTx(ctx, tx, battleID, events.StreamCombat, EventHiketeiaBroken, map[string]any{
			"battle_id": battleID, "attacker": attacker.ownerID, "host_owner": host,
			"kharis_lost": before - after, "kharis_before": before, "kharis_after": after,
		}, worldID, nil); err != nil {
			return fmt.Errorf("hiketeia: record: %w", err)
		}
	}
	return nil
}
