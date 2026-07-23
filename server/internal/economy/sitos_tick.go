package economy

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SitosTransactionPayload is the outcome recorded for one Sitos action (Fas 2.3:
// events store outcomes, not intentions). GoodDelta is signed from the
// settlement's perspective (negative on a buy = grain left the city; positive on
// a sell = grain arrived). SilverDelta is signed from the fund's perspective
// (negative = fund paid out, positive = fund took in). "noop" is a documented
// value but is never actually emitted (no transaction ⇒ no event — see the plan).
type SitosTransactionPayload struct {
	Good        string  `json:"good"`
	Kind        string  `json:"kind"` // "buy" | "sell" | "tax" | "noop"
	SilverDelta float64 `json:"silver_delta"`
	GoodDelta   float64 `json:"grain_delta"`
	RefPrice    float64 `json:"ref_price"`
	FundSilver  float64 `json:"fund_silver"`
}

// SitosFundReleasePayload is the outcome of releasing a fund's over-cap overhang
// into the settlement's liquid silver (silver-plan Del B). silver_moved > 0
// always (no event on noop); fund_after is the fund balance after the move and is
// always ≥ cap. A distinct event type — NOT a new "kind" on SitosTransaction,
// whose semantics are frozen (event-versioning rule).
//
// Diegetic meaning (recorded here, NOT surfaced in the MVP): the fund is winding
// down reserve it holds beyond what price stabilization needs — "the grain-watcher
// repays the city" — so a future keryx/web feedback line has a legible story to
// tell, not an unexplained silver payout.
type SitosFundReleasePayload struct {
	SettlementID uuid.UUID `json:"settlement_id"`
	SilverMoved  float64   `json:"silver_moved"`
	FundAfter    float64   `json:"fund_after"`
	Cap          float64   `json:"cap"`
}

// SitosTickHandler is the self-rescheduling per-world stabilization pass. It runs
// every tick (cadence +1, not daily), taxing a slice of each settlement's silver
// into its fund and then buying surplus / selling shortage for subsistence goods
// at a smoothed reference price — always with silver strictly conserved.
//
// TODO: idempotent — like the kharis/colony daily ticks this handler applies a
// tax + stabilization each run and reschedules on its last line; a worker retry
// between commit and markDone would re-tax. This matches the accepted precedent
// for the other self-rescheduling ticks in this codebase (see CLAUDE.md Fas 2.2);
// tightening all of them to strict idempotency is tracked separately.
type SitosTickHandler struct {
	pool      *pgxpool.Pool
	scheduler *events.Scheduler
	store     *events.Store
	hub       Broadcaster
	cfg       SitosConfig
}

// NewSitosTickHandler creates a SitosTickHandler.
func NewSitosTickHandler(pool *pgxpool.Pool, sched *events.Scheduler, store *events.Store, hub Broadcaster, cfg SitosConfig) *SitosTickHandler {
	return &SitosTickHandler{pool: pool, scheduler: sched, store: store, hub: hub, cfg: cfg}
}

// Handle processes one ScheduledSitosTick event for a world.
func (h *SitosTickHandler) Handle(ctx context.Context, e events.ScheduledEvent) error {
	// Grain's base_value anchors every capacity figure. On error fall back to the
	// seed-data default (3.0) so a transient hiccup degrades gracefully.
	grainBaseValue := 3.0
	if v, err := GoodBaseValue(ctx, h.pool, GoodGrain); err == nil {
		grainBaseValue = v
	} else {
		slog.Warn("sitos tick: grain base value lookup failed, using default", "err", err)
	}

	rows, err := h.pool.Query(ctx,
		`SELECT id FROM settlements
		 WHERE world_id = $1 AND owner_id IS NOT NULL AND state NOT IN ('sunk', 'collapsed')`,
		e.WorldID,
	)
	if err != nil {
		return fmt.Errorf("sitos tick: query settlements: %w", err)
	}
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, id := range ids {
		if err := h.tickSettlement(ctx, id, e.WorldID, grainBaseValue); err != nil {
			slog.Error("sitos tick: settlement failed", "settlement", id, "err", err)
		}
	}

	// Reschedule next tick (cadence +1). Last line, matching the kharis/colony
	// precedent: a reschedule failure is the only thing that retries the pass.
	return h.scheduler.EnqueueTickRecurring(ctx, e.WorldID, events.ScheduledSitosTick,
		struct{}{}, e.DueTick, 1)
}

// tickSettlement runs the tax + stabilization for one settlement in a single TX.
func (h *SitosTickHandler) tickSettlement(ctx context.Context, settlementID, worldID uuid.UUID, grainBaseValue float64) error {
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var currentTick, population int
	var fundSilver float64
	if err := tx.QueryRow(ctx,
		`SELECT current_world_tick(), population, GREATEST(0, sitos_fund_silver)
		 FROM settlements WHERE id = $1 FOR UPDATE`,
		settlementID,
	).Scan(&currentTick, &population, &fundSilver); err != nil {
		return fmt.Errorf("load settlement: %w", err)
	}

	fundCap := FundCap(population, grainBaseValue, h.cfg)
	if fundSilver > fundCap {
		fundSilver = fundCap // defensive clamp (e.g. after a pop drop shrank the cap)
	}

	// Tax leg (guarded — never more than the settlement holds, never over cap).
	fundSilver, err = h.applyTax(ctx, tx, settlementID, worldID, population, fundSilver, fundCap)
	if err != nil {
		return fmt.Errorf("tax: %w", err)
	}

	// Stabilization leg, per subsistence good. Thread fundSilver sequentially so
	// each good sees the balance left by the previous one.
	for _, good := range h.cfg.SubsistenceGoods {
		fundSilver, err = h.stabilizeGood(ctx, tx, settlementID, worldID, good, currentTick, fundSilver, fundCap)
		if err != nil {
			return fmt.Errorf("stabilize %s: %w", good, err)
		}
	}

	// Release leg: push any fund overhang above cap into the settlement's liquid
	// silver, so the ~89% of M0 dammsuget into funds circulates. Reads the fund
	// from the DB (locked above) rather than the clamped local — over-cap funds
	// (e.g. after SITOS_FUND_CAP_MULT was lowered) live only in the DB.
	if err := h.releaseOverhang(ctx, tx, settlementID, worldID, fundCap); err != nil {
		return fmt.Errorf("release: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	// Low-fund notice (best-effort, outside the tx). Grain-net-negative-3-ticks
	// notice is intentionally deferred (nice-to-have, needs a counter) — see
	// temenos_sitos.md.
	if h.hub != nil && fundCap > 0 && fundSilver < 0.10*fundCap {
		var ownerID uuid.UUID
		if err := h.pool.QueryRow(ctx, `SELECT owner_id FROM settlements WHERE id = $1`, settlementID).Scan(&ownerID); err == nil {
			_ = h.hub.NotifyPlayer(ctx, worldID, ownerID, "SitosFundLow", 2, map[string]any{
				"settlement_id": settlementID,
				"fund_silver":   fundSilver,
				"fund_cap":      fundCap,
			})
		}
	}
	return nil
}

// applyTax moves up to (pop × taxRate / TicksPerDay) silver from the settlement
// into its fund, gated by both the settlement's silver stock and the fund's cap
// headroom. Returns the fund balance after the tax.
func (h *SitosTickHandler) applyTax(ctx context.Context, tx pgx.Tx, settlementID, worldID uuid.UUID, population int, fundSilver, fundCap float64) (float64, error) {
	headroom := fundCap - fundSilver
	if headroom <= 0 {
		return fundSilver, nil
	}

	var settlementSilver float64
	err := tx.QueryRow(ctx,
		`SELECT COALESCE(GREATEST(0, settled(amount, rate, calc_tick)), 0)
		 FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'silver'`,
		settlementID,
	).Scan(&settlementSilver)
	if errors.Is(err, pgx.ErrNoRows) {
		return fundSilver, nil
	}
	if err != nil {
		return fundSilver, fmt.Errorf("load silver: %w", err)
	}

	desired := float64(population) * h.cfg.TaxRate / float64(events.TicksPerDay)
	tax := desired
	if settlementSilver < tax {
		tax = settlementSilver
	}
	if headroom < tax {
		tax = headroom
	}
	if tax <= 0 {
		return fundSilver, nil
	}

	if _, err := tx.Exec(ctx,
		`UPDATE settlement_goods
		    SET amount = settled(amount, rate, calc_tick) - $1,
		        calc_tick = current_world_tick()
		  WHERE settlement_id = $2 AND good_key = 'silver'`,
		tax, settlementID,
	); err != nil {
		return fundSilver, fmt.Errorf("debit settlement silver: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`UPDATE settlements SET sitos_fund_silver = LEAST($1, GREATEST(0, sitos_fund_silver) + $2) WHERE id = $3`,
		fundCap, tax, settlementID,
	); err != nil {
		return fundSilver, fmt.Errorf("credit fund: %w", err)
	}

	newFund := fundSilver + tax
	_, _ = h.store.Append(ctx, settlementID, events.StreamProvince, "SitosTransaction",
		SitosTransactionPayload{Good: GoodSilver, Kind: "tax", SilverDelta: tax, GoodDelta: 0, FundSilver: newFund},
		worldID, nil)
	return newFund, nil
}

// releaseOverhang moves any Sitos fund above cap into the settlement's liquid
// silver, bounded by the liquid silver cap's headroom. This is the sole
// conservation-preserving path for over-cap funds: the alternative — letting the
// cap's LEAST() clamps in the tax/sell legs silently claw the overhang down —
// would destroy silver outright. The moved amount is computed BEFORE any UPDATE
// (triple-gate principle: never clip after silver has left a party), so both
// legs move exactly the same amount and silver stays conserved. Fund_after is
// always ≥ cap: if liquid headroom can't absorb the whole overhang, the rest is
// released on later ticks.
func (h *SitosTickHandler) releaseOverhang(ctx context.Context, tx pgx.Tx, settlementID, worldID uuid.UUID, fundCap float64) error {
	var fund float64
	if err := tx.QueryRow(ctx,
		`SELECT GREATEST(0, sitos_fund_silver) FROM settlements WHERE id = $1`,
		settlementID,
	).Scan(&fund); err != nil {
		return fmt.Errorf("load fund: %w", err)
	}
	overhang := fund - fundCap
	if overhang <= 0 {
		return nil // fund within cap — noop
	}

	var liquid, silverCap float64
	err := tx.QueryRow(ctx,
		`SELECT COALESCE(GREATEST(0, settled(amount, rate, calc_tick)), 0), COALESCE(cap, 1000)
		 FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'silver'`,
		settlementID,
	).Scan(&liquid, &silverCap)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // no silver row to receive the release — leave the fund as is
	}
	if err != nil {
		return fmt.Errorf("load silver: %w", err)
	}

	headroom := silverCap - liquid
	if headroom <= 0 {
		return nil // liquid already at cap; release the rest next tick
	}
	moved := overhang
	if headroom < moved {
		moved = headroom
	}
	if moved <= 0 {
		return nil
	}

	// moved ≤ headroom ⇒ liquid+moved ≤ cap, so LEAST never clips (conserved).
	if _, err := tx.Exec(ctx,
		`UPDATE settlement_goods
		    SET amount = LEAST(cap, settled(amount, rate, calc_tick) + $1),
		        calc_tick = current_world_tick()
		  WHERE settlement_id = $2 AND good_key = 'silver'`,
		moved, settlementID,
	); err != nil {
		return fmt.Errorf("credit settlement silver: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`UPDATE settlements SET sitos_fund_silver = GREATEST(0, sitos_fund_silver - $1) WHERE id = $2`,
		moved, settlementID,
	); err != nil {
		return fmt.Errorf("debit fund: %w", err)
	}

	_, _ = h.store.Append(ctx, settlementID, events.StreamProvince, "SitosFundRelease",
		SitosFundReleasePayload{
			SettlementID: settlementID,
			SilverMoved:  moved,
			FundAfter:    fund - moved,
			Cap:          fundCap,
		},
		worldID, nil)
	return nil
}

// stabilizeGood evaluates and applies the fund's buy/sell action for one good.
// Returns the fund balance after the action (unchanged on noop).
func (h *SitosTickHandler) stabilizeGood(ctx context.Context, tx pgx.Tx, settlementID, worldID uuid.UUID, good string, currentTick int, fundSilver, fundCap float64) (float64, error) {
	var amount, rate, cap, baseValue float64
	var calcTick int
	err := tx.QueryRow(ctx,
		`SELECT sg.amount, sg.rate, sg.cap, sg.calc_tick, g.base_value
		 FROM settlement_goods sg JOIN goods g ON g.key = sg.good_key
		 WHERE sg.settlement_id = $1 AND sg.good_key = $2`,
		settlementID, good,
	).Scan(&amount, &rate, &cap, &calcTick, &baseValue)
	if errors.Is(err, pgx.ErrNoRows) {
		return fundSilver, nil // good not tracked here (e.g. no fish) — skip
	}
	if err != nil {
		return fundSilver, fmt.Errorf("load good: %w", err)
	}

	var settlementSilver, settlementSilverCap float64
	err = tx.QueryRow(ctx,
		`SELECT COALESCE(GREATEST(0, settled(amount, rate, calc_tick)), 0), COALESCE(cap, 1000)
		 FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'silver'`,
		settlementID,
	).Scan(&settlementSilver, &settlementSilverCap)
	if errors.Is(err, pgx.ErrNoRows) {
		settlementSilver, settlementSilverCap = 0, 1000
	} else if err != nil {
		return fundSilver, fmt.Errorf("load silver: %w", err)
	}

	stock := amount + rate*float64(currentTick-calcTick)
	if stock < 0 {
		stock = 0
	}
	reference := ProductionReference(rate)
	refPrice := RefPrice(baseValue, amount, rate, float64(calcTick), currentTick, h.cfg)
	actualPrice := LocalPrice(baseValue, stock, rate)

	action := EvaluateSitosAction(refPrice, actualPrice, stock, reference,
		fundSilver, fundCap, settlementSilver, settlementSilverCap)
	if action.Kind == "noop" {
		return fundSilver, nil
	}

	var goodDelta, silverDelta, newFund float64
	switch action.Kind {
	case "buy":
		// Fund buys surplus grain: grain leaves the city (destroyed), silver goes
		// settlement←fund. Grain is the free sink; silver is conserved.
		if _, err := tx.Exec(ctx,
			`UPDATE settlement_goods SET amount = GREATEST(0, settled(amount, rate, calc_tick) - $1), calc_tick = current_world_tick()
			  WHERE settlement_id = $2 AND good_key = $3`,
			action.Quantity, settlementID, good,
		); err != nil {
			return fundSilver, fmt.Errorf("debit good: %w", err)
		}
		if _, err := tx.Exec(ctx,
			`UPDATE settlement_goods SET amount = LEAST(cap, settled(amount, rate, calc_tick) + $1), calc_tick = current_world_tick()
			  WHERE settlement_id = $2 AND good_key = 'silver'`,
			action.SilverMoved, settlementID,
		); err != nil {
			return fundSilver, fmt.Errorf("credit settlement silver: %w", err)
		}
		if _, err := tx.Exec(ctx,
			`UPDATE settlements SET sitos_fund_silver = GREATEST(0, sitos_fund_silver - $1) WHERE id = $2`,
			action.SilverMoved, settlementID,
		); err != nil {
			return fundSilver, fmt.Errorf("debit fund: %w", err)
		}
		goodDelta = -action.Quantity
		silverDelta = -action.SilverMoved
		newFund = fundSilver - action.SilverMoved

	case "sell":
		// Fund sells grain into a shortage: grain arrives (created), silver goes
		// settlement→fund. Grain is the free source; silver is conserved.
		if _, err := tx.Exec(ctx,
			`UPDATE settlement_goods SET amount = LEAST(cap, settled(amount, rate, calc_tick) + $1), calc_tick = current_world_tick()
			  WHERE settlement_id = $2 AND good_key = $3`,
			action.Quantity, settlementID, good,
		); err != nil {
			return fundSilver, fmt.Errorf("credit good: %w", err)
		}
		if _, err := tx.Exec(ctx,
			`UPDATE settlement_goods SET amount = GREATEST(0, settled(amount, rate, calc_tick) - $1), calc_tick = current_world_tick()
			  WHERE settlement_id = $2 AND good_key = 'silver'`,
			action.SilverMoved, settlementID,
		); err != nil {
			return fundSilver, fmt.Errorf("debit settlement silver: %w", err)
		}
		if _, err := tx.Exec(ctx,
			`UPDATE settlements SET sitos_fund_silver = LEAST($1, GREATEST(0, sitos_fund_silver) + $2) WHERE id = $3`,
			fundCap, action.SilverMoved, settlementID,
		); err != nil {
			return fundSilver, fmt.Errorf("credit fund: %w", err)
		}
		goodDelta = action.Quantity
		silverDelta = action.SilverMoved
		newFund = fundSilver + action.SilverMoved
	}

	_, _ = h.store.Append(ctx, settlementID, events.StreamProvince, "SitosTransaction",
		SitosTransactionPayload{
			Good: good, Kind: action.Kind, SilverDelta: silverDelta,
			GoodDelta: goodDelta, RefPrice: refPrice, FundSilver: newFund,
		},
		worldID, nil)

	// Fas 2c: the fund's safety net (selling emergency stock into a shortage)
	// previously only showed up in `ticklog` — a Wanax had to think to go
	// looking for it after the fact. The "sell" leg IS the rescue case (the
	// settlement receives good, paying silver); "buy" is routine surplus
	// absorption and stays silent as before.
	if h.hub != nil && action.Kind == "sell" {
		var ownerID uuid.UUID
		if err := h.pool.QueryRow(ctx, `SELECT owner_id FROM settlements WHERE id = $1`, settlementID).Scan(&ownerID); err == nil {
			_ = h.hub.NotifyPlayer(ctx, worldID, ownerID, "SitosIntervention", 2, map[string]any{
				"settlement_id": settlementID,
				"good":          good,
				"quantity":      action.Quantity,
				"silver_cost":   action.SilverMoved,
			})
		}
	}
	return newFund, nil
}

// TODO: Fas 3 — loyalty coupling (price deviation → loyalty delta via an emitted
// event, never a direct UPDATE; asymmetric penalty>bonus slope). Deferred — see
// temenos_sitos.md §Fas 3. Until then the ticklog loyalty row is "—".
