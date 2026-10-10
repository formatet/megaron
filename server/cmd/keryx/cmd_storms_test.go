package main

import (
	"strings"
	"testing"
)

func TestFormatStorms(t *testing.T) {
	got, err := formatStorms([]byte(`{"tick":9,"storms":[
	  {"tier":"live","heading":"NE","seen_tick":9,"hexes":[{"q":2,"r":0},{"q":3,"r":0},{"q":2,"r":1}]},
	  {"tier":"remembered","seen_tick":3,"hexes":[{"q":12,"r":0},{"q":13,"r":0},{"q":12,"r":1}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Storm in sight, drifting NE: (2,0) (3,0) (2,1)",
		"Storm last seen on day 3: (12,0) (13,0) (12,1) (it has moved since)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q:\n%s", want, got)
		}
	}
	if strings.Index(got, "in sight") > strings.Index(got, "last seen") {
		t.Errorf("live storms come first:\n%s", got)
	}
	empty, _ := formatStorms([]byte(`{"tick":1,"storms":[]}`))
	if !strings.Contains(empty, "No storms known") {
		t.Errorf("empty output = %q", empty)
	}
}
