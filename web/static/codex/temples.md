A **[[temple]]** ([[buildings]]) is how you keep [[gods|the gods]]' favour ([[kharis]]). Every temple is dedicated to **one god**: [[asterion]], [[potnia]], the [[sea-daimons|Sea Daimons]] of Stillness or [[velchanos]]. A city may hold several temples, but **at most one per god**. They are named by their god: Temple of Potnia, Temple of Velchanos, and so on. Each temple needs two things from you.

## Building and upgrading temples {#temples-building-one}

Choose the god when you build. On the web, **City → Construct** lists one row per god; in Keryx, `keryx build --type temple --god potnia`. Building the same god's temple again raises that temple's level. Each prayer is said at its own god's temple ([[rites]]): [[swift-feet]] at Asterion's, Potnia's two prayers at Potnia's, [[following-seas]] at the Sea Daimons', [[green-shoots]] and [[the-young-gods-arm]] at Velchanos's, [[stout-hulls]] at the Sea Daimons'. Every prayer works from a level-1 temple. Demolishing a temple frees only its own workers.

## Temple workers {#temple-workers}

**City → [[catchment]] & workplaces → Temple**: place or remove whole [[citizens]] with the same controls as other workplaces. In Keryx, use `staff temple +1` or `staff temple -1`; `roster` includes them. When a city has several temples, name the god: `keryx staff temple +1 --god velchanos`. The web shows one card per temple.

Temple levels 1/2/3 provide **2/4/8 workplaces**. A worker stands at one temple only and cannot work another job at the same time. Founding grants no automatic temple staffing. **Temple workplaces unfilled** on **Kult → Kharis** means a temple has empty places: place citizens in it from **City**.

Temple levels 1/2/3 have **weight 1/4/9**. Effective weight is temple weight × staffed fraction × paid tithe fraction. Each effective weight produces the same small amount of kharis each game day (the table below gives the total per level). An empty temple gives nothing.

## A temple’s daily offering {#daily-offerings}

Each temple pays its daily tithe from its own city. Temples in the same city draw in god order (Asterion, Potnia, Stillness, Velchanos), each against what the earlier temples left. Kharis remains one pool for the realm.

The tithe of each level is in the table under Temple levels and the Kharis ceiling, below.

The grain part uses **grain first, then [[fish|fish]], then [[timber|timber]], then [[livestock]]** (this is the order of payment, not a list, so it is not alphabetical), at their divine value (livestock counts as 2). Silver never replaces grain. Each part is paid separately: a part that cannot be paid in full takes nothing. No part replaces another. A level-2 temple without silver still receives 75% effect if its grain part is paid; a level-3 temple with only its grain part receives 70%. [[rites|Prayers]] keep their own offerings.

### The rich offering {#rich-offering}

On top of the tithe, each temple can have a **rich offering**. Turn it on per temple and choose one good from the city's store: **Kult → Temples**, *Add rich offering*, or `keryx rich-offering --god potnia --good fish` (`--off` to stop). Each game day, after every tithe in the city is taken, the temple also receives a small amount of that good: half the tithe's grain value, divided by the good's divine value and by the god's taste for it. Dearer goods, and goods the god prizes, mean a smaller amount: a taste of ×2 halves it. Goods not listed count at ×1.

<!-- tastes -->
| God | Prizes in a rich offering |
|---|---|
| Asterion | [[silver|silver]] ×2, [[stone|stone]] ×1.5 |
| Potnia | [[livestock|livestock]] ×2, [[fish|fish]] ×1.5 |
| The Sea Daimons of Stillness | [[fish|fish]] ×2, [[oil|oil]] ×1.5 |
| Velchanos | [[wine|wine]] ×2, [[grain|grain]] ×1.5, [[livestock|livestock]] ×1.5 |
<!-- /tastes -->

If the rich offering is paid in full, the temple's effective weight is **multiplied by 1.5**. If the good is missing, nothing is taken and there is no partial bonus. It does not raise your kharis ceiling, and it gives nothing while the tithe itself goes unpaid.

### Planning temple offerings {#daily-offerings-2}

**Kult → Temples** and `keryx status` show the next tithe, each paid or missing part, staffing, weight and effective contribution. These are forecasts against current local stocks; the next service charges what it can actually pay.

## Temple levels and the Kharis ceiling {#temple-levels}

With full staffing and tithe, a level-1 temple raises kharis in a one-city realm by **0.25 per game day**. Compare your realm demand and given weight in **Kult → Kharis**. A temple does not remove its city from the demand, nor protect low standing from [[divine-wrath|divine wrath]].

<!-- temple-levels -->
| Level | Workplaces | Weight | Kharis per game day, full staff and tithe | With rich offering | Kharis ceiling | Daily tithe |
|---|---|---|---|---|---|---|
| 1 | 2 | 1 | 0.58 | 0.88 | 50 | 0.25 [[grain|grain]] (100%) |
| 2 | 4 | 4 | 2.33 | 3.50 | 75 | 0.75 [[grain|grain]] (75%) + 0.1 [[silver|silver]] (25%) |
| 3 | 8 | 9 | 5.25 | 7.88 | 100 | 1.75 [[grain|grain]] (70%) + 0.25 [[silver|silver]] (20%) + 0.1 [[incense|incense]] (10%) |
<!-- /temple-levels -->

**Your best temple sets your kharis ceiling:** a realm with no temple has the lowest ceiling, and each temple level raises it (the Kharis ceiling column above gives the numbers). Everyday prayers work at a level-1 temple. Spontaneous blessings need 60 kharis and a maintained temple. Only a level-2 temple or higher lets kharis grow that far. Kharis retained above a lowered ceiling still counts. Levelling a temple further needs [[cedar|cedar]].

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
Each temple rests on its own after it answers; a higher level rests shorter, and only a fully staffed temple recovers (see [[rites]]). **Kult → Kharis** explains your
realm's standing, daily change and current divine risk; see [[kharis]].

`keryx status` lists all your temples. `keryx rite --list` lists prayers across
your realm with the `--settlement` to use for each; add `--settlement <id>` to
filter the list to one city.

## Daily upkeep of the cult {#cult-upkeep}

Realm demand grows with each city: **the first active city demands 1, the second adds 2, the third adds 3**, and so on. All active cities count, including the capital, with or without temples. Sunk, collapsed and razed cities do not count. Population, armies and crews do not change demand. Cult upkeep is a fixed amount per point of demand each game day (see the upkeep table in [[kharis]]).

One, two, three and four cities demand **1, 3, 6 and 10** respectively. Four fully staffed level-1 temples in a four-city realm give 4 weight: **about 2.33 income − 3.33 upkeep = −1 kharis per game day**. A realm with no active city has zero demand.

### Temple production and punishment risk {#cult-upkeep-2}

**Kult → Kharis** shows, for each game day, what your temples produce, the realm's cult upkeep and the net change. Staffing and paid tithe both multiply temple weight. Improve either to raise its contribution; raising the temple level increases weight and tithe requirements.

Below **30 Kharis**, there is a 2% chance of punishment each game day, 4% below 5, **with or without a temple**. An actual punishment gives **seven game days of protection**. A game day follows this world's pace; Kult and `keryx status` show the protection's end in your local clock. Nothing taken means no new protection. See [[divine-wrath]]. A punishment chooses a random eligible city of yours with something it can take; it need not strike the capital.

## Demolishing a temple {#temple-demolition}

Voluntarily demolishing a [[temples|temple]] angers its god. Your realm loses **10 kharis per temple level**, down to a **floor of 1**. This is an immediate consequence of your own demolition, separate from the daily chance of [[divine-wrath|divine punishment]]. It does not grant punishment protection.

Before you confirm, **City → Demolish** and `keryx demolish --type temple --god potnia` show the actual kharis loss, standing afterward and the ceiling before and after. Keryx asks for confirmation; non-interactive or `--json` use without `--yes` previews the consequences without demolishing. Run again with `--yes` to confirm. The result and the **god is angered** notification record what actually happened. Nothing is refunded, and only that temple's workers become idle.

Your best remaining temple determines the ceiling: the Kharis ceiling column of the temple table above gives the number for each level, and a realm with no temple has the lowest. Another equally high temple can keep the ceiling unchanged. **Demolition costs kharis immediately; the lower ceiling takes no additional kharis.** Standing above it declines by ordinary daily cult upkeep; temple gains do not offset this decline. Once standing is at or below the ceiling, ordinary gain and upkeep apply again. The voluntary demolition loss still applies.

A temple destroyed by **plundering** creates **no demolition debt** for its owner. Its loss can lower the ceiling and remove its production. Cancelling construction is not demolition and does not incur the kharis loss.
