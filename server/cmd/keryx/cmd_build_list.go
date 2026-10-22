package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

type buildListQuote struct {
	Type     string             `json:"type"`
	Current  int                `json:"current_level"`
	Next     *int               `json:"next_level"`
	AtMax    bool               `json:"at_max_level"`
	Costs    map[string]float64 `json:"costs"`
	Silver   float64            `json:"cost_silver"`
	Duration int                `json:"duration_game_days"`
	Q        *int               `json:"hex_q,omitempty"`
	R        *int               `json:"hex_r,omitempty"`
	NewSite  bool               `json:"new_site,omitempty"`
	GodID    string             `json:"god_id,omitempty"`
}

func listCityBuilds(c *Client, world, prov string) error {
	data, err := c.get(fmt.Sprintf("/api/v1/worlds/%s/provinces/%s", world, prov))
	if err != nil {
		return err
	}
	var city struct {
		Settlement struct {
			Name   string           `json:"name"`
			Quotes []buildListQuote `json:"build_quotes"`
		} `json:"settlement"`
	}
	if err = json.Unmarshal(data, &city); err != nil {
		return err
	}
	if city.Settlement.Quotes == nil {
		return fmt.Errorf("build prices for this city were not reported by the server")
	}
	data, err = c.get("/api/v1/buildings")
	if err != nil {
		return err
	}
	var catalogue []map[string]any
	if err = json.Unmarshal(data, &catalogue); err != nil {
		return err
	}
	for _, b := range catalogue {
		quotes := []buildListQuote{}
		for _, q := range city.Settlement.Quotes {
			if b["type"] == q.Type {
				quotes = append(quotes, q)
			}
		}
		b["build_quotes"] = quotes
	}
	if jsonMode {
		printJSON(catalogue)
		return nil
	}
	fmt.Printf("Construction in %s — prices for the next level, not a reservation.\n", city.Settlement.Name)
	fmt.Println("Build the same building again to raise its level. Hex rows refer to that exact site; a new hex starts at L1.")
	fmt.Println("Build queue: `build --queue` shows remaining queue slots.")
	fmt.Println("A temple is built for one god, one temple per god: `build --type temple --god <god>` (the Site column names the god).")
	fmt.Printf("%-15s %-12s %-7s %-7s %-42s %-5s %s\n", "Type", "Site", "Current", "Next", "Costs (next level)", "Days", "Purpose / requires")
	for _, b := range catalogue {
		for _, q := range city.Settlement.Quotes {
			if b["type"] != q.Type {
				continue
			}
			site := "city"
			if q.GodID != "" {
				site = q.GodID
			} else if q.NewSite {
				site = "new hex"
			} else if q.Q != nil && q.R != nil {
				site = fmt.Sprintf("(%d,%d)", *q.Q, *q.R)
			}
			next, days, cost := "max", "—", "—"
			if q.Next != nil {
				next = fmt.Sprintf("L%d", *q.Next)
				days = fmt.Sprint(q.Duration)
				var parts []string
				for good, n := range q.Costs {
					parts = append(parts, fmt.Sprintf("%s×%g", good, n))
				}
				sort.Strings(parts)
				if q.Silver > 0 {
					parts = append(parts, fmt.Sprintf("silver×%g", q.Silver))
				}
				cost = strings.Join(parts, " ")
			}
			requires := fmt.Sprint(b["purpose"])
			if coastal, _ := b["requires_coastal"].(bool); coastal {
				requires += "; coastal city"
			}
			for _, key := range []string{"requires_deposits", "requires_terrain"} {
				if v, ok := b[key]; ok {
					requires += "; " + fmt.Sprint(v)
				}
			}
			fmt.Printf("%-15s %-12s L%-6d %-7s %-42s %-5s %s\n", q.Type, site, q.Current, next, cost, days, requires)
		}
	}
	return nil
}
