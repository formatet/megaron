package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

func outpostFlowsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "outpost-flows",
		Short: "Show what your outposts produce and which settlement they feed",
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)
			path := fmt.Sprintf("/api/v1/worlds/%s/outpost-flows", cfg.WorldID)
			data, err := c.get(path)
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			var flows []map[string]any
			if err := json.Unmarshal(data, &flows); err != nil {
				return err
			}
			if len(flows) == 0 {
				fmt.Println("No outpost flows — establish outposts to produce resources.")
				return nil
			}
			fmt.Printf("%-12s  %-10s  %10s  %-20s  %s\n",
				"Good", "Rate/day", "Province", "Home", "Terrain")
			fmt.Println("──────────────────────────────────────────────────────────────────────")
			for _, f := range flows {
				good, _ := f["good_key"].(string)
				rateM, _ := f["rate_per_min"].(float64)
				home, _ := f["home_settlement_name"].(string)
				terrain, _ := f["terrain"].(string)
				q, _ := f["q"].(float64)
				r, _ := f["r"].(float64)
				rateD := rateM * 60 * 24
				loc := fmt.Sprintf("(%d,%d)", int(q), int(r))
				fmt.Printf("%-12s  %10.1f  %10s  %-20s  %s\n", good, rateD, loc, home, terrain)
			}
			return nil
		},
	}
}
