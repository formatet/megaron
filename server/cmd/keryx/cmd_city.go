package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

// cityCmd handles `keryx city [stad]` — the read-only view of a settlement's
// gubbe placement: 18 catchment hexes (ordinal, terrain, per-good occupancy
// and marginal yield) plus building workplaces plus the idle pool. P0-UI
// answer 7's keryx surface for P5 (megaron_plan_fysisk_gubbemodell.md): the
// address space is hex-ordinal 1..18 (server-resolved, hexgrid.RingOrdinal)
// — the same numbers `place`/`staff` take.
func cityCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "city [stad]",
		Short: "Show a settlement's catchment hexes, building slots and idle pool (default: your capital)",
		Long: `Every catchment hex and building workplace, with its own occupancy and the
yield the NEXT citizen placed there would add ("next"). Grain has no cap — any
number of citizens can farm the same hex. A hex or building can support more
than one good; each is its own row.

Use the hex ordinal (#) with ` + "`keryx place`" + ` and the building name with
` + "`keryx staff`" + `.`,
		Example: `  keryx city
  keryx city Knossos`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := newClient(cfg)
			prov := cfg.ProvinceID
			if len(args) == 1 {
				resolved, err := resolveProvince(c, cfg.WorldID, args[0])
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

			// Best-effort second read of /provinces/{id} for the food_gubbar_*
			// figures (megaron_plan_omfordelningsmatningen.md) — a failure here
			// (e.g. founder phase) never blocks the catchment/building listing
			// below, it just means no food line.
			fs, _ := fetchFoodStatus(c, cfg.WorldID, prov)
			surplus := foodSurplus(fs)

			fmt.Printf("Citizens: %d/%d placed, %d idle\n\n", opts.TotalGubbar-opts.PoolSize, opts.TotalGubbar, opts.PoolSize)

			if fs != nil {
				if surplus > 0 {
					fmt.Printf("Food: %d needed, %d placed — %d could move (marked %s below); see `keryx place <good> <ordinal> -n`\n\n",
						fs.Required, fs.Placed, surplus, foodSurplusMarker)
				} else {
					fmt.Printf("Food: %d needed, %d placed\n\n", fs.Required, fs.Placed)
				}
			}

			// foodMarker flags a hex/building's grain or fish row when the city
			// carries a food surplus overall, so the surplus named above points
			// at concrete rows instead of leaving the Wanax to guess which of
			// several hexes it sits on. It marks every occupied food row, not a
			// chosen subset — the specific citizen to move is the Wanax's call,
			// not this command's (no auto-release, no recommendation).
			foodMarker := func(g placementGood) string {
				if surplus > 0 && g.Placed > 0 && isFoodGood(g.GoodKey) {
					return "  " + foodSurplusMarker + " food surplus"
				}
				return ""
			}

			fmt.Println("Catchment:")
			if len(opts.Hexes) == 0 {
				fmt.Println("  (no scouted catchment hexes yet)")
			}
			for _, h := range opts.Hexes {
				if len(h.Goods) == 0 {
					fmt.Printf("  #%-3d %-14s (no producible good)\n", h.HexOrdinal, h.Terrain)
					continue
				}
				for i, g := range h.Goods {
					if i == 0 {
						fmt.Printf("  #%-3d %-14s %s%s\n", h.HexOrdinal, h.Terrain, goodCell(g), foodMarker(g))
					} else {
						fmt.Printf("  %-4s %-14s %s%s\n", "", "", goodCell(g), foodMarker(g))
					}
				}
			}

			if len(opts.Buildings) > 0 {
				fmt.Println("\nBuildings:")
				for _, b := range opts.Buildings {
					for i, g := range b.Goods {
						if i == 0 {
							fmt.Printf("  %-14s L%-2d %s%s\n", b.BuildingType, b.Level, goodCell(g), foodMarker(g))
						} else {
							fmt.Printf("  %-14s %-3s %s%s\n", "", "", goodCell(g), foodMarker(g))
						}
					}
				}
			}
			return nil
		},
	}
	return cmd
}
