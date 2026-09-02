package main

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"formatet/megaron/server/internal/province"
	"formatet/megaron/server/internal/unit"
	"github.com/spf13/cobra"
)

// unusedCatchmentDeposits returns the ore deposit types present in a settlement's
// 7-hex catchment (server-computed top-level "catchment_deposits" on the province
// response) that have no matching extraction building yet — "mine" services both
// copper and tin, "silver_mine" services silver (see api/handlers/province.go's
// build-gate: BuildingType == "mine" || "silver_mine"). Cedar has no mine-equivalent
// gate, so it is intentionally not flagged here.
//
// P1a (soak 2026-07-18): `status` only ever showed Copper/Tin as a PRODUCED good
// (after a mine already existed) — a player who never built one saw no signal that
// an ore sat unused in their own catchment, waiting to be mined.
func unusedCatchmentDeposits(catchmentDeposits []any, buildings []any) []string {
	hasMine := false
	hasSilverMine := false
	for _, it := range buildings {
		m, _ := it.(map[string]any)
		switch m["type"] {
		case "mine":
			hasMine = true
		case "silver_mine":
			hasSilverMine = true
		}
	}
	var unused []string
	for _, d := range catchmentDeposits {
		ds, _ := d.(string)
		switch ds {
		case "copper", "tin":
			if !hasMine {
				unused = append(unused, ds)
			}
		case "silver":
			if !hasSilverMine {
				unused = append(unused, ds)
			}
		}
	}
	return unused
}

// timberBottleneckWarning flags the classic early wall (sondrunda 2026-07-24
// runda 2): timber gates almost all early building — harbour 140, barracks 80,
// foundry 80, temple 60 (internal/province/building.go) — yet nothing in the
// interface named it as the critical early resource. Two probes founded the
// same day: Polydamas never allocated labor to timber → 0 production → fully
// blocked from expansion until trade rescued it; Antenor put 25% on timber →
// ~12k/day → never blocked. Pure so it is unit-testable without a server; rate
// is the settlement's net timber rate per tick (production − consumption).
func timberBottleneckWarning(rate float64) string {
	if rate > 0.01 {
		return ""
	}
	return "⚠ Timber-produktion ~0 — timber gatear hamn (140)/barracks (80)/foundry (80)/temple (60). Allokera labor: `keryx allocate --timber <n>`"
}

// productionHorizonTicks is how far ahead a Wanax is expected to plan
// PRODUCTION — a different question from netUpkeepWarningRunwayTicks, which
// is how urgently a SHORTAGE demands reacting to. Deliberately NOT the same
// constant (Timothy 2026-08-26, correcting the first pass at this slice,
// which reused netUpkeepWarningRunwayTicks here): a future retune of the
// shortage window must not silently move the surplus one, and the two
// questions have different answers — a shortage inside a messenger
// round-trip isn't yet an emergency because there's no time to react, while
// a production surplus is judged against how much a settlement could ever
// plausibly use, a longer view. 60 echoes economy.SitosConfig's own
// GranaryCapDays default (internal/economy/sitos_config.go) — this codebase
// already treats 60 game days as "how much surplus is worth planning
// around" for food; applied here to production sinks generally. A plain
// literal, not a reference to that config (which is env-tunable and lives
// in a package too heavy to import into a CLI, see grainConsumptionPerCitizenPerTick
// below) — so this and the granary's own number can drift apart on purpose,
// never silently in lockstep.
const productionHorizonTicks = 60

// grainConsumptionPerCitizenPerTick mirrors economy.GrainConsumptionPerCitizenPerTick
// (internal/economy/recompute.go) as a literal rather than an import: economy
// pulls clock/events/gossip/hexgrid into what is otherwise a small,
// dependency-free CLI binary, for one float that only ever changes with a
// design conversation, not a code change here.
// 0,5 → 0,005 (mig 136, dagsverkesskalan) — hålls i lås med sin motpart i
// internal/economy/recompute.go enligt dess egen varningskommentar.
const grainConsumptionPerCitizenPerTick = 0.005

// livestockFoodValue mirrors economy's own (unexported, so unimportable
// regardless of package weight) constant of the same name
// (internal/economy/recompute.go — Timothy 2026-08-07: "jag tycker nästan
// att ett kreatur kan få leverera 200 mat om det dödas").
// 200 → 166,67 (mig 136, dagsverkesskalan) — hålls i lås med sin motpart.
const livestockFoodValue = 166.67

// sinkContext bundles what sinkCapacities needs about ONE settlement — all
// of it already present in the /provinces payload `status` already parses,
// so gathering it costs no extra server round trip.
type sinkContext struct {
	population        float64
	buildingLevels    map[string]int // building_type → current level; absent/0 = not built
	wallLevel         int            // 0-3
	armyUpkeepGrain   float64        // this settlement's own current per-tick draw
	armyUpkeepSilver  float64
	templeOilPerTick  float64 // sum of temple_offers[].oil_needed for this settlement
	templeWinePerTick float64 // sum of temple_offers[].wine_needed
}

// remainingBuildingCosts sums the material (+ silver) cost of every building
// level this settlement could still build or upgrade to, FROM ITS CURRENT
// LEVEL — "allt du fortfarande kan bygga här", a flat one-shot number, not a
// rate, so it is not scaled by any horizon.
//
// province.BuildingWall's own BuildingSpecs entry is skipped on purpose: a
// wall is priced exclusively through WallLevelSpecs
// (api/handlers/province.go's BuildHandler branches on
// `req.BuildingType == "wall"` and never charges BuildingSpecs[BuildingWall]),
// and a wall never gets a `buildings` table row either — so without this
// skip, BuildingSpecs' own "not built yet" branch would see a wall as
// eternally unbuilt and double its material cost on top of WallLevelSpecs'
// real one, forever.
func remainingBuildingCosts(buildingLevels map[string]int, wallLevel int) map[string]float64 {
	total := map[string]float64{}
	for bt := range province.BuildingSpecs {
		if bt == province.BuildingWall {
			continue
		}
		maxLvl := 1
		if province.LevelledBuildings[bt] {
			maxLvl = province.MaxBuildingLevel
		}
		cur := buildingLevels[string(bt)]
		for level := cur + 1; level <= maxLvl; level++ {
			spec, ok := province.LevelledSpec(bt, level)
			if !ok {
				continue
			}
			for good, amt := range spec.Costs {
				total[good] += amt
			}
			total["silver"] += spec.CostSilver
		}
	}
	for level := wallLevel + 1; level <= 3; level++ {
		if spec, ok := province.WallLevelSpecs[level]; ok {
			for good, amt := range spec.Costs {
				total[good] += amt
			}
			total["silver"] += spec.CostSilver
		}
	}
	return total
}

// sinkCapacities is the corrected form of the first pass's boolean
// knownSinkGoods (megaron_plan_omfordelningsmatningen.md §3-4; corrected
// 2026-08-26 during review, after the first version's sink set was MEASURED
// against the live goods catalogue rather than reasoned about — the measurement
// showed it warned on fish/livestock and stayed silent on timber, exactly
// inverting the intent. Not a Timothy decision; a review finding):
// a present/absent check cannot
// distinguish a real-but-tiny sink from one that actually matters, and it
// silenced fish/livestock outright even though the population eating them
// is the single biggest sink in the game. Every good gets a SIZE instead —
// `status` then compares that size against the actual stock.
//
// Four sources, added together because they behave differently over time:
//   - Bygge: remainingBuildingCosts above — flat, not scaled by the horizon.
//   - Upkeep: this settlement's OWN current army_upkeep grain/silver draw
//     (server-computed — the same figures `status`'s own Army/Upkeep line
//     shows), × productionHorizonTicks.
//   - Tempel: the "kharis oil/wine" finding kept from the first pass, now
//     sized instead of boolean — oil/wine temple maintenance
//     (kharis.OfferOilPerTemple/OfferWinePerTemple, internal/kharis/tick.go)
//     is a real, deterministic per-tick draw. Read off temple_offers' own
//     oil_needed/wine_needed (already served to `status`) rather than
//     importing kharis, which would pull ai/economy/events/hexgrid/
//     religion/unit into a CLI binary for two floats.
//   - Mat: economy.FoodConsumptionSplit's fallback chain (grain → fish →
//     livestock, internal/economy/recompute.go:417-446) shares ONE demand
//     pool — the population's food need over the horizon
//     (pop × grainConsumptionPerCitizenPerTick × horizon). Each of the
//     three is measured against that SAME total independently rather than
//     a strict running remainder — a documented simplification: modelling
//     the joint chain precisely would need every settlement's grain/fish/
//     livestock STOCK threaded through this function too, for a warning
//     heuristic that does not need that precision. No single one of the
//     three could ever be worth more than the whole population's need, and
//     the case this exists to catch — is the pile absurd relative to what
//     the population could ever eat (Timothy 2026-08-23's fish question) —
//     is answered correctly either way. Livestock is converted to
//     animal-equivalent via livestockFoodValue.
//
// Refining recipes (internal/economy/recipe.go) are NOT sized here — a
// recipe ingredient's real constraint is scarcity of the OTHER input, not
// overproduction, so `status` treats it as an open/unlimited sink instead
// (see the recipe-fetching code at the call site).
func sinkCapacities(ctx sinkContext) map[string]float64 {
	cap := remainingBuildingCosts(ctx.buildingLevels, ctx.wallLevel)

	cap["grain"] += ctx.armyUpkeepGrain * productionHorizonTicks
	cap["silver"] += ctx.armyUpkeepSilver * productionHorizonTicks

	cap["oil"] += ctx.templeOilPerTick * productionHorizonTicks
	cap["wine"] += ctx.templeWinePerTick * productionHorizonTicks

	foodDemand := ctx.population * grainConsumptionPerCitizenPerTick * productionHorizonTicks
	cap["grain"] += foodDemand
	cap["fish"] += foodDemand
	cap["livestock"] += foodDemand / livestockFoodValue

	return cap
}

// openEndedSinkGoods returns the good_keys with an UNBOUNDED sink — today,
// exactly the ingredients of refining recipes (internal/economy/recipe.go,
// DB-seeded, not a Go map), read via the existing GET /api/v1/recipes
// endpoint (RecipeCatalogue) rather than a new one. A recipe ingredient's
// capacity is treated as infinite by the caller: its scarcity comes from the
// OTHER input running out, never from having "too much" of it, so it never
// belongs in the surplus warning regardless of stock size. Best-effort like
// fetchGubbeCountsByGood: a failed or unparsable response just means no
// goods are marked open-ended, never a blocked `status`.
func openEndedSinkGoods(c *Client) map[string]bool {
	open := map[string]bool{}
	data, err := c.get("/api/v1/recipes")
	if err != nil {
		return open
	}
	var recipes []struct {
		Ingredients []struct {
			GoodKey string `json:"good_key"`
		} `json:"ingredients"`
	}
	if json.Unmarshal(data, &recipes) != nil {
		return open
	}
	for _, r := range recipes {
		for _, ing := range r.Ingredients {
			open[ing.GoodKey] = true
		}
	}
	return open
}

// surplusWithoutSinkWarning names a good producing "rakt ned i marken":
// positive rate, at least one gubbe actually placed on it (otherwise there
// is nothing to move — some production is unconditional trickle, not
// placed labour, and telling a Wanax to `place` a gubbe that doesn't exist
// would be an actionable-looking lie), and a stock that EXCEEDS what every
// known sink (capacity, from sinkCapacities — pass math.Inf(1) for a good
// with an open-ended sink such as a recipe ingredient, which then never
// fires) could plausibly absorb.
func surplusWithoutSinkWarning(label string, amount, rateVal, capacity float64, gubbar int) string {
	if rateVal <= 0 || gubbar <= 0 || amount <= capacity {
		return ""
	}
	who := fmt.Sprintf("%d gubbar", gubbar)
	if gubbar == 1 {
		who = "1 gubbe"
	}
	return fmt.Sprintf("⚠ %-8s %6s  %s  — known sinks absorb at most %s within %d tick; %s produces with no receiver (`keryx place`)",
		label, resource(amount), rate(rateVal), resource(capacity), productionHorizonTicks, who)
}

// capitalize upper-cases a good_key's first letter for display ("stone" →
// "Stone"). good_key values are single-word ASCII (production_rules.good_key),
// so byte slicing is safe.
func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// localDone parses an RFC3339 (UTC) completion timestamp and formats it in the
// player's local time, matching `unit march`'s ETA display — a raw UTC string
// like "2026-07-02T04:37:52Z" otherwise forces manual timezone math.
func localDone(iso string) string {
	if t, err := time.Parse(time.RFC3339, iso); err == nil {
		return t.Local().Format("15:04 Jan 2")
	}
	return iso
}

// buildQueueETA formats a build-queue entry's completion. A row STILL in the
// queue whose complete_at has already passed is not usable yet — the
// BuildComplete event (which inserts the buildings row and removes the queue
// entry) hasn't fired. Show "finishing…" rather than a past timestamp that reads
// as done, so `staff foundry` answering "no such building" moments after the
// build no longer surprises. Once the event fires the entry is gone from the queue.
func buildQueueETA(c *Client, iso string) string {
	if t, err := time.Parse(time.RFC3339, iso); err == nil {
		if !t.After(time.Now()) {
			return "finishing…"
		}
		return gameETA(c, t)
	}
	return iso
}

// netUpkeepWarningRunwayTicks names how many ticks of stock buffer counts as
// urgent enough to interrupt a Wanax with a ⚠. Below it, the buffer cannot
// outlast a full messenger round-trip's worth of corrective action (raise
// labor, recruit less, trade) before running dry — above it, a negative net
// is normal balance, not an emergency. One tick is one game day ("Ticket ÄR
// dygnet", canon 2026-08-06) regardless of a world's wall-clock tick pace, so
// this is a game-day figure, not a wall-clock one.
const netUpkeepWarningRunwayTicks = 30

// armyUpkeepWarning grinds the ⚠ line under the post-upkeep Netto row on
// STOCK RUNWAY, not on the sign of the net rate alone (megaron_plan_cli_sanning
// §A; soak 2026-07-24 + 2026-08-23 confirmed independently by two agents):
// `status` said, in the same breath, "⚠ silver täcker inte arméns sold" and
// "lager 30.5k räcker ~29027 tick" (~120 world-years) — Talos motivated a
// grounding with "stop the silver bleed" while holding 30 474 silver. A
// negative net with decades of buffer is an upplysning, not a varning. Grain
// and silver are graded symmetrically. Pure — unit-testable without a server.
func armyUpkeepWarning(netG, netS, grainStock, silverStock float64) string {
	// runway reports how many ticks the stock covers at this net rate (0 when
	// already empty — the most urgent case, not a silently-dropped one) and
	// whether that counts as critical. net >= 0 is never critical regardless
	// of stock — a rate that isn't shrinking the buffer is not an emergency.
	runway := func(net, stock float64) (note string, critical bool) {
		if net >= 0 {
			return "", false
		}
		ticks := 0.0
		if stock > 0 {
			ticks = stock / -net
		}
		note = fmt.Sprintf(" — stock %s lasts ~%.0f tick at this rate", resource(stock), ticks)
		return note, ticks < netUpkeepWarningRunwayTicks
	}
	gNote, gCritical := runway(netG, grainStock)
	sNote, sCritical := runway(netS, silverStock)
	// Name WHICH half is short, and the matching consequence — a city with
	// healthy grain and a critical silver runway should never read as a
	// famine (soak 2026-07-22, two playtesters in a row).
	switch {
	case gCritical && sCritical:
		return "  ⚠ neither grain nor silver covers the army's upkeep — units starve/desert once the stock runs out" + gNote + sNote + " (see `keryx recruit --list`)"
	case gCritical:
		return "  ⚠ grain doesn't cover the army's upkeep — units can starve" + gNote + " (silver is fine; see `keryx recruit --list`)"
	case sCritical:
		return "  ⚠ silver doesn't cover the army's pay — units can desert" + sNote + " (food is fine; see `keryx recruit --list`)"
	}
	return ""
}

// sitosGranaryState names why the granary is doing what it's doing this tick
// (megaron_plan_sitos_utlosning.md §5) — coverage, low and high are the same
// figures `status` already reads off the settlement payload (server-computed
// via economy.CoverageDays/GranaryCap/cfg.LowDays/cfg.HighDays; nothing here
// re-derives a threshold). total is the granary's current stock, net the
// city's food_net_per_tick — needed only to tell an empty-but-filling granary
// apart from an empty-and-draining one; coverage alone is a stock figure and
// says nothing about direction (a newly founded city sits near zero coverage
// while producing a large surplus, and at 60 min/tick that state lasts real
// days). Pure — unit-testable without a server.
func sitosGranaryState(coverage, low, high, total, net float64) string {
	switch {
	case coverage < low && total <= 0 && net > 0:
		return "empty — but the stock is growing, coverage is rising"
	case coverage < low && total <= 0:
		return "EMPTY and the stock is shrinking — the city is unprotected"
	case coverage < low:
		return "releasing food to the city"
	case coverage <= high:
		return "resting — neither storing nor releasing"
	default:
		return "storing away a tenth of the surplus"
	}
}

// multiCityHint (legibility fix, 2026-07-24 — three separate soak rounds):
// `status` shows exactly ONE settlement — the capital by default, or whichever
// `--province <id>` names — and that scope was invisible. It was repeatedly
// misread as the building list wrongly aggregating cities (a probe with two
// cities saw only the capital's temple level and assumed the other city's
// buildings were merged in) and as the Army block contradicting `unit list`'s
// global total (this city's garrison vs. every unit everywhere). Returns ""
// when the player owns only one settlement (settlement_cap.used <= 1) so the
// normal case stays free of this line — no aggregation, no new endpoint,
// purely naming the existing scope.
func multiCityHint(name string, settlementsUsed float64) string {
	if settlementsUsed <= 1 {
		return ""
	}
	return fmt.Sprintf("This is %s. You have %.0f cities — `keryx settlements` lists them, `keryx status --province <id>` shows another.",
		name, settlementsUsed)
}

func statusCmd() *cobra.Command {
	var provinceID string
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show your province status (defaults to your capital; --province inspects a colony)",
		// --json documentation (megaron_plan_cli_sanning §B): reproduced against
		// the live acceptance world 2026-08-24, the raw payload always HAD
		// grain/silver — nested, not at the top level a script naturally
		// guesses. An agent transcript (speldygnstest, friktion_ariadne_sarpedon.md)
		// read `settlement.grain`/`settlement.silver` (no such keys exist) and
		// reported them as "null" — that's a .get()-on-a-missing-key default,
		// not a server bug. This is a documentation gap, not a data gap; name
		// the real path so the next parser doesn't repeat the guess.
		Long: `--json returns the server's raw province payload unmodified. Per-good stock,
rate and cap live NESTED under settlement.resources, not at the settlement
top level:

  settlement.resources.<good>.amount   current stock (e.g. .grain.amount, .silver.amount)
  settlement.resources.<good>.rate     net per tick (production − consumption)
  settlement.resources.<good>.cap      storage cap

There is no top-level settlement.grain / settlement.silver / settlement.storage_cap
field — looking one up there returns nothing, not a real null from the server.
Some derived figures DO live at the settlement top level: grain_prod_rate,
grain_consum_rate, net_grain_per_tick_after_upkeep, net_silver_per_tick_after_upkeep.`,
		Example: `  keryx status
  keryx status --province <province-id>   # inspect a colony
  keryx status --json | jq '.settlement.resources.silver.amount'`,
		Args: rejectPositionalArgs("province"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)
			// Default to the capital; --province lets you inspect any province you own
			// (the server FOW/ownership-gates it), mirroring `build --province`.
			prov := cfg.ProvinceID
			if provinceID != "" {
				resolved, err := resolveProvince(c, cfg.WorldID, provinceID)
				if err != nil {
					return err
				}
				prov = resolved
			}
			// Founder-fas: ingen province än — det vandrande hostet ÄR statusen.
			if prov == "" {
				if jsonMode {
					data, err := c.get(fmt.Sprintf("/api/v1/worlds/%s/founding/status", cfg.WorldID))
					if err != nil {
						return err
					}
					printRawJSON(data)
					return nil
				}
				fp, err := fetchFoundingStatus(c)
				if err != nil {
					return err
				}
				if fp.Active {
					return printFoundingStatus(fp)
				}
				return fmt.Errorf("no province in config and no active founder phase — rejoin the world or set province_id")
			}
			path := fmt.Sprintf("/api/v1/worlds/%s/provinces/%s", cfg.WorldID, prov)
			data, err := c.get(path)
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			var p map[string]any
			if err := json.Unmarshal(data, &p); err != nil {
				return err
			}
			sett, _ := p["settlement"].(map[string]any)
			if sett == nil {
				fmt.Println("No settlement here.")
				return nil
			}
			name, _ := sett["name"].(string)
			culture, _ := sett["culture"].(string)
			pop, _ := sett["population"].(float64)
			labor, _ := sett["labor_pool"].(float64)
			walls, _ := sett["walls"].(float64)
			loyalty, _ := sett["loyalty"].(float64)
			coastal, _ := p["coastal"].(bool)
			coastalNote := ""
			if coastal {
				coastalNote = "  [coastal — can build harbour → ships]"
			}
			settlementsNote := ""
			settlementsUsed := 0.0
			if cap, ok := sett["settlement_cap"].(map[string]any); ok {
				used, _ := cap["used"].(float64)
				max, _ := cap["max"].(float64)
				settlementsNote = fmt.Sprintf("  Settlements: %.0f/%.0f", used, max)
				settlementsUsed = used
			}
			fmt.Printf("%s [%s]  Pop: %s  Labor: %s  Walls: %.0f/3  Loyalty: %.0f%s%s\n",
				name, culture, resource(pop), resource(labor), walls, loyalty, settlementsNote, coastalNote)
			if besieged, _ := sett["besieged"].(bool); besieged {
				fmt.Println("  ⚔ BESIEGED — an enemy holds an access road, catchment production is choked")
			}
			if hint := multiCityHint(name, settlementsUsed); hint != "" {
				fmt.Println(hint)
			}
			fmt.Println("  Loyalty 1–4 (1=lowest; revolt also requires a hostile garrison majority + a triggering event)")
			// P11 (soak 2026-07-18): loyalty had no visible raising lever — colonies
			// sat at 1–2 with no signal why, or what to do about it. The mechanic
			// already exists server-side (internal/loyalty: welfare.go daily
			// kharis/feeding/diet ticks, decay.go neglect, colony.go overextension,
			// borrowed_army.go, plus gift/battle deltas in api/handlers/settlement.go
			// + internal/combat/unit_arrival.go) — it was just never surfaced. Pull
			// the settlement's own loyalty-log (already-existing endpoint, never
			// wired into keryx) so a Wanax sees WHY loyalty moved, not just the number.
			printLoyaltyLog(c, cfg.WorldID, sett)
			fmt.Println()

			// Legibilitet (2026-07-24, playtest sondrundor 2026-07-23/24): a new
			// founder sat with zero trade contacts and nothing here pointed toward
			// the existing way to get one. Reuses the /actions endpoint's "message"
			// verb gate (see noTradeContactsHint in cmd_actions.go) rather than
			// adding a new field — best-effort, never blocks `status`.
			printNoTradeContactsHint(c, cfg.WorldID, prov)

			// Sitos-magasinet: the food the city has set aside, and the number the
			// whole mechanic turns on — ticks of food covered. Coverage is what
			// triggers both legs, so the reserve is printed WITH it and with the
			// two thresholds; a reserve alone would show an answer and hide the
			// question. Always shown, empty granary included: "0 undan" on a city
			// at 4 ticks' coverage is exactly the state a Wanax must be able to see.
			if sitos, ok := sett["sitos"].(map[string]any); ok {
				total, _ := sitos["granary_total"].(float64)
				gcap, _ := sitos["granary_cap"].(float64)
				cov, _ := sitos["coverage_ticks"].(float64)
				low, _ := sitos["low_ticks"].(float64)
				high, _ := sitos["high_ticks"].(float64)
				parts := ""
				if pg, ok := sitos["granary_per_good"].(map[string]any); ok && len(pg) > 0 {
					keys := make([]string, 0, len(pg))
					for k := range pg {
						keys = append(keys, k)
					}
					sort.Strings(keys)
					for _, k := range keys {
						v, _ := pg[k].(float64)
						if v <= 0 {
							continue
						}
						if parts != "" {
							parts += ", "
						}
						parts += fmt.Sprintf("%s %s", resource(v), k)
					}
					if parts != "" {
						parts = " (" + parts + ")"
					}
				}
				// Coverage is a STOCK figure and says nothing about direction. A
				// newly founded city sits near zero coverage while producing a
				// large surplus, and at 60 min/tick that state lasts real days —
				// so "empty, no help coming" would cry famine over a city that is
				// filling up fast. The net food rate is what tells them apart.
				net, _ := sitos["food_net_per_tick"].(float64)
				state := sitosGranaryState(cov, low, high, total, net)
				fmt.Printf("Sitos granary: %s stored%s / cap %s · coverage %.1f tick (granary fills above %.0f, drains below %.0f) · %s\n\n",
					resource(total), parts, resource(gcap), cov, high, low, state)
			}

			// "Last tick" summary: summarizes the journal (keryx ticklog)
			// without replacing it.
			if lt, ok := sett["last_tick"].(map[string]any); ok {
				tk, _ := lt["tick"].(float64)
				sitosInterventions, _ := lt["sitos_interventions"].(float64)
				sitosFoodIn, _ := lt["sitos_food_in"].(float64)
				sitosFoodOut, _ := lt["sitos_food_out"].(float64)
				prodN := 0
				if p, ok := lt["production"].(map[string]any); ok {
					prodN = len(p)
				}
				consN := 0
				if c2, ok := lt["consumption"].(map[string]any); ok {
					consN = len(c2)
				}
				// Sitos itemization: what MOVED, in food. There is no silver leg
				// left to net out (mig 106), so the short form is "granary was idle"
				// rather than a delta of zero silver — a number that would now
				// always be 0 and tell the Wanax nothing.
				sitosNote := "Sitos granary was idle"
				if sitosInterventions > 0 {
					detail := ""
					if sitosFoodIn > 0 {
						detail = fmt.Sprintf("city received %s food from the granary", resource(sitosFoodIn))
					}
					if sitosFoodOut > 0 {
						if detail != "" {
							detail += " / "
						}
						detail += fmt.Sprintf("stored away %s food", resource(sitosFoodOut))
					}
					if detail == "" {
						detail = "no food moved"
					}
					sitosNote = detail
				}
				fmt.Printf("Last tick (%d): %d goods produced, %d consumed, %s  ·  keryx ticklog for details\n\n",
					int(tk), prodN, consN, sitosNote)
			}

			// Resources: silver + the bronze-chain goods live in resources as
			// {amount,rate,cap} objects; kharis is the per-Wanax pool exposed at the
			// settlement top level. Silver always prints (even 0); grain + the metals
			// print when present so a colony's tin/copper output is visible here, not
			// only via `goods`.
			fmt.Println("Resources")
			fmt.Println("  (rate = net: production − consumption, per tick)")
			if res, ok := sett["resources"].(map[string]any); ok {
				printRes := func(label, key string, always bool) {
					rd, ok := res[key].(map[string]any)
					if !ok {
						return
					}
					amt, _ := rd["amount"].(float64)
					rt, _ := rd["rate"].(float64)
					if always || amt > 0 || rt != 0 {
						line := fmt.Sprintf("  %-8s %6s  %s", label, resource(amt), rate(rt))
						if rt < 0 {
							line += " net"
							// Real shortage risk: current stock runs out within the next
							// tick at this net rate — most negative nettos are a stable
							// balance a stock buffer absorbs, not an emergency (DEL C
							// grain-netto-märkning: don't cry wolf).
							if amt/-rt < 1 {
								line += "  ⚠ runs out within one tick"
							}
						}
						fmt.Println(line)
					}
				}
				printRes("Silver", "silver", true)

				// Grain: itemized prod/konsum/netto per TICK (DEL C fuller fix,
				// GREENLIT 2026-07-12) instead of one unmarked netto rate — a lone
				// negative number reads as an alarm when it's often just normal
				// balance. Since Utfodringsordningen D1 (megaron_plan_utfodringsordningen.md,
				// 2026-08-26) the stored rate itself is raw, un-netted production —
				// the population's food is debited once a day from STOCK by FoodTick,
				// not folded into this rate — so grain_prod_rate/grain_consum_rate are
				// the status endpoint's own economy.GrainBalance (D6) split, not a
				// re-derivation of the mechanic here.
				if gRd, ok := res["grain"].(map[string]any); ok {
					gAmt, _ := gRd["amount"].(float64)
					gProdRate, _ := sett["grain_prod_rate"].(float64)
					gConsumRate, _ := sett["grain_consum_rate"].(float64)
					if gAmt > 0 || gProdRate != 0 || gConsumRate != 0 {
						prodTick := gProdRate
						consumTick := gConsumRate
						netTick := prodTick - consumTick
						line := fmt.Sprintf("  %-8s %6s  prod %.1f − consum %.1f = net %+.1f /tick",
							"Grain", resource(gAmt), prodTick, consumTick, netTick)
						// food_gubbar_required/placed/self_sufficient (P4-arvet,
						// megaron_plan_p4_arvet_i_province.md §2) replace the old
						// weight-based break-even hint: how many gubbar the catchment's
						// food slots need, out of SAME greedy loop founding/growth use.
						if req, ok := sett["food_gubbar_required"].(float64); ok {
							if suff, ok2 := sett["food_self_sufficient"].(bool); ok2 && !suff {
								line += fmt.Sprintf("  ⚠ the catchment doesn't feed the population even with all %d gubbar", int(req))
							} else {
								placed, _ := sett["food_gubbar_placed"].(float64)
								line += fmt.Sprintf("  (%d gubbar needed for food · %d placed)", int(req), int(placed))
							}
						}
						fmt.Println(line)
					}
				}

				// Netto EFTER arméns upkeep (P6, soak 2026-07-18): grain/silver "netto"
				// above is citizens only — army upkeep is a separate once-daily debit
				// (keryx ticklog), never folded into that rate. A galley disbanded the
				// instant it garrisoned in a city whose grain netto looked healthy. This
				// line is the number to check BEFORE `recruit`/building another ship —
				// `recruit --list` shows the same math per unit type.
				if netG, ok := sett["net_grain_per_tick_after_upkeep"].(float64); ok {
					netS, _ := sett["net_silver_per_tick_after_upkeep"].(float64)
					grainStock := 0.0
					if rd, ok := res["grain"].(map[string]any); ok {
						grainStock, _ = rd["amount"].(float64)
					}
					silverStock := 0.0
					if rd, ok := res["silver"].(map[string]any); ok {
						silverStock, _ = rd["amount"].(float64)
					}
					warn := armyUpkeepWarning(netG, netS, grainStock, silverStock)
					fmt.Printf("  %-8s %+.1f grain/tick, %+.1f silver/tick (after the army's upkeep)%s\n",
						"Net", netG, netS, warn)
				}

				// Fish (fisk-föder-befolkningen, 2026-07-31): a coastal/river
				// city's fish covers whatever grain does not reach — surfaced
				// here so a Wanax can see the second half of their food
				// balance, not just grain's already-netted line above.
				printRes("Fish", "fish", false)
				printRes("Timber", "timber", false)
				printRes("Stone", "stone", false)
				printRes("Copper", "copper", false)
				printRes("Tin", "tin", false)
				printRes("Bronze", "bronze", false)

				// Timber bottleneck (sondrunda 2026-07-24 runda 2): see
				// timberBottleneckWarning — printRes above only shows the Timber line
				// at all when amount or rate is non-zero, so a city with true 0%
				// timber labor got no timber line AND no warning. Check the rate
				// directly so the wall is named before a Wanax hits it.
				timberRate := 0.0
				if td, ok := res["timber"].(map[string]any); ok {
					timberRate, _ = td["rate"].(float64)
				}
				if w := timberBottleneckWarning(timberRate); w != "" {
					fmt.Printf("  %s\n", w)
				}

				// "Produktion utan mottagare" (megaron_plan_omfordelningsmatningen.md
				// §3-4, corrected 2026-08-26 — see sinkCapacities' doc comment for
				// why a present/absent check was replaced by a sized one). Iterates
				// every good_key res actually holds, so wine/oil/cedar/etc. get the
				// same treatment as any other good, no per-good special-casing here.
				buildingLevels := map[string]int{}
				if bs, ok := sett["buildings"].([]any); ok {
					for _, it := range bs {
						m, _ := it.(map[string]any)
						t, _ := m["type"].(string)
						lvl, _ := m["level"].(float64)
						if t != "" {
							buildingLevels[t] = int(lvl)
						}
					}
				}
				armyUpkeepGrain, armyUpkeepSilver := 0.0, 0.0
				if up, ok := sett["army_upkeep"].(map[string]any); ok {
					armyUpkeepGrain, _ = up["grain"].(float64)
					armyUpkeepSilver, _ = up["silver"].(float64)
				}
				templeOil, templeWine := 0.0, 0.0
				if temples, ok := sett["temple_offers"].([]any); ok {
					for _, it := range temples {
						m, _ := it.(map[string]any)
						oilN, _ := m["oil_needed"].(float64)
						wineN, _ := m["wine_needed"].(float64)
						templeOil += oilN
						templeWine += wineN
					}
				}
				capacities := sinkCapacities(sinkContext{
					population:        pop,
					buildingLevels:    buildingLevels,
					wallLevel:         int(walls),
					armyUpkeepGrain:   armyUpkeepGrain,
					armyUpkeepSilver:  armyUpkeepSilver,
					templeOilPerTick:  templeOil,
					templeWinePerTick: templeWine,
				})
				openSinks := openEndedSinkGoods(c)
				capacityFor := func(k string) float64 {
					if openSinks[k] {
						return math.Inf(1)
					}
					return capacities[k]
				}

				surplusGoods := make([]string, 0, len(res))
				for k := range res {
					surplusGoods = append(surplusGoods, k)
				}
				sort.Strings(surplusGoods)
				var candidates []string
				for _, k := range surplusGoods {
					rd, ok := res[k].(map[string]any)
					if !ok {
						continue
					}
					amt, _ := rd["amount"].(float64)
					rt, _ := rd["rate"].(float64)
					if rt > 0 && amt > capacityFor(k) {
						candidates = append(candidates, k)
					}
				}
				if len(candidates) > 0 {
					gubbeCounts := fetchGubbeCountsByGood(c, cfg.WorldID, prov)
					for _, k := range candidates {
						rd, _ := res[k].(map[string]any)
						amt, _ := rd["amount"].(float64)
						rt, _ := rd["rate"].(float64)
						if line := surplusWithoutSinkWarning(capitalize(k), amt, rt, capacityFor(k), gubbeCounts[k]); line != "" {
							fmt.Printf("  %s\n", line)
						}
					}
				}
			}
			// Obruten deposit i catchmenten (P1a, soak 2026-07-18): se
			// unusedCatchmentDeposits — flaggar koppar/tenn/silver som ligger i
			// stadens 7-hex catchment men saknar mine/silver_mine.
			if cd, ok := p["catchment_deposits"].([]any); ok {
				buildings, _ := sett["buildings"].([]any)
				if unused := unusedCatchmentDeposits(cd, buildings); len(unused) > 0 {
					fmt.Printf("  ⚠ Unmined deposit in the catchment: %s — build a mine/silver_mine here to extract it\n",
						strings.Join(unused, ", "))
				}
			}
			// Kharis (PLAN B, megaron_kult_legibilitet_plan.md): kharis is now
			// DAILY-maintenance-driven, not per-tick — a per-tick rate rendered
			// "+0.0/tick" for any typical passive value (A4a-buggen). Show the mood
			// (gynnsamhets-signal, never a computed odds — see `rite --list`) and the
			// passive geographic rate per TICK instead.
			kv, _ := sett["kharis"].(float64)
			mood, _ := sett["kharis_mood"].(string)
			kpd, _ := sett["kharis_per_tick"].(float64)
			kcap, _ := sett["kharis_cap"].(float64)
			mtl, _ := sett["max_temple_level"].(float64)
			knet, _ := sett["kharis_net_per_tick"].(float64)
			netKnown, _ := sett["kharis_net_known"].(bool)
			// The DAILY MAINTENANCE net (temple gain − decay) is what actually moves
			// kharis — the passive geographic rate alone hid a fading L1 Wanax behind
			// "passiv +0.1/tick" (sondrunda 2026-07-24). Show the net when we have it.
			netStr := fmt.Sprintf("passive %+.1f/tick", kpd)
			if netKnown {
				netStr = fmt.Sprintf("net %+.1f/tick (temple − decay)", knet)
			}
			if kcap > 0 {
				fmt.Printf("  %-8s %6s  (%s) · cap %.0f · %s\n", "Kharis", resource(kv), mood, kcap, netStr)
			} else {
				fmt.Printf("  %-8s %6s  (%s) · %s\n", "Kharis", resource(kv), mood, netStr)
			}
			// Legibilitet (2026-07-24): ett L1-tempel HÅLLER standing men klättrar inte
			// förbi sitt tak — utan denna rad läser en spelare "kharis fastnat på 22" som
			// en bugg (sondrunda 2026-07-24). Taket = 25×(1+nivå): L1=50, L2=75, L3=100.
			if mtl >= 1 && kcap < 100 {
				line := fmt.Sprintf("  → the cap %.0f is set by your level-%.0f temple; your kharis holds but doesn't climb — raise cult labor (`keryx allocate --cult`) or build a higher temple.\n", kcap, mtl)
				if netKnown && knet > 0.05 {
					line = fmt.Sprintf("  → your kharis is climbing toward the cap %.0f (set by your level-%.0f temple) — build a higher temple to raise the cap further.\n", kcap, mtl)
				} else if netKnown && knet < -0.05 {
					line = fmt.Sprintf("  → the cap %.0f is set by your level-%.0f temple, but your kharis is falling toward the floor — raise cult labor (if the temple has room) or build a higher temple.\n", kcap, mtl)
				}
				fmt.Print(line)
			}

			// Kult: per tempel-stad, dagens offer-krav vs oil/vin-lager — svarar
			// direkt på "kommer min kharis klättra idag" utan att vänta på tick.
			if idle, _ := sett["kharis_devotion_idle"].(bool); idle {
				fmt.Println("  → your temple can employ MORE devotion than you've allocated — raise cult labor (`keryx allocate --cult <%>`) to fill it and let kharis climb.")
			}
			if temples, ok := sett["temple_offers"].([]any); ok {
				if len(temples) == 0 {
					fmt.Println("  Temples: none — kharis doesn't climb without a temple + offerings.")
				}
				anyUnfed := false
				for _, it := range temples {
					m, _ := it.(map[string]any)
					name, _ := m["name"].(string)
					oil, _ := m["oil"].(float64)
					wine, _ := m["wine"].(float64)
					oilNeeded, _ := m["oil_needed"].(float64)
					wineNeeded, _ := m["wine_needed"].(float64)
					fed, _ := m["fed"].(bool)
					mark := "✓"
					if !fed {
						mark = "✗"
						anyUnfed = true
					}
					fmt.Printf("  Temple in %s: needs %.0f oil + %.0f wine/tick — stock: oil %s, wine %s  %s\n",
						name, oilNeeded, wineNeeded, resource(oil), resource(wine), mark)
				}
				if mood == "Suspicious" || mood == "Wrathful" || anyUnfed {
					fmt.Println("  → feed the temples (build up oil/wine) or cast a rite — see `keryx rite --list`.")
				}
			}
			fmt.Println()

			army, _ := sett["army"].(map[string]any)
			if army != nil {
				fmt.Printf("Army (garrison in %s)\n", name)
				// jsonKey = province.ArmyComposition's Go field name (no JSON tags,
				// so it serializes verbatim); dbType feeds the shared display map.
				units := []struct{ jsonKey, dbType string }{
					{"Spearman", "spearman"}, {"WarChariot", "war_chariot"},
					{"Ship", "galley"}, {"EliteInfantry", "elite_infantry"},
					{"WarGalley", "war_galley"}, {"Merchantman", "merchantman"},
				}
				// Kohortuppdelning (A16, 2026-07-25): each line above is a SUM across
				// every garrisoned unit of that type (api/handlers/db.go
				// loadSettlement) — "Spearman 100" can silently be two separate
				// 100-cap cohorts, only visible today by cross-referencing `unit
				// list` by hand (settlement + status). One fetch for the whole Army
				// block, not per type — see fetchGarrisonCohorts.
				settlementID, _ := sett["id"].(string)
				cohortsByType := fetchGarrisonCohorts(c, cfg.WorldID, settlementID)
				for _, u := range units {
					v, _ := army[u.jsonKey].(float64)
					if v > 0 {
						fmt.Printf("  %-10s %4.0f\n", unit.DisplayName(u.dbType), v)
						for _, line := range cohortLines(cohortsByType[u.dbType]) {
							fmt.Println(line)
						}
					}
				}
				// Upkeep this city pays each tick — every unit it supports, wherever it
				// stands (grain shortage → attrition, silver shortage → desertion).
				// Same figures the upkeep tick debits.
				if up, ok := sett["army_upkeep"].(map[string]any); ok {
					g, _ := up["grain"].(float64)
					s, _ := up["silver"].(float64)
					if g > 0 || s > 0 {
						// Say what the figure covers. It sits directly under a list of
						// the units STANDING here, but since mig 100 it is the bill for
						// everything this city supports — so a Wanax with half the army
						// in the field reads "100 spearmen, upkeep for 200" and thinks
						// the number is broken.
						fmt.Printf("  %-10s %.1f grain, %.1f silver / tick  (everything the city pays — including field units)\n", "Upkeep", g, s)
						// Del C: soldiers standing in the town that pays them spend
						// their sold there. Shown as its own line because it is the
						// only reason the net below is not gross — an invisible flow
						// is one the Wanax can neither plan for nor exploit.
						if circ, ok := sett["army_upkeep_circulated_silver"].(float64); ok && circ > 0 {
							fmt.Printf("  %-10s %.1f silver / tick back into the city (garrison's pay)\n", "", circ)
						}
					}
				}
				// Legibilitet (2026-07-24): namnge att detta bara är garnisonen HÄR —
				// `unit list` räknar hela rostret (fält, andra städer, ockuperade ruiner).
				fmt.Println("  Field units and other cities are shown in `keryx unit list`.")
			}

			// Completed buildings — so the agent doesn't re-queue what already exists.
			if bs, ok := sett["buildings"].([]any); ok && len(bs) > 0 {
				fmt.Println("\nBuildings")
				for _, it := range bs {
					m, _ := it.(map[string]any)
					t, _ := m["type"].(string)
					lvl, _ := m["level"].(float64)
					fmt.Printf("  %-12s L%.0f\n", t, lvl)
				}
			}

			if bq, ok := sett["build_queue"].([]any); ok && len(bq) > 0 {
				fmt.Println("\nConstruction")
				for _, it := range bq {
					m, _ := it.(map[string]any)
					t, _ := m["type"].(string)
					ca, _ := m["complete_at"].(string)
					fmt.Printf("  %-12s %s\n", t, buildQueueETA(c, ca))
				}
			}

			if tus, ok := sett["training_units"].([]any); ok && len(tus) > 0 {
				// One line per maturing unit: forming (gathering men), training
				// (full at 100, counting down to garrison), or naval building.
				fmt.Println("\nTraining")
				for _, it := range tus {
					m, _ := it.(map[string]any)
					u, _ := m["unit"].(string)
					sz, _ := m["size"].(float64)
					status, _ := m["status"].(string)
					cat, _ := m["category"].(string)
					name := unit.DisplayName(u)
					ready := ""
					if ra, ok := m["ready_at"].(string); ok && ra != "" {
						// arrivalETA (game-days-first), not localDone — this is an
						// ETA, not history like the loyalty log below. English,
						// matching every other ETA surface in keryx (rad K).
						ready = " — ready " + arrivalETA(c, ra)
					}
					switch {
					case cat == "naval":
						fmt.Printf("  %-10s building%s\n", name, ready)
					case status == "training":
						fmt.Printf("  %-10s %.0f/100 · training%s\n", name, sz, ready)
					default: // forming
						fmt.Printf("  %-10s %.0f/100 · forming (%.0f left to recruit)\n", name, sz, 100-sz)
					}
				}
			}
			return nil
		},
	}
	cmd.Flags().SortFlags = false
	cmd.Flags().StringVar(&provinceID, "province", "", "province ID to inspect (default: your capital)")
	return cmd
}

// loyaltyLogEntry mirrors one row of the settlement loyalty-log endpoint
// (api/handlers/settlement.go SettlementHandler.LoyaltyLog, wired at GET
// .../settlements/:id/loyalty-log in cmd/server/main.go).
type loyaltyLogEntry struct {
	EventType    string `json:"event_type"`
	LoyaltyDelta int    `json:"loyalty_delta"`
	Reason       string `json:"reason"`
	CreatedAt    string `json:"created_at"`
}

// loyaltyLegend is the static "what moves this number" explainer (P11, soak
// 2026-07-18: "loyalty stuck at 1-2, no visible raising mechanic"). The
// mechanic already exists server-side — daily welfare ticks for kharis/
// feeding/diet variety (internal/loyalty/welfare.go), neglect decay
// (decay.go), colony overextension (colony.go), borrowed-army penalties
// (borrowed_army.go), plus instant gift (api/handlers/settlement.go Gift)
// and battle deltas (internal/combat/unit_arrival.go applyBattleLoyalty) — it
// was just never surfaced to a Wanax. This legend names the actual levers so
// `status` teaches the mechanic instead of just showing a stuck number.
const loyaltyLegend = "  Raised by: kharis ≥ favor threshold, fed/varied diet (daily welfare tick), " +
	"gifts ≥50 silver-equivalent (`keryx transfer`), won/defended battles.\n" +
	"  Lowered by: starvation, too many colonies (overextension), neglect (>2 days without a gift), " +
	"lost battles, a borrowed army kept too long."

// formatLoyaltyLog turns a settlement's loyalty-log entries into the lines
// `status` prints under the Loyalty line — most recent first, capped at 5,
// always followed by loyaltyLegend so the mechanic is explained even when a
// fired event's reason string is terse. Pure — no DB, no HTTP — so this is
// unit-testable without a live server.
func formatLoyaltyLog(entries []loyaltyLogEntry) []string {
	if len(entries) == 0 {
		return []string{"  No loyalty events yet.", loyaltyLegend}
	}
	lines := []string{"  Recent loyalty events:"}
	n := 5
	if len(entries) < n {
		n = len(entries)
	}
	for _, e := range entries[:n] {
		lines = append(lines, fmt.Sprintf("    %+d  %-20s %s (%s)", e.LoyaltyDelta, e.EventType, e.Reason, localDone(e.CreatedAt)))
	}
	lines = append(lines, loyaltyLegend)
	return lines
}

// printNoTradeContactsHint fetches this province's /actions capabilities and
// prints noTradeContactsHint's message (cmd_actions.go) when this Wanax has
// zero foreign settlements in vision yet — best-effort, mirroring
// printLoyaltyLog: never blocks `status` if the request or parse fails.
func printNoTradeContactsHint(c *Client, worldID, prov string) {
	data, err := c.get(fmt.Sprintf("/api/v1/worlds/%s/provinces/%s/actions", worldID, prov))
	if err != nil {
		return
	}
	var verbs []actionVerb
	if err := json.Unmarshal(data, &verbs); err != nil {
		return
	}
	if hint := noTradeContactsHint(verbs); hint != "" {
		fmt.Println(hint)
		fmt.Println()
	}
}

// printLoyaltyLog fetches and prints this settlement's recent loyalty-changing
// events, falling back to the static legend alone when the settlement ID is
// missing, the request fails, or the response doesn't parse — best-effort,
// never blocks `status`.
func printLoyaltyLog(c *Client, worldID string, sett map[string]any) {
	settlementID, _ := sett["id"].(string)
	if settlementID == "" {
		fmt.Println(loyaltyLegend)
		return
	}
	data, err := c.get(fmt.Sprintf("/api/v1/worlds/%s/settlements/%s/loyalty-log", worldID, settlementID))
	if err != nil {
		fmt.Println(loyaltyLegend)
		return
	}
	var entries []loyaltyLogEntry
	if jerr := json.Unmarshal(data, &entries); jerr != nil {
		fmt.Println(loyaltyLegend)
		return
	}
	for _, line := range formatLoyaltyLog(entries) {
		fmt.Println(line)
	}
}

// fetchGarrisonCohorts reads every unit the player owns (GET
// /worlds/{worldID}/units — same endpoint `keryx unit list` uses) and groups
// this settlement's garrison-status units by type. This is the split hidden
// behind each Army aggregate line: province.go's SUM(size) (api/handlers/
// db.go loadSettlement) filters on the identical status='garrison', so a
// correct grouping's per-type Size sum always equals the aggregate number
// printed above it — if a live server ever disagrees, that is a bug in this
// function or in the aggregate, not something to paper over.
//
// Best-effort like fetchDevotionShare (cmd_allocate.go): returns nil on any
// failure (empty settlementID, request error, bad JSON). `status` is a
// read-only view; one extra endpoint failing must never block it — it just
// loses the cohort breakdown and falls back to the aggregate-only line.
func fetchGarrisonCohorts(c *Client, worldID, settlementID string) map[string][]unitRow {
	if settlementID == "" {
		return nil
	}
	data, err := c.get(fmt.Sprintf("/api/v1/worlds/%s/units", worldID))
	if err != nil {
		return nil
	}
	var resp struct {
		Units []unitRow `json:"units"`
	}
	if json.Unmarshal(data, &resp) != nil {
		return nil
	}
	out := make(map[string][]unitRow)
	for _, u := range resp.Units {
		if u.Status != "garrison" || u.SettlementID == nil || *u.SettlementID != settlementID {
			continue
		}
		out[u.Type] = append(out[u.Type], u)
	}
	return out
}

// fetchGubbeCountsByGood tallies settlement_placement rows by good_key, via
// GET .../placements — the same endpoint `keryx place`/`staff` already write
// through (api/handlers/settlement_placement.go Placements). Reused as-is
// rather than adding a new query: P1's postmortem found the same catchment
// query duplicated at 13 call sites from not checking first
// (megaron_plan_sten_stock.md §5).
//
// Best-effort like fetchGarrisonCohorts above: returns nil on any failure, so
// a transient error degrades the ceiling warning to "gubbe count unknown"
// (prints 0) rather than blocking the read-only status view.
func fetchGubbeCountsByGood(c *Client, worldID, provinceID string) map[string]int {
	if provinceID == "" {
		return nil
	}
	data, err := c.get(fmt.Sprintf("/api/v1/worlds/%s/provinces/%s/placements", worldID, provinceID))
	if err != nil {
		return nil
	}
	var resp struct {
		Placements []struct {
			GoodKey string `json:"good_key"`
		} `json:"placements"`
	}
	if json.Unmarshal(data, &resp) != nil {
		return nil
	}
	counts := make(map[string]int)
	for _, pl := range resp.Placements {
		counts[pl.GoodKey]++
	}
	return counts
}

// cohortLines formats the per-cohort breakdown shown under an Army aggregate
// line. Returns nil for 0 or 1 cohorts: a single cohort already equals the
// aggregate line above it, so a sub-line would repeat that number without
// saying anything new — only ≥2 cohorts is new information, and only ≥2 is
// worth the extra noise in the common case.
//
// Shows the full unit ID (not truncated) because it is the identifier
// `keryx unit march --unit <id>` takes — the whole point of naming the split
// is that a Wanax (or LLM agent) can act on ONE cohort, so the line must be
// directly pasteable, not just legible. Ships also carry a Name
// (internal/unit/model.go: Name is set only for naval units); shown after
// the ID when present, for a human reading at a glance.
func cohortLines(cohorts []unitRow) []string {
	if len(cohorts) < 2 {
		return nil
	}
	sorted := make([]unitRow, len(cohorts))
	copy(sorted, cohorts)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Size != sorted[j].Size {
			return sorted[i].Size > sorted[j].Size
		}
		return sorted[i].ID < sorted[j].ID
	})
	lines := make([]string, 0, len(sorted))
	for _, u := range sorted {
		line := fmt.Sprintf("      %-36s %4d", u.ID, u.Size)
		if u.Name != nil && *u.Name != "" {
			line += "  " + *u.Name
		}
		lines = append(lines, line)
	}
	return lines
}
