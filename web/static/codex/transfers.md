A **transfer** moves goods between two of your *own* settlements. No consent, no negotiation — simpler than [[trade]] with a stranger.

Open **Economy → Transfer**, choose *From*, *To*, the goods and their quantities, and press **Send →**. Below, **Your cargo in transit** lists everything on the road — which city it left, which city it is bound for, and its arrival time. A notification tells you when it is delivered.

## Physical cargo {#transfers-cargo-is-physical}

A caravan is people and pack animals walking across the map. Others can see it on the road, but not what it carries — the cargo is a secret until someone takes it. **It can be intercepted and seized** — yours by others, theirs by you. A hostile unit on watch ([[stances|sentry]]) near the road will stop a caravan that passes. Only [[messengers]] are safe. When a land caravan is seized, both sides are told where, and exactly what cargo changed hands; the seized goods go to the raider's capital.

An internal transfer carries exactly the same risk as a trade or a gift: on land only raiders, at sea also [[sea|storms]]. Whose goods they are never changes the risk.

To move goods regularly without being there, use a [[routes|standing order]].

## Naval transfer requirements {#transfers-sending-it-by-sea}

With automatic routing, when both ends are coastal or harboured and a sea lane connects them, a transfer sails instead of walking — **if you have a free [[galley]] or [[emporos]] waiting in the sending port** ([[sea]]). No free hull there, and no land road either, and the transfer is refused with the reason why; build one at a [[shipyard|Shipyard]], or wait for one to come home.

The ship is bound to the round trip: it cannot be given any other order until it has sailed back and unloaded, empty, at its own port. What it can carry is limited by the hull's own capacity — a shipment above the limit is refused before any goods leave your stock. **One good takes one cargo place**, whether [[grain|grain]], [[stone|stone]] or [[silver|silver]]: **Galley 20, Emporos 120 goods of any kind**. The form shows the ready hull and a live **Cargo used / capacity** meter; an overload tells you how much to remove. Land caravans have no cargo limit.

A laden ship caught at sea does not always lose the same way a caravan does. One of three things happens, and you are told which:

### Capture, damage and loss of cargo {#transfers-sending-it-by-sea-2}

- **Captured** — the raider takes the cargo *and* the ship. It sails off under a new flag.
- **Damaged** — half of every good is lost overboard in the fight; the ship limps home with the rest.
- **Sunk** — the cargo and the ship are both gone.

Of every ten ships caught, about four are captured, four limp home damaged and two are sunk. <!-- src: server/internal/transport/intercept.go rollNavalSeizureOutcome -->

Sending to another Wanax is a [[gifts|gift or tribute]], with nothing requested in return.

## Cargo routes and travel time {#transfers-route-and-journey-time}

Cargo follows a real route around impassable ground. Its path and the [[terrain|terrain]] costs
are fixed when each leg departs. Caravan time is **1.5 times the march time on that
same path**, rounded once to the nearest whole game day, with at least one game day per leg.
The return leg is priced separately because it enters different terrain. Cargo
quantity limits a ship's hold; it does not change journey speed.

Without a traversable road or an eligible ship on a sea lane, a shipment is refused
before goods are taken. A standing route pauses and explains the obstacle. The
arrival time shown in your cargo list is the scheduled delivery; delays or
interception can prevent that delivery.

In Keryx, `transfer` sends from your **selected city**, which may be a [[colonies|colony]].
Check it with `keryx status`, or choose the sending city explicitly:
`keryx transfer --from Knossos --dest Mallia --good grain --qty 50`.
The sending city cannot also be the destination. A rejected self-transfer sends
nothing; use `--from` to choose the other city you meant to send from.

### Port approaches and mixed cargo {#transfers-route-and-journey-time-2}

A coastal city can load or receive through any adjacent navigable water hex. The sea route compares those approaches and chooses the fastest reachable one; it does not fix the port to an arbitrary side of the city. Journeys already at sea keep their saved route.

Choose several goods in the same form to send them together in **one shipment**:
one caravan or one ship, one arrival, one shared fate if intercepted or sunk.
Keryx accepts paired flags: `keryx transfer --dest Mallia --good grain --qty 12 --good timber --qty 8`.
Keryx shows the same cargo total and hull capacity before dispatch.

## Choosing land or sea {#transfers-choose-land-or-sea}

Economy → Transfer compares **Land** and **Sea** before you send a transfer or gift. Both show the journey in game days and real time, its arrival estimate, risk and requirements. An unavailable alternative explains why: a missing connected path, no departure/arrival coast or harbour, or no free ship with cargo space. Times assume departure now; an accepted agreement or standing route departs later and checks again. A preview reserves nothing.

Land cargo can be intercepted. Sea cargo can be intercepted and the ship can meet [[storms|storms]]; the same risks apply to your own goods, gifts, trade and standing routes. Sea needs your available Galley or Emporos at departure, enough cargo space and the existing voyage provisions.

Your selection is saved. An explicitly chosen land route never turns into a sea voyage, or vice versa, when conditions change. A standing route waits or pauses according to its existing rules if it cannot use the chosen mode. Older orders without a selection keep automatic routing.

### Land and sea choice in Keryx {#transfers-choose-land-or-sea-2}

Keryx: `transfer --by land --good grain --qty 50 --dest Petras`, or `--by sea`. The same flag works with `trade-offer` and `route`. Omit it for the existing automatic choice; the preview lists alternatives. For trade, the agreed choice covers the physical goods and payment legs, not the [[runners|runner]] carrying the offer.
