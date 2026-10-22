package kharis

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/religion"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// civilianRecoveryTicks is a notice debounce, not a change to meal outcomes.
const civilianRecoveryTicks = 5

// committedNoticeDelivery broadcasts an already durable row; it never inserts.
// The optional consumer interface keeps a missing live hub from losing history.
type committedNoticeDelivery interface {
	DeliverCommittedNotification(context.Context, uuid.UUID, uuid.UUID, string, string, int, any)
}
type transitionNotice struct {
	event        *events.Event
	world, owner uuid.UUID
	id, kind     string
	level        int
	body         map[string]any
}

func (h *TickHandler) deliverTransitions(ctx context.Context, list []transitionNotice) {
	for _, n := range list {
		h.store.RecordCommitted(ctx, n.event)
		if d, ok := h.hub.(committedNoticeDelivery); ok {
			d.DeliverCommittedNotification(ctx, n.world, n.owner, n.id, n.kind, n.level, n.body)
		}
	}
}

func kharisBand(amount float64) string {
	switch {
	case amount >= religion.MoodFavorable:
		return "Favorable"
	case amount >= religion.MoodIndifferent:
		return "Indifferent"
	case amount >= religion.MoodSuspicious:
		return "Suspicious"
	default:
		return "Wrathful"
	}
}

// rememberInitialBand precedes the existing maintenance claim. If notification
// persistence fails after maintenance, retry still compares against this baseline.
func declineState(amount float64) string {
	if amount >= religion.MoodIndifferent {
		return "above"
	}
	return "below"
}
func (h *TickHandler) rememberInitialBand(ctx context.Context, w wanaxSnap, world uuid.UUID) error {
	for _, state := range []struct{ condition, value string }{{"kharis_band", kharisBand(w.kharis)}, {"kharis_decline", declineState(w.kharis)}} {
		_, err := h.pool.Exec(ctx, `INSERT INTO kharis_notice_state(world_id,player_id,scope_id,condition,state,observed_tick)
 SELECT $1,$2,$2,$3,$4,current_tick-1 FROM worlds WHERE id=$1 ON CONFLICT DO NOTHING`, world, w.playerID, state.condition, state.value)
		if err != nil {
			return err
		}
	}
	return nil
}

func (h *TickHandler) recordTransition(ctx context.Context, tx pgx.Tx, world, owner, scope uuid.UUID, condition, initial, next string, tick int, kind string, level int, body map[string]any) (*transitionNotice, error) {
	if _, err := tx.Exec(ctx, `INSERT INTO kharis_notice_state(world_id,player_id,scope_id,condition,state,observed_tick) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT DO NOTHING`, world, owner, scope, condition, initial, tick-1); err != nil {
		return nil, err
	}
	var prior string
	var observed int
	var previousContext []byte
	if err := tx.QueryRow(ctx, `SELECT state,observed_tick,context FROM kharis_notice_state WHERE world_id=$1 AND player_id=$2 AND scope_id=$3 AND condition=$4 FOR UPDATE`, world, owner, scope, condition).Scan(&prior, &observed, &previousContext); err != nil {
		return nil, err
	}
	if observed >= tick {
		return nil, nil
	}
	// Reuse this owner's durable context and observation claim. Duplicate ticks
	// cannot advance recovery; unobserved ticks cannot prove fed meals. Ownership
	// closure bypasses recovery because it makes no claim about a meal.
	if (condition == "civilian_starvation" || strings.HasPrefix(condition, "storage_cap:")) && body["reason"] != "no_longer_owned_or_active" {
		streakKey := "fed_streak"
		if strings.HasPrefix(condition, "storage_cap:") {
			streakKey = "clear_streak"
		}
		fed := 0
		if prior == "true" && next == "false" {
			var previous map[string]json.RawMessage
			if err := json.Unmarshal(previousContext, &previous); err != nil {
				return nil, fmt.Errorf("load civilian recovery context: %w", err)
			}
			if observed == tick-1 {
				if raw := previous[streakKey]; raw != nil {
					if err := json.Unmarshal(raw, &fed); err != nil {
						return nil, err
					}
				}
			}
			fed++
			if fed < civilianRecoveryTicks {
				next = "true"
			}
		}
		body[streakKey] = fed
	}
	if condition == "kharis_band" {
		body["from_band"] = prior
		body["to_band"] = next
	}
	body["world_tick"] = tick
	body["player_id"] = owner
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `UPDATE kharis_notice_state SET state=$5,observed_tick=$6,context=$7 WHERE world_id=$1 AND player_id=$2 AND scope_id=$3 AND condition=$4`, world, owner, scope, condition, next, tick, raw); err != nil {
		return nil, err
	}
	if prior == next || kind == "" {
		return nil, nil
	}
	stream := events.StreamProvince
	if condition == "kharis_band" || condition == "kharis_decline" {
		stream = events.StreamReligion
	}
	event, err := h.store.AppendTx(ctx, tx, scope, stream, kind, body, world, nil)
	if err != nil {
		return nil, err
	}
	var id string
	if err = tx.QueryRow(ctx, `INSERT INTO notifications(world_id,player_id,kind,level,body_json,world_tick) VALUES($1,$2,$3,$4,$5,$6) RETURNING id`, world, owner, kind, level, raw, tick).Scan(&id); err != nil {
		return nil, fmt.Errorf("persist transition notice: %w", err)
	}
	return &transitionNotice{event, world, owner, id, kind, level, body}, nil
}

func (h *TickHandler) observeKharisBand(ctx context.Context, w wanaxSnap, world uuid.UUID) error {
	// Use the next-tick forecast, including the local gifts that remain after today's service.
	net, _, _, _, err := ProjectDailyNet(ctx, h.pool, w.playerID, world)
	if err != nil {
		return err
	}
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var tick int
	if err = tx.QueryRow(ctx, `SELECT current_tick FROM worlds WHERE id=$1 FOR SHARE`, world).Scan(&tick); err != nil {
		return err
	}
	var amount float64
	var sid, pid uuid.UUID
	var name string
	var q, r int
	err = tx.QueryRow(ctx, `SELECT GREATEST(0,settled(pwr.kharis_amount,pwr.kharis_rate,pwr.kharis_calc_tick)),s.id,s.name,s.province_id,p.map_q,p.map_r FROM player_world_records pwr JOIN settlements s ON s.id=pwr.settlement_id AND s.owner_id=pwr.player_id JOIN provinces p ON p.id=s.province_id WHERE pwr.world_id=$1 AND pwr.player_id=$2 FOR SHARE OF pwr,s`, world, w.playerID).Scan(&amount, &sid, &name, &pid, &q, &r)
	if err != nil {
		return err
	}
	band := kharisBand(amount)
	level := 3
	if band == "Suspicious" || band == "Wrathful" {
		level = 2
	}
	body := map[string]any{"settlement_id": sid, "settlement_name": name, "name": name, "province_id": pid, "q": q, "r": r, "kharis": amount}
	n, err := h.recordTransition(ctx, tx, world, w.playerID, w.playerID, "kharis_band", kharisBand(w.kharis), band, tick, "KharisBandChanged", level, body)
	if err != nil {
		return err
	}
	kind := ""
	days := 0
	if net < 0 {
		kind = "KharisDeclining"
		days = int(math.Max(0, math.Ceil((amount-WrathThreshold)/(-net))))
	}
	warningBody := map[string]any{"settlement_id": sid, "name": name, "province_id": pid, "q": q, "r": r, "kharis": amount, "net_per_tick": net, "days_to_15": days, "wrath_threshold": WrathThreshold}
	// Record both directions, but announce only a downward crossing with negative net.
	if declineState(amount) != "below" {
		kind = ""
	}
	warning, err := h.recordTransition(ctx, tx, world, w.playerID, w.playerID, "kharis_decline", declineState(w.kharis), declineState(amount), tick, kind, 2, warningBody)
	if err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	var notices []transitionNotice
	if n != nil {
		notices = append(notices, *n)
	}
	if warning != nil {
		notices = append(notices, *warning)
	}
	h.deliverTransitions(ctx, notices)
	return nil
}

// applyStarvation owns both hunger transitions after the food pass. Garrison
// hunger follows upkeep's pre-loss witness (including a final death); civilian
// hunger follows food_unmet_amount, the same actual meal outcome growth reads.
// The conditions remain independent, even in the same settlement.
func (h *TickHandler) applyStarvation(ctx context.Context, world uuid.UUID) error {
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var tick int
	if err = tx.QueryRow(ctx, `SELECT current_tick FROM worlds WHERE id=$1 FOR SHARE`, world).Scan(&tick); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT s.id,s.owner_id,s.name,s.province_id,p.map_q,p.map_r,s.food_unmet_amount,EXISTS(SELECT 1 FROM events e WHERE e.world_id=$1 AND e.event_type='UnitAttrition' AND e.world_tick=$2 AND e.payload->>'garrison_settlement_id'=s.id::text AND e.payload->>'reason' IN ('grain_shortage','food_shortage')) FROM settlements s JOIN provinces p ON p.id=s.province_id WHERE s.world_id=$1 AND s.owner_id IS NOT NULL AND s.state NOT IN ('sunk','collapsed','razed') ORDER BY s.id FOR SHARE OF s`, world, tick)
	if err != nil {
		return err
	}
	type city struct {
		id, owner, province uuid.UUID
		name                string
		q, r                int
		hungry              bool
		unmet               float64
	}
	var cities []city
	for rows.Next() {
		var c city
		if err = rows.Scan(&c.id, &c.owner, &c.name, &c.province, &c.q, &c.r, &c.unmet, &c.hungry); err != nil {
			rows.Close()
			return err
		}
		cities = append(cities, c)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	var notices []transitionNotice
	var audit []*events.Event
	for _, c := range cities {
		next, kind, level := "false", "GarrisonStarvationEnded", 3
		if c.hungry {
			next, kind, level = "true", "GarrisonStarvationStarted", 2
		}
		body := map[string]any{"food_category": true, "settlement_id": c.id, "settlement_name": c.name, "name": c.name, "province_id": c.province, "q": c.q, "r": c.r}
		n, err := h.recordTransition(ctx, tx, world, c.owner, c.id, "garrison_starvation", "false", next, tick, kind, level, body)
		if err != nil {
			return err
		}
		if n != nil {
			notices = append(notices, *n)
		}
		civilNext, civilKind, civilLevel := "false", "CivilianStarvationEnded", 3
		if c.unmet > 0 {
			civilNext, civilKind, civilLevel = "true", "CivilianStarvationStarted", 2
		}
		civilBody := map[string]any{"food_category": true, "settlement_id": c.id, "settlement_name": c.name, "name": c.name, "province_id": c.province, "q": c.q, "r": c.r, "unmet": c.unmet}
		civil, err := h.recordTransition(ctx, tx, world, c.owner, c.id, "civilian_starvation", "false", civilNext, tick, civilKind, civilLevel, civilBody)
		if err != nil {
			return err
		}
		if civil != nil {
			notices = append(notices, *civil)
		}
		// Keep the established daily StarvationDamage outcome semantics. Transition
		// notifications are separate, so neither day-by-day hunger nor retries spam.
		if c.hungry {
			var exists bool
			if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM events WHERE world_id=$1 AND stream_id=$2 AND event_type='StarvationDamage' AND world_tick=$3)`, world, c.id, tick).Scan(&exists); err != nil {
				return err
			}
			if !exists {
				e, err := h.store.AppendTx(ctx, tx, c.id, events.StreamProvince, "StarvationDamage", map[string]any{"reason": "no_food"}, world, nil)
				if err != nil {
					return err
				}
				audit = append(audit, e)
			}
		}
	}
	// Close a former owner's/inactive city's warning using only the context
	// observed while that owner held the city. Never look up the new owner's city name,
	// location, garrison or food when telling the former owner this scope ended.
	closed, err := h.closeInactiveStarvationScopes(ctx, tx, world, tick)
	if err != nil {
		return err
	}
	notices = append(notices, closed...)
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	for _, e := range audit {
		h.store.RecordCommitted(ctx, e)
	}
	h.deliverTransitions(ctx, notices)
	return nil
}

func (h *TickHandler) closeInactiveStarvationScopes(ctx context.Context, tx pgx.Tx, world uuid.UUID, tick int) ([]transitionNotice, error) {
	rows, err := tx.Query(ctx, `SELECT ns.player_id,ns.scope_id,ns.condition,ns.context
 FROM kharis_notice_state ns WHERE ns.world_id=$1 AND ns.condition IN ('garrison_starvation','civilian_starvation') AND ns.state='true' AND ns.observed_tick<$2 AND NOT EXISTS(SELECT 1 FROM settlements s WHERE s.world_id=ns.world_id AND s.id=ns.scope_id AND s.owner_id=ns.player_id AND s.state NOT IN ('sunk','collapsed','razed')) ORDER BY ns.player_id,ns.scope_id`, world, tick)
	if err != nil {
		return nil, err
	}
	type previous struct {
		owner, scope uuid.UUID
		raw          []byte
		condition    string
	}
	var prior []previous
	for rows.Next() {
		var p previous
		if err = rows.Scan(&p.owner, &p.scope, &p.condition, &p.raw); err != nil {
			rows.Close()
			return nil, err
		}
		prior = append(prior, p)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, err
	}
	var notices []transitionNotice
	for _, p := range prior {
		var body map[string]any
		if err = json.Unmarshal(p.raw, &body); err != nil {
			return nil, fmt.Errorf("load previous garrison context: %w", err)
		}
		body["reason"] = "no_longer_owned_or_active"
		kind := "GarrisonStarvationEnded"
		if p.condition == "civilian_starvation" {
			kind = "CivilianStarvationEnded"
		}
		n, err := h.recordTransition(ctx, tx, world, p.owner, p.scope, p.condition, "false", "false", tick, kind, 3, body)
		if err != nil {
			return nil, err
		}
		if n != nil {
			notices = append(notices, *n)
		}
	}
	return notices, nil
}
