package economy

import (
	"math"
	"testing"
)

func TestSoldierWagesUseActualMenAndHomeHalf(t *testing.T) {
	for _, tc := range []struct {
		typ, category string
		size, crew    int
		field         float64
	}{
		{"spearman", "land", 100, 0, 0}, {"elite_infantry", "land", 100, 0, 1.5}, {"war_chariot", "land", 50, 0, 1},
		{"galley", "naval", 1, 20, .2}, {"war_galley", "naval", 1, 7, .07}, {"merchantman", "naval", 1, 0, 0},
	} {
		for _, status := range []string{"garrison", "repairing", "fitting_out", "marching", "positioned", "freighting", "embarked", "forming", "laid_up"} {
			if tc.category == "naval" && status == "embarked" || tc.category == "land" && status == "freighting" {
				continue
			}
			want := tc.field
			if status == "garrison" || status == "repairing" || status == "fitting_out" || status == "forming" {
				want *= .5
			}
			if status == "laid_up" {
				want = 0
			}
			got := UnitUpkeep(tc.typ, tc.category, tc.size, status, tc.crew).Silver
			if math.Abs(got-want) > 1e-9 {
				t.Errorf("%s size%d crew%d %s pay=%v want%v", tc.typ, tc.size, tc.crew, status, got, want)
			}
		}
	}
}
