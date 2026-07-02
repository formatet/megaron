package main

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/spf13/cobra"
)

// ticklogCmd renders the per-city tick-journal: production, consumption, discrete
// events (Sitos, trade, builds) and a loyalty row (— until Fas 3), one block per
// tick. Newest-first by default; --asc for chronological. Alias: journal.
func ticklogCmd() *cobra.Command {
	var provinceID string
	var last int
	var asc bool
	cmd := &cobra.Command{
		Use:     "ticklog",
		Aliases: []string{"journal"},
		Short:   "Per-tick journal for a city: prod/cons, trade, Sitos, builds (newest first)",
		Example: `  poleia ticklog --last 10
  poleia ticklog --province <province-id> --last 20 --asc`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)
			prov := cfg.ProvinceID
			if provinceID != "" {
				prov = provinceID
			}
			order := ""
			if asc {
				order = "&order=asc"
			}
			path := fmt.Sprintf("/api/v1/worlds/%s/provinces/%s/ticklog?last=%d%s",
				cfg.WorldID, prov, last, order)
			data, err := c.get(path)
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			var resp struct {
				CurrentTick int `json:"current_tick"`
				Ticks       []struct {
					Tick        int                `json:"tick"`
					Production  map[string]float64 `json:"production"`
					Consumption map[string]float64 `json:"consumption"`
					Loyalty     string             `json:"loyalty"`
					Events      []struct {
						Type    string          `json:"type"`
						Payload json.RawMessage `json:"payload"`
					} `json:"events"`
				} `json:"ticks"`
			}
			if err := json.Unmarshal(data, &resp); err != nil {
				return err
			}
			if len(resp.Ticks) == 0 {
				fmt.Println("No ticks to show.")
				return nil
			}
			for _, t := range resp.Ticks {
				fmt.Printf("── Tick %d ──\n", t.Tick)
				fmt.Printf("  Prod:  %s\n", fmtFlows(t.Production))
				fmt.Printf("  Kons:  %s\n", fmtFlows(t.Consumption))
				if len(t.Events) == 0 {
					fmt.Printf("  Händelser: —\n")
				} else {
					for _, e := range t.Events {
						fmt.Printf("  %s\n", renderTickEvent(e.Type, e.Payload))
					}
				}
				loy := t.Loyalty
				if loy == "" {
					loy = "—"
				}
				fmt.Printf("  Lojalitet: %s\n\n", loy)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&provinceID, "province", "", "province ID (default: your capital)")
	cmd.Flags().IntVar(&last, "last", 10, "number of most-recent ticks to show")
	cmd.Flags().BoolVar(&asc, "asc", false, "chronological order (oldest first)")
	return cmd
}

// fmtFlows renders a good→rate map deterministically (sorted keys).
func fmtFlows(m map[string]float64) string {
	if len(m) == 0 {
		return "—"
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := ""
	for i, k := range keys {
		if i > 0 {
			out += ", "
		}
		out += fmt.Sprintf("%s %+.2f/tick", k, m[k])
	}
	return out
}

// renderTickEvent gives a one-line human rendering of a journal event.
func renderTickEvent(etype string, payload json.RawMessage) string {
	switch etype {
	case "SitosTransaction":
		var p struct {
			Good        string  `json:"good"`
			Kind        string  `json:"kind"`
			SilverDelta float64 `json:"silver_delta"`
			GoodDelta   float64 `json:"grain_delta"`
			RefPrice    float64 `json:"ref_price"`
		}
		_ = json.Unmarshal(payload, &p)
		switch p.Kind {
		case "tax":
			return fmt.Sprintf("Sitos: skatt +%.1f silver → fonden", p.SilverDelta)
		case "buy":
			return fmt.Sprintf("Sitos: köpte %.1f %s (ref %.2f), staden +%.1f silver",
				-p.GoodDelta, p.Good, p.RefPrice, -p.SilverDelta)
		case "sell":
			return fmt.Sprintf("Sitos: sålde %.1f %s (ref %.2f), staden −%.1f silver",
				p.GoodDelta, p.Good, p.RefPrice, p.SilverDelta)
		default:
			return "Sitos: " + p.Kind
		}
	case "TradeDelivery":
		var p struct {
			GoodKey  string  `json:"good_key"`
			Quantity float64 `json:"quantity"`
		}
		_ = json.Unmarshal(payload, &p)
		return fmt.Sprintf("Handel: mottog %.1f %s", p.Quantity, p.GoodKey)
	case "BuildComplete", "ScheduledBuildComplete":
		var p struct {
			BuildingType string `json:"building_type"`
		}
		_ = json.Unmarshal(payload, &p)
		if p.BuildingType != "" {
			return "Bygg: " + p.BuildingType + " klar"
		}
		return "Bygg: klar"
	default:
		return etype
	}
}
