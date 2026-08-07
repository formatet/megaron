// Package kharis implements the daily temple maintenance tick for Megaron.
// Kharis is a reciprocal relationship between a settlement and its gods.
// Settlements that maintain their temples accumulate divine favour;
// those that neglect maintenance lose it — and eventually suffer.
package kharis

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"sort"

	"formatet/megaron/server/internal/ai"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/hexgrid"
	"formatet/megaron/server/internal/religion"
	"formatet/megaron/server/internal/unit"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Kharis omdesign (Timothy 2026-07-09, temenos_kharis.md §"KANONISK OMDESIGN"):
// the pool moved from a 0–2000 scale to a single hidden 0–100 number. All
// thresholds below are STRAWMAN — balance is calibrated later
// (temenos_balans_spakar.md §9). See megaron_kharis_plan.md FAS 0.
const (
	punishThreshold   = 30.0 // kharis below this risks divine punishment (was 100/2000)
	punishProbability = 0.30 // 30% chance of divine punishment per missed day below threshold
	blessThreshold    = 60.0 // kharis above this may attract divine favour (was 200/2000)
	blessProbability  = 0.15 // 15% chance of divine blessing per maintained day above threshold
)

// kharisPerTempleDay is what one FULLY DEVOTED temple earns its Wanax per day
// at tier 1 (mig 094, "kult är ingen vara"). The old model summed a `cult` STOCK
// to derive a RATE — a fake good with weight 0 that nobody could trade, eat or
// carry, and whose zero weight divided by zero in the sack loot query (3670805).
// The tick now reads temple state directly: presence × devotion × whether it was
// fed.
//
// Calibrated against decayBas (1.0/day) and against what a temple can actually
// EMPLOY (templeDevotionCapacity): a level-1 temple staffed to its capacity and
// fed climbs slowly (0.15 × 8.0 = 1.2 vs 1.0 decay = +0.2/day), an unfed or
// unstaffed one fades, and a larger temple — which can employ more of the city —
// climbs proportionally faster. "Skött tempel ≈ långsam klättring, försummelse =
// fade." Strawman — temenos_balans_spakar.md §9.
//
// Raised 1.2 → 8.0 (Timothy 2026-07-23): at 1.2 even a temple staffed to the
// full level-1 capacity earned 0.18/day against 1.0 decay, so EVERY Wanax faded
// no matter what they did — a fade nobody could escape is not a choice.
const kharisPerTempleDay = 8.0

// templeDevotionCapacity is how much of a city's population a temple of this
// level can put to work at the altar, as a share (Timothy 2026-07-23: the
// temple's maximum is "antal som kan sysselsättas i det"). Devotion allocated
// beyond it is not served by the building and does not count.
//
// Level 1 lands exactly on the 0.15 floor LaborAlloc already applies, so every
// existing city is staffed to capacity from the day this lands — and the only
// way to devote MORE of a city to the gods is a larger temple. That is what
// makes temple levels worth having.
func templeDevotionCapacity(level int) float64 {
	if level < 1 {
		level = 1
	}
	return TempleDevotionPerLevel * float64(level)
}

// TempleDevotionPerLevel is both the level-1 capacity and the step per level.
// TempleDevotionPerLevel is exported so the status surfaces can show the same
// capacity the tick enforces, rather than duplicating the number.
const TempleDevotionPerLevel = 0.15

// FAS 2 — natural depreciation + imperie-belastning (Timothy 2026-07-09 kharis
// omdesign, temenos_kharis.md §"KANONISK OMDESIGN" FAS 2/3): dailyDecay =
// decayBas + decayPerKoloni × colonies without their own temple beyond
// decayFreeColonies free ones. A Wanax who expands without building temples in
// the new colonies pays for it; a temple (presence of the building alone
// suffices) in a colony zeroes that colony's contribution.
//
// NET-NEUTRAL recalibration (Timothy 2026-07-11, A#4 kharis-rot): decayBas was
// 4.0, which made a maintained temple net NEGATIVE (~−1.8/day vs the passive
// geographic kharis_rate ~0.6/day + cult-gain), so kharis bled to the floor and
// bless (≥60) was unreachable — and on a sped-up world (TICK_SECONDS) a restart's
// tick catch-up replayed dozens of those net-negative days in one burst, flooring
// every Wanax to 1. Intent now: PASSIVE (bare geographic kharis_rate, no active
// offering) ≈ neutral / slow fade — "the relationship is tended actively"; an
// OFFERING-fed temple is the only way UP (rites SPEND standing, they don't add
// kharis). decayBas ≈ the typical passive rate achieves that shape regardless of
// the exact cult-gain (design target ~0.8/day, live-observed ~2.2 — the climb
// RATE is uncertain and must be re-measured at the next soak, but the SIGN is
// right either way). Tunbar. temenos_balans_spakar.md §9 · temenos_kharis.md.
// decayBas raised 1.0 → 1.8 (Timothy 2026-07-24, "för hög och enkel"): at 1.0 a
// bare L1 temple staffed only to the 0.15 devotion FLOOR still netted +0.70/day
// (soak-measured: gain 1.70 − decay 1.00), so EVERY Wanax climbed to their ceiling
// over a long soak with zero active effort — auto-substitution feeds every temple
// and an L1 temple can employ only the floor, so nobody could climb faster or
// slower than anyone else. At 1.8 the floor-staffed L1 temple nets ≈ −0.1/day (a
// slow fade), and only a temple large enough to employ MORE devotion than the floor
// (level ≥ 2, which also raises the ceiling) climbs — restoring the documented
// intent "passiv ≈ neutral/fade, aktivt skött (större tempel) = enda vägen upp"
// without touching feedTempleBySubstitution (so the wine-lockout stays fixed).
// Tunbar — temenos_balans_spakar.md §9.
const (
	decayBas          = 1.8 // base daily decay: floor-staffed L1 nets ~neutral/slow-fade; L2+ climbs
	decayPerKoloni    = 1.0 // extra daily decay per templeless colony beyond decayFreeColonies
	decayFreeColonies = 4   // this many templeless colonies cost nothing extra
)

// FAS 3 — offer-underhåll (Timothy 2026-07-09 kharis omdesign, temenos_kharis.md
// §"KANONISK OMDESIGN" §4): each temple consumes a small daily material offer
// (oil+wine) from ITS OWN settlement's goods — offerings are local, you feed
// the temple where it stands, not from a shared pool. Strawman quantities —
// temenos_balans_spakar.md §9. Exported (PLAN B, megaron_kult_legibilitet_plan.md)
// so the status endpoint (api/handlers/province.go) can show the same
// requirement the tick actually enforces instead of duplicating the numbers.
const (
	OfferOilPerTemple  = 2.0
	OfferWinePerTemple = 1.0
)

// templeTierMultiplier is the FAS 4 hook point — DEFERRED, not built in this
// branch (megaron_kharis_plan.md: "Kräver tempel-nivåer/upgrade-tiers som inte
// finns än"). It depends on the city-building upgrade epic (temenos_
// stadsbyggnad.md) and is sequenced after the kharis core (FAS 0-3). For now
// every temple is level 1 and this always returns 1.0 (no-op multiplier) —
// nothing in FAS 0-3 calls it yet.
// TODO: once temple upgrade tiers exist, multiply gain (and/or effective cap)
// by this — "coola tempel" per the design doc §4.
func templeTierMultiplier(level int) float64 {
	return 1.0
}

// kharisFloor is the "heligt golv" the kharis METER itself never crosses below —
// distinct from riteFloor (api/handlers/settlement.go), which is the rite SUCCESS
// floor. Design text: "kharis_amount ∈ [0, 100] ... aldrig exakt 0 — gudarna
// lyssnar alltid ibland." No exact number is given in the design docs; 1.0 is a
// strawman pick, easy to retune — temenos_balans_spakar.md §9.
const kharisFloor = 1.0

// grainPerCitizen is the grain cost of one new citizen at the daily growth
// tick — makes growth a real economic draw on grain instead of a binary gate.
//
// Calibration story (see TestApplyDecay_GrainFundedGrowth_* for the measured
// numbers): a naive read of "consume 50–70% of surplus" against the good's
// storage CAP (1000) doesn't hold up once measured — the decay step above
// writes an uncapped settled() value, so any self-sufficient catchment's raw
// daily accrual (rate, per tick) is many multiples of the 1000 cap
// (≈5450 for the minimal one-plains-tile guaranteed floor, ≈13000+ for a
// two-plains-tile catchment at start pop 5000). A modest draw against that
// (25 × desired_new, ≈500/day) vanishes into the overshoot and
// RecomputeProduction's own end-of-tick LEAST(cap,…) clamp re-pins the stock
// at 1000 regardless — satisfying neither "cap un-pinned" nor "richer
// catchment grows faster" (both catchments simply re-saturate identically).
// grainPerCitizen=300 instead prices growth against that *raw* daily accrual:
// the minimal one-plains catchment can only ever afford ~17–18 of the 21
// desired new citizens/day (its cost, 21×300=6300, exceeds its ≈5450
// accrual), spending nearly all of it and leaving a small-but-always-positive
// remainder (1–300 grain, varies day to day, never zero — proven over 40 days
// in TestApplyDecay_GrainFundedGrowth_MinimalCitySelfSufficient) — this is
// what makes success criterion #2 (cap un-pinned) hold. A richer catchment
// (≥2 grain tiles) has proportionally more accrual against the SAME cost
// (desired_new depends on population/soft-cap only, not catchment), so it
// affords desired growth in full every day and grows measurably faster —
// criterion #3 — while its own stock re-saturates at cap (expected: its
// surplus genuinely exceeds what a day's growth can spend). The floor-division
// throttle (§ actual_new = floor(grain_now/grainPerCitizen) when unaffordable)
// necessarily leaves a remainder in [0, grainPerCitizen) — occasionally small
// in absolute terms on a given day — but it is mathematically always ≥ 0 and
// never sign-flips negative (GREATEST(0,…) floors throughout), and a second
// same-tick firing is a safe no-op (draw=0 when nothing is affordable). If
// this ever measures as breaking the never-starve invariant for some other
// catchment shape, lower it.
//
// REVIEWED AGAINST P1 (catchment 7→19, megaron_plan_fysisk_gubbemodell.md,
// 2026-08-07) — left unchanged. The "minimal one-plains-tile" floor this
// number is calibrated against is a TERRAIN LUCK case (does a founding site
// have any grain-producing tile at all), not a catchment-SIZE case — a
// minimal catchment can still be "1 plains tile among 18" exactly as it was
// "1 plains tile among 6", so its raw accrual barely moves: ~5450/day
// pre-P1 → ~5500/day post-P1 (+NearjordGrainPerTick, economy/recompute.go,
// the only genuinely new floor contribution). A RICH catchment's ceiling
// triples (TestP1_ProductionMultiplierVsPreP1Catchment,
// internal/economy/catchment_p1_balance_test.go — 18 vs 6 ring hexes at
// identical density), but that never raises how fast a city can grow: growth
// is capped at desired_new (population/soft-cap only, economy-accrual-
// independent above the affordability floor) — surplus beyond what a day's
// growth can spend just re-saturates the grain stock, exactly as documented
// above. A bigger catchment gives a Wanax more OPTIONS (more terrain variety
// to allocate labor across via LaborCapacity's existing caps), not an
// automatic multiply-by-three in actual production — allocation is still
// player-chosen. Left as a strawman for soak-testing, per this constant's
// existing calibration story, not re-derived from first principles.
const grainPerCitizen = 300.0

// starvationPopLossRatePerTick is the fraction of population a starving city
// loses per tick (−0.5%/tick). Single source of truth for BOTH sides:
// applyDecay's SQL binds it as a parameter — ROUND(pop * (1 - $3)::numeric) —
// to write the real pop mutation, and applyStarvationWarning multiplies by it
// to report the modelled pop_loss in the critical SubsistenceWarning payload.
// Retune here and both move together. (Reported loss stays an estimate: the
// SQL rounds survivors and floors at 101, the warning truncates the loss —
// same rate, not same delta.)
const starvationPopLossRatePerTick = 0.005

// TickHandler applies daily temple maintenance to all active settlements in a world.
type TickHandler struct {
	pool      *pgxpool.Pool
	scheduler *events.Scheduler
	store     *events.Store
	hub       Broadcaster
}

// NewTickHandler creates a TickHandler. hub may be nil (tests) — every
// NotifyPlayer call is nil-guarded, matching the other tick/upkeep handlers.
func NewTickHandler(pool *pgxpool.Pool, sched *events.Scheduler, store *events.Store, hub Broadcaster) *TickHandler {
	return &TickHandler{pool: pool, scheduler: sched, store: store, hub: hub}
}

// wanaxSnap holds the per-Wanax state needed for daily temple maintenance.
// Kharis lives on player_world_records; devotionSum aggregates temple staffing across all
// of the player's settlements in this world.
type wanaxSnap struct {
	playerID     uuid.UUID
	settlementID uuid.UUID // capital settlement (for event emission and divine effects)
	kharis       float64
	kharisCap    float64
	// devotionSum is Σ over the Wanax's temple cities of that city's devotion
	// weight (settlement_labor 'cult', 0..1) — "how many temples, and are they
	// staffed", in one number.
	devotionSum        float64
	templeCities       int
	maxTempleLevel     int // grandest temple's level — sets the kharis ceiling
	templelessColonies int // FAS 2: non-capital settlements with no temple building
}

// Handle processes a KharisTick scheduled event.
func (h *TickHandler) Handle(ctx context.Context, e events.ScheduledEvent) error {
	// ── 0. The gods reprice the world ──────────────────────────────────────
	// Scarcity-driven divine valuation, recomputed once here rather than per
	// rite — a prayer must never pay for a world-wide scan
	// (temenos_prayers_komposition_plan.md). Best-effort: a failed reprice must
	// not abort the day's kharis maintenance, and the previous day's valuation
	// stays readable, so the worst case is a stale price list.
	if err := religion.RecomputeDivineValuations(ctx, h.pool, e.WorldID); err != nil {
		slog.Error("divine valuation reprice failed", "err", err, "world", e.WorldID)
	}

	// ── 1. Kharis maintenance: one tick per player_world_record ────────────
	rows, err := h.pool.Query(ctx,
		`SELECT pwr.player_id, s.id AS capital_id,
		    GREATEST(0, settled(pwr.kharis_amount, pwr.kharis_rate, pwr.kharis_calc_tick)) AS kharis,
		    pwr.kharis_cap,
		    COALESCE((
		        -- Devotion counts only up to what the temple can EMPLOY: a
		        -- level-1 temple works $2 of the city, level 2 twice that, and
		        -- anything allocated beyond that has no altar to serve at.
		        SELECT SUM(LEAST(GREATEST(0, COALESCE(sl.weight, 0)), $2::float8 * GREATEST(1, b.level)))
		        FROM settlements s2
		        JOIN buildings b ON b.settlement_id = s2.id AND b.building_type = 'temple'
		        LEFT JOIN settlement_labor sl ON sl.settlement_id = s2.id AND sl.good_key = 'cult'
		        WHERE s2.owner_id = pwr.player_id AND s2.world_id = pwr.world_id
		          AND s2.state NOT IN ('sunk', 'collapsed', 'razed')
		    ), 0) AS devotion_sum,
		    COALESCE((
		        SELECT COUNT(DISTINCT s2.id)
		        FROM settlements s2
		        JOIN buildings b ON b.settlement_id = s2.id AND b.building_type = 'temple'
		        WHERE s2.owner_id = pwr.player_id AND s2.world_id = pwr.world_id
		          AND s2.state NOT IN ('sunk', 'collapsed', 'razed')
		    ), 0) AS temple_cities,
		    COALESCE((
		        -- The grandest temple sets the Wanax's kharis ceiling (see
		        -- templeKharisCeiling). MAX, not SUM: ten modest shrines must not
		        -- add up to what one great temple earns.
		        SELECT MAX(b.level)
		        FROM settlements s4
		        JOIN buildings b ON b.settlement_id = s4.id AND b.building_type = 'temple'
		        WHERE s4.owner_id = pwr.player_id AND s4.world_id = pwr.world_id
		          AND s4.state NOT IN ('sunk', 'collapsed', 'razed')
		    ), 0) AS max_temple_level,
		    COALESCE((
		        SELECT COUNT(*)
		        FROM settlements s3
		        WHERE s3.owner_id = pwr.player_id AND s3.world_id = pwr.world_id
		          AND s3.is_capital = false AND s3.state NOT IN ('sunk', 'collapsed')
		          AND NOT EXISTS (
		              SELECT 1 FROM buildings b
		              WHERE b.settlement_id = s3.id AND b.building_type = 'temple'
		          )
		    ), 0) AS templeless_colonies
		 FROM player_world_records pwr
		 JOIN settlements s ON s.owner_id = pwr.player_id AND s.world_id = pwr.world_id AND s.is_capital = true
		 WHERE pwr.world_id = $1`,
		e.WorldID, TempleDevotionPerLevel,
	)
	if err != nil {
		return fmt.Errorf("query player_world_records for kharis tick: %w", err)
	}
	defer rows.Close()

	var snaps []wanaxSnap
	for rows.Next() {
		var w wanaxSnap
		if err := rows.Scan(&w.playerID, &w.settlementID,
			&w.kharis, &w.kharisCap, &w.devotionSum, &w.templeCities,
			&w.maxTempleLevel, &w.templelessColonies); err == nil {
			// The grandest temple binds the ceiling from here on — processMaintenance
			// and everything downstream sees one already-resolved cap.
			w.kharisCap = EffectiveKharisCap(w.kharisCap, w.maxTempleLevel)
			snaps = append(snaps, w)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, w := range snaps {
		if err := h.processMaintenance(ctx, w, e.WorldID, e.ID); err != nil {
			slog.Error("kharis maintenance failed", "player", w.playerID, "err", err)
		}
	}

	// ── 2. AI governor ticks (per settlement, unchanged) ──────────────────
	aiRows, err := h.pool.Query(ctx,
		`SELECT id FROM settlements WHERE world_id = $1 AND governor_is_ai = true AND state != 'sunk'`,
		e.WorldID,
	)
	if err == nil {
		defer aiRows.Close()
		for aiRows.Next() {
			var sid uuid.UUID
			if aiRows.Scan(&sid) == nil {
				if err := ai.PassiveGovernorTick(ctx, h.pool, sid, e.WorldID); err != nil {
					slog.Warn("passive governor tick failed", "settlement", sid, "err", err)
				}
			}
		}
	}

	h.applyDecay(ctx, e.WorldID)
	h.applyStarvationWarning(ctx, e.WorldID)
	h.applyStarvation(ctx, e.WorldID)
	h.applySubsistenceCritical(ctx, e.WorldID)
	h.accumulatePrestige(ctx, e.WorldID)

	return h.scheduler.EnqueueTickRecurring(ctx, e.WorldID, events.ScheduledKharisTick,
		struct{}{}, e.DueTick, events.MacroTickInterval)
}

// computeDailyDecay is the FAS 2 imperie-belastning formula: dailyDecay =
// decayBas + decayPerKoloni × colonies without their own temple beyond
// decayFreeColonies free ones. Pure function — unit-testable without a DB.
func computeDailyDecay(templelessColonies int) float64 {
	over := templelessColonies - decayFreeColonies
	if over < 0 {
		over = 0
	}
	return decayBas + decayPerKoloni*float64(over)
}

// kharisCapPerTempleLevel — the grandest temple a Wanax has built sets how close
// to the gods they can come. Ceiling = kharisCapPerTempleLevel × (1 + max temple
// level), so L1 = 50, L2 = 75, L3 = 100 (= the record's own kharis_cap).
//
// Why this exists (Timothy 2026-07-23, option (c) of three): after
// kharisPerTempleDay was raised 1.2 → 8.0 to stop the universal fade, and
// feedTempleBySubstitution made every temple always feedable, the two together
// let EVERY Wanax climb unobstructed to the cap. Measured after a 5 h soak: 13 of
// 17 Wanaxes sat at exactly 100, which pins the rite chance at riteCeil — 152
// rites, 140 successes (92 %). The rite had stopped being a wager and become a
// fee.
//
// Level 1 deliberately lands BELOW blessThreshold (60): a Wanax with only a
// modest temple keeps their kharis and their rites, but never attracts divine
// favour. Reaching the gods' notice requires a grander temple. That makes temple
// levels matter in both directions — capacity (how many may serve) and ceiling
// (how far devotion can carry you) — instead of only the first.
//
// Note this option was unbuildable until 8b85ee2 (same day): temples could not be
// levelled at all, so a level-based ceiling would have frozen every Wanax under
// 60 permanently rather than giving them something to climb toward.
const kharisCapPerTempleLevel = 25.0

// TempleKharisCeiling returns the kharis ceiling earned by a Wanax's grandest
// temple. Level 0 (no temple) still yields a floor-ish ceiling of 25 — such a
// Wanax decays toward kharisFloor anyway, since devotion with no altar is 0.
func TempleKharisCeiling(maxTempleLevel int) float64 {
	if maxTempleLevel < 0 {
		maxTempleLevel = 0
	}
	return kharisCapPerTempleLevel * float64(1+maxTempleLevel)
}

// EffectiveKharisCap is the binding ceiling for a Wanax: the lower of the record's
// own kharis_cap and what their grandest temple earns them.
func EffectiveKharisCap(recordCap float64, maxTempleLevel int) float64 {
	if ceiling := TempleKharisCeiling(maxTempleLevel); ceiling < recordCap {
		return ceiling
	}
	return recordCap
}

// clampKharis bounds newKharis to [kharisFloor, cap] — the "heligt golv" (never
// exactly 0) and the Wanax's kharis_cap (100 by default post-FAS-0 migration).
func clampKharis(newKharis, cap float64) float64 {
	if newKharis < kharisFloor {
		return kharisFloor
	}
	if newKharis > cap {
		return cap
	}
	return newKharis
}

// computeOfferFraction is the FAS 3 gain-scaling formula: fed/total temples.
// 0 when there are no temples to feed — defensive only; a maintained day
// (templeCities > 0) implies at least one temple stands, so
// total should never be 0 on the only call site that matters. Pure function —
// unit-testable without a DB.
func computeOfferFraction(fed, total int) float64 {
	if total <= 0 {
		return 0
	}
	return float64(fed) / float64(total)
}

// applyTempleOffering consumes OfferOilPerTemple/OfferWinePerTemple from each
// of the Wanax's temple-having settlements' OWN oil/wine stock. A settlement is
// "fed" only if it can afford BOTH goods in full (checked before either is
// deducted, so a partial offer never happens). Returns (fed, total) for
// computeOfferFraction.
func (h *TickHandler) applyTempleOffering(ctx context.Context, playerID, worldID uuid.UUID) (fed, total int) {
	rows, err := h.pool.Query(ctx,
		`SELECT s.id,
		    COALESCE((SELECT settled(sg.amount, sg.rate, sg.calc_tick)
		              FROM settlement_goods sg WHERE sg.settlement_id = s.id AND sg.good_key = 'oil'), 0) AS oil,
		    COALESCE((SELECT settled(sg.amount, sg.rate, sg.calc_tick)
		              FROM settlement_goods sg WHERE sg.settlement_id = s.id AND sg.good_key = 'wine'), 0) AS wine
		 FROM settlements s
		 WHERE s.owner_id = $1 AND s.world_id = $2 AND s.state NOT IN ('sunk', 'collapsed')
		   AND EXISTS (SELECT 1 FROM buildings b WHERE b.settlement_id = s.id AND b.building_type = 'temple')`,
		playerID, worldID,
	)
	if err != nil {
		slog.Error("temple offering query failed", "player", playerID, "err", err)
		return 0, 0
	}
	defer rows.Close()

	type templeGoods struct {
		id        uuid.UUID
		oil, wine float64
	}
	var temples []templeGoods
	for rows.Next() {
		var t templeGoods
		if rows.Scan(&t.id, &t.oil, &t.wine) == nil {
			temples = append(temples, t)
		}
	}
	if err := rows.Err(); err != nil {
		slog.Error("temple offering rows error", "player", playerID, "err", err)
	}

	for _, t := range temples {
		total++
		if t.oil < OfferOilPerTemple || t.wine < OfferWinePerTemple {
			// The traditional offering is out of reach — but an altar takes what
			// it is given. Timothy 2026-07-22: "samma sak gäller vid kult och
			// stående gudstjänster", and until now only the SCARCITY half of that
			// was built: the standing offering still demanded exactly 2 oil +
			// 1 wine. A Wanax with no access to wine could therefore never feed a
			// temple, never earn kharis, and stayed pinned at the floor below
			// every prayer's MinKharis — a permanent lockout with no way out from
			// inside the game (soak 2026-07-23: Antilokhos, kharis 1.0, 28k oil,
			// 0 wine, no wine seller within reach).
			if h.feedTempleBySubstitution(ctx, t.id, worldID) {
				fed++
			}
			continue
		}
		if _, err := h.pool.Exec(ctx,
			`UPDATE settlement_goods SET
			   amount    = GREATEST(0, settled(amount, rate, calc_tick) - $2),
			   calc_tick = current_world_tick()
			 WHERE settlement_id = $1 AND good_key = 'oil'`,
			t.id, OfferOilPerTemple,
		); err != nil {
			slog.Error("temple offering oil deduction failed", "settlement", t.id, "err", err)
			continue
		}
		if _, err := h.pool.Exec(ctx,
			`UPDATE settlement_goods SET
			   amount    = GREATEST(0, settled(amount, rate, calc_tick) - $2),
			   calc_tick = current_world_tick()
			 WHERE settlement_id = $1 AND good_key = 'wine'`,
			t.id, OfferWinePerTemple,
		); err != nil {
			slog.Error("temple offering wine deduction failed", "settlement", t.id, "err", err)
			continue
		}
		fed++
	}
	return fed, total
}

func (h *TickHandler) processMaintenance(ctx context.Context, w wanaxSnap, worldID uuid.UUID, eventID int64) error {
	// Fas 2.2 exactly-once claim, scoped per (event_id, player_id) — migration 098.
	// Handle fans ONE ScheduledKharisTick across every Wanax and this function
	// commits each one separately (offering charge, kharis UPDATE, events), while
	// the worker only marks the event done AFTER Handle returns. Without the claim
	// a crash or a 5s G2 timeout part-way through the fan-out replays the whole
	// pass: the day's decay applied twice and the temple offering charged twice.
	// Claim-then-mutate rather than one big transaction: this function spans
	// several independent writes across packages, and wrapping them all would be a
	// far larger change than the risk warrants. The trade is a claimed-but-crashed
	// Wanax skipping ONE day's maintenance instead of getting two — for a decay
	// curve, missing a day is the strictly safer failure. Same pattern CLAUDE.md
	// lists as accepted (INSERT … ON CONFLICT DO NOTHING for projection writes).
	claim, err := h.pool.Exec(ctx,
		`INSERT INTO processed_tick_claims (event_id, scope_id) VALUES ($1, $2)
		 ON CONFLICT DO NOTHING`, eventID, w.playerID)
	if err != nil {
		return fmt.Errorf("claim kharis tick: %w", err)
	}
	if claim.RowsAffected() == 0 {
		return nil // this event already applied to this Wanax
	}

	// dailyDecay applies EVERY day, maintained or not (replacing the old
	// missed-day-only 10%/day decayOnMissed, retired in FAS 2). Post net-neutral
	// recalibration (Timothy 2026-07-11, see decayBas above) the base term is small
	// — ≈ the passive geographic kharis_rate — so a bare passive Wanax nets ~neutral
	// and an offering-fed temple climbs; it is no longer a hard "sjunker alltid"
	// drain. gain (from cult production, FAS 3-scaled by material offer) is the only
	// term that differs between the two branches below; bless/punish eligibility
	// still follows the maintained/missed split (temenos_balans_spakar.md §9: bless
	// only on a maintained day, punish only on a missed one).
	// A day is "maintained" when the Wanax holds at least one standing temple —
	// the institution, not a stockpile. Devotion and feeding then decide how much
	// that temple is worth.
	maintained := w.templeCities > 0

	// FAS 3 — offer-underhåll: a maintained day's cult-gain is scaled by how
	// many of the Wanax's temples were actually fed a material offer today.
	// Full offer everywhere -> full gain; no offer anywhere -> zero gain (the
	// dailyDecay below then wins outright regardless of cult labor). Cult-labor
	// still never costs kharis — only goods+labor. Offering is only attempted
	// on a maintained day: gain is 0×anything otherwise, so there's no point
	// draining a Wanax's oil/wine for a day that already produces no gain.
	var gain float64
	var offerFed, offerTotal int
	var offerFraction float64
	if maintained {
		// Devotion is the labor share serving the temple — read, never produced
		// (mig 094). A temple with no one tending it is a building, not a cult.
		devotionGain := w.devotionSum * kharisPerTempleDay
		offerFed, offerTotal = h.applyTempleOffering(ctx, w.playerID, worldID)
		offerFraction = computeOfferFraction(offerFed, offerTotal)

		// The gods reckon the offering by what the WORLD can spare, not by a
		// fixed price (Timothy 2026-07-22: "samma sak gäller vid kult och
		// stående gudstjänster"). Feeding a temple oil during an oil shortage is
		// a greater gift than feeding it oil in a year of plenty, and earns more.
		scarcity := h.templeOfferScarcity(ctx, worldID)
		gain = devotionGain * offerFraction * scarcity

		_, _ = h.store.Append(ctx, w.settlementID, events.StreamProvince, "KharisOffering",
			map[string]any{
				"temples_fed":       offerFed,
				"temples_total":     offerTotal,
				"offer_fraction":    offerFraction,
				"devotion_sum":      w.devotionSum,
				"scarcity_factor":   scarcity,
				"gain_before_offer": devotionGain,
				"kharis_gain":       gain,
			},
			worldID, nil)
	}

	dailyDecay := computeDailyDecay(w.templelessColonies)
	newKharis := clampKharis(w.kharis+gain-dailyDecay, w.kharisCap)

	// 1. Write the netted kharis value.
	if _, err := h.pool.Exec(ctx,
		`UPDATE player_world_records SET
		   kharis_amount    = $1,
		   kharis_calc_tick = current_world_tick()
		 WHERE player_id = $2 AND world_id = $3`,
		newKharis, w.playerID, worldID,
	); err != nil {
		return fmt.Errorf("update kharis: %w", err)
	}

	if maintained {
		// 2. Event + divine effects. (There used to be a step here that zeroed
		// settlement_goods good_key='cult' across the player's settlements —
		// removed 2026-08-03: migration 094 deleted every such row and nothing
		// recreates one with a positive amount, so the UPDATE had matched zero
		// rows since 094 landed. Cult is devotion — internal/economy.GoodCult
		// doc comment and settlement_labor, not a settlement_goods stock to
		// consume.)
		_, _ = h.store.Append(ctx, w.settlementID, events.StreamProvince, "KharisMaintained",
			map[string]any{
				"devotion_sum":        w.devotionSum,
				"kharis_gain":         gain,
				"daily_decay":         dailyDecay,
				"net":                 gain - dailyDecay,
				"templeless_colonies": w.templelessColonies,
				"offer_fraction":      offerFraction,
				"temples_fed":         offerFed,
				"temples_total":       offerTotal,
			},
			worldID, nil)
		if newKharis >= blessThreshold && rand.Float64() < blessProbability {
			h.applyDivineBlessing(ctx, w.settlementID, worldID)
		}
		if rand.Float64() < 0.20 {
			h.generateOmen(ctx, w.settlementID, worldID)
		}
	} else {
		// No temple production this day — dailyDecay above is the only kharis
		// change (gain=0). Punish roll only fires on this (missed) branch.
		_, _ = h.store.Append(ctx, w.settlementID, events.StreamProvince, "KharisMissedMaintenance",
			map[string]any{
				"reason":              "no_cult_production",
				"daily_decay":         dailyDecay,
				"templeless_colonies": w.templelessColonies,
			},
			worldID, nil)
		if newKharis < punishThreshold && rand.Float64() < punishProbability {
			h.applyDivinePunishment(ctx, w.settlementID, worldID)
		}
	}

	// 3. Derive mood and write back cult_level (drives prestige + display).
	derived := deriveMood(newKharis)
	_, _ = h.pool.Exec(ctx,
		`UPDATE player_world_records SET cult_level = $1
		 WHERE player_id = $2 AND world_id = $3`,
		derived, w.playerID, worldID,
	)
	return nil
}

// deriveMood maps the 0–100 kharis level to a mood label (replaces player-set
// cult_level). This is the SINGLE canonical threshold table — mood, rite success
// (settlement.go), and api/handlers.kharisToMood (web.go) all read the same four
// tiers (60/30/10, strawman — temenos_balans_spakar.md §9) so there is no longer a
// dual scale. Swedish labels for the two lower tiers ("tveksam"/"vredgad") are new
// strawman coinages — the design doc only names the English mood words for those.
func deriveMood(kharis float64) string {
	switch {
	case kharis >= 60:
		return "overdadig" // Favorable
	case kharis >= 30:
		return "vardig" // Indifferent
	case kharis >= 10:
		return "tveksam" // Suspicious
	default:
		return "vredgad" // Wrathful
	}
}

// applyDecay applies 1% daily decay to grain and timber stocks, resets
// invasions_today, and adjusts population. (Rite success is driven by Kharis
// mood, not a stored strength stat — there is nothing per-temple to regenerate.)
func (h *TickHandler) applyDecay(ctx context.Context, worldID uuid.UUID) {
	// Decay grain and timber by 1% per day. Population grain-consumption is NOT
	// applied here anymore: it is folded into grain's net rate in
	// economy.RecomputeProduction (continuous per-tick draw), so it never exceeds
	// the grain cap and a self-sufficient city holds a stable positive stock.
	// Cedar is a luxury store-of-value (ädelträ) and does not rot.
	if _, err := h.pool.Exec(ctx,
		`UPDATE settlement_goods sg SET
		   amount = GREATEST(0, settled(sg.amount, sg.rate, sg.calc_tick) * 0.99),
		   calc_tick = current_world_tick()
		 FROM settlements s
		 WHERE sg.settlement_id = s.id
		   AND s.world_id = $1 AND s.owner_id IS NOT NULL AND s.state != 'sunk'
		   AND sg.good_key IN ('grain', 'timber')`,
		worldID,
	); err != nil {
		slog.Error("goods decay failed", "world", worldID, "err", err)
	}

	// Reset invasions_today, update population, and — grain-funded growth —
	// draw the grain cost of whatever growth is actually affordable.
	//
	// Growth model (daily tick):
	//   pop ≥ 100  → proportional: 0.5% base × food-variety multiplier × soft-cap factor
	//                gives a DESIRED new-citizen count; food_variety = 1.0 (base,
	//                first economy.FoodGoods item present — normally grain) +
	//                0.1 per additional distinct FoodGoods item present, capped
	//                at 4 extras (max 1.4), soft_cap = max(0, 1 − pop/30000) →
	//                growth → 0 near 30000.
	//                That desired growth then costs desired_new × grainPerCitizen
	//                grain: if the settled grain stock affords it in full, all of
	//                it is applied and the cost is deducted; if not, growth is
	//                throttled to floor(grain_now / grainPerCitizen) citizens and
	//                grain is drawn down to (near) zero. Growth never grows the
	//                city for grain it doesn't have.
	//   starvation → −0.5% (pop ≥ 100), floor 101 (collapse fires for pop ≤ 100).
	//                Unchanged — no grain is drawn on the starvation path.
	//
	// C-collapse: the floor is 101, not 50. Any settlement that would drop below 101
	// from starvation is held at 101 here; a follow-up query then schedules
	// CollapseSettlement events for all settlements at pop ≤ 100.
	//
	// Single CTE-chained statement (not a bare TX) so the population increment and
	// the grain deduction are computed ONCE from the same snapshot and applied
	// atomically — pop-added always equals grain-drawn/grainPerCitizen, never more.
	//
	// grain_now reads the raw settled() value (uncapped) — the same value the
	// rest of the codebase treats as "available now" before a write clamps it.
	// This matters for catchment differentiation (success criterion #3): the
	// good's storage cap (1000) is a fixed constant unrelated to a catchment's
	// richness, so clamping grain_now to it before pricing growth would make
	// every self-sufficient catchment (however rich) read identically and grow
	// at the identical rate — erasing the very signal geography is supposed to
	// provide. Leaving it uncapped means a poor catchment's genuinely smaller
	// daily accrual can fall short of desired growth's cost (throttling it)
	// while a rich catchment's larger accrual doesn't — see
	// TestApplyDecay_GrainFundedGrowth_GeographyDifferentiates.
	if _, err := h.pool.Exec(ctx,
		`WITH growth_calc AS (
		     SELECT
		         s.id,
		         s.population AS pop,
		         COALESCE(
		             (SELECT settled(sg.amount, sg.rate, sg.calc_tick)
		              FROM settlement_goods sg
		              WHERE sg.settlement_id = s.id AND sg.good_key = 'grain'), 0
		         ) AS grain_now,
		         -- Variety reads economy.FoodGoods — the SAME list
		         -- loyalty/welfare.go's diet-variety threshold reads (S2,
		         -- megaron_plan_foda_konsistens.md: "en lista som båda läser").
		         -- Before S2 this counted fish/oil/wine/livestock but not grain
		         -- (a flat 1.0 base regardless of whether grain was present),
		         -- while welfare.go counted grain but not livestock — the same
		         -- good meant different things depending on who asked. The
		         -- unified count-of-distinct-foods-present minus one (floored at
		         -- 0) reproduces the exact old numbers for every settlement that
		         -- HAS grain (base 1.0 + 0.1 per additional food type, capped at
		         -- the same four extras) — growth only ever applies when
		         -- grain_now > 0 (see the growing flag below), so this is not a
		         -- balance change, only a shared source of truth.
		         (1.0 + 0.1 * GREATEST(0, (
		             SELECT COUNT(*) FROM settlement_goods sg
		             WHERE sg.settlement_id = s.id AND sg.good_key = ANY($4)
		               AND COALESCE(sg.amount, 0) > 0
		         ) - 1)) AS variety,
		         GREATEST(0, 1.0 - s.population::float / 30000.0) AS softcap
		     FROM settlements s
		     WHERE s.world_id = $1 AND s.owner_id IS NOT NULL AND s.state NOT IN ('sunk', 'collapsed')
		 ),
		 resolved AS (
		     SELECT
		         id, pop, grain_now,
		         (grain_now > 0) AS growing,
		         GREATEST(1, ROUND(pop * 0.005 * variety * softcap)) AS desired_new
		     FROM growth_calc
		 ),
		 priced AS (
		     SELECT
		         id, pop, grain_now, growing,
		         CASE
		             WHEN NOT growing THEN 0
		             WHEN grain_now >= desired_new * $2::float THEN desired_new
		             ELSE FLOOR(grain_now / $2::float)
		         END AS actual_new
		     FROM resolved
		 ),
		 final AS (
		     SELECT
		         id, grain_now,
		         GREATEST(101, LEAST(30000,
		             -- Starvation: retain (1 - starvationPopLossRatePerTick) of pop. The
		             -- ::numeric cast keeps this exact numeric ROUND (half away from
		             -- zero) — a bare float8 product would round half-to-even and drift
		             -- by ±1 at pop ≡ 100 (mod 200); verified over pop 101..30000.
		             CASE WHEN growing THEN pop + actual_new ELSE ROUND(pop * (1 - $3::float8)::numeric) END
		         )) AS new_pop,
		         CASE WHEN growing THEN actual_new * $2::float ELSE 0 END AS grain_draw
		     FROM priced
		 ),
		 pop_upd AS (
		     UPDATE settlements s SET
		         invasions_today = 0,
		         population = f.new_pop
		     FROM final f
		     WHERE f.id = s.id
		     RETURNING s.id
		 ),
		 grain_upd AS (
		     UPDATE settlement_goods sg SET
		         amount    = GREATEST(0, f.grain_now - f.grain_draw),
		         calc_tick = current_world_tick()
		     FROM final f
		     WHERE f.grain_draw > 0 AND sg.settlement_id = f.id AND sg.good_key = 'grain'
		     RETURNING sg.settlement_id
		 )
		 SELECT count(*) FROM pop_upd`,
		worldID, grainPerCitizen, starvationPopLossRatePerTick, economy.FoodGoods,
	); err != nil {
		slog.Error("daily decay failed", "world", worldID, "err", err)
	}

	// C-collapse: schedule CollapseSettlement for any settlement that has already
	// reached pop ≤ 100 (e.g. from overmobilisation via Recruit). The bulk UPDATE
	// above floors at 101 so starvation alone won't create new ≤100 cases in one
	// tick, but once pop is already at 101 and starvation fires, the GREATEST(101,…)
	// clips it — meaning starvation settlement-death takes a second tick to manifest.
	// This is acceptable: starvation collapse is a gradual process.
	collapseRows, err := h.pool.Query(ctx,
		`SELECT id FROM settlements
		 WHERE world_id = $1 AND owner_id IS NOT NULL
		   AND state NOT IN ('sunk', 'collapsed')
		   AND population <= 100`,
		worldID,
	)
	if err == nil {
		var collapseIDs []uuid.UUID
		for collapseRows.Next() {
			var sid uuid.UUID
			if collapseRows.Scan(&sid) == nil {
				collapseIDs = append(collapseIDs, sid)
			}
		}
		collapseRows.Close()
		var currentTick int
		_ = h.pool.QueryRow(ctx, `SELECT current_world_tick()`).Scan(&currentTick)
		for _, sid := range collapseIDs {
			if err := h.scheduler.EnqueueTick(ctx, worldID, events.ScheduledCollapseSettlement,
				struct {
					SettlementID uuid.UUID `json:"settlement_id"`
					WorldID      uuid.UUID `json:"world_id"`
					Cause        string    `json:"cause"`
				}{SettlementID: sid, WorldID: worldID, Cause: "starvation"},
				currentTick,
			); err != nil {
				slog.Warn("collapse: could not schedule collapse event",
					"settlement", sid, "err", err)
			}
		}
	}

	// Recompute production for all active settlements: population changed, so
	// labor_pool (and therefore rates) must be updated.
	sidRows, err := h.pool.Query(ctx,
		`SELECT id FROM settlements
		 WHERE world_id = $1 AND owner_id IS NOT NULL AND state NOT IN ('sunk', 'collapsed')`,
		worldID,
	)
	if err == nil {
		var ids []uuid.UUID
		for sidRows.Next() {
			var sid uuid.UUID
			if sidRows.Scan(&sid) == nil {
				ids = append(ids, sid)
			}
		}
		sidRows.Close()
		for _, sid := range ids {
			if err := economy.RecomputeProduction(ctx, h.pool, sid); err != nil {
				slog.Warn("recompute after pop tick failed", "settlement", sid, "err", err)
			}
		}
	}
}

// SubsistenceWarning tiers. Numeric notification levels follow the codebase's
// convention (level ≤2 = urgent, 3 = info; see web notif styling + combat
// notifyUnitLoss): critical is the most severe.
const (
	subsistenceKind = "SubsistenceWarning"

	tierYellow   = "yellow"   // grain net < 0 (any buffer) — a heads-up
	tierRed      = "red"      // net < 0 AND empties within one game-day
	tierCritical = "critical" // grain empty AND population being ground down

	levelYellow   = 3 // info
	levelRed      = 2 // urgent
	levelCritical = 1 // urgent, top priority
)

// applyStarvationWarning is the proactive counterpart to applyStarvation
// (Sparta-forensiken 2026-07-12): the old code only warned via gossip_events —
// a LIMIT-30 minor channel — and stayed SILENT the moment grain hit zero (the
// `settled(...) > 0` gate), i.e. exactly when the collapse began. It now emits
// through the notify hub (NotifyPlayer → persistent notifications feed) in two
// escalating tiers while grain is still positive:
//   - yellow: grain net rate < 0 (regardless of buffer) — "grain is falling".
//   - red:    net < 0 AND it empties within the next tick.
//
// The grain-empty case (population already dropping) is the critical tier and is
// emitted from applySubsistenceCritical below, so it fires EVERY starving day.
//
// Dedupe: an insert is skipped if an UNREAD SubsistenceWarning for the same
// settlement+tier already exists — otherwise a sped-up world (short TICK_SECONDS)
// would re-fire every catch-up day and spam the feed. Once the Wanax reads it,
// a still-holding trend warns again — a reminder, not spam.
func (h *TickHandler) applyStarvationWarning(ctx context.Context, worldID uuid.UUID) {
	if h.hub == nil {
		return
	}
	rows, err := h.pool.Query(ctx,
		`SELECT s.id, s.owner_id, s.name,
		        settled(sg.amount, sg.rate, sg.calc_tick) AS grain_now,
		        sg.rate AS grain_rate
		 FROM settlements s
		 JOIN settlement_goods sg ON sg.settlement_id = s.id AND sg.good_key = 'grain'
		 WHERE s.world_id = $1 AND s.owner_id IS NOT NULL AND s.state != 'sunk'
		   AND sg.rate < 0
		   AND settled(sg.amount, sg.rate, sg.calc_tick) > 0`,
		worldID,
	)
	if err != nil {
		slog.Error("starvation warning tick failed", "world", worldID, "err", err)
		return
	}
	type warn struct {
		id, ownerID uuid.UUID
		name        string
		grainNow    float64
		grainRate   float64
	}
	var list []warn
	for rows.Next() {
		var wr warn
		if err := rows.Scan(&wr.id, &wr.ownerID, &wr.name, &wr.grainNow, &wr.grainRate); err == nil {
			list = append(list, wr)
		}
	}
	rows.Close()

	for _, wr := range list {
		ticksToEmpty := wr.grainNow / -wr.grainRate // grainRate < 0 by the WHERE clause
		netPerTick := wr.grainRate
		ticksLeft := ticksToEmpty

		tier, level := tierYellow, levelYellow
		if ticksToEmpty <= 1 {
			tier, level = tierRed, levelRed
		}
		h.emitSubsistenceWarning(ctx, worldID, wr.ownerID, wr.id, wr.name, tier, level, netPerTick, ticksLeft, 0)
	}
}

// applySubsistenceCritical emits the critical SubsistenceWarning for every owned
// settlement whose grain has hit zero and whose net rate is still negative —
// the case that used to be entirely silent (grain 0 → the old warning's
// `> 0` gate suppressed it while pop was actually collapsing). It runs after
// applyDecay/applyStarvation so the population figure is post-reduction; the
// reported pop_loss is the modelled daily draw (starvationPopLossRatePerTick),
// not a re-read of the write. Fires every starving day (dedupe still applies).
func (h *TickHandler) applySubsistenceCritical(ctx context.Context, worldID uuid.UUID) {
	if h.hub == nil {
		return
	}
	rows, err := h.pool.Query(ctx,
		`SELECT s.id, s.owner_id, s.name, s.population,
		        COALESCE(sg.rate, 0) AS grain_rate
		 FROM settlements s
		 JOIN settlement_goods sg ON sg.settlement_id = s.id AND sg.good_key = 'grain'
		 WHERE s.world_id = $1 AND s.owner_id IS NOT NULL AND s.state != 'sunk'
		   AND sg.rate < 0
		   AND settled(sg.amount, sg.rate, sg.calc_tick) <= 0`,
		worldID,
	)
	if err != nil {
		slog.Error("subsistence critical tick failed", "world", worldID, "err", err)
		return
	}
	type crit struct {
		id, ownerID uuid.UUID
		name        string
		population  int
		grainRate   float64
	}
	var list []crit
	for rows.Next() {
		var c crit
		if err := rows.Scan(&c.id, &c.ownerID, &c.name, &c.population, &c.grainRate); err == nil {
			list = append(list, c)
		}
	}
	rows.Close()

	for _, c := range list {
		netPerTick := c.grainRate
		popLoss := int(float64(c.population) * starvationPopLossRatePerTick)
		h.emitSubsistenceWarning(ctx, worldID, c.ownerID, c.id, c.name, tierCritical, levelCritical, netPerTick, 0, popLoss)
	}
}

// emitSubsistenceWarning inserts one SubsistenceWarning notification for the
// settlement owner via the hub, unless an unread one of the same settlement+tier
// already exists (dedupe). Payload matches the plan: settlement_id, name, tier,
// net_per_tick, ticks_left, pop_loss.
func (h *TickHandler) emitSubsistenceWarning(ctx context.Context, worldID, ownerID, settlementID uuid.UUID, name, tier string, level int, netPerTick, ticksLeft float64, popLoss int) {
	var exists bool
	if err := h.pool.QueryRow(ctx,
		`SELECT EXISTS (
		    SELECT 1 FROM notifications
		    WHERE world_id = $1 AND player_id = $2 AND kind = $3 AND read_at IS NULL
		      AND body_json->>'settlement_id' = $4 AND body_json->>'tier' = $5
		 )`,
		worldID, ownerID, subsistenceKind, settlementID.String(), tier,
	).Scan(&exists); err != nil {
		slog.Error("subsistence warning dedupe check failed", "settlement", settlementID, "err", err)
		return
	}
	if exists {
		return
	}
	payload := map[string]any{
		"settlement_id": settlementID,
		"name":          name,
		"tier":          tier,
		// net_per_tick/ticks_left are the current names (tick == day now, mig 109).
		// net_per_day/days_left are kept alongside with the SAME value — this
		// payload is persisted to `notifications`, and old rows already carry
		// the day-named keys with that meaning. Semantics are frozen (CLAUDE.md
		// "Events"): don't remove the old keys, they'd make old notifications
		// unreadable. Readers prefer *_per_tick/*_ticks and fall back to
		// *_per_day/*_days.
		"net_per_tick": netPerTick,
		"ticks_left":   ticksLeft,
		"net_per_day":  netPerTick,
		"days_left":    ticksLeft,
		"pop_loss":     popLoss,
	}
	_ = h.hub.NotifyPlayer(ctx, worldID, ownerID, subsistenceKind, level, payload)
}

// applyStarvation punishes settlements where grain has hit zero: infantry and
// chariots each lose 5% (minimum 1 unit) per day.
func (h *TickHandler) applyStarvation(ctx context.Context, worldID uuid.UUID) {
	// The standing army lives in the units table (settlements.* army columns
	// retired, SB7). Select starving settlements that have a garrison to attrit,
	// collect them, then attrit + notify (a fresh conn per Exec — don't mutate
	// while iterating the cursor).
	rows, err := h.pool.Query(ctx,
		`SELECT s.id, s.owner_id, s.name FROM settlements s
		 WHERE s.world_id = $1 AND s.owner_id IS NOT NULL AND s.state != 'sunk'
		   AND EXISTS (SELECT 1 FROM units u
		               WHERE u.settlement_id = s.id AND u.status = 'garrison'
		                 AND u.type IN ('spearman','war_chariot') AND u.size > 0)
		   AND COALESCE(
		           (SELECT settled(sg.amount, sg.rate, sg.calc_tick)
		            FROM settlement_goods sg
		            WHERE sg.settlement_id = s.id AND sg.good_key = 'grain'), 0) <= 0`,
		worldID,
	)
	if err != nil {
		slog.Error("starvation tick failed", "world", worldID, "err", err)
		return
	}
	type starving struct {
		id, owner uuid.UUID
		name      string
	}
	var list []starving
	for rows.Next() {
		var s starving
		if err := rows.Scan(&s.id, &s.owner, &s.name); err == nil {
			list = append(list, s)
		}
	}
	rows.Close()

	for _, s := range list {
		// Grain attrition lives in ONE place: combat/upkeep.go applyAttrition. This
		// handler used to attrit here too (−5%, min 1) while upkeep took its flat −10
		// the same day, in the same poll batch, against the same unit — combined
		// ~14–15%/day, some 50% more than upkeep alone, hitting small garrisons hardest.
		// Two mechanics written 19 days apart (70199f3 / 203af2c) that were never
		// cross-checked; nothing anywhere documented the sum as intended. Removed here
		// rather than in upkeep because upkeep owns the complete lifecycle (disband,
		// cargo cascade, UnitAttrition event, owner notification) — this copy had only
		// the size decrement and disbanded silently. Decision: Timothy, 2026-07-25.
		// starvation_attrition_test.go guards the single-source-of-truth.
		// StarvationDamage stays: it is the audit/flavour signal, and the owner-facing
		// warning is applySubsistenceCritical via the notify hub.
		_, _ = h.store.Append(ctx, s.id, events.StreamProvince, "StarvationDamage",
			map[string]any{"reason": "no_food"}, worldID, nil)
		// Owner notice is now the critical SubsistenceWarning (applySubsistenceCritical,
		// via the notify hub) — the old gossip_events line here was the "wrong channel
		// for your own city's status" the Sparta-forensiken flagged (buried in a LIMIT-30
		// minor feed), so it is retired.
		slog.Info("starvation damage applied", "settlement", s.id)
	}
}

// applyDivinePunishment randomly selects and applies one divine punishment.
func (h *TickHandler) applyDivinePunishment(ctx context.Context, settlementID, worldID uuid.UUID) {
	type punishment struct {
		name string
		text string
		sql  string
	}

	punishments := []punishment{
		{
			"chariot_loss",
			"The gods have scattered your war chariots in the night. Chariots have perished.",
			`UPDATE units SET size = GREATEST(0, size - GREATEST(1, size/5)), updated_at = now()
			 WHERE settlement_id = $1 AND status = 'garrison' AND type = 'war_chariot'`,
		},
		{
			"ship_loss",
			"A divine storm has claimed a vessel from your harbour.",
			`UPDATE units SET status = 'disbanded', updated_at = now()
			 WHERE id = (SELECT id FROM units WHERE settlement_id = $1 AND status = 'garrison' AND type = 'galley' ORDER BY size LIMIT 1)`,
		},
		{
			"harvest_failure",
			"The fields lie fallow by divine will. Half your grain stores have rotted.",
			`UPDATE settlement_goods SET
			   amount  = GREATEST(0, settled(amount, rate, calc_tick) * 0.5),
			   calc_tick = current_world_tick()
			 WHERE settlement_id = $1 AND good_key = 'grain'`,
		},
		{
			"garrison_plague",
			"A dark pestilence has moved through the barracks. Many hoplites have fallen.",
			`UPDATE units SET size = GREATEST(0, size - GREATEST(1, size/5)), updated_at = now()
			 WHERE settlement_id = $1 AND status = 'garrison' AND type = 'spearman'`,
		},
	}

	p := punishments[rand.Intn(len(punishments))]
	if _, err := h.pool.Exec(ctx, p.sql, settlementID); err != nil {
		slog.Error("divine punishment failed", "settlement", settlementID, "type", p.name, "err", err)
		return
	}
	// Disband any garrison unit reduced to nothing by the punishment (no-op for
	// the grain-only harvest_failure case). Army lives in units (SB7).
	_, _ = h.pool.Exec(ctx,
		`UPDATE units SET status = 'disbanded', updated_at = now()
		 WHERE settlement_id = $1 AND status = 'garrison' AND size <= 0`, settlementID)

	_, _ = h.store.Append(ctx, settlementID, events.StreamProvince, "DivinePunishment",
		map[string]any{"type": p.name}, worldID, nil)
	h.addDivineGossip(ctx, settlementID, worldID, "divine_wrath", p.text)
	slog.Info("divine punishment applied", "settlement", settlementID, "type", p.name)
}

// applyDivineBlessing randomly selects and applies one divine blessing for settlements
// that maintain high kharis. Mirror of applyDivinePunishment.
func (h *TickHandler) applyDivineBlessing(ctx context.Context, settlementID, worldID uuid.UUID) {
	type blessing struct {
		name string
		text string
		sql  string
	}

	blessings := []blessing{
		{
			"harvest_blessing",
			"The gods smile upon your fields. An abundant harvest fills your granaries.",
			`UPDATE settlement_goods SET
			   amount  = LEAST(cap, settled(amount, rate, calc_tick) * 1.25),
			   calc_tick = current_world_tick()
			 WHERE settlement_id = $1 AND good_key = 'grain'`,
		},
		{
			// Army lives in the units table (SB7) — handled by applyArmyBlessing,
			// not a settlements UPDATE. Empty sql signals the code path below.
			"divine_recruits",
			"Warriors answer a divine call and join your ranks. New hoplites have arrived.",
			"",
		},
		{
			"sea_blessing",
			"Poseidon guides a vessel to your harbour. A trireme joins your fleet.",
			"",
		},
	}

	b := blessings[rand.Intn(len(blessings))]
	// Poseidon does not beach triremes inland: sea_blessing requires a coastal
	// settlement (a sea tile among the province's six neighbours — coast is a
	// neighbourhood property, not a terrain). Inland cities get divine recruits
	// instead. Before this gate, blessing-ships spawned landlocked (Sparta,
	// 2026-07-13) and could never depart — the march handler 422s on "no
	// adjacent sea hex".
	if b.name == "sea_blessing" {
		// Coastal adjacency, NOT catchment — this checks the settlement's
		// immediate 6 neighbours regardless of hexgrid.CatchmentRadius (P1
		// doubled the catchment ring, but "does the coast touch this city"
		// stays a radius-1 question). Offsets sourced from hexgrid.Neighbors
		// (relative to Coord{}, i.e. the raw offsets) instead of a hardcoded
		// tuple list.
		neighbors := hexgrid.Neighbors(hexgrid.Coord{})
		offQ, offR := hexgrid.QRArrays(neighbors[:])
		var coastal bool
		if err := h.pool.QueryRow(ctx,
			`SELECT EXISTS (
			   SELECT 1
			   FROM settlements s
			   JOIN provinces p ON p.id = s.province_id
			   JOIN unnest($3::int[], $4::int[]) AS off(dq, dr) ON true
			   JOIN map_tiles t ON t.world_id = $2 AND t.q = p.map_q + off.dq AND t.r = p.map_r + off.dr
			   WHERE s.id = $1 AND t.terrain IN ('coastal_sea','deep_sea','river','river_ford'))`,
			settlementID, worldID, offQ, offR,
		).Scan(&coastal); err != nil || !coastal {
			for i := range blessings {
				if blessings[i].name == "divine_recruits" {
					b = blessings[i]
					break
				}
			}
		}
	}
	if b.sql != "" {
		if _, err := h.pool.Exec(ctx, b.sql, settlementID); err != nil {
			slog.Error("divine blessing failed", "settlement", settlementID, "type", b.name, "err", err)
			return
		}
	} else if err := h.applyArmyBlessing(ctx, settlementID, worldID, b.name); err != nil {
		slog.Error("divine army blessing failed", "settlement", settlementID, "type", b.name, "err", err)
		return
	}

	_, _ = h.store.Append(ctx, settlementID, events.StreamProvince, "DivineBlessing",
		map[string]any{"type": b.name}, worldID, nil)
	h.addDivineGossip(ctx, settlementID, worldID, "divine_favour", b.text)
	slog.Info("divine blessing applied", "settlement", settlementID, "type", b.name)
}

// applyArmyBlessing grants the army-boosting divine blessings against the units
// table (SB7: the army lives in units, not the retired settlements.* columns).
func (h *TickHandler) applyArmyBlessing(ctx context.Context, settlementID, worldID uuid.UUID, name string) error {
	switch name {
	case "divine_recruits":
		// Reinforce the strongest garrison spearman unit (min +2); if the
		// settlement has none, form a fresh small garrison.
		//
		// LEAST(economy.MaxUnitSize, …): this grew by 20 % with no ceiling until
		// 2026-07-23, and because it always picks the LARGEST garrison spearman it
		// reinforced the same unit every time — compounding. Five units had reached
		// 1.86–2.13 billion men, saturated against the int32 ceiling, and one of
		// them founded a colony that minted 99.5 % of the world's silver. A
		// blessing may fill a unit to the recruitment ceiling; it may not exceed it.
		tag, err := h.pool.Exec(ctx,
			`UPDATE units SET size = LEAST($2, size + GREATEST(2, size/5)), updated_at = now()
			 WHERE id = (SELECT id FROM units
			             WHERE settlement_id = $1 AND status = 'garrison' AND type = 'spearman'
			             ORDER BY size DESC LIMIT 1)`,
			settlementID, economy.MaxUnitSize)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return h.insertGarrisonUnit(ctx, settlementID, worldID, "spearman", "land", 2, 0)
		}
		return nil
	case "sea_blessing":
		return h.insertGarrisonUnit(ctx, settlementID, worldID, "galley", "naval", 1, 20)
	}
	return nil
}

// insertGarrisonUnit forms a new garrison unit for the settlement's owner.
func (h *TickHandler) insertGarrisonUnit(ctx context.Context, settlementID, worldID uuid.UUID, utype, category string, size, crew int) error {
	var ownerID *uuid.UUID
	if err := h.pool.QueryRow(ctx,
		`SELECT owner_id FROM settlements WHERE id = $1`, settlementID,
	).Scan(&ownerID); err != nil {
		return err
	}
	if ownerID == nil {
		return nil // ownerless settlement — no one to receive the unit
	}
	ordinal, err := unit.AllocateOrdinal(ctx, h.pool, settlementID, utype)
	if err != nil {
		return fmt.Errorf("divine recruit ordinal: %w", err)
	}
	_, err = h.pool.Exec(ctx,
		`INSERT INTO units (world_id, owner_id, type, category, size, crew, status,
		                    settlement_id, support_settlement_id, ordinal)
		 VALUES ($1, $2, $3, $4, $5, $6, 'garrison', $7, $7, $8)`,
		worldID, *ownerID, utype, category, size, crew, settlementID, ordinal)
	return err
}

// generateOmen produces an atmospheric temple omen (20% chance per maintained day).
// Omens are written to the gossip stream and appear in the player's Rumours panel.
func (h *TickHandler) generateOmen(ctx context.Context, settlementID, worldID uuid.UUID) {
	omens := []string{
		"The heart of the offering lay clean and red. The gods are pleased.",
		"Smoke rose straight toward heaven — a season of calm and steady winds.",
		"The sacred birds ate freely from the offered grain. The harvest will be generous.",
		"The flame consumed the offering without hesitation. Order holds for now.",
		"A serpent crossed the temple threshold and departed unharmed. Old powers watch this place.",
		"Birds flew westward in tight formation. Something stirs beyond your sight.",
		"The offering was pale but the liver whole. The gods withhold judgement.",
		"Clouds gathered during the rite, then passed without rain. The future is contested.",
		"The sacred flame guttered three times before catching. Patience is called for.",
		"A dark mark appeared near the gate of the liver — a shadow at the threshold.",
		"The birds fell silent for a long time before resuming their cries. The gods listen.",
		"Wind shifted against the smoke during the final prayer. Something turns.",
		"Two ravens circled the altar three times. The gods debate.",
		"The entrails were tangled — an augur's nightmare. Ambiguity rules this season.",
		"A child laughed outside the temple during the rite. The gods find something amusing.",
	}
	text := omens[rand.Intn(len(omens))]
	h.addDivineGossip(ctx, settlementID, worldID, "omen", text)
}

// addDivineGossip inserts a gossip event for the owner of the given settlement.
func (h *TickHandler) addDivineGossip(ctx context.Context, settlementID, worldID uuid.UUID, category, text string) {
	var ownerID *uuid.UUID
	var name string
	_ = h.pool.QueryRow(ctx,
		`SELECT owner_id, name FROM settlements WHERE id = $1`,
		settlementID,
	).Scan(&ownerID, &name)
	if ownerID == nil {
		return
	}
	_, _ = h.pool.Exec(ctx,
		`INSERT INTO gossip_events (world_id, recipient_id, source_region, category, text)
		 VALUES ($1, $2, $3, $4, $5)`,
		worldID, *ownerID, name, category, text,
	)
}

// accumulatePrestige adds daily prestige to the world based on active cult devotion.
// One point per active (non-Wrathful) settlement, plus a tier bonus (vardig+1,
// overdadig+2 — strawman, rescaled for the 4-tier mood table, FAS 0).
// Prestige feeds into the collapse risk algorithm.
func (h *TickHandler) accumulatePrestige(ctx context.Context, worldID uuid.UUID) {
	// Prestige is driven by cult level — now lives on player_world_records.
	_, err := h.pool.Exec(ctx,
		`UPDATE worlds SET prestige = prestige + (
		    SELECT COALESCE(SUM(
		        1 + CASE pwr.cult_level
		            WHEN 'vardig'    THEN 1
		            WHEN 'overdadig' THEN 2
		            ELSE 0
		        END
		    ), 0)
		    FROM player_world_records pwr
		    WHERE pwr.world_id = $1 AND pwr.cult_level != 'vredgad'
		)
		WHERE id = $1`,
		worldID,
	)
	if err != nil {
		slog.Error("prestige accumulation failed", "world", worldID, "err", err)
	}
}

// templeOfferScarcity values the standing temple offering (oil + wine) at the
// gods' current reckoning against its plain base value. 1.0 in a year of plenty;
// above it when the world is short of what the altars burn.
//
// This is the same principle the composed prayer offering uses, applied to the
// standing cult — Timothy 2026-07-22: "samma sak gäller vid kult och stående
// gudstjänster". Bounded: a shortage may double what devotion earns, never more,
// so a scarcity spike cannot mint kharis.
func (h *TickHandler) templeOfferScarcity(ctx context.Context, worldID uuid.UUID) float64 {
	values, err := religion.LoadDivineValues(ctx, h.pool, worldID)
	if err != nil || len(values) == 0 {
		return 1.0 // never let a missing price list change the day's outcome
	}
	offering := map[string]float64{"oil": OfferOilPerTemple, "wine": OfferWinePerTemple}

	var divine, base float64
	for good, amount := range offering {
		divine += amount * values[good]
		base += amount * baseValues[good]
	}
	if base <= 0 || divine <= 0 {
		return 1.0
	}
	return clampTempleScarcity(divine / base)
}

// clampTempleScarcity bounds what a shortage can be worth to a temple. Pure, so
// the bound is testable without a rig: abundance never PUNISHES a tended temple
// (it simply stops paying extra), and a scarcity spike can never mint kharis.
func clampTempleScarcity(factor float64) float64 {
	if factor < 1.0 {
		return 1.0
	}
	if factor > templeScarcityCeil {
		return templeScarcityCeil
	}
	return factor
}

// baseValues mirrors the goods catalogue for the two goods the altars burn.
// Read from the catalogue would mean a query per Wanax per day for two constants
// that have not moved since migration 008.
var baseValues = map[string]float64{"oil": 4, "wine": 5}

// templeScarcityCeil bounds what a shortage can be worth to a temple.
const templeScarcityCeil = 2.0

// feedTempleBySubstitution feeds a temple that cannot afford the traditional
// oil+wine with whatever else its city holds, to the same divine worth.
//
// The gods are not quartermasters: what matters is that the altar received a gift
// worth what it is due, not that the gift arrived in the customary jars. Valued
// with the same reckoning a composed prayer offering uses, so the two halves of
// the cult cannot disagree about what a thing is worth.
//
// Returns true when the temple was fed. Deliberately silent on failure: a city
// with nothing to spare simply goes unfed, which is the existing behaviour.
func (h *TickHandler) feedTempleBySubstitution(ctx context.Context, settlementID, worldID uuid.UUID) bool {
	values, err := religion.LoadDivineValues(ctx, h.pool, worldID)
	if err != nil || len(values) == 0 {
		return false
	}
	due := OfferOilPerTemple*values["oil"] + OfferWinePerTemple*values["wine"]
	if due <= 0 {
		return false
	}

	// What the city can spare, dearest to the gods first — an altar fed with
	// something precious needs less of it, and the city keeps more of its bulk.
	rows, err := h.pool.Query(ctx,
		`SELECT sg.good_key, GREATEST(0, settled(sg.amount, sg.rate, sg.calc_tick))
		 FROM settlement_goods sg
		 WHERE sg.settlement_id = $1 AND sg.good_key <> 'silver'
		   AND GREATEST(0, settled(sg.amount, sg.rate, sg.calc_tick)) > 0`,
		settlementID)
	if err != nil {
		return false
	}
	type stock struct {
		key    string
		amount float64
		value  float64
	}
	var available []stock
	for rows.Next() {
		var st stock
		if rows.Scan(&st.key, &st.amount) == nil {
			st.value = values[st.key]
			if st.value > 0 {
				available = append(available, st)
			}
		}
	}
	rows.Close()
	if len(available) == 0 {
		return false
	}
	sort.Slice(available, func(i, j int) bool { return available[i].value > available[j].value })

	// Plan the whole gift before touching anything: a half-paid offering would
	// take goods and feed nothing.
	paid := 0.0
	take := map[string]float64{}
	for _, st := range available {
		if paid >= due {
			break
		}
		want := (due - paid) / st.value
		if want > st.amount {
			want = st.amount
		}
		if want <= 0 {
			continue
		}
		take[st.key] = want
		paid += want * st.value
	}
	if paid < due {
		return false // the city genuinely has nothing worth the altar today
	}

	for good, amount := range take {
		if _, err := h.pool.Exec(ctx,
			`UPDATE settlement_goods SET
			   amount    = GREATEST(0, settled(amount, rate, calc_tick) - $2),
			   calc_tick = current_world_tick()
			 WHERE settlement_id = $1 AND good_key = $3`,
			settlementID, amount, good,
		); err != nil {
			slog.Error("substituted temple offering deduction failed",
				"settlement", settlementID, "good", good, "err", err)
			return false
		}
	}
	return true
}
