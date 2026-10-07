package economy

import "testing"

func TestDefaultGoodStorageCapEconomy(t *testing.T) {
	for _, key := range []string{"grain", "fish", "livestock", "cedar", "silver", "unknown"} {
		if got := goodCap(key); got != 1_000_000 {
			t.Errorf("goodCap(%q)=%g want unchanged 1000000", key, got)
		}
	}
}
