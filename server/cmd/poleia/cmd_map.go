package main

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/spf13/cobra"
)

// hexDist returns the axial hex distance between two coordinates.
func hexDist(q1, r1, q2, r2 int) int {
	dq, dr := q1-q2, r1-r2
	s := dq + dr
	abs := func(x int) int {
		if x < 0 {
			return -x
		}
		return x
	}
	return (abs(dq) + abs(dr) + abs(s)) / 2
}

func mapCmd() *cobra.Command {
	var radius int

	cmd := &cobra.Command{
		Use:   "map",
		Short: "Show nearby visible land hexes — colony/outpost candidates and ore deposits (sorted by distance)",
		Example: `  poleia map
  poleia map --radius 12`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)

			// 1. Own coordinates from status.
			statusData, err := c.get(fmt.Sprintf("/api/v1/worlds/%s/provinces/%s", cfg.WorldID, cfg.ProvinceID))
			if err != nil {
				return err
			}
			var status struct {
				MapTile struct{ Q, R int } `json:"map_tile"`
			}
			_ = json.Unmarshal(statusData, &status)
			oq, or := status.MapTile.Q, status.MapTile.R

			// 2. All visible tiles.
			mapData, err := c.get(fmt.Sprintf("/api/v1/worlds/%s/map", cfg.WorldID))
			if err != nil {
				return err
			}
			var tiles []struct {
				Q             int    `json:"q"`
				R             int    `json:"r"`
				Terrain       string `json:"terrain"`
				Coastal       bool   `json:"coastal"`
				Visible       bool   `json:"visible"`
				CopperDeposit bool   `json:"copper_deposit"`
				TinDeposit    bool   `json:"tin_deposit"`
				CedarDeposit  bool   `json:"cedar_deposit"`
				SilverDeposit bool   `json:"silver_deposit"`
			}
			_ = json.Unmarshal(mapData, &tiles)

			// 3. Settled provinces → occupied set.
			provData, _ := c.get(fmt.Sprintf("/api/v1/worlds/%s/provinces", cfg.WorldID))
			var markers []struct {
				Q int `json:"q"`
				R int `json:"r"`
			}
			_ = json.Unmarshal(provData, &markers)
			occupied := map[[2]int]bool{}
			for _, m := range markers {
				occupied[[2]int{m.Q, m.R}] = true
			}

			// 4. Filter to visible, non-fog tiles within radius; rank by distance.
			// Sea tiles are included so the player can see where the coast is.
			type cand struct {
				Q             int    `json:"q"`
				R             int    `json:"r"`
				Terrain       string `json:"terrain"`
				Coastal       bool   `json:"coastal,omitempty"`
				Distance      int    `json:"distance"`
				Occupied      bool   `json:"occupied,omitempty"`
				CopperDeposit bool   `json:"copper_deposit,omitempty"`
				TinDeposit    bool   `json:"tin_deposit,omitempty"`
				CedarDeposit  bool   `json:"cedar_deposit,omitempty"`
				SilverDeposit bool   `json:"silver_deposit,omitempty"`
			}
			var out []cand
			for _, t := range tiles {
				if !t.Visible || t.Terrain == "fog" {
					continue
				}
				d := hexDist(oq, or, t.Q, t.R)
				if d > radius {
					continue
				}
				out = append(out, cand{
					Q: t.Q, R: t.R, Terrain: t.Terrain, Coastal: t.Coastal, Distance: d,
					Occupied:      occupied[[2]int{t.Q, t.R}],
					CopperDeposit: t.CopperDeposit, TinDeposit: t.TinDeposit,
					CedarDeposit: t.CedarDeposit, SilverDeposit: t.SilverDeposit,
				})
			}
			sort.Slice(out, func(i, j int) bool { return out[i].Distance < out[j].Distance })

			if jsonMode {
				b, _ := json.Marshal(out)
				fmt.Println(string(b))
				return nil
			}

			// Count visible sea and coastal land hexes for the summary line.
			seaCount := 0
			coastalLand := 0
			for _, t := range out {
				if t.Terrain == "deep_sea" || t.Terrain == "coastal_sea" {
					seaCount++
				} else if t.Coastal {
					coastalLand++
				}
			}
			fmt.Printf("Your hex: (%d,%d) · radius %d · %d visible hexes (%d sea, %d coastal land):\n\n",
				oq, or, radius, len(out), seaCount, coastalLand)
			for _, t := range out {
				tag := ""
				if t.Terrain == "deep_sea" || t.Terrain == "coastal_sea" {
					// Sea hexes: no deposit/occupied tags, just label
					fmt.Printf("  (%3d,%3d) d%-2d %-20s[sea]\n", t.Q, t.R, t.Distance, t.Terrain)
					continue
				}
				if t.Occupied {
					tag = " [settled]"
				}
				coastTag := ""
				if t.Coastal {
					coastTag = " [coastal—can build harbour]"
				}
				dep := ""
				impassable := t.Terrain == "mountain_limestone" || t.Terrain == "mountain_red"
				for _, item := range []struct {
					label string
					has   bool
				}{
					{"copper", t.CopperDeposit},
					{"tin", t.TinDeposit},
					{"cedar", t.CedarDeposit},
					{"silver", t.SilverDeposit},
				} {
					if item.has {
						dep += " +" + item.label
						if impassable {
							dep += "(colonize adjacent hex)"
						}
					}
				}
				fmt.Printf("  (%3d,%3d) d%-2d %-20s%s%s%s\n", t.Q, t.R, t.Distance, t.Terrain, dep, coastTag, tag)
			}
			fmt.Print("\nTo act on a hex, march a land unit there (find unit IDs with `poleia unit list`):\n" +
				"  poleia unit march --unit <id> --q <Q> --r <R>\n" +
				"  poleia unit march --unit <id> --q <Q> --r <R> --intent colonize --name <name>\n" +
				"Intent: colonize — founds a colony when the unit arrives on an empty hex.\n" +
				"Note: ore on mountain terrain is impassable — colonize an ADJACENT passable hex\n" +
				"      so the deposit falls in the new colony's catchment.\n")
			return nil
		},
	}
	cmd.Flags().IntVar(&radius, "radius", 8, "max hex distance to list")
	return cmd
}
