package capabilities

import "fmt"

// canMessage requires a contacted foreign settlement — the same FOW gate
// messenger Send enforces. TODO: Fas 3 unify with handler gate.
func canMessage(cc checkContext) Verb {
	foreign := cc.visibleForeignSettlements()
	ok := foreign > 0
	return verb("message", CategoryDiplomacy,
		"Send a free-text message to another Wanax (no goods).",
		[]Requirement{
			req("a contacted foreign settlement (FOW-visible)", ok,
				fmt.Sprintf("%d visible foreign settlement(s)", foreign),
				"march or send a messenger to scout a neighbour before contacting it"),
		})
}

func canReply(cc checkContext) Verb {
	n := cc.repliableInbox()
	ok := n > 0
	return verb("reply", CategoryDiplomacy,
		"Reply to a delivered inbox message.",
		[]Requirement{
			req("a delivered message awaiting reply", ok,
				fmt.Sprintf("%d repliable message(s)", n),
				"wait for a messenger to arrive, or check `keryx inbox`"),
		})
}

// canMessenger is the generalized Send command (message + optional trade
// offer in one call) — same FOW gate as message/trade-offer.
func canMessenger(cc checkContext) Verb {
	foreign := cc.visibleForeignSettlements()
	ok := foreign > 0
	return verb("messenger", CategoryDiplomacy,
		"Send a messenger to another settlement, optionally carrying a trade offer.",
		[]Requirement{
			req("a contacted foreign settlement (FOW-visible)", ok,
				fmt.Sprintf("%d visible foreign settlement(s)", foreign),
				"march or send a messenger to scout a neighbour before contacting it"),
		})
}
