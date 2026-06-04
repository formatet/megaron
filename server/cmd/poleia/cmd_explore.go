package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

func exploreCmd() *cobra.Command {
	var q, r int
	var ships int

	cmd := &cobra.Command{
		Use:   "explore",
		Short: "Send ships to explore a coastal or sea hex, revealing fog-of-war",
		Example: `  poleia explore --q 5 --r -3 --ships 2
  poleia explore --q 18 --r 0 --ships 1`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if ships <= 0 {
				return fmt.Errorf("--ships must be at least 1")
			}
			c := newClient(cfg)
			body := map[string]any{
				"target_q": q,
				"target_r": r,
				"intent":   "explore",
				"ship":     ships,
			}
			path := fmt.Sprintf("/api/v1/worlds/%s/provinces/%s/march", cfg.WorldID, cfg.ProvinceID)
			data, err := c.post(path, body)
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			var resp map[string]any
			json.Unmarshal(data, &resp)
			dist, _ := resp["distance"].(float64)
			fmt.Printf("Fleet dispatched to (%d,%d) · %.0f hexes · tiles reveal on arrival, ships return home\n", q, r, dist)
			return nil
		},
	}

	cmd.Flags().IntVar(&q, "q", 0, "hex Q coordinate (required)")
	cmd.Flags().IntVar(&r, "r", 0, "hex R coordinate (required)")
	cmd.Flags().IntVar(&ships, "ships", 1, "number of Triremes")
	_ = cmd.MarkFlagRequired("q")
	_ = cmd.MarkFlagRequired("r")
	return cmd
}
