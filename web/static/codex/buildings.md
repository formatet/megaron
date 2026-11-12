Buildings provide workplaces, defences and the facilities needed for recruitment and trade.

- One building is built at a time; two more wait in the queue.
- Farm, Lumbermill, Mine and Stone Quarry stand on one catchment hex, one per hex, with 4 workplaces each.
- Levels go to 3; each raises every worker's rate on that hex by 70 %.
- The wall sets the population ceiling: 20 citizens, then 50, 100, 200 (a citizen is 100 people). <!-- src: server/internal/economy/population_cap.go populationCaps; hex_rules.go -->
- Demolishing refunds nothing.

## All buildings {#buildings-details}

<!-- list: building -->
| Name | In short |
|---|---|
| [[barracks]] | Barracks provide facilities for recruiting land forces. |
| [[farm]] | A Farm expands the grain workplaces and production of one chosen catchment hex. |
| [[foundry]] | A Foundry turns copper and tin into bronze. |
| [[granary]] | A Granary expands food storage and is required by wall upgrades. |
| [[harbour]] | A Harbour opens deep-water fishing places in a coastal city’s catchment. |
| [[lumbermill]] | A Lumbermill improves wood production on one chosen catchment hex. |
| [[market]] | A Market gives your city a market board: one line saying what the city buys and one saying what it sells, up to three goods each. |
| [[mine]] | A Mine works the mineral deposit on one chosen catchment hex. |
| [[olive-press]] | An Olive Press refines oil from the city’s groves. |
| [[pen]] | A Pen increases the herd a city can hold. |
| [[shipyard]] | A Shipyard builds ships and repairs damaged hulls in a coastal city. |
| [[stable]] | A Stable enables chariot recruitment. |
| [[stone-quarry]] | A Stone Quarry improves stone production on one chosen land hex. |
| [[storehouse]] | A Storehouse holds everything a city keeps except food, the herd and cult. |
| [[temple]] | A Temple is a workplace dedicated to one god, opening that god’s prayers and maintaining kharis. |
| [[walls]] | Walls defend the city and determine its population ceiling. |
| [[winery]] | A Winery refines wine from the city’s vines, which grow only on hills — without a hill in the city's reach it makes nothing. |
| [[workshop]] | A Workshop is where a city's builders work. |
<!-- /list -->

### Walls, population and coastal access {#buildings-details-3}

**The wall is the ceiling.** No wall: 20 citizens; Palisade 50; [[walls|Stone Wall]] 100; [[bronze|Bronze]] Wall 200. At the ceiling a city stops growing, losing no one. Each wall needs a [[granary]] of the same level first. See [[growth]].

**The coast rule.** A Harbour or Shipyard needs the city's own hex to border the sea or a [[rivers|river]]; an inland city needs a coastal colony ([[colonies]]).

## Buildings on catchment hexes {#hex-buildings}

**[[farm]], [[lumbermill]], [[mine]] and [[stone-quarry]] stand on a hex, not in the city.** You choose which hex in your [[catchment]] gets it, and it helps only that hex. A Mine on a [[copper|copper]], [[silver|silver]] or [[tin|tin]] deposit works that deposit; there is no separate silver building.

**One building per hex**, yours or a neighbour's on a shared hex.

### City buildings and Harbour reach {#hex-buildings-2}

Every other building, including **Harbour**, stands in the city. A Harbour's fishing reaches every deep-sea hex in your catchment; see [[fishing]] and [[harbour]].

## Hex building production {#hex-production-buildings}

Each of the four adds **4 workplaces** to its hex and multiplies each worker's output: ×1.7 at level 1, ×2.4 at level 2, ×3.1 at level 3. A silver deposit gives nothing without its mine.

## Building upgrades and materials {#building-levels}

Buildings level up to three. Building the same thing again raises it one level (for a hex building, pick the same hex). A temple is built for a god: Construct lists one row per god (`keryx build --type temple --god potnia`). **Levels 2 and 3 of a Harbour, Shipyard or [[temple]]** also need **[[cedar|cedar]]**, scarce and often only available by [[trade]].

Construct rows show price, time and **Build**; a hex building's row says **Choose a hex →**, then click a highlighted hex. Short of [[timber|timber]], **cedar** pays the difference one for one. Workplaces need people placed on them ([[citizens]]); the market needs none.

### Construction queues and hex selectors {#building-levels-2}

The queue holds the active build and two waiting (`keryx build --queue`). Builders in a [[workshop]] speed the active build up to 3×. Hex buildings use `place`, not `staff`. `build --hex` accepts `3,-1` or the City ordinal `6`/`#6`.

## Demolishing a finished building {#demolition}

A finished building can be torn down: **Demolish** in the Buildings tab, or `keryx demolish --hex 3,-1` (hex building) / `keryx demolish --type market` (city building).

**Nothing is refunded.** Workers go idle and can be placed again ([[citizens]]).

A temple also costs **10 [[kharis|kharis]] per level**, down to 1. See [[temple-demolition]].

### Cancelling builds and finding completed sites {#demolition-2}

Something still under construction is cancelled, not demolished, and its cost refunded: ✕ in the Build queue, or `keryx cancel-build`.

`keryx build --list [city]` shows the current level and next cost (default: your capital).
