A **stance** tells a unit how to behave where it stands. Set it in the order menu when you march, or later from **War → Forces** with the stance selector and **Set** — which, for a unit in the field, is an order carried by [[runners|Runner]].

## Choosing a stance {#stances-details}

- **fortify** — dig in and hold this ground. The unit cannot move until the stance is cleared. [[battle]] participation stays at 90%.
- **storm** — assault posture against an adjacent city. The wall stops only half as many hits, but the defenders’ hits on your men count one and a half times ([[battle]]).
- **sentry** — watch this ground. The unit spots marches passing **within 2 hexes** and **intercepts** enemy [[transfers|caravans]] there, checked every game day. [[messengers|Messengers]] are never stopped. You are alerted when it catches something.

## Changing stance by Runner {#stances-a-unit-already-on-the-march}

You can give a marching unit a stance too — from **War → Forces**, or by picking a stance when you right-click a new destination for it. The order goes by [[runners|Runner]], and the Runner has to **catch up** with the unit: the game tells you where and roughly when it will reach it. From then the unit carries the stance, and it takes hold **where the unit stops** — fortify digs in there, sentry watches that ground, storm goes into the assault it is marching on. A stance never turns a unit off its road.

If the unit is too far ahead for any Runner to overtake, the Runner follows it to its destination and the stance applies where it stopped.

## Taking a shared catchment hex {#stances-taking-a-shared-hex}

A unit in **fortify** or **sentry** on a hex that a neighbouring city holds lets you take it by placing a [[citizens|citizen]] there — see [[catchment]]. A unit that is only passing through does not.

## Sentry and sieges {#stances-sentry-is-a-siege}

Forces in sentry on the hexes an enemy city works stop those hexes feeding it. Hold that long enough and the city gives up without a battle — see [[sieges]].

## The cost of a watch {#stances-a-watch-is-not-free}

A unit standing in the field eats more than one in garrison, with nothing to forage ([[upkeep]]). Post watches where they buy you warning time — on the roads an enemy would take — not everywhere. See [[defence]].

From the command line there are also dedicated watch orders: a ship on **patrol** at a coastal hex, which comes home by itself after **24 game days** on station, and a land unit **posted** as a forward watch until you recall it ([[keryx]]).

## Battle retreat orders {#stances-when-to-retreat}

When your men break off a battle is set in two places:

- **For the whole realm** — **War → Forces → When to retreat**, or `keryx retreat-default` ([[keryx]]). Choose **Hold to the last man** or **Retreat when it goes badly** (the default). The latter withdraws when 20% of the side’s starting strength remains. Every unit carries this into a battle it enters. It applies at once, since no one has to carry it anywhere, but only to battles entered from then on — a battle already under way keeps what its forces brought.
- **For one unit, in the battle it is fighting now** — the **this battle** selector on its card in **War → Forces** (it only appears while the unit is fighting), or `keryx unit retreat-order`. Choose the same two modes. It overrides the realm setting for that battle only. On a field unit it is an order, and travels by [[runners|Runner]].

In Keryx, use `--hold-to-last-man` or `--when-losing` with either command. Fixed loss percentages are no longer a choice.
