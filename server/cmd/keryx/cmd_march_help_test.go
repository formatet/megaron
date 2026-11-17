package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/pflag"
)

func helpOutput(t *testing.T, args ...string) string {
	t.Helper()
	root := newRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// `march --help` is short, names every visible flag the command defines (read
// from the flag set) and points at the long form; `keryx help march` prints it.
func TestMarchHelpIsShortAndComplete(t *testing.T) {
	out := helpOutput(t, "march", "--help")
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) > 30 {
		t.Fatalf("march --help is %d lines, want <= 30", len(lines))
	}
	unitMarchCmd().Flags().VisitAll(func(f *pflag.Flag) {
		if !f.Hidden && !strings.Contains(out, "--"+f.Name) {
			t.Errorf("march --help does not name --%s", f.Name)
		}
	})
	if last := lines[len(lines)-1]; last != "Long form: keryx help march" {
		t.Fatalf("last line = %q, want the long-form pointer", last)
	}
	long := helpOutput(t, "help", "march")
	for _, want := range []string{"Conquest choice", "Exploring:", "--intent land"} {
		if !strings.Contains(long, want) {
			t.Errorf("keryx help march lost %q", want)
		}
	}
}
