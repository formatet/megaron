package capabilities

import "fmt"

// canTradeOffer covers the "buy" mode of trade-offer: request a good from a
// contacted Wanax in exchange for silver you hold.
// TODO: Fas 3 unify with handler gate.
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

// canSell covers the "sell" mode: offer a good you hold in exchange for silver.
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
	return verb("sell", CategoryTrade,
		"Send a sell offer (offer a good, want silver) to a contacted Wanax — bilateral, FOW-gated.",
		[]Requirement{
			req("a contacted foreign settlement (FOW-visible)", foreignOK,
				fmt.Sprintf("%d visible foreign settlement(s)", foreign),
				"march or send a messenger to scout a neighbour before contacting it"),
			req("a good in stock to sell", goodOK, detail,
				"allocate labor to a producible good, or wait for stock to accrue"),
		})
}

func canTradeAccept(cc checkContext) Verb {
	ok := cc.pendingInboundTradeOffer()
	return verb("trade-accept", CategoryTrade,
		"Accept a pending inbound trade offer.",
		[]Requirement{
			req("a pending inbound trade offer", ok,
				boolDetail(ok, "an offer is waiting", "no pending inbound offer"),
				"wait for a Wanax to send a trade offer, or check `poleia inbox`"),
		})
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
