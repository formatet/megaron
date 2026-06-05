package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// allocateCmd sends a PUT .../labor request to set citizen allocations per good.
// Citizens are provided as --<good> <antal> flags (positive integers, not normalized).
func allocateCmd() *cobra.Command {
	knownGoods := []string{
		"grain", "timber", "cedar", "stone", "copper", "tin",
		"fish", "wine", "oil", "horses", "bronze", "livestock", "silver",
	}
	rawCitizens := make(map[string]*int, len(knownGoods))

	cmd := &cobra.Command{
		Use:   "allocate",
		Short: "Tilldela citizens per vara (arbetsallokering)",
		Long: `Tilldela ditt samhälles citizens till producerbara varor.
Ange antal citizens per vara — summan får ej överstiga labor_pool.
Icke-producerbara varor ignoreras av servern.

Exempel:
  poleia allocate --timber 40 --stone 30 --grain 30
  poleia allocate --grain 50 --fish 20   (resten är idle)`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			citizens := make(map[string]int)
			for _, key := range knownGoods {
				ptr := rawCitizens[key]
				if ptr != nil && *ptr > 0 {
					citizens[key] = *ptr
				}
			}
			if len(citizens) == 0 {
				return fmt.Errorf("ingen vara angiven — använd t.ex. --timber 40 --grain 30")
			}

			c := newClient(cfg)
			path := fmt.Sprintf("/api/v1/worlds/%s/provinces/%s/labor", cfg.WorldID, cfg.ProvinceID)
			data, err := c.put(path, map[string]any{"citizens": citizens})
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
				fmt.Printf("  Labor pool:   %d workers\n", int(lp))
			}
			if idle, ok := resp["idle_citizens"].(float64); ok {
				fmt.Printf("  Idle workers: %d\n", int(idle))
			}
			fmt.Println()
			if cm, ok := resp["citizens"].(map[string]any); ok {
				order := []string{"grain", "timber", "cedar", "stone", "copper", "tin", "fish", "wine", "oil"}
				for _, key := range order {
					if v, ok := cm[key].(float64); ok {
						fmt.Printf("  %-12s %d workers\n", key, int(v))
					}
				}
				for k, v := range cm {
					inOrder := false
					for _, o := range order {
						if o == k {
							inOrder = true
							break
						}
					}
					if !inOrder {
						if f, ok := v.(float64); ok {
							fmt.Printf("  %-12s %d workers\n", k, int(f))
						}
					}
				}
			}
			return nil
		},
	}

	for _, key := range knownGoods {
		var v int
		rawCitizens[key] = &v
		cmd.Flags().IntVar(&v, key, 0, fmt.Sprintf("antal citizens till %s", key))
	}

	// --raw "timber=40,stone=30" för programmatisk användning.
	var raw string
	cmd.Flags().StringVar(&raw, "raw", "", "comma-separated key=value (t.ex. timber=40,grain=30)")
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
					*rawCitizens[k] = v
					break
				}
			}
		}
		return nil
	}
	return cmd
}
