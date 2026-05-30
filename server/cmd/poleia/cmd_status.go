package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show your province status",
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)
			path := fmt.Sprintf("/api/v1/worlds/%s/provinces/%s", cfg.WorldID, cfg.ProvinceID)
			data, err := c.get(path)
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			var p map[string]any
			if err := json.Unmarshal(data, &p); err != nil {
				return err
			}
			sett, _ := p["settlement"].(map[string]any)
			if sett == nil {
				fmt.Println("No settlement here.")
				return nil
			}
			name, _ := sett["name"].(string)
			culture, _ := sett["culture"].(string)
			pop, _ := sett["population"].(float64)
			walls, _ := sett["walls"].(float64)
			loyalty, _ := sett["loyalty"].(float64)
			fmt.Printf("%s [%s]  Pop: %s  Walls: %.0f/3  Loyalty: %.0f\n\n",
				name, culture, resource(pop), walls, loyalty)

			res, _ := sett["resources"].(map[string]any)
			if res != nil {
				fmt.Println("Resources")
				for _, k := range []string{"gold", "kharis"} {
					label := map[string]string{"gold": "Gold", "kharis": "Kharis"}[k]
					v, _ := res[k].(float64)
					r, _ := res[k+"_rate"].(float64)
					fmt.Printf("  %-8s %6s  %s\n", label, resource(v), rate(r))
				}
				fmt.Println()
			}

			army, _ := sett["army"].(map[string]any)
			if army != nil {
				fmt.Println("Army")
				units := []struct{ key, label string }{
					{"Infantry", "Hoplites"}, {"Cavalry", "Hippeis"}, {"Priest", "Hiereus"},
					{"Ship", "Trireme"}, {"EliteInfantry", "Agema"}, {"Catapult", "Siege"},
				}
				for _, u := range units {
					v, _ := army[u.key].(float64)
					if v > 0 {
						fmt.Printf("  %-10s %4.0f\n", u.label, v)
					}
				}
			}
			return nil
		},
	}
}
