package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

func disbandCmd() *cobra.Command {
	var hoplites, chariots, hiereus, trireme, agema, warGalley, merchantman int
	cmd := &cobra.Command{
		Use:   "disband",
		Short: "Release units back to population (they return to civilian life)",
		Example: `  keryx disband --hoplites 20
  keryx disband --hoplites 10 --chariots 5 --agema 2
  keryx disband --trireme 3 --agema 1
  keryx disband --war-galley 2 --merchantman 1`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if hoplites+chariots+hiereus+trireme+agema+warGalley+merchantman == 0 {
				return fmt.Errorf("specify at least one unit type to disband")
			}
			c := newClient(cfg)
			path := fmt.Sprintf("/api/v1/worlds/%s/provinces/%s/disband", cfg.WorldID, cfg.ProvinceID)
			data, err := c.post(path, map[string]any{
				"spearman":       hoplites,
				"war_chariot":    chariots,
				"priest":         hiereus,
				"ship":           trireme,
				"elite_infantry": agema,
				"war_galley":     warGalley,
				"merchantman":    merchantman,
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
	cmd.Flags().IntVar(&chariots, "chariots", 0, "war chariots to disband")
	cmd.Flags().IntVar(&hiereus, "hiereus", 0, "priests to disband")
	cmd.Flags().IntVar(&trireme, "trireme", 0, "galleys to disband")
	cmd.Flags().IntVar(&agema, "agema", 0, "elite infantry to disband")
	cmd.Flags().IntVar(&warGalley, "war-galley", 0, "war galleys to disband")
	cmd.Flags().IntVar(&merchantman, "merchantman", 0, "merchantmen to disband")
	return cmd
}
