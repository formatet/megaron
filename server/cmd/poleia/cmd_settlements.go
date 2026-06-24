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
			fmt.Printf("%-20s  %-12s  %-36s  %s\n", "Name", "Relation", "Province ID (--province)", "Settlement ID")
			fmt.Println("──────────────────────────────────────────────────────────────────────────────────────────────────────────")
			for _, m := range markers {
				sid, _ := m["settlement_id"].(string)
				if sid == "" {
					continue
				}
				pid, _ := m["id"].(string) // province ID — used with --province flag
				name, _ := m["name"].(string)
				own, _ := m["own"].(bool)
				allied, _ := m["allied"].(bool)
				rel := "foreign"
				if own {
					rel = "own"
				} else if allied {
					rel = "allied"
				}
				fmt.Printf("%-20s  %-12s  %-36s  %s\n", name, rel, pid, sid)
			}
			return nil
		},
	}
}
