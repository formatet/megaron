package kharis

import (
	"context"
	"encoding/json"
	"fmt"
	"math"

	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type omenBand struct{ Gap, Radius, Precision int }

func omenLevel(k float64) omenBand {
	switch {
	case k < 5:
		return omenBand{12, 3, 0}
	case k < 30:
		return omenBand{8, 5, 1}
	case k < 60:
		return omenBand{5, 8, 2}
	case k < 90:
		return omenBand{3, 12, 3}
	default:
		return omenBand{2, 16, 4}
	}
}

type omenFact struct {
	Kind, ID, Good string
	Q, R, Distance int
	Net            float64
	Critical       bool
}
type omenOutcome struct {
	FactID       string `json:"fact_id"`
	Category     string `json:"category"`
	Region       string `json:"region"`
	Precision    int    `json:"precision"`
	ObservedTick int    `json:"observed_tick"`
	True         bool   `json:"true"`
	Text         string `json:"text"`
	Q            *int   `json:"q,omitempty"`
	R            *int   `json:"r,omitempty"`
}

// Observation identifiers are opaque random IDs. A hash of low-entropy map
// coordinates would let a reader brute-force an unseen deposit's location.
func omenID() string { return uuid.NewString() }

func omenDirection(q, r int) string {
	x := 2*q + r
	if math.Abs(float64(x)) > 3*math.Abs(float64(r)) {
		if x > 0 {
			return "east"
		}
		return "west"
	}
	if r < 0 {
		if x > 0 {
			return "north-east"
		}
		if x < 0 {
			return "north-west"
		}
		return "north"
	}
	if x > 0 {
		return "south-east"
	}
	if x < 0 {
		return "south-west"
	}
	return "south"
}

// A template is usable only while its observed predicate is true. Exact hexes
// are a separate, rare top-band choice and only for an urgent coastal storm.
func omenText(f omenFact, level omenBand, originQ, originR int, specific bool) (string, int, bool) {
	precision := min(level.Precision, 3)
	where := "near the temple"
	if precision >= 1 && f.Distance > 0 {
		where = "to the " + omenDirection(f.Q-originQ, f.R-originR)
	}
	if precision >= 2 {
		distance := "farther from the temple"
		if f.Distance <= 3 {
			distance = "close by"
		}
		where += " — " + distance
	}
	switch f.Kind {
	case "storm":
		if f.ID == "" {
			return "", 0, false
		}
		if level.Precision == 4 && specific && f.Critical {
			return fmt.Sprintf("Dark water in the offering: a storm now troubles the sea at (%d,%d).", f.Q, f.R), 4, true
		}
		return "Dark water in the offering: a storm moves on the sea " + where + ".", precision, true
	case "deposit":
		if f.Good == "" {
			return "", 0, false
		}
		substance := "metal-bearing ground"
		if precision >= 3 {
			substance = f.Good + "-bearing ground"
		}
		return "A bright vein in the altar stone: " + substance + " lies " + where + ".", precision, true
	case "food_surplus":
		if f.Net <= 0 {
			return "", 0, false
		}
		return "Full bowls at the altar: the fields now provide more food than the townsfolk eat.", precision, true
	case "food_deficit":
		if f.Net >= 0 {
			return "", 0, false
		}
		return "A thin offering: the townsfolk now eat more food than their fields provide.", precision, true
	}
	return "", 0, false
}

// generateOmen replaces ungrounded atmospheric sentences. Realm cooldown,
// observed fact, text, event and gossip commit atomically; retries cannot reroll.
func (h *TickHandler) generateOmen(ctx context.Context, settlementID, world uuid.UUID) error {
	tx, err := h.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var player uuid.UUID
	var standing float64
	if err = tx.QueryRow(ctx, `SELECT s.owner_id,settled(p.kharis_amount,p.kharis_rate,p.kharis_calc_tick) FROM settlements s JOIN player_world_records p ON p.player_id=s.owner_id AND p.world_id=s.world_id WHERE s.id=$1 AND s.world_id=$2 FOR UPDATE OF p`, settlementID, world).Scan(&player, &standing); err == pgx.ErrNoRows {
		return nil
	} else if err != nil {
		return err
	}
	var current int
	if err = tx.QueryRow(ctx, `SELECT current_tick FROM worlds WHERE id=$1`, world).Scan(&current); err != nil {
		return err
	}
	band := omenLevel(standing)
	var origin uuid.UUID
	var region string
	var oq, or, pop int
	var coastal bool
	err = tx.QueryRow(ctx, `SELECT s.id,s.name,p.map_q,p.map_r,s.population,COALESCE(p.coastal,false) FROM settlements s JOIN provinces p ON p.id=s.province_id JOIN buildings b ON b.settlement_id=s.id AND b.building_type='temple' WHERE s.world_id=$1 AND s.owner_id=$2 AND s.state='active' ORDER BY s.is_capital DESC,s.id LIMIT 1`, world, player).Scan(&origin, &region, &oq, &or, &pop, &coastal)
	if err == pgx.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO kharis_notice_state(world_id,player_id,scope_id,condition,state,observed_tick) VALUES($1,$2,$2,'truthful_omen_v1','empty',0) ON CONFLICT DO NOTHING`, world, player); err != nil {
		return err
	}
	var previous int
	var state string
	if err = tx.QueryRow(ctx, `SELECT observed_tick,state FROM kharis_notice_state WHERE world_id=$1 AND player_id=$2 AND scope_id=$2 AND condition='truthful_omen_v1' FOR UPDATE`, world, player).Scan(&previous, &state); err != nil {
		return err
	}
	if state != "empty" && current-previous < band.Gap {
		return nil
	}
	facts, err := loadOmenFacts(ctx, tx, world, origin, oq, or, pop, coastal, current, band.Radius)
	if err != nil {
		return err
	}
	if len(facts) == 0 {
		return nil
	}
	fact := facts[current%len(facts)]
	specific := false
	if band.Precision == 4 && fact.Critical {
		specific = h.randomFloat() < .10
	}
	text, precision, valid := omenText(fact, band, oq, or, specific)
	if !valid {
		return nil
	}
	outcome := omenOutcome{FactID: omenID(), Category: fact.Kind, Region: region, Precision: precision, ObservedTick: current, True: true, Text: text}
	if precision == 4 {
		outcome.Q = &fact.Q
		outcome.R = &fact.R
	}
	raw, err := json.Marshal(outcome)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO gossip_events(world_id,recipient_id,source_region,category,text,fact_data) VALUES($1,$2,$3,'omen',$4,$5)`, world, player, region, text, raw); err != nil {
		return err
	}
	event, err := h.store.AppendTx(ctx, tx, origin, events.StreamProvince, "OmenObservedV1", outcome, world, nil)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE kharis_notice_state SET state='observed',observed_tick=$3,context=$4 WHERE world_id=$1 AND player_id=$2 AND scope_id=$2 AND condition='truthful_omen_v1'`, world, player, current, raw); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	h.store.RecordCommitted(ctx, event)
	return nil
}
func loadOmenFacts(ctx context.Context, tx pgx.Tx, world, city uuid.UUID, q, r, pop int, coastal bool, current, radius int) ([]omenFact, error) {
	facts := []omenFact{}
	rows, err := tx.Query(ctx, `SELECT t.storm_id::text,t.q,t.r FROM sea_storm_track t JOIN sea_storms s ON s.id=t.storm_id WHERE s.world_id=$1 AND t.tick=(SELECT max(t2.tick) FROM sea_storm_track t2 WHERE t2.storm_id=t.storm_id AND t2.tick<=$2) AND greatest(abs(t.q-$3),abs(t.r-$4),abs(t.q+t.r-$3-$4))<=$5 ORDER BY t.storm_id,t.slot`, world, current, q, r, radius)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		f := omenFact{Kind: "storm"}
		if err = rows.Scan(&f.ID, &f.Q, &f.R); err != nil {
			rows.Close()
			return nil, err
		}
		f.Distance = omenDistance(q, r, f.Q, f.R)
		f.Critical = coastal && f.Distance <= 3
		facts = append(facts, f)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	rows, err = tx.Query(ctx, `SELECT q,r,CASE WHEN copper_deposit = true THEN 'copper' WHEN tin_deposit = true THEN 'tin' WHEN silver_deposit = true THEN 'silver' END FROM map_tiles WHERE world_id=$1 AND (copper_deposit = true OR tin_deposit = true OR silver_deposit = true) AND greatest(abs(q-$2),abs(r-$3),abs(q+r-$2-$3))<=$4 ORDER BY greatest(abs(q-$2),abs(r-$3),abs(q+r-$2-$3)),q,r LIMIT 8`, world, q, r, radius)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		f := omenFact{Kind: "deposit"}
		if err = rows.Scan(&f.Q, &f.R, &f.Good); err != nil {
			rows.Close()
			return nil, err
		}
		f.Distance = omenDistance(q, r, f.Q, f.R)
		facts = append(facts, f)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	var foodRate float64
	if err = tx.QueryRow(ctx, `SELECT COALESCE(sum(rate),0) FROM settlement_goods WHERE settlement_id=$1 AND good_key=ANY($2)`, city, economy.FoodGoods).Scan(&foodRate); err != nil {
		return nil, err
	}
	net := economy.FoodNet(foodRate, pop)
	if math.Abs(net) > 1e-9 {
		kind := "food_surplus"
		if net < 0 {
			kind = "food_deficit"
		}
		facts = append(facts, omenFact{Kind: kind, Q: q, R: r, Net: net})
	}
	return facts, nil
}
func omenDistance(q, r, x, y int) int { return max(absOmen(q-x), absOmen(r-y), absOmen(q+r-x-y)) }
func absOmen(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
