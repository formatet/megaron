package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// allocateCmd sends a PUT .../labor request to set labor allocations per good.
// Allocations are provided as --<good> <percent> flags (0–100, share of population).
// Σ percent must not exceed 100; the stored weight auto-scales with population.
func allocateCmd() *cobra.Command {
	knownGoods := []string{
		"grain", "timber", "cedar", "stone", "copper", "tin",
		"fish", "wine", "oil", "horses", "bronze", "livestock", "silver",
		"purple", "pottery", "cult",
	}
	rawPercent := make(map[string]*int, len(knownGoods))

	var provinceID string
	cmd := &cobra.Command{
		Use:   "allocate",
		Short: "Set population labor allocation per good (%, defaults to capital; --province for a colony)",
		Long: `Allocate a share (%) of your settlement's population to producible goods.

REPLACES the whole split: any good you do not name drops to 0%. Name every good
you want worked, not just the one you are changing. Run without flags to read the
current allocation without touching it.
(One exception: a city with a temple keeps a 15% cult share, re-applied by the
server and additive — it is not taken from the goods you named.)

Give a percent per good — the sum must not exceed 100.
The share auto-scales with population (pop grows, the worker count grows).
Non-producible goods are rejected by the server.

Examples:
  poleia allocate                                            (show current split)
  poleia allocate --timber 40 --stone 30 --grain 30
  poleia allocate --grain 50 --fish 20                       (the rest is idle)
  poleia allocate --grain 70 --tin 30 --province <prov-id>   (allocate a colony)`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			percent := make(map[string]int)
			for _, key := range knownGoods {
				ptr := rawPercent[key]
				if ptr != nil && *ptr > 0 {
					percent[key] = *ptr
				}
			}
			c := newClient(cfg)
			// Default to the capital; --province lets you allocate any province you own
			// (the server ownership-gates it), mirroring `build`/`status --province`.
			prov := cfg.ProvinceID
			if provinceID != "" {
				resolved, err := resolveProvince(c, cfg.WorldID, provinceID)
				if err != nil {
					return err
				}
				prov = resolved
			}

			// No flags = read-only view. Checking the current split before changing
			// it is the natural first move, and erroring out on it pushed a
			// playtester into issuing a WRITE just to see the state (soak
			// 2026-07-22) — with replace-all semantics that is the expensive way to
			// look at something.
			if len(percent) == 0 {
				return printCurrentAllocation(c, prov)
			}
			path := fmt.Sprintf("/api/v1/worlds/%s/provinces/%s/labor", cfg.WorldID, prov)
			data, err := c.put(path, map[string]any{"percent": percent})
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			var resp map[string]any
			if err := json.Unmarshal(data, &resp); err != nil {
				return err
			}
			// Say it plainly: this call REPLACED the whole split. A playtester ran
			// `allocate --pottery 15` meaning "add pottery", zeroed grain, and spent
			// the next turns hunting a phantom bug while the city starved (soak
			// 2026-07-22) — the break-even warning had fired correctly, but the
			// mental model was "adjust one good", so it read as nonsense.
			fmt.Println("Labor allocation REPLACED (goods you did not name drop to 0%; a temple's 15% cult floor is restored automatically):")
			if lp, ok := resp["labor_pool"].(float64); ok {
				fmt.Printf("  Population:  %d\n", int(lp))
			}
			if idle, ok := resp["idle_percent"].(float64); ok {
				idleC, _ := resp["idle_citizens"].(float64)
				fmt.Printf("  Idle:        %.0f%% (%d citizens)\n", idle, int(idleC))
			}
			fmt.Println()
			pm, _ := resp["percent"].(map[string]any)
			cm, _ := resp["citizens"].(map[string]any)
			if pm != nil {
				order := []string{"grain", "timber", "cedar", "stone", "copper", "tin", "fish", "wine", "oil", "silver"}
				printed := make(map[string]bool)
				printRow := func(key string) {
					pct, _ := pm[key].(float64)
					cit, _ := cm[key].(float64)
					fmt.Printf("  %-12s %3.0f%%  (%d citizens)\n", key, pct, int(cit))
				}
				for _, key := range order {
					if _, ok := pm[key].(float64); ok {
						printRow(key)
						printed[key] = true
					}
				}
				for k := range pm {
					if !printed[k] {
						printRow(k)
					}
				}
			}
			// Break-even guardrail (DEL D): the allocation IS applied, but if the
			// grain share is below break-even the city will slowly starve — surface
			// it after confirming the change so it isn't missed.
			if warning, ok := resp["warning"].(string); ok && warning != "" {
				fmt.Println()
				fmt.Printf("  ⚠ %s\n", warning)
			}
			return nil
		},
	}

	for _, key := range knownGoods {
		var v int
		rawPercent[key] = &v
		cmd.Flags().IntVar(&v, key, 0, fmt.Sprintf("share (%%) of population to %s", key))
	}
	cmd.Flags().StringVar(&provinceID, "province", "", "province ID to allocate (default: your capital)")

	// --raw "timber=40,stone=30" for programmatic use.
	var raw string
	cmd.Flags().StringVar(&raw, "raw", "", "comma-separated key=value in percent (e.g. timber=40,grain=30)")
	cmd.PreRunE = func(cmd *cobra.Command, _ []string) error {
		if raw == "" {
			return nil
		}
		for _, pair := range strings.Split(raw, ",") {
			parts := strings.SplitN(pair, "=", 2)
			if len(parts) != 2 {
				continue
			}
			v, err := strconv.Atoi(strings.TrimSpace(parts[1]))
			if err != nil || v <= 0 {
				continue
			}
			key := strings.TrimSpace(parts[0])
			for _, k := range knownGoods {
				if k == key {
					*rawPercent[k] = v
					break
				}
			}
		}
		return nil
	}
	return cmd
}

// printCurrentAllocation implements bare `poleia allocate`: the settlement's
// current labor split, read-only. Built from the province goods endpoint, which
// already carries percent/citizens/idle_citizens per good — no new route, and
// the same numbers a PUT would echo back.
func printCurrentAllocation(c *Client, provinceID string) error {
	data, err := c.get(fmt.Sprintf("/api/v1/worlds/%s/provinces/%s/goods", cfg.WorldID, provinceID))
	if err != nil {
		return err
	}
	if jsonMode {
		printRawJSON(data)
		return nil
	}
	var goods []struct {
		Key          string  `json:"key"`
		Percent      float64 `json:"percent"`
		Citizens     int     `json:"citizens"`
		LaborPool    int     `json:"labor_pool"`
		IdleCitizens int     `json:"idle_citizens"`
		Producible   bool    `json:"producible"`
	}
	if err := json.Unmarshal(data, &goods); err != nil {
		return err
	}

	// Devotion no longer rides on the goods list: mig 094 made cult a labor share
	// that produces nothing, so its settlement_goods row is gone and with it the
	// only place the share was visible. A mechanic the player cannot see is a
	// mechanic they cannot tend — read it from the labor endpoint instead.
	devotion := fetchDevotionShare(c, provinceID)

	fmt.Println("Current labor allocation:")
	var pool, idle int
	allocated := 0.0
	type row struct {
		key      string
		pct      float64
		citizens int
	}
	var rows []row
	hasCult := false
	for _, g := range goods {
		if g.LaborPool > 0 {
			pool, idle = g.LaborPool, g.IdleCitizens
		}
		if g.Percent > 0 {
			rows = append(rows, row{g.Key, g.Percent, g.Citizens})
			allocated += g.Percent
			if g.Key == "cult" {
				hasCult = true
			}
		}
	}
	if devotion > 0 {
		rows = append(rows, row{"cult (devotion)", devotion * 100, int(devotion * float64(pool))})
		hasCult = true
	}
	fmt.Printf("  Population:  %d\n", pool)
	if pool > 0 {
		// Idle comes from the server's own citizen count, not 100−Σpercent: the
		// stored weights can sum past 100 (they are per-good fractions, and the
		// PUT ceiling only constrains one call), which made the derived figure
		// print a negative idle share.
		fmt.Printf("  Idle:        %.0f%% (%d citizens)\n", 100*float64(idle)/float64(pool), idle)
	}
	if hasCult && allocated > 100 {
		// Not over-subscription: LaborAlloc re-applies a 0.15 cult floor to any
		// city with a temple, and that share is additive by design (the same
		// citizens serve the temple alongside other duties), so the total reads
		// above 100 on purpose.
		fmt.Printf("  (totals %.0f%% — a temple's cult share is additive, not taken from the others)\n", allocated)
	}
	fmt.Println()
	if len(rows) == 0 {
		fmt.Println("  (nothing allocated — every citizen is idle)")
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].pct > rows[j].pct })
	for _, r := range rows {
		fmt.Printf("  %-12s %3.0f%%  (%d citizens)\n", r.key, r.pct, r.citizens)
	}
	if hasCult {
		// Devotion is capped by what the temple can employ (15% per level), and
		// anything above that has no altar to serve at — it would silently pay
		// nothing. Say so where the number is chosen.
		fmt.Println("\n  cult = devotion: the share serving the temple. It produces no good — the kharis")
		fmt.Println("  tick reads it. A temple employs 15% of the city per level, and devotion beyond")
		fmt.Println("  that is not served: to devote more, raise the temple.")
	}
	fmt.Println("\nTo change it, name EVERY good you want worked — `allocate` replaces the whole split,")
	fmt.Println("it does not adjust one good (e.g. `poleia allocate --grain 80 --oil 20`).")
	return nil
}

// fetchDevotionShare reads the settlement's devotion (the share serving the
// temple) from the province GET. It is not in the goods list: mig 094 made cult
// a labor weight that produces nothing, so it has no settlement_goods row —
// which is exactly why it had to be surfaced somewhere else before a Wanax could
// tend it. Returns 0 on any failure; devotion is worth showing, never worth
// failing a read-only view over.
func fetchDevotionShare(c *Client, provinceID string) float64 {
	data, err := c.get(fmt.Sprintf("/api/v1/worlds/%s/provinces/%s", cfg.WorldID, provinceID))
	if err != nil {
		return 0
	}
	var p struct {
		Settlement struct {
			Devotion float64 `json:"devotion"`
		} `json:"settlement"`
	}
	if json.Unmarshal(data, &p) != nil {
		return 0
	}
	return p.Settlement.Devotion
}
