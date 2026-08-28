package combat

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"os"
	"strconv"

	"formatet/megaron/server/internal/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// UpkeepSoldShare reads UPKEEP_SOLD_SHARE — the fraction of a garrisoned unit's
// silver upkeep the soldiers spend back into their garrison town (silver-plan
// Del C). Default 0.7; 0 = exactly the pre-Del-C behaviour (whole upkeep leaves
// the world). Clamped to [0,1]. Read once at handler construction (main.go),
// override via env + systemctl restart — same pattern as the Sitos tunables.
//
// Exported because the read surfaces (api/handlers) must project the same net
// drain the tick actually takes; a second copy of the default would drift.
func UpkeepSoldShare() float64 {
	s := 0.7
	if v := os.Getenv("UPKEEP_SOLD_SHARE"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			s = f
		}
	}
	if s < 0 {
		return 0
	}
	if s > 1 {
		return 1
	}
	return s
}

// UpkeepSpec — grain + silver per upkeep-period för en full-size enhet.
type UpkeepSpec struct {
	Grain  float64
	Silver float64
}

// UpkeepSpecs: landenheter skalas med size/100; navala är flat (size=1).
// Präst = ingen upkeep (kult kostar inget löpande).
//
// Landenheternas grain (SLICE A, Timothy 2026-08-05): kalibrerad så att en
// garnisonerad full-size (100) enhet äter EXAKT vad en civil äter —
// pop×0.5/dag (economy/recompute.go:359) — dvs 50 korn/dygn för 100 man.
// Typvariationen är avsiktligt bevarad (elit äter mer, stridsvagnen har
// hästar). UnitUpkeep dubblar grain ovanpå detta i fält (marching/positioned).
//
// Navala talen är ORÖRDA av samma beslut: naval upkeep är per skrov, skalar
// inte med size alls, och "dubbelt så mycket som en medborgare" har därför
// ingen innebörd för en besättning på det sättet landenheternas har för en
// soldat. UnitUpkeep dubblar därför aldrig navalt grain, oavsett status.
//
// Silverkolumnen halverad (SLICE B, Timothy 2026-08-05): AG3 = lägre
// silverupkeep, formen "halvera hela tabellen". Grain rörs inte — slice A
// satte det. nomadicHostRationTicks (api/handlers/nomadic_host.go) dubblades
// i SAMMA slice, annars hade halveringen tyst halverat startsilvret också
// och grinden (480 ska räcka märkbart längre än 48 speldygn) hade landat på
// exakt samma 48 som innan.
// Grain-kolumnen ÷100 (megaron_plan_dagsverkesskalan, mig 136, 2026-08-27) —
// SAMMA faktor som GrainConsumptionPerCitizenPerTick (0,5 → 0,005), inte
// grain-varans divisor 43,2. Skälet är en likhet som måste överleva
// omskalningen: en landkohort är 100 man och åt före exakt vad en gubbe
// (100 invånare) åt, 50 mot 50. Efter migrationen äter båda 0,50. Hade upkeep
// följt varudivisorn i stället hade soldaten plötsligt ätit dubbelt mot
// civilbefolkningen utan att någon beslutat det.
//
// Silver rörs INTE — det är en valuta och skalas inte i mig 136 (se
// migrationens huvudkommentar: kollapsmekaniken är en egen systemfråga).
var UpkeepSpecs = map[string]UpkeepSpec{
	"spearman":       {Grain: 0.50, Silver: 1},
	"elite_infantry": {Grain: 0.60, Silver: 2},
	"war_chariot":    {Grain: 0.80, Silver: 3},
	"galley":         {Grain: 0.04, Silver: 1.5},
	"war_galley":     {Grain: 0.06, Silver: 2.5},
	"merchantman":    {Grain: 0.03, Silver: 1},
}

// UnitUpkeep returns the grain + silver one unit costs per upkeep-period (the
// daily upkeep tick). Land units scale with size/100; naval and everything else
// are flat (per vessel); unknown types cost nothing. This is the single
// source of truth for the scaling — both the charging loop (Handle) and the army
// read surface (api/handlers) call it, so shown upkeep can never drift from what
// is actually debited.
//
// Kanon 2026-08-05: a soldier is a person. Garrisoned, he eats like a civilian
// (UpkeepSpecs' land grain figures are calibrated to that anchor — see
// economy/recompute.go:359, pop×0.5/day). In the field — status "marching" or
// "positioned" — he eats double: mobilising costs, and standing out costs more
// than standing home. Silver (sold) never changes with status — the pay is the
// same wherever the man stands. Naval upkeep is per hull, not per person, and
// is untouched by status entirely (see UpkeepSpecs' comment).
func UnitUpkeep(unitType, category string, size int, status string) UpkeepSpec {
	spec, ok := UpkeepSpecs[unitType]
	if !ok {
		return UpkeepSpec{}
	}
	if category != "land" {
		return spec // naval/other: flat, status never changes it
	}
	f := float64(size) / 100.0
	grain := spec.Grain * f
	// "embarked" (SLICE, 2026-08-05): a cohort aboard a ship is maximally away
	// from the city's stores — field ration applies exactly like marching/
	// positioned. It is grouped WITH them here so this stays the single trigger
	// set for the doubling; Handle's own status filter below must list the same
	// three-plus-embarked set or a billed status stops being billed.
	if status == "marching" || status == "positioned" || status == "embarked" {
		grain *= upkeepFieldGrainFactor
	}
	return UpkeepSpec{Grain: grain, Silver: spec.Silver * f}
}

// VoyageRation är vad ett skepp äter per dygn till sjöss, inklusive en eventuell
// embarkerad kohort — alltså exakt det tal provianteringen ska täcka.
//
// Den bor HÄR, bredvid UnitUpkeep, med flit: provianten och dragningen måste
// räknas ur samma källa, annars provianterar man för ett tal och debiterar ett
// annat, och skillnaden syns först som ett skepp som svälter mitt i en resa det
// betalade fullt för.
//
// Kohorten ombord äter FÄLTRANSON (upkeepFieldGrainFactor): en embarkerad
// soldat är maximalt borta från stadens förråd, vilket är precis den motivering
// 'embarked' lades till i fältmängden för (2026-08-05, embarkerad ranson).
// cargoType/cargoSize är nollvärden när skeppet går tomt.
func VoyageRation(shipType string, shipSize int, cargoType string, cargoSize int) float64 {
	ration := UnitUpkeep(shipType, "naval", shipSize, "positioned").Grain
	if cargoType != "" && cargoSize > 0 {
		ration += UnitUpkeep(cargoType, "land", cargoSize, "embarked").Grain
	}
	return ration
}

// VoyageProvisions är vad som ska dras ur hemstaden vid avfärd:
// dygnsranson × (ut + station + hem).
//
// Hemresan approximeras som SYMMETRISK med utresan. Den exakta hemvägen är inte
// känd vid utskick (skeppet kan bli omdirigerat, och A* körs mot ett annat
// startläge), och de två felen är inte lika dyra: överproviantering kommer hem
// igen och lastas av, underproviantering strandar ett skepp. Approximationen
// ska därför alltid luta uppåt.
//
// stationTicks är 0 för en vanlig marsch och SentryPatrolTicks för en
// sjösentry, som per definition ligger stilla hela sin patrull.
func VoyageProvisions(ration float64, travelTicks, stationTicks int) float64 {
	if travelTicks < 1 {
		travelTicks = 1
	}
	if stationTicks < 0 {
		stationTicks = 0
	}
	return ration * float64(2*travelTicks+stationTicks)
}

// ProvisionDaysLeft är matmätarens tal (Timothy 2026-08-26: "någon slags
// matmätare som spelaren kan se"). DYGN, inte råa korn — världen mäts i
// speldygn, och "14 dygn" är något en spelare kan handla på. Golvdivision:
// mätaren får aldrig visa en dag skeppet inte har mat för hela.
func ProvisionDaysLeft(provisions, ration float64) int {
	if ration <= 0 {
		return 0
	}
	if provisions <= 0 {
		return 0
	}
	return int(provisions / ration)
}

const (
	upkeepFieldGrainFactor = 2.0 // korn i fält (marching/positioned/embarked) mot garnison
	// upkeepAttritionStepPercent är en ANDEL av size, inte ett mantal (fram
	// till 2026-08-25 var detta ett FLAT mantal — 10 — vilket utplånade en
	// redan decimerad kohort nästan helt på en enda tick: en 12-mannarest
	// tappade lika många man som en fräsch 100-kohort. Talet 10 är oförändrat,
	// bara tolkningen: en 100-kohort tappar fortfarande 10, en 12-rest tappar
	// nu 2. Se megaron_plan_upkeep_attrition.md §Form.
	upkeepAttritionStepPercent = 10.0
	// navalAttritionCrewStep är ett STRAWMAN-tal — flottans besättningsförlust
	// per tick vid grain-brist är ännu okalibrerad mot en levande värld (spärren
	// på kalibrering lyftes 2026-08-24 i och med körordningsfixen, men ingen
	// mätning har körts). Formen (träffa crew, inte size/hull) är låst; TALET
	// är inte. En galley (crew 20) håller ~10 tick, en war_galley (crew 50)
	// ~25, en merchantman (crew 10) ~5. Kalibrera via speldygnstest/soak, inte
	// härifrån. megaron_plan_upkeep_attrition.md §Form, §Steg.
	navalAttritionCrewStep = 2
	upkeepDesertionStep    = 10 // män förlorade per tick vid silver-brist (efter tröskel)
	// 3 → 72 (Timothy 2026-08-06), an INVARIANT-PRESERVING retune, not a balance
	// change: this counts macro-tick FIRINGS, and those went from every 24 ticks
	// to every tick when the tick became the day. 3 firings × 24 ticks = 72 ticks
	// before; 72 firings × 1 tick = the same 72 ticks after — identical real-time
	// behaviour (72 h at 60 min/tick), so the asynchronicity gate is untouched.
	// Left at 3 it would have deserted an army after 3 real hours, i.e. while its
	// Wanax slept. Fiction reads right too: 72 ticks unpaid before a cohort melts.
	// (The balans stack's own upkeepDesertionPeriods=3 assumed the pre-tick=day
	// once-per-day firing; under MacroTickInterval=1 the equivalent is 72.)
	upkeepDesertionTicks = 72 // obetalda silver-ticks före desertering börjar
)

// upkeepUnpaidWarningKind is the forewarning event type / notification kind fired
// each period a unit's silver upkeep goes unpaid, before desertion actually starts
// (SLICE A, 2026-07-31). Matches the codebase's convention (UnitDeserted,
// UnitAttrition) of using the same string for both the events-log entry and the
// NotifyPlayer kind.
const upkeepUnpaidWarningKind = "UpkeepUnpaid"

// upkeepMacroTickPayload is the payload for the recurring upkeep tick.
type upkeepMacroTickPayload struct{}

// upkeepUnitRow holds the columns we need per unit during the upkeep loop.
type upkeepUnitRow struct {
	id       uuid.UUID
	ownerID  uuid.UUID
	unitType string
	category string
	size     int
	crew     int
	// carrierID är skeppet som bär den här enheten (bara satt för 'embarked'):
	// kohorten ombord äter ur skeppets proviant, inte ur staden.
	carrierID     *uuid.UUID
	settlementID  *uuid.UUID
	unpaidPeriods int
	cargoUnitID   *uuid.UUID
	status        string
	// supportSettlementID är den AUKTORITATIVA betalaren (mig 100,
	// megaron_aktorer_plan.md §3.1). Den är NULL när staden är borta eller har
	// bytt ägare — se queryns korrelerade subselect — och det betyder att ingen
	// sold betalas, inte att någon annan stad tar över.
	supportSettlementID *uuid.UUID
}

// UpkeepHandler applies grain + silver upkeep to all active units each tick.
// Grain-brist → attrition; silver-brist → desertering after upkeepDesertionTicks.
type UpkeepHandler struct {
	pool      *pgxpool.Pool
	scheduler *events.Scheduler
	store     *events.Store
	hub       Broadcaster
	soldShare float64 // Del C: garrison sold spent back into its town
}

// NewUpkeepHandler creates an UpkeepHandler. hub may be nil (tests) — every
// NotifyPlayer call is nil-guarded, matching the other combat handlers. The
// sold-circulation share is read from env here; tests set h.soldShare directly.
func NewUpkeepHandler(pool *pgxpool.Pool, sched *events.Scheduler, store *events.Store, hub Broadcaster) *UpkeepHandler {
	return &UpkeepHandler{pool: pool, scheduler: sched, store: store, hub: hub, soldShare: UpkeepSoldShare()}
}

// Handle processes a ScheduledUpkeepTick event.
func (h *UpkeepHandler) Handle(ctx context.Context, e events.ScheduledEvent) error {
	// 1. Load all active units in the world.
	//
	// Units belonging to a player still in the founder phase are skipped: their
	// keep is already folded into founder_phase's grain/silver drain rate, and
	// they have no settlement at all yet. Charging them here would bill the same
	// cohort twice (temenos_nomadic_host_bygg.md B3). The exclusion lifts by
	// itself at founding, when active flips to false.
	//
	// Subselecten på support_settlement_id gör TVÅ kontroller i ett svep: att
	// staden fortfarande finns, och att den fortfarande ägs av enhetens ägare.
	// Faller staden — förstörd eller erövrad — blir kolumnen NULL här, och
	// enheten behandlas som obetald. Det är regeln, inte ett fel: det finns
	// ingen väg att rädda ett förband vars stad fallit (§3.1 punkt 1 och 3).
	// 'embarked' was missing from this filter until 2026-08-05 (embarkerad
	// ranson): a land unit aboard a ship paid nothing at all, neither grain nor
	// silver, for as long as it stood embarked. Before slice A's ×10 land-grain
	// recalibration that was a rounding error (5 grain/day for a full cohort);
	// after it, 100 grain/day quietly waived — "load the army onto a ship" had
	// become a way to stop feeding it. UnitUpkeep's own field-ration trigger
	// set (above) must list the same statuses, or a status stops being billed
	// without stopping being billable.
	// 'repairing' added for the same reason (megaron_plan_skeppsreparation.md
	// Slice C, 2026-08-16): naval upkeep is flat-per-hull and "untouched by
	// status entirely" (UnitUpkeep's own doc comment) — leaving a ship parked
	// in a shipyard queue out of this filter would have made permanent repair
	// a free way to dodge silver upkeep.
	rows, err := h.pool.Query(ctx,
		`SELECT u.id, u.owner_id, u.type, u.category, u.size, u.crew, u.settlement_id,
		        u.unpaid_periods, u.cargo_unit_id,
		        (SELECT c.id FROM units c
		          WHERE c.cargo_unit_id = u.id AND c.world_id = u.world_id),
		        (SELECT s.id FROM settlements s
		          WHERE s.id = u.support_settlement_id AND s.owner_id = u.owner_id),
		        u.status
		 FROM units u
		 WHERE u.world_id = $1
		   AND u.status IN ('garrison', 'marching', 'positioned', 'embarked', 'repairing')
		   AND NOT EXISTS (
		       SELECT 1 FROM founder_phase fp
		       WHERE fp.world_id = u.world_id
		         AND fp.owner_id = u.owner_id
		         AND fp.active
		   )`,
		e.WorldID,
	)
	if err != nil {
		return fmt.Errorf("upkeep: query units: %w", err)
	}
	defer rows.Close()

	var units []upkeepUnitRow
	for rows.Next() {
		var u upkeepUnitRow
		if err := rows.Scan(&u.id, &u.ownerID, &u.unitType, &u.category,
			&u.size, &u.crew, &u.settlementID, &u.unpaidPeriods, &u.cargoUnitID, &u.carrierID,
			&u.supportSettlementID, &u.status); err != nil {
			return fmt.Errorf("upkeep: scan unit: %w", err)
		}
		units = append(units, u)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	// 2. Den försörjande staden är den enda betalaren.
	//
	// Detta ERSÄTTER en tyst huvudstadsfallback (mig 100). Tidigare returnerade
	// den här funktionen units.settlement_id om den var satt och annars ägarens
	// huvudstad — vilket betydde att varje marscherande enhet och varje skepp
	// till sjöss debiterades huvudstaden av misstag, inte av design. Den
	// betalande staden var ingen designad sanning; den var en tystnad.
	//
	// Nu är den ett stabilt faktum spelaren kan se och planera mot: förbandet
	// betalas hela sitt liv av staden som reste det, oavsett var det står.
	// Saknas den staden finns ingen ersättare — vägen nedåt går till
	// applyAttrition och recordUnpaid, samma maskineri som all annan uteblivning.
	payingSettlement := func(u upkeepUnitRow) (uuid.UUID, bool) {
		if u.supportSettlementID != nil {
			return *u.supportSettlementID, true
		}
		return uuid.UUID{}, false
	}

	// Per-settlement silver/grain accounting for the UpkeepSettled audit event
	// (silver flow bookkeeping, Del A). Keyed by paying settlement; units with no
	// paying settlement (no capital yet) are not attributed.
	aggs := make(map[uuid.UUID]*upkeepAgg)
	agg := func(sid uuid.UUID) *upkeepAgg {
		a := aggs[sid]
		if a == nil {
			a = &upkeepAgg{}
			aggs[sid] = a
		}
		return a
	}

	// 3. Process each unit.
	for _, u := range units {
		up := UnitUpkeep(u.unitType, u.category, u.size, u.status)
		if up.Grain == 0 && up.Silver == 0 {
			continue // unknown type — no upkeep
		}

		// G2 idempotency (migration 098's processed_tick_claims — same guard
		// colony.go's applyColonyPenalty and borrowed_army.go use for this exact
		// class of bug): Handle fans ONE scheduled event across every active unit
		// and mutates settlement_goods/units directly with plain UPDATEs, no
		// FOR UPDATE, no per-event transaction. A G2 handler timeout or a crash
		// mid-loop leaves the scheduled event unprocessed, so events.Worker
		// re-claims and re-runs Handle from the top — which, before this guard,
		// re-deducted grain/silver, re-incremented unpaid_periods and re-emitted
		// UnitAttrition/UnitDeserted/UpkeepUnpaid for every unit already charged.
		//
		// Scope is the unit id: the query above returns each active unit at most
		// once per Handle call, so (event_id, unit_id) is exactly the grain a
		// single event legitimately touches once. A genuinely NEW recurring tick
		// never reuses e.ID — EnqueueTickRecurring always inserts a fresh
		// scheduled_events row — so this only ever suppresses a true replay of
		// the SAME event, never a later day's charge.
		claim, err := h.pool.Exec(ctx,
			`INSERT INTO processed_tick_claims (event_id, scope_id) VALUES ($1, $2)
			 ON CONFLICT DO NOTHING`, e.ID, u.id)
		if err != nil {
			slog.Error("upkeep: claim failed", "unit", u.id, "err", err)
			continue
		}
		if claim.RowsAffected() == 0 {
			continue // this event already charged this unit's upkeep
		}

		grainNeed := up.Grain
		silverNeed := up.Silver

		sid, hasSid := payingSettlement(u)

		// Track whether grain already disbanded the unit this tick.
		disbanded := false

		// ── Grain upkeep ─────────────────────────────────────────────────────
		//
		// Sjövägen äter ur SKEPPETS lager, inte ur staden
		// (megaron_plan_skeppsproviant.md §5). Fram till 2026-08-26 läste den här
		// grenen support_settlement_id utan någon distansterm, så en galär tjugo
		// hexar ut åt ur hemstadens magasin varje tick, omedelbart — maten
		// struntade i den regel allt annat i spelet lyder.
		//
		// En embarkerad kohort äter ur det BÄRANDE skeppets lager, av samma skäl
		// som den betalar fältranson: den är ombord, inte hemma.
		//
		// Faller dragningen ur provianten går den INTE vidare till staden om
		// enheten är ute — det vore teleporteringen tillbaka genom en bakdörr.
		// Ett skepp i hamn (garrison/repairing) har normalt tom proviant, den
		// lastades av vid hemkomsten, och faller därför rätt ner i stadsgrenen.
		provisionSource := provisionSourceFor(u)
		if grainNeed > 0 && provisionSource != nil {
			tag, perr := h.pool.Exec(ctx,
				`UPDATE units SET provisions = provisions - $1, updated_at = now()
				 WHERE id = $2 AND provisions >= $1`,
				grainNeed, *provisionSource,
			)
			switch {
			case perr != nil:
				slog.Error("upkeep: provision draw failed", "unit", u.id, "err", perr)
			case tag.RowsAffected() == 1:
				grainNeed = 0 // fed from the ship's own stores
			case atHomePort(u):
				// Docked with empty stores — the town feeds it, as before.
			default:
				// At sea with nothing left. This is the exception the
				// provisioning exists to make rare; today it bites as attrition.
				disbanded = h.applyAttrition(ctx, u, grainNeed, e.WorldID, sid)
				grainNeed = 0
			}
		}

		if grainNeed > 0 {
			if !hasSid {
				// No paying settlement — treat as grain shortage.
				disbanded = h.applyAttrition(ctx, u, grainNeed, e.WorldID, sid)
			} else {
				tag, err := h.pool.Exec(ctx,
					`UPDATE settlement_goods
					 SET amount  = settled(amount, rate, calc_tick) - $1,
					     calc_tick = current_world_tick()
					 WHERE settlement_id = $2
					   AND good_key = 'grain'
					   AND settled(amount, rate, calc_tick) >= $1`,
					grainNeed, sid,
				)
				if err != nil {
					slog.Error("upkeep: grain deduction failed", "unit", u.id, "err", err)
				} else if tag.RowsAffected() == 0 {
					// Grain shortage — attrition.
					disbanded = h.applyAttrition(ctx, u, grainNeed, e.WorldID, sid)
				} else {
					agg(sid).grainTotal += grainNeed
				}
			}
		}

		if disbanded {
			continue
		}

		// ── Silver upkeep ────────────────────────────────────────────────────
		if silverNeed <= 0 {
			continue
		}

		if !hasSid {
			// No paying settlement — treat as unpaid (unknown loyalty → baseline).
			h.recordUnpaid(ctx, u, e.WorldID, defaultLoyalty, sid, silverNeed)
			continue
		}

		// L2: the supplying settlement's loyalty scales desertion severity.
		loyalty := settlementLoyalty(ctx, h.pool, sid)

		// Del C — sold circulation: soldiers standing in the town that pays them
		// spend soldShare of their pay back into it; a unit anywhere else is a full
		// sink — campaigns cost for real.
		//
		// The discriminator is `unit stands in its own paying settlement`, NOT the
		// original plan's "settlement_id set vs null". Migration 100 replaced the
		// silent capital fallback with support_settlement_id as the sole payer, so
		// settlement_id no longer separates garrison from field: since mig 100 the
		// recruitment path sets BOTH columns, and a field unit keeps its raising
		// town as payer while standing elsewhere. Comparing the two columns keeps
		// payer = recipient, which is what the plan chose for the MVP.
		// (Crediting a town the unit garrisons but which does NOT pay it would be
		// the metropolis→colony sold flow — deliberately still deferred; the
		// circulated_to map already carries that variant without a schema change.)
		//
		// The affordability gate stays on the FULL upkeep N (payroll liquidity — the
		// war-chest), while the net debit is only (1−share)·N. Deduct + credit are a
		// SINGLE atomic statement, never two loose Execs. Since eff ≤ cap and
		// credit = share·N ≤ N, eff − N + credit ≤ eff ≤ cap, so the outer LEAST
		// never clips — the credit is always applied in full (no spill), which is
		// why silver_circulated below is exactly `credit`.
		var credit float64
		if u.settlementID != nil && *u.settlementID == sid {
			credit = h.soldShare * silverNeed
		}

		tag, err := h.pool.Exec(ctx,
			`UPDATE settlement_goods
			 SET amount   = LEAST(cap, LEAST(settled(amount, rate, calc_tick), cap) - $1 + $2),
			     calc_tick = current_world_tick()
			 WHERE settlement_id = $3 AND good_key = 'silver'
			   AND LEAST(settled(amount, rate, calc_tick), cap) >= $1`,
			silverNeed, credit, sid,
		)
		if err != nil {
			slog.Error("upkeep: silver deduction failed", "unit", u.id, "err", err)
			continue
		}

		if tag.RowsAffected() > 0 {
			// Paid. Gross = full upkeep N; of that, `credit` circulated back into the
			// garrison town (→ circulated_to at emit) and (1−share)·N left the world.
			a := agg(sid)
			a.unitsPaid++
			a.silverGross += silverNeed
			a.silverCirculated += credit
			a.silverDestroyed += silverNeed - credit
			// Reset unpaid_periods if needed.
			if u.unpaidPeriods > 0 {
				if _, err := h.pool.Exec(ctx,
					`UPDATE units SET unpaid_periods = 0 WHERE id = $1`,
					u.id,
				); err != nil {
					slog.Error("upkeep: reset unpaid_periods failed", "unit", u.id, "err", err)
				}
			}
		} else {
			// Unpaid — silver the town couldn't afford. silver_unpaid keeps this
			// out of silver_destroyed: it never left the world, the gate stopped it.
			a := agg(sid)
			a.unitsUnpaid++
			a.silverUnpaid += silverNeed
			h.recordUnpaid(ctx, u, e.WorldID, loyalty, sid, silverNeed)
		}
	}

	// Silver flow bookkeeping (Del A): one UpkeepSettled per paying settlement,
	// one SilverAudit for the world — both best-effort, after the loop.
	//
	// UpkeepSettled needs no separate claim: aggs is built ONLY from units that
	// passed the per-unit claim above, so on a pure replay of an already fully
	// processed event aggs is empty and the loop below emits nothing. SilverAudit
	// is different — it snapshots the world's CURRENT silver stock unconditionally,
	// independent of aggs, so without its own claim a replay would append a
	// second (redundant) snapshot event even though no unit was re-charged. Scope
	// is a hash of the world id, not the world id itself, so this claim can never
	// collide with a per-unit claim above — the same derived-scope convention
	// borrowed_army.go uses when one event needs more than one independent claim.
	h.emitUpkeepSettled(ctx, e.WorldID, aggs)
	auditScope := uuid.NewSHA1(e.WorldID, []byte("silver_audit"))
	auditClaim, err := h.pool.Exec(ctx,
		`INSERT INTO processed_tick_claims (event_id, scope_id) VALUES ($1, $2)
		 ON CONFLICT DO NOTHING`, e.ID, auditScope)
	if err != nil {
		slog.Error("upkeep: silver audit claim failed", "world", e.WorldID, "err", err)
	} else if auditClaim.RowsAffected() > 0 {
		h.emitSilverAudit(ctx, e.WorldID)
	}

	// 4. Re-enqueue for the next macro-tick cycle.
	return h.scheduler.EnqueueTickRecurring(ctx, e.WorldID, events.ScheduledUpkeepTick,
		upkeepMacroTickPayload{}, e.DueTick, events.MacroTickInterval)
}

// notifyUnitLoss pushes a player-facing notification for grain attrition or
// silver desertion. Before this, these losses were audit-only (an event append
// with no NotifyPlayer), so units starved/deserted to death entirely silently —
// a whole army could evaporate without a single chip (Sparta 5000→300, live
// 2026-07-12). level 2 = the unit was destroyed; 3 = it merely bled men.
func (h *UpkeepHandler) notifyUnitLoss(ctx context.Context, u upkeepUnitRow, worldID, sid uuid.UUID, kind, reason string, lost int, disbanded bool) {
	if h.hub == nil {
		return
	}
	level := 3
	if disbanded {
		level = 2
	}
	// Dedupe (DEL D, megaron_ekonomi_legibilitet_plan.md): a unit bleeding men
	// day after day would otherwise notify every upkeep tick — spam on a sped-up
	// world. Skip if an UNREAD notification of the same kind for this unit already
	// exists. A destruction (disbanded) is never suppressed: it's the outcome the
	// Wanax most needs to see, even if an earlier bleed is still unread.
	if !disbanded {
		var exists bool
		if err := h.pool.QueryRow(ctx,
			`SELECT EXISTS (
			    SELECT 1 FROM notifications
			    WHERE world_id = $1 AND player_id = $2 AND kind = $3 AND read_at IS NULL
			      AND body_json->>'unit_id' = $4
			 )`,
			worldID, u.ownerID, kind, u.id.String(),
		).Scan(&exists); err == nil && exists {
			return
		}
	}
	payload := map[string]any{
		"unit_id":   u.id,
		"unit_type": u.unitType,
		"lost":      lost,
		"disbanded": disbanded,
		"reason":    reason,
	}
	if sid != (uuid.UUID{}) {
		payload["settlement_id"] = sid
	}
	_ = h.hub.NotifyPlayer(ctx, worldID, u.ownerID, kind, level, payload)
}

// notifyUpkeepUnpaid warns the owner that a unit's silver upkeep went unpaid this
// period — the forewarning that used to not exist at all (SLICE A): the counter
// climbed from 1 to 2 with zero player-facing signal until desertion itself fired.
// level matches the file's existing scale (3 = info, 2 = urgent): the final unpaid
// period before desertion (periodsUntilDesertion == 1) is urgent, any earlier one
// is merely informational.
//
// Dedupe is keyed on kind+unit_id+unpaid_periods, NOT kind+unit_id alone like
// notifyUnitLoss above. Each unpaid_periods value is a materially more urgent
// warning than the last (level drops 3→2 on the final period before desertion),
// so an unread period-1 warning must never suppress the period-2 "last chance"
// escalation — that would recreate exactly the silent gap this slice closes. The
// narrower key only suppresses an exact repeat of the same period position (e.g.
// a sped-up world's upkeep tick somehow re-processing one period).
func (h *UpkeepHandler) notifyUpkeepUnpaid(ctx context.Context, u upkeepUnitRow, worldID, sid uuid.UUID, unpaidPeriods, periodsUntilDesertion int, silverNeed float64) {
	if h.hub == nil {
		return
	}
	level := 3
	if periodsUntilDesertion == 1 {
		level = 2
	}

	var exists bool
	if err := h.pool.QueryRow(ctx,
		`SELECT EXISTS (
		    SELECT 1 FROM notifications
		    WHERE world_id = $1 AND player_id = $2 AND kind = $3 AND read_at IS NULL
		      AND body_json->>'unit_id' = $4
		      AND (body_json->>'unpaid_periods')::int = $5
		 )`,
		worldID, u.ownerID, upkeepUnpaidWarningKind, u.id.String(), unpaidPeriods,
	).Scan(&exists); err == nil && exists {
		return
	}

	payload := map[string]any{
		"unit_id":                 u.id,
		"unit_type":               u.unitType,
		"unpaid_periods":          unpaidPeriods,
		"periods_until_desertion": periodsUntilDesertion,
		"silver_unpaid":           silverNeed,
	}
	if sid != (uuid.UUID{}) {
		payload["settlement_id"] = sid
	}
	_ = h.hub.NotifyPlayer(ctx, worldID, u.ownerID, upkeepUnpaidWarningKind, level, payload)
}

// cascadeCargoDisband disbands a ship's embarked cargo unit when the ship itself
// is disbanded (grain attrition or silver desertion). Mirrors collapse.go's
// cargo cascade — without this, a deserted/starved ship's cargo_unit_id points
// at a unit stuck in 'embarked' with no ship, unreachable by march/unload/disband.
//
// Owner notification (r6 audit, 2026-07-24): the ship's own notifyUnitLoss call
// (in applyAttrition/recordUnpaid) reports the SHIP's men lost — a galley's own
// size, typically small — never the embarked land unit's, which is a separate,
// often much larger unit that would otherwise vanish from the owner's roster
// with no explanation at all. reason mirrors the ship's own loss reason
// ("grain_shortage" | "silver_shortage") so the cause reads the same for both
// notifications.
func (h *UpkeepHandler) cascadeCargoDisband(ctx context.Context, worldID, shipID uuid.UUID, cargoUnitID *uuid.UUID, reason string) {
	if cargoUnitID == nil {
		return
	}
	var cargoOwnerID uuid.UUID
	var cargoType string
	var cargoSize int
	if err := h.pool.QueryRow(ctx,
		`SELECT owner_id, type, size FROM units WHERE id = $1`, *cargoUnitID,
	).Scan(&cargoOwnerID, &cargoType, &cargoSize); err != nil {
		slog.Error("upkeep: load cargo unit before disband", "ship", shipID, "cargo", *cargoUnitID, "err", err)
		return
	}
	if _, err := h.pool.Exec(ctx,
		`UPDATE units SET status = 'disbanded', updated_at = now() WHERE id = $1 AND status = 'embarked'`,
		*cargoUnitID,
	); err != nil {
		slog.Error("upkeep: disband cargo unit after ship loss", "ship", shipID, "cargo", *cargoUnitID, "err", err)
		return
	}
	_, _ = h.store.Append(ctx, *cargoUnitID, events.StreamCombat, "UnitLostAtSea",
		map[string]any{
			"unit_id": *cargoUnitID,
			"ship_id": shipID,
			"lost":    cargoSize,
			"reason":  reason,
		},
		worldID, nil,
	)
	if h.hub != nil {
		_ = h.hub.NotifyPlayer(ctx, worldID, cargoOwnerID, "UnitLostAtSea", 2, map[string]any{
			"unit_id":   *cargoUnitID,
			"unit_type": cargoType,
			"lost":      cargoSize,
			"disbanded": true,
			"reason":    reason,
			"ship_id":   shipID,
		})
	}
}

// provisionSourceFor returns the unit whose `provisions` should feed this unit
// this tick, or nil when the unit eats from a settlement as it always has.
//
// Two cases, one rule — "you eat from where you are":
//   - a naval unit eats its own stores;
//   - an embarked cohort eats the stores of the ship carrying it (carrierID),
//     which is why the SELECT resolves the carrier.
//
// Land units on land are never a provision case: they can be reached by runner
// and resupplied, and the right answer there is foraging, not stores
// (megaron_plan_skeppsproviant.md §2 punkt 5).
func provisionSourceFor(u upkeepUnitRow) *uuid.UUID {
	if u.category == "naval" {
		id := u.id
		return &id
	}
	if u.status == "embarked" && u.carrierID != nil {
		return u.carrierID
	}
	return nil
}

// atHomePort reports whether the unit is sitting at a settlement rather than out
// in the world — the only case where falling back to the town's granary is not
// the teleporting logistics this mechanic removes.
func atHomePort(u upkeepUnitRow) bool {
	return u.status == "garrison" || u.status == "repairing"
}

// applyAttrition removes men (land) or crew (naval) from the unit due to grain
// shortage. Land loses upkeepAttritionStepPercent of its SIZE, proportionally —
// a fresh 100-man cohort bleeds more men in absolute terms than an already-thin
// 12-man remnant, and neither is wiped on a single tick. Naval loses CREW, not
// size: a ship's size is its hull count (always 1, pre-Slice-B skeppsreparation)
// so draining it like a land cohort killed the ship outright on the very first
// missed ration. The hull (size) is untouched; the unit is only lost once crew
// reaches 0. See megaron_plan_upkeep_attrition.md.
//
// Returns true if the unit was disbanded. sid = the settlement that failed to feed
// it (uuid.Nil if none), passed through to the notification for deep-linking.
func (h *UpkeepHandler) applyAttrition(ctx context.Context, u upkeepUnitRow, _ float64, worldID, sid uuid.UUID) bool {
	var lost int
	var disbanded bool
	var updateErr error

	if u.category == "naval" {
		lost = navalAttritionCrewStep
		if lost > u.crew {
			lost = u.crew
		}
		newCrew := u.crew - lost
		if newCrew <= 0 {
			_, updateErr = h.pool.Exec(ctx,
				`UPDATE units SET status = 'disbanded', crew = 0, updated_at = now() WHERE id = $1`,
				u.id,
			)
			disbanded = true
		} else {
			_, updateErr = h.pool.Exec(ctx,
				`UPDATE units SET crew = $1, updated_at = now() WHERE id = $2`,
				newCrew, u.id,
			)
		}
	} else {
		lost = int(math.Ceil(float64(u.size) * upkeepAttritionStepPercent / 100))
		if lost > u.size {
			lost = u.size
		}
		newSize := u.size - lost
		if newSize <= 0 {
			_, updateErr = h.pool.Exec(ctx,
				`UPDATE units SET status = 'disbanded', size = 0, updated_at = now() WHERE id = $1`,
				u.id,
			)
			disbanded = true
		} else {
			_, updateErr = h.pool.Exec(ctx,
				`UPDATE units SET size = $1, updated_at = now() WHERE id = $2`,
				newSize, u.id,
			)
		}
	}
	if updateErr != nil {
		slog.Error("upkeep: attrition update failed", "unit", u.id, "err", updateErr)
	}
	if disbanded {
		h.cascadeCargoDisband(ctx, worldID, u.id, u.cargoUnitID, "grain_shortage")
	}

	_, _ = h.store.Append(ctx, u.id, events.StreamCombat, "UnitAttrition",
		map[string]any{
			"unit_id":   u.id,
			"lost":      lost,
			"disbanded": disbanded,
			"reason":    "grain_shortage",
		},
		worldID, nil,
	)
	slog.Info("upkeep: grain attrition", "unit", u.id, "lost", lost, "disbanded", disbanded)
	h.notifyUnitLoss(ctx, u, worldID, sid, "UnitAttrition", "grain_shortage", lost, disbanded)
	return disbanded
}

// recordUnpaid increments unpaid_periods and applies desertion if the threshold is reached.
// loyalty is the supplying settlement's loyalty (L2): lower loyalty ⇒ more men desert.
// sid = the settlement that failed to pay (uuid.Nil if none), for the notification deep-link.
// silverNeed = the silver upkeep that could not be debited this period, for the
// forewarning payload (SLICE A) — best-effort, always available from the caller's
// scope since it's the same amount the failed UPDATE just tried to deduct.
func (h *UpkeepHandler) recordUnpaid(ctx context.Context, u upkeepUnitRow, worldID uuid.UUID, loyalty int, sid uuid.UUID, silverNeed float64) {
	np := u.unpaidPeriods + 1

	if np >= upkeepDesertionTicks {
		// Desertion — severity scales with the supplying settlement's loyalty.
		lost := desertionStepForLoyalty(loyalty)
		if lost > u.size {
			lost = u.size
		}
		newSize := u.size - lost

		var disbanded bool
		var updateErr error
		if newSize <= 0 {
			_, updateErr = h.pool.Exec(ctx,
				`UPDATE units SET status = 'disbanded', size = 0, unpaid_periods = $1, updated_at = now() WHERE id = $2`,
				np, u.id,
			)
			disbanded = true
		} else {
			_, updateErr = h.pool.Exec(ctx,
				`UPDATE units SET size = $1, unpaid_periods = $2, updated_at = now() WHERE id = $3`,
				newSize, np, u.id,
			)
		}
		if updateErr != nil {
			slog.Error("upkeep: desertion update failed", "unit", u.id, "err", updateErr)
		}
		if disbanded {
			h.cascadeCargoDisband(ctx, worldID, u.id, u.cargoUnitID, "silver_shortage")
		}

		_, _ = h.store.Append(ctx, u.id, events.StreamCombat, "UnitDeserted",
			map[string]any{
				"unit_id":   u.id,
				"lost":      lost,
				"disbanded": disbanded,
				"reason":    "silver_shortage",
			},
			worldID, nil,
		)
		slog.Info("upkeep: silver desertion", "unit", u.id, "lost", lost, "disbanded", disbanded)
		h.notifyUnitLoss(ctx, u, worldID, sid, "UnitDeserted", "silver_shortage", lost, disbanded)
	} else {
		// Not yet at threshold — increment the counter AND warn the player now.
		// Before SLICE A (megaron_todo.md, 2026-07-31) this branch was entirely
		// silent: unpaid_periods climbed 1 → 2 with no signal at all, and the
		// player's first notice was the desertion itself once np reached
		// upkeepDesertionTicks. Two full silent days of unpaid upkeep (72 ticks,
		// upkeep runs every tick) is exactly the gap this closes.
		if _, err := h.pool.Exec(ctx,
			`UPDATE units SET unpaid_periods = $1 WHERE id = $2`,
			np, u.id,
		); err != nil {
			slog.Error("upkeep: increment unpaid_periods failed", "unit", u.id, "err", err)
			return
		}

		periodsUntilDesertion := upkeepDesertionTicks - np
		payload := map[string]any{
			"unit_id":                 u.id,
			"unit_type":               u.unitType,
			"unpaid_periods":          np,
			"periods_until_desertion": periodsUntilDesertion,
			"silver_unpaid":           silverNeed,
			"reason":                  "silver_shortage",
		}
		if sid != (uuid.UUID{}) {
			payload["settlement_id"] = sid
		}
		_, _ = h.store.Append(ctx, u.id, events.StreamCombat, upkeepUnpaidWarningKind,
			payload, worldID, nil,
		)
		h.notifyUpkeepUnpaid(ctx, u, worldID, sid, np, periodsUntilDesertion, silverNeed)
	}
}
