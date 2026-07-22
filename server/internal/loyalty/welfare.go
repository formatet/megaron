package loyalty

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/tick"
)

// Wave L1 (Timothy 2026-07-11): stads-loyalty välfärdssignaler. Unit/desertion
// effects from welfare are a SEPARATE later wave (L2) — this handler never
// touches combat/upkeep/resolver/unit_arrival.

// kharisBlessThresholdRef mirrors kharis.blessThreshold (internal/kharis/tick.go:27,
// currently 60.0 on the 0–100 scale). Not imported directly — kharis is not
// exported and pulling in the kharis package here would drag its wider tick
// machinery into loyalty for a single number. Retune together if kharis's own
// threshold changes. Calibration, not an invariant.
const kharisBlessThresholdRef = 60.0

// varietyThreshold is how many distinct FoodGoods (economy.FoodGoods) a
// settlement must stock a meaningful amount of to count as "varied diet".
// Calibration — Timothy's "ännu mer upp" ask, not an invariant.
const varietyThreshold = 2

// foodStockMinimum is the smallest settled() stock of a food good that counts
// as "the city actually has it" for the variety count — guards against dust
// amounts. Calibration.
const foodStockMinimum = 1.0

// netDeltaMin/netDeltaMax clamp the total daily welfare swing before it's
// handed to AppendLoyaltyEvent (which separately clamps the settlement's
// loyalty itself to [1,4]). Calibration.
const (
	netDeltaMin = -2
	netDeltaMax = 2
)

// welfareEventTypes are the loyalty_events.event_type values this handler can
// write. Used both to pick the emitted row's type and to guard idempotency
// (no welfare event of any of these types already written this game-day).
const (
	welfareEventWellFavoured = "well_favoured"
	welfareEventWellFed      = "well_fed"
	welfareEventStarving     = "starving"
	welfareEventVariedDiet   = "varied_diet"
)

// welfareWindowSeconds is one game-day expressed in wall-clock SECONDS at the
// current tick cadence — the same tick-substrate-derived conversion decay.go
// uses for its (2-game-day) grace window (see decayGraceSeconds), applied here
// to a single game-day so the idempotency guard below tracks the tick substrate
// rather than a raw wall-clock constant. Uses tick.TickSeconds (not TickMinutes,
// which floors to 1 minute and inflates the window ~10× on a sub-minute cadence).
func welfareWindowSeconds() int {
	return events.TicksPerDay * tick.TickSeconds
}

// WelfareHandler applies daily loyalty welfare signals (kharis favour, feeding,
// starvation, diet variety) to all active settlements in a world.
type WelfareHandler struct {
	pool       *pgxpool.Pool
	scheduler  *events.Scheduler
	eventStore *events.Store
}

// NewWelfareHandler creates a WelfareHandler.
func NewWelfareHandler(pool *pgxpool.Pool, sched *events.Scheduler, store *events.Store) *WelfareHandler {
	return &WelfareHandler{pool: pool, scheduler: sched, eventStore: store}
}

// welfareRow is one settlement's raw welfare inputs for a day.
type welfareRow struct {
	settlementID uuid.UUID
	kharis       float64
	grainStock   float64
	grainRate    float64
	foodVariety  int
}

// Handle processes a LoyaltyWelfareTick scheduled event.
func (h *WelfareHandler) Handle(ctx context.Context, e events.ScheduledEvent) error {
	rows, err := h.pool.Query(ctx,
		`SELECT
		    s.id,
		    COALESCE(settled(pwr.kharis_amount, pwr.kharis_rate, pwr.kharis_calc_tick), 0) AS kharis,
		    COALESCE((SELECT settled(sg.amount, sg.rate, sg.calc_tick)
		              FROM settlement_goods sg
		              WHERE sg.settlement_id = s.id AND sg.good_key = 'grain'), 0) AS grain_stock,
		    COALESCE((SELECT sg.rate FROM settlement_goods sg
		              WHERE sg.settlement_id = s.id AND sg.good_key = 'grain'), 0) AS grain_rate,
		    (SELECT COUNT(*) FROM settlement_goods sg
		     WHERE sg.settlement_id = s.id
		       AND sg.good_key = ANY($2)
		       AND settled(sg.amount, sg.rate, sg.calc_tick) >= $3) AS food_variety
		 FROM settlements s
		 LEFT JOIN player_world_records pwr
		   ON pwr.player_id = s.owner_id AND pwr.world_id = s.world_id
		 WHERE s.world_id = $1 AND s.state = 'active' AND s.owner_id IS NOT NULL
		   AND NOT EXISTS (
		       SELECT 1 FROM loyalty_events le
		       WHERE le.settlement_id = s.id
		         AND le.event_type IN ($4, $5, $6, $7)
		         AND le.created_at > now() - ($8 * interval '1 second')
		   )`,
		e.WorldID, economy.FoodGoods, foodStockMinimum,
		welfareEventWellFavoured, welfareEventWellFed, welfareEventStarving, welfareEventVariedDiet,
		welfareWindowSeconds(),
	)
	if err != nil {
		return fmt.Errorf("query settlements for welfare tick: %w", err)
	}

	var due []welfareRow
	for rows.Next() {
		var w welfareRow
		if err := rows.Scan(&w.settlementID, &w.kharis, &w.grainStock, &w.grainRate, &w.foodVariety); err == nil {
			due = append(due, w)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	for _, w := range due {
		if err := h.applyWelfare(ctx, w, e.WorldID); err != nil {
			slog.Error("loyalty welfare failed", "settlement", w.settlementID, "err", err)
		}
	}

	return h.scheduler.EnqueueTickRecurring(ctx, e.WorldID, events.ScheduledLoyaltyWelfareTick,
		DailyTickPayload{}, e.DueTick, events.TicksPerDay)
}

func (h *WelfareHandler) applyWelfare(ctx context.Context, w welfareRow, worldID uuid.UUID) error {
	// "Mätt vs svält" = kan staden föda sig? Grain flödar genom varje tick
	// (grain-cap-pegging borttagen) så en självförsörjande stad håller bara
	// ~1 ticks buffert — DÄRFÖR duger inte ProductionReference (~3-dygns-ankare)
	// som tröskel; den kallade friska huvudstäder "svält". Rätt diskriminator är
	// grain-NETTOT: rate ≥ 0 (självförsörjande/överskott) = mätt; rate < 0 eller
	// tomt/negativt lager (redan i underskott) = svält. Verifierat mot rådata
	// (c3c289e5 2026-07-11): friska huvudstäder rate +300…+2300, svältande −6…−15.
	kharisGood := w.kharis >= kharisBlessThresholdRef
	starving := w.grainStock <= 0 || w.grainRate < 0
	fed := !starving
	varied := w.foodVariety >= varietyThreshold

	delta := welfareDelta(kharisGood, fed, starving, w.foodVariety)
	delta = clampNetDelta(delta)
	if delta == 0 {
		return nil // no brus — nothing meaningfully changed today.
	}

	eventType, factors := classifyWelfare(kharisGood, fed, starving, varied)
	reason := strings.Join(factors, "; ")

	return AppendLoyaltyEvent(ctx, h.pool, h.eventStore, w.settlementID, worldID, eventType, delta, reason)
}

// welfareDelta computes the net daily loyalty delta for one settlement from
// its standing welfare conditions. Pure function — no DB, no clamping (the
// caller clamps to [netDeltaMin, netDeltaMax] before emitting).
func welfareDelta(kharisGood, fed, starving bool, foodVariety int) int {
	delta := 0
	if kharisGood {
		delta++
	}
	if fed {
		delta++
	} else if starving {
		delta--
	}
	if foodVariety >= varietyThreshold {
		delta++
	}
	return delta
}

// clampNetDelta bounds a computed daily welfare delta to [netDeltaMin, netDeltaMax].
func clampNetDelta(delta int) int {
	if delta < netDeltaMin {
		return netDeltaMin
	}
	if delta > netDeltaMax {
		return netDeltaMax
	}
	return delta
}

// classifyWelfare picks the single event_type to record for a multi-factor day
// (priority: starving > well_favoured > well_fed > varied_diet — starving is
// the most urgent signal and always surfaces if present; varied_diet is the
// weakest/only signal it's the sole condition met) and lists every
// contributing factor for the reason string, so a multi-cause day is still
// fully legible even though only one event_type is stored.
func classifyWelfare(kharisGood, fed, starving, varied bool) (eventType string, factors []string) {
	switch {
	case starving:
		eventType = welfareEventStarving
	case kharisGood:
		eventType = welfareEventWellFavoured
	case fed:
		eventType = welfareEventWellFed
	case varied:
		eventType = welfareEventVariedDiet
	}

	if kharisGood {
		factors = append(factors, "well favoured (kharis at or above bless threshold)")
	}
	if fed {
		factors = append(factors, "well fed (grain stock at or above the comfortable-buffer reference)")
	} else if starving {
		factors = append(factors, "starving (grain stock below the comfortable-buffer reference)")
	}
	if varied {
		factors = append(factors, "varied diet (2+ food goods stocked)")
	}
	return eventType, factors
}
