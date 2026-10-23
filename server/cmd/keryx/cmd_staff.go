package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

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
	var provinceID, god string
	cmd := &cobra.Command{
		Use:   "staff <byggnad> [+n|-n]",
		Short: "Show a named building workplace, or place/remove citizens with +n/-n",
		Long: `<building> is a city workplace (e.g. olive_press). Hex buildings use place. +n places n more idle
citizens there; -n returns n citizens to the idle pool. The building name is required. Omit the delta to just
see current occupancy. A market has no workplaces and needs no workers. An unstaffed shipyard stands still; one worker keeps it working. A city may hold one temple per god: with more than one, name it with --god.`,
		Example: `  keryx staff shipyard +1
  keryx staff temple +1 --god potnia
  keryx staff olive_press
  keryx staff olive_press +2
  keryx staff olive_press -1 --province <prov-id>`,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("building name is required: keryx staff <building> (see keryx city for workplaces, or keryx roster for all assignments)")
			}
			return cobra.RangeArgs(1, 2)(cmd, args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			buildingType := args[0]
			if buildingType == "market" {
				const reason = "The market has no workplaces and needs no workers."
				if len(args) > 1 {
					return fmt.Errorf("%s", reason)
				}
				if jsonMode {
					printJSON(map[string]any{"building_type": "market", "workplaces": 0, "reason": reason})
				} else {
					fmt.Println(reason)
				}
				return nil
			}
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
			var gods []string
			for i := range opts.Buildings {
				b := &opts.Buildings[i]
				if b.BuildingType != buildingType {
					continue
				}
				if b.GodID != "" {
					gods = append(gods, b.GodID)
					if god != "" && b.GodID != god {
						continue
					}
				}
				if building != nil {
					// Two temples and no --god: the choice is the Wanax's.
					return fmt.Errorf("this city has a temple to each of: %s — choose one with --god", strings.Join(gods, ", "))
				}
				building = b
			}
			if building == nil && god != "" && len(gods) > 0 {
				return fmt.Errorf("no temple to %s here — this city has: %s", god, strings.Join(gods, ", "))
			}
			if building == nil {
				for _, h := range opts.Hexes {
					if h.Building != nil && h.Building.Type == buildingType {
						prefix := "keryx place"
						if prov != cfg.ProvinceID {
							prefix += " --province " + prov
						}
						roles := []string{}
						for _, g := range h.Goods {
							if g.MarginalYield > 0 && hexBuildingGood(buildingType, g.GoodKey) {
								roles = append(roles, fmt.Sprintf("%s %s %d", prefix, g.GoodKey, h.HexOrdinal))
							}
						}
						delta := "+1"
						if len(args) > 1 {
							delta = args[1]
						}
						return fmt.Errorf("the %s is a hex building — staff it with: %s %s (hex #%d)", buildingType, strings.Join(roles, " or "), delta, h.HexOrdinal)
					}
				}

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
				label := buildingType
				if building.Name != "" {
					label = building.Name
				}
				for _, g := range building.Goods {
					fmt.Printf("%-14s L%-2d %s\n", label, building.Level, goodCell(g))
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
					body := map[string]any{"target_kind": "building", "building_type": buildingType, "good_key": good.GoodKey}
					if building.GodID != "" {
						body["god"] = building.GodID
					}
					if _, err := c.post(path, body); err != nil {
						if applied == 0 && !jsonMode {
							return err
						}
						stopErr = err
						break
					}
				}
				if !jsonMode {
					if applied == delta {
						fmt.Printf("Placed %s on %s.\n", citizenCount(applied), buildingType)
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
							fmt.Printf("Removed %s — only %d were staffed there.\n", citizenCount(removed), want)
						} else {
							fmt.Printf("Removed %s from %s.\n", citizenCount(removed), buildingType)
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
			fmt.Printf("  %s left in the idle pool.\n", citizenCount(opts2.PoolSize))
			if applied == 0 && stopErr != nil {
				return stopErr
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&provinceID, "province", "", "province ID/name (default: your capital)")
	cmd.Flags().StringVar(&god, "god", "", "which temple, when the city has one to several gods")
	// A bare "-1" delta is otherwise misparsed by pflag as an attempted
	// shorthand flag ("unknown shorthand flag: '1' in -1"). Disabling
	// interspersion stops flag parsing at the first positional (<byggnad>),
	// so --province must come before it: `staff --province X foo -1`, not
	// after — the standard cobra/pflag fix for a command that takes a
	// signed-number positional.
	cmd.Flags().SetInterspersed(false)
	return cmd
}

// fetchBuiltBuildingLevel checks the settlement's real building list (GET
// .../buildings — every built row, unlike placement-options which only lists
// buildings with a production_rules row) for buildingType. Best-effort: any
// transport or decode error reads as "not built" rather than surfacing a
// second error on top of the one staffCmd is already constructing.
func fetchBuiltBuildingLevel(c *Client, worldID, provinceID, buildingType string) (level int, built bool) {
	data, err := c.get(fmt.Sprintf("/api/v1/worlds/%s/provinces/%s/buildings", worldID, provinceID))
	if err != nil {
		return 0, false
	}
	var rows []struct {
		Type  string `json:"type"`
		Level int    `json:"level"`
	}
	if err := json.Unmarshal(data, &rows); err != nil {
		return 0, false
	}
	for _, r := range rows {
		if r.Type == buildingType {
			return r.Level, true
		}
	}
	return 0, false
}

func hexBuildingGood(building, good string) bool {
	switch building {
	case "farm":
		return good == "grain"
	case "lumbermill":
		return good == "timber" || good == "cedar"
	case "stonequarry":
		return good == "stone"
	case "mine":
		return good == "copper" || good == "tin" || good == "silver"
	}
	return false
}
