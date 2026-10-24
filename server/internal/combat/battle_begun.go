package combat

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type battleBegunOutcomes struct {
	events  []*events.Event
	notices []expeditionOutcome
}

// The battle row is locked by initiateOrJoinBattle. Its archive row is the
// durable per-battle/per-Wanax receipt, even after it is read or muted. All
// entry points persist inside the battle transaction and deliver after commit.
func (h *UnitArrivalHandler) persistBattleBegun(ctx context.Context, tx pgx.Tx, world, battle uuid.UUID, q, r int, joined bool) error {
	rows, err := tx.Query(ctx, `SELECT DISTINCT COALESCE(NULLIF(p.wanax_name,''),'an enemy'), p.id FROM battle_participants bp JOIN players p ON p.id=bp.owner_id WHERE bp.battle_id=$1 AND bp.side='attacker' ORDER BY p.id`, battle)
	if err != nil {
		return err
	}
	var names []string
	for rows.Next() {
		var name string
		var id uuid.UUID
		if err = rows.Scan(&name, &id); err != nil {
			rows.Close()
			return err
		}
		names = append(names, name)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	if len(names) == 0 {
		return nil
	}
	rows, err = tx.Query(ctx, `WITH defenders AS (
 SELECT owner_id FROM battle_participants WHERE battle_id=$1 AND side='defender'
 UNION SELECT s.owner_id FROM settlements s JOIN provinces p ON p.id=s.province_id WHERE s.world_id=$2 AND s.state='active' AND s.owner_id IS NOT NULL AND p.map_q=$3 AND p.map_r=$4
 ) SELECT owner_id FROM defenders d WHERE NOT EXISTS (SELECT 1 FROM battle_participants bp WHERE bp.battle_id=$1 AND bp.owner_id=d.owner_id AND bp.side='attacker') AND NOT EXISTS (SELECT 1 FROM notifications n WHERE n.world_id=$2 AND n.player_id=d.owner_id AND n.kind='BattleBegun' AND n.body_json->>'battle_id'=$1::text) ORDER BY owner_id`, battle, world, q, r)
	if err != nil {
		return err
	}
	var owners []uuid.UUID
	for rows.Next() {
		var owner uuid.UUID
		if err = rows.Scan(&owner); err != nil {
			rows.Close()
			return err
		}
		owners = append(owners, owner)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	for _, owner := range owners {
		body := map[string]any{"battle_id": battle, "role": "defender", "opponent_name": strings.Join(names, ", "), "q": q, "r": r, "occurred_at": h.clk.Now().UTC(), "joined": joined}
		if name, ok := settlementNameAt(ctx, tx, world, q, r); ok {
			body["place"] = name
		}
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		var id string
		if err = tx.QueryRow(ctx, `INSERT INTO notifications(world_id,player_id,kind,level,body_json) VALUES($1,$2,'BattleBegun',2,$3) RETURNING id`, world, owner, raw).Scan(&id); err != nil {
			return fmt.Errorf("persist battle begun: %w", err)
		}
		if h.battleOutcomes != nil {
			h.battleOutcomes.notices = append(h.battleOutcomes.notices, expeditionOutcome{owner: owner, id: id, kind: "BattleBegun", level: 2, body: body})
		}
	}
	return nil
}

func (h *UnitArrivalHandler) deliverBattleBegun(ctx context.Context, world uuid.UUID) {
	if h.battleOutcomes == nil {
		return
	}
	for _, event := range h.battleOutcomes.events {
		h.eventStore.RecordCommitted(ctx, event)
	}
	if delivery, ok := h.hub.(CommittedNotificationDelivery); ok {
		for _, n := range h.battleOutcomes.notices {
			delivery.DeliverCommittedNotification(ctx, world, n.owner, n.id, n.kind, n.level, n.body)
		}
	}
}
