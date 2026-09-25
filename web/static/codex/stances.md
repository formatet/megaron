A **stance** tells a unit how to behave where it stands. Set it in the order menu when you march, or later from **War → Army** with the stance selector and **Set** — which, for a unit in the field, is an order carried by [[runners|Runner]].

- **fortify** — dig in. Better defence, and more of your men engage when attacked. The unit cannot move until the stance is cleared.
- **storm** — assault posture against an adjacent city. Halves what its wall absorbs, at the cost of much heavier losses of your own.
- **sentry** — watch this ground. The unit spots marches passing nearby and **intercepts** enemy caravans within reach. You are alerted when it catches something.

## A unit already on the march

You can give a marching unit a stance too — from **War → Army**, or by picking a stance when you right-click a new destination for it. The order goes by [[runners|Runner]], and the Runner has to **catch up** with the unit: the game tells you where and roughly when it will reach it. From then the unit carries the stance, and it takes hold **where the unit stops** — fortify digs in there, sentry watches that ground, storm goes into the assault it is marching on. A stance never turns a unit off its road.

If the unit is too far ahead for any Runner to overtake, the Runner follows it to its destination and the stance applies where it stopped.

## Sentry is a siege

Units in sentry on the hexes an enemy city works stop those hexes feeding it. Hold that long enough and the city gives up without a battle — see [[sieges]].

## A watch is not free

A unit standing in the field eats more than one in garrison, with nothing to forage ([[upkeep]]). Post watches where they buy you warning time — on the roads an enemy would take — not everywhere. See [[defence]].

From the command line there are also dedicated watch orders: a ship on **patrol** at a coastal hex, which comes home by itself when the patrol ends, and a land unit **posted** as a forward watch until you recall it ([[keryx]]).

## When to retreat

When your men break off a battle is set in two places:

- **For the whole realm** — **War → Army → When to retreat**, or `keryx retreat-default` ([[keryx]]). Choose a share of losses, **hold to the last man**, or leave it to the troops' [[loyalty]] (the default: the more loyal their city, the longer they hold). Every unit carries this into a battle it enters. It applies at once, since no one has to carry it anywhere, but only to battles entered from then on — a battle already under way keeps what its units brought.
- **For one unit, in the battle it is fighting now** — the **this battle** selector on its card in **War → Army** (it only appears while the unit is fighting), or `keryx unit retreat-order`. It overrides the realm setting for that battle only. On a field unit it is an order, and travels by [[runners|Runner]].
