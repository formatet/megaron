Open **City → Buildings**. The tab shows what is **built** (and its level), the **build queue** (with an ETA and a **✕** to cancel), **training** (units in progress) and **Construct**. Choose from the Construct list — each entry names the building, its cost and what it does — and press **+ Build**. A notification tells you when it is done.

| Building | What it is for |
|---|---|
| **Farm** | Stands on a grain hex you choose and works that field. |
| **Harbour** | Works the settlement's coastal waters and enables sea trade ([[sea]]). Stands in the city — see below. |
| **Shipyard** | Builds and repairs ships (needs a coastal city — an adjacent sea hex). |
| **Lumbermill** | Stands on a forest hex and works its stands. |
| **Stone Quarry** | Stands on a hex; its crew cut stone in the quarry itself, whatever the hex. |
| **Mine** | Stands on an ore deposit hex and extracts whatever it holds — copper, tin or silver. |
| **Foundry** | Smelts copper and tin into bronze ([[bronze]]). The gate to elite soldiers and war galleys. |
| **Barracks** | Enables recruiting infantry and war chariots ([[units]]). |
| **Stable** | Enables war chariots. |
| **Olive Press** | Refines a press-worker's oil from the settlement's groves. |
| **Winery** | Refines a vintner's wine from the settlement's vines. |
| **Market** | Enables trade offers and updates market price snapshots. |
| **Temple** | Enables rites, produces cult, and unlocks oracle prayers. More important than it looks — see [[temples]]. |
| **Wall** | Palisade, then stone, then bronze. Absorbs damage in [[battle]]. |

## On a hex

**Farm, Mine, Lumbermill and Stone Quarry stand on a hex, not in the city.** When you build one of these four, you choose which hex in your [[catchment]] it goes on, and it only ever helps that one hex. You may build more than one of the same kind — two Farms on two different grain hexes each raise their own hex's cap, independently. A Mine's hex decides what it produces: build it on a copper, tin or silver deposit and it works that deposit; there is no separate building for silver.

Every other building, including **Harbour**, stands in the city itself. Harbour is the one exception worth naming: though it stands in the city (and needs the city's own hex to border the sea), its effect reaches every coastal-sea hex in your catchment at once, not just one.

## Levels

Buildings level up to three. **A higher level makes the same crew produce more** — it raises the output per worker, not how many workers the building employs. Farms are the deliberate exception: their level raises how many people a grain hex can hold.

Levelling a Harbour, Shipyard or Temple further also needs **cedar**, which is scarce and often only available by [[trade]].

A building produces nothing without people in it. Staff it from the centre **City** hex of the placement grid ([[citizens]]).

## The real numbers

The table above only says what a building is *for*. For what it actually *produces*, look at the
Construct list when picking a type, or at the Built list for a building you already own — both show
the real, live numbers, not a rounded-off promise.

Each line reads as **workers × output per worker, per tick**, e.g. `grain on plains: 4 × 1.00/tick →
L1 8 × 2.70 · L2 10 × 2.70 · L3 12 × 2.70`. The first pair (`4 × 1.00`) is what the hex gives with
**no building** at all (`none` if the hex gives nothing of it without one); the arrow (`→`) leads to what each level of the building adds instead. A level
raises the **output per worker**, not how many workers there are — the same crew simply produces more
at L2 than at L1. **Grain is the one exception**: there, a level adds *more workers* the hex can hold,
at the same rate each. If a number looks bad — falling, not rising — that's the real number, shown
as it is.
