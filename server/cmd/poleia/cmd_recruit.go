package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var unitAliases = map[string]string{
	"hoplites": "spearman", "hoplite": "spearman", "inf": "spearman", "infantry": "spearman", "spearman": "spearman", "spear": "spearman",
	"chariot": "war_chariot", "chariots": "war_chariot", "cha": "war_chariot", "war_chariot": "war_chariot",
	"trireme": "ship", "ship": "ship", "shp": "ship",
	"war_galley": "war_galley", "wargalley": "war_galley", "warship": "war_galley",
	"merchantman": "merchantman", "merchant": "merchantman", "trader": "merchantman",
	"agema": "elite_infantry", "elite": "elite_infantry", "eli": "elite_infantry", "elite_infantry": "elite_infantry",
}

func recruitCmd() *cobra.Command {
	var unit string
	var men int
	var count int
	var name string

	cmd := &cobra.Command{
		Use:   "recruit",
		Short: "Recruit men into a land unit, or build a ship (naval units build one vessel at a time)",
		Example: `  poleia recruit --unit hoplites --men 10
  poleia recruit --unit chariot --men 50
  poleia recruit --unit trireme --name Asterion
  poleia recruit --unit war_galley --count 3`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			apiUnit, ok := unitAliases[unit]
			if !ok {
				return fmt.Errorf("unknown unit %q — use: hoplites, chariot, trireme, war_galley, merchantman, agema", unit)
			}
			isNaval := apiUnit == "ship" || apiUnit == "war_galley" || apiUnit == "merchantman"
			if !isNaval {
				if men <= 0 || men%10 != 0 {
					return fmt.Errorf("--men must be a positive multiple of 10 (e.g. 10, 20, … 100)")
				}
				if men > 100 {
					return fmt.Errorf("--men cannot exceed 100 per recruit call")
				}
			}
			if count > 1 && !isNaval {
				return fmt.Errorf("count gäller bara skepp; landenheter växer via --men")
			}
			if count < 1 || count > 20 {
				return fmt.Errorf("--count must be 1–20")
			}
			if name != "" && !isNaval {
				return fmt.Errorf("--name gäller bara skepp")
			}
			c := newClient(cfg)
			path := fmt.Sprintf("/api/v1/worlds/%s/provinces/%s/recruit", cfg.WorldID, cfg.ProvinceID)
			body := map[string]any{"unit_type": apiUnit, "count": count}
			if !isNaval {
				body["men"] = men
			}
			if name != "" {
				body["name"] = name
			}
			data, err := c.post(path, body)
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			if isNaval {
				var resp struct {
					Names      []string  `json:"names"`
					CompleteAt time.Time `json:"complete_at"`
				}
				_ = json.Unmarshal(data, &resp)
				if count > 1 {
					fmt.Printf("Building %d× %s: %s\n", count, unit, strings.Join(resp.Names, ", "))
				} else if len(resp.Names) == 1 {
					fmt.Printf("Building 1 %s — %q\n", unit, resp.Names[0])
				} else {
					fmt.Printf("Building 1 %s\n", unit)
				}
				if !resp.CompleteAt.IsZero() {
					fmt.Printf("Ready %s — not deployable until then (`poleia unit list` shows the ETA).\n",
						resp.CompleteAt.Local().Format("15:04 Jan 2"))
				}
			} else {
				fmt.Printf("Recruiting %d men as %s\n", men, unit)
				fmt.Println("Note: a land unit must reach 100 men before it can march or colonize. " +
					"Recruit more of the same type into this settlement, then `poleia unit list` " +
					"(watch `deployable`/`men_to_deploy`).")
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&unit, "unit", "u", "", "unit type (required)")
	cmd.Flags().IntVarP(&men, "men", "n", 10, "men to recruit (multiple of 10, max 100; ignored for ships)")
	cmd.Flags().IntVarP(&count, "count", "c", 1, "number of vessels to build in one call (ships only, 1–20)")
	cmd.Flags().StringVar(&name, "name", "", "ship name (ships only; omit for a suggested name)")
	_ = cmd.MarkFlagRequired("unit")
	return cmd
}
