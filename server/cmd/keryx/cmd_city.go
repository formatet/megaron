package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

// cityCmd handles `keryx city [city]` — the read-only view of a settlement's
// gubbe placement: 18 catchment hexes (ordinal, terrain, per-good occupancy
// and marginal yield) plus building workplaces plus the idle pool. P0-UI
// answer 7's keryx surface for P5 (megaron_plan_fysisk_gubbemodell.md): the
// address space is hex-ordinal 1..18 (server-resolved, hexgrid.RingOrdinal)
// — the same numbers `place`/`staff` take.
func cityCmd() *cobra.Command {
	var provinceID string
	cmd := &cobra.Command{
		Use:   "city [city]",
		Short: "Show a settlement's catchment hexes, building slots and idle pool (default: your capital)",
		Long: `Every catchment hex and building workplace, with its own occupancy and the
yield the NEXT citizen placed there would add ("next"). Each good has its own
limit on that hex. A hex or building can support more
than one good; each is its own row.

Use the hex ordinal (#) with ` + "`keryx place`" + ` and the building name with
` + "`keryx staff`" + `.`,
		Example: `  keryx city
  keryx city Knossos`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := newClient(cfg)
			prov, err := selectedProvince(c, provinceID, args)
			if err != nil {
				return err
			}
			if err := errNoCityYet(prov); err != nil {
				return err
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

			fmt.Println(citizenScaleNote)
			fmt.Printf("Citizens: %d/%d placed, %d idle\n\n", opts.TotalGubbar-opts.PoolSize, opts.TotalGubbar, opts.PoolSize)

			if fs != nil {
				fmt.Println(fs.Summary)
				if fs.HarvestBlessing != "" {
					fmt.Println(fs.HarvestBlessing)
				}
				fmt.Printf("Food: %s needed for people on best food slots, %d on food\n", citizenCount(fs.Required), fs.Placed)
				if fs.Automatic != "" {
					fmt.Println(fs.Automatic)
				}
				fmt.Println()
			}

			fmt.Println("Catchment:")
			if len(opts.Hexes) == 0 {
				fmt.Println("  (no scouted catchment hexes yet)")
			}
			for _, h := range opts.Hexes {
				// hex-bound production building standing on THIS hex (farm/mine/
				// lumbermill/stonequarry, megaron_plan_byggnad_pa_hex.md §A) —
				// printed once per hex, on its first line, so a player sees at a
				// glance which hex a building is on without cross-referencing
				// `status`'s flat Buildings list.
				bldSuffix := ""
				if h.Building != nil {
					if h.Terrain != "" {
						bldSuffix = fmt.Sprintf("  [%s L%d @ (%d,%d) %s]", h.Building.Type, h.Building.Level, h.HexQ, h.HexR, h.Terrain)
					} else {
						bldSuffix = fmt.Sprintf("  [%s L%d @ (%d,%d)]", h.Building.Type, h.Building.Level, h.HexQ, h.HexR)
					}
				}
				bldSuffix += heldHexNote(h)
				if len(h.Goods) == 0 {
					fmt.Printf("  #%-3d (%d,%d) %-14s (no producible good)%s\n", h.HexOrdinal, h.HexQ, h.HexR, namedRiverTerrain(h.Terrain, h.RiverName), bldSuffix)
					continue
				}
				for i, g := range h.Goods {
					if i == 0 {
						fmt.Printf("  #%-3d (%d,%d) %-14s %s%s%s\n", h.HexOrdinal, h.HexQ, h.HexR, namedRiverTerrain(h.Terrain, h.RiverName), goodCell(g), "", bldSuffix)
					} else {
						fmt.Printf("  %-4s %-14s %s%s\n", "", "", goodCell(g), "")
					}
				}
			}

			if len(opts.Buildings) > 0 {
				fmt.Println("\nBuildings:")
				for _, b := range opts.Buildings {
					for i, g := range b.Goods {
						if i == 0 {
							fmt.Printf("  %-14s L%-2d %s%s\n", b.BuildingType, b.Level, goodCell(g), "")
						} else {
							fmt.Printf("  %-14s %-3s %s%s\n", "", "", goodCell(g), "")
						}
					}
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&provinceID, "province", "", "city name or province ID (default: capital)")
	return cmd
}

// errNoCityYet turns the empty province ID of a player who has not founded
// anything yet into a pointer at the founding flow (instead of the server's 400).
func errNoCityYet(prov string) error {
	if prov == "" {
		return fmt.Errorf("you have no city yet — found one first: keryx founding status")
	}
	return nil
}
