package main

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// The root help is a map, not a list (order-keryx-llm-20261110 step 1).
// Hidden commands still run; they are only left out of `keryx --help`.

type helpGroup struct {
	id, title string
	names     []string
}

// helpGroups is the order and content of the root help. "Start here" is
// printed with one-line glosses, the rest as wrapped name lists.
var helpGroups = []helpGroup{
	{"start", "Start here", []string{"dossier", "actions", "city", "codex"}},
	{"build", "Build and work", []string{"build", "cancel-build", "demolish", "place", "staff", "recruit", "abandon"}},
	{"move", "Move and fight", []string{"unit", "march", "recall", "redirect", "reinforce", "build-road", "disband", "retreat-default", "occupation"}},
	{"goods", "Goods and talk", []string{"transfer", "route", "routes", "route-pause", "route-resume", "route-delete", "trade-offer", "trade-accept", "trade-decline", "trade-cancel", "message", "reply", "inbox", "outbox", "market-board", "cargo", "passage", "call-back", "runners"}},
	{"gods", "Gods", []string{"rite", "rich-offering"}},
	{"look", "Look closer", []string{"map", "hex", "army", "status", "goods", "ticklog", "cities", "sightings", "places", "roads", "storms", "notifications", "letters", "gossip"}},
	{"account", "Account", []string{"login", "join", "founding", "worlds", "report", "version", "dispatches", "watch", "agora", "password", "rise", "leave"}},
}

var startGlosses = map[string]string{
	"dossier": "everything you know — read it first each round",
	"actions": "what you can do now and what is locked",
	"city":    "one city in detail (--city <name>)",
	"codex":   "the rules",
}

// hiddenFromHelp: still working, not listed. Everything here either repeats
// part of `dossier`/`city`/`status`, is an admin tool, or is a post-MVP stub.
var hiddenFromHelp = map[string]bool{
	"me": true, "brief": true, "idle": true, "roster": true, "placements": true, "forces": true,
	"settlements": true, "diplomacy": true, "messenger": true, "god": true, "reports": true,
	"completion": true,
}

func isKingdomStub(name string) bool { return strings.HasPrefix(name, "kingdom") }

// applyHelpGroups assigns every top-level command to a cobra group or hides it.
func applyHelpGroups(root *cobra.Command) {
	byName := map[string]*helpGroup{}
	for i := range helpGroups {
		g := &helpGroups[i]
		root.AddGroup(&cobra.Group{ID: g.id, Title: g.title})
		for _, n := range g.names {
			byName[n] = g
		}
	}
	root.CompletionOptions.HiddenDefaultCmd = true
	root.SetHelpCommandGroupID("account")
	root.SetCompletionCommandGroupID("account")
	for _, c := range root.Commands() {
		name := c.Name()
		if isKingdomStub(name) || hiddenFromHelp[name] {
			c.Hidden = true
		}
		if g, ok := byName[name]; ok {
			c.GroupID = g.id
		}
	}
	defaultHelp := root.HelpFunc()
	root.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		if cmd != root {
			defaultHelp(cmd, args)
			return
		}
		writeRootHelp(root.OutOrStdout(), root)
	})
}

func writeRootHelp(w io.Writer, root *cobra.Command) {
	fmt.Fprintln(w, "Keryx — Megaron's Bronze Age grand strategy CLI.")
	fmt.Fprintln(w, "Environment: POLEIA_CONFIG selects a config file; POLEIA_SERVER and POLEIA_TOKEN override connection settings.")
	fmt.Fprintln(w, "\nUsage:\n  keryx <command> [flags]   (-j/--json prints JSON)")
	for _, g := range helpGroups {
		fmt.Fprintf(w, "\n%s:\n", g.title)
		var items []string
		for _, n := range g.names {
			c := findTopLevel(root, n)
			if c == nil || c.Hidden {
				continue
			}
			if g.id == "start" {
				fmt.Fprintf(w, "  %-8s %s\n", n, startGlosses[n])
				continue
			}
			if n == "unit" {
				var subs []string
				for _, s := range c.Commands() {
					if !s.Hidden && s.Name() != "help" {
						subs = append(subs, s.Name())
					}
				}
				sort.Strings(subs)
				n = "unit <" + strings.Join(subs, "|") + ">"
			}
			items = append(items, n)
		}
		printWrapped(w, items)
	}
	fmt.Fprintln(w, "\nEach round: keryx dossier, then keryx actions.")
	fmt.Fprintln(w, "Every command has --help with copy-paste examples.")
}

func findTopLevel(root *cobra.Command, name string) *cobra.Command {
	for _, c := range root.Commands() {
		if c.Name() == name {
			return c
		}
	}
	return nil
}

func printWrapped(w io.Writer, items []string) {
	line := " "
	for _, it := range items {
		if len(line)+len(it)+1 > 78 && len(line) > 1 {
			fmt.Fprintln(w, line)
			line = " "
		}
		line += " " + it
	}
	if len(line) > 1 {
		fmt.Fprintln(w, line)
	}
}
