package main

import (
	"encoding/json"
	"fmt"
	"github.com/spf13/cobra"
)

func unitCancelTrainingCmd() *cobra.Command {
	var id string
	cmd := &cobra.Command{Use: "cancel-training", Short: "Cancel unfinished recruitment and return its people and reserved resources", Args: noPositionalArgs(), RunE: func(cmd *cobra.Command, args []string) error {
		data, err := newClient(cfg).post(fmt.Sprintf("/api/v1/worlds/%s/units/%s/cancel-training", cfg.WorldID, id), nil)
		if err != nil {
			return err
		}
		if jsonMode {
			printRawJSON(data)
			return nil
		}
		var result struct {
			PopRestored int                `json:"pop_restored"`
			Already     bool               `json:"already_cancelled"`
			Refunded    map[string]float64 `json:"refunded"`
		}
		if err := json.Unmarshal(data, &result); err != nil {
			return err
		}
		if result.Already {
			fmt.Println("Training was already cancelled.")
			return nil
		}
		fmt.Printf("Training cancelled. %d people returned; reserved resources refunded.\n", result.PopRestored)
		return nil
	}}
	cmd.Flags().StringVar(&id, "unit", "", "unit ID (required)")
	cmd.MarkFlagRequired("unit")
	return cmd
}
