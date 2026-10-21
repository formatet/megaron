Everything your cities make, eat and trade is a **good**. The **Economy (⚖)** drawer shows your goods across every settlement, in four tabs: **Goods**, **Transfer**, **Automation** and **Wants**.

| Good | What it is for |
|---|---|
| **Grain, fish, oil, wine** | Equal food goods, eaten in proportion to availability ([[food]]). Oil and wine also serve as offerings for [[rites|prayers]]. |
| **Livestock** | Breeds itself in the pen (a calf per ten animals, fed with grain). Each animal is 3 food; kept as emergency food for the people when the four food goods do not cover their meal ([[food]]). |
| **Timber, stone** | The material of nearly every [[buildings|building]]. Timber comes from olive groves, maquis, hills and river valleys in your [[catchment]]. Early on, timber is almost always your bottleneck. |
| **Cedar** | Found only where there is a deposit, but a cedar forest yields far more wood than an olive grove. Levels your best buildings; builds war galleys. **Cedar stands in for timber:** whatever a cost needs in timber and you lack, your cedar covers one for one. |
| **Copper, tin** | The two halves of bronze — never found together ([[bronze]]). |
| **Bronze** | Elite infantry, war chariots, war galleys, the best walls. |
| **Silver** | The currency, and the only thing that pays soldiers ([[silver]]). |
| **Incense** | Rare; found only on marked dry hexes and needing no building. An offering of it pleases the gods more than any other gift ([[incense]]). |

## The four tabs

- **Goods** — stock, production and use of each good, city by city. The settlements table compares people, food produced minus food eaten, food days and shortage status; select a city to close Economy and open its City drawer. Goods piling up with nothing using them are marked — see [[surplus]].
- **Transfer** — move goods between your own cities ([[transfers]]).
- **Automation** — [[routes|standing orders]] that move goods for you while you are away.
- **Wants** — shortages and surpluses in every city you have [[contact|contacted]]: where to [[trade]].

## Offerings and trade

The gods weigh goods differently: bronze can count for far more than grain. **Kult**, at the temple, and `keryx rite --list` show their current reckoning before each god's preferences. What the world can spare and what the god favours both matter ([[rites]]). Wanaxes agree their own trade prices.

Production per day is output before meals and other uses. Food net combines grain, fish, oil and wine production minus population demand. The city has one stock of each good; food coverage counts these four goods together. Shortfall waiting estimates assume current production continues, before other use.

Food for people combines the four food goods minus the population’s meal. **Food after army** subtracts the food supplied by this city to its supported troops as well. The army shares grain, fish, oil and wine in the same proportions; its silver pay is shown separately. Population growth still uses the population-only food balance.


## Storage capacity

Each good has its own storage capacity in each city. A city stores **2 000 of each food** — grain, fish, oil and wine — and a [[buildings|Granary]] raises that to 4 000, 10 000 and 20 000. The herd has its own pens. A city that already held more food when this ceiling came in keeps it and can eat it, but it never fills back above the ceiling. **StorageCapacityWarning** is an advance dispatch when the server forecasts a stock nearing its cap. Keryx status and the City drawer show the good, stored amount, capacity and **full in N days**. The estimate uses the current net rate and deliveries on the way; it is a forecast, so changing production or orders can change it.

The forecast counts your own transports and accepted trade deliveries that you know about. An unsolicited gift from another Wanax is known only when it arrives, so it is not included beforehand.

Send goods away by [[transfers|transfer]] or use them in [[trade]]. Warehouse construction and storage-pool expansion are not available yet.

A **StorageOverflow** notification means goods have already been lost because storage was full. It records the city, good, quantity lost and the source, such as production, a delivery, returned provisions or a refund. It does not mean the entire shipment was lost: only the reported excess failed to fit. Production into a store that stays full is lost every day; you are told when it starts, not every day it goes on. If the forecast clears and the city later approaches capacity again, a new warning can be sent.
