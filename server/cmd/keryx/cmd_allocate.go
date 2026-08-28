package main

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/spf13/cobra"
)

// allocateCmd sends a PUT .../labor request to set temple devotion (cult).
// Production goods moved to gubbe placement in P4 (2026-08-08) — the old
// per-good percent split is a dead write for everything except cult, so this
// command only exposes --cult (megaron_plan_riv_procentallokeringen.md).
func allocateCmd() *cobra.Command {
	var cultPercent int
	var provinceID string
	cmd := &cobra.Command{
		Use:   "allocate",
		Short: "Set temple devotion (cult %, defaults to capital; --province for a colony)",
		Long: `Set the share (%) of your settlement's population devoted to the temple (cult).

Production is set by placing gubbar on hexes/buildings — see 'keryx place',
'keryx staff', 'keryx city'. This command is only for cult (temple devotion),
which is ADDITIVE (it does not compete with placed workers). It is allocatable
up to your temple's capacity — 15% per temple level (L1=15%, L2=30%, L3=45%) —
with a 15% floor a temple city always keeps. Raising --cult toward the cap is
what makes a bigger temple's kharis climb.

Run without flags to read the current production split and devotion without
touching either.

Examples:
  keryx allocate                                  (show current split + devotion)
  keryx allocate --cult 30                         (devote 30% of the city to the temple)
  keryx allocate --cult 30 --province <prov-id>    (allocate a colony's temple)`,
		Args: noPositionalArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
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
			// 2026-07-22).
			if cultPercent == 0 {
				return printCurrentAllocation(c, prov)
			}
			path := fmt.Sprintf("/api/v1/worlds/%s/provinces/%s/labor", cfg.WorldID, prov)
			data, err := c.put(path, map[string]any{"percent": map[string]int{"cult": cultPercent}})
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
			if cp, ok := resp["cult_percent"].(float64); ok {
				fmt.Printf("Cult devotion set to %.0f%%.\n", cp)
			}
			if msg, ok := resp["message"].(string); ok && msg != "" {
				fmt.Println(msg)
			}
			return nil
		},
	}

	cmd.Flags().IntVar(&cultPercent, "cult", 0, "share (%) of population devoted to the temple")
	cmd.Flags().StringVar(&provinceID, "province", "", "province ID to allocate (default: your capital)")
	return cmd
}

// printCurrentAllocation implements bare `keryx allocate`: the settlement's
// current production split (real, placement-driven) plus cult devotion,
// read-only. Built from the province goods endpoint, which already carries
// percent/citizens/idle_citizens per good — no new route needed.
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
		}
	}
	if extras.Devotion > 0 {
		rows = append(rows, row{"cult (devotion)", extras.Devotion * 100, int(extras.Devotion * float64(pool)), false})
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
		line := fmt.Sprintf("  %-12s %3.0f%%  (%d citizens)", r.key, r.pct, r.citizens)
		if r.atCap {
			// Same silent-waste class as the workplace-capacity marker below:
			// the good's stock is full, so the labor sitting on it produces
			// nothing. This is the one place a Wanax actually chooses the split,
			// so say it here, not just in `keryx goods`.
			line += "  — at storage ceiling, produces nothing"
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
	fmt.Println("\nProduction is set by placing gubbar on hexes/buildings — see `keryx place`,")
	fmt.Println("`keryx staff`, `keryx city`. `allocate` only sets cult (temple devotion): `keryx allocate --cult 30`.")
	return nil
}

// settlementExtras holds the one province-GET field printCurrentAllocation
// needs beyond the goods list: devotion (cult's share, absent from the goods
// list since mig 094). Used to carry the break-even grain weight too until
// megaron_plan_p4_arvet_i_province.md removed it — LaborAlloc's grain
// guardrail warned about a starvation its own (inert, post-P4) lever cannot
// cause; `keryx status` carries the P4-correct food_gubbar_required/placed
// numbers instead.
type settlementExtras struct {
	Devotion float64
}

// fetchSettlementExtras reads devotion from the province GET — no new route,
// and no second fetch beyond the one devotion already required. Returns the
// zero value on any failure; not worth failing a read-only view over.
func fetchSettlementExtras(c *Client, provinceID string) settlementExtras {
	data, err := c.get(fmt.Sprintf("/api/v1/worlds/%s/provinces/%s", cfg.WorldID, provinceID))
	if err != nil {
		return settlementExtras{}
	}
	var p struct {
		Settlement struct {
			Devotion float64 `json:"devotion"`
		} `json:"settlement"`
	}
	if json.Unmarshal(data, &p) != nil {
		return settlementExtras{}
	}
	return settlementExtras{Devotion: p.Settlement.Devotion}
}
