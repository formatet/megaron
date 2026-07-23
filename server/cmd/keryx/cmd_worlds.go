package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

func worldsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "worlds",
		Short: "List available worlds",
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)
			data, err := c.get("/api/v1/worlds")
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			var worlds []map[string]any
			if err := json.Unmarshal(data, &worlds); err != nil {
				return err
			}
			for _, w := range worlds {
				id, _ := w["id"].(string)
				name, _ := w["name"].(string)
				state, _ := w["state"].(string)
				era, _ := w["era_number"].(float64)
				active := " "
				if cfg != nil && cfg.WorldID == id {
					active = "▶"
				}
				fmt.Printf("%s %-24s  [%s]  Era %.0f  %s\n", active, name, state, era, id)
			}
			return nil
		},
	}
}
