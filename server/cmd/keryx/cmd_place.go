package main

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

// placeCmd handles `keryx place <roll> <ordinal> [+n|-n]` — put or remove
// gubbar on catchment hex <ordinal> (1..18, from `keryx city`) doing good
// <roll>. The Wanax chooses WHERE, not which numbered gubbe (P0-UI answer 2)
// — the server assigns the gubbe ordinal on placement. Moving a placed gubbe
// is free and immediate: unplace it here with a negative delta, then
// `keryx staff <byggnad> +1` (or `place` on a different hex) — same delta
// convention as `staff` (cmd_staff.go), its sibling for building slots.
func placeCmd() *cobra.Command {
	var provinceID string
	cmd := &cobra.Command{
		Use:   "place <roll> <ordinal> [+n|-n]",
		Short: "Place or remove gubbar on catchment hex <ordinal> doing <roll> (good)",
		Long: `<roll> is a good key (grain, timber, fish, ...) and <ordinal> is the hex
number (1..18) from ` + "`keryx city`" + `. Rejected if the hex has no such production
option, is already full for that good (grain is never full), or hasn't been
scouted yet.

Omit the delta to place a single gubbe (default +1, unchanged from before).
+n places n more; -n returns n gubbar from that hex to the idle pool — free
and immediate. To move a gubbe from a hex into a building:

  keryx place grain 5 -1
  keryx staff olive_press +1`,
		Example: `  keryx place grain 5
  keryx place livestock 5 --province <prov-id>
  keryx place grain 5 +2
  keryx place grain 5 -1`,
		Args: cobra.RangeArgs(2, 3),
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
			path := fmt.Sprintf("/api/v1/worlds/%s/provinces/%s/placements", cfg.WorldID, prov)

			if len(args) == 2 {
				return placeSingle(c, path, cfg.WorldID, prov, ordinal, roll)
			}

			delta, derr := strconv.Atoi(args[2])
			if derr != nil || delta == 0 {
				return fmt.Errorf("delta must be a non-zero signed number (e.g. +2 or -1), got %q", args[2])
			}
			if delta > 0 {
				return placeMany(c, path, cfg.WorldID, prov, ordinal, roll, delta)
			}
			return unplaceMany(c, path, cfg.WorldID, prov, ordinal, roll, -delta)
		},
	}
	cmd.Flags().StringVar(&provinceID, "province", "", "province ID/name to place in (default: your capital)")
	// A bare "-1" delta is otherwise misparsed by pflag as an attempted
	// shorthand flag ("unknown shorthand flag: '1' in -1"). Disabling
	// interspersion stops flag parsing at the first positional (<roll>),
	// so --province must come before it — the standard cobra/pflag fix for a
	// command that takes a signed-number positional (same as `staff`).
	cmd.Flags().SetInterspersed(false)
	return cmd
}

// placeSingle is the original (pre-delta) behaviour: place exactly one
// gubbe. Output is byte-for-byte what `place <roll> <ordinal>` printed
// before this command grew a delta argument — an existing agent config and
// keryx_playtest depend on this exact two-argument call form.
func placeSingle(c *Client, path, worldID, prov string, ordinal int, roll string) error {
	data, err := c.post(path, map[string]any{"target_kind": "hex", "hex_ordinal": ordinal, "good_key": roll})
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
	if opts, oerr := fetchPlacementOptions(c, worldID, prov); oerr == nil {
		printHexGoodRow(opts, ordinal, roll)
		fmt.Printf("  %d idle gubbar left.\n", opts.PoolSize)
	}
	return nil
}

// placeMany places delta gubbar one at a time (server assigns each
// gubbe_ordinal), stopping at the first failure — same abort/report
// convention as staff's positive branch (cmd_staff.go:79-97).
func placeMany(c *Client, path, worldID, prov string, ordinal int, roll string, delta int) error {
	applied := 0
	var stopErr error
	for ; applied < delta; applied++ {
		if _, err := c.post(path, map[string]any{"target_kind": "hex", "hex_ordinal": ordinal, "good_key": roll}); err != nil {
			if applied == 0 && !jsonMode {
				return err
			}
			stopErr = err
			break
		}
	}
	if !jsonMode {
		if applied == delta {
			fmt.Printf("Placed %d gubbe(s) on hex #%d doing %s.\n", applied, ordinal, roll)
		} else {
			fmt.Printf("Placed %d of %d requested (stopped: %v).\n", applied, delta, stopErr)
		}
	}

	opts, oerr := fetchPlacementOptions(c, worldID, prov)
	if oerr != nil {
		if applied == 0 && stopErr != nil {
			return stopErr
		}
		return oerr
	}
	if jsonMode {
		printJSON(map[string]any{
			"hex_ordinal": ordinal,
			"good_key":    roll,
			"requested":   delta,
			"applied":     applied,
			"idle_after":  opts.PoolSize,
		})
		if applied == 0 && stopErr != nil {
			return stopErr
		}
		return nil
	}
	printHexGoodRow(opts, ordinal, roll)
	fmt.Printf("  %d idle gubbar left.\n", opts.PoolSize)
	if applied == 0 && stopErr != nil {
		return stopErr
	}
	return nil
}

// unplaceMany removes up to `requested` gubbar from hex <ordinal> doing
// <roll>, clipped to how many are actually placed there — mirrors staff's
// negative branch (cmd_staff.go:98-122) exactly, including its "only N were
// placed there" wording. Deliberately named gubbeOrdinal below (never bare
// "ordinal") so it can never be confused with the hex ordinal parameter.
func unplaceMany(c *Client, path, worldID, prov string, ordinal int, roll string, requested int) error {
	opts, err := fetchPlacementOptions(c, worldID, prov)
	if err != nil {
		return err
	}
	var placedOrdinals []int
	for _, h := range opts.Hexes {
		if h.HexOrdinal != ordinal {
			continue
		}
		for _, g := range h.Goods {
			if g.GoodKey == roll {
				placedOrdinals = g.PlacedOrdinals
			}
		}
	}
	want := requested
	if want > len(placedOrdinals) {
		want = len(placedOrdinals)
	}

	applied := 0
	var stopErr error
	for ; applied < want; applied++ {
		gubbeOrdinal := placedOrdinals[applied]
		if _, derr := c.delete(fmt.Sprintf("%s/%d", path, gubbeOrdinal)); derr != nil {
			stopErr = derr
			break
		}
	}
	removed := applied

	if !jsonMode {
		if removed == want {
			if want < requested {
				fmt.Printf("Removed %d gubbe(s) — only %d were placed there.\n", removed, want)
			} else {
				fmt.Printf("Removed %d gubbe(s) from hex #%d.\n", removed, ordinal)
			}
		} else {
			fmt.Printf("Removed %d of %d requested (stopped: %v).\n", removed, want, stopErr)
		}
	}

	opts2, oerr := fetchPlacementOptions(c, worldID, prov)
	if oerr != nil {
		if removed == 0 && stopErr != nil {
			return stopErr
		}
		return oerr
	}
	if jsonMode {
		printJSON(map[string]any{
			"hex_ordinal": ordinal,
			"good_key":    roll,
			"requested":   requested,
			"removed":     removed,
			"idle_after":  opts2.PoolSize,
		})
		if removed == 0 && stopErr != nil {
			return stopErr
		}
		return nil
	}
	printHexGoodRow(opts2, ordinal, roll)
	fmt.Printf("  %d idle gubbar left.\n", opts2.PoolSize)
	if removed == 0 && stopErr != nil {
		return stopErr
	}
	return nil
}

// printHexGoodRow prints the single "#ordinal terrain good-cell" row from a
// placement-options snapshot — the same row `keryx city` shows, so the
// player sees exactly what changed.
func printHexGoodRow(opts *placementOptionsResp, ordinal int, roll string) {
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
}
