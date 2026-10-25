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
	if math.Abs(v) < 0.05 {
		return "about even"
	}
	// %+.1f supplies its own sign (was "+%.1f", which mangled negative rates
	// into "+-5.3/tick" — DEL C grain-netto-märkning surfaced this).
	return fmt.Sprintf("%+.1f per game day", v)
}

// Duration formatting uses days of the world.
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
		return number + " " + qualifier + "game day"
	}
	return number + " " + qualifier + "game days"
}

// formatDuration adds real-time guidance from this world's runtime cadence.
// No world reply means no invented wall duration.
func formatDuration(c *Client, value any, precision ...string) string {
	label := formatDays(value, precision...)
	if c == nil {
		return label
	}
	cadence, ok := c.TickSeconds()
	if !ok || math.IsNaN(cadence) || math.IsInf(cadence, 0) {
		return label
	}
	n, err := strconv.ParseFloat(fmt.Sprint(value), 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) || n < 0 {
		return label
	}
	hours := n * cadence / 3600
	quantity, unit := hours, "hour"
	if hours < 1 {
		quantity, unit = hours*60, "minute"
	}
	rounded := math.Round(quantity*100) / 100
	number := strconv.FormatFloat(rounded, 'f', -1, 64)
	if rounded == 0 && quantity > 0 {
		number = "less than 0.01"
	}
	if rounded != 1 {
		unit += "s"
	}
	return label + " · about " + number + " " + unit
}

// formatWorldTime projects absolute ticks through this invocation's world anchor.
// No anchor means no invented date and no fallback to an internal tick number.
func formatWorldTime(c *Client, value any) string {
	number, err := strconv.ParseFloat(fmt.Sprint(value), 64)
	if err != nil || math.IsNaN(number) || math.IsInf(number, 0) || number < 0 || number != math.Trunc(number) || number >= float64(math.MaxInt) {
		return "time unavailable"
	}
	if at, ok := c.WorldTickETA(int(number)); ok {
		return clockTime(at)
	}
	return "time unavailable"
}

func firstTimeClient(clients []*Client) *Client {
	if len(clients) > 0 {
		return clients[0]
	}
	return nil
}

func clockTime(t time.Time) string { return clockTimeAt(t, time.Now()) }
func clockTimeAt(t, now time.Time) string {
	t, now = t.In(keryxTZ), now.In(keryxTZ)
	date := func(v time.Time) time.Time { return time.Date(v.Year(), v.Month(), v.Day(), 12, 0, 0, 0, keryxTZ) }
	day := date(t)
	today := date(now)
	if day.Equal(today) {
		return "today " + t.Format("15:04")
	}
	if day.Equal(today.AddDate(0, 0, -1)) {
		return "yesterday " + t.Format("15:04")
	}
	if day.Equal(today.AddDate(0, 0, 1)) {
		return "tomorrow " + t.Format("15:04")
	}
	if day.After(today.AddDate(0, 0, -7)) && day.Before(today.AddDate(0, 0, 7)) {
		return t.Format("Mon 15:04")
	}
	if t.Year() != now.Year() {
		return t.Format("2 Jan 2006 15:04")
	}
	return t.Format("2 Jan 15:04")
}
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

// gameETA renders an absolute arrival timestamp as a local clock/date.
func gameETA(c *Client, t time.Time) string {
	if t.IsZero() {
		return "arrival time unavailable"
	}
	return wallClock(t)
}

// arrivalETA formats an ISO fallback when no authoritative tick is available.
func arrivalETA(c *Client, iso string) string {
	t, err := time.Parse(time.RFC3339, iso)
	if err != nil {
		return "arrival time unavailable"
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
			fmt.Printf("  waiting in %s for any ship to sail that way\n", port)
		} else {
			fmt.Printf("  waiting for any ship to sail that way\n")
		}
	}
}

// transferArrival displays the authoritative rounded journey, never a second
// estimate from distance, weight or local wall time. Old responses retain their
// timestamp display.
func transferArrival(resp map[string]any, clients ...*Client) string {
	c := firstTimeClient(clients)
	if version, _ := resp["route_version"].(float64); version == 2 {
		if at, ok := resp["arrives_at"].(string); ok {
			return "arrives " + arrivalETA(c, at) + fmt.Sprintf(" (journey: %s)", formatDuration(c, resp["travel_ticks"], "%.0f"))
		}
	}
	if arrival, ok := resp["arrival_tick"].(float64); ok {
		if duration, ok := resp["travel_ticks"].(float64); ok {
			return fmt.Sprintf("arrives %s (journey: %s)", formatWorldTime(c, arrival), formatDuration(c, duration, "%.0f"))
		}
		return fmt.Sprintf("arrives %s", formatWorldTime(c, arrival))
	}
	if at, ok := resp["arrives_at"].(string); ok {
		return "arrives " + arrivalETA(c, at)
	}
	return "arrival pending"
}

func tradeArrival(c *Client, resp map[string]any, good bool) string {
	key, timestamp := "silver_arrival_tick", "silver_arrives_at"
	if good {
		key, timestamp = "goods_arrival_tick", "goods_arrives_at"
	}
	if version, _ := resp["route_version"].(float64); version == 2 {
		if stamp, ok := resp[timestamp].(string); ok {
			return arrivalETA(c, stamp)
		}
	}
	if at, ok := resp[key].(float64); ok {
		return fmt.Sprintf("%s", formatWorldTime(c, at))
	}
	at, _ := resp[timestamp].(string)
	return arrivalETA(c, at)
}

// printSupersededOrderNote is a dispatch receipt, not proof that the new order arrived.
func printSupersededOrderNote(resp map[string]any) {
	ids, ok := resp["superseded_messenger_ids"].([]any)
	if !ok || len(ids) == 0 {
		return
	}
	if len(ids) == 1 {
		fmt.Println("  Earlier unboarded order withdrawn; its Runner is walking home.")
		return
	}
	fmt.Printf("  %d earlier unboarded orders withdrawn; their Runners are walking home.\n", len(ids))
}
