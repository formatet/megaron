package main

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

// TestUnusedCatchmentDeposits reproduces the P1a soak gap (2026-07-18): `status`
// showed Copper/Tin only as a produced good after a mine already existed, so a
// player who never built one never learned their own catchment held an ore.
func TestUnusedCatchmentDeposits(t *testing.T) {
	building := func(typ string) any { return map[string]any{"type": typ, "level": 1.0} }

	tests := []struct {
		name       string
		deposits   []any
		buildings  []any
		wantUnused []string
	}{
		{
			name:       "no deposits in catchment",
			deposits:   []any{},
			buildings:  nil,
			wantUnused: nil,
		},
		{
			name:       "copper and tin present, no mine built",
			deposits:   []any{"copper", "tin"},
			buildings:  nil,
			wantUnused: []string{"copper", "tin"},
		},
		{
			name:       "copper and tin present, mine already built",
			deposits:   []any{"copper", "tin"},
			buildings:  []any{building("mine")},
			wantUnused: nil,
		},
		{
			name:       "silver present, no silver_mine built",
			deposits:   []any{"silver"},
			buildings:  []any{building("mine")}, // a plain mine does not cover silver
			wantUnused: []string{"silver"},
		},
		{
			name:       "silver present, silver_mine built",
			deposits:   []any{"silver"},
			buildings:  []any{building("silver_mine")},
			wantUnused: nil,
		},
		{
			name:       "cedar deposit is never flagged (no mine-equivalent gate)",
			deposits:   []any{"cedar"},
			buildings:  nil,
			wantUnused: nil,
		},
		{
			name:       "mixed: tin unused, silver already mined",
			deposits:   []any{"tin", "silver"},
			buildings:  []any{building("silver_mine")},
			wantUnused: []string{"tin"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := unusedCatchmentDeposits(tt.deposits, tt.buildings)
			if !reflect.DeepEqual(got, tt.wantUnused) {
				t.Errorf("unusedCatchmentDeposits() = %#v, want %#v", got, tt.wantUnused)
			}
		})
	}
}

// TestMultiCityHint reproduces the recurring soak-round misreading of
// `status`'s hidden per-settlement scope (see multiCityHint's doc comment):
// with only one settlement the hint must stay silent (no noise in the normal
// case), and with more than one it must name the current city and point at
// `keryx settlements` / `keryx status --province <id>` for the rest.
func TestMultiCityHint(t *testing.T) {
	tests := []struct {
		name           string
		settlementName string
		used           float64
		wantEmpty      bool
	}{
		{name: "single settlement stays silent", settlementName: "Mycenae", used: 1, wantEmpty: true},
		{name: "zero (missing settlement_cap) stays silent", settlementName: "Mycenae", used: 0, wantEmpty: true},
		{name: "two settlements renders the hint", settlementName: "Mycenae", used: 2, wantEmpty: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := multiCityHint(tt.settlementName, tt.used)
			if tt.wantEmpty {
				if got != "" {
					t.Errorf("multiCityHint(%q, %v) = %q, want empty", tt.settlementName, tt.used, got)
				}
				return
			}
			if got == "" {
				t.Errorf("multiCityHint(%q, %v) = empty, want non-empty hint", tt.settlementName, tt.used)
			}
			if !strings.Contains(got, tt.settlementName) {
				t.Errorf("multiCityHint(%q, %v) = %q, want it to name the settlement", tt.settlementName, tt.used, got)
			}
			if !strings.Contains(got, "keryx settlements") || !strings.Contains(got, "keryx status --province") {
				t.Errorf("multiCityHint(%q, %v) = %q, want it to point at both escape hatches", tt.settlementName, tt.used, got)
			}
		})
	}
}

// TestTimberBottleneckWarning reproduces the sondrunda 2026-07-24 runda 2 gap:
// Polydamas never allocated labor to timber and got no signal that this would
// block nearly all early building, while Antenor (25% timber, ~12k/day) hit no
// wall. The warning must fire on ~0 production and stay silent once a Wanax is
// actually producing timber, however small the rate.
func TestTimberBottleneckWarning(t *testing.T) {
	tests := []struct {
		name    string
		rate    float64
		wantHit bool
	}{
		{"zero production", 0, true},
		{"negligible production (float noise)", 0.001, true},
		{"negative net (consuming stock, not producing)", -0.5, true},
		{"small but real production", 0.5, false},
		{"healthy production (Antenor-like)", 500, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := timberBottleneckWarning(tt.rate)
			if hit := got != ""; hit != tt.wantHit {
				t.Errorf("timberBottleneckWarning(%v) = %q, wantHit %v", tt.rate, got, tt.wantHit)
			}
		})
	}
}

// TestArmyUpkeepWarning reproduces the "silver warning cries wolf" gap
// (megaron_plan_cli_sanning §A; soak 2026-07-24 + 2026-08-23, two independent
// agents): the old code chose the ⚠ line on the SIGN of the net rate alone,
// so "silver 30.5k, ~29027 tick of runway" and "silver 200, same runway
// exhausted in a handful of ticks" printed the identical warning. Talos
// grounded a colony over "stop the silver bleed" while holding 30 474 silver.
// The fix grinds on runway (stock / -net vs netUpkeepWarningRunwayTicks), not
// on sign — these are exactly the plan's acceptance pair (30k silver silent,
// 200 silver warns) plus the symmetric grain case and the stock=0 edge the
// old runway() silently dropped.
func TestArmyUpkeepWarning(t *testing.T) {
	const weakNegRate = -7.0 // soak 2026-07-24: a probe disbanded units over exactly this rate

	tests := []struct {
		name                    string
		netG, netS              float64
		grainStock, silverStock float64
		wantWarn                bool
		wantContains            []string
	}{
		{
			name: "plan's acceptance pair: 30k silver, weak negative net — silent",
			netG: 100, netS: weakNegRate, grainStock: 5000, silverStock: 30000,
			wantWarn: false,
		},
		{
			name: "plan's acceptance pair: 200 silver, same weak negative net — warns",
			netG: 100, netS: weakNegRate, grainStock: 5000, silverStock: 200,
			wantWarn:     true,
			wantContains: []string{"silver doesn't cover", "desert", "food is fine"},
		},
		{
			name: "grain graded symmetrically: 30k grain, weak negative net — silent",
			netG: weakNegRate, netS: 100, grainStock: 30000, silverStock: 5000,
			wantWarn: false,
		},
		{
			name: "grain graded symmetrically: 200 grain, same weak negative net — warns",
			netG: weakNegRate, netS: 100, grainStock: 200, silverStock: 5000,
			wantWarn:     true,
			wantContains: []string{"grain doesn't cover", "starve", "silver is fine"},
		},
		{
			name: "both critical — combined warning, not two separate lies",
			netG: weakNegRate, netS: weakNegRate, grainStock: 200, silverStock: 200,
			wantWarn:     true,
			wantContains: []string{"neither grain nor silver"},
		},
		{
			name: "positive net never warns regardless of stock",
			netG: 10, netS: 10, grainStock: 0, silverStock: 0,
			wantWarn: false,
		},
		{
			name: "already empty and falling is critical, not silently dropped (old runway() bug)",
			netG: 100, netS: weakNegRate, grainStock: 5000, silverStock: 0,
			wantWarn:     true,
			wantContains: []string{"stock 0 lasts ~0 tick"},
		},
		{
			name: "just above the threshold stays silent",
			netG: 100, netS: -1, grainStock: 5000, silverStock: netUpkeepWarningRunwayTicks + 1,
			wantWarn: false,
		},
		{
			name: "just below the threshold warns",
			netG: 100, netS: -1, grainStock: 5000, silverStock: netUpkeepWarningRunwayTicks - 1,
			wantWarn: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := armyUpkeepWarning(tt.netG, tt.netS, tt.grainStock, tt.silverStock)
			if hit := got != ""; hit != tt.wantWarn {
				t.Fatalf("armyUpkeepWarning(...) = %q, wantWarn=%v", got, tt.wantWarn)
			}
			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("armyUpkeepWarning(...) = %q, want it to contain %q", got, want)
				}
			}
		})
	}
}

// TestFoodGubbarLine reproduces the soak finding of
// megaron_plan_omfordelningsmatningen.md (Timothy 2026-09-02): five of five
// soak cities stood 5-10x overstaffed on food (7 required / 70 placed, etc.)
// and `status` already showed both numbers but never named the surplus or
// the verb — a Wanax had to subtract 70-7 by hand and had no pointer to the
// surface that lets them act. This locks: the surplus is exactly
// placed-required (not placed alone, not required alone), the surplus line
// only appears when there IS a surplus, a correctly-staffed city gets the
// plain line with no surplus text, and an insufficient catchment still gets
// the pre-existing shortage warning untouched.
func TestFoodGubbarLine(t *testing.T) {
	tests := []struct {
		name           string
		req, placed    int
		selfSufficient bool
		want           string
	}{
		{
			name: "soak finding: 7 required, 70 placed — names the 63 that could move",
			req:  7, placed: 70, selfSufficient: true,
			want: "  (7 citizens needed for food · 70 placed — 63 could move; see 'keryx city')",
		},
		{
			name: "exactly staffed — no surplus text",
			req:  7, placed: 7, selfSufficient: true,
			want: "  (7 citizens needed for food · 7 placed)",
		},
		{
			name: "understaffed but still self-sufficient (e.g. fish covers the rest) — no surplus text",
			req:  8, placed: 5, selfSufficient: true,
			want: "  (8 citizens needed for food · 5 placed)",
		},
		{
			name: "insufficient catchment — pre-existing shortage warning, never a surplus claim",
			req:  20, placed: 20, selfSufficient: false,
			want: "  ⚠ the catchment doesn't feed the population even with all 20 citizens",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := foodGubbarLine(tt.req, tt.placed, tt.selfSufficient)
			if got != tt.want {
				t.Errorf("foodGubbarLine(%d, %d, %v) = %q, want %q", tt.req, tt.placed, tt.selfSufficient, got, tt.want)
			}
		})
	}
}

// TestSitosGranaryState reproduces §5 of megaron_plan_sitos_utlosning.md: the
// granary's state text (printed under the "Sitos-magasinet" line in `status`)
// was inline in statusCmd's RunE, reachable only through a live server round
// trip. sitosGranaryState pulls out the pure decision — three coverage bands
// relative to the SAME cfg.LowDays/HighDays thresholds economy.
// EvaluateGranaryAction uses — so the three bands are unit-testable without a
// DB or HTTP server.
func TestSitosGranaryState(t *testing.T) {
	const low, high = 10.0, 30.0

	tests := []struct {
		name                 string
		coverage, total, net float64
		want                 string
	}{
		{
			name:     "under LowDays, not empty — releasing to the city",
			coverage: 5, total: 500, net: -3,
			want: "releasing food to the city",
		},
		{
			name:     "between LowDays and HighDays — resting",
			coverage: 20, total: 500, net: 1,
			want: "resting — neither storing nor releasing",
		},
		{
			name:     "over HighDays — tithing the surplus",
			coverage: 40, total: 500, net: 1,
			want: "storing away a tenth of the surplus",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sitosGranaryState(tt.coverage, low, high, tt.total, tt.net)
			if got != tt.want {
				t.Errorf("sitosGranaryState(%v, %v, %v, %v, %v) = %q, want %q",
					tt.coverage, low, high, tt.total, tt.net, got, tt.want)
			}
		})
	}
}

func strp(s string) *string { return &s }

// TestCohortLines reproduces the A16 gap: `status` printed one aggregate
// number per unit type ("Spearman 100") with no way to tell, from `status`
// alone, that it was two separately-marchable 100-cap cohorts. cohortLines is
// the pure formatting half (no HTTP) — see fetchGarrisonCohorts for the fetch
// + grouping half.
func TestCohortLines(t *testing.T) {
	t.Run("single cohort stays silent — it already equals the aggregate line", func(t *testing.T) {
		got := cohortLines([]unitRow{{ID: "11111111-1111-1111-1111-111111111111", Size: 100}})
		if got != nil {
			t.Errorf("cohortLines(1 cohort) = %v, want nil", got)
		}
	})

	t.Run("zero cohorts stays silent", func(t *testing.T) {
		if got := cohortLines(nil); got != nil {
			t.Errorf("cohortLines(nil) = %v, want nil", got)
		}
	})

	t.Run("two cohorts: full ID (pasteable into unit march), sorted largest first", func(t *testing.T) {
		cohorts := []unitRow{
			{ID: "aaaaaaaa-0000-0000-0000-000000000000", Size: 40},
			{ID: "bbbbbbbb-0000-0000-0000-000000000000", Size: 60},
		}
		got := cohortLines(cohorts)
		if len(got) != 2 {
			t.Fatalf("cohortLines(2 cohorts) = %d lines, want 2: %v", len(got), got)
		}
		if !strings.Contains(got[0], "bbbbbbbb-0000-0000-0000-000000000000") || !strings.Contains(got[0], "60") {
			t.Errorf("first line = %q, want the 60-strong cohort's full ID and size first", got[0])
		}
		if !strings.Contains(got[1], "aaaaaaaa-0000-0000-0000-000000000000") || !strings.Contains(got[1], "40") {
			t.Errorf("second line = %q, want the 40-strong cohort's full ID and size second", got[1])
		}
	})

	t.Run("named cohort (ship) shows the name alongside the ID", func(t *testing.T) {
		cohorts := []unitRow{
			{ID: "aaaaaaaa-0000-0000-0000-000000000000", Size: 1, Name: strp("Halcyon")},
			{ID: "bbbbbbbb-0000-0000-0000-000000000000", Size: 1, Name: strp("Persephone")},
		}
		got := cohortLines(cohorts)
		joined := strings.Join(got, "\n")
		if !strings.Contains(joined, "Halcyon") || !strings.Contains(joined, "Persephone") {
			t.Errorf("cohortLines(named ships) = %q, want both names present", joined)
		}
	})

	t.Run("cohort sum matches the aggregate it sits under (SUM(size), status='garrison')", func(t *testing.T) {
		cohorts := []unitRow{
			{ID: "aaaaaaaa-0000-0000-0000-000000000000", Size: 55},
			{ID: "bbbbbbbb-0000-0000-0000-000000000000", Size: 45},
		}
		sum := 0
		for _, c := range cohorts {
			sum += c.Size
		}
		if sum != 100 {
			t.Fatalf("test fixture bug: cohorts sum to %d, want 100", sum)
		}
		if got := cohortLines(cohorts); len(got) != 2 {
			t.Errorf("cohortLines() = %v, want one line per cohort", got)
		}
	})
}

// TestFetchGarrisonCohorts covers the fetch + filter + group half: it must
// scope to the given settlement, keep only status="garrison" (the same
// filter the province aggregate's SUM(size) uses), group by type, and
// degrade to nil (never error, never panic) on a bad settlement ID, a
// request failure, or unparseable JSON — mirroring fetchDevotionShare's
// contract that a read view never fails over a best-effort extra.
func TestFetchGarrisonCohorts(t *testing.T) {
	const settlementID = "22222222-2222-2222-2222-222222222222"
	const otherSettlementID = "33333333-3333-3333-3333-333333333333"

	t.Run("groups by type, scoped to settlement, garrison-only", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"units":[
				{"id":"a1","type":"spearman","status":"garrison","size":60,"settlement_id":"` + settlementID + `"},
				{"id":"a2","type":"spearman","status":"garrison","size":40,"settlement_id":"` + settlementID + `"},
				{"id":"a3","type":"spearman","status":"forming","size":30,"settlement_id":"` + settlementID + `"},
				{"id":"a4","type":"spearman","status":"garrison","size":100,"settlement_id":"` + otherSettlementID + `"},
				{"id":"a5","type":"war_chariot","status":"garrison","size":20,"settlement_id":"` + settlementID + `"},
				{"id":"a6","type":"spearman","status":"marching","size":10}
			]}`))
		}))
		defer ts.Close()

		cfg := &Config{Server: ts.URL, WorldID: "world-1"}
		c := newClient(cfg)

		got := fetchGarrisonCohorts(c, "world-1", settlementID)
		if len(got["spearman"]) != 2 {
			t.Fatalf("spearman cohorts = %d, want 2 (forming + other-settlement + no-settlement excluded): %+v", len(got["spearman"]), got["spearman"])
		}
		sum := 0
		for _, u := range got["spearman"] {
			sum += u.Size
		}
		if sum != 100 {
			t.Errorf("spearman cohort sum = %d, want 100 (matches what the aggregate line would show)", sum)
		}
		if len(got["war_chariot"]) != 1 {
			t.Errorf("war_chariot cohorts = %d, want 1", len(got["war_chariot"]))
		}
	})

	t.Run("empty settlementID degrades to nil without a request", func(t *testing.T) {
		called := false
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"units":[]}`))
		}))
		defer ts.Close()
		cfg := &Config{Server: ts.URL, WorldID: "world-1"}
		c := newClient(cfg)

		if got := fetchGarrisonCohorts(c, "world-1", ""); got != nil {
			t.Errorf("fetchGarrisonCohorts(empty settlementID) = %v, want nil", got)
		}
		if called {
			t.Error("fetchGarrisonCohorts(empty settlementID) made an HTTP request, want none")
		}
	})

	t.Run("server error degrades to nil, never errors", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer ts.Close()
		cfg := &Config{Server: ts.URL, WorldID: "world-1"}
		c := newClient(cfg)

		if got := fetchGarrisonCohorts(c, "world-1", settlementID); got != nil {
			t.Errorf("fetchGarrisonCohorts(server error) = %v, want nil", got)
		}
	})

	t.Run("unparseable JSON degrades to nil", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`not json`))
		}))
		defer ts.Close()
		cfg := &Config{Server: ts.URL, WorldID: "world-1"}
		c := newClient(cfg)

		if got := fetchGarrisonCohorts(c, "world-1", settlementID); got != nil {
			t.Errorf("fetchGarrisonCohorts(bad JSON) = %v, want nil", got)
		}
	})
}
