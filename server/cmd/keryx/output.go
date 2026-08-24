package main

import (
	"encoding/json"
	"fmt"
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

// arrivalETA renders a server RFC3339 arrival timestamp as a legible relative
// countdown ("in 3h 20m") for a dispatch confirmation. Megaron measures time in
// game-days, so a raw UTC-nanosecond timestamp is noise to a player — this is the
// same convention the inbox/cargo/sightings views already use. Falls back to the
// raw string if it can't be parsed, so an upstream format change degrades to the
// old behaviour rather than dropping the ETA entirely.
func arrivalETA(iso string) string {
	if t, err := time.Parse(time.RFC3339, iso); err == nil {
		return "in " + countdown(t)
	}
	return iso
}
