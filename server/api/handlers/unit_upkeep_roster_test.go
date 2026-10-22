package handlers

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"formatet/megaron/server/internal/clock"
	"formatet/megaron/server/internal/unit"
)

func TestUnitRosterRationsAndExplicitZeroSalary(t *testing.T) {
	for _, tc := range []struct {
		typ, status   string
		size          int
		grain, silver float64
		context       string
	}{
		{"spearman", "garrison", 100, .5, 0, "active"},
		{"spearman", "marching", 100, 1, 0, "active"},
		{"spearman", "forming", 40, .2, 0, "inactive"},
		{"elite_infantry", "garrison", 100, .6, .75, "active"},
		{"galley", "freighting", 1, .20, .2, "active"},
		{"nomadic_host", "positioned", 1, 0, 0, "none"},
	} {
		t.Run(tc.typ+"/"+tc.status, func(t *testing.T) {
			u := &unit.Unit{Type: unit.Type(tc.typ), Category: unit.CategoryOf(unit.Type(tc.typ)), Size: tc.size, Crew: unit.CrewFor(unit.Type(tc.typ)), Status: unit.Status(tc.status)}
			got := unitSummaries([]*unit.Unit{u}, 0, clock.NewTestClock(time.Unix(0, 0)), nil, "Wanax", 0, false)[0]
			if math.Abs(got.UpkeepGrainPerTick-tc.grain) > 1e-9 || got.UpkeepSilverPerTick != tc.silver || got.UpkeepContext != tc.context {
				t.Fatalf("roster upkeep %v/%v/%s, want%v/%v/%s", got.UpkeepGrainPerTick, got.UpkeepSilverPerTick, got.UpkeepContext, tc.grain, tc.silver, tc.context)
			}
			raw, err := json.Marshal(got)
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]any
			if err = json.Unmarshal(raw, &fields); err != nil {
				t.Fatal(err)
			}
			if v, ok := fields["upkeep_silver_per_tick"]; !ok || v.(float64) != tc.silver {
				t.Fatalf("salary missing/wrong in JSON: %s", raw)
			}
		})
	}
}
