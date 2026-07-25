package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// rejectPositionalArgs returns a cobra.PositionalArgs validator that turns a
// stray positional argument into an actionable error instead of the silent
// drop cobra performs by default.
//
// The bug this closes: `keryx build harbour` (meant as `keryx build --type
// harbour`) used to be accepted — cobra's zero-value Args takes any number of
// positionals when a command declares none — so "harbour" was thrown away,
// buildingType stayed "", and build's "no --type → show catalogue" branch
// fired. The player got a page of catalogue output with no indication their
// command hadn't run at all: not an error, not a hint, just something
// plausible-looking. Every keryx leaf command except `actions [category]`
// (its one legitimate positional — see cmd_actions.go, which declares
// cobra.MaximumNArgs(1) and actually reads args[0]) is flag-only, and every
// one of them had this same silent-swallow bug.
//
// This matters beyond a papercut: keryx is played by LLM agents as well as
// humans (a bot fleet drives the live world), and "silently did the wrong
// thing while printing something that reads like success" is exactly the
// failure mode that burns an agent's turn and corrupts whatever it measured
// off the (wrong) output.
//
// suggestFlag names the flag — without its leading dashes — that a stray
// positional most likely meant, e.g. "type" for `build`, "recipe" for
// `craft`. The error then rewrites the attempted invocation in the flag form
// the caller almost certainly wanted. Pass "" (or use noPositionalArgs
// below) for a command with no single obvious flag to guess — inventing a
// suggestion there would be more misleading than none.
func rejectPositionalArgs(suggestFlag string) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return nil
		}
		word, shown := "argument", fmt.Sprintf("%q", args[0])
		if len(args) > 1 {
			quoted := make([]string, len(args))
			for i, a := range args {
				quoted[i] = fmt.Sprintf("%q", a)
			}
			word, shown = "arguments", strings.Join(quoted, " ")
		}
		if suggestFlag == "" {
			return fmt.Errorf("unexpected %s %s — %s takes flags, not positional arguments (see --help)",
				word, shown, cmd.CommandPath())
		}
		return fmt.Errorf("unexpected %s %s — keryx takes flags, not positional arguments.\nDid you mean:  %s --%s %s",
			word, shown, cmd.CommandPath(), suggestFlag, args[0])
	}
}

// noPositionalArgs rejects any positional argument with a plain "this command
// takes flags" message, for a command where no single flag is the obvious
// intended target — see rejectPositionalArgs' doc comment for the full why.
func noPositionalArgs() cobra.PositionalArgs {
	return rejectPositionalArgs("")
}
