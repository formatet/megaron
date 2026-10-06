package main

import (
	"encoding/json"
	"fmt"
	"net/url"
)

// Preview shares the order's resolved arguments, but never dispatches it.
func showMarchPreview(c *Client, path string, order map[string]any) error {
	query := url.Values{}
	for key, value := range order {
		query.Set(key, fmt.Sprint(value))
	}
	data, err := c.get(path + "?" + query.Encode())
	if err != nil {
		return err
	}
	if jsonMode {
		printRawJSON(data)
		return nil
	}
	var forecast struct {
		Available     bool   `json:"available"`
		Reason        string `json:"reason"`
		ArrivalTick   int64  `json:"arrival_tick"`
		DurationTicks int64  `json:"duration_ticks"`
	}
	if err := json.Unmarshal(data, &forecast); err != nil {
		return err
	}
	if forecast.Available {
		fmt.Printf("Estimated arrival: game day %d (journey: %d game days), if dispatched now. Conditions may change.\n", forecast.ArrivalTick, forecast.DurationTicks)
	} else {
		switch forecast.Reason {
		case "courier_required":
			fmt.Println("Arrival unavailable: a Runner must deliver this order before the march begins.")
		case "unknown_terrain":
			fmt.Println("Arrival unavailable: the route crosses unknown terrain.")
		default:
			fmt.Println("Arrival estimate unavailable.")
		}
	}
	fmt.Println("No order sent. Run again without --preview to send it.")
	return nil
}
