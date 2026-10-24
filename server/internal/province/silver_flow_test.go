package province

import (
	"formatet/megaron/server/internal/unit"
	"reflect"
	"testing"
)

func TestSilverFlowRecruitCosts(t *testing.T) {
	for _, tc := range []struct {
		kind string
		want map[string]float64
	}{
		{"spearman", map[string]float64{"grain": 12}},
		{"galley", map[string]float64{"timber": 30}},
		{"merchantman", map[string]float64{"cedar": 90}},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			n := 100
			if unit.CategoryOf(unit.Type(tc.kind)) == unit.CategoryNaval {
				n = unit.CrewFor(unit.Type(tc.kind))
			}
			got := map[string]float64{}
			for k, v := range UnitSpecs[tc.kind].Costs {
				got[k] = v * float64(n)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("cost=%v want=%v", got, tc.want)
			}
		})
	}
}
