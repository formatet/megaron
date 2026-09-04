package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

// joinResp mirrors POST /worlds/:worldID/join (api/handlers/join.go) — the
// wire JSON only, matching the convention in cmd_founding.go. Fields cover
// both the fresh-join (201) and the idempotent-existing (200) shapes: a
// fresh join returns host_unit_id/tile/culture/population, an existing
// settlement returns province_id/existing, an existing wandering host
// returns host_unit_id/existing.
type joinResp struct {
	Existing   bool   `json:"existing"`
	ProvinceID string `json:"province_id"`
	HostUnitID string `json:"host_unit_id"`
	Tile       struct {
		Q int
		R int
	} `json:"tile"`
	Culture    string `json:"culture"`
	Population int    `json:"population"`
}

// joinCmd implements `keryx join` — without it, a player with only keryx
// (no raw curl) had no way into a world at all (megaron_plan_tva_gate1_slices
// §1, 2026-09-04: `grep -rn "/join" cmd/keryx/` gave zero hits).
//
// No --name/--culture flags: the server does accept a province_name field,
// but it only feeds a uniqueness check on join — it is never stored or
// passed to seedNomadicHost, so nothing at founding ever reads it back
// (verified in api/handlers/join.go, 2026-09-04). Exposing a flag whose only
// effect is "reject if taken, otherwise silently discard" would read as
// reserving a name when it does not. The name that actually sticks is
// `founding settle --name`. Culture is MVP-locked to minoan server-side
// regardless of what's sent, so there is nothing for a flag to control yet.
func joinCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "join",
		Short: "Join the active world as a Nomadic Host — the first step before founding a city",
		Long: `Join the active world (cfg.WorldID). You arrive as a Nomadic Host: a people
on the move with a few months of rations, not yet a city. Run
'keryx founding status' next to see the host, then 'keryx unit march' to
find a site and 'keryx founding settle' to found your metropolis there.

Safe to run twice: if you already have a settlement or a wandering host in
this world, the server returns that instead of erroring, and this command
reports it rather than failing.`,
		Example: `  keryx join
  keryx join --json`,
		Args: noPositionalArgs(), // no flags at all — nothing to guess
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)
			data, err := c.post(fmt.Sprintf("/api/v1/worlds/%s/join", cfg.WorldID), map[string]any{})
			if err != nil {
				return err
			}

			// The world changed shape: the player now has a host (or already a
			// province). Re-resolve into the config so province-scoped verbs work
			// immediately — same pattern as `login` and `founding settle`. A brand
			// new host has no province yet, so this stays empty until founding;
			// that is expected, not a failure.
			if pid := autoDetectProvince(c, cfg.WorldID); pid != "" {
				cfg.ProvinceID = pid
				_ = saveConfig(cfg)
			}

			if jsonMode {
				printRawJSON(data)
				return nil
			}

			var resp joinResp
			_ = json.Unmarshal(data, &resp)

			if resp.Existing {
				if resp.ProvinceID != "" {
					fmt.Println("You already hold a metropolis in this world — run: keryx status")
				} else {
					fmt.Println("You already have a wandering host in this world — run: keryx founding status")
				}
				return nil
			}

			fmt.Printf("Joined! A Nomadic Host of %d people sets out at (%d,%d).\n",
				resp.Population, resp.Tile.Q, resp.Tile.R)
			fmt.Println("  Run: keryx founding status")
			return nil
		},
	}
}
