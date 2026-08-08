package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

// briefCmd handles `keryx brief` — a one-line-per-settlement idle-gubbe
// digest across everything the Wanax owns. Scoped narrowly to placement
// (P0-UI answer 7's verb list): the full cross-system away-report
// (frånvarorapporten) is separate, unbuilt, future work
// (megaron_todo.md §SENARE) — this is not that, just the placement corner
// of it, cheap because P5 already computes idle pools per settlement.
func briefCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "brief",
		Short: "One line per settlement you own: idle gubbar, if any",
		Args:  noPositionalArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)
			data, err := c.get(fmt.Sprintf("/api/v1/worlds/%s/provinces", cfg.WorldID))
			if err != nil {
				return err
			}
			var markers []map[string]any
			if err := json.Unmarshal(data, &markers); err != nil {
				return err
			}

			type row struct {
				name        string
				provinceID  string
				pool, total int
				err         error
			}
			var rows []row
			for _, m := range markers {
				own, _ := m["own"].(bool)
				sid, _ := m["settlement_id"].(string)
				if !own || sid == "" {
					continue
				}
				name, _ := m["name"].(string)
				pid, _ := m["id"].(string)
				opts, oerr := fetchPlacementOptions(c, cfg.WorldID, pid)
				r := row{name: name, provinceID: pid, err: oerr}
				if oerr == nil {
					r.pool, r.total = opts.PoolSize, opts.TotalGubbar
				}
				rows = append(rows, r)
			}

			if jsonMode {
				printJSON(rows)
				return nil
			}
			if len(rows) == 0 {
				fmt.Println("No settlements owned.")
				return nil
			}
			anyIdle := false
			for _, r := range rows {
				if r.err != nil {
					fmt.Printf("%-20s  (could not read: %v)\n", r.name, r.err)
					continue
				}
				if r.pool == 0 {
					fmt.Printf("%-20s  fully staffed (%d/%d)\n", r.name, r.total, r.total)
					continue
				}
				anyIdle = true
				fmt.Printf("%-20s  %d/%d idle — run `keryx city %s`\n", r.name, r.pool, r.total, r.name)
			}
			if !anyIdle {
				fmt.Println("\nEvery gubbe you have is placed.")
			}
			return nil
		},
	}
	return cmd
}
