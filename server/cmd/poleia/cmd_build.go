package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

func buildCmd() *cobra.Command {
	var buildingType string
	var provinceID string
	var list bool
	var queue bool

	cmd := &cobra.Command{
		Use:   "build",
		Short: "Start construction of a building (--list for options, --queue to see what's queued; defaults to capital)",
		Example: `  poleia build --list
  poleia build --type farm
  poleia build --type harbour          # requires coastal (adjacent sea hex)
  poleia build --type mine --province <province-id>   # build in a colony
  poleia build --type winery           # produces nothing unless a hills tile is in catchment
  poleia build --queue                 # see what's already queued, with cancel-build IDs`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)

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

			// --list (or no --type): show the building catalogue and exit.
			if list || buildingType == "" {
				data, err := c.get("/api/v1/buildings")
				if err != nil {
					return err
				}
				if jsonMode {
					printRawJSON(data)
					return nil
				}
				var catalogue []struct {
					Type             string             `json:"type"`
					Costs            map[string]float64 `json:"costs"`
					CostSilver       float64            `json:"cost_silver"`
					DurationMinutes  float64            `json:"duration_minutes"`
					RequiresCoastal  bool               `json:"requires_coastal"`
					RequiresDeposits []string           `json:"requires_deposits"`
					RequiresTerrain  []string           `json:"requires_terrain"`
					Purpose          string             `json:"purpose"`
				}
				if err := json.Unmarshal(data, &catalogue); err != nil {
					return err
				}
				fmt.Println("Build queue: `build --queue` shows how many slots your settlement has left.")
				fmt.Println()
				fmt.Printf("%-14s  %-28s  %-8s  %-30s  %s\n", "Type", "Costs", "Mins", "Requires", "Purpose")
				fmt.Println(strings.Repeat("─", 105))
				for _, b := range catalogue {
					// Format costs: "timber×50 stone×20"
					costParts := make([]string, 0, len(b.Costs))
					for g, q := range b.Costs {
						costParts = append(costParts, fmt.Sprintf("%s×%.0f", g, q))
					}
					sort.Strings(costParts)
					if b.CostSilver > 0 {
						costParts = append(costParts, fmt.Sprintf("silver×%.0f", b.CostSilver))
					}
					costStr := strings.Join(costParts, " ")

					// Format gate requirements
					reqs := []string{}
					if b.RequiresCoastal {
						reqs = append(reqs, "coastal (adj sea)")
					}
					for _, d := range b.RequiresDeposits {
						reqs = append(reqs, d+" deposit")
					}
					if len(b.RequiresTerrain) > 0 {
						// P10 (soak 2026-07-18): buildings whose ENTIRE production is
						// terrain-locked (e.g. winery — hills only, no fallback rule)
						// produced nothing off that terrain, silently, until you tried
						// it. Surface the requirement up front.
						reqs = append(reqs, strings.Join(b.RequiresTerrain, "/")+" terrain")
					}
					reqStr := strings.Join(reqs, ", ")
					if reqStr == "" {
						reqStr = "—"
					}

					fmt.Printf("%-14s  %-28s  %-8.0f  %-30s  %s\n",
						b.Type, costStr, b.DurationMinutes, reqStr, b.Purpose)
				}
				return nil
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
			path := fmt.Sprintf("/api/v1/worlds/%s/provinces/%s/build", cfg.WorldID, prov)
			data, err := c.post(path, map[string]string{"building_type": buildingType})
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			fmt.Printf("Construction queued: %s\n", buildingType)
			return nil
		},
	}

	cmd.Flags().SortFlags = false
	cmd.Flags().StringVar(&provinceID, "province", "", "province ID to build in (default: your capital)")
	cmd.Flags().StringVarP(&buildingType, "type", "t", "", "building type (omit to see list)")
	cmd.Flags().BoolVar(&list, "list", false, "show the building catalogue and exit")
	cmd.Flags().BoolVar(&queue, "queue", false, "show this settlement's build queue (with queue IDs for cancel-build) and exit")
	return cmd
}

// printBuildQueue implements `build --queue`: shows what's already queued in a
// settlement, including each entry's queue ID, so `cancel-build --id` is
// actually usable — build previously only ever said "already in the queue"
// with no way to see what, or its ID. The data itself already exists (province
// GET returns build_queue, the same field `poleia status` prints without the
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
		fmt.Printf("Queue: %d/%.0f slots used\n", len(bq), qmax)
	}
	if len(bq) == 0 {
		fmt.Println("Build queue is empty.")
		return nil
	}
	fmt.Printf("%-14s  %-38s  %s\n", "Type", "Queue ID (cancel-build --id)", "Done")
	fmt.Println(strings.Repeat("─", 90))
	for _, it := range bq {
		m, _ := it.(map[string]any)
		t, _ := m["type"].(string)
		id, _ := m["id"].(string)
		ca, _ := m["complete_at"].(string)
		fmt.Printf("%-14s  %-38s  %s\n", t, id, buildQueueETA(ca))
	}
	return nil
}

func cancelBuildCmd() *cobra.Command {
	var queueID string

	cmd := &cobra.Command{
		Use:     "cancel-build",
		Short:   "Cancel a queued building and refund costs",
		Example: `  poleia cancel-build --id <queue-id>`,
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
