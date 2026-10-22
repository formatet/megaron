A **Temple** ([[buildings]]) is how you keep the gods' favour ([[kharis]]). Every temple is dedicated to **one god**: Potnia, Velchanos, Asterion or the Sea Daimons of Stillness. A city may hold several temples, but **at most one per god**. They are named by their god: Temple of Potnia, Temple of Velchanos, and so on. Each temple needs two things from you.

## Building one

Choose the god when you build. On the web, **City → Construct** lists one row per god; in Keryx, `keryx build --type temple --god potnia`. Building the same god's temple again raises that temple's level. Each prayer is said at its own god's temple ([[rites]]): Potnia's two prayers at Potnia's, Green Shoots at Velchanos's, The Thread Shows a Path at Asterion's, Favourable Passage at the Sea Daimons'. Every prayer works from a level-1 temple. Demolishing a temple frees only its own workers.

## Devotion — people

**City → Catchment & workplaces → Temple**: place or remove whole [[citizens]] with the same controls as other workplaces. In Keryx, use `staff temple +1` or `staff temple -1`; `roster` includes them. When a city has several temples, name the god: `keryx staff temple +1 --god velchanos`. The web shows one card per temple.

Each temple level provides **2 workplaces**: level 1 holds 2, level 2 holds 4 and level 3 holds 6. A worker stands at one temple only. These people leave the idle pool, and cannot work another job at the same time. An empty temple has no devotion gain. Founding a city grants no automatic temple staffing.

Each placed temple worker earns **1.05 kharis per day** before the offering factor, scarcity and realm decay. A fully staffed level-1 temple earns **2.1** before these adjustments. Kult shows local staffing and the projected realm net. <!-- src: server/internal/kharis/tick.go KharisPerTempleWorker -->

## Offerings — local early goods

Each temple has its own daily reference gift of **0.25 grain**, taken only from its own city's stock. When a city holds several temples, they are planned in god-ID order — Asterion, Potnia, the Sea Daimons of Stillness, Velchanos — each against what the earlier ones left in the store. A temple's workers earn kharis only on days that temple was fed. Kharis stays one pool for the whole realm. Fish, timber and silver can replace grain at the same divine value; a gift may combine goods. The amount of a substitute can change with the world's divine values. <!-- src: server/internal/kharis/temple_offering.go TempleOfferingGrain -->

The temple uses **grain first, then fish, then timber, and silver last**. Silver is never required. Oil and wine are not used for this standing gift; [[rites|prayers]] keep their own offerings.

**Kult → Temple offerings** and `keryx status` show one row per temple with the reference, equivalent alternatives, the stock it would find after earlier temples, and the planned next gift, with a ✓ or ✗. If the city cannot fund the whole gift, nothing is taken and that temple earns no offering gain for the day. Devotion and the daily decay still determine your net kharis change.

## Level

With full devotion and its daily gift, a level-1 temple can raise kharis in a small realm. Compare its gain with the cost of all your active cities in **Kult → Daily decay** and the projected net. A temple does not cancel city costs, but while one stands the gods do not punish you. Higher-level temples can employ more devotion and make it grow faster when staffed and fed.

**Your best temple sets your kharis ceiling:** no temple 40, level 1 50, level 2 75, level 3 100. A level-1 temple can never bring a blessing; a higher-level temple raises the ceiling. Levelling a temple further needs cedar.
