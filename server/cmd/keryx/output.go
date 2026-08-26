package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
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
	return fmt.Sprintf("%+.1f/tick", v)
}

// countdown formats the time remaining until t (e.g. a pending trade offer's
// escrow expires_at) as a short human string, for inbox/outbox display —
// without this, a pending offer's silver/goods stayed locked with no visible
// deadline (Fas 2b).
func countdown(t time.Time) string {
	remaining := time.Until(t)
	if remaining <= 0 {
		return "any moment"
	}
	if remaining < time.Hour {
		return fmt.Sprintf("%dm", int(remaining.Minutes()))
	}
	if remaining < 24*time.Hour {
		return fmt.Sprintf("%dh %dm", int(remaining.Hours()), int(remaining.Minutes())%60)
	}
	return fmt.Sprintf("%dd %dh", int(remaining.Hours()/24), int(remaining.Hours())%24)
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

// gameETA renders the time remaining until t as game-days-first — Megaron's
// actual time unit, one tick = one game-day ("Ticket ÄR dygnet",
// megaron_plan_ticket_ar_dygnet) — with the Europe/Stockholm wall clock as
// secondary support in parens, e.g. "in 3 game-days (19:04 Aug 24)" (rad K,
// megaron_plan_cli_sanning.md: a game measured in speldygn was showing raw
// wall-clock countdowns instead). English throughout, including the unit —
// "game-days" not "days": every call site around this reads as English
// ("arrives", "the runner reaches it", "ready"), and at a slow tick pace
// (e.g. 60 min/tick) a bare "in 3 days" would contradict its own wall-clock
// parenthetical (which might say "tonight"). Naming the unit removes the
// contradiction and teaches the tick/wall-clock exchange rate for free.
//
// Days are rounded UP: a Wanax plans in whole game-days, and "0 game-days
// left" would read as "already here" when it isn't — except when it
// genuinely IS here or past due, where "any moment" (countdown's own word
// for the same state) is used instead of a false "days" figure.
//
// Degrades to the old wall-clock-relative countdown ("in 3h 20m (...)") when
// c can't report the server's tick cadence (TickSeconds' second return is
// false) — never drop the ETA, same discipline arrivalETA already had for an
// unparseable timestamp.
func gameETA(c *Client, t time.Time) string {
	wall := t.In(keryxTZ).Format("15:04 Jan 2")
	tickSeconds, ok := c.TickSeconds()
	if !ok {
		return fmt.Sprintf("in %s (%s)", countdown(t), wall)
	}
	days := time.Until(t).Seconds() / tickSeconds
	if days <= 0 {
		return fmt.Sprintf("any moment (%s)", wall)
	}
	return fmt.Sprintf("in %d game-days (%s)", int(math.Ceil(days)), wall)
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
