package main

import (
	"bytes"
	"strings"
	"testing"
)

// Swedish words and float noise stay out of what keryx prints (order-keryx-llm step 5).
func TestNoSwedishWordsInAnyHelp(t *testing.T) {
	root := newRootCommand()
	var walk func(path []string)
	walk = func(path []string) {
		fresh := newRootCommand()
		var buf bytes.Buffer
		fresh.SetOut(&buf)
		fresh.SetErr(&buf)
		fresh.SetArgs(append(append([]string{}, path...), "--help"))
		if err := fresh.Execute(); err != nil {
			t.Fatalf("%v: %v", path, err)
		}
		low := strings.ToLower(buf.String())
		for _, bad := range []string{"gubb", "byggnad", "<stad>"} {
			if strings.Contains(low, bad) {
				t.Errorf("%v --help contains %q", path, bad)
			}
		}
		cmd, _, _ := fresh.Find(path)
		for _, s := range cmd.Commands() {
			walk(append(append([]string{}, path...), s.Name()))
		}
	}
	for _, c := range root.Commands() {
		walk([]string{c.Name()})
	}
}

func TestMarchHelpHasOneDestinationForm(t *testing.T) {
	root := newRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"march", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, gone := range []string{"Terrain passability", "river_ford", "--q ", "--r ", "--target"} {
		if strings.Contains(out, gone) {
			t.Errorf("march help still shows %q", gone)
		}
	}
	if !strings.Contains(out, "Terrain: keryx codex marching") || !strings.Contains(out, "--to") {
		t.Errorf("march help should point at the Codex and document --to:\n%s", out)
	}
	// hidden, but still working
	for _, flag := range []string{"q", "r", "target"} {
		if f := root.Commands()[0].Flag(flag); f != nil {
			_ = f
		}
	}
	cmd, _, _ := newRootCommand().Find([]string{"march"})
	for _, flag := range []string{"q", "r", "target"} {
		if f := cmd.Flag(flag); f == nil || !f.Hidden {
			t.Errorf("--%s must stay as a hidden flag", flag)
		}
	}
}

func TestToTakesQRAsAHex(t *testing.T) {
	cmd, _, _ := newRootCommand().Find([]string{"march"})
	if err := cmd.ParseFlags([]string{"--to", "5,-3"}); err != nil {
		t.Fatal(err)
	}
	q, r, name, given, err := resolveCityTarget(cmd, nil, "5,-3", "", 0, 0)
	if err != nil || !given || q != 5 || r != -3 || name != "" {
		t.Fatalf("got %d,%d %q %v %v", q, r, name, given, err)
	}
}

func TestRecruitMenIsHiddenButAccepted(t *testing.T) {
	cmd, _, _ := newRootCommand().Find([]string{"recruit"})
	if f := cmd.Flag("men"); f == nil || !f.Hidden {
		t.Fatal("--men must exist and be hidden")
	}
}

func TestDaysNeverLeakFloatNoise(t *testing.T) {
	for in, want := range map[float64]string{359.9999: "360 game days", 12: "12 game days", 2.54: "2.5 game days", 1: "1 game day", 0.30000000000000004: "0.3 game days"} {
		if got := formatDays(in); got != want {
			t.Errorf("formatDays(%v) = %q, want %q", in, got, want)
		}
		if got := formatDays(in, "%g"); got != want {
			t.Errorf("formatDays(%v, %%g) = %q, want %q", in, got, want)
		}
	}
	if got := formatDays(7.25, "%.2f"); got != "7.25 game days" {
		t.Errorf("an explicit precision is kept: %q", got)
	}
}
