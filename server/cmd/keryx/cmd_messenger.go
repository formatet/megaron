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
		// --id and --text are both required and equally plausible for a
		// stray positional — no single one is the obvious guess.
		Args: noPositionalArgs(),
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
			returnsAt, _ := resp["returns_at"].(string)
			fmt.Printf("Messenger returning · arrives %s\n", arrivalETA(c, returnsAt))
			printPassageNote(resp)
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
		Args:  rejectPositionalArgs("id"),
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
			if kind, _ := resp["kind"].(string); kind == "sell" {
				fmt.Printf("Trade accepted · %.0f %s incoming · silver sent: %.0f · goods arrive %s · silver arrives %s\n",
					resp["quantity"], resp["good_key"], resp["silver_paid"], tradeArrival(c, resp, true), tradeArrival(c, resp, false))
			} else {
				fmt.Printf("Trade accepted · %.0f %s outgoing · silver incoming: %.0f · goods arrive %s · silver arrives %s\n",
					resp["quantity"], resp["good_key"], resp["silver_paid"], tradeArrival(c, resp, true), tradeArrival(c, resp, false))
			}
			// sjöhandel mellan spelare (megaron_plan_sjohandel_mellan_spelare.md
			// R3): a sea-going trade names the initiator's own ship — it sails
			// both legs and isn't free again until leg 2 lands home.
			if shipName, _ := resp["ship_name"].(string); shipName != "" {
				fmt.Printf("  ⛵ carried by sea on %s — the ship isn't free again until it sails home with the return leg\n", shipName)
			}
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
		Args:  rejectPositionalArgs("id"),
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
		Args:    rejectPositionalArgs("id"),
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

// passageCmd is "Ordna passage" (megaron_plan_ordna_passage.md, slice 3b-3):
// arrange for one of your own ships, standing in port, to carry a runner
// that is currently waiting for passage — outbound, or fetched back from a
// foreign port.
func passageCmd() *cobra.Command {
	var msgID, shipID string
	cmd := &cobra.Command{
		Use:   "passage",
		Short: "Arrange for a ship to carry a runner across the sea",
		Example: `  keryx passage --id <messenger-id> --ship <ship-id>
  (find the messenger id with: keryx outbox — it names the ship choices too)`,
		Args: rejectPositionalArgs("id"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if msgID == "" {
				return fmt.Errorf("--id required (find the id with: keryx outbox)")
			}
			if shipID == "" {
				return fmt.Errorf("--ship required (find eligible ships with: keryx outbox, or: keryx unit)")
			}
			c := newClient(cfg)
			path := fmt.Sprintf("/api/v1/worlds/%s/messengers/%s/passage", cfg.WorldID, msgID)
			data, err := c.post(path, map[string]any{"ship_id": shipID})
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
			var eta string
			if arrT, ok := resp["arrives_at"].(string); ok {
				eta = arrT
			}
			fmt.Printf("Passage arranged — ship %v sails, runner aboard when it departs. Arrives %v\n", resp["unit_id"], eta)
			return nil
		},
	}
	cmd.Flags().StringVar(&msgID, "id", "", "messenger ID (find with: keryx outbox)")
	cmd.Flags().StringVar(&shipID, "ship", "", "ship ID (galley or merchantman, garrisoned; find with: keryx outbox or keryx unit)")
	return cmd
}

// callBackCmd is "kalla tillbaka" (megaron_plan_ordna_passage.md, slice
// 3b-4, R5): the other choice for a runner stuck waiting for passage in your
// OWN port — turn it around and walk it home over land, delivering nothing.
// A runner already aboard a ship, or waiting for its return leg's pickup in
// a foreign port, cannot be called back this way.
func callBackCmd() *cobra.Command {
	var msgID string
	cmd := &cobra.Command{
		Use:   "call-back",
		Short: "Call a runner stuck waiting for passage back home, undelivered",
		Example: `  keryx call-back --id <messenger-id>
  (find the messenger id with: keryx outbox)`,
		Args: rejectPositionalArgs("id"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if msgID == "" {
				return fmt.Errorf("--id required (find the id with: keryx outbox)")
			}
			c := newClient(cfg)
			path := fmt.Sprintf("/api/v1/worlds/%s/messengers/%s/call-back", cfg.WorldID, msgID)
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
			var eta string
			if arrT, ok := resp["returns_at"].(string); ok {
				eta = arrT
			}
			fmt.Printf("Runner called back — walking home undelivered. Arrives %v\n", eta)
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
		Args:  noPositionalArgs(), // no flags at all — nothing to guess
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
				if passageStatus, _ := m["passage_status"].(string); passageStatus == "unknown" {
					status = "no word"
				}
				line := fmt.Sprintf("→ %s  [%s]  (%s)  id:%s", dest, status, when, id)
				// megaron_plan_budet_liftar.md: a sea-lifted runner's current
				// state, once it has any (a purely-land runner never sets these).
				if passageStatus, _ := m["passage_status"].(string); passageStatus == "awaiting_passage" {
					port, _ := m["passage_port"].(string)
					if port != "" {
						line += fmt.Sprintf("  [waiting in %s for a ship]", port)
					} else {
						line += "  [waiting for a ship]"
					}
					// 3b-3 (megaron_plan_ordna_passage.md): the server-validated
					// "arrange passage" affordance — only shown when a real choice
					// of ship exists (or the server says none does).
					if canArrange, _ := m["can_arrange_passage"].(bool); canArrange {
						if port != "" {
							line += fmt.Sprintf(" — arrange passage with a ship there: keryx passage --id %s --ship <id>", id)
						} else {
							line += fmt.Sprintf(" — arrange passage: keryx passage --id %s --ship <id>", id)
						}
						if ships, ok := m["eligible_ships"].([]any); ok {
							switch len(ships) {
							case 0:
								line += " (no eligible ship there yet)"
							default:
								var names []string
								for _, raw := range ships {
									if s, ok := raw.(map[string]any); ok {
										if n, _ := s["name"].(string); n != "" {
											names = append(names, n)
										}
									}
								}
								if len(names) > 0 {
									line += fmt.Sprintf(" (ships: %s)", strings.Join(names, ", "))
								}
							}
						}
					}
					// 3b-4 (megaron_plan_ordna_passage.md, R5): the other choice —
					// give up on this errand and bring the runner straight home,
					// undelivered. Only ever offered for an outbound runner in its
					// OWN port (can_call_back is false for a foreign-port pickup).
					if canCallBack, _ := m["can_call_back"].(bool); canCallBack {
						line += fmt.Sprintf(" — or call it back, undelivered: keryx call-back --id %s", id)
					}
				} else if carrier, _ := m["carrier_name"].(string); carrier != "" {
					line += fmt.Sprintf("  [aboard %s]", carrier)
				}
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
								line += fmt.Sprintf("  arrives %s", gameETA(c, arrT))
							}
						}
					}
					// Awaiting response (legibility) — the complement of the "arrives
					// in" case above: once the messenger has actually arrived, the
					// offer sits in the recipient's inbox awaiting accept/decline.
					// Without this the line looked identical to "still travelling"
					// except for the [delivered] tag, so a Wanax couldn't tell "on
					// the way" from "there, waiting on them" at a glance.
					if offerStatus == "pending" && status == "delivered" {
						line += fmt.Sprintf("  waiting on a reply from %s", dest)
					}
					// Escrow countdown (Fas 2b) — a pending offer's expires_at wasn't
					// shown anywhere before, so there was no visible deadline for when
					// the lock releases.
					if offerStatus == "pending" {
						if expStr, ok := m["expires_at"].(string); ok {
							if expT, err := time.Parse(time.RFC3339, expStr); err == nil {
								line += fmt.Sprintf("  expires %s", gameETA(c, expT))
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
						if eta := deliveryETALine(c, offer); eta != "" {
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
func deliveryETALine(c *Client, offer map[string]any) string {
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
		return fmt.Sprintf("%s %s", label, gameETA(c, t))
	}
	goodsAt, _ := offer["goods_arrives_at"].(string)
	silverAt, _ := offer["silver_arrives_at"].(string)
	var parts []string
	if at, ok := offer["goods_arrival_tick"].(float64); ok {
		parts = append(parts, fmt.Sprintf("goods scheduled tick %.0f", at))
	} else if s := fmtLeg("goods", goodsAt); s != "" {
		parts = append(parts, s)
	}
	if at, ok := offer["silver_arrival_tick"].(float64); ok {
		parts = append(parts, fmt.Sprintf("silver scheduled tick %.0f", at))
	} else if s := fmtLeg("silver", silverAt); s != "" {
		parts = append(parts, s)
	}
	// sjöhandel mellan spelare (megaron_plan_sjohandel_mellan_spelare.md R3):
	// stamped onto trade_offer at accept time — empty when the trade walked.
	if shipName, _ := offer["ship_name"].(string); shipName != "" {
		parts = append(parts, fmt.Sprintf("by sea on %s", shipName))
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
