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

			// 4. Filter to visible, non-sea, non-fog land within radius; rank by distance.
			type cand struct {
				Q             int    `json:"q"`
				R             int    `json:"r"`
				Terrain       string `json:"terrain"`
				Distance      int    `json:"distance"`
				Occupied      bool   `json:"occupied"`
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
				if t.Terrain == "deep_sea" || t.Terrain == "coastal_sea" {
					continue
				}
				d := hexDist(oq, or, t.Q, t.R)
				if d > radius {
					continue
				}
				out = append(out, cand{
					Q: t.Q, R: t.R, Terrain: t.Terrain, Distance: d,
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
			fmt.Printf("Your hex: (%d,%d) · %d visible land hexes within radius %d (d=0 = your hex):\n\n", oq, or, len(out), radius)
			for _, t := range out {
				tag := ""
				if t.Occupied {
					tag = " [settled]"
				}
				dep := ""
				for label, has := range map[string]bool{"copper": t.CopperDeposit, "tin": t.TinDeposit, "cedar": t.CedarDeposit, "silver": t.SilverDeposit} {
					if has {
						dep += " +" + label
					}
				}
				fmt.Printf("  (%3d,%3d) d%-2d %-20s%s%s\n", t.Q, t.R, t.Distance, t.Terrain, dep, tag)
			}
			fmt.Print("\nTo act on a hex, march a land unit there (find unit IDs with `poleia unit list`):\n" +
				"  poleia unit march --unit <id> --q <Q> --r <R>\n" +
				"  poleia unit march --unit <id> --q <Q> --r <R> --intent colonize --name <name>\n" +
				"Intent: colonize — founds a colony when the unit arrives on an empty hex.\n")
			return nil
		},
	}
	cmd.Flags().IntVar(&radius, "radius", 8, "max hex distance to list")
	return cmd
}
