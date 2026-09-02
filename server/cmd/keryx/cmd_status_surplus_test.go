package main

// megaron_plan_omfordelningsmatningen.md §3-4, corrected 2026-08-26: the
// first pass at this slice used a present/absent sink check
// (staticKnownSinkGoods/knownSinkGoods) that Timothy MEASURED against the
// live building/upkeep/prayer catalogue and found broken two ways at once —
// timber has a real sink (buildings) that is real but numerically tiny next
// to its production rate, so presence alone could never fire on the plan's
// own motivating example; fish and livestock had NO listed sink at all even
// though the population eating them is the single biggest sink in the game.
// This file replaces the boolean tests with sized ones
// (remainingBuildingCosts/sinkCapacities) and adds the two rött-före cases
// the coordinator named as the slice's actual reason to exist: (d) fish with
// a normal population must stay silent, (e) timber with a stock far beyond
// every remaining building cost must warn.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"formatet/megaron/server/internal/province"
)

// allBuildingsMaxed is a test fixture: every building at its maximum level
// (BuildingWall excluded — it is priced through WallLevelSpecs, not
// BuildingSpecs, see remainingBuildingCosts' own doc comment).
func allBuildingsMaxed() map[string]int {
	maxed := map[string]int{}
	for bt := range province.BuildingSpecs {
		if bt == province.BuildingWall {
			continue
		}
		lvl := 1
		if province.LevelledBuildings[bt] {
			lvl = province.MaxBuildingLevel
		}
		maxed[string(bt)] = lvl
	}
	return maxed
}

func TestRemainingBuildingCosts(t *testing.T) {
	t.Run("nothing built yet — full cost for everything, wall included", func(t *testing.T) {
		got := remainingBuildingCosts(map[string]int{}, 0)
		if got["timber"] <= 0 {
			t.Errorf("remainingBuildingCosts()[timber] = %v, want > 0 (nothing built)", got["timber"])
		}
		if got["bronze"] <= 0 {
			t.Errorf("remainingBuildingCosts()[bronze] = %v, want > 0 (wall level 3 costs bronze)", got["bronze"])
		}
	})

	t.Run("everything already at max level — nothing left to build", func(t *testing.T) {
		got := remainingBuildingCosts(allBuildingsMaxed(), 3)
		for good, amt := range got {
			if amt != 0 {
				t.Errorf("remainingBuildingCosts(maxed, wall=3)[%s] = %v, want 0", good, amt)
			}
		}
	})

	t.Run("wall priced only through WallLevelSpecs — not double-counted via BuildingSpecs[wall]", func(t *testing.T) {
		got := remainingBuildingCosts(allBuildingsMaxed(), 0)
		want := map[string]float64{}
		for _, spec := range province.WallLevelSpecs {
			for good, amt := range spec.Costs {
				want[good] += amt
			}
		}
		for good, wantAmt := range want {
			if got[good] != wantAmt {
				t.Errorf("remainingBuildingCosts(maxed, wall=0)[%s] = %v, want exactly WallLevelSpecs' %v (a BuildingSpecs[wall] double count would inflate this)",
					good, got[good], wantAmt)
			}
		}
	})
}

func TestSinkCapacities(t *testing.T) {
	t.Run("food demand scales with population and covers grain/fish/livestock", func(t *testing.T) {
		got := sinkCapacities(sinkContext{population: 1000, buildingLevels: allBuildingsMaxed(), wallLevel: 3})
		wantDemand := 1000.0 * grainConsumptionPerCitizenPerTick * productionHorizonTicks
		if got["fish"] != wantDemand {
			t.Errorf("sinkCapacities()[fish] = %v, want the population food pool %v (nothing built costs fish)", got["fish"], wantDemand)
		}
		if got["grain"] != wantDemand {
			t.Errorf("sinkCapacities()[grain] = %v, want %v (no army upkeep in this case)", got["grain"], wantDemand)
		}
		wantLivestock := wantDemand / livestockFoodValue
		if got["livestock"] != wantLivestock {
			t.Errorf("sinkCapacities()[livestock] = %v, want %v (food pool converted to animal-equivalent)", got["livestock"], wantLivestock)
		}
	})

	t.Run("army upkeep adds to grain/silver over the horizon", func(t *testing.T) {
		got := sinkCapacities(sinkContext{buildingLevels: allBuildingsMaxed(), wallLevel: 3, armyUpkeepGrain: 10, armyUpkeepSilver: 5})
		if want := 10.0 * productionHorizonTicks; got["grain"] != want {
			t.Errorf("sinkCapacities()[grain] = %v, want %v (upkeep × horizon, zero population)", got["grain"], want)
		}
		if want := 5.0 * productionHorizonTicks; got["silver"] != want {
			t.Errorf("sinkCapacities()[silver] = %v, want %v (upkeep × horizon)", got["silver"], want)
		}
	})

	t.Run("temple maintenance adds to oil/wine over the horizon", func(t *testing.T) {
		got := sinkCapacities(sinkContext{buildingLevels: allBuildingsMaxed(), wallLevel: 3, templeOilPerTick: 2, templeWinePerTick: 1})
		if want := 2.0 * productionHorizonTicks; got["oil"] != want {
			t.Errorf("sinkCapacities()[oil] = %v, want %v (kharis oil/wine finding, sized)", got["oil"], want)
		}
		if want := 1.0 * productionHorizonTicks; got["wine"] != want {
			t.Errorf("sinkCapacities()[wine] = %v, want %v", got["wine"], want)
		}
	})

	t.Run("a good with no building/upkeep/food/temple source has zero capacity", func(t *testing.T) {
		got := sinkCapacities(sinkContext{population: 1000, buildingLevels: allBuildingsMaxed(), wallLevel: 3})
		if got["purple"] != 0 {
			t.Errorf("sinkCapacities()[purple] = %v, want 0 (no known consumer at all)", got["purple"])
		}
	})
}

func TestOpenEndedSinkGoods(t *testing.T) {
	t.Run("marks recipe ingredients as open-ended", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"id":1,"output_key":"bronze","output_qty":1,"building_type":"foundry",
				"ingredients":[{"good_key":"copper","quantity":2},{"good_key":"tin","quantity":1}]}]`))
		}))
		defer ts.Close()
		cfg := &Config{Server: ts.URL, WorldID: "world-1"}
		c := newClient(cfg)

		got := openEndedSinkGoods(c)
		if !got["copper"] || !got["tin"] {
			t.Errorf("openEndedSinkGoods() = %v, want copper and tin", got)
		}
	})

	t.Run("server error degrades to empty, never blocks status", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer ts.Close()
		cfg := &Config{Server: ts.URL, WorldID: "world-1"}
		c := newClient(cfg)

		if got := openEndedSinkGoods(c); len(got) != 0 {
			t.Errorf("openEndedSinkGoods() on server error = %v, want empty", got)
		}
	})
}

// TestSurplusWithoutSinkWarning is the slice's rött-före. (a)-(c) are the
// original three acceptance cases, now expressed as capacity rather than a
// boolean; (d) and (e) are the two the coordinator added after measuring —
// the actual reason this slice exists.
func TestSurplusWithoutSinkWarning(t *testing.T) {
	t.Run("(a) real sink, stock within capacity — silent (invariant: never fires within a real sink's reach)", func(t *testing.T) {
		got := surplusWithoutSinkWarning("Grain", 50, 10, 600, 3) // capacity 600 > stock 50
		if got != "" {
			t.Errorf("surplusWithoutSinkWarning(within capacity) = %q, want \"\"", got)
		}
	})

	t.Run("(b) zero capacity, large stock, citizens placed — warns with citizen count", func(t *testing.T) {
		got := surplusWithoutSinkWarning("Purple", 10_000, 5, 0, 3)
		if got == "" {
			t.Fatal("surplusWithoutSinkWarning(zero capacity, stock 10000) = \"\", want a warning")
		}
		for _, want := range []string{"Purple", "3 citizens", "known sinks", "keryx place"} {
			if !strings.Contains(got, want) {
				t.Errorf("surplusWithoutSinkWarning() = %q, want it to contain %q", got, want)
			}
		}
	})

	t.Run("(c) zero capacity, large stock, zero citizens — silent (nothing to move)", func(t *testing.T) {
		got := surplusWithoutSinkWarning("Purple", 10_000, 5, 0, 0)
		if got != "" {
			t.Errorf("surplusWithoutSinkWarning(zero citizens) = %q, want \"\"", got)
		}
	})

	t.Run("(d) fish, large stock but a normal population — silent (Timothy 2026-08-23's fish question)", func(t *testing.T) {
		pop := 2000.0
		capacities := sinkCapacities(sinkContext{population: pop, buildingLevels: allBuildingsMaxed(), wallLevel: 3})
		fishStock := capacities["fish"] * 0.5 // comfortably inside what the population could eat
		got := surplusWithoutSinkWarning("Fish", fishStock, 50, capacities["fish"], 3)
		if got != "" {
			t.Errorf("surplusWithoutSinkWarning(fish, normal population) = %q, want \"\" — this is exactly the case a boolean sink check got wrong", got)
		}
	})

	t.Run("(e) timber stock far beyond every remaining building cost — warns (the plan's own motivating example)", func(t *testing.T) {
		// Nothing built is the WORST case for capacity (maximum possible
		// remaining cost — see TestRemainingBuildingCosts) and the real
		// example (54 900 timber, +720/tick) still dwarfs it.
		capacities := sinkCapacities(sinkContext{buildingLevels: map[string]int{}, wallLevel: 0})
		got := surplusWithoutSinkWarning("Timber", 54_900, 720, capacities["timber"], 3)
		if got == "" {
			t.Fatal("surplusWithoutSinkWarning(timber, 54900, capacity from an empty settlement) = \"\", want a warning — a boolean sink check could never fire here")
		}
	})

	t.Run("silent with rate <= 0 regardless of capacity/citizens", func(t *testing.T) {
		if got := surplusWithoutSinkWarning("Fish", 54_900, 0, 0, 3); got != "" {
			t.Errorf("surplusWithoutSinkWarning(rate=0) = %q, want \"\"", got)
		}
		if got := surplusWithoutSinkWarning("Fish", 54_900, -5, 0, 3); got != "" {
			t.Errorf("surplusWithoutSinkWarning(rate<0) = %q, want \"\"", got)
		}
	})

	t.Run("singular citizen wording", func(t *testing.T) {
		got := surplusWithoutSinkWarning("Fish", 1000, 10, 0, 1)
		if !strings.Contains(got, "1 citizen") || strings.Contains(got, "1 citizens") {
			t.Errorf("surplusWithoutSinkWarning() = %q, want singular \"1 citizen\"", got)
		}
	})

	t.Run("boundary: exactly at capacity stays silent, one over warns", func(t *testing.T) {
		if got := surplusWithoutSinkWarning("Fish", 100, 5, 100, 2); got != "" {
			t.Errorf("surplusWithoutSinkWarning(amount == capacity) = %q, want \"\" (capacity can still absorb it)", got)
		}
		if got := surplusWithoutSinkWarning("Fish", 101, 5, 100, 2); got == "" {
			t.Error("surplusWithoutSinkWarning(amount just over capacity) = \"\", want a warning")
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
