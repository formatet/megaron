A Shipyard builds ships and repairs damaged hulls in a coastal city. Its water must lead to the Thalassa: on a [[lake]] with no [[rivers|river]] or sea way out, it is refused. It must have a worker to run its jobs, more workers make them faster, and the yard's workers are shared between the ships it works on; see [[shipyard-workers]] for staffing, pauses and paid work.

Build it through **City → Buildings → Construct**, or `keryx build --type shipyard`. Recruit ships in **War → Recruit**. The [[harbour]] is a separate building.

<!-- facts: building shipyard -->
| Reference | Current rule |
|---|---|
| Cost, level 1 | 12 stone · 24 timber (or cedar) |
| Build time, level 1 | 12 game days |
| Maximum level | 3 |
| Cost to level 2 | 4 cedar · 24 stone · 48 timber (or cedar) |
| Cost to level 3 | 8 cedar · 48 stone · 96 timber (or cedar) |
| Site | The city must border navigable water. |
<!-- /facts -->

## Shipyard workers {#shipyard-workers}

**An unstaffed shipyard stands still. Each worker speeds the work up, to three.**

Place a citizen in the shipyard through the City's workplace controls, or use `keryx staff shipyard +1 --province <city>`. The worker comes from the same workforce as farmers and temple workers. A shipyard has **one place per level**: 1, 2 or 3 workers.

The yard's pace is its number of workers, **1×, 2× or 3×**, and **it is shared equally by every ship the yard is working on**, building or repairing. A ship alone gets the whole pace; two ships get half each; three ships a third each. Example: a job of 12 game days of work takes 12 game days for one ship with one worker; with three workers and three ships, each ship gets one worker's worth and a 12-day job takes 12 game days; with three workers and two ships it takes 8; with three workers and one ship, 4. When a ship is finished (or a new one starts), the others' pace and ready times update at once, and the work already done is kept.

**Repairs at the same time** are limited by the yard's workplaces: **1, 2 or 3 ships by level**. Repairs already running when you have more than the limit carry on; only new repairs are refused. At most 10 ships can wait in one city's build queue.

You can pay for an order while the shipyard is unstaffed. It waits with **“waiting for shipyard workers”** and has no ready time. Removing the last worker pauses every build and repair; completed work and paid materials are kept. Staff the shipyard again to resume the remaining work and see a new ready time.

Work already under way when this rule takes effect keeps its remaining work. Workers are not assigned automatically.

See [[citizens|Citizens and placement]], [[units|Soldiers and ships]] and [[sea|The Thalassa]].
