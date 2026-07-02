package main

import (
	"encoding/json"
	"fmt"
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
	}
	rawPercent := make(map[string]*int, len(knownGoods))

	var provinceID string
	cmd := &cobra.Command{
		Use:   "allocate",
		Short: "Set population labor allocation per good (%, defaults to capital; --province for a colony)",
		Long: `Allocate a share (%) of your settlement's population to producible goods.
Give a percent per good — the sum must not exceed 100.
The share auto-scales with population (pop grows, the worker count grows).
Non-producible goods are rejected by the server.

Examples:
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
			if len(percent) == 0 {
				return fmt.Errorf("no good given — use e.g. --timber 40 --grain 30")
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
			fmt.Println("Labor allocation updated:")
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
