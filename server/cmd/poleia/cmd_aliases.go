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
