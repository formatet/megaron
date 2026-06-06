package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

func outpostCmd() *cobra.Command {
	var q, r int
	var hoplites, chariots, hiereus, trireme, agema int

	cmd := &cobra.Command{
		Use:   "outpost",
		Short: "Send a garrison to hold an empty resource hex",
		Example: `  poleia outpost --q 14 --r -3 --hoplites 20
  poleia outpost --q 5 --r 2 --hoplites 15 --chariots 5`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)
			body := map[string]any{
				"target_q":       q,
				"target_r":       r,
				"intent":         "outpost",
				"infantry":       hoplites,
				"chariot":        chariots,
				"priest":         hiereus,
				"ship":           trireme,
				"elite_infantry": agema,
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
			fmt.Printf("Outpost garrison dispatched to (%d,%d) · %.0f hexes · production flows on arrival\n", q, r, dist)
			return nil
		},
	}

	cmd.Flags().IntVar(&q, "q", 0, "hex Q coordinate (required)")
	cmd.Flags().IntVar(&r, "r", 0, "hex R coordinate (required)")
	cmd.Flags().IntVar(&hoplites, "hoplites", 0, "number of Hoplites")
	cmd.Flags().IntVar(&chariots, "chariots", 0, "number of War Chariots")
	cmd.Flags().IntVar(&hiereus, "hiereus", 0, "number of Hiereus")
	cmd.Flags().IntVar(&trireme, "trireme", 0, "number of Triremes")
	cmd.Flags().IntVar(&agema, "agema", 0, "number of Agema")
	_ = cmd.MarkFlagRequired("q")
	_ = cmd.MarkFlagRequired("r")
	return cmd
}

func outpostRecallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "outpost-recall <outpost-province-id>",
		Short: "Recall a garrison from an outpost (stops production flow, returns units)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			outpostProvinceID := args[0]
			c := newClient(cfg)
			path := fmt.Sprintf("/api/v1/worlds/%s/provinces/%s/outpost",
				cfg.WorldID, outpostProvinceID)
			data, err := c.delete(path)
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			fmt.Println("Outpost recalled — garrison is returning home, production flow stopped.")
			return nil
		},
	}
}
