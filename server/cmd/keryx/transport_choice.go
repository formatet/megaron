package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

func validateTransportBy(by string) error {
	if by != "" && by != "land" && by != "sea" {
		return fmt.Errorf("--by must be land or sea")
	}
	return nil
}

type transportOption struct {
	By            string   `json:"by"`
	Available     bool     `json:"available"`
	Reason        string   `json:"reason"`
	TravelTicks   float64  `json:"travel_ticks"`
	TravelSeconds float64  `json:"travel_seconds"`
	ArrivesAt     string   `json:"arrives_at"`
	Risk          string   `json:"risk"`
	Requirements  []string `json:"requirements"`
}

func printTransportOptions(options []transportOption, selected string) {
	for _, o := range options {
		detail := o.Reason
		if o.Available {
			detail = fmt.Sprintf("%s · about %g minutes", formatDays(o.TravelTicks), o.TravelSeconds/60)
			if o.ArrivesAt != "" {
				detail += " · arrives " + o.ArrivesAt
			}
		}
		mark := ""
		if selected == o.By {
			mark = " [selected]"
		}
		fmt.Printf("%s%s: %s · %s", o.By, mark, detail, o.Risk)
		if len(o.Requirements) > 0 {
			fmt.Print(" · " + strings.Join(o.Requirements, "; "))
		}
		fmt.Println()
	}
}
func previewTransportOptions(c *Client, origin, destination, by string, quantity float64) error {
	path := fmt.Sprintf("/api/v1/worlds/%s/settlements/%s/transport-options?destination_id=%s&quantity=%g", cfg.WorldID, url.PathEscape(origin), url.QueryEscape(destination), quantity)
	data, err := c.get(path)
	if err != nil {
		return err
	}
	var p struct {
		Options  []transportOption `json:"options"`
		Selected string            `json:"selected_by"`
	}
	if err = json.Unmarshal(data, &p); err != nil {
		return err
	}
	if !jsonMode {
		selected := by
		if selected == "" {
			selected = p.Selected
		}
		printTransportOptions(p.Options, selected)
	}
	if by != "" {
		for _, o := range p.Options {
			if o.By == by {
				if !o.Available {
					return fmt.Errorf("%s route unavailable: %s", by, o.Reason)
				}
				return nil
			}
		}
		return fmt.Errorf("%s route was not offered", by)
	}
	return nil
}
