package main

import (
	"encoding/json"
	"fmt"

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
