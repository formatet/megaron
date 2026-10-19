package combat

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type SilverWanaxTotal struct {
	OwnerID     uuid.UUID `json:"owner_id"`
	TotalSilver float64   `json:"total_silver"`
}

type SilverWanaxDistribution struct {
	Population   string  `json:"population"`
	Count        int     `json:"count"`
	TotalSilver  float64 `json:"total_silver"`
	MedianSilver float64 `json:"median_silver"`
	MaxSilver    float64 `json:"max_silver"`
	ZeroCount    int     `json:"zero_count"`
	ZeroShare    float64 `json:"zero_share"`
}

// V2 has its own baseline and units. V1 payloads are never decoded into this
// schema and no wealth distribution is broadcast as player information.
type SilverAuditV2Payload struct {
	AuditTick           int                     `json:"audit_tick"`
	PreviousAuditTick   *int                    `json:"previous_audit_tick"`
	CitySilver          float64                 `json:"city_silver"`
	BuyEscrowSilver     float64                 `json:"buy_escrow_silver"`
	SellEscrowSilver    float64                 `json:"sell_escrow_silver"`
	FounderSilver       float64                 `json:"founder_silver"`
	CarriedSilver       float64                 `json:"carried_silver"`
	TransportSilver     float64                 `json:"transport_silver"`
	DeferredTradeSilver float64                 `json:"deferred_trade_silver"`
	UnownedSilver       float64                 `json:"unowned_silver"`
	TotalSilver         float64                 `json:"total_silver"`
	MiningCreditedTotal float64                 `json:"mining_credited_total"`
	MiningEpoch         uuid.UUID               `json:"mining_epoch"`
	MinedSinceLast      float64                 `json:"mined_since_last"`
	AuditPrevTotal      float64                 `json:"audit_prev_total"`
	NetDelta            float64                 `json:"net_delta"`
	WanaxDistribution   SilverWanaxDistribution `json:"wanax_distribution"`
	WanaxTotals         []SilverWanaxTotal      `json:"wanax_totals"`
}

// emitSilverAuditV2 realizes capped mining and snapshots every bucket in one
// repeatable-read transaction. Claim, watermark and event commit together: an
// interrupted pass may retry without losing mining or duplicating the snapshot.
func (h *UpkeepHandler) emitSilverAuditV2(ctx context.Context, e events.ScheduledEvent) error {
	tx, err := h.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "silver_audit_v2:"+e.WorldID.String()); err != nil {
		return err
	}
	scope := uuid.NewSHA1(e.WorldID, []byte("silver_audit_v2"))
	claim, err := tx.Exec(ctx, `INSERT INTO processed_tick_claims(event_id,scope_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, e.ID, scope)
	if err != nil {
		return err
	}
	if claim.RowsAffected() == 0 {
		return nil
	}
	var p SilverAuditV2Payload
	if err := tx.QueryRow(ctx, `SELECT current_tick FROM worlds WHERE id=$1`, e.WorldID).Scan(&p.AuditTick); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE settlement_goods g
	 SET amount=storage_capped(g.settlement_id,g.good_key,g.amount+g.rate*GREATEST(0,$2-g.calc_tick),g.cap,'production'),calc_tick=$2
	 FROM settlements s WHERE g.settlement_id=s.id AND s.world_id=$1
	 AND s.state NOT IN ('sunk','collapsed') AND g.good_key='silver' AND g.rate>0 AND g.calc_tick<$2`, e.WorldID, p.AuditTick); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO silver_audit_epochs(world_id) VALUES($1) ON CONFLICT DO NOTHING`, e.WorldID); err != nil {
		return err
	}
	if err := tx.QueryRow(ctx, `SELECT (SELECT COALESCE(SUM(credited_silver),0) FROM silver_mining_totals WHERE world_id=$1),instrumentation_epoch FROM silver_audit_epochs WHERE world_id=$1`, e.WorldID).Scan(&p.MiningCreditedTotal, &p.MiningEpoch); err != nil {
		return err
	}
	byOwner := map[uuid.UUID]float64{}
	cohort, err := tx.Query(ctx, `SELECT player_id FROM player_world_records WHERE world_id=$1`, e.WorldID)
	if err != nil {
		return err
	}
	for cohort.Next() {
		var id uuid.UUID
		if err := cohort.Scan(&id); err != nil {
			cohort.Close()
			return err
		}
		byOwner[id] = 0
	}
	err = cohort.Err()
	cohort.Close()
	if err != nil {
		return err
	}
	// A shadow transport is the same manifest as its scheduled arrival, never
	// an additional purse. Deferred return silver exists before leg two only.
	rows, err := tx.Query(ctx, `SELECT owner_id,bucket,SUM(amount) FROM (
	 SELECT s.owner_id,'city'::text bucket,GREATEST(0,LEAST(g.cap,g.amount+g.rate*GREATEST(0,$2-g.calc_tick))) amount
	 FROM settlement_goods g JOIN settlements s ON s.id=g.settlement_id
	 WHERE s.world_id=$1 AND s.state NOT IN ('sunk','collapsed') AND g.good_key='silver'
	 UNION ALL SELECT m.sender_id,'buy_escrow',GREATEST(0,COALESCE((m.trade_offer->>'offer_silver')::float,0))
	 FROM messengers m WHERE m.world_id=$1 AND m.trade_offer->>'status'='pending' AND m.trade_offer->>'kind'='buy'
	 UNION ALL SELECT m.sender_id,'sell_escrow',GREATEST(0,COALESCE((m.trade_offer->>'offer_qty')::float,0))
	 FROM messengers m WHERE m.world_id=$1 AND m.trade_offer->>'status'='pending' AND m.trade_offer->>'kind'='sell' AND m.trade_offer->>'offer_good'='silver'
	 UNION ALL SELECT f.owner_id,'founder',GREATEST(0,f.silver_amount+f.silver_rate*GREATEST(0,$2-f.calc_tick))
	 FROM founder_phase f WHERE f.world_id=$1 AND f.active
	 UNION ALL SELECT u.owner_id,'carried',GREATEST(0,u.carried_silver)
	 FROM units u WHERE u.world_id=$1 AND u.status<>'disbanded'
	 UNION ALL SELECT t.owner_id,'transport',GREATEST(0,g.quantity)
	 FROM transports t JOIN transport_goods g ON g.transport_id=t.id
	 WHERE t.world_id=$1 AND t.status='in_transit' AND g.good_key='silver'
	 UNION ALL SELECT (se.payload->'then_return'->>'owner_id')::uuid,'deferred_trade',GREATEST(0,COALESCE((se.payload->'then_return'->>'quantity')::float,0))
	 FROM scheduled_events se JOIN transports t ON t.id::text=se.payload->>'transport_id'
	 WHERE se.world_id=$1 AND t.world_id=$1 AND se.event_type='TradeDelivery'
	 AND t.status='in_transit' AND se.payload->'then_return'->>'good_key'='silver'
	 ) assets GROUP BY owner_id,bucket`, e.WorldID, p.AuditTick)
	if err != nil {
		return err
	}
	for rows.Next() {
		var owner *uuid.UUID
		var bucket string
		var amount float64
		if err := rows.Scan(&owner, &bucket, &amount); err != nil {
			rows.Close()
			return err
		}
		switch bucket {
		case "city":
			p.CitySilver += amount
		case "buy_escrow":
			p.BuyEscrowSilver += amount
		case "sell_escrow":
			p.SellEscrowSilver += amount
		case "founder":
			p.FounderSilver += amount
		case "carried":
			p.CarriedSilver += amount
		case "transport":
			p.TransportSilver += amount
		case "deferred_trade":
			p.DeferredTradeSilver += amount
		default:
			rows.Close()
			return fmt.Errorf("unknown silver bucket %s", bucket)
		}
		p.TotalSilver += amount
		if owner == nil {
			p.UnownedSilver += amount
		} else {
			byOwner[*owner] += amount
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	p.WanaxTotals = []SilverWanaxTotal{}
	values := make([]float64, 0, len(byOwner))
	d := SilverWanaxDistribution{Population: "world records and asset owners, including AI and dispossessed Wanax"}
	for id, amount := range byOwner {
		p.WanaxTotals = append(p.WanaxTotals, SilverWanaxTotal{id, amount})
		values = append(values, amount)
		d.TotalSilver += amount
		if amount == 0 {
			d.ZeroCount++
		}
		if amount > d.MaxSilver {
			d.MaxSilver = amount
		}
	}
	sort.Slice(p.WanaxTotals, func(i, j int) bool { return p.WanaxTotals[i].OwnerID.String() < p.WanaxTotals[j].OwnerID.String() })
	sort.Float64s(values)
	d.Count = len(values)
	if d.Count > 0 {
		d.MedianSilver = values[d.Count/2]
		if d.Count%2 == 0 {
			d.MedianSilver = (values[d.Count/2-1] + values[d.Count/2]) / 2
		}
		d.ZeroShare = float64(d.ZeroCount) / float64(d.Count)
	}
	p.WanaxDistribution = d
	p.AuditPrevTotal = p.TotalSilver
	var raw []byte
	err = tx.QueryRow(ctx, `SELECT payload FROM events WHERE world_id=$1 AND event_type='SilverAuditV2' ORDER BY id DESC LIMIT 1`, e.WorldID).Scan(&raw)
	if err == nil {
		var previous SilverAuditV2Payload
		if err := json.Unmarshal(raw, &previous); err != nil {
			return err
		}
		if previous.MiningEpoch == p.MiningEpoch {
			p.PreviousAuditTick = &previous.AuditTick
			p.AuditPrevTotal = previous.TotalSilver
			p.MinedSinceLast = p.MiningCreditedTotal - previous.MiningCreditedTotal
		}
	} else if err != pgx.ErrNoRows {
		return err
	}
	p.NetDelta = p.TotalSilver - p.AuditPrevTotal
	stored, err := h.store.AppendTx(ctx, tx, e.WorldID, events.StreamWorld, "SilverAuditV2", p, e.WorldID, nil)
	if err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	h.store.RecordCommitted(ctx, stored)
	return nil
}
