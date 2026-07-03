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
	return fmt.Sprintf("+%.1f/tick", v)
}

// unitDisplayNames is the ONE canonical display name per unit type (DB key),
// shared by `status`, `unit list`, and recruit help so the same unit never
// shows two different names in the CLI (Fas 2g) — recruiting via `--unit
// hoplites` used to show up as the raw DB key "spearman" in `unit list` while
// `status` already called it "Hoplites". DB keys themselves are untouched.
var unitDisplayNames = map[string]string{
	"spearman":       "Hoplites",
	"war_chariot":    "War Chariot",
	"priest":         "Hiereus",
	"ship":           "Galley",
	"galley":         "Galley",
	"elite_infantry": "Agema",
	"war_galley":     "War Galley",
	"merchantman":    "Merchantman",
}

// unitDisplayName returns the canonical display name for a unit's DB type
// key, falling back to the raw key for any type not yet in the table.
func unitDisplayName(dbType string) string {
	if label, ok := unitDisplayNames[dbType]; ok {
		return label
	}
	return dbType
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
