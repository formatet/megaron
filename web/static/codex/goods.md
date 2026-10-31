Everything your cities make, eat and trade is a **good**. The **Economy (⚖)** drawer shows your goods across every settlement, in three tabs: **Goods**, **Transfer** and **Routes & cargo**.

## All goods {#goods-details}

<!-- list: good -->
| Name | In short |
|---|---|
| [[silver]] | Silver is the currency and pays your professional forces. |
| [[incense]] | Incense is the smoke the gods like best, and it grows in very few places. |
| [[bronze]] | Bronze is copper and tin smelted together in a Foundry, and the two never lie close: the scarcity that shapes the whole world. |
| [[cedar]] | Cedar is scarce wood used for ships and advanced building upgrades. |
| [[copper]] | Copper is an ore used with tin to make bronze. |
| [[devotion]] | Devotion is work performed by temple workers, not a stockpiled commodity. |
| [[fish]] | Fish is a food good caught by citizens working fishing places. |
| [[grain]] | Grain is a food good produced by citizens working suitable fields. |
| [[livestock]] | Livestock is the herd kept by a city. |
| [[oil]] | Oil is a food and offering good. |
| [[shipbuilding]] | Shipbuilding is service work performed at a Shipyard, not cargo stored in a warehouse. |
| [[stone]] | Stone is building material quarried from land hexes. |
| [[timber]] | Timber is building material gathered by citizens on suitable wooded ground. |
| [[tin]] | Tin is the scarce ore needed with copper to make bronze. |
| [[wine]] | Wine is not food: it is the drink of the gods and the feast, offered in prayers. |
<!-- /list -->

## How the goods fit together

- **Food** is grain, fish and oil, eaten in proportion to what the city has ([[food]]). Wine is not food. Livestock is the emergency reserve.
- **Timber and stone** build nearly everything. Early on, timber is almost always your bottleneck.
- **Cedar stands in for timber:** whatever a cost needs in timber and you lack, your cedar covers one for one. Cedar also levels your best buildings and builds the [[war-galley|War Galley]].
- **Copper and tin** are the two halves of bronze, and they are never found together. Bronze arms the [[elite-infantry]], the [[war-chariot]], the War Galley and the best walls.
- **Silver** is the currency, and the only thing that pays professional soldiers and crews.
- **Incense** is rare, found only on marked dry hexes, and pleases [[gods|the gods]] more than any other offering.

## Economy tabs {#goods-the-three-tabs}

- **Goods** — stock, production and use of each good, city by city. The settlements table compares people, food produced minus food eaten, food game days and shortage status; select a city to close Economy and open its City drawer. Goods piling up with nothing using them are marked — see [[surplus]].
- **Transfer** — move goods between your own cities ([[transfers]]).
- **Routes & cargo** — [[routes|standing orders]] that move goods for you while you are away.

To learn what another city needs or can spare, ask its Wanax in a [[messengers|letter]] through **Diplomacy → Correspondence**, then agree a [[trade|trade offer]].

## Goods for offerings and trade {#goods-offerings-and-trade}

The gods weigh goods differently: bronze can count for far more than grain. **Kult**, at the temple, and `keryx rite --list` show their current reckoning before each god's preferences. What the world can spare and what the god favours both matter ([[rites]]). Wanaxes agree their own trade prices.

Production per game day is output before meals and other uses. Food net combines grain, fish and oil production minus population demand. The city has one stock of each good; food coverage counts these three goods together. Shortfall waiting estimates assume current production continues, before other use.

Food for people combines the three food goods minus the population’s meal. **Food after army** subtracts the food supplied by this city to its supported troops as well. The army shares grain, fish and oil in the same proportions; its silver pay is shown separately. Population growth still uses the population-only food balance.

## Storage capacity and forecasts {#goods-storage-capacity}

Each good has its own storage capacity in each city. A city stores **2 000 of each food** — grain, fish and oil — and a [[buildings|Granary]] raises that to 4 000, 10 000 and 20 000. The herd has its own pens. A city that already held more food when this ceiling came in keeps it and can eat it, but it never fills back above the ceiling. **StorageCapacityWarning** is an advance dispatch when the server forecasts a stock nearing its cap. Keryx status and the City drawer show the good, stored amount, capacity and **full in N game days**. The estimate uses the current net rate and deliveries on the way; it is a forecast, so changing production or orders can change it.

The forecast counts your own transports and accepted trade deliveries that you know about. An unsolicited gift from another Wanax is known only when it arrives, so it is not included beforehand.

Send goods away by [[transfers|transfer]] or use them in [[trade]]. Warehouse construction and storage-pool expansion are not available yet.

### Goods lost to full storage {#goods-storage-capacity-2}

A **StorageOverflow** notification means goods have already been lost because storage was full. It records the city, good, quantity lost and the source, such as production, a delivery, returned provisions or a refund. It does not mean the entire shipment was lost: only the reported excess failed to fit. Production into a store that stays full is lost every game day; you are told when it starts, not every game day it goes on, even when the store refills to the brim every few game days. Once nothing of that good has been lost for 24 game days, a new loss is told again. If the forecast clears and the city later approaches capacity again, a new warning can be sent.
