package main

import (
	"encoding/json"
	"fmt"
	"github.com/spf13/cobra"
	"sort"
	"strings"
)

// The existing Cities directory owns knowledge. Rumours never supply targets.
type destinationCity struct {
	ID        string `json:"settlement_id"`
	Name      string `json:"name"`
	Owner     string `json:"owner"`
	Own       bool   `json:"own"`
	Knowledge string `json:"knowledge"`
	Q         *int   `json:"q"`
	R         *int   `json:"r"`
}

func destinationCities(c *Client, worldID string) ([]destinationCity, error) {
	data, err := c.get("/api/v1/worlds/" + worldID + "/cities")
	if err != nil {
		return nil, err
	}
	var all []destinationCity
	if err := json.Unmarshal(data, &all); err != nil {
		return nil, err
	}
	cities := make([]destinationCity, 0, len(all))
	for _, city := range all {
		if city.Knowledge == "known" && city.Q != nil && city.R != nil && city.Name != "" {
			cities = append(cities, city)
		}
	}
	sort.SliceStable(cities, func(i, j int) bool {
		a, b := cities[i], cities[j]
		if a.Own != b.Own {
			return a.Own
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.ID < b.ID
	})
	return cities, nil
}
func resolveCityTarget(cmd *cobra.Command, c *Client, to, target string, q, r int) (int, int, string, bool, error) {
	if cmd.Flags().Changed("to") {
		if cmd.Flags().Changed("target") || cmd.Flags().Changed("q") || cmd.Flags().Changed("r") {
			return 0, 0, "", false, fmt.Errorf("give the target as either --to city, --target q,r or --q/--r")
		}
		name := strings.TrimSpace(to)
		if name == "" {
			return 0, 0, "", false, fmt.Errorf("--to needs a city name or q,r")
		}
		if strings.Contains(name, ",") {
			if hq, hr, perr := parseQR(name); perr == nil { // --to 5,-3 is a hex
				return hq, hr, "", true, nil
			}
		}
		cities, err := destinationCities(c, cfg.WorldID)
		if err != nil {
			return 0, 0, "", false, fmt.Errorf("could not load known cities: %w", err)
		}
		var matches []destinationCity
		for _, city := range cities {
			if strings.EqualFold(city.Name, name) {
				matches = append(matches, city)
			}
		}
		if len(matches) == 0 {
			return 0, 0, "", false, fmt.Errorf("you know no city called %q", name)
		}
		if len(matches) > 1 {
			labels := make([]string, 0, len(matches))
			for _, city := range matches {
				owner := city.Owner
				if city.Own {
					owner = "yours"
				}
				labels = append(labels, fmt.Sprintf("%s — %s (%d,%d)", city.Name, owner, *city.Q, *city.R))
			}
			return 0, 0, "", false, fmt.Errorf("city name %q is ambiguous: %s; use --target q,r", name, strings.Join(labels, "; "))
		}
		city := matches[0]
		return *city.Q, *city.R, city.Name, true, nil
	}
	tq, tr, given, err := resolveTargetHex(cmd, target, q, r)
	return tq, tr, "", given, err
}

// Receipt lookup is best effort after acceptance: a directory outage cannot
// turn a sent order into a reported failure. --to already supplied its name.
func cityTargetLabel(c *Client, q, r int, name string) string {
	if name == "" {
		if cities, err := destinationCities(c, cfg.WorldID); err == nil {
			for _, city := range cities {
				if *city.Q == q && *city.R == r {
					name = city.Name
					break
				}
			}
		}
	}
	if name != "" {
		return fmt.Sprintf("%s (%d,%d)", name, q, r)
	}
	return fmt.Sprintf("(%d,%d)", q, r)
}

// yourCitiesHint names the Wanax's own cities for a "no such city" error, so a
// wrong name answers with the right ones. Best effort: a failed read adds nothing.
func yourCitiesHint(c *Client, worldID string) string {
	cities, err := destinationCities(c, worldID)
	if err != nil {
		return ""
	}
	var names []string
	for _, city := range cities {
		if city.Own {
			names = append(names, city.Name)
		}
	}
	if len(names) == 0 {
		return ""
	}
	return " — your cities: " + strings.Join(names, ", ")
}

// addCityFlag is the ONE documented way to name the city a command acts in:
// `--city <name-or-id>`. It binds to the same variable as the command's older
// flag(s) (--province, --from, --settlement), which keep working but are
// hidden. withPositional also lets the name be given as the single positional
// argument ("keryx goods Phaistos"); use it only where the command has no other
// positional and no destination city that the bare name could be mistaken for.
func addCityFlag(cmd *cobra.Command, target *string, withPositional bool, hide ...string) {
	usage := "city name or id (default: your capital)"
	cmd.Flags().StringVar(target, "city", "", usage)
	for _, name := range hide {
		if f := cmd.Flags().Lookup(name); f != nil {
			f.Hidden = true
		}
	}
	if !withPositional {
		return
	}
	cmd.Args = cobra.MaximumNArgs(1)
	orig := cmd.RunE
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if len(args) == 1 {
			if *target != "" && *target != args[0] {
				return fmt.Errorf("give the city once: either %q or --city %q", args[0], *target)
			}
			*target = args[0]
			args = nil
		}
		return orig(cmd, args)
	}
}

// hideHexFlags keeps --q/--r/--target working but leaves them out of --help:
// `--to <city|q,r>` is the one documented destination form.
func hideHexFlags(cmd *cobra.Command) {
	for _, name := range []string{"q", "r", "target"} {
		if f := cmd.Flags().Lookup(name); f != nil {
			f.Hidden = true
		}
	}
}
