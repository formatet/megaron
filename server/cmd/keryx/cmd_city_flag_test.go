package main

import (
	"fmt"
	"testing"

	"github.com/spf13/cobra"
)

// Every command that acts in one of your cities takes --city <name-or-id>,
// before or after its positionals; the older flags stay but are hidden.
var cityFlagCommands = []struct {
	path     []string
	pos      []string
	old      []string
	nameOnly bool // also accepts the bare name as its only positional
}{
	{[]string{"actions"}, nil, []string{"province"}, false},
	{[]string{"build"}, nil, []string{"province"}, false},
	{[]string{"city"}, nil, []string{"province"}, false},
	{[]string{"placements"}, nil, []string{"province"}, false},
	{[]string{"demolish"}, nil, []string{"province"}, false},
	{[]string{"status"}, nil, []string{"province"}, false},
	{[]string{"place"}, []string{"grain", "5", "+2"}, []string{"province"}, false},
	{[]string{"staff"}, []string{"shipyard", "-1"}, []string{"province"}, false},
	{[]string{"disband"}, nil, []string{"province"}, true},
	{[]string{"idle"}, nil, []string{"province"}, true},
	{[]string{"map"}, nil, []string{"province"}, true},
	{[]string{"recruit"}, nil, []string{"province"}, true},
	{[]string{"rich-offering"}, nil, []string{"province"}, true},
	{[]string{"ticklog"}, nil, []string{"province"}, true},
	{[]string{"goods"}, nil, []string{"province"}, true},
	{[]string{"rite"}, nil, []string{"province"}, false},
	{[]string{"abandon"}, nil, []string{"settlement"}, true},
	{[]string{"transfer"}, nil, []string{"province", "from"}, false},
	{[]string{"route"}, nil, []string{"from"}, false},
	{[]string{"message"}, nil, []string{"from"}, false},
	{[]string{"trade-offer"}, nil, []string{"from"}, false},
	{[]string{"messenger"}, nil, []string{"from"}, false},
	{[]string{"market-board"}, nil, nil, true},
}

func cityFlagValueAfterParse(t *testing.T, path []string, tokens []string) string {
	t.Helper()
	root := newRootCommand()
	cmd, _, err := root.Find(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.ParseFlags(tokens); err != nil {
		t.Fatalf("%v %v: %v", path, tokens, err)
	}
	if cmd.Args != nil {
		if err := cmd.Args(cmd, cmd.Flags().Args()); err != nil {
			t.Fatalf("%v %v: args rejected: %v", path, tokens, err)
		}
	}
	return cmd.Flag("city").Value.String()
}

func TestCityFlagOnEveryCityCommand(t *testing.T) {
	for _, tc := range cityFlagCommands {
		name := fmt.Sprint(tc.path)
		t.Run(name, func(t *testing.T) {
			before := append([]string{"--city", "Phaistos"}, tc.pos...)
			after := append(append([]string{}, tc.pos...), "--city", "Phaistos")
			for _, tokens := range [][]string{before, after} {
				if got := cityFlagValueAfterParse(t, tc.path, tokens); got != "Phaistos" {
					t.Errorf("%v: --city = %q, want Phaistos", tokens, got)
				}
			}
			root := newRootCommand()
			cmd, _, _ := root.Find(tc.path)
			if f := cmd.Flag("city"); f == nil || f.Hidden {
				t.Error("--city must exist and be visible")
			}
			for _, o := range tc.old {
				if f := cmd.Flag(o); f == nil || !f.Hidden {
					t.Errorf("old flag --%s must still exist and be hidden", o)
				}
			}
			if tc.nameOnly {
				if err := cmd.ParseFlags([]string{"Phaistos"}); err != nil {
					t.Fatal(err)
				}
				if err := cmd.Args(cmd, cmd.Flags().Args()); err != nil {
					t.Errorf("bare city name rejected: %v", err)
				}
			}
		})
	}
}

// The positional name and --city land in the same variable, and giving both
// different values is an error rather than a silent pick.
func TestCityPositionalWrapper(t *testing.T) {
	run := func(args ...string) (string, error) {
		var city string
		cmd := &cobra.Command{Use: "x", RunE: func(*cobra.Command, []string) error { return nil }}
		addCityFlag(cmd, &city, true)
		cmd.SetArgs(args)
		cmd.SilenceUsage, cmd.SilenceErrors = true, true
		err := cmd.Execute()
		return city, err
	}
	if got, err := run("Phaistos"); err != nil || got != "Phaistos" {
		t.Errorf("positional: %q %v", got, err)
	}
	if got, err := run("--city", "Phaistos"); err != nil || got != "Phaistos" {
		t.Errorf("flag: %q %v", got, err)
	}
	if got, err := run("Phaistos", "--city", "Phaistos"); err != nil || got != "Phaistos" {
		t.Errorf("same twice: %q %v", got, err)
	}
	if _, err := run("Phaistos", "--city", "Knossos"); err == nil {
		t.Error("two different cities must be an error")
	}
	if _, err := run("a", "b"); err == nil {
		t.Error("two positionals must be an error")
	}
}
