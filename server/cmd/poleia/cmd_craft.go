package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

func craftCmd() *cobra.Command {
	var qty float64

	cmd := &cobra.Command{
		Use:   "craft",
		Short: "Smelt bronze at the foundry (2 copper + 1 tin → 1 bronze)",
		Example: `  poleia craft --qty 5`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)
			path := fmt.Sprintf("/api/v1/worlds/%s/provinces/%s/craft", cfg.WorldID, cfg.ProvinceID)
			data, err := c.post(path, map[string]any{
				"recipe_id": 1,
				"quantity":  qty,
			})
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			var resp map[string]any
			json.Unmarshal(data, &resp)
			produced, _ := resp["produced"].(float64)
			fmt.Printf("Smelted %.1f bronze (%.1f requested)\n", produced, qty)
			return nil
		},
	}

	cmd.Flags().Float64VarP(&qty, "qty", "q", 1, "quantity of bronze to smelt")
	return cmd
}
