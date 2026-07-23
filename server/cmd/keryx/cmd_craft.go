package main

import (
	"encoding/json"
	"fmt"
	"strconv"
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
	var provinceID string

	cmd := &cobra.Command{
		Use:   "craft",
		Short: "Craft a recipe at your settlement (bronze@foundry, luxury@market)",
		Example: `  keryx craft --recipe bronze --qty 5   # smelt 5 bronze (foundry required)
  keryx craft --recipe luxury --qty 2   # craft 2 luxury (market required)
  keryx craft --recipe 1 --qty 5        # same as bronze (numeric ID still works)`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// Resolve recipe name → id if --recipe is a name, not a number.
			// A bare numeric --recipe (e.g. "1") is used as the id directly —
			// the help text and error string both advertise this, and LLM
			// agents overwhelmingly pass the numeric form.
			if recipeName != "" {
				if n, numErr := strconv.Atoi(strings.TrimSpace(recipeName)); numErr == nil {
					recipeID = n
					recipeName = ""
				}
			}
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
			// Craft at a chosen province (e.g. a foundry colony), defaulting to the
			// capital — matches --province on build/goods/recruit. Bronze often lives
			// at an ore colony, not the capital, so a capital-only craft was a wall.
			prov := cfg.ProvinceID
			if provinceID != "" {
				prov = provinceID
			}
			c := newClient(cfg)
			path := fmt.Sprintf("/api/v1/worlds/%s/provinces/%s/craft", cfg.WorldID, prov)
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
	cmd.Flags().StringVar(&provinceID, "province", "", "province ID to craft in (default: your capital)")
	return cmd
}
