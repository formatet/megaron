package main

import (
	"fmt"
	"strings"
)

// Values and affected goods come from the settlement response, never CLI tuning.
func harvestBlessingLine(sett map[string]any) string {
	b, _ := sett["harvest_blessing"].(map[string]any)
	remaining, _ := b["remaining_ticks"].(float64)
	bonus, _ := b["yield_bonus"].(float64)
	if remaining <= 0 || bonus <= 0 {
		return ""
	}
	raw, _ := b["goods"].([]any)
	goods := make([]string, 0, len(raw))
	for _, v := range raw {
		if g, ok := v.(string); ok {
			goods = append(goods, g)
		}
	}
	if len(goods) == 0 {
		return ""
	}
	day := "game days"
	if remaining == 1 {
		day = "game day"
	}
	return fmt.Sprintf("Blessed harvest: +%g%% %s yield · %g %s left", bonus*100, strings.Join(goods, ", "), remaining, day)
}

// stormFishingLine names the city's fishing grounds a storm stands on now
// (Timothy 2026-10-24: storm stops the fishing); "" in calm weather.
func stormFishingLine(sett map[string]any) string {
	n, _ := sett["storm_no_fishing"].(float64)
	if n <= 0 {
		return ""
	}
	grounds := "fishing grounds"
	if n == 1 {
		grounds = "fishing ground"
	}
	return fmt.Sprintf("Storm over %g %s — its fishers stay but catch nothing while it stands.", n, grounds)
}

// cityBlessingLines names the fertility and Builder's Blessing standing over the city (steg 5a), one line each.
func cityBlessingLines(sett map[string]any) []string {
	list, _ := sett["city_blessings"].([]any)
	var out []string
	for _, raw := range list {
		b, _ := raw.(map[string]any)
		name, _ := b["prayer"].(string)
		scope, _ := b["scope"].(string)
		days, _ := b["remaining_game_days"].(float64)
		effect, _ := b["effect"].(string)
		where := "on this city"
		if scope == "realm" {
			where = "on the whole realm"
		}
		what := "people and herds grow half as fast again"
		if effect == "blessed_building" {
			what = "what is left of each build takes three quarters of the time, and so do builds begun"
		}
		out = append(out, fmt.Sprintf("%s %s: %s · %s left", name, where, what, formatDays(int(days))))
	}
	return out
}

// scapegoatLine says that a goat stands for the realm (from kharis_wrath); "" when none does.
func scapegoatLine(wrath map[string]any) string {
	days, _ := wrath["scapegoat_remaining_game_days"].(float64)
	if days <= 0 {
		return ""
	}
	return fmt.Sprintf("A scapegoat stands for your realm for %s: the next divine punishment is borne by it.", formatDays(int(days)))
}
