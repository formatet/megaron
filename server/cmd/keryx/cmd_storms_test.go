package main

import (
	"strings"
	"testing"
)

func TestFormatStorms(t *testing.T) {
	got, err := formatStorms([]byte(`{"tick":9,"storms":[
	  {"id":"a1b2c3d4-0000-0000-0000-000000000001","name":"Skyla","tier":"live","heading":"NE","seen_tick":9,"hexes":[{"q":2,"r":0},{"q":3,"r":0},{"q":2,"r":1}]},
	  {"id":"e5f6a7b8-0000-0000-0000-000000000002","can_name":true,"tier":"remembered","seen_tick":3,"hexes":[{"q":12,"r":0},{"q":13,"r":0},{"q":12,"r":1}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Storm Skyla in sight, drifting NE: (2,0) (3,0) (2,1) [a1b2c3d4]",
		"Storm last seen on day 3: (12,0) (13,0) (12,1) (it has moved since) [e5f6a7b8] — you were first to meet it: keryx storms name e5f6a7b8 \"<name>\"",
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

func TestResolveStormID(t *testing.T) {
	list := []byte(`{"storms":[{"id":"a1b2c3d4-1"},{"id":"a1ffffff-2"},{"id":"e5f6a7b8-3"}]}`)
	if id, err := resolveStormID(list, "e5f6"); err != nil || id != "e5f6a7b8-3" {
		t.Errorf("unique prefix = %q, %v", id, err)
	}
	if _, err := resolveStormID(list, "a1"); err == nil || !strings.Contains(err.Error(), "matches 2") {
		t.Errorf("ambiguous prefix err = %v", err)
	}
	if _, err := resolveStormID(list, "zz"); err == nil {
		t.Error("unknown prefix must fail")
	}
	if _, err := resolveStormID(list, ""); err == nil {
		t.Error("empty prefix must fail")
	}
}
