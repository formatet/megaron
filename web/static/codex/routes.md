A **standing order** is a land or sea cargo route that runs by itself. For a game that runs while you sleep, the **Keep it stocked** box in **Economy → Send goods** may be the most important control you have: check it, and the numbers mean *keep this much there*. The city you send from crews the route.

In **Send goods**, set *From* and *To* (one of your own cities), check **Keep it stocked**, then:

## Standing route stock targets {#routes-details}

- **Keep at destination** — type beside each good the stock to keep there, for example 200 [[grain|grain]] and 50 [[fish|fish]]. The caravan keeps those goods topped up to that level at the destination.
- **Bring home** (under **More**) — type beside a good the stock to leave at the destination, for example twenty [[stone|stone]]. Zero means bring home everything available. Leave the rows empty for no return cargo. On the way back it brings home anything above that floor.

### Managing standing routes {#routes-details-2}

Press **Keep it stocked →**. Existing routes are listed under **On the road** with **Pause**, **Resume** and **Delete**. Each route shows **Keep at destination** (the target stock) and **Bring home above** (the stock left there before loading return cargo), including a zero floor. **Return cargo: none** means no goods are configured for the homeward leg. These are route settings; the cargo in each trip depends on available surplus and capacity. `keryx routes` shows the same settings. A route pauses itself when it cannot run — when there is no surplus to send, or no spare [[citizens|citizen]] to crew the caravan — and the list says why. You are told each time a route sends a caravan, and when it pauses.

## Uses for standing routes {#routes-what-to-use-it-for}

- Keeping a young [[colonies|colony]] fed until its own fields carry it.
- Supplying a city that has a mine but no farmland.
- Pulling stone and [[timber|timber]] to where you build.

Route caravans are [[transfers|cargo]] like any other, and can be raided on the road.

## Standing sea routes {#routes-sea-routes}

With automatic routing, a route between two coastal or harboured settlements with a sea lane between them sails instead of walking — if you have a free [[galley]] or [[emporos]] in the sending port ([[sea]]). That ship is then **locked to the route for as long as it runs**, including the time it spends docked between trips: it cannot be sent anywhere else, loaded, or repaired until the route is paused or deleted. No free hull, and no land road either, pauses the route with the same reason a shortfall would. The ship's hold limits both legs: out, it carries what fits; home, it brings back what fits above your floors, and the rest waits at the destination for the next trip.

Pause or delete a sea route and its ship comes free again: at once if it is sitting in port, empty-handed at the next sweep if it was sitting in the destination, or as soon as it finishes the leg it is already on.

## Standing route journey times {#routes-route-and-journey-time}

Cargo follows a real route around impassable ground. Its path and the [[terrain|terrain]] costs
are fixed when each leg departs. Caravan time is **1.5 times the march time on that
same path**, rounded once to the nearest whole game day, with at least one game day per leg.
The return leg is priced separately because it enters different terrain. The number
of goods limits a ship's hold; it does not change journey speed.

Without a traversable road or an eligible ship on a sea lane, a shipment is refused
before goods are taken. A standing route pauses and explains the obstacle. The
arrival game day shown in your cargo list is the scheduled delivery; delays or
interception can prevent that delivery.

## Following a route’s cargo {#routes-follow-your-cargo}

**Economy → On the road** shows the ship or caravan travelling on each
standing route, with its cargo and arrival. Other shipments are listed
separately. Both land and sea cargo live here; **War → Marches** is for the
forces you order to move. A route's ship remains in **War → Forces → Ships**,
with its route named on the card.

Choose **By land** or **By sea** in Send goods before creating a route; Keryx uses `route --by land` or `route --by sea`. The choice is saved for subsequent departures. The comparison explains travel time, risk and unavailable alternatives; see [[transfers|land or sea]].
