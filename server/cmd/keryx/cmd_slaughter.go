package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

// slaughterCmd handles `keryx slaughter` — S1c
// (megaron_plan_foda_konsistens.md): trade one livestock for +10 population
// right now, the herd's strongest sink. Player-initiated only (never
// automatic — see the handler's own doc comment). Same --province
// default-to-capital convention as `place`/`staff`/`city`.
func slaughterCmd() *cobra.Command {
	var provinceID string
	cmd := &cobra.Command{
		Use:   "slaughter",
		Short: "Slaughter one livestock for +10 population right now",
		Long: `Trades one animal from your herd for ten citizens, immediately. Requires at
least one livestock in stock — refused cleanly if the herd is empty. If the
ten new citizens cross a new full hundred, the newly-born gubbe is
auto-placed on the best available food hex, same as ordinary population
growth.`,
		Example: `  keryx slaughter
  keryx slaughter --province <prov-id>`,
		Args: rejectPositionalArgs("province"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)
			prov := cfg.ProvinceID
			if provinceID != "" {
				resolved, rerr := resolveProvince(c, cfg.WorldID, provinceID)
				if rerr != nil {
					return rerr
				}
				prov = resolved
			}

			data, err := c.post(fmt.Sprintf("/api/v1/worlds/%s/provinces/%s/slaughter-livestock", cfg.WorldID, prov), map[string]any{})
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			var resp struct {
				Population   int `json:"population"`
				GubbarPlaced int `json:"gubbar_placed"`
			}
			if err := json.Unmarshal(data, &resp); err != nil {
				return fmt.Errorf("parse response: %w", err)
			}
			fmt.Printf("Slaughtered one animal. Population is now %d.\n", resp.Population)
			if resp.GubbarPlaced > 0 {
				fmt.Printf("A new gubbe was born and auto-placed on the best food hex.\n")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&provinceID, "province", "", "province ID/name (default: your capital)")
	return cmd
}
