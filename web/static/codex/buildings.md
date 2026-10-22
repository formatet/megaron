Open **City → Buildings**. The tab shows what is **built** (and its level), the **build queue** (with an ETA and a **✕** to cancel), **training** (units in progress) and **Construct**. Choose from the Construct list — each entry names the building and its cost — and press **+ Build**. A notification tells you when it is done.

| Building | What it is for |
|---|---|
| **Farm** | Stands on a grain hex you choose and works that field. |
| **Harbour** | Opens deep-water fishing places and enables sea trade ([[harbour]]). Stands in the city — see below. |
| **Shipyard** | Builds and repairs ships (needs a coastal city). |
| **Lumbermill** | Stands on olive groves, cedar forest, maquis, hills or river valleys and raises their wood production. |
| **Stone Quarry** | Stands on a land hex you choose and works the stone there — far more on hills and limestone. |
| **Mine** | Stands on an ore deposit hex and extracts whatever it holds — copper, tin or silver. |
| **Foundry** | Smelts copper and tin into bronze ([[bronze]]). The gate to elite soldiers and war galleys. |
| **Barracks** | Enables recruiting infantry and war chariots ([[units]]). |
| **Stable** | Enables war chariots. |
| **Olive Press** | Refines a press-worker's oil from the settlement's groves. Works inside the city. |
| **Winery** | Refines a vintner's wine from the settlement's vines. Works inside the city. |
| **Pen** | Raises the herd cap by 100 per level (50 without one). Animals above the cap are eaten, then lost ([[food]]). Works inside the city. |
| **Market** | Enables trade offers and updates market price snapshots. |
| **Temple** | Enables rites, produces cult, and unlocks oracle prayers. Built for one god; a city can hold one temple per god. More important than it looks — see [[temples]]. |
| **Granary** | A city stores 2 000 of each food — grain, fish, oil and wine, not the herd. Granary level 1 to 3 raises that to 4 000, 10 000 and 20 000. A wall of level N needs a Granary of level N. Works inside the city. |
| **Wall** | Palisade, then Stone Wall, then Bronze Wall. Absorbs damage in [[battle]], and **the walls set how many people the city holds** (see below). |

**The wall is the ceiling.** A city with no wall holds 2 000 people, a Palisade 5 000, a Stone Wall 10 000, a Bronze Wall 20 000. A city at its ceiling loses no one; it simply stops growing until you raise the wall. Each wall needs a Granary of the same level first — Construct shows the next wall greyed out with "needs a Granary of level 2" until you have it. See [[growth]].

**The coast rule.** A city can build a Harbour or a Shipyard only if its own hex borders the sea or a river; an inland city never can, so to reach the sea you must found a coastal colony ([[colonies]]). The [[founding]] forecast tells you which kind of site you are looking at.

## On a hex

**Farm, Mine, Lumbermill and Stone Quarry stand on a hex, not in the city.** When you build one of these four, you choose which hex in your [[catchment]] it goes on, and it only ever helps that one hex. You may build more than one of the same kind — two Farms on two different grain hexes each raise their own hex's cap, independently. A Mine's hex decides what it produces: build it on a copper, tin or silver deposit and it works that deposit; there is no separate building for silver.

**One building per hex.** A hex that already has one of these four — yours, or a neighbouring city's on a hex you share — cannot take another. Building the same kind again on its own hex raises its level.

Every other building, including **Harbour**, stands in the city itself. Harbour is the one exception worth naming: though it stands in the city (and needs the city's own hex to border the sea), its fishing effect reaches every deep-sea hex in your catchment at once. Harbour levels 1, 2 and 3 open 1, 2 and 4 places per deep-water hex; each fisher catches 1 fish per day. It gives no fishing multiplier or extra coastal places. See [[fishing]] and [[harbour]].

## The rule

A farm, mine, lumbermill or stonequarry makes **space for four more workers** on its hex. Each of these buildings multiplies the hex's own **production** per worker: ×1.7 at level 1, ×2.4 at level 2, ×3.1 at level 3. Silver can only be mined with a mine — without one a silver deposit gives nothing. Olive press, winery and foundry work inside the city as before.

## Levels

Buildings level up to three. **A higher level makes every worker on the hex produce more** — see the rule above.

A temple is built for a god: Construct lists one row per god (`keryx build --type temple --god potnia`), and building that god's temple again raises it. Building the same building again raises it by one level; for a hex building, choose the same hex. **Levels 2 and 3 of a Harbour, Shipyard or Temple** also need **cedar**, which is scarce and often only available by [[trade]].

A building costs what it is made of: **timber** for the Farm, Market, Stable, Harbour, Shipyard and Palisade; **stone** for the Stone Quarry, Mine, Barracks, Temple and Stone Wall. The prices are shown live in Construct. If you are short of timber, your **cedar** pays the difference one for one — in buildings, ships and chariots alike.

A building produces nothing without people in it. Staff it from the centre **City** hex of the placement grid ([[citizens]]).


If cancelling a queued build is refused, the reason appears in the Buildings tab.

The construction queue has a fixed limit, shown in `keryx build --queue`; population and building upgrades do not add slots. Finish or cancel a queued building before starting another when it is full. Hex buildings use `place`, not `staff`: for a lumbermill use `keryx place timber <hex> +2` (cedar on a cedar hex). `build --hex` accepts coordinates such as `3,-1` or the City ordinal `6`/`#6`.

## Demolishing and rebuilding

A finished building can be torn down so its hex or slot is free for something else. In the Buildings tab press **Demolish** beside the building and confirm; with keryx, `keryx demolish --hex 3,-1` for a hex building or `keryx demolish --type market` for a city one. Only your own city's buildings can be demolished.

The place is free at once — the new building then takes its normal time to build ([[catchment]] has the one-building-per-hex rule). **Nothing is refunded**, and anyone working in the building or on the places it made is lifted off and goes idle; they are not lost, and you can place them again ([[citizens]]). A hex keeps its base places, so a demolished farm's field still holds its first workers. A silver deposit gives nothing without its mine, so everyone mining it goes idle.

Something still under construction is not demolished but cancelled, with its cost refunded: the ✕ in the Build queue, or `keryx cancel-build`.

`keryx build --list [city]` and Construct show the current level and the cost of the next level. Without a city name, Keryx uses your capital. Each hex building has its own level and upgrade price. A refusal names the building and the level it would reach.
