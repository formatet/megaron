package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

func recallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "recall <march-id>",
		Short: "Recall an outgoing march (returns army home)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			marchID := args[0]
			c := newClient(cfg)
			path := fmt.Sprintf("/api/v1/worlds/%s/provinces/%s/marches/%s",
				cfg.WorldID, cfg.ProvinceID, marchID)
			data, err := c.delete(path)
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			var resp map[string]any
			json.Unmarshal(data, &resp)
			fmt.Println("March recalled — army is returning home.")
			return nil
		},
	}
}
