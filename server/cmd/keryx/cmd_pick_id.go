package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Ids are optional where there is only one thing to act on (order-keryx-llm
// step 3). pickID resolves a command's --id:
//   - a full id is trusted as before;
//   - a short unique prefix is expanded against the candidates;
//   - no id: exactly one candidate -> use it and say which; none -> say so;
//     several -> print them as ready command lines and stop. Never guesses.
type idCandidate struct {
	ID    string
	Label string
	Extra string // verb-specific payload (e.g. the province a queue item lives in)
}

func isFullID(s string) bool { return len(s) == 36 && strings.Count(s, "-") == 4 }

// looksLikeIDPrefix: only hex digits and dashes can be the start of a UUID; anything
// else is passed through to the server exactly as before.
func looksLikeIDPrefix(s string) bool {
	for _, r := range strings.ToLower(s) {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f' || r == '-') {
			return false
		}
	}
	return true
}

func pickID(verb, what, given, suffix string, load func() ([]idCandidate, error)) (idCandidate, error) {
	if isFullID(given) || (given != "" && !looksLikeIDPrefix(given)) {
		return idCandidate{ID: given}, nil
	}
	cands, err := load()
	if given != "" {
		if err != nil {
			return idCandidate{ID: given}, nil
		}
		var hit []idCandidate
		for _, cd := range cands {
			if strings.HasPrefix(strings.ToLower(cd.ID), strings.ToLower(given)) {
				hit = append(hit, cd)
			}
		}
		switch len(hit) {
		case 0:
			return idCandidate{ID: given}, nil // let the server judge it
		case 1:
			return hit[0], nil
		}
		return idCandidate{}, ambiguousIDs(verb, what, given, suffix, hit)
	}
	if err != nil {
		return idCandidate{}, err
	}
	switch len(cands) {
	case 0:
		return idCandidate{}, fmt.Errorf("there is no %s to %s", what, verb)
	case 1:
		note := fmt.Sprintf("Using the only %s: %s (id %s)", what, cands[0].Label, cands[0].ID)
		if jsonMode {
			fmt.Fprintln(os.Stderr, note)
		} else {
			fmt.Println(note)
		}
		return cands[0], nil
	}
	return idCandidate{}, ambiguousIDs(verb, what, "", suffix, cands)
}

func ambiguousIDs(verb, what, given, suffix string, cands []idCandidate) error {
	var b strings.Builder
	if given != "" {
		fmt.Fprintf(&b, "%q starts %d %ss — pick one:", given, len(cands), what)
	} else {
		fmt.Fprintf(&b, "%d %ss — pick one:", len(cands), what)
	}
	for _, cd := range cands {
		fmt.Fprintf(&b, "\n  keryx %s --id %s%s  # %s", verb, uniqueShortID(cd.ID, cands), suffix, cd.Label)
	}
	return fmt.Errorf("%s", b.String())
}

// uniqueShortID is the first 8 characters unless that is not unique among the candidates.
func uniqueShortID(id string, cands []idCandidate) string {
	if len(id) <= 8 {
		return id
	}
	for _, cd := range cands {
		if cd.ID != id && strings.HasPrefix(cd.ID, id[:8]) {
			return id
		}
	}
	return id[:8]
}

// --- candidate loaders -------------------------------------------------------

func offerLabel(m map[string]any, offer map[string]any, incoming bool) string {
	name, _ := m["from_name"].(string)
	prep := "from"
	if !incoming {
		name, _ = m["destination_name"].(string)
		prep = "to"
	}
	var good string
	var qty, silver float64
	if kind, _ := offer["kind"].(string); kind == "sell" {
		good, _ = offer["offer_good"].(string)
		qty, _ = offer["offer_qty"].(float64)
		silver, _ = offer["want_silver"].(float64)
	} else {
		good, _ = offer["want_good"].(string)
		qty, _ = offer["want_qty"].(float64)
		silver, _ = offer["offer_silver"].(float64)
	}
	return fmt.Sprintf("%.0f %s for %.0f silver %s %s", qty, good, silver, prep, name)
}

func pendingOffer(offer map[string]any) bool {
	st, has := offer["status"].(string)
	return !has || st == "pending"
}

func inboxCandidates(c *Client, wantOffer bool) ([]idCandidate, error) {
	data, err := c.get(fmt.Sprintf("/api/v1/worlds/%s/messengers/inbox", cfg.WorldID))
	if err != nil {
		return nil, err
	}
	var msgs []map[string]any
	if err := json.Unmarshal(data, &msgs); err != nil {
		return nil, err
	}
	var out []idCandidate
	for _, m := range msgs {
		id, _ := m["id"].(string)
		if id == "" {
			continue
		}
		offer, hasOffer := m["trade_offer"].(map[string]any)
		if wantOffer {
			if hasOffer && pendingOffer(offer) {
				out = append(out, idCandidate{ID: id, Label: offerLabel(m, offer, true)})
			}
			continue
		}
		if hasOffer || m["reply_window_closed"] == true {
			continue
		}
		from, _ := m["from_name"].(string)
		text, _ := m["message"].(string)
		if r := []rune(text); len(r) > 40 {
			text = string(r[:40]) + "…"
		}
		out = append(out, idCandidate{ID: id, Label: fmt.Sprintf("%q from %s", text, from)})
	}
	return out, nil
}

func outboxCandidates(c *Client, keep func(m map[string]any) (string, bool)) ([]idCandidate, error) {
	msgs, err := fetchOutbox(c)
	if err != nil {
		return nil, err
	}
	var out []idCandidate
	for _, m := range msgs {
		id, _ := m["id"].(string)
		if label, ok := keep(m); ok && id != "" {
			out = append(out, idCandidate{ID: id, Label: label})
		}
	}
	return out, nil
}

func outboxPendingOffer(m map[string]any) (string, bool) {
	if offer, ok := m["trade_offer"].(map[string]any); ok && pendingOffer(offer) {
		return offerLabel(m, offer, false), true
	}
	return "", false
}

func outboxWaitingRunner(flag string) func(m map[string]any) (string, bool) {
	return func(m map[string]any) (string, bool) {
		if ps, _ := m["passage_status"].(string); ps != "awaiting_passage" {
			return "", false
		}
		if ok, _ := m[flag].(bool); !ok {
			return "", false
		}
		dest, _ := m["destination_name"].(string)
		port, _ := m["passage_port"].(string)
		label := "runner to " + dest
		if port != "" {
			label += " waiting in " + port
		}
		return label, true
	}
}

func standingOrderCandidates(c *Client, status string) ([]idCandidate, error) {
	data, err := c.get(fmt.Sprintf("/api/v1/worlds/%s/standing-orders", cfg.WorldID))
	if err != nil {
		return nil, err
	}
	var orders []map[string]any
	if err := json.Unmarshal(data, &orders); err != nil {
		return nil, err
	}
	var out []idCandidate
	for _, o := range orders {
		id, _ := o["id"].(string)
		st, _ := o["status"].(string)
		if id == "" || (status != "" && st != status) {
			continue
		}
		out = append(out, idCandidate{ID: id, Label: fmt.Sprintf("%v → %v [%s]", o["from_name"], o["to_name"], st)})
	}
	return out, nil
}
