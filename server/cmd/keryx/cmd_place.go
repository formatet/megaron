package main

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

// placeCmd handles `keryx place <roll> <ordinal>` — put the next free gubbe
// on catchment hex <ordinal> (1..18, from `keryx city`) doing good <roll>.
// The Wanax chooses WHERE, not which numbered gubbe (P0-UI answer 2) — the
// server assigns the gubbe ordinal. Moving a placed gubbe is free: run
// `place` again with a different ordinal, the old spot just gets unplaced
// separately (`keryx staff` for buildings, or re-run `place` after an
// implicit unplace once the web/keryx "move" affordance lands).
func placeCmd() *cobra.Command {
	var provinceID string
	cmd := &cobra.Command{
		Use:   "place <roll> <ordinal>",
		Short: "Place the next idle gubbe on catchment hex <ordinal> doing <roll> (good)",
		Long: `<roll> is a good key (grain, timber, fish, ...) and <ordinal> is the hex
number (1..18) from ` + "`keryx city`" + `. Rejected if the hex has no such production
option, is already full for that good (grain is never full), or hasn't been
scouted yet.`,
		Example: `  keryx place grain 5
  keryx place livestock 5 --province <prov-id>`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			roll := args[0]
			ordinal, err := strconv.Atoi(args[1])
			if err != nil || ordinal < 1 {
				return fmt.Errorf("ordinal must be a positive number (1..18, see `keryx city`), got %q", args[1])
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

			data, err := c.post(fmt.Sprintf("/api/v1/worlds/%s/provinces/%s/placements", cfg.WorldID, prov),
				map[string]any{"target_kind": "hex", "hex_ordinal": ordinal, "good_key": roll})
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			var resp struct {
				GubbeOrdinal int `json:"gubbe_ordinal"`
			}
			if jerr := json.Unmarshal(data, &resp); jerr != nil {
				return jerr
			}
			fmt.Printf("Placed gubbe #%d on hex #%d doing %s.\n", resp.GubbeOrdinal, ordinal, roll)

			// Show what changed — same "confirm against the resulting state" pattern
			// as allocate's post-write echo, using the SAME hex row `city` would show.
			if opts, oerr := fetchPlacementOptions(c, cfg.WorldID, prov); oerr == nil {
				for _, h := range opts.Hexes {
					if h.HexOrdinal != ordinal {
						continue
					}
					for _, g := range h.Goods {
						if g.GoodKey == roll {
							fmt.Printf("  #%-3d %-14s %s\n", h.HexOrdinal, h.Terrain, goodCell(g))
						}
					}
				}
				fmt.Printf("  %d idle gubbar left.\n", opts.PoolSize)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&provinceID, "province", "", "province ID/name to place in (default: your capital)")
	return cmd
}
