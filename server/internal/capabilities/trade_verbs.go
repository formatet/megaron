package capabilities

import "fmt"

// HintTradeAcceptInsolvent is trade-accept's actionable hint when the
// accepting settlement cannot yet cover a pending offer's cost. Shared
// verbatim between canTradeAccept's solvency requirement and
// MessengerHandler.TradeAccept's 422 (Fas 3 anti-drift) — the two check
// different scopes (canTradeAccept's earliest pending offer, in general, vs.
// TradeAccept's one specific offer being accepted), so a shared constant
// keeps the actionable text identical without forcing the handler through
// the aggregate checker for a per-offer decision.
const HintTradeAcceptInsolvent = "you can't yet cover this offer's cost — wait for silver/goods to accrue, or decline the offer"

// CanTradeOffer exposes canTradeOffer to api/handlers.MessengerHandler.Send,
// whose trade_offer (kind="buy") precondition uses it (Fas 3 anti-drift).
func CanTradeOffer(cc checkContext) Verb { return canTradeOffer(cc) }

// CanSell exposes canSell to api/handlers.MessengerHandler.Send, whose
// trade_offer (kind="sell") precondition uses it (Fas 3 anti-drift).
func CanSell(cc checkContext) Verb { return canSell(cc) }

// CanTradeAccept exposes canTradeAccept for symmetry with the other exported
// checkers; MessengerHandler.TradeAccept reuses HintTradeAcceptInsolvent
// directly rather than the whole verb (see that constant's doc).
func CanTradeAccept(cc checkContext) Verb { return canTradeAccept(cc) }

// canTradeOffer covers the "buy" mode of trade-offer: request a good from a
// contacted Wanax in exchange for silver you hold.
// Fas 3: api/handlers.MessengerHandler.Send calls CanTradeOffer/CanSell as an
// early aggregate precondition (sound: if NO foreign settlement is visible at
// all, the specific destination Send targets — which must itself be a
// foreign settlement — cannot be visible either). Send's own per-destination
// FOW check remains the authoritative, more specific gate below that.
func canTradeOffer(cc checkContext) Verb {
	foreign := cc.visibleForeignSettlements()
	foreignOK := foreign > 0
	silver := cc.goodAmount("silver")
	silverOK := silver > 0
	return verb("trade-offer", CategoryTrade,
		"Send a buy offer (want a good, offer silver) to a contacted Wanax — bilateral, FOW-gated.",
		[]Requirement{
			req("a contacted foreign settlement (FOW-visible)", foreignOK,
				fmt.Sprintf("%d visible foreign settlement(s)", foreign),
				"march or send a messenger to scout a neighbour before contacting it"),
			req("silver to offer", silverOK,
				fmt.Sprintf("silver %.0f", silver),
				"sell goods or tax production for silver first"),
		})
}

// canSell covers the "sell" mode of trade-offer: offer a good you hold in
// exchange for silver. Named "trade-offer" (not "sell") in the actions
// listing — "sell" isn't a CLI command, it's the sell-mode flag combo
// (--offer-good/--offer-qty/--want-silver) on `poleia trade-offer`; a
// distinct verb name that names no real command left agents unable to act
// on the hint (Fas 1b anti-drift).
// TODO: Fas 3 unify with handler gate.
func canSell(cc checkContext) Verb {
	foreign := cc.visibleForeignSettlements()
	foreignOK := foreign > 0
	good, qty := cc.anySellableGood()
	goodOK := good != ""
	detail := "no tradeable surplus goods in stock"
	if goodOK {
		detail = fmt.Sprintf("%s %.0f in stock", good, qty)
	}
	return verb("trade-offer", CategoryTrade,
		"Send a sell offer to a contacted Wanax: trade-offer --offer-good <good> --offer-qty <n> --want-silver <n> — bilateral, FOW-gated.",
		[]Requirement{
			req("a contacted foreign settlement (FOW-visible)", foreignOK,
				fmt.Sprintf("%d visible foreign settlement(s)", foreign),
				"march or send a messenger to scout a neighbour before contacting it"),
			req("a good in stock to sell", goodOK, detail,
				"allocate labor to a producible good, or wait for stock to accrue"),
		})
}

// canTradeAccept gates on a pending inbound offer plus (temenos_capabilities.md
// Fas 3.5) solvency: whether this settlement can currently cover the
// EARLIEST such offer's cost. Solvency is only meaningful once a pending
// offer exists, so the second requirement is only appended then.
func canTradeAccept(cc checkContext) Verb {
	ok := cc.pendingInboundTradeOffer()
	reqs := []Requirement{
		req("a pending inbound trade offer", ok,
			boolDetail(ok, "an offer is waiting", "no pending inbound offer"),
			"wait for a Wanax to send a trade offer, or check `poleia inbox`"),
	}
	if ok {
		solvent, detail := cc.pendingOfferAffordable()
		reqs = append(reqs, req("solvent for the pending offer's cost", solvent, detail,
			HintTradeAcceptInsolvent))
	}
	return verb("trade-accept", CategoryTrade, "Accept a pending inbound trade offer.", reqs)
}

// pendingOfferAffordable reports whether this settlement can currently
// afford to accept its EARLIEST pending inbound trade offer — the acceptor's
// side of the cost (silver for a "sell" offer, the wanted good for a "buy"
// offer). Listing-level only: MessengerHandler.TradeAccept checks the
// SPECIFIC offer being accepted via TradeOfferAffordable directly, since a
// settlement may have more than one pending offer outstanding at once.
func (cc checkContext) pendingOfferAffordable() (ok bool, detail string) {
	if !cc.hasSettlement() {
		return true, ""
	}
	var kind, wantGood string
	var wantQty, wantSilver float64
	err := cc.pool.QueryRow(cc.ctx,
		`SELECT COALESCE(trade_offer->>'kind','buy'),
		        COALESCE(trade_offer->>'want_good',''),
		        COALESCE((trade_offer->>'want_qty')::float,0),
		        COALESCE((trade_offer->>'want_silver')::float,0)
		   FROM messengers
		  WHERE destination_id = $1 AND status = 'delivered'
		    AND trade_offer IS NOT NULL AND trade_offer->>'status' = 'pending'
		  ORDER BY arrives_at ASC LIMIT 1`,
		cc.settlementID,
	).Scan(&kind, &wantGood, &wantQty, &wantSilver)
	if err != nil {
		return true, ""
	}
	afford, have := TradeOfferAffordable(cc, kind, wantGood, wantQty, wantSilver)
	need, key := wantQty, wantGood
	if kind == "sell" {
		need, key = wantSilver, "silver"
	}
	return afford, fmt.Sprintf("%s %.0f/%.0f", key, have, need)
}

// TradeOfferAffordable reports whether cc's settlement holds enough to cover
// the ACCEPTOR side of a trade offer: silver >= wantSilver for a "sell" offer
// (the accepting buyer pays silver), or wantGood >= wantQty for a "buy" offer
// (the accepting seller ships goods) — mirrors MessengerHandler.TradeAccept's
// own deduction gate exactly (temenos_capabilities.md Fas 3.5).
func TradeOfferAffordable(cc checkContext, kind, wantGood string, wantQty, wantSilver float64) (ok bool, have float64) {
	if kind == "sell" {
		have = cc.goodAmount("silver")
		return have >= wantSilver, have
	}
	have = cc.goodAmount(wantGood)
	return have >= wantQty, have
}

func canTradeDecline(cc checkContext) Verb {
	ok := cc.pendingInboundTradeOffer()
	return verb("trade-decline", CategoryTrade,
		"Decline a pending inbound trade offer.",
		[]Requirement{
			req("a pending inbound trade offer", ok,
				boolDetail(ok, "an offer is waiting", "no pending inbound offer"),
				"wait for a Wanax to send a trade offer, or check `poleia inbox`"),
		})
}

func canTradeCancel(cc checkContext) Verb {
	ok := cc.pendingOutboundTradeOffer()
	return verb("trade-cancel", CategoryTrade,
		"Cancel your own pending outgoing trade offer and reclaim escrowed silver/goods.",
		[]Requirement{
			req("a pending outgoing trade offer", ok,
				boolDetail(ok, "an offer is outstanding", "no pending outbound offer"),
				"send a trade offer first, or check `poleia outbox`"),
		})
}

// canTransfer is the internal (own -> own) logistics verb — CLI aliases
// `transfer` and `trade` both hit ProvinceHandler.Trade, which rejects any
// destination not owned by the caller. Requires a second own settlement and
// a good to move. TODO: Fas 3 unify with handler gate.
func canTransfer(cc checkContext) Verb {
	total, _ := cc.ownSettlements()
	destOK := total >= 2
	good, qty := cc.anySellableGood()
	goodOK := good != ""
	detail := "no goods in stock to move"
	if goodOK {
		detail = fmt.Sprintf("%s %.0f in stock", good, qty)
	}
	return verb("transfer", CategoryTrade,
		"Send goods to one of your own settlements — no consent needed, no loss.",
		[]Requirement{
			req("a second own settlement to send to", destOK,
				fmt.Sprintf("%d/2 own settlements", total),
				"found or hold a colony before transferring between your own cities"),
			req("a good in stock to move", goodOK, detail,
				"allocate labor to a producible good, or wait for stock to accrue"),
		})
}

// canGift is the loyalty-boosting caravan from the player's capital to one
// of their own colonies (api/handlers.SettlementHandler.Gift). Distinct from
// canTransfer (own→own logistics, any good, no loyalty effect): gift moves
// only silver/grain and applies +1 loyalty at 50+ silver-equivalent sent.
func canGift(cc checkContext) Verb {
	_, nonCapital := cc.ownSettlements()
	destOK := nonCapital >= 1
	silver := cc.capitalGoodAmount("silver")
	grain := cc.capitalGoodAmount("grain")
	haveOK := silver > 0 || grain > 0
	return verb("gift", CategoryTrade,
		"Send silver/grain from your capital to one of your own colonies — boosts loyalty at 50+ silver-equivalent.",
		[]Requirement{
			req("a colony to gift to", destOK,
				fmt.Sprintf("%d colonies", nonCapital),
				"found or hold a colony before sending a gift"),
			req("silver or grain at your capital", haveOK,
				fmt.Sprintf("silver %.0f, grain %.0f", silver, grain),
				"tax production for silver, or wait for grain to accrue"),
		})
}

// anySellableGood returns the first non-silver good in stock (amount > 0),
// deterministically ordered, for use as an example in a Detail string.
func (cc checkContext) anySellableGood() (key string, qty float64) {
	if !cc.hasSettlement() {
		return "", 0
	}
	err := cc.pool.QueryRow(cc.ctx,
		`SELECT good_key, GREATEST(0, settled(amount, rate, calc_tick)) AS have
		   FROM settlement_goods
		  WHERE settlement_id = $1 AND good_key != 'silver'
		    AND settled(amount, rate, calc_tick) > 0
		  ORDER BY good_key LIMIT 1`,
		cc.settlementID,
	).Scan(&key, &qty)
	if err != nil {
		return "", 0
	}
	return key, qty
}
