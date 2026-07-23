package main

import (
	"encoding/json"
	"fmt"
	"math"
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

// compassDirection returns an 8-point compass bearing (N/NE/E/SE/S/SW/W/NW) from
// origin to target. The CLI has no visual grid, so raw (q,r) pairs alone make
// adjacency/direction hard to judge — this gives a coarse orientation aid
// alongside the exact coordinates already shown. Mirrors the angle bucketing in
// internal/province/hex.go's FuzzyBearing (kept local rather than imported —
// the CLI is a pure HTTP client, deliberately decoupled from server internals).
func compassDirection(oq, or, tq, tr int) string {
	if oq == tq && or == tr {
		return ""
	}
	dq := float64(tq - oq)
	dr := float64(tr - or)
	x := dq + dr/2
	y := dr * math.Sqrt(3) / 2
	angle := math.Atan2(y, x)
	if angle < 0 {
		angle += 2 * math.Pi
	}
	sectors := [8]string{"E", "NE", "N", "NW", "W", "SW", "S", "SE"}
	return sectors[int(math.Round(angle/(math.Pi/4)))%8]
}

func mapCmd() *cobra.Command {
	var radius int
	var provinceID string

	cmd := &cobra.Command{
		Use:   "map",
		Short: "Show nearby visible land hexes — colony candidates and ore deposits (sorted by distance)",
		Example: `  keryx map
  keryx map --radius 12
  keryx map --province <province-id>   # inspect a colony's surroundings`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)

			// Default to the capital; --province lets you inspect any province you own
			// (the server verifies ownership), mirroring `build`/`allocate`.
			prov := cfg.ProvinceID
			if provinceID != "" {
				resolved, err := resolveProvince(c, cfg.WorldID, provinceID)
				if err != nil {
					return err
				}
				prov = resolved
			}

			// 1. Own coordinates from status — or, in founder-fas (ingen
			// province än), from the wandering host's position.
			var oq, or int
			if prov == "" {
				fp, err := fetchFoundingStatus(c)
				if err != nil {
					return err
				}
				if !fp.Active || fp.Q == nil || fp.R == nil {
					return fmt.Errorf("no province in config and no active founder phase — rejoin the world or set province_id")
				}
				oq, or = *fp.Q, *fp.R
			} else {
				statusData, err := c.get(fmt.Sprintf("/api/v1/worlds/%s/provinces/%s", cfg.WorldID, prov))
				if err != nil {
					return err
				}
				var status struct {
					MapTile struct{ Q, R int } `json:"map_tile"`
				}
				_ = json.Unmarshal(statusData, &status)
				oq, or = status.MapTile.Q, status.MapTile.R
			}

			// 2. All visible tiles (three tiers: live / remembered / fog — see
			// temenos_synlighet.md). Fog tiles carry only q/r + frontier.
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
				Tier          string `json:"tier"`
				Frontier      bool   `json:"frontier"`
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

			// 4. Split into three tiers within radius (temenos_synlighet.md):
			// live (fresh, full detail) / remembered (dimmed, frozen snapshot) /
			// fog (frontier only — the edge of the known world). Sea tiles are
			// included in live/remembered so the player can see where the coast is.
			type cand struct {
				Q             int    `json:"q"`
				R             int    `json:"r"`
				Terrain       string `json:"terrain"`
				Coastal       bool   `json:"coastal,omitempty"`
				Distance      int    `json:"distance"`
				Tier          string `json:"tier"`
				Occupied      bool   `json:"occupied,omitempty"`
				CopperDeposit bool   `json:"copper_deposit,omitempty"`
				TinDeposit    bool   `json:"tin_deposit,omitempty"`
				CedarDeposit  bool   `json:"cedar_deposit,omitempty"`
				SilverDeposit bool   `json:"silver_deposit,omitempty"`
			}
			var out []cand
			type frontierHex struct {
				Q, R     int
				Distance int
			}
			var frontier []frontierHex
			for _, t := range tiles {
				d := hexDist(oq, or, t.Q, t.R)
				if d > radius {
					continue
				}
				if !t.Visible || t.Terrain == "fog" {
					if t.Frontier {
						frontier = append(frontier, frontierHex{Q: t.Q, R: t.R, Distance: d})
					}
					continue
				}
				tier := t.Tier
				if tier == "" {
					tier = "live" // back-compat with servers predating the tier field
				}
				out = append(out, cand{
					Q: t.Q, R: t.R, Terrain: t.Terrain, Coastal: t.Coastal, Distance: d, Tier: tier,
					Occupied:      occupied[[2]int{t.Q, t.R}],
					CopperDeposit: t.CopperDeposit, TinDeposit: t.TinDeposit,
					CedarDeposit: t.CedarDeposit, SilverDeposit: t.SilverDeposit,
				})
			}
			sort.Slice(out, func(i, j int) bool { return out[i].Distance < out[j].Distance })
			sort.Slice(frontier, func(i, j int) bool { return frontier[i].Distance < frontier[j].Distance })

			if jsonMode {
				b, _ := json.Marshal(map[string]any{"tiles": out, "frontier": frontier})
				fmt.Println(string(b))
				return nil
			}

			// Count visible sea and coastal land hexes for the summary line.
			seaCount := 0
			coastalLand := 0
			liveCount := 0
			for _, t := range out {
				if t.Tier == "live" {
					liveCount++
				}
				if t.Terrain == "deep_sea" || t.Terrain == "coastal_sea" {
					seaCount++
				} else if t.Coastal {
					coastalLand++
				}
			}
			fmt.Printf("Your hex: (%d,%d) · radius %d · %d known hexes (%d live, %d remembered; %d sea, %d coastal land):\n\n",
				oq, or, radius, len(out), liveCount, len(out)-liveCount, seaCount, coastalLand)
			for _, t := range out {
				dim := ""
				if t.Tier == "remembered" {
					dim = " [remembered]"
				}
				bearing := compassDirection(oq, or, t.Q, t.R)
				if t.Terrain == "deep_sea" || t.Terrain == "coastal_sea" {
					// Sea hexes: no deposit/occupied tags, just label
					fmt.Printf("  (%3d,%3d) d%-2d %-3s %-20s[sea]%s\n", t.Q, t.R, t.Distance, bearing, t.Terrain, dim)
					continue
				}
				tag := ""
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
				fmt.Printf("  (%3d,%3d) d%-2d %-3s %-20s%s%s%s%s\n", t.Q, t.R, t.Distance, bearing, t.Terrain, dep, coastTag, tag, dim)
			}
			if len(frontier) > 0 {
				fmt.Printf("\nFrontier — unexplored hexes bordering the known world (%d, nearest first):\n", len(frontier))
				max := len(frontier)
				if max > 15 {
					max = 15
				}
				for _, f := range frontier[:max] {
					fmt.Printf("  (%3d,%3d) d%-2d %-3s\n", f.Q, f.R, f.Distance, compassDirection(oq, or, f.Q, f.R))
				}
				if len(frontier) > max {
					fmt.Printf("  … and %d more\n", len(frontier)-max)
				}
			}
			fmt.Print("\nTo act on a hex, march a land unit there (find unit IDs with `keryx unit list`):\n" +
				"  keryx unit march --unit <id> --q <Q> --r <R>\n" +
				"  keryx unit march --unit <id> --q <Q> --r <R> --intent colonize --name <name>\n" +
				"Intent: colonize — founds a colony when the unit arrives on an empty hex.\n" +
				"Note: ore on mountain terrain is impassable — colonize an ADJACENT passable hex\n" +
				"      so the deposit falls in the new colony's catchment.\n" +
				"To explore: march a unit toward a frontier hex above — the route is revealed\n" +
				"      and remembered on arrival, pushing the frontier outward.\n")
			return nil
		},
	}
	cmd.Flags().SortFlags = false
	cmd.Flags().StringVar(&provinceID, "province", "", "province to center the map on (default: your capital)")
	cmd.Flags().IntVar(&radius, "radius", 8, "max hex distance to list")
	return cmd
}
