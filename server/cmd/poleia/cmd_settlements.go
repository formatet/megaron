package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

func settlementsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "settlements",
		Short: "List visible settlements (use names with trade/messenger)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)
			data, err := c.get(fmt.Sprintf("/api/v1/worlds/%s/provinces", cfg.WorldID))
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			var markers []map[string]any
			if err := json.Unmarshal(data, &markers); err != nil {
				return err
			}
			fmt.Printf("%-20s  %-10s  %-9s  %-36s  %s\n", "Name", "Relation", "Role", "Province ID (--province)", "Settlement ID")
			fmt.Println("──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────")
			for _, m := range markers {
				sid, _ := m["settlement_id"].(string)
				if sid == "" {
					continue
				}
				pid, _ := m["id"].(string) // province ID — used with --province flag
				name, _ := m["name"].(string)
				own, _ := m["own"].(bool)
				allied, _ := m["allied"].(bool)
				state, _ := m["state"].(string)
				// A razed/collapsed settlement is a ruin, not "foreign" — say so
				// (owner_id is NULL on both, which otherwise falls through to foreign).
				rel := "foreign"
				switch {
				case state == "razed" || state == "collapsed":
					rel = state
				case own:
					rel = "own"
				case allied:
					rel = "allied"
				}
				// Role: only meaningful for the player's own settlements (is_capital
				// is set server-side only for own rows).
				role := "—"
				if own {
					isOutpost, _ := m["is_outpost"].(bool)
					isCapital, _ := m["is_capital"].(bool)
					switch {
					case isOutpost:
						role = "outpost"
					case isCapital:
						role = "capital"
					default:
						role = "colony"
					}
				}
				fmt.Printf("%-20s  %-10s  %-9s  %-36s  %s\n", name, rel, role, pid, sid)
			}
			return nil
		},
	}
}
