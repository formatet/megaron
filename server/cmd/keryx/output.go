package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strconv"
	"time"
)

// jsonMode is set by the --json flag.
var jsonMode bool

func printJSON(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func printRawJSON(data []byte) {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		fmt.Println(string(data))
		return
	}
	printJSON(v)
}

func die(msg string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+msg+"\n", args...)
	os.Exit(1)
}

func resource(v float64) string {
	if v >= 1000 {
		return fmt.Sprintf("%.1fk", v/1000)
	}
	return fmt.Sprintf("%.0f", v)
}

func rate(v float64) string {
	if v == 0 {
		return "—"
	}
	// %+.1f supplies its own sign (was "+%.1f", which mangled negative rates
	// into "+-5.3/tick" — DEL C grain-netto-märkning surfaced this).
	return fmt.Sprintf("%+.1f/day", v)
}

// World-unit formatting is shared by every Keryx duration and absolute day.
func formatDays(value any, precision ...string) string {
	number := fmt.Sprint(value)
	if len(precision) > 0 {
		number = fmt.Sprintf(precision[0], value)
	}
	qualifier := ""
	if len(precision) > 1 {
		qualifier = precision[1] + " "
	}
	n, _ := strconv.ParseFloat(number, 64)
	if n == 1 {
		return number + " " + qualifier + "day"
	}
	return number + " " + qualifier + "days"
}
func formatDay(value any, precision ...string) string {
	number := fmt.Sprint(value)
	if len(precision) > 0 {
		number = fmt.Sprintf(precision[0], value)
	}
	return "day " + number
}
func clockTime(t time.Time) string { return t.In(keryxTZ).Format("Mon Jan 2 15:04") }
func wallClock(t time.Time) string { return "≈ " + clockTime(t) }

// countdown formats the time remaining until t (e.g. a pending trade offer's
// escrow expires_at) as a short human string, for inbox/outbox display —
// without this, a pending offer's silver/goods stayed locked with no visible
// deadline (Fas 2b).
func countdown(t time.Time) string {
	remaining := time.Until(t)
	if remaining <= 0 {
		return "any moment"
	}
	return wallClock(t)
}

// keryxTZ is the timezone every keryx ETA's wall-clock support renders in —
// Sweden, never the machine's local zone and never UTC (feedback_timezone:
// a session never shows UTC). Mirrors internal/chronicle's localTZ. Falls
// back to time.Local if tzdata is missing.
var keryxTZ = func() *time.Location {
	if loc, err := time.LoadLocation("Europe/Stockholm"); err == nil {
		return loc
	}
	return time.Local
}()

// gameETA renders world days alongside the wall-clock instant. A wait with
// any time remaining rounds up. Missing world cadence falls back to clock/date.
func gameETA(c *Client, t time.Time) string {
	wall := wallClock(t)
	tickSeconds, ok := c.TickSeconds()
	if !ok {
		return countdown(t)
	}
	days := time.Until(t).Seconds() / tickSeconds
	if days <= 0 {
		return fmt.Sprintf("any moment (%s)", wall)
	}
	whole := int(math.Ceil(days))
	return fmt.Sprintf("in %s (%s)", formatDays(whole, "%d"), wall)
}

// arrivalETA parses a server RFC3339 arrival timestamp and renders it via
// gameETA. Falls back to the raw string if it can't be parsed, so an
// upstream format change degrades to the old behaviour rather than dropping
// the ETA entirely.
func arrivalETA(c *Client, iso string) string {
	t, err := time.Parse(time.RFC3339, iso)
	if err != nil {
		return iso
	}
	return gameETA(c, t)
}

// printPassageNote (megaron_plan_budet_liftar.md) prints, when present, the
// one line every dispatching command (message/reply/order) shares: the
// runner needs to cross the sea and is waiting in its own port for a ship —
// there is no other way across any more (the abstract instant crossing is
// gone, megaron_plan_ordna_passage.md slice 3b-4). resp is the raw JSON body
// of a Send/SendFromHost/Reply/march-order response — a no-op if the server
// did not set passage_status (the ordinary, unaffected land case).
func printPassageNote(resp map[string]any) {
	if status, _ := resp["passage_status"].(string); status == "awaiting_passage" {
		port, _ := resp["passage_port"].(string)
		if port != "" {
			fmt.Printf("  waiting in %s for a ship of yours to sail that way\n", port)
		} else {
			fmt.Printf("  waiting for a ship of yours to sail that way\n")
		}
	}
}

// transferArrival displays the authoritative rounded journey, never a second
// estimate from distance, weight or local wall time. Old responses retain their
// timestamp display.
func transferArrival(resp map[string]any) string {
	if arrival, ok := resp["arrival_tick"].(float64); ok {
		if duration, ok := resp["travel_ticks"].(float64); ok {
			return fmt.Sprintf("arrives %s (journey: %s)", formatDay(arrival, "%.0f"), formatDays(duration, "%.0f"))
		}
		return fmt.Sprintf("arrives %s", formatDay(arrival, "%.0f"))
	}
	if at, ok := resp["arrives_at"].(string); ok {
		return "arrives " + at
	}
	return "arrival pending"
}

func tradeArrival(c *Client, resp map[string]any, good bool) string {
	key, timestamp := "silver_arrival_tick", "silver_arrives_at"
	if good {
		key, timestamp = "goods_arrival_tick", "goods_arrives_at"
	}
	if at, ok := resp[key].(float64); ok {
		return fmt.Sprintf("%s", formatDay(at, "%.0f"))
	}
	at, _ := resp[timestamp].(string)
	return arrivalETA(c, at)
}
