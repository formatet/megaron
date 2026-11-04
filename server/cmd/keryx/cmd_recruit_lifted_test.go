package main

import "testing"

func TestWorkersLiftedLine(t *testing.T) {
	if got := workersLiftedLine([]byte(`{"pop_drawn":100}`)); got != "" {
		t.Errorf("no lift: %q", got)
	}
	data := []byte(`{"workers_lifted":[{"ordinal":3}],"workers_lifted_text":"A citizen left their work in Gournia: 1 on grain at (30, 45) (men were drafted)."}`)
	if got := workersLiftedLine(data); got != "A citizen left their work in Gournia: 1 on grain at (30, 45) (men were drafted)." {
		t.Errorf("lift: %q", got)
	}
}
