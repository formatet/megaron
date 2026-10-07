Beyond your metropolis you may hold more settlements — up to **five** in all. <!-- src: server/internal/province/training.go MaxSettlementsPerWanax -->

## Founding a colony

Right-click the site, tick **Found a new settlement on arrival**, name it, choose the unit, and **March →**. Or left-click empty land and press **Colonize →**. You get the same [[catchment]] forecast as when you [[founding|founded]] your first city.

**The unit is consumed.** Those men become the colony's population. You cannot recall them and you cannot change your mind.

**They carry the colony's silver.** The city they leave pays the colonists' purse the moment they set out — the colony does not get silver from nowhere. Sent from a city with too little, they take what there is, and the order tells you how much short they are. A colony founded with nothing cannot pay its troops' [[upkeep]]. A purse that never reaches a new colony comes back to whichever of your cities the men walk into. <!-- src: server/internal/combat/march_start.go colonistPurse -->

You are notified when the colony is founded, with how its food looks.

## Colonies are not metropolises

There is no free starter Farm. If the ground cannot feed them, you must — by [[transfers|transfer]] or a [[routes|standing order]] — until they stand on their own. A colony on a rich mineral site with no farmland is a real strategy, but only if you have already built the grain route that keeps it alive.

Each colony without a temple, beyond a handful, makes your [[kharis]] decay faster.

## Loyalty

A colony's willingness to be yours is its [[loyalty]].

## Abandoning a colony

Open **War → Recruit → Abandon settlement**, choose the colony and press **Abandon**. Confirm only when you mean to give it up: this cannot be undone. In Keryx, use `abandon --settlement <settlement-id>`.

You can abandon only an active colony you own. **Your capital cannot be abandoned.** The colony leaves your realm, its garrison is disbanded, and troops aboard ships in that garrison are disbanded too. Its people do not move to your capital. The ground becomes open for founding again, and you regain a settlement slot.

After confirmation, the colony disappears from your owned settlements. Keryx names the colony you gave up; the web map and settlement list refresh. Abandoning is a peaceful withdrawal, not [[loyalty|collapse]].
