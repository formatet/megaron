package main

import "testing"

// TestAliasCommandsRegistered covers P9: `army` and `march` must exist as
// top-level verbs (the guesses a new Wanax/agent reaches for) alongside the
// canonical `unit list` / `unit march`, without removing or renaming those.
func TestAliasCommandsRegistered(t *testing.T) {
	t.Run("army aliases unit list", func(t *testing.T) {
		army := armyAliasCmd()
		if army.Use != "army" {
			t.Fatalf("armyAliasCmd().Use = %q, want %q", army.Use, "army")
		}
		if army.RunE == nil {
			t.Fatal("armyAliasCmd() has no RunE")
		}
	})

	t.Run("march aliases unit march with identical flags", func(t *testing.T) {
		march := marchAliasCmd()
		if march.Use != "march" {
			t.Fatalf("marchAliasCmd().Use = %q, want %q", march.Use, "march")
		}
		for _, flag := range []string{"unit", "q", "r", "stance", "intent", "name", "mode", "yes"} {
			if march.Flags().Lookup(flag) == nil {
				t.Errorf("marchAliasCmd() missing --%s flag (must match `unit march`)", flag)
			}
		}
	})

	t.Run("canonical unit subcommands untouched", func(t *testing.T) {
		u := unitCmd()
		names := map[string]bool{}
		for _, c := range u.Commands() {
			names[c.Name()] = true
		}
		for _, want := range []string{"list", "march", "sentry", "recall", "redirect", "stance", "load", "unload"} {
			if !names[want] {
				t.Errorf("unit subcommand %q missing — alias must not remove existing verbs", want)
			}
		}
	})
}
