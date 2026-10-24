package main

import (
	"encoding/json"
	"fmt"
	"github.com/spf13/cobra"
	"net/url"
)

func correspondenceCmd() *cobra.Command {
	var caseID, query, cursor string
	var limit int
	cmd := &cobra.Command{Use: "correspondence", Short: "Read your personal case archive", Args: noPositionalArgs(), RunE: func(cmd *cobra.Command, _ []string) error {
		if limit < 1 || limit > 100 {
			return fmt.Errorf("--limit must be between 1 and 100")
		}
		if caseID != "" && query != "" {
			return fmt.Errorf("--case and --search cannot be combined")
		}
		params := url.Values{"limit": {fmt.Sprint(limit)}}
		if query != "" {
			params.Set("q", query)
		}
		if cursor != "" {
			params.Set("cursor", cursor)
		}
		path := fmt.Sprintf("/api/v1/worlds/%s/correspondence/cases", cfg.WorldID)
		if caseID != "" {
			path += "/" + url.PathEscape(caseID)
		}
		data, err := newClient(cfg).get(path + "?" + params.Encode())
		if err != nil {
			return err
		}
		if jsonMode {
			printRawJSON(data)
			return nil
		}
		var page struct {
			Subject string `json:"subject"`
			Items   []struct {
				ID              string          `json:"id"`
				Subject         string          `json:"subject"`
				CounterpartName string          `json:"counterpart_name"`
				CityName        string          `json:"city_name"`
				Preview         string          `json:"preview"`
				LatestAt        string          `json:"latest_at"`
				LetterCount     int             `json:"letter_count"`
				SenderName      string          `json:"sender_name"`
				RecipientName   string          `json:"recipient_name"`
				OriginName      string          `json:"origin_name"`
				DestinationName string          `json:"destination_name"`
				Message         string          `json:"message_text"`
				Reply           string          `json:"reply_text"`
				SentAt          string          `json:"sent_at"`
				TradeOffer      json.RawMessage `json:"trade_offer"`
			} `json:"items"`
			Next string `json:"next_cursor"`
		}
		if err := json.Unmarshal(data, &page); err != nil {
			return err
		}
		fmt.Println("Personal archive · historical names · listing does not mark letters read.")
		if caseID != "" {
			fmt.Println(page.Subject)
		}
		for _, item := range page.Items {
			if caseID == "" {
				fmt.Printf("%s · %s · %s\n  %s\n  %d letters · case %s\n", item.CounterpartName, item.CityName, item.Subject, item.Preview, item.LetterCount, item.ID)
			} else {
				fmt.Printf("%s · %s → %s · %s\n%s\n", item.SenderName, item.OriginName, item.RecipientName, item.DestinationName, item.Message)
				if item.Reply != "" {
					fmt.Printf("Reply: %s\n", item.Reply)
				}
				if len(item.TradeOffer) > 0 && string(item.TradeOffer) != "null" {
					fmt.Printf("Recorded offer: %s\n", item.TradeOffer)
				}
			}
		}
		if len(page.Items) == 0 {
			fmt.Println("No personal correspondence on this page.")
		}
		if page.Next != "" {
			fmt.Printf("Older: keryx correspondence --limit %d --cursor %q", limit, page.Next)
			if caseID != "" {
				fmt.Printf(" --case %q", caseID)
			}
			if query != "" {
				fmt.Printf(" --search %q", query)
			}
			fmt.Println()
		}
		return nil
	}}
	cmd.Flags().StringVar(&caseID, "case", "", "open a stable case ID")
	cmd.Flags().StringVar(&query, "search", "", "find person, city, subject or good")
	cmd.Flags().StringVar(&cursor, "cursor", "", "opaque next-page cursor")
	cmd.Flags().IntVar(&limit, "limit", 30, "page size (1–100)")
	return cmd
}
