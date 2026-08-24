package main

// megaron_plan_sten_stock.md §5/§6.1 criterion 4: a generic "produktion rakt
// ned i marken" signal for ANY good at/near its storage cap with positive
// rate — not a stone specialer. These tests are the plan's own rött-före:
// each neutralizes the condition under test (amount below the 5% band, rate
// non-positive, cap unset) and checks the warning does NOT fire, then checks
// it DOES fire once the condition holds.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResourceAtStorageCeiling(t *testing.T) {
	tests := []struct {
		name              string
		amount, rate, cap float64
		want              bool
	}{
		{"well under cap, producing", 500, 10, 1_000_000, false},
		{"exactly at 95% band, producing", 950_000, 10, 1_000_000, true},
		{"just under the 5% band (94.9%)", 949_000, 10, 1_000_000, false},
		{"at cap, producing", 1_000_000, 10, 1_000_000, true},
		{"at cap, zero rate — nothing being wasted right now", 1_000_000, 0, 1_000_000, false},
		{"at cap, negative rate — draining, not overflowing", 1_000_000, -5, 1_000_000, false},
		{"at cap, cap unset (0) — no real ceiling to hit", 1_000_000, 10, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resourceAtStorageCeiling(tt.amount, tt.rate, tt.cap); got != tt.want {
				t.Errorf("resourceAtStorageCeiling(%v,%v,%v) = %v, want %v",
					tt.amount, tt.rate, tt.cap, got, tt.want)
			}
		})
	}
}

func TestStockCeilingWarning(t *testing.T) {
	t.Run("fires with the right gubbe count when at ceiling", func(t *testing.T) {
		got := stockCeilingWarning("Stone", 1_000_000, 48, 1_000_000, 2)
		if got == "" {
			t.Fatal("stockCeilingWarning at cap with positive rate returned \"\", want a warning line")
		}
		for _, want := range []string{"Stone", "2 gubbar", "lagertaket", "keryx place"} {
			if !strings.Contains(got, want) {
				t.Errorf("stockCeilingWarning() = %q, want it to contain %q", got, want)
			}
		}
	})

	t.Run("singular gubbe wording", func(t *testing.T) {
		got := stockCeilingWarning("Wine", 1_000_000, 5, 1_000_000, 1)
		if !strings.Contains(got, "1 gubbe") || strings.Contains(got, "1 gubbar") {
			t.Errorf("stockCeilingWarning() = %q, want singular \"1 gubbe\"", got)
		}
	})

	// Rött-före: neutralize the ceiling condition (amount far below cap) and
	// confirm the warning does NOT fire — proves the test isn't vacuously true.
	t.Run("silent well under cap", func(t *testing.T) {
		if got := stockCeilingWarning("Stone", 100, 48, 1_000_000, 2); got != "" {
			t.Errorf("stockCeilingWarning() = %q, want \"\" (nowhere near the cap)", got)
		}
	})

	t.Run("silent at cap with zero rate", func(t *testing.T) {
		if got := stockCeilingWarning("Stone", 1_000_000, 0, 1_000_000, 2); got != "" {
			t.Errorf("stockCeilingWarning() = %q, want \"\" (nothing flowing in, nothing wasted)", got)
		}
	})

	t.Run("still fires with zero gubbar (unconditional trickle)", func(t *testing.T) {
		got := stockCeilingWarning("Timber", 1_000_000, 3, 1_000_000, 0)
		if got == "" {
			t.Fatal("stockCeilingWarning() = \"\", want a warning even with 0 placed gubbar (trickle overflow is still real)")
		}
		if !strings.Contains(got, "0 gubbar") {
			t.Errorf("stockCeilingWarning() = %q, want it to name 0 gubbar", got)
		}
	})
}

func TestCapitalize(t *testing.T) {
	tests := []struct{ in, want string }{
		{"stone", "Stone"}, {"wine", "Wine"}, {"", ""}, {"oil", "Oil"},
	}
	for _, tt := range tests {
		if got := capitalize(tt.in); got != tt.want {
			t.Errorf("capitalize(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestFetchGubbeCountsByGood(t *testing.T) {
	t.Run("tallies placements by good_key", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"placements":[
				{"gubbe_ordinal":1,"target_kind":"building","building_type":"stonequarry","good_key":"stone"},
				{"gubbe_ordinal":2,"target_kind":"building","building_type":"stonequarry","good_key":"stone"},
				{"gubbe_ordinal":3,"target_kind":"hex","hex_q":1,"hex_r":0,"good_key":"grain"}
			],"total_gubbar":5,"pool_size":2}`))
		}))
		defer ts.Close()
		cfg := &Config{Server: ts.URL, WorldID: "world-1"}
		c := newClient(cfg)

		got := fetchGubbeCountsByGood(c, "world-1", "prov-1")
		if got["stone"] != 2 {
			t.Errorf("gubbe count for stone = %d, want 2", got["stone"])
		}
		if got["grain"] != 1 {
			t.Errorf("gubbe count for grain = %d, want 1", got["grain"])
		}
	})

	t.Run("empty provinceID degrades to nil without a request", func(t *testing.T) {
		called := false
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"placements":[]}`))
		}))
		defer ts.Close()
		cfg := &Config{Server: ts.URL, WorldID: "world-1"}
		c := newClient(cfg)

		if got := fetchGubbeCountsByGood(c, "world-1", ""); got != nil {
			t.Errorf("fetchGubbeCountsByGood(empty provinceID) = %v, want nil", got)
		}
		if called {
			t.Error("fetchGubbeCountsByGood(empty provinceID) made an HTTP request, want none")
		}
	})

	t.Run("server error degrades to nil, never errors", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer ts.Close()
		cfg := &Config{Server: ts.URL, WorldID: "world-1"}
		c := newClient(cfg)

		if got := fetchGubbeCountsByGood(c, "world-1", "prov-1"); got != nil {
			t.Errorf("fetchGubbeCountsByGood(server error) = %v, want nil", got)
		}
	})
}

