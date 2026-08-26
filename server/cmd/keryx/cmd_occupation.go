package main

// keryx occupation order — S3/S6 (megaron_plan_erovring.md): the runner-
// delivered choice for a city you currently hold under occupation
// (sack/burn/annex). Doing nothing leaves it occupied — the default.
// keryx-yta rule (megaron_moc.md): everything in temenos must be visible AND
// actionable in keryx, not just the web.

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

func occupationCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "occupation",
		Short: "Manage a city you currently hold under occupation",
	}
	cmd.AddCommand(occupationOrderCmd())
	return cmd
}

func occupationOrderCmd() *cobra.Command {
	var settlementID, action string
	var goods []string

	cmd := &cobra.Command{
		Use:   "order",
		Short: "Send a Runner with your choice for an occupied city: sack, burn, or annex",
		Long: `A city your army holds under occupation stays with its original owner until
you choose otherwise. Doing nothing leaves it occupied (the default —
reversible, least destructive). This command sends a Runner to carry your
choice; it executes only when the Runner arrives, not instantly.

  sack  — loot the city (silver plus a share of its other goods, weighted by
          portability) and weaken it: population -⅓, its strongest production
          building drops one level. The city stays with its original owner.
  burn  — sack, then raze the city outright. It becomes ownerless and its hex
          cannot be recolonized until the karens (a fixed number of ticks)
          has passed.
  annex — take the city for good. Only possible once the occupation has gone
          unchallenged long enough (see the CityAnnexReady notification) —
          any attack on the occupied city resets that clock.`,
		Example: `  keryx occupation order --settlement <id> --action sack
  keryx occupation order --settlement <id> --action sack --goods silver,bronze
  keryx occupation order --settlement <id> --action annex`,
		Args: rejectPositionalArgs("occupation order"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)
			path := fmt.Sprintf("/api/v1/worlds/%s/settlements/%s/occupation-order", cfg.WorldID, settlementID)
			body := map[string]any{"action": action}
			if len(goods) > 0 {
				body["goods"] = goods
			}
			data, err := c.post(path, body)
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			var resp map[string]any
			_ = json.Unmarshal(data, &resp)
			fmt.Printf("A Runner carries your occupation order (%s) to %s", action, settlementID[:8])
			if courierAt, _ := resp["courier_arrives_at"].(string); courierAt != "" {
				if t, err := time.Parse(time.RFC3339, courierAt); err == nil {
					fmt.Printf("; the runner reaches it %s", gameETA(c, t))
				}
			}
			fmt.Println(" — it executes on delivery.")
			return nil
		},
	}

	cmd.Flags().StringVar(&settlementID, "settlement", "", "occupied settlement UUID (required)")
	cmd.Flags().StringVar(&action, "action", "", "sack | burn | annex (required)")
	cmd.Flags().StringSliceVar(&goods, "goods", nil, "sack/burn only: which goods to loot (default: every lootable good)")
	_ = cmd.MarkFlagRequired("settlement")
	_ = cmd.MarkFlagRequired("action")
	return cmd
}
