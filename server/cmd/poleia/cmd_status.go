package main

import (
	"encoding/json"
	"fmt"
	"time"

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
			fmt.Printf("%s [%s]  Pop: %s  Labor: %s  Walls: %.0f/3  Loyalty: %.0f%s\n\n",
				name, culture, resource(pop), resource(labor), walls, loyalty, coastalNote)

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
			if res, ok := sett["resources"].(map[string]any); ok {
				printRes := func(label, key string, always bool) {
					rd, ok := res[key].(map[string]any)
					if !ok {
						return
					}
					amt, _ := rd["amount"].(float64)
					rt, _ := rd["rate"].(float64)
					if always || amt > 0 || rt != 0 {
						fmt.Printf("  %-8s %6s  %s\n", label, resource(amt), rate(rt))
					}
				}
				printRes("Silver", "silver", true)
				printRes("Grain", "grain", false)
				printRes("Timber", "timber", false)
				printRes("Stone", "stone", false)
				printRes("Copper", "copper", false)
				printRes("Tin", "tin", false)
				printRes("Bronze", "bronze", false)
			}
			kv, _ := sett["kharis"].(float64)
			kr, _ := sett["kharis_rate"].(float64)
			fmt.Printf("  %-8s %6s  %s\n", "Kharis", resource(kv), rate(kr))
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
						fmt.Printf("  %-10s %4.0f\n", unitDisplayName(u.dbType), v)
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
					fmt.Printf("  %.0f× %-10s done %s\n", c, unitDisplayName(u), localDone(ca))
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&provinceID, "province", "", "province ID to inspect (default: your capital)")
	return cmd
}
