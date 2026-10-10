package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

// riseCmd and leaveCmd are the two choices a Wanax has once their last city
// has fallen (POST /worlds/:id/rise and /leave, api/handlers/rise.go;
// megaron_sista_staden.md). The web offers the same two on the epitaph page.
func riseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rise",
		Short: "After your last city fell: gather the survivors as a new host on another shore",
		Long: `Only for a Wanax who has lost every city in this world. The survivors set out
as a small Nomadic Host (400 to 800 people, one company of spearmen, little
silver) on another landmass than the one that fell. Your letters, contacts and
map knowledge stay yours. Then found a city again as at the start:
'keryx founding status', 'keryx unit march', 'keryx founding settle'.

Safe to run twice: if you already have a host, it reports that one.`,
		Example: `  keryx rise
  keryx rise --json`,
		Args: noPositionalArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)
			data, err := c.post(fmt.Sprintf("/api/v1/worlds/%s/rise", cfg.WorldID), map[string]any{})
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			var resp joinResp
			_ = json.Unmarshal(data, &resp)
			if resp.Existing {
				fmt.Println("You already have a wandering host in this world — run: keryx founding status")
				return nil
			}
			fmt.Printf("The survivors gather. A host of %d people sets out at (%d,%d).\n",
				resp.Population, resp.Tile.Q, resp.Tile.R)
			fmt.Println("  Run: keryx founding status")
			return nil
		},
	}
}

func leaveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "leave",
		Short: "After your last city fell: leave this world (your account remains)",
		Long: `Only for a Wanax who has lost every city in this world. You leave this world
for good and cannot rise in it again. Your account remains for the next world.
To delete the account itself, email the admin.`,
		Example: `  keryx leave`,
		Args:    noPositionalArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)
			data, err := c.post(fmt.Sprintf("/api/v1/worlds/%s/leave", cfg.WorldID), map[string]any{})
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			fmt.Println("You have left this world. Your account remains for the next one.")
			return nil
		},
	}
}
