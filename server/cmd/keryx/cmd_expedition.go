package main

// Upptäckarexpeditionen (megaron_plan_upptackarexpeditionen.md): the
// human-readable lines for an expedition's two notifications. Mirrors web's
// notifText 'ExpeditionTurnedHome' and expeditionReportText
// (web/static/js/megaron/ui/format.js) so both surfaces say the same thing.

import (
	"encoding/json"
	"fmt"
	"strings"
)

type expeditionFind struct {
	Kind  string `json:"kind"`
	Q     int    `json:"q"`
	R     int    `json:"r"`
	Name  string `json:"name"`
	Owner string `json:"owner"`
}

type expeditionBody struct {
	Name       string           `json:"name"`
	AreaQ      int              `json:"area_q"`
	AreaR      int              `json:"area_r"`
	Reason     string           `json:"reason"`
	ArriveTick *int             `json:"arrive_tick"`
	TicksOut   int              `json:"ticks_out"`
	Furthest   int              `json:"furthest"`
	HexesSeen  int              `json:"hexes_seen"`
	Finds      []expeditionFind `json:"finds"`
}

func printExpeditionLine(n notificationItem) {
	var b expeditionBody
	if err := json.Unmarshal(n.Body, &b); err != nil {
		return
	}
	if n.Kind == "ExpeditionTurnedHome" {
		fmt.Printf("      %s\n", expeditionTurnedText(b))
	} else {
		fmt.Printf("      %s\n", expeditionReportText(b))
	}
}

func expeditionSubject(b expeditionBody) string {
	if b.Name != "" {
		return b.Name
	}
	return "The expedition"
}

func expeditionTurnedText(b expeditionBody) string {
	why := map[string]string{
		"half_time":  "half its time is spent",
		"area_known": "nothing there is left unseen",
		"no_path":    "the rest of it cannot be reached",
	}[b.Reason]
	if why == "" {
		why = "its search is over"
	}
	home := ""
	if b.ArriveTick != nil {
		home = fmt.Sprintf(" — home by %s", formatDay(*b.ArriveTick, "%d"))
	}
	return fmt.Sprintf("%s turns home from the land around (%d, %d): %s%s",
		expeditionSubject(b), b.AreaQ, b.AreaR, why, home)
}

func expeditionReportText(b expeditionBody) string {
	var kinds []string
	byKind := map[string][]string{}
	var cities []string
	for _, f := range b.Finds {
		if f.Kind == "city" {
			c := f.Name
			if f.Owner != "" {
				c += " (" + f.Owner + ")"
			}
			cities = append(cities, fmt.Sprintf("%s at (%d, %d)", c, f.Q, f.R))
			continue
		}
		if _, seen := byKind[f.Kind]; !seen {
			kinds = append(kinds, f.Kind)
		}
		byKind[f.Kind] = append(byKind[f.Kind], fmt.Sprintf("(%d, %d)", f.Q, f.R))
	}
	var parts []string
	for _, k := range kinds {
		parts = append(parts, k+" at "+strings.Join(byKind[k], ", "))
	}
	if len(cities) > 0 {
		parts = append(parts, "cities: "+strings.Join(cities, ", "))
	}
	found := "nothing of value"
	if len(parts) > 0 {
		found = strings.Join(parts, "; ")
	}
	return fmt.Sprintf("%s is home after %s, %d hexes out at the furthest — saw %d hexes around (%d, %d): %s", expeditionSubject(b), formatDays(b.TicksOut, "%d"), b.Furthest, b.HexesSeen, b.AreaQ, b.AreaR, found)
}
