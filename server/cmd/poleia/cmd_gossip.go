package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func gossipCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "gossip",
		Short: "Show recent rumours and events",
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)
			path := fmt.Sprintf("/api/v1/worlds/%s/gossip", cfg.WorldID)
			data, err := c.get(path)
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			var items []map[string]any
			if err := json.Unmarshal(data, &items); err != nil {
				return err
			}
			if len(items) == 0 {
				fmt.Println("No rumours yet.")
				return nil
			}
			for _, g := range items {
				region, _ := g["source_region"].(string)
				text, _ := g["text"].(string)
				tsStr, _ := g["generated_at"].(string)
				var when string
				if t, err := time.Parse(time.RFC3339, tsStr); err == nil {
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
				fmt.Printf("[%s]  %s\n  %s\n\n", when, region, text)
			}
			return nil
		},
	}
}

func messengerCmd() *cobra.Command {
	var destName, message, wantGood string
	var wantQty, offerSilver float64
	cmd := &cobra.Command{
		Use:   "messenger",
		Short: "Send a messenger to another settlement",
		Example: `  poleia messenger --to Korinth --message "Need grain urgently"
  poleia messenger --to Korinth --message "Buy grain offer" --want-good grain --want-qty 100 --offer-silver 80`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)

			// Load visible provinces to resolve own settlement ID and destination.
			provinces, err := c.get(fmt.Sprintf("/api/v1/worlds/%s/provinces", cfg.WorldID))
			if err != nil {
				return err
			}
			var markers []map[string]any
			if err := json.Unmarshal(provinces, &markers); err != nil {
				return err
			}
			var destID, destSettleName, ownSettlementID string
			for _, m := range markers {
				if own, _ := m["own"].(bool); own {
					ownSettlementID, _ = m["settlement_id"].(string)
				}
				n, _ := m["name"].(string)
				if strings.EqualFold(n, destName) {
					destID, _ = m["settlement_id"].(string)
					destSettleName = n
				}
			}
			if ownSettlementID == "" {
				return fmt.Errorf("could not find own settlement")
			}
			// If not found in FOW provinces, fall back to wanaxes (public list).
			if destID == "" {
				wdata, werr := c.get(fmt.Sprintf("/api/v1/worlds/%s/wanaxes", cfg.WorldID))
				if werr == nil {
					var wanaxes []map[string]any
					if json.Unmarshal(wdata, &wanaxes) == nil {
						for _, w := range wanaxes {
							n, _ := w["name"].(string)
							if strings.EqualFold(n, destName) {
								destID, _ = w["settlement_id"].(string)
								destSettleName = n
								break
							}
						}
					}
				}
			}
			if destID == "" {
				return fmt.Errorf("no settlement named %q found in visible provinces or world wanaxes", destName)
			}

			body := map[string]any{
				"destination_id": destID,
				"message":        message,
			}
			if wantGood != "" && wantQty > 0 && offerSilver > 0 {
				body["trade_offer"] = map[string]any{
					"want_good":  wantGood,
					"want_qty":   wantQty,
					"offer_gold": offerSilver,
				}
			}

			path := fmt.Sprintf("/api/v1/worlds/%s/settlements/%s/messengers", cfg.WorldID, ownSettlementID)
			data, err := c.post(path, body)
			if err != nil {
				return err
			}
			if jsonMode {
				printRawJSON(data)
				return nil
			}
			var resp map[string]any
			json.Unmarshal(data, &resp)
			arrivesAt, _ := resp["arrives_at"].(string)
			if wantGood != "" {
				fmt.Printf("Trade offer dispatched to %s (want %.0f %s, offer %.0f silver) · arrives %s\n",
					destSettleName, wantQty, wantGood, offerSilver, arrivesAt)
			} else {
				fmt.Printf("Messenger dispatched to %s · arrives %s\n", destSettleName, arrivesAt)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&destName, "to", "t", "", "destination settlement name (required)")
	cmd.Flags().StringVarP(&message, "message", "m", "", "message text (required)")
	cmd.Flags().StringVar(&wantGood, "want-good", "", "good to request (e.g. grain, cedar)")
	cmd.Flags().Float64Var(&wantQty, "want-qty", 0, "quantity of good to request")
	cmd.Flags().Float64Var(&offerSilver, "offer-silver", 0, "silver to offer in exchange")
	_ = cmd.MarkFlagRequired("to")
	_ = cmd.MarkFlagRequired("message")
	return cmd
}
