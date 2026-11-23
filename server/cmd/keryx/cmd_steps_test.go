package main

import (
	"strings"
	"testing"
)

func TestRenderFirstStepsMarksDoneAndFillsWhen(t *testing.T) {
	body := `{"hidden":false,"done_count":1,"total":2,"next":"build","closing_text":"The end.","steps":[
	 {"key":"found","done":true,"text":"Find a home.","ready_tick":null},
	 {"key":"build","done":false,"text":"The Granary is ready {when}. Come back then.","ready_tick":1010}]}`
	out, err := renderFirstSteps(nil, []byte(body))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"(1/2)", "[x] found", "[ ] build", "ready soon. Come back then."} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "The end.") || strings.Contains(out, "{when}") {
		t.Errorf("closing line must wait for next==\"\", and the token must be filled:\n%s", out)
	}
}

func TestRenderFirstStepsClosingAndHidden(t *testing.T) {
	out, _ := renderFirstSteps(nil, []byte(`{"hidden":true,"done_count":4,"total":4,"next":"","closing_text":"That is the beginning.","steps":[]}`))
	if !strings.Contains(out, "That is the beginning.") || !strings.Contains(out, "steps --show") {
		t.Errorf("closing + hidden hint expected:\n%s", out)
	}
}
