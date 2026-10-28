[[fish]] feeds your people just like [[grain|grain]] ([[food]]). Place [[citizens]] on a fishing hex in **City**, or use `keryx place fish <hex> +1`. The available places and the next citizen’s output come from that city’s current production options.

| Water | Fishing places | Fish per fisher per game day |
|---|---|---|
| Coastal water | 2 | 1 |
| Bay | 2 | 2 |
| Deep water | 0, 1, 2 or 4 at harbour levels 0, 1, 2 or 3 | 1 |

## Fishing places, bays and storms {#fishing-details}

A **bay** is [[coastal-sea|coastal water]] with land on at least four of its six sides. Rivers and [[river-ford|fords]] count as land for this rule; coastal water, deep water and the Thalassa do not. A missing hex beyond the map edge does not count as land. A bay doubles each fisher’s catch; it has the same two places as other coastal water.

Deep-water fishing requires the city’s [[harbour]]. The harbour supplies deep-water places without multiplying the catch. It adds no places or production to coastal water or bays. Rivers and fords keep their own fishing places and production.

A harbour creates places, not workers. Put citizens on the deep-water hexes to use them. Higher harbour levels open more places there, while each fisher still catches 1 fish per game day.

**A storm stops the fishing.** While a [[storms|storm]] stands on a fishing hex, the fishers stay placed there but catch nothing. They fish again the game day it has moved on. **City** shows *Storm over N fishing grounds*, and `keryx city` and `keryx status` print the same line: *its fishers stay but catch nothing while it stands*.

## Deep-water fishing {#deep-water-fishing}

A harbour stands in the city and opens fishing places on **every deep-water hex in its [[catchment]]**. Build it through **City → Buildings → Construct**, or `keryx build --type harbour`. The city must meet the coast rule described in [[buildings]].

| Harbour level | Fishing places on each deep-water hex |
|---|---|
| No harbour | 0 |
| Level 1 | 1 |
| Level 2 | 2 |
| Level 3 | 4 |

Each deep-water fisher catches **1 fish per game day**, at every harbour level. The harbour has no fishing multiplier and adds no coastal places. Coastal water always has two places producing 1 fish per fisher per game day; a bay has two places producing 2 each. See [[fishing]] for the bay rule. Rivers and fords receive no harbour bonus.

The harbour has no city workplaces to staff. Place your [[citizens]] on the deep-water hexes themselves. Its role in sea trade is separate from fishing. A [[shipyard|Shipyard]] is a separate building for building and repairing [[sea|ships]].
