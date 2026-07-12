package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// noisyNotificationKinds are the notification kinds collapsed to a summary
// by default (both the default-view exclusion and the ×N grouping below) —
// an explicit allowlist, not "anything repetitive", so a future high-signal
// kind (e.g. SubsistenceWarning) is never silently swallowed just because
// it happens to fire often. Today: only Sitos' routine noise (~99% of the
// feed — see megaron_ekonomi_legibilitet_plan.md DEL B).
var noisyNotificationKinds = []string{"SitosIntervention"}

type notificationItem struct {
	ID        string          `json:"id"`
	Kind      string          `json:"kind"`
	Level     int             `json:"level"`
	Body      json.RawMessage `json:"body"`
	CreatedAt string          `json:"created_at"`
	ReadAt    *string         `json:"read_at"`
}

func isNoisyNotificationKind(kind string) bool {
	for _, k := range noisyNotificationKinds {
		if k == kind {
			return true
		}
	}
	return false
}

func notificationAge(createdAt string) string {
	t, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return ""
	}
	ago := time.Since(t)
	switch {
	case ago < time.Hour:
		return fmt.Sprintf("%dm ago", int(ago.Minutes()))
	case ago < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(ago.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(ago.Hours()/24))
	}
}

func printNotificationRow(n notificationItem) {
	marker := " "
	if n.ReadAt == nil {
		marker = "*"
	}
	fmt.Printf("%s[%s]  %-20s  %s\n", marker, notificationAge(n.CreatedAt), n.Kind, string(n.Body))
}

// notificationsCmd surfaces the persistent notifications feed (server since
// mig 045/06-10) — previously invisible in keryx entirely: arrivals, colony
// foundings, build/train completions, trade events etc. fired server-side
// with nowhere to see them in the CLI (Fas 2h/keryx-surface rule: everything
// in temenos must be visible AND actionable in keryx).
//
// Fas 2026-07-12 (DEL B, megaron_ekonomi_legibilitet_plan.md): the default
// view was ~99% SitosIntervention noise burying real events (TradeDelivery,
// UnitArrived, ...). Default now excludes noisyNotificationKinds and prints
// a one-line pointer to them; --kind/--exclude give explicit control.
func notificationsCmd() *cobra.Command {
	var unreadOnly, markRead bool
	var kindFilter, excludeFilter string
	cmd := &cobra.Command{
		Use:   "notifications",
		Short: "Show your notification feed (arrivals, completions, trade events, ...)",
		Example: `  poleia notifications
  poleia notifications --unread
  poleia notifications --kind SitosIntervention
  poleia notifications --exclude SitosIntervention
  poleia notifications --mark-read`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)
			if markRead {
				if _, err := c.post(fmt.Sprintf("/api/v1/worlds/%s/notifications/read-all", cfg.WorldID), nil); err != nil {
					return err
				}
				fmt.Println("All notifications marked read.")
				return nil
			}

			basePath := fmt.Sprintf("/api/v1/worlds/%s/notifications", cfg.WorldID)

			// Default (no --kind/--exclude given): hide the noisy kinds so
			// they don't crowd real signal out of the server's LIMIT 100
			// window. Explicit --kind/--exclude always win outright.
			usingDefaultNoiseFilter := kindFilter == "" && excludeFilter == ""

			params := url.Values{}
			if unreadOnly {
				params.Set("unread", "true")
			}
			switch {
			case kindFilter != "":
				params.Set("kind", kindFilter)
			case excludeFilter != "":
				params.Set("exclude", excludeFilter)
			default:
				params.Set("exclude", strings.Join(noisyNotificationKinds, ","))
			}

			path := basePath
			if enc := params.Encode(); enc != "" {
				path += "?" + enc
			}
			data, err := c.get(path)
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			var resp struct {
				Notifications []notificationItem `json:"notifications"`
				Unread        int                `json:"unread"`
			}
			if err := json.Unmarshal(data, &resp); err != nil {
				return err
			}

			// Best-effort count of what the default filter hid, for the
			// "+N ... --kind X för alla" pointer. Skipped entirely when the
			// caller already asked for a specific --kind/--exclude.
			hiddenCounts := map[string]int{}
			if usingDefaultNoiseFilter {
				for _, kind := range noisyNotificationKinds {
					countParams := url.Values{}
					if unreadOnly {
						countParams.Set("unread", "true")
					}
					countParams.Set("kind", kind)
					countData, err := c.get(basePath + "?" + countParams.Encode())
					if err != nil {
						continue
					}
					var countResp struct {
						Notifications []json.RawMessage `json:"notifications"`
					}
					if json.Unmarshal(countData, &countResp) == nil {
						hiddenCounts[kind] = len(countResp.Notifications)
					}
				}
			}

			if len(resp.Notifications) == 0 && len(hiddenCounts) == 0 {
				fmt.Println("No notifications.")
				return nil
			}

			fmt.Printf("%d notification(s), %d unread\n", len(resp.Notifications), resp.Unread)
			fmt.Println("────────────────────────────────────────────────────────────")

			// Non-noisy notifications shown in full, at the top, unabridged.
			grouped := map[string][]notificationItem{}
			for _, n := range resp.Notifications {
				if isNoisyNotificationKind(n.Kind) {
					grouped[n.Kind] = append(grouped[n.Kind], n)
				} else {
					printNotificationRow(n)
				}
			}

			// Noisy kinds present in this response (e.g. explicit --kind
			// SitosIntervention) collapse to one "×N" line instead of
			// flooding the terminal.
			for _, kind := range noisyNotificationKinds {
				occ := grouped[kind]
				if len(occ) == 0 {
					continue
				}
				latest := occ[0] // server orders created_at DESC
				marker := " "
				for _, n := range occ {
					if n.ReadAt == nil {
						marker = "*"
						break
					}
				}
				fmt.Printf("%s[%s]  %-20s  ×%d (senaste: %s)\n", marker, notificationAge(latest.CreatedAt), kind, len(occ), string(latest.Body))
			}

			// Default-view summary for kinds excluded from the fetch entirely.
			for _, kind := range noisyNotificationKinds {
				if n := hiddenCounts[kind]; n > 0 {
					fmt.Printf("+%d %s — `poleia notifications --kind %s` för alla\n", n, kind, kind)
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&unreadOnly, "unread", false, "show only unread notifications")
	cmd.Flags().BoolVar(&markRead, "mark-read", false, "mark all notifications as read and exit")
	cmd.Flags().StringVar(&kindFilter, "kind", "", "only show these notification kinds (comma-separated, e.g. SitosIntervention)")
	cmd.Flags().StringVar(&excludeFilter, "exclude", "", "hide these notification kinds (comma-separated)")
	return cmd
}
