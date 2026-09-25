package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

// retreatDefaultCmd shows or sets the realm-wide retreat default
// (GET/PUT /worlds/{w}/retreat-default, mig 145): the retreat order every
// unit of yours carries into a battle it enters from now on. The per-battle
// override is `keryx unit retreat-order`.
func retreatDefaultCmd() *cobra.Command {
	var retreatAtLoss float64
	var holdToLastMan, byLoyalty bool

	cmd := &cobra.Command{
		Use:   "retreat-default",
		Short: "Show or set when your armies retreat by default (realm-wide)",
		Long: `Show or set your realm-wide retreat default — when a side of yours breaks off a
battle and withdraws with its survivors.

With no flag it shows the current setting. Set exactly one of:
  --retreat-at-loss <f>  break when the side is down to this fraction (0-1) of its
                         starting strength (0.25 = break after losing three quarters)
  --hold-to-last-man     never break — fight to annihilation
  --by-loyalty           the default: break earlier or later by the troops' loyalty

It applies at once, but only to units that ENTER a battle after the change; a
battle already under way keeps what its units carried in. To change a unit in a
battle now, use "keryx unit retreat-order".`,
		Example: `  keryx retreat-default
  keryx retreat-default --retreat-at-loss 0.5
  keryx retreat-default --hold-to-last-man
  keryx retreat-default --by-loyalty`,
		Args: noPositionalArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)
			path := fmt.Sprintf("/api/v1/worlds/%s/retreat-default", cfg.WorldID)
			set := 0
			for _, f := range []string{"retreat-at-loss", "hold-to-last-man", "by-loyalty"} {
				if cmd.Flags().Changed(f) {
					set++
				}
			}
			var data []byte
			var err error
			switch set {
			case 0:
				data, err = c.get(path)
			case 1:
				body := map[string]any{}
				switch {
				case cmd.Flags().Changed("retreat-at-loss"):
					body["retreat_at_loss"] = retreatAtLoss
				case cmd.Flags().Changed("hold-to-last-man"):
					body["hold_to_last_man"] = holdToLastMan
				default:
					body["by_loyalty"] = byLoyalty
				}
				data, err = c.put(path, body)
			default:
				return fmt.Errorf("set only one of --retreat-at-loss, --hold-to-last-man or --by-loyalty")
			}
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			var resp struct {
				RetreatAtLoss *float64 `json:"retreat_at_loss"`
				HoldToLastMan bool     `json:"hold_to_last_man"`
			}
			if err := json.Unmarshal(data, &resp); err != nil {
				return err
			}
			fmt.Println("Retreat default: " + describeRetreatDefault(resp.RetreatAtLoss, resp.HoldToLastMan))
			if set == 1 {
				fmt.Println("Applies to units entering a battle from now on; battles under way keep what they had.")
			}
			return nil
		},
	}
	cmd.Flags().Float64Var(&retreatAtLoss, "retreat-at-loss", 0,
		"fraction of starting strength (0-1) at which a side of yours breaks and retreats")
	cmd.Flags().BoolVar(&holdToLastMan, "hold-to-last-man", false, "never break — fight to annihilation")
	cmd.Flags().BoolVar(&byLoyalty, "by-loyalty", false, "break by the troops' loyalty (the default)")
	return cmd
}

func describeRetreatDefault(retreatAtLoss *float64, hold bool) string {
	switch {
	case hold:
		return "hold to the last man"
	case retreatAtLoss != nil:
		return fmt.Sprintf("retreat at %.0f%% losses (when down to %.0f%% of starting strength)",
			(1-*retreatAtLoss)*100, *retreatAtLoss*100)
	default:
		return "by the troops' loyalty — the more loyal their city, the longer they hold"
	}
}
