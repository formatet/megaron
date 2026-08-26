package main

// megaron_plan_omfordelningsmatningen.md §3-4: the storage-ceiling signal
// (resourceAtStorageCeiling/stockCeilingWarning, megaron_plan_sten_stock.md
// §5) is INERT — economy.goodCap is loosened to 1_000_000 for this dev phase
// (internal/economy/recompute.go:951-953), and 0 of 2 511 samples ever
// reached it. This file replaces those tests with the new anchor: sink
// presence (staticKnownSinkGoods/knownSinkGoods) and reach
// (surplusWithoutSinkWarning). Rött-före per test: each neutralizes the
// condition under test and checks the warning does NOT fire, then checks it
// DOES fire once the condition holds.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestStaticKnownSinkGoods pins down the four code-derived sources
// (buildings + upgrades, walls, upkeep, prayer offerings) without a server —
// and, just as importantly, that goods with NO code-defined consumer today
// (fish, pottery, purple, livestock) are correctly absent. Timothy's own
// 2026-08-23 question ("why doesn't the AI stop producing fish it already
// has in excess?") is exactly the "fish has no known sink" case reproduced
// here.
func TestStaticKnownSinkGoods(t *testing.T) {
	sinks := staticKnownSinkGoods()

	for _, good := range []string{
		"timber", "stone", // base BuildingSpecs.Costs
		"cedar",  // LevelCedarCost, via LevelledSpec level 2/3
		"bronze", // WallLevelSpecs level 3
		"grain", "silver", // combat.UpkeepSpec{Grain,Silver}
		"oil", "wine", // religion.PrayerSpecs' Offering maps
	} {
		if !sinks[good] {
			t.Errorf("staticKnownSinkGoods()[%q] = false, want true", good)
		}
	}

	// Rött-före: goods with genuinely no building/wall/upkeep/prayer consumer
	// today must NOT be flagged as sunk, or the invariant (never silence a
	// good with a real sink) would be trivially satisfied by marking
	// everything as sunk.
	for _, good := range []string{"fish", "pottery", "purple", "livestock"} {
		if sinks[good] {
			t.Errorf("staticKnownSinkGoods()[%q] = true, want false (no code-defined consumer)", good)
		}
	}

	// copper/tin are ONLY consumed via the DB-seeded bronze recipe
	// (internal/economy/recipe.go) — staticKnownSinkGoods never calls the
	// server, so it must not see them. knownSinkGoods (below) adds them.
	for _, good := range []string{"copper", "tin"} {
		if sinks[good] {
			t.Errorf("staticKnownSinkGoods()[%q] = true, want false (recipe ingredients need the server)", good)
		}
	}
}

func TestKnownSinkGoods(t *testing.T) {
	t.Run("adds recipe ingredients from the server on top of the static set", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"id":1,"output_key":"bronze","output_qty":1,"building_type":"foundry",
				"ingredients":[{"good_key":"copper","quantity":2},{"good_key":"tin","quantity":1}]}]`))
		}))
		defer ts.Close()
		cfg := &Config{Server: ts.URL, WorldID: "world-1"}
		c := newClient(cfg)

		got := knownSinkGoods(c)
		for _, good := range []string{"copper", "tin", "grain", "silver"} {
			if !got[good] {
				t.Errorf("knownSinkGoods()[%q] = false, want true", good)
			}
		}
	})

	t.Run("server error degrades to the static set, never blocks status", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer ts.Close()
		cfg := &Config{Server: ts.URL, WorldID: "world-1"}
		c := newClient(cfg)

		got := knownSinkGoods(c)
		if !got["grain"] || !got["silver"] {
			t.Errorf("knownSinkGoods() on server error = %v, want the static set preserved", got)
		}
		if got["copper"] {
			t.Error("knownSinkGoods() on server error should not have recipe-derived goods")
		}
	})
}

// TestSurplusWithoutSinkWarning is the slice's rött-före: the three
// constructed cases from the brief's acceptance criteria, plus the boundary
// and wording checks the old stockCeilingWarning test carried.
func TestSurplusWithoutSinkWarning(t *testing.T) {
	t.Run("(a) real sink, short reach — silent (invariant: never fires with a real sink)", func(t *testing.T) {
		// hasKnownSink=true alone must silence it even though rate and gubbar
		// would otherwise qualify — this is the hard invariant from the brief.
		got := surplusWithoutSinkWarning("Grain", 50, 10, true, 3)
		if got != "" {
			t.Errorf("surplusWithoutSinkWarning(known sink, short reach) = %q, want \"\"", got)
		}
	})

	t.Run("(b) no sink, long reach, gubbar placed — warns with gubbe count", func(t *testing.T) {
		got := surplusWithoutSinkWarning("Timber", 54_900, 720, false, 3)
		if got == "" {
			t.Fatal("surplusWithoutSinkWarning(no sink, long reach, 3 gubbar) = \"\", want a warning")
		}
		for _, want := range []string{"Timber", "3 gubbar", "ingen känd sänka", "keryx place"} {
			if !strings.Contains(got, want) {
				t.Errorf("surplusWithoutSinkWarning() = %q, want it to contain %q", got, want)
			}
		}
	})

	t.Run("(c) no sink, long reach, zero gubbar — silent (nothing to move)", func(t *testing.T) {
		got := surplusWithoutSinkWarning("Fish", 54_900, 720, false, 0)
		if got != "" {
			t.Errorf("surplusWithoutSinkWarning(no sink, long reach, 0 gubbar) = %q, want \"\"", got)
		}
	})

	t.Run("silent with rate <= 0 regardless of sink/gubbar", func(t *testing.T) {
		if got := surplusWithoutSinkWarning("Fish", 54_900, 0, false, 3); got != "" {
			t.Errorf("surplusWithoutSinkWarning(rate=0) = %q, want \"\"", got)
		}
		if got := surplusWithoutSinkWarning("Fish", 54_900, -5, false, 3); got != "" {
			t.Errorf("surplusWithoutSinkWarning(rate<0) = %q, want \"\"", got)
		}
	})

	t.Run("singular gubbe wording", func(t *testing.T) {
		got := surplusWithoutSinkWarning("Fish", 1000, 10, false, 1)
		if !strings.Contains(got, "1 gubbe") || strings.Contains(got, "1 gubbar") {
			t.Errorf("surplusWithoutSinkWarning() = %q, want singular \"1 gubbe\"", got)
		}
	})

	t.Run("just above the runway threshold stays silent — fresh production line, not yet settled", func(t *testing.T) {
		amount := float64(netUpkeepWarningRunwayTicks - 1) // ticks < threshold
		if got := surplusWithoutSinkWarning("Fish", amount, 1, false, 2); got != "" {
			t.Errorf("surplusWithoutSinkWarning(runway just under threshold) = %q, want \"\"", got)
		}
	})

	t.Run("just below the runway threshold warns", func(t *testing.T) {
		amount := float64(netUpkeepWarningRunwayTicks + 1)
		if got := surplusWithoutSinkWarning("Fish", amount, 1, false, 2); got == "" {
			t.Error("surplusWithoutSinkWarning(runway just over threshold) = \"\", want a warning")
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
