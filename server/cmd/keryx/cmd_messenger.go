package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func replyCmd() *cobra.Command {
	var msgID, text string
	cmd := &cobra.Command{
		Use:   "reply",
		Short: "Reply to an inbox message",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if msgID == "" || text == "" {
				return fmt.Errorf("--id and --text required")
			}
			c := newClient(cfg)
			path := fmt.Sprintf("/api/v1/worlds/%s/messengers/%s/reply", cfg.WorldID, msgID)
			data, err := c.post(path, map[string]string{"reply": text})
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			var resp map[string]any
			if err := json.Unmarshal(data, &resp); err != nil {
				return err
			}
			fmt.Printf("Messenger returning · arrives %v\n", resp["returns_at"])
			return nil
		},
	}
	cmd.Flags().StringVar(&msgID, "id", "", "messenger ID (from inbox --output json)")
	cmd.Flags().StringVar(&text, "text", "", "reply text")
	return cmd
}

func tradeAcceptCmd() *cobra.Command {
	var msgID string
	cmd := &cobra.Command{
		Use:   "trade-accept",
		Short: "Accept a trade offer from inbox",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if msgID == "" {
				return fmt.Errorf("--id required")
			}
			c := newClient(cfg)
			path := fmt.Sprintf("/api/v1/worlds/%s/messengers/%s/trade-accept", cfg.WorldID, msgID)
			data, err := c.post(path, map[string]any{})
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			var resp map[string]any
			if err := json.Unmarshal(data, &resp); err != nil {
				return err
			}
			fmt.Printf("Trade accepted · %.0f %s incoming · silver paid: %.0f · goods arrive %v\n",
				resp["quantity"], resp["good_key"], resp["silver_paid"], resp["goods_arrives_at"])
			return nil
		},
	}
	cmd.Flags().StringVar(&msgID, "id", "", "messenger ID")
	return cmd
}

func tradeDeclineCmd() *cobra.Command {
	var msgID string
	cmd := &cobra.Command{
		Use:   "trade-decline",
		Short: "Decline a trade offer from inbox",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if msgID == "" {
				return fmt.Errorf("--id required")
			}
			c := newClient(cfg)
			path := fmt.Sprintf("/api/v1/worlds/%s/messengers/%s/trade-decline", cfg.WorldID, msgID)
			data, err := c.post(path, map[string]any{})
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			fmt.Println("Trade offer declined.")
			return nil
		},
	}
	cmd.Flags().StringVar(&msgID, "id", "", "messenger ID")
	return cmd
}

func tradeCancelCmd() *cobra.Command {
	var msgID string
	cmd := &cobra.Command{
		Use:     "trade-cancel",
		Short:   "Cancel an outgoing pending trade offer and reclaim escrowed silver",
		Example: `  keryx trade-cancel --id <messenger-id>`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if msgID == "" {
				return fmt.Errorf("--id required (find the id with: keryx outbox)")
			}
			c := newClient(cfg)
			path := fmt.Sprintf("/api/v1/worlds/%s/messengers/%s/trade-cancel", cfg.WorldID, msgID)
			data, err := c.post(path, map[string]any{})
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			var resp map[string]any
			if err := json.Unmarshal(data, &resp); err != nil {
				return err
			}
			status, _ := resp["status"].(string)
			silver, _ := resp["silver_refunded"].(float64)
			if status == "cancelled" {
				fmt.Printf("Offer cancelled · %.0f silver refunded\n", silver)
			} else {
				fmt.Printf("Offer already resolved (status: %s)\n", status)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&msgID, "id", "", "messenger ID (find with: keryx outbox)")
	return cmd
}

// outboxCmd lists your last 20 sent messengers (with trade_offer details).
func outboxCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "outbox",
		Short: "List sent messengers and their pending trade offers",
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)
			// Resolve own settlement_ids from the province markers (cfg stores province_id, not settlement_id).
			// Aggregate sent messengers across ALL owned settlements — a pending offer may have been
			// sent from a colony, and listing only one settlement made it look like the outbox was empty
			// while the server still rejected re-sends with "check your outbox".
			provinces, err := c.get(fmt.Sprintf("/api/v1/worlds/%s/provinces", cfg.WorldID))
			if err != nil {
				return err
			}
			var markers []map[string]any
			_ = json.Unmarshal(provinces, &markers)
			var ownSettlementIDs []string
			for _, m := range markers {
				if own, _ := m["own"].(bool); own {
					if sid, _ := m["settlement_id"].(string); sid != "" {
						ownSettlementIDs = append(ownSettlementIDs, sid)
					}
				}
			}
			msgs := []map[string]any{}
			// Host-origin messengers (founder phase, mig 087) live on their own
			// endpoint — merged in so the correspondence is readable both while the
			// host wanders (no settlement exists) and after founding (replies to a
			// dissolved host still come home).
			if hostData, herr := c.get(fmt.Sprintf("/api/v1/worlds/%s/founding/messengers", cfg.WorldID)); herr == nil {
				var part []map[string]any
				if json.Unmarshal(hostData, &part) == nil {
					msgs = append(msgs, part...)
				}
			}
			if len(ownSettlementIDs) == 0 && len(msgs) == 0 {
				return fmt.Errorf("could not find own settlement")
			}
			for _, sid := range ownSettlementIDs {
				path := fmt.Sprintf("/api/v1/worlds/%s/settlements/%s/messengers", cfg.WorldID, sid)
				data, err := c.get(path)
				if err != nil {
					return err
				}
				var part []map[string]any
				if err := json.Unmarshal(data, &part); err != nil {
					return err
				}
				msgs = append(msgs, part...)
			}
			if jsonMode {
				printJSON(msgs)
				return nil
			}
			if len(msgs) == 0 {
				fmt.Println("Outbox empty.")
				return nil
			}
			for _, m := range msgs {
				id, _ := m["id"].(string)
				dest, _ := m["destination_name"].(string)
				status, _ := m["status"].(string)
				sentStr, _ := m["sent_at"].(string)
				var when string
				if t, err := time.Parse(time.RFC3339, sentStr); err == nil {
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
				line := fmt.Sprintf("→ %s  [%s]  (%s)  id:%s", dest, status, when, id)
				// The reply rides home with the returning messenger — without this
				// line the correspondence's whole payoff was --json-only.
				if reply, ok := m["reply_text"].(string); ok && reply != "" {
					line += fmt.Sprintf("  svar: %q", reply)
				}
				if offer, ok := m["trade_offer"].(map[string]any); ok {
					offerStatus, _ := offer["status"].(string)
					offerKind, _ := offer["kind"].(string)
					if offerKind == "sell" {
						good, _ := offer["offer_good"].(string)
						qty, _ := offer["offer_qty"].(float64)
						silver, _ := offer["want_silver"].(float64)
						line += fmt.Sprintf("  trade: sell %.0f %s for %.0f silver [%s]", qty, good, silver, offerStatusLabel(offerStatus))
						if offerStatus == "pending" {
							line += fmt.Sprintf("  (%.0f %s escrowed — cancel with: keryx trade-cancel --id %s)", qty, good, id)
						}
					} else {
						good, _ := offer["want_good"].(string)
						qty, _ := offer["want_qty"].(float64)
						silver, _ := offer["offer_silver"].(float64)
						line += fmt.Sprintf("  trade: want %.0f %s for %.0f silver [%s]", qty, good, silver, offerStatusLabel(offerStatus))
						// Pending buy offers have the buyer's silver held in escrow until the
						// seller accepts/declines or the offer expires (then it's refunded).
						if offerStatus == "pending" {
							line += fmt.Sprintf("  (%.0f silver escrowed — cancel with: keryx trade-cancel --id %s)", silver, id)
						}
					}
					// Arrival ETA (B2) — while the messenger is still travelling, the
					// destination can't yet see or act on this offer at all; without
					// this the outbox showed no timeline until the (later) escrow
					// deadline below.
					if offerStatus == "pending" && status != "delivered" {
						if arrStr, ok := m["arrives_at"].(string); ok {
							if arrT, err := time.Parse(time.RFC3339, arrStr); err == nil {
								line += fmt.Sprintf("  arrives in %s", countdown(arrT))
							}
						}
					}
					// Escrow countdown (Fas 2b) — a pending offer's expires_at wasn't
					// shown anywhere before, so there was no visible deadline for when
					// the lock releases.
					if offerStatus == "pending" {
						if expStr, ok := m["expires_at"].(string); ok {
							if expT, err := time.Parse(time.RFC3339, expStr); err == nil {
								line += fmt.Sprintf("  expires in %s", countdown(expT))
							}
						}
					}
					// Delivery ETA (P5) — once accepted, trade-accept's response was the
					// ONLY place goods_arrives_at/silver_arrives_at ever appeared; outbox
					// (checked any time after) showed just "[accepted]" with no timeline,
					// so a Wanax had no way to tell "still in transit" from "lost/stuck".
					// The handler now stamps both ETAs onto trade_offer itself at accept
					// time (api/handlers/messenger.go TradeAccept) so this can read them back.
					if offerStatus == "accepted" {
						if eta := deliveryETALine(offer); eta != "" {
							line += eta
						}
					}
				}
				fmt.Println(line)
			}
			// Total escrow exposure (P5c) — each pending offer's lock was only ever
			// shown one-at-a-time on its own line; a Wanax with several pending
			// offers out had no single place to see how much silver/goods were
			// tied up in total. Silence here reads as "my resources are free" when
			// they may not be.
			if summary := escrowExposureSummary(msgs); summary != "" {
				fmt.Println(summary)
			}
			return nil
		},
	}
}

// offerStatusLabel turns a raw trade_offer status into a phrase that says what
// actually happened to the escrow. The raw words mislead in both directions:
// "returned" reads as a rejection but is the SUCCESS terminal (trade_return.go
// sets it only after the return caravan credited the goods home), and
// "expired"/"declined"/"cancelled" gave no hint that the escrow came back.
func offerStatusLabel(status string) string {
	switch status {
	case "returned":
		return "completed — return caravan home"
	case "accepted":
		return "accepted — in transit"
	case "declined":
		return "declined — escrow refunded"
	case "expired":
		return "expired unanswered — escrow refunded"
	case "cancelled":
		return "withdrawn — escrow refunded"
	default:
		return status
	}
}

// deliveryETALine formats the goods/silver delivery ETA for an ACCEPTED trade
// offer (as persisted by TradeAccept onto trade_offer — goods_arrives_at /
// silver_arrives_at). Returns "" if neither timestamp is present (e.g. an
// older offer accepted before this field existed). Uses whichever of the two
// legs is still in the future; a leg already in the past reads "delivered".
func deliveryETALine(offer map[string]any) string {
	fmtLeg := func(label, raw string) string {
		if raw == "" {
			return ""
		}
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return ""
		}
		if t.Before(time.Now()) {
			return fmt.Sprintf("%s delivered", label)
		}
		return fmt.Sprintf("%s in %s", label, countdown(t))
	}
	goodsAt, _ := offer["goods_arrives_at"].(string)
	silverAt, _ := offer["silver_arrives_at"].(string)
	var parts []string
	if s := fmtLeg("goods", goodsAt); s != "" {
		parts = append(parts, s)
	}
	if s := fmtLeg("silver", silverAt); s != "" {
		parts = append(parts, s)
	}
	if len(parts) == 0 {
		return ""
	}
	return "  " + strings.Join(parts, " · ")
}

// escrowExposureSummary aggregates the escrow locked by every PENDING trade
// offer in msgs (as returned by outbox/ListSent) into one human-readable
// line: total silver (from pending buy offers) plus total quantity per good
// (from pending sell offers). Returns "" if nothing is currently escrowed.
func escrowExposureSummary(msgs []map[string]any) string {
	var totalSilver float64
	goodsLocked := map[string]float64{}
	var goodOrder []string
	pendingCount := 0

	for _, m := range msgs {
		offer, ok := m["trade_offer"].(map[string]any)
		if !ok {
			continue
		}
		if status, _ := offer["status"].(string); status != "pending" {
			continue
		}
		pendingCount++
		kind, _ := offer["kind"].(string)
		if kind == "sell" {
			good, _ := offer["offer_good"].(string)
			qty, _ := offer["offer_qty"].(float64)
			if _, seen := goodsLocked[good]; !seen {
				goodOrder = append(goodOrder, good)
			}
			goodsLocked[good] += qty
		} else {
			silver, _ := offer["offer_silver"].(float64)
			totalSilver += silver
		}
	}

	if pendingCount == 0 {
		return ""
	}

	parts := []string{}
	if totalSilver > 0 {
		parts = append(parts, fmt.Sprintf("%.0f silver", totalSilver))
	}
	for _, good := range goodOrder {
		parts = append(parts, fmt.Sprintf("%.0f %s", goodsLocked[good], good))
	}

	offerWord := "offer"
	if pendingCount != 1 {
		offerWord = "offers"
	}
	return fmt.Sprintf("⚠ Escrow exposure: %s locked across %d pending %s",
		strings.Join(parts, " + "), pendingCount, offerWord)
}
