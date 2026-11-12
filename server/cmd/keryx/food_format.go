package main

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Food quantities and durations come from the server; this only orders labels.
// The first three are the real foods (callers slice [:3]); livestock is the reserve.
var foodDisplayGoods = []string{"fish", "grain", "oil", "livestock"}

// foodListGoods is the order they are listed to the player: alphabetical.
var foodListGoods = []string{"fish", "grain", "livestock", "oil"}

func foodGoodsText(goods map[string]any) string {
	parts := []string{}
	for _, good := range foodListGoods {
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

// foodAnswerLine is the one answer to "is this city's food going up or down, and how
// long does it last?": the forecast's net after everything that eats (citizens, army,
// temples, herd) and, when it is falling, the days left. The web says the same words
// (food_line.js foodAnswerValue). "" when the server sent no forecast.
// foodAnswer reads forecast.food (net, and the days left when falling) out of a
// /provinces/{id} settlement. ok is false when the server sent no forecast.
func foodAnswer(sett map[string]any) (net float64, left *float64, ok bool) {
	raw, err := json.Marshal(sett["forecast"])
	if err != nil {
		return 0, nil, false
	}
	var forecast struct {
		Food *struct {
			Net  *float64 `json:"net"`
			Days *float64 `json:"days"`
		} `json:"food"`
	}
	if json.Unmarshal(raw, &forecast) != nil || forecast.Food == nil || forecast.Food.Net == nil {
		return 0, nil, false
	}
	return *forecast.Food.Net, forecast.Food.Days, true
}

func foodAnswerNet(sett map[string]any) (float64, bool) {
	net, _, ok := foodAnswer(sett)
	return net, ok
}

// foodAnswerLine is the one answer to "is this city's food going up or down, and how
// long does it last?": the forecast's net after everything that eats (citizens, army,
// temples, herd) and, when it is falling, the days left. The web says the same words
// (food_line.js foodAnswerValue). "" when the server sent no forecast.
func foodAnswerLine(sett map[string]any) string {
	net, left, ok := foodAnswer(sett)
	if !ok {
		return ""
	}
	shown := math.Round(math.Abs(net)*10) / 10
	if shown == 0 {
		return "Food about even"
	}
	sign := "+"
	if net < 0 {
		sign = "\u2212"
	}
	line := fmt.Sprintf("Food %s%s per game day", sign, strconv.FormatFloat(shown, 'f', -1, 64))
	if left != nil && net < 0 {
		if *left > 1000 {
			return line + " · lasts over 1,000 game days"
		}
		line += " · lasts " + formatDays(*left, "%g")
	}
	return line
}

func cityFoodLine(sett map[string]any) string {
	food, _ := sett["food"].(map[string]any)
	goods, _ := food["goods"].(map[string]any)
	detail := foodGoodsText(goods)
	answer := foodAnswerLine(sett)
	switch {
	case answer != "" && detail != "":
		return answer + " (stocks: " + detail + ")"
	case answer != "":
		return answer
	case detail != "":
		return "Food stocks: " + detail
	}
	return "Food stores not reported."
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
		herdPart = fmt.Sprintf(" + herd %.2f", surplus)
	}
	return fmt.Sprintf("  Growth   production %.1f%s − people eat %.1f = growth surplus %s per game day before army", production, herdPart, consumption, formatNetPerTick(net))
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
