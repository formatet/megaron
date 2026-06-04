package main

import (
	"encoding/json"
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
