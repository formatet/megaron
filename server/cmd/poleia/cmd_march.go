package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func marchCmd() *cobra.Command {
	var target, intent string
	var hoplites, hippeis, hiereus, trireme, agema int

	cmd := &cobra.Command{
		Use:   "march",
		Short: "Send an army to a province",
		Example: `  poleia march --target Korinth --intent attack --hoplites 50
  poleia march --target Argos --intent support --hoplites 20 --hippeis 5`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)

			// Resolve target name → province ID.
			targetID, err := resolveTarget(c, cfg.WorldID, target)
			if err != nil {
				return fmt.Errorf("resolve target %q: %w", target, err)
			}

			body := map[string]any{
				"target_id":       targetID,
				"intent":          intent,
				"infantry":        hoplites,
				"cavalry":         hippeis,
				"priest":          hiereus,
				"ship":            trireme,
				"elite_infantry":  agema,
				"catapult":        0,
			}
			path := fmt.Sprintf("/api/v1/worlds/%s/provinces/%s/march", cfg.WorldID, cfg.ProvinceID)
			data, err := c.post(path, body)
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			var resp map[string]any
			json.Unmarshal(data, &resp)
			dist, _ := resp["distance"].(float64)
			fmt.Printf("Army marching to %s [%s] · %.0f hexes\n", target, intent, dist)
			return nil
		},
	}

	cmd.Flags().StringVarP(&target, "target", "t", "", "target province name or UUID (required)")
	cmd.Flags().StringVarP(&intent, "intent", "i", "attack", "intent: attack|support|reinforce")
	cmd.Flags().IntVar(&hoplites, "hoplites", 0, "number of Hoplites")
	cmd.Flags().IntVar(&hippeis, "hippeis", 0, "number of Hippeis")
	cmd.Flags().IntVar(&hiereus, "hiereus", 0, "number of Hiereus")
	cmd.Flags().IntVar(&trireme, "trireme", 0, "number of Triremes")
	cmd.Flags().IntVar(&agema, "agema", 0, "number of Agema")
	_ = cmd.MarkFlagRequired("target")
	return cmd
}

// resolveTarget returns a province ID for the given name or UUID string.
func resolveTarget(c *Client, worldID, nameOrID string) (string, error) {
	// If it looks like a UUID, use directly.
	if len(nameOrID) == 36 && strings.Count(nameOrID, "-") == 4 {
		return nameOrID, nil
	}

	data, err := c.get("/api/v1/worlds/" + worldID + "/provinces")
	if err != nil {
		return "", err
	}
	var markers []map[string]any
	if err := json.Unmarshal(data, &markers); err != nil {
		return "", err
	}

	needle := strings.ToLower(nameOrID)
	var matches []map[string]any
	for _, m := range markers {
		n, _ := m["name"].(string)
		if strings.ToLower(n) == needle {
			matches = append(matches, m)
		}
	}
	if len(matches) == 0 {
		// Partial match fallback.
		for _, m := range markers {
			n, _ := m["name"].(string)
			if strings.Contains(strings.ToLower(n), needle) {
				matches = append(matches, m)
			}
		}
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("no visible province named %q", nameOrID)
	}
	if len(matches) > 1 {
		names := make([]string, len(matches))
		for i, m := range matches {
			n, _ := m["name"].(string)
			names[i] = n
		}
		return "", fmt.Errorf("ambiguous: matches %s", strings.Join(names, ", "))
	}
	id, _ := matches[0]["id"].(string)
	return id, nil
}
