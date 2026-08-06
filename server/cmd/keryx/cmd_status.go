package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

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
// as done, so `craft` answering "foundry required" moments after the build no
// longer surprises. Once the event fires the entry is gone from the queue.
func buildQueueETA(iso string) string {
	if t, err := time.Parse(time.RFC3339, iso); err == nil {
		if !t.After(time.Now()) {
			return "finishing…"
		}
		return t.Local().Format("15:04 Jan 2")
	}
	return iso
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
	return fmt.Sprintf("Detta är %s. Du har %.0f städer — `keryx settlements` listar dem, `keryx status --province <id>` visar en annan.",
		name, settlementsUsed)
}

func statusCmd() *cobra.Command {
	var provinceID string
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show your province status (defaults to your capital; --province inspects a colony)",
		Example: `  keryx status
  keryx status --province <province-id>   # inspect a colony`,
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
			if hint := multiCityHint(name, settlementsUsed); hint != "" {
				fmt.Println(hint)
			}
			fmt.Println("  Loyalty 1–4 (1=lägst; revolt kräver även fientlig garnison-majoritet + utlösande händelse)")
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
				state := "lägger undan ett tionde av överskottet"
				switch {
				case cov < low && total <= 0 && net > 0:
					state = "tomt — men lagret växer, täckningen stiger"
				case cov < low && total <= 0:
					state = "TOMT och lagret krymper — staden är utan skydd"
				case cov < low:
					state = "släpper mat till staden"
				case cov <= high:
					state = "vilar — varken undan eller ut"
				}
				fmt.Printf("Sitos-magasinet: %s undan%s / tak %s · täckning %.1f tick (magasinet fyller över %.0f, tömmer under %.0f) · %s\n\n",
					resource(total), parts, resource(gcap), cov, high, low, state)
			}

			// "Senaste tick"-sammanfattning: summerar journalen (keryx ticklog)
			// utan att ersätta den.
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
				// Sitos-itemisering: what MOVED, in food. There is no silver leg
				// left to net out (mig 106), so the short form is "magasinet vilade"
				// rather than a delta of zero silver — a number that would now
				// always be 0 and tell the Wanax nothing.
				sitosNote := "Sitos-magasinet vilade"
				if sitosInterventions > 0 {
					detail := ""
					if sitosFoodIn > 0 {
						detail = fmt.Sprintf("staden fick %s mat ur magasinet", resource(sitosFoodIn))
					}
					if sitosFoodOut > 0 {
						if detail != "" {
							detail += " / "
						}
						detail += fmt.Sprintf("la undan %s mat", resource(sitosFoodOut))
					}
					if detail == "" {
						detail = "ingen mat flyttades"
					}
					sitosNote = detail
				}
				fmt.Printf("Senaste tick (%d): %d varor produceras, %d förbrukas, %s  ·  keryx ticklog för detaljer\n\n",
					int(tk), prodN, consN, sitosNote)
			}

			// Resources: silver + the bronze-chain goods live in resources as
			// {amount,rate,cap} objects; kharis is the per-Wanax pool exposed at the
			// settlement top level. Silver always prints (even 0); grain + the metals
			// print when present so a colony's tin/copper output is visible here, not
			// only via `goods`.
			fmt.Println("Resources")
			fmt.Println("  (rate = netto: produktion − konsumtion, per tick)")
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
							line += " netto"
							// Real shortage risk: current stock runs out within the next
							// tick at this net rate — most negative nettos are a stable
							// balance a stock buffer absorbs, not an emergency (DEL C
							// grain-netto-märkning: don't cry wolf).
							if amt/-rt < 1 {
								line += "  ⚠ tar slut inom en tick"
							}
						}
						fmt.Println(line)
					}
				}
				printRes("Silver", "silver", true)

				// Grain: itemized prod/konsum/netto per TICK (DEL C fuller fix,
				// GREENLIT 2026-07-12) instead of one unmarked netto rate — the stored
				// rate is already net, so a negative number alone reads as an alarm
				// when it's often just normal balance. Components are additive fields
				// the status endpoint derives from the same consumption formula
				// RecomputeProduction folds into grain's rate (economy.
				// GrainConsumptionPerCitizenPerTick), not a re-derivation of the mechanic.
				if gRd, ok := res["grain"].(map[string]any); ok {
					gAmt, _ := gRd["amount"].(float64)
					gProdRate, _ := sett["grain_prod_rate"].(float64)
					gConsumRate, _ := sett["grain_consum_rate"].(float64)
					if gAmt > 0 || gProdRate != 0 || gConsumRate != 0 {
						prodTick := gProdRate
						consumTick := gConsumRate
						netTick := prodTick - consumTick
						line := fmt.Sprintf("  %-8s %6s  prod %.1f − konsum %.1f = netto %+.1f /tick",
							"Grain", resource(gAmt), prodTick, consumTick, netTick)
						if be, ok := sett["breakeven_grain_weight"].(float64); ok {
							line += fmt.Sprintf("  (break-even grain-vikt ≥%.0f%%)", be*100)
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
					warn := ""
					// Name WHICH half is short, and the matching consequence: the old
					// string fired on either and always said "svälta/desertera", so a
					// city with 118k grain and a silver deficit read as a famine
					// (soak 2026-07-22, two playtesters in a row).
					// Runway: a negative net only bites when the stock runs out. A probe
					// disbanded 100 spearmen over a −7/tick silver warning while holding
					// 41k silver (~5000 ticks of runway) — soak 2026-07-24. Name how long
					// the buffer covers it so the warning isn't read as imminent.
					runway := func(key string, netPerDay float64) string {
						if netPerDay >= 0 {
							return ""
						}
						stock := 0.0
						if rd, ok := res[key].(map[string]any); ok {
							stock, _ = rd["amount"].(float64)
						}
						if stock <= 0 {
							return ""
						}
						return fmt.Sprintf(" — lager %s räcker ~%.0f tick i denna takt", resource(stock), stock/-netPerDay)
					}
					switch {
					case netG < 0 && netS < 0:
						warn = "  ⚠ varken grain eller silver täcker arméns upkeep — enheter svälter/deserterar när lagren tar slut" + runway("grain", netG) + runway("silver", netS) + " (se `keryx recruit --list`)"
					case netG < 0:
						warn = "  ⚠ grain täcker inte arméns upkeep — enheter kan svälta" + runway("grain", netG) + " (silvret räcker; se `keryx recruit --list`)"
					case netS < 0:
						warn = "  ⚠ silver täcker inte arméns sold — enheter kan desertera" + runway("silver", netS) + " (maten räcker; se `keryx recruit --list`)"
					}
					fmt.Printf("  %-8s %+.1f grain/tick, %+.1f silver/tick (efter arméns upkeep)%s\n",
						"Netto", netG, netS, warn)
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
			}
			// Obruten deposit i catchmenten (P1a, soak 2026-07-18): se
			// unusedCatchmentDeposits — flaggar koppar/tenn/silver som ligger i
			// stadens 7-hex catchment men saknar mine/silver_mine.
			if cd, ok := p["catchment_deposits"].([]any); ok {
				buildings, _ := sett["buildings"].([]any)
				if unused := unusedCatchmentDeposits(cd, buildings); len(unused) > 0 {
					fmt.Printf("  ⚠ Obruten deposit i catchmenten: %s — bygg mine/silver_mine här för att utvinna\n",
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
			netStr := fmt.Sprintf("passiv %+.1f/tick", kpd)
			if netKnown {
				netStr = fmt.Sprintf("netto %+.1f/tick (tempel − decay)", knet)
			}
			if kcap > 0 {
				fmt.Printf("  %-8s %6s  (%s) · tak %.0f · %s\n", "Kharis", resource(kv), mood, kcap, netStr)
			} else {
				fmt.Printf("  %-8s %6s  (%s) · %s\n", "Kharis", resource(kv), mood, netStr)
			}
			// Legibilitet (2026-07-24): ett L1-tempel HÅLLER standing men klättrar inte
			// förbi sitt tak — utan denna rad läser en spelare "kharis fastnat på 22" som
			// en bugg (sondrunda 2026-07-24). Taket = 25×(1+nivå): L1=50, L2=75, L3=100.
			if mtl >= 1 && kcap < 100 {
				line := fmt.Sprintf("  → taket %.0f sätts av ditt nivå-%.0f-tempel; din kharis håller men klättrar inte — höj kult-labor (`keryx allocate --cult`) eller bygg ett högre tempel.\n", kcap, mtl)
				if netKnown && knet > 0.05 {
					line = fmt.Sprintf("  → din kharis klättrar mot taket %.0f (satt av ditt nivå-%.0f-tempel) — bygg ett högre tempel för att höja taket vidare.\n", kcap, mtl)
				} else if netKnown && knet < -0.05 {
					line = fmt.Sprintf("  → taket %.0f sätts av ditt nivå-%.0f-tempel, men din kharis faller mot golvet — höj kult-labor (om templet har plats) eller bygg ett högre tempel.\n", kcap, mtl)
				}
				fmt.Print(line)
			}

			// Kult: per tempel-stad, dagens offer-krav vs oil/vin-lager — svarar
			// direkt på "kommer min kharis klättra idag" utan att vänta på tick.
			if idle, _ := sett["kharis_devotion_idle"].(bool); idle {
				fmt.Println("  → ditt tempel kan sysselsätta MER hängivenhet än du allokerat — höj kult-labor (`keryx allocate --cult <%>`) för att fylla det och få kharis att klättra.")
			}
			if temples, ok := sett["temple_offers"].([]any); ok {
				if len(temples) == 0 {
					fmt.Println("  Tempel: inga — kharis klättrar inte utan tempel + offer.")
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
					fmt.Printf("  Tempel i %s: kräver %.0f olja + %.0f vin/tick — lager: olja %s, vin %s  %s\n",
						name, oilNeeded, wineNeeded, resource(oil), resource(wine), mark)
				}
				if mood == "Suspicious" || mood == "Wrathful" || anyUnfed {
					fmt.Println("  → mata templen (bygg upp olja/vin) eller kasta rit — se `keryx rite --list`.")
				}
			}
			fmt.Println()

			army, _ := sett["army"].(map[string]any)
			if army != nil {
				fmt.Printf("Army (garnison i %s)\n", name)
				// jsonKey = province.ArmyComposition's Go field name (no JSON tags,
				// so it serializes verbatim); dbType feeds the shared display map.
				units := []struct{ jsonKey, dbType string }{
					{"Spearman", "spearman"}, {"WarChariot", "war_chariot"}, {"Priest", "priest"},
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
						fmt.Printf("  %-10s %.1f grain, %.1f silver / tick  (allt staden betalar — även fältenheter)\n", "Upkeep", g, s)
						// Del C: soldiers standing in the town that pays them spend
						// their sold there. Shown as its own line because it is the
						// only reason the net below is not gross — an invisible flow
						// is one the Wanax can neither plan for nor exploit.
						if circ, ok := sett["army_upkeep_circulated_silver"].(float64); ok && circ > 0 {
							fmt.Printf("  %-10s %.1f silver / tick tillbaka i staden (garnisonens sold)\n", "", circ)
						}
					}
				}
				// Legibilitet (2026-07-24): namnge att detta bara är garnisonen HÄR —
				// `unit list` räknar hela rostret (fält, andra städer, ockuperade ruiner).
				fmt.Println("  Fältenheter och andra städer syns i `keryx unit list`.")
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
					fmt.Printf("  %-12s %s\n", t, buildQueueETA(ca))
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
						ready = " — klar " + localDone(ra)
					}
					switch {
					case cat == "naval":
						fmt.Printf("  %-10s bygger%s\n", name, ready)
					case status == "training":
						fmt.Printf("  %-10s %.0f/100 · tränar%s\n", name, sz, ready)
					default: // forming
						fmt.Printf("  %-10s %.0f/100 · formerar (%.0f kvar att rekrytera)\n", name, sz, 100-sz)
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
const loyaltyLegend = "  Höjs av: kharis ≥ favör-tröskel, mätt/varierad kost (dagliga welfare-tick), " +
	"gåvor ≥50 silver-motsvarande (`keryx transfer`), vunna/försvarade strider.\n" +
	"  Sänks av: svält, för många kolonier (överexpansion), försummelse (>2 dygn utan gåva), " +
	"förlorade strider, lånad armé kvar för länge."

// formatLoyaltyLog turns a settlement's loyalty-log entries into the lines
// `status` prints under the Loyalty line — most recent first, capped at 5,
// always followed by loyaltyLegend so the mechanic is explained even when a
// fired event's reason string is terse. Pure — no DB, no HTTP — so this is
// unit-testable without a live server.
func formatLoyaltyLog(entries []loyaltyLogEntry) []string {
	if len(entries) == 0 {
		return []string{"  Inga lojalitetshändelser ännu.", loyaltyLegend}
	}
	lines := []string{"  Senaste lojalitetshändelser:"}
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
