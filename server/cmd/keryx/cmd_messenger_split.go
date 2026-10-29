package main

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

// messageCmd sends a free-text message to another Wanax's settlement.
// No goods — pure diplomatic communication. FOW-gated on the server.
// This is SB8 verb #1: distinct from trade-offer and transfer.
func messageCmd() *cobra.Command {
	var destName, text, fromName, tellPlace string
	var fromHost bool
	cmd := &cobra.Command{
		Use:   "message",
		Short: "Send a free-text message to another Wanax (no goods)",
		Example: `  keryx message --to Korinth --text "Greetings, neighbour"
  keryx message --from-host --to Korinth --text "A people seeks passage"
  keryx message --to Korinth --text "Hear of this wood" --tell-place "Dark Wood"`,
		Args: rejectPositionalArgs("to"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if fromHost && fromName != "" {
				return fmt.Errorf("--from and --from-host cannot be combined — the host IS the sender")
			}
			if fromHost && tellPlace != "" {
				return fmt.Errorf("--tell-place goes with a letter from a city, not from the host")
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
				fmt.Printf("Messenger dispatched from the host to %s · arrives %s\n", resolvedName, arrivalETA(c, arrivesAt))
				printPassageNote(resp)
				return nil
			}

			destID, destName, ownSettlementID, err := resolveMessengerDest(markers, wanaxes, destName, fromName)
			if err != nil {
				return err
			}

			path = fmt.Sprintf("/api/v1/worlds/%s/settlements/%s/messengers", cfg.WorldID, ownSettlementID)
			body := map[string]any{
				"destination_id": destID,
				"message":        text,
				// No trade_offer — pure message
			}
			if tellPlace != "" {
				story, err := resolveTellPlace(c, markers, tellPlace)
				if err != nil {
					return err
				}
				body["tell_place"] = story
			}
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
			fmt.Printf("Message dispatched to %s · arrives %s\n", destName, arrivalETA(c, arrivesAt))
			if tellPlace != "" {
				fmt.Printf("It tells of %s — what your people had seen of it as the Runner left.\n", tellPlace)
			}
			printPassageNote(resp)
			return nil
		},
	}
	cmd.Flags().StringVarP(&destName, "to", "t", "", "destination settlement name (required)")
	cmd.Flags().StringVarP(&text, "text", "x", "", "message text (required)")
	cmd.Flags().StringVar(&fromName, "from", "", "your city to send from (default: capital)")
	cmd.Flags().BoolVar(&fromHost, "from-host", false, "send from your wandering Nomadic Host (founder phase) instead of a city")
	cmd.Flags().StringVar(&tellPlace, "tell-place", "", "tell of a named place or a city your people have seen (see keryx places)")
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
	var by, counterID string

	cmd := &cobra.Command{
		Use:   "trade-offer",
		Short: "Send a trade offer (bilateral consent, FOW-gated) to another Wanax",
		Example: `  keryx trade-offer --to Korinth --want-good grain --want-qty 100 --offer-silver 80
  keryx trade-offer --to Athenai --offer-good copper --offer-qty 50 --want-silver 120`,
		Args: rejectPositionalArgs("to"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := validateTransportBy(by); err != nil {
				return err
			}
			buySet := wantGood != "" && wantQty > 0 && offerSilver > 0
			sellSet := offerGood != "" && offerQty > 0 && wantSilver > 0
			if buySet == sellSet {
				return fmt.Errorf("specify either a buy offer (--want-good --want-qty --offer-silver) OR a sell offer (--offer-good --offer-qty --want-silver), not both/incomplete")
			}

			if counterID != "" {
				if _, err := uuid.Parse(counterID); err != nil {
					return fmt.Errorf("--counter requires the exact original messenger UUID")
				}
				if destName != "" || fromName != "" {
					return fmt.Errorf("--counter binds the original cities; omit --to and --from")
				}
			} else if destName == "" {
				return fmt.Errorf("provide --to or --counter")
			}
			c := newClient(cfg)

			var destID, resolvedName, ownSettlementID string
			if counterID == "" {
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

				destID, resolvedName, ownSettlementID, err = resolveMessengerDest(markers, wanaxes, destName, fromName)
				if err != nil {
					return err
				}

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

			quantity := max(wantQty, offerSilver)
			if sellSet {
				quantity = max(offerQty, wantSilver)
			}
			if err := func() error {
				if counterID != "" {
					return nil
				}
				return previewTransportOptions(c, ownSettlementID, destID, by, quantity)
			}(); err != nil {
				return err
			}
			if by != "" {
				tradeOffer["by"] = by
			}
			path := fmt.Sprintf("/api/v1/worlds/%s/settlements/%s/messengers", cfg.WorldID, ownSettlementID)
			body := map[string]any{"destination_id": destID, "message": msgText, "trade_offer": tradeOffer}
			if counterID != "" {
				path = fmt.Sprintf("/api/v1/worlds/%s/messengers/%s/trade-counter", cfg.WorldID, counterID)
				delete(body, "destination_id")
			}
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
			if counterID != "" {
				fmt.Printf("Counter-offer dispatched · arrives %s. A new agreement is needed.\n", arrivalETA(c, arrivesAt))
			} else if buySet {
				fmt.Printf("Trade offer dispatched to %s (want %.0f %s, offer %.0f silver) · arrives %s\n",
					resolvedName, wantQty, wantGood, offerSilver, arrivalETA(c, arrivesAt))
			} else {
				fmt.Printf("Sell offer dispatched to %s (selling %.0f %s for %.0f silver) · arrives %s\n",
					resolvedName, offerQty, offerGood, wantSilver, arrivalETA(c, arrivesAt))
			}
			printPassageNote(resp)
			return nil
		},
	}
	cmd.Flags().StringVar(&by, "by", "", "goods route: land or sea (omitted: automatic)")

	cmd.Flags().StringVarP(&destName, "to", "t", "", "destination settlement name (required)")
	cmd.Flags().StringVar(&wantGood, "want-good", "", "good key to request — must be a tradeable good (GET /api/v1/goods lists them; an unknown key is rejected with the full list) (buy mode)")
	cmd.Flags().Float64Var(&wantQty, "want-qty", 0, "quantity of good to request (buy mode)")
	cmd.Flags().Float64Var(&offerSilver, "offer-silver", 0, "silver to offer in exchange (buy mode)")
	cmd.Flags().StringVar(&offerGood, "offer-good", "", "good key to sell — must be a tradeable good (GET /api/v1/goods lists them; an unknown key is rejected with the full list) (sell mode)")
	cmd.Flags().Float64Var(&offerQty, "offer-qty", 0, "quantity of good to sell (sell mode)")
	cmd.Flags().Float64Var(&wantSilver, "want-silver", 0, "silver to request in exchange (sell mode)")
	cmd.Flags().StringVar(&fromName, "from", "", "your city to send/pay from (default: capital)")
	cmd.Flags().StringVarP(&msgText, "message", "m", "", "optional message text (auto-generated if omitted)")
	cmd.Flags().StringVar(&counterID, "counter", "", "counter a delivered offer by its exact messenger UUID; omit --to/--from")
	return cmd
}
