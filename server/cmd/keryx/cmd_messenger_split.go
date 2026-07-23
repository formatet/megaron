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
	var destName, text, fromName string
	var fromHost bool
	cmd := &cobra.Command{
		Use:   "message",
		Short: "Send a free-text message to another Wanax (no goods)",
		Example: `  keryx message --to Korinth --text "Greetings, neighbour"
  keryx message --from-host --to Korinth --text "A people seeks passage"`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if fromHost && fromName != "" {
				return fmt.Errorf("--from och --from-host kan inte kombineras — hostet ÄR avsändaren")
			}
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

			// Founder phase: no settlement to send from — the host itself is the
			// origin (POST /founding/messengers, mig 087 unit-origin). Free, like
			// every messenger; no trade from the host.
			var path, resolvedName string
			if fromHost {
				destID, name, err := resolveDestByName(markers, wanaxes, destName)
				if err != nil {
					return err
				}
				resolvedName = name
				path = fmt.Sprintf("/api/v1/worlds/%s/founding/messengers", cfg.WorldID)
				data, err := c.post(path, map[string]any{
					"destination_id": destID,
					"message":        text,
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
				fmt.Printf("Messenger dispatched from the host to %s · arrives %s\n", resolvedName, arrivesAt)
				return nil
			}

			destID, destName, ownSettlementID, err := resolveMessengerDest(markers, wanaxes, destName, fromName)
			if err != nil {
				return err
			}

			path = fmt.Sprintf("/api/v1/worlds/%s/settlements/%s/messengers", cfg.WorldID, ownSettlementID)
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
	cmd.Flags().StringVar(&fromName, "from", "", "your city to send from (default: capital)")
	cmd.Flags().BoolVar(&fromHost, "from-host", false, "send from your wandering Nomadic Host (founder phase) instead of a city")
	_ = cmd.MarkFlagRequired("to")
	_ = cmd.MarkFlagRequired("text")
	return cmd
}

// tradeOfferCmd sends a structured trade offer to another Wanax.
// Requires the destination to be within your scouted visibility (FOW-gated server-side).
// The recipient must explicitly accept; no goods move without consent.
// This is SB8 verb #3: distinct from message (no goods) and transfer (own→own, no consent).
//
// Two modes:
//   - buy:  --want-good G --want-qty N --offer-silver S  (buyer escrows silver)
//   - sell: --offer-good G --offer-qty N --want-silver S (seller escrows goods)
func tradeOfferCmd() *cobra.Command {
	var destName, wantGood, offerGood, msgText, fromName string
	var wantQty, offerSilver, offerQty, wantSilver float64

	cmd := &cobra.Command{
		Use:   "trade-offer",
		Short: "Send a trade offer (bilateral consent, FOW-gated) to another Wanax",
		Example: `  keryx trade-offer --to Korinth --want-good grain --want-qty 100 --offer-silver 80
  keryx trade-offer --to Athenai --offer-good copper --offer-qty 50 --want-silver 120`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			buySet := wantGood != "" && wantQty > 0 && offerSilver > 0
			sellSet := offerGood != "" && offerQty > 0 && wantSilver > 0
			if buySet == sellSet {
				return fmt.Errorf("ange antingen en köpoffert (--want-good --want-qty --offer-silver) ELLER en säljoffert (--offer-good --offer-qty --want-silver), inte båda/ofullständigt")
			}

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

			destID, resolvedName, ownSettlementID, err := resolveMessengerDest(markers, wanaxes, destName, fromName)
			if err != nil {
				return err
			}

			var tradeOffer map[string]any
			if buySet {
				if msgText == "" {
					msgText = fmt.Sprintf("Trade offer: %g %s for %g silver", wantQty, wantGood, offerSilver)
				}
				tradeOffer = map[string]any{
					"kind":         "buy",
					"want_good":    wantGood,
					"want_qty":     wantQty,
					"offer_silver": offerSilver,
				}
			} else {
				if msgText == "" {
					msgText = fmt.Sprintf("Selling %g %s for %g silver", offerQty, offerGood, wantSilver)
				}
				tradeOffer = map[string]any{
					"kind":        "sell",
					"offer_good":  offerGood,
					"offer_qty":   offerQty,
					"want_silver": wantSilver,
				}
			}

			path := fmt.Sprintf("/api/v1/worlds/%s/settlements/%s/messengers", cfg.WorldID, ownSettlementID)
			data, err := c.post(path, map[string]any{
				"destination_id": destID,
				"message":        msgText,
				"trade_offer":    tradeOffer,
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
			if buySet {
				fmt.Printf("Trade offer dispatched to %s (want %.0f %s, offer %.0f silver) · arrives %s\n",
					resolvedName, wantQty, wantGood, offerSilver, arrivesAt)
			} else {
				fmt.Printf("Sell offer dispatched to %s (selling %.0f %s for %.0f silver) · arrives %s\n",
					resolvedName, offerQty, offerGood, wantSilver, arrivesAt)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&destName, "to", "t", "", "destination settlement name (required)")
	cmd.Flags().StringVar(&wantGood, "want-good", "", "good key to request, e.g. grain, copper, cedar (buy mode)")
	cmd.Flags().Float64Var(&wantQty, "want-qty", 0, "quantity of good to request (buy mode)")
	cmd.Flags().Float64Var(&offerSilver, "offer-silver", 0, "silver to offer in exchange (buy mode)")
	cmd.Flags().StringVar(&offerGood, "offer-good", "", "good key to sell, e.g. copper, cedar (sell mode)")
	cmd.Flags().Float64Var(&offerQty, "offer-qty", 0, "quantity of good to sell (sell mode)")
	cmd.Flags().Float64Var(&wantSilver, "want-silver", 0, "silver to request in exchange (sell mode)")
	cmd.Flags().StringVar(&fromName, "from", "", "your city to send/pay from (default: capital)")
	cmd.Flags().StringVarP(&msgText, "message", "m", "", "optional message text (auto-generated if omitted)")
	_ = cmd.MarkFlagRequired("to")
	return cmd
}
