package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// reportCmd sends a bug/design/confused report (B1, megaron_mvp_mandag.md
// §B1). The server stamps player, tick and world itself — --q/--r are the
// only optional context a caller might add (a hex the report is about).
func reportCmd() *cobra.Command {
	var kind, text string
	var q, r int
	var hasQR bool

	cmd := &cobra.Command{
		Use:   "report",
		Short: "Send a bug/design/confused report",
		Example: `  keryx report --text "the notification text is unreadable"
  keryx report --kind design --text "silver upkeep feels punishing"
  keryx report --kind confused --text "why did my army not board the ship?" --q 12 --r -4`,
		Args: rejectPositionalArgs("text"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.TrimSpace(text) == "" {
				return fmt.Errorf("--text is required")
			}
			if kind != "bug" && kind != "design" && kind != "confused" {
				return fmt.Errorf("--kind must be bug, design or confused")
			}
			hasQR = cmd.Flags().Changed("q") || cmd.Flags().Changed("r")

			body := map[string]any{"kind": kind, "body": text}
			if hasQR {
				body["q"] = q
				body["r"] = r
			}

			c := newClient(cfg)
			path := fmt.Sprintf("/api/v1/worlds/%s/reports", cfg.WorldID)
			data, err := c.post(path, body)
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			var resp struct {
				Tick int `json:"tick"`
			}
			_ = json.Unmarshal(data, &resp)
			fmt.Printf("Report sent (tick %d). Thank you.\n", resp.Tick)
			return nil
		},
	}
	cmd.Flags().StringVar(&kind, "kind", "bug", "bug, design or confused")
	cmd.Flags().StringVar(&text, "text", "", "the report text")
	cmd.Flags().IntVar(&q, "q", 0, "optional hex q the report is about")
	cmd.Flags().IntVar(&r, "r", 0, "optional hex r the report is about")
	return cmd
}

// reportsCmd is Timothy's read path — admin-key gated, mirrors `keryx god`.
// No admin UI exists or is planned; this is it.
func reportsCmd() *cobra.Command {
	var adminKey string
	cmd := &cobra.Command{
		Use:   "reports",
		Short: "List player reports for this world (admin only)",
		Example: `  POLEIA_ADMIN_KEY=secret keryx reports
  keryx reports --key secret --json`,
		Args: rejectPositionalArgs("key"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if adminKey == "" {
				adminKey = os.Getenv("POLEIA_ADMIN_KEY")
			}
			if adminKey == "" {
				return fmt.Errorf("admin key required: set POLEIA_ADMIN_KEY or use --key")
			}

			c := newClient(cfg)
			c.extraHeaders = map[string]string{"X-Admin-Key": adminKey}

			path := fmt.Sprintf("/api/v1/admin/worlds/%s/reports", cfg.WorldID)
			data, err := c.get(path)
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}

			var resp struct {
				Reports []struct {
					Player    string          `json:"player"`
					Kind      string          `json:"kind"`
					Body      string          `json:"body"`
					Q         *int            `json:"q"`
					R         *int            `json:"r"`
					View      *string         `json:"view"`
					Context   json.RawMessage `json:"context"`
					Tick      int             `json:"tick"`
					CreatedAt string          `json:"created_at"`
				} `json:"reports"`
			}
			if err := json.Unmarshal(data, &resp); err != nil {
				return fmt.Errorf("parse response: %w", err)
			}
			if len(resp.Reports) == 0 {
				fmt.Println("No reports.")
				return nil
			}
			for _, rr := range resp.Reports {
				pos := ""
				if rr.Q != nil && rr.R != nil {
					pos = fmt.Sprintf(" @(%d,%d)", *rr.Q, *rr.R)
				}
				view := ""
				if rr.View != nil && *rr.View != "" {
					view = " [" + *rr.View + "]"
				}
				ctx := ""
				if len(rr.Context) > 0 && string(rr.Context) != "null" {
					ctx = " " + string(rr.Context)
				}
				fmt.Printf("[tick %d] %-9s %-16s%s%s — %s%s\n",
					rr.Tick, rr.Kind, rr.Player, pos, view, rr.Body, ctx)
			}
			fmt.Printf("\n%d report(s)\n", len(resp.Reports))
			return nil
		},
	}
	cmd.Flags().StringVar(&adminKey, "key", "", "admin key (overrides POLEIA_ADMIN_KEY env var)")
	return cmd
}
