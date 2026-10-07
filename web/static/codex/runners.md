**Command is never instant.** This is the rule that makes Megaron different from most strategy games.

When you tell a unit in the field to turn around, change course or change stance, you do not reach the unit — you send a **Runner**, a real person who must walk to it. The game answers with something like *"Recall order sent by Runner — reaches the unit today 22:40."* Until then the unit keeps doing what it was last told.

The one exception is your [[horde|Nomadic Host]]: you travel with it in person, so it obeys at once.

## Why this is good for you

Because orders move at walking pace, **a defender who is offline is not helpless.** An attacker has to cross ground you can see, and your answer usually has less ground to cross than his army does. Distance is the defender's ally. See [[defence]].

## When an order fails

Sometimes a Runner arrives and the order can no longer be carried out. You get an *Order failed* notification that names the order and the reason. Look at the unit in **War → Army** and decide again.

Recall and redirect are sent from **War → Army**, or — for redirect — by right-clicking a new destination for a unit that is already marching. See [[marching]].

## Catching up with a moving unit

A unit on the march does not wait for its orders. A Runner sent to it — to recall it, redirect it or change its [[stances|stance]] — is aimed at the point on its road where he can **catch up** with it, and the game tells you when he will. Runners are quicker than soldiers, but not endlessly: a recall or redirect that no Runner can deliver before the march ends is refused at once, since there would be nothing left to turn. A stance order still goes — the Runner follows the unit to where it stops.

Letters to other Wanaxes travel the same way, but they are sacred and cannot be stopped: see [[messengers]].

## Crossing the sea

A Runner whose road to your unit crosses open water cannot swim it — it waits in your own port for one of your ships to sail that way, the same as a letter does. See [[sea]] for how the crossing works, and what happens if the ship carrying it is lost.

## Recalling several units

In **War → Army → Recall units**, send all matching units home. Optionally filter
by their current destination (`q,r`) or march intent. Keryx offers the same selection:
`keryx recall --all`, `keryx recall --all --target 5,-3`, or
`keryx recall --all --intent explore`. Use `--unit <id>` for one unit.

A group shares one Runner only when its saved roads put every recipient at the
same meeting hex on the exact same game day. The Runner can arrive early and wait
there. Other recipients get their own Runners; sharing a destination alone does
not make a group. Each unit receives its own order delivery and turns home only
when reached. The reply lists groups, singles and every unit's result, including
named refusals. A unit with a recall or redirect already on its way is refused;
wait for delivery before giving it another such order. An order that later misses
its recipient produces an *Order failed* notice.
