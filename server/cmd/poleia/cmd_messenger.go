package main

import (
	"encoding/json"
	"fmt"
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
			if len(ownSettlementIDs) == 0 {
				return fmt.Errorf("could not find own settlement")
			}
			var msgs []map[string]any
			for _, sid := range ownSettlementIDs {
				path := fmt.Sprintf("/api/v1/worlds/%s/settlements/%s/messengers", cfg.WorldID, sid)
				data, err := c.get(path)
				if err != nil {
					return err
				}
				if jsonMode {
					printRawJSON(data)
					continue
				}
				var part []map[string]any
				if err := json.Unmarshal(data, &part); err != nil {
					return err
				}
				msgs = append(msgs, part...)
			}
			if jsonMode {
				return nil
			}
			if len(msgs) == 0 {
				fmt.Println("Outbox empty.")
				return nil
			}
			for _, m := range msgs {
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
				line := fmt.Sprintf("→ %s  [%s]  (%s)", dest, status, when)
				if offer, ok := m["trade_offer"].(map[string]any); ok {
					offerStatus, _ := offer["status"].(string)
					good, _ := offer["want_good"].(string)
					qty, _ := offer["want_qty"].(float64)
					silver, _ := offer["offer_silver"].(float64)
					line += fmt.Sprintf("  trade: want %.0f %s for %.0f silver [%s]", qty, good, silver, offerStatus)
				}
				fmt.Println(line)
			}
			return nil
		},
	}
}
