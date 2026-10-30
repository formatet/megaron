A **[[temple]]** ([[buildings]]) is how you keep [[gods|the gods]]' favour ([[kharis]]). Every temple is dedicated to **one god**: [[potnia]], [[velchanos]], [[asterion]] or the [[sea-daimons|Sea Daimons]] of Stillness. A city may hold several temples, but **at most one per god**. They are named by their god: Temple of Potnia, Temple of Velchanos, and so on. Each temple needs two things from you.

## Building and upgrading temples {#temples-building-one}

Choose the god when you build. On the web, **City → Construct** lists one row per god; in Keryx, `keryx build --type temple --god potnia`. Building the same god's temple again raises that temple's level. Each prayer is said at its own god's temple ([[rites]]): Potnia's two prayers at Potnia's, [[green-shoots]] at Velchanos's, [[the-thread-shows-a-path]] at Asterion's, [[favourable-passage]] at the Sea Daimons'. Every prayer works from a level-1 temple. Demolishing a temple frees only its own workers.

## Temple workers {#temple-workers}

**City → [[catchment]] & workplaces → Temple**: place or remove whole [[citizens]] with the same controls as other workplaces. In Keryx, use `staff temple +1` or `staff temple -1`; `roster` includes them. When a city has several temples, name the god: `keryx staff temple +1 --god velchanos`. The web shows one card per temple.

Temple levels 1/2/3 provide **2/4/8 workplaces**. A worker stands at one temple only and cannot work another job at the same time. Founding grants no automatic temple staffing.

Temple levels 1/2/3 have **weight 1/4/9**. Effective weight is temple weight × staffed fraction × paid tithe fraction. Each effective weight produces **2.1 kharis per game day**, multiplied by the existing scarcity factor of 1–2. An empty temple gives nothing.

## A temple’s daily offering {#daily-offerings}

Each temple pays its daily tithe from its own city. Temples in the same city draw in god order (Asterion, Potnia, Stillness, Velchanos), each against what the earlier temples left. Kharis remains one pool for the realm.

| Level | [[grain]] part | [[silver]] part | [[incense]] part | Effect shares |
|---|---|---|---|---|
| 1 | 0.25 grain | — | — | 100% |
| 2 | 0.75 grain | 0.1 silver | — | 75% / 25% |
| 3 | 1.75 grain | 0.25 silver | 0.1 incense | 70% / 20% / 10% |

The grain part uses **grain first, then [[fish|fish]], then [[timber|timber]]**, at equal divine value. Silver never replaces grain. Each part is paid separately: a part that cannot be paid in full takes nothing. No part replaces another. A level-2 temple without silver still receives 75% effect if its grain part is paid; a level-3 temple with only its grain part receives 70%. [[rites|Prayers]] keep their own offerings.

### Planning temple offerings {#daily-offerings-2}

**Kult → Temples** and `keryx status` show the next tithe, each paid or missing part, staffing, weight and effective contribution. These are forecasts against current local stocks; the next service charges what it can actually pay.

## Temple levels and the Kharis ceiling {#temple-levels}

With full staffing and tithe, a level-1 temple raises kharis in a one-city realm by **0.9 per game day** before scarcity. Compare your realm demand and given weight in **Kult → Kharis**. A temple does not remove its city from the demand, nor protect low standing from [[divine-wrath|divine wrath]].

**Your best temple sets your kharis ceiling:** no temple 40, level 1 50, level 2 75, level 3 100. Everyday prayers work at a level-1 temple. Spontaneous blessings need 60 kharis and a maintained temple. Only a level-2 temple or higher lets kharis grow that far. Kharis retained above a lowered ceiling still counts. Levelling a temple further needs [[cedar|cedar]].

### Temple demolition and realm temple lists {#temple-levels-2}

Demolishing a temple voluntarily costs **10 kharis per level**, with a floor of **1**; plundering causes no such debt. The demolition loss is immediate. A lower ceiling takes no additional kharis; above it, daily cult [[upkeep|upkeep]] lowers kharis and temple workers and offerings do not raise it. The confirmation shows the loss and any ceiling change. See [[temple-demolition]].

## Finding your temples {#temples-find-your-temples}

**Kult → Temples** lists every standing temple in your realm, with its city, god,
level, workers and next daily offering. **Manage in City** opens that temple's
city for staffing or building. Demolition asks for confirmation and shows the
current kharis loss and new ceiling. Temples you have not built are listed last.

**Kult → Prayers** lists prayers across your cities, with the temple and city
where each can be offered. Prayers you can prepare now come first; prayers
whose temples are missing come last. Each Offer uses that row's city and stock.
Gods' waiting times still apply across your realm. **Kult → Kharis** explains your
realm's standing, daily change and current divine risk; see [[kharis]].

`keryx status` lists all your temples. `keryx rite --list` lists prayers across
your realm with the `--settlement` to use for each; add `--settlement <id>` to
filter the list to one city.

## Daily upkeep of the cult {#cult-upkeep}

Realm demand is **the number of active cities squared**. All active cities count, including the capital, with or without temples. Sunk, collapsed and razed cities do not count. Population, armies and crews do not change demand. Cult upkeep is **1.2 × demand per game day**.

One, two, three and four cities demand **1, 4, 9 and 16** respectively. Four fully staffed level-1 temples in a four-city realm give 4 weight: **8.4 income − 19.2 upkeep = −10.8 kharis per game day**, before scarcity. A realm with no active city has zero demand.

### Temple production and punishment risk {#cult-upkeep-2}

**Kult → Kharis** and `keryx status` show: “The gods ask 16 of your realm; your temples give 7.” Staffing and paid tithe both multiply temple weight. Improve either to raise its contribution; raising the temple level increases weight and tithe requirements.

Below **15 Kharis**, there is a 3% chance of punishment each game day **with or without a temple**. An actual punishment gives **seven game days of protection**. A game day follows this world's pace; Kult and `keryx status` show the protection's end in your local clock. Nothing taken means no new protection. See [[divine-wrath]]. A punishment chooses a random eligible city of yours with something it can take; it need not strike the capital.

## Demolishing a temple {#temple-demolition}

Voluntarily demolishing a [[temples|temple]] angers its god. Your realm loses **10 kharis per temple level**, down to a **floor of 1**. This is an immediate consequence of your own demolition, separate from the daily chance of [[divine-wrath|divine punishment]]. It does not grant punishment protection.

Before you confirm, **City → Demolish** and `keryx demolish --type temple --god potnia` show the actual kharis loss, standing afterward and the ceiling before and after. Keryx asks for confirmation; non-interactive or `--json` use without `--yes` previews the consequences without demolishing. Run again with `--yes` to confirm. The result and the **god is angered** notification record what actually happened. Nothing is refunded, and only that temple's workers become idle.

Your best remaining temple determines the ceiling: no temple **40**, level 1 **50**, level 2 **75**, level 3 **100**. Another equally high temple can keep the ceiling unchanged. **Demolition costs kharis immediately; the lower ceiling takes no additional kharis.** Standing above it declines by ordinary daily cult upkeep; temple gains do not offset this decline. Once standing is at or below the ceiling, ordinary gain and upkeep apply again. The voluntary demolition loss still applies.

A temple destroyed by **plundering** creates **no demolition debt** for its owner. Its loss can lower the ceiling and remove its production. Cancelling construction is not demolition and does not incur the kharis loss.
