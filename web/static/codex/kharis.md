**Kharis** is your standing with the gods — a relationship, not a resource. It is held for your whole realm, not per city, and your [[account|Account]] summary shows its current amount. The **Kult (⛩)** drawer shows it as a mood:

| Mood | Meaning |
|---|---|
| **Favorable** | The gods may bless you: a blessed harvest (a quarter more food for 25 days, fish excepted) or unexpected recruits. |
| **Indifferent** | They have noticed you and do nothing. |
| **Suspicious** (5 to below 30) | Below 15, a 3% chance of punishment each day, with or without a temple; recent punishment gives seven game days of protection. |
| **Wrathful** (below 5) | The same risk below 15, except during protection: chariots break, ships are lost, food rots or plague takes a garrison. |

Blessings and punishments arrive as notifications.

A new Wanax starts at **40 Kharis**, before founding. Rising again keeps your standing with the gods, with the floor as a minimum. Kharis never falls all the way to nothing. It fades from the start; founding does not pause decay.

## Upkeep of the cult

Daily upkeep is **0.1 base + 0.2 per city beyond the first + 0.1 per 1,000 residents**. It counts all your active cities, including your capital, whether or not they have temples. Sunk, collapsed and razed cities do not count. Residents are the actual population in those cities, including people short of a whole citizen; armies and crews away from the population are not counted a second time. This is the upkeep of the cult, rather than an extra penalty that accelerates as your realm grows.

For **three cities with 2,500 residents altogether**, upkeep is **0.1 + 0.4 + 0.25 = 0.75 kharis per day**. A realm without a city has the base cost only.

**Kult → Upkeep of the cult** and `keryx status` show the base, cities and residents separately, alongside projected temple production and net. One more fed temple worker gives **at least +1.05 kharis per day**, and more when the cult’s reference offering of grain is scarce in the world. This describes the extra worker's yield: employing one also requires a free citizen, an available temple workplace and that temple's daily offering. A temple earns favour; it does not remove its city's cost.

Below **15 Kharis**, there is a 3% chance of punishment each day **with or without a temple**. An actual punishment gives **seven game days of protection**. A game day follows this world's pace; Kult and `keryx status` show the protection's end in your local clock. Nothing taken means no new protection. See [[divine-wrath]]. A punishment chooses a random eligible city of yours with something it can take; it need not strike the capital.

## How to raise it

Three things must be true, every day:

1. **You have a temple.** Without one, kharis decays. Each temple belongs to one god, a city may hold one per god, and kharis is still one pool for the realm.
2. **People serve in it** — placed temple workers, 2 workplaces per level in each temple. Each earns 1.05 kharis per day before the offering factor, scarcity and decay. See [[temples]].
3. **The temple is fed** — each temple's own daily reference gift of 0.25 grain, with local fish, timber or silver as substitutes of equal divine value. See [[temples]] for the selection order and planned gift.

In a small realm, a fully staffed, fed level-1 temple can earn more favour than the realm loses each day. Compare the projected net; a larger realm may need more temples or higher levels. Larger temples can employ more devotion and raise kharis faster when staffed and fed.

**Your best temple sets your ceiling:** no temple **40**, level 1 **50**, level 2 **75**, level 3 **100**. Everyday prayers work at a level-1 temple. Spontaneous blessings need **60 kharis** and a maintained temple. Only a level-2 temple or higher lets kharis grow that far. Kharis retained above a lowered ceiling still counts.

Kharis decays every day regardless. Additional cities and residents increase cult upkeep even when every city has a temple ([[colonies]]).

Kharis is a requirement for [[rites|prayers]], never spent. Everyday prayers answer when their requirements are met; they cost goods and waiting time.

An ongoing **blessed harvest** appears beside Food in the city drawer and in
`keryx status` and `keryx city`, with its yield bonus, affected goods and days left.
The row disappears when the blessing ends; fish receives no harvest bonus.

The temple shows how much each good counts with the gods, before a particular god's preferences. What the world can spare can raise what devotion earns; this is the gods' reckoning, not a buying price. The ordinary daily offering and your choice of devotion remain separate from a prayer's one-time gift.

A notification marks each change between **Favorable**, **Indifferent**, **Suspicious** and **Wrathful**, with the new Kharis value and recorded day. Check **Kult** for decay and temple support; staff and feed your [[temples]].

Demolishing a temple voluntarily costs **10 kharis per level**, with a floor of **1**; plundering causes no such debt. The demolition loss is immediate. A lower ceiling takes no additional kharis; above it, daily cult upkeep lowers kharis and temple workers and offerings do not raise it. The confirmation shows the loss and any ceiling change. See [[temple-demolition]].
