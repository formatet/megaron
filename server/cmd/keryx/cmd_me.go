package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

func meCmd() *cobra.Command {
	return &cobra.Command{
		Use: "me", Short: "Show your own realm in the selected world", Args: noPositionalArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			data, err := newClient(cfg).get(fmt.Sprintf("/api/v1/worlds/%s/me", cfg.WorldID))
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			var profile struct {
				WanaxName   string  `json:"wanax_name"`
				FoundedTick *int    `json:"founded_tick"`
				Cities      int     `json:"cities"`
				Population  int     `json:"population"`
				Kharis      float64 `json:"kharis"`
			}
			if err := json.Unmarshal(data, &profile); err != nil {
				return err
			}
			founded := "Not yet founded"
			if profile.FoundedTick != nil {
				founded = fmt.Sprintf("Since day %d", *profile.FoundedTick)
			}
			fmt.Printf("Wanax: %s\nFounded: %s\nCities: %d\nPopulation: %d\nKharis: %.1f\n", profile.WanaxName, founded, profile.Cities, profile.Population, profile.Kharis)
			return nil
		},
	}
}
