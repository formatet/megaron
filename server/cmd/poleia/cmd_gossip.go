package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func gossipCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "gossip",
		Short: "Show recent rumours and events",
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)
			path := fmt.Sprintf("/api/v1/worlds/%s/gossip", cfg.WorldID)
			data, err := c.get(path)
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			var items []map[string]any
			if err := json.Unmarshal(data, &items); err != nil {
				return err
			}
			if len(items) == 0 {
				fmt.Println("No rumours yet.")
				return nil
			}
			for _, g := range items {
				region, _ := g["source_region"].(string)
				text, _ := g["text"].(string)
				tsStr, _ := g["generated_at"].(string)
				importance, _ := g["importance"].(string)
				hops, _ := g["hops"].(float64)
				var when string
				if t, err := time.Parse(time.RFC3339, tsStr); err == nil {
					ago := time.Since(t)
					switch {
					case ago < time.Hour:
						when = fmt.Sprintf("%dm ago", int(ago.Minutes()))
					case ago < 24*time.Hour:
						when = fmt.Sprintf("%dh ago", int(ago.Hours()))
					default:
						when = fmt.Sprintf("%dd ago", int(ago.Hours()/24))
					}
				}
				marker := " "
				if importance == "major" {
					marker = "!" // major rumor: falls a settlement, travels several hops
				}
				hopLabel := ""
				if hops > 0 {
					hopLabel = fmt.Sprintf(" (heard %d hop%s away)", int(hops), plural(int(hops)))
				}
				fmt.Printf("%s[%s]  %s%s\n  %s\n\n", marker, when, region, hopLabel, text)
			}
			return nil
		},
	}
}

// plural returns "s" unless n == 1 — used for "N hops away".
func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func messengerCmd() *cobra.Command {
	var destName, message, wantGood, fromName string
	var wantQty, offerSilver float64
	cmd := &cobra.Command{
		Use:   "messenger",
		Short: "Send a messenger to another settlement",
		Example: `  poleia messenger --to Korinth --message "Need grain urgently"
  poleia messenger --to Korinth --message "Buy grain offer" --want-good grain --want-qty 100 --offer-silver 80`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)

			// Load visible provinces to resolve own settlement ID and destination.
			provinces, err := c.get(fmt.Sprintf("/api/v1/worlds/%s/provinces", cfg.WorldID))
			if err != nil {
				return err
			}
			var markers []map[string]any
			if err := json.Unmarshal(provinces, &markers); err != nil {
				return err
			}
			// Fall back to the public wanaxes list when the destination is not yet
			// visible in our fog-of-war provinces.
			var wanaxes []map[string]any
			if wdata, werr := c.get(fmt.Sprintf("/api/v1/worlds/%s/wanaxes", cfg.WorldID)); werr == nil {
				_ = json.Unmarshal(wdata, &wanaxes)
			}

			destID, destSettleName, ownSettlementID, err := resolveMessengerDest(markers, wanaxes, destName, fromName)
			if err != nil {
				return err
			}

			body := map[string]any{
				"destination_id": destID,
				"message":        message,
			}
			if wantGood != "" && wantQty > 0 && offerSilver > 0 {
				body["trade_offer"] = map[string]any{
					"want_good":    wantGood,
					"want_qty":     wantQty,
					"offer_silver": offerSilver,
				}
			}

			path := fmt.Sprintf("/api/v1/worlds/%s/settlements/%s/messengers", cfg.WorldID, ownSettlementID)
			data, err := c.post(path, body)
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			var resp map[string]any
			json.Unmarshal(data, &resp)
			arrivesAt, _ := resp["arrives_at"].(string)
			if wantGood != "" {
				fmt.Printf("Trade offer dispatched to %s (want %.0f %s, offer %.0f silver) · arrives %s\n",
					destSettleName, wantQty, wantGood, offerSilver, arrivesAt)
			} else {
				fmt.Printf("Messenger dispatched to %s · arrives %s\n", destSettleName, arrivesAt)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&destName, "to", "t", "", "destination settlement name (required)")
	cmd.Flags().StringVarP(&message, "message", "m", "", "message text (required)")
	cmd.Flags().StringVar(&fromName, "from", "", "your city to send from (default: capital — the silver hub)")
	cmd.Flags().StringVar(&wantGood, "want-good", "", "good to request (e.g. grain, cedar)")
	cmd.Flags().Float64Var(&wantQty, "want-qty", 0, "quantity of good to request")
	cmd.Flags().Float64Var(&offerSilver, "offer-silver", 0, "silver to offer in exchange")
	_ = cmd.MarkFlagRequired("to")
	_ = cmd.MarkFlagRequired("message")
	return cmd
}

// resolveMessengerDest finds the caller's own settlement ID plus the destination
// settlement ID for destName. It looks in the fog-of-war province markers first
// (where own settlements are flagged "own": true), then falls back to the public
// wanaxes list. Messengers cannot be sent to one's own settlement — that case is
// rejected up front with an actionable error so the agent picks a real neighbour
// instead of bouncing off the server's 400.
func resolveMessengerDest(markers, wanaxes []map[string]any, destName, fromName string) (destID, resolvedName, ownID string, err error) {
	if strings.TrimSpace(destName) == "" {
		return "", "", "", fmt.Errorf("destination name is empty — use --to <settlement name>")
	}
	// Pick the origin city deterministically. Default is the capital (the marker
	// whose province id matches cfg.ProvinceID) — it holds the silver by the
	// self-sufficiency invariant, so buy-offers pass the buyer-solvency check.
	// --from <city> overrides it. Previously this loop kept the LAST own marker,
	// so offers shipped from an arbitrary (often silver-less) colony and the
	// recipient's accept failed with "buyer has insufficient silver".
	var capitalID, firstOwnID, fromID string
	for _, m := range markers {
		if own, _ := m["own"].(bool); own {
			sid, _ := m["settlement_id"].(string)
			if sid != "" {
				if firstOwnID == "" {
					firstOwnID = sid
				}
				if pid, _ := m["id"].(string); pid == cfg.ProvinceID {
					capitalID = sid
				}
				if fromName != "" {
					if n, _ := m["name"].(string); strings.EqualFold(n, fromName) {
						fromID = sid
					}
				}
			}
		}
		if n, _ := m["name"].(string); strings.EqualFold(n, destName) {
			destID, _ = m["settlement_id"].(string)
			resolvedName = n
		}
	}
	switch {
	case fromName != "":
		if fromID == "" {
			return "", "", "", fmt.Errorf("you have no settlement named %q to send from — pick one of your own cities (see `settlements`)", fromName)
		}
		ownID = fromID
	case capitalID != "":
		ownID = capitalID
	default:
		ownID = firstOwnID
	}
	if ownID == "" {
		return "", "", "", fmt.Errorf("could not find own settlement")
	}
	if destID == "" {
		for _, w := range wanaxes {
			if n, _ := w["name"].(string); strings.EqualFold(n, destName) {
				destID, _ = w["settlement_id"].(string)
				resolvedName = n
				break
			}
		}
	}
	if destID == "" {
		// P9: --to only ever matches a SETTLEMENT name. A Wanax (ruler) name looks
		// just as plausible to type — it's the column right next to the city name
		// in `cities`/`diplomacy` — but silently fails the same generic lookup.
		// Detect that specific mix-up and name the actual city(s) to use instead.
		if cities := settlementsRuledBy(wanaxes, destName); len(cities) > 0 {
			return "", "", "", fmt.Errorf("%q is a Wanax (ruler) name, not a settlement — messengers go to a CITY. "+
				"%s rules: %s — try --to %s", destName, destName, strings.Join(cities, ", "), cities[0])
		}
		return "", "", "", fmt.Errorf("no settlement named %q in view — run 'cities' to list reachable settlements; only known (not rumour-only) rows can be traded with, so expand to discover more", destName)
	}
	if destID == ownID {
		return "", "", "", fmt.Errorf("%q is your own settlement — messengers go to other Wanaxes; pick a neighbour from `cities` (rows without ★, knowledge=known) or scout to discover new settlements", destName)
	}
	return destID, resolvedName, ownID, nil
}

// resolveDestByName is the destination half of resolveMessengerDest, for a
// sender with no own settlement: the nomadic host (founder phase). Same lookup
// order — visible province markers first, then the public wanaxes list.
func resolveDestByName(markers, wanaxes []map[string]any, destName string) (destID, resolvedName string, err error) {
	if strings.TrimSpace(destName) == "" {
		return "", "", fmt.Errorf("destination name is empty — use --to <settlement name>")
	}
	for _, m := range markers {
		if n, _ := m["name"].(string); strings.EqualFold(n, destName) {
			destID, _ = m["settlement_id"].(string)
			resolvedName = n
		}
	}
	if destID == "" {
		for _, w := range wanaxes {
			if n, _ := w["name"].(string); strings.EqualFold(n, destName) {
				destID, _ = w["settlement_id"].(string)
				resolvedName = n
				break
			}
		}
	}
	if destID == "" {
		// P9: same Wanax-name/settlement-name mix-up as resolveMessengerDest.
		if cities := settlementsRuledBy(wanaxes, destName); len(cities) > 0 {
			return "", "", fmt.Errorf("%q is a Wanax (ruler) name, not a settlement — messengers go to a CITY. "+
				"%s rules: %s — try --to %s", destName, destName, strings.Join(cities, ", "), cities[0])
		}
		return "", "", fmt.Errorf("no settlement named %q in view — wander closer (the host's march sweeps fog) and check 'poleia cities'", destName)
	}
	return destID, resolvedName, nil
}

// settlementsRuledBy returns the (sorted, deduplicated) settlement names in
// wanaxes owned by the Wanax (ruler/username) called wanaxName — used to turn
// "message --to <king name>" (a settlement lookup miss) into an actionable
// hint naming the actual cities that ruler holds, instead of a bare "not
// found" that leaves the mix-up invisible.
func settlementsRuledBy(wanaxes []map[string]any, wanaxName string) []string {
	seen := map[string]bool{}
	var cities []string
	for _, w := range wanaxes {
		owner, _ := w["owner"].(string)
		if !strings.EqualFold(owner, wanaxName) {
			continue
		}
		name, _ := w["name"].(string)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		cities = append(cities, name)
	}
	sort.Strings(cities)
	return cities
}
