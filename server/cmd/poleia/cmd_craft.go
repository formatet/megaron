package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

func craftCmd() *cobra.Command {
	var qty float64
	var recipeID int

	cmd := &cobra.Command{
		Use:   "craft",
		Short: "Craft a recipe (1=bronze@foundry, 2=luxury@market)",
		Example: `  poleia craft --qty 5            # smelt 5 bronze
  poleia craft --recipe 2 --qty 2  # craft 2 luxury`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)
			path := fmt.Sprintf("/api/v1/worlds/%s/provinces/%s/craft", cfg.WorldID, cfg.ProvinceID)
			data, err := c.post(path, map[string]any{
				"recipe_id": recipeID,
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
			output, _ := resp["output_key"].(string)
			if output == "" {
				output = "goods"
			}
			fmt.Printf("Crafted %.1f %s (%.1f requested)\n", produced, output, qty)
			return nil
		},
	}

	cmd.Flags().Float64VarP(&qty, "qty", "q", 1, "quantity to craft")
	cmd.Flags().IntVarP(&recipeID, "recipe", "r", 1, "recipe id (1=bronze, 2=luxury)")
	return cmd
}
