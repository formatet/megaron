package main

import (
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"strconv"
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
		marchSilverForecast
	}
	if err := json.Unmarshal(data, &forecast); err != nil {
		return err
	}
	if forecast.Available {
		fmt.Printf("Estimated arrival: %s (journey: %s), if dispatched now. Conditions may change.\n", formatDay(forecast.ArrivalTick, "%d"), formatDays(forecast.DurationTicks, "%d"))
	} else {
		switch forecast.Reason {
		case "courier_required", "redirect":
			fmt.Println("Arrival unavailable: a Runner must deliver this order before the march begins.")
		case "unknown_terrain":
			fmt.Println("Arrival unavailable: the route crosses unknown terrain.")
		default:
			fmt.Println("Arrival estimate unavailable.")
		}
	}
	if line := marchSilverLine(forecast.marchSilverForecast); line != "" {
		fmt.Println(line)
	}
	fmt.Println("No order sent. Run again without --preview to send it.")
	return nil
}

// Financial fields are independent of the arrival estimate. Pointers preserve
// compatibility with older servers and distinguish missing rates from zero.
type marchSilverForecast struct {
	SilverPayerCount  int      `json:"silver_payer_count"`
	UpkeepSilverHome  *float64 `json:"upkeep_silver_home"`
	UpkeepSilverField *float64 `json:"upkeep_silver_field"`
	SilverRunwayTicks *float64 `json:"silver_runway_ticks"`
}

func marchSilverLine(f marchSilverForecast) string {
	if f.UpkeepSilverHome == nil || f.UpkeepSilverField == nil {
		return ""
	}
	runway := "no silver drain"
	if f.SilverRunwayTicks != nil {
		prefix := "treasury lasts "
		if f.SilverPayerCount > 1 {
			prefix = "shortest treasury lasts "
		}
		runway = prefix + formatDays(math.Round(*f.SilverRunwayTicks*10)/10, "%g")
	}
	rate := func(value float64) string {
		return strconv.FormatFloat(math.Round(value*10000)/10000, 'f', -1, 64)
	}
	return fmt.Sprintf("Pay: %s → %s silver per day in the field · %s", rate(*f.UpkeepSilverHome), rate(*f.UpkeepSilverField), runway)
}
