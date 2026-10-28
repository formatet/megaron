**A trade offer travels attached to a letter.** There is no market screen and no public listings. You write to a specific city and pin an offer to the message — so you can only trade with cities you have [[contact|contacted]].

The **[[market]]** needs no workers. It has no staffing places; trade does not depend on assigning [[citizens|citizens]] to it.

## Making a trade offer {#trade-making-an-offer}

In a **Diplomacy → Correspondence** conversation (start one with **Known → Write**), expand **+ Attach trade offer** and choose:

- **Buy** — the good you want, the quantity, and the silver you offer. Your silver is held in escrow.
- **Sell** — the good you offer, the quantity, and the silver you want. Your goods are held in escrow.

Then **Dispatch →**. The offer walks with the [[messengers|messenger]], so it takes game days to arrive.

## Accepting or declining an offer {#trade-answering-an-offer}

A trade offer is a proposal whose goods or silver are held aside until it is answered or expires.

See [[trade-offer-expiry]] for the reply window.

An incoming offer appears in the conversation, for example *"Wants 40 [[tin|tin]] · Offers 200 silver"*, with **Accept ✓**, **Counter-offer** and **Decline ✗**. Your own pending offers carry **Cancel offer ✗** — withdraw it and your escrow comes back. If nobody answers before it expires, the escrow comes back too. Nothing is ever lost to silence.

The runner who carried your offer waits at the other city only while the offer is open: once it is accepted, declined or cancelled, the runner turns for home that same game day. **You learn the answer when the runner is home** — the news travels with the runner, never ahead of it. A declined offer's escrow travels with the runner too: your goods or silver are back in your stores the game day the runner is home. If the runner is lost at sea on the way, the escrow comes back at once. If nobody answers, no news comes: the offer lapses at its deadline, the escrow is back in your stores that same game day, and your letters show it as expired.

**Counter-offer.** If you like the trade but not the numbers, press **Counter-offer**. One line opens with the same good and the roles turned round — *Sell [40] tin for [200] silver* — and you change the numbers and press **Send**. Your own runner carries your terms back, and your goods or silver are held aside as in any offer. The other offer is declined in the same moment, and its runner goes home with its escrow. The other ruler must agree to your new terms; nothing is settled until they do. If your counter-offer cannot be sent (you lack the goods, or the offer has just expired), nothing changes and the original offer still stands.

## Delivering a trade agreement {#trade-delivery}

An accepted trade is carried along its agreed land or sea route. It arrives as a *Trade delivered* notification. On land nothing is lost by chance; like any [[transfers|cargo]] it can be raided. At sea every ship risks [[sea|storms]].

### When it goes by sea

With automatic routing, if both cities are coastal or harboured and a sea lane connects them, the trade sails instead of walking — carried by **your own** ship, whichever of you *sent* the offer. It makes the whole round trip: out with your half of the deal when the other side accepts, home again with their half. The other side's ships are never touched.

If you send an offer and have no free [[galley]] or [[emporos]] in your own port, and no land road connects the two cities either, the offer cannot be sent at all. If your only ship is taken by something else between sending and the other side's answer, their **Accept** fails and tells them so — your offer simply stays open until it expires or a ship frees up. A shipment too heavy for the ship's hold is refused the same way, naming the capacity.

### Returning damaged trade cargo {#trade-delivery-2}

The ship is not free for anything else until it is home — outbound and return both count, same [[transfers|three fates]] (captured, damaged, sunk) as any other cargo at sea. A **Damaged** outcome on the way home still gets half the cargo back to you, never to the other side.

## Finding trade partners {#trade-finding-partners}

Ask the Wanax of a [[contact|contacted]] city what they need or can spare. Write a [[messengers|letter]] in **Diplomacy → Correspondence** (start one with **Known → Write**), then negotiate in the reply before attaching a trade offer.

What is most worth trading? [[silver]], and the half of [[bronze]] you do not have.

## Trade agreement routes {#trade-route-and-journey-time}

Cargo follows a real route around impassable ground. Its path and the [[terrain|terrain]] costs
are fixed when each leg departs. Caravan time is **1.5 times the march time on that
same path**, rounded once to the nearest whole game day, with at least one game day per leg.
The return leg is priced separately because it enters different terrain. The number
of goods limits a ship's hold; it does not change journey speed.

Without a traversable road or an eligible ship on a sea lane, a shipment is refused
before goods are taken. A standing route pauses and explains the obstacle. The
arrival game day shown in your cargo list is the scheduled delivery; delays or
interception can prevent that delivery.

The seller receives the full agreed silver payment when it arrives. [[temple|Temples]] take no share of religious-goods sales.

Cargo follows its planned road or sea route one province at a time. The map and `keryx cargo` show its current province and arrival time. A province is reached only after its share of the full journey; the final province is reached at the announced arrival time.

### A trade agreement’s land or sea choice {#trade-route-and-journey-time-2}

Attach a trade offer in Letters and choose **Land** or **Sea** for the goods and payment. The runner still follows its own physical route. The choice stays with the agreement and is checked again on acceptance; Keryx uses `trade-offer --by land` or `--by sea`. See [[transfers|route requirements and risks]].

## Trade offer expiry {#trade-offer-expiry}

An offer’s expiry is the deadline for accepting or declining it.

New offers stay open for **24 game days after arrival**, and their runner waits for that same window. Offers already sent keep their recorded deadline. **Diplomacy → Letters** shows the reply deadline on your local clock; Keryx `outbox` shows when a pending offer expires.

No dispatch announces an expiry. At the deadline the offer lapses, its goods or silver are released back to your stores, and the letter reads *Expired without answer*.

<!-- src: server/internal/messenger/handler.go OfferExpiryTicks; saved ScheduledOfferExpiry deadlines remain unchanged -->

See [[trade-answering-an-offer]], [[trade]] and [[runner-return]].
