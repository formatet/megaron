package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

func goodsCmd() *cobra.Command {
	var provinceID string
	cmd := &cobra.Command{
		Use:   "goods",
		Short: "Show goods inventory and local prices (defaults to capital; --province for a colony)",
		Example: `  poleia goods
  poleia goods --province <province-id>   # inspect a colony`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)
			// Default to the capital; --province lets you inspect any province you own,
			// mirroring `build`/`status --province`.
			prov := cfg.ProvinceID
			if provinceID != "" {
				resolved, err := resolveProvince(c, cfg.WorldID, provinceID)
				if err != nil {
					return err
				}
				prov = resolved
			}
			path := fmt.Sprintf("/api/v1/worlds/%s/provinces/%s/goods", cfg.WorldID, prov)
			data, err := c.get(path)
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			var goods []map[string]any
			if err := json.Unmarshal(data, &goods); err != nil {
				return err
			}
			if len(goods) == 0 {
				fmt.Println("No goods available.")
				return nil
			}
			// labor_pool och idle_citizens är identiska på varje rad — visa en gång.
			if lp, ok := goods[0]["labor_pool"].(float64); ok {
				idle := 0.0
				if ic, ok2 := goods[0]["idle_citizens"].(float64); ok2 {
					idle = ic
				}
				fmt.Printf("Labor pool: %d workers  ·  Idle: %d workers\n\n", int(lp), int(idle))
			}
			fmt.Printf("%-10s  %8s  %8s  %6s  %8s  %8s\n",
				"Good", "Stock", "Rate/d", "Workers", "Yield/w", "Price")
			fmt.Println("────────────────────────────────────────────────────────────────")
			for _, g := range goods {
				key, _ := g["key"].(string)
				stock, _ := g["amount"].(float64)
				rateT, _ := g["rate_per_tick"].(float64)
				price, _ := g["price"].(float64)
				citizens, _ := g["citizens"].(float64)
				yieldW, _ := g["yield_per_worker"].(float64)
				producible, _ := g["producible"].(bool)
				rateD := rateT * 24 // per-tick × 24 ticks/day
				workersStr := fmt.Sprintf("%d", int(citizens))
				yieldStr := fmt.Sprintf("%.4f", yieldW)
				if !producible {
					// Terrain cannot produce this good — grey it out for the agent.
					workersStr = "—"
					yieldStr = "—"
				}
				fmt.Printf("%-10s  %8.1f  %8.1f  %6s  %8s  %8.1f\n",
					key, stock, rateD, workersStr, yieldStr, price)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&provinceID, "province", "", "province ID to inspect (default: your capital)")
	return cmd
}

func transferCmd() *cobra.Command {
	var good string
	var qty float64
	var destName string

	cmd := &cobra.Command{
		Use:     "transfer",
		Short:   "Send goods to one of your own settlements (internal logistics, no loss)",
		Example: `  poleia transfer --good grain --qty 50 --dest Korinth`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)
			destID, err := resolveSettlement(c, cfg.WorldID, destName)
			if err != nil {
				return fmt.Errorf("resolve destination %q: %w", destName, err)
			}
			path := fmt.Sprintf("/api/v1/worlds/%s/provinces/%s/trade", cfg.WorldID, cfg.ProvinceID)
			data, err := c.post(path, map[string]any{
				"good_key":       good,
				"quantity":       qty,
				"destination_id": destID,
			})
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
			mins, _ := resp["travel_min"].(float64)
			fmt.Printf("Transfer dispatched: %.1f %s → %s · arrives in %.0f min\n", qty, good, destName, mins)
			return nil
		},
	}

	cmd.Flags().StringVarP(&good, "good", "g", "", "good key (e.g. grain, timber, silver)")
	cmd.Flags().Float64VarP(&qty, "qty", "q", 0, "quantity to send")
	cmd.Flags().StringVarP(&destName, "dest", "d", "", "destination settlement name")
	_ = cmd.MarkFlagRequired("good")
	_ = cmd.MarkFlagRequired("qty")
	_ = cmd.MarkFlagRequired("dest")
	return cmd
}

func tradeCmd() *cobra.Command {
	var good string
	var qty float64
	var destName string

	cmd := &cobra.Command{
		Use:     "trade",
		Short:   "Send a trade route to another settlement",
		Example: `  poleia trade --good grain --qty 10 --dest Korinth`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)

			// Resolve destination name → settlement ID.
			destID, err := resolveSettlement(c, cfg.WorldID, destName)
			if err != nil {
				return fmt.Errorf("resolve destination %q: %w", destName, err)
			}

			path := fmt.Sprintf("/api/v1/worlds/%s/provinces/%s/trade", cfg.WorldID, cfg.ProvinceID)
			data, err := c.post(path, map[string]any{
				"good_key":       good,
				"quantity":       qty,
				"destination_id": destID,
			})
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
			mins, _ := resp["travel_min"].(float64)
			bonus, _ := resp["distance_bonus"].(float64)
			delivered, _ := resp["delivered_qty"].(float64)
			fmt.Printf("Trade route created: %.1f× %s → %.1f delivered · %.0f min · +%.0f%% distance bonus\n",
				qty, good, delivered, mins, (bonus-1)*100)
			return nil
		},
	}

	cmd.Flags().StringVarP(&good, "good", "g", "", "good key (e.g. grain, copper)")
	cmd.Flags().Float64VarP(&qty, "qty", "q", 0, "quantity to send")
	cmd.Flags().StringVarP(&destName, "dest", "d", "", "destination settlement name or UUID")
	_ = cmd.MarkFlagRequired("good")
	_ = cmd.MarkFlagRequired("qty")
	_ = cmd.MarkFlagRequired("dest")
	return cmd
}
