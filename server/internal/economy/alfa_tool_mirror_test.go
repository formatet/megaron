package economy

import (
	"os"
	"regexp"
	"strconv"
	"testing"
)

// tools/alfa_economy.py mirrors the food rate to compute matnetto outside the
// server. A mirror that drifts would make the alpha analysis lie, so it is bound
// here to the authoritative constant (megaron_plan_alfaanalys).
func TestAlfaEconomyMirrorsFoodRate(t *testing.T) {
	src, err := os.ReadFile("../../../tools/alfa_economy.py")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^FOOD_PER_CITIZEN\s*=\s*([0-9.]+)`).FindSubmatch(src)
	if m == nil {
		t.Fatal("FOOD_PER_CITIZEN not found in tools/alfa_economy.py")
	}
	got, _ := strconv.ParseFloat(string(m[1]), 64)
	if got != GrainConsumptionPerCitizenPerTick {
		t.Fatalf("alfa_economy.py FOOD_PER_CITIZEN = %v, economy says %v", got, GrainConsumptionPerCitizenPerTick)
	}
}
