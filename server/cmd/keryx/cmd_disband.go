package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"formatet/megaron/server/internal/unit"
	"github.com/spf13/cobra"
)

// disbandWireToType maps the disband endpoint's request/response JSON keys to
// the unit.DisplayName DB-type key they name — "ship" is the endpoint's wire
// key for the canonical "galley" type (see the post() call below), so it's
// the one entry that doesn't match its own DB type.
var disbandWireToType = map[string]string{
	"spearman":       string(unit.TypeSpearman),
	"war_chariot":    string(unit.TypeWarChariot),
	"ship":           string(unit.TypeGalley),
	"elite_infantry": string(unit.TypeEliteInfantry),
	"war_galley":     string(unit.TypeWarGalley),
	"merchantman":    string(unit.TypeMerchantman),
}

// formatDisbandResult renders the server's {"disbanded":{...},"pop_restored":
// N,"population":N} response as one line. A tyst fallback that guesses a
// population figure is worse than omitting it — pop_restored missing (old
// server) prints only what was disbanded, never a fabricated "+0".
func formatDisbandResult(resp map[string]any) string {
	var parts []string
	if disbanded, ok := resp["disbanded"].(map[string]any); ok {
		var keys []string
		for k, v := range disbanded {
			if n, _ := v.(float64); n > 0 {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		for _, k := range keys {
			n := int(disbanded[k].(float64))
			typ := disbandWireToType[k]
			if typ == "" {
				typ = k
			}
			parts = append(parts, fmt.Sprintf("%d %s", n, unit.DisplayName(typ)))
		}
	}
	line := "Disbanded"
	if len(parts) > 0 {
		line += " " + strings.Join(parts, ", ")
	}
	if popRestored, ok := resp["pop_restored"].(float64); ok {
		population, hasPop := resp["population"].(float64)
		if hasPop {
			line += fmt.Sprintf(" · +%d population (now %d)", int(popRestored), int(population))
		} else {
			line += fmt.Sprintf(" · +%d population", int(popRestored))
		}
	}
	return line
}

func disbandCmd() *cobra.Command {
	var spearman, warChariot, galley, eliteInfantry, warGalley, merchantman int
	cmd := &cobra.Command{
		Use:   "disband",
		Short: "Disband units — their men return to civilian life",
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
			fmt.Println(formatDisbandResult(resp))
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
