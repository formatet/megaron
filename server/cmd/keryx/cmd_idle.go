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

			if opts.PoolSize == 0 {
				fmt.Printf("All %s placed.\n", citizenCount(opts.TotalGubbar))
				return nil
			}
			fmt.Printf("%s idle (of %s).\n", citizenCount(opts.PoolSize), citizenCount(opts.TotalGubbar))
			fmt.Println("Run `keryx city` to see open catchment hexes and buildings.")
			return nil
		},
	}
	cmd.Flags().StringVar(&provinceID, "province", "", "province ID/name (default: your capital)")
	return cmd
}
