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
// response) that have no matching extraction building yet — "mine" now services
// copper, tin AND silver (silver_mine is retired, megaron_plan_byggnad_pa_hex.md
// §A: a mine on a silver-deposit hex produces silver, exactly like copper/tin on
// their own hexes). Cedar has no mine-equivalent gate, so it is intentionally not
// flagged here.
//
// This stays a settlement-wide "any mine anywhere" check, same as before mine
// became hex-bound — catchment_deposits itself is an aggregate (any deposit
// anywhere in the ring), so a per-hex-accurate answer isn't derivable from the
// data this already has; it remains a heads-up, not a precise claim.
//
// P1a (soak 2026-07-18): `status` only ever showed Copper/Tin as a PRODUCED good
// (after a mine already existed) — a player who never built one saw no signal that
// an ore sat unused in their own catchment, waiting to be mined.
func unusedCatchmentDeposits(catchmentDeposits []any, buildings []any) []string {
	hasMine := false
	for _, it := range buildings {
		m, _ := it.(map[string]any)
		if m["type"] == "mine" {
			hasMine = true
		}
	}
	var unused []string
	for _, d := range catchmentDeposits {
		ds, _ := d.(string)
		switch ds {
		case "copper", "tin", "silver":
			if !hasMine {
				unused = append(unused, ds)
			}
		}
	}
	return unused
}

// incenseInCatchmentLine tells the Wanax that incense lies in the catchment: a
// rare good needing no building, only gubbar on its hex, that the gods prize.
// "" when the deposit list holds none.
func incenseInCatchmentLine(catchmentDeposits []any) string {
	for _, d := range catchmentDeposits {
		if ds, _ := d.(string); ds == "incense" {
			return "Incense lies in the catchment - no building needed: place gubbar on its hex (`keryx place incense <ordinal>`, see `keryx city`). Offered at a rite it moves the odds further than any other gift."
		}
	}
	return ""
}

// timberBottleneckWarning flags the classic early wall (sondrunda 2026-07-24
// runda 2): timber gates almost all early building (internal/province/building.go) — yet nothing in the
// interface named it as the critical early resource. Two probes founded the
// same day: Polydamas never allocated labor to timber → 0 production → fully
// blocked from expansion until trade rescued it; Antenor put 25% on timber →
// ~12k/day → never blocked. Pure so it is unit-testable without a server; rate
// is the settlement's net timber rate per tick (production − consumption).
func timberBottleneckWarning(rate float64) string {
	if rate > 0.01 {
		return ""
	}
	var costs []string
	for _, kind := range []province.BuildingType{province.BuildingHarbour, province.BuildingBarracks, province.BuildingFoundry, province.BuildingTemple} {
		costs = append(costs, fmt.Sprintf("%s (%g)", kind, province.BuildingSpecs[kind].Costs["timber"]))
	}
	return "⚠ Timber production ~0 — timber gates " + strings.Join(costs, "/") + ". Place workers on a timber hex: `keryx place timber <ordinal>` (see `keryx city`)."
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
// plausibly use, a longer view. This production planning horizon is independent
// of the window used for urgent upkeep warnings.
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
// 1 per djur (Timothy 2026-10-31; var 3 i order 33, 1 i order 28b, före det 166,67) — hålls i lås med sin motpart.
const livestockFoodValue = 1

// sinkContext bundles what sinkCapacities needs about ONE settlement — all
// of it already present in the /provinces payload `status` already parses,
// so gathering it costs no extra server round trip.
type sinkContext struct {
	population            float64
	buildingLevels        map[string]int // building_type → current level; absent/0 = not built
	hexSites              *hexBuildSites // nil = placement-options unreadable → hex-bound types fall back to buildingLevels
	wallLevel             int            // 0-3
	armyUpkeepGrain       float64        // this settlement's own current per-tick draw
	armyUpkeepSilver      float64
	templeOfferingPerTick map[string]float64 // planned local debits from temple_offers[].offering
}

// localTempleOffering counts only this city's planned gift. The status payload
// includes realm-wide temple rows; another city's goods are not a local sink.
func localTempleOffering(sett map[string]any) map[string]float64 {
	take := map[string]float64{}
	id, _ := sett["id"].(string)
	temples, _ := sett["temple_offers"].([]any)
	for _, item := range temples {
		row, _ := item.(map[string]any)
		cityID, _ := row["settlement_id"].(string)
		if id == "" || cityID != id {
			continue
		}
		goods, _ := row["offering"].(map[string]any)
		for good, value := range goods {
			if amount, ok := value.(float64); ok && amount > 0 {
				take[good] += amount
			}
		}
	}
	return take
}

func templeGoodsText(raw any, separator string) string {
	goods, _ := raw.(map[string]any)
	keys := make([]string, 0, len(goods))
	for key := range goods {
		keys = append(keys, key)
	}
	order := map[string]int{"grain": 0, "fish": 1, "timber": 2, "silver": 3}
	sort.Slice(keys, func(i, j int) bool {
		a, aOK := order[keys[i]]
		b, bOK := order[keys[j]]
		if aOK != bOK {
			return aOK
		}
		if aOK && a != b {
			return a < b
		}
		return keys[i] < keys[j]
	})
	parts := []string{}
	for _, good := range keys {
		amount, ok := goods[good].(float64)
		if ok {
			parts = append(parts, fmt.Sprintf("%.4g %s", amount, good))
		}
	}
	if len(parts) == 0 {
		return "—"
	}
	return strings.Join(parts, separator)
}

func templeOfferingLine(row map[string]any) string {
	name, _ := row["name"].(string)
	temple, _ := row["temple"].(string)
	if temple == "" {
		temple = "Temple"
	}
	fed, _ := row["fed"].(bool)
	mark, next := "✗", "unfunded (nothing taken)"
	if fraction, _ := row["paid_fraction"].(float64); fed || fraction > 0 {
		mark, next = "✓", templeGoodsText(row["offering"], " + ")
	}
	if parts, ok := row["parts"].([]any); ok {
		for _, raw := range parts {
			if part, ok := raw.(map[string]any); ok {
				state := "missing"
				if part["paid"] == true {
					state = "paid"
				}
				next += fmt.Sprintf(" · %v %s", part["good"], state)
			}
		}
		next += fmt.Sprintf(" · %.0f%% of tithe · weight %v, gives %v", row["paid_fraction"].(float64)*100, row["weight"], row["effective_weight"])
	}
	// Each temple has its own gift; a city's temples take from one store in turn,
	// so "stock" is what the temples before this one left.
	return fmt.Sprintf("  %s in %s: reference %s per game day — alternatives: %s — stock: %s — next gift: %s  %s",
		temple, name, templeGoodsText(row["required_goods"], " + "), templeGoodsText(row["alternatives"], " or "),
		templeGoodsText(row["stock"], ", "), next, mark)
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
//
// A hex-bound type (farm/mine/lumbermill/stonequarry) is not one building
// per city but one per hex (megaron_plan_byggnad_pa_hex.md §A): with sites
// given, it sums the remaining ladder of EVERY standing one plus the full
// ladder on every hex it could still go (placement-options'
// valid_hexes_for_building) — Timothy 2026-09-30. Without sites it falls back
// to buildingLevels, one per type.
func remainingBuildingCosts(buildingLevels map[string]int, wallLevel int, sites *hexBuildSites) map[string]float64 {
	total := map[string]float64{}
	for bt := range province.BuildingSpecs {
		if bt == province.BuildingWall {
			continue
		}
		maxLvl := 1
		if province.LevelledBuildings[bt] {
			maxLvl = province.MaxBuildingLevel
		}
		ladder := func(cur int) {
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
		if sites != nil && province.HexBoundBuildings[bt] {
			for _, cur := range sites.built[string(bt)] {
				ladder(cur)
			}
			for i := 0; i < sites.free[string(bt)]; i++ {
				ladder(0)
			}
			continue
		}
		ladder(buildingLevels[string(bt)])
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

// hexBuildSites is where this settlement's hex-bound buildings stand and could
// still stand, read off GET .../placement-options.
type hexBuildSites struct {
	built map[string][]int // type → level of each standing one
	free  map[string]int   // type → number of hexes it could still be built on
}

// hexBuildSitesFrom reads hexBuildSites off a placement-options response.
func hexBuildSitesFrom(resp *placementOptionsResp) *hexBuildSites {
	sites := &hexBuildSites{built: map[string][]int{}, free: map[string]int{}}
	for _, h := range resp.Hexes {
		if h.Building != nil {
			sites.built[h.Building.Type] = append(sites.built[h.Building.Type], h.Building.Level)
		}
	}
	for typ, hexes := range resp.ValidHexesForBuilding {
		sites.free[typ] = len(hexes)
	}
	return sites
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
//   - Upkeep: this settlement's OWN current army_upkeep food/silver draw
//     (server-computed — the same figures `status`'s own Army/Upkeep line
//     shows), × productionHorizonTicks.
//   - Tempel: the server's planned local offering, × productionHorizonTicks.
//     Each city's own temple_offers[].offering specifies the actual goods,
//     including mixed substitutions at the world's divine values. An unfunded
//     gift has no planned debit. No client-side recipe or valuation is needed.
//   - Food: each interchangeable food good can meet the full demand pool.
//     These are independent upper-bound sink capacities, not a forecast of
//     the actual proportional meal. Livestock covers only civilian emergency
//     meals, converted via livestockFoodValue. Army food adds capacity for
//     grain, fish and oil, never livestock. Wine is not food (mig 224).
//
// Refining recipes (internal/economy/recipe.go) are NOT sized here — a
// recipe ingredient's real constraint is scarcity of the OTHER input, not
// overproduction, so `status` treats it as an open/unlimited sink instead
// (see the recipe-fetching code at the call site).
func sinkCapacities(ctx sinkContext) map[string]float64 {
	cap := remainingBuildingCosts(ctx.buildingLevels, ctx.wallLevel, ctx.hexSites)

	for _, good := range foodDisplayGoods[:3] {
		cap[good] += ctx.armyUpkeepGrain * productionHorizonTicks
	}
	cap["silver"] += ctx.armyUpkeepSilver * productionHorizonTicks

	for good, amount := range ctx.templeOfferingPerTick {
		cap[good] += amount * productionHorizonTicks
	}

	foodDemand := ctx.population * grainConsumptionPerCitizenPerTick * productionHorizonTicks
	for _, good := range foodDisplayGoods[:3] {
		cap[good] += foodDemand
	}
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
	// Haiku-8 read "5 citizens … with no receiver" as idle citizens; name the good they work.
	who := fmt.Sprintf("%d citizens on %s make", gubbar, strings.ToLower(label))
	if gubbar == 1 {
		who = fmt.Sprintf("1 citizen on %s makes", strings.ToLower(label))
	}
	return fmt.Sprintf("⚠ %-8s %6s  production %s  — %s more than known sinks absorb (at most %s within %s); move some with `keryx place` or trade the surplus", label, resource(amount), rate(rateVal), who, resource(capacity), formatDays(productionHorizonTicks, "%d"))
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

// foodGubbarLine reports minimum food staffing and actual placements.
// A minimum on the best slots does not identify removable workers.
func foodGubbarLine(req, placed int, selfSufficient bool) string {
	if !selfSufficient {
		return fmt.Sprintf("  ⚠ the catchment doesn't feed the population even with all %s", citizenCount(req))
	}
	return fmt.Sprintf("  (%s needed for people on best food slots · %d on food)", citizenCount(req), placed)
}

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
		note = fmt.Sprintf(" — stock %s lasts ~%s at this rate", resource(stock), formatDays(ticks, "%.0f"))
		return note, ticks < netUpkeepWarningRunwayTicks
	}
	gNote, gCritical := runway(netG, grainStock)
	sNote, sCritical := runway(netS, silverStock)
	// Name WHICH half is short, and the matching consequence — a city with
	// healthy grain and a critical silver runway should never read as a
	// famine (soak 2026-07-22, two playtesters in a row).
	switch {
	case gCritical && sCritical:
		return "  ⚠ neither grain nor silver covers the army's upkeep — forces starve/desert once the stock runs out" + gNote + sNote + " (see `keryx recruit --list`)"
	case gCritical:
		return "  ⚠ grain doesn't cover the army's upkeep — forces can starve" + gNote + " (silver is fine; see `keryx recruit --list`)"
	case sCritical:
		return "  ⚠ silver doesn't cover the army's pay — forces can desert" + sNote + " (food is fine; see `keryx recruit --list`)"
	}
	return ""
}

// wallsHoldLine says what the walls hold (the population ceiling) and which
// Granary the next wall needs. Only the ceiling — never "room for N more". ""
// when the server did not report population_cap. The web drawer
// (city_production.js wallsHoldText) says the same words.
func wallsHoldLine(popCap, walls, siloLevel float64) string {
	if popCap <= 0 {
		return ""
	}
	line := fmt.Sprintf("Walls hold %s.", resource(popCap))
	if walls < 3 {
		line += fmt.Sprintf(" The next wall needs a Granary of level %.0f (yours: %.0f) — `keryx build --type granary`.", walls+1, siloLevel)
	}
	return line
}

// growthLine says in one sentence what the population is doing and why. state
// and net are the server's growth_state / food_net_per_tick (the tick gate's
// own inputs); "" when the server did not report a state. The web drawer
// (city_production.js growthLineText) says the same words.
func growthLine(pop float64, state string, net float64) string {
	display := fmt.Sprintf("%+.1f", net)
	if math.Abs(net) < 0.05 && net != 0 {
		display = "about even"
	}
	switch state {
	case "growing":
		return fmt.Sprintf("Population %.0f — growing: food net %s per game day", pop, display)
	case "holding":
		return fmt.Sprintf("Population %.0f — not growing: food net %s per game day (needs more than 0)", pop, display)
	case "capped":
		return fmt.Sprintf("Population %.0f — not growing: the walls are full", pop)
	case "shrinking":
		return fmt.Sprintf("Population %.0f — shrinking: hunger", pop)
	}
	return ""
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

// nextGameDayLine says when the next game day begins, in wall-clock terms,
// from the world anchor the client already fetched (the anchor is the wall
// time of the world's current tick, so the next one is current+1). Empty when
// the anchor is unknown — no invented time. Display only: `now` is wall clock,
// the sanctioned keryx-display exception.
func nextGameDayLine(c *Client, now time.Time) string {
	if c == nil {
		return ""
	}
	if _, ok := c.TickSeconds(); !ok || c.worldTick == nil {
		return ""
	}
	at, ok := c.WorldTickETA(*c.worldTick + 1)
	if !ok {
		return ""
	}
	left := at.Sub(now)
	if left <= 0 {
		return "Next game day: due now"
	}
	mins := int(math.Ceil(left.Minutes()))
	in := fmt.Sprintf("%d min", mins)
	if mins >= 60 {
		in = fmt.Sprintf("%d h %d min", mins/60, mins%60)
	}
	return fmt.Sprintf("Next game day: %s (in %s)", clockTimeAt(at, now), in)
}

func statusCmd() *cobra.Command {
	var provinceID string
	cmd := &cobra.Command{
		Use:   "status [city]",
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
  settlement.resources.<good>.rate     stored production rate per game day; meals and army upkeep are separate
  settlement.resources.<good>.cap      storage cap

There is no top-level settlement.grain / settlement.silver / settlement.storage_cap
field — looking one up there returns nothing, not a real null from the server.
Some derived figures DO live at the settlement top level: grain_prod_rate,
grain_consum_rate, net_grain_per_tick_after_upkeep, net_silver_per_tick_after_upkeep.`,
		Example: `  keryx status
  keryx status --province <province-id>   # inspect a colony
  keryx status --json | jq '.settlement.resources.silver.amount'`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := newClient(cfg)
			if len(args) > 0 {
				if provinceID != "" {
					return fmt.Errorf("choose a city either positionally or with --province")
				}
				provinceID = args[0]
			}
			// Player report 4eb54d52 ("no surface addresses me by name"): the very
			// top of status, ahead of the founder/settlement branch below, so it
			// shows for a nomadic host too. Suppressed in --json mode, which emits
			// the server's raw payload verbatim.
			if !jsonMode {
				printWanaxGreeting(c, cfg.WorldID)
			}
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
				return fmt.Errorf("no city and no host in this world — if your last city fell, run: keryx rise (or keryx leave); if you never joined, run: keryx join")
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
			if _, foreign := sett["intel"]; foreign {
				for _, line := range foreignCityLines(sett, time.Now()) {
					fmt.Println(line)
				}
				return nil
			}
			name, _ := sett["name"].(string)
			culture, _ := sett["culture"].(string)
			pop, _ := sett["population"].(float64)
			labor, _ := sett["labor_pool"].(float64)
			walls, _ := sett["walls"].(float64)
			popCap, _ := sett["population_cap"].(float64)
			siloLevel, _ := sett["silo_level"].(float64)
			coastal, _ := p["coastal"].(bool)
			coastalNote := ""
			if coastal {
				coastalNote = "  [coastal — ships need a harbour and a shipyard]"
			}
			settlementsNote := ""
			settlementsUsed := 0.0
			if cap, ok := sett["settlement_cap"].(map[string]any); ok {
				used, _ := cap["used"].(float64)
				max, _ := cap["max"].(float64)
				settlementsNote = fmt.Sprintf("  Settlements: %.0f/%.0f", used, max)
				settlementsUsed = used
			}
			fmt.Println(citizenScaleNote)
			fmt.Printf("%s [%s]  Pop: %s people  Labor: %s  Walls: %.0f/3%s%s\n",
				name, culture, resource(pop), citizenCount(int(math.Floor(labor/100))), walls, settlementsNote, coastalNote)
			if line := wallsHoldLine(popCap, walls, siloLevel); line != "" {
				fmt.Println(line)
			}
			if food, ok := sett["food"].(map[string]any); ok {
				gs, _ := food["growth_state"].(string)
				net, _ := food["food_net_per_tick"].(float64)
				if line := growthLine(pop, gs, net); line != "" {
					fmt.Println(line)
				}
			}
			if line := nextCitizenLine(c, sett); line != "" {
				fmt.Println(line)
			}
			if besieged, _ := sett["besieged"].(bool); besieged {
				fmt.Println("  ⚔ BESIEGED — an enemy holds an access road, catchment production is choked")
			}
			if hint := multiCityHint(name, settlementsUsed); hint != "" {
				fmt.Println(hint)
			}
			fmt.Println()

			// Legibilitet (2026-07-24, playtest sondrundor 2026-07-23/24): a new
			// founder sat with zero trade contacts and nothing here pointed toward
			// the existing way to get one. Reuses the /actions endpoint's "message"
			// verb gate (see noTradeContactsHint in cmd_actions.go) rather than
			// adding a new field — best-effort, never blocks `status`.
			printNoTradeContactsHint(c, cfg.WorldID, prov)

			if line := harvestBlessingLine(sett); line != "" {
				fmt.Println(line)
			}
			if line := stormFishingLine(sett); line != "" {
				fmt.Println(line)
			}

			fmt.Println(cityFoodLine(sett))
			if line := herdLine(sett); line != "" {
				fmt.Println(line)
			}
			for _, line := range storageWarningLines(sett) {
				fmt.Println(line)
			}
			fmt.Println()

			if line := nextGameDayLine(c, time.Now()); line != "" {
				fmt.Println(line)
			}

			// "Last tick" summary: summarizes the journal (keryx ticklog)
			// without replacing it.
			if lt, ok := sett["last_tick"].(map[string]any); ok {
				tk, _ := lt["tick"].(float64)
				prodN := 0
				if p, ok := lt["production"].(map[string]any); ok {
					prodN = len(p)
				}
				consN := 0
				if c2, ok := lt["consumption"].(map[string]any); ok {
					consN = len(c2)
				}
				fmt.Printf("Last %s: %d goods produced, %d consumed  ·  keryx ticklog for details\n\n",
					formatWorldTime(c, tk), prodN, consN)
			}

			// Resources: silver + the bronze-chain goods live in resources as
			// {amount,rate,cap} objects; kharis is the per-Wanax pool exposed at the
			// settlement top level. Silver always prints (even 0); grain + the metals
			// print when present so a colony's tin/copper output is visible here, not
			// only via `goods`.
			fmt.Println("Resources")
			fmt.Println("  (per game day; food and army balances are itemized below)")
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
								line += "  ⚠ runs out within one game day"
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
				if line := peopleFoodLine(sett); line != "" {
					fmt.Println(line)
					food, _ := sett["food"].(map[string]any)
					if tick, known := food["consumed_tick"].(float64); known {
						consumed, _ := food["consumed"].(map[string]any)
						detail := foodGoodsText(consumed)
						if detail == "" {
							detail = "none"
						}
						fmt.Printf("  Last meal: %s · %s\n", detail, formatWorldTime(c, tick))
					}
				}
				if gRd, ok := res["grain"].(map[string]any); ok {
					gAmt, _ := gRd["amount"].(float64)
					gProdRate, _ := sett["grain_prod_rate"].(float64)
					gConsumRate, _ := sett["grain_consum_rate"].(float64)
					if gAmt > 0 || gProdRate != 0 || gConsumRate != 0 {
						prodTick := gProdRate
						consumTick := gConsumRate
						netTick := prodTick - consumTick
						fish := 0.0
						if row, ok := res["fish"].(map[string]any); ok {
							fish, _ = row["rate"].(float64)
						}
						netTick += fish
						if food, ok := sett["food"].(map[string]any); ok {
							if full, ok := food["food_net_per_tick"].(float64); ok {
								netTick = full
							}
						}
						if peopleFoodLine(sett) == "" {
							fmt.Printf("  Food     grain %.1f + fish %.1f − people eat %.1f = net %s per game day (before army)\n", prodTick, fish, consumTick, formatNetPerTick(netTick))
						}
						line := fmt.Sprintf("  %-8s %6s  production %.1f per game day",
							"Grain", resource(gAmt), prodTick)
						// food_gubbar_required/placed/self_sufficient (P4-arvet,
						// megaron_plan_p4_arvet_i_province.md §2) replace the old
						// weight-based break-even hint: how many gubbar the catchment's
						// food slots need, out of SAME greedy loop founding/growth use.
						if req, ok := sett["food_gubbar_required"].(float64); ok {
							placed, _ := sett["food_gubbar_placed"].(float64)
							suff, _ := sett["food_self_sufficient"].(bool)
							line += foodGubbarLine(int(req), int(placed), suff)
							if automatic := automaticFoodLine(sett); automatic != "" {
								line += "\n  " + automatic
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
				if _, ok := sett["net_grain_per_tick_after_upkeep"].(float64); ok {
					netS, _ := sett["net_silver_per_tick_after_upkeep"].(float64)
					grainStock := 0.0
					if rd, ok := res["grain"].(map[string]any); ok {
						grainStock, _ = rd["amount"].(float64)
					}
					silverStock := 0.0
					if rd, ok := res["silver"].(map[string]any); ok {
						silverStock, _ = rd["amount"].(float64)
					}
					up, _ := sett["army_upkeep"].(map[string]any)
					armyGrain, _ := up["grain"].(float64)
					grainProduced, _ := sett["grain_prod_rate"].(float64)
					// New servers report the shared food balance and itemized stock.
					warn := armyUpkeepWarning(grainProduced-armyGrain, netS, grainStock, silverStock)
					if _, shared := up["food"]; shared {
						food, _ := sett["food"].(map[string]any)
						stocks, _ := food["goods"].(map[string]any)
						stock := 0.0
						for _, good := range foodDisplayGoods[:3] {
							amount, _ := stocks[good].(float64)
							stock += amount
						}
						if net, known := sett["food_net_per_tick_after_upkeep"].(float64); known {
							warn = strings.ReplaceAll(armyUpkeepWarning(net, netS, stock, silverStock), "grain", "food")
						} else {
							warn = ""
						}
					}
					fmt.Println(armyFoodLine(sett))
					fmt.Printf("  Silver   net %+.1f per game day after army pay\n", netS)
					if warn != "" {
						fmt.Println(warn)
					}
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
					armyUpkeepGrain, _ = upkeepFood(up)
					armyUpkeepSilver, _ = up["silver"].(float64)
				}
				templeOffering := localTempleOffering(sett)
				var hexSites *hexBuildSites
				if po, err := fetchPlacementOptions(c, cfg.WorldID, prov); err == nil {
					hexSites = hexBuildSitesFrom(po)
				}
				capacities := sinkCapacities(sinkContext{
					population:            pop,
					buildingLevels:        buildingLevels,
					hexSites:              hexSites,
					wallLevel:             int(walls),
					armyUpkeepGrain:       armyUpkeepGrain,
					armyUpkeepSilver:      armyUpkeepSilver,
					templeOfferingPerTick: templeOffering,
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
					if isFoodGood(k) {
						continue
					} // Food placement advice has one source: foodGubbarLine.
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
			// stadens catchment men saknar mine. silver_mine avskaffad
			// (megaron_plan_byggnad_pa_hex.md §A) — mine på en silverhex ger
			// silver, precis som koppar/tenn, så det finns bara ett meddelande nu.
			if cd, ok := p["catchment_deposits"].([]any); ok {
				buildings, _ := sett["buildings"].([]any)
				if unused := unusedCatchmentDeposits(cd, buildings); len(unused) > 0 {
					fmt.Printf("  ⚠ Unmined deposit in the catchment: %s — build a mine on its hex to extract it (a hex holds one building; if another stands there, demolish it first with `keryx demolish`)\n",
						strings.Join(unused, ", "))
				}
			}
			if cd, ok := p["catchment_deposits"].([]any); ok {
				if line := incenseInCatchmentLine(cd); line != "" {
					fmt.Printf("  %s\n", line)
				}
			}
			// Kharis is realm-wide: show authoritative net and the city cost separately.
			kv, _ := sett["kharis"].(float64)
			mood, _ := sett["kharis_mood"].(string)
			kcap, _ := sett["kharis_cap"].(float64)
			knet, _ := sett["kharis_net_per_tick"].(float64)
			netKnown, _ := sett["kharis_net_known"].(bool)
			// The DAILY MAINTENANCE net (temple gain − decay) is what actually moves
			// kharis — the passive geographic rate alone hid a fading L1 Wanax behind
			// "passiv +0.1/tick" (sondrunda 2026-07-24). Show the net when we have it.
			netStr := "net not reported"
			if netKnown {
				netStr = fmt.Sprintf("net %+.1f per game day (temple − decay)", knet)
				if kcap > 0 && kv > kcap {
					netStr = fmt.Sprintf("net %+.1f per game day (upkeep only above ceiling)", knet)
				}
			}
			if kcap > 0 {
				fmt.Printf("  %-8s %6s  (%s) · ceiling %.0f · %s\n", "Kharis", resource(kv), mood, kcap, netStr)
			} else {
				fmt.Printf("  %-8s %6s  (%s) · %s\n", "Kharis", resource(kv), mood, netStr)
			}
			if standing, ok := sett["kharis_standing"].(map[string]any); ok {
				if short, ok := standing["short"].(string); ok {
					fmt.Println("  " + short)
				}
			}
			if line := kharisDecayLine(sett); line != "" {
				fmt.Println("  " + line)
			}
			if production, ok := sett["kharis_production_per_tick"].(float64); ok {
				if netKnown {
					fmt.Printf("  Temple production: %.2f kharis per game day\n", production)
				}
			}
			if worker, ok := sett["kharis_per_temple_worker"].(float64); ok && !(kcap > 0 && kv > kcap) {
				fmt.Printf("  Per extra temple worker: +%.2f\n", worker)
			}
			if threshold, ok := sett["kharis_blessing_threshold"].(float64); ok {
				fmt.Printf("  Blessings from kharis %.0f\n", threshold)
			}
			if wrath, ok := sett["kharis_wrath"].(map[string]any); ok {
				threshold, _ := wrath["threshold"].(float64)
				probability, _ := wrath["daily_probability"].(float64)
				wrathful := ""
				if wt, ok := wrath["wrathful_threshold"].(float64); ok {
					wp, _ := wrath["wrathful_probability"].(float64)
					wrathful = fmt.Sprintf(", %.0f%% below %.0f", wp*100, wt)
				}
				fmt.Printf("  Wrath risk below %.0f: %.0f%% / game day%s\n", threshold, probability*100, wrathful)
				if remaining, _ := wrath["protection_remaining_days"].(float64); remaining > 0 {
					if iso, ok := wrath["protection_until"].(string); ok {
						if at, err := time.Parse(time.RFC3339Nano, iso); err == nil {
							fmt.Printf("  Protected from divine punishment for %s · until %s. After that the daily risk returns while kharis is below %.0f.\n", formatDuration(c, remaining, "%g"), clockTime(at), threshold)
						}
					}
				}
			}
			if line := kharisCapLine(sett); line != "" {
				fmt.Print(line)
			}

			realmCultShown := printRealmTemples(c)

			// Kult: each temple city's reference, stock and planned local gift — answers
			// direkt på "kommer min kharis klättra idag" utan att vänta på tick.

			if temples, ok := sett["temple_offers"].([]any); ok && !realmCultShown {
				if len(temples) == 0 {
					fmt.Println("  Temples: none — kharis doesn't climb without a temple + offerings.")
				}
				anyUnfed := false
				for _, it := range temples {
					m, _ := it.(map[string]any)
					fed, _ := m["fed"].(bool)
					anyUnfed = anyUnfed || !fed
					fmt.Println(templeOfferingLine(m))
				}
				if mood == "Suspicious" || mood == "Wrathful" || anyUnfed {
					fmt.Println("  → feed the temples with local grain, fish, timber or silver or cast a rite — see `keryx rite --list`.")
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
					{"Ship", "galley"}, {"Bronze Guard", "elite_infantry"},
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
					g, label := upkeepFood(up)
					s, _ := up["silver"].(float64)
					if g > 0 || s > 0 {
						// Say what the figure covers. It sits directly under a list of
						// the units STANDING here, but since mig 100 it is the bill for
						// everything this city supports — so a Wanax with half the army
						// in the field reads "100 spearmen, upkeep for 200" and thinks
						// the number is broken.
						fmt.Printf("  %-10s %s %s, %s silver per game day  (everything the city pays — including field forces)\n", "Upkeep", trimFloat(g), label, trimFloat(s))
					}
				}
				// Legibilitet (2026-07-24): namnge att detta bara är garnisonen HÄR —
				// `unit list` räknar hela rostret (fält, andra städer, ockuperade ruiner).
				fmt.Println("  Field forces and other cities are shown in `keryx unit list`.")
			}

			// Completed buildings — so the agent doesn't re-queue what already exists.
			if bs, ok := sett["buildings"].([]any); ok && len(bs) > 0 {
				fmt.Println("\nBuildings")
				for _, it := range bs {
					m, _ := it.(map[string]any)
					t := buildingLabel(m)
					lvl, _ := m["level"].(float64)
					fmt.Printf("  %-12s L%.0f\n", t, lvl)
				}
			}

			if bq, ok := sett["build_queue"].([]any); ok && len(bq) > 0 {
				fmt.Println("\nConstruction")
				for _, it := range bq {
					m, _ := it.(map[string]any)
					t := buildingLabel(m)
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
						// ETA rather than history. English,
						// matching every other ETA surface in keryx (rad K).
						ready = " — ready " + arrivalETA(c, ra)
					}
					switch {
					case m["shipyard_waiting"] == true:
						fmt.Printf("  %-10s waiting for shipyard workers\n", name)
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

type wanaxMeResp struct {
	WanaxName string `json:"wanax_name"`
}

// fetchMe calls GET /auth/me and returns the Wanax's display name, or "" on
// any failure — best-effort, same pattern as fetchFoodStatus: a name greeting
// must never block `status`.
func fetchMe(c *Client) string {
	data, err := c.get("/api/v1/auth/me")
	if err != nil {
		return ""
	}
	var me wanaxMeResp
	if err := json.Unmarshal(data, &me); err != nil {
		return ""
	}
	return me.WanaxName
}

// printWanaxGreeting addresses the Wanax and shows the current world anchor in clock time.
func printWanaxGreeting(c *Client, worldID string) {
	name := fetchMe(c)
	if name == "" {
		return
	}
	if tick, ok := fetchWorldTick(c, worldID); ok {
		fmt.Printf("Wanax %s · %s\n\n", name, formatWorldTime(c, tick))
		return
	}
	fmt.Printf("Wanax %s\n\n", name)
}

// fetchWorldTick reads the world's current tick (1 tick = 1 day on player surfaces).
func fetchWorldTick(c *Client, worldID string) (int, bool) {
	if worldID == "" {
		return 0, false
	}
	data, err := c.get("/api/v1/worlds/" + worldID)
	if err != nil {
		return 0, false
	}
	var w struct {
		CurrentTick *int `json:"current_tick"`
	}
	if err := json.Unmarshal(data, &w); err != nil || w.CurrentTick == nil {
		return 0, false
	}
	return *w.CurrentTick, true
}

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

func armyFoodLine(sett map[string]any) string {
	up, _ := sett["army_upkeep"].(map[string]any)
	grain, label := upkeepFood(up)
	net, ok := sett["food_net_per_tick_after_upkeep"].(float64)
	if !ok {
		food, _ := sett["food"].(map[string]any)
		before, known := food["food_net_per_tick"].(float64)
		if !known {
			return "  Army food balance not reported"
		}
		net = before - grain
	}
	return fmt.Sprintf("  Army     eats %s %s per game day → food net %s per game day after army", trimFloat(grain), label, formatNetPerTick(net))
}

// kharisDecayLine renders the server's realm projection without a client formula.
// Missing fields are unavailable, never an invented zero cost.
func kharisDecayLine(sett map[string]any) string {
	if demand, ok := sett["kharis_demand"].(float64); ok {
		if given, ok := sett["kharis_given_weight"].(float64); ok {
			return fmt.Sprintf("Demand: %g · Given: %.2f", demand, given)
		}
	}

	total, totalOK := sett["kharis_daily_decay"].(float64)
	if !totalOK {
		return "Cult upkeep not reported"
	}
	return fmt.Sprintf("Upkeep: −%.2f / game day", total)

}

// kharisCapLine explains the temple cap. Legibilitet (2026-07-24): ett L1-tempel
// HÅLLER standing men klättrar inte förbi sitt tak — utan denna rad läser en spelare
// "kharis fastnat på 22" som en bugg (sondrunda 2026-07-24). Haiku-8: at the cap the
// net still reads positive, so "climbing" must not be said once the cap is reached.
func kharisCapLine(sett map[string]any) string {
	kv, _ := sett["kharis"].(float64)
	kcap, _ := sett["kharis_cap"].(float64)
	mtl, _ := sett["max_temple_level"].(float64)
	knet, _ := sett["kharis_net_per_tick"].(float64)
	netKnown, _ := sett["kharis_net_known"].(bool)
	if kcap > 0 && kv > kcap {
		return fmt.Sprintf("  → Above ceiling (%.0f)\n", kcap)
	}
	if mtl < 1 || kcap >= 100 {
		return ""
	}
	switch {
	case netKnown && knet > 0.05 && kv >= kcap-0.05:
		return fmt.Sprintf("  → your kharis is at the ceiling %.0f (set by your level-%.0f temple) — build a higher temple to raise the ceiling further.\n", kcap, mtl)
	case netKnown && knet > 0.05:
		return fmt.Sprintf("  → your kharis is climbing toward the ceiling %.0f (set by your level-%.0f temple) — build a higher temple to raise the ceiling further.\n", kcap, mtl)
	case netKnown && knet < -0.05:
		if idle, _ := sett["kharis_devotion_idle"].(bool); idle {
			return fmt.Sprintf("  → ceiling %.0f is set by your level-%.0f temple; kharis is declining. Your temple has empty workplaces: assign a free citizen with `keryx staff temple +1` and supply its daily offering.\n", kcap, mtl)
		}
		return fmt.Sprintf("  → ceiling %.0f is set by your level-%.0f temple; kharis is declining. Check temple staffing and daily offering supplies in `keryx city`.\n", kcap, mtl)
	}
	if idle, _ := sett["kharis_devotion_idle"].(bool); idle {
		return fmt.Sprintf("  → ceiling %.0f is set by your level-%.0f temple; kharis holds. Your temple has empty workplaces: assign a free citizen with `keryx staff temple +1` and supply its daily offering.\n", kcap, mtl)
	}
	return fmt.Sprintf("  → ceiling %.0f is set by your level-%.0f temple; kharis holds. Check temple staffing and daily offering supplies in `keryx city`.\n", kcap, mtl)
}

// buildingLabel names a temple after its god ("Temple of Potnia"); every
// other building by its type.
func buildingLabel(m map[string]any) string {
	if name, _ := m["name"].(string); name != "" {
		return name
	}
	t, _ := m["type"].(string)
	return province.BuildingDisplayName(t)
}
