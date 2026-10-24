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
	return fmt.Sprintf("Storm over %g %s — no fishing there while it stands.", n, grounds)
}
