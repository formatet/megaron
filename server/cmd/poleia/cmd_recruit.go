package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var unitAliases = map[string]string{
	"hoplites": "infantry", "hoplite": "infantry", "inf": "infantry", "infantry": "infantry",
	"chariot": "chariot", "chariots": "chariot", "cha": "chariot", "war_chariot": "chariot",
	"hiereus": "priest", "priest": "priest", "pri": "priest",
	"trireme": "ship", "ship": "ship", "shp": "ship",
	"agema": "elite_infantry", "elite": "elite_infantry", "eli": "elite_infantry",
}

func recruitCmd() *cobra.Command {
	var unit string
	var count int

	cmd := &cobra.Command{
		Use:   "recruit",
		Short: "Recruit units",
		Example: `  poleia recruit --unit hoplites --count 20
  poleia recruit --unit chariot --count 5`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			apiUnit, ok := unitAliases[unit]
			if !ok {
				return fmt.Errorf("unknown unit %q — use: hoplites, chariot, hiereus, trireme, agema", unit)
			}
			c := newClient(cfg)
			path := fmt.Sprintf("/api/v1/worlds/%s/provinces/%s/recruit", cfg.WorldID, cfg.ProvinceID)
			data, err := c.post(path, map[string]any{"unit_type": apiUnit, "count": count})
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			fmt.Printf("Training %d %s\n", count, unit)
			return nil
		},
	}

	cmd.Flags().StringVarP(&unit, "unit", "u", "", "unit type (required)")
	cmd.Flags().IntVarP(&count, "count", "n", 1, "number to recruit")
	_ = cmd.MarkFlagRequired("unit")
	return cmd
}
