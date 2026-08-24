package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestResolveProvince_AllAddressForms verifies that a province-UUID, a settlement-UUID,
// a name (case-insensitive), and a "q,r" coordinate all resolve to the same province ID —
// the four address forms the CLI's --province flag must accept (see cmd_resolve.go).
func TestResolveProvince_AllAddressForms(t *testing.T) {
	const provID = "33333333-3333-3333-3333-333333333333"
	const settID = "44444444-4444-4444-4444-444444444444"

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/provinces") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[{"id":"` + provID + `","settlement_id":"` + settID + `",` +
				`"name":"Korinth","q":48,"r":33}]`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	c := newClient(&Config{Server: ts.URL})

	cases := []struct {
		name  string
		input string
	}{
		{"province-uuid", provID},
		{"settlement-uuid", settID},
		{"name-lowercase", "korinth"},
		{"name-exact-case", "Korinth"},
		{"coordinate", "48,33"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveProvince(c, "world-1", tc.input)
			if err != nil {
				t.Fatalf("resolveProvince(%q): unexpected error: %v", tc.input, err)
			}
			if got != provID {
				t.Errorf("resolveProvince(%q) = %q, want %q", tc.input, got, provID)
			}
		})
	}
}

// TestResolveProvince_NoMatch verifies the actionable-error voice for the two failure
// shapes: a UUID-looking value with no visible match, and a name with no visible match.
func TestResolveProvince_NoMatch(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))
	}))
	defer ts.Close()

	c := newClient(&Config{Server: ts.URL})

	t.Run("unknown-uuid", func(t *testing.T) {
		unknown := "55555555-5555-5555-5555-555555555555"
		_, err := resolveProvince(c, "world-1", unknown)
		if err == nil {
			t.Fatal("expected error for unknown UUID, got nil")
		}
		if !strings.Contains(err.Error(), "keryx settlements") {
			t.Errorf("expected actionable hint to run `keryx settlements`, got: %v", err)
		}
	})

	t.Run("unknown-name", func(t *testing.T) {
		_, err := resolveProvince(c, "world-1", "Nowhereton")
		if err == nil {
			t.Fatal("expected error for unknown name, got nil")
		}
		if !strings.Contains(err.Error(), "Nowhereton") {
			t.Errorf("expected error to name the input, got: %v", err)
		}
	})
}

// TestResolveUnitID exercises Rad H (megaron_plan_cli_sanning.md): every
// dispatch confirmation in cmd_unit.go echoes an 8-char unitID[:8], and
// pasting that fragment back into --unit used to fail with an opaque
// "invalid unit ID" (HTTP 400) instead of resolving like `place`/`staff`'s
// name-or-id flags already do. resolveUnitID closes that gap by treating
// anything shorter than a full UUID as a prefix against GET .../units.
func TestResolveUnitID(t *testing.T) {
	const fullID = "8afb6a29-2c62-4c63-82db-893ddb14a479"
	const otherID = "8afb6a29-dddd-dddd-dddd-dddddddddddd" // shares the same 8-char prefix

	unitsJSON := `{"units":[
		{"id":"` + fullID + `","type":"spearman","display_name":"1st Spearmen of Gournia"},
		{"id":"9c845d8b-7b78-437b-8cf4-fedeef26a6c7","type":"galley","display_name":"White Dolphin"}
	]}`
	ambiguousJSON := `{"units":[
		{"id":"` + fullID + `","type":"spearman","display_name":"1st Spearmen of Gournia"},
		{"id":"` + otherID + `","type":"spearman","display_name":"2nd Spearmen of Gournia"}
	]}`

	newServer := func(t *testing.T, body string) *Client {
		t.Helper()
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasSuffix(r.URL.Path, "/units") {
				t.Fatalf("unexpected request path %q", r.URL.Path)
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(body))
		}))
		t.Cleanup(ts.Close)
		return newClient(&Config{Server: ts.URL})
	}

	t.Run("exact UUID is trusted without a network call", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Fatalf("resolveUnitID made an HTTP call for an exact UUID: %s", r.URL.Path)
		}))
		defer ts.Close()
		c := newClient(&Config{Server: ts.URL})

		got, err := resolveUnitID(c, "world-1", fullID)
		if err != nil || got != fullID {
			t.Fatalf("got (%q, %v), want (%q, nil)", got, err, fullID)
		}
	})

	t.Run("unique prefix resolves, matching unit list's own truncated echo", func(t *testing.T) {
		c := newServer(t, unitsJSON)
		got, err := resolveUnitID(c, "world-1", "8afb6a29") // the exact unitID[:8] form dispatch confirmations print
		if err != nil || got != fullID {
			t.Fatalf("got (%q, %v), want (%q, nil)", got, err, fullID)
		}
	})

	t.Run("prefix match is case-insensitive", func(t *testing.T) {
		c := newServer(t, unitsJSON)
		got, err := resolveUnitID(c, "world-1", "8AFB6A29")
		if err != nil || got != fullID {
			t.Fatalf("got (%q, %v), want (%q, nil)", got, err, fullID)
		}
	})

	t.Run("no match names the input and points at unit list", func(t *testing.T) {
		c := newServer(t, unitsJSON)
		_, err := resolveUnitID(c, "world-1", "zzzzzzzz")
		if err == nil {
			t.Fatal("expected error for unmatched prefix, got nil")
		}
		if !strings.Contains(err.Error(), "zzzzzzzz") || !strings.Contains(err.Error(), "keryx unit list") {
			t.Errorf("expected error naming the input and pointing at `keryx unit list`, got: %v", err)
		}
	})

	t.Run("ambiguous prefix lists every candidate instead of guessing", func(t *testing.T) {
		c := newServer(t, ambiguousJSON)
		_, err := resolveUnitID(c, "world-1", "8afb6a29")
		if err == nil {
			t.Fatal("expected error for ambiguous prefix, got nil")
		}
		for _, want := range []string{fullID, otherID} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("expected both candidates listed, missing %q in: %v", want, err)
			}
		}
	})
}
