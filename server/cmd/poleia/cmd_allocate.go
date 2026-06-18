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

	cmd := &cobra.Command{
		Use:   "allocate",
		Short: "Tilldela andel av befolkningen per vara (arbetsallokering, %)",
		Long: `Tilldela en andel (%) av ditt samhälles befolkning till producerbara varor.
Ange procent per vara — summan får ej överstiga 100.
Andelen auto-skalar med befolkningen (växer pop, växer antalet arbetare).
Icke-producerbara varor avvisas av servern.

Exempel:
  poleia allocate --timber 40 --stone 30 --grain 30
  poleia allocate --grain 50 --fish 20   (resten är idle)`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			percent := make(map[string]int)
			for _, key := range knownGoods {
				ptr := rawPercent[key]
				if ptr != nil && *ptr > 0 {
					percent[key] = *ptr
				}
			}
			if len(percent) == 0 {
				return fmt.Errorf("ingen vara angiven — använd t.ex. --timber 40 --grain 30")
			}

			c := newClient(cfg)
			path := fmt.Sprintf("/api/v1/worlds/%s/provinces/%s/labor", cfg.WorldID, cfg.ProvinceID)
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
			fmt.Println("Arbetsallokering uppdaterad:")
			if lp, ok := resp["labor_pool"].(float64); ok {
				fmt.Printf("  Befolkning:  %d\n", int(lp))
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
		cmd.Flags().IntVar(&v, key, 0, fmt.Sprintf("andel (%%) av befolkningen till %s", key))
	}

	// --raw "timber=40,stone=30" för programmatisk användning.
	var raw string
	cmd.Flags().StringVar(&raw, "raw", "", "comma-separated key=value i procent (t.ex. timber=40,grain=30)")
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
