package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func formatDisbandResult(resp map[string]any) string {
	city, _ := resp["settlement_name"].(string)
	lines := []string{}
	if units, ok := resp["disbanded"].([]any); ok {
		for _, raw := range units {
			if u, ok := raw.(map[string]any); ok {
				name, _ := u["display_name"].(string)
				people, known := u["people_returned"].(float64)
				if name == "" || !known {
					continue
				}
				noun, verb := "people", "return"
				if people == 1 {
					noun, verb = "person", "returns"
				}
				lines = append(lines, fmt.Sprintf("%s disbanded — %d %s %s to %s.", name, int(people), noun, verb, city))
			}
		}
	}
	if len(lines) == 0 {
		return "Unit disbanded. Refresh units to see the result."
	}
	return strings.Join(lines, "\n")
}

// Resolve only among the player's units, preserving ambiguity instead of guessing.
func disbandUnitIDs(c *Client, selectors []string) ([]string, error) {
	data, err := c.get(fmt.Sprintf("/api/v1/worlds/%s/units", cfg.WorldID))
	if err != nil {
		return nil, err
	}
	var list struct {
		Units []unitRow `json:"units"`
	}
	if err = json.Unmarshal(data, &list); err != nil {
		return nil, err
	}
	ids := []string{}
	seen := map[string]bool{}
	for _, selector := range selectors {
		needle := strings.ToLower(strings.TrimSpace(selector))
		matches := []unitRow{}
		for _, u := range list.Units {
			if needle != "" && (strings.HasPrefix(strings.ToLower(u.ID), needle) || strings.EqualFold(u.DisplayName, selector) || (u.Name != nil && strings.EqualFold(*u.Name, selector))) {
				matches = append(matches, u)
			}
		}
		if len(matches) != 1 {
			return nil, fmt.Errorf("%q matches %d units — choose a unique name or id from keryx units", selector, len(matches))
		}
		if seen[matches[0].ID] {
			return nil, fmt.Errorf("choose each whole unit only once")
		}
		seen[matches[0].ID] = true
		ids = append(ids, matches[0].ID)
	}
	return ids, nil
}

func disbandCmd() *cobra.Command {
	var selectors []string
	var province string
	cmd := &cobra.Command{Use: "disband", Short: "Disband a whole unit in a city; its remaining people return there", Args: noPositionalArgs(), Example: `  keryx disband --unit "1st Spearmen of Phaistos"
  keryx disband --unit <unit-id> --province Phaistos
  keryx disband --unit <unit-id> --unit <another-unit-id>`, RunE: func(cmd *cobra.Command, _ []string) error {
		if len(selectors) == 0 {
			return fmt.Errorf("disband a whole unit by its id or unique name: use --unit (see keryx units)")
		}
		c := newClient(cfg)
		city, err := selectedProvince(c, province, nil)
		if err != nil {
			return err
		}
		ids, err := disbandUnitIDs(c, selectors)
		if err != nil {
			return err
		}
		data, err := c.post(fmt.Sprintf("/api/v1/worlds/%s/provinces/%s/disband", cfg.WorldID, city), map[string]any{"unit_ids": ids})
		if err != nil {
			return err
		}
		if jsonMode {
			printRawJSON(data)
			return nil
		}
		var resp map[string]any
		if err = json.Unmarshal(data, &resp); err != nil {
			return err
		}
		fmt.Println(formatDisbandResult(resp))
		return nil
	}}
	cmd.Flags().StringArrayVar(&selectors, "unit", nil, "whole unit ID, unique ID prefix or name (repeat to select several)")
	cmd.Flags().StringVar(&province, "province", "", "city name or province ID (default: selected city)")
	return cmd
}
