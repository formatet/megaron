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
	day := "days"
	if remaining == 1 {
		day = "day"
	}
	return fmt.Sprintf("Blessed harvest: +%g%% %s yield · %g %s left", bonus*100, strings.Join(goods, ", "), remaining, day)
}
