package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

// idleCmd handles `keryx idle` — how many gubbar are sitting unplaced right
// now (P0-UI's "synlig ledig-pool", not auto-placed). Deliberately thin:
// full browsing of what's open belongs to `keryx city`, this just answers
// "do I have work to do here."
func idleCmd() *cobra.Command {
	var provinceID string
	cmd := &cobra.Command{
		Use:   "idle",
		Short: "Show how many citizens are idle (unplaced) in a settlement (default: your capital)",
		Args:  noPositionalArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)
			prov := cfg.ProvinceID
			if provinceID != "" {
				resolved, err := resolveProvince(c, cfg.WorldID, provinceID)
				if err != nil {
					return err
				}
				prov = resolved
			}
			opts, err := fetchPlacementOptions(c, cfg.WorldID, prov)
			if err != nil {
				return err
			}
			if jsonMode {
				printJSON(opts)
				return nil
			}

			// Best-effort second read for food_gubbar_* (see cmd_city.go's
			// fetchFoodStatus) — a settlement can show zero idle and still be
			// 63 citizens misplaced on food (megaron_plan_omfordelningsmatningen.md,
			// Timothy 2026-09-02); "idle" alone is not the whole allocation
			// picture, only the unplaced corner of it.
			fs, _ := fetchFoodStatus(c, cfg.WorldID, prov)
			surplus := foodSurplus(fs)

			if opts.PoolSize == 0 {
				fmt.Printf("All %d citizens are placed.\n", opts.TotalGubbar)
				if surplus > 0 {
					fmt.Printf("%d of them are on food beyond what's needed — zero idle isn't the same as well placed. Run `keryx city` to see where.\n", surplus)
				}
				return nil
			}
			fmt.Printf("%d of %d citizens are idle.\n", opts.PoolSize, opts.TotalGubbar)
			if surplus > 0 {
				fmt.Printf("%d more are on food beyond what's needed, on top of the idle above.\n", surplus)
			}
			fmt.Println("Run `keryx city` to see open catchment hexes and buildings.")
			return nil
		},
	}
	cmd.Flags().StringVar(&provinceID, "province", "", "province ID/name (default: your capital)")
	return cmd
}
