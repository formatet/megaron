package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

func scoutCmd() *cobra.Command {
	var q, r int
	var hoplites, hippeis int

	cmd := &cobra.Command{
		Use:   "scout",
		Short: "Send scouts to reveal fog-of-war at a hex",
		Example: `  poleia scout --q 18 --r -2 --hoplites 5
  poleia scout --q 10 --r 4 --hippeis 3`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)
			body := map[string]any{
				"target_q": q,
				"target_r": r,
				"intent":   "scout",
				"infantry": hoplites,
				"cavalry":  hippeis,
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
			fmt.Printf("Scouts dispatched to (%d,%d) · %.0f hexes · hex reveals on arrival, scouts return home\n", q, r, dist)
			return nil
		},
	}

	cmd.Flags().IntVar(&q, "q", 0, "hex Q coordinate (required)")
	cmd.Flags().IntVar(&r, "r", 0, "hex R coordinate (required)")
	cmd.Flags().IntVar(&hoplites, "hoplites", 1, "number of Hoplites")
	cmd.Flags().IntVar(&hippeis, "hippeis", 0, "number of Hippeis")
	_ = cmd.MarkFlagRequired("q")
	_ = cmd.MarkFlagRequired("r")
	return cmd
}
