A Shipyard builds ships and repairs damaged hulls in a coastal city. Its water must lead to the Thalassa: on a [[lake]] with no [[rivers|river]] or sea way out, it is refused. It must have a worker to run its jobs, and more workers make them faster; see [[shipyard-workers]] for staffing, pauses and paid work.

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

The pace is the number of workers: **1×, 2× or 3×**. A job of 12 game days of work takes 12, 6 or 4 game days with one, two or three workers. The existing limits on simultaneous shipbuilding and repairs still apply.

You can pay for an order while the shipyard is unstaffed. It waits with **“waiting for shipyard workers”** and has no ready time. Removing the last worker pauses every build and repair; completed work and paid materials are kept. Staff the shipyard again to resume the remaining work and see a new ready time.

Work already under way when this rule takes effect keeps its remaining work. Workers are not assigned automatically.

See [[citizens|Citizens and placement]], [[units|Soldiers and ships]] and [[sea|The Thalassa]].
