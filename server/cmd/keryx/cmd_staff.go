package main

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

// staffCmd handles `keryx staff <byggnad> [+n|-n]` — building workplaces
// (P0-UI's "STADENS ARBETSPLATSER") aren't addressed by hex ordinal, so this
// is the `place` sibling for them. No delta given → read-only view of that
// building's current staffing (same "no flags = show state" convention as
// `allocate`). A building with more than one producible good is out of scope
// today (none exist yet, economy.LoadBuildingProductionOptions's doc
// comment) — staff refuses to guess which one a bare delta means.
func staffCmd() *cobra.Command {
	var provinceID string
	cmd := &cobra.Command{
		Use:   "staff <byggnad> [+n|-n]",
		Short: "Place or remove n gubbar from a building workplace (default: show current staffing)",
		Long: `<byggnad> is a building type (e.g. stonequarry). +n places n more idle
gubbar there; -n returns n gubbar to the idle pool. Omit the delta to just
see current occupancy.`,
		Example: `  keryx staff stonequarry
  keryx staff stonequarry +2
  keryx staff stonequarry -1 --province <prov-id>`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			buildingType := args[0]
			c := newClient(cfg)
			prov := cfg.ProvinceID
			if provinceID != "" {
				resolved, rerr := resolveProvince(c, cfg.WorldID, provinceID)
				if rerr != nil {
					return rerr
				}
				prov = resolved
			}

			opts, err := fetchPlacementOptions(c, cfg.WorldID, prov)
			if err != nil {
				return err
			}
			var building *placementBuilding
			for i := range opts.Buildings {
				if opts.Buildings[i].BuildingType == buildingType {
					building = &opts.Buildings[i]
					break
				}
			}
			if building == nil {
				return fmt.Errorf("no built workplace %q here (see `keryx city` for what's built)", buildingType)
			}
			if len(building.Goods) == 0 {
				return fmt.Errorf("%s has no producible good yet", buildingType)
			}
			if len(args) == 1 {
				if jsonMode {
					printJSON(building)
					return nil
				}
				for _, g := range building.Goods {
					fmt.Printf("%-14s L%-2d %s\n", buildingType, building.Level, goodCell(g))
				}
				return nil
			}
			if len(building.Goods) > 1 {
				return fmt.Errorf("%s has more than one producible good — not supported yet, use `keryx city` and `keryx place`", buildingType)
			}
			good := building.Goods[0]

			delta, derr := strconv.Atoi(args[1])
			if derr != nil || delta == 0 {
				return fmt.Errorf("delta must be a non-zero signed number (e.g. +2 or -1), got %q", args[1])
			}

			path := fmt.Sprintf("/api/v1/worlds/%s/provinces/%s/placements", cfg.WorldID, prov)
			applied := 0
			var stopErr error
			if delta > 0 {
				for ; applied < delta; applied++ {
					if _, err := c.post(path, map[string]any{"target_kind": "building", "building_type": buildingType, "good_key": good.GoodKey}); err != nil {
						if applied == 0 && !jsonMode {
							return err
						}
						stopErr = err
						break
					}
				}
				if !jsonMode {
					if applied == delta {
						fmt.Printf("Placed %d gubbe(s) on %s.\n", applied, buildingType)
					} else {
						fmt.Printf("Placed %d of %d requested (stopped: %v).\n", applied, delta, stopErr)
					}
				}
			} else {
				want := -delta
				if want > len(good.PlacedOrdinals) {
					want = len(good.PlacedOrdinals)
				}
				for ; applied < want; applied++ {
					ordinal := good.PlacedOrdinals[applied]
					if _, err := c.delete(fmt.Sprintf("%s/%d", path, ordinal)); err != nil {
						stopErr = err
						break
					}
				}
				applied = -applied // report removals as a negative delta, symmetric with the request
				if !jsonMode {
					removed := -applied
					if removed == want {
						if want < -delta {
							fmt.Printf("Removed %d gubbe(s) — only %d were staffed there.\n", removed, want)
						} else {
							fmt.Printf("Removed %d gubbe(s) from %s.\n", removed, buildingType)
						}
					} else {
						fmt.Printf("Removed %d of %d requested (stopped: %v).\n", removed, want, stopErr)
					}
				}
			}

			opts2, oerr := fetchPlacementOptions(c, cfg.WorldID, prov)
			if oerr != nil {
				if applied == 0 && stopErr != nil {
					return stopErr
				}
				return oerr
			}
			var after *placementGood
			for _, b := range opts2.Buildings {
				if b.BuildingType != buildingType {
					continue
				}
				for i := range b.Goods {
					if b.Goods[i].GoodKey == good.GoodKey {
						after = &b.Goods[i]
					}
				}
			}
			if jsonMode {
				printJSON(map[string]any{
					"building_type": buildingType,
					"requested":     delta,
					"applied":       applied,
					"good":          after,
					"idle_after":    opts2.PoolSize,
				})
				if applied == 0 && stopErr != nil {
					return stopErr
				}
				return nil
			}
			if after != nil {
				fmt.Printf("  %s\n", goodCell(*after))
			}
			fmt.Printf("  %d idle gubbar left.\n", opts2.PoolSize)
			if applied == 0 && stopErr != nil {
				return stopErr
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&provinceID, "province", "", "province ID/name (default: your capital)")
	// A bare "-1" delta is otherwise misparsed by pflag as an attempted
	// shorthand flag ("unknown shorthand flag: '1' in -1"). Disabling
	// interspersion stops flag parsing at the first positional (<byggnad>),
	// so --province must come before it: `staff --province X foo -1`, not
	// after — the standard cobra/pflag fix for a command that takes a
	// signed-number positional.
	cmd.Flags().SetInterspersed(false)
	return cmd
}
