package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
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
	CanPrepareOffering    bool               `json:"can_prepare_offering"`
	GodID                 string             `json:"god_id"`
	DeparturePending      bool               `json:"departure_pending"`
	Default               bool               `json:"is_default"`
	ActiveRemainingDays   int                `json:"active_remaining_game_days"`
	ID                    string             `json:"id"`
	Name                  string             `json:"name"`
	God                   string             `json:"god"`
	Effect                string             `json:"effect"`
	MinKharis             float64            `json:"min_kharis"`
	Offering              map[string]float64 `json:"offering"`
	Favours               map[string]float64 `json:"favours"`
	OfferingBaseline      float64            `json:"offering_baseline"`
	UnavailableReason     string             `json:"unavailable_reason"`
	Affordable            bool               `json:"affordable"`
	CooldownRemainingDays int                `json:"cooldown_remaining_game_days"`
}

// riteProvinceStatus is the slice of the province status response this file needs.
type riteProvinceStatus struct {
	Settlement struct {
		DivineValues     map[string]float64    `json:"divine_values"`
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
		Example: `  keryx rite --list
  keryx rite --prayer <prayer-id>
  keryx rite --prayer <prayer-id> --settlement <settlement-uuid>
  keryx rite --prayer <prayer-id> --offer grain=10,silver=3
  keryx rite --prayer <prayer-id> --yes
  keryx rite --json`,
		Args: rejectPositionalArgs("prayer"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)

			// --list: show this culture's available prayers (id + affordability) from
			// the province status endpoint, which exposes `available_prayers`. Works for
			// any culture — no hardcoded akhaier_* IDs.
			if list {
				// The same city a cast would use (--settlement, else the capital),
				// not the login default — GPT19: a colony without a temple listed
				// "no temple" while the cast from the capital succeeded.
				listProvince := cfg.ProvinceID
				provs, err := c.get(fmt.Sprintf("/api/v1/worlds/%s/provinces", cfg.WorldID))
				if err != nil {
					return err
				}
				var listMarkers []map[string]any
				_ = json.Unmarshal(provs, &listMarkers)
				if sid := riteSettlement(listMarkers, settlementID); sid != "" {
					for _, m := range listMarkers {
						if msid, _ := m["settlement_id"].(string); msid == sid {
							listProvince, _ = m["id"].(string)
							break
						}
					}
				}
				data, err := c.get(fmt.Sprintf("/api/v1/worlds/%s/provinces/%s", cfg.WorldID, listProvince))
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
				fmt.Printf("The gods are %s (kharis %s).\n\n", p.Settlement.KharisMood, prayerKharisReading(p.Settlement.Kharis))
				printDivineValues(p.Settlement.DivineValues)
				fmt.Printf("%-28s  %-20s  %-8s  %-22s  %-45s  %-12s  %s\n",
					"Prayer ID", "Name", "MinKhar", "Offering", "Effect", "Ready", "Cooldown")
				for _, pr := range p.Settlement.AvailablePrayers {
					ready, cooldownStr := prayerReadiness(pr)
					fmt.Printf("%-28s  %-20s  %-8.0f  %-22s  %-45s  %-12s  %s\n",
						pr.ID, pr.Name, pr.MinKharis, formatOffering(pr.Offering), pr.Effect, ready, cooldownStr)
					// The temple's reading: an offering is composed now, and its
					// worth turns on world scarcity a Wanax cannot see through the
					// fog. The temple knows — so it says so here, where the choice
					// is made.
					if taste := favouredGoods(pr.Favours); taste != "" {
						fmt.Printf("    %s favours %s", pr.God, taste)
						if pr.OfferingBaseline > 0 {
							fmt.Printf("  ·  expects an offering worth ~%.0f (--offer good=amount)", pr.OfferingBaseline)
						}
						fmt.Println()
					} else if pr.God != "" {
						fmt.Printf("    %s\n", pr.God)
					}
				}
				fmt.Println("\nKharis is a requirement, never spent. Everyday prayers answer when their requirements are met; legacy prayers may remain silent. An accepted offering is spent either way.")
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
				settlementID = riteSettlement(markers, "")
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
					fmt.Println("Cast cancelled — nothing was offered or spent.")
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
				// What the offering achieved — separate from whether the gods
				// answered. The odds stay hidden; whether MORE would help does not.
				OfferingVerdict string `json:"offering_verdict"`
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
			if resp.OfferingVerdict != "" {
				fmt.Printf("Offering: %s\n", resp.OfferingVerdict)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&list, "list", false, "list this culture's available prayers (id + affordability) and exit")
	cmd.Flags().StringVar(&settlementID, "settlement", "", "settlement UUID (defaults to your capital)")
	cmd.Flags().StringVar(&prayerID, "prayer", "", "prayer ID (run --list to see your culture's prayers; defaults to the culture's listed default prayer)")
	cmd.Flags().StringVar(&targetID, "target", "", "target province UUID (for future targeted prayers)")
	cmd.Flags().BoolVar(&yes, "yes", false, "skip the pre-cast confirmation (required for non-interactive/agent use)")
	cmd.Flags().StringVar(&offer, "offer", "",
		"composed offering, e.g. --offer wine=20,oil=10 — worth at least the traditional offering; the gods value what the world is short of")
	cmd.Flags().Float64Var(&offerMultiplier, "offer-multiplier", 0,
		"scale the material offering (0.5-2.0) of an old prayer without a god; everyday prayers always take their traditional offering")
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
		if prayerID == "" && pr.Default {
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
	if chosen.GodID == "" && offerMultiplier != 0 && offerMultiplier != 1 {
		fmt.Printf("Offer multiplier: %.2fx — changes the offering and the gods' response.\n", offerMultiplier)
	}
	fmt.Println(prayerPaymentText(*chosen))
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

// favouredGoods names the goods a god weighs above the neutral 1.0, heaviest
// first — the short answer to "what should I bring?". Only the top few: the full
// taste table is noise at the moment of choosing.
func favouredGoods(favours map[string]float64) string {
	type pair struct {
		good   string
		weight float64
	}
	var liked []pair
	for good, weight := range favours {
		if weight > 1.0 {
			liked = append(liked, pair{good, weight})
		}
	}
	if len(liked) == 0 {
		return ""
	}
	sort.Slice(liked, func(i, j int) bool { return liked[i].weight > liked[j].weight })
	if len(liked) > 4 {
		liked = liked[:4]
	}
	parts := make([]string, 0, len(liked))
	for _, l := range liked {
		parts = append(parts, fmt.Sprintf("%s ×%.1f", l.good, l.weight))
	}
	return strings.Join(parts, ", ")
}

// gameDaysRemaining uses the shared Keryx unit formatter.
func gameDaysRemaining(days int) string { return formatDays(days) + " remaining" }

// prayerReadiness keeps a pending one-use gift distinct from a timed effect.
func prayerReadiness(pr riteAvailablePrayer) (string, string) {
	if pr.CanPrepareOffering && !pr.Affordable {
		return "compose", "traditional gift unavailable; use --offer"
	}
	if pr.UnavailableReason != "" {
		return "no: " + pr.UnavailableReason, "—"
	}
	if pr.DeparturePending {
		status := "awaiting next departure"
		if pr.CooldownRemainingDays > 0 {
			status += "; " + gameDaysRemaining(pr.CooldownRemainingDays) + " wait"
		}
		return "pending", status
	}
	if pr.ActiveRemainingDays > 0 {
		status := gameDaysRemaining(pr.ActiveRemainingDays) + " active"
		if pr.CooldownRemainingDays > 0 {
			status += "; " + gameDaysRemaining(pr.CooldownRemainingDays) + " wait"
		}
		return "active", status
	}
	if pr.CooldownRemainingDays > 0 {
		return "cooldown", gameDaysRemaining(pr.CooldownRemainingDays)
	}
	if !pr.Affordable {
		return "no: check temple, kharis and offering", "—"
	}
	return "yes", "—"
}

// Floor the displayed precision so a value below a threshold never rounds up to it.
func prayerKharisReading(value float64) string {
	return fmt.Sprintf("%.1f", math.Floor(value*10)/10)
}

func printDivineValues(values map[string]float64) {
	fmt.Println("Offering values — per unit, before each god's preferences; not trade prices:")
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		fmt.Println("  Current offering values unavailable.")
	}
	for _, key := range keys {
		fmt.Printf("  %-12s %.4g\n", key, values[key])
	}
	fmt.Println()
}

// riteSettlement is the city a rite uses: the one named with --settlement,
// else the own capital, else any own settlement. --list and the cast share it.
func riteSettlement(markers []map[string]any, given string) string {
	if given != "" {
		return given
	}
	for _, m := range markers {
		if own, _ := m["own"].(bool); own {
			if isCapital, _ := m["is_capital"].(bool); isCapital {
				sid, _ := m["settlement_id"].(string)
				return sid
			}
		}
	}
	for _, m := range markers {
		if own, _ := m["own"].(bool); own {
			sid, _ := m["settlement_id"].(string)
			return sid
		}
	}
	return ""
}

// The server's stable god key distinguishes everyday prayers from legacy ones.
func prayerPaymentText(prayer riteAvailablePrayer) string {
	if prayer.GodID != "" {
		return "Kharis is a requirement, not spent. When the requirements are met the god answers; more offering will not improve the answer."
	}
	return "The offering is consumed even if the gods stay silent; kharis is a requirement, not spent."
}
