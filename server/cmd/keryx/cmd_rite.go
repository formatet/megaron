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
	// Bönegrunden: the temple rests, not the god.
	Temple            string `json:"temple"`
	RestRemainingDays int    `json:"rest_remaining_game_days"`
	RestPaused        bool   `json:"rest_paused"`
	TempleLevel       int    `json:"temple_level"`
	TempleStaffed     int    `json:"temple_staffed"`
	TempleCapacity    int    `json:"temple_capacity"`
	BaseDurationDays  int    `json:"base_duration_game_days"`
	// Blessing prayers (steg 2a): what may be named and what each step costs (x the traditional gift).
	Targets          *riteTargets   `json:"targets"`
	ScopeCostFactors map[string]int `json:"scope_cost_factors"`
}

type riteTargets struct {
	Unit []struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Where   string `json:"where"`
		Forever bool   `json:"forever"`
	} `json:"unit"`
	City struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	} `json:"city"`
	Realm struct {
		Count int `json:"count"`
	} `json:"realm"`
	// Always: a city or realm blessing needs nothing under way now (Runner/caravan prayers hold for departures too).
	Always bool `json:"always"`
	// Storm prayers: Kind "storm" picks one of Storms; Kind "hex" takes a sea hex of your map as q,r.
	Kind   string `json:"kind"`
	Storms []struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Where string `json:"where"`
	} `json:"storms"`
	// Kind "city" (The Walls Fall): a foreign city of your map with a remembered wall.
	Cities []struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Where string `json:"where"`
	} `json:"cities"`
	// Kind "place" (The Bird Oracle): Cities (yours and remembered) or Places (named, anchor known).
	Places []struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Where string `json:"where"`
	} `json:"places"`
}

// scaledOffering is the traditional gift times a target step's factor.
func scaledOffering(offering map[string]float64, factor int) map[string]float64 {
	out := make(map[string]float64, len(offering))
	for g, n := range offering {
		out[g] = n * float64(factor)
	}
	return out
}

// printRiteTargets lists what a blessing prayer may be laid on and what each step costs.
func printRiteTargets(p riteAvailablePrayer, settlementID string) {
	if p.Targets == nil {
		return
	}
	switch p.Targets.Kind {
	case "storm":
		if len(p.Targets.Storms) == 0 {
			fmt.Printf("  No named storm on your storm map: a storm must be named (keryx storms name) before it can be stilled.\n")
			return
		}
		fmt.Printf("  Choose a storm with --storm <name> (one gift, whoever's ships sail there):\n")
		for _, st := range p.Targets.Storms {
			fmt.Printf("        %s (%s) %s\n", st.Name, st.Where, st.ID)
		}
		return
	case "city":
		if len(p.Targets.Cities) == 0 {
			fmt.Printf("  No foreign city with a wall you remember: see one with its wall (keryx cities) before Potnia can be asked.\n")
			return
		}
		fmt.Printf("  Choose a city with --target <name> (one gift; the wall falls a level 3 game days after the prayer, and you will not be told):\n")
		for _, c := range p.Targets.Cities {
			fmt.Printf("        %s (%s) %s\n", c.Name, c.Where, c.ID)
		}
		return
	case "place":
		if len(p.Targets.Cities) == 0 && len(p.Targets.Places) == 0 {
			fmt.Printf("  No named place on your map: Asterion's birds need a city or a named land you know.\n")
			return
		}
		fmt.Printf("  Choose a place with --target <name> (one gift; you see within 2 hexes of it for 1 game day, live; the watched are told nothing):\n")
		for _, c := range p.Targets.Cities {
			fmt.Printf("        %s (%s) %s\n", c.Name, c.Where, c.ID)
		}
		for _, pl := range p.Targets.Places {
			fmt.Printf("        %s (%s)\n", pl.Name, pl.Where)
		}
		return
	case "hex":
		fmt.Printf("  Choose a sea hex of your own map with --hex q,r — a storm is born there (one gift, no more for a richer one).\n")
		return
	}
	f := p.ScopeCostFactors
	if p.Targets.Kind == "world" {
		fmt.Printf("  Choose how wide with --scope; the wider, the dearer (gift × step):\n")
		if f["city"] > 0 {
			fmt.Printf("    --scope city  ×%d  %s  — %s only\n", f["city"], formatOffering(scaledOffering(p.Offering, f["city"])), p.Targets.City.Name)
		}
		fmt.Printf("    --scope realm ×%d  %s  — all %d of your cities\n", f["realm"], formatOffering(scaledOffering(p.Offering, f["realm"])), p.Targets.Realm.Count)
		return
	}
	fmt.Printf("  Choose a target with --scope; the wider, the dearer (gift × step):\n")
	fmt.Printf("    --scope unit  ×%d  %s  — one named, with --target <name|id>\n", f["unit"], formatOffering(scaledOffering(p.Offering, f["unit"])))
	for _, u := range p.Targets.Unit {
		note := ""
		if u.Forever {
			note = " — already blessed for ever"
		}
		fmt.Printf("        %s (%s) %s%s\n", u.Name, u.Where, u.ID, note)
	}
	if f["permanent"] > 0 {
		fmt.Printf("    --scope permanent ×%d  %s  — one named, for ever (never a city or the realm), with --target <name|id>\n", f["permanent"], formatOffering(scaledOffering(p.Offering, f["permanent"])))
	}
	if p.Targets.Always {
		what := "Runner and caravan"
		if p.ID == "minoan_asterion_thread_v2" {
			what = "land caravan" // the Thread never reaches a Runner
		}
		fmt.Printf("    --scope city  ×%d  %s  — every %s of %s, now on the road (%d) and while the prayer lasts\n", f["city"], formatOffering(scaledOffering(p.Offering, f["city"])), what, p.Targets.City.Name, p.Targets.City.Count)
		fmt.Printf("    --scope realm ×%d  %s  — all of yours, now on the road (%d) and while the prayer lasts\n", f["realm"], formatOffering(scaledOffering(p.Offering, f["realm"])), p.Targets.Realm.Count)
		return
	}
	fmt.Printf("    --scope city  ×%d  %s  — the %d supported by %s\n", f["city"], formatOffering(scaledOffering(p.Offering, f["city"])), p.Targets.City.Count, p.Targets.City.Name)
	fmt.Printf("    --scope realm ×%d  %s  — all %d\n", f["realm"], formatOffering(scaledOffering(p.Offering, f["realm"])), p.Targets.Realm.Count)
}

// resolveRiteTarget turns --target (a name or an id) into a unit id from the prayer's own target list.
func resolveRiteTarget(p riteAvailablePrayer, arg string) (string, error) {
	if p.Targets == nil {
		return arg, nil
	}
	var names []string
	var hits []string
	for _, u := range p.Targets.Unit {
		names = append(names, fmt.Sprintf("%s (%s)", u.Name, u.Where))
		if strings.EqualFold(u.ID, arg) {
			return u.ID, nil
		}
		if strings.Contains(strings.ToLower(u.Name), strings.ToLower(arg)) {
			hits = append(hits, u.ID)
		}
	}
	for _, u := range p.Targets.Unit {
		if strings.EqualFold(u.Name, arg) {
			return u.ID, nil
		}
	}
	if len(hits) == 1 {
		return hits[0], nil
	}
	list := strings.Join(names, ", ")
	if list == "" {
		list = "none"
	}
	if len(hits) > 1 {
		return "", fmt.Errorf("%q matches more than one — name it fully or use the id. Yours: %s", arg, list)
	}
	return "", fmt.Errorf("no unit of yours called %q for this prayer. Yours: %s", arg, list)
}

// resolveRiteStorm turns --storm (a name or an id) into a storm id from the prayer's storm list.
func resolveRiteStorm(p riteAvailablePrayer, arg string) (string, error) {
	if p.Targets == nil || p.Targets.Kind != "storm" {
		return "", fmt.Errorf("--storm goes with a prayer that stills a storm (minoan_stillness_calm)")
	}
	var names, hits []string
	for _, st := range p.Targets.Storms {
		names = append(names, st.Name)
		if strings.EqualFold(st.ID, arg) || strings.EqualFold(st.Name, arg) {
			return st.ID, nil
		}
		if arg != "" && strings.Contains(strings.ToLower(st.Name), strings.ToLower(arg)) {
			hits = append(hits, st.ID)
		}
	}
	if len(hits) == 1 {
		return hits[0], nil
	}
	list := strings.Join(names, ", ")
	if list == "" {
		list = "none: name a storm first"
	}
	if len(hits) > 1 {
		return "", fmt.Errorf("%q matches more than one storm — name it fully. On your map: %s", arg, list)
	}
	return "", fmt.Errorf("no named storm %q on your storm map. On your map: %s", arg, list)
}

// resolveRiteCity turns --target (a name or an id) into a city id from the prayer's city list.
func resolveRiteCity(p riteAvailablePrayer, arg string) (string, error) {
	var names, hits []string
	for _, c := range p.Targets.Cities {
		names = append(names, c.Name)
		if strings.EqualFold(c.ID, arg) || strings.EqualFold(c.Name, arg) {
			return c.ID, nil
		}
		if arg != "" && strings.Contains(strings.ToLower(c.Name), strings.ToLower(arg)) {
			hits = append(hits, c.ID)
		}
	}
	if len(hits) == 1 {
		return hits[0], nil
	}
	list := strings.Join(names, ", ")
	if list == "" {
		list = "none: see a walled city first"
	}
	if len(hits) > 1 {
		return "", fmt.Errorf("%q matches more than one city — name it fully. On your map: %s", arg, list)
	}
	return "", fmt.Errorf("no foreign city %q with a wall you remember. On your map: %s", arg, list)
}

// resolveRitePlace turns --target (a name or an id) into what the server takes: a city id, or a place name.
func resolveRitePlace(p riteAvailablePrayer, arg string) (string, error) {
	type opt struct{ name, post string }
	var all []opt
	for _, c := range p.Targets.Cities {
		all = append(all, opt{c.Name, c.ID})
	}
	for _, pl := range p.Targets.Places {
		all = append(all, opt{pl.Name, pl.Name})
	}
	var names, hits []string
	for _, o := range all {
		names = append(names, o.name)
		if strings.EqualFold(o.post, arg) || strings.EqualFold(o.name, arg) {
			return o.post, nil
		}
		if arg != "" && strings.Contains(strings.ToLower(o.name), strings.ToLower(arg)) {
			hits = append(hits, o.post)
		}
	}
	list := strings.Join(names, ", ")
	if list == "" {
		list = "none"
	}
	if len(hits) == 1 {
		return hits[0], nil
	}
	if len(hits) > 1 {
		return "", fmt.Errorf("%q matches more than one place — name it fully. On your map: %s", arg, list)
	}
	return "", fmt.Errorf("no city or named place %q on your map. On your map: %s", arg, list)
}

// riteProvinceStatus is the slice of the province status response this file needs.
type riteProvinceStatus struct {
	Settlement struct {
		DivineValues   map[string]float64 `json:"divine_values"`
		Kharis         float64            `json:"kharis"`
		KharisStanding struct {
			Short string `json:"short"`
		} `json:"kharis_standing"`
		KharisMood       string                `json:"kharis_mood"`
		AvailablePrayers []riteAvailablePrayer `json:"available_prayers"`
		Name             string                `json:"name"`
		Food             struct {
			Goods              map[string]float64 `json:"goods"`
			ConsumptionPerTick float64            `json:"consumption_per_tick"`
		} `json:"food"`
	} `json:"settlement"`
}

// offerFoodText says what a gift takes from the city's food and whether a
// day's eating is left after it — same line as the web's offering row
// (byggplan A.3: Lato gave 28 of 29 grain and starved six days).
func offerFoodText(offering, food map[string]float64, eat float64, city string) string {
	if city == "" {
		city = "the city"
	}
	var parts []string
	var taken, total float64
	for _, g := range []string{"fish", "grain", "oil"} {
		total += food[g]
		if n := offering[g]; n > 0 {
			taken += n
			parts = append(parts, fmt.Sprintf("takes %.0f of %s's %.0f %s", n, city, math.Floor(food[g]), g))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	text := "The offering " + strings.Join(parts, ", ") + "."
	if left := math.Max(0, total-taken); eat > 0 && left < eat {
		text += fmt.Sprintf(" That leaves %.0f food, less than %s eats in a game day (%s).", math.Floor(left), city, strconv.FormatFloat(math.Round(eat*10)/10, 'f', -1, 64))
	}
	return text
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
		fmt.Fprintf(&b, "%s×%s ", g, strconv.FormatFloat(offering[g], 'f', -1, 64))
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
	var settlementID, provinceArg string
	var prayerID string
	var targetID string
	var stormArg, hexArg string
	var scope string
	var list bool
	var yes bool
	var offerMultiplier float64
	var offer string

	cmd := &cobra.Command{
		Use:   "rite",
		Short: "Perform a cultural prayer at your settlement (requires temple + offering)",
		Example: `  keryx rite --list
  keryx rite --prayer <prayer-id>
  keryx rite --prayer <prayer-id> --province Knossos
  keryx rite --prayer <prayer-id> --offer grain=10,silver=3
  keryx rite --prayer minoan_stillness_hull --scope unit --target "Horns of Crete"
  keryx rite --prayer minoan_stillness_hull --scope permanent --target "Horns of Crete"
  keryx rite --prayer minoan_velchanos_strength --scope realm
  keryx rite --prayer minoan_stillness_calm --storm "Old Grey"
  keryx rite --prayer minoan_stillness_wake --hex 12,-3
  keryx rite --prayer minoan_potnia_walls_fall --target "Mycenae"
  keryx rite --prayer minoan_asterion_bird_oracle --target "Mycenae"
  keryx rite --prayer <prayer-id> --yes
  keryx rite --json`,
		Args: rejectPositionalArgs("prayer"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)
			if provinceArg != "" {
				if settlementID != "" {
					return fmt.Errorf("give --province or --settlement, not both")
				}
				provs, err := c.get(fmt.Sprintf("/api/v1/worlds/%s/provinces", cfg.WorldID))
				if err != nil {
					return err
				}
				var all []map[string]any
				_ = json.Unmarshal(provs, &all)
				if settlementID = riteSettlementByProvince(all, provinceArg); settlementID == "" {
					return fmt.Errorf("no city of yours called or numbered %q — see keryx status", provinceArg)
				}
			}

			// --list: show this culture's available prayers (id + affordability) from
			// the province status endpoint, which exposes `available_prayers`. Works for
			// any culture — no hardcoded akhaier_* IDs.
			if list {
				return listRealmPrayers(c, settlementID)
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

			// Parse once: the confirmation and POST must describe the same gift.
			var composed map[string]float64
			if offer != "" {
				parsed, parseErr := parseOffering(offer)
				if parseErr != nil {
					return parseErr
				}
				composed = parsed
			}

			// A storm prayer names a storm (--storm) or a sea hex (--hex); both travel as "target".
			if stormArg != "" || hexArg != "" {
				if stormArg != "" && hexArg != "" || scope != "" || targetID != "" {
					return fmt.Errorf("give --storm or --hex alone, without --scope or --target")
				}
				if hexArg != "" {
					q, r, ok := strings.Cut(hexArg, ",")
					if _, e1 := strconv.Atoi(strings.TrimSpace(q)); !ok || e1 != nil {
						return fmt.Errorf("--hex wants q,r, for example --hex 12,-3")
					}
					if _, e2 := strconv.Atoi(strings.TrimSpace(r)); e2 != nil {
						return fmt.Errorf("--hex wants q,r, for example --hex 12,-3")
					}
					targetID = hexArg
				} else {
					view, err := loadRealmCult(c)
					if err != nil {
						return err
					}
					for _, p := range view.Prayers {
						if p.SettlementID == settlementID && p.ID == prayerID {
							if targetID, err = resolveRiteStorm(p.riteAvailablePrayer, stormArg); err != nil {
								return err
							}
							break
						}
					}
					if targetID == "" {
						targetID = stormArg
					}
				}
			}

			// A blessing prayer names its target: --scope unit|city|realm, and for unit a
			// --target (name or id) picked from the prayer's own list. Refused here, before
			// anything is offered, with the choices in the error.
			if (scope != "" || targetID != "") && stormArg == "" && hexArg == "" {
				view, err := loadRealmCult(c)
				if err != nil {
					return err
				}
				for _, p := range view.Prayers {
					if p.SettlementID != settlementID || p.Targets == nil || !(p.ID == prayerID || (prayerID == "" && p.Default)) {
						continue
					}
					if p.Targets.Kind == "place" {
						if scope != "" {
							return fmt.Errorf("%s takes --target <place> only, without --scope", p.Name)
						}
						if targetID, err = resolveRitePlace(p.riteAvailablePrayer, targetID); err != nil {
							return err
						}
						break
					}
					if p.Targets.Kind == "city" {
						if scope != "" {
							return fmt.Errorf("%s takes --target <city> only, without --scope", p.Name)
						}
						if targetID, err = resolveRiteCity(p.riteAvailablePrayer, targetID); err != nil {
							return err
						}
						break
					}
					if scope == "" {
						scope = "unit"
					}
					if p.Targets.Kind == "world" && p.ScopeCostFactors[scope] == 0 {
						return fmt.Errorf("%s cannot be said for --scope %q: choose %s (see rite --list)", p.Name, scope, worldScopes(p.ScopeCostFactors))
					}
					switch scope {
					case "unit", "permanent":
						if targetID == "" {
							_, err := resolveRiteTarget(p.riteAvailablePrayer, "")
							return fmt.Errorf("--scope %s needs --target <name|id>: %w", scope, err)
						}
						if targetID, err = resolveRiteTarget(p.riteAvailablePrayer, targetID); err != nil {
							return err
						}
					case "city", "realm":
						if targetID != "" {
							return fmt.Errorf("--target goes with --scope unit or permanent only")
						}
					default:
						return fmt.Errorf("--scope must be unit, city, realm or permanent")
					}
					break
				}
			}

			// Pre-cast confirmation (A7 core): shows mood + offer + effect + the miss-note
			// before the offering is spent. Skipped via --yes, --json, or when stdin isn't
			// a terminal — an unanswered prompt would otherwise hang the agent harness.
			autoConfirm := yes || jsonMode || !stdinIsTerminal()
			if !autoConfirm {
				confirmed, err := confirmRiteCast(c, markers, settlementID, prayerID, scope, offerMultiplier, composed)
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
			if scope != "" {
				body["target_scope"] = scope
			}
			if targetID != "" {
				body["target"] = targetID
			}
			if offerMultiplier != 0 {
				body["offer_multiplier"] = offerMultiplier
			}
			// Composed offering: bring what you judge worthy instead of the
			// prayer's inherited recipe. Worth is the fixed divine value times this
			// god's taste, weighed against the same god's traditional recipe —
			// so the old recipe still reads as exactly adequate.
			if offer != "" {
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
				RestDays   int            `json:"rest_game_days"`
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
				printSpedUp(resp.Effect)
				if resp.RestDays > 0 {
					fmt.Printf("The temple rests %s.\n", formatDays(resp.RestDays))
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
	cmd.Flags().StringVar(&provinceArg, "province", "", "city name or province ID, like status/place/build (default: your capital)")
	cmd.Flags().StringVar(&settlementID, "settlement", "", "settlement UUID (same as --province, kept for old scripts)")
	cmd.Flags().StringVar(&prayerID, "prayer", "", "prayer ID (run --list to see your culture's prayers; defaults to the culture's listed default prayer)")
	cmd.Flags().StringVar(&targetID, "target", "", "the unit, ship, Runner or caravan a blessing is laid on, or the foreign city whose wall Potnia is asked to bring down (minoan_potnia_walls_fall), or the city or named place Asterion's birds watch (minoan_asterion_bird_oracle), by name or id (see --list)")
	cmd.Flags().StringVar(&stormArg, "storm", "", "the named storm to still (minoan_stillness_calm), by name or id; see --list")
	cmd.Flags().StringVar(&hexArg, "hex", "", "the sea hex of your own map to wake a storm on (minoan_stillness_wake), as q,r")
	cmd.Flags().StringVar(&scope, "scope", "", "how wide a blessing reaches: unit | city | realm, or permanent (one named unit, for ever, ×20) — the wider, the dearer (see --list)")
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
func confirmRiteCast(c *Client, markers []map[string]any, settlementID, prayerID, scope string, offerMultiplier float64, composed map[string]float64) (bool, error) {
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
	if p.Settlement.KharisStanding.Short != "" {
		fmt.Println(p.Settlement.KharisStanding.Short)
	}
	fmt.Printf("Prayer: %s (%s)\n", chosen.Name, chosen.God)
	fmt.Printf("Effect: %s\n", chosen.Effect)
	if chosen.BaseDurationDays > 0 {
		fmt.Printf("Lasts %s with the traditional gift; a larger gift lasts longer.\n", formatDays(chosen.BaseDurationDays))
	}
	previewGift := chosen.Offering
	if f := chosen.ScopeCostFactors[scope]; f > 1 {
		previewGift = scaledOffering(previewGift, f)
		fmt.Printf("Target: %s — the gift is ×%d.\n", scope, f)
	}
	if len(composed) > 0 {
		previewGift = composed
	}
	fmt.Printf("Offer: %s\n", formatOffering(previewGift))
	if line := offerFoodText(previewGift, p.Settlement.Food.Goods, p.Settlement.Food.ConsumptionPerTick, p.Settlement.Name); line != "" {
		fmt.Println(line)
	}
	if len(composed) == 0 && chosen.GodID == "" && offerMultiplier != 0 && offerMultiplier != 1 {
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

// restText says how long the temple still rests and, when it lacks servants,
// that the rest stands still — "Temple of Potnia rests 3 game days — paused: 2
// of 4 servants". Without a rest field (legacy prayers) it is the plain wait.
func restText(pr riteAvailablePrayer) string {
	if pr.RestRemainingDays <= 0 {
		return gameDaysRemaining(pr.CooldownRemainingDays)
	}
	temple := pr.Temple
	if temple == "" {
		temple = "Temple"
	}
	text := fmt.Sprintf("%s rests %s", temple, formatDays(pr.RestRemainingDays))
	if pr.RestPaused {
		text += fmt.Sprintf(" — paused: %d of %d servants", pr.TempleStaffed, pr.TempleCapacity)
	}
	return text
}

func restOrWait(pr riteAvailablePrayer) string {
	if pr.RestRemainingDays > 0 {
		return restText(pr)
	}
	return gameDaysRemaining(pr.CooldownRemainingDays) + " wait"
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
			status += "; " + restOrWait(pr)
		}
		return "pending", status
	}
	if pr.ActiveRemainingDays > 0 {
		status := gameDaysRemaining(pr.ActiveRemainingDays) + " active"
		if pr.CooldownRemainingDays > 0 {
			status += "; " + restOrWait(pr)
		}
		return "active", status
	}
	if pr.CooldownRemainingDays > 0 {
		return "cooldown", restText(pr)
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
		return fmt.Sprintf("Required kharis: %.0f", prayer.MinKharis)
	}
	return "The offering is consumed even if the gods stay silent."
}

// riteSettlementByProvince resolves --province the way the other city verbs
// read it — city name, province ID or settlement ID — to one of your own
// settlements (GPT-player logs 2026-10-24: rite alone took --settlement).
func riteSettlementByProvince(markers []map[string]any, arg string) string {
	for _, m := range markers {
		if own, _ := m["own"].(bool); !own {
			continue
		}
		name, _ := m["name"].(string)
		pid, _ := m["id"].(string)
		sid, _ := m["settlement_id"].(string)
		if strings.EqualFold(name, arg) || pid == arg || sid == arg {
			return sid
		}
	}
	return ""
}

// worldScopes names the steps a world blessing offers, e.g. "city or realm".
func worldScopes(f map[string]int) string {
	var out []string
	for _, k := range []string{"city", "realm"} {
		if f[k] > 0 {
			out = append(out, k)
		}
	}
	return strings.Join(out, " or ")
}

// printSpedUp tells which journeys under way a speed prayer shortened and by how much.
func printSpedUp(effect map[string]any) {
	list, _ := effect["sped_up"].([]any)
	for _, raw := range list {
		m, _ := raw.(map[string]any)
		kind, _ := m["kind"].(string)
		id, _ := m["id"].(string)
		oldT, _ := m["old_arrive_tick"].(float64)
		newT, _ := m["new_arrive_tick"].(float64)
		if kind == "build" || kind == "ship" {
			// the Builder's Blessing: a build or a ship under construction (steg 5a)
			oldD, _ := m["old_due_tick"].(float64)
			newD, _ := m["new_due_tick"].(float64)
			what := map[string]string{"build": "Building", "ship": "Ship under construction"}[kind]
			if oldD > newD {
				fmt.Printf("  %s %s is ready %s sooner.\n", what, id, formatDays(int(oldD-newD)))
			} else {
				fmt.Printf("  %s %s is cut to three quarters of the time left.\n", what, id)
			}
			continue
		}
		what := map[string]string{"unit": "Ship", "transport": "Caravan", "messenger": "Runner"}[kind]
		if what == "" {
			what = kind
		}
		fmt.Printf("  %s %s arrives %s sooner.\n", what, id, formatDays(int(oldT-newT)))
	}
}
