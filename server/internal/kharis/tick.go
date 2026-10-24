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

	"formatet/megaron/server/internal/ai"
	"formatet/megaron/server/internal/economy"
	"formatet/megaron/server/internal/events"
	"formatet/megaron/server/internal/religion"
	"formatet/megaron/server/internal/tick"
	"formatet/megaron/server/internal/unit"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Kharis omdesign (Timothy 2026-07-09, temenos_kharis.md §"KANONISK OMDESIGN"):
// the pool moved from a 0–2000 scale to a single hidden 0–100 number. All
// thresholds below are STRAWMAN — balance is calibrated later
// (temenos_balans_spakar.md §9). See megaron_kharis_plan.md FAS 0.
const (
	blessThreshold   = 60.0 // kharis above this may attract divine favour (was 200/2000)
	blessProbability = 0.15 // 15% chance of divine blessing per maintained day above threshold
)

// StartingKharis is the founding value and no-temple ceiling (Timothy 2026-10-18).
// Existing Wanaxes are not lifted to this value.
const StartingKharis = 40.0

// PunishmentProbability is independent of temple presence (R5).
// Protection after an actual punishment is checked separately against realm state.
func PunishmentProbability(kharis float64, _ bool) float64 {
	if kharis >= WrathThreshold {
		return 0
	}
	return WrathDailyProbability
}

// KharisPerTempleWorker is the one gain coefficient per placed temple worker.
// Timothy 2026-10-22: two workers keep full L1 gain at 2.1 before modifiers.
const KharisPerTempleWorker = 1.05

// Cult upkeep covers residents and every city beyond the first.
// A temple supplies gain; it never exempts its city or residents from upkeep.
const decayBas = 0.1

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
const kharisFloor = religion.RealmKharisFloor

// Floor is the minimum realm Kharis, including a Host before founding.
const Floor = kharisFloor

// growthRatePerTick is the most a fed city grows in one tick, as a fraction of
// its population, before the food-variety multiplier (the wall sets the ceiling, no soft cap).
//
// Tillväxten (Timothy 2026-09-26, megaron_styrande_beslut): growth costs NO
// food. A city grows only when it has an ÖVERFLÖD — economy.FoodNet > 0, it
// produces more food per tick than its population eats — and then always at
// this full rate × variety up to the wall ceiling, never scaled to the size of the
// surplus. The surplus itself stays in the stores. This replaces the
// grain-priced model (grainPerCitizen 2,0 + growthGrainReserve 6,94, 2026-08-19
// "surplus → growth, not storage"), under which growth spent every grain above
// the reserve and the stores never filled.
const growthRatePerTick = 0.005

// starvationPopLossRatePerTick is the fraction of population a starving city
// loses per tick (−0.5%/tick). Single source of truth for BOTH sides:
// applyDecay's SQL binds it as a parameter — ROUND(pop * (1 - $3)::numeric) —
// to write the real pop mutation, and applyStarvationWarning multiplies by it
// to report the modelled pop_loss in the critical SubsistenceWarning payload.
// Retune here and both move together. (Reported loss stays an estimate: the
// SQL rounds survivors (at least one lost), the warning truncates the loss —
// same rate, not same delta.)
const starvationPopLossRatePerTick = 0.005

// TickHandler applies daily temple maintenance to all active settlements in a world.
type TickHandler struct {
	pool              *pgxpool.Pool
	scheduler         *events.Scheduler
	store             *events.Store
	hub               Broadcaster
	randomFloat       func() float64
	storageForecaster StorageForecaster
}

// NewTickHandler creates a TickHandler. hub may be nil (tests) — every
// NotifyPlayer call is nil-guarded, matching the other tick/upkeep handlers.
func NewTickHandler(pool *pgxpool.Pool, sched *events.Scheduler, store *events.Store, hub Broadcaster) *TickHandler {
	return &TickHandler{pool: pool, scheduler: sched, store: store, hub: hub, randomFloat: rand.Float64}
}

// wanaxSnap holds the per-Wanax state needed for daily temple maintenance.
// Kharis lives on player_world_records; devotionSum aggregates temple staffing across all
// of the player's settlements in this world.
type wanaxSnap struct {
	playerID     uuid.UUID
	settlementID uuid.UUID // capital settlement (for event emission and divine effects)
	kharis       float64
	kharisCap    float64
	// Actual placed temple workers, bounded by standing temple capacity.
	devotionSum    float64
	templeCities   int
	maxTempleLevel int // grandest temple's level — sets the kharis ceiling
	cityCount      int // all owned active cities, including the capital
	population     int // settlements.population in those same cities
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
            0::float8 AS devotion_sum,
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
		          AND s3.state NOT IN ('sunk', 'collapsed', 'razed')
		    ), 0) AS city_count,
            COALESCE((SELECT SUM(s5.population) FROM settlements s5
              WHERE s5.owner_id=pwr.player_id AND s5.world_id=pwr.world_id
                AND s5.state NOT IN ('sunk','collapsed','razed')),0) AS population
		 FROM player_world_records pwr
		 JOIN settlements s ON s.owner_id = pwr.player_id AND s.world_id = pwr.world_id AND s.is_capital = true
		 WHERE pwr.world_id = $1
         ORDER BY pwr.player_id`,
		e.WorldID,
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
			&w.maxTempleLevel, &w.cityCount, &w.population); err == nil {
			// The grandest temple binds the ceiling from here on — processMaintenance
			// and everything downstream sees one already-resolved cap.
			w.kharisCap = EffectiveKharisCap(w.kharisCap, w.maxTempleLevel)
			snaps = append(snaps, w)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	rows.Close()
	for _, w := range snaps {
		staffed, _, err := economy.RealmTempleStaffing(ctx, h.pool, w.playerID, e.WorldID)
		if err != nil {
			return fmt.Errorf("load temple staffing: %w", err)
		}
		w.devotionSum = float64(staffed)
		if err := h.processMaintenance(ctx, w, e.WorldID, e.ID); err != nil {
			// Observation is an auxiliary projection: retain its atomic baseline
			// for the next tick, while continuing maintenance for other Wanaxes.
			slog.Error("kharis maintenance failed", "player", w.playerID, "world", e.WorldID, "err", err)
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

	h.applyDecay(ctx, e.WorldID, e.ID)
	h.applyStarvationWarning(ctx, e.WorldID)
	if err := h.applyStarvation(ctx, e.WorldID); err != nil {
		// A notification outage must never stop the world's recurring tick.
		slog.Error("starvation observation failed", "world", e.WorldID, "err", err)
	}
	h.applySubsistenceCritical(ctx, e.WorldID)
	if err := h.observeStorage(ctx, e.WorldID); err != nil {
		slog.Error("storage observation failed", "world", e.WorldID, "err", err)
	}

	return h.scheduler.EnqueueTickRecurring(ctx, e.WorldID, events.ScheduledKharisTick,
		struct{}{}, e.DueTick, events.MacroTickInterval)
}

// computeDailyDecay is cult upkeep: base + additional cities + residents.
// Residents are settlements.population, not rounded workers or army size.
func computeDailyDecay(cityCount, population int) float64 {
	return decayBas + cityDecay(cityCount) + populationDecay(population)
}
func cityDecay(cityCount int) float64        { return .2 * float64(max(0, cityCount-1)) }
func populationDecay(population int) float64 { return .1 * float64(max(0, population)) / 1000 }

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
// temple. Level 0 (no temple) holds the founding value 40 — such a
// Wanax decays toward kharisFloor anyway, since devotion with no altar is 0.
func TempleKharisCeiling(maxTempleLevel int) float64 {
	if maxTempleLevel <= 0 {
		return StartingKharis
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

// maintenanceStanding preserves existing standing above a lowered temple
// ceiling. Only ordinary burden acts there; offerings cannot offset it. Once
// standing crosses below the ceiling, normal netted production resumes.
func maintenanceStanding(current, gain, burden, cap float64) float64 {
	if current > cap {
		return max(kharisFloor, current-burden)
	}
	return clampKharis(current+gain-burden, cap)
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

// applyTempleOffering charges every temple its own gift, city by city, and
// returns how many temples were fed and how many workers serve those temples.
func (h *TickHandler) applyTempleOffering(ctx context.Context, playerID, worldID uuid.UUID) (fed, total int, fedWorkers float64) {
	values, err := religion.LoadDivineValues(ctx, h.pool, worldID)
	if err != nil {
		slog.Error("temple offering valuations", "err", err)
		return 0, 0, 0
	}
	temples, err := economy.RealmTemples(ctx, h.pool, playerID, worldID)
	if err != nil {
		slog.Error("temple offering staffing", "err", err)
		return 0, 0, 0
	}
	workers := map[uuid.UUID]int{}
	for _, t := range temples {
		workers[t.BuildingID] = t.Staffed
	}
	offers, err := loadTempleOffers(ctx, h.pool, playerID, worldID, values)
	if err != nil {
		slog.Error("temple offering cities", "err", err)
		return 0, 0, 0
	}
	total = len(offers)
	charged := map[uuid.UUID]bool{}
	for _, offer := range offers {
		if charged[offer.SettlementID] {
			continue
		}
		charged[offer.SettlementID] = true
		paid, err := h.chargeTempleOffering(ctx, offer.SettlementID, values)
		if err != nil {
			slog.Error("temple offering debit", "settlement", offer.SettlementID, "err", err)
			continue
		}
		for id := range paid {
			fed++
			fedWorkers += float64(workers[id])
		}
	}
	return fed, total, fedWorkers
}

func (h *TickHandler) processMaintenance(ctx context.Context, w wanaxSnap, worldID uuid.UUID, eventID int64) error {
	if err := h.rememberInitialBand(ctx, w, worldID); err != nil {
		return err
	}
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
		return h.observeKharisBand(ctx, w, worldID) // maintenance already applied; retry durable observation
	}

	// A day is "maintained" when the Wanax holds at least one standing temple;
	// that controls gain and eligibility for spontaneous blessings.
	maintained := w.templeCities > 0

	// FAS 3 — offer-underhåll: a maintained day's cult-gain is scaled by how
	// many of the Wanax's temples were actually fed a material offer today.
	// Full offer everywhere -> full gain; no offer anywhere -> zero gain (the
	// dailyDecay below then wins outright regardless of cult labor). Cult-labor
	// still never costs kharis — only goods+labor. Offering is only attempted
	// on a maintained day: gain is 0×anything otherwise, so there's no point
	// draining a Wanax's local goods for a day that already produces no gain.
	var gain float64
	var offerFed, offerTotal int
	var offerFraction float64
	if maintained {
		// Devotion counts actual workers serving the temple — read, never produced
		// (mig 094). A temple with no one tending it is a building, not a cult.
		devotionGain := w.devotionSum * KharisPerTempleWorker
		var fedWorkers float64
		offerFed, offerTotal, fedWorkers = h.applyTempleOffering(ctx, w.playerID, worldID)
		offerFraction = computeOfferFraction(offerFed, offerTotal)

		// The gods reckon the offering by what the WORLD can spare, not by a
		// fixed price (Timothy 2026-07-22: "samma sak gäller vid kult och
		// stående gudstjänster"). Feeding a temple grain during a grain shortage is
		// a greater gift than feeding it grain in a year of plenty, and earns more.
		scarcity := h.templeOfferScarcity(ctx, worldID)
		// Each temple's workers count only when that temple was fed (ORDER 15).
		gain = fedWorkers * KharisPerTempleWorker * scarcity

		_, _ = h.store.Append(ctx, w.settlementID, events.StreamProvince, "KharisOffering",
			map[string]any{
				"temples_fed":         offerFed,
				"temples_total":       offerTotal,
				"offer_fraction":      offerFraction,
				"temple_workers":      w.devotionSum,
				"fed_temple_workers":  fedWorkers,
				"scarcity_factor":     scarcity,
				"valuation_version":   religion.LaborValuationVersion,
				"offering_value_unit": religion.LaborValueUnit,
				"gain_before_offer":   devotionGain,
				"kharis_gain":         gain,
			},
			worldID, nil)
	}

	dailyDecay := computeDailyDecay(w.cityCount, w.population)
	newKharis := maintenanceStanding(w.kharis, gain, dailyDecay, w.kharisCap)

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
		// doc comment and placement role, not a settlement_goods stock to
		// consume.)
		_, _ = h.store.Append(ctx, w.settlementID, events.StreamProvince, "KharisMaintained",
			map[string]any{
				"temple_workers": w.devotionSum,
				"kharis_gain":    gain,
				"daily_decay":    dailyDecay,
				"net":            gain - dailyDecay,
				// Legacy gain/net describe available cult output. Add the
				// committed outcome separately; older rows keep their meaning.
				"standing_before":      w.kharis,
				"standing_after":       newKharis,
				"applied_net":          newKharis - w.kharis,
				"effective_ceiling":    w.kharisCap,
				"above_ceiling":        w.kharis > w.kharisCap,
				"ceiling_rule_version": "retained_standing_v1",
				"city_count":           w.cityCount,
				"realm_population":     w.population,
				"decay_rule_version":   "cult_population_v1",
				"rule_version":         "temple_workers_v2",
				"offer_fraction":       offerFraction,
				"temples_fed":          offerFed,
				"temples_total":        offerTotal,
			},
			worldID, nil)
		if newKharis >= blessThreshold && rand.Float64() < blessProbability {
			h.applyDivineBlessing(ctx, w.settlementID, worldID, w.playerID)
		}
		if rand.Float64() < 0.20 {
			h.generateOmen(ctx, w.settlementID, worldID)
		}
	} else {
		// No temple production this day — dailyDecay above is the only kharis
		// change (gain=0). This event still means no temple production;
		// it does not claim that punishment happened.
		_, _ = h.store.Append(ctx, w.settlementID, events.StreamProvince, "KharisMissedMaintenance",
			map[string]any{
				"reason":             "no_cult_production",
				"daily_decay":        dailyDecay,
				"city_count":         w.cityCount,
				"realm_population":   w.population,
				"decay_rule_version": "cult_population_v1",
				"rule_version":       "temple_workers_v2",
				"founding_grace":     false, // legacy outcome field: no grace under this rule
			},
			worldID, nil)
	}

	// The roll uses the post-maintenance value.
	wrath, err := LoadWrathStatus(ctx, h.pool, w.playerID, worldID)
	if err != nil {
		return err
	}
	if probability := PunishmentProbability(newKharis, maintained); probability > 0 && wrath.ProtectionRemainingDays == 0 && h.randomFloat() < probability {
		if err := h.applyDivinePunishment(ctx, w.settlementID, worldID, w.playerID); err != nil {
			return err
		}
	}

	// 3. Derive mood and write back cult_level (drives display).
	derived := deriveMood(newKharis)
	_, _ = h.pool.Exec(ctx,
		`UPDATE player_world_records SET cult_level = $1
		 WHERE player_id = $2 AND world_id = $3`,
		derived, w.playerID, worldID,
	)
	return h.observeKharisBand(ctx, w, worldID)
}

// deriveMood maps the 0–100 kharis level to a mood label (replaces player-set
// cult_level). The four tiers are religion.MoodFavorable/MoodIndifferent/
// MoodSuspicious (60/30/5) — the single canonical threshold table, shared with
// api/handlers.kharisToMood (web.go) and religion.PrayerSpec.MinKharis
// (megaron_plan_kultbrunnen.md §6, 2026-08-25 — the lowest tier was 10 here
// while the lowest prayer gate was 5; both now read the same constant). Rite
// success (settlement.go riteSuccessChance) does NOT read this table — it has
// been a continuous kharis/100 formula since FAS 1, no tier lookup. Swedish
// labels for the two lower tiers ("tveksam"/"vredgad") are strawman coinages —
// the design doc only names the English mood words for those.
func deriveMood(kharis float64) string {
	switch {
	case kharis >= religion.MoodFavorable:
		return "overdadig" // Favorable
	case kharis >= religion.MoodIndifferent:
		return "vardig" // Indifferent
	case kharis >= religion.MoodSuspicious:
		return "tveksam" // Suspicious
	default:
		return "vredgad" // Wrathful
	}
}

// applyDecay applies 1% daily decay to grain and timber stocks, resets
// invasions_today, and adjusts population. (Rite success is driven by Kharis
// mood, not a stored strength stat — there is nothing per-temple to regenerate.)
//
// eventID (G2, migration 098's processed_tick_claims) scopes exactly-once
// claims over the two accumulator effects here — same idea as
// processMaintenance's per-player claim:
//
//   - the flat ×0.99 grain/timber decay below, claimed WORLD-scoped
//     (hash(worldID, "goods_decay")); and
//   - the growth/grain-draw CTE further down, claimed per SETTLEMENT.
//
// Both move state on every call, so a worker retry of the SAME
// ScheduledKharisTick event (crash/G2-timeout between commit and markDone,
// events.Worker re-running Handle from the top) would otherwise shave the
// decay twice and grow/draw twice for a day that never happened. The two
// claims use different scope keys and so never collide, letting a partial
// replay (crash after the decay commits, before growth) redo exactly the
// half that didn't land. Both were found in the G2 idempotency sweep; the
// decay half was left unclaimed then and closed here (the growth half and the
// sibling processMaintenance/ColonyPenaltyHandler were fixed under migration
// 098). The recompute/collapse steps below are NOT claimed — they are
// read-driven off already-committed state, not accumulators.
func (h *TickHandler) applyDecay(ctx context.Context, worldID uuid.UUID, eventID int64) {
	// Decay grain and timber by 1% per day. Population grain-consumption is NOT
	// applied here anymore: it is folded into grain's net rate in
	// economy.RecomputeProduction (continuous per-tick draw), so it never exceeds
	// the grain cap and a self-sufficient city holds a stable positive stock.
	// Cedar is a luxury store-of-value (ädelträ) and does not rot.
	//
	// G2 exactly-once: fold a world-scoped claim into the decay statement so a
	// worker replay of the SAME ScheduledKharisTick event cannot shave the ×0.99
	// twice. Scope is a hash of the world id, NOT a settlement id, so this claim
	// can never collide with the per-settlement growth claim below — the same
	// derived-scope convention upkeep.go's silver_audit and borrowed_army.go use
	// when one event needs more than one independent claim. On a replay the INSERT
	// hits ON CONFLICT, `claim` returns no row, the EXISTS guard is false, and the
	// UPDATE matches zero rows — a no-op, not a second 1% shave. Data-modifying
	// WITH clauses run exactly once regardless of the main query, so the claim is
	// always attempted; the UPDATE only follows on the run that wins the claim.
	decayScope := uuid.NewSHA1(worldID, []byte("goods_decay"))
	if _, err := h.pool.Exec(ctx,
		`WITH claim AS (
		     INSERT INTO processed_tick_claims (event_id, scope_id)
		     VALUES ($2, $3) ON CONFLICT DO NOTHING
		     RETURNING scope_id
		 )
		 UPDATE settlement_goods sg SET
		   amount = GREATEST(0, settled(sg.amount, sg.rate, sg.calc_tick) * 0.99),
		   calc_tick = current_world_tick()
		 FROM settlements s
		 WHERE sg.settlement_id = s.id
		   AND s.world_id = $1 AND s.owner_id IS NOT NULL AND s.state != 'sunk'
		   AND sg.good_key IN ('grain', 'timber')
		   AND EXISTS (SELECT 1 FROM claim)`,
		worldID, eventID, decayScope,
	); err != nil {
		slog.Error("goods decay failed", "world", worldID, "err", err)
	}

	// Reset invasions_today and update population.
	//
	// Growth model (daily tick, Timothy 2026-09-26 — see growthRatePerTick):
	//   starving  (food_unmet_amount > 0, written by FoodTick at 55)
	//             → −0.5% (starvationPopLossRatePerTick), at least one person
	//               per tick; no floor — collapse fires below
	//               economy.CollapseBelowPopulation.
	//   fed, food_net ≤ 0 → population holds. The city eats what it makes (or
	//               lives off its stores) — no överflöd, no growth.
	//   fed, food_net > 0 → + GREATEST(1, ROUND(pop × growthRatePerTick ×
	//               variety)). variety = 1.0 + 0.1 per additional
	//               distinct food with at least economy.VarietyMinShare of this
	//               tick's actual meal (cap 1.4);
	//               (no soft cap, Timothy 2026-10-21: full rate up to the ceiling, then stop),
	//               clamped to cap − pop; cap = economy.PopulationCap(wall_level) — the wall is the ceiling. At or over the cap:
	//               pop holds (no growth, never a loss). Costs no food.
	//
	// food_net is economy.FoodNet written in SQL: Σ rate over
	// economy.FoodGoods minus pop × GrainConsumptionPerCitizenPerTick. The
	// surfaces' food_net_per_tick (api/handlers/province.go) reads the same
	// function, so "+X food/day" on screen and "this city grows" agree.
	// TestApplyDecay_GrowthGateMatchesFoodNet pins the two against each other.
	//
	// C-collapse: there is no population floor here (Timothy 2026-10-10). Until
	// then GREATEST(101, …) lifted every city to 101 before the collapse query
	// ran, so starvation could never kill one. The follow-up query schedules
	// CollapseSettlement for settlements below economy.CollapseBelowPopulation.
	growthRows, err := h.pool.Query(ctx,
		`WITH claim AS (
		     -- G2 exactly-once claim (migration 098): one row per (event_id,
		     -- settlement_id). A replay of the same event finds every eligible
		     -- settlement already claimed, ON CONFLICT DO NOTHING returns nothing
		     -- for it, and growth_calc's JOIN below excludes it — a no-op, not a
		     -- second growth. A later day's tick carries a different
		     -- event_id and claims fresh.
		     INSERT INTO processed_tick_claims (event_id, scope_id)
		     SELECT $5::bigint, s.id FROM settlements s
		     WHERE s.world_id = $1 AND s.owner_id IS NOT NULL AND s.state NOT IN ('sunk', 'collapsed')
		     ON CONFLICT DO NOTHING
		     RETURNING scope_id
		 ),
		 growth_calc AS (
		     SELECT
		         s.id,
		         s.population AS pop,
		         -- Utfodringsordningen D4 (megaron_plan_utfodringsordningen.md,
		         -- 2026-08-26): whether the population actually ate today, written
		         -- by FoodTick (priority 55, runs before this at 60).
		         s.food_unmet_amount AS unmet,
		         -- economy.FoodNet: food produced per tick minus what the
		         -- population eats per tick.
		         (SELECT COALESCE(SUM(sg.rate), 0) FROM settlement_goods sg
		          WHERE sg.settlement_id = s.id AND sg.good_key = ANY($6))
		           - s.population * $2::float AS food_net,
		         -- Actual FoodTick outcome, not stores left after the meal.
		         (1.0 + 0.1 * LEAST(4,GREATEST(0, (
		             SELECT COUNT(*) FROM jsonb_each_text(s.food_consumed) AS meal(good,qty)
		             WHERE (meal.good=ANY($4) OR meal.good='livestock') AND meal.qty::numeric>0
		               AND meal.qty::numeric >= $8::numeric * (
		                   SELECT SUM(part.qty::numeric) FROM jsonb_each_text(s.food_consumed) AS part(good,qty)
		                   WHERE (part.good=ANY($4) OR part.good='livestock') AND part.qty::numeric>0
		               )
		               AND s.food_consumed_tick=current_world_tick()
		         ) - 1))) AS variety,
		         -- economy.PopulationCap: the wall is the ceiling. The table is bound as
		         -- $9 so the SQL has no second copy of the numbers.
		         ($9::int[])[LEAST(GREATEST(s.wall_level, 0), 3) + 1] AS pop_cap
		     FROM settlements s
		     JOIN claim c ON c.scope_id = s.id
		 ),
		 final AS (
		     SELECT
		         id, pop AS old_pop,
		         GREATEST(0,
		             CASE
		                 -- Starvation: retain (1 - starvationPopLossRatePerTick) of pop. The
		                 -- ::numeric cast keeps this exact numeric ROUND (half away from
		                 -- zero) — a bare float8 product would round half-to-even and drift
		                 -- by ±1 at pop ≡ 100 (mod 200); verified over pop 101..30000.
		                 -- LEAST(…, pop - 1): at least one person per tick, or a
		                 -- city under 100 would round its loss to nothing and
		                 -- starve forever without dying.
		                 WHEN unmet > 0 THEN LEAST(ROUND(pop * (1 - $3::float8)::numeric), pop - 1)
		                 -- At or over the cap: hold. A city over its cap loses no one
		                 -- (Timothy 2026-10-21) — it just does not grow until the wall
		                 -- is raised. The growth is clamped to the room left, so a city
		                 -- never overshoots, and nothing else clamps pop downward.
		                 WHEN food_net > 0 AND pop < pop_cap THEN
		                     pop + LEAST(pop_cap - pop,
		                                 GREATEST(1, ROUND(pop * $7::float8 * variety)))
		                 ELSE pop
		             END
		         ) AS new_pop
		     FROM growth_calc
		 ),
		 pop_upd AS (
		     UPDATE settlements s SET
		         invasions_today = 0,
		         population = f.new_pop
		     FROM final f
		     WHERE f.id = s.id
		     RETURNING s.id, f.old_pop, f.new_pop
		 )
		 SELECT id, old_pop, new_pop FROM pop_upd`,
		worldID, economy.GrainConsumptionPerCitizenPerTick, starvationPopLossRatePerTick, economy.FoodGoods, eventID,
		economy.FoodGoods, growthRatePerTick, economy.VarietyMinShare, economy.PopulationCaps(),
	)
	if err != nil {
		slog.Error("daily decay failed", "world", worldID, "err", err)
	} else {
		// Timothy2026-10-20: new whole citizens remain free for the player
		// to place. Still drain growth rows before reinforcement and other DB work.
		var crossings []popCrossing
		for growthRows.Next() {
			var c popCrossing
			if err := growthRows.Scan(&c.id, &c.oldPop, &c.newPop); err != nil {
				slog.Warn("daily decay: scan growth row", "err", err)
				continue
			}
			crossings = append(crossings, c)
		}
		growthRows.Close()

		// Manskaps-underhåll (megaron_plan_rekryteringsmodell.md, Timothy
		// 2026-08-19): trickle men into any reinforcing cohort sitting in its
		// origin city, cannibalizing THIS tick's growth (crossings' new_pop −
		// old_pop, already written above), never the settlement's standing
		// population. Must run AFTER the growth UPDATE has committed (it has —
		// growthRows already scanned) so there is a real growth figure to divide
		// between "stays as city growth" and "goes to the cohort".
		h.applyReinforcement(ctx, worldID, eventID, crossings)
	}

	// C-collapse: schedule CollapseSettlement for any settlement below
	// economy.CollapseBelowPopulation — starved there by the UPDATE above, or
	// overmobilised by Recruit.
	collapseRows, err := h.pool.Query(ctx,
		`SELECT id FROM settlements
		 WHERE world_id = $1 AND owner_id IS NOT NULL
		   AND state NOT IN ('sunk', 'collapsed')
		   AND population < $2`,
		worldID, economy.CollapseBelowPopulation,
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
			// Prune BEFORE recompute (megaron_plan_placeringsbeskarning.md,
			// Föda S1b's ≤floor(pop/100) invariant) so a population drop from
			// ANY source this tick — starvation above, but also battle loss,
			// occupation, desertion that landed between ticks — is caught here,
			// not just the starvation branch, and this tick's rate already
			// reflects the pruned placement set instead of a tick-stale one.
			if _, err := economy.PrunePlacementsToPopulation(ctx, h.pool, sid); err != nil {
				slog.Warn("prune placements to population failed", "settlement", sid, "err", err)
			}
			if err := economy.RecomputeProduction(ctx, h.pool, sid); err != nil {
				slog.Warn("recompute after pop tick failed", "settlement", sid, "err", err)
			}
			// Blockad med enhet (megaron_plan_blockad_med_enhet.md, Timothy
			// 2026-09-05): RecomputeProduction above already zeroed any
			// blocked hex's yield live, every call — this is the once-a-day
			// TRANSITION check (did a hex start/stop being blocked since
			// yesterday) that fires the owner's dispatch (plan §3.4). Must
			// run AFTER RecomputeProduction so a hex pruned to zero placements
			// this same tick doesn't get compared against a stale set.
			if err := economy.SyncHexBlockade(ctx, h.pool, h.store, h.hub, worldID, sid); err != nil {
				slog.Warn("sync hex blockade failed", "settlement", sid, "err", err)
			}
			// Belägringens start-/lyft-dispatch (megaron_plan_belagringsdispatch.md,
			// Timothy 2026-09-06): RecomputeProduction above already wrote this
			// tick's fresh `besieged` value — this is the once-a-day TRANSITION
			// check (did the siege start or lift since yesterday) that fires the
			// owner's dispatch, mirroring SyncHexBlockade's own placement above.
			if err := economy.SyncSiegeState(ctx, h.pool, h.store, h.hub, worldID, sid); err != nil {
				slog.Warn("sync siege state failed", "settlement", sid, "err", err)
			}
		}
	}

	h.applySiegeStarvationClock(ctx, worldID)
}

// applySiegeStarvationClock is belägring S3 (megaron_plan_belagring.md §S3,
// §Beslutade delbeslut 3): a besieged settlement whose food need is NOT
// fully covered — settlements.food_unmet_amount > 0, written earlier this
// same tick by FoodTick (priority 55, Utfodringsordningen D4,
// megaron_plan_utfodringsordningen.md) after the WHOLE grain→fish→livestock
// fallback chain came up short — accrues one CONSECUTIVE day on
// settlements.siege_starvation_ticks (mig 123). Food covered, or the
// blockade lifted (besieged flips false), resets it to 0. Must run AFTER
// the RecomputeProduction loop above so `besieged` is this tick's fresh
// value, not yesterday's.
//
// Anchor swapped from grain's rate < 0 to food_unmet_amount (2026-08-26):
// grain's rate can no longer go negative since D1 (RecomputeProduction
// writes raw, un-netted production) — the old predicate would have gone
// permanently, silently dead the moment D1 landed. Byt inte det ena utan
// det andra (megaron_plan_utfodringsordningen.md §5) — both are fixed here,
// in the same commit.
//
// Runs as a single CTE-chained statement (same shape as the growth query
// above) so the read (old counter) and write (new counter) come from the
// same snapshot: `capitulating` is computed from siege_state's old_ticks,
// BEFORE `updated` applies the write — a settlement can only ever cross the
// threshold once per starvation episode, because the same UPDATE resets its
// own counter back to 0 the instant it crosses (defensive: if the
// capitulation handler is ever delayed, tomorrow's tick won't re-enqueue a
// second SiegeCapitulation for the same episode; a fresh 30-day siege after
// a failed capitulation is a new episode and correctly counts again).
//
// kharis owns the counter but not the transition: combat (which performs
// the actual occupied-state change) is not a downward dependency of kharis
// (G1 — package dependency order), so crossing the threshold only ENQUEUES
// ScheduledSiegeCapitulation, exactly like the CollapseSettlement scheduling
// above crosses the same boundary for starvation-collapse.
func (h *TickHandler) applySiegeStarvationClock(ctx context.Context, worldID uuid.UUID) {
	rows, err := h.pool.Query(ctx,
		`WITH siege_state AS (
		     SELECT s.id, s.siege_starvation_ticks AS old_ticks, s.besieged, s.food_unmet_amount AS unmet
		     FROM settlements s
		     WHERE s.world_id = $1 AND s.owner_id IS NOT NULL AND s.state = 'active'
		 ),
		 capitulating AS (
		     SELECT id FROM siege_state
		     WHERE besieged AND unmet > 0 AND old_ticks + 1 >= $2
		 ),
		 updated AS (
		     UPDATE settlements s SET siege_starvation_ticks = CASE
		             WHEN ss.besieged AND ss.unmet > 0 THEN
		                 CASE WHEN ss.old_ticks + 1 >= $2 THEN 0 ELSE ss.old_ticks + 1 END
		             ELSE 0
		         END
		     FROM siege_state ss
		     WHERE ss.id = s.id AND (ss.besieged OR ss.old_ticks <> 0)
		     RETURNING s.id
		 )
		 SELECT id FROM capitulating`,
		worldID, economy.SiegeCapitulationTicks,
	)
	if err != nil {
		slog.Error("siege starvation clock tick failed", "world", worldID, "err", err)
		return
	}
	var capitulatedIDs []uuid.UUID
	for rows.Next() {
		var sid uuid.UUID
		if rows.Scan(&sid) == nil {
			capitulatedIDs = append(capitulatedIDs, sid)
		}
	}
	rows.Close()
	if len(capitulatedIDs) == 0 {
		return
	}
	var currentTick int
	_ = h.pool.QueryRow(ctx, `SELECT current_world_tick()`).Scan(&currentTick)
	for _, sid := range capitulatedIDs {
		if err := h.scheduler.EnqueueTick(ctx, worldID, events.ScheduledSiegeCapitulation,
			struct {
				SettlementID uuid.UUID `json:"settlement_id"`
				WorldID      uuid.UUID `json:"world_id"`
			}{SettlementID: sid, WorldID: worldID},
			currentTick,
		); err != nil {
			slog.Warn("siege capitulation: could not schedule event", "settlement", sid, "err", err)
		}
	}
}

// popCrossing is one settlement's population before/after the daily growth
// step inside applyDecay — the same figure both the P4 gubbe-placement loop
// and the manskaps-underhåll reinforce trickle (applyReinforcement) key off:
// newPop − oldPop is exactly how many citizens this settlement grew by THIS
// tick, i.e. the only pool a reinforcing cohort is allowed to draw from.
type popCrossing struct {
	id             uuid.UUID
	oldPop, newPop int
}

// applyReinforcement trickles decimated land cohorts back toward 100 men,
// economy.ReinforceMenPerTick men at a time per cohort, per tick
// (megaron_plan_rekryteringsmodell.md, Timothy 2026-08-19). It runs after
// applyDecay's own growth UPDATE has already committed settlements.population
// = old_pop + actual_new (crossings carries both), so every man this step
// hands to a cohort is given BACK out of that same increment — population
// after this step is old_pop + (actual_new − refillSpent), which is always
// ≥ old_pop. The city's standing population is never touched: a refill can
// only slow this tick's growth, never reverse it.
//
// Eligibility mirrors the Reinforce handler exactly: status='garrison',
// settlement_id = origin_settlement_id, reinforcing = true, size < 100. A
// cohort that marched out of its origin (or out of garrison at all) is
// caught by the unconditional cleanup below and stops trickling, holding
// whatever size it had reached — no claim needed for that part, it is a
// state fixup, not an accumulator (same "recompute/collapse steps are not
// claimed" rule this file already documents above).
//
// G2 idempotency: the actual increment+charge+give-back is claimed per
// (eventID, unit.id) via processed_tick_claims — a THIRD, practically-disjoint
// scope namespace under this same eventID, alongside the goods-decay hash
// scope and the per-settlement growth scope above. Migration 098's own
// comment already accepts this pattern ("the key includes event_id ... they
// cannot collide") for two scope kinds under one event; a unit id is simply
// a third UUID space that will not collide with either.
func (h *TickHandler) applyReinforcement(ctx context.Context, worldID uuid.UUID, eventID int64, crossings []popCrossing) {
	// Unconditional: a cohort that is no longer garrisoned at its own origin
	// stops reinforcing right away, regardless of whether this settlement grew
	// this tick. Runs every call — idempotent by construction (an UPDATE that
	// matches zero rows on replay is a no-op, not a second effect).
	if _, err := h.pool.Exec(ctx,
		`UPDATE units SET reinforcing = false, updated_at = now()
		 WHERE world_id = $1 AND reinforcing = true
		   AND NOT (status = 'garrison' AND settlement_id = origin_settlement_id)`,
		worldID,
	); err != nil {
		slog.Error("reinforcement: clear stale reinforcing flag failed", "world", worldID, "err", err)
	}

	for _, c := range crossings {
		budget := c.newPop - c.oldPop // men this settlement actually grew by this tick
		if budget <= 0 {
			continue
		}

		rows, err := h.pool.Query(ctx,
			`SELECT id, owner_id, type, size FROM units
			 WHERE settlement_id = $1 AND origin_settlement_id = $1
			   AND status = 'garrison' AND reinforcing = true
			 ORDER BY created_at`,
			c.id,
		)
		if err != nil {
			slog.Warn("reinforcement: list candidates failed", "settlement", c.id, "err", err)
			continue
		}
		type candidate struct {
			id      uuid.UUID
			ownerID uuid.UUID
			utype   string
			size    int
		}
		var candidates []candidate
		for rows.Next() {
			var cd candidate
			if err := rows.Scan(&cd.id, &cd.ownerID, &cd.utype, &cd.size); err == nil {
				candidates = append(candidates, cd)
			}
		}
		rows.Close()

		// Deterministic order (created_at, i.e. oldest cohort first) — several
		// reinforcing cohorts in the same city split one shared growth budget,
		// so SOME order is needed; oldest-first is the least surprising.
		for _, cd := range candidates {
			if budget <= 0 {
				break
			}
			refill := economy.ReinforceMenPerTick
			if budget < refill {
				refill = budget
			}
			if room := economy.MaxUnitSize - cd.size; room < refill {
				refill = room
			}
			if refill <= 0 {
				continue
			}

			// Pro-rata resource cost, same per-man table Recruit charges at full
			// price. Throttle down to whatever the city can actually afford this
			// tick (the simpler of the two options the plan allows — no partial
			// good-by-good partial fill, no deferral to a future tick with a
			// reserved claim).
			perMan := economy.RecruitCostPerMan(cd.utype)
			if len(perMan) > 0 {
				affordable, err := h.affordableRefill(ctx, c.id, perMan, refill)
				if err != nil {
					slog.Warn("reinforcement: affordability check failed",
						"settlement", c.id, "unit", cd.id, "err", err)
					continue
				}
				refill = affordable
			}
			if refill <= 0 {
				continue
			}

			// G2 claim: only the run that wins (event_id, unit_id) may apply the
			// increment/charge/give-back below. A worker replay of the same
			// ScheduledKharisTick event finds this row already claimed and moves on.
			claimTag, err := h.pool.Exec(ctx,
				`INSERT INTO processed_tick_claims (event_id, scope_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
				eventID, cd.id,
			)
			if err != nil {
				slog.Warn("reinforcement: claim failed", "unit", cd.id, "err", err)
				continue
			}
			if claimTag.RowsAffected() == 0 {
				continue // already processed this event for this unit
			}

			newSize := cd.size + refill
			stillReinforcing := newSize < economy.MaxUnitSize
			if _, err := h.pool.Exec(ctx,
				`UPDATE units SET size = $1, reinforcing = $2, updated_at = now() WHERE id = $3`,
				newSize, stillReinforcing, cd.id,
			); err != nil {
				slog.Error("reinforcement: grow unit failed", "unit", cd.id, "err", err)
				continue
			}
			for good, perManCost := range perMan {
				if perManCost <= 0 {
					continue
				}
				if _, err := h.pool.Exec(ctx,
					`UPDATE settlement_goods SET
					   amount = GREATEST(0, settled(amount, rate, calc_tick) - $1),
					   calc_tick = current_world_tick()
					 WHERE settlement_id = $2 AND good_key = $3`,
					perManCost*float64(refill), c.id, good,
				); err != nil {
					slog.Warn("reinforcement: charge goods failed",
						"settlement", c.id, "good", good, "err", err)
				}
			}
			// Give back only the men actually diverted to this cohort — the rest
			// of `budget` stays as this tick's city growth (already written by the
			// growth UPDATE above; this only ever subtracts, never adds beyond it).
			if _, err := h.pool.Exec(ctx,
				`UPDATE settlements SET population = population - $1 WHERE id = $2`,
				refill, c.id,
			); err != nil {
				slog.Error("reinforcement: debit population growth failed", "settlement", c.id, "err", err)
				continue
			}
			budget -= refill

			if h.store != nil {
				_, _ = h.store.Append(ctx, cd.id, events.StreamType(unit.StreamUnit), unit.EventUnitReinforced,
					unit.UnitReinforcedPayload{
						UnitID:       cd.id,
						SettlementID: c.id,
						SizeBefore:   cd.size,
						SizeAfter:    newSize,
						PopDrawn:     refill,
					}, worldID, nil)
			}
		}
	}
}

// affordableRefill throttles a proposed refill count down to whatever
// settlementID can actually pay for at perMan cost per good — the same
// "grow only as far as resources allow" rule applyDecay's own grain-funded
// growth already applies to citizens, extended here across every good in the
// unit type's per-man cost table (not just grain). Reads a fresh snapshot per
// good; a mid-tick recruit spending the same stock between this read and the
// later UPDATE is priced against a stale snapshot — acceptable for a value
// this small (at most ReinforceMenPerTick men) against the brute-force-simple
// alternative of locking every settlement_goods row here too.
func (h *TickHandler) affordableRefill(ctx context.Context, settlementID uuid.UUID, perMan map[string]float64, want int) (int, error) {
	affordable := want
	for good, cost := range perMan {
		if cost <= 0 {
			continue
		}
		var have float64
		if err := h.pool.QueryRow(ctx,
			`SELECT COALESCE((SELECT settled(amount, rate, calc_tick) FROM settlement_goods
			                   WHERE settlement_id = $1 AND good_key = $2), 0)`,
			settlementID, good,
		).Scan(&have); err != nil {
			return 0, err
		}
		if maxByGood := int(have / cost); maxByGood < affordable {
			affordable = maxByGood
		}
	}
	if affordable < 0 {
		affordable = 0
	}
	return affordable, nil
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

// applyDivinePunishment randomly selects and applies one divine punishment.
//
// Each punishment's UPDATE now runs through RETURNING (wrapped in a CTE that
// keeps the pre-update value alongside the post-update one, same pattern as
// applyDecay's pop_upd/grain_upd above) so the actual amount taken is known,
// not just which of the four punishments fired. This is a HARD-rule fix
// (CLAUDE.md §Events: "events store outcomes, not intentions" —
// {"type":"chariot_loss","amount":3}, never a bare roll-pending marker): the
// old code ran the mutation through h.pool.Exec and threw the result away,
// so the DivinePunishment event carried "type" only and no notification was
// ever sent at all. "type" and its four values are UNCHANGED below — this
// only ADDS "amount" beside it; old event rows and old readers keep working.
func (h *TickHandler) applyDivinePunishment(ctx context.Context, settlementID, worldID, ownerID uuid.UUID) error {
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	anchor, err := tick.LoadAnchor(ctx, tx, worldID)
	if err != nil {
		return err
	}
	// The shield claim, actual loss, event and notification commit as one outcome.
	if _, err := tx.Exec(ctx, `INSERT INTO kharis_notice_state(world_id,player_id,scope_id,condition,state,observed_tick)
 VALUES($1,$2,$2,'divine_punishment','none',$3) ON CONFLICT DO NOTHING`, worldID, ownerID, anchor.Tick); err != nil {
		return err
	}
	var until int
	if err := tx.QueryRow(ctx, `SELECT COALESCE((context->>'until_tick')::int,0) FROM kharis_notice_state
 WHERE world_id=$1 AND player_id=$2 AND scope_id=$2 AND condition='divine_punishment' FOR UPDATE`, worldID, ownerID).Scan(&until); err != nil {
		return err
	}
	if anchor.Tick < until {
		return nil
	}

	type punishment struct {
		name string
		text string
		sql  string
	}

	punishments := []punishment{
		{
			"chariot_loss",
			"The gods have scattered your war chariots in the night. Chariots have perished.",
			`WITH victims AS (
			     SELECT id, size FROM units
			     WHERE settlement_id = $1 AND world_id=$2 AND owner_id=$3 AND size>0 AND status = 'garrison' AND type = 'war_chariot'
			 ), upd AS (
			     UPDATE units u SET size = GREATEST(0, v.size - GREATEST(1, v.size/5)), updated_at = now()
			     FROM victims v WHERE u.id = v.id
			     RETURNING v.size - u.size AS lost
			 )
			 SELECT COALESCE(SUM(lost), 0)::float8 FROM upd`,
		},
		{
			"ship_loss",
			"A divine storm has claimed a vessel from your harbour.",
			`WITH victim AS (
			     SELECT id FROM units
			     WHERE settlement_id = $1 AND world_id=$2 AND owner_id=$3 AND size>0 AND status = 'garrison' AND type = 'galley'
			     ORDER BY size, id LIMIT 1
			 ), upd AS (
			     UPDATE units u SET status = 'disbanded', updated_at = now()
			     FROM victim v WHERE u.id = v.id
			     RETURNING 1
			 )
			 SELECT COUNT(*)::float8 FROM upd`,
		},
		{
			"harvest_failure",
			"The fields lie fallow by divine will. Half your grain stores have rotted.",
			`WITH old AS (
			     SELECT settled(amount, rate, calc_tick) AS grain_now
			     FROM settlement_goods WHERE settlement_id = $1 AND good_key = 'grain'
                 AND EXISTS(SELECT 1 FROM settlements s WHERE s.id=$1 AND s.world_id=$2 AND s.owner_id=$3)
			 ), upd AS (
			     UPDATE settlement_goods sg SET
			        amount    = GREATEST(0, o.grain_now * 0.5),
			        calc_tick = current_world_tick()
			     FROM old o
			     WHERE sg.settlement_id = $1 AND sg.good_key = 'grain'
			     RETURNING o.grain_now - sg.amount AS lost
			 )
			 SELECT COALESCE(SUM(lost), 0)::float8 FROM upd`,
		},
		{
			"garrison_plague",
			"A dark pestilence has moved through the barracks. Many hoplites have fallen.",
			`WITH victims AS (
			     SELECT id, size FROM units
			     WHERE settlement_id = $1 AND world_id=$2 AND owner_id=$3 AND size>0 AND status = 'garrison' AND type = 'spearman'
			 ), upd AS (
			     UPDATE units u SET size = GREATEST(0, v.size - GREATEST(1, v.size/5)), updated_at = now()
			     FROM victims v WHERE u.id = v.id
			     RETURNING v.size - u.size AS lost
			 )
			 SELECT COALESCE(SUM(lost), 0)::float8 FROM upd`,
		},
	}

	// Choose a city first, uniformly among owned active cities with something
	// to take. Then choose only feasible effects there. The caller's capital is
	// merely the audit origin; it must not constrain the target.
	var settlementName string
	var eligible []int
	err = tx.QueryRow(ctx, `
 WITH candidates AS (
  SELECT s.id, s.name, ARRAY_REMOVE(ARRAY[
   CASE WHEN EXISTS(SELECT 1 FROM units u WHERE u.settlement_id=s.id AND u.world_id=$1 AND u.owner_id=$2 AND u.status='garrison' AND u.type='war_chariot' AND u.size>0) THEN 0 END,
   CASE WHEN EXISTS(SELECT 1 FROM units u WHERE u.settlement_id=s.id AND u.world_id=$1 AND u.owner_id=$2 AND u.status='garrison' AND u.type='galley' AND u.size>0) THEN 1 END,
   CASE WHEN EXISTS(SELECT 1 FROM settlement_goods sg WHERE sg.settlement_id=s.id AND sg.good_key='grain' AND settled(sg.amount,sg.rate,sg.calc_tick)>0) THEN 2 END,
   CASE WHEN EXISTS(SELECT 1 FROM units u WHERE u.settlement_id=s.id AND u.world_id=$1 AND u.owner_id=$2 AND u.status='garrison' AND u.type='spearman' AND u.size>0) THEN 3 END
  ],NULL) AS effects
  FROM settlements s WHERE s.world_id=$1 AND s.owner_id=$2
    AND s.state NOT IN ('sunk','collapsed','razed')
 )
 SELECT id,name,effects FROM candidates WHERE cardinality(effects)>0 ORDER BY random() LIMIT 1`, worldID, ownerID).Scan(&settlementID, &settlementName, &eligible)
	if err == pgx.ErrNoRows {
		return nil
	}
	if err != nil {
		return fmt.Errorf("choose divine punishment target: %w", err)
	}
	p := punishments[eligible[rand.Intn(len(eligible))]]
	// The galley the storm will take, named before it is disbanded (Haiku-7: a
	// player saw the galley arrive, then vanish, and could not tell it was lost).
	// Same victim rule as the ship_loss SQL.
	var unitName string
	if p.name == "ship_loss" {
		var victimID uuid.UUID
		if err = tx.QueryRow(ctx,
			`SELECT id FROM units
			 WHERE settlement_id = $1 AND world_id=$2 AND owner_id=$3 AND size>0 AND status = 'garrison' AND type = 'galley'
			 ORDER BY size, id LIMIT 1`, settlementID, worldID, ownerID).Scan(&victimID); err == nil {
			unitName = unit.LoadDisplayName(ctx, tx, victimID)
		}
	}
	var amount float64
	if err = tx.QueryRow(ctx, p.sql, settlementID, worldID, ownerID).Scan(&amount); err != nil {
		return fmt.Errorf("apply divine punishment: %w", err)
	}
	// Disband any garrison unit reduced to nothing by the punishment (no-op for
	// the grain-only harvest_failure case). Army lives in units (SB7).
	_, err = tx.Exec(ctx,
		`UPDATE units SET status = 'disbanded', updated_at = now()
		 WHERE settlement_id = $1 AND status = 'garrison' AND size <= 0`, settlementID)
	if err != nil {
		return err
	}

	// Nothing to take, nothing to announce (acceptance sweep 2026-08-24). The
	// roll picks one of four punishments blind, so a Wanax with no chariots and
	// no ships draws chariot_loss/ship_loss half the time — and before this
	// guard that still emitted a DivinePunishment event, divine gossip, and a
	// LEVEL 2 (urgent) notification reading {"type":"ship_loss","amount":0}:
	// "a divine storm has claimed a vessel from your harbour" to a player who
	// owns no vessels. Observed twice in the first six ticks of a fresh world.
	//
	// This is the same rule the fix that introduced `amount` was written to
	// serve (CLAUDE.md §Events, "events store outcomes, not intentions"): an
	// outcome of nothing is not an outcome to report. No balance changes — the
	// roll and what it takes are untouched, only what is said about it. The
	// gods reached for something that was not there.
	if amount <= 0 {
		slog.Info("divine punishment rolled but found nothing to take",
			"settlement", settlementID, "type", p.name)
		return nil
	}

	until = anchor.Tick + WrathProtectionTicks
	untilAt := anchor.WallAt(int64(until) * 1000)
	body := punishmentPayload(p.name, amount, settlementName, unitName)
	body["player_id"] = ownerID
	body["settlement_id"] = settlementID
	body["protection_until_tick"] = until
	body["protection_until"] = untilAt
	if _, err := tx.Exec(ctx, `UPDATE kharis_notice_state SET state='protected',observed_tick=$3,context=jsonb_build_object('until_tick',$4::int)
 WHERE world_id=$1 AND player_id=$2 AND scope_id=$2 AND condition='divine_punishment'`, worldID, ownerID, anchor.Tick, until); err != nil {
		return err
	}
	event, err := h.store.AppendTx(ctx, tx, settlementID, events.StreamProvince, "DivinePunishment", body, worldID, nil)
	if err != nil {
		return err
	}
	var noticeID string
	if err := tx.QueryRow(ctx, `INSERT INTO notifications(world_id,player_id,kind,level,body_json,world_tick)
 VALUES($1,$2,'DivinePunishment',2,$3,$4) RETURNING id`, worldID, ownerID, body, anchor.Tick).Scan(&noticeID); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	h.store.RecordCommitted(ctx, event)
	h.addDivineGossip(ctx, settlementID, worldID, "divine_wrath", p.text)
	if d, ok := h.hub.(committedNoticeDelivery); ok {
		d.DeliverCommittedNotification(ctx, worldID, ownerID, noticeID, "DivinePunishment", 2, body)
	} else if h.hub != nil {
		_ = h.hub.NotifyPlayer(ctx, worldID, ownerID, "DivinePunishment", 2, body)
	}
	slog.Info("divine punishment applied", "settlement", settlementID, "type", p.name, "amount", amount)
	return nil
}

// punishmentPayload is the DivinePunishment body. unit_name (additive) names
// the ship a ship_loss took; it is absent for every other punishment.
func punishmentPayload(kind string, amount float64, settlementName, unitName string) map[string]any {
	body := map[string]any{"type": kind, "amount": amount, "name": settlementName}
	if unitName != "" {
		body["unit_name"] = unitName
	}
	return body
}

// applyDivineBlessing randomly selects and applies one divine blessing for settlements
// that maintain high kharis. Mirror of applyDivinePunishment — verified to carry the
// SAME two bugs (no notification, no captured outcome) and fixed the same way: each
// branch's mutation is now scanned via RETURNING (or, for the army branches, via
// applyArmyBlessing's own RETURNING/insert), and the amount rides alongside "type"
// (unchanged) in both the event payload and a new NotifyPlayer call.
func (h *TickHandler) applyDivineBlessing(ctx context.Context, settlementID, worldID, ownerID uuid.UUID) {
	type blessing struct {
		name string
		text string
		sql  string
	}

	blessings := []blessing{
		{
			// "bountiful_harvest" (Timothy 2026-10-10) replaces "harvest_blessing"
			// (×1.25 on the grain store, amount = grain gained). A new type, not a
			// reinterpretation: old events keep their meaning. amount = the days
			// the yield bonus lasts. Empty sql: applied via economy below.
			"bountiful_harvest",
			"The gods smile upon your fields. For a season they bear more than any hand could make them.",
			"",
		},
		{
			// Army lives in the units table (SB7) — handled by applyArmyBlessing,
			// not a settlements UPDATE. Empty sql signals the code path below.
			"divine_recruits",
			"Warriors answer a divine call and join your ranks. New hoplites have arrived.",
			"",
		},
	}

	b := blessings[rand.Intn(len(blessings))]
	// No sea_blessing (Timothy 2026-10-10): a ship arrives with certainty when a
	// Wanax founds the first city by the sea — Poseidon's galley, whose crew is
	// drawn from the city — and never otherwise. The blessing conjured a galley
	// AND its 20 crew out of nothing; disbanding it turned them into citizens.
	// Old DivineBlessing events with type "sea_blessing" keep their meaning.
	var settlementName string
	if err := h.pool.QueryRow(ctx,
		`SELECT name FROM settlements WHERE id = $1`, settlementID,
	).Scan(&settlementName); err != nil {
		slog.Error("divine blessing: load settlement name", "settlement", settlementID, "err", err)
	}

	var amount float64
	if b.name == "bountiful_harvest" {
		if _, err := economy.ApplyHarvestBlessing(ctx, h.pool, settlementID); err != nil {
			slog.Error("divine blessing failed", "settlement", settlementID, "type", b.name, "err", err)
			return
		}
		amount = economy.HarvestBlessingTicks
	} else if b.sql != "" {
		if err := h.pool.QueryRow(ctx, b.sql, settlementID).Scan(&amount); err != nil {
			slog.Error("divine blessing failed", "settlement", settlementID, "type", b.name, "err", err)
			return
		}
	} else {
		gained, err := h.applyArmyBlessing(ctx, settlementID, worldID, b.name)
		if err != nil {
			slog.Error("divine army blessing failed", "settlement", settlementID, "type", b.name, "err", err)
			return
		}
		amount = float64(gained)
	}

	_, _ = h.store.Append(ctx, settlementID, events.StreamProvince, "DivineBlessing",
		map[string]any{"type": b.name, "amount": amount, "name": settlementName}, worldID, nil)
	h.addDivineGossip(ctx, settlementID, worldID, "divine_favour", b.text)
	// Fel A/B fix, mirroring applyDivinePunishment above. Level 3 (info) —
	// good news, same tier as ColonyFounded/TrainComplete, not urgent.
	// "name": see applyDivinePunishment's comment on the same field — additive,
	// old fields unchanged.
	if h.hub != nil && ownerID != uuid.Nil {
		_ = h.hub.NotifyPlayer(ctx, worldID, ownerID, "DivineBlessing", 3, map[string]any{
			"settlement_id": settlementID,
			"type":          b.name,
			"amount":        amount,
			"name":          settlementName,
		})
	}
	slog.Info("divine blessing applied", "settlement", settlementID, "type", b.name, "amount", amount)
}

// applyArmyBlessing grants the army-boosting divine blessings against the units
// table (SB7: the army lives in units, not the retired settlements.* columns).
// Returns the amount gained (men for divine_recruits) so
// the caller can report it — previously discarded (plan §Hål 1 Fel B, same bug
// as applyDivinePunishment's SQL).
func (h *TickHandler) applyArmyBlessing(ctx context.Context, settlementID, worldID uuid.UUID, name string) (int, error) {
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
		//
		// RETURNING the pre/post size delta (same CTE pattern as
		// applyDivinePunishment above) doubles as the RowsAffected==0 check the
		// old h.pool.Exec used: no matching spearman garrison unit → the "victim"
		// CTE is empty → "upd" is empty → the final SELECT returns zero rows →
		// pgx.ErrNoRows, exactly the old tag.RowsAffected()==0 branch.
		const formedSize = 2
		var gained int
		err := h.pool.QueryRow(ctx,
			`WITH victim AS (
			     SELECT id, size FROM units
			     WHERE settlement_id = $1 AND status = 'garrison' AND type = 'spearman'
			     ORDER BY size DESC LIMIT 1
			 ), upd AS (
			     UPDATE units u SET size = LEAST($2, v.size + GREATEST(2, v.size/5)), updated_at = now()
			     FROM victim v WHERE u.id = v.id
			     RETURNING u.size - v.size AS gained
			 )
			 SELECT gained FROM upd`,
			settlementID, economy.MaxUnitSize,
		).Scan(&gained)
		if err != nil {
			if err == pgx.ErrNoRows {
				if err := h.insertGarrisonUnit(ctx, settlementID, worldID, "spearman", "land", formedSize, 0); err != nil {
					return 0, err
				}
				return formedSize, nil
			}
			return 0, err
		}
		return gained, nil
	}
	return 0, nil
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

// templeOfferScarcity values the standing temple reference offering (grain) at the
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
	baseValues, err := religion.LoadLaborValues(ctx, h.pool)
	return scarcityFromValues(values, baseValues, err)
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

// templeScarcityCeil bounds what a shortage can be worth to a temple.
const templeScarcityCeil = 2.0
