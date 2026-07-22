package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// riteAvailablePrayer mirrors province.go's available_prayers shape (the
// prayerRow struct in api/handlers/province.go). Shared by --list and the
// pre-cast confirmation so both read the exact same fields.
type riteAvailablePrayer struct {
	ID                    string             `json:"id"`
	Name                  string             `json:"name"`
	God                   string             `json:"god"`
	Effect                string             `json:"effect"`
	MinKharis             float64            `json:"min_kharis"`
	Offering              map[string]float64 `json:"offering"`
	Affordable            bool               `json:"affordable"`
	CooldownRemainingMins float64            `json:"cooldown_remaining_minutes"`
}

// riteProvinceStatus is the slice of the province status response this file needs.
type riteProvinceStatus struct {
	Settlement struct {
		Kharis           float64               `json:"kharis"`
		KharisMood       string                `json:"kharis_mood"`
		AvailablePrayers []riteAvailablePrayer `json:"available_prayers"`
	} `json:"settlement"`
}

// formatOffering renders a material offering as "grain×25 oil×15", sorted for
// stable output. Used by --list and the pre-cast confirmation.
func formatOffering(offering map[string]float64) string {
	keys := make([]string, 0, len(offering))
	for g := range offering {
		keys = append(keys, g)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, g := range keys {
		fmt.Fprintf(&b, "%s×%.0f ", g, offering[g])
	}
	if b.Len() == 0 {
		return "—"
	}
	return strings.TrimSpace(b.String())
}

// stdinIsTerminal reports whether stdin is an interactive terminal. When it
// isn't (piped input, no input at all — the agent-harness case), the rite
// pre-cast confirmation must not block waiting for a y/N it will never get.
func stdinIsTerminal() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

func riteCmd() *cobra.Command {
	var settlementID string
	var prayerID string
	var targetID string
	var list bool
	var yes bool
	var offerMultiplier float64
	var offer string

	cmd := &cobra.Command{
		Use:   "rite",
		Short: "Perform a cultural prayer at your settlement (requires temple + offering)",
		Example: `  poleia rite --list
  poleia rite --prayer <prayer-id>
  poleia rite --prayer <prayer-id> --settlement <settlement-uuid>
  poleia rite --prayer <prayer-id> --offer-multiplier 2.0
  poleia rite --prayer <prayer-id> --yes
  poleia rite --json`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)

			// --list: show this culture's available prayers (id + affordability) from
			// the province status endpoint, which exposes `available_prayers`. Works for
			// any culture — no hardcoded akhaier_* IDs.
			if list {
				data, err := c.get(fmt.Sprintf("/api/v1/worlds/%s/provinces/%s", cfg.WorldID, cfg.ProvinceID))
				if err != nil {
					return err
				}
				if jsonMode {
					printRawJSON(data)
					return nil
				}
				var p riteProvinceStatus
				if err := json.Unmarshal(data, &p); err != nil {
					return fmt.Errorf("parse response: %w", err)
				}
				if len(p.Settlement.AvailablePrayers) == 0 {
					fmt.Println("No prayers available (no settlement here, or none for this culture).")
					return nil
				}
				// Gynnsamhet is read via the gods' mood, never a computed percentage
				// (Timothy 2026-07-11 hard invariant — "gudarna är inte maskiner").
				fmt.Printf("The gods are %s (kharis %.0f).\n\n", p.Settlement.KharisMood, p.Settlement.Kharis)
				fmt.Printf("%-28s  %-20s  %-8s  %-22s  %-45s  %-12s  %s\n",
					"Prayer ID", "Name", "MinKhar", "Offering", "Effect", "Ready", "Cooldown")
				for _, pr := range p.Settlement.AvailablePrayers {
					ready := "yes"
					cooldownStr := "—"
					if pr.CooldownRemainingMins > 0 {
						ready = "cooldown"
						mins := pr.CooldownRemainingMins
						if mins < 60 {
							cooldownStr = fmt.Sprintf("%.0fm remaining", mins)
						} else {
							cooldownStr = fmt.Sprintf("%.1fh remaining", mins/60)
						}
					} else if !pr.Affordable {
						ready = "no"
					}
					fmt.Printf("%-28s  %-20s  %-8.0f  %-22s  %-45s  %-12s  %s\n",
						pr.ID, pr.Name, pr.MinKharis, formatOffering(pr.Offering), pr.Effect, ready, cooldownStr)
				}
				fmt.Println("\nThe offering is consumed even if the gods stay silent — they are fickle.")
				return nil
			}

			// Resolve own capital settlement if --settlement not given. Markers are kept
			// around (not discarded) so the pre-cast confirmation below can find this
			// settlement's province without a second /provinces round-trip.
			var markers []map[string]any
			if settlementID == "" {
				provs, err := c.get(fmt.Sprintf("/api/v1/worlds/%s/provinces", cfg.WorldID))
				if err != nil {
					return err
				}
				_ = json.Unmarshal(provs, &markers)
				for _, m := range markers {
					if own, _ := m["own"].(bool); own {
						if isCapital, _ := m["is_capital"].(bool); isCapital {
							settlementID, _ = m["settlement_id"].(string)
							break
						}
					}
				}
				// Fallback: any own settlement.
				if settlementID == "" {
					for _, m := range markers {
						if own, _ := m["own"].(bool); own {
							settlementID, _ = m["settlement_id"].(string)
							break
						}
					}
				}
			}
			if settlementID == "" {
				return fmt.Errorf("could not find own settlement — use --settlement <id>")
			}

			// Pre-cast confirmation (A7 core): shows mood + offer + effect + the miss-note
			// before the offering is spent. Skipped via --yes, --json, or when stdin isn't
			// a terminal — an unanswered prompt would otherwise hang the agent harness.
			autoConfirm := yes || jsonMode || !stdinIsTerminal()
			if !autoConfirm {
				confirmed, err := confirmRiteCast(c, markers, settlementID, prayerID, offerMultiplier)
				if err != nil {
					return err
				}
				if !confirmed {
					fmt.Println("Cast cancelled.")
					return nil
				}
			}

			// Build request body. Empty fields are omitted so the server applies defaults.
			body := map[string]any{}
			if prayerID != "" {
				body["prayer"] = prayerID
			}
			if targetID != "" {
				body["target"] = targetID
			}
			if offerMultiplier != 0 {
				body["offer_multiplier"] = offerMultiplier
			}
			// Composed offering: bring what you judge worthy instead of the
			// prayer's inherited recipe. Worth is the world's scarcity times this
			// god's taste, weighed against the same god's traditional recipe —
			// so the old recipe still reads as exactly adequate.
			if offer != "" {
				composed, perr := parseOffering(offer)
				if perr != nil {
					return perr
				}
				body["offering"] = composed
			}

			path := fmt.Sprintf("/api/v1/worlds/%s/settlements/%s/rite", cfg.WorldID, settlementID)
			data, err := c.post(path, body)
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}

			var resp struct {
				Success    bool           `json:"success"`
				Mood       string         `json:"mood"`
				Prayer     string         `json:"prayer"`
				EffectType string         `json:"effect_type"`
				Effect     map[string]any `json:"effect"`
				Message    string         `json:"message"`
			}
			if err := json.Unmarshal(data, &resp); err != nil {
				return fmt.Errorf("parse response: %w", err)
			}

			fmt.Printf("Divine mood: %s\n", resp.Mood)
			if resp.Prayer != "" {
				fmt.Printf("Prayer: %s\n", resp.Prayer)
			}
			if resp.Success {
				fmt.Printf("Success: %s\n", resp.Message)
				if resp.EffectType != "" {
					fmt.Printf("Effect: %s\n", resp.EffectType)
				}
			} else {
				fmt.Printf("Failed: %s\n", resp.Message)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&list, "list", false, "list this culture's available prayers (id + affordability) and exit")
	cmd.Flags().StringVar(&settlementID, "settlement", "", "settlement UUID (defaults to your capital)")
	cmd.Flags().StringVar(&prayerID, "prayer", "", "prayer ID (run --list to see your culture's prayers; defaults to culture battle_frenzy)")
	cmd.Flags().StringVar(&targetID, "target", "", "target province UUID (for future targeted prayers)")
	cmd.Flags().BoolVar(&yes, "yes", false, "skip the pre-cast confirmation (required for non-interactive/agent use)")
	cmd.Flags().StringVar(&offer, "offer", "",
		"composed offering, e.g. --offer wine=20,oil=10 (overrides --offer-multiplier; the gods value what the world is short of)")
	cmd.Flags().Float64Var(&offerMultiplier, "offer-multiplier", 0,
		"scale the material offering (0.5-2.0; a fatter offer pleases the gods more, a stingy one less — omit for baseline 1.0)")
	return cmd
}

// confirmRiteCast fetches the target settlement's live mood + the chosen prayer's
// offer/effect and asks the player to confirm before the offering is spent — it
// is consumed even on a miss, and that has no other warning (A7). Returns false
// (no error) if the player declines. Never shows a computed success percentage —
// only the gods' mood (Timothy 2026-07-11 hard invariant).
func confirmRiteCast(c *Client, markers []map[string]any, settlementID, prayerID string, offerMultiplier float64) (bool, error) {
	if markers == nil {
		provs, err := c.get(fmt.Sprintf("/api/v1/worlds/%s/provinces", cfg.WorldID))
		if err != nil {
			return false, err
		}
		_ = json.Unmarshal(provs, &markers)
	}
	var provinceID string
	for _, m := range markers {
		if sid, _ := m["settlement_id"].(string); sid == settlementID {
			provinceID, _ = m["id"].(string)
			break
		}
	}
	if provinceID == "" {
		// Can't build a preview — still require an explicit confirmation rather than
		// silently casting blind.
		fmt.Printf("Could not fetch a preview for settlement %s.\n", settlementID)
		return askYesNo("Cast anyway?")
	}

	data, err := c.get(fmt.Sprintf("/api/v1/worlds/%s/provinces/%s", cfg.WorldID, provinceID))
	if err != nil {
		return false, err
	}
	var p riteProvinceStatus
	if err := json.Unmarshal(data, &p); err != nil {
		return false, fmt.Errorf("parse response: %w", err)
	}

	// Resolve which prayer will actually be cast: explicit --prayer, or the
	// culture's battle_frenzy default (mirrors religion.DefaultBattleFrenzyFor
	// server-side, without needing the culture string — available_prayers is
	// already culture-filtered).
	var chosen *riteAvailablePrayer
	for i := range p.Settlement.AvailablePrayers {
		pr := &p.Settlement.AvailablePrayers[i]
		if prayerID != "" && pr.ID == prayerID {
			chosen = pr
			break
		}
		if prayerID == "" && strings.HasSuffix(pr.ID, "_battle_frenzy") {
			chosen = pr
			break
		}
	}
	if chosen == nil {
		fmt.Printf("Could not find prayer %q for this settlement's culture.\n", prayerID)
		return askYesNo("Cast anyway?")
	}

	fmt.Printf("The gods are %s (kharis %.0f).\n", p.Settlement.KharisMood, p.Settlement.Kharis)
	fmt.Printf("Prayer: %s (%s)\n", chosen.Name, chosen.God)
	fmt.Printf("Effect: %s\n", chosen.Effect)
	fmt.Printf("Offer: %s\n", formatOffering(chosen.Offering))
	if offerMultiplier != 0 && offerMultiplier != 1.0 {
		if offerMultiplier > 1.0 {
			fmt.Printf("Offer multiplier: %.2fx — a fatter offer pleases the gods more.\n", offerMultiplier)
		} else {
			fmt.Printf("Offer multiplier: %.2fx — a stingier offer pleases the gods less.\n", offerMultiplier)
		}
	}
	fmt.Println("The offering is consumed even if the gods stay silent.")
	return askYesNo("Cast?")
}

// askYesNo prints a "<prompt> [y/N]: " and reads a line from stdin. Anything
// other than a leading y/Y is a decline — the default is "no".
func askYesNo(prompt string) (bool, error) {
	fmt.Printf("%s [y/N]: ", prompt)
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil && line == "" {
		return false, nil
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes", nil
}

// parseOffering reads --offer wine=20,oil=10 into the map the Rite endpoint takes.
// Same shape as allocate --raw, so the two read alike.
func parseOffering(raw string) (map[string]float64, error) {
	out := map[string]float64{}
	for _, pair := range strings.Split(raw, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("bad --offer entry %q — use good=amount, e.g. wine=20,oil=10", pair)
		}
		amount, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		if err != nil || amount <= 0 {
			return nil, fmt.Errorf("bad amount in %q — must be a positive number", pair)
		}
		out[strings.TrimSpace(parts[0])] = amount
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("--offer was empty — name at least one good, e.g. --offer wine=20")
	}
	return out, nil
}
