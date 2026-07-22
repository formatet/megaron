package combat

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/google/uuid"
	"formatet/megaron/server/internal/events"
)

// Silver flow bookkeeping (temenos_silver_cirkulation_plan.md, Del A). Two new
// event types — never a re-interpretation of an existing one (event-versioning
// rule) — turn the world's silver from an unmeasurable stock into an auditable
// flow: where it is, where it came from, where it went. Del B/C tune behaviour;
// this part only measures, so a baseline exists before anything changes.

// upkeepAgg accumulates one settlement's silver/grain outflow across the upkeep
// loop, so the audit event is one row per paying settlement per day (not per
// unit — that would be an event flood).
type upkeepAgg struct {
	unitsPaid        int
	unitsUnpaid      int
	grainTotal       float64
	silverGross      float64 // silver actually debited for upkeep
	silverCirculated float64 // credited back to a garrison town (Del C; 0 until then)
	silverDestroyed  float64 // left the world entirely
	silverUnpaid     float64 // gate-stopped: never debited, never left the world
}

// UpkeepSettledPayload is the per-settlement daily upkeep outcome. For paid units
// silver_gross = silver_circulated + silver_destroyed; silver_unpaid is the
// separate bucket for upkeep the affordability gate stopped (kept out of
// destroyed so the flow analysis can tell real destruction from a gate-stop —
// review condition, Perplexity). circulated_to maps recipient settlement → amount
// for the credit leg; empty until Del C, present in the schema from the first
// version so the later payer-variant (metropolis-as-payer) needs no schema change.
type UpkeepSettledPayload struct {
	SettlementID     uuid.UUID             `json:"settlement_id"`
	UnitsPaid        int                   `json:"units_paid"`
	UnitsUnpaid      int                   `json:"units_unpaid"`
	GrainTotal       float64               `json:"grain_total"`
	SilverGross      float64               `json:"silver_gross"`
	SilverCirculated float64               `json:"silver_circulated"`
	SilverDestroyed  float64               `json:"silver_destroyed"`
	SilverUnpaid     float64               `json:"silver_unpaid"`
	CirculatedTo     map[uuid.UUID]float64 `json:"circulated_to"`
}

// SilverAuditPayload is the world's daily silver stock-take. Stocks are point-
// in-time; mined_since_last is the only inflow term. No destruction figure is
// stored: the residual is derived offline as `unattributed_delta` =
// audit_prev_total + mined_since_last − current_total (genesis seeds fold into
// it). It is deliberately NOT called "destruction" — it mixes rites, builds,
// recruitment and genesis until those are separately instrumented, so the name
// must not claim more than it knows (review condition, Perplexity). The largest
// sink (upkeep) is itemised via UpkeepSettled; the rest stays unattributed here.
type SilverAuditPayload struct {
	AuditTick      int     `json:"audit_tick"`
	LiquidTotal    float64 `json:"liquid_total"`
	FundTotal      float64 `json:"fund_total"`
	EscrowTotal    float64 `json:"escrow_total"`
	MinedSinceLast float64 `json:"mined_since_last"`
	AuditPrevTotal float64 `json:"audit_prev_total"`
	NetDelta       float64 `json:"net_delta"`
}

func (p SilverAuditPayload) total() float64 { return p.LiquidTotal + p.FundTotal + p.EscrowTotal }

// emitUpkeepSettled appends one UpkeepSettled event per settlement that paid (or
// failed to pay) upkeep this tick. Best-effort audit — a failed append never
// aborts the upkeep pass (same posture as the UnitAttrition/UnitDeserted audit
// appends above).
func (h *UpkeepHandler) emitUpkeepSettled(ctx context.Context, worldID uuid.UUID, aggs map[uuid.UUID]*upkeepAgg) {
	for sid, a := range aggs {
		// circulated_to: recipient → amount. In the MVP payer=recipient, so a single
		// entry {sid: circulated} when anything circulated; empty ({}) otherwise. The
		// map (not a scalar) is what carries the later metropolis-as-payer variant.
		circulatedTo := map[uuid.UUID]float64{}
		if a.silverCirculated > 0 {
			circulatedTo[sid] = a.silverCirculated
		}
		_, _ = h.store.Append(ctx, sid, events.StreamProvince, "UpkeepSettled",
			UpkeepSettledPayload{
				SettlementID:     sid,
				UnitsPaid:        a.unitsPaid,
				UnitsUnpaid:      a.unitsUnpaid,
				GrainTotal:       a.grainTotal,
				SilverGross:      a.silverGross,
				SilverCirculated: a.silverCirculated,
				SilverDestroyed:  a.silverDestroyed,
				SilverUnpaid:     a.silverUnpaid,
				CirculatedTo:     circulatedTo,
			},
			worldID, nil)
	}
}

// emitSilverAudit takes the world's silver stock-take once per daily upkeep tick
// and appends a SilverAudit event. It reads the previous audit (if any) to fill
// audit_prev_total and to size mined_since_last against real elapsed ticks, so
// catch-up batching can never distort the mined term.
func (h *UpkeepHandler) emitSilverAudit(ctx context.Context, worldID uuid.UUID) {
	var currentTick int
	if err := h.pool.QueryRow(ctx,
		`SELECT current_tick FROM worlds WHERE id = $1`, worldID,
	).Scan(&currentTick); err != nil {
		slog.Error("silver audit: read world tick failed", "world", worldID, "err", err)
		return
	}

	// Active-settlement filter mirrors the Sitos tick's, so the audit and the
	// stabilizer agree on which settlements are "in the world".
	var liquid, fund, mineRate float64
	if err := h.pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(GREATEST(0, settled(sg.amount, sg.rate, sg.calc_tick))), 0)
		 FROM settlement_goods sg
		 JOIN settlements s ON s.id = sg.settlement_id
		 WHERE s.world_id = $1 AND s.owner_id IS NOT NULL
		   AND s.state NOT IN ('sunk', 'collapsed') AND sg.good_key = 'silver'`,
		worldID,
	).Scan(&liquid); err != nil {
		slog.Error("silver audit: liquid sum failed", "world", worldID, "err", err)
		return
	}
	if err := h.pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(GREATEST(0, sitos_fund_silver)), 0)
		 FROM settlements
		 WHERE world_id = $1 AND owner_id IS NOT NULL AND state NOT IN ('sunk', 'collapsed')`,
		worldID,
	).Scan(&fund); err != nil {
		slog.Error("silver audit: fund sum failed", "world", worldID, "err", err)
		return
	}
	// mined_since_last = Σ(rate) over silver rows that mine (rate>0) × elapsed ticks.
	if err := h.pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(sg.rate), 0)
		 FROM settlement_goods sg
		 JOIN settlements s ON s.id = sg.settlement_id
		 WHERE s.world_id = $1 AND s.owner_id IS NOT NULL
		   AND s.state NOT IN ('sunk', 'collapsed')
		   AND sg.good_key = 'silver' AND sg.rate > 0`,
		worldID,
	).Scan(&mineRate); err != nil {
		slog.Error("silver audit: mine rate sum failed", "world", worldID, "err", err)
		return
	}

	// Escrow = silver locked in still-pending BUY offers. SELL offers escrow goods,
	// not silver; accepted/expired offers have already settled (see messenger.go /
	// trade.go). offer_silver lives in the messenger's trade_offer JSON.
	var escrow float64
	if err := h.pool.QueryRow(ctx,
		`SELECT COALESCE(SUM((trade_offer->>'offer_silver')::float), 0)
		 FROM messengers
		 WHERE world_id = $1 AND trade_offer->>'status' = 'pending'
		   AND trade_offer->>'kind' = 'buy'`,
		worldID,
	).Scan(&escrow); err != nil {
		slog.Error("silver audit: escrow sum failed", "world", worldID, "err", err)
		return
	}

	// Previous audit anchors the delta + the mined window. Absent on the first run.
	prevTotal := liquid + fund + escrow // first audit ⇒ delta 0
	var mined float64
	var raw []byte
	if err := h.pool.QueryRow(ctx,
		`SELECT payload FROM events
		 WHERE world_id = $1 AND event_type = 'SilverAudit'
		 ORDER BY id DESC LIMIT 1`,
		worldID,
	).Scan(&raw); err == nil {
		var prev SilverAuditPayload
		if json.Unmarshal(raw, &prev) == nil {
			prevTotal = prev.total()
			elapsed := currentTick - prev.AuditTick
			if elapsed > 0 {
				mined = mineRate * float64(elapsed)
			}
		}
	}

	current := liquid + fund + escrow
	_, _ = h.store.Append(ctx, worldID, events.StreamWorld, "SilverAudit",
		SilverAuditPayload{
			AuditTick:      currentTick,
			LiquidTotal:    liquid,
			FundTotal:      fund,
			EscrowTotal:    escrow,
			MinedSinceLast: mined,
			AuditPrevTotal: prevTotal,
			NetDelta:       current - prevTotal,
		},
		worldID, nil)
}
