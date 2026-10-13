A **Temple** ([[buildings]]) is how you keep the gods' favour ([[kharis]]). It needs two things from you.

## Devotion — people

**City → Production → More → Devotion**: type a percentage of the city and press **Assign →**. Devotion is *additive*: it does not take away from the [[citizens]] you have placed on hexes.

A temple can only employ so much of the city — **15% per temple level** — and the row tells you when you are at the limit. <!-- src: server/internal/kharis/tick.go TempleDevotionPerLevel -->

## Offerings — local early goods

Each temple's daily reference gift is **0.25 grain**, taken only from its own city's stock. Fish, timber and silver can replace grain at the same divine value; a gift may combine goods. The amount of a substitute can change with the world's divine values. <!-- src: server/internal/kharis/temple_offering.go TempleOfferingGrain -->

The temple uses **grain first, then fish, then timber, and silver last**. Silver is never required. Oil and wine are not used for this standing gift; [[rites|prayers]] keep their own offerings.

**Kult → Temple offerings** and `keryx status` show the reference, equivalent alternatives, local stock and planned next gift, with a ✓ or ✗. If the city cannot fund the whole gift, nothing is taken and that temple earns no offering gain for the day. Devotion and the daily decay still determine your net kharis change.

## Level

With full devotion and its daily gift, your first level-1 temple lets kharis grow slowly toward early [[rites|prayers]]. Higher-level temples can employ more devotion and make it grow faster when staffed and fed.

**Your best temple sets your kharis ceiling.** A level-1 temple can never bring a blessing; a higher-level temple raises the ceiling. Levelling a temple further needs cedar.
