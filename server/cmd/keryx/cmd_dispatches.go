package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
)

// dispatchesCmd shows or changes which notification kinds become dispatches
// (GET/PUT/DELETE /api/v1/notification-preferences, megaron_plan_dispatches.md
// §2/§6) — the web's "Stop these as dispatches" checkbox. Server-side and per
// player, so a mute set here holds on the web and vice versa. Muting never
// touches the archive: `keryx notifications` still lists every row.
func dispatchesCmd() *cobra.Command {
	var mute, unmute string

	cmd := &cobra.Command{
		Use:   "dispatches",
		Short: "Show or change which notification kinds reach you as dispatches",
		Long: `Show or change which notification kinds reach you as dispatches — the pushed
alerts that arrive while you play (the chips in the web's top bar, the live
lines in "keryx watch").

With no flag it lists the kinds you have muted. Set one of:
  --mute <Kind>    stop this kind arriving as a dispatch
  --unmute <Kind>  let it arrive again

<Kind> is the name "keryx notifications" prints in its second column, e.g.
ColonyFounded. Muting only silences the dispatch — every event is still kept
in "keryx notifications". The setting is shared with the web.`,
		Example: `  keryx dispatches
  keryx dispatches --mute ScoutReport
  keryx dispatches --unmute ScoutReport`,
		Args: noPositionalArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if mute != "" && unmute != "" {
				return fmt.Errorf("set only one of --mute or --unmute")
			}
			c := newClient(cfg)
			const base = "/api/v1/notification-preferences"
			switch {
			case mute != "":
				if _, err := c.put(base+"/"+url.PathEscape(mute), nil); err != nil {
					return err
				}
				fmt.Printf("%s will no longer arrive as a dispatch. It is still kept in keryx notifications.\n", mute)
				return nil
			case unmute != "":
				if _, err := c.delete(base + "/" + url.PathEscape(unmute)); err != nil {
					return err
				}
				fmt.Printf("%s arrives as a dispatch again.\n", unmute)
				return nil
			}
			data, err := c.get(base)
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			var resp struct {
				MutedKinds []string `json:"muted_kinds"`
			}
			if err := json.Unmarshal(data, &resp); err != nil {
				return err
			}
			fmt.Println(describeMutedKinds(resp.MutedKinds))
			return nil
		},
	}
	cmd.Flags().StringVar(&mute, "mute", "", "notification kind to stop as a dispatch")
	cmd.Flags().StringVar(&unmute, "unmute", "", "notification kind to let through as a dispatch again")
	return cmd
}

func describeMutedKinds(kinds []string) string {
	if len(kinds) == 0 {
		return "Every notification kind arrives as a dispatch — none muted."
	}
	return "Muted as dispatches (still kept in keryx notifications): " + strings.Join(kinds, ", ")
}
