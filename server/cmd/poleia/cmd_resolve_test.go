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
		if !strings.Contains(err.Error(), "poleia settlements") {
			t.Errorf("expected actionable hint to run `poleia settlements`, got: %v", err)
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
