package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// actionVerb mirrors internal/capabilities.Verb — kept as a separate CLI-side
// type (not shared with the server module) so the client only depends on the
// wire JSON shape, matching the convention every other cmd_*.go file follows.
type actionVerb struct {
	Name         string              `json:"name"`
	Category     string              `json:"category"`
	Purpose      string              `json:"purpose"`
	Available    bool                `json:"available"`
	Requirements []actionRequirement `json:"requirements"`
}

type actionRequirement struct {
	Text      string `json:"text"`
	Satisfied bool   `json:"satisfied"`
	Detail    string `json:"detail"`
	Hint      string `json:"hint"`
}

// categoryOrder is the fixed display order for the six locked categories
// (temenos_capabilities.md — "Kategori-taxonomi"). Keep in sync with
// internal/capabilities/registry.go.
var categoryOrder = []string{"province", "military", "trade", "diplomacy", "kingdom", "cult"}

func actionsCmd() *cobra.Command {
	var provinceID string

	cmd := &cobra.Command{
		Use:   "actions [category]",
		Short: "Show what you can do right now — and what's missing for what you can't (--province for a colony)",
		Long: `Server-authoritative capabilities surface: every mutating verb, whether it's
available right now, and for locked verbs exactly what live gap blocks it and
how to close it.

Progressive disclosure:
  keryx actions              category overview with available/locked counts
  keryx actions <category>   drill into one category's verbs, grouped
                               Available now / Locked — with requirement,
                               live gap, and how to unlock it
  keryx actions --json       the full raw array (all verbs, all categories)

Categories: province, military, trade, diplomacy, kingdom, cult`,
		Example: `  keryx actions
  keryx actions military
  keryx actions military --province <province-id>
  keryx actions --json`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := newClient(cfg)
			prov := cfg.ProvinceID
			if provinceID != "" {
				resolved, err := resolveProvince(c, cfg.WorldID, provinceID)
				if err != nil {
					return err
				}
				prov = resolved
			}
			// Founder phase: no province exists yet, and the server's /actions is
			// settlement-scoped (403s at the ownership gate before any verb check) —
			// so the host's verbs are surfaced client-side here instead. Everything
			// in temenos must be visible AND actionable in keryx.
			if prov == "" {
				return printFounderActions(c)
			}
			path := fmt.Sprintf("/api/v1/worlds/%s/provinces/%s/actions", cfg.WorldID, prov)
			data, err := c.get(path)
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			var verbs []actionVerb
			if err := json.Unmarshal(data, &verbs); err != nil {
				return fmt.Errorf("parse response: %w", err)
			}

			if len(args) == 0 {
				printCategoryOverview(verbs)
				if hint := noTradeContactsHint(verbs); hint != "" {
					fmt.Println()
					fmt.Println(hint)
				}
				return nil
			}
			category := strings.ToLower(args[0])
			if !isKnownCategory(category) {
				return fmt.Errorf("unknown category %q — one of: %s", category, strings.Join(categoryOrder, ", "))
			}
			printCategoryDetail(verbs, category)
			return nil
		},
	}
	cmd.Flags().StringVar(&provinceID, "province", "", "province ID to inspect (default: your capital)")
	return cmd
}

// printFounderActions is the founder-phase action surface: what a Wanax whose
// people still wander can actually do. Built client-side from /founding/status
// because the capabilities endpoint requires an owned settlement.
func printFounderActions(c *Client) error {
	fp, err := fetchFoundingStatus(c)
	if err != nil {
		return err
	}
	if !fp.Active {
		return fmt.Errorf("no province in config and no active founder phase — run 'keryx login' again")
	}
	hostID := "<host-id>"
	if fp.HostUnitID != nil {
		hostID = *fp.HostUnitID
	}
	verbs := []actionVerb{
		{Name: "march", Category: "military", Available: true,
			Purpose: fmt.Sprintf("Travel: keryx unit march --unit %s --q <q> --r <r>", hostID)},
		{Name: "settle", Category: "province", Available: true,
			Purpose: "Found the metropolis where the host stands (irreversible): keryx founding settle"},
		{Name: "message", Category: "diplomacy", Available: true,
			Purpose: "Messenger from the host (free, FOW-gated): keryx message --from-host --to <city> --text \"...\""},
		{Name: "founding-status", Category: "province", Available: true,
			Purpose: "The people, escort stores, position: keryx founding status"},
	}
	if jsonMode {
		printJSON(verbs)
		return nil
	}
	fmt.Println("Founder phase — your people are still traveling; no city verbs until the metropolis is founded.")
	fmt.Println(strings.Repeat("─", 60))
	fmt.Println("\nAvailable now")
	for _, v := range verbs {
		fmt.Printf("  %-16s %s\n", v.Name, v.Purpose)
	}
	fmt.Println("\nLocked — everything city-bound (build, recruit, trade, rite …) unlocks with: keryx founding settle")
	return nil
}

func isKnownCategory(c string) bool {
	for _, k := range categoryOrder {
		if k == c {
			return true
		}
	}
	return false
}

// noTradeContactsHint returns a discoverability nudge when this Wanax has
// zero foreign settlements within vision — the FOW gate that BOTH `message`
// and `trade-offer` require (internal/capabilities/diplomacy_verbs.go
// canMessage, trade_verbs.go canTradeOffer/canSell: "a contacted foreign
// settlement (FOW-visible)"). `message`'s only requirement IS that gate, so
// verb.Available==false for "message" is an unambiguous "no trade contacts
// yet" signal — no new server field needed, this reuses the existing
// /actions response `keryx actions`/`status` already fetch.
//
// Playtest gap (sondrundor 2026-07-23/24, megaron_todo.md "cities-
// discoverability"): a new founder had no trade contacts and nothing in
// `actions`/`status` pointed toward the EXISTING way to get one — send a
// unit outward or send a messenger once a neighbour is visible. Returns ""
// once at least one foreign settlement is visible (message unlocks).
func noTradeContactsHint(verbs []actionVerb) string {
	for _, v := range verbs {
		if v.Name == "message" && v.Category == "diplomacy" {
			if v.Available {
				return ""
			}
			return "No trade contacts yet — trade needs a foreign settlement within your vision. " +
				"Run `keryx cities` to see known/rumoured neighbours (march outward or colonise to reach one), " +
				"then `keryx message --to <name>` or `keryx trade-offer` once one is visible."
		}
	}
	return ""
}

func printCategoryOverview(verbs []actionVerb) {
	byCategory := map[string][]actionVerb{}
	for _, v := range verbs {
		byCategory[v.Category] = append(byCategory[v.Category], v)
	}
	fmt.Println("What you can do — by category (keryx actions <category> for details)")
	fmt.Println(strings.Repeat("─", 60))
	for _, cat := range categoryOrder {
		list := byCategory[cat]
		available := 0
		for _, v := range list {
			if v.Available {
				available++
			}
		}
		fmt.Printf("  %-10s  %d available · %d locked\n", cat, available, len(list)-available)
	}
}

func printCategoryDetail(verbs []actionVerb, category string) {
	var available, locked []actionVerb
	for _, v := range verbs {
		if v.Category != category {
			continue
		}
		if v.Available {
			available = append(available, v)
		} else {
			locked = append(locked, v)
		}
	}
	sort.Slice(available, func(i, j int) bool { return available[i].Name < available[j].Name })
	sort.Slice(locked, func(i, j int) bool { return locked[i].Name < locked[j].Name })

	fmt.Printf("%s — %d available · %d locked\n", category, len(available), len(locked))
	fmt.Println(strings.Repeat("─", 60))

	fmt.Println("\nAvailable now")
	if len(available) == 0 {
		fmt.Println("  (none)")
	}
	for _, v := range available {
		fmt.Printf("  %-16s %s\n", v.Name, v.Purpose)
	}

	fmt.Println("\nLocked — here's how to unlock it")
	if len(locked) == 0 {
		fmt.Println("  (none)")
	}
	for _, v := range locked {
		fmt.Printf("  %-16s %s\n", v.Name, v.Purpose)
		for _, r := range v.Requirements {
			if r.Satisfied {
				fmt.Printf("      ✓ %s\n", r.Text)
				continue
			}
			fmt.Printf("      ✗ %s — %s\n", r.Text, r.Detail)
			fmt.Printf("        → %s\n", r.Hint)
		}
	}
}
