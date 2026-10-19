package main

import (
	"os/exec"
	"testing"

	"github.com/spf13/cobra"
)

// commandAt builds a minimal "keryx <use>" tree so CommandPath() renders the
// full invocation (e.g. "keryx build"), matching what a real leaf command
// sees once registered under root in main.go.
func commandAt(use string) *cobra.Command {
	root := &cobra.Command{Use: "keryx"}
	child := &cobra.Command{Use: use}
	root.AddCommand(child)
	return child
}

func TestRejectPositionalArgs(t *testing.T) {
	t.Run("no args returns nil", func(t *testing.T) {
		cmd := commandAt("build")
		if err := rejectPositionalArgs("type")(cmd, nil); err != nil {
			t.Fatalf("got error %v, want nil", err)
		}
	})

	t.Run("one unexpected arg names it and the suggested flag", func(t *testing.T) {
		cmd := commandAt("build")
		err := rejectPositionalArgs("type")(cmd, []string{"harbour"})
		if err == nil {
			t.Fatal("got nil error, want one naming the stray argument")
		}
		got := err.Error()
		for _, want := range []string{`"harbour"`, "keryx build --type harbour"} {
			if !contains(got, want) {
				t.Errorf("error %q missing %q", got, want)
			}
		}
	})

	t.Run("several args are all named", func(t *testing.T) {
		cmd := commandAt("recruit")
		err := rejectPositionalArgs("unit")(cmd, []string{"hoplites", "extra"})
		if err == nil {
			t.Fatal("got nil error, want one naming both stray arguments")
		}
		got := err.Error()
		for _, want := range []string{`"hoplites"`, `"extra"`, "arguments", "keryx recruit --unit hoplites"} {
			if !contains(got, want) {
				t.Errorf("error %q missing %q", got, want)
			}
		}
	})

	t.Run("no suggested flag falls back to the generic form", func(t *testing.T) {
		cmd := commandAt("cities")
		err := noPositionalArgs()(cmd, []string{"foo"})
		if err == nil {
			t.Fatal("got nil error, want one rejecting the stray argument")
		}
		got := err.Error()
		for _, want := range []string{`"foo"`, "keryx cities takes flags, not positional arguments"} {
			if !contains(got, want) {
				t.Errorf("error %q missing %q", got, want)
			}
		}
		if contains(got, "Did you mean") {
			t.Errorf("generic form must not invent a flag suggestion, got %q", got)
		}
	})
}

func TestSuggestedArgumentSurvivesShellCopy(t *testing.T) {
	for _, arg := range []string{"harbour", "a report with spaces", "don't split this $(printf BAD)", "line1\nline2", ""} {
		out, err := exec.Command("sh", "-c", "printf '%s' "+commandArgument(arg)).Output()
		if err != nil || string(out) != arg {
			t.Errorf("argument %q became %q, error %v", arg, out, err)
		}
	}
}
