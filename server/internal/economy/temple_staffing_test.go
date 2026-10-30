package economy

import "testing"

func TestTempleHasTwoRealWorkplacesPerLevel(t *testing.T) {
	for level := 1; level <= 3; level++ {
		if got := WorkplaceSlots("temple", level); got != []int{0, 2, 4, 8}[level] {
			t.Fatalf("temple L%d has %d places, want %d", level, got, []int{0, 2, 4, 8}[level])
		}
	}
}
