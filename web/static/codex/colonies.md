Beyond your metropolis you may hold more settlements — up to **five** in all. <!-- src: server/internal/province/training.go MaxSettlementsPerWanax -->

## Founding a colony {#colonies-founding-a-colony}

Right-click the site, select **Found a new settlement on arrival**, [[naming-the-land|name it]], choose the unit, and **March →**. Or left-click empty land and press **Colonize →**. You get the same [[catchment]] forecast as when you [[founding|founded]] your first city.

**The unit is consumed.** Those men become the colony's population — a cohort of 100 founds a colony of 100, and nobody else comes with them. You cannot recall them and you cannot change your mind. To make a colony bigger, march more men there and [[units|disband]] them: they settle as [[citizens|citizens]].

**They carry a colony purse.** The fixed purse comes from the city they leave: **30 [[silver|silver]] for [[spearmen|spearmen]], 60 for [[elite-infantry]], 90 for [[war-chariot|chariots]]**. It becomes the colony's starting silver, independently of daily pay. A colony's spearmen draw no daily silver; professionals and ships still need pay ([[upkeep]]). Send more with a [[transfers|transfer]] when it needs it. Sent from a city with too little, they take what there is, and the order tells you how much short they are. A purse that never reaches a new colony comes back to whichever of your cities the men walk into. <!-- src: server/internal/combat/march_start.go colonistPurse -->

### Reaching a new colony {#colonies-founding-a-colony-2}

You are notified when the colony is founded, with how its food looks.

## Colonies and metropolises {#colonies-colonies-are-not-metropolises}

There is no free starter [[farm]]. If the ground cannot feed them, you must — by [[transfers|transfer]] or a [[routes|standing order]] — until they stand on their own. A colony on a rich mineral site with no farmland is a real strategy, but only if you have already built the [[grain|grain]] route that keeps it alive.

Every active city, including your capital, counts toward [[kharis|upkeep of the cult]], even with a temple. Realm demand is active cities squared; daily cult upkeep is 1.2 times that demand. Population does not add to it. Sunk, collapsed and razed cities do not count. Check the realm breakdown in **Kult** or `keryx status` before expanding.

## Abandoning a colony {#colonies-abandoning-a-colony}

Open **War → Recruit → Abandon settlement**, choose the colony and press **Abandon**. The confirmation appears in the same row: press **Abandon** there to give it up, or **Keep settlement** to cancel. This cannot be undone. In Keryx, use `abandon --settlement <settlement-id>`.

You can abandon only an active colony you own. **Your capital cannot be abandoned.** The colony leaves your realm, its garrison is disbanded, and troops aboard ships in that garrison are disbanded too. Its people do not move to your capital. The ground becomes open for founding again, and you regain a settlement slot.

After confirmation, the colony disappears from your owned settlements. Keryx names the colony you gave up; the web map and settlement list refresh. Abandoning is a peaceful withdrawal.

Founding adds no grain to the colony's stores. Produce food or send it by [[transfers|internal transfer]]; the starting silver is only the carried purse.
