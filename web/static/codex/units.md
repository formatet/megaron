The **War (⚔)** drawer has three tabs: **Army**, **Recruit** and **Movements**.

## Recruiting

**War → Recruit.** Choose the city at the top if you hold several. Each row shows the unit's live cost, or is greyed out with the reason — a missing building, or not enough of something.

| Unit | Needs | Notes |
|---|---|---|
| **Spearman** | Barracks | The levy. 12 grain per hundred men; no silver. |
| **Elite Infantry** | Barracks + Foundry | Costs [[bronze]]. Far harder per man. |
| **War Chariot** | Stable | Costs bronze. The hardest hitter on the field. |
| **Galley** | Shipyard | 30 timber; no silver. Transport and sight at sea ([[sea]]). |
| **War Galley** | Shipyard + Foundry | Costs cedar. The warship. |
| **Emporos** | Shipyard | 16 cedar; no timber or silver. Trade hull. Does not fight. |

Land units train as a whole hundred-man cohort (**Train 100**). Ships are built one at a time and can be named. Their cost and population draft cover the ship's crew, rather than a hundred-man cohort. Rowers eat like spearmen: the actual surviving crew draws a port ration at home and double on a mission ([[upkeep]]). Rejected recruitment consumes no people or goods. You are notified when training finishes. A cohort still gathering men or training cannot march. Once ready, it can still march after losing men; it does not have to refill to 100 first.

Every cohort is numbered by its home city: **2nd Spearmen of Knossos**. A lost regiment's number is never given out again. Before you found your first city, your [[horde]]'s two escort cohorts carry your own name instead — **1st** and **2nd Spearmen of** *you* — and become the new city's 1st and 2nd when you settle.

## Recruits are your own people

A cohort is drafted out of your city's population and stays out of it until disbanded. Soldiers still eat. **Spearmen, Galleys and Emporoi draw 0 silver per day**. Elite infantry, chariots and War Galleys draw pay in [[silver]]. Home service costs half field pay: a full elite cohort draws 0.75 / 1.5 silver per day at home / in the field, and a full chariot cohort 2 / 4. A full War Galley draws 0.25 / 0.5 ([[upkeep]]). An army bigger than your city can feed and pay is not a strong army; it is a famine with spears.

In each combat round, **100 active spearmen roll 50 dice**. Half the active men roll, rounded up: one man rolls one die, three roll two. Elite infantry rolls three dice per active man and chariots five per chariot. A **War Galley rolls 5 dice**, against a **Galley's 1** before hull damage and other battle adjustments.

**Disband** selects a whole unit by name in City → Garrison. Its remaining people return to that city: a full Spearmen cohort returns 100 people; a damaged cohort returns only its survivors. There is no soldier-quantity input. Ships return their actual crew; a laid-up hull returns no crew a second time. Keryx: `disband --unit "<unit name>"` or `disband --unit <unit-id> --province <city>`; repeat `--unit` to select several whole units in that city. The receipt names each unit before its returned people and city: “2nd Spearmen of Phaistos disbanded — 41 people return to Phaistos.” Selecting several units gives one sentence per unit. Silver is unchanged.

An empty ship in your own active Harbour can be **Laid up** from its Army card. Its crew returns to the port's population and can work there; the hull draws no food or silver and remains in port. The card says **Laid up** and offers **Fit out**. Fitting out takes the ship's full crew from that city's current population and prepares it for **three days**. It cannot sail or be cancelled during that time, and its crew draws home-rate food and, for a War Galley, half field pay from the start. If the city is annexed, its laid-up and fitting-out ships go with the city. Disbanding a laid-up ship removes the hull without returning the same crew twice; choose the particular hull to disband by its name. Keryx: `unit lay-up --unit ID` and `unit fit-out --unit ID`.

## The Army tab

Every unit you own, with its state and current grain and silver upkeep per day in plain words. **March** is shown while it can move; a damaged land cohort garrisoned in its **original home city** can also **Reinforce** — another city you own does not qualify. A land unit on the march shows **Recall**. The card also shows stance and **Set**, battle retreat, **Redirect**, **Fetch by ship**, **Load** / **Unload** or **Repair**, whenever that unit can use them. Choose **Destination city** in a march or redirect form to target a known city quickly; map clicks remain available. Keryx accepts `--to "City name"` on march, post and redirect. See [[marching]], [[stances]] and [[sea]].

**Movements** lists your units on the march (**Outgoing**) and every foreign unit you can see marching on one of your settlements (**Incoming**), soonest arrival first. You only see an incoming column once one of your eyes has it in sight — the *foreign march sighted* notification fires at the same moment ([[defence]]).

Recruitment confirms that your order was received. While a unit is still training, use **Cancel training** in War → Army to return its people and reserved resources. A ship under construction has **Cancel build**. In Keryx use `unit cancel-training --unit ID`. Finished units use Disband instead.
