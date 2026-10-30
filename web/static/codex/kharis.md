**Kharis** is your standing with [[gods|the gods]] — a relationship, not a resource. It is held for your whole realm, not per city, and your [[account|Account]] summary shows its current amount. The **Kult (⛩)** drawer shows the number, mood and its current meaning:

## Divine moods {#kharis-details}

| Mood | Meaning |
|---|---|
| **Favorable** (60 and above) | The gods may bless you: a blessed harvest (a quarter more food for 25 game days, [[fish|fish]] excepted) or unexpected recruits. |
| **Indifferent** (30 to below 60) | They have noticed you and do nothing. |
| **Suspicious** (5 to below 30) | Below 15, a 3% chance of punishment each game day, with or without a temple; recent punishment gives seven game days of protection. |
| **Wrathful** (below 5) | The same risk below 15, except during protection: [[war-chariot|chariots]] break, ships are lost, food rots or plague takes a garrison. |

A standing temple and at least **60 kharis** give a **15% chance of a spontaneous blessing each game day**. This is separate from a prayer: kharis is a requirement for prayers, never spent. The number is your standing, not a percentage. Protection temporarily removes punishment risk; a favorable mood without a standing temple cannot bring spontaneous blessings. These are dice rolls, not promises.

### Starting standing and true omens {#kharis-details-2}

Higher kharis also brings true omens more often, from farther away and with more detail. Only the highest standing can rarely point to a concrete place for an urgent fact. Omens never reveal exact strength, future arrival times or routes, and do not reveal unknown place names.

Blessings and punishments arrive as notifications.

A new Wanax starts at **40 Kharis**, before founding. Rising again keeps your standing with the gods, with the floor as a minimum. Kharis never falls all the way to nothing. It fades from the start; founding does not pause decay.

## Maintaining divine standing {#raising-kharis}

Three things must be true, every game day:

1. **You have a temple.** Without one, kharis decays. Each temple belongs to one god, a city may hold one per god, and kharis is still one pool for the realm.
2. **People serve in it** — levels 1/2/3 have 2/4/8 workplaces and weights 1/4/9. Staffing scales the weight; see [[temples]].
3. **The tithe is paid** — [[grain|grain]] at every level, [[silver|silver]] from level 2, [[incense|incense]] at level 3. Each paid part adds its share of effect; see [[daily-offerings]].

In a small realm, a fully staffed, fed level-1 temple can earn more favour than the realm loses each game day. Compare the projected net; a larger realm may need more temples or higher levels. Larger temples can employ more devotion and raise kharis faster when staffed and fed.

### The Kharis ceiling and offerings {#raising-kharis-2}

**Your best temple sets your ceiling:** no temple **40**, level 1 **50**, level 2 **75**, level 3 **100**. Everyday prayers work at a level-1 temple. Spontaneous blessings need **60 kharis** and a maintained temple. Only a level-2 temple or higher lets kharis grow that far. Kharis retained above a lowered ceiling still counts.

Kharis changes by **2.1 × effective temple weight × scarcity − 1.2 × active cities²** per game day. Population does not contribute. The floor and temple ceiling still apply; above a lowered ceiling, [[upkeep|upkeep]] continues without temple gain offsetting it.

Kharis is a requirement for [[rites|prayers]], never spent. Everyday prayers answer when their requirements are met; they cost goods and waiting time.

An ongoing **blessed harvest** appears beside Food in the city drawer and in
`keryx status` and `keryx city`, with its yield bonus, affected goods and game days left.
The row disappears when the blessing ends; fish receives no harvest bonus.

The temple shows how much each good counts with the gods, before a particular god's preferences. What the world can spare can raise what devotion earns; this is the gods' reckoning, not a buying price. The ordinary daily offering and your choice of devotion remain separate from a prayer's one-time gift.

### Kult tabs and changes in standing {#raising-kharis-3}

A notification marks each change between **Favorable**, **Indifferent**, **Suspicious** and **Wrathful**, with the new Kharis value and recorded game day. Check **Kult** for decay and temple support; staff and feed your [[temples]].

Demolishing a temple voluntarily costs **10 kharis per level**, with a floor of **1**; plundering causes no such debt. The demolition loss is immediate. A lower ceiling takes no additional kharis; above it, daily cult upkeep lowers kharis and temple workers and offerings do not raise it. The confirmation shows the loss and any ceiling change. See [[temple-demolition]].

The **Kult** drawer has three tabs: **Kharis** for this realm-wide standing and
its daily change, **Prayers** for prayers in each of your cities, and **[[temple|Temples]]**
for every temple's god, city, level, workers and daily gift. Your standing is
shared; an offering still comes from the city named in its prayer row. See
[[temples]] for the controls.

## Divine wrath {#divine-wrath}

The gods may punish a realm **below 15 kharis**, with or without a [[temples|temple]]. Each game day brings a **3% chance**. At exactly 15 there is no risk. A punishment chooses one of your active cities that has something it can take.

### Warnings of divine wrath {#divine-wrath-warning-before-danger}

When your standing crosses **30 downward** and the projected net is negative, a warning tells you your kharis, net per game day and how many game days it would take to reach 15 at that rate. It is sent once per downward crossing. A positive or zero net gives no warning. Further decline below 30 does not repeat it; recovery above 30 allows a new downward crossing to warn again.

The estimate assumes that net continues. Staff and feed your temples, then check **Kult** or `keryx status`. A mood change still has its own notice.

### Protection after punishment {#divine-wrath-seven-game-days-after-punishment}

An actual loss gives your whole realm **seven game days of protection from further divine punishment**. A game day follows this world's pace; **Kult**, `keryx status` and the punishment notice show the protection's end in your local clock. Repeated processing does not extend it. At the end, the ordinary risk resumes if kharis is still below 15. The protection follows the Wanax, across their cities and temples.

If there is nothing to take, there is no punishment and no new protection. Protection does not stop cult upkeep or prevent losses from [[food|hunger]], [[battle|battle]] or weather.

### Temples and divine punishment {#divine-wrath-temple-levels}

Everyday prayers work at a level-1 temple. Spontaneous blessings need **60 kharis** and a maintained temple. Only a level-2 temple or higher lets kharis grow that far. Kharis retained above a lowered ceiling still counts. The ceilings remain **50 / 75 / 100** for temple levels 1 / 2 / 3. See [[kharis]] and [[rites]].

See [[religion-rule-change]] for the one-time transition protection.
