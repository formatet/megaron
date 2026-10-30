package kharis

import (
	"context"
	"encoding/json"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const ReligionTransitionProtectionTicks = 14

// CompleteReligionTransition runs once before workers/HTTP, atomically with its
// notice and shield. No restart or later founded realm can receive a second grant.
func CompleteReligionTransition(ctx context.Context, pool *pgxpool.Pool) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var done bool
	if err = tx.QueryRow(ctx, `SELECT completed FROM religion_v2_transition WHERE id=true FOR UPDATE`).Scan(&done); err != nil {
		return err
	}
	if done {
		return nil
	}
	rows, err := tx.Query(ctx, `SELECT p.player_id,p.world_id,w.current_tick,count(s.id)::int
 FROM player_world_records p JOIN worlds w ON w.id=p.world_id
 JOIN settlements s ON s.owner_id=p.player_id AND s.world_id=p.world_id AND s.state NOT IN ('sunk','collapsed','razed')
 WHERE w.status='active' GROUP BY p.player_id,p.world_id,w.current_tick ORDER BY p.world_id,p.player_id`)
	if err != nil {
		return err
	}
	type realm struct {
		owner, world uuid.UUID
		tick, cities int
	}
	var realms []realm
	for rows.Next() {
		var x realm
		if err = rows.Scan(&x.owner, &x.world, &x.tick, &x.cities); err != nil {
			rows.Close()
			return err
		}
		realms = append(realms, x)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	store := events.NewStore(pool)
	for _, x := range realms {
		offers, err := LoadTempleOffers(ctx, tx, x.owner, x.world)
		if err != nil {
			return err
		}
		until := x.tick + ReligionTransitionProtectionTicks
		if _, err = tx.Exec(ctx, `INSERT INTO kharis_notice_state(world_id,player_id,scope_id,condition,state,observed_tick,context)
 VALUES($1,$2,$2,'divine_punishment','protected',$3,jsonb_build_object('until_tick',$4::int))
 ON CONFLICT(world_id,player_id,scope_id,condition) DO UPDATE SET state='protected',observed_tick=EXCLUDED.observed_tick,
 context=jsonb_build_object('until_tick',GREATEST(COALESCE((kharis_notice_state.context->>'until_tick')::int,0),$4::int))`, x.world, x.owner, x.tick, until); err != nil {
			return err
		}
		body := map[string]any{"demand": RealmDemand(x.cities), "given_weight": GivenWeight(offers), "protection_ticks": ReligionTransitionProtectionTicks, "protection_until_tick": until}
		if _, err = store.AppendTx(ctx, tx, x.owner, events.StreamReligion, "ReligionRuleChangedV2", body, x.world, nil); err != nil {
			return err
		}
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO notifications(world_id,player_id,kind,level,body_json,world_tick) VALUES($1,$2,'ReligionRuleChangedV2',2,$3,$4)`, x.world, x.owner, raw, x.tick); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE religion_v2_transition SET completed=true WHERE id=true`); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
