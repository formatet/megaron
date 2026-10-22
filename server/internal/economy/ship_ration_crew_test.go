package economy

import (
	"math"
	"strconv"
	"testing"
)

func TestRowersEatSoldierRations(t *testing.T) {
	for _, boat := range []struct {
		kind string
		crew int
	}{{"galley", 20}, {"war_galley", 50}, {"merchantman", 10}} {
		for _, crew := range []int{boat.crew, 2, 0} {
			for _, status := range []string{"garrison", "repairing", "fitting_out", "marching", "positioned", "freighting", "laid_up"} {
				t.Run(boat.kind+"/"+status+"/"+strconv.Itoa(crew), func(t *testing.T) {
					soldierStatus := "garrison"
					if status == "marching" || status == "positioned" || status == "freighting" {
						soldierStatus = "positioned"
					}
					want := UnitUpkeep("spearman", "land", crew, soldierStatus, 0).Grain
					if status == "laid_up" {
						want = 0
					}
					got := UnitUpkeep(boat.kind, "naval", 1, status, crew)
					if math.Abs(got.Grain-want) > 1e-9 {
						t.Fatalf("crew %d: got %+v want food %g", crew, got, want)
					}
				})
			}
		}
	}
}
func TestRowersDeriveFoodFromSpearmanAndVoyageMatchesActualCrew(t *testing.T) {
	old := UpkeepSpecs["spearman"]
	defer func() { UpkeepSpecs["spearman"] = old }()
	changed := old
	changed.Grain *= 1.7
	UpkeepSpecs["spearman"] = changed
	for _, kind := range []string{"galley", "war_galley", "merchantman"} {
		want := changed.Grain * 2 / 100 * FieldGrainFactor
		got := VoyageRation(kind, 1, "", 0, 2)
		if math.Abs(got-want) > 1e-9 {
			t.Fatalf("%s crew2: %g want derived %g", kind, got, want)
		}
		laden := VoyageRation(kind, 1, "spearman", 37, 2)
		cargo := UnitUpkeep("spearman", "land", 37, "embarked", 0).Grain
		if math.Abs(laden-got-cargo) > 1e-9 {
			t.Fatalf("cargo charged incorrectly: %g vs %g", laden-got, cargo)
		}
	}
}
