package main

import (
	"fmt"
	"strings"
)

// Food quantities and durations come from the server; this only orders labels.
var foodDisplayGoods = []string{"grain", "fish", "oil", "livestock"}

func foodGoodsText(goods map[string]any) string {
	parts := []string{}
	for _, good := range foodDisplayGoods {
		if qty, ok := goods[good].(float64); ok {
			amount := strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.4f", qty), "0"), ".")
			if qty > 0 && qty < 0.0001 {
				amount = "<0.0001"
			}
			parts = append(parts, fmt.Sprintf("%s %s", amount, good))
		}
	}
	return strings.Join(parts, ", ")
}

func cityFoodLine(sett map[string]any) string {
	food, _ := sett["food"].(map[string]any)
	cov, ok := food["coverage_ticks"].(float64)
	if !ok {
		return "Food stores not reported."
	}
	goods, _ := food["goods"].(map[string]any)
	detail := foodGoodsText(goods)
	if detail == "" {
		detail = "city grain + fish stocks"
	}
	return fmt.Sprintf("Food for %s at current consumption (%s)", formatDays(cov, "%.1f"), detail)
}

func peopleFoodLine(sett map[string]any) string {
	food, _ := sett["food"].(map[string]any)
	production, ok := food["production_per_tick"].(float64)
	consumption, consOK := food["consumption_per_tick"].(float64)
	net, netOK := food["food_net_per_tick"].(float64)
	if !ok || !consOK || !netOK {
		return ""
	}
	herdPart := ""
	herd, _ := food["herd"].(map[string]any)
	if surplus, _ := herd["surplus_rate"].(float64); surplus > 0 {
		herdPart = fmt.Sprintf(" + herd %.1f", surplus)
	}
	return fmt.Sprintf("  Food     production %.1f%s − people eat %.1f = net %s per game day (before army)", production, herdPart, consumption, formatNetPerTick(net))
}

func upkeepFood(up map[string]any) (float64, string) {
	if value, ok := up["food"].(float64); ok {
		return value, "food"
	}
	value, _ := up["grain"].(float64)
	return value, "grain"
}

// herdLine is the pen's one line: count / cap, and what a full pen means.
func herdLine(sett map[string]any) string {
	food, _ := sett["food"].(map[string]any)
	herd, _ := food["herd"].(map[string]any)
	count, ok1 := herd["count"].(float64)
	capacity, ok2 := herd["cap"].(float64)
	if !ok1 || !ok2 {
		return ""
	}
	if count >= capacity {
		return fmt.Sprintf("Herd %.0f / %.0f — full: new animals are eaten, then lost. A pen raises the cap.", count, capacity)
	}
	line := fmt.Sprintf("Herd %.0f / %.0f", count, capacity)
	if born, _ := herd["births_next"].(float64); born > 0 {
		in, _ := herd["births_in"].(float64)
		grain, _ := herd["births_grain"].(float64)
		calves, days := "calves", "game days"
		if born == 1 {
			calves = "calf"
		}
		if in == 1 {
			days = "game day"
		}
		line += fmt.Sprintf(" — %.0f %s in %.0f %s (%.0f grain)", born, calves, in, days, grain)
	}
	return line
}

// The server owns progress and the growth forecast. Share this row between
// status and city rather than estimating population growth in either client.
func nextCitizenLine(c *Client, sett map[string]any) string {
	progress, ok := sett["next_citizen_progress"].(float64)
	if !ok {
		return ""
	}
	food, _ := sett["food"].(map[string]any)
	state, _ := food["growth_state"].(string)
	ending := map[string]string{"holding": "not growing", "capped": "the walls are full", "shrinking": "shrinking"}[state]
	ticks, hasETA := sett["next_citizen_in_ticks"].(float64)
	if ending == "" && hasETA && ticks > 0 {
		duration := strings.SplitN(formatDuration(c, ticks, "%g"), " · ", 2)
		if ticks <= 1 {
			ending = "next game day"
		} else {
			ending = "in about " + duration[0]
		}
		if len(duration) > 1 {
			ending += " (" + duration[1] + ")"
		}
	}
	if ending == "" && state == "growing" {
		ending = "the walls are full before then"
	} else if ending == "" {
		ending = "estimate unavailable"
	}
	return fmt.Sprintf("Next citizen %g / 100 · %s", progress, ending)
}
