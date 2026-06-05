package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// allocateCmd sends a PUT .../labor request to set labor allocation weights.
// Weights are provided as --<good> <value> flags (positive numbers, normalized to 1.0).
func allocateCmd() *cobra.Command {
	// We accept any good key via --<good> flag; map of known goods for convenience.
	knownGoods := []string{
		"grain", "timber", "cedar", "stone", "copper", "tin",
		"fish", "wine", "oil", "horses", "bronze", "livestock", "silver",
	}
	rawWeights := make(map[string]*float64, len(knownGoods))

	cmd := &cobra.Command{
		Use:   "allocate",
		Short: "Set labor allocation weights (production sliders)",
		Long: `Allocate your settlement's workers between producible goods.
Weights are normalized to 100% automatically — just provide relative numbers.
Only producible goods (terrain-possible) can be allocated; others are silently ignored.

Example:
  poleia allocate --timber 50 --stone 30 --grain 20
  poleia allocate --grain 1 --timber 1   (equal split, normalized to 50/50)`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			weights := make(map[string]float64)
			for _, key := range knownGoods {
				ptr := rawWeights[key]
				if ptr != nil && *ptr > 0 {
					weights[key] = *ptr
				}
			}
			// Also parse any extra --good=value pairs passed via --extra flag-style (cobra handles known flags only).
			if len(weights) == 0 {
				return fmt.Errorf("no goods specified — use e.g. --timber 50 --grain 30")
			}

			c := newClient(cfg)
			path := fmt.Sprintf("/api/v1/worlds/%s/provinces/%s/labor", cfg.WorldID, cfg.ProvinceID)
			data, err := c.put(path, map[string]any{"weights": weights})
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
			if wm, ok := resp["weights"].(map[string]any); ok {
				// Sort for stable output.
				order := []string{"grain", "timber", "cedar", "stone", "copper", "tin", "fish", "wine", "oil"}
				for _, key := range order {
					if v, ok := wm[key].(float64); ok {
						fmt.Printf("  %-12s %5.1f%%\n", key, v*100)
					}
				}
				// Print any remaining goods not in order list.
				for k, v := range wm {
					inOrder := false
					for _, o := range order {
						if o == k {
							inOrder = true
							break
						}
					}
					if !inOrder {
						if f, ok := v.(float64); ok {
							fmt.Printf("  %-12s %5.1f%%\n", k, f*100)
						}
					}
				}
			}
			return nil
		},
	}

	for _, key := range knownGoods {
		var v float64
		rawWeights[key] = &v
		cmd.Flags().Float64Var(&v, key, 0, fmt.Sprintf("weight for %s (any positive number, normalized)", key))
	}

	// Support --raw "timber=50,stone=30" for programmatic use.
	var raw string
	cmd.Flags().StringVar(&raw, "raw", "", "raw weights as comma-separated key=value pairs")
	cmd.PreRunE = func(cmd *cobra.Command, _ []string) error {
		if raw == "" {
			return nil
		}
		for _, pair := range strings.Split(raw, ",") {
			parts := strings.SplitN(pair, "=", 2)
			if len(parts) != 2 {
				continue
			}
			v, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
			if err != nil || v <= 0 {
				continue
			}
			key := strings.TrimSpace(parts[0])
			for _, k := range knownGoods {
				if k == key {
					*rawWeights[k] = v
					break
				}
			}
		}
		return nil
	}
	return cmd
}
