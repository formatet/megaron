The **War (⚔)** drawer has three tabs: **Army**, **Recruit** and **Movements**.

## Recruiting

**War → Recruit.** Choose the city at the top if you hold several. Each row shows the unit's live cost, or is greyed out with the reason — a missing building, or not enough of something.

| Unit | Needs | Notes |
|---|---|---|
| **Spearman** | Barracks | The levy. Cheap, fast. |
| **Elite Infantry** | Barracks + Foundry | Costs [[bronze]]. Far harder per man. |
| **War Chariot** | Stable | Costs bronze. The hardest hitter on the field. |
| **Galley** | Shipyard | Transport and sight at sea ([[sea]]). |
| **War Galley** | Shipyard + Foundry | Costs cedar. The warship. |
| **Emporos** | Shipyard | Trade hull. Does not fight. |

Land units train as a whole hundred-man cohort (**Train 100**). Ships are built one at a time and can be named. Their cost and population draft cover the ship's crew, rather than a hundred-man cohort. Rejected recruitment consumes no people or goods. You are notified when training finishes. A cohort still gathering men or training cannot march. Once ready, it can still march after losing men; it does not have to refill to 100 first.

Every cohort is numbered by its home city: **2nd Spearmen of Knossos**. A lost regiment's number is never given out again. Before you found your first city, your [[horde]]'s two escort cohorts carry your own name instead — **1st** and **2nd Spearmen of** *you* — and become the new city's 1st and 2nd when you settle.

## Recruits are your own people

A cohort is drafted out of your city's population and stays out of it until disbanded. Soldiers still eat. **Spearmen live on grain and draw 0 silver per day**; elite infantry, chariots and ships also draw pay in [[silver]] ([[upkeep]]). An army bigger than your city can feed and pay is not a strong army; it is a famine with spears.

**Disband** returns the men to your city.

An empty ship in your own active Harbour can be **Laid up** from its Army card. Its crew returns to the port's population and can work there; the hull draws no food or silver and remains in port. The card says **Laid up** and offers **Fit out**. Fitting out takes the ship's full crew from that city's current population and prepares it for **three days**. It cannot sail or be cancelled during that time, and its crew draws ordinary ship upkeep from the start. If the city is annexed, its laid-up and fitting-out ships go with the city. Disbanding a laid-up ship removes the hull without returning the same crew twice; a city's Disband command chooses laid-up hulls before crewed ships of the same type. Keryx: `unit lay-up --unit ID` and `unit fit-out --unit ID`.

## The Army tab

Every unit you own, with its state and current grain and silver upkeep per day in plain words. **March** is shown while it can move; a damaged land cohort garrisoned in its **original home city** can also **Reinforce** — another city you own does not qualify. A land unit on the march shows **Recall**. The card also shows stance and **Set**, battle retreat, **Redirect**, **Fetch by ship**, **Load** / **Unload** or **Repair**, whenever that unit can use them. See [[marching]], [[stances]] and [[sea]].

**Movements** lists your units on the march (**Outgoing**) and every foreign unit you can see marching on one of your settlements (**Incoming**), soonest arrival first. You only see an incoming column once one of your eyes has it in sight — the *foreign march sighted* notification fires at the same moment ([[defence]]).

Recruitment confirms that your order was received. While a unit is still training, use **Cancel training** in War → Army to return its people and reserved resources. A ship under construction has **Cancel build**. In Keryx use `unit cancel-training --unit ID`. Finished units use Disband instead.
