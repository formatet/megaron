package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

func abandonCmd() *cobra.Command {
	var settlementID string

	cmd := &cobra.Command{
		Use:   "abandon",
		Short: "Abandon one of your colonies, freeing its hex and a settlement slot",
		Long: `Voluntarily give up a colony. Its garrison is disbanded and its hex becomes
colonisable again. Your capital cannot be abandoned. Use this to make room when
you are at the settlement cap, or to shed a poorly-placed colony.`,
		Example: `  keryx abandon --city <name-or-id>
  keryx abandon --city Knossos --json`,
		Args: rejectPositionalArgs("settlement"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if settlementID == "" {
				return fmt.Errorf("--city <name> is required (abandon never defaults to your capital)")
			}
			c := newClient(cfg)
			sid, rerr := resolveSettlement(c, cfg.WorldID, settlementID)
			if rerr != nil {
				return rerr
			}

			path := fmt.Sprintf("/api/v1/worlds/%s/settlements/%s/abandon", cfg.WorldID, sid)
			data, err := c.post(path, map[string]any{})
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}

			var resp struct {
				Abandoned string `json:"abandoned"`
				Name      string `json:"name"`
				Message   string `json:"message"`
			}
			if err := json.Unmarshal(data, &resp); err != nil {
				return fmt.Errorf("parse response: %w", err)
			}
			fmt.Println(resp.Message)
			return nil
		},
	}

	cmd.Flags().StringVar(&settlementID, "settlement", "", "city name or settlement UUID to abandon (required)")
	addCityFlag(cmd, &settlementID, true, "settlement")
	return cmd
}
