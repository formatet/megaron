package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

func disbandCmd() *cobra.Command {
	var spearman, warChariot, galley, eliteInfantry, warGalley, merchantman int
	cmd := &cobra.Command{
		Use:   "disband",
		Short: "Release units back to population (they return to civilian life)",
		Example: `  keryx disband --spearman 20
  keryx disband --spearman 10 --war-chariot 5 --elite-infantry 2
  keryx disband --galley 3 --elite-infantry 1
  keryx disband --war-galley 2 --merchantman 1`,
		// No single flag is the obvious guess — one flag per unit type, so
		// there is no "the" flag to name the way build has --type.
		Args: noPositionalArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if spearman+warChariot+galley+eliteInfantry+warGalley+merchantman == 0 {
				return fmt.Errorf("specify at least one unit type to disband")
			}
			c := newClient(cfg)
			path := fmt.Sprintf("/api/v1/worlds/%s/provinces/%s/disband", cfg.WorldID, cfg.ProvinceID)
			data, err := c.post(path, map[string]any{
				"spearman":       spearman,
				"war_chariot":    warChariot,
				"ship":           galley, // server's disband JSON key for the canonical "galley" DB type
				"elite_infantry": eliteInfantry,
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
	cmd.Flags().IntVar(&spearman, "spearman", 0, "spearmen to disband")
	cmd.Flags().IntVar(&warChariot, "war-chariot", 0, "war chariots to disband")
	cmd.Flags().IntVar(&galley, "galley", 0, "galleys to disband")
	cmd.Flags().IntVar(&eliteInfantry, "elite-infantry", 0, "elite infantry to disband")
	cmd.Flags().IntVar(&warGalley, "war-galley", 0, "war galleys to disband")
	cmd.Flags().IntVar(&merchantman, "merchantman", 0, "merchantmen to disband")

	// Retired flag names (namn-hygien-svepet, mig 084, renamed the unit
	// TYPES but missed this CLI's flags — internal/unit/naming_test.go
	// TestRetiredNamesNeverSurface only ever checked DisplayName(), never
	// flag names; megaron_plan_cli_sanning §E). Kept as hidden aliases,
	// bound to the same variables: agent configs in keryx_playtest may still
	// pass the old names, and a silent break is a worse lie than an old
	// label — see naming_test.go's flag/example sweep for why these stay.
	cmd.Flags().IntVar(&spearman, "hoplites", 0, "deprecated: use --spearman")
	cmd.Flags().IntVar(&warChariot, "chariots", 0, "deprecated: use --war-chariot")
	cmd.Flags().IntVar(&galley, "trireme", 0, "deprecated: use --galley")
	cmd.Flags().IntVar(&eliteInfantry, "agema", 0, "deprecated: use --elite-infantry")
	for old, canonical := range map[string]string{
		"hoplites": "--spearman", "chariots": "--war-chariot",
		"trireme": "--galley", "agema": "--elite-infantry",
	} {
		_ = cmd.Flags().MarkDeprecated(old, "use "+canonical+" instead")
		_ = cmd.Flags().MarkHidden(old)
	}
	return cmd
}
