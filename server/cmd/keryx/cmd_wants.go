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
				// Rad L, megaron_plan_cli_sanning.md: `wants` only ever covers
				// settlements you've actually contacted (temenos_gossip.md PASS
				// 2b) — an empty response is often TRUE, not broken: either you
				// haven't reached anyone yet, or everyone you HAVE reached
				// currently shows no shortage/surplus (market_snapshots only
				// updates on contact, so a balanced city stays silent here too).
				// The old single message ("send a messenger... to observe
				// markets") presumed the first case and told a Wanax who had
				// already contacted several cities to do something he'd already
				// done. Both real causes point at the same next action — reach
				// a NEW settlement — so naming both here doesn't need a count
				// this endpoint can't give: an exact "N contacted, 0 shown"
				// split needs a field on GET .../market/wants that doesn't
				// exist yet (province.go, out of scope for this row).
				fmt.Println("No shortages or surplus to show right now.")
				fmt.Println("This can mean two different things: you haven't sent a messenger or trade")
				fmt.Println("offer to any settlement yet, OR the settlements you HAVE reached simply show")
				fmt.Println("no shortage/surplus at the moment (a balanced city stays silent here too).")
				fmt.Println("`keryx settlements` lists what you already know; `keryx messenger` reaches a new one.")
				return nil
			}
			// How many settlements this response actually covers — the plan's
			// "say how many cities are included" so the surface's FOW-gated
			// reach is visible, not implied. A settlement can carry both a want
			// and a surplus at once, so count distinct names, not list lengths.
			contacted := map[string]bool{}
			for _, s := range resp.Wants {
				contacted[s.Name] = true
			}
			for _, s := range resp.Surplus {
				contacted[s.Name] = true
			}
			fmt.Printf("Market signal from %d contacted settlement(s):\n\n", len(contacted))
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
