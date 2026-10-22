package main

import (
	"encoding/json"
	"fmt"
	"github.com/spf13/cobra"
	"sort"
	"strings"
)

// The existing Cities directory owns knowledge. Rumours never supply targets.
type destinationCity struct {
	ID        string `json:"settlement_id"`
	Name      string `json:"name"`
	Owner     string `json:"owner"`
	Own       bool   `json:"own"`
	Knowledge string `json:"knowledge"`
	Q         *int   `json:"q"`
	R         *int   `json:"r"`
}

func destinationCities(c *Client, worldID string) ([]destinationCity, error) {
	data, err := c.get("/api/v1/worlds/" + worldID + "/cities")
	if err != nil {
		return nil, err
	}
	var all []destinationCity
	if err := json.Unmarshal(data, &all); err != nil {
		return nil, err
	}
	cities := make([]destinationCity, 0, len(all))
	for _, city := range all {
		if city.Knowledge == "known" && city.Q != nil && city.R != nil && city.Name != "" {
			cities = append(cities, city)
		}
	}
	sort.SliceStable(cities, func(i, j int) bool {
		a, b := cities[i], cities[j]
		if a.Own != b.Own {
			return a.Own
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.ID < b.ID
	})
	return cities, nil
}
func resolveCityTarget(cmd *cobra.Command, c *Client, to, target string, q, r int) (int, int, string, bool, error) {
	if cmd.Flags().Changed("to") {
		if cmd.Flags().Changed("target") || cmd.Flags().Changed("q") || cmd.Flags().Changed("r") {
			return 0, 0, "", false, fmt.Errorf("give the target as either --to city, --target q,r or --q/--r")
		}
		name := strings.TrimSpace(to)
		if name == "" {
			return 0, 0, "", false, fmt.Errorf("--to needs a city name")
		}
		cities, err := destinationCities(c, cfg.WorldID)
		if err != nil {
			return 0, 0, "", false, fmt.Errorf("could not load known cities: %w", err)
		}
		var matches []destinationCity
		for _, city := range cities {
			if strings.EqualFold(city.Name, name) {
				matches = append(matches, city)
			}
		}
		if len(matches) == 0 {
			return 0, 0, "", false, fmt.Errorf("you know no city called %q", name)
		}
		if len(matches) > 1 {
			labels := make([]string, 0, len(matches))
			for _, city := range matches {
				owner := city.Owner
				if city.Own {
					owner = "yours"
				}
				labels = append(labels, fmt.Sprintf("%s — %s (%d,%d)", city.Name, owner, *city.Q, *city.R))
			}
			return 0, 0, "", false, fmt.Errorf("city name %q is ambiguous: %s; use --target q,r", name, strings.Join(labels, "; "))
		}
		city := matches[0]
		return *city.Q, *city.R, city.Name, true, nil
	}
	tq, tr, given, err := resolveTargetHex(cmd, target, q, r)
	return tq, tr, "", given, err
}

// Receipt lookup is best effort after acceptance: a directory outage cannot
// turn a sent order into a reported failure. --to already supplied its name.
func cityTargetLabel(c *Client, q, r int, name string) string {
	if name == "" {
		if cities, err := destinationCities(c, cfg.WorldID); err == nil {
			for _, city := range cities {
				if *city.Q == q && *city.R == r {
					name = city.Name
					break
				}
			}
		}
	}
	if name != "" {
		return fmt.Sprintf("%s (%d,%d)", name, q, r)
	}
	return fmt.Sprintf("(%d,%d)", q, r)
}
