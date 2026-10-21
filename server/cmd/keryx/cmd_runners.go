package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// runnersCmd is the Diplomacy → Runners tab for keryx: the same server read
// (GET /worlds/{id}/runners), the same groups and fields.
func runnersCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "runners",
		Short: "Where your runners are, and the other Wanaxes' runners you know of",
		Args:  noPositionalArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			data, err := newClient(cfg).get(fmt.Sprintf("/api/v1/worlds/%s/runners", cfg.WorldID))
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			out, err := renderRunners(data)
			if err != nil {
				return err
			}
			fmt.Print(out)
			return nil
		},
	}
}

type runnersResp struct {
	Own []struct {
		ID                string  `json:"id"`
		Group             string  `json:"group"`
		OrderSummary      string  `json:"order_summary"`
		ToCity            string  `json:"to_city"`
		ToWanax           string  `json:"to_wanax"`
		ArrivesAt         *string `json:"arrives_at"`
		PortName          string  `json:"port_name"`
		WaitingSince      *string `json:"waiting_since"`
		Explanation       string  `json:"explanation"`
		LeavesAt          *string `json:"leaves_at"`
		ShipName          string  `json:"ship_name"`
		ShipWanax         string  `json:"ship_wanax"`
		BoardedAt         *string `json:"boarded_at"`
		LandsAt           *string `json:"lands_at"`
		CanCallBack       bool    `json:"can_call_back"`
		CanArrangePassage bool    `json:"can_arrange_passage"`
		EligibleShips     []struct {
			ID             string `json:"id"`
			Name           string `json:"name"`
			SettlementName string `json:"settlement_name"`
		} `json:"eligible_ships"`
	} `json:"own"`
	Foreign []struct {
		Group       string  `json:"group"`
		SenderWanax string  `json:"sender_wanax"`
		Where       string  `json:"where"`
		WaitingFor  string  `json:"waiting_for"`
		Since       *string `json:"since"`
		ShipName    string  `json:"ship_name"`
		HeadingTo   string  `json:"heading_to"`
		LandsAt     *string `json:"lands_at"`
	} `json:"foreign"`
}

func possessive(name string) string {
	if strings.HasSuffix(name, "s") {
		return name + "'"
	}
	return name + "'s"
}

func runnerTime(s *string) string {
	if s == nil {
		return ""
	}
	t, err := time.Parse(time.RFC3339, *s)
	if err != nil {
		return ""
	}
	return clockTime(t)
}

// renderRunners prints /runners grouped exactly as the web tab does.
func renderRunners(data []byte) (string, error) {
	var resp runnersResp
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", err
	}
	if len(resp.Own) == 0 && len(resp.Foreign) == 0 {
		return "No runners on the road, and none of anyone else's that you know of.\n", nil
	}
	var b strings.Builder
	titles := []struct{ group, title string }{
		{"waiting", "WAITING FOR A SHIP"}, {"on_the_way", "ON THE WAY"}, {"aboard", "ABOARD"},
		{"awaiting_answer", "WAITING FOR AN ANSWER"}, {"homebound", "HOMEBOUND"}, {"no_word", "NO WORD"},
	}
	for _, g := range titles {
		first := true
		for _, m := range resp.Own {
			if m.Group != g.group {
				continue
			}
			if first {
				fmt.Fprintf(&b, "%s\n", g.title)
				first = false
			}
			to := m.OrderSummary
			if to == "" && m.ToCity != "" {
				to = "→ " + m.ToCity
				if m.ToWanax != "" {
					to += " (Wanax " + m.ToWanax + ")"
				}
			}
			switch g.group {
			case "waiting":
				fmt.Fprintf(&b, "  %s  in %s since %s  id:%s\n", to, m.PortName, runnerTime(m.WaitingSince), m.ID)
				if m.Explanation != "" {
					fmt.Fprintf(&b, "    %s\n", m.Explanation)
				}
				if m.CanArrangePassage {
					if len(m.EligibleShips) == 0 {
						fmt.Fprintf(&b, "    No ship of yours is free to carry it yet.\n")
					}
					for _, s := range m.EligibleShips {
						fmt.Fprintf(&b, "    keryx passage --id %s --ship %s    (%s, in %s)\n", m.ID, s.ID, s.Name, s.SettlementName)
					}
				}
				if m.CanCallBack {
					fmt.Fprintf(&b, "    keryx call-back --id %s\n", m.ID)
				}
			case "aboard":
				fmt.Fprintf(&b, "  %s  aboard Wanax %s %s, since %s; the ship makes landfall %s; arrives %s\n", to,
					possessive(m.ShipWanax), m.ShipName, runnerTime(m.BoardedAt), runnerTime(m.LandsAt), runnerTime(m.ArrivesAt))
			case "awaiting_answer":
				line := fmt.Sprintf("  %s  since %s", to, runnerTime(m.WaitingSince))
				if m.LeavesAt != nil {
					line += "; heads home " + runnerTime(m.LeavesAt) + " if unanswered"
				}
				b.WriteString(line + "\n")
			case "homebound":
				fmt.Fprintf(&b, "  %s  home %s\n", to, runnerTime(m.ArrivesAt))
			case "no_word":
				fmt.Fprintf(&b, "  no word has come from it\n")
			default:
				fmt.Fprintf(&b, "  %s  arrives %s\n", to, runnerTime(m.ArrivesAt))
			}
		}
		if !first {
			b.WriteString("\n")
		}
	}
	foreignTitles := []struct{ group, title string }{{"in_your_city", "IN YOUR CITIES"}, {"aboard_your_ship", "ABOARD YOUR SHIPS"}}
	for _, g := range foreignTitles {
		first := true
		for _, m := range resp.Foreign {
			if m.Group != g.group {
				continue
			}
			if first {
				fmt.Fprintf(&b, "%s\n", g.title)
				first = false
			}
			who := "Wanax " + possessive(m.SenderWanax) + " runner"
			if g.group == "in_your_city" {
				wait := "waiting for your answer"
				if m.WaitingFor == "a_ship" {
					wait = "waiting for a ship"
				}
				fmt.Fprintf(&b, "  %s in %s, %s\n", who, m.Where, wait)
			} else {
				line := fmt.Sprintf("  %s aboard %s, landfall %s", who, m.ShipName, runnerTime(m.LandsAt))
				if m.HeadingTo != "" {
					line += " at " + m.HeadingTo
				}
				b.WriteString(line + "\n")
			}
		}
		if !first {
			b.WriteString("\n")
		}
	}
	return b.String(), nil
}
