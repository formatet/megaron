A **standing order** is a caravan route that runs by itself. For a game that runs while you sleep, the **Economy → Automation** tab may be the most important screen you have.

Under **New standing order**, set *From*, *To*, and which end supplies the caravan's crew. Then:

- **Keep at destination** — choose a good and the stock to keep there, for example two hundred grain and fifty fish. Use **Add a good** for another row and **Remove** to drop a row. The caravan keeps those goods topped up to that level at the destination.
- **Bring home** — choose a good and the stock to leave at the destination, for example twenty stone. Zero means bring home everything available. Leave the rows empty for no return cargo. On the way back it brings home anything above that floor.

Press **Create route**. Existing routes are listed below with **Pause**, **Resume** and **Delete**. A route pauses itself when it cannot run — when there is no surplus to send, or no spare citizen to crew the caravan — and the list says why. You are told each time a route sends a caravan, and when it pauses.

## What to use it for

- Keeping a young [[colonies|colony]] fed until its own fields carry it.
- Supplying a city that has a mine but no farmland.
- Pulling timber and stone to where you build.

Route caravans are [[transfers|cargo]] like any other, and can be raided on the road.

## Sea routes

A route between two coastal or harboured settlements with a sea lane between them sails instead of walking — if you have a free Galley or Emporos in the sending port ([[sea]]). That ship is then **locked to the route for as long as it runs**, including the time it spends docked between trips: it cannot be sent anywhere else, loaded, or repaired until the route is paused or deleted. No free hull, and no land road either, pauses the route with the same reason a shortfall would. The ship's hold limits both legs: out, it carries what fits; home, it brings back what fits above your floors, and the rest waits at the destination for the next trip.

Pause or delete a sea route and its ship comes free again: at once if it is sitting in port, empty-handed at the next sweep if it was sitting in the destination, or as soon as it finishes the leg it is already on.

## Route and journey time

Cargo follows a real route around impassable ground. Its path and the terrain costs
are fixed when each leg departs. Caravan time is **1.5 times the march time on that
same path**, rounded once to the nearest whole tick, with at least one tick per leg.
The return leg is priced separately because it enters different terrain. Cargo
weight limits a ship's hold; it does not change journey speed.

Without a traversable road or an eligible ship on a sea lane, a shipment is refused
before goods are taken. A standing route pauses and explains the obstacle. The
arrival tick shown in your cargo list is the scheduled delivery; delays or
interception can prevent that delivery.
