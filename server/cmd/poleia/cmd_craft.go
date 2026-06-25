package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// recipeNameToID maps human-readable recipe names to their integer IDs.
// ID 1 = bronze (copper + tin → bronze, requires foundry)
// ID 2 = luxury (prestige goods, requires market)
var recipeNameToID = map[string]int{
	"bronze": 1,
	"luxury": 2,
}

func craftCmd() *cobra.Command {
	var qty float64
	var recipeID int
	var recipeName string

	cmd := &cobra.Command{
		Use:   "craft",
		Short: "Craft a recipe at your settlement (bronze@foundry, luxury@market)",
		Example: `  poleia craft --recipe bronze --qty 5   # smelt 5 bronze (foundry required)
  poleia craft --recipe luxury --qty 2   # craft 2 luxury (market required)
  poleia craft --recipe 1 --qty 5        # same as bronze (numeric ID still works)`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// Resolve recipe name → id if --recipe is a name, not a number.
			if recipeName != "" {
				id, ok := recipeNameToID[strings.ToLower(recipeName)]
				if !ok {
					names := make([]string, 0, len(recipeNameToID))
					for k := range recipeNameToID {
						names = append(names, k)
					}
					return fmt.Errorf("unknown recipe %q — use one of: %s (or numeric id 1=bronze, 2=luxury)",
						recipeName, strings.Join(names, ", "))
				}
				recipeID = id
			}
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
	cmd.Flags().StringVarP(&recipeName, "recipe", "r", "", "recipe name (bronze, luxury) or numeric id (1, 2)")
	return cmd
}
