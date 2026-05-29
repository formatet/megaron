package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func buildCmd() *cobra.Command {
	var buildingType string

	cmd := &cobra.Command{
		Use:   "build",
		Short: "Start construction of a building",
		Example: `  poleia build --type farm
  poleia build --type barracks`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)
			path := fmt.Sprintf("/api/v1/worlds/%s/provinces/%s/build", cfg.WorldID, cfg.ProvinceID)
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

	cmd.Flags().StringVarP(&buildingType, "type", "t", "", "building type (required)")
	_ = cmd.MarkFlagRequired("type")
	return cmd
}
