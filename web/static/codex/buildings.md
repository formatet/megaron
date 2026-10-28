Buildings provide workplaces, defences and the facilities needed for recruitment and trade.

Open **City → Buildings**. The tab shows what is **built** (and its level), the **build queue** (with an ETA and a **✕** to cancel), **training** (forces in progress) and **Construct**. Choose from the Construct list — each entry names the building and its cost — and press **+ Build**. A notification tells you when it is done.

## All buildings {#buildings-details}

<!-- list: building -->
| Name | In short |
|---|---|
| [[shipyard]] | A Shipyard builds ships and repairs damaged hulls in a coastal city. |
| [[harbour]] | A Harbour opens deep-water fishing places in a coastal city’s catchment. |
| [[farm]] | A Farm expands the grain workplaces and production of one chosen catchment hex. |
| [[barracks]] | Barracks provide facilities for recruiting land forces. |
| [[mine]] | A Mine works the mineral deposit on one chosen catchment hex. |
| [[lumbermill]] | A Lumbermill improves wood production on one chosen catchment hex. |
| [[stone-quarry]] | A Stone Quarry improves stone production on one chosen land hex. |
| [[market]] | A Market enables trade offers and updates market prices without employing workers. |
| [[walls]] | Walls defend the city and determine its population ceiling. |
| [[foundry]] | A Foundry turns copper and tin into bronze. |
| [[stable]] | A Stable enables chariot recruitment. |
| [[temple]] | A Temple is a workplace dedicated to one god, opening that god’s prayers and maintaining kharis. |
| [[olive-press]] | An Olive Press refines oil from the city’s groves. |
| [[winery]] | A Winery refines wine from the city’s vines, which grow only on hills — without a hill in the city's reach it makes nothing. |
| [[pen]] | A Pen increases the herd a city can hold. |
| [[granary]] | A Granary expands food storage and is required by wall upgrades. |
<!-- /list -->

### Walls, population and coastal access {#buildings-details-3}

**The wall is the ceiling.** A city with no wall holds 2 000 people, a Palisade 5 000, a [[walls|Stone Wall]] 10 000, a [[bronze|Bronze]] Wall 20 000. A city at its ceiling loses no one; it simply stops growing until you raise the wall. Each wall needs a [[granary]] of the same level first — Construct shows the next wall greyed out with "needs a Granary of level 2" until you have it. See [[growth]].

**The coast rule.** A city can build a Harbour or a Shipyard only if its own hex borders the sea or a [[rivers|river]]; an inland city never can, so to reach the sea you must found a coastal colony ([[colonies]]). The [[founding]] forecast tells you which kind of site you are looking at.

## Buildings on catchment hexes {#hex-buildings}

**[[farm]], [[mine]], [[lumbermill]] and [[stone-quarry]] stand on a hex, not in the city.** When you build one of these four, you choose which hex in your [[catchment]] it goes on, and it only ever helps that one hex. You may build more than one of the same kind — two Farms on two different [[grain|grain]] hexes each raise their own hex's cap, independently. A Mine's hex decides what it produces: build it on a [[copper|copper]], [[tin|tin]] or [[silver|silver]] deposit and it works that deposit; there is no separate building for silver.

**One building per hex.** A hex that already has one of these four — yours, or a neighbouring city's on a hex you share — cannot take another. Building the same kind again on its own hex raises its level.

### City buildings and Harbour reach {#hex-buildings-2}

Every other building, including **Harbour**, stands in the city itself. Harbour is the one exception worth naming: though it stands in the city (and needs the city's own hex to border the sea), its fishing effect reaches every deep-sea hex in your catchment at once. Harbour levels 1, 2 and 3 open 1, 2 and 4 places per deep-water hex; each fisher catches 1 [[fish|fish]] per game day. It gives no fishing multiplier or extra coastal places. See [[fishing]] and [[harbour]].

## Hex building production {#hex-production-buildings}

A farm, mine, lumbermill or stonequarry makes **space for four more workers** on its hex. Each of these buildings multiplies the hex's own **production** per worker: ×1.7 at level 1, ×2.4 at level 2, ×3.1 at level 3. Silver can only be mined with a mine — without one a silver deposit gives nothing. Olive press, winery and foundry work inside the city as before.

## Building upgrades and materials {#building-levels}

Buildings level up to three. **A higher level makes every worker on the hex produce more** — see the rule above.

A temple is built for a god: Construct lists one row per god (`keryx build --type temple --god potnia`), and building that god's temple again raises it. Building the same building again raises it by one level; for a hex building, choose the same hex. **Levels 2 and 3 of a Harbour, Shipyard or [[temple]]** also need **[[cedar|cedar]]**, which is scarce and often only available by [[trade]].

A building costs what it is made of: **[[timber|timber]]** for the Farm, [[market]], [[stable]], Harbour, Shipyard and Palisade; **[[stone|stone]]** for the Stone Quarry, Mine, [[barracks]], Temple and Stone Wall. The prices are shown live in Construct. If you are short of timber, your **cedar** pays the difference one for one — in buildings, ships and [[war-chariot|chariots]] alike.

Production workplaces need people assigned to them. The market has no workplaces and needs no workers; former market workers are free to assign elsewhere, with no population lost. Staff a workplace from the centre **City** hex of the placement grid ([[citizens]]).

### Construction queues and hex selectors {#building-levels-2}

If cancelling a queued build is refused, the reason appears in the Buildings tab.

The construction queue has a fixed limit, shown in `keryx build --queue`; population and building upgrades do not add slots. Finish or cancel a queued building before starting another when it is full. Hex buildings use `place`, not `staff`: for a lumbermill use `keryx place timber <hex> +2` (cedar on a cedar hex). `build --hex` accepts coordinates such as `3,-1` or the City ordinal `6`/`#6`.

## Demolishing a finished building {#demolition}

A finished building can be torn down so its hex or slot is free for something else. In the Buildings tab press **Demolish** beside the building and confirm; with keryx, `keryx demolish --hex 3,-1` for a hex building or `keryx demolish --type market` for a city one. Only your own city's buildings can be demolished.

The place is free at once — the new building then takes its normal time to build ([[catchment]] has the one-building-per-hex rule). **Nothing is refunded**, and anyone working in the building or on the places it made is lifted off and goes idle; they are not lost, and you can place them again ([[citizens]]). A hex keeps its base places, so a demolished farm's field still holds its first workers. A silver deposit gives nothing without its mine, so everyone mining it goes idle.

A temple also costs **10 [[kharis|kharis]] per level**, down to 1. The confirmation shows the actual loss and any lowered ceiling; existing standing is not cut to the new ceiling. See [[temple-demolition]].

### Cancelling builds and finding completed sites {#demolition-2}

Something still under construction is not demolished but cancelled, with its cost refunded: the ✕ in the Build queue, or `keryx cancel-build`.

`keryx build --list [city]` and Construct show the current level and the cost of the next level. Without a city name, Keryx uses your capital. Each hex building has its own level and upgrade price. A refusal names the building and the level it would reach.

Construction dispatches name the building, its city and the exact hex for a field building. **Staff Lumbermill** (or the named workplace) opens that city's Production grid with the completed site selected, so you can place a worker there. Buildings without a workplace offer **Show city** instead. The build queue repeats the city and field hex beside its completion time; Keryx shows the same place with `build --queue`.
