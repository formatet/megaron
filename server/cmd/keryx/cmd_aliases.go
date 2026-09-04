package main

import "github.com/spf13/cobra"

// Top-level discoverability aliases (P9 — CLI verb/flag hygiene).
//
// A fresh player (or LLM agent) guesses the verb a wargame CLI "should" have —
// `army` to see your forces, `march` to move a unit — before ever discovering
// they live under `unit list` / `unit march`. cobra reports "unknown command"
// for the guess today with no pointer to the real one. These register the
// SAME behaviour under the guessed name too, so both work; nothing existing
// is removed or renamed (`unit list`/`unit march` keep working exactly as
// before). Each call below builds a fresh command instance (its own local
// flag variables) — a cobra.Command can only ever have one parent, so the
// underlying unitListCmd/unitMarchCmd builders are invoked twice on purpose,
// once for the `unit` subcommand tree and once here for the root alias.

// armyAliasCmd is `army` — an alias for `unit list`.
func armyAliasCmd() *cobra.Command {
	c := unitListCmd()
	c.Use = "army"
	c.Short = "List your units (alias for `unit list`)"
	return c
}

// marchAliasCmd is `march` — an alias for `unit march`. unitMarchCmd's Use is
// already "march", so this only needs a clarifying Short; the rest (flags,
// Long, Example, RunE) carries over unchanged.
func marchAliasCmd() *cobra.Command {
	c := unitMarchCmd()
	c.Short = "Order a unit to march to a hex (alias for `unit march`)"
	return c
}

// reinforceAliasCmd is `reinforce` — an alias for `unit reinforce`.
// unitReinforceCmd's Use is already "reinforce", so only the Short changes.
func reinforceAliasCmd() *cobra.Command {
	c := unitReinforceCmd()
	c.Short = "Refill a thinned land cohort (alias for `unit reinforce`)"
	return c
}

// recallAliasCmd is `recall` — an alias for `unit recall`. unitRecallCmd's
// Use is already "recall", so only the Short changes (megaron_plan_cli_sanning
// §J). `keryx actions military` lists "recall" as a bare verb name in the
// same flat list as "march" and "disband" — both real top-level commands —
// with no textual claim either way about which verbs are top-level; a Wanax
// (or agent) who has just used `keryx march`/`keryx army`/`keryx reinforce`
// at the top level has no way to tell "recall" apart and reasonably guesses
// `keryx recall` next. Same guessability gap the other aliases exist for.
func recallAliasCmd() *cobra.Command {
	c := unitRecallCmd()
	c.Short = "Recall a marching unit — turn it home (alias for `unit recall`)"
	return c
}

// redirectAliasCmd is `redirect` — an alias for `unit redirect`.
// unitRedirectCmd's Use is already "redirect", so only the Short changes.
// Same guessability gap as recallAliasCmd: a Wanax who has just used `keryx
// march`/`keryx recall` at the top level has no textual reason to expect
// `redirect` to live one level deeper than its siblings.
func redirectAliasCmd() *cobra.Command {
	c := unitRedirectCmd()
	c.Short = "Redirect a marching unit to a new hex (alias for `unit redirect`)"
	return c
}

// placementsAliasCmd is `placements` — an alias for `city`. The verbs that
// actually change placements are named `place`/`staff`, so an agent that
// wants to SEE them reasonably guesses the plural noun rather than either
// verb (megaron_plan_cli_sanning). cityCmd's Use is already "city [stad]", so
// this only needs a distinct Use/Short; the rest (Long, Example, RunE)
// carries over unchanged.
func placementsAliasCmd() *cobra.Command {
	c := cityCmd()
	c.Use = "placements [stad]"
	c.Short = "Show a settlement's catchment hexes, building slots and idle pool (alias for `city`)"
	return c
}
