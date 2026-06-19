package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

// messageCmd sends a free-text message to another Wanax's settlement.
// No goods — pure diplomatic communication. FOW-gated on the server.
// This is SB8 verb #1: distinct from trade-offer and transfer.
func messageCmd() *cobra.Command {
	var destName, text string
	cmd := &cobra.Command{
		Use:     "message",
		Short:   "Send a free-text message to another Wanax (no goods)",
		Example: `  poleia message --to Korinth --text "Greetings, neighbour"`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)

			provinces, err := c.get(fmt.Sprintf("/api/v1/worlds/%s/provinces", cfg.WorldID))
			if err != nil {
				return err
			}
			var markers []map[string]any
			if err := json.Unmarshal(provinces, &markers); err != nil {
				return err
			}
			var wanaxes []map[string]any
			if wdata, werr := c.get(fmt.Sprintf("/api/v1/worlds/%s/wanaxes", cfg.WorldID)); werr == nil {
				_ = json.Unmarshal(wdata, &wanaxes)
			}

			destID, destName, ownSettlementID, err := resolveMessengerDest(markers, wanaxes, destName)
			if err != nil {
				return err
			}

			path := fmt.Sprintf("/api/v1/worlds/%s/settlements/%s/messengers", cfg.WorldID, ownSettlementID)
			data, err := c.post(path, map[string]any{
				"destination_id": destID,
				"message":        text,
				// No trade_offer — pure message
			})
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
			fmt.Printf("Message dispatched to %s · arrives %s\n", destName, arrivesAt)
			return nil
		},
	}
	cmd.Flags().StringVarP(&destName, "to", "t", "", "destination settlement name (required)")
	cmd.Flags().StringVarP(&text, "text", "x", "", "message text (required)")
	_ = cmd.MarkFlagRequired("to")
	_ = cmd.MarkFlagRequired("text")
	return cmd
}

// tradeOfferCmd sends a structured trade offer to another Wanax.
// Requires the destination to be within your scouted visibility (FOW-gated server-side).
// The recipient must explicitly accept; no goods move without consent.
// This is SB8 verb #3: distinct from message (no goods) and transfer (own→own, no consent).
func tradeOfferCmd() *cobra.Command {
	var destName, wantGood, msgText string
	var wantQty, offerSilver float64

	cmd := &cobra.Command{
		Use:   "trade-offer",
		Short: "Send a trade offer (bilateral consent, FOW-gated) to another Wanax",
		Example: `  poleia trade-offer --to Korinth --want-good grain --want-qty 100 --offer-silver 80`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c := newClient(cfg)

			provinces, err := c.get(fmt.Sprintf("/api/v1/worlds/%s/provinces", cfg.WorldID))
			if err != nil {
				return err
			}
			var markers []map[string]any
			if err := json.Unmarshal(provinces, &markers); err != nil {
				return err
			}
			var wanaxes []map[string]any
			if wdata, werr := c.get(fmt.Sprintf("/api/v1/worlds/%s/wanaxes", cfg.WorldID)); werr == nil {
				_ = json.Unmarshal(wdata, &wanaxes)
			}

			destID, resolvedName, ownSettlementID, err := resolveMessengerDest(markers, wanaxes, destName)
			if err != nil {
				return err
			}

			if msgText == "" {
				msgText = fmt.Sprintf("Trade offer: %g %s for %g silver", wantQty, wantGood, offerSilver)
			}

			path := fmt.Sprintf("/api/v1/worlds/%s/settlements/%s/messengers", cfg.WorldID, ownSettlementID)
			data, err := c.post(path, map[string]any{
				"destination_id": destID,
				"message":        msgText,
				"trade_offer": map[string]any{
					"want_good":    wantGood,
					"want_qty":     wantQty,
					"offer_silver": offerSilver,
				},
			})
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
			fmt.Printf("Trade offer dispatched to %s (want %.0f %s, offer %.0f silver) · arrives %s\n",
				resolvedName, wantQty, wantGood, offerSilver, arrivesAt)
			return nil
		},
	}
	cmd.Flags().StringVarP(&destName, "to", "t", "", "destination settlement name (required)")
	cmd.Flags().StringVar(&wantGood, "want-good", "", "good key to request (e.g. grain, copper, cedar)")
	cmd.Flags().Float64Var(&wantQty, "want-qty", 0, "quantity of good to request")
	cmd.Flags().Float64Var(&offerSilver, "offer-silver", 0, "silver to offer in exchange")
	cmd.Flags().StringVarP(&msgText, "message", "m", "", "optional message text (auto-generated if omitted)")
	_ = cmd.MarkFlagRequired("to")
	_ = cmd.MarkFlagRequired("want-good")
	_ = cmd.MarkFlagRequired("want-qty")
	_ = cmd.MarkFlagRequired("offer-silver")
	return cmd
}
