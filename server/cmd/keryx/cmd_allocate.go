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
// Σ percent must not exceed 100. Post-P4 only the cult share is live; the other
// goods' weights are inert (production derives from gubbe placement, not weights).
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
Cult (devotion) is the exception and is ADDITIVE: it sits on top of the goods you
name and is NOT counted in the ≤100% sum, so devoting more of the city never
starves production. It is allocatable up to your temple's capacity — 15% per temple
level (L1=15%, L2=30%, L3=45%) — with a 15% floor a temple city always keeps.
Raising --cult toward the cap is what makes a bigger temple's kharis climb.

Give a percent per good — the sum (excluding cult) must not exceed 100.
NOTE (post-P4): for every good EXCEPT cult these shares are currently INERT —
production is set by placing gubbar on hexes/buildings, not by these weights.
Only cult (devotion) is live here. Non-producible goods are rejected by the server.

Grain has a break-even floor: below it, the city slowly starves. The floor is
per catchment (it moves as tiles change), so it is not printed here — run
allocate with no flags first to see the current grain share against it
before you write a new split.

Examples:
  keryx allocate                                            (show current split)
  keryx allocate --timber 40 --stone 30 --grain 30
  keryx allocate --grain 50 --fish 20                       (the rest is idle)
  keryx allocate --grain 45 --timber 25 --oil 10 --cult 30  (fill an L2 temple → kharis climbs)
  keryx allocate --grain 70 --tin 30 --province <prov-id>   (allocate a colony)`,
		// No single flag is the obvious guess for a stray positional here — the
		// command takes one flag per good (--timber, --grain, ...), so there is
		// no "the" flag to name the way build has --type.
		Args: noPositionalArgs(),
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
			// it after confirming the change so it isn't missed. Say plainly that
			// it is reversible: an LLM playtest probe that only ever saw this
			// post-hoc line read it as "I already made an irreversible mistake"
			// (soak 2026-07-25) — it is not one, the split is just another `allocate`
			// call away from changing, same as `keryx allocate` with no flags shows.
			if warning, ok := resp["warning"].(string); ok && warning != "" {
				fmt.Println()
				fmt.Printf("  ⚠ %s\n", warning)
				fmt.Println("  This is reversible: run `allocate` again with a higher grain share to fix it.")
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

// printCurrentAllocation implements bare `keryx allocate`: the settlement's
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
		Amount       float64 `json:"amount"`
		Cap          float64 `json:"cap"`
	}
	if err := json.Unmarshal(data, &goods); err != nil {
		return err
	}

	// Devotion no longer rides on the goods list: mig 094 made cult a labor share
	// that produces nothing, so its settlement_goods row is gone and with it the
	// only place the share was visible. A mechanic the player cannot see is a
	// mechanic they cannot tend — read it from the labor endpoint instead.
	// It also carries the break-even grain weight, so this is a single fetch,
	// not a third round-trip added on top of the goods call above.
	extras := fetchSettlementExtras(c, provinceID)

	fmt.Println("Current labor allocation:")
	var pool, idle int
	allocated := 0.0
	type row struct {
		key      string
		pct      float64
		citizens int
		atCap    bool
	}
	var rows []row
	hasCult := false
	grainAllocated := false
	for _, g := range goods {
		if g.LaborPool > 0 {
			pool, idle = g.LaborPool, g.IdleCitizens
		}
		if g.Percent > 0 {
			rows = append(rows, row{g.Key, g.Percent, g.Citizens, atStorageCeiling(g.Amount, g.Cap)})
			allocated += g.Percent
			if g.Key == "cult" {
				hasCult = true
			}
			if g.Key == "grain" {
				grainAllocated = true
			}
		}
	}
	if extras.Devotion > 0 {
		rows = append(rows, row{"cult (devotion)", extras.Devotion * 100, int(extras.Devotion * float64(pool)), false})
		hasCult = true
	}
	// Break-even preflight (companion to the post-hoc PUT warning below): this
	// is the read-only view a Wanax checks BEFORE writing, so the threshold
	// belongs here, next to the grain row, not only echoed back after a write.
	// A catchment that cannot grow grain at all reports BreakevenWeight == nil
	// — no weight would help there, so stay silent rather than print a
	// misleading "0%". If grain sits at 0% but the catchment DOES have a real
	// floor, that is the most dangerous split of all, so show a grain row
	// anyway instead of letting "no grain line" silently read as "no problem".
	if extras.BreakevenWeight != nil && !grainAllocated {
		rows = append(rows, row{"grain", 0, 0, false})
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
		line := fmt.Sprintf("  %-12s %3.0f%%  (%d citizens)", r.key, r.pct, r.citizens)
		if r.atCap {
			// Same silent-waste class as the workplace-capacity marker below:
			// the good's stock is full, so the labor sitting on it produces
			// nothing. This is the one place a Wanax actually chooses the split,
			// so say it here, not just in `keryx goods`.
			line += "  — at storage ceiling, produces nothing"
		}
		if r.key == "grain" {
			line += grainBreakevenNote(r.pct, extras.BreakevenWeight)
		}
		fmt.Println(line)
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
	fmt.Println("it does not adjust one good (e.g. `keryx allocate --grain 80 --oil 20`).")
	return nil
}

// grainBreakevenNote renders the break-even suffix for the grain row in
// printCurrentAllocation. Unit trap, the reason this is its own function
// instead of inline arithmetic: pct is a PERCENT (0..100, the same unit as
// every other row in this view — it comes straight from the goods list),
// while breakevenWeight is a WEIGHT (0..1) straight off the province GET —
// the same number the PUT .../labor handler compares against weights["grain"]
// (api/handlers/province.go, the LaborAllocated break-even guardrail — named,
// not line-numbered, because those numbers drift). Mix the two units up and
// the comparison fires 100x too
// eagerly or not at all; the *100 scaling happens here, once. Returns "" when
// there is no threshold to report (nil weight — the catchment cannot grow
// grain at all, and no grain weight would fix that, so staying silent beats
// printing a misleading "0%").
func grainBreakevenNote(pct float64, breakevenWeight *float64) string {
	if breakevenWeight == nil {
		return ""
	}
	bePct := *breakevenWeight * 100
	if pct < bePct {
		return fmt.Sprintf("  — BELOW break-even (≥%.0f%%): this weight slowly starves the city", bePct)
	}
	return fmt.Sprintf("  — break-even ≥%.0f%%", bePct)
}

// settlementExtras holds the two province-GET fields printCurrentAllocation
// needs beyond the goods list: devotion (cult's share, absent from the goods
// list since mig 094) and the break-even grain weight (the same number the PUT
// .../labor warning compares against — nil when the catchment cannot grow
// grain at all).
type settlementExtras struct {
	Devotion        float64
	BreakevenWeight *float64
}

// fetchSettlementExtras reads devotion + the break-even grain weight from the
// province GET in one call — no new route, and no second fetch beyond the one
// devotion already required. Returns the zero value on any failure; both
// fields are worth showing, neither is worth failing a read-only view over.
func fetchSettlementExtras(c *Client, provinceID string) settlementExtras {
	data, err := c.get(fmt.Sprintf("/api/v1/worlds/%s/provinces/%s", cfg.WorldID, provinceID))
	if err != nil {
		return settlementExtras{}
	}
	var p struct {
		Settlement struct {
			Devotion        float64  `json:"devotion"`
			BreakevenWeight *float64 `json:"breakeven_grain_weight"`
		} `json:"settlement"`
	}
	if json.Unmarshal(data, &p) != nil {
		return settlementExtras{}
	}
	return settlementExtras{Devotion: p.Settlement.Devotion, BreakevenWeight: p.Settlement.BreakevenWeight}
}
