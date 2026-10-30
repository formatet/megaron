package main

import (
	"fmt"
	"strings"
	"time"
)

// foreignCityLines renders another ruler's city as the server whitelisted it
// (megaron_kontrakt_frammande_stad): what you see, when, and from whom — never
// a number the server did not send. now is passed for tests.
func foreignCityLines(sett map[string]any, now time.Time) []string {
	str := func(m map[string]any, k string) string { v, _ := m[k].(string); return v }
	when := func(raw string) string {
		at, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return raw
		}
		return clockTimeAt(at, now)
	}
	intel, _ := sett["intel"].(map[string]any)
	head := str(sett, "name")
	if owner := str(sett, "owner_name"); owner != "" {
		head += " — " + owner + "'s city"
	}
	lines := []string{head}

	var seen []string
	if s := str(sett, "size"); s != "" {
		seen = append(seen, s)
	}
	if w, ok := sett["walls"].(float64); ok {
		if w > 0 {
			seen = append(seen, "walls "+strings.Repeat("▮", int(w)))
		} else {
			seen = append(seen, "no walls")
		}
	}
	if b, _ := sett["besieged"].(bool); b {
		seen = append(seen, "under siege")
	}
	if names := buildingNames(sett["buildings"], false); names != "" {
		seen = append(seen, names)
	}
	if at := str(intel, "seen_at"); at != "" {
		label := "Seen " + when(at)
		if live, _ := intel["seen_live"].(bool); live {
			label = "Seen now"
		}
		lines = append(lines, label+": "+strings.Join(seen, ", ")+".")
	} else if len(seen) > 0 {
		lines = append(lines, "Known: "+strings.Join(seen, ", ")+".")
	}

	if c, ok := intel["contact"].(map[string]any); ok {
		var parts []string
		if s := str(c, "size"); s != "" {
			parts = append(parts, s)
		}
		if f := str(c, "food"); f != "" {
			parts = append(parts, f)
		}
		if goods, ok := c["goods"].([]any); ok {
			for _, g := range goods {
				if gm, ok := g.(map[string]any); ok {
					parts = append(parts, str(gm, "word")+" "+str(gm, "good"))
				}
			}
		}
		line := contactSourceLine(str(intel, "contact_source")) + " " + when(str(intel, "contact_at")) + ": " + strings.Join(parts, ", ") + "."
		lines = append(lines, line)
		if names := buildingNames(c["buildings"], true); names != "" {
			lines = append(lines, "  Buildings then: "+names+".")
		}
	} else {
		lines = append(lines, "No one of yours has been inside. Send a messenger, a caravan or a gift to learn more.")
	}
	if b, ok := sett["market_board"].(map[string]any); ok {
		lines = append(lines, "Market board: "+describeReading(b))
	}
	return lines
}

func contactSourceLine(source string) string {
	switch source {
	case "messenger":
		return "Your messenger was there"
	case "caravan":
		return "Your caravan was there"
	case "trade":
		return "Your traders were there"
	case "market_board":
		return "Your people read its market board"
	}
	return "Your people were there"
}

func buildingNames(raw any, withLevel bool) string {
	list, _ := raw.([]any)
	var out []string
	for _, b := range list {
		m, _ := b.(map[string]any)
		name, _ := m["name"].(string)
		if lvl, ok := m["level"].(float64); withLevel && ok {
			name = fmt.Sprintf("%s L%.0f", name, lvl)
		}
		out = append(out, name)
	}
	return strings.Join(out, ", ")
}
