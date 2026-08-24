package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
)

// capturePrint runs f with stdout redirected and returns what it wrote.
func capturePrint(t *testing.T, f func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	f()
	_ = w.Close()
	os.Stdout = orig
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("read captured output: %v", err)
	}
	return buf.String()
}

func divineNotification(t *testing.T, kind string, amount float64) notificationItem {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"type": kind, "amount": amount})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	return notificationItem{Kind: "DivinePunishment", Level: 2, Body: raw}
}

// A Wanax who loses a fifth of their garrison to divine wrath was shown
// {"type":"garrison_plague","amount":20,...} and nothing else — P4 hål 1 gave
// the notification a real amount but never a sentence, and the plan's own
// acceptance criterion was a notice that NAMES what was taken and how much.
// Found in the acceptance sweep 2026-08-24.
func TestDivinePunishmentLine_NamesWhatWasTakenAndHowMuch(t *testing.T) {
	cases := []struct {
		kind   string
		amount float64
		want   []string
	}{
		{"garrison_plague", 20, []string{"pest", "20"}},
		{"chariot_loss", 18, []string{"stridsvagnar", "18"}},
		{"ship_loss", 1, []string{"storm", "1"}},
		{"harvest_failure", 1200, []string{"spannmål"}},
	}
	for _, tc := range cases {
		out := capturePrint(t, func() {
			printDivinePunishmentLine(divineNotification(t, tc.kind, tc.amount))
		})
		if strings.TrimSpace(out) == "" {
			t.Errorf("%s: printed nothing — the player sees only raw JSON", tc.kind)
			continue
		}
		for _, want := range tc.want {
			if !strings.Contains(out, want) {
				t.Errorf("%s: line %q does not mention %q", tc.kind, strings.TrimSpace(out), want)
			}
		}
	}
}

// The server no longer announces a punishment that took nothing, but a client
// must not depend on that: old rows with amount 0 are already in the log and
// their semantics are frozen forever (CLAUDE.md §Events). Saying "a divine
// storm claimed a vessel" to a Wanax who owns no vessels is the lie either way.
func TestDivinePunishmentLine_SaysNothingWhenNothingWasTaken(t *testing.T) {
	out := capturePrint(t, func() {
		printDivinePunishmentLine(divineNotification(t, "ship_loss", 0))
	})
	if strings.TrimSpace(out) != "" {
		t.Errorf("a punishment that took nothing printed %q, want silence", strings.TrimSpace(out))
	}
}

// An unrecognised punishment type must fall through to the raw JSON the caller
// already printed, never to an invented sentence.
func TestDivinePunishmentLine_UnknownTypeInventsNothing(t *testing.T) {
	out := capturePrint(t, func() {
		printDivinePunishmentLine(divineNotification(t, "locust_swarm", 40))
	})
	if strings.TrimSpace(out) != "" {
		t.Errorf("unknown punishment type printed %q, want silence", strings.TrimSpace(out))
	}
}

// The help a new Wanax copies must use the vocabulary `recruit --list` prints.
// The aliases stay working — this is about what the CLI TEACHES, not what it
// accepts. Found in the acceptance sweep 2026-08-24: every example named a
// retired form (hoplites/chariot/ship) that --list never shows.
func TestRecruitExamplesUseCanonicalUnitNames(t *testing.T) {
	example := recruitCmd().Example
	for _, retired := range []string{"hoplites", "--unit chariot", "--unit ship"} {
		if strings.Contains(example, retired) {
			t.Errorf("recruit's examples still teach %q — use the name `recruit --list` prints", retired)
		}
	}
	for _, canonical := range []string{"spearman", "war_chariot", "galley"} {
		if !strings.Contains(example, canonical) {
			t.Errorf("recruit's examples never show the canonical %q", canonical)
		}
	}
}
