package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func stormsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "storms",
		Short: "Storms at sea you can see, and where you last saw the others",
		Long: `A storm is three connected sea hexes that drift slowly. You see one only inside your
sight; afterwards it is remembered where you LAST saw it (with the day), not where it is now.
A ship that sails into a storm takes 2 hull damage (of 5), once per storm however long it stays inside.`,
		Args: noPositionalArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)
			data, err := c.get(fmt.Sprintf("/api/v1/worlds/%s/storms", cfg.WorldID))
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			out, err := formatStorms(data)
			if err != nil {
				return err
			}
			fmt.Print(out)
			return nil
		},
	}
}

// formatStorms renders GET /storms for a person: live storms with their heading, then the
// remembered ones with the day they were last seen.
func formatStorms(data []byte) (string, error) {
	var resp struct {
		Storms []struct {
			Tier     string `json:"tier"`
			Heading  string `json:"heading"`
			SeenTick int    `json:"seen_tick"`
			Hexes    []struct {
				Q int `json:"q"`
				R int `json:"r"`
			} `json:"hexes"`
		} `json:"storms"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", err
	}
	if len(resp.Storms) == 0 {
		return "No storms known. A storm shows only inside your sight.\n", nil
	}
	hexes := func(i int) string {
		var parts []string
		for _, h := range resp.Storms[i].Hexes {
			parts = append(parts, fmt.Sprintf("(%d,%d)", h.Q, h.R))
		}
		return strings.Join(parts, " ")
	}
	var b strings.Builder
	for i, s := range resp.Storms {
		if s.Tier != "live" {
			continue
		}
		fmt.Fprintf(&b, "Storm in sight, drifting %s: %s\n", s.Heading, hexes(i))
	}
	for i, s := range resp.Storms {
		if s.Tier == "live" {
			continue
		}
		fmt.Fprintf(&b, "Storm last seen on day %d: %s (it has moved since)\n", s.SeenTick, hexes(i))
	}
	return b.String(), nil
}
