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
	var count int

	cmd := &cobra.Command{
		Use:   "recruit",
		Short: "Recruit men into a unit (multiples of 10, max 100 per batch); --count builds N ships in one call",
		Example: `  poleia recruit --unit hoplites --men 10
  poleia recruit --unit chariot --men 50
  poleia recruit --unit trireme --men 20 --count 5`,
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
			isNaval := apiUnit == "ship" || apiUnit == "war_galley" || apiUnit == "merchantman"
			if count > 1 && !isNaval {
				return fmt.Errorf("count gäller bara skepp; landenheter växer via --men")
			}
			if count < 1 || count > 20 {
				return fmt.Errorf("--count must be 1–20")
			}
			c := newClient(cfg)
			path := fmt.Sprintf("/api/v1/worlds/%s/provinces/%s/recruit", cfg.WorldID, cfg.ProvinceID)
			data, err := c.post(path, map[string]any{"unit_type": apiUnit, "men": men, "count": count})
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			if count > 1 {
				fmt.Printf("Recruiting %d× %s (%d crew each)\n", count, unit, men)
			} else {
				fmt.Printf("Recruiting %d men as %s\n", men, unit)
			}
			if !isNaval {
				fmt.Println("Note: a land unit must reach 100 men before it can march or colonize. " +
					"Recruit more of the same type into this settlement, then `poleia unit list` " +
					"(watch `deployable`/`men_to_deploy`).")
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&unit, "unit", "u", "", "unit type (required)")
	cmd.Flags().IntVarP(&men, "men", "n", 10, "men to recruit (multiple of 10, max 100)")
	cmd.Flags().IntVarP(&count, "count", "c", 1, "number of vessels to build in one call (ships only, 1–20)")
	_ = cmd.MarkFlagRequired("unit")
	return cmd
}
