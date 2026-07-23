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
		return fmt.Errorf("ingen provins i config och ingen aktiv founder-fas — kör 'keryx login' igen")
	}
	hostID := "<host-id>"
	if fp.HostUnitID != nil {
		hostID = *fp.HostUnitID
	}
	verbs := []actionVerb{
		{Name: "march", Category: "military", Available: true,
			Purpose: fmt.Sprintf("Vandra: keryx unit march --unit %s --q <q> --r <r>", hostID)},
		{Name: "settle", Category: "province", Available: true,
			Purpose: "Grunda huvudstaden där hostet står (oåterkalleligt): keryx founding settle"},
		{Name: "message", Category: "diplomacy", Available: true,
			Purpose: "Budbärare från hostet (gratis, FOW-gatead): keryx message --from-host --to <stad> --text \"...\""},
		{Name: "founding-status", Category: "province", Available: true,
			Purpose: "Folket, eskortens förråd, position: keryx founding status"},
	}
	if jsonMode {
		printJSON(verbs)
		return nil
	}
	fmt.Println("Founder-fas — ditt folk vandrar ännu; inga stadsverb förrän huvudstaden är grundad.")
	fmt.Println(strings.Repeat("─", 60))
	fmt.Println("\nAvailable now")
	for _, v := range verbs {
		fmt.Printf("  %-16s %s\n", v.Name, v.Purpose)
	}
	fmt.Println("\nLocked — allt stadsbundet (build, recruit, trade, rite …) låses upp av: keryx founding settle")
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
