package main

import (
	"encoding/json"
	"fmt"
	"github.com/spf13/cobra"
	"net/url"
	"sort"
	"strings"
	"time"
)

type letterPerson struct {
	ID          string     `json:"counterpart_id"`
	Name        string     `json:"counterpart_name"`
	Preview     string     `json:"preview"`
	State       string     `json:"state"`
	ReplyBefore *time.Time `json:"reply_before"`
}

func lettersState(state string) string {
	switch state {
	case "waiting_you":
		return "Waiting on you"
	case "waiting_them":
		return "Waiting on them"
	default:
		return "Done"
	}
}
func lettersCmd() *cobra.Command {
	return &cobra.Command{Use: "letters [wanax]", Short: "Read letters by Wanax", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		c := newClient(cfg)
		path := fmt.Sprintf("/api/v1/worlds/%s/letters", cfg.WorldID)
		data, err := c.get(path)
		if err != nil {
			return err
		}
		var page struct {
			Items []letterPerson `json:"items"`
		}
		if err = json.Unmarshal(data, &page); err != nil {
			return err
		}
		if len(args) == 0 {
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			for _, p := range page.Items {
				fmt.Printf("%s · %s", p.Name, lettersState(p.State))
				if p.ReplyBefore != nil {
					fmt.Printf(" · before %s", clockTime(*p.ReplyBefore))
				}
				fmt.Printf("\n  %s\n", p.Preview)
			}
			if len(page.Items) == 0 {
				fmt.Println("No letters yet.")
			}
			return nil
		}
		var matches []letterPerson
		for _, p := range page.Items {
			if p.ID == args[0] || strings.EqualFold(p.Name, args[0]) {
				matches = append(matches, p)
			}
		}
		if len(matches) == 0 {
			return fmt.Errorf("no letters with Wanax %q", args[0])
		}
		if len(matches) > 1 {
			return fmt.Errorf("more than one Wanax named %q; use their ID from letters --json", args[0])
		}
		data, err = c.get(path + "/" + url.PathEscape(matches[0].ID))
		if err != nil {
			return err
		}
		if jsonMode {
			printRawJSON(data)
			return nil
		}
		var thread struct {
			Name  string `json:"counterpart_name"`
			Items []struct {
				ID          string             `json:"id"`
				Sender      string             `json:"sender_name"`
				Direction   string             `json:"direction"`
				Message     string             `json:"message"`
				Reply       string             `json:"reply_text"`
				SentAt      time.Time          `json:"sent_at"`
				State       string             `json:"offer_state"`
				Actions     []string           `json:"actions"`
				ReplyBefore *time.Time         `json:"reply_before"`
				Missing     string             `json:"missing_payment"`
				Goods       map[string]float64 `json:"goods"`
				Offer       *struct {
					CounterOfferID string  `json:"counter_offer_id"`
					Kind           string  `json:"kind"`
					OfferGood      string  `json:"offer_good"`
					OfferQty       float64 `json:"offer_qty"`
					WantGood       string  `json:"want_good"`
					WantQty        float64 `json:"want_qty"`
					WantSilver     float64 `json:"want_silver"`
					OfferSilver    float64 `json:"offer_silver"`
				} `json:"trade_offer"`
			} `json:"items"`
		}
		if err = json.Unmarshal(data, &thread); err != nil {
			return err
		}
		fmt.Println(thread.Name)
		for _, m := range thread.Items {
			who := m.Sender
			if m.Direction == "outgoing" {
				who = "You"
			}
			fmt.Printf("\n%s · %s\n%s\n", who, clockTime(m.SentAt), m.Message)
			goods := make([]string, 0, len(m.Goods))
			for key := range m.Goods {
				goods = append(goods, key)
			}
			sort.Strings(goods)
			for _, key := range goods {
				fmt.Printf("%g %s\n", m.Goods[key], key)
			}
			if m.Reply != "" {
				fmt.Printf("Reply: %s\n", m.Reply)
			}
			if o := m.Offer; o != nil {
				prefix := "Offer"
				if o.CounterOfferID != "" {
					fmt.Printf("Counter-offer to %s\n", o.CounterOfferID)
				}
				if m.State == "accepted" || m.State == "returned" {
					prefix = "Agreed"
				} else if m.State != "pending" && m.State != "" {
					prefix = m.State
				}
				if o.Kind == "sell" {
					fmt.Printf("%s: %g %s for %g silver\n", prefix, o.OfferQty, o.OfferGood, o.WantSilver)
				} else {
					fmt.Printf("%s: %g %s for %g silver\n", prefix, o.WantQty, o.WantGood, o.OfferSilver)
				}
			}
			if m.Missing != "" {
				fmt.Println(m.Missing)
			}
			for _, a := range m.Actions {
				switch a {
				case "reply":
					fmt.Printf("Send with their runner")
					if m.ReplyBefore != nil {
						fmt.Printf(" (leaves %s)", clockTime(*m.ReplyBefore))
					}
					fmt.Printf(": keryx reply --id %s --text \"...\"\n", m.ID)
				case "trade_accept":
					fmt.Printf("Accept: keryx trade-accept --id %s\n", m.ID)
				case "trade_counter":
					if m.Offer != nil {
						if m.Offer.Kind == "sell" {
							fmt.Printf("Counter-offer: keryx trade-offer --counter %s --want-good %s --want-qty %g --offer-silver %g\n", m.ID, m.Offer.OfferGood, m.Offer.OfferQty, m.Offer.WantSilver)
						} else {
							fmt.Printf("Counter-offer: keryx trade-offer --counter %s --offer-good %s --offer-qty %g --want-silver %g\n", m.ID, m.Offer.WantGood, m.Offer.WantQty, m.Offer.OfferSilver)
						}
					}
				case "trade_decline":
					fmt.Printf("Decline: keryx trade-decline --id %s\n", m.ID)
				case "trade_cancel":
					fmt.Printf("Cancel: keryx trade-cancel --id %s\n", m.ID)
				}
			}
		}
		return nil
	}}
}
