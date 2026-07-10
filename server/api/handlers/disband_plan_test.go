package handlers

// SB7: disband consumes garrison units from the units table (the single source
// of truth) instead of decrementing the retired settlements.* army columns.
// disbandPlan is the pure consumption arithmetic; these cases pin its edges:
// smallest-first order, whole vs partial consumption, and over-requesting.

import "testing"

func TestDisbandPlan(t *testing.T) {
	cases := []struct {
		name  string
		sizes []int
		men   int
		want  []int
	}{
		{"none requested", []int{100, 41}, 0, []int{0, 0}},
		{"no units", nil, 50, nil},
		{"exact single unit", []int{100}, 100, []int{100}},
		{"partial single unit shrinks", []int{100}, 30, []int{30}},
		{"smallest first, then partial", []int{41, 100}, 60, []int{41, 19}},
		{"whole first unit only", []int{41, 100}, 41, []int{41, 0}},
		{"over-request disbands all available", []int{41, 100}, 500, []int{41, 100}},
		{"spans multiple whole units", []int{10, 20, 30}, 30, []int{10, 20, 0}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := disbandPlan(tc.sizes, tc.men)
			if len(got) != len(tc.want) {
				t.Fatalf("disbandPlan(%v, %d) len = %d, want %d (%v)", tc.sizes, tc.men, len(got), len(tc.want), got)
			}
			total := 0
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("disbandPlan(%v, %d)[%d] = %d, want %d", tc.sizes, tc.men, i, got[i], tc.want[i])
				}
				if got[i] < 0 || got[i] > tc.sizes[i] {
					t.Errorf("disbandPlan(%v, %d)[%d] = %d out of [0,%d]", tc.sizes, tc.men, i, got[i], tc.sizes[i])
				}
				total += got[i]
			}
			// Never disband more than requested.
			if total > tc.men {
				t.Errorf("disbandPlan(%v, %d) total = %d > requested", tc.sizes, tc.men, total)
			}
		})
	}
}
