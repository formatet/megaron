package main

import (
	"fmt"
	"strings"
)

// Food quantities and durations come from the server; this only orders labels.
var foodDisplayGoods = []string{"grain", "fish", "oil", "wine", "livestock"}

func foodGoodsText(goods map[string]any) string {
	parts := []string{}
	for _, good := range foodDisplayGoods {
		if qty, ok := goods[good].(float64); ok {
			parts = append(parts, fmt.Sprintf("%s %s", strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.4f", qty), "0"), "."), good))
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
	return fmt.Sprintf("  Food     production %.1f − people eat %.1f = net %s per day (before army)", production, consumption, formatNetPerTick(net))
}

func upkeepFood(up map[string]any) (float64, string) {
	if value, ok := up["food"].(float64); ok {
		return value, "food"
	}
	value, _ := up["grain"].(float64)
	return value, "grain"
}
