package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"formatet/megaron/server/internal/unit"
	"github.com/spf13/cobra"
)

var unitAliases = map[string]string{
	"hoplites": "spearman", "hoplite": "spearman", "inf": "spearman", "infantry": "spearman", "spearman": "spearman", "spear": "spearman",
	"chariot": "war_chariot", "chariots": "war_chariot", "cha": "war_chariot", "war_chariot": "war_chariot",
	"trireme": "galley", "ship": "galley", "shp": "galley", "galley": "galley",
	"war_galley": "war_galley", "wargalley": "war_galley", "warship": "war_galley",
	"merchantman": "merchantman", "merchant": "merchantman", "trader": "merchantman",
	"agema": "elite_infantry", "elite": "elite_infantry", "eli": "elite_infantry", "elite_infantry": "elite_infantry",
}

func recruitCmd() *cobra.Command {
	var unit string
	var men int
	var count int
	var name string
	var provinceID string
	var list bool

	cmd := &cobra.Command{
		Use:   "recruit",
		Short: "Recruit men into a land unit, or build a ship (--list to see all recruitable units)",
		Example: `  poleia recruit --list
  poleia recruit --unit hoplites --men 10
  poleia recruit --unit chariot --men 50
  poleia recruit --unit ship --name Asterion
  poleia recruit --unit war_galley --count 3
  poleia recruit --unit hoplites --men 10 --province <province-id>   # recruit in a colony`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)
			// Default to the capital; --province lets you target any province you own
			// (the server verifies ownership), mirroring `build`/`allocate`.
			prov := cfg.ProvinceID
			if provinceID != "" {
				resolved, err := resolveProvince(c, cfg.WorldID, provinceID)
				if err != nil {
					return err
				}
				prov = resolved
			}

			// --list: show the recruitable-unit catalogue (cost, gate, affordability)
			// and exit — no recruiting happens.
			if list {
				return printRecruitCatalogue(c, cfg.WorldID, prov)
			}
			if unit == "" {
				return fmt.Errorf("--unit is required (or use --list to see recruitable units)")
			}

			apiUnit, ok := unitAliases[unit]
			if !ok {
				return fmt.Errorf("unknown unit %q — use: hoplites, chariot, galley, war_galley, merchantman, agema (or `poleia recruit --list`)", unit)
			}
			isNaval := apiUnit == "galley" || apiUnit == "war_galley" || apiUnit == "merchantman"
			if !isNaval {
				if men <= 0 || men%10 != 0 {
					return fmt.Errorf("--men must be a positive multiple of 10 (e.g. 10, 20, … 100)")
				}
				if men > 100 {
					return fmt.Errorf("--men cannot exceed 100 per recruit call")
				}
			}
			if count > 1 && !isNaval {
				return fmt.Errorf("count gäller bara skepp; landenheter växer via --men")
			}
			if count < 1 || count > 20 {
				return fmt.Errorf("--count must be 1–20")
			}
			if name != "" && !isNaval {
				return fmt.Errorf("--name gäller bara skepp")
			}
			path := fmt.Sprintf("/api/v1/worlds/%s/provinces/%s/recruit", cfg.WorldID, prov)
			body := map[string]any{"unit_type": apiUnit, "count": count}
			if !isNaval {
				body["men"] = men
			}
			if name != "" {
				body["name"] = name
			}
			data, err := c.post(path, body)
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			if isNaval {
				var resp struct {
					Names      []string  `json:"names"`
					CompleteAt time.Time `json:"complete_at"`
				}
				_ = json.Unmarshal(data, &resp)
				if count > 1 {
					fmt.Printf("Building %d× %s: %s\n", count, unit, strings.Join(resp.Names, ", "))
				} else if len(resp.Names) == 1 {
					fmt.Printf("Building 1 %s — %q\n", unit, resp.Names[0])
				} else {
					fmt.Printf("Building 1 %s\n", unit)
				}
				if !resp.CompleteAt.IsZero() {
					fmt.Printf("Ready %s — not deployable until then (`poleia unit list` shows the ETA).\n",
						resp.CompleteAt.Local().Format("15:04 Jan 2"))
				}
			} else {
				fmt.Printf("Recruiting %d men as %s\n", men, unit)
				fmt.Println("Note: a land unit must reach 100 men before it can march or colonize. " +
					"Recruit more of the same type into this settlement, then `poleia unit list` " +
					"(watch `deployable`/`men_to_deploy`).")
			}
			// Upkeep warning (P6, soak 2026-07-18): present on either path when the
			// server projects this unit will push the settlement into grain/silver
			// deficit once it garrisons.
			var warnResp struct {
				UpkeepWarning string `json:"upkeep_warning"`
			}
			if json.Unmarshal(data, &warnResp) == nil && warnResp.UpkeepWarning != "" {
				fmt.Println(warnResp.UpkeepWarning)
			}
			return nil
		},
	}

	cmd.Flags().SortFlags = false
	cmd.Flags().StringVar(&provinceID, "province", "", "province to recruit in (default: your capital)")
	cmd.Flags().StringVarP(&unit, "unit", "u", "", "unit type (required unless --list)")
	cmd.Flags().IntVarP(&men, "men", "n", 10, "men to recruit (multiple of 10, max 100; ignored for ships)")
	cmd.Flags().IntVarP(&count, "count", "c", 1, "number of vessels to build in one call (ships only, 1–20)")
	cmd.Flags().StringVar(&name, "name", "", "ship name (ships only; omit for a suggested name)")
	cmd.Flags().BoolVar(&list, "list", false, "show the recruitable-unit catalogue and exit")
	return cmd
}

// printRecruitCatalogue implements `recruit --list`: fetches the static unit
// catalogue (/api/v1/units — cost, pop floor, duration, building gate) and
// joins it with the target settlement's can_recruit affordability, which the
// province GET already computes server-side (the same field `poleia status`
// would read) — no new endpoint needed for that half. Without this a Wanax had
// to guess --unit names blind (verified playtest friction).
func printRecruitCatalogue(c *Client, worldID, provinceID string) error {
	data, err := c.get("/api/v1/units")
	if err != nil {
		return err
	}
	var catalogue []struct {
		Type             string             `json:"type"`
		Costs            map[string]float64 `json:"costs"`
		BatchMen         int                `json:"batch_men"`
		PopCost          int                `json:"pop_cost"`
		DurationMinutes  float64            `json:"duration_minutes"`
		RequiresBarracks bool               `json:"requires_barracks"`
		RequiresStable   bool               `json:"requires_stable"`
		RequiresHarbour  bool               `json:"requires_harbour"`
		RequiresFoundry  bool               `json:"requires_foundry"`
	}
	if jsonMode {
		printRawJSON(data)
		return nil
	}
	if err := json.Unmarshal(data, &catalogue); err != nil {
		return err
	}

	// Affordability against the target settlement — reuse can_recruit from the
	// province GET (already mirrors the real Recruit handler's gates).
	// upkeep_grain_per_day/upkeep_silver_per_day + sustainable (P6, soak
	// 2026-07-18) project what this unit costs once it garrisons against the
	// settlement's current net-after-upkeep capacity — the warning `recruit`
	// itself gives at the moment of the POST, surfaced here BEFORE committing.
	type recruitAffordInfo struct {
		canRecruit  bool
		upkeepGrain float64
		upkeepSilv  float64
		sustainable bool
	}
	afford := map[string]recruitAffordInfo{}
	if sdata, serr := c.get(fmt.Sprintf("/api/v1/worlds/%s/provinces/%s", worldID, provinceID)); serr == nil {
		var p map[string]any
		if json.Unmarshal(sdata, &p) == nil {
			if sett, ok := p["settlement"].(map[string]any); ok {
				if cr, ok := sett["can_recruit"].([]any); ok {
					for _, it := range cr {
						m, _ := it.(map[string]any)
						u, _ := m["unit"].(string)
						can, _ := m["can_recruit"].(bool)
						ug, _ := m["upkeep_grain_per_day"].(float64)
						us, _ := m["upkeep_silver_per_day"].(float64)
						sustain, sustainOK := m["sustainable"].(bool)
						if !sustainOK {
							sustain = true // older server without the field: don't false-warn
						}
						afford[u] = recruitAffordInfo{canRecruit: can, upkeepGrain: ug, upkeepSilv: us, sustainable: sustain}
					}
				}
			}
		}
	}

	fmt.Printf("%-24s  %-14s  %-28s  %-6s  %-5s  %-16s  %-6s  %s\n",
		"Type (--unit)", "Batch", "Cost", "Mins", "Pop", "Requires", "Afford", "Upkeep/day (once garrisoned)")
	fmt.Println(strings.Repeat("─", 110))
	for _, u := range catalogue {
		label := u.Type
		if dn := unit.DisplayName(u.Type); dn != u.Type {
			label = fmt.Sprintf("%s (%s)", u.Type, dn)
		}

		isNaval := u.Type == "galley" || u.Type == "war_galley" || u.Type == "merchantman"
		batch := fmt.Sprintf("%d men", u.BatchMen)
		if isNaval {
			batch = fmt.Sprintf("1 vessel (%d crew)", u.BatchMen)
		}

		costParts := make([]string, 0, len(u.Costs))
		for g, q := range u.Costs {
			costParts = append(costParts, fmt.Sprintf("%s×%s", g, trimFloat(q)))
		}
		sort.Strings(costParts)
		costStr := strings.Join(costParts, " ")

		reqs := []string{}
		if u.RequiresBarracks {
			reqs = append(reqs, "barracks")
		}
		if u.RequiresStable {
			reqs = append(reqs, "stable")
		}
		if u.RequiresHarbour {
			reqs = append(reqs, "harbour")
		}
		if u.RequiresFoundry {
			reqs = append(reqs, "foundry")
		}
		reqStr := strings.Join(reqs, "+")
		if reqStr == "" {
			reqStr = "—"
		}

		affordStr := "?"
		upkeepStr := ""
		if info, ok := afford[u.Type]; ok {
			if info.canRecruit {
				affordStr = "✓"
			} else {
				affordStr = "✗"
			}
			if info.upkeepGrain > 0 || info.upkeepSilv > 0 {
				upkeepStr = fmt.Sprintf("%.1f grain, %.1f silver", info.upkeepGrain, info.upkeepSilv)
				if !info.sustainable {
					upkeepStr += "  ⚠ city can't carry this yet"
				}
			}
		}

		fmt.Printf("%-24s  %-14s  %-28s  %-6.0f  %-5d  %-16s  %-6s  %s\n",
			label, batch, costStr, u.DurationMinutes, u.PopCost, reqStr, affordStr, upkeepStr)
	}
	return nil
}

// trimFloat formats a quantity with up to 2 decimals, trimming trailing zeros
// (30 -> "30", 6.25 -> "6.25") so batch costs read cleanly.
func trimFloat(v float64) string {
	s := strconv.FormatFloat(v, 'f', 2, 64)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	return s
}
