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

Only settlements you have CONTACTED show market data here — the list is fog-of-war
gated. A settlement appears once your own messenger or caravan has reached it; a
city you only know by rumour (visible in "keryx settlements") will NOT show its
wants until you send a "keryx messenger" to it. Discovery is earned by contact.

Stock and rate are always firsthand — observed by your own messenger or caravan
reaching the settlement (temenos_gossip.md PASS 2b: gossip only ever tells you
a settlement exists and a coarse industry hint, never its detailed market).`,
		Args: noPositionalArgs(), // no flags at all — nothing to guess
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
						Good  string  `json:"good"`
						Stock float64 `json:"stock"`
						Rate  float64 `json:"rate"`
					} `json:"goods"`
				} `json:"wants"`
				Surplus []struct {
					Name  string `json:"name"`
					Goods []struct {
						Good  string  `json:"good"`
						Stock float64 `json:"stock"`
						Rate  float64 `json:"rate"`
					} `json:"goods"`
				} `json:"surplus"`
			}
			if err := json.Unmarshal(data, &resp); err != nil {
				return err
			}
			if len(resp.Wants) == 0 && len(resp.Surplus) == 0 {
				fmt.Println("No market data yet — send a messenger or trade offer to observe markets.")
				return nil
			}
			if len(resp.Wants) > 0 {
				fmt.Println("SHORTAGES (good to sell here):")
				for _, s := range resp.Wants {
					fmt.Printf("  %s:\n", s.Name)
					for _, g := range s.Goods {
						fmt.Printf("    %s: %.0f (%s)\n", g.Good, g.Stock, rateStr(g.Rate))
					}
				}
			}
			if len(resp.Surplus) > 0 {
				fmt.Println("\nSURPLUS (good to buy here):")
				for _, s := range resp.Surplus {
					fmt.Printf("  %s:\n", s.Name)
					for _, g := range s.Goods {
						fmt.Printf("    %s: %.0f (%s)\n", g.Good, g.Stock, rateStr(g.Rate))
					}
				}
			}
			return nil
		},
	}
}

// rateStr renders a per-tick rate with a direction arrow, e.g. "▼ -2.0/tick"
// for a draining good or "▲ +8.0/tick" for a producing one.
func rateStr(rate float64) string {
	arrow := "▲"
	if rate < 0 {
		arrow = "▼"
	}
	return fmt.Sprintf("%s %+.1f/tick", arrow, rate)
}
