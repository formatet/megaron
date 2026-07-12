package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/poleia/server/internal/events"
	"github.com/poleia/server/internal/unit"
	"github.com/spf13/cobra"
)

// localDone parses an RFC3339 (UTC) completion timestamp and formats it in the
// player's local time, matching `unit march`'s ETA display — a raw UTC string
// like "2026-07-02T04:37:52Z" otherwise forces manual timezone math.
func localDone(iso string) string {
	if t, err := time.Parse(time.RFC3339, iso); err == nil {
		return t.Local().Format("15:04 Jan 2")
	}
	return iso
}

func statusCmd() *cobra.Command {
	var provinceID string
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show your province status (defaults to your capital; --province inspects a colony)",
		Example: `  poleia status
  poleia status --province <province-id>   # inspect a colony`,
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
			if cap, ok := sett["settlement_cap"].(map[string]any); ok {
				used, _ := cap["used"].(float64)
				max, _ := cap["max"].(float64)
				settlementsNote = fmt.Sprintf("  Settlements: %.0f/%.0f", used, max)
			}
			fmt.Printf("%s [%s]  Pop: %s  Labor: %s  Walls: %.0f/3  Loyalty: %.0f%s%s\n",
				name, culture, resource(pop), resource(labor), walls, loyalty, settlementsNote, coastalNote)
			fmt.Println("  Loyalty 1–4 (1=lägst; revolt kräver även fientlig garnison-majoritet + utlösande händelse)")
			fmt.Println()

			// Sitos-fonden (grain reserve): the automatic last-resort counterparty
			// for subsistence goods. Always shown so its silver + reference price
			// are legible every tick.
			if sitos, ok := sett["sitos"].(map[string]any); ok {
				fund, _ := sitos["fund_silver"].(float64)
				cap, _ := sitos["fund_cap"].(float64)
				rt, _ := sitos["fund_rate_per_tick"].(float64)
				ref, _ := sitos["ref_price_grain"].(float64)
				floor, _ := sitos["ref_price_floor"].(float64)
				ceil, _ := sitos["ref_price_ceiling"].(float64)
				fmt.Printf("Sitos-fonden (spannmålsreserv): %s silver (+%.1f/tick, cap %s) · Referenspris grain: %.2f silver/enhet (golv %.1f, tak %.1f)\n\n",
					resource(fund), rt, resource(cap), ref, floor, ceil)
			}

			// "Senaste tick"-sammanfattning: summerar journalen (poleia ticklog)
			// utan att ersätta den.
			if lt, ok := sett["last_tick"].(map[string]any); ok {
				tk, _ := lt["tick"].(float64)
				sitosDelta, _ := lt["sitos_delta"].(float64)
				prodN := 0
				if p, ok := lt["production"].(map[string]any); ok {
					prodN = len(p)
				}
				consN := 0
				if c2, ok := lt["consumption"].(map[string]any); ok {
					consN = len(c2)
				}
				fmt.Printf("Senaste tick (%d): %d varor produceras, %d förbrukas, Sitos-delta %+.1f silver  ·  poleia ticklog för detaljer\n\n",
					int(tk), prodN, consN, sitosDelta)
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
							// Real shortage risk: current stock runs out inside a day
							// (events.TicksPerDay ticks) at this net rate — most negative
							// nettos are a stable balance a stock buffer absorbs, not an
							// emergency (DEL C grain-netto-märkning: don't cry wolf).
							if amt/-rt < float64(events.TicksPerDay) {
								line += "  ⚠ tar slut inom ett dygn"
							}
						}
						fmt.Println(line)
					}
				}
				printRes("Silver", "silver", true)

				// Grain: itemized prod/konsum/netto per DYGN (DEL C fuller fix,
				// GREENLIT 2026-07-12) instead of one unmarked netto rate — the stored
				// rate is already net, so a negative number alone reads as an alarm
				// when it's often just normal balance. Components are additive fields
				// the status endpoint derives from the same consumption formula
				// RecomputeProduction folds into grain's rate (economy.
				// GrainConsumptionPerCitizenPerDay), not a re-derivation of the mechanic.
				if gRd, ok := res["grain"].(map[string]any); ok {
					gAmt, _ := gRd["amount"].(float64)
					gProdRate, _ := sett["grain_prod_rate"].(float64)
					gConsumRate, _ := sett["grain_consum_rate"].(float64)
					if gAmt > 0 || gProdRate != 0 || gConsumRate != 0 {
						prodDay := gProdRate * float64(events.TicksPerDay)
						consumDay := gConsumRate * float64(events.TicksPerDay)
						netDay := prodDay - consumDay
						line := fmt.Sprintf("  %-8s %6s  prod %.1f − konsum %.1f = netto %+.1f /dygn",
							"Grain", resource(gAmt), prodDay, consumDay, netDay)
						if be, ok := sett["breakeven_grain_weight"].(float64); ok {
							line += fmt.Sprintf("  (break-even grain-vikt ≥%.0f%%)", be*100)
						}
						fmt.Println(line)
					}
				}

				printRes("Timber", "timber", false)
				printRes("Stone", "stone", false)
				printRes("Copper", "copper", false)
				printRes("Tin", "tin", false)
				printRes("Bronze", "bronze", false)
			}
			// Kharis (PLAN B, megaron_kult_legibilitet_plan.md): kharis is now
			// DAILY-maintenance-driven, not per-tick — a per-tick rate rendered
			// "+0.0/tick" for any typical passive value (A4a-buggen). Show the mood
			// (gynnsamhets-signal, never a computed odds — see `rite --list`) and the
			// passive geographic rate per DYGN instead.
			kv, _ := sett["kharis"].(float64)
			mood, _ := sett["kharis_mood"].(string)
			kpd, _ := sett["kharis_per_day"].(float64)
			fmt.Printf("  %-8s %6s  (%s) · passiv %+.1f/dygn\n", "Kharis", resource(kv), mood, kpd)

			// Kult: per tempel-stad, dagens offer-krav vs oil/vin-lager — svarar
			// direkt på "kommer min kharis klättra idag" utan att vänta på tick.
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
					fmt.Printf("  Tempel i %s: kräver %.0f olja + %.0f vin/dygn — lager: olja %s, vin %s  %s\n",
						name, oilNeeded, wineNeeded, resource(oil), resource(wine), mark)
				}
				if mood == "Suspicious" || mood == "Wrathful" || anyUnfed {
					fmt.Println("  → mata templen (bygg upp olja/vin) eller kasta rit — se `poleia rite --list`.")
				}
			}
			fmt.Println()

			army, _ := sett["army"].(map[string]any)
			if army != nil {
				fmt.Println("Army")
				// jsonKey = province.ArmyComposition's Go field name (no JSON tags,
				// so it serializes verbatim); dbType feeds the shared display map.
				units := []struct{ jsonKey, dbType string }{
					{"Spearman", "spearman"}, {"WarChariot", "war_chariot"}, {"Priest", "priest"},
					{"Ship", "ship"}, {"EliteInfantry", "elite_infantry"},
					{"WarGalley", "war_galley"}, {"Merchantman", "merchantman"},
				}
				for _, u := range units {
					v, _ := army[u.jsonKey].(float64)
					if v > 0 {
						fmt.Printf("  %-10s %4.0f\n", unit.DisplayName(u.dbType), v)
					}
				}
				// Upkeep the standing garrison drains each day (grain shortage → attrition,
				// silver shortage → desertion). Same figures the daily upkeep tick debits.
				if up, ok := sett["army_upkeep"].(map[string]any); ok {
					g, _ := up["grain"].(float64)
					s, _ := up["silver"].(float64)
					if g > 0 || s > 0 {
						fmt.Printf("  %-10s %.1f grain, %.1f silver / day\n", "Upkeep", g, s)
					}
				}
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
					fmt.Printf("  %-12s done %s\n", t, localDone(ca))
				}
			}

			if tq, ok := sett["training_queue"].([]any); ok && len(tq) > 0 {
				fmt.Println("\nTraining")
				for _, it := range tq {
					m, _ := it.(map[string]any)
					u, _ := m["unit"].(string)
					c, _ := m["count"].(float64)
					ca, _ := m["complete_at"].(string)
					fmt.Printf("  %.0f× %-10s done %s\n", c, unit.DisplayName(u), localDone(ca))
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&provinceID, "province", "", "province ID to inspect (default: your capital)")
	return cmd
}
