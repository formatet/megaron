package main

import "testing"

func TestShipyardShareNote(t *testing.T) {
	for _, c := range []struct {
		workers, sharing int
		want             string
	}{
		{3, 3, " · 3 workers shared by 3 ships"},
		{1, 2, " · 1 worker shared by 2 ships"},
		{3, 1, ""},
		{0, 2, ""},
		{0, 0, ""},
	} {
		if got := shipyardShareNote(c.workers, c.sharing); got != c.want {
			t.Errorf("shipyardShareNote(%d,%d) = %q, want %q", c.workers, c.sharing, got, c.want)
		}
	}
}
