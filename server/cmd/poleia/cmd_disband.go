package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

func disbandCmd() *cobra.Command {
	var hoplites, hippeis, hiereus, trireme, agema int
	cmd := &cobra.Command{
		Use:   "disband",
		Short: "Release units back to population (they return to civilian life)",
		Example: `  poleia disband --hoplites 20
  poleia disband --hoplites 10 --hippeis 5`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if hoplites+hippeis+hiereus+trireme+agema == 0 {
				return fmt.Errorf("specify at least one unit type to disband")
			}
			c := newClient(cfg)
			path := fmt.Sprintf("/api/v1/worlds/%s/provinces/%s/disband", cfg.WorldID, cfg.ProvinceID)
			data, err := c.post(path, map[string]any{
				"infantry":       hoplites,
				"cavalry":        hippeis,
				"priest":         hiereus,
				"ship":           trireme,
				"elite_infantry": agema,
			})
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
			pop, _ := resp["pop_restored"].(float64)
			fmt.Printf("Disbanded · +%d population\n", int(pop))
			return nil
		},
	}
	cmd.Flags().IntVar(&hoplites, "hoplites", 0, "infantry to disband")
	cmd.Flags().IntVar(&hippeis, "hippeis", 0, "cavalry to disband")
	cmd.Flags().IntVar(&hiereus, "hiereus", 0, "priests to disband")
	cmd.Flags().IntVar(&trireme, "trireme", 0, "ships to disband")
	cmd.Flags().IntVar(&agema, "agema", 0, "elite infantry to disband")
	return cmd
}
