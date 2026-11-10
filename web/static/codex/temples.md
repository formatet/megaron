A **[[temple]]** ([[buildings]]) is how you keep [[gods|the gods]]' favour ([[kharis]]).

- A city may hold several temples, **at most one per god**: [[asterion]], [[potnia]], the [[sea-daimons|Sea Daimons]] of Stillness, [[velchanos]].
- Levels 1/2/3 have 2/4/8 workplaces; an unstaffed temple gives nothing.
- Each temple pays a daily tithe from its own city; unpaid parts lose effect.
- Your best temple sets your kharis ceiling: 50, 75 or 100.
- Demolishing a temple costs 10 kharis per level. <!-- src: reference.json kharis levels, demolition_per_level -->

## Building and upgrading temples {#temples-building-one}

Choose the god when you build: **City → Construct** has one row per god; Keryx `keryx build --type temple --god potnia`. Building the same god's temple again raises its level. Each prayer is said at its own god's temple and works from level 1 ([[rites]]).

## Temple workers {#temple-workers}

Place or remove whole [[citizens]] as at any workplace; Keryx `staff temple +1 --god velchanos` (name the god when a city has several). A worker stands at one temple only. **Temple workplaces unfilled** on **Kult → Kharis** means empty places.

Levels 1/2/3 have **weight 1/4/9**; effective weight = weight × staffed fraction × paid tithe fraction, and each point yields the same small kharis per game day (table below).

## A temple’s daily offering {#daily-offerings}

Each temple pays its tithe from its own city. Temples in one city pay in god order (Asterion, Potnia, Stillness, Velchanos), each against what the earlier ones left. The tithe per level is in the table below.

The grain part is paid with **grain first, then [[fish|fish]], then [[timber|timber]], then [[livestock]]** (counts as 2). Silver never replaces grain. Each part is paid separately: a part that cannot be paid in full takes nothing. A level-2 temple without silver still gets 75% effect; a level-3 temple with only its grain part gets 70%. [[rites|Prayers]] keep their own offerings.

### The rich offering {#rich-offering}

Each temple can add a **rich offering**: pick one good from the city's store (**Kult → Temples**, or `keryx rich-offering --god potnia --good fish`; `--off` to stop). Each game day, after all tithes, the temple also receives half the tithe's grain value in that good, divided by the good's divine value and by the god's taste for it; a taste of ×2 halves the amount. Other goods count at ×1.

<!-- tastes -->
| God | Prizes in a rich offering |
|---|---|
| Asterion | [[silver|silver]] ×2, [[stone|stone]] ×1.5 |
| Potnia | [[livestock|livestock]] ×2, [[fish|fish]] ×1.5 |
| The Sea Daimons of Stillness | [[fish|fish]] ×2, [[oil|oil]] ×1.5 |
| Velchanos | [[wine|wine]] ×2, [[grain|grain]] ×1.5, [[livestock|livestock]] ×1.5 |
<!-- /tastes -->

If it is paid in full, the temple's effective weight is **multiplied by 1.5**; if the good is missing, nothing is taken and there is no partial bonus. It gives nothing while the tithe goes unpaid.

### Planning temple offerings {#daily-offerings-2}

**Kult → Temples** and `keryx status` forecast each tithe against current stocks.

## Temple levels and the Kharis ceiling {#temple-levels}

A temple does not remove its city from the demand ([[kharis]]), nor protect low standing from [[divine-wrath|divine wrath]].

<!-- temple-levels -->
| Level | Workplaces | Weight | Kharis per game day, full staff and tithe | With rich offering | Kharis ceiling | Daily tithe |
|---|---|---|---|---|---|---|
| 1 | 2 | 1 | 0.58 | 0.88 | 50 | 0.25 [[grain|grain]] (100%) |
| 2 | 4 | 4 | 2.33 | 3.50 | 75 | 0.75 [[grain|grain]] (75%) + 0.1 [[silver|silver]] (25%) |
| 3 | 8 | 9 | 5.25 | 7.88 | 100 | 1.75 [[grain|grain]] (70%) + 0.25 [[silver|silver]] (20%) + 0.1 [[incense|incense]] (10%) |
<!-- /temple-levels -->

**Your best temple sets your kharis ceiling**; with no temple it is the lowest (table above). Spontaneous blessings need 60 kharis and a maintained temple. Levelling a temple further needs [[cedar|cedar]].

### Temple demolition and realm temple lists {#temple-levels-2}

See [[temple-demolition]] for the 10-per-level cost.

## Finding your temples {#temples-find-your-temples}

**Kult → Temples** lists every standing temple with city, god, level, workers and next offering; **Manage in City** opens it. **Kult → Prayers** lists prayers by temple and city, ready ones first. Each temple rests on its own after answering a prayer; a higher level rests shorter, and only a fully staffed temple recovers ([[rites]]). `keryx status` lists your temples; `keryx rite --list` lists prayers.

## Daily upkeep of the cult {#cult-upkeep}

Realm demand grows with each active city (the first demands 1, the second adds 2, the third adds 3 ...), capital included. Sunk, collapsed and razed cities do not count; population and armies do not. [[upkeep|Upkeep]] per point of demand is in the table in [[kharis]].

### Temple production and punishment risk {#cult-upkeep-2}

**Kult → Kharis** shows, per game day, what your temples produce, the cult upkeep and the net change. Below **30 kharis** there is a 2% chance of punishment each game day, 4% below 5, with or without a temple. A punishment gives **seven game days of protection**; nothing taken means none. It strikes a random eligible city that has something to take, not necessarily the capital ([[divine-wrath]]).

## Demolishing a temple {#temple-demolition}

Voluntarily demolishing a [[temples|temple]] angers its god: **10 kharis per temple level**, down to a **floor of 1**, immediately. It grants no punishment protection. Before you confirm, **City → Demolish** or `keryx demolish --type temple --god potnia` shows the loss and the ceiling before and after; without `--yes` (or with `--json`) Keryx only previews. Nothing is refunded; only that temple's workers go idle.

Your best remaining temple sets the ceiling, so an equally high temple keeps it. **The lower ceiling takes no additional kharis**: standing above it declines by ordinary cult upkeep, with no temple gain to offset it.

A temple destroyed by **plundering** creates no demolition debt. Cancelling construction is not demolition and costs no kharis.
