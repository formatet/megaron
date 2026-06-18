package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

func wantsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "wants",
		Short: "Show goods in shortage at settlements you have price-knowledge of",
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)
			path := fmt.Sprintf("/api/v1/worlds/%s/market/wants", cfg.WorldID)
			data, err := c.get(path)
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			var resp struct {
				Wants []struct {
					Name       string `json:"name"`
					ObservedAt string `json:"observed_at"`
					Goods      []struct {
						Good      string  `json:"good"`
						WantLevel string  `json:"want_level"`
						Price     float64 `json:"price"`
						BaseValue float64 `json:"base_value"`
					} `json:"goods"`
				} `json:"wants"`
			}
			if err := json.Unmarshal(data, &resp); err != nil {
				return err
			}
			if len(resp.Wants) == 0 {
				fmt.Println("No known settlements with shortages.")
				return nil
			}
			for _, s := range resp.Wants {
				fmt.Printf("%s:\n", s.Name)
				for _, g := range s.Goods {
					fmt.Printf("  %s (%s) — price %.0f (base %.0f)\n",
						g.Good, g.WantLevel, g.Price, g.BaseValue)
				}
			}
			return nil
		},
	}
}
