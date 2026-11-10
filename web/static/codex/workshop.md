A Workshop is where a city's builders work. Its builders make construction faster; without it, or without builders, the city still builds at its base pace.

Build or upgrade it in **City → Buildings → Construct**, or use `keryx build --type workshop`.

<!-- facts: building workshop -->
| Reference | Current rule |
|---|---|
| Cost, level 1 | 12 stone · 10 timber (or cedar) |
| Build time, level 1 | 8 game days |
| Maximum level | 3 |
| Cost to level 2 | 24 stone · 20 timber (or cedar) |
| Cost to level 3 | 48 stone · 40 timber (or cedar) |
<!-- /facts -->

## Builders and construction pace {#workshop-builders}

A city builds **one building at a time**. The others wait in the queue, up to two behind the active one (three in all). When the active build is done, or cancelled, the next one starts. Only the active build has a ready time; a waiting build shows **next in line** until its turn comes.

The active build's pace is the number of **builders** placed in the Workshop: **1×, 2× or 3×**, one place per Workshop level. A city with no Workshop, or a Workshop with nobody in it, builds at the base pace 1×, so the first Workshop can be built at all. A build of 12 game days takes 12, 6 or 4 game days with one, two or three builders.

Place builders through the City's workplace controls, or use `keryx staff workshop +1 --city <city>`. They come from the same workforce as farmers and temple workers, and are never placed automatically. Adding or removing a builder re-times the active build from the work that is left: nothing is lost or counted twice, and the new ready time appears at once.

Cancelling a build refunds its cost as before. Cancelling the active one starts the next at once; cancelling a waiting one changes nothing else.

See [[buildings]], [[citizens|Citizens and placement]] and [[shipyard]], whose workers set the pace of ships the same way.
