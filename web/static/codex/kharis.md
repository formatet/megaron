**Kharis** is your standing with [[gods|the gods]], held for your whole realm.

- A new Wanax starts at 40; it never falls below 1.
- Each active city adds to the realm's demand, which fades kharis every game day.
- Staffed, fed temples raise it; your best temple sets the ceiling (50, 75, 100; 40 with none).
- Below 30 the gods may punish you; a punishment gives 7 game days of protection.
- Prayers need a minimum standing but never spend it. <!-- src: reference.json kharis -->

## What the gods ask, in numbers {#kharis-numbers}

Every prayer asks for a minimum standing, and each has its own offering and rest.

<!-- kharis-prayers -->
| Prayer | God | Required kharis | Traditional offering | Temple’s rest at level 1 / 2 / 3 |
|---|---|---|---|---|
| [[following-seas|Following Seas]] | [[sea-daimons|The Sea Daimons of Stillness]] | 5 | 6 [[fish|fish]] · 3 [[silver|silver]] | 12 / 6 / 4 game days |
| [[fruitful-season|Fruitful Season]] | [[potnia|Potnia]] | 5 | 4 [[grain|grain]] · 4 [[livestock|livestock]] | 12 / 6 / 4 game days |
| [[green-shoots|Green Shoots]] | [[velchanos|Velchanos]] | 30 | 15 [[grain|grain]] · 2 [[silver|silver]] | 25 / 13 / 9 game days |
| [[mistress-of-the-walls|Mistress of the Walls]] | [[potnia|Potnia]] | 5 | 8 [[grain|grain]] · 4 [[timber|timber]] (or [[cedar|cedar]]) | 24 / 12 / 8 game days |
| [[potnias-vision|Potnia's Vision]] | [[potnia|Potnia]] | 5 | 10 [[grain|grain]] · 3 [[silver|silver]] | 24 / 12 / 8 game days |
| [[still-the-storm|Still the Storm]] | [[sea-daimons|The Sea Daimons of Stillness]] | 5 | 10 [[fish|fish]] · 6 [[oil|oil]] | 12 / 6 / 4 game days |
| [[stout-hulls|Stout Hulls]] | [[sea-daimons|The Sea Daimons of Stillness]] | 5 | 8 [[fish|fish]] · 4 [[oil|oil]] | 12 / 6 / 4 game days |
| [[swift-feet|Swift Feet]] | [[asterion|Asterion]] | 5 | 3 [[silver|silver]] · 4 [[timber|timber]] (or [[cedar|cedar]]) | 12 / 6 / 4 game days |
| [[the-bird-oracle|The Bird Oracle]] | [[asterion|Asterion]] | 30 | 10 [[silver|silver]] · 8 [[wine|wine]] | 24 / 12 / 8 game days |
| [[builders-blessing|The Builder's Blessing]] | [[velchanos|Velchanos]] | 5 | 3 [[timber|timber]] (or [[cedar|cedar]]) · 2 [[wine|wine]] | 12 / 6 / 4 game days |
| [[the-bulls-horns|The Bull's Horns]] | [[asterion|Asterion]] | 5 | 5 [[livestock|livestock]] · 2 [[silver|silver]] | 12 / 6 / 4 game days |
| [[the-scapegoat|The Scapegoat]] | [[potnia|Potnia]] | 5 | 3 [[livestock|livestock]] | 12 / 6 / 4 game days |
| [[the-thread|The Thread]] | [[asterion|Asterion]] | 5 | 2 [[silver|silver]] · 3 [[wine|wine]] | 12 / 6 / 4 game days |
| [[the-walls-fall|The Walls Fall]] | [[potnia|Potnia]] | 5 | 12 [[livestock|livestock]] · 8 [[silver|silver]] | 24 / 12 / 8 game days |
| [[the-young-gods-arm|The Young God's Arm]] | [[velchanos|Velchanos]] | 5 | 6 [[livestock|livestock]] · 4 [[wine|wine]] | 12 / 6 / 4 game days |
| [[wake-the-storm|Wake the Storm]] | [[sea-daimons|The Sea Daimons of Stillness]] | 5 | 10 [[fish|fish]] · 6 [[wine|wine]] | 12 / 6 / 4 game days |
<!-- /kharis-prayers -->

Kharis is never spent on a prayer; it is only a minimum you must hold.

<!-- kharis-demand -->
| Active cities | Realm demand | Upkeep per game day | Temple weight to hold steady |
|---|---|---|---|
| 1 | 1 | 0.33 | 0.57 |
| 2 | 3 | 1.00 | 1.71 |
| 3 | 6 | 2.00 | 3.43 |
| 4 | 10 | 3.33 | 5.71 |
| 5 | 15 | 5.00 | 8.57 |
| 6 | 21 | 7.00 | 12.00 |
<!-- /kharis-demand -->

Where your kharis stands decides the risk and the favour you meet.

<!-- kharis-standing -->
| Kharis | What it means |
|---|---|
| 1 | The floor: kharis never falls below this. |
| below 5 | 4% chance of punishment each game day. |
| below 30 | 2% chance of punishment each game day. |
| 30 and above | No punishment. |
| 40 | A new Wanax starts here. It is also the ceiling with no temple. |
| 50 | The ceiling set by a level 1 temple. |
| 60 and above | With a maintained temple: 15% chance of a spontaneous blessing each game day. |
| 75 | The ceiling set by a level 2 temple. |
| 100 | The ceiling set by a level 3 temple. |
<!-- /kharis-standing -->

## Divine moods {#kharis-details}

| Mood | Meaning |
|---|---|
| **Favorable** (60 and above) | Blessings: a harvest a quarter richer for 25 game days ([[fish|fish]] excepted) or unexpected recruits. At sea a 15% chance that a [[storms|storm]]'s blow is held back. In [[battle]], about 5% more dice. |
| **Indifferent** (30 to below 60) | They have noticed you and do nothing. |
| **Suspicious** (5 to below 30) | The risk of punishment begins ([[divine-wrath]]). |
| **Wrathful** (below 5) | Punishment is likelier: [[war-chariot|chariots]] break, ships are lost, food rots or plague takes a garrison. At sea a 15% chance that a [[storms|storm]] takes one more hull. In [[battle]], about 5% fewer dice. |

### Starting standing and true omens {#kharis-details-2}

Higher kharis brings true omens more often and in more detail; they never reveal exact strength, arrival times, routes or unknown place names.

A new Wanax starts at **40 Kharis**, before founding; rising again keeps your standing. With one city and no temple about 10 points fade in 30 game days: build a temple early.

## Maintaining divine standing {#raising-kharis}

Kharis holds only while a temple exists, is **staffed** and has its **tithe paid** ([[temples]], [[daily-offerings]]). A larger realm needs more temples or higher levels. **Kult → Kharis** shows **Per extra temple worker: +x.xx**, the least one more fed worker adds per game day before upkeep.

### The Kharis ceiling and offerings {#raising-kharis-2}

**Your best temple sets your ceiling** (table above); with no temple it is the lowest.

Kharis changes by **what your temples produce − what your realm's demand costs** per game day ([[cult-upkeep]]). Population does not contribute. **Above ceiling (N)** on the Kult drawer: [[upkeep|upkeep]] continues and temples add nothing until standing is back at or below N.

### Kult tabs and changes in standing {#raising-kharis-3}

A notification marks each change of mood; your [[account|Account]] summary shows the amount. The **Kult** drawer has three tabs: **Kharis**, **Prayers** and **[[temple|Temples]]** ([[temples]]). Demolishing a temple costs 10 kharis per level ([[temple-demolition]]).

## Divine wrath {#divine-wrath}

The gods may punish a realm **below 30 kharis**, with or without a [[temples|temple]]: a **2% chance** each game day while **Suspicious** (5 to below 30), **4%** once **Wrathful** (below 5). At exactly 30 there is no risk. It strikes one active city that has something to take.

### Warnings of divine wrath {#divine-wrath-warning-before-danger}

Crossing **30 downward** with a negative net sends one warning with the game days until Wrathful (5); it repeats only after you recover above 30.

### Protection after punishment {#divine-wrath-seven-game-days-after-punishment}

An actual loss gives your whole realm **seven game days of protection**; a punishment borne by [[the-scapegoat]] does too. Nothing to take means neither. Protection does not stop cult upkeep or losses from [[food|hunger]], [[battle|battle]] or weather.

### Temples and divine punishment {#divine-wrath-temple-levels}

Everyday prayers work at a level-1 temple ([[rites]]).

Transition protection: [[religion-rule-change]].
