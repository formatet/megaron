package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// reportContextLines renders a report's context blob as indented, readable
// lines instead of the raw JSON tail that used to be glued onto the end of the
// report line.
//
// The client now attaches diagnostics to every report (browser, window size,
// the last refusals the server gave, the last script errors — see
// web/static/js/megaron/ui/diagnostics.js), which answers the questions the
// 2026-09-04 and 09-09 sweeps kept having to ask by hand. Dumped raw, that
// turns every report into an unreadable one-line wall, so the reading surface
// has to grow with the data.
//
// Anything not recognised is preserved verbatim on a `context:` line — this
// must never silently drop a field, since the blob is free-form by design
// (mig 123) and will keep gaining keys.
func reportContextLines(raw json.RawMessage) []string {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var blob map[string]json.RawMessage
	if err := json.Unmarshal(raw, &blob); err != nil {
		return []string{"context: " + string(raw)} // not an object — show it as-is
	}

	var out []string

	if c, ok := blob["client"]; ok {
		var cl struct {
			UA       string  `json:"ua"`
			Viewport string  `json:"viewport"`
			DPR      float64 `json:"dpr"`
			Lang     string  `json:"lang"`
		}
		if json.Unmarshal(c, &cl) == nil {
			parts := []string{}
			if cl.Viewport != "" {
				px := cl.Viewport
				if cl.DPR != 0 && cl.DPR != 1 {
					px = fmt.Sprintf("%s @%gx", px, cl.DPR)
				}
				parts = append(parts, px)
			}
			if cl.Lang != "" {
				parts = append(parts, cl.Lang)
			}
			line := "client: " + cl.UA
			if len(parts) > 0 {
				line += "  (" + strings.Join(parts, " · ") + ")"
			}
			out = append(out, line)
		}
		delete(blob, "client")
	}

	if f, ok := blob["recent_api_failures"]; ok {
		var fails []struct {
			Path   string `json:"path"`
			Status int    `json:"status"`
			Body   string `json:"body"`
		}
		if json.Unmarshal(f, &fails) == nil {
			// Newest last, matching the buffer's own order — the refusal that
			// prompted the report is the one at the bottom.
			for _, x := range fails {
				out = append(out, fmt.Sprintf("refused: %d %s — %s", x.Status, x.Path, x.Body))
			}
		}
		delete(blob, "recent_api_failures")
	}

	if e, ok := blob["recent_js_errors"]; ok {
		var errs []struct {
			Message string `json:"message"`
			Source  string `json:"source"`
		}
		if json.Unmarshal(e, &errs) == nil {
			for _, x := range errs {
				line := "js error: " + x.Message
				if x.Source != "" {
					line += " (" + x.Source + ")"
				}
				out = append(out, line)
			}
		}
		delete(blob, "recent_js_errors")
	}

	// Whatever is left is the entity context (settlement_id, unit_ids,
	// march_ctx_dest, and anything a future drawer adds).
	if len(blob) > 0 {
		if rest, err := json.Marshal(blob); err == nil {
			out = append(out, "context: "+string(rest))
		}
	}
	return out
}

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
				fmt.Printf("[tick %d] %-9s %-16s%s%s — %s\n",
					rr.Tick, rr.Kind, rr.Player, pos, view, rr.Body)
				for _, line := range reportContextLines(rr.Context) {
					fmt.Printf("    %s\n", line)
				}
			}
			fmt.Printf("\n%d report(s)\n", len(resp.Reports))
			return nil
		},
	}
	cmd.Flags().StringVar(&adminKey, "key", "", "admin key (overrides POLEIA_ADMIN_KEY env var)")
	return cmd
}
