package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// recipeNameToID is the OFFLINE fast path for --recipe <name>: the recipes
// that exist today, resolved without a round-trip so the common invocation
// stays as cheap as the numeric form.
// ID 1 = bronze (copper + tin → bronze, requires foundry)
//
// A miss here is NOT an error — resolveRecipeID falls through to the server's
// /api/v1/recipes catalogue, so a recipe added by a future migration is
// reachable by name without touching this map. Keeping the map means the
// name players actually type never pays for a request; keeping the fallback
// means this map going stale degrades to "one extra GET", not "unknown recipe".
var recipeNameToID = map[string]int{
	"bronze": 1,
}

// recipeCatalogueEntry is the subset of GET /api/v1/recipes this command needs.
// The endpoint returns the full ingredient list too; craft only has to turn a
// name into an id, so it deliberately ignores the rest rather than modelling a
// shape it would then have to keep in sync.
type recipeCatalogueEntry struct {
	ID        int    `json:"id"`
	OutputKey string `json:"output_key"`
}

// resolveRecipeID turns a --recipe name into a recipe id: the offline map
// first, then the server's catalogue. Never called for a numeric --recipe.
//
// Error strings name what the SERVER actually has whenever it could be
// reached, because a frozen list in an error message is the same staleness
// bug one layer down — a player (or an agent) told "use one of: bronze" has
// no way to discover a second recipe that already exists.
func resolveRecipeID(c *Client, name string) (int, error) {
	key := strings.ToLower(strings.TrimSpace(name))
	if id, ok := recipeNameToID[key]; ok {
		return id, nil
	}

	data, err := c.get("/api/v1/recipes")
	if err != nil {
		// Offline (or the server is down): fall back to naming what we know
		// rather than swallowing the miss. Say WHY the list may be partial so
		// the reader knows a valid-but-unlisted name is possible.
		return 0, fmt.Errorf("unknown recipe %q — known recipes: %s (could not reach the server for the full list: %v)",
			name, strings.Join(knownRecipeNames(), ", "), err)
	}
	var catalogue []recipeCatalogueEntry
	if err := json.Unmarshal(data, &catalogue); err != nil {
		return 0, fmt.Errorf("unknown recipe %q — known recipes: %s (could not read the server's recipe list: %v)",
			name, strings.Join(knownRecipeNames(), ", "), err)
	}
	names := make([]string, 0, len(catalogue))
	for _, r := range catalogue {
		if strings.EqualFold(r.OutputKey, key) {
			return r.ID, nil
		}
		names = append(names, r.OutputKey)
	}
	sort.Strings(names)
	return 0, fmt.Errorf("unknown recipe %q — use one of: %s (or the numeric id)",
		name, strings.Join(names, ", "))
}

// knownRecipeNames returns the offline map's names, sorted — Go randomises map
// iteration, so without this the same mistake prints a different error every
// run, which is noise for a human and a diff for an agent comparing output.
func knownRecipeNames() []string {
	names := make([]string, 0, len(recipeNameToID))
	for k := range recipeNameToID {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

func craftCmd() *cobra.Command {
	var qty float64
	var recipeID int
	var recipeName string
	var provinceID string

	cmd := &cobra.Command{
		Use:   "craft",
		Short: "Craft a recipe at your settlement (bronze@foundry)",
		Example: `  keryx craft --recipe bronze --qty 5   # smelt 5 bronze (foundry required)
  keryx craft --recipe 1 --qty 5        # same as bronze (numeric ID still works)`,
		Args: rejectPositionalArgs("recipe"),
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
			c := newClient(cfg)
			if recipeName != "" {
				id, err := resolveRecipeID(c, recipeName)
				if err != nil {
					return err
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
	cmd.Flags().StringVarP(&recipeName, "recipe", "r", "", "recipe name (bronze, or any the server publishes) or numeric id")
	cmd.Flags().StringVar(&provinceID, "province", "", "province ID to craft in (default: your capital)")
	return cmd
}
