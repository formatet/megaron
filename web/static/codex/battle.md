**There is no declaration of war.** If your army arrives where a hostile army is, or a unit on watch catches a march going by, a battle starts by itself.

## When battle begins {#battle-when-battle-begins}

If your forces defend a hex, or you own the city there, you receive a dispatch when battle begins: *"Idomeneus — battle has begun at Petras"*, with its clock time. It names the opposing Wanax and the place you can already see; it reveals no march route or home city. Select **Take me there** to see the hex. With sound on, you hear the same clash as for a battle's outcome.

Each defending Wanax receives one notice per battle, even with several forces. Reinforcements do not repeat it; a new defending Wanax receives a first notice when their forces join. You can mute the dispatch and still find it in **Notifications**.

## Battle rounds {#battle-how-a-battle-runs}

**A battle lasts several game days.** Each game day the two sides clash three times. In every clash, 90% of each side's men take part and roll twelve-sided dice. **A 12 is a hit**, and every hit costs the enemy one man, one chariot, or a blow to a ship's hull. Losses are shared among a side's forces in proportion to their size.

| | Dice in each clash |
|---|---|
| [[spearmen|Spearmen]] | 1 for every 2 men |
| [[elite-infantry|Bronze Guard]] | 3 per man |
| [[war-chariot|Chariot]] | 5 per chariot |
| [[war-galley|War Galley]] | 5 per ship |
| [[galley|Galley]], [[emporos|Emporos]] | 1 per ship |

So 100 spearmen put 90 men in a clash, roll 45 dice and can expect about 4 hits; 100 Bronze Guard roll 270 dice and can expect about 22. This is what [[bronze]] buys. A damaged ship rolls fewer dice, in proportion to its remaining hull.

**[[walls|Walls]].** Each game day the wall takes the first **20 hits per wall level** aimed at the defenders before any of them falls. An attacker in the **storm** stance halves what the wall stops, but every hit the defenders land on the stormers counts **one and a half times** ([[stances]]).

**Breaking.** A side that has lost enough routs and withdraws with its survivors; annihilation is the exception. You choose how much is "enough" for your whole realm under **War → Forces → When to retreat**: **Hold to the last man** or **Retreat when it goes badly** (the default, retreating when 20% of starting strength remains). You can override it for one unit in the battle it is fighting; see [[stances]]. If both sides break on the same game day, both withdraw and nobody wins.

**Reinforcements** that reach a battle already under way join it.

<!-- src: server/internal/combat/battle.go battleRoundsPerTick, combatParticipation, battleDiceSides, battleDiceMultiplier, wallAbsorbPerLevel, stormWallAbsorbDivisor, stormAttackerLossMultiplier; resolver.go combatRoutFraction -->

## Battle reports and survivors {#battle-after-the-battle}

You are told the outcome, with the losses on both sides — for example *"Victory at Petras — your [[spearmen|spearmen]] (100→61) vs Idomeneus's chariots (40→12). The field was taken."* You hear the clash of arms if sound is on.

**Ships after a battle.** Every surviving ship of a side loses hull in proportion to that side's losses; soldiers aboard lose the same share of their men. A ship whose hull reaches 0 sinks with everyone on board. Repair it at a [[shipyard|Shipyard]] ([[ship-types|Ships]]). <!-- src: server/internal/combat/ship_hull.go -->

Winning at a city does not make it yours. It becomes **occupied** — see [[occupation]]. A city can also be taken without any battle at all, by [[food|hunger]]: see [[sieges]].

**[[mistress-of-the-walls]]** ([[rites]]) strengthens only the casting Wanax's own garrison defending the blessed city, for 6 game days. It gives no bonus to an attacker, a field army or a new owner of the city.
