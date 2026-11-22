package main

import (
	"encoding/json"
	"fmt"
	"math"
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
			Name      string           `json:"name"`
			Quotes    []buildListQuote `json:"build_quotes"`
			Resources map[string]struct {
				Amount float64 `json:"amount"`
			} `json:"resources"`
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
	fmt.Printf("%-15s %-12s %-7s %-7s %-42s %-5s %s\n", "Type", "Site", "Current", "Next", "Costs (next level)", "game days", "Purpose / requires · afford")
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
			next, days, cost, afford := "max", "—", "—", "—"
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
				afford = affordText(q, city.Settlement.Resources != nil, func(good string) float64 {
					return city.Settlement.Resources[good].Amount
				})
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
			typ := q.Type
			if typ == "silo" { // the player's word; build --type granary maps it back
				typ = "granary"
			}
			row := fmt.Sprintf("%-15s %-12s L%-6d %-7s %-42s %-5s %s", typ, site, q.Current, next, cost, days, requires)
			if q.Next != nil { // at max level there is nothing to build
				// Afford goes last: its width varies, so a column would push the rest out of line.
				row += "  · afford: " + afford + "  → " + buildCommandLine(city.Settlement.Name, typ, q)
			}
			fmt.Println(row)
		}
	}
	return nil
}

// buildCommandLine is the exact command that builds (or levels up) this row.
func buildCommandLine(cityName, typ string, q buildListQuote) string {
	line := "keryx build --city " + commandArgument(cityName) + " --type " + typ
	switch {
	case q.GodID != "":
		line += " --god " + q.GodID
	case q.NewSite:
		line += " --hex <q,r>"
	case q.Q != nil && q.R != nil:
		line += fmt.Sprintf(" --hex %d,%d", *q.Q, *q.R)
	}
	return line
}

// affordText says whether the city holds the quoted cost: "yes", the web's
// "needs 4 stone, city has 1" (build_rows.js shortfalls: spare cedar covers
// timber, silver is a good), or "?" when the server reported no stock.
func affordText(q buildListQuote, known bool, stock func(string) float64) string {
	if !known {
		return "?"
	}
	need := map[string]float64{}
	for good, n := range q.Costs {
		need[good] = n
	}
	if q.Silver > 0 {
		need["silver"] = q.Silver
	}
	goods := make([]string, 0, len(need))
	for good := range need {
		goods = append(goods, good)
	}
	sort.Strings(goods)
	var lacks []string
	for _, good := range goods {
		per := need[good]
		if !(per > 0) {
			continue
		}
		have := stock(good)
		if good == "timber" {
			have += math.Max(0, stock("cedar")-need["cedar"])
		}
		if per-have > 1e-6 {
			lacks = append(lacks, fmt.Sprintf("needs %d %s, city has %d", int(math.Ceil(per-1e-6)), good, int(math.Floor(have+1e-6))))
		}
	}
	if len(lacks) == 0 {
		return "yes"
	}
	return strings.Join(lacks, "; ")
}
