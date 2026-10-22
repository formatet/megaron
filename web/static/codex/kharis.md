**Kharis** is your standing with the gods — a relationship, not a resource. It is held for your whole realm, not per city, and your [[account|Account]] summary shows its current amount. The **Kult (⛩)** drawer shows it as a mood:

| Mood | Meaning |
|---|---|
| **Favorable** | The gods may bless you: a blessed harvest (a quarter more food for 25 days, fish excepted) or unexpected recruits. |
| **Indifferent** | They have noticed you and do nothing. |
| **Suspicious** (5 to below 30) | Without a standing temple, a 3% chance of punishment each day. |
| **Wrathful** (below 5) | The same risk, and the gods are close to silent: chariots break, ships are lost, food rots or plague takes a garrison. |

Blessings and punishments arrive as notifications.

A new Wanax starts at **40 Kharis**, before founding. Rising again keeps your standing with the gods, with the floor as a minimum. Kharis never falls all the way to nothing. It fades from the start; founding does not pause decay.

## The cost of your realm

Daily decay is **0.1 base + 0.1 × N × (N − 1) / 2**, where **N** counts all your active cities, including your capital, whether or not they have temples. Sunk, collapsed and razed cities do not count. Five cities cost **1.0 per day**, plus the **0.1 base**: **1.1 total**. Each additional city adds more cost than the last.

**Kult → Daily decay** and `keryx status` show the base, city count, city cost and total alongside the projected net. A temple earns favour; it does not remove a city's cost. A standing temple protects you from punishment: below 30 Kharis without one, there is a 3% chance each day. A punishment chooses a random eligible city of yours with something it can take; it need not strike the capital.

## How to raise it

Three things must be true, every day:

1. **You have a temple.** Without one, kharis decays.
2. **People serve in it** — placed temple workers, 2 workplaces per level. Each earns 1.05 kharis per day before the offering factor, scarcity and decay. See [[temples]].
3. **The temple is fed** — a daily reference gift of 0.25 grain, with local fish, timber or silver as substitutes of equal divine value. See [[temples]] for the selection order and planned gift.

In a small realm, a fully staffed, fed level-1 temple can earn more favour than the realm loses each day. Compare the projected net; a larger realm may need more temples or higher levels. Larger temples can employ more devotion and raise kharis faster when staffed and fed.

**Your best temple sets your ceiling:** no temple **40**, level 1 **50**, level 2 **75**, level 3 **100**. A level-1 temple can never earn you a blessing. To reach blessings, you need a higher-level temple.

Kharis decays every day regardless. Expansion increases that cost even when every city has a temple ([[colonies]]).

To *spend* kharis on a request, see [[rites]].

An ongoing **blessed harvest** appears beside Food in the city drawer and in
`keryx status` and `keryx city`, with its yield bonus, affected goods and days left.
The row disappears when the blessing ends; fish receives no harvest bonus.

The temple shows how much each good counts with the gods, before a particular god's preferences. What the world can spare can raise what devotion earns; this is the gods' reckoning, not a buying price. The ordinary daily offering and your choice of devotion remain separate from a prayer's one-time gift.

A notification marks each change between **Favorable**, **Indifferent**, **Suspicious** and **Wrathful**, with the new Kharis value and recorded day. Check **Kult** for decay and temple support; staff and feed your [[temples]].
