package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var unitAliases = map[string]string{
	"hoplites": "spearman", "hoplite": "spearman", "inf": "spearman", "infantry": "spearman", "spearman": "spearman", "spear": "spearman",
	"chariot": "war_chariot", "chariots": "war_chariot", "cha": "war_chariot", "war_chariot": "war_chariot",
	"trireme": "ship", "ship": "ship", "shp": "ship",
	"agema": "elite_infantry", "elite": "elite_infantry", "eli": "elite_infantry",
}

func recruitCmd() *cobra.Command {
	var unit string
	var men int

	cmd := &cobra.Command{
		Use:   "recruit",
		Short: "Recruit men into a unit (multiples of 10, max 100 per batch)",
		Example: `  poleia recruit --unit hoplites --men 10
  poleia recruit --unit chariot --men 50`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			apiUnit, ok := unitAliases[unit]
			if !ok {
				return fmt.Errorf("unknown unit %q — use: hoplites, chariot, trireme, agema", unit)
			}
			if men <= 0 || men%10 != 0 {
				return fmt.Errorf("--men must be a positive multiple of 10 (e.g. 10, 20, … 100)")
			}
			if men > 100 {
				return fmt.Errorf("--men cannot exceed 100 per recruit call")
			}
			c := newClient(cfg)
			path := fmt.Sprintf("/api/v1/worlds/%s/provinces/%s/recruit", cfg.WorldID, cfg.ProvinceID)
			data, err := c.post(path, map[string]any{"unit_type": apiUnit, "men": men})
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			fmt.Printf("Recruiting %d men as %s\n", men, unit)
			if apiUnit != "ship" && apiUnit != "war_galley" && apiUnit != "merchantman" {
				fmt.Println("Note: a land unit must reach 100 men before it can march or colonize. " +
					"Recruit more of the same type into this settlement, then `poleia unit list` " +
					"(watch `deployable`/`men_to_deploy`).")
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&unit, "unit", "u", "", "unit type (required)")
	cmd.Flags().IntVarP(&men, "men", "n", 10, "men to recruit (multiple of 10, max 100)")
	_ = cmd.MarkFlagRequired("unit")
	return cmd
}
