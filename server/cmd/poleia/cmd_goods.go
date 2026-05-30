package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

func goodsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "goods",
		Short: "Show goods inventory and local prices",
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)
			path := fmt.Sprintf("/api/v1/worlds/%s/provinces/%s/goods", cfg.WorldID, cfg.ProvinceID)
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
			fmt.Printf("%-10s  %8s  %8s  %8s\n", "Good", "Stock", "Rate/d", "Price")
			fmt.Println("──────────────────────────────────────────────")
			for _, g := range goods {
				key, _ := g["key"].(string)
				stock, _ := g["amount"].(float64)
				rateM, _ := g["rate_per_min"].(float64)
				price, _ := g["price"].(float64)
				rateD := rateM * 60 * 24
				fmt.Printf("%-10s  %8.1f  %8.1f  %8.1f\n", key, stock, rateD, price)
			}
			return nil
		},
	}
}

func tradeCmd() *cobra.Command {
	var good string
	var qty float64
	var destID string

	cmd := &cobra.Command{
		Use:   "trade",
		Short: "Send a trade route to another settlement",
		Example: `  poleia trade --good grain --qty 10 --dest <settlement-id>`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)
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
			fmt.Printf("Trade route created: %.1f× %s · arrives in %.0f min\n", qty, good, mins)
			return nil
		},
	}

	cmd.Flags().StringVarP(&good, "good", "g", "", "good key (e.g. grain, copper)")
	cmd.Flags().Float64VarP(&qty, "qty", "q", 0, "quantity to send")
	cmd.Flags().StringVarP(&destID, "dest", "d", "", "destination settlement ID")
	_ = cmd.MarkFlagRequired("good")
	_ = cmd.MarkFlagRequired("qty")
	_ = cmd.MarkFlagRequired("dest")
	return cmd
}
