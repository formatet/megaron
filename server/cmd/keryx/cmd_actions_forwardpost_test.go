package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Tests för P7 §B (megaron_plan_utforskaren.md): `keryx actions` är den ENDA
// ytan som får LÄRA UT den framskjutna posten — har spelaren en garnisonerad
// landenhet ska den nämna strategin. Spärren är lika viktig som verbet: den
// får ALDRIG föreslå VAR (Timothys definition av upptäckt förbjuder det).

func unitsServer(t *testing.T, unitsJSON string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/units") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"units":` + unitsJSON + `}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
}

// TestForwardPostHint_GarrisonedLandUnit_MentionsPost is the positive case:
// a garrisoned land unit in the roster should make `actions` teach the
// strategy.
func TestForwardPostHint_GarrisonedLandUnit_MentionsPost(t *testing.T) {
	ts := unitsServer(t, `[{"id":"u1","category":"land","status":"garrison"}]`)
	defer ts.Close()

	c := newClient(&Config{Server: ts.URL})
	hint := forwardPostHint(c, "world-1")
	if hint == "" {
		t.Fatal("expected a hint when a garrisoned land unit exists, got none")
	}
	if !strings.Contains(hint, "unit post") {
		t.Errorf("hint doesn't point at the actual command: %q", hint)
	}
	if !strings.Contains(strings.ToLower(hint), "double") {
		t.Errorf("hint doesn't mention the field cost (kontrakt C): %q", hint)
	}
}

// TestForwardPostHint_NeverNamesAPlace is the hard barrier (Timothy's
// definition of discovery): the hint must never contain concrete map
// coordinates suggesting WHERE to post the unit — only the generic
// <q>/<r> placeholders that mirror the command's own flag names.
func TestForwardPostHint_NeverNamesAPlace(t *testing.T) {
	ts := unitsServer(t, `[{"id":"u1","category":"land","status":"garrison"}]`)
	defer ts.Close()

	c := newClient(&Config{Server: ts.URL})
	hint := forwardPostHint(c, "world-1")
	if hint == "" {
		t.Fatal("expected a hint")
	}
	// The only coordinate-shaped tokens allowed are the literal placeholders.
	if !strings.Contains(hint, "<q>") || !strings.Contains(hint, "<r>") {
		t.Errorf("hint should show the command's placeholder flags, not a concrete site: %q", hint)
	}
}

// TestForwardPostHint_NoGarrisonedLandUnit_Silent: no land unit in garrison
// (e.g. only ships, or only units already out on the map) means nothing to
// teach right now.
func TestForwardPostHint_NoGarrisonedLandUnit_Silent(t *testing.T) {
	cases := []struct {
		name  string
		units string
	}{
		{"only a ship", `[{"id":"s1","category":"naval","status":"garrison"}]`},
		{"land unit already posted", `[{"id":"u1","category":"land","status":"positioned","stance":"sentry"}]`},
		{"no units at all", `[]`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ts := unitsServer(t, tc.units)
			defer ts.Close()
			c := newClient(&Config{Server: ts.URL})
			if hint := forwardPostHint(c, "world-1"); hint != "" {
				t.Errorf("expected no hint (%s), got: %q", tc.name, hint)
			}
		})
	}
}
