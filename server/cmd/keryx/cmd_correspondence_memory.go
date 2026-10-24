package main

import (
	"encoding/json"
	"fmt"
	"github.com/spf13/cobra"
	"net/url"
)

func correspondenceMemoryCmd() *cobra.Command {
	var caseID, cursor string
	var limit int
	cmd := &cobra.Command{Use: "correspondence-memory", Short: "Read accepted agreements and your own letter passages", Args: noPositionalArgs(), RunE: func(cmd *cobra.Command, _ []string) error {
		if caseID == "" {
			return fmt.Errorf("--case required")
		}
		if limit < 1 || limit > 100 {
			return fmt.Errorf("--limit must be between 1 and 100")
		}
		q := url.Values{"limit": {fmt.Sprint(limit)}}
		if cursor != "" {
			q.Set("cursor", cursor)
		}
		data, err := newClient(cfg).get(fmt.Sprintf("/api/v1/worlds/%s/correspondence/cases/%s/memory?%s", cfg.WorldID, url.PathEscape(caseID), q.Encode()))
		if err != nil {
			return err
		}
		if jsonMode {
			printRawJSON(data)
			return nil
		}
		var page struct {
			Transactions []struct {
				ID            string          `json:"id"`
				CaseID        string          `json:"case_id"`
				Subject       string          `json:"subject"`
				SenderName    string          `json:"sender_name"`
				RecipientName string          `json:"recipient_name"`
				Terms         json.RawMessage `json:"terms"`
			} `json:"transactions"`
			Citations []struct {
				ID       string `json:"id"`
				LetterID string `json:"letter_id"`
				CaseID   string `json:"case_id"`
				Source   string `json:"source"`
				Quote    string `json:"quote"`
			} `json:"citations"`
			Next string `json:"next_cursor"`
		}
		if err = json.Unmarshal(data, &page); err != nil {
			return err
		}
		fmt.Println("Accepted agreements · delivery outcome unavailable in this record")
		for _, item := range page.Transactions {
			fmt.Printf("%s · %s → %s\n  %s\n  Source: keryx correspondence --case %s --letter %s\n", item.Subject, item.SenderName, item.RecipientName, item.Terms, item.CaseID, item.ID)
		}
		fmt.Println("My private notes from letters (no shared agreement)")
		for _, item := range page.Citations {
			fmt.Printf("%s: %s\n  Source: keryx correspondence --case %s --letter %s · note %s\n", item.Source, item.Quote, item.CaseID, item.LetterID, item.ID)
		}
		if page.Next != "" {
			fmt.Printf("Older: keryx correspondence-memory --case %q --limit %d --cursor %q\n", caseID, limit, page.Next)
		}
		return nil
	}}
	cmd.Flags().StringVar(&caseID, "case", "", "case ID identifying the counterpart")
	cmd.Flags().StringVar(&cursor, "cursor", "", "opaque next-page cursor")
	cmd.Flags().IntVar(&limit, "limit", 30, "page size (1–100)")
	return cmd
}
func correspondenceCiteCmd() *cobra.Command {
	var caseID, letter, source, quote string
	cmd := &cobra.Command{Use: "correspondence-cite", Short: "Keep an exact passage as your private note", Args: noPositionalArgs(), RunE: func(cmd *cobra.Command, _ []string) error {
		if caseID == "" || letter == "" || quote == "" || (source != "message" && source != "reply") {
			return fmt.Errorf("--case, --letter, --quote and --source message|reply required")
		}
		data, err := newClient(cfg).post(fmt.Sprintf("/api/v1/worlds/%s/correspondence/cases/%s/citations", cfg.WorldID, url.PathEscape(caseID)), map[string]string{"letter_id": letter, "source": source, "quote": quote})
		if err != nil {
			return err
		}
		if jsonMode {
			printRawJSON(data)
		} else {
			fmt.Println("Kept as your private note. This creates no shared agreement.")
		}
		return nil
	}}
	cmd.Flags().StringVar(&caseID, "case", "", "case ID identifying the counterpart")
	cmd.Flags().StringVar(&letter, "letter", "", "source letter ID")
	cmd.Flags().StringVar(&source, "source", "message", "message or reply")
	cmd.Flags().StringVar(&quote, "quote", "", "exact passage from the source")
	return cmd
}
func correspondenceCiteRemoveCmd() *cobra.Command {
	var caseID, id string
	cmd := &cobra.Command{Use: "correspondence-cite-remove", Short: "Remove one of your private letter notes", Args: noPositionalArgs(), RunE: func(cmd *cobra.Command, _ []string) error {
		if caseID == "" || id == "" {
			return fmt.Errorf("--case and --id required")
		}
		_, err := newClient(cfg).delete(fmt.Sprintf("/api/v1/worlds/%s/correspondence/cases/%s/citations/%s", cfg.WorldID, url.PathEscape(caseID), url.PathEscape(id)))
		if err != nil {
			return err
		}
		if jsonMode {
			printRawJSON([]byte("{}"))
		} else {
			fmt.Println("Private note removed.")
		}
		return nil
	}}
	cmd.Flags().StringVar(&caseID, "case", "", "case ID identifying the counterpart")
	cmd.Flags().StringVar(&id, "id", "", "your private citation ID")
	return cmd
}
