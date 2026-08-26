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

	t.Run("recall aliases unit recall with identical flags", func(t *testing.T) {
		recall := recallAliasCmd()
		if recall.Use != "recall" {
			t.Fatalf("recallAliasCmd().Use = %q, want %q", recall.Use, "recall")
		}
		if recall.RunE == nil {
			t.Fatal("recallAliasCmd() has no RunE")
		}
		if recall.Flags().Lookup("unit") == nil {
			t.Error("recallAliasCmd() missing --unit flag (must match `unit recall`)")
		}
	})

	t.Run("canonical unit subcommands untouched", func(t *testing.T) {
		u := unitCmd()
		names := map[string]bool{}
		for _, c := range u.Commands() {
			names[c.Name()] = true
		}
		// "sentry" was renamed to "patrol" 2026-08-26 (megaron_plan_cli_sanning):
		// the same word named two different orders depending on which flag
		// carried it — a land STANCE you hold vs. this naval march INTENT that
		// patrols and returns home. Checked separately below via cobra's own
		// alias resolution, not by Name() (an alias never appears there).
		for _, want := range []string{"list", "march", "patrol", "recall", "redirect", "stance", "load", "unload"} {
			if !names[want] {
				t.Errorf("unit subcommand %q missing — alias must not remove existing verbs", want)
			}
		}
	})

	t.Run("unit sentry still resolves as a deprecated alias for unit patrol", func(t *testing.T) {
		u := unitCmd()
		found, _, err := u.Find([]string{"sentry"})
		if err != nil {
			t.Fatalf("`unit sentry` no longer resolves: %v", err)
		}
		if found.Name() != "patrol" {
			t.Errorf("`unit sentry` resolved to %q, want the \"patrol\" command", found.Name())
		}
	})

	t.Run("placements aliases city", func(t *testing.T) {
		placements := placementsAliasCmd()
		if placements.Name() != "placements" {
			t.Fatalf("placementsAliasCmd().Name() = %q, want %q", placements.Name(), "placements")
		}
		if placements.RunE == nil {
			t.Fatal("placementsAliasCmd() has no RunE")
		}
	})
}
