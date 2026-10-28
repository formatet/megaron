**City → Catchment & workplaces → Temple**: place or remove whole [[citizens]] with the same controls as other workplaces. In Keryx, use `staff temple +1` or `staff temple -1`; `roster` includes them. When a city has several temples, name the god: `keryx staff temple +1 --god velchanos`. The web shows one card per temple.

Each temple level provides **2 workplaces**: level 1 holds 2, level 2 holds 4 and level 3 holds 6. A worker stands at one temple only. These people leave the idle pool, and cannot work another job at the same time. An empty temple has no devotion gain. Founding a city grants no automatic temple staffing.

Each placed temple worker earns **1.05 kharis per game day** before the offering factor, scarcity and realm decay. A fully staffed level-1 temple earns **2.1** before these adjustments. Kult shows local staffing and the projected realm net. <!-- src: server/internal/kharis/tick.go KharisPerTempleWorker -->
