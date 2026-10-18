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
	var holdToLastMan, whenLosing bool

	cmd := &cobra.Command{
		Use:   "retreat-default",
		Short: "Show or set when your armies retreat by default (realm-wide)",
		Long: `Show or set your realm-wide retreat default — when a side of yours breaks off a
battle and withdraws with its survivors.

With no flag it shows the current setting. Set exactly one of:
  --hold-to-last-man     Hold to the last man
  --when-losing          Retreat when it goes badly (the default, at the fixed loss threshold)

It applies at once, but only to units that ENTER a battle after the change; a
battle already under way keeps what its units carried in. To change a unit in a
battle now, use "keryx unit retreat-order".`,
		Example: `  keryx retreat-default
  keryx retreat-default --hold-to-last-man
  keryx retreat-default --when-losing`,
		Args: noPositionalArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)
			path := fmt.Sprintf("/api/v1/worlds/%s/retreat-default", cfg.WorldID)
			set := 0
			for _, f := range []string{"hold-to-last-man", "when-losing"} {
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
				body, modeErr := retreatModeBody(holdToLastMan, whenLosing)
				if modeErr != nil {
					return modeErr
				}
				data, err = c.put(path, body)
			default:
				return fmt.Errorf("set exactly one of --hold-to-last-man or --when-losing")
			}
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			var resp struct {
				HoldToLastMan bool `json:"hold_to_last_man"`
			}
			if err := json.Unmarshal(data, &resp); err != nil {
				return err
			}
			fmt.Println("Retreat default: " + describeRetreatDefault(resp.HoldToLastMan))
			if set == 1 {
				fmt.Println("Applies to units entering a battle from now on; battles under way keep what they had.")
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&holdToLastMan, "hold-to-last-man", false, "never break — fight to annihilation")
	cmd.Flags().BoolVar(&whenLosing, "when-losing", false, "retreat when losses reach the fixed threshold (the default)")
	return cmd
}

func describeRetreatDefault(hold bool) string {
	if hold {
		return "Hold to the last man"
	}
	return "Retreat when it goes badly"
}

func retreatModeBody(hold, losing bool) (map[string]any, error) {
	if hold == losing {
		return nil, fmt.Errorf("choose exactly one of --hold-to-last-man or --when-losing")
	}
	if hold {
		return map[string]any{"hold_to_last_man": true}, nil
	}
	return map[string]any{"when_losing": true}, nil
}
