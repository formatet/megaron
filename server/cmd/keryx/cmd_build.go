package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"formatet/megaron/server/internal/religion"
	"github.com/spf13/cobra"
)

// parseHexFlag parses "--hex q,r" into two ints. Kept separate from the
// server's own hex validation (catchment/terrain/deposit) — this is only the
// wire-format parse, everything else is the server's call and its error
// comes back verbatim (megaron_plan_byggnad_pa_hex.md §A2).
func parseHexFlag(s string) (q, r int, err error) {
	parts := strings.SplitN(s, ",", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("--hex wants \"q,r\", got %q", s)
	}
	q, err = strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0, fmt.Errorf("--hex: invalid q %q", parts[0])
	}
	r, err = strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return 0, 0, fmt.Errorf("--hex: invalid r %q", parts[1])
	}
	return q, r, nil
}

func buildCmd() *cobra.Command {
	var buildingType string
	var provinceID string
	var hexFlag, god string
	var list bool
	var queue bool

	cmd := &cobra.Command{
		Use:   "build [city]",
		Short: "Start construction of a building (--list for options, --queue to see what's queued; defaults to capital)",
		Example: `  keryx build --list
  keryx build --type farm --hex 3,-1          # farm/mine/lumbermill/stonequarry are hex-bound: pick a catchment hex
  keryx build --type harbour          # requires coastal (adjacent sea hex)
  keryx build --type mine --hex 5,2 --province <province-id>   # build in a colony
  keryx build --type winery           # produces nothing unless a hills/plains/scrub_maquis tile is in catchment
  keryx build --queue                 # see what's already queued, with cancel-build IDs`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 && buildingType == "" && !list && !queue {
				return fmt.Errorf("no building type given — use --type <building> (for example: keryx build --type harbour), or add --list to view the catalogue for a city")
			}
			c := newClient(cfg)
			if len(args) > 0 {
				if provinceID != "" {
					return fmt.Errorf("choose a city either positionally or with --province")
				}
				provinceID = args[0]
			}

			// --queue: show this settlement's build queue (with queue IDs for
			// cancel-build) and exit. Added because `build` used to only say
			// "already in the queue" without any way to see what — or its ID.
			if queue {
				prov := cfg.ProvinceID
				if provinceID != "" {
					resolved, err := resolveProvince(c, cfg.WorldID, provinceID)
					if err != nil {
						return err
					}
					prov = resolved
				}
				return printBuildQueue(c, cfg.WorldID, prov)
			}

			// The list quotes the selected city's next construction targets.
			if list || buildingType == "" {
				prov := cfg.ProvinceID
				if provinceID != "" {
					resolved, err := resolveProvince(c, cfg.WorldID, provinceID)
					if err != nil {
						return err
					}
					prov = resolved
				}
				return listCityBuilds(c, cfg.WorldID, prov)
			}

			// Default to the capital; --province lets you build in any province you own
			// (the server verifies ownership). Without this, every build hit the capital.
			prov := cfg.ProvinceID
			if provinceID != "" {
				resolved, err := resolveProvince(c, cfg.WorldID, provinceID)
				if err != nil {
					return err
				}
				prov = resolved
			}
			body := map[string]any{"building_type": buildingType}
			if god != "" {
				body["god"] = god
			}
			effectLine := ""
			if hexFlag != "" {
				q, r, _, perr := resolveHexFlag(c, cfg.WorldID, prov, hexFlag)
				if perr != nil {
					return perr
				}
				hexFlag = fmt.Sprintf("%d,%d", q, r)
				body["hex_q"] = q
				body["hex_r"] = r
				// Read before the POST so a new build shows `effect` and an
				// upgrade shows `upgrade_effect`. Best-effort: a failed read
				// never blocks the build.
				if po, perr := fetchPlacementOptions(c, cfg.WorldID, prov); perr == nil {
					effectLine = buildEffectLine(po, buildingType, q, r)
				}
			}
			path := fmt.Sprintf("/api/v1/worlds/%s/provinces/%s/build", cfg.WorldID, prov)
			data, err := c.post(path, body)
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			if hexFlag != "" {
				if effectLine != "" {
					fmt.Println(effectLine)
				}
				fmt.Printf("Construction queued: %s @ (%s)\n", buildingType, hexFlag)
			} else if god != "" {
				fmt.Printf("Construction queued: %s\n", religion.TempleName(god))
			} else {
				fmt.Printf("Construction queued: %s\n", buildingType)
			}
			return nil
		},
	}

	cmd.Flags().SortFlags = false
	cmd.Flags().StringVar(&provinceID, "province", "", "province ID to build in (default: your capital)")
	cmd.Flags().StringVarP(&buildingType, "type", "t", "", "building type (omit to see list)")
	cmd.Flags().StringVar(&god, "god", "", "the god a temple is built for (one temple per god and city): "+religion.TempleGodIDs())
	cmd.Flags().StringVar(&hexFlag, "hex", "", "catchment q,r or ordinal 6/#6 — required for hex-bound building types")
	cmd.Flags().BoolVar(&list, "list", false, "show the building catalogue and exit")
	cmd.Flags().BoolVar(&queue, "queue", false, "show this settlement's build queue (with queue IDs for cancel-build) and exit")
	return cmd
}

// printBuildQueue implements `build --queue`: shows what's already queued in a
// settlement, including each entry's queue ID, so `cancel-build --id` is
// actually usable — build previously only ever said "already in the queue"
// with no way to see what, or its ID. The data itself already exists (province
// GET returns build_queue, the same field `keryx status` prints without the
// ID); this just gives it its own focused, ID-inclusive view.
func printBuildQueue(c *Client, worldID, provinceID string) error {
	data, err := c.get(fmt.Sprintf("/api/v1/worlds/%s/provinces/%s", worldID, provinceID))
	if err != nil {
		return err
	}
	if jsonMode {
		printRawJSON(data)
		return nil
	}
	var p map[string]any
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	sett, _ := p["settlement"].(map[string]any)
	if sett == nil {
		fmt.Println("No settlement here.")
		return nil
	}
	bq, _ := sett["build_queue"].([]any)
	// P10 (soak 2026-07-18): the queue depth cap (build's 422 said "max N
	// concurrent" only AFTER a 3rd build was refused) is now printed up front
	// here, every time — not just when you hit it.
	if qmax, ok := sett["build_queue_max"].(float64); ok {
		fmt.Printf("Queue: %d/%.0f slots used — fixed limit; population and buildings do not add slots. Finish or cancel a build.\n", len(bq), qmax)
	}
	if len(bq) == 0 {
		fmt.Println("Build queue is empty.")
		return nil
	}
	fmt.Printf("%-14s  %-38s  %s\n", "Type", "Queue ID (cancel-build --id)", "Done")
	fmt.Println(strings.Repeat("─", 90))
	for _, it := range bq {
		m, _ := it.(map[string]any)
		t := buildingLabel(m)
		id, _ := m["id"].(string)
		ca, _ := m["complete_at"].(string)
		fmt.Printf("%-14s  %-38s  %s\n", t, id, buildQueueETA(c, ca))
	}
	return nil
}

func cancelBuildCmd() *cobra.Command {
	var queueID string

	cmd := &cobra.Command{
		Use:     "cancel-build",
		Short:   "Cancel a queued building and refund costs",
		Example: `  keryx cancel-build --id <queue-id>`,
		Args:    rejectPositionalArgs("id"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)
			path := fmt.Sprintf("/api/v1/worlds/%s/provinces/%s/build-queue/%s",
				cfg.WorldID, cfg.ProvinceID, queueID)
			data, err := c.delete(path)
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			var resp map[string]any
			if err := json.Unmarshal(data, &resp); err != nil {
				return err
			}
			cancelled, _ := resp["cancelled"].(string)
			fmt.Printf("Cancelled: %s — costs refunded\n", cancelled)
			return nil
		},
	}

	cmd.Flags().StringVar(&queueID, "id", "", "build queue ID (from status or build output)")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}
