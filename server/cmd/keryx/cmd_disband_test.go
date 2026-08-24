package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"formatet/megaron/server/internal/unit"
)

// TestDisbandCanonicalFlags locks megaron_plan_cli_sanning §E: the flag names
// must match the canonical unit-type vocabulary (internal/unit.Type* /
// DisplayName's keys), not the retired taxonomy ("hoplites", "agema",
// "trireme" — TestRetiredNamesNeverSurface in internal/unit/naming_test.go
// forbids those from reaching a player, but that test only ever checked
// DisplayName(), never this CLI's flags, so the sweep missed disband).
// disbandRetiredFlags is unit.RetiredIdentifiers (the single shared retired-
// vocabulary source — see naming_test.go TestRetiredIdentifiersStayRetired)
// restricted to the identifiers disband ever used, plus "chariots": not a
// retired taxonomy word (it's not in DisplayName's forbidden list), but a
// pre-existing mismatch against the canonical "war_chariot" type that this
// same row fixes for the same reason — one shared root, named separately.
func disbandRetiredFlags(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, id := range unit.RetiredIdentifiers {
		if id == "hoplites" || id == "agema" || id == "trireme" {
			out = append(out, id)
		}
	}
	return append(out, "chariots")
}

func TestDisbandCanonicalFlags(t *testing.T) {
	cmd := disbandCmd()
	for _, want := range []string{"spearman", "war-chariot", "galley", "elite-infantry", "war-galley", "merchantman"} {
		if cmd.Flags().Lookup(want) == nil {
			t.Errorf("disbandCmd() missing canonical --%s flag", want)
		}
	}
	// Retired names must still exist as hidden aliases — a silent break is
	// worse than an old label, since keryx_playtest agent configs may still
	// send them — but must never appear in --help output.
	for _, retired := range disbandRetiredFlags(t) {
		f := cmd.Flags().Lookup(retired)
		if f == nil {
			t.Fatalf("disbandCmd() dropped retired --%s outright — must stay as a hidden alias", retired)
		}
		if !f.Hidden {
			t.Errorf("--%s must be Hidden (dolt alias), not surfaced in --help", retired)
		}
		if f.Deprecated == "" {
			t.Errorf("--%s must carry a Deprecated message pointing at the canonical flag", retired)
		}
	}
}

// TestDisbandExampleUsesCanonicalNames guards against the Example text
// silently reverting to the retired vocabulary the next time this file is
// touched — the exact gap that let the flag mismatch through undetected in
// the first place (§E's "Extra" instruction).
func TestDisbandExampleUsesCanonicalNames(t *testing.T) {
	cmd := disbandCmd()
	for _, retired := range disbandRetiredFlags(t) {
		if containsToken(cmd.Example, "--"+retired) {
			t.Errorf("disbandCmd().Example still uses retired flag --%s — must use the canonical name", retired)
		}
	}
	for _, canonical := range []string{"--spearman", "--war-chariot", "--galley", "--elite-infantry"} {
		if !containsToken(cmd.Example, canonical) {
			t.Errorf("disbandCmd().Example missing canonical flag %q", canonical)
		}
	}
}

func containsToken(s, tok string) bool {
	for i := 0; i+len(tok) <= len(s); i++ {
		if s[i:i+len(tok)] == tok {
			return true
		}
	}
	return false
}

// TestDisbandOldFlagsMapToCanonicalJSONKeys proves the hidden aliases are
// not just cosmetic: --hoplites/--chariots/--trireme/--agema must post the
// SAME server JSON keys as their canonical replacements (spearman/
// war_chariot/ship/elite_infantry — "ship" is the disband endpoint's
// existing wire key for the canonical "galley" unit type, api/handlers/
// province.go Disband's req struct; unchanged by this row). No live HTTP —
// a local httptest server captures the request body.
func TestDisbandOldFlagsMapToCanonicalJSONKeys(t *testing.T) {
	var gotBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"pop_restored":0}`))
	}))
	defer ts.Close()

	origCfg, origJSON := cfg, jsonMode
	cfg = &Config{Server: ts.URL, WorldID: "world-1", ProvinceID: "prov-1"}
	jsonMode = true
	t.Cleanup(func() { cfg, jsonMode = origCfg, origJSON })

	tests := []struct {
		name    string
		args    []string
		wantKey string
	}{
		{"retired --hoplites maps to spearman", []string{"--hoplites", "5"}, "spearman"},
		{"retired --chariots maps to war_chariot", []string{"--chariots", "5"}, "war_chariot"},
		{"retired --trireme maps to ship (galley)", []string{"--trireme", "5"}, "ship"},
		{"retired --agema maps to elite_infantry", []string{"--agema", "5"}, "elite_infantry"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotBody = nil
			cmd := disbandCmd()
			if err := cmd.ParseFlags(tt.args); err != nil {
				t.Fatalf("ParseFlags(%v) = %v", tt.args, err)
			}
			if err := cmd.RunE(cmd, nil); err != nil {
				t.Fatalf("RunE(%v) = %v", tt.args, err)
			}
			v, _ := gotBody[tt.wantKey].(float64)
			if v != 5 {
				t.Errorf("body[%q] = %v, want 5 (request body: %+v)", tt.wantKey, gotBody[tt.wantKey], gotBody)
			}
		})
	}
}
