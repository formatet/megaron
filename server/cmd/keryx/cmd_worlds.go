package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

func worldsCmd() *cobra.Command {
	var selected string
	cmd := &cobra.Command{
		Use:   "worlds",
		Short: "List available worlds",
		Args:  noPositionalArgs(), // no flags at all — nothing to guess
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)
			data, err := c.get("/api/v1/worlds")
			if err != nil {
				return err
			}
			if selected != "" {
				var worlds []map[string]any
				if err := json.Unmarshal(data, &worlds); err != nil {
					return err
				}
				found := false
				for _, w := range worlds {
					if w["id"] == selected {
						found = true
					}
				}
				if !found {
					return fmt.Errorf("world %s is not in keryx worlds", selected)
				}
				cfg.WorldID = selected
				cfg.ProvinceID = autoDetectProvince(c, selected)
				if err := saveConfig(cfg); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Selected world %s\n", selected)
				return nil
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
	cmd.Flags().StringVar(&selected, "select", "", "select a world by its id and save it")
	return cmd
}
