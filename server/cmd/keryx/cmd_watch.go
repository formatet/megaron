package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/spf13/cobra"
)

// wsMsg mirrors internal/notify.Msg on the wire — kept as a separate
// CLI-side type (not shared with the server module) so the client only
// depends on the wire JSON shape, matching the convention every other
// cmd_*.go file follows (see actionVerb in cmd_actions.go).
type wsMsg struct {
	Kind    string          `json:"kind"`
	WorldID string          `json:"world_id"`
	Payload json.RawMessage `json:"payload"`
}

// wsURL derives the WebSocket URL for worldID from the configured server
// URL — http→ws, https→wss, any other scheme is a hard error — preserving
// host, port and any path prefix. Egen ren funktion + test
// (megaron_plan_keryx_strom.md §3 point 1): the scheme rewrite is exactly
// the kind of line that quietly breaks against https in prod while looking
// fine against http locally.
func wsURL(server, worldID string) (string, error) {
	u, err := url.Parse(server)
	if err != nil {
		return "", fmt.Errorf("parse server URL: %w", err)
	}
	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	default:
		return "", fmt.Errorf("unsupported server scheme %q (need http or https)", u.Scheme)
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/ws/" + worldID
	u.RawQuery = ""
	return u.String(), nil
}

// wsDialTarget bundles the derived URL with the Bearer auth header watch's
// dial needs — same scheme every other keryx request uses (Client.doWithHeal).
func wsDialTarget(server, token, worldID string) (string, http.Header, error) {
	u, err := wsURL(server, worldID)
	if err != nil {
		return "", nil, err
	}
	header := http.Header{}
	if token != "" {
		header.Set("Authorization", "Bearer "+token)
	}
	return u, header, nil
}

// watchKindFilter parses --kind into a lookup set. nil (not empty!) means
// "no filter" — distinguishes an unset flag from one that matched nothing.
func watchKindFilter(raw string) map[string]bool {
	if raw == "" {
		return nil
	}
	set := map[string]bool{}
	for _, k := range strings.Split(raw, ",") {
		k = strings.TrimSpace(k)
		if k != "" {
			set[k] = true
		}
	}
	return set
}

func watchAllowed(kinds map[string]bool, kind string) bool {
	if kinds == nil {
		return true
	}
	return kinds[kind]
}

// printJSONLine writes v as a single compact JSON line — unlike printJSON
// (pretty-indented, for one-shot command output), a stream is meant to be
// piped and read one event per line.
func printJSONLine(v any) {
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	fmt.Println(string(data))
}

// watchCatchup fetches unread notifications via the existing REST endpoint
// and prints them with the same renderer `keryx notifications` uses — the
// ikapp-läsning (§3 point 2/3 of the plan): a player must see what happened
// before `watch` connected, not just what arrives after.
func watchCatchup(c *Client, worldID string, kinds map[string]bool) error {
	path := fmt.Sprintf("/api/v1/worlds/%s/notifications?unread=true", worldID)
	data, err := c.get(path)
	if err != nil {
		return err
	}
	var resp struct {
		Notifications []notificationItem `json:"notifications"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return err
	}
	// Server orders created_at DESC; print oldest-first so the catch-up
	// reads like a log leading up to now.
	for i := len(resp.Notifications) - 1; i >= 0; i-- {
		n := resp.Notifications[i]
		if !watchAllowed(kinds, n.Kind) {
			continue
		}
		if jsonMode {
			printJSONLine(n)
			continue
		}
		printNotificationRow(n)
	}
	return nil
}

// printWatchMessage renders one live push message. --json prints the
// server's Msg verbatim, one line, no reinterpretation (§3 point 5). The
// human view mirrors notifications' row format but with a wall-clock
// receipt time in place of created_at/read-state, which a push message
// doesn't carry — then shares printNotificationDetail for the actionable
// follow-up line, so the same event reads identically in the feed and live.
func printWatchMessage(m wsMsg) {
	if jsonMode {
		printJSONLine(m)
		return
	}
	n := notificationItem{Kind: m.Kind, Body: m.Payload}
	kind := m.Kind
	if tier := subsistenceTier(n); tier != "" {
		kind = m.Kind + " [" + subsistenceTierLabel(tier) + "]"
	}
	fmt.Printf(" [%s]  %-20s  %s\n", time.Now().Format("15:04:05"), kind, string(m.Payload))
	printNotificationDetail(n)
}

// watchOptions configures one runWatch pass over a live connection.
type watchOptions struct {
	kinds map[string]bool
	count int // 0 = unlimited
}

// runWatch reads pushed messages off conn until opts.count non-heartbeat
// messages have been shown (0 = forever) or the connection errs out.
// Returns how many messages it displayed and the read error (nil only when
// opts.count was reached), so the caller can decide whether to reconnect and
// how many are still owed toward the original --count.
func runWatch(conn *websocket.Conn, opts watchOptions) (int, error) {
	shown := 0
	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return shown, err
		}
		var m wsMsg
		if err := json.Unmarshal(raw, &m); err != nil {
			continue // not a Msg we understand — never crash the stream on a stray frame
		}
		if m.Kind == "Heartbeat" {
			continue // app-level liveness frame (internal/notify/hub.go) — never a notification
		}
		if !watchAllowed(opts.kinds, m.Kind) {
			continue
		}
		printWatchMessage(m)
		shown++
		if opts.count > 0 && shown >= opts.count {
			return shown, nil
		}
	}
}

func nextBackoff(d time.Duration) time.Duration {
	d *= 2
	if d > 30*time.Second {
		return 30 * time.Second
	}
	return d
}

// watchCmd is the thin WebSocket client against the hub the server already
// runs (api/handlers/ws.go, internal/notify/hub.go) — zero new server code
// (megaron_plan_keryx_strom.md). It never mutates anything: it reads the
// unread-notifications catch-up, then listens, then exits.
func watchCmd() *cobra.Command {
	var count int
	var kindFilter string
	var noCatchup bool

	cmd := &cobra.Command{
		Use:   "watch",
		Short: "Listen for live push notifications over WebSocket (a wakeup call, not a ledger)",
		Long: `Opens a WebSocket to the world's push hub and prints each notification as it
arrives. This is a WAKEUP CALL, not the ledger: a message dropped to a slow
client, or one that arrived while watch was disconnected, is not resent over
the stream, and a world-wide broadcast is never persisted at all — 'keryx
notifications' is the durable source of truth, 'watch' only tells you when to
look at it. watch never mutates any state.

On start (unless --no-catchup) it fetches unread notifications first, so
nothing that happened before the connection opened is missed.`,
		Example: `  keryx watch
  keryx watch --json
  keryx watch --count 1
  keryx watch --kind ForeignMarchSighted,BattleWon,BattleLost`,
		Args: noPositionalArgs(),
		RunE: func(_ *cobra.Command, _ []string) error {
			c := newClient(cfg)
			kinds := watchKindFilter(kindFilter)

			if !noCatchup {
				if err := watchCatchup(c, cfg.WorldID, kinds); err != nil {
					return err
				}
			}

			target, header, err := wsDialTarget(cfg.Server, cfg.Token, cfg.WorldID)
			if err != nil {
				return err
			}

			shownTotal := 0
			backoff := time.Second
			reconnecting := false
			for {
				conn, _, dialErr := websocket.DefaultDialer.Dial(target, header)
				if dialErr != nil {
					if !jsonMode {
						fmt.Fprintf(os.Stderr, "  kunde inte ansluta (%v) — försöker igen om %s\n", dialErr, backoff)
					}
					time.Sleep(backoff)
					backoff = nextBackoff(backoff)
					reconnecting = true
					continue
				}
				if reconnecting && !jsonMode {
					fmt.Println("  strömmen återansluten")
				}
				backoff = time.Second

				remaining := 0
				if count > 0 {
					remaining = count - shownTotal
				}
				shown, _ := runWatch(conn, watchOptions{kinds: kinds, count: remaining})
				conn.Close()
				shownTotal += shown

				if count > 0 && shownTotal >= count {
					return nil
				}
				if !jsonMode {
					fmt.Println("  strömmen bröts — läser ikapp")
				}
				if err := watchCatchup(c, cfg.WorldID, kinds); err != nil && !jsonMode {
					fmt.Fprintf(os.Stderr, "  ikapp-läsning misslyckades: %v\n", err)
				}
				reconnecting = true
				time.Sleep(backoff)
			}
		},
	}
	cmd.Flags().IntVar(&count, "count", 0, "stop after N messages (0 = forever)")
	cmd.Flags().StringVar(&kindFilter, "kind", "", "only show these notification kinds (comma-separated)")
	cmd.Flags().BoolVar(&noCatchup, "no-catchup", false, "skip the unread-notifications catch-up read on connect")
	return cmd
}
