package economy

import "testing"

func TestMarketHasNoWorkplacesAtAnyLevel(t *testing.T) {
	for level := 1; level <= 3; level++ {
		if got := WorkplaceSlots("market", level); got != 0 {
			t.Fatalf("market L%d still employs %d workers", level, got)
		}
	}
}
