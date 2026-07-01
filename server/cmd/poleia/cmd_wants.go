package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

func wantsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "wants",
		Short: "Show goods in shortage (wants) and surplus (exports) at known settlements",
		Long: `Show goods in shortage (wants) and surplus (exports) at known settlements.

Prices are always firsthand — observed by your own messenger or caravan
reaching the settlement (temenos_gossip.md PASS 2b: gossip only ever tells you
a settlement exists and a coarse industry hint, never its detailed market).`,
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
				Surplus []struct {
					Name  string `json:"name"`
					Goods []struct {
						Good      string  `json:"good"`
						Price     float64 `json:"price"`
						BaseValue float64 `json:"base_value"`
						Stock     float64 `json:"stock"`
					} `json:"goods"`
				} `json:"surplus"`
			}
			if err := json.Unmarshal(data, &resp); err != nil {
				return err
			}
			if len(resp.Wants) == 0 && len(resp.Surplus) == 0 {
				fmt.Println("No price data yet — send a messenger or trade offer to observe markets.")
				return nil
			}
			if len(resp.Wants) > 0 {
				fmt.Println("SHORTAGES (good to sell here):")
				for _, s := range resp.Wants {
					fmt.Printf("  %s:\n", s.Name)
					for _, g := range s.Goods {
						fmt.Printf("    %s (%s) — price %.0f (base %.0f)\n",
							g.Good, g.WantLevel, g.Price, g.BaseValue)
					}
				}
			}
			if len(resp.Surplus) > 0 {
				fmt.Println("\nSURPLUS (good to buy here):")
				for _, s := range resp.Surplus {
					fmt.Printf("  %s:\n", s.Name)
					for _, g := range s.Goods {
						fmt.Printf("    %s — price %.0f (base %.0f) stock %.0f\n",
							g.Good, g.Price, g.BaseValue, g.Stock)
					}
				}
			}
			return nil
		},
	}
}
