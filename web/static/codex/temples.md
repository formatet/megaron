A **Temple** ([[buildings]]) is how you keep the gods' favour ([[kharis]]). It needs two things from you.

## Devotion — people

**City → Catchment & workplaces → Temple**: place or remove whole [[citizens]] with the same controls as other workplaces. In Keryx, use `staff temple +1` or `staff temple -1`; `roster` includes them.

Each temple level provides **2 workplaces**: level 1 holds 2, level 2 holds 4 and level 3 holds 6. These people leave the idle pool, and cannot work another job at the same time. An empty temple has no devotion gain. Founding a city grants no automatic temple staffing.

Each placed temple worker earns **1.05 kharis per day** before the offering factor, scarcity and realm decay. A fully staffed level-1 temple earns **2.1** before these adjustments. Kult shows local staffing and the projected realm net. <!-- src: server/internal/kharis/tick.go KharisPerTempleWorker -->

## Offerings — local early goods

Each temple's daily reference gift is **0.25 grain**, taken only from its own city's stock. Fish, timber and silver can replace grain at the same divine value; a gift may combine goods. The amount of a substitute can change with the world's divine values. <!-- src: server/internal/kharis/temple_offering.go TempleOfferingGrain -->

The temple uses **grain first, then fish, then timber, and silver last**. Silver is never required. Oil and wine are not used for this standing gift; [[rites|prayers]] keep their own offerings.

**Kult → Temple offerings** and `keryx status` show the reference, equivalent alternatives, local stock and planned next gift, with a ✓ or ✗. If the city cannot fund the whole gift, nothing is taken and that temple earns no offering gain for the day. Devotion and the daily decay still determine your net kharis change.

## Level

With full devotion and its daily gift, a level-1 temple can raise kharis in a small realm. Compare its gain with the cost of all your active cities in **Kult → Daily decay** and the projected net. A temple does not cancel city costs, but while one stands the gods do not punish you. Higher-level temples can employ more devotion and make it grow faster when staffed and fed.

**Your best temple sets your kharis ceiling:** no temple 40, level 1 50, level 2 75, level 3 100. A level-1 temple can never bring a blessing; a higher-level temple raises the ceiling. Levelling a temple further needs cedar.
